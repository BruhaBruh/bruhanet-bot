package remnawave

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
)

func (rw *remnawave) Users(ctx context.Context) iter.Seq2[api.UserItemInfo, error] {
	return func(yield func(api.UserItemInfo, error) bool) {
		var cursor string
		for {
			if err := ctx.Err(); err != nil {
				yield(api.UserItemInfo{}, err)
				return
			}

			users, next, hasMore, err := rw.getUsersPage(ctx, cursor)
			if err != nil {
				yield(api.UserItemInfo{}, err)
				return
			}

			for _, u := range users {
				if !yield(u, nil) {
					return
				}
			}

			if !hasMore || next == "" || next == cursor {
				return
			}
			cursor = next
		}
	}
}

func (rw *remnawave) getUsersPage(
	ctx context.Context,
	cursor string,
) (users []api.UserItemInfo, next string, hasMore bool, err error) {
	params := api.UsersGetUsersStreamParams{Size: api.NewOptInt(1000)}
	if cursor != "" {
		params.Cursor = api.NewOptString(cursor)
	}

	resp, err := rw.client.Users().GetUsersStream(ctx, params)
	if err != nil {
		return nil, "", false, errors.Join(ErrFailedToMakeRequest, err)
	}

	r, ok := resp.(*api.GetUsersStreamResponse)
	if !ok {
		return nil, "", false, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %+V", resp))
	}

	body := r.GetResponse()
	if body.HasMore && !body.NextCursor.Null {
		return body.Users, body.NextCursor.Value, true, nil
	}
	return body.Users, "", false, nil
}

// TelegramID — Telegram ID юзера панели; ok=false, если не задан/null/0.
func TelegramID(u api.UserItemInfo) (int64, bool) {
	id, ok := u.TelegramId.Get()
	if !ok || id == 0 {
		return 0, false
	}
	return int64(id), true
}

// TelegramIDs — уникальные Telegram ID юзеров панели в порядке первого появления;
// ошибка панели обрывает список.
func TelegramIDs(users iter.Seq2[api.UserItemInfo, error]) ([]int64, error) {
	var ids []int64
	seen := make(map[int64]struct{})
	for user, err := range users {
		if err != nil {
			return nil, err
		}
		id, ok := TelegramID(user)
		if !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}
