package outbox

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bruhabruh/bruhanet/bot/internal/content"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
	"github.com/bruhabruh/bruhanet/core/broadcast"
	"github.com/bruhabruh/bruhanet/core/model"
	"github.com/bruhabruh/bruhanet/core/payflow"
	"github.com/bruhabruh/bruhanet/core/settings"
	"github.com/bruhabruh/bruhanet/core/store"
)

type fakeQueue struct {
	mu   sync.Mutex
	next int64
	rows []model.OutboxItem

	broadcasts map[uuid.UUID]*model.Broadcast // счётчики «в БД»
	direct     map[int64]string               // direct_id → статус
	blocked    []int64
	received   map[int64]int
	markErr    error
}

func (q *fakeQueue) MarkBotBlocked(_ context.Context, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.blocked = append(q.blocked, id)
	return q.markErr
}

func (q *fakeQueue) AddBroadcastReceived(_ context.Context, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.received == nil {
		q.received = map[int64]int{}
	}
	q.received[id]++
	return nil
}

func (q *fakeQueue) addBroadcast(total int) uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.broadcasts == nil {
		q.broadcasts = map[uuid.UUID]*model.Broadcast{}
	}
	id := uuid.New()
	q.broadcasts[id] = &model.Broadcast{ID: id, Total: total}
	return id
}

func (q *fakeQueue) BroadcastDelivered(_ context.Context, id uuid.UUID, ok bool) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	b := q.broadcasts[id]
	if b == nil {
		return store.ErrNotFound
	}
	if ok {
		b.Sent++
	} else {
		b.Failed++
	}
	return nil
}

func (q *fakeQueue) FinishBroadcast(_ context.Context, id uuid.UUID) (model.Broadcast, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	b := q.broadcasts[id]
	if b == nil {
		return model.Broadcast{}, store.ErrNotFound
	}
	now := time.Now()
	b.FinishedAt = &now
	return *b, nil
}

func (q *fakeQueue) DirectDelivered(_ context.Context, id int64, ok bool) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.direct == nil {
		q.direct = map[int64]string{}
	}
	q.direct[id] = map[bool]string{true: model.DirectSent, false: model.DirectFailed}[ok]
	return nil
}

func (q *fakeQueue) broadcast(id uuid.UUID) model.Broadcast {
	q.mu.Lock()
	defer q.mu.Unlock()
	return *q.broadcasts[id]
}

func (q *fakeQueue) add(ms ...model.OutboxMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, m := range ms {
		q.next++
		q.rows = append(q.rows, model.OutboxItem{ID: q.next, OutboxMessage: m})
	}
}

func (q *fakeQueue) OutboxBatch(_ context.Context, limit int) ([]model.OutboxItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.rows[:min(limit, len(q.rows))]), nil
}

func (q *fakeQueue) DeleteOutbox(_ context.Context, id int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.rows = slices.DeleteFunc(q.rows, func(i model.OutboxItem) bool { return i.ID == id })
	return nil
}

func (q *fakeQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.rows)
}

// drain гоняет воркер, пока очередь не опустеет (или таймаут), и останавливает его.
func drain(t *testing.T, w *Worker, q *fakeQueue) {
	t.Helper()
	w.interval, w.idle = time.Millisecond, time.Millisecond
	w.backoff = func(int) time.Duration { return time.Millisecond }
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	deadline := time.Now().Add(10 * time.Second)
	for q.len() > 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	if q.len() != 0 {
		t.Fatalf("queue not drained: %d left", q.len())
	}
}

func setup(t *testing.T) (*tgtest.Server, *fakeQueue, *Worker) {
	t.Helper()
	srv := tgtest.NewServer(t)
	q := &fakeQueue{}
	return srv, q, New(q, srv.Bot(t), nil, nil)
}

func count(event, result string) float64 {
	return testutil.ToFloat64(observability.OutboxMessages.WithLabelValues(event, result))
}

