package remnawave

import (
	"context"
	"net/http"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
)

type remnawave struct {
	client *api.ClientExt
}

func New(
	panelURL string,
	token string,
	cookie string,
) (*remnawave, error) {
	base, err := api.NewClient(
		panelURL,
		api.StaticToken{Token: token},
		api.WithRequestEditor(func(ctx context.Context, req *http.Request) error {
			req.Header.Add("Cookie", cookie)
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return &remnawave{
		client: api.NewClientExt(base),
	}, nil
}
