package tgtest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const Token = "test-token"

type Call struct {
	Method string
	Params url.Values
}

type Server struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []Call
}

func NewServer(t testing.TB) *Server {
	t.Helper()
	s := &Server{}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *Server) Options() []bot.Option {
	return []bot.Option{
		bot.WithSkipGetMe(),
		bot.WithServerURL(s.srv.URL),
		bot.WithNotAsyncHandlers(),
	}
}

func (s *Server) Bot(t testing.TB, opts ...bot.Option) *bot.Bot {
	t.Helper()
	b, err := bot.New(Token, append(s.Options(), opts...)...)
	if err != nil {
		t.Fatalf("bot.New: %v", err)
	}
	return b
}

func (s *Server) Calls(method string) []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Call
	for _, c := range s.calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	_ = r.ParseMultipartForm(1 << 20) //nolint:gosec // G120 false positive: body already capped by MaxBytesReader above
	method := path.Base(r.URL.Path)
	s.mu.Lock()
	s.calls = append(s.calls, Call{Method: method, Params: r.Form})
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch method {
	case "answerCallbackQuery", "deleteMessage":
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	default:
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`)
	}
}

func Message(userID int64, text string) *models.Update {
	return &models.Update{
		ID: 1,
		Message: &models.Message{
			ID:   10,
			From: &models.User{ID: userID, FirstName: "Test"},
			Chat: models.Chat{ID: userID, Type: models.ChatTypePrivate},
			Text: text,
		},
	}
}

func Photo(userID int64) *models.Update {
	u := Message(userID, "")
	u.Message.Photo = []models.PhotoSize{{FileID: "photo"}}
	return u
}

func Callback(userID int64, data string) *models.Update {
	return &models.Update{
		ID: 2,
		CallbackQuery: &models.CallbackQuery{
			ID:   "cb-1",
			From: models.User{ID: userID, FirstName: "Test"},
			Data: data,
			Message: models.MaybeInaccessibleMessage{
				Type: models.MaybeInaccessibleMessageTypeMessage,
				Message: &models.Message{
					ID:   20,
					Chat: models.Chat{ID: userID, Type: models.ChatTypePrivate},
				},
			},
		},
	}
}

func Logger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func LogContext() (context.Context, *bytes.Buffer) {
	logger, buf := Logger()
	return stlog.Wrap(context.Background(), logger), buf
}

func FindLog(t testing.TB, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for line := range strings.Lines(buf.String()) {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("невалидная строка лога %q: %v", line, err)
		}
		if entry["msg"] == msg {
			return entry
		}
	}
	t.Fatalf("лог %q не найден в:\n%s", msg, buf.String())
	return nil
}
