package tgtest

import (
	"context"
	"fmt"
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

// Remnawave — фейк панели для тестов хендлеров.
type Remnawave struct {
	mu      sync.Mutex
	now     func() time.Time
	users   map[int64]api.UserItemInfo
	updates []RemnawaveUpdate
	err     error
	updErr  error
}

func NewRemnawave(now func() time.Time) *Remnawave {
	return &Remnawave{now: now, users: make(map[int64]api.UserItemInfo)}
}

func (r *Remnawave) AddUser(telegramID int64, expireAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[telegramID] = newUser(telegramID, expireAt)
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

func newUser(telegramID int64, expireAt time.Time) api.UserItemInfo {
	return api.UserItemInfo{
		Status:          api.UserItemInfoStatusACTIVE,
		ExpireAt:        expireAt,
		SubscriptionUrl: fmt.Sprintf("https://sub.example/%d", telegramID),
	}
}
