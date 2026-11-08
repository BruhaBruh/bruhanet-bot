package store

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bruhabruh/bruhanet/core/broadcast"
	"github.com/bruhabruh/bruhanet/core/model"
)

var bt0 = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// seedUser заводит юзера с заданным сроком (nil — без подписки).
func seedUser(t *testing.T, s *Store, id int64, expire *time.Time) {
	t.Helper()
	if err := s.TouchUser(t.Context(), model.UserSeen{TelegramID: id, At: bt0}); err != nil {
		t.Fatal(err)
	}
	if expire != nil {
		if err := s.SetUserPanel(t.Context(), id, *expire, "ACTIVE", bt0); err != nil {
			t.Fatal(err)
		}
	}
}

func after(d time.Duration) *time.Time { v := bt0.Add(d); return &v }

func draft(text string) model.BroadcastDraft {
	return model.BroadcastDraft{IdempotencyKey: uuid.New(), Text: text, Audience: broadcast.AudienceAll}
}

func outboxChats(t *testing.T, s *Store, event string) []int64 {
	t.Helper()
	got, err := s.OutboxBatch(t.Context(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	var out []int64
	for _, m := range got {
		if m.EventType == event {
			out = append(out, m.ChatID)
		}
	}
	return out
}

// Флаг шаблона — только у текста с переменными: обычный текст воркер не трогает и users не читает.
func TestTemplateFlag(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 10, nil)
	if _, _, err := s.SendBroadcast(ctx, draft("Привет, {{.name}}"), 1, bt0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SendBroadcast(ctx, draft("обычный {{ текст"), 1, bt0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDirect(ctx, 10, "{{.expireAt}}", "", nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]bool{}
	for _, m := range got {
		flags[m.Text] = m.Template
	}
	if !flags["Привет, {{.name}}"] || flags["обычный {{ текст"] || !flags["{{.expireAt}}"] {
		t.Fatalf("flags = %v", flags)
	}
}

// Review Focus: повтор с тем же ключом не создаёт ни рассылки, ни строк outbox; получатели считаются из users на момент отправки.
func TestSendBroadcastIdempotent(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 10, nil)
	seedUser(t, s, 20, nil)
	d := draft("<b>hi</b>")
	d.Photo = "file1"
	d.Silent = true
	d.Buttons = []broadcast.Button{{Text: "Сайт", URL: "https://e.com", Style: "primary"}, {Text: "Ещё", URL: "https://e.com/2", Inline: true}}
	b, created, err := s.SendBroadcast(ctx, d, 1, bt0)
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if b.Status != broadcast.StatusSending || b.Total != 2 || b.Text != d.Text || b.Photo != "file1" || b.FinishedAt != nil ||
		!b.Silent || len(b.Buttons) != 2 || !b.Buttons[1].Inline || b.Buttons[0].Inline || b.ScheduledAt != nil || b.Audience != "all" {
		t.Fatalf("broadcast: %+v", b)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("outbox %+v err=%v", got, err)
	}
	// OutboxBatch отдаёт сначала всё, что не рассылка (итог админу), затем строки рассылки: [итог, 10, 20]
	for i, chat := range []int64{10, 20} {
		m := got[i+1]
		if m.EventType != "broadcast" || m.ChatID != chat || m.Text != d.Text || m.Photo != "file1" || !m.Silent ||
			m.ParseMode != "HTML" || m.Broadcast != b.ID.String() ||
			m.Keyboard == nil || m.Keyboard.InlineKeyboard[0][0].URL != "https://e.com" || m.Keyboard.InlineKeyboard[0][0].Style != "primary" {
			t.Fatalf("row %d: %+v", i+1, m)
		}
	}
	if m := got[0]; m.EventType != "broadcast_summary" || m.ChatID != 1 || m.Broadcast != b.ID.String() || m.ParseMode != "HTML" || m.Keyboard != nil {
		t.Fatalf("summary: %+v", m)
	}

	seedUser(t, s, 30, nil)
	again, created, err := s.SendBroadcast(ctx, model.BroadcastDraft{IdempotencyKey: d.IdempotencyKey, Text: "other", Audience: "all"}, 1, bt0)
	if err != nil || created || again.ID != b.ID || again.Text != d.Text || again.Total != 2 {
		t.Fatalf("again=%+v created=%v err=%v", again, created, err)
	}
	if n, _ := s.OutboxCount(ctx); n != 3 {
		t.Fatalf("outbox = %d after repeat", n)
	}
}

func TestSendBroadcastRejectsBadAudience(t *testing.T) {
	s := testStore(t)
	d := draft("x")
	d.Audience = broadcast.AudienceExpiring // без дней
	if _, _, err := s.SendBroadcast(t.Context(), d, 1, bt0); !errors.Is(err, broadcast.ErrAudience) {
		t.Fatalf("err = %v", err)
	}
	if n, _ := s.OutboxCount(t.Context()); n != 0 {
		t.Fatal("outbox must stay empty")
	}
}

// Review Focus 2: счётчики в БД, итог из них; завершение переводит sending в sent.
func TestBroadcastCounters(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	for _, id := range []int64{1, 2, 3} {
		seedUser(t, s, id, nil)
	}
	b, _, err := s.SendBroadcast(ctx, draft("x"), 9, bt0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ok := range []bool{true, true, false} {
		if err = s.BroadcastDelivered(ctx, b.ID, ok); err != nil {
			t.Fatal(err)
		}
	}
	f, err := s.FinishBroadcast(ctx, b.ID)
	if err != nil || f.Sent != 2 || f.Failed != 1 || f.Total != 3 || f.FinishedAt == nil || f.Photo != "" || f.Status != broadcast.StatusSent {
		t.Fatalf("finish %+v err=%v", f, err)
	}
	if err = s.BroadcastDelivered(ctx, uuid.New(), true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown delivered: %v", err)
	}
	if _, err = s.FinishBroadcast(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown finish: %v", err)
	}
}

func TestListBroadcasts(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, nil)
	ids := make([]uuid.UUID, 0, 3)
	for range 3 {
		b, _, err := s.SendBroadcast(ctx, draft("x"), 9, bt0)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}
	got, total, err := s.ListBroadcasts(ctx, model.BroadcastFilter{Limit: 2})
	if err != nil || total != 3 || len(got) != 2 || got[0].ID != ids[2] || got[1].ID != ids[1] {
		t.Fatalf("page1 %+v total=%d err=%v", got, total, err)
	}
	got, total, err = s.ListBroadcasts(ctx, model.BroadcastFilter{Limit: 2, Offset: 2})
	if err != nil || total != 3 || len(got) != 1 || got[0].ID != ids[0] {
		t.Fatalf("page2 %+v total=%d err=%v", got, total, err)
	}
}

func TestListBroadcastsFilters(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, nil)
	sent, _, _ := s.SendBroadcast(ctx, draft("sent"), 9, bt0)
	late := draft("late")
	late.ScheduledAt = after(48 * time.Hour)
	later := draft("later")
	later.ScheduledAt = after(24 * time.Hour)
	l1, _, _ := s.SendBroadcast(ctx, late, 9, bt0)
	l2, _, _ := s.SendBroadcast(ctx, later, 9, bt0)
	d, err := s.SaveDraft(ctx, uuid.New(), draft("черновик"))
	if err != nil {
		t.Fatal(err)
	}

	ids := func(f model.BroadcastFilter) []uuid.UUID {
		f.Limit = 50
		got, total, err := s.ListBroadcasts(ctx, f)
		if err != nil || total != len(got) {
			t.Fatalf("%+v: total=%d len=%d err=%v", f, total, len(got), err)
		}
		out := make([]uuid.UUID, len(got))
		for i, b := range got {
			out[i] = b.ID
		}
		return out
	}
	if got := ids(model.BroadcastFilter{Statuses: []string{"scheduled"}}); len(got) != 2 || got[0] != l2.ID || got[1] != l1.ID {
		t.Fatalf("scheduled must go by start time: %v", got)
	}
	if got := ids(model.BroadcastFilter{Statuses: []string{"sending", "sent"}}); len(got) != 1 || got[0] != sent.ID {
		t.Fatalf("sent: %v", got)
	}
	if got := ids(model.BroadcastFilter{Statuses: []string{"draft"}}); len(got) != 1 || got[0] != d.ID {
		t.Fatalf("draft: %v", got)
	}
	// период по created_at: [From, To)
	var created time.Time
	if err := s.pool.QueryRow(ctx, `SELECT created_at FROM broadcasts WHERE id = $1`, sent.ID).Scan(&created); err != nil {
		t.Fatal(err)
	}
	if got := ids(model.BroadcastFilter{From: created.Add(-time.Hour), To: created.Add(time.Hour)}); len(got) != 4 {
		t.Fatalf("period: %v", got)
	}
	if got := ids(model.BroadcastFilter{From: created.Add(time.Hour)}); len(got) != 0 {
		t.Fatalf("from in future: %v", got)
	}
	if got := ids(model.BroadcastFilter{To: created.Add(-time.Hour)}); len(got) != 0 {
		t.Fatalf("to in past: %v", got)
	}
}

// Запланированная рассылка не уходит раньше времени, стартует ровно один раз и фиксирует получателей на момент старта.
func TestClaimDue(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 10, nil)
	d := draft("позже")
	d.ScheduledAt = after(time.Hour)
	b, created, err := s.SendBroadcast(ctx, d, 1, bt0)
	if err != nil || !created || b.Status != broadcast.StatusScheduled || b.Total != 0 || b.ScheduledAt == nil {
		t.Fatalf("scheduled %+v created=%v err=%v", b, created, err)
	}
	// двойной клик: тот же ключ — та же рассылка, одна строка
	if again, created, err := s.SendBroadcast(ctx, d, 1, bt0); err != nil || created || again.ID != b.ID {
		t.Fatalf("repeat %+v created=%v err=%v", again, created, err)
	}
	if n, _ := s.OutboxCount(ctx); n != 0 {
		t.Fatalf("nothing is enqueued before the time: %d", n)
	}
	if n, err := s.ClaimDue(ctx, 1, bt0.Add(59*time.Minute)); err != nil || n != 0 {
		t.Fatalf("early claim n=%d err=%v", n, err)
	}
	seedUser(t, s, 20, nil) // пришёл после планирования — попадает: аудитория считается при старте
	n, err := s.ClaimDue(ctx, 1, bt0.Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("claim n=%d err=%v", n, err)
	}
	got, _ := s.Broadcast(ctx, b.ID)
	if got.Status != broadcast.StatusSending || got.Total != 2 {
		t.Fatalf("after claim %+v", got)
	}
	if chats := outboxChats(t, s, "broadcast"); len(chats) != 2 {
		t.Fatalf("recipients = %v", chats)
	}
	if n, err := s.ClaimDue(ctx, 1, bt0.Add(2*time.Hour)); err != nil || n != 0 {
		t.Fatalf("second claim n=%d err=%v", n, err)
	}
	if c := outboxChats(t, s, "broadcast_summary"); len(c) != 1 {
		t.Fatalf("summaries = %v", c)
	}
}

