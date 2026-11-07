package remnawave

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
)

// subpageUserAgent — какой клиент панель «видит» при выборе конфига страницы подписки.
const subpageUserAgent = "BruhaNet-MiniApp"

// SubpageConfig — сырой JSON конфига страницы подписки юзера (платформы, приложения, шаги инструкции).
// Два вызова: по shortUuid панель отдаёт uuid конфига юзера, по uuid — сам конфиг. Формат конфига зависит от версии
// панели, поэтому здесь он остаётся сырым; разбирает и чистит его api. Конфиг у юзера не задан — ErrSubpageConfigNotSet,
// юзера нет — ErrUserNotFound.
func (rw *remnawave) SubpageConfig(ctx context.Context, shortUUID string) (json.RawMessage, error) {
	resp, err := rw.client.Subscriptions().GetSubpageConfigByShortUuid(ctx, &api.GetSubpageConfigByShortUuidBody{
		RequestHeaders: api.GetSubpageConfigByShortUuidBodyRequestHeaders{"user-agent": subpageUserAgent},
	}, shortUUID)
	if err != nil {
		return nil, errors.Join(ErrFailedToMakeRequest, err)
	}
	var id api.NilUUID
	switch r := resp.(type) {
	case *api.GetSubpageConfigByShortUuidResponse:
		id = r.Response.SubpageConfigUuid
	case *api.NotFoundError:
		return nil, ErrUserNotFound
	default:
		return nil, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %T", resp))
	}
	if id.Null {
		return nil, ErrSubpageConfigNotSet
	}
	cfg, err := rw.client.SubscriptionPageConfig().GetConfigByUuid(ctx, id.Value.String())
	if err != nil {
		return nil, errors.Join(ErrFailedToMakeRequest, err)
	}
	switch r := cfg.(type) {
	case *api.SubpageConfigResponseResponse:
		return json.RawMessage(r.Response.Config), nil
	case *api.NotFoundError:
		return nil, ErrSubpageConfigNotSet
	default:
		return nil, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %T", cfg))
	}
}