// Review Focus 5: строка, поставленная до перехода на HTML (MarkdownV2), уходит как есть.
func TestWorkerSendsAndDeletes(t *testing.T) {
	srv, q, w := setup(t)
	before := count("reminder", "sent")
	q.add(model.OutboxMessage{EventType: "reminder", ChatID: 7, Text: "hi", ParseMode: "MarkdownV2"})
	drain(t, w, q)
	calls := srv.Calls("sendMessage")
	if len(calls) != 1 || calls[0].Params.Get("chat_id") != "7" || calls[0].Params.Get("text") != "hi" ||
		calls[0].Params.Get("parse_mode") != "MarkdownV2" {
		t.Fatalf("calls: %+v", calls)
	}
	if count("reminder", "sent")-before != 1 {
		t.Fatal("sent metric not incremented")
	}
}

func TestWorkerCopy(t *testing.T) {
	srv, q, w := setup(t)
	q.add(model.OutboxMessage{EventType: "broadcast", ChatID: 7, FromChatID: 9, MessageID: 5})
	drain(t, w, q)
	calls := srv.Calls("copyMessage")
	if len(calls) != 1 || calls[0].Params.Get("from_chat_id") != "9" || calls[0].Params.Get("message_id") != "5" {
		t.Fatalf("calls: %+v", calls)
	}
	if len(srv.Calls("sendMessage")) != 0 {
		t.Fatal("unexpected sendMessage")
	}
}

func TestWorkerForbiddenDropped(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(7, 403, "Forbidden: bot was blocked by the user")
	q.add(model.OutboxMessage{EventType: "reminder", ChatID: 7, Text: "hi"})
	drain(t, w, q)
	if n := len(srv.Calls("sendMessage")); n != 1 {
		t.Fatalf("attempts = %d", n)
	}
}

func TestTooManyRequestsParsed(t *testing.T) {
	if _, err := tgtestErr(t); err == nil {
		t.Fatal("want error")
	}
}

func TestWorker429(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChatRetryAfter(7, 1)
	dropped := count("reminder", "dropped")
	q.add(model.OutboxMessage{EventType: "reminder", ChatID: 7, Text: "hi"})
	go func() {
		time.Sleep(300 * time.Millisecond)
		srv.FailChat(7, 0, "")
	}()
	start := time.Now()
	drain(t, w, q)
	if time.Since(start) < time.Second {
		t.Fatalf("no retry_after pause: %v", time.Since(start))
	}
	if n := len(srv.Calls("sendMessage")); n != 2 {
		t.Fatalf("attempts = %d", n)
	}
	if count("reminder", "dropped") != dropped {
		t.Fatal("dropped grew")
	}
}

func TestWorkerGivesUp(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(7, 500, "Internal")
	dropped := count("reminder", "dropped")
	q.add(model.OutboxMessage{EventType: "reminder", ChatID: 7, Text: "hi"})
	drain(t, w, q)
	if n := len(srv.Calls("sendMessage")); n != maxAttempts {
		t.Fatalf("attempts = %d", n)
	}
	if count("reminder", "dropped")-dropped != 1 {
		t.Fatal("dropped not incremented")
	}
}

