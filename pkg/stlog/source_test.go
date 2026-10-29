package stlog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bruhabruh/bruhanet/pkg/stlog"
)

func TestSourceIsCaller(t *testing.T) {
	var buf bytes.Buffer
	l := stlog.New(stlog.WithWriter(&buf), stlog.WithDefault(false), stlog.WithJSON(true), stlog.WithSource())
	stlog.Info(stlog.Wrap(context.Background(), l), "hi")
	var m struct {
		Source struct{ File string } `json:"source"`
	}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(m.Source.File, "source_test.go") {
		t.Fatalf("source = %s", m.Source.File)
	}
}
