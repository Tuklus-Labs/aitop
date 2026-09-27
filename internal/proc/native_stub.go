//go:build !darwin

package proc

import "github.com/Tuklus-Labs/aitop/internal/types"

// Linux and other Unix builds keep the procfs implementation as their native
// path. A non-Darwin root is always a fixture/procfs root; this stub exists so
// the common collector can dispatch without importing platform code.
func nativeRoot(string) bool { return false }

func walkNative() ([]types.Process, error) {
	return nil, errNativeUnavailable
}

func nativeInitHost(*Host) {}

func nativeHostSample(h *Host) HostSample {
	return HostSample{NumCPU: h.ncpu, BootTime: h.btime, MemTotal: h.memTot, ClkTck: h.clk}
}

func nativeClkTck() (int64, bool) { return 0, false }
