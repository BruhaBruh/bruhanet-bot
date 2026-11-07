// Package server — HTTP API админки: вход по одноразовой ссылке, сессии, статика SPA.
package server

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/texts"
)

// Store — методы core/store, которыми пользуется api.
type Store interface {
	ConsumeLoginToken(ctx context.Context, hash []byte, now time.Time) error
	CreateSession(ctx context.Context, hash []byte, telegramID int64, expiresAt time.Time) error
	SessionTelegramID(ctx context.Context, hash []byte, now time.Time) (int64, error)
	DeleteSession(ctx context.Context, hash []byte) error
	ListUsers(ctx context.Context, f model.UserFilter) ([]model.UserSummary, int, error)
	UserPromos(ctx context.Context, id int64) ([]model.UserPromo, error)
	WithPanelSyncLock(ctx context.Context, fn func(context.Context) error) error
	ApplyPanelSync(ctx context.Context, users []model.PanelUser, started, finished time.Time) (int, error)
	LastPanelSync(ctx context.Context) (model.PanelSync, error)
	UserByID(ctx context.Context, id int64) (model.UserSummary, error)
	UserStats(ctx context.Context, telegramID, excludeID int64) (model.UserStats, error)
	PaymentsByUser(ctx context.Context, id int64) ([]model.Payment, error)
	PaymentByID(ctx context.Context, id int64) (model.Payment, error)
	ResolvePaymentRef(ctx context.Context, id int64, status model.PaymentStatus, at time.Time, referralDays int,
		apply func(model.Payment) ([]model.OutboxMessage, error)) (model.Payment, error)
	// Рефералка: дни пригласившему (payflow.ReferralStore) и строки карточки юзера.
	GrantReferral(ctx context.Context, referredID int64, now time.Time,
		apply func(referrerID int64, username string, days int) ([]model.OutboxMessage, error)) error
	UserReferral(ctx context.Context, id int64) (model.UserReferral, error)
	SetUserPanel(ctx context.Context, id int64, expireAt time.Time, status string, at time.Time) error
	ListPayments(ctx context.Context, f model.PaymentFilter) ([]model.Payment, int, error)
	Revenue(ctx context.Context, from, to time.Time) (int, error)
	TextOverrides(ctx context.Context) (map[string]string, error)
	SetText(ctx context.Context, key, body string) error
	ResetText(ctx context.Context, key string) error
	ClearTextBody(ctx context.Context, key string) error
	TextStyles(ctx context.Context) (map[string]string, error)
	SetTextStyle(ctx context.Context, key string, style *string) error
	MessageKeyboards(ctx context.Context) (map[string]texts.MsgLayout, error)
	SaveMessageKeyboard(ctx context.Context, key string, l texts.MsgLayout) error
	DeleteMessageKeyboard(ctx context.Context, key string) error
	SettingsRaw(ctx context.Context) (map[string][]byte, error)
	SetSettings(ctx context.Context, values map[string][]byte) error
	Enqueue(ctx context.Context, m model.OutboxMessage) error
	SendBroadcast(ctx context.Context, d model.BroadcastDraft, adminID int64, now time.Time) (model.Broadcast, bool, error)
	SaveDraft(ctx context.Context, id uuid.UUID, d model.BroadcastDraft) (model.Broadcast, error)
	UpdateScheduled(ctx context.Context, id uuid.UUID, d model.BroadcastDraft) (model.Broadcast, error)
	CancelBroadcast(ctx context.Context, id uuid.UUID) (model.Broadcast, error)
	DeleteBroadcast(ctx context.Context, id uuid.UUID) error
	Broadcast(ctx context.Context, id uuid.UUID) (model.Broadcast, error)
	ListBroadcasts(ctx context.Context, f model.BroadcastFilter) ([]model.Broadcast, int, error)
	AudienceCounts(ctx context.Context, now time.Time, days int) (model.AudienceCounts, error)
	CreateDirect(ctx context.Context, telegramID int64, text, photo string, photoID *uuid.UUID) (int64, error)
	SavePhoto(ctx context.Context, content []byte, contentType string) (uuid.UUID, error)
	Photo(ctx context.Context, id uuid.UUID) ([]byte, string, error)
	PhotoExists(ctx context.Context, id uuid.UUID) (bool, error)
	DirectMessages(ctx context.Context, telegramID int64) ([]model.DirectMessage, error)
	UserByUsername(ctx context.Context, username string) (model.UserSummary, error)
	CreatePromo(ctx context.Context, p model.Promo) (model.Promo, error)
	PromoByID(ctx context.Context, id int64) (model.Promo, error)
	ListPromos(ctx context.Context, f model.PromoFilter, now time.Time) ([]model.PromoWithUsage, error)
	DisablePromo(ctx context.Context, id int64) error
	DeletePromo(ctx context.Context, id int64) error
	EnablePromo(ctx context.Context, id int64) error
	UserGrowth(ctx context.Context, prevFrom, weekFrom, to time.Time) (total, week, prev int, err error)
	CountPromoActivations(ctx context.Context, from, to time.Time) (int, error)
	PromoActivations(ctx context.Context, id int64) ([]model.PromoActivation, error)
	GrantDays(ctx context.Context, telegramID, adminID int64, days int, now time.Time, apply func() ([]model.OutboxMessage, error)) error
	SetUserBlocked(ctx context.Context, telegramID int64, blocked bool, at time.Time) error
	UserGrants(ctx context.Context, telegramID int64, limit int) ([]model.Grant, error)
	// Подарки: выдача админом, отмена готового, список со страницами.
	CreateAdminGiftsOnce(ctx context.Context, key uuid.UUID, months, count int, at time.Time) ([]model.Gift, bool, error)
	CancelGift(ctx context.Context, id int64) error
	DeleteGift(ctx context.Context, id int64) error
	ListGifts(ctx context.Context, f model.GiftFilter) ([]model.Gift, int, error)
}

