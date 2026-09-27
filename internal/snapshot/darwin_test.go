//go:build darwin && cgo

package snapshot

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

func TestDarwinLiveAgentSnapshot(t *testing.T) {
	// A small owned process exercises OS discovery without launching another
	// test suite or waiting for macOS to inspect a freshly copied Go binary.
	body, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	childExe := filepath.Join(dir, "claude")
	if err := os.WriteFile(childExe, body, 0o700); err != nil {
		t.Fatal(err)
	}
	// A copied Apple platform binary needs its own signature before execution.
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", childExe).CombinedOutput(); err != nil {
		t.Fatalf("sign test helper: %v: %s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, childExe, "15")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	e := &Engine{ProcRoot: "/proc"}
	e.tickProc()
	first := e.Snapshot()
	if first == nil || first.Host.MemTotal == 0 || first.Host.Uptime <= 0 {
		t.Fatalf("native collection produced no usable host snapshot: %+v", first)
	}
	time.Sleep(100 * time.Millisecond)
	e.tickProc()
	s := e.Snapshot()
	var found bool
	for _, row := range s.Rows {
		if row.Process.PID != int32(cmd.Process.Pid) {
			continue
		}
		found = true
		p := row.Process
		if p.Runtime != types.RuntimeClaude || p.RSS == 0 || p.StartTime == 0 || !p.CPUKnown || p.CWD == "" || p.State == 0 {
			t.Fatalf("native agent row lacks process evidence: %+v", p)
		}
	}
	if !found {
		t.Fatalf("live claude PID %d missing from %d rows", cmd.Process.Pid, len(s.Rows))
	}
	dump, err := ToDumpV2(s, time.Now())
	if err != nil || dump.Schema != 2 || len(dump.Rows) == 0 {
		t.Fatalf("native snapshot cannot produce schema 2: err=%v rows=%d", err, len(dump.Rows))
	}
}
