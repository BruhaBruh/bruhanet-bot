package outbox

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/bruhabruh/bruhanet/bot/internal/model"
	"github.com/bruhabruh/bruhanet/bot/internal/observability"
	"github.com/bruhabruh/bruhanet/bot/internal/tgtest"
)

type fakeQueue struct {
	mu   sync.Mutex
	next int64
	rows []model.OutboxItem
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
	return srv, q, New(q, srv.Bot(t))
}

func count(event, result string) float64 {
	return testutil.ToFloat64(observability.OutboxMessages.WithLabelValues(event, result))
}

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

func TestWorkerBroadcastSummary(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(8, 403, "Forbidden")
	q.add(
		model.OutboxMessage{EventType: "broadcast", ChatID: 7, FromChatID: 1, MessageID: 2, Broadcast: "u"},
		model.OutboxMessage{EventType: "broadcast", ChatID: 8, FromChatID: 1, MessageID: 2, Broadcast: "u"},
		model.OutboxMessage{EventType: "broadcast_summary", ChatID: 1, ParseMode: "MarkdownV2", Broadcast: "u"},
	)
	drain(t, w, q)
	calls := srv.Calls("sendMessage")
	if len(calls) != 1 || calls[0].Params.Get("chat_id") != "1" ||
		calls[0].Params.Get("text") != "Разослано: *1*, не доставлено: *1*" {
		t.Fatalf("calls: %+v", calls)
	}
	if len(w.stats) != 0 {
		t.Fatal("stats not cleaned")
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
	it := model.OutboxItem{ID: 1, OutboxMessage: model.OutboxMessage{
		EventType: "broadcast", ChatID: 7, FromChatID: 1, MessageID: 2, Broadcast: "u",
	}}
	q.add(it.OutboxMessage)
	if w.process(t.Context(), it) {
		t.Fatal("process must report delete failure")
	}
	if c := w.stats["u"]; c != nil {
		t.Fatalf("counted before delete: %+v", c)
	}
	if !w.process(t.Context(), it) {
		t.Fatal("second process must succeed")
	}
	if c := w.stats["u"]; c == nil || c.sent != 1 {
		t.Fatalf("stats = %+v, want sent=1", c)
	}
}

func TestWorkerSummaryTallyCleanedOnDrop(t *testing.T) {
	srv, q, w := setup(t)
	srv.FailChat(1, 403, "Forbidden")
	w.stats["u"] = &counts{sent: 1}
	it := model.OutboxItem{ID: 1, OutboxMessage: model.OutboxMessage{EventType: "broadcast_summary", ChatID: 1, Broadcast: "u"}}
	q.add(it.OutboxMessage)
	w.process(t.Context(), it)
	if len(w.stats) != 0 {
		t.Fatalf("stats leaked: %v", w.stats)
	}
}