// Review Focus: два воркера и рестарт не стартуют одну рассылку дважды (реальный Postgres, -race).
func TestClaimDueConcurrent(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	for id := int64(1); id <= 5; id++ {
		seedUser(t, s, id, nil)
	}
	const broadcasts = 4
	for range broadcasts {
		d := draft("x")
		d.ScheduledAt = after(time.Minute)
		if _, _, err := s.SendBroadcast(ctx, d, 99, bt0); err != nil {
			t.Fatal(err)
		}
	}
	var (
		wg    sync.WaitGroup
		total atomic.Int64
	)
	for range 8 {
		wg.Go(func() {
			n, err := s.ClaimDue(ctx, 99, bt0.Add(time.Hour))
			if err != nil {
				t.Error(err)
			}
			total.Add(int64(n))
		})
	}
	wg.Wait()
	if total.Load() != broadcasts {
		t.Fatalf("started %d, want %d", total.Load(), broadcasts)
	}
	per := map[string]int{}
	got, _ := s.OutboxBatch(ctx, 1000)
	for _, m := range got {
		per[m.EventType+m.Broadcast]++
	}
	if len(per) != broadcasts*2 {
		t.Fatalf("outbox groups = %d", len(per))
	}
	for k, n := range per {
		if (strings.HasPrefix(k, "broadcast_summary") && n != 1) || (!strings.HasPrefix(k, "broadcast_summary") && n != 5) {
			t.Fatalf("%s: %d rows", k, n)
		}
	}
}

