package middleware_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

func chain(h bot.HandlerFunc, mws ...bot.Middleware) bot.HandlerFunc {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}

func noop(context.Context, *bot.Bot, *models.Update) {}

func boom(context.Context, *bot.Bot, *models.Update) { panic("boom") }

func TestRequestID(t *testing.T) {
	var id string
	var info *middleware.UpdateInfo
	chain(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		id = middleware.RequestIDFrom(ctx)
		info = middleware.UpdateInfoFrom(ctx)
	}, middleware.RequestID)(context.Background(), nil, tgtest.Message(42, "x"))
	if id == "" {
		t.Fatal("request id пустой")
	}
	if info == nil {
		t.Fatal("UpdateInfo не положен в ctx")
	}
}

func TestUpdateInfoNilSafe(t *testing.T) {
	var info *middleware.UpdateInfo
	info.SetRoute("x")
	info.MarkAnswered()
	info.MarkPanicked()
	if info.Route() != "" || info.Answered() || info.Panicked() {
		t.Fatal("nil UpdateInfo должен возвращать нулевые значения")
	}
}

func TestLoggerWritesUpdateHandled(t *testing.T) {
	ctx, buf := tgtest.LogContext()
	h := chain(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		middleware.UpdateInfoFrom(ctx).SetRoute("default")
	}, middleware.RequestID, middleware.TelegramID, middleware.Logger)
	h(ctx, nil, tgtest.Message(42, "secret-text"))

	line := tgtest.FindLog(t, buf, "update handled")
	if line["level"] != "INFO" || line["route"] != "default" {
		t.Fatalf("line = %v", line)
	}
	if line["telegram_id"] != float64(42) || line["chat_id"] != float64(42) || line["update_type"] != "message" {
		t.Fatalf("поля апдейта: %v", line)
	}
	if _, ok := line["update_id"]; ok {
		t.Fatalf("update_id не должен логироваться: %v", line)
	}
	if line["request_id"] == "" || line["request_id"] == nil {
		t.Fatalf("нет request_id: %v", line)
	}
	if _, ok := line["duration_ms"]; !ok {
		t.Fatalf("нет duration_ms: %v", line)
	}
	if strings.Contains(buf.String(), "secret-text") {
		t.Fatal("текст сообщения попал в лог")
	}
}

func TestTelegramIDLogsCallbackData(t *testing.T) {
	ctx, buf := tgtest.LogContext()
	chain(noop, middleware.RequestID, middleware.TelegramID, middleware.Logger)(ctx, nil, tgtest.Callback(42, "pay"))
	line := tgtest.FindLog(t, buf, "update handled")
	if line["callback_data"] != "pay" || line["update_type"] != "callback" {
		t.Fatalf("line = %v", line)
	}
}

func TestWithLogger(t *testing.T) {
	logger, buf := tgtest.Logger()
	chain(noop, middleware.WithLogger(logger), middleware.RequestID, middleware.Logger)(context.Background(), nil, tgtest.Message(1, "x"))
	tgtest.FindLog(t, buf, "update handled")
}

func TestRecoveryInner(t *testing.T) {
	ctx, buf := tgtest.LogContext()
	chain(boom, middleware.RequestID, middleware.Logger, middleware.Recovery)(ctx, nil, tgtest.Message(42, "x"))

	p := tgtest.FindLog(t, buf, "panic recovered")
	if p["panic"] != "boom" || p["stack"] == "" || p["level"] != "ERROR" {
		t.Fatalf("panic line = %v", p)
	}
	if strings.Contains(buf.String(), "update handled") {
		t.Fatalf("при панике update handled не пишется, лог:\n%s", buf.String())
	}
}

func TestRecoveryOuterWithoutInfo(t *testing.T) {
	ctx, buf := tgtest.LogContext()
	middleware.Recovery(boom)(ctx, nil, tgtest.Message(1, "x"))
	tgtest.FindLog(t, buf, "panic recovered")
}

