package heartbeat

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectFreshHeartbeatLabelsHeph(t *testing.T) {
	dir := t.TempDir()
	body := `{"schema":1,"pid":7,"starttime":9,"name":"Heph","project":"aitop","model":"claude-fable-5","updated_at":"2026-08-21T08:00:00Z"}`
	p := filepath.Join(dir, "7.json")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(p, now, now); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(dir, now, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 1 {
		t.Fatalf("fresh-heartbeat-emits-one-overlay violated: n=%d", len(ovs))
	}
	o := ovs[0]
	if !o.Heartbeat || o.ProvenName != "Heph" || o.PID != 7 || o.StartTime != 9 {
		t.Fatalf("heartbeat-proof-fields violated: %+v", o)
	}
	if o.CostUSD != nil {
		t.Fatalf("never-invent-cost violated")
	}
	if o.Project != "aitop" || o.Model != "claude-fable-5" {
		t.Fatalf("heartbeat-project-model violated: proj=%q model=%q", o.Project, o.Model)
	}
}

func TestStaleHeartbeatIsDropped(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "7.json")
	if err := os.WriteFile(p, []byte(`{"schema":1,"pid":7,"name":"Heph"}`), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * time.Second)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	ovs, err := Collect(dir, time.Now(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 0 {
		t.Fatalf("stale-heartbeat-is-dropped violated: n=%d", len(ovs))
	}
}
