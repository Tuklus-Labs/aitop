package forks

import (
	"encoding/json"
	"os"
	"path/filepath"

	"aitop/internal/types"
)

type file struct {
	Parent       string `json:"parent"`
	ForkOf       string `json:"fork_of"`
	Kind         string `json:"kind"`
	CapsuleID    string `json:"capsule_id"`
	Worktree     string `json:"worktree"`
	ChildSession string `json:"child_session"`
}

// Collect reads $DIR/*.json fork sidecars into Overlay-only children.
// A missing dir is empty, not an error.
func Collect(dir string) ([]types.Overlay, error) {
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
		var f file
		if json.Unmarshal(raw, &f) != nil || f.ChildSession == "" {
			continue
		}
		out = append(out, types.Overlay{
			SessionID:     f.ChildSession,
			ParentSession: f.Parent,
			ForkOf:        f.ForkOf,
			Kind:          f.Kind,
			Worktree:      f.Worktree,
			CapsuleID:     f.CapsuleID,
			PID:           0,
		})
	}
	return out, nil
}

// DefaultDir is $AITOP_FORKS_DIR, else $XDG_RUNTIME_DIR/aitop/forks.
func DefaultDir() string {
	if v := os.Getenv("AITOP_FORKS_DIR"); v != "" {
		return v
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "aitop", "forks")
}
