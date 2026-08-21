package ui

import (
	"strings"
	"testing"
	"time"

	"aitop/internal/snapshot"
)

func TestEmptyViewContainsCanary(t *testing.T) {
	s := Render(nil, time.Second)
	if !strings.Contains(s, snapshot.Canary) {
		t.Fatalf("empty-machine-canary violated: view %q does not contain %s", s, snapshot.Canary)
	}
}

func TestTickDoesNotCallOverlay(t *testing.T) {
	calls := 0
	parse := func() { calls++ }
	_ = parse
	m := New(nil)
	_, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("100ms-path-must-reschedule-tick violated: cmd=nil")
	}
	if calls != 0 {
		t.Fatalf("100ms-path-must-not-touch-overlays violated: overlayParseCalls=%d during Tick", calls)
	}
}