// Правка и отмена гоняются со стартом: старт выигрывает целиком, правка/отмена получают ErrBroadcastLocked.
func TestEditAndCancelVersusClaim(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, nil)
	d := draft("v1")
	d.ScheduledAt = after(time.Minute)
	b, _, err := s.SendBroadcast(ctx, d, 9, bt0)
	if err != nil {
		t.Fatal(err)
	}

	edit := draft("v2")
	edit.Silent = true
	edit.ScheduledAt = after(2 * time.Hour)
	e, err := s.UpdateScheduled(ctx, b.ID, edit)
	if err != nil || e.Text != "v2" || !e.Silent || !e.ScheduledAt.Equal(*edit.ScheduledAt) || e.Status != broadcast.StatusScheduled {
		t.Fatalf("edit %+v err=%v", e, err)
	}
	if n, _ := s.ClaimDue(ctx, 9, bt0.Add(time.Hour)); n != 0 {
		t.Fatal("moved time must be honoured")
	}
	if n, _ := s.ClaimDue(ctx, 9, bt0.Add(3*time.Hour)); n != 1 {
		t.Fatal("must start")
	}
	if _, err = s.UpdateScheduled(ctx, b.ID, edit); !errors.Is(err, ErrBroadcastLocked) {
		t.Fatalf("edit after start: %v", err)
	}
	if _, err = s.CancelBroadcast(ctx, b.ID); !errors.Is(err, ErrBroadcastLocked) {
		t.Fatalf("cancel after start: %v", err)
	}
	if _, err = s.UpdateScheduled(ctx, uuid.New(), edit); !errors.Is(err, ErrNotFound) {
		t.Fatalf("edit unknown: %v", err)
	}
	if got := outboxChats(t, s, "broadcast"); len(got) != 1 {
		t.Fatalf("recipients = %v", got)
	}
	// содержимое v2 ушло получателю
	all, _ := s.OutboxBatch(ctx, 10)
	i := slices.IndexFunc(all, func(m model.OutboxItem) bool { return m.EventType == "broadcast" })
	if i < 0 || all[i].Text != "v2" || !all[i].Silent {
		t.Fatalf("sent %+v", all)
	}

	// отмена: больше не стартует, повтор безвреден
	d2 := draft("cancel me")
	d2.ScheduledAt = after(time.Minute)
	c, _, _ := s.SendBroadcast(ctx, d2, 9, bt0)
	cc, err := s.CancelBroadcast(ctx, c.ID)
	if err != nil || cc.Status != broadcast.StatusCancelled {
		t.Fatalf("cancel %+v err=%v", cc, err)
	}
	if _, err = s.CancelBroadcast(ctx, c.ID); err != nil {
		t.Fatalf("repeat cancel: %v", err)
	}
	if _, err = s.UpdateScheduled(ctx, c.ID, edit); !errors.Is(err, ErrBroadcastLocked) {
		t.Fatalf("edit cancelled: %v", err)
	}
	before, _ := s.OutboxCount(ctx)
	if n, _ := s.ClaimDue(ctx, 9, bt0.Add(24*time.Hour)); n != 0 {
		t.Fatal("cancelled must not start")
	}
	if after, _ := s.OutboxCount(ctx); after != before {
		t.Fatal("outbox changed")
	}
}

