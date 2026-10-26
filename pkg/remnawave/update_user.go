package remnawave

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	"github.com/google/uuid"
)

func (rw *remnawave) UpdateUser(
	ctx context.Context,
	telegramID int64,
	username string,
	expireAt time.Time,
	squadUUID uuid.UUID,
) (*api.UserItemInfo, error) {
	user, err := rw.GetUser(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	resp, err := rw.client.Users().UpdateUser(ctx, &api.UpdateUserBody{
		ID:                   api.NewOptInt(user.ID),
		Status:               api.NewOptUpdateUserBodyStatus(api.UpdateUserBodyStatusACTIVE),
		ExpireAt:             api.NewOptDateTime(expireAt),
		Description:          api.NewOptNilString(fmt.Sprintf("@%s (%d)", username, telegramID)),
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
