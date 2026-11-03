// Package telegram — Bot API для api: только загрузка фото, без поллинга.
package telegram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

var ErrNoPhoto = errors.New("telegram: no photo in response")

type Client struct{ b *bot.Bot }

func New(token string, opts ...bot.Option) (*Client, error) {
	opts = append([]bot.Option{
		bot.WithSkipGetMe(),
		bot.WithHTTPClient(0, &http.Client{Timeout: 30 * time.Second}),
	}, opts...)
	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{b: b}, nil
}

// SendPhoto загружает фото в чат и возвращает file_id самого большого размера.
func (c *Client) SendPhoto(ctx context.Context, chatID int64, r io.Reader, filename, caption string) (string, error) {
	msg, err := c.b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:  chatID,
		Photo:   &models.InputFileUpload{Filename: filename, Data: r},
		Caption: caption,
	})
	if ue, ok := errors.AsType[*url.Error](err); ok {
		// в URL — токен бота: в ошибку и логи попадает только причина
		return "", errors.Join(errors.New("telegram sendPhoto"), ue.Err)
	}
	if err != nil {
		return "", err
	}
	var best *models.PhotoSize
	for i, p := range msg.Photo {
		if best == nil || p.Width*p.Height > best.Width*best.Height {
			best = &msg.Photo[i]
		}
	}
	if best == nil {
		return "", ErrNoPhoto
	}
	return best.FileID, nil
}
