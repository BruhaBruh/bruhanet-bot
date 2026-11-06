package telegram_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-telegram/bot"

	"github.com/bruhabruh/bruhanet/api/internal/telegram"
)

func TestSendPhoto(t *testing.T) {
	var form map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendPhoto") {
			t.Errorf("path = %s", r.URL.Path)
		}
		form = map[string]string{}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		for p, err := mr.NextPart(); err == nil; p, err = mr.NextPart() {
			b, _ := io.ReadAll(p)
			form[p.FormName()] = string(b) + "|" + p.FileName()
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"},
			"photo":[{"file_id":"small","width":90,"height":90},{"file_id":"big","width":1280,"height":960},{"file_id":"mid","width":320,"height":240}]}}`))
	}))
	defer srv.Close()

	tg, err := telegram.New("secret-token", bot.WithServerURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	id, err := tg.SendPhoto(context.Background(), 42, strings.NewReader("IMG"), "a.png", "Фото для рассылки")
	if err != nil || id != "big" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if form["chat_id"] != "42|" || form["photo"] != "IMG|a.png" || form["caption"] != "Фото для рассылки|" {
		t.Fatalf("form = %v", form)
	}

	srv.Close()
	_, err = tg.SendPhoto(context.Background(), 42, strings.NewReader("IMG"), "a.png", "")
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("err = %v", err)
	}
}

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