// Параллельные правка и старт не теряют и не дублируют: исход ровно один из двух.
func TestEditRacesClaim(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, nil)
	for i := range 20 {
		d := draft("v1")
		d.ScheduledAt = after(time.Minute)
		b, _, err := s.SendBroadcast(ctx, d, 9, bt0)
		if err != nil {
			t.Fatal(err)
		}
		edit := draft("v2")
		edit.ScheduledAt = after(time.Minute)
		var (
			wg      sync.WaitGroup
			editErr error
		)
		wg.Go(func() { _, editErr = s.UpdateScheduled(ctx, b.ID, edit) })
		wg.Go(func() { _, _ = s.ClaimDue(ctx, 9, bt0.Add(time.Hour)) })
		wg.Wait()
		// правка держала блокировку — старт её пропустил (SKIP LOCKED) и подберёт на следующем тике
		_, _ = s.ClaimDue(ctx, 9, bt0.Add(time.Hour))
		got, _ := s.Broadcast(ctx, b.ID)
		if got.Status != broadcast.StatusSending {
			t.Fatalf("iter %d: status %s", i, got.Status)
		}
		// что ушло получателю, то и записано в рассылке
		var sentText string
		batch, _ := s.OutboxBatch(ctx, 1000)
		for _, m := range batch {
			if m.Broadcast == b.ID.String() && m.EventType == "broadcast" {
				sentText = m.Text
			}
		}
		if sentText != got.Text || (editErr == nil) != (got.Text == "v2") {
			t.Fatalf("iter %d: sent %q row %q editErr %v", i, sentText, got.Text, editErr)
		}
		for _, m := range batch {
			_ = s.DeleteOutbox(ctx, m.ID)
		}
	}
}

