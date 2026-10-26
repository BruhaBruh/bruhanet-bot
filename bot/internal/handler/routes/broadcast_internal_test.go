package routes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bruhabruh/bruhanet/bot/internal/config"
	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

// Middleware Track отвязывает ctx хендлера от отмены, поэтому ветку ctx.Done
// проверяем прямым вызовом хендлера.
func TestBroadcastSendInterrupted(t *testing.T) {
	srv := tgtest.NewServer(t)
	rw := tgtest.NewRemnawave(time.Now)
	for id := int64(42); id < 45; id++ {
		rw.AddUser(id, time.Now())
	}
	store := fsm.NewInMemory()
	h := &handlers{
		cfg: &config.Config{Telegram: config.TelegramConfig{AdminID: 1}},
		rw:  rw, fsm: store, now: time.Now,
	}
	f := fsm.For(store, 1)
	if err := fsm.Put(t.Context(), f, stateConfirmBroadcast, model.Broadcast{ChatID: 1, MessageID: 10}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(fsm.WithContext(t.Context(), f))
	cancel()

	h.broadcastSend(ctx, srv.Bot(t), tgtest.Callback(1, "bc_send"))

	calls := srv.Calls("sendMessage")
	if text := calls[len(calls)-1].Params.Get("text"); !strings.Contains(text, "Прервано, не отправлено: *2*") {
		t.Fatalf("text = %s", text)
	}
}