// Итог рассылки собирается из счётчиков стора; 403 получателя — failed+1.
func TestWorkerBroadcastSummary(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(8, 403, "Forbidden: bot was blocked by the user")
	id := q.addBroadcast(2)
	sent, failed := testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("sent")),
		testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("failed"))
	q.add(
		model.OutboxMessage{EventType: "broadcast", ChatID: 7, Text: "hi", ParseMode: "HTML", Broadcast: id.String()},
		model.OutboxMessage{EventType: "broadcast", ChatID: 8, Text: "hi", ParseMode: "HTML", Broadcast: id.String()},
		model.OutboxMessage{EventType: "broadcast_summary", ChatID: 1, ParseMode: "HTML", Broadcast: id.String()},
	)
	drain(t, w, q)
	if b := q.broadcast(id); b.Sent != 1 || b.Failed != 1 || b.FinishedAt == nil {
		t.Fatalf("store counters: %+v", b)
	}
	calls := srv.Calls("sendMessage")
	last := calls[len(calls)-1]
	if len(calls) != 3 || last.Params.Get("chat_id") != "1" || last.Params.Get("parse_mode") != "HTML" ||
		last.Params.Get("text") != "Разослано: <b>1</b>, не доставлено: <b>1</b>" {
		t.Fatalf("calls: %+v", calls)
	}
	if testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("sent"))-sent != 1 ||
		testutil.ToFloat64(observability.BroadcastMessages.WithLabelValues("failed"))-failed != 1 {
		t.Fatal("broadcast metrics")
	}
}

// Review Focus 2: рестарт посреди рассылки — новый Worker, итог из стора полный.
func TestWorkerBroadcastSurvivesRestart(t *testing.T) {
	srv, q, w := setup(t)
	id := q.addBroadcast(3)
	for _, chat := range []int64{7, 8, 9} {
		q.add(model.OutboxMessage{EventType: "broadcast", ChatID: chat, Text: "hi", ParseMode: "HTML", Broadcast: id.String()})
	}
	q.add(model.OutboxMessage{EventType: "broadcast_summary", ChatID: 1, ParseMode: "HTML", Broadcast: id.String()})

	first, _ := q.OutboxBatch(t.Context(), 2)
	for _, it := range first { // до «рестарта» ушли двое
		if !w.process(t.Context(), it) {
			t.Fatal("process failed")
		}
	}
	w2 := New(q, srv.Bot(t), nil, nil)
	drain(t, w2, q)
	calls := srv.Calls("sendMessage")
	if got := calls[len(calls)-1].Params.Get("text"); got != "Разослано: <b>3</b>, не доставлено: <b>0</b>" {
		t.Fatalf("summary = %q", got)
	}
}

// Строки /a до этого деплоя: рассылки нет в таблице — ничего не считаем, итог-заглушка.
func TestWorkerLegacyBroadcast(t *testing.T) {
	for _, mode := range []string{"HTML", "MarkdownV2"} {
		t.Run(mode, func(t *testing.T) {
			srv, q, w := setup(t)
			legacy := uuid.NewString()
			q.add(
				model.OutboxMessage{EventType: "broadcast", ChatID: 7, FromChatID: 1, MessageID: 2, Broadcast: legacy},
				model.OutboxMessage{EventType: "broadcast", ChatID: 8, FromChatID: 1, MessageID: 2, Broadcast: "not-a-uuid"},
				model.OutboxMessage{EventType: "broadcast_summary", ChatID: 1, ParseMode: mode, Broadcast: legacy},
			)
			drain(t, w, q)
			if n := len(srv.Calls("copyMessage")); n != 2 {
				t.Fatalf("copies = %d", n)
			}
			calls := srv.Calls("sendMessage")
			if len(calls) != 1 || calls[0].Params.Get("text") != "Рассылка завершена" || calls[0].Params.Get("parse_mode") != mode {
				t.Fatalf("calls: %+v", calls)
			}
		})
	}
}

func TestWorkerPhoto(t *testing.T) {
	srv, q, w := setup(t)
	q.add(model.OutboxMessage{EventType: "direct", ChatID: 7, Text: "<b>hi</b>", ParseMode: "HTML", Photo: "file-1", DirectID: 3})
	drain(t, w, q)
	calls := srv.Calls("sendPhoto")
	if len(calls) != 1 || calls[0].Params.Get("chat_id") != "7" || calls[0].Params.Get("photo") != "file-1" ||
		calls[0].Params.Get("caption") != "<b>hi</b>" || calls[0].Params.Get("parse_mode") != "HTML" {
		t.Fatalf("calls: %+v", calls)
	}
	if len(srv.Calls("sendMessage")) != 0 {
		t.Fatal("unexpected sendMessage")
	}
	if q.direct[3] != model.DirectSent {
		t.Fatalf("direct = %v", q.direct)
	}
}

