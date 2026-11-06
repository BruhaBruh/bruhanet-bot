package remnawave

import (
	"context"
	"net/http"
	"strconv"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace/noop"
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
		// Спаны и метрики делает observability-декоратор; ogen по умолчанию пишет в глобальный provider (дубли и мусорные трейсы проб).
		api.WithTracerProvider(noop.NewTracerProvider()),
		api.WithMeterProvider(metricnoop.NewMeterProvider()),
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

// panelName — имя юзера в панели: она требует непустое, а username в Telegram есть не у всех.
func panelName(username string, telegramID int64) string {
	if username != "" {
		return username
	}
	return strconv.FormatInt(telegramID, 10)
}