func TestAudiences(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, after(30*24*time.Hour)) // активный, далеко
	seedUser(t, s, 2, after(3*24*time.Hour))  // активный, истекает через 3 дня
	seedUser(t, s, 3, after(-time.Hour))      // истёк
	seedUser(t, s, 4, nil)                    // не было подписки
	seedUser(t, s, 5, after(24*time.Hour))    // активный, через сутки: граница 1 дня
	seedUser(t, s, 6, after(time.Hour))       // заблокировал бота
	seedUser(t, s, 7, after(time.Hour))       // заблокирован админом
	seedUser(t, s, 8, after(0))               // срок ровно сейчас — не активен
	if err := s.MarkBotBlocked(ctx, 6); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE users SET blocked_at = now() WHERE telegram_id = 7`); err != nil {
		t.Fatal(err)
	}

	c, err := s.AudienceCounts(ctx, bt0, 3)
	if err != nil || c != (model.AudienceCounts{All: 6, Active: 3, NoSubscription: 3, Expiring: 2}) {
		t.Fatalf("counts %+v err=%v", c, err)
	}
	if c, _ := s.AudienceCounts(ctx, bt0, 0); c.Expiring != 0 {
		t.Fatalf("no days: %+v", c)
	}
	days := func(n int) *int { return &n }
	for _, tc := range []struct {
		aud  string
		days *int
		want []int64
	}{
		{"all", nil, []int64{1, 2, 3, 4, 5, 8}},
		{"active", nil, []int64{1, 2, 5}},
		{"no_subscription", nil, []int64{3, 4, 8}},
		{"expiring", days(3), []int64{2, 5}},
		{"expiring", days(1), []int64{5}},
		{"expiring", days(60), []int64{1, 2, 5}},
	} {
		got, err := recipients(ctx, s.pool, tc.aud, tc.days, bt0)
		if err != nil || !slices.Equal(got, tc.want) {
			t.Errorf("%s/%v: got %v want %v err=%v", tc.aud, tc.days, got, tc.want, err)
		}
		// подсчёт на экране совпадает с реальными получателями при отправке
		d := draft("x")
		d.Audience, d.AudienceDays = tc.aud, tc.days
		b, _, err := s.SendBroadcast(ctx, d, 99, bt0)
		if err != nil || b.Total != len(tc.want) {
			t.Errorf("%s: total %d want %d err=%v", tc.aud, b.Total, len(tc.want), err)
		}
	}
}

// Черновик: создаётся по id клиента, повторное сохранение — то же, отправка выдаёт ключ; ушедший не правится.
func TestDraftLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, nil)
	id := uuid.New()
	empty := model.BroadcastDraft{Audience: "all"}
	d, err := s.SaveDraft(ctx, id, empty) // пустой черновик допустим
	if err != nil || d.Status != broadcast.StatusDraft || d.Text != "" {
		t.Fatalf("draft %+v err=%v", d, err)
	}
	upd := draft("правка")
	upd.Buttons = []broadcast.Button{{Text: "a", URL: "https://e.com"}}
	d2, err := s.SaveDraft(ctx, id, upd)
	if err != nil || d2.ID != id || d2.Text != "правка" || len(d2.Buttons) != 1 {
		t.Fatalf("update %+v err=%v", d2, err)
	}
	if n, _ := s.OutboxCount(ctx); n != 0 {
		t.Fatal("draft must not enqueue")
	}

	send := draft("правка")
	send.DraftID = id
	b, created, err := s.SendBroadcast(ctx, send, 9, bt0)
	if err != nil || !created || b.ID != id || b.Status != broadcast.StatusSending || b.Total != 1 {
		t.Fatalf("send %+v created=%v err=%v", b, created, err)
	}
	if again, created, err := s.SendBroadcast(ctx, send, 9, bt0); err != nil || created || again.ID != id {
		t.Fatalf("repeat %+v created=%v err=%v", again, created, err)
	}
	if _, err = s.SaveDraft(ctx, id, empty); !errors.Is(err, ErrBroadcastLocked) {
		t.Fatalf("save after send: %v", err)
	}
	other := draft("другое")
	other.DraftID = id
	if _, _, err = s.SendBroadcast(ctx, other, 9, bt0); !errors.Is(err, ErrBroadcastLocked) {
		t.Fatalf("send sent draft with new key: %v", err)
	}
	missing := draft("нет")
	missing.DraftID = uuid.New()
	if _, _, err = s.SendBroadcast(ctx, missing, 9, bt0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing draft: %v", err)
	}
	if n, _ := s.OutboxCount(ctx); n != 2 {
		t.Fatalf("outbox = %d", n)
	}
	// черновик можно отменить; пустой текст вне черновика не пройдёт CHECK
	c, err := s.SaveDraft(ctx, uuid.New(), empty)
	if err != nil {
		t.Fatal(err)
	}
	if cc, err := s.CancelBroadcast(ctx, c.ID); err != nil || cc.Status != broadcast.StatusCancelled {
		t.Fatalf("cancel draft %+v err=%v", cc, err)
	}
	blank := draft("")
	if _, _, err = s.SendBroadcast(ctx, blank, 9, bt0); err == nil {
		t.Fatal("empty non-draft must be refused")
	}
}

// Миграция: существующие строки получают статус по finished_at.
func TestMigrationBackfillsStatus(t *testing.T) {
	s := testStoreRaw(t)
	ctx := t.Context()
	p, err := s.provider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.UpTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	done, running := uuid.New(), uuid.New()
	for _, q := range []string{
		`INSERT INTO broadcasts (id, idempotency_key, text, total, finished_at) VALUES ('` + done.String() + `', gen_random_uuid(), 'a', 1, now())`,
		`INSERT INTO broadcasts (id, idempotency_key, text, total) VALUES ('` + running.String() + `', gen_random_uuid(), 'b', 1)`,
	} {
		if _, err = s.pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[uuid.UUID]string{done: "sent", running: "sending"} {
		b, err := s.Broadcast(ctx, id)
		if err != nil || b.Status != want || b.Audience != "all" || b.Buttons == nil || b.Silent {
			t.Fatalf("%s: %+v err=%v", want, b, err)
		}
	}
}

func TestDirectMessages(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	id1, err := s.CreateDirect(ctx, 5, "hello", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.CreateDirect(ctx, 5, "", "pic", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateDirect(ctx, 6, "other", "", nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.OutboxBatch(ctx, 10)
	if err != nil || len(got) != 3 {
		t.Fatalf("outbox %+v err=%v", got, err)
	}
	if m := got[1]; m.EventType != "direct" || m.ChatID != 5 || m.Photo != "pic" || m.DirectID != id2 || m.ParseMode != "HTML" {
		t.Fatalf("direct row: %+v", m)
	}

	if err = s.DirectDelivered(ctx, id1, true); err != nil {
		t.Fatal(err)
	}
	if err = s.DirectDelivered(ctx, id2, false); err != nil {
		t.Fatal(err)
	}
	if err = s.DirectDelivered(ctx, 999, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	ms, err := s.DirectMessages(ctx, 5)
	if err != nil || len(ms) != 2 {
		t.Fatalf("messages %+v err=%v", ms, err)
	}
	if ms[0].ID != id2 || ms[0].Status != model.DirectFailed || ms[0].Photo != "pic" ||
		ms[1].ID != id1 || ms[1].Status != model.DirectSent || ms[1].Text != "hello" || ms[1].TelegramID != 5 || ms[1].CreatedAt.IsZero() {
		t.Fatalf("messages: %+v", ms)
	}
	if ms, err = s.DirectMessages(ctx, 7); err != nil || len(ms) != 0 {
		t.Fatalf("empty: %+v err=%v", ms, err)
	}
}

// Рассылка, которая стабильно не стартует, не держит очередь: следующие уходят, а после maxClaimFails её отменяют.
func TestClaimDueSkipsPoisoned(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 10, nil)
	// самая ранняя: expiring без числа дней — ValidateAudience отказывает
	poisonID := uuid.New()
	if _, err := s.pool.Exec(ctx, `INSERT INTO broadcasts (id, idempotency_key, status, text, audience, scheduled_at, total)
		VALUES ($1, $2, 'scheduled', 'x', 'expiring', $3, 0)`, poisonID, uuid.New(), bt0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	d := draft("нормальная")
	d.ScheduledAt = after(time.Hour)
	good, _, err := s.SendBroadcast(ctx, d, 1, bt0)
	if err != nil {
		t.Fatal(err)
	}
	now := bt0.Add(2 * time.Hour)
	for i := 1; i <= maxClaimFails; i++ {
		n, err := s.ClaimDue(ctx, 1, now)
		if err == nil {
			t.Fatalf("pass %d: poisoned error must be reported", i)
		}
		wantN := 0
		if i == 1 {
			wantN = 1 // хорошая стартует в первом же проходе, несмотря на отравленную впереди
		}
		if n != wantN {
			t.Fatalf("pass %d: n=%d want %d", i, n, wantN)
		}
	}
	if g, _ := s.Broadcast(ctx, good.ID); g.Status != broadcast.StatusSending {
		t.Fatalf("good status = %s", g.Status)
	}
	if p, _ := s.Broadcast(ctx, poisonID); p.Status != broadcast.StatusCancelled {
		t.Fatalf("poisoned status = %s", p.Status)
	}
	if n, err := s.ClaimDue(ctx, 1, now); err != nil || n != 0 {
		t.Fatalf("after cancel n=%d err=%v", n, err)
	}
}

func TestDeleteBroadcast(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, nil)
	photo, err := s.SavePhoto(ctx, []byte("IMG"), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	// черновик с картинкой, запланированная, отменённая: удаляются, outbox не трогается
	dd := draft("draft")
	dd.PhotoID = &photo
	dr, err := s.SaveDraft(ctx, uuid.New(), dd)
	if err != nil {
		t.Fatal(err)
	}
	ds := draft("later")
	ds.ScheduledAt = after(time.Hour)
	sc, _, _ := s.SendBroadcast(ctx, ds, 9, bt0)
	dc := draft("cancelled")
	dc.ScheduledAt = after(time.Hour)
	ca, _, _ := s.SendBroadcast(ctx, dc, 9, bt0)
	if _, err = s.CancelBroadcast(ctx, ca.ID); err != nil {
		t.Fatal(err)
	}
	before, _ := s.OutboxCount(ctx)
	for _, b := range []model.Broadcast{dr, sc, ca} {
		if err = s.DeleteBroadcast(ctx, b.ID); err != nil {
			t.Fatalf("delete %s: %v", b.Status, err)
		}
		if _, err = s.Broadcast(ctx, b.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("still there: %v", err)
		}
		if err = s.DeleteBroadcast(ctx, b.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("repeat: %v", err)
		}
	}
	if after, _ := s.OutboxCount(ctx); after != before {
		t.Fatalf("outbox changed %d -> %d", before, after)
	}
	// картинка осталась без ссылок: суточная чистка заберёт
	if n, err := s.DeleteStalePhotos(ctx, time.Now().Add(time.Second)); err != nil || n != 1 {
		t.Fatalf("stale photos removed %d, %v", n, err)
	}

	// стартовавшая и завершённая — история
	for _, finish := range []bool{false, true} {
		d := draft("go")
		d.ScheduledAt = after(time.Minute)
		b, _, _ := s.SendBroadcast(ctx, d, 9, bt0)
		if n, _ := s.ClaimDue(ctx, 9, bt0.Add(time.Hour)); n != 1 {
			t.Fatal("must start")
		}
		if finish {
			if _, err = s.FinishBroadcast(ctx, b.ID); err != nil {
				t.Fatal(err)
			}
		}
		if err = s.DeleteBroadcast(ctx, b.ID); !errors.Is(err, ErrNotDeletable) {
			t.Fatalf("finish=%v: %v", finish, err)
		}
		if _, err = s.Broadcast(ctx, b.ID); err != nil {
			t.Fatalf("history lost: %v", err)
		}
	}
}

// Удаление и старт идут параллельно: исход ровно один — удалена без следа либо стартовала и удалить нельзя.
func TestDeleteRacesClaim(t *testing.T) {
	s := testStore(t)
	ctx := t.Context()
	seedUser(t, s, 1, nil)
	for i := range 20 {
		d := draft("v")
		d.ScheduledAt = after(time.Minute)
		b, _, err := s.SendBroadcast(ctx, d, 9, bt0)
		if err != nil {
			t.Fatal(err)
		}
		var (
			wg     sync.WaitGroup
			delErr error
		)
		wg.Go(func() { delErr = s.DeleteBroadcast(ctx, b.ID) })
		wg.Go(func() { _, _ = s.ClaimDue(ctx, 9, bt0.Add(time.Hour)) })
		wg.Wait()
		_, _ = s.ClaimDue(ctx, 9, bt0.Add(time.Hour)) // пропущенную удалением строки уже нет
		got, getErr := s.Broadcast(ctx, b.ID)
		batch, _ := s.OutboxBatch(ctx, 1000)
		sent := 0
		for _, m := range batch {
			if m.Broadcast == b.ID.String() {
				sent++
			}
		}
		switch {
		case delErr == nil:
			if !errors.Is(getErr, ErrNotFound) || sent != 0 {
				t.Fatalf("iter %d: deleted but row err=%v outbox=%d", i, getErr, sent)
			}
		case errors.Is(delErr, ErrNotDeletable):
			if getErr != nil || got.Status != broadcast.StatusSending || sent == 0 {
				t.Fatalf("iter %d: refused but %+v err=%v outbox=%d", i, got, getErr, sent)
			}
		default:
			t.Fatalf("iter %d: %v", i, delErr)
		}
		for _, m := range batch {
			_ = s.DeleteOutbox(ctx, m.ID)
		}
	}
}
