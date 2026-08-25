package act

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	// VRAMGuess is the packing stand-in when the live instance has no RSS.
	VRAMGuess = 8 << 30 // 8 GiB
	// DefaultWatchdog is the vram-watchdog used/total ceiling. We do not talk
	// to the watchdog process; we refuse a spawn that would cross the same line.
	DefaultWatchdog = 0.85
)

// PackInput is the packing gate. VRAM and MemAvailable are injected; tests
// fake them. A nil or failing VRAM reader is closed (refuse), not open.
type PackInput struct {
	VRAM         func() (used, total uint64, err error)
	GPUHeavy     bool
	Hybrid       bool // --n-cpu-moe: GPU-heavy if LiveRSS would push used/total over Watchdog
	CPUOffload   bool // --no-kv-offload without ngl, or ngl 0
	LiveRSS      uint64
	Watchdog     float64 // 0 means DefaultWatchdog
	N            int     // fanout copies; 0 or 1 means one
	MemAvailable uint64
}

// Pack refuses a spawn that does not fit. GPU-heavy needs free VRAM strictly
// greater than LiveRSS (or 8GiB if LiveRSS is 0). RAM/CPU-offload needs
// n*RSS (or 8GiB guess) at most half of MemAvailable.
func Pack(in PackInput) error {
	n := in.N
	if n < 1 {
		n = 1
	}
	wd := in.Watchdog
	if wd <= 0 {
		wd = DefaultWatchdog
	}
	need := in.LiveRSS
	if need == 0 {
		need = VRAMGuess
	}

	if in.Hybrid && !in.GPUHeavy {
		if in.VRAM == nil {
			return fmt.Errorf("vram: no reader")
		}
		used, total, err := in.VRAM()
		if err != nil {
			return fmt.Errorf("vram: %w", err)
		}
		if total > 0 {
			projected := used + need*uint64(n)
			if float64(projected)/float64(total) > wd {
				in.GPUHeavy = true
			}
		}
	}
	if in.GPUHeavy {
		return gpuPack(in, need, n, wd)
	}
	if in.CPUOffload {
		return ramPack(in, need, n)
	}
	return nil
}

func gpuPack(in PackInput, need uint64, n int, wd float64) error {
	if in.VRAM == nil {
		return fmt.Errorf("vram: no reader")
	}
	used, total, err := in.VRAM()
	if err != nil {
		return fmt.Errorf("vram: %w", err)
	}
	if total == 0 {
		return fmt.Errorf("vram: total is 0")
	}
	ratio := float64(used) / float64(total)
	if ratio >= wd {
		return fmt.Errorf("vram-watchdog threshold: used/total=%.3f >= %.2f", ratio, wd)
	}
	want := need * uint64(n)
	var free uint64
	if total > used {
		free = total - used
	}
	needName := fmt.Sprintf("%d bytes RSS", need)
	if in.LiveRSS == 0 {
		needName = "8GiB guess"
	}
	if free <= want {
		return fmt.Errorf("vram: free %d bytes not greater than need %d (%s, n=%d)", free, want, needName, n)
	}
	projected := used + want
	if float64(projected)/float64(total) >= wd {
		return fmt.Errorf("vram-watchdog threshold: projected used/total=%.3f >= %.2f", float64(projected)/float64(total), wd)
	}
	return nil
}

func ramPack(in PackInput, need uint64, n int) error {
	if in.MemAvailable == 0 {
		return fmt.Errorf("ram: MemAvailable unknown")
	}
	half := in.MemAvailable / 2
	want := need * uint64(n)
	if want > half {
		return fmt.Errorf("ram: n*rss %d exceeds half MemAvailable %d", want, half)
	}
	return nil
}

// GPUHeavy reports argv with -ngl/--n-gpu-layers > 0 and without --no-kv-offload.
func GPUHeavy(argv []string) bool {
	if hasArg(argv, "--no-kv-offload") {
		return false
	}
	ngl, ok := nglOf(argv)
	return ok && ngl > 0
}

// CPUOffload is --no-kv-offload without ngl, or ngl 0.
func CPUOffload(argv []string) bool {
	ngl, has := nglOf(argv)
	noKV := hasArg(argv, "--no-kv-offload")
	if has && ngl == 0 {
		return true
	}
	if noKV && !has {
		return true
	}
	return false
}

// HybridMOE is --n-cpu-moe present on argv.
func HybridMOE(argv []string) bool {
	return hasArg(argv, "--n-cpu-moe")
}

func nglOf(argv []string) (int, bool) {
	s, ok := argValue(argv, "-ngl", "--n-gpu-layers")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func argValue(argv []string, names ...string) (string, bool) {
	for i, a := range argv {
		for _, name := range names {
			if a == name {
				if i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
					return argv[i+1], true
				}
				return "", false
			}
			if strings.HasPrefix(a, name+"=") {
				return strings.TrimPrefix(a, name+"="), true
			}
		}
	}
	return "", false
}

func hasArg(argv []string, name string) bool {
	for _, a := range argv {
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}

// ReadVRAM is the production reader. rocm-smi failure is closed: the caller
// must refuse the spawn rather than assume VRAM is free.
func ReadVRAM() (used, total uint64, err error) {
	bin, err := exec.LookPath("rocm-smi")
	if err != nil {
		bin = "/opt/rocm/bin/rocm-smi"
	}
	out, err := exec.Command(bin, "--showmeminfo", "vram", "--csv").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("vram: rocm-smi failed: %w", err)
	}
	return parseROCmVRAM(out)
}

func parseROCmVRAM(csv []byte) (used, total uint64, err error) {
	raw := strings.ReplaceAll(string(csv), "\r\n", "\n")
	var rows [][]string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, ","))
	}
	if len(rows) < 2 {
		return 0, 0, fmt.Errorf("vram: rocm-smi csv too short")
	}
	header := rows[0]
	usedIdx, totalIdx := -1, -1
	for i, c := range header {
		cl := strings.ToLower(strings.TrimSpace(c))
		if strings.Contains(cl, "used") && strings.Contains(cl, "memory") {
			usedIdx = i
		} else if strings.Contains(cl, "total") && strings.Contains(cl, "memory") {
			totalIdx = i
		}
	}
	if usedIdx < 0 || totalIdx < 0 {
		usedIdx, totalIdx = 2, 1 // live rocm-smi: device, total, used
	}
	for _, cols := range rows[1:] {
		if usedIdx >= len(cols) || totalIdx >= len(cols) {
			continue
		}
		u, err1 := strconv.ParseUint(strings.TrimSpace(cols[usedIdx]), 10, 64)
		tot, err2 := strconv.ParseUint(strings.TrimSpace(cols[totalIdx]), 10, 64)
		if err1 != nil || err2 != nil || tot == 0 {
			continue
		}
		return u, tot, nil
	}
	return 0, 0, fmt.Errorf("vram: rocm-smi csv had no data rows")
}