// Рассылка с кнопками и «без звука»: клавиатура и disable_notification уходят и в текст, и в фото; обычное сообщение — без них.
func TestWorkerBroadcastButtonsAndSilent(t *testing.T) {
	srv, q, w := setup(t)
	kb := broadcast.Keyboard([]broadcast.Button{{Text: "Сайт", URL: "https://e.com", Style: "primary"}, {Text: "Ещё", URL: "https://e.org"}})
	q.add(
		model.OutboxMessage{EventType: "broadcast", ChatID: 7, Text: "t", ParseMode: "HTML", Keyboard: kb, Silent: true},
		model.OutboxMessage{EventType: "broadcast", ChatID: 8, Text: "cap", ParseMode: "HTML", Photo: "f1", Keyboard: kb, Silent: true},
		model.OutboxMessage{EventType: "broadcast", ChatID: 9, Text: "plain", ParseMode: "HTML"},
	)
	drain(t, w, q)
	for _, c := range append(srv.Calls("sendMessage"), srv.Calls("sendPhoto")...) {
		chat := c.Params.Get("chat_id")
		markup := c.Params.Get("reply_markup")
		silent := c.Params.Get("disable_notification")
		if chat == "9" {
			if markup != "" || silent == "true" {
				t.Errorf("plain: markup=%q silent=%q", markup, silent)
			}
			continue
		}
		if silent != "true" || !strings.Contains(markup, `"url":"https://e.com"`) || !strings.Contains(markup, `"style":"primary"`) ||
			!strings.Contains(markup, `"text":"Ещё"`) {
			t.Errorf("chat %s: markup=%q silent=%q", chat, markup, silent)
		}
	}
	if len(srv.Calls("sendPhoto")) != 1 || len(srv.Calls("sendMessage")) != 2 {
		t.Fatalf("calls: msg=%d photo=%d", len(srv.Calls("sendMessage")), len(srv.Calls("sendPhoto")))
	}
}

// Review Focus 4: юзер заблокировал бота — личное сообщение помечено «не доставлено».
func TestWorkerDirectForbidden(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(7, 403, "Forbidden: bot was blocked by the user")
	q.add(
		model.OutboxMessage{EventType: "direct", ChatID: 7, Text: "hi", ParseMode: "HTML", DirectID: 1},
		model.OutboxMessage{EventType: "direct", ChatID: 7, Text: "hi", Photo: "f", ParseMode: "HTML", DirectID: 2},
	)
	drain(t, w, q)
	if q.direct[1] != model.DirectFailed || q.direct[2] != model.DirectFailed {
		t.Fatalf("direct = %v", q.direct)
	}
}

func tgtestErr(t *testing.T) (*bot.Bot, error) {
	t.Helper()
	srv := tgtest.NewServer(t)
	srv.FailChatRetryAfter(7, 3)
	b := srv.Bot(t)
	_, err := b.SendMessage(t.Context(), &bot.SendMessageParams{ChatID: 7, Text: "x"})
	var tm *bot.TooManyRequestsError
	if !errors.As(err, &tm) || tm.RetryAfter != 3 {
		t.Fatalf("err = %v", err)
	}
	return b, err
}

// ctxQueue — fakeQueue, у которого DeleteOutbox падает при отменённом ctx и при failDeletes > 0.
type ctxQueue struct {
	*fakeQueue
	failDeletes int
}

func (q *ctxQueue) DeleteOutbox(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.failDeletes > 0 {
		q.failDeletes--
		return errors.New("db down")
	}
	return q.fakeQueue.DeleteOutbox(ctx, id)
}

