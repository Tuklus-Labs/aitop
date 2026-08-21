package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aitop/internal/join"
	"aitop/internal/types"
)

const tailBytes = 256 * 1024
const headBytes = 8192

func Collect(home string, fds map[int32][]string) ([]types.Overlay, error) {
	var out []types.Overlay
	seen := map[string]bool{}
	for pid, paths := range fds {
		var rollout string
		for _, p := range paths {
			if strings.Contains(p, "rollout-") && strings.HasSuffix(p, ".jsonl") {
				rollout = p
				break
			}
		}
		if rollout == "" {
			continue
		}
		if seen[rollout] {
			continue
		}
		seen[rollout] = true
		ov, err := ParseRollout(rollout)
		if err != nil {
			continue
		}
		ov.PID = pid
		if home != "" && !strings.HasPrefix(rollout, home) {
			// still accept; live fds point at ~/.codex
		}
		out = append(out, ov)
	}
	return out, nil
}

func ParseRollout(path string) (types.Overlay, error) {
	f, err := os.Open(path)
	if err != nil {
		return types.Overlay{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return types.Overlay{}, err
	}
	headN := int64(headBytes)
	if st.Size() < headN {
		headN = st.Size()
	}
	head := make([]byte, headN)
	if _, err := io.ReadFull(f, head); err != nil && err != io.ErrUnexpectedEOF {
		return types.Overlay{}, err
	}
	var tail []byte
	if st.Size() > headN {
		off := st.Size() - tailBytes
		if off < headN {
			off = headN
		}
		if _, err := f.Seek(off, io.SeekStart); err == nil {
			tail, _ = io.ReadAll(f)
		}
	}
	ov := types.Overlay{Runtime: types.RuntimeCodex, SessionPath: path}
	scanJSONL(head, &ov)
	if len(tail) > 0 {
		scanJSONL(tail, &ov)
	}
	return ov, nil
}

func scanJSONL(chunk []byte, ov *types.Overlay) {
	sc := bufio.NewScanner(bytes.NewReader(chunk))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var rec struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		switch rec.Type {
		case "session_meta":
			var p struct {
				SessionID string `json:"session_id"`
				CWD       string `json:"cwd"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil {
				if p.SessionID != "" {
					ov.SessionID = p.SessionID
				}
				if p.CWD != "" {
					ov.OverlayCWD = p.CWD
					ov.Project = join.ProjectName(p.CWD)
				}
			}
		case "turn_context":
			var p struct {
				Model string `json:"model"`
				CWD   string `json:"cwd"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil {
				if p.Model != "" {
					ov.Model = p.Model
					ov.ProvenName = seatName(p.Model)
				}
				if p.CWD != "" && ov.Project == "" {
					ov.OverlayCWD = p.CWD
					ov.Project = join.ProjectName(p.CWD)
				}
			}
		case "event_msg":
			var p struct {
				Type string `json:"type"`
				Info struct {
					Last struct {
						Total int64 `json:"total_tokens"`
					} `json:"last_token_usage"`
					Total struct {
						Input      int64 `json:"input_tokens"`
						Cached     int64 `json:"cached_input_tokens"`
						CacheWrite int64 `json:"cache_write_input_tokens"`
						Output     int64 `json:"output_tokens"`
						Total      int64 `json:"total_tokens"`
					} `json:"total_token_usage"`
					Window int64 `json:"model_context_window"`
				} `json:"info"`
			}
			if json.Unmarshal(rec.Payload, &p) == nil && p.Type == "token_count" {
				if p.Info.Last.Total != 0 {
					v := p.Info.Last.Total
					ov.TokensUsed = &v
				}
				if p.Info.Window != 0 {
					w := p.Info.Window
					ov.ContextWindow = &w
				}
				if tt := p.Info.Total; tt.Total != 0 {
					// input_tokens is the whole prompt; cached is the subset
					// served from cache. Bill the difference as fresh input.
					ov.Usage = types.Usage{
						Input:      tt.Input - tt.Cached,
						CacheRead:  tt.Cached,
						CacheWrite: tt.CacheWrite,
						Output:     tt.Output,
						Known:      true,
					}
				}
			}
		}
	}
}

func seatName(model string) string {
	switch model {
	case "gpt-5.6-sol":
		return "Sol"
	case "gpt-5.6-luna":
		return "Luna"
	case "gpt-5.6-terra":
		return "Terra"
	default:
		return ""
	}
}

func LiveFDs(procRoot string) map[int32][]string {
	if procRoot == "" {
		procRoot = "/proc"
	}
	ents, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	out := map[int32][]string{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid, err := parsePID(e.Name())
		if err != nil {
			continue
		}
		comm, _ := os.ReadFile(filepath.Join(procRoot, e.Name(), "comm"))
		c := strings.TrimSpace(string(comm))
		if c != "codex" && c != "ChatGPT" {
			continue
		}
		fdDir := filepath.Join(procRoot, e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		var paths []string
		for _, fd := range fds {
			t, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(t, "rollout-") || strings.Contains(t, "thread-writer-locks") {
				paths = append(paths, t)
			}
		}
		if len(paths) > 0 {
			out[pid] = paths
		}
	}
	return out
}

func parsePID(name string) (int32, error) {
	n := 0
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return 0, os.ErrInvalid
		}
		n = n*10 + int(name[i]-'0')
	}
	return int32(n), nil
}
