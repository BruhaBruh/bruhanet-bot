package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/robfig/cron/v3"
)

func newBufLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})), buf
}

func TestCronPanicLogsPanicRecovered(t *testing.T) {
	l, buf := newBufLogger(slog.LevelInfo)
	cron.Recover(cronLogger(l))(cron.FuncJob(func() { panic("boom") })).Run()

	var e map[string]any
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatalf("лог %q: %v", buf.String(), err)
	}
	if e["level"] != "ERROR" || e["msg"] != "panic recovered" || e["component"] != "reminder" {
		t.Fatalf("entry = %v", e)
	}
}

func TestCronLoggerInfoIsDebug(t *testing.T) {
	l, buf := newBufLogger(slog.LevelInfo)
	cronLogger(l).Info("skip")
	if buf.Len() != 0 {
		t.Fatalf("unexpected log: %s", buf.String())
	}
}
