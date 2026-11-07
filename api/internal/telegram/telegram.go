// Package telegram — Bot API для api: чтение файлов, имена и аватарки, без поллинга.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var (
	ErrNoPhoto     = errors.New("telegram: no photo in response")
	ErrFileTooBig  = errors.New("telegram: file too big")
	ErrFileRefused = errors.New("telegram: file download refused")
)

// MaxFileSize — больше Bot API не отдаёт.
const MaxFileSize = 20 << 20

type Client struct {
	b  *bot.Bot
	hc *http.Client
}

func New(token string, opts ...bot.Option) (*Client, error) {
	hc := &http.Client{Timeout: 30 * time.Second}
	opts = append([]bot.Option{bot.WithSkipGetMe(), bot.WithHTTPClient(0, hc)}, opts...)
	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{b: b, hc: hc}, nil
}

// clean убирает URL из ошибки: в нём токен бота, в ошибку и логи попадает только причина.
func clean(op string, err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return errors.Join(errors.New(op), ue.Err)
	}
	return err
}

// OpenFile отдаёт поток файла, не больше MaxFileSize; ссылка с токеном не покидает этот метод.
// Файла нет — ошибка с bot.ErrorBadRequest.
func (c *Client) OpenFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	f, err := c.b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, clean("telegram getFile", err)
	}
	if f.FileSize > MaxFileSize {
		return nil, ErrFileTooBig
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.b.FileDownloadLink(f), http.NoBody)
	if err != nil {
		return nil, errors.New("telegram file: bad request") // в err — URL с токеном
	}
	resp, err := c.hc.Do(req) //nolint:gosec // G704: хост — сервер Bot API из конфига клиента
	if err != nil {
		return nil, clean("telegram file", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, errors.Join(ErrFileRefused, fmt.Errorf("status %d", resp.StatusCode))
	}
	if resp.ContentLength > MaxFileSize {
		_ = resp.Body.Close()
		return nil, ErrFileTooBig
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, MaxFileSize), resp.Body}, nil
}

// Me — username и имя бота (getMe).
func (c *Client) Me(ctx context.Context) (username, name string, err error) {
	u, err := c.b.GetMe(ctx)
	if err != nil {
		return "", "", clean("telegram getMe", err)
	}
	return u.Username, u.FirstName, nil
}

// Chat — username и имя собеседника (getChat): имя и фамилия, у группы — название.
func (c *Client) Chat(ctx context.Context, chatID int64) (username, name string, err error) {
	ch, err := c.b.GetChat(ctx, &bot.GetChatParams{ChatID: chatID})
	if err != nil {
		return "", "", clean("telegram getChat", err)
	}
	name = strings.TrimSpace(ch.FirstName + " " + ch.LastName)
	if name == "" {
		name = ch.Title
	}
	return ch.Username, name, nil
}

// avatarMin — наименьший размер аватарки, который берём: меньше мылится на ретине.
const avatarMin = 160

// BotID — id бота (getMe): аватарку бота отдаёт getUserProfilePhotos по этому id.
func (c *Client) BotID(ctx context.Context) (int64, error) {
	u, err := c.b.GetMe(ctx)
	if err != nil {
		return 0, clean("telegram getMe", err)
	}
	return u.ID, nil
}

// ProfilePhoto — file_id текущей аватарки: самый маленький размер не меньше avatarMin, иначе самый большой.
// Фото нет (или скрыто настройками приватности) — ErrNoPhoto.
func (c *Client) ProfilePhoto(ctx context.Context, userID int64) (string, error) {
	res, err := c.b.GetUserProfilePhotos(ctx, &bot.GetUserProfilePhotosParams{UserID: userID, Limit: 1})
	if err != nil {
		return "", clean("telegram getUserProfilePhotos", err)
	}
	if len(res.Photos) == 0 || len(res.Photos[0]) == 0 {
		return "", ErrNoPhoto
	}
	var best *models.PhotoSize
	for i, p := range res.Photos[0] {
		side := min(p.Width, p.Height)
		switch {
		case best == nil:
			best = &res.Photos[0][i]
		case side >= avatarMin && (min(best.Width, best.Height) < avatarMin || side < min(best.Width, best.Height)):
			best = &res.Photos[0][i]
		case min(best.Width, best.Height) < avatarMin && side > min(best.Width, best.Height):
			best = &res.Photos[0][i]
		}
	}
	return best.FileID, nil
}
