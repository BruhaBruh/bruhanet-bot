package tgtest

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"sync"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

type RemnawaveUpdate struct {
	TelegramID int64
	ExpireAt   time.Time
}

// DeviceLimitWrite — вызов SetDeviceLimit.
type DeviceLimitWrite struct {
	TelegramID int64
	Limit      int
}

// Remnawave — фейк панели для тестов хендлеров.
type Remnawave struct {
	mu         sync.Mutex
	now        func() time.Time
	users      map[int64]api.UserItemInfo
	updates    []RemnawaveUpdate
	limits     []DeviceLimitWrite
	err        error
	updErr     error
	limErr     error
	limFailErr error
	limFails   int // сколько ближайших вызовов SetDeviceLimit упадёт с limFailErr, потом успех
	failAfter  int
	failErr    error
	devices    map[int][]api.Device // подключённые устройства по числовому id юзера панели
	devErr     error
	cleared    []int // userID, у которых вызвали DeleteAllUserDevices
}

func NewRemnawave(now func() time.Time) *Remnawave {
	return &Remnawave{now: now, users: make(map[int64]api.UserItemInfo)}
}

func (r *Remnawave) AddUser(telegramID int64, expireAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[telegramID] = newUser(telegramID, expireAt)
}

// AddUserAs добавляет юзера панели под ключом key с заданным Telegram ID.
func (r *Remnawave) AddUserAs(key, telegramID int64, expireAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[key] = newUser(telegramID, expireAt)
}

// AddUserWithoutTelegramID добавляет юзера панели без привязки к Telegram.
func (r *Remnawave) AddUserWithoutTelegramID(key int64, expireAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := newUser(key, expireAt)
	u.TelegramId = api.NilInt{Null: true}
	r.users[key] = u
}

// FailUsersAfter: Users отдаёт n юзеров, затем err; nil — снять.
func (r *Remnawave) FailUsersAfter(n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failAfter, r.failErr = n, err
}

// Users отдаёт юзеров по возрастанию ключа.
func (r *Remnawave) Users(_ context.Context) iter.Seq2[api.UserItemInfo, error] {
	r.mu.Lock()
	keys := make([]int64, 0, len(r.users))
	for k := range r.users {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	list := make([]api.UserItemInfo, len(keys))
	for i, k := range keys {
		list[i] = r.users[k]
	}
	failAfter, failErr := r.failAfter, r.failErr
	r.mu.Unlock()
	return func(yield func(api.UserItemInfo, error) bool) {
		for i, u := range list {
			if failErr != nil && i == failAfter {
				yield(api.UserItemInfo{}, failErr)
				return
			}
			if !yield(u, nil) {
				return
			}
		}
		if failErr != nil && failAfter >= len(list) {
			yield(api.UserItemInfo{}, failErr)
		}
	}
}

// FailWith заставляет все методы возвращать err; nil — снять.
func (r *Remnawave) FailWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// FailUpdateWith заставляет возвращать err только UpdateUser; nil — снять.
func (r *Remnawave) FailUpdateWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updErr = err
}

// FailLimitTimes: ближайшие n вызовов SetDeviceLimit возвращают err, дальше всё проходит (временный сбой панели).
func (r *Remnawave) FailLimitTimes(n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limFails, r.limFailErr = n, err
}

// FailLimitWith заставляет возвращать err только SetDeviceLimit; nil — снять.
func (r *Remnawave) FailLimitWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limErr = err
}

func (r *Remnawave) ExpireAt(telegramID int64) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.users[telegramID].ExpireAt
}

func (r *Remnawave) Updates() []RemnawaveUpdate {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RemnawaveUpdate(nil), r.updates...)
}

// DeviceLimitWrites — все вызовы SetDeviceLimit по порядку.
func (r *Remnawave) DeviceLimitWrites() []DeviceLimitWrite {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]DeviceLimitWrite(nil), r.limits...)
}