func TestWorker429ZeroRetryAfterDoesNotSpin(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChatRetryAfter(7, 0)
	it := model.OutboxItem{ID: 1, OutboxMessage: model.OutboxMessage{EventType: "reminder", ChatID: 7, Text: "hi"}}
	q.add(it.OutboxMessage)
	ctx, cancel := context.WithTimeout(t.Context(), 400*time.Millisecond)
	defer cancel()
	w.process(ctx, it)
	if n := len(srv.Calls("sendMessage")); n != 1 {
		t.Fatalf("attempts = %d, want 1 (sleep >= 1s)", n)
	}
}

func TestWorkerDeleteSurvivesCanceledCtx(t *testing.T) {
	_, q, w := setup(t)
	cq := &ctxQueue{fakeQueue: q}
	w.q = cq
	it := model.OutboxItem{ID: 1, OutboxMessage: model.OutboxMessage{EventType: "reminder", ChatID: 7}}
	q.add(it.OutboxMessage)
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // отмена между отправкой и удалением
	if !w.remove(ctx, it, true) || q.len() != 0 {
		t.Fatalf("delete failed under canceled ctx, left=%d", q.len())
	}
}

func TestWorkerTallyAfterDelete(t *testing.T) {
	_, q, w := setup(t)
	cq := &ctxQueue{fakeQueue: q, failDeletes: 1}
	w.q = cq
	id := q.addBroadcast(1)
	it := model.OutboxItem{ID: 1, OutboxMessage: model.OutboxMessage{
		EventType: "broadcast", ChatID: 7, Text: "hi", Broadcast: id.String(),
	}}
	q.add(it.OutboxMessage)
	if w.process(t.Context(), it) {
		t.Fatal("process must report delete failure")
	}
	if b := q.broadcast(id); b.Sent != 0 {
		t.Fatalf("counted before delete: %+v", b)
	}
	if !w.process(t.Context(), it) {
		t.Fatal("second process must succeed")
	}
	if b := q.broadcast(id); b.Sent != 1 {
		t.Fatalf("broadcast = %+v, want sent=1", b)
	}
}

// Одна рассылка = +1 к bot_broadcasts_total при обработке итога, в том числе у строк /a.
func TestWorkerBroadcastsTotal(t *testing.T) {
	_, q, w := setup(t)
	before := testutil.ToFloat64(observability.Broadcasts)
	q.add(
		model.OutboxMessage{EventType: "broadcast_summary", ChatID: 1, ParseMode: "HTML", Broadcast: q.addBroadcast(0).String()},
		model.OutboxMessage{EventType: "broadcast_summary", ChatID: 1, ParseMode: "MarkdownV2", Broadcast: uuid.NewString()},
	)
	drain(t, w, q)
	if got := testutil.ToFloat64(observability.Broadcasts) - before; got != 2 {
		t.Fatalf("broadcasts_total delta = %v, want 2", got)
	}
}

func TestWorkerForbiddenMarksUserBlocked(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(7, 403, "Forbidden: bot was blocked by the user")
	srv.FailChat(8, 400, "Bad Request: chat not found")
	q.add(
		model.OutboxMessage{EventType: "payment_result", ChatID: 7, Text: "x"},
		model.OutboxMessage{EventType: "reminder", ChatID: 8, Text: "x"},
	)
	drain(t, w, q)
	if !slices.Equal(q.blocked, []int64{7}) {
		t.Fatalf("blocked = %v", q.blocked)
	}

	// Сбой записи флага не держит очередь.
	q.markErr = errors.New("db down")
	q.add(model.OutboxMessage{EventType: "reminder", ChatID: 7, Text: "x"})
	drain(t, w, q)
}

