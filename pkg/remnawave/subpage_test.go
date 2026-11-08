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
	return subpagePanelList(t, first, gotBody, `{"response":{"total":0,"configs":[]}}`)
}

// list — ответ на список конфигов панели (GET /api/subscription-page-configs).
func subpagePanelList(t *testing.T, first func(w http.ResponseWriter), gotBody *map[string]any, list string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/subscription-page-configs" {
			_, _ = w.Write([]byte(list))
			return
		}
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

const nullSubpage = `{"response":{"subpageConfigUuid":null,"webpageAllowed":true}}`

// Конфиг юзеру не назначен (uuid null) — страница подписки показывает конфиг по умолчанию: берём «Default», иначе первый по позиции.
func TestSubpageConfigFallsBackToDefault(t *testing.T) {
	cfgs := func(names ...string) string {
		items := make([]string, 0, len(names))
		for i, n := range names {
			items = append(items, `{"uuid":"7d1f5b9e-0000-4000-8000-00000000000`+string(rune('1'+i))+`","viewPosition":`+string(rune('3'-i))+`,"name":"`+n+`","tags":[],"config":{"pick":"`+n+`"}}`)
		}
		return `{"response":{"total":` + string(rune('0'+len(names))) + `,"configs":[` + strings.Join(items, ",") + `]}}`
	}
	for name, tc := range map[string]struct{ list, want string }{
		"named Default wins":          {cfgs("Zeta", "Default", "Alpha"), `{"pick":"Default"}`},
		"no Default: lowest position": {cfgs("Zeta", "Alpha"), `{"pick":"Alpha"}`},
	} {
		srv := subpagePanelList(t, func(w http.ResponseWriter) { _, _ = w.Write([]byte(nullSubpage)) }, nil, tc.list)
		rw, _ := remnawave.New(srv.URL, "t", "c=1")
		raw, err := rw.SubpageConfig(context.Background(), "abc123")
		srv.Close()
		if err != nil || string(raw) != tc.want {
			t.Fatalf("%s: raw=%s err=%v", name, raw, err)
		}
	}
}

func TestConnectionKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/connection-keys/42") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"no","errorCode":"A001"}`))
			return
		}
		_, _ = w.Write([]byte(`{"response":{"enabledKeys":["vless://u@h:443#A","hysteria2://x@h:1#B"],"hiddenKeys":["hidden://x"],"disabledKeys":["no://x"]}}`))
	}))
	defer srv.Close()
	rw, _ := remnawave.New(srv.URL, "t", "c=1")
	keys, err := rw.ConnectionKeys(context.Background(), 42)
	if err != nil || len(keys) != 2 || keys[0] != "vless://u@h:443#A" || keys[1] != "hysteria2://x@h:1#B" {
		t.Fatalf("keys=%v err=%v", keys, err)
	}
	if _, err = rw.ConnectionKeys(context.Background(), 7); !errors.Is(err, remnawave.ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}
