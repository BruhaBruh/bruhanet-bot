package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/Jolymmiles/remnawave-api-go/v3/api"

	"github.com/bruhabruh/bruhanet/pkg/remnawave"
)

func miniDevicePanel() *fakePanel {
	return &fakePanel{
		user: activeUser(now.Add(24 * time.Hour)),
		devices: []api.Device{
			{Hwid: "hw-1", Platform: api.NewNilString("iOS"), DeviceModel: api.NewNilString("iPhone")},
			{Hwid: "hw-2"},
		},
	}
}

func TestMiniDevicesList(t *testing.T) {
	h, _, _ := newMiniPanel(t, miniDevicePanel())
	code, m, raw := getMini(t, h, http.MethodGet, "/m/api/devices", "")
	items, _ := m["items"].([]any)
	if code != 200 || len(items) != 2 || m["limit"] != float64(5) || items[0].(map[string]any)["hwid"] != "hw-1" {
		t.Fatalf("%d %s", code, raw)
	}
	h, _, _ = newMiniPanel(t, &fakePanel{err: remnawave.ErrUserNotFound})
	if code, m, _ := getMini(t, h, http.MethodGet, "/m/api/devices", ""); code != 200 || len(m["items"].([]any)) != 0 {
		t.Fatalf("not in panel must be an empty list, got %d %v", code, m)
	}
}

func TestMiniDevicesDelete(t *testing.T) {
	panel := miniDevicePanel()
	h, _, _ := newMiniPanel(t, panel)
	if code, _, raw := getMini(t, h, http.MethodDelete, "/m/api/devices/hw-1", ""); code != 204 {
		t.Fatalf("delete one: %d %s", code, raw)
	}
	if code, _, _ := getMini(t, h, http.MethodDelete, "/m/api/devices", ""); code != 204 {
		t.Fatalf("delete all: %d", code)
	}
	// панель получает пару (числовой id юзера в панели, hwid): id юзера из подписи, не из запроса
	if len(panel.deleted) != 2 || panel.deleted[0] != "7:hw-1" || panel.deleted[1] != "7:*" {
		t.Fatalf("panel calls = %v", panel.deleted)
	}
	if code, _, _ := getMini(t, h, http.MethodDelete, "/m/api/devices/bad%01hwid", ""); code != 400 {
		t.Fatalf("control char in hwid: %d", code)
	}
}

func TestMiniDevicesDeleteErrors(t *testing.T) {
	p := miniDevicePanel()
	p.devErr = remnawave.ErrDeviceNotFound
	h, _, _ := newMiniPanel(t, p)
	// ErrDeviceNotFound отдаёт и UserDevices, но удаление проверяем напрямую
	if code, _, _ := getMini(t, h, http.MethodDelete, "/m/api/devices/hw-9", ""); code != 404 {
		t.Fatalf("unknown device: %d", code)
	}
	h, _, _ = newMiniPanel(t, &fakePanel{err: remnawave.ErrUserNotFound})
	if code, _, _ := getMini(t, h, http.MethodDelete, "/m/api/devices/hw-1", ""); code != 404 {
		t.Fatalf("not in panel: %d", code)
	}
}
