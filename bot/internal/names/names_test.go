package names

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

type saved struct {
	username, first, last string
}

type fakeStore struct {
	ids   []int64
	saved map[int64]saved
	limit int
	order []int64
}

func (f *fakeStore) NamesDue(_ context.Context, _ time.Time, limit int) ([]int64, error) {
	f.limit = limit
	return f.ids, nil
}

func (f *fakeStore) SaveChatName(_ context.Context, id int64, u, first, last string, _ time.Time) error {
	f.saved[id] = saved{u, first, last}
	f.order = append(f.order, id)
	return nil
}

func run(ids []int64, res map[int64]error, chats map[int64]Chat) (*fakeStore, []int64) {
	f := &fakeStore{ids: ids, saved: map[int64]saved{}}
	var asked []int64
	j := New(f, func(_ context.Context, id int64) (Chat, error) {
		asked = append(asked, id)
		return chats[id], res[id]
	}, nil)
	j.gap = 0
	j.Run(context.Background())
	return f, asked
}

func TestRunFillsAndMarksRefusals(t *testing.T) {
	f, asked := run([]int64{1, 2, 3, 4},
		map[int64]error{
			2: fmt.Errorf("x: %w", bot.ErrorForbidden),
			3: fmt.Errorf("x: %w", bot.ErrorBadRequest),
			4: errors.New("network"),
		},
		map[int64]Chat{1: {Username: "ann", FirstName: "Анна", LastName: "К"}})
	if f.limit != batch || len(asked) != 4 {
		t.Fatalf("limit %d asked %v", f.limit, asked)
	}
	if f.saved[1] != (saved{"ann", "Анна", "К"}) {
		t.Fatalf("filled: %+v", f.saved[1])
	}
	// отказ Telegram — пустое значение (store не стирает имеющееся), сетевой сбой — без отметки
	if f.saved[2] != (saved{}) || f.saved[3] != (saved{}) {
		t.Fatalf("refused: %+v %+v", f.saved[2], f.saved[3])
	}
	if _, ok := f.saved[4]; ok {
		t.Fatal("network failure must not mark the user as checked")
	}
}

func TestRunStopsOnRateLimit(t *testing.T) {
	f, asked := run([]int64{1, 2, 3}, map[int64]error{2: &bot.TooManyRequestsError{Message: "slow", RetryAfter: 30}}, nil)
	if len(asked) != 2 {
		t.Fatalf("asked %v", asked)
	}
	if _, ok := f.saved[1]; !ok {
		t.Fatal("first must be marked")
	}
	if _, ok := f.saved[2]; ok || len(f.saved) != 1 {
		t.Fatalf("429 must not mark: %v", f.saved)
	}
}
