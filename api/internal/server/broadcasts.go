package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/broadcast"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/core/texts"
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

// validateMessage — те же правила, что у текстов-сообщений, переменные только общие; с фото лимит подписи 1024
// по худшему случаю, фото без текста допустимо.
func validateMessage(text string, hasPhoto bool, custom map[string]string) error {
	if hasPhoto && strings.TrimSpace(text) == "" {
		return nil
	}
	def := texts.Def{Kind: texts.KindMessage}
	if err := texts.Validate(def, text, custom); err != nil {
		return err
	}
	if n := texts.WorstLen(def, text, custom); hasPhoto && n > maxCaptionLen {
		return errors.Join(texts.ErrTooLong, fmt.Errorf("подпись к фото может оказаться длиннее %d символов (с самыми длинными значениями переменных — %d); самая длинная — {{.%s}}", maxCaptionLen, n, texts.WidestVar(def, text, custom)))
	}
	return nil
}

// readMessage читает и проверяет message; при ошибке сам отвечает 400.
func (s *server) readMessage(w http.ResponseWriter, r *http.Request, in any, m *message) bool {
	if !readJSONMax(w, r, in, maxTextBody, false) {
		return false
	}
	if strings.TrimSpace(m.Text) == "" {
		m.Text = ""
	}
	cur, _, err := s.current(r.Context())
	if err != nil {
		internal(w, r, "settings", err)
		return false
	}
	if err := validateMessage(m.Text, m.Photo != "", cur.Vars); err != nil {
		writeInvalid(w, texts.Problem(err))
		return false
	}
	return true
}

func (s *server) previewBroadcast(w http.ResponseWriter, r *http.Request) {
	var in message
	if !s.readMessage(w, r, &in, &in) {
		return
	}
	err := s.Store.Enqueue(r.Context(), model.OutboxMessage{
		EventType: "text_test", ChatID: s.AdminID, Text: in.Text, Photo: in.Photo, ParseMode: "HTML", Template: texts.HasVars(in.Text),
	})
	if err != nil {
		internal(w, r, "enqueue broadcast preview", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) recipients(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.AudienceCounts(r.Context(), s.Now(), 0)
	if err != nil {
		internal(w, r, "audience counts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": c.All})
}

// scheduleLayout — datetime-local из панели: московское время без пояса.
const scheduleLayout = "2006-01-02T15:04"

// parseSchedule разбирает «ГГГГ-ММ-ДДTЧЧ:ММ» по Москве (без DST — сдвиг однозначен). Пусто — отправить сейчас (nil).
// Время не позже now — ошибка: прошедшее расписание не принимаем.
func parseSchedule(v string, now time.Time) (*time.Time, error) {
	if v == "" {
		return nil, nil //nolint:nilnil // пусто = «сейчас»
	}
	t, err := time.ParseInLocation(scheduleLayout, v, moscow)
	if err != nil {
		return nil, errors.New("время отправки: формат ГГГГ-ММ-ДДTЧЧ:ММ по московскому времени")
	}
	if !t.After(now) {
		return nil, errors.New("время отправки уже прошло")
	}
	t = t.UTC()
	return &t, nil
}

// broadcastIn — тело POST/PUT рассылки.
type broadcastIn struct {
	IdempotencyKey uuid.UUID `json:"idempotencyKey"`
	message
	ScheduledAt string `json:"scheduledAt"` // МСК, пусто — сразу
}

func (s *server) readBroadcast(w http.ResponseWriter, r *http.Request, in *broadcastIn) (*time.Time, bool) {
	if !s.readMessage(w, r, in, &in.message) {
		return nil, false
	}
	at, err := parseSchedule(in.ScheduledAt, s.Now())
	if err != nil {
		writeInvalid(w, err.Error())
		return nil, false
	}
	return at, true
}

func (s *server) createBroadcast(w http.ResponseWriter, r *http.Request) {
	var in broadcastIn
	at, ok := s.readBroadcast(w, r, &in)
	if !ok {
		return
	}
	if in.IdempotencyKey == uuid.Nil {
		writeBadRequest(w)
		return
	}
	b, created, err := s.Store.SendBroadcast(r.Context(),
		model.BroadcastDraft{IdempotencyKey: in.IdempotencyKey, Text: in.Text, Photo: in.Photo, Audience: broadcast.AudienceAll, ScheduledAt: at},
		s.AdminID, s.Now())
	if err != nil {
		broadcastError(w, r, "create broadcast", err)
		return
	}
	code := http.StatusOK // повтор с тем же ключом — та же рассылка
	if created {
		code = http.StatusCreated
	}
	writeJSON(w, code, map[string]model.Broadcast{"broadcast": b})
}

// broadcastError: стартовавшая/отменённая рассылка — 409, нет такой — 404.
func broadcastError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	switch {
	case errors.Is(err, store.ErrBroadcastLocked):
		writeError(w, http.StatusConflict, "locked")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	default:
		internal(w, r, msg, err)
	}
}

func broadcastID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBadRequest(w)
	}
	return id, err == nil
}

// updateBroadcast меняет запланированную рассылку до старта; стартовала — 409.
func (s *server) updateBroadcast(w http.ResponseWriter, r *http.Request) {
	id, ok := broadcastID(w, r)
	if !ok {
		return
	}
	var in broadcastIn
	at, ok := s.readBroadcast(w, r, &in)
	if !ok {
		return
	}
	if at == nil {
		writeInvalid(w, "у запланированной рассылки нужно время отправки")
		return
	}
	b, err := s.Store.UpdateScheduled(r.Context(), id,
		model.BroadcastDraft{Text: in.Text, Photo: in.Photo, Audience: broadcast.AudienceAll, ScheduledAt: at})
	if err != nil {
		broadcastError(w, r, "update broadcast", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]model.Broadcast{"broadcast": b})
}

func (s *server) cancelBroadcast(w http.ResponseWriter, r *http.Request) {
	id, ok := broadcastID(w, r)
	if !ok {
		return
	}
	b, err := s.Store.CancelBroadcast(r.Context(), id)
	if err != nil {
		broadcastError(w, r, "cancel broadcast", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]model.Broadcast{"broadcast": b})
}

func (s *server) broadcast(w http.ResponseWriter, r *http.Request) {
	id, ok := broadcastID(w, r)
	if !ok {
		return
	}
	b, err := s.Store.Broadcast(r.Context(), id)
	if err != nil {
		broadcastError(w, r, "get broadcast", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]model.Broadcast{"broadcast": b})
}

func (s *server) broadcasts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		var err error
		if page, err = strconv.Atoi(v); err != nil || page < 1 || page > 1_000_000 {
			writeBadRequest(w)
			return
		}
	}
	f := model.BroadcastFilter{Limit: broadcastsPage, Offset: (page - 1) * broadcastsPage}
	if v := q.Get("status"); v != "" {
		f.Statuses = strings.Split(v, ",")
		if slices.ContainsFunc(f.Statuses, func(st string) bool { return !slices.Contains(broadcast.Statuses, st) }) {
			writeBadRequest(w)
			return
		}
	}
	items, total, err := s.Store.ListBroadcasts(r.Context(), f)
	if err != nil {
		internal(w, r, "list broadcasts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}
