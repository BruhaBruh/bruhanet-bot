package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/pkg/remnawave"
	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

const (
	broadcastsPage = 20
	maxCaptionLen  = 1024 // подпись к фото в Telegram, UTF-16
)

// message — тело рассылки и личного сообщения: Telegram HTML и необязательный file_id фото.
type message struct {
	Text  string `json:"text"`
	Photo string `json:"photo"`
}

// validateMessage — те же правила, что у текстов-сообщений (без переменных); с фото лимит подписи 1024,
// фото без текста допустимо.
func validateMessage(text string, hasPhoto bool) error {
	if hasPhoto && strings.TrimSpace(text) == "" {
		return nil
	}
	if err := texts.Validate(texts.Def{Kind: texts.KindMessage}, text); err != nil {
		return err
	}
	if n := texts.VisibleLen(text); hasPhoto && n > maxCaptionLen {
		return errors.Join(texts.ErrTooLong, fmt.Errorf("подпись к фото длиннее %d символов (%d)", maxCaptionLen, n))
	}
	return nil
}

// readMessage читает и проверяет message; при ошибке сам отвечает 400.
func readMessage(w http.ResponseWriter, r *http.Request, in any, m *message) bool {
	if !readJSONMax(w, r, in, maxTextBody, false) {
		return false
	}
	if strings.TrimSpace(m.Text) == "" {
		m.Text = ""
	}
	if err := validateMessage(m.Text, m.Photo != ""); err != nil {
		writeInvalid(w, texts.Problem(err))
		return false
	}
	return true
}

// recipientIDs — уникальные telegram ID из панели; ошибка панели — уже отвеченный 502.
func (s *server) recipientIDs(w http.ResponseWriter, r *http.Request) ([]int64, bool) {
	ctx, cancel := context.WithTimeout(r.Context(), s.PanelTimeout)
	defer cancel()
	ids, err := remnawave.TelegramIDs(s.Panel.Users(ctx))
	if err != nil {
		stlog.Warn(r.Context(), "panel users", "error", err)
		writeError(w, http.StatusBadGateway, "panel_unavailable")
		return nil, false
	}
	return ids, true
}

func (s *server) previewBroadcast(w http.ResponseWriter, r *http.Request) {
	var in message
	if !readMessage(w, r, &in, &in) {
		return
	}
	err := s.Store.Enqueue(r.Context(), model.OutboxMessage{
		EventType: "text_test", ChatID: s.AdminID, Text: in.Text, Photo: in.Photo, ParseMode: "HTML",
	})
	if err != nil {
		internal(w, r, "enqueue broadcast preview", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) recipients(w http.ResponseWriter, r *http.Request) {
	if ids, ok := s.recipientIDs(w, r); ok {
		writeJSON(w, http.StatusOK, map[string]int{"count": len(ids)})
	}
}

func (s *server) createBroadcast(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IdempotencyKey uuid.UUID `json:"idempotencyKey"`
		message
	}
	if !readMessage(w, r, &in, &in.message) {
		return
	}
	if in.IdempotencyKey == uuid.Nil {
		writeBadRequest(w)
		return
	}
	ids, ok := s.recipientIDs(w, r)
	if !ok {
		return
	}
	b, created, err := s.Store.CreateBroadcast(r.Context(),
		model.BroadcastDraft{IdempotencyKey: in.IdempotencyKey, Text: in.Text, Photo: in.Photo}, ids, s.AdminID)
	if err != nil {
		internal(w, r, "create broadcast", err)
		return
	}
	code := http.StatusOK // повтор с тем же ключом — та же рассылка
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]model.Broadcast{"broadcast": b})
}

func (s *server) broadcasts(w http.ResponseWriter, r *http.Request) {
	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		var err error
		if page, err = strconv.Atoi(v); err != nil || page < 1 || page > 1_000_000 {
			writeBadRequest(w)
			return
		}
	}
	items, total, err := s.Store.ListBroadcasts(r.Context(), broadcastsPage, (page-1)*broadcastsPage)
	if err != nil {
		internal(w, r, "list broadcasts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}
