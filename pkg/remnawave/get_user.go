package remnawave

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
)

func (rw *remnawave) GetUser(
	ctx context.Context,
	telegramID int64,
) (*api.UserItemInfo, error) {
	resp, err := rw.client.Users().GetUsersStream(ctx, api.UsersGetUsersStreamParams{
		Size:       api.NewOptInt(1),
		TelegramId: api.NewOptString(fmt.Sprintf("%d", telegramID)),
	})
	if err != nil {
		return nil, errors.Join(ErrFailedToMakeRequest, err)
	}
	r, ok := resp.(*api.GetUsersStreamResponse)
	if !ok {
		return nil, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %+V", resp))
	}
	body := r.GetResponse()
	if len(body.Users) == 0 {
		return nil, ErrUserNotFound
	}
	user := body.Users[0]
	return &user, nil
}
