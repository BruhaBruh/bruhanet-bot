package server

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/broadcast"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/store"
	"github.com/bruhabruh/bruhanet/core/texts"
	"github.com/bruhabruh/bruhanet/core/usererr"
)

const (
	broadcastsPage = 20
	maxCaptionLen  = 1024 // подпись к фото в Telegram, UTF-16
)

// message — тело рассылки и личного сообщения: Telegram HTML и необязательное фото. Фото — либо PhotoID (картинка,
// загруженная через POST /api/photos), либо Photo (file_id Telegram: старые черновики и дубли старых рассылок); не оба.
type message struct {
	Text    string     `json:"text"`
	Photo   string     `json:"photo"`
	PhotoID *uuid.UUID `json:"photoId"`
}

func (m *message) hasPhoto() bool { return m.Photo != "" || m.PhotoID != nil }

// checkPhotoRef: одно из двух полей, а загруженная картинка должна быть в БД; при ошибке сам отвечает 400.
func (s *server) checkPhotoRef(w http.ResponseWriter, r *http.Request, m *message) bool {
	if m.Photo != "" && m.PhotoID != nil {
		writeInvalid(w, "укажите одно фото: photo или photoId")
		return false
	}
	if m.PhotoID == nil {
		return true
	}
	ok, err := s.Store.PhotoExists(r.Context(), *m.PhotoID)
	if err != nil {
		internal(w, r, "photo exists", err)
		return false
	}
	if !ok {
		writeInvalid(w, "фото не найдено, загрузите его заново")
	}
	return ok
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
		return errors.Join(texts.ErrTooLong, usererr.Errorf("photo caption may exceed %d characters (worst case %d, widest variable %s)", "подпись к фото может оказаться длиннее %d символов (с самыми длинными значениями переменных — %d); самая длинная — {{.%s}}", maxCaptionLen, n, texts.WidestVar(def, text, custom)))
	}
	return nil
}

// readMessage читает и проверяет message; при ошибке сам отвечает 400.
func (s *server) readMessage(w http.ResponseWriter, r *http.Request, in any, m *message) bool {
	if !readJSONMax(w, r, in, maxTextBody, false) {
		return false
	}
	return s.checkMessage(w, r, m)
}

// checkMessage проверяет уже прочитанный message.
func (s *server) checkMessage(w http.ResponseWriter, r *http.Request, m *message) bool {
	if strings.TrimSpace(m.Text) == "" {
		m.Text = ""
	}
	if !s.checkPhotoRef(w, r, m) {
		return false
	}
	cur, _, err := s.current(r.Context())
	if err != nil {
		internal(w, r, "settings", err)
		return false
	}
	if err := validateMessage(m.Text, m.hasPhoto(), cur.Vars); err != nil {
		writeInvalid(w, texts.Problem(err))
		return false
	}
	return true
}

// previewBroadcast шлёт рассылку как есть (с кнопками и «без звука») админу.
func (s *server) previewBroadcast(w http.ResponseWriter, r *http.Request) {
	var in struct {
		message
		Buttons []broadcast.Button `json:"buttons"`
		Silent  bool               `json:"silent"`
	}
	if !s.readMessage(w, r, &in, &in.message) {
		return
	}
	if err := broadcast.ValidateButtons(in.Buttons); err != nil {
		writeInvalid(w, texts.Problem(err))
		return
	}
	err := s.Store.Enqueue(r.Context(), model.OutboxMessage{
		EventType: "text_test", ChatID: s.AdminID, Text: in.Text, Photo: in.Photo, PhotoID: idString(in.PhotoID), ParseMode: "HTML", Template: texts.HasVars(in.Text),
		Keyboard: broadcast.Keyboard(in.Buttons), Silent: in.Silent,
	})
	if err != nil {
		internal(w, r, "enqueue broadcast preview", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// recipients — сколько получателей в аудиториях сейчас. Без audience — все аудитории сразу (expiring — если задан
// корректный days), с audience — одна: {"count": n}. Отправка пересчитывает аудиторию в момент старта.
func (s *server) recipients(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var days *int
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeBadRequest(w)
			return
		}
		days = &n
	}
	aud := q.Get("audience")
	if aud != "" {
		if days != nil && aud != broadcast.AudienceExpiring {
			days = nil // число дней нужно только expiring
		}
		if err := broadcast.ValidateAudience(aud, days); err != nil {
			writeInvalid(w, texts.Problem(err))
			return
		}
	}
	n := 0
	if days != nil && *days >= broadcast.MinDays && *days <= broadcast.MaxDays {
		n = *days
	}
	c, err := s.Store.AudienceCounts(r.Context(), s.Now(), n)
	if err != nil {
		internal(w, r, "audience counts", err)
		return
	}
	switch aud {
	case broadcast.AudienceAll:
		writeJSON(w, http.StatusOK, map[string]int{"count": c.All})
	case broadcast.AudienceActive:
		writeJSON(w, http.StatusOK, map[string]int{"count": c.Active})
	case broadcast.AudienceNoSubscription:
		writeJSON(w, http.StatusOK, map[string]int{"count": c.NoSubscription})
	case broadcast.AudienceExpiring:
		writeJSON(w, http.StatusOK, map[string]int{"count": c.Expiring})
	case broadcast.AudienceTrial:
		writeJSON(w, http.StatusOK, map[string]int{"count": c.Trial})
	default:
		out := map[string]int{"all": c.All, "active": c.Active, "no_subscription": c.NoSubscription, "trial": c.Trial}
		if n > 0 {
			out["expiring"] = c.Expiring
		}
		writeJSON(w, http.StatusOK, out)
	}
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
		return nil, usererr.Errorf("invalid schedule format", "время отправки: формат ГГГГ-ММ-ДДTЧЧ:ММ по московскому времени")
	}
	if !t.After(now) {
		return nil, usererr.Errorf("schedule is in the past", "время отправки уже прошло")
	}
	t = t.UTC()
	return &t, nil
}

