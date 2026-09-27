//go:build darwin && !cgo

package proc

import (
	"errors"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

var errNativeUnavailable = errors.New("Darwin process collector requires cgo")

func nativeRoot(root string) bool { return root == "" || root == "/proc" }

func walkNative() ([]types.Process, error) {
	return nil, errNativeUnavailable
}

func nativeInitHost(*Host) {}

func nativeHostSample(h *Host) HostSample {
	return HostSample{NumCPU: h.ncpu, BootTime: h.btime, MemTotal: h.memTot, ClkTck: h.clk}
}

func nativeClkTck() (int64, bool) { return 100, true }
