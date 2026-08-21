package proc

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// HostSample is the machine-wide context the header paints. Everything here
// comes from files the 100ms path is allowed to read: /proc/stat, /proc/uptime,
// /proc/meminfo (read once; MemTotal does not move, MemAvailable is re-read).
type HostSample struct {
	CPUBusyPct float64 // all cores, 0..100
	CPUKnown   bool    // false on the first sample (no delta yet)
	NumCPU     int
	MemTotal   uint64 // bytes
	MemAvail   uint64 // bytes
	Uptime     float64
	BootTime   int64 // unix seconds, from /proc/stat btime
	ClkTck     int64
}

type cpuTotals struct {
	busy, total uint64
}

// Host tracks /proc/stat deltas between samples.
type Host struct {
	root   string
	clk    int64
	ncpu   int
	prev   cpuTotals
	havePr bool
	btime  int64
	memTot uint64
}

func NewHost(root string, clkTck int64) *Host {
	if root == "" {
		root = "/proc"
	}
	if clkTck <= 0 {
		clkTck = 100
	}
	h := &Host{root: root, clk: clkTck, ncpu: runtime.NumCPU()}
	if b, err := os.ReadFile(filepath.Join(root, "stat")); err == nil {
		h.btime, _ = parseBtime(b)
	}
	if b, err := os.ReadFile(filepath.Join(root, "meminfo")); err == nil {
		h.memTot, _ = parseMeminfo(b, "MemTotal")
	}
	return h
}

// Sample reads the current state and returns the busy percentage since the
// previous call. The first call reports CPUKnown=false rather than 0.
func (h *Host) Sample() HostSample {
	s := HostSample{NumCPU: h.ncpu, BootTime: h.btime, MemTotal: h.memTot, ClkTck: h.clk}
	if b, err := os.ReadFile(filepath.Join(h.root, "stat")); err == nil {
		if cur, err := ParseCPULine(b); err == nil {
			if h.havePr {
				dt := int64(cur.total) - int64(h.prev.total)
				db := int64(cur.busy) - int64(h.prev.busy)
				if dt > 0 && db >= 0 {
					s.CPUBusyPct = 100 * float64(db) / float64(dt)
					s.CPUKnown = true
				}
			}
			h.prev = cur
			h.havePr = true
		}
	}
	if b, err := os.ReadFile(filepath.Join(h.root, "uptime")); err == nil {
		s.Uptime, _ = ParseUptime(b)
	}
	if b, err := os.ReadFile(filepath.Join(h.root, "meminfo")); err == nil {
		s.MemAvail, _ = parseMeminfo(b, "MemAvailable")
		if s.MemTotal == 0 {
			s.MemTotal, _ = parseMeminfo(b, "MemTotal")
		}
	}
	return s
}

// ParseCPULine reads the aggregate "cpu" line of /proc/stat.
// Fields: user nice system idle iowait irq softirq steal guest guest_nice.
// busy = everything except idle and iowait (btop's definition).
func ParseCPULine(stat []byte) (cpuTotals, error) {
	sc := bufio.NewScanner(bytes.NewReader(stat))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 5 || f[0] != "cpu" {
			continue
		}
		var vals [10]uint64
		for i := 1; i < len(f) && i <= 10; i++ {
			v, err := strconv.ParseUint(f[i], 10, 64)
			if err != nil {
				return cpuTotals{}, fmt.Errorf("stat-cpu-field %d: %w", i, err)
			}
			vals[i-1] = v
		}
		var total uint64
		for _, v := range vals[:8] { // user..steal; guest time is already in user
			total += v
		}
		idle := vals[3] + vals[4]
		return cpuTotals{busy: total - idle, total: total}, nil
	}
	return cpuTotals{}, fmt.Errorf("stat-cpu-line: no aggregate cpu line")
}

func parseBtime(stat []byte) (int64, error) {
	sc := bufio.NewScanner(bytes.NewReader(stat))
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "btime "); ok {
			return strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
		}
	}
	return 0, fmt.Errorf("stat-btime: missing")
}

// ParseUptime reads the first field of /proc/uptime (seconds since boot).
func ParseUptime(b []byte) (float64, error) {
	f := strings.Fields(string(b))
	if len(f) < 1 {
		return 0, fmt.Errorf("uptime: empty")
	}
	return strconv.ParseFloat(f[0], 64)
}

// parseMeminfo returns the named field in bytes (meminfo reports kB).
func parseMeminfo(b []byte, key string) (uint64, error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		rest, ok := strings.CutPrefix(line, key+":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 1 {
			return 0, fmt.Errorf("meminfo %s: no value", key)
		}
		kb, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return 0, err
		}
		return kb * 1024, nil
	}
	return 0, fmt.Errorf("meminfo %s: missing", key)
}

// Age converts a /proc/<pid>/stat starttime (clock ticks since boot) into a
// duration using the host uptime. Zero uptime means unknown; returns 0.
func Age(uptime float64, starttime uint64, clkTck int64) time.Duration {
	if uptime <= 0 || clkTck <= 0 {
		return 0
	}
	started := float64(starttime) / float64(clkTck)
	d := uptime - started
	if d < 0 {
		d = 0
	}
	return time.Duration(d * float64(time.Second))
}
