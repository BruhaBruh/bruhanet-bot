package fsm_test

import (
	"context"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/pkg/fsm"
)

func TestMiddleware(t *testing.T) {
	store := fsm.NewInMemory()
	var got *fsm.FSM
	h := fsm.Middleware(store)(func(ctx context.Context, _ *bot.Bot, _ *models.Update) {
		got = fsm.From(ctx)
	})

	h(context.Background(), nil, &models.Update{Message: &models.Message{From: &models.User{ID: 42}}})
	if got == nil || got.UserID() != 42 {
		t.Fatalf("ожидался FSM юзера 42, получено %+v", got)
	}

	got = nil
	h(context.Background(), nil, &models.Update{})
	if got != nil {
		t.Fatal("для апдейта без юзера FSM в ctx быть не должно")
	}
}