// SetDeviceLimit запоминает лимит в юзере панели; сбой UpdateUser (FailUpdateWith) действует и тут.
func (r *Remnawave) SetDeviceLimit(_ context.Context, telegramID int64, limit int) (*api.UserItemInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	if r.updErr != nil {
		return nil, r.updErr
	}
	if r.limErr != nil {
		return nil, r.limErr
	}
	if r.limFails > 0 {
		r.limFails--
		return nil, r.limFailErr
	}
	u, ok := r.users[telegramID]
	if !ok {
		return nil, remnawave.ErrUserNotFound
	}
	u.HwidDeviceLimit = api.NewNilInt(limit)
	r.users[telegramID] = u
	r.limits = append(r.limits, DeviceLimitWrite{TelegramID: telegramID, Limit: limit})
	return &u, nil
}

func (r *Remnawave) GetUser(_ context.Context, telegramID int64) (*api.UserItemInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	u, ok := r.users[telegramID]
	if !ok {
		return nil, remnawave.ErrUserNotFound
	}
	return &u, nil
}

func (r *Remnawave) CreateUser(_ context.Context, telegramID int64, _ string, _ uuid.UUID) (*api.UserItemInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	u, ok := r.users[telegramID]
	if !ok {
		u = newUser(telegramID, r.now())
		r.users[telegramID] = u
	}
	return &u, nil
}

func (r *Remnawave) UpdateUser(_ context.Context, telegramID int64, _ string, expireAt time.Time, _ uuid.UUID) (*api.UserItemInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	if r.updErr != nil {
		return nil, r.updErr
	}
	u, ok := r.users[telegramID]
	if !ok {
		return nil, remnawave.ErrUserNotFound
	}
	u.ExpireAt = expireAt
	r.users[telegramID] = u
	r.updates = append(r.updates, RemnawaveUpdate{TelegramID: telegramID, ExpireAt: expireAt})
	return &u, nil
}

// AddUserCreatedAt добавляет юзера с заданными сроком и временем создания в панели.
func (r *Remnawave) AddUserCreatedAt(telegramID int64, expireAt, createdAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u := newUser(telegramID, expireAt)
	u.CreatedAt = createdAt
	r.users[telegramID] = u
}

// SetDevices кладёт юзеру n подключённых устройств (числовой id юзера панели в фейке равен Telegram ID).
func (r *Remnawave) SetDevices(telegramID int64, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.devices == nil {
		r.devices = map[int][]api.Device{}
	}
	list := make([]api.Device, n)
	for i := range list {
		list[i] = api.Device{Hwid: fmt.Sprintf("hwid-%d-%d", telegramID, i)}
	}
	r.devices[int(telegramID)] = list
}

// FailDevicesWith: UserDevices и DeleteAllUserDevices возвращают err; nil — снять.
func (r *Remnawave) FailDevicesWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.devErr = err
}

// ClearedDevices — userID, у которых очистили устройства, по порядку.
func (r *Remnawave) ClearedDevices() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.cleared...)
}

func (r *Remnawave) UserDevices(_ context.Context, userID int) ([]api.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.devErr != nil {
		return nil, r.devErr
	}
	return append([]api.Device(nil), r.devices[userID]...), nil
}

func (r *Remnawave) DeleteAllUserDevices(_ context.Context, userID int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.devErr != nil {
		return r.devErr
	}
	delete(r.devices, userID)
	r.cleared = append(r.cleared, userID)
	return nil
}

// DeviceLimit — лимит устройств юзера в панели; 0 — не записывался.
func (r *Remnawave) DeviceLimit(telegramID int64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.users[telegramID].HwidDeviceLimit.Value
}

func newUser(telegramID int64, expireAt time.Time) api.UserItemInfo {
	return api.UserItemInfo{
		ID:              int(telegramID),
		CreatedAt:       expireAt, // в тестах юзер заведён в момент срока: ни одного выданного вне бота дня
		Status:          api.UserItemInfoStatusACTIVE,
		TelegramId:      api.NewNilInt(int(telegramID)),
		ExpireAt:        expireAt,
		SubscriptionUrl: fmt.Sprintf("https://sub.example/%d", telegramID),
	}
}