func TestWorkerCountsReceivedBroadcasts(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(8, 403, "Forbidden: bot was blocked by the user")
	id := q.addBroadcast(2).String()
	q.add(
		model.OutboxMessage{EventType: "broadcast", ChatID: 7, Text: "x", Broadcast: id},
		model.OutboxMessage{EventType: "broadcast", ChatID: 8, Text: "x", Broadcast: id},
		model.OutboxMessage{EventType: "broadcast", ChatID: 7, Text: "y", Broadcast: "legacy"},
		model.OutboxMessage{EventType: "direct", ChatID: 7, Text: "z", DirectID: 1},
		model.OutboxMessage{EventType: "reminder", ChatID: 7, Text: "z"},
	)
	drain(t, w, q)
	if q.received[7] != 2 || q.received[8] != 0 {
		t.Fatalf("received = %v", q.received)
	}
}

// Рассылка с переменными: на каждого получателя свои значения, значения экранированы, строка без флага — как есть.
func TestWorkerRendersTemplate(t *testing.T) {
	srv := tgtest.NewServer(t)
	q := &fakeQueue{}
	src := &tgtest.Source{}
	src.SetSettings(map[string][]byte{"vars": []byte(`{"my":"x<y"}`)})
	src.SetContact(7, model.UserContact{FirstName: "<Анна>"})
	src.SetContact(8, model.UserContact{FirstName: "Боб"})
	cnt := content.New(src, settings.Defaults(map[int]int{1: 1}, "+7", "Банк", "https://t.me/s"), time.Hour, nil)
	w := New(q, srv.Bot(t), cnt.Render, nil)

	const tpl = "Привет, <b>{{.name}}</b> {{.my}} {{.gone}}"
	q.add(model.OutboxMessage{EventType: "broadcast", ChatID: 7, Text: tpl, ParseMode: "HTML", Template: true})
	q.add(model.OutboxMessage{EventType: "direct", ChatID: 8, Text: tpl, Photo: "AgAD", ParseMode: "HTML", Template: true})
	q.add(model.OutboxMessage{EventType: "broadcast", ChatID: 9, Text: "старая {{.name}}", ParseMode: "HTML"})
	drain(t, w, q)

	msgs := srv.Calls("sendMessage")
	if len(msgs) != 2 {
		t.Fatalf("sendMessage: %+v", msgs)
	}
	if got, want := msgs[0].Params.Get("text"), "Привет, <b>&lt;Анна&gt;</b> x&lt;y "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := msgs[1].Params.Get("text"); got != "старая {{.name}}" {
		t.Fatalf("строка без флага изменилась: %q", got)
	}
	photos := srv.Calls("sendPhoto")
	if len(photos) != 1 || photos[0].Params.Get("caption") != "Привет, <b>Боб</b> x&lt;y " {
		t.Fatalf("sendPhoto: %+v", photos)
	}
}

func TestWorkerPaymentResultRendersOnDelivery(t *testing.T) {
	srv := tgtest.NewServer(t)
	q := &fakeQueue{}
	exp := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	var gotKey string
	var gotExp time.Time
	w := New(q, srv.Bot(t), nil, func(_ context.Context, chatID int64, key string, expireAt time.Time) (string, *models.InlineKeyboardMarkup) {
		gotKey, gotExp = key, expireAt
		return "готово " + strconv.FormatInt(chatID, 10), &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "Меню", CallbackData: "menu"}}}}
	})
	q.add(model.OutboxMessage{EventType: "payment_result", ChatID: 7, TextKey: "payment.confirmed", ExpireAt: exp, ParseMode: "HTML"})
	// строка от прошлого деплоя: текст уже готов
	q.add(model.OutboxMessage{EventType: "payment_result", ChatID: 8, Text: "старая", ParseMode: "HTML"})
	drain(t, w, q)

	msgs := srv.Calls("sendMessage")
	if len(msgs) != 2 || msgs[0].Params.Get("text") != "готово 7" || !strings.Contains(msgs[0].Params.Get("reply_markup"), "Меню") ||
		msgs[1].Params.Get("text") != "старая" {
		t.Fatalf("sendMessage: %+v", msgs)
	}
	if gotKey != "payment.confirmed" || !gotExp.Equal(exp) {
		t.Fatalf("notice(%q, %v)", gotKey, gotExp)
	}
}