// Panel — Remnawave (pkg/remnawave).
type Panel interface {
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
	CreateUser(ctx context.Context, telegramID int64, username string, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	UpdateUser(ctx context.Context, telegramID int64, username string, expireAt time.Time, squadUUID uuid.UUID) (*api.UserItemInfo, error)
	SetDeviceLimit(ctx context.Context, telegramID int64, limit int) (*api.UserItemInfo, error)
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
	// HWID-устройства юзера по числовому id в панели.
	UserDevices(ctx context.Context, userID int) ([]api.Device, error)
	DeleteUserDevice(ctx context.Context, userID int, hwid string) error
	DeleteAllUserDevices(ctx context.Context, userID int) error
	// SubpageConfig — сырой конфиг страницы подписки юзера (инструкции подключения Mini App).
	SubpageConfig(ctx context.Context, shortUUID string) (json.RawMessage, error)
}

// PhotoSender — чтение файлов из Telegram (api/internal/telegram): чеки платежей. Фото рассылок в Telegram не
// заливаются: байты лежат в БД, их отправляет бот при доставке.
type PhotoSender interface {
	OpenFile(ctx context.Context, fileID string) (io.ReadCloser, error)
}

type Deps struct {
	Store    Store
	Panel    Panel
	Telegram PhotoSender
	// Avatars — аватарки из Telegram; nil — /api/avatars/* всегда 404.
	Avatars AvatarSource
	// Identity — getMe/getChat для /api/me и /auth/bot; nil — имена пустые.
	Identity Identity
	// Defaults — настройки по умолчанию из env (те же PAYMENT_*/SUPPORT_URL, что у бота).
	Defaults settings.Settings
	// PanelTimeout — потолок на один вызов панели; 0 → 5с.
	PanelTimeout time.Duration
	// Squad — внутренний сквад Remnawave для записи срока.
	Squad   uuid.UUID
	AdminID int64
	BaseURL string
	Now     func() time.Time
	Static  fs.FS
	// Mini App (NewMiniApp): токен бота для проверки initData, origin для проверки Origin у мутаций, статика SPA.
	BotToken   string
	MiniOrigin string
	MiniStatic fs.FS
}

// Requests — api_http_requests_total; route — шаблон маршрута ServeMux, без id.
var Requests = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "api_http_requests_total", Help: "HTTP requests to the admin api.",
}, []string{"route", "code"})

type server struct {
	Deps
	secure     bool
	botCache   lazyWho
	adminCache lazyWho
	avatars    *avatarCache
	mini       *limiter // лимиты Mini App; nil у админки
}

