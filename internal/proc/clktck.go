package proc

import (
	"encoding/binary"
	"os"
	"sync"
)

const atCLKTCK = 17 // AT_CLKTCK in the ELF auxiliary vector

var (
	clkOnce sync.Once
	clkVal  int64 = 100
)

// ClkTck returns sysconf(_SC_CLK_TCK) without cgo by reading AT_CLKTCK from
// /proc/self/auxv. Falls back to 100 (every Linux box this will run on).
func ClkTck() int64 {
	clkOnce.Do(func() {
		if v, ok := parseAuxv("/proc/self/auxv"); ok {
			clkVal = v
		}
	})
	return clkVal
}

func parseAuxv(path string) (int64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	for i := 0; i+16 <= len(b); i += 16 {
		key := binary.LittleEndian.Uint64(b[i:])
		val := binary.LittleEndian.Uint64(b[i+8:])
		if key == 0 {
			break
		}
		if key == atCLKTCK && val > 0 && val < 100000 {
			return int64(val), true
		}
	}
	return 0, false
}
