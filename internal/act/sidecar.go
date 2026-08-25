package act

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type forkSidecar struct {
	Parent       string `json:"parent"`
	ForkOf       string `json:"fork_of"`
	Kind         string `json:"kind"`
	CapsuleID    string `json:"capsule_id"`
	Worktree     string `json:"worktree"`
	ChildSession string `json:"child_session"`
}

// ForksRoot is $AITOP_FORKS_DIR, else $XDG_RUNTIME_DIR/aitop/forks.
func ForksRoot() string {
	if v := os.Getenv("AITOP_FORKS_DIR"); v != "" {
		return v
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "aitop", "forks")
}

func (a *Actor) writeForkSidecar(in Intent, cap Capsule, spawned Spawned) error {
	if a == nil || a.ForksDir == "" || spawned.SessionID == "" {
		return nil
	}
	id := spawned.SessionID
	if id != filepath.Base(id) || id == "." || id == ".." {
		return fmt.Errorf("fork sidecar: invalid child session %q", id)
	}
	if err := os.MkdirAll(a.ForksDir, 0o755); err != nil {
		return err
	}
	kind := cap.Kind
	if kind == "" {
		kind = "fork"
		if in.Op == OpClone {
			kind = "clone"
		}
	}
	parent := in.Target.SessionID
	body := forkSidecar{
		Parent:       parent,
		ForkOf:       parent,
		Kind:         kind,
		CapsuleID:    firstNonEmpty(spawned.CapsuleID, cap.ID),
		Worktree:     firstNonEmpty(spawned.Worktree, in.Target.Worktree, cap.Child.CWD),
		ChildSession: id,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	path := filepath.Join(a.ForksDir, id+".json")
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
