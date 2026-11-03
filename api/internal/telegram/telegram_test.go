package telegram_test

import (
	"context"
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