func TestAutoAnswer(t *testing.T) {
	t.Run("answers when handler did not", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		chain(noop, middleware.RequestID, middleware.AutoAnswer())(context.Background(), srv.Bot(t), tgtest.Callback(42, "pay"))
		calls := srv.Calls("answerCallbackQuery")
		if len(calls) != 1 || calls[0].Params.Get("text") != "" || calls[0].Params.Get("callback_query_id") != "cb-1" {
			t.Fatalf("calls = %+v", calls)
		}
	})

	t.Run("does not answer twice", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		h := func(ctx context.Context, b *bot.Bot, u *models.Update) {
			middleware.Answer(ctx, b, u, "hi")
			middleware.Answer(ctx, b, u, "again")
		}
		chain(h, middleware.RequestID, middleware.AutoAnswer())(context.Background(), srv.Bot(t), tgtest.Callback(42, "pay"))
		calls := srv.Calls("answerCallbackQuery")
		if len(calls) != 1 || calls[0].Params.Get("text") != "hi" {
			t.Fatalf("calls = %+v", calls)
		}
	})

	t.Run("answers after inner panic", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		chain(boom, middleware.RequestID, middleware.AutoAnswer(), middleware.Recovery)(context.Background(), srv.Bot(t), tgtest.Callback(42, "pay"))
		if calls := srv.Calls("answerCallbackQuery"); len(calls) != 1 {
			t.Fatalf("calls = %+v", calls)
		}
	})

	t.Run("ignores messages", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		chain(noop, middleware.RequestID, middleware.AutoAnswer())(context.Background(), srv.Bot(t), tgtest.Message(42, "x"))
		if calls := srv.Calls("answerCallbackQuery"); len(calls) != 0 {
			t.Fatalf("calls = %+v", calls)
		}
	})

	t.Run("route message wins over global", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		chain(noop, middleware.RequestID, middleware.AutoAnswer(), middleware.AutoAnswer("Готово", "Ждите проверку"))(
			context.Background(), srv.Bot(t), tgtest.Callback(42, "pay"))
		calls := srv.Calls("answerCallbackQuery")
		if len(calls) != 1 || calls[0].Params.Get("text") != "Готово\nЖдите проверку" {
			t.Fatalf("calls = %+v", calls)
		}
	})

	t.Run("route message skipped on panic", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		chain(boom, middleware.RequestID, middleware.AutoAnswer(), middleware.Recovery, middleware.AutoAnswer("Готово"))(
			context.Background(), srv.Bot(t), tgtest.Callback(42, "pay"))
		calls := srv.Calls("answerCallbackQuery")
		if len(calls) != 1 || calls[0].Params.Get("text") != "" {
			t.Fatalf("calls = %+v", calls)
		}
	})
}

func TestAdminOnly(t *testing.T) {
	t.Run("denies non admin", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		ctx, buf := tgtest.LogContext()
		called := false
		h := func(context.Context, *bot.Bot, *models.Update) { called = true }
		chain(h, middleware.RequestID, middleware.AutoAnswer(), middleware.AdminOnly(1))(ctx, srv.Bot(t), tgtest.Callback(2, "adm_ok"))
		if called {
			t.Fatal("хендлер вызван для не-админа")
		}
		calls := srv.Calls("answerCallbackQuery")
		if len(calls) != 1 || calls[0].Params.Get("text") != "Недостаточно прав" {
			t.Fatalf("calls = %+v", calls)
		}
		if l := tgtest.FindLog(t, buf, "admin route denied"); l["level"] != "WARN" {
			t.Fatalf("line = %v", l)
		}
	})

	t.Run("allows admin", func(t *testing.T) {
		srv := tgtest.NewServer(t)
		called := false
		h := func(context.Context, *bot.Bot, *models.Update) { called = true }
		chain(h, middleware.RequestID, middleware.AdminOnly(1))(context.Background(), srv.Bot(t), tgtest.Callback(1, "adm_ok"))
		if !called {
			t.Fatal("хендлер не вызван для админа")
		}
	})
}

func TestTrackDetachesCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var wg sync.WaitGroup
	var err error
	chain(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		err = ctx.Err()
	}, middleware.Track(&wg))(ctx, nil, tgtest.Message(1, "x"))
	if err != nil {
		t.Fatalf("ctx хендлера отменён: %v", err)
	}
}

func TestTrackCountsInFlight(t *testing.T) {
	var wg sync.WaitGroup
	started, release := make(chan struct{}), make(chan struct{})
	go chain(func(context.Context, *bot.Bot, *models.Update) {
		close(started)
		<-release
	}, middleware.Track(&wg))(context.Background(), nil, tgtest.Message(1, "x"))
	<-started

	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	select {
	case <-drained:
		t.Fatal("wg.Wait вернулся при работающем хендлере")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("wg.Wait не вернулся после завершения хендлера")
	}
}
