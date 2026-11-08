package remnawave_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

const subpageUUID = "7d1f5b9e-0000-4000-8000-0000000000aa"

// panel: первый вызов (по shortUuid) отдаёт uuid конфига, второй (subscription-page-configs) — сам конфиг.
func subpagePanel(t *testing.T, first func(w http.ResponseWriter), gotBody *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "subscription-page-configs") {
			_, _ = w.Write([]byte(`{"response":{"uuid":"` + subpageUUID + `","viewPosition":0,"name":"main","tags":[],"config":{"platforms":{"ios":[]}}}}`))
			return
		}
		if gotBody != nil {
			_ = json.NewDecoder(r.Body).Decode(gotBody)
		}
		first(w)
	}))
}

func TestSubpageConfig(t *testing.T) {
	var body map[string]any
	srv := subpagePanel(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"response":{"subpageConfigUuid":"` + subpageUUID + `","webpageAllowed":true}}`))
	}, &body)
	defer srv.Close()
	rw, err := remnawave.New(srv.URL, "t", "c=1")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := rw.SubpageConfig(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"platforms":{"ios":[]}}` {
		t.Fatalf("config = %s", raw)
	}
	if h, _ := body["requestHeaders"].(map[string]any); h["user-agent"] != "BruhaNet-MiniApp" {
		t.Fatalf("request headers = %v", body)
	}
}

func TestSubpageConfigNotSet(t *testing.T) {
	srv := subpagePanel(t, func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"response":{"subpageConfigUuid":null,"webpageAllowed":true}}`))
	}, nil)
	defer srv.Close()
	rw, _ := remnawave.New(srv.URL, "t", "c=1")
	if _, err := rw.SubpageConfig(context.Background(), "abc123"); !errors.Is(err, remnawave.ErrSubpageConfigNotSet) {
		t.Fatalf("err = %v", err)
	}
}

func TestSubpageConfigErrors(t *testing.T) {
	for code, want := range map[int]error{http.StatusNotFound: remnawave.ErrUserNotFound, http.StatusInternalServerError: nil} {
		srv := subpagePanel(t, func(w http.ResponseWriter) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"timestamp":"2026-11-08T00:00:00Z","path":"/x","message":"err","errorCode":"A001"}`))
		}, nil)
		rw, _ := remnawave.New(srv.URL, "t", "c=1")
		_, err := rw.SubpageConfig(context.Background(), "abc123")
		srv.Close()
		if err == nil || (want != nil && !errors.Is(err, want)) || (want == nil && errors.Is(err, remnawave.ErrUserNotFound)) {
			t.Fatalf("code %d: err = %v, want %v", code, err, want)
		}
	}
}

// shortUuid — секрет подписки: ошибка транспорта (в тексте *url.Error есть полный адрес запроса) не должна его нести в логи.
func TestSubpageConfigErrorDoesNotLeakShortUUID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // дальше соединение отвергается, и ошибка содержит адрес запроса
	rw, err := remnawave.New(url, "t", "c=1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = rw.SubpageConfig(context.Background(), "SECRETSHORT")
	if err == nil || !errors.Is(err, remnawave.ErrFailedToMakeRequest) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "SECRETSHORT") {
		t.Fatalf("the error leaks the short uuid: %v", err)
	}
}
