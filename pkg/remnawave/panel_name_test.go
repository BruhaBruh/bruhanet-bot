package remnawave

import "testing"

func TestPanelName(t *testing.T) {
	if got := panelName("ann", 7); got != "ann" {
		t.Fatalf("got %q", got)
	}
	if got := panelName("", 7); got != "7" {
		t.Fatalf("got %q", got)
	}
}
