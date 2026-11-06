package remnawave

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
)

// SetDeviceLimit записывает в панель лимит устройств (база + купленные) и больше ничего не трогает.
// Вызывается только при событии с устройствами: обычные UpdateUser/CreateUser лимит не пишут,
// а синхронизация панели лимиты не перезаписывает.
func (rw *remnawave) SetDeviceLimit(ctx context.Context, telegramID int64, limit int) (*api.UserItemInfo, error) {
	user, err := rw.GetUser(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	resp, err := rw.client.Users().UpdateUser(ctx, &api.UpdateUserBody{
		ID:              api.NewOptInt(user.ID),
		HwidDeviceLimit: api.NewOptNilInt(limit),
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

// DeviceLimit — лимит устройств юзера в панели; ok=false, если не задан (действует умолчание панели).
func DeviceLimit(u api.UserItemInfo) (limit int, ok bool) {
	if u.HwidDeviceLimit.Null {
		return 0, false
	}
	return u.HwidDeviceLimit.Value, true
}
