package middleware_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/model"
)

func TestLastSeenThrottle(t *testing.T) {
	var got []model.UserSeen
	var fail error
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	touch := func(_ context.Context, u model.UserSeen) error {
		got = append(got, u)
		return fail
	}
	h := chain(noop, middleware.LastSeen(touch, 5*time.Minute, func() time.Time { return now }))
	send := func(id int64) {
		u := tgtest.Message(id, "x")
		u.Message.From.FirstName = "Анна"
		h(t.Context(), nil, u)
	}

	send(1)
	if len(got) != 1 || got[0].TelegramID != 1 || got[0].FirstName != "Анна" || !got[0].At.Equal(now) {
		t.Fatalf("first: %+v", got)
	}
	now = now.Add(4 * time.Minute)
	send(1)
	if len(got) != 1 {
		t.Fatalf("inside window: %d writes", len(got))
	}
	send(2) // у другого юзера своё окно
	if len(got) != 2 {
		t.Fatalf("other user: %d writes", len(got))
	}
	now = now.Add(time.Minute)
	send(1)
	if len(got) != 3 {
		t.Fatalf("after window: %d writes", len(got))
	}

	// Сбой не мешает апдейту и не съедает окно: следующий апдейт повторит запись.
	fail = errors.New("db down")
	send(3)
	fail = nil
	send(3)
	if len(got) != 5 {
		t.Fatalf("retry after failure: %d writes", len(got))
	}
}

func TestLastSeenRunsHandlerOnFailureAndSkipsForeignUpdates(t *testing.T) {
	writes, handled := 0, 0
	touch := func(context.Context, model.UserSeen) error { writes++; return errors.New("db down") }
	h := middleware.LastSeen(touch, time.Minute, time.Now)(func(context.Context, *bot.Bot, *models.Update) { handled++ })

	group := tgtest.Message(1, "x")
	group.Message.Chat.Type = models.ChatTypeGroup
	h(t.Context(), nil, group)
	h(t.Context(), nil, &models.Update{MyChatMember: &models.ChatMemberUpdated{}})
	if writes != 0 || handled != 2 {
		t.Fatalf("foreign updates: writes=%d handled=%d", writes, handled)
	}

	h(t.Context(), nil, tgtest.Callback(1, "menu"))
	if writes != 1 || handled != 3 {
		t.Fatalf("callback: writes=%d handled=%d", writes, handled)
	}
}
