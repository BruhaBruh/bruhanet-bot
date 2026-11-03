package remnawave_test

import (
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

func users(err error, ids ...int64) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) {
		for _, id := range ids {
			var u api.UserItemInfo
			if id != 0 {
				u.TelegramId.SetTo(int(id))
			}
			if !yield(u, nil) {
				return
			}
		}
		if err != nil {
			yield(api.UserItemInfo{}, err)
		}
	}
}

func TestTelegramIDs(t *testing.T) {
	got, err := remnawave.TelegramIDs(users(nil, 5, 0, 3, 5, 7, 3))
	if err != nil || !slices.Equal(got, []int64{5, 3, 7}) {
		t.Fatalf("got %v, %v", got, err)
	}
	boom := errors.New("boom")
	if _, err := remnawave.TelegramIDs(users(boom, 1, 2)); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}
