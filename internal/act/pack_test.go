package act

import (
	"errors"
	"strings"
	"testing"
)

// Captured qwen38 ExecStart argv (house unit, 2026-08-24). GPU-heavy:
// -ngl 99, no --no-kv-offload.
var qwen38Argv = []string{
	"/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server",
	"-m", "/home/aegis/Models/Qwen3.8-27B/Qwen3.8-27B-Q4_K_M.gguf",
	"-ngl", "99",
	"-c", "262144",
	"-np", "1",
	"--slot-save-path", "/home/aegis/Models/kv-slot-cache",
	"-a", "Qwen3.8-27B",
	"--port", "8193",
	"--host", "127.0.0.1",
	"--no-webui",
}

func TestPackingGateRefusesSecondGPUHeavy(t *testing.T) {
	vram := func() (used, total uint64, err error) { return 23000 << 20, 24576 << 20, nil }
	err := Pack(PackInput{
		VRAM:     vram,
		GPUHeavy: true,
		LiveRSS:  20 << 30,
		Watchdog: 0.85,
	})
	if err == nil || !strings.Contains(err.Error(), "vram") {
		t.Fatalf("packing-gate-refuses-second-gpu-heavy violated: %v", err)
	}
}

func TestPackingGateAllowsWhenVRAMFree(t *testing.T) {
	err := Pack(PackInput{
		VRAM:     func() (uint64, uint64, error) { return 1 << 20, 24576 << 20, nil },
		GPUHeavy: true,
		LiveRSS:  1 << 20,
		Watchdog: 0.85,
	})
	if err != nil {
		t.Fatalf("packing-gate-allows-when-vram-free violated: %v", err)
	}
}

