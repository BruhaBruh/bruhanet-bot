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
		return nil, errors.Join(ErrFailedToMakeRequest, redact(err, shortUUID))
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
		return rw.defaultSubpageConfig(ctx)
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

// defaultSubpageConfig — конфиг по умолчанию, когда юзеру он не назначен (так же ведёт себя страница подписки): конфиг
// с именем «Default», иначе первый по позиции. Конфигов в панели нет — ErrSubpageConfigNotSet.
func (rw *remnawave) defaultSubpageConfig(ctx context.Context) (json.RawMessage, error) {
	resp, err := rw.client.SubscriptionPageConfig().GetAllConfigs(ctx)
	if err != nil {
		return nil, errors.Join(ErrFailedToMakeRequest, err)
	}
	list, ok := resp.(*api.SubpageConfigsResponseResponse)
	if !ok {
		return nil, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %T", resp))
	}
	var pick *api.Config
	for i := range list.Response.Configs {
		c := &list.Response.Configs[i]
		switch {
		case c.Name == "Default":
			return json.RawMessage(c.Config), nil
		case pick == nil || c.ViewPosition < pick.ViewPosition:
			pick = c
		}
	}
	if pick == nil {
		return nil, ErrSubpageConfigNotSet
	}
	return json.RawMessage(pick.Config), nil
}

// ConnectionKeys — включённые ключи подключения юзера (vless://…#название, hysteria2://…), по числовому id в панели.
// Ссылки — доступ к VPN: наружу отдавать только по явному действию юзера.
func (rw *remnawave) ConnectionKeys(ctx context.Context, userID int) ([]string, error) {
	resp, err := rw.client.Subscriptions().GetConnectionKeysByUserId(ctx, userID)
	if err != nil {
		return nil, errors.Join(ErrFailedToMakeRequest, err)
	}
	switch r := resp.(type) {
	case *api.GetConnectionKeysByUserIdResponse:
		return r.Response.EnabledKeys, nil
	case *api.NotFoundError:
		return nil, ErrUserNotFound
	default:
		return nil, errors.Join(ErrInvalidResponseType, fmt.Errorf("response type: %T", resp))
	}
}