func New(d Deps) http.Handler {
	if d.PanelTimeout == 0 {
		d.PanelTimeout = 5 * time.Second
	}
	s := &server{Deps: d, secure: strings.HasPrefix(d.BaseURL, "https://"), avatars: newAvatarCache()}
	// default-deny: всё под /api/ живёт в apiMux и проходит jsonOnly (дёшево, без БД) и сессию.
	// Тот же *Request уходит во вложенный мукс, он перезаписывает r.Pattern — метрика видит точный шаблон.
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/me", s.me)
	apiMux.HandleFunc("POST /api/logout", s.logout)
	apiMux.HandleFunc("GET /api/overview", s.overview)
	apiMux.HandleFunc("GET /api/users", s.users)
	apiMux.HandleFunc("GET /api/users/{id}", s.userCard)
	apiMux.HandleFunc("GET /api/sync", s.lastSync)
	apiMux.HandleFunc("POST /api/sync", s.runSync)
	apiMux.HandleFunc("GET /api/payments", s.payments)
	apiMux.HandleFunc("GET /api/payments/{id}/photo", s.paymentPhoto)
	apiMux.HandleFunc("GET /api/photos/{id}", s.photo)
	apiMux.HandleFunc("GET /api/avatars/{id}", s.avatarImage)
	apiMux.HandleFunc("POST /api/payments/{id}/confirm", s.resolvePayment(true))
	apiMux.HandleFunc("POST /api/payments/{id}/decline", s.resolvePayment(false))
	apiMux.HandleFunc("GET /api/texts", s.texts)
	apiMux.HandleFunc("PUT /api/texts/{key}", s.putText)
	apiMux.HandleFunc("DELETE /api/texts/{key}", s.deleteText)
	apiMux.HandleFunc("POST /api/texts/{key}/test", s.testText)
	apiMux.HandleFunc("GET /api/uxmap", s.uxMap)
	apiMux.HandleFunc("PUT /api/texts/{key}/keyboard", s.putMessageKeyboard)
	apiMux.HandleFunc("DELETE /api/texts/{key}/keyboard", s.deleteMessageKeyboard)
	apiMux.HandleFunc("GET /api/settings", s.settings)
	apiMux.HandleFunc("PUT /api/settings", s.putSettings)
	apiMux.HandleFunc("POST /api/broadcasts/preview-send", s.previewBroadcast)
	apiMux.HandleFunc("GET /api/broadcasts/recipients", s.recipients)
	apiMux.HandleFunc("POST /api/broadcasts", s.createBroadcast)
	apiMux.HandleFunc("GET /api/broadcasts", s.broadcasts)
	apiMux.HandleFunc("GET /api/broadcasts/{id}", s.broadcast)
	apiMux.HandleFunc("PUT /api/broadcasts/{id}", s.updateBroadcast)
	apiMux.HandleFunc("POST /api/broadcasts/{id}/cancel", s.cancelBroadcast)
	apiMux.HandleFunc("DELETE /api/broadcasts/{id}", s.deleteBroadcast)
	apiMux.HandleFunc("GET /api/users/{id}/devices", s.devices)
	apiMux.HandleFunc("DELETE /api/users/{id}/devices", s.deleteAllDevices)
	apiMux.HandleFunc("DELETE /api/users/{id}/devices/{hwid}", s.deleteDevice)
	apiMux.HandleFunc("POST /api/users/{id}/messages", s.directMessage)
	apiMux.HandleFunc("POST /api/users/{id}/grant", s.grantDays)
	apiMux.HandleFunc("POST /api/users/{id}/block", s.setBlocked(true))
	apiMux.HandleFunc("POST /api/users/{id}/unblock", s.setBlocked(false))
	apiMux.HandleFunc("GET /api/promos", s.promos)
	apiMux.HandleFunc("POST /api/promos", s.createPromo)
	apiMux.HandleFunc("POST /api/promos/{id}/enable", s.enablePromo)
	apiMux.HandleFunc("POST /api/promos/{id}/disable", s.disablePromo)
	apiMux.HandleFunc("DELETE /api/promos/{id}", s.deletePromo)
	apiMux.HandleFunc("GET /api/promos/{id}/activations", s.promoActivations)
	apiMux.HandleFunc("GET /api/gifts", s.gifts)
	apiMux.HandleFunc("POST /api/gifts", s.createGifts)
	apiMux.HandleFunc("POST /api/gifts/{id}/cancel", s.cancelGift)
	apiMux.HandleFunc("DELETE /api/gifts/{id}", s.deleteGift)
	apiMux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth", s.auth)
	mux.HandleFunc("GET /auth/bot", s.authBot)
	mux.Handle("/api/", jsonOnly(s.requireSession(apiMux)))
	// единственное исключение из jsonOnly: загрузка фото рассылки — multipart; сессия обязательна, тип проверяет сам хендлер.
	mux.Handle("POST /api/photos", s.requireSession(http.HandlerFunc(s.uploadPhoto)))
	mux.HandleFunc("/", s.static) // без метода: "GET /" конфликтует с "/api/"
	return metrics(securityHeaders(csp(d.Static), true, mux))
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		route := r.Pattern // ServeMux проставляет шаблон в тот же *Request
		if route == "" {
			route = "unmatched"
		}
		Requests.WithLabelValues(route, strconv.Itoa(sw.code)).Inc()
	})
}