func TestPackingGateWatchdogThreshold(t *testing.T) {
	// 20890/24576 ~= 0.8501 >= 0.85, remainder still > LiveRSS.
	used := uint64(20890) << 20
	total := uint64(24576) << 20
	err := Pack(PackInput{
		VRAM:     func() (uint64, uint64, error) { return used, total, nil },
		GPUHeavy: true,
		LiveRSS:  1 << 20,
		Watchdog: 0.85,
	})
	if err == nil {
		t.Fatalf("packing-gate-watchdog-threshold violated: err=nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "watchdog") && !strings.Contains(msg, "vram") {
		t.Fatalf("packing-gate-watchdog-threshold violated: %v", err)
	}
}

func TestPackingGateVRAMReaderErrorIsClosed(t *testing.T) {
	err := Pack(PackInput{
		VRAM:     func() (uint64, uint64, error) { return 0, 0, errors.New("rocm-smi: exit 1") },
		GPUHeavy: true,
		LiveRSS:  1 << 20,
	})
	if err == nil || !strings.Contains(err.Error(), "vram") {
		t.Fatalf("packing-gate-vram-error-is-closed violated: %v", err)
	}
}

func TestPackingGateNilVRAMReaderIsClosed(t *testing.T) {
	err := Pack(PackInput{GPUHeavy: true, LiveRSS: 1 << 20})
	if err == nil || !strings.Contains(err.Error(), "vram") {
		t.Fatalf("packing-gate-nil-vram-reader-is-closed violated: %v", err)
	}
}

func TestPackingGateRefusesRAMWhenHalfMemAvailable(t *testing.T) {
	err := Pack(PackInput{
		CPUOffload:   true,
		LiveRSS:      10 << 30,
		N:            3,
		MemAvailable: 40 << 30, // half is 20 GiB; 3*10 GiB exceeds it
	})
	if err == nil {
		t.Fatalf("packing-gate-ram-half-memavailable violated: err=nil")
	}
}

func TestPackingGateAllowsRAMWhenUnderHalf(t *testing.T) {
	err := Pack(PackInput{
		CPUOffload:   true,
		LiveRSS:      1 << 30,
		N:            2,
		MemAvailable: 40 << 30,
	})
	if err != nil {
		t.Fatalf("packing-gate-ram-allows-under-half violated: %v", err)
	}
}

func TestPackingGateHybridPushesWatchdog(t *testing.T) {
	total := uint64(24576) << 20
	used := uint64(20000) << 20
	err := Pack(PackInput{
		VRAM:     func() (uint64, uint64, error) { return used, total, nil },
		Hybrid:   true,
		GPUHeavy: false,
		LiveRSS:  20 << 30,
		Watchdog: 0.85,
	})
	if err == nil || !strings.Contains(err.Error(), "vram") {
		t.Fatalf("packing-gate-hybrid-still-gpu-heavy-if-watchdog violated: %v", err)
	}
}

func TestPackingGateLiveRSSZeroUses8GiBGuess(t *testing.T) {
	// 24 GiB card, 20 GiB used, 4 GiB free, LiveRSS=0 => 8 GiB guess, refuse.
	err := Pack(PackInput{
		VRAM:     func() (uint64, uint64, error) { return 20 << 30, 24 << 30, nil },
		GPUHeavy: true,
		LiveRSS:  0,
		Watchdog: 0.85,
	})
	if err == nil || !strings.Contains(err.Error(), "vram") {
		t.Fatalf("packing-gate-8gib-guess-named-in-error violated: %v", err)
	}
}

func TestGPUHeavyQwen38Argv(t *testing.T) {
	if !GPUHeavy(qwen38Argv) {
		t.Fatalf("qwen38-argv-is-gpu-heavy violated: argv=%v", qwen38Argv)
	}
}

func TestGPUHeavyQwen38LiveExecStart(t *testing.T) {
	argv := []string{
		"/home/aegis/Projects/llama-cpp-turboquant/build-sync/bin/llama-server",
		"-m", "/home/aegis/Models/Qwen3.8-27B/Qwen3.8-27B-Q4_K_M.gguf",
		"--mmproj", "/home/aegis/Models/Qwen3.8-27B-FP8/mmproj-Qwen3.8-27b-FP8-F16.gguf",
		"-ngl", "99", "-fa", "on", "-ctk", "turbo3", "-ctv", "turbo3",
		"-c", "262144", "-np", "1", "--cache-reuse", "256",
		"--slot-save-path", "/home/aegis/Models/kv-slot-cache",
		"-a", "Qwen3.8-27B", "--port", "8193", "--host", "127.0.0.1", "--no-webui",
	}
	if !GPUHeavy(argv) {
		t.Fatalf("qwen38-live-execstart-is-gpu-heavy violated")
	}
}

func TestNoKVOffloadWithoutNGLIsNotGPUHeavy(t *testing.T) {
	argv := []string{"llama-server", "-m", "x.gguf", "--no-kv-offload", "--port", "8200"}
	if GPUHeavy(argv) {
		t.Fatalf("no-kv-offload-without-ngl-is-not-gpu-heavy violated")
	}
	if !CPUOffload(argv) {
		t.Fatalf("no-kv-offload-without-ngl-is-cpu-offload violated")
	}
}

func TestNGLZeroIsNotGPUHeavy(t *testing.T) {
	argv := []string{"llama-server", "-m", "x.gguf", "-ngl", "0", "--port", "8201"}
	if GPUHeavy(argv) {
		t.Fatalf("ngl-0-is-not-gpu-heavy violated")
	}
	if !CPUOffload(argv) {
		t.Fatalf("ngl-0-is-cpu-offload violated")
	}
}

func TestNGPULayersEqualsFormIsGPUHeavy(t *testing.T) {
	argv := []string{"llama-server", "--n-gpu-layers=99", "-m", "x.gguf"}
	if !GPUHeavy(argv) {
		t.Fatalf("n-gpu-layers-equals-is-gpu-heavy violated")
	}
}

func TestParseROCmSmiCSVMatchesLiveShape(t *testing.T) {
	csv := []byte("device,VRAM Total Memory (B),VRAM Total Used Memory (B)\ncard0,25753026560,1852891136\n")
	used, total, err := parseROCmVRAM(csv)
	if err != nil {
		t.Fatalf("parse-rocm-smi-csv: %v", err)
	}
	if total != 25753026560 || used != 1852891136 {
		t.Fatalf("parse-rocm-smi-csv-live-shape violated: used=%d total=%d", used, total)
	}
}

// nvidia-smi answers in MiB while rocm-smi answers in bytes. The caller
// compares the result against a byte budget and cannot tell the two apart, so
// a missing conversion would not error, it would silently read a 24 GiB card
// as 24 KiB of headroom and let every spawn through.
func TestParseNvidiaVRAMConvertsMiBToBytes(t *testing.T) {
	const mib = 1024 * 1024
	used, total, err := parseNvidiaVRAM([]byte("1024, 24576\n"))
	if err != nil {
		t.Fatalf("parse-nvidia-smi-csv: %v", err)
	}
	if used != 1024*mib || total != 24576*mib {
		t.Fatalf("parse-nvidia-smi-units-are-bytes violated: used=%d total=%d want used=%d total=%d",
			used, total, 1024*mib, 24576*mib)
	}
}

// A spawn lands on one card, so the emptiest card's headroom is not the
// headroom that matters. Reporting the fullest is what makes a refusal honest.
func TestParseNvidiaVRAMPicksFullestCard(t *testing.T) {
	const mib = 1024 * 1024
	used, total, err := parseNvidiaVRAM([]byte("512, 8192\n7000, 8192\n256, 8192\n"))
	if err != nil {
		t.Fatalf("parse-nvidia-smi-csv: %v", err)
	}
	if used != 7000*mib || total != 8192*mib {
		t.Fatalf("parse-nvidia-smi-fullest-card-wins violated: used=%d total=%d", used, total)
	}
}

// No cards is not zero usage. Returning (0,0,nil) here would read as a wholly
// empty GPU and turn a fail-closed gate into a fail-open one.
func TestParseNvidiaVRAMRefusesEmptyOutput(t *testing.T) {
	for _, in := range []string{"", "\n\n", "no devices were found\n", "abc, def\n"} {
		if _, _, err := parseNvidiaVRAM([]byte(in)); err == nil {
			t.Fatalf("parse-nvidia-smi-empty-is-an-error violated: %q parsed without error", in)
		}
	}
}
