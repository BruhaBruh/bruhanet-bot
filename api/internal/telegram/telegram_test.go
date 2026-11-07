package telegram_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/api/internal/telegram"
)

func TestOpenFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getFile"):
			switch r.FormValue("file_id") {
			case "big":
				_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"big","file_size":99999999,"file_path":"photos/b.jpg"}}`))
			case "gone":
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: wrong file_id"}`))
			case "denied":
				_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"denied","file_path":"photos/denied.jpg"}}`))
			default:
				_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"ok","file_size":3,"file_path":"photos/a.jpg"}}`))
			}
		case r.URL.Path == "/file/botsecret-token/photos/a.jpg":
			_, _ = w.Write([]byte("IMG"))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()
	tg, err := telegram.New("secret-token", bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	f, err := tg.OpenFile(context.Background(), "ok")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil || string(b) != "IMG" {
		t.Fatalf("body = %q, err = %v", b, err)
	}

	if _, err = tg.OpenFile(context.Background(), "big"); !errors.Is(err, telegram.ErrFileTooBig) {
		t.Fatalf("big: %v", err)
	}
	if _, err = tg.OpenFile(context.Background(), "gone"); !errors.Is(err, bot.ErrorBadRequest) {
		t.Fatalf("gone: %v", err)
	}
	if _, err = tg.OpenFile(context.Background(), "denied"); !errors.Is(err, telegram.ErrFileRefused) || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("denied: %v", err)
	}

	srv.Close()
	if _, err = tg.OpenFile(context.Background(), "ok"); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("token in error: %v", err)
	}
}

func TestMeAndChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":9,"is_bot":true,"first_name":"BruhaNet Payment","username":"bruhanet_payment_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/getChat"):
			_ = r.ParseMultipartForm(1 << 20)
			if r.FormValue("chat_id") != "42" {
				_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"type":"private","username":"ann","first_name":"Anna","last_name":"K"}}`))
		}
	}))
	defer srv.Close()
	tg, err := telegram.New("secret-token", bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if u, n, err := tg.Me(context.Background()); err != nil || u != "bruhanet_payment_bot" || n != "BruhaNet Payment" {
		t.Fatalf("Me = %q %q %v", u, n, err)
	}
	if u, n, err := tg.Chat(context.Background(), 42); err != nil || u != "ann" || n != "Anna K" {
		t.Fatalf("Chat = %q %q %v", u, n, err)
	}
	if _, _, err := tg.Chat(context.Background(), 7); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("err = %v", err)
	}
}

func TestProfilePhoto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID int64 `json:"user_id"`
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			_ = r.ParseMultipartForm(1 << 20)
			req.UserID, _ = strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		} else {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":9,"is_bot":true,"first_name":"B"}}`))
		case req.UserID == 1:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"total_count":2,"photos":[[{"file_id":"s","width":160,"height":160},{"file_id":"m","width":320,"height":320},{"file_id":"x","width":640,"height":640},{"file_id":"t","width":64,"height":64}],[{"file_id":"old","width":160,"height":160}]]}}`))
		case req.UserID == 2:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"total_count":1,"photos":[[{"file_id":"a","width":64,"height":64},{"file_id":"b","width":128,"height":128}]]}}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":{"total_count":0,"photos":[]}}`))
		}
	}))
	defer srv.Close()
	tg, err := telegram.New("secret-token", bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if id, err := tg.BotID(context.Background()); err != nil || id != 9 {
		t.Fatalf("BotID = %d %v", id, err)
	}
	if id, err := tg.ProfilePhoto(context.Background(), 1); err != nil || id != "s" {
		t.Fatalf("smallest >=160: %q %v", id, err)
	}
	if id, err := tg.ProfilePhoto(context.Background(), 2); err != nil || id != "b" {
		t.Fatalf("all small -> largest: %q %v", id, err)
	}
	if _, err := tg.ProfilePhoto(context.Background(), 3); !errors.Is(err, telegram.ErrNoPhoto) {
		t.Fatalf("no photo: %v", err)
	}
	srv.Close()
	if _, err := tg.ProfilePhoto(context.Background(), 1); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("token in error: %v", err)
	}
}
