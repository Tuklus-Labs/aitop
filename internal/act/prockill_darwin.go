//go:build darwin

package act

import "github.com/Tuklus-Labs/aitop/internal/proc"

// macOS has no procfs stat file. The native proc collector is the single
// source of PID identity for both occupancy and actions, so every identity
// check observes its StartTime through proc.Walk before a signal is sent and
// again during the SIGINT grace period.
func liveStart(root string, pid int32) (uint64, bool) {
	procs, err := proc.Walk(root)
	if err != nil {
		return 0, false
	}
	for _, p := range procs {
		if p.PID == pid {
			return p.StartTime, p.StartTime != 0
		}
	}
	return 0, false
}
