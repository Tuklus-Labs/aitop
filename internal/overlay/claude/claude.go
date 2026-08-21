package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"

	"aitop/internal/join"
	"aitop/internal/types"
)

type sidecar struct {
	PID        json.RawMessage `json:"pid"`
	SessionID  string          `json:"sessionId"`
	CWD        string          `json:"cwd"`
	ProcStart  json.RawMessage `json:"procStart"`
	Kind       string          `json:"kind"`
	Entrypoint string          `json:"entrypoint"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
}

func Collect(claudeHome string) ([]types.Overlay, error) {
	dir := filepath.Join(claudeHome, "sessions")
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []types.Overlay
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s sidecar
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		pid, ok := parseUint32(s.PID)
		if !ok || pid == 0 {
			continue
		}
		start, _ := parseUint64(s.ProcStart)
		ov := types.Overlay{
			SessionID:  s.SessionID,
			PID:        int32(pid),
			StartTime:  start,
			Runtime:    types.RuntimeClaude,
			Project:    join.ProjectName(s.CWD),
			Title:      s.Name,
			OverlayCWD: s.CWD,
		}
		out = append(out, ov)
	}
	return out, nil
}

func parseUint32(raw json.RawMessage) (uint32, bool) {
	v, ok := parseUint64(raw)
	return uint32(v), ok
}

func parseUint64(raw json.RawMessage) (uint64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var n uint64
	if json.Unmarshal(raw, &n) == nil {
		return n, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, err := strconv.ParseUint(s, 10, 64)
		return n, err == nil
	}
	return 0, false
}