// broadcastIn — тело POST/PUT рассылки.
type broadcastIn struct {
	IdempotencyKey uuid.UUID `json:"idempotencyKey"`
	DraftID        uuid.UUID `json:"draftId"` // отправка черновика
	message
	Audience     string             `json:"audience"` // пусто — all
	AudienceDays *int               `json:"audienceDays"`
	Silent       bool               `json:"silent"`
	Buttons      []broadcast.Button `json:"buttons"`
	ScheduledAt  string             `json:"scheduledAt"` // МСК, пусто — сразу
}

// readBroadcast читает и полностью проверяет тело отправки или правки запланированной; при ошибке сам отвечает 400.
func (s *server) readBroadcast(w http.ResponseWriter, r *http.Request, in *broadcastIn) (*time.Time, bool) {
	if !readJSONMax(w, r, in, maxTextBody, false) {
		return nil, false
	}
	return s.checkBroadcast(w, r, in)
}

func (s *server) checkBroadcast(w http.ResponseWriter, r *http.Request, in *broadcastIn) (*time.Time, bool) {
	if !s.checkMessage(w, r, &in.message) {
		return nil, false
	}
	if in.Audience == "" {
		in.Audience = broadcast.AudienceAll
	}
	if err := broadcast.ValidateAudience(in.Audience, in.AudienceDays); err != nil {
		writeInvalid(w, texts.Problem(err))
		return nil, false
	}
	if err := broadcast.ValidateButtons(in.Buttons); err != nil {
		writeInvalid(w, texts.Problem(err))
		return nil, false
	}
	at, err := parseSchedule(in.ScheduledAt, s.Now())
	if err != nil {
		writeInvalid(w, usererr.Message(err))
		return nil, false
	}
	return at, true
}

// Границы полей черновика: он может быть неполным и невалидным (идёт автосохранением), но не безразмерным.
const (
	draftButtonText = 200
	draftButtonURL  = 2048
)

