// Package server — HTTP API админки: вход по одноразовой ссылке, сессии, статика SPA.
package server

import (
	"context"
	"io"
	"io/fs"
	"iter"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/settings"
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
	ListPayments(ctx context.Context, f model.PaymentFilter) ([]model.Payment, int, error)
	Revenue(ctx context.Context, from, to time.Time) (int, error)
	TextOverrides(ctx context.Context) (map[string]string, error)
	SetText(ctx context.Context, key, body string) error
	ResetText(ctx context.Context, key string) error
	SettingsRaw(ctx context.Context) (map[string][]byte, error)
	SetSettings(ctx context.Context, values map[string][]byte) error
	Enqueue(ctx context.Context, m model.OutboxMessage) error
	CreateBroadcast(ctx context.Context, d model.BroadcastDraft, recipients []int64, adminID int64) (model.Broadcast, bool, error)
	ListBroadcasts(ctx context.Context, limit, offset int) ([]model.Broadcast, int, error)
	CreateDirect(ctx context.Context, telegramID int64, text, photo string) (int64, error)
	DirectMessages(ctx context.Context, telegramID int64) ([]model.DirectMessage, error)
	UserByUsername(ctx context.Context, username string) (model.UserSummary, error)
	CreatePromo(ctx context.Context, p model.Promo) (model.Promo, error)
	PromoByID(ctx context.Context, id int64) (model.Promo, error)
	ListPromos(ctx context.Context, activeOnly bool, now time.Time) ([]model.PromoWithUsage, error)
	DisablePromo(ctx context.Context, id int64) error
	PromoActivations(ctx context.Context, id int64) ([]model.PromoActivation, error)
}

// Panel — Remnawave (pkg/remnawave).
type Panel interface {
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
	Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error]
}

// PhotoSender — загрузка фото в Telegram (api/internal/telegram); возвращает file_id.
type PhotoSender interface {
	SendPhoto(ctx context.Context, chatID int64, r io.Reader, filename, caption string) (fileID string, err error)
}

type Deps struct {
	Store    Store
	Panel    Panel
	Telegram PhotoSender
	// Defaults — настройки по умолчанию из env (те же PAYMENT_*/SUPPORT_URL, что у бота).
	Defaults settings.Settings
	// PanelTimeout — потолок на один вызов панели; 0 → 5с.
	PanelTimeout time.Duration
	AdminID      int64
	BaseURL      string
	Now          func() time.Time
	Static       fs.FS
}

// Requests — api_http_requests_total; route — шаблон маршрута ServeMux, без id.
var Requests = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "api_http_requests_total", Help: "HTTP requests to the admin api.",
}, []string{"route", "code"})

type server struct {
	Deps
	secure bool
}

func New(d Deps) http.Handler {
	if d.PanelTimeout == 0 {
		d.PanelTimeout = 5 * time.Second
	}
	s := &server{Deps: d, secure: strings.HasPrefix(d.BaseURL, "https://")}
	// default-deny: всё под /api/ живёт в apiMux и проходит jsonOnly (дёшево, без БД) и сессию.
	// Тот же *Request уходит во вложенный мукс, он перезаписывает r.Pattern — метрика видит точный шаблон.
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/me", s.me)
	apiMux.HandleFunc("POST /api/logout", s.logout)
	apiMux.HandleFunc("GET /api/users", s.users)
	apiMux.HandleFunc("GET /api/users/{id}", s.userCard)
	apiMux.HandleFunc("GET /api/sync", s.lastSync)
	apiMux.HandleFunc("POST /api/sync", s.runSync)
	apiMux.HandleFunc("GET /api/payments", s.payments)
	apiMux.HandleFunc("GET /api/texts", s.texts)
	apiMux.HandleFunc("PUT /api/texts/{key}", s.putText)
	apiMux.HandleFunc("DELETE /api/texts/{key}", s.deleteText)
	apiMux.HandleFunc("POST /api/texts/{key}/test", s.testText)
	apiMux.HandleFunc("GET /api/settings", s.settings)
	apiMux.HandleFunc("PUT /api/settings", s.putSettings)
	apiMux.HandleFunc("POST /api/broadcasts/preview-send", s.previewBroadcast)
	apiMux.HandleFunc("GET /api/broadcasts/recipients", s.recipients)
	apiMux.HandleFunc("POST /api/broadcasts", s.createBroadcast)
	apiMux.HandleFunc("GET /api/broadcasts", s.broadcasts)
	apiMux.HandleFunc("POST /api/users/{id}/messages", s.directMessage)
	apiMux.HandleFunc("GET /api/promos", s.promos)
	apiMux.HandleFunc("POST /api/promos", s.createPromo)
	apiMux.HandleFunc("POST /api/promos/{id}/disable", s.disablePromo)
	apiMux.HandleFunc("GET /api/promos/{id}/activations", s.promoActivations)
	apiMux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth", s.auth)
	mux.Handle("/api/", jsonOnly(s.requireSession(apiMux)))
	// единственное исключение из jsonOnly: загрузка фото — multipart; сессия обязательна, тип проверяет сам хендлер.
	mux.Handle("POST /api/photos", s.requireSession(http.HandlerFunc(s.uploadPhoto)))
	mux.HandleFunc("/", s.static) // без метода: "GET /" конфликтует с "/api/"
	return metrics(mux)
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
