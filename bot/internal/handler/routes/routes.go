package routes

import (
	"context"
	"iter"
	"sync"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/router"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/pkg/callback"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

const (
	routeMenu = "menu"
	routeInfo = "info"
	routePay  = "pay"
)

// Аргумент админ-кнопок — id платежа.
var (
	confirmRoute = callback.New[int64]("adm_ok")
	declineRoute = callback.New[int64]("adm_no")
)

type Remnawave interface {
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
	CreateUser(ctx context.Context, telegramID int64, username string, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	UpdateUser(ctx context.Context, telegramID int64, username string, expireAt time.Time, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	// SetDeviceLimit пишет лимит устройств (база + купленные) — только при событии с устройствами.
	SetDeviceLimit(ctx context.Context, telegramID int64, limit int) (*api.UserItemInfo, error)
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
}

// Payments — хранилище платежей (store.Store); ошибки — store.ErrNotFound / store.ErrNotPending.
type Payments interface {
	CreatePayment(ctx context.Context, p model.Payment, rollback func(old model.Payment) error, readPrev func(ctx context.Context) (time.Time, error)) (model.Payment, *model.Payment, error)
	PendingPayment(ctx context.Context, telegramID int64) (model.Payment, error)
	PaymentByID(ctx context.Context, id int64) (model.Payment, error)
	SubmitPayment(ctx context.Context, id int64, adminMessageID int, photoFileID string, at time.Time) error
	GrantAdvance(ctx context.Context, id int64, apply func(model.Payment)) error
	UserStats(ctx context.Context, telegramID, excludeID int64) (model.UserStats, error)
	ResolvePayment(ctx context.Context, id int64, status model.PaymentStatus, at time.Time,
		apply func(model.Payment) ([]model.OutboxMessage, error)) (model.Payment, error)
	// ResolvePaymentRef — ResolvePayment с рефералкой (payflow.Store); payByPromo зовёт ResolvePayment без неё.
	ResolvePaymentRef(ctx context.Context, id int64, status model.PaymentStatus, at time.Time, referralDays int,
		apply func(model.Payment) ([]model.OutboxMessage, error)) (model.Payment, error)
	CancelPayment(ctx context.Context, id int64) error
	// Подарки: активация (payflow.GiftStore).
	RedeemGift(ctx context.Context, code string, redeemerID int64, now time.Time,
		apply func(g model.Gift) ([]model.OutboxMessage, error)) (model.Gift, error)
	// Рефералка: пригласивший пишется один раз при первом создании юзера; дни пригласившему — payflow.ReferralStore.
	RecordReferral(ctx context.Context, u model.UserSeen, referrerID int64) (bool, error)
	GrantReferral(ctx context.Context, referredID int64, now time.Time,
		apply func(referrerID int64, username string, days int) ([]model.OutboxMessage, error)) error
	CreateLoginToken(ctx context.Context, hash []byte, expiresAt time.Time) error
	TouchUser(ctx context.Context, u model.UserSeen) error
	SetUserPanel(ctx context.Context, id int64, expireAt time.Time, status string, at time.Time) error
	// Промокоды; PromoByCode нормализует ввод (trim + upper), нет кода — store.ErrNotFound.
	PromoByCode(ctx context.Context, code string) (model.Promo, error)
	PromoByID(ctx context.Context, id int64) (model.Promo, error)
	PromoUsage(ctx context.Context, promoID, telegramID int64) (used int, alreadyActivated bool, err error)
	HasConfirmed(ctx context.Context, telegramID int64) (bool, error)
	// UserBlocked — ручная блокировка админом; для раннего отказа, окончательно решает транзакция выдачи.
	UserBlocked(ctx context.Context, telegramID int64) (bool, error)
	// ExtraDevices — купленные юзером устройства сверх базы.
	ExtraDevices(ctx context.Context, telegramID int64) (int, error)
	ActivateDaysPromo(ctx context.Context, promoID, telegramID int64, now time.Time, apply func(p model.Promo) error) error
	// Пробный период: TrialAllowed — дешёвая проверка для меню, StartTrial — выдача в транзакции (payflow.TrialStore).
	TrialAllowed(ctx context.Context, telegramID int64, advanceLimit int) (bool, error)
	StartTrial(ctx context.Context, telegramID int64, username string, days, advanceLimit int, now time.Time,
		apply func() ([]model.OutboxMessage, error)) error
}

type Deps struct {
	Config    *config.Config
	FSM       fsm.Storage
	Remnawave Remnawave
	Payments  Payments
	// Content — тексты юзеру и настройки (тарифы, реквизиты, поддержка); cfg.Payment/cfg.Support тут не читаются.
	Content *content.Provider
	// Now — источник времени; nil = time.Now.
	Now func() time.Time
	// Fails — счётчик неудач для защиты от перебора (коды подарка); nil — без лимита.
	Fails FailCounter
}

// FailCounter — неудачные попытки по ключу в окне: Fail записывает (ключ живёт window с первой), Fails читает.
type FailCounter interface {
	Fails(ctx context.Context, key string) (int, error)
	Fail(ctx context.Context, key string, window time.Duration) error
}

type handlers struct {
	cfg *config.Config
	rw  Remnawave
	pay Payments
	fsm fsm.Storage
	c   *content.Provider
	now func() time.Time
	// fails — счётчик неудач; nil — без лимита.
	fails FailCounter

	// me — username бота из getMe (botUsername).
	meMu sync.Mutex
	me   string
}

// Register — единственное место регистрации роутов бота.
func Register(r *router.Router, d Deps) {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	h := &handlers{cfg: d.Config, fsm: d.FSM, rw: writeThrough{d.Remnawave, d.Payments}, pay: d.Payments, c: d.Content, now: now, fails: d.Fails}
	r.Callback(routeMenu, h.start)
	r.Callback(routeInfo, h.info)
	r.Callback(routePay, h.tariffs)
	r.Callback(routeReferral, h.referral)
	r.Callback(routeTrial, h.trial)
	r.Callback(routeTrialYes, h.trialYes)
	r.Callback(priceRoute.Name(), h.price)
	r.Callback(buyRoute.Name(), h.buy)
	r.Callback(buyNoDevRoute.Name(), h.buyNoDevices)
	r.Callback(routeGift, h.gift)
	r.Callback(giftPriceRoute.Name(), h.giftPrice)
	r.Callback(giftBuyRoute.Name(), h.giftBuy)
	r.Callback(routeDevices, h.devices)
	r.Callback(routeDevicesBuy, h.devicesBuyMenu)
	r.Callback(routeDevicesCustom, h.devicesCustom)
	r.Callback(devPickRoute.Name(), h.devicesPick)
	r.Callback(devYesRoute.Name(), h.devicesBuy)
	r.Callback(havePromoRoute.Name(), h.havePromo)
	r.Callback(buyPromoRoute.Name(), h.buyPromo)
	r.Callback(routePromo, h.promoMenu)
	r.Command("login", h.login)
	// ожидание кода — до Photo и Default: любое сообщение в этих стейтах — код (команды и фото выходят сами)
	r.Message("promo_payment", h.paymentPromoText, router.InState(stateAwaitPromo))
	r.Message("promo_menu", h.menuPromoText, router.InState(stateAwaitPromoDays))
	r.Message("devices_number", h.devicesNumberText, router.InState(stateAwaitDevices))
	r.Photo("payment", h.photo, router.InState(stateAwaitPhoto, stateInReview))
	adminOnly := middleware.AdminOnly(d.Config.Telegram.AdminID)
	r.Callback(confirmRoute.Name(), h.confirm, adminOnly)
	r.Callback(declineRoute.Name(), h.decline, adminOnly)
	r.Default(h.start)
}