func closed(chat int64, msg int, confirmed bool) model.OutboxMessage {
	return model.OutboxMessage{EventType: payflow.EventAdminClosed, ChatID: chat, MessageID: msg, Confirmed: confirmed, Months: 6, Price: 800}
}

func TestWorkerAdminClosed(t *testing.T) {
	srv, q, w := setup(t)
	confirmed := testutil.ToFloat64(observability.Payments.WithLabelValues("confirmed", "6"))
	rubles := testutil.ToFloat64(observability.PaymentsConfirmedRubles.WithLabelValues("6"))
	declined := testutil.ToFloat64(observability.Payments.WithLabelValues("declined", "6"))

	q.add(closed(1, 5, true), closed(1, 6, false), closed(1, 0, true))
	drain(t, w, q)

	edits := srv.Calls("editMessageReplyMarkup")
	if len(edits) != 2 || edits[0].Params.Get("message_id") != "5" || edits[0].Params.Get("chat_id") != "1" ||
		!strings.Contains(edits[0].Params.Get("reply_markup"), `"inline_keyboard":[]`) {
		t.Fatalf("edits: %+v", edits)
	}
	reacts := srv.Calls("setMessageReaction")
	if len(reacts) != 2 || !strings.Contains(reacts[0].Params.Get("reaction"), "👍") || !strings.Contains(reacts[1].Params.Get("reaction"), "👎") {
		t.Fatalf("reactions: %+v", reacts)
	}
	if n := len(srv.Calls("copyMessage")) + len(srv.Calls("sendMessage")); n != 0 {
		t.Fatalf("лишние сообщения: %d", n)
	}
	// метрики — как у решения кнопками, в том числе без сообщения в боте
	if got := testutil.ToFloat64(observability.Payments.WithLabelValues("confirmed", "6")); got != confirmed+2 {
		t.Fatalf("confirmed = %v, was %v", got, confirmed)
	}
	if got := testutil.ToFloat64(observability.PaymentsConfirmedRubles.WithLabelValues("6")); got != rubles+1600 {
		t.Fatalf("rubles = %v, was %v", got, rubles)
	}
	if got := testutil.ToFloat64(observability.Payments.WithLabelValues("declined", "6")); got != declined+1 {
		t.Fatalf("declined = %v, was %v", got, declined)
	}
}

// Кнопок уже нет или сообщение удалено: 400 не мешает очереди, строка уходит.
func TestWorkerAdminClosedBadRequest(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailMethod("editMessageReplyMarkup", 400, "Bad Request: message is not modified")
	q.add(closed(1, 5, true), model.OutboxMessage{EventType: "reminder", ChatID: 9, Text: "дальше"})
	drain(t, w, q)
	if len(srv.Calls("setMessageReaction")) != 1 {
		t.Fatalf("после 400 на кнопках реакция всё равно идёт")
	}
	if msgs := srv.Calls("sendMessage"); len(msgs) != 1 || msgs[0].Params.Get("text") != "дальше" {
		t.Fatalf("очередь встала: %+v", msgs)
	}
}

// 5xx: строка повторяется, после восстановления реакция и снятие кнопок проходят.
func TestWorkerAdminClosedRetries(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailMethod("editMessageReplyMarkup", 500, "Internal")
	q.add(closed(1, 5, false))
	w.backoff = func(int) time.Duration { return time.Millisecond }
	if w.process(t.Context(), q.rows[0]) {
		t.Fatal("сбой должен оставить строку первой")
	}
	if q.len() != 1 || len(srv.Calls("setMessageReaction")) != 0 {
		t.Fatalf("строка удалена при 5xx: %d", q.len())
	}
	srv.FailMethod("editMessageReplyMarkup", 0, "")
	drain(t, w, q)
	if len(srv.Calls("setMessageReaction")) != 1 {
		t.Fatal("нет реакции после восстановления")
	}
}
