// Package telegram — Bot API для api: загрузка фото и чтение файлов, без поллинга.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// SendPhoto загружает фото в чат и возвращает file_id самого большого размера.
func (c *Client) SendPhoto(ctx context.Context, chatID int64, r io.Reader, filename, caption string) (string, error) {
	msg, err := c.b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:  chatID,
		Photo:   &models.InputFileUpload{Filename: filename, Data: r},
		Caption: caption,
	})
	if err != nil {
		return "", clean("telegram sendPhoto", err)
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
