package remnawave_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

func TestUserDevices(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"response":{"total":1,"devices":[{"hwid":"h1","userId":42,"platform":"ios","osVersion":null,"deviceModel":null,"userAgent":null,"requestIp":null,"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:05Z"}]}}`))
	}))
	defer srv.Close()
	rw, err := remnawave.New(srv.URL, "t", "c=1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := rw.UserDevices(context.Background(), 42)
	if err != nil || len(got) != 1 || got[0].Hwid != "h1" || got[0].Platform.Value != "ios" || !got[0].OsVersion.Null {
		t.Fatalf("got %+v err %v", got, err)
	}
	if path == "" {
		t.Fatal("no request")
	}
}

func TestDeleteDevices(t *testing.T) {
	var bodies []map[string]any
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusNotFound {
			_, _ = w.Write([]byte(`{"message":"nope"}`))
			return
		}
		_, _ = w.Write([]byte(`{"response":{"total":0,"devices":[]}}`))
	}))
	defer srv.Close()
	rw, err := remnawave.New(srv.URL, "t", "c=1")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := rw.DeleteUserDevice(ctx, 42, "h1"); err != nil {
		t.Fatal(err)
	}
	if err := rw.DeleteAllUserDevices(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0]["hwid"] != "h1" || bodies[0]["userId"] != float64(42) || len(bodies[1]) != 1 || bodies[1]["userId"] != float64(42) {
		t.Fatalf("bodies = %v", bodies)
	}
	status = http.StatusNotFound
	if err := rw.DeleteUserDevice(ctx, 42, "h1"); !errors.Is(err, remnawave.ErrDeviceNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, err := rw.UserDevices(ctx, 42); !errors.Is(err, remnawave.ErrUserNotFound) {
		t.Fatalf("err = %v", err)
	}
	status = http.StatusInternalServerError
	if err := rw.DeleteAllUserDevices(ctx, 42); err == nil || errors.Is(err, remnawave.ErrDeviceNotFound) {
		t.Fatalf("err = %v", err)
	}
}
