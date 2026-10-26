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
