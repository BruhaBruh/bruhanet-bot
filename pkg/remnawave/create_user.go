package remnawave

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"
)

func (rw *remnawave) CreateUser(
	ctx context.Context,
	telegramID int64,
	username string,
	squadUUID uuid.UUID,
) (*api.UserItemInfo, error) {
	existingUser, _ := rw.GetUser(ctx, telegramID)
	if existingUser != nil {
		return existingUser, nil
	}
	resp, err := rw.client.Users().CreateUser(ctx, &api.CreateUserBody{
		Username:             username,
		Status:               api.NewOptCreateUserBodyStatus(api.CreateUserBodyStatusACTIVE),
		ExpireAt:             time.Now(),
		Description:          api.NewOptString(fmt.Sprintf("@%s (%d)", username, telegramID)),
		TelegramId:           api.NewOptNilInt(int(telegramID)),
		ActiveInternalSquads: []uuid.UUID{squadUUID},
	})
	if err != nil {
		return nil, errors.Join(ErrFailedToMakeRequest, err)
	}
	r, ok := resp.(*api.UserResponse)
	if !ok {
		return nil, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %+V", resp))
	}
	body := r.GetResponse()
	return &body, nil
}
