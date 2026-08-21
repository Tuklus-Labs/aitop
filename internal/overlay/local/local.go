// Package local decorates local inference backends with what their systemd
// unit says they are. The classifier names a llama-server row from its
// cgroup (hermes-qwen38 -> qwen38); the unit's Description is the human
// line ("Iris: Qwen3.8-27B GPU-resident, ...") and lives in a file, so it is
// an overlay, read off-tick and cached per unit.
package local

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"aitop/internal/types"
)

// localComms are the comms the classifier can file under locals. python is
// here for talaria and the model proxy; a python under some other unit just
// gains that unit's description as its title, which is never wrong.
var localComms = map[string]bool{"llama-server": true, "local-brain": true, "ollama": true, "vllm": true, "python": true, "python3": true}

var (
	mu    sync.Mutex
	descs = map[string]string{} // unit -> description ("" = looked up, none)
)

// UnitDirs returns where unit files live, user first.
func UnitDirs() []string {
	var dirs []string
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		dirs = append(dirs, filepath.Join(base, "systemd", "user"))
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "systemd", "user"))
	}
	return append(dirs, "/etc/systemd/system", "/usr/lib/systemd/system", "/lib/systemd/system")
}

// Collect scans procRoot for local backend comms and emits one overlay per
// pid whose unit has a Description. unitDirs nil means UnitDirs().
func Collect(procRoot string, unitDirs []string) []types.Overlay {
	if procRoot == "" {
		procRoot = "/proc"
	}
	if unitDirs == nil {
		unitDirs = UnitDirs()
	}
	ents, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	var out []types.Overlay
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid, ok := parsePID(e.Name())
		if !ok {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "comm"))
		if err != nil || !localComms[strings.TrimSpace(string(comm))] {
			continue
		}
		cg, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cgroup"))
		if err != nil {
			continue
		}
		unit := unitOf(string(cg))
		if unit == "" {
			continue
		}
		desc := Description(unit, unitDirs)
		if desc == "" {
			continue
		}
		out = append(out, types.Overlay{PID: pid, Runtime: types.RuntimeLocal, Title: desc, SessionName: unit})
	}
	return out
}

// Description reads Description= from the first unit file found for unit.
func Description(unit string, unitDirs []string) string {
	mu.Lock()
	if d, ok := descs[unit]; ok {
		mu.Unlock()
		return d
	}
	mu.Unlock()
	var desc string
	for _, dir := range unitDirs {
		raw, err := os.ReadFile(filepath.Join(dir, unit+".service"))
		if err != nil {
			// Template instances (parlor-sidecar@x) live in the template file.
			if at := strings.IndexByte(unit, '@'); at >= 0 {
				raw, err = os.ReadFile(filepath.Join(dir, unit[:at+1]+".service"))
			}
			if err != nil {
				continue
			}
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Description="); ok {
				desc = strings.TrimSpace(v)
				break
			}
		}
		if desc != "" {
			break
		}
	}
	mu.Lock()
	descs[unit] = desc
	mu.Unlock()
	return desc
}

// unitOf returns "foo" when the cgroup's final path component is
// foo.service. The user manager (user@1000.service) and session scopes sit
// higher up the path and are not a process's own unit.
func unitOf(cgroup string) string {
	for _, line := range strings.Split(cgroup, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.LastIndexByte(line, '/'); i >= 0 {
			line = line[i+1:]
		}
		if !strings.HasSuffix(line, ".service") {
			continue
		}
		u := strings.TrimSuffix(line, ".service")
		if strings.HasPrefix(u, "user@") || u == "" {
			continue
		}
		return u
	}
	return ""
}

func parsePID(name string) (int32, bool) {
	n := 0
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return 0, false
		}
		n = n*10 + int(name[i]-'0')
	}
	return int32(n), n > 0
}
