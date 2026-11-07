package config_test

import (
	"testing"

	"github.com/bruhabruh/bruhanet/api/internal/config"
	"github.com/bruhabruh/bruhanet/pkg/cfg"
)

func TestMiniAppConfig(t *testing.T) {
	tests := []struct {
		name string
		c    config.MiniAppConfig
		ok   bool
	}{
		{"disabled", config.MiniAppConfig{}, true},
		{"addr without origin", config.MiniAppConfig{Addr: ":8092"}, false},
		{"http origin", config.MiniAppConfig{Addr: ":8092", Origin: "http://app.example.test"}, false},
		{"https origin", config.MiniAppConfig{Addr: ":8092", Origin: "https://app.example.test"}, true},
		{"origin without addr", config.MiniAppConfig{Origin: "https://app.example.test"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := cfg.Validate(tc.c); (err == nil) != tc.ok {
				t.Fatalf("Validate = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}
