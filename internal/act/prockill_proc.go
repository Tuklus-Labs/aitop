//go:build !darwin

package act

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/Tuklus-Labs/aitop/internal/proc"
)

func liveStart(root string, pid int32) (uint64, bool) {
	raw, err := os.ReadFile(filepath.Join(root, strconv.Itoa(int(pid)), "stat"))
	if err != nil {
		return 0, false
	}
	st, err := proc.ParseStat(string(raw))
	if err != nil {
		return 0, false
	}
	return st.StartTime, true
}
