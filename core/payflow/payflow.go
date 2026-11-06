// Package payflow — решение по платежу, одно на бота и панель: даты, запись в панель и уведомления.
package payflow

import (
	"context"
	"errors"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
)

// Month — месяц подписки.
const Month = 30 * 24 * time.Hour

// События outbox.
const (
	EventResult      = "payment_result"
	EventAdminClosed = "payment_admin_closed"
)

// События подарков: результат платежа покупателю (ссылка или отказ) и активация (активировавшему и покупателю).
const (
	EventGiftResult   = "gift_result"
	EventGiftRedeemed = "gift_redeemed"
)

// ErrPanel — панель не приняла новый срок; транзакция откатана, повтор считает ту же дату.
var ErrPanel = errors.New("payflow: panel update failed")

type Panel interface {
	UpdateUser(ctx context.Context, telegramID int64, username string, expireAt time.Time, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	// SetDeviceLimit пишет только лимит устройств; срок и остальное не трогает.
	SetDeviceLimit(ctx context.Context, telegramID int64, limit int) (*api.UserItemInfo, error)
}

// Store — core/store: ошибки store.ErrNotFound и store.ErrNotPending проходят как есть.
type Store interface {
	ResolvePaymentRef(ctx context.Context, id int64, status model.PaymentStatus, at time.Time, referralDays int,
		apply func(model.Payment) ([]model.OutboxMessage, error)) (model.Payment, error)
}

// Decision — что решил админ. AdminID не ноль — решение из панели: в той же транзакции кладётся
// EventAdminClosed, воркер закрывает им сообщение админа в боте (в боте его закрывает сам хендлер).
type Decision struct {
	ID      int64
	Approve bool
	At      time.Time
	Squad   uuid.UUID
	AdminID int64
	// DevicesBase — устройств в базе (settings.DevicesBase): лимит в панели = база + доп. устройства.
	DevicesBase int
	// ReferralDays — дни рефералки (settings.ReferralDays); 0 — рефералка выключена. Бонус другу складывается с этим же
	// подтверждением: одна запись срока в панель. Пригласивший получает свои дни отдельно (PayReferrer, если p.ReferralPending).
	ReferralDays int
}

// ExtendedExpire — срок после подтверждения: от прежнего срока или от at, если он в прошлом.
func ExtendedExpire(prev, at time.Time, months int) time.Time {
	if at.After(prev) {
		prev = at
	}
	return prev.Add(time.Duration(months) * Month)
}

// ErrDevicesBase — не задана база устройств, лимит в панель не пишем.
var ErrDevicesBase = errors.New("payflow: devices base is not set")

// ApplyDeviceLimit пишет лимит устройств в панель, если подтверждение платежа меняет доп. устройства юзера
// (p.UserExtra — текущие под блокировкой, store.ResolvePayment): докупка — всегда, подписка — только при изменении числа.
// Отклонение и неизменное число панель не трогают. Тот же вызов нужен любому месту, которое подтверждает платёж само.
func ApplyDeviceLimit(ctx context.Context, panel Panel, p model.Payment, approve bool, base int) error {
	next := p.ExtraAfter(p.UserExtra)
	// Первое подтверждение подписки после пробного пишет лимит всегда: в панели стоит пробный, а не base+extra.
	convert := p.TrialConvert && p.Kind != model.KindDevices
	if !approve || (next == p.UserExtra && !convert) {
		return nil
	}
	if base < 1 {
		return ErrDevicesBase
	}
	_, err := panel.SetDeviceLimit(ctx, p.TelegramID, base+next)
	return err
}

// Resolve под блокировкой строки платежа записывает в панель абсолютную дату (подтверждение — ExtendedExpire,
// отклонение — возврат PrevExpireAt) и кладёт уведомления в outbox. Текст и клавиатуру юзеру рендерит воркер при доставке.
// Сбой после UpdateUser (ApplyDeviceLimit, коммит) безопасен без отката срока: запись абсолютная от PrevExpireAt,
// повтор админа считает ту же дату, а не продлевает продлённое (в отличие от payByPromo, где повтор — новый платёж
// с PrevExpireAt из панели; там срок возвращается RestoreTerm).
// Платёж за устройства срок не трогает вовсе: подтверждение пишет только лимит устройств, отклонение — ничего.
func Resolve(ctx context.Context, st Store, panel Panel, d Decision) (model.Payment, error) {
	status, key := model.PaymentDeclined, "payment.declined"
	if d.Approve {
		status, key = model.PaymentConfirmed, "payment.confirmed"
	}
	return st.ResolvePaymentRef(ctx, d.ID, status, d.At, d.ReferralDays, func(p model.Payment) ([]model.OutboxMessage, error) {
		var msgs []model.OutboxMessage
		if p.Kind == model.KindDevices {
			if err := ApplyDeviceLimit(ctx, panel, p, d.Approve, d.DevicesBase); err != nil {
				return nil, errors.Join(ErrPanel, err)
			}
			key = "payment.devices_declined"
			if d.Approve {
				key = "payment.devices_confirmed"
			}
			msgs = []model.OutboxMessage{{
				EventType: EventResult, ChatID: p.TelegramID, TextKey: key, Devices: p.ExtraDevices, ParseMode: "HTML",
			}}
			return withAdminClosed(msgs, p, d), nil
		}
		if p.Kind == model.KindGift {
			// Подарок срок, устройства и панель покупателя не трогает: подтверждение открывает ссылку, отклонение гасит.
			msg := model.OutboxMessage{EventType: EventGiftResult, ChatID: p.TelegramID, TextKey: "gift.declined", Months: p.Month, ParseMode: "HTML"}
			if d.Approve {
				msg.TextKey, msg.GiftCode = "gift.ready", p.GiftCode
			}
			return withAdminClosed([]model.OutboxMessage{msg}, p, d), nil
		}
		expireAt := p.PrevExpireAt
		if d.Approve {
			expireAt = ExtendedExpire(p.PrevExpireAt, d.At, p.Month).Add(time.Duration(p.ReferralFriendDays) * 24 * time.Hour)
		}
		if _, err := panel.UpdateUser(ctx, p.TelegramID, p.Username, expireAt, d.Squad); err != nil {
			return nil, errors.Join(ErrPanel, err)
		}
		if err := ApplyDeviceLimit(ctx, panel, p, d.Approve, d.DevicesBase); err != nil {
			return nil, errors.Join(ErrPanel, err)
		}
		msgs = []model.OutboxMessage{{EventType: EventResult, ChatID: p.TelegramID, TextKey: key, ExpireAt: expireAt, ParseMode: "HTML"}}
		if p.ReferralFriendDays > 0 {
			msgs = append(msgs, model.OutboxMessage{
				EventType: EventReferralFriend, ChatID: p.TelegramID, TextKey: "referral.friend_bonus",
				Days: p.ReferralFriendDays, ExpireAt: expireAt, ParseMode: "HTML",
			})
		}
		return withAdminClosed(msgs, p, d), nil
	})
}

func withAdminClosed(msgs []model.OutboxMessage, p model.Payment, d Decision) []model.OutboxMessage {
	if d.AdminID != 0 {
		msgs = append(msgs, model.OutboxMessage{
			EventType: EventAdminClosed, ChatID: d.AdminID, MessageID: p.AdminMessageID,
			Confirmed: d.Approve, Months: p.Month, Price: p.Price, PayKind: string(p.Kind),
		})
	}
	return msgs
}

// EventGrant — уведомление юзеру о днях, выданных из панели.
const EventGrant = "grant_days"

// ErrNotRestored — после сбоя коммита прежний срок в панели вернуть не удалось: расхождение, дочинит синхронизация.
var ErrNotRestored = errors.New("payflow: panel expire not restored")

// ErrGrantDays — дни вне 1..MaxGrantDays.
var ErrGrantDays = errors.New("payflow: grant days out of range")

// MaxGrantDays — потолок одной выдачи.
const MaxGrantDays = 365

// GrantPanel — панель для выдачи дней: создать юзера, прочитать и записать срок.
type GrantPanel interface {
	Panel
	CreateUser(ctx context.Context, telegramID int64, username string, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
}

// GrantStore — core/store: ErrNotFound (нет юзера) проходит как есть.
type GrantStore interface {
	GrantDays(ctx context.Context, telegramID, adminID int64, days int, now time.Time,
		apply func() ([]model.OutboxMessage, error)) error
}

// GrantRequest — выдача дней админом.
type GrantRequest struct {
	TelegramID int64
	Username   string
	Days       int
	AdminID    int64
	At         time.Time
	Squad      uuid.UUID
}

// GrantDays продлевает юзера на Days дней от later(срок в панели, At) — абсолютной датой — и возвращает её.
// Всё под транзакцией store.GrantDays (сдвиг pending-платежа, строка grants, уведомление в outbox); ошибка панели
// откатывает транзакцию (ErrPanel). Если панель уже продлена, а коммит не прошёл, прежний срок возвращается —
// только пока срок в панели всё ещё наш (иначе его успел поменять админ — не затираем).
func GrantDays(ctx context.Context, st GrantStore, panel GrantPanel, r GrantRequest) (time.Time, error) {
	if r.Days < 1 || r.Days > MaxGrantDays {
		return time.Time{}, ErrGrantDays
	}
	var g termGrant
	err := st.GrantDays(ctx, r.TelegramID, r.AdminID, r.Days, r.At, func() ([]model.OutboxMessage, error) {
		return g.apply(ctx, panel, r, EventGrant, "grant.days")
	})
	return g.newExpire, g.restore(ctx, panel, r, err)
}

// termGrant — продление срока на дни в панели внутри транзакции выдачи и его откат при сбое коммита.
type termGrant struct{ oldExpire, newExpire time.Time }

func (g *termGrant) apply(ctx context.Context, panel GrantPanel, r GrantRequest, event, key string) ([]model.OutboxMessage, error) {
	u, err := panel.CreateUser(ctx, r.TelegramID, r.Username, r.Squad)
	if err != nil {
		return nil, errors.Join(ErrPanel, err)
	}
	next := u.ExpireAt
	if r.At.After(next) {
		next = r.At
	}
	next = next.Add(time.Duration(r.Days) * 24 * time.Hour)
	if _, err = panel.UpdateUser(ctx, r.TelegramID, r.Username, next, r.Squad); err != nil {
		return nil, errors.Join(ErrPanel, err)
	}
	g.oldExpire, g.newExpire = u.ExpireAt, next
	return []model.OutboxMessage{{
		EventType: event, ChatID: r.TelegramID, TextKey: key, Days: r.Days, ExpireAt: next, ParseMode: "HTML",
	}}, nil
}

// restore после сбоя транзакции возвращает прежний срок, если панель уже продлена и срок в ней всё ещё наш.
func (g *termGrant) restore(ctx context.Context, panel GrantPanel, r GrantRequest, err error) error {
	if err == nil || g.newExpire.IsZero() {
		return err
	}
	return errors.Join(err, RestoreTerm(ctx, panel, r.TelegramID, r.Username, g.newExpire, g.oldExpire, r.Squad))
}

// TermPanel — чтение и запись срока.
type TermPanel interface {
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
	UpdateUser(ctx context.Context, telegramID int64, username string, expireAt time.Time, squadUUID uuid.UUID) (*api.UserItemInfo, error)
}

// RestoreTerm возвращает срок old, если в панели всё ещё written (иначе срок успели поменять — не затираем).
// Нужна после сбоя транзакции, когда срок уже записан: иначе повтор прочтёт продлённый срок как прежний.
// Ошибка — ErrNotRestored (расхождение, дочинит синхронизация).
func RestoreTerm(ctx context.Context, panel TermPanel, telegramID int64, username string, written, old time.Time, squad uuid.UUID) error {
	// ponytail: GetUser→UpdateUser не атомарен; окно — миллисекунды после редкого сбоя коммита.
	cctx := context.WithoutCancel(ctx)
	cur, err := panel.GetUser(cctx, telegramID)
	if err != nil {
		return errors.Join(ErrNotRestored, err)
	}
	if !cur.ExpireAt.Equal(written) {
		return nil
	}
	if _, err = panel.UpdateUser(cctx, telegramID, username, old, squad); err != nil {
		return errors.Join(ErrNotRestored, err)
	}
	return nil
}

// Уведомления рефералки: другу (в подтверждении первой оплаты) и пригласившему (отдельной транзакцией позже).
const (
	EventReferralFriend   = "referral_friend"
	EventReferralReferrer = "referral_referrer"
)

// ReferralStore — core/store: ошибки ErrNotFound, ErrBlocked, ErrAlreadyRewarded проходят как есть.
type ReferralStore interface {
	GrantReferral(ctx context.Context, referredID int64, now time.Time,
		apply func(referrerID int64, username string, days int) ([]model.OutboxMessage, error)) error
}

// PayReferrer выдаёт пригласившему дни за друга referredID: абсолютная дата от later(срок в панели, at), панель внутри
// транзакции store.GrantReferral (под замком срока пригласившего), уведомление в outbox. Идемпотентен: если дни уже
// выданы — store.ErrAlreadyRewarded (не сбой); заблокированному — store.ErrBlocked (награда остаётся до снятия блокировки).
// Ошибка панели откатывает транзакцию (ErrPanel), повтор считает ту же дату; после сбоя коммита срок откатывается как у GrantDays.
func PayReferrer(ctx context.Context, st ReferralStore, panel GrantPanel, referredID int64, at time.Time, squad uuid.UUID) (time.Time, error) {
	var g termGrant
	var r GrantRequest
	err := st.GrantReferral(ctx, referredID, at, func(referrerID int64, username string, days int) ([]model.OutboxMessage, error) {
		r = GrantRequest{TelegramID: referrerID, Username: username, Days: days, At: at, Squad: squad}
		return g.apply(ctx, panel, r, EventReferralReferrer, "referral.referrer_bonus")
	})
	return g.newExpire, g.restore(ctx, panel, r, err)
}

// EventTrial — уведомление юзеру о выданном пробном периоде.
const EventTrial = "trial_started"

// TrialSlack — насколько срок только что созданного панельного юзера может опережать его создание: больше —
// срок выдан не этим ботом (старый платящий клиент вне нашей БД), пробный не положен.
const TrialSlack = time.Hour

// TrialPanelOK — срок юзера в панели не мешает пробному: он не действует и не выдан вне бота (дальше создания больше
// чем на TrialSlack). Юзера в панели нет — не мешает.
func TrialPanelOK(u *api.UserItemInfo, at time.Time) bool {
	return u == nil || (!u.ExpireAt.After(at) && !u.ExpireAt.After(u.CreatedAt.Add(TrialSlack)))
}

// TrialPanel — панель для пробного периода: прочитать, создать, продлить, записать лимит устройств.
type TrialPanel interface {
	GrantPanel
}

// TrialStore — core/store: StartTrial.
type TrialStore interface {
	StartTrial(ctx context.Context, telegramID int64, username string, days, advanceLimit int, now time.Time,
		apply func() ([]model.OutboxMessage, error)) error
}

// TrialRequest — выдача пробного периода. NotFound определяет «нет юзера в панели» в ошибке GetUser (core не знает pkg).
type TrialRequest struct {
	TelegramID   int64
	Username     string
	Days         int
	Devices      int // лимит устройств на пробном
	BaseDevices  int // база: к ней возвращается лимит при откате
	AdvanceLimit int
	At           time.Time
	Squad        uuid.UUID
	NotFound     func(error) bool
}

// StartTrial выдаёт пробный: абсолютный срок At+Days суток и лимит Devices в панели внутри транзакции store.StartTrial.
// Юзер, уже имеющий в панели оплаченный срок (срок в будущем или заметно дальше создания), пробного не получает.
// Ошибка панели откатывает транзакцию (ErrPanel); записанное в панель до сбоя (срок, лимит) возвращается назад,
// пока оно ещё наше. Возвращает новый срок.
func StartTrial(ctx context.Context, st TrialStore, panel TrialPanel, r TrialRequest) (time.Time, error) {
	if r.Days < 1 || r.Devices < 1 {
		return time.Time{}, ErrGrantDays
	}
	var oldExpire, newExpire time.Time
	var oldLimit int
	limitWritten := false
	err := st.StartTrial(ctx, r.TelegramID, r.Username, r.Days, r.AdvanceLimit, r.At, func() ([]model.OutboxMessage, error) {
		u, err := panel.GetUser(ctx, r.TelegramID)
		switch {
		case err == nil:
			if !TrialPanelOK(u, r.At) {
				return nil, model.ErrTrialUnavailable
			}
		case r.NotFound != nil && r.NotFound(err):
			if u, err = panel.CreateUser(ctx, r.TelegramID, r.Username, r.Squad); err != nil {
				return nil, errors.Join(ErrPanel, err)
			}
		default:
			return nil, errors.Join(ErrPanel, err)
		}
		oldLimit = r.BaseDevices
		if !u.HwidDeviceLimit.Null && u.HwidDeviceLimit.Value > 0 {
			oldLimit = u.HwidDeviceLimit.Value
		}
		next := r.At.Add(time.Duration(r.Days) * 24 * time.Hour)
		oldExpire, newExpire = u.ExpireAt, next
		if _, err = panel.UpdateUser(ctx, r.TelegramID, r.Username, next, r.Squad); err != nil {
			newExpire = time.Time{}
			return nil, errors.Join(ErrPanel, err)
		}
		limitWritten = true // писали или пытались: откат вернёт прежний лимит
		if _, err = panel.SetDeviceLimit(ctx, r.TelegramID, r.Devices); err != nil {
			return nil, errors.Join(ErrPanel, err)
		}
		return []model.OutboxMessage{{
			EventType: EventTrial, ChatID: r.TelegramID, TextKey: "trial.started", Days: r.Days, Devices: r.Devices,
			ExpireAt: next, ParseMode: "HTML",
		}}, nil
	})
	if err != nil && !newExpire.IsZero() {
		// ponytail: GetUser→UpdateUser не атомарен; окно — миллисекунды после редкого сбоя.
		cctx := context.WithoutCancel(ctx)
		cur, gerr := panel.GetUser(cctx, r.TelegramID)
		switch {
		case gerr != nil:
			err = errors.Join(err, ErrNotRestored, gerr)
		case cur.ExpireAt.Equal(newExpire):
			if _, uerr := panel.UpdateUser(cctx, r.TelegramID, r.Username, oldExpire, r.Squad); uerr != nil {
				err = errors.Join(err, ErrNotRestored, uerr)
			}
			if limitWritten {
				if _, lerr := panel.SetDeviceLimit(cctx, r.TelegramID, oldLimit); lerr != nil {
					err = errors.Join(err, ErrNotRestored, lerr)
				}
			}
		}
	}
	return newExpire, err
}

// GiftStore — core/store: ErrGiftUnavailable (любая причина отказа) проходит как есть.
type GiftStore interface {
	RedeemGift(ctx context.Context, code string, redeemerID int64, now time.Time,
		apply func(g model.Gift) ([]model.OutboxMessage, error)) (model.Gift, error)
}

// GiftRedeem — активация подарка юзером. Prepare — необязательный шаг перед продлением внутри транзакции (бот заводит
// нового юзера в панели с лимитом устройств); его сбой откатывает активацию.
type GiftRedeem struct {
	Code       string
	TelegramID int64
	Username   string
	At         time.Time
	Squad      uuid.UUID
	Prepare    func(ctx context.Context) error
}

// RedeemGift продлевает юзера на месяцы подарка (месяц = 30 суток) абсолютной датой от later(срок в панели, At) под
// транзакцией store.RedeemGift: ошибка панели откатывает её (ErrPanel, подарок остаётся ready, повтор считает ту же дату);
// сбой коммита после записи в панель возвращает прежний срок, пока он ещё наш (как у GrantDays). Уведомления: активировавшему
// и, если покупатель известен и это не он сам, покупателю (без имени активировавшего). Возвращает подарок и новый срок.
// Отказ по любой причине — model.ErrGiftUnavailable.
func RedeemGift(ctx context.Context, st GiftStore, panel GrantPanel, r GiftRedeem) (model.Gift, time.Time, error) {
	var g termGrant
	var req GrantRequest
	gift, err := st.RedeemGift(ctx, r.Code, r.TelegramID, r.At, func(gf model.Gift) ([]model.OutboxMessage, error) {
		if r.Prepare != nil {
			if err := r.Prepare(ctx); err != nil {
				return nil, errors.Join(ErrPanel, err)
			}
		}
		req = GrantRequest{TelegramID: r.TelegramID, Username: r.Username, Days: gf.Months * 30, At: r.At, Squad: r.Squad}
		msgs, err := g.apply(ctx, panel, req, EventGiftRedeemed, "gift.redeemed")
		if err != nil {
			return nil, err
		}
		msgs[0].Months = gf.Months
		if gf.BuyerID != nil && *gf.BuyerID != r.TelegramID {
			msgs = append(msgs, model.OutboxMessage{
				EventType: EventGiftRedeemed, ChatID: *gf.BuyerID, TextKey: "gift.redeemed_buyer", Months: gf.Months, ParseMode: "HTML",
			})
		}
		return msgs, nil
	})
	return gift, g.newExpire, g.restore(ctx, panel, req, err)
}