// normalizeDraft приводит тело черновика к хранимому виду. Содержимое не проверяется на правила отправки (они
// сработают при отправке), но неизвестная аудитория, плохое время или слишком длинные поля — 400; кривые дни
// у expiring просто не сохраняются.
func normalizeDraft(in *broadcastIn) (*time.Time, error) {
	if strings.TrimSpace(in.Text) == "" {
		in.Text = ""
	}
	if in.Audience == "" {
		in.Audience = broadcast.AudienceAll
	}
	if !slices.Contains(broadcast.Audiences, in.Audience) {
		return nil, usererr.Errorf("unknown audience", "неизвестная аудитория")
	}
	if in.Audience != broadcast.AudienceExpiring || in.AudienceDays == nil || *in.AudienceDays < broadcast.MinDays || *in.AudienceDays > broadcast.MaxDays {
		in.AudienceDays = nil
	}
	if len(in.Buttons) > broadcast.MaxButtons {
		return nil, usererr.Errorf("too many buttons", "кнопок может быть не больше 3")
	}
	for i, b := range in.Buttons {
		if utf8.RuneCountInString(b.Text) > draftButtonText || len(b.URL) > draftButtonURL {
			return nil, usererr.Errorf("button is too long", "слишком длинная кнопка")
		}
		if !slices.Contains(texts.Styles, b.Style) {
			in.Buttons[i].Style = ""
		}
	}
	if len(in.Photo) > 512 {
		return nil, usererr.Errorf("invalid photo id", "некорректное фото")
	}
	if in.ScheduledAt == "" {
		return nil, nil //nolint:nilnil // у черновика времени может не быть
	}
	t, err := time.ParseInLocation(scheduleLayout, in.ScheduledAt, moscow)
	if err != nil {
		return nil, usererr.Errorf("invalid schedule format", "время отправки: формат ГГГГ-ММ-ДДTЧЧ:ММ по московскому времени")
	}
	t = t.UTC()
	return &t, nil
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
		model.BroadcastDraft{
			IdempotencyKey: in.IdempotencyKey, DraftID: in.DraftID, Text: in.Text, Photo: in.Photo, PhotoID: in.PhotoID, Audience: in.Audience, AudienceDays: in.AudienceDays,
			Silent: in.Silent, Buttons: in.Buttons, ScheduledAt: at,
		},
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
	case errors.Is(err, store.ErrNotDeletable):
		writeError(w, http.StatusConflict, "not_deletable")
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

// updateBroadcast: черновик (или ещё не существующий id — создаётся) сохраняется без проверки содержимого, повтор
// идемпотентен; запланированная проверяется полностью, как при отправке; стартовала или отменена — 409.
func (s *server) updateBroadcast(w http.ResponseWriter, r *http.Request) {
	id, ok := broadcastID(w, r)
	if !ok {
		return
	}
	var in broadcastIn
	if !readJSONMax(w, r, &in, maxTextBody, false) {
		return
	}
	cur, err := s.Store.Broadcast(r.Context(), id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		internal(w, r, "get broadcast", err)
		return
	}
	var b model.Broadcast
	switch {
	case errors.Is(err, store.ErrNotFound) || cur.Status == broadcast.StatusDraft:
		if !s.checkPhotoRef(w, r, &in.message) {
			return
		}
		at, nerr := normalizeDraft(&in)
		if nerr != nil {
			writeInvalid(w, usererr.Message(nerr))
			return
		}
		b, err = s.Store.SaveDraft(r.Context(), id, model.BroadcastDraft{
			Text: in.Text, Photo: in.Photo, PhotoID: in.PhotoID, Audience: in.Audience, AudienceDays: in.AudienceDays,
			Silent: in.Silent, Buttons: in.Buttons, ScheduledAt: at,
		})
	case cur.Status == broadcast.StatusScheduled:
		at, ok := s.checkBroadcast(w, r, &in)
		if !ok {
			return
		}
		if at == nil {
			writeInvalid(w, "у запланированной рассылки нужно время отправки")
			return
		}
		b, err = s.Store.UpdateScheduled(r.Context(), id, model.BroadcastDraft{
			Text: in.Text, Photo: in.Photo, PhotoID: in.PhotoID, Audience: in.Audience, AudienceDays: in.AudienceDays,
			Silent: in.Silent, Buttons: in.Buttons, ScheduledAt: at,
		})
	default:
		writeError(w, http.StatusConflict, "locked")
		return
	}
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

// deleteBroadcast стирает черновик, запланированную или отменённую рассылку; стартовавшая и отправленная — 409
// not_deletable (история со счётчиками), не стартовавшая, но занятая стартом — 409 locked.
func (s *server) deleteBroadcast(w http.ResponseWriter, r *http.Request) {
	id, ok := broadcastID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteBroadcast(r.Context(), id); err != nil {
		broadcastError(w, r, "delete broadcast", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	// Период по created_at: дни по МСК, to включительно; границы необязательны.
	for key, dst := range map[string]*time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(key); v != "" {
			d, err := time.ParseInLocation(time.DateOnly, v, moscow)
			if err != nil {
				writeBadRequest(w)
				return
			}
			*dst = d
		}
	}
	if !f.To.IsZero() {
		f.To = f.To.AddDate(0, 0, 1)
	}
	if !f.From.IsZero() && !f.To.IsZero() && !f.To.After(f.From) {
		writeBadRequest(w)
		return
	}
	items, total, err := s.Store.ListBroadcasts(r.Context(), f)
	if err != nil {
		internal(w, r, "list broadcasts", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

// idString — id картинки для payload outbox; пусто — без картинки.
func idString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
