package langguard_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bruhabruh/bruhanet/core/langguard"
)

// TestNoCyrillicInDeveloperText fails when Cyrillic appears in errors, logs or
// panics. A deliberate Russian human-facing literal is excused with a
// `//lang:ru` comment on the call line or the line above (see CLAUDE.md).
func TestNoCyrillicInDeveloperText(t *testing.T) {
	hits, err := langguard.Scan([]string{"../../core", "../../bot", "../../api", "../../pkg"}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		t.Errorf("%s:%d: %s: %q (translate to English or mark //lang:ru)", h.File, h.Line, h.Call, h.Text)
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	src := `package x

import (
	"os"
	"path/filepath"
	"errors"
	"fmt"
	"log/slog"
)

func invalid(msg string) error { return errors.New(msg) }

var (
	a = errors.New("плохо")
	b = fmt.Errorf("ok %w: %s", a, "ещё")
	c = invalid("плохо")
	//lang:ru
	d = errors.New("можно")
	e = usererr.Errorf("bad", "хорошо")
	f = usererr.Errorf("плохо", "хорошо")
)

func g() { slog.Info("ok", "cause", "причина") }
`
	if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	hits, err := langguard.Scan([]string{dir}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	// a, b, c, f, g
	if len(hits) != 5 {
		t.Fatalf("hits = %+v", hits)
	}
}
