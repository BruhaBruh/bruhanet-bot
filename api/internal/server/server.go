// Package server — HTTP API админки: вход по одноразовой ссылке, сессии, статика SPA.
package server

import (
	"context"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/prometheus/client_golang/prometheus"
)

// Store — методы core/store, которыми пользуется api.
type Store interface {
	ConsumeLoginToken(ctx context.Context, hash []byte, now time.Time) error
	CreateSession(ctx context.Context, hash []byte, telegramID int64, expiresAt time.Time) error
	SessionTelegramID(ctx context.Context, hash []byte, now time.Time) (int64, error)
	DeleteSession(ctx context.Context, hash []byte) error
}

// Panel — Remnawave (pkg/remnawave).
type Panel interface {
	GetUser(ctx context.Context, telegramID int64) (*api.UserItemInfo, error)
}

type Deps struct {
	Store   Store
	Panel   Panel
	AdminID int64
	BaseURL string
	Now     func() time.Time
	Static  fs.FS
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
	s := &server{Deps: d, secure: strings.HasPrefix(d.BaseURL, "https://")}
	// default-deny: всё под /api/ живёт в apiMux и проходит jsonOnly (дёшево, без БД) и сессию.
	// Тот же *Request уходит во вложенный мукс, он перезаписывает r.Pattern — метрика видит точный шаблон.
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("GET /api/me", s.me)
	apiMux.HandleFunc("POST /api/logout", s.logout)
	apiMux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth", s.auth)
	mux.Handle("/api/", jsonOnly(s.requireSession(apiMux)))
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
