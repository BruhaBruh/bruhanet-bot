package routes

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/bruhabruh/bruhanet/bot/internal/handler/middleware"
	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/pkg/fsm"
	"github.com/bruhabruh/bruhanet/pkg/keyboard"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
	"github.com/bruhabruh/bruhanet/pkg/tgupdate"
)

const (
	routeBroadcastSend   = "bc_send"
	routeBroadcastCancel = "bc_cancel"

	// broadcastInterval держит скорость ниже лимита Telegram (~30 сообщений/с).
	broadcastInterval = 40 * time.Millisecond
)

func (h *handlers) isAdmin(u *models.Update) bool {
	return tgupdate.UserID(u) == h.cfg.Telegram.AdminID
}

func broadcastCancelButton() keyboard.Button {
	return keyboard.Callback("Отмена", routeBroadcastCancel).Style(keyboard.StyleDanger)
}

func (h *handlers) broadcastStart(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.isAdmin(u) {
		h.start(ctx, b, u)
		return
	}
	if err := fsm.Put(ctx, fsm.From(ctx), stateAwaitBroadcast, nil); err != nil {
		stlog.Error(ctx, "failed to start broadcast", "error", err)
		return
	}
	send(ctx, b, h.cfg.Telegram.AdminID, keyboard.Inline(keyboard.Row(broadcastCancelButton())),
		"Пришлите сообщение для рассылки")
}

// broadcastRecipients — уникальные Telegram ID юзеров панели в порядке первого появления;
// complete=false, если список оборвался ошибкой панели.
func (h *handlers) broadcastRecipients(ctx context.Context) (ids []int64, complete bool) {
	seen := make(map[int64]struct{})
	for user, err := range h.rw.Users(ctx) {
		if err != nil {
			stlog.Error(ctx, "failed to list remnawave users", "error", err)
			return ids, false
		}
		id, ok := remnawave.TelegramID(user)
		if !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, true
}

func (h *handlers) broadcastMessage(ctx context.Context, b *bot.Bot, u *models.Update) {
	if !h.isAdmin(u) {
		h.start(ctx, b, u)
		return
	}
	ids, complete := h.broadcastRecipients(ctx)
	if !complete {
		h.showError(ctx, b, u)
		return
	}
	bc := model.Broadcast{ChatID: u.Message.Chat.ID, MessageID: u.Message.ID}
	if err := copyMessage(ctx, b, h.cfg.Telegram.AdminID, bc); err != nil {
		stlog.Warn(ctx, "failed to copy broadcast preview", "error", err)
		send(ctx, b, h.cfg.Telegram.AdminID, keyboard.Inline(keyboard.Row(broadcastCancelButton())),
			"Не удалось показать превью, пришлите другое сообщение")
		return
	}
	if err := fsm.Put(ctx, fsm.From(ctx), stateConfirmBroadcast, bc); err != nil {
		stlog.Error(ctx, "failed to save broadcast", "error", err)
		h.showError(ctx, b, u)
		return
	}
	send(ctx, b, h.cfg.Telegram.AdminID, keyboard.Inline(keyboard.Row(
		keyboard.Callback("Отправить", routeBroadcastSend).Style(keyboard.StyleSuccess),
		broadcastCancelButton(),
	)), "Так увидят *"+strconv.Itoa(len(ids))+"* "+pluralPeople(len(ids))+"\\. Отправить?")
}

func copyMessage(ctx context.Context, b *bot.Bot, to int64, bc model.Broadcast) error {
	_, err := b.CopyMessage(ctx, &bot.CopyMessageParams{ChatID: to, FromChatID: bc.ChatID, MessageID: bc.MessageID})
	return err
}

func (h *handlers) broadcastSend(ctx context.Context, b *bot.Bot, u *models.Update) {
	f := fsm.From(ctx)
	bc, err := fsm.Data[model.Broadcast](ctx, f)
	if err == nil {
		err = fsm.Transition(ctx, f, stateConfirmBroadcast, fsm.StateNone, nil)
	}
	if errors.Is(err, fsm.ErrStateMismatch) || errors.Is(err, fsm.ErrNoData) {
		middleware.Answer(ctx, b, u, "Рассылка уже отправлена или отменена")
		return
	}
	if err != nil {
		stlog.Error(ctx, "failed to start broadcast send", "error", err)
		middleware.Answer(ctx, b, u, "Ошибка, попробуйте ещё раз")
		return
	}

	middleware.Answer(ctx, b, u, "Рассылка запущена")
	show(ctx, b, u, nil, "Рассылка идёт…")
	ids, complete := h.broadcastRecipients(ctx)
	var sent, failed int
	interrupted := false
loop:
	for i, id := range ids {
		if i > 0 {
			select {
			case <-ctx.Done():
				interrupted = true
				break loop
			case <-time.After(broadcastInterval):
			}
		}
		if err := copyMessage(ctx, b, id, bc); err != nil {
			stlog.Warn(ctx, "failed to deliver broadcast", "error", err, "telegram_id", id)
			failed++
			continue
		}
		sent++
	}
	rows := []string{"Разослано: *" + strconv.Itoa(sent) + "*, не доставлено: *" + strconv.Itoa(failed) + "*"}
	if interrupted {
		rows = append(rows, "Прервано, не отправлено: *"+strconv.Itoa(len(ids)-sent-failed)+"*")
	}
	if !complete {
		rows = append(rows, "Список пользователей получен не полностью")
	}
	// ctx мог быть отменён при остановке — итог всё равно пробуем отправить.
	send(context.WithoutCancel(ctx), b, h.cfg.Telegram.AdminID, nil, rows...)
}

func (h *handlers) broadcastCancel(ctx context.Context, b *bot.Bot, u *models.Update) {
	f := fsm.From(ctx)
	st, err := f.State(ctx)
	if err == nil && (st == stateAwaitBroadcast || st == stateConfirmBroadcast) {
		err = fsm.Transition(ctx, f, st, fsm.StateNone, nil)
	} else if err == nil {
		err = fsm.ErrStateMismatch
	}
	if errors.Is(err, fsm.ErrStateMismatch) {
		middleware.Answer(ctx, b, u, "Рассылка уже отправлена или отменена")
		return
	}
	if err != nil {
		stlog.Error(ctx, "failed to cancel broadcast", "error", err)
	}
	show(ctx, b, u, nil, "Рассылка отменена")
}
