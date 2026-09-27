//go:build darwin && cgo

package proc

import (
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

func TestDarwinWalkFindsTheTestProcess(t *testing.T) {
	procs, err := Walk("")
	if err != nil {
		t.Fatalf("native process walk failed: %v", err)
	}
	pid := int32(os.Getpid())
	var self *types.Process
	for i := range procs {
		if procs[i].PID == pid {
			self = &procs[i]
			break
		}
	}
	if self == nil {
		t.Fatalf("native walk omitted its own pid %d", pid)
	}
	if self.StartTime == 0 || self.Comm == "" || self.RSS == 0 {
		t.Fatalf("native self process lacks occupancy identity: %+v", *self)
	}
	for _, p := range procs {
		if p.State < '!' || p.State > '~' {
			t.Fatalf("native process state is not terminal-safe: pid=%d state=%d", p.PID, p.State)
		}
	}
}

func TestDarwinNativeCmdlineMatchesOwnArgvAndExcludesEnvironment(t *testing.T) {
	got := nativeArgs(int32(os.Getpid()))
	if !reflect.DeepEqual(got, os.Args) {
		t.Fatalf("native argv mismatch: got=%q want=%q", got, os.Args)
	}
	for _, arg := range got {
		if arg == "PATH="+os.Getenv("PATH") || arg == "HOME="+os.Getenv("HOME") {
			t.Fatalf("native argv leaked environment data: %q", arg)
		}
	}
}

func TestDarwinHostSampleUsesNativeCounters(t *testing.T) {
	h := NewHost("", ClkTck())
	first := h.Sample()
	second := h.Sample()
	if first.NumCPU <= 0 || first.MemTotal == 0 || first.MemAvail == 0 || first.Uptime <= 0 {
		t.Fatalf("native host sample missing machine context: first=%+v", first)
	}
	if second.BootTime == 0 || second.ClkTck != 100 {
		t.Fatalf("native host sample lost boot/tick identity: second=%+v", second)
	}
}

func TestDarwinCPUTimeMatchesRusage(t *testing.T) {
	read := func() (uint64, int64) {
		t.Helper()
		p, ok := nativeFullProcess(int32(os.Getpid()))
		if !ok {
			t.Fatal("cannot read own CPU counters")
		}
		var r syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
			t.Fatal(err)
		}
		micros := (r.Utime.Sec+r.Stime.Sec)*1_000_000 + int64(r.Utime.Usec+r.Stime.Usec)
		return p.Utime + p.Stime, micros
	}
	before, beforeUS := read()
	until := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(until) {
	}
	after, afterUS := read()
	procUS := int64(after-before) * 1_000_000 / ClkTck()
	wantUS := afterUS - beforeUS
	delta := procUS - wantUS
	if delta < -40_000 || delta > 40_000 {
		t.Fatalf("native CPU time uses the wrong units: collector=%dus rusage=%dus", procUS, wantUS)
	}
}
