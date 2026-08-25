package act

import (
	"context"
	"errors"

	"aitop/internal/types"
)

// ErrUnsupported is returned by Adapter methods the runtime cannot honor.
// Footer text must contain "unsupported" so the refusal is loud.
var ErrUnsupported = errors.New("unsupported")

// Target is the occupancy row an intent acts on. Join key remains
// (pid, starttime) for live processes; Key is the UI row key.
type Target struct {
	Key          string
	Runtime      types.Runtime
	PID          int32
	StartTime    uint64
	SessionID    string
	Unit         string
	CWD          string
	Model        string
	Worktree     string
	Overlay      types.Overlay
	Argv         []string // live cmdline or template ExecStart; required for local fanout
	TemplatePort int      // --port of the template; never reused by a clone
	RSS          uint64   // live instance RSS, bytes; packing uses an 8GiB guess when 0
}

// Spawned is what an adapter reports after Fork/Clone. PID is 0 until /proc
// sees the child; join nests by SessionID / sidecar, not unix ppid.
type Spawned struct {
	SessionID string
	PID       int32
	Worktree  string
	CapsuleID string
}

// Capsule is a stub. Full writer (json+md under XDG_RUNTIME_DIR) is Task 8.
type Capsule struct {
	Schema int    `json:"schema"`
	ID     string `json:"id"`
	Kind   string `json:"kind"`
}

// Adapter is one runtime family. Missing methods return ErrUnsupported;
// they do not no-op. Exec and HTTP live in the per-runtime packages, not here.
type Adapter interface {
	Name() types.Runtime
	Fork(ctx context.Context, t Target, cap Capsule, model string) (Spawned, error)
	Clone(ctx context.Context, t Target, cap Capsule) (Spawned, error)
	Message(ctx context.Context, t Target, text string) error
	Restart(ctx context.Context, t Target) error
	Kill(ctx context.Context, t Target) error
	Promote(ctx context.Context, t Target, spec string) error
	Budget(ctx context.Context, t Target, spec string) error
	Merge(ctx context.Context, parent, winner, loser Target) error
	Transcript(ctx context.Context, t Target) (path string, err error)
	Fanout(ctx context.Context, t Target, n int) error
}
