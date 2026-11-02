package routes

import (
	"context"
	"iter"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/handler/router"
	"github.com/bruhabruh/bruhanet/bot/internal/model"
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
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
}

// Payments — хранилище платежей (store.Store); ошибки — store.ErrNotFound / store.ErrNotPending.
type Payments interface {
	CreatePayment(ctx context.Context, p model.Payment, rollback func(old model.Payment) error) (model.Payment, *model.Payment, error)
	PendingPayment(ctx context.Context, telegramID int64) (model.Payment, error)
	SubmitPayment(ctx context.Context, id int64, adminMessageID int, at time.Time) error
	MarkAdvance(ctx context.Context, id int64) error
	UserStats(ctx context.Context, telegramID, excludeID int64) (model.UserStats, error)
	ResolvePayment(ctx context.Context, id int64, status model.PaymentStatus, at time.Time,
		apply func(model.Payment) (model.OutboxMessage, error)) (model.Payment, error)
	EnqueueMany(ctx context.Context, ms []model.OutboxMessage) error
}

type Deps struct {
	Config    *config.Config
	FSM       fsm.Storage
	Remnawave Remnawave
	Payments  Payments
	// Now — источник времени; nil = time.Now.
	Now func() time.Time
}

type handlers struct {
	cfg *config.Config
	rw  Remnawave
	pay Payments
	fsm fsm.Storage
	now func() time.Time
}

// Register — единственное место регистрации роутов бота.
func Register(r *router.Router, d Deps) {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	h := &handlers{cfg: d.Config, fsm: d.FSM, rw: d.Remnawave, pay: d.Payments, now: now}
	r.Callback(routeMenu, h.start)
	r.Callback(routeInfo, h.info)
	r.Callback(routePay, h.tariffs)
	r.Callback(priceRoute.Name(), h.price)
	r.Callback(buyRoute.Name(), h.buy)
	// Message-роуты матчатся в порядке регистрации: рассылка раньше оплаты.
	r.Message("broadcast", h.broadcastMessage, router.InState(stateAwaitBroadcast))
	r.Command("a", h.broadcastStart)
	r.Photo("payment", h.photo, router.InState(stateAwaitPhoto, stateInReview))
	adminOnly := middleware.AdminOnly(d.Config.Telegram.AdminID)
	r.Callback(confirmRoute.Name(), h.confirm, adminOnly)
	r.Callback(declineRoute.Name(), h.decline, adminOnly)
	r.Callback(routeBroadcastSend, h.broadcastSend, adminOnly)
	r.Callback(routeBroadcastCancel, h.broadcastCancel, adminOnly)
	r.Default(h.start)
}
