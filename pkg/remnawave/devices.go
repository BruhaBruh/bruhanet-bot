package remnawave

import (
	"context"
	"errors"
	"fmt"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
)

// UserDevices — HWID-устройства юзера; userID — числовой id в панели (api.UserItemInfo.ID).
func (rw *remnawave) UserDevices(ctx context.Context, userID int) ([]api.Device, error) {
	resp, err := rw.client.HwidUserDevices().GetUserHwidDevices(ctx, userID)
	if err != nil {
		return nil, errors.Join(ErrFailedToMakeRequest, err)
	}
	switch r := resp.(type) {
	case *api.HwidDevicesResponse:
		return r.Response.Devices, nil
	case *api.NotFoundError:
		return nil, ErrUserNotFound
	default:
		return nil, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %T", resp))
	}
}

// DeleteUserDevice удаляет одно устройство юзера; панель ищет его по паре (userID, hwid), чужое не заденет.
func (rw *remnawave) DeleteUserDevice(ctx context.Context, userID int, hwid string) error {
	resp, err := rw.client.HwidUserDevices().DeleteUserHwidDevice(ctx, &api.DeleteUserHwidDeviceBody{UserId: userID, Hwid: hwid})
	return deviceResult(resp, err)
}

// DeleteAllUserDevices удаляет все устройства юзера.
func (rw *remnawave) DeleteAllUserDevices(ctx context.Context, userID int) error {
	resp, err := rw.client.HwidUserDevices().DeleteAllUserHwidDevices(ctx, &api.DeleteAllUserHwidDevicesBody{UserId: userID})
	return deviceResult(resp, err)
}

func deviceResult(resp any, err error) error {
	if err != nil {
		return errors.Join(ErrFailedToMakeRequest, err)
	}
	switch resp.(type) {
	case *api.HwidDevicesResponse:
		return nil
	case *api.NotFoundError:
		return ErrDeviceNotFound
	default:
		return errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %T", resp))
	}
}
