package heartbeat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

type file struct {
	Schema    int    `json:"schema"`
	PID       int32  `json:"pid"`
	StartTime uint64 `json:"starttime"`
	Name      string `json:"name"`
	Project   string `json:"project"`
	Model     string `json:"model"`
	UpdatedAt string `json:"updated_at"`
}

func Collect(dir string, now time.Time, ttl time.Duration) ([]types.Overlay, error) {
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
		path := filepath.Join(dir, e.Name())
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		if now.Sub(st.ModTime()) > ttl {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var f file
		if json.Unmarshal(raw, &f) != nil || f.PID == 0 {
			continue
		}
		if f.Schema != 0 && f.Schema != 1 {
			continue
		}
		out = append(out, types.Overlay{
			PID:        f.PID,
			StartTime:  f.StartTime,
			ProvenName: f.Name,
			Project:    f.Project,
			Model:      f.Model,
			Heartbeat:  true,
		})
	}
	return out, nil
}

func DefaultDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "aitop", "hb")
	}
	return filepath.Join(os.TempDir(), "aitop", "hb")
}
