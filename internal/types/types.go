package types

import "time"

type Runtime string

const (
	RuntimeGrok    Runtime = "grok"
	RuntimeClaude  Runtime = "claude"
	RuntimeCodex   Runtime = "codex"
	RuntimeHermes  Runtime = "hermes"
	RuntimeParlor  Runtime = "parlor"
	RuntimeForge   Runtime = "forge"
	RuntimeLocal   Runtime = "local" // local inference backends: llama-server units, ollama, model proxy
	RuntimeUnknown Runtime = ""
)

type Role string

const (
	RolePrimary  Role = "primary"
	RoleSubagent Role = "subagent"
	RoleSidecar  Role = "sidecar"
	RoleDesktop  Role = "desktop"
	RoleWorkflow Role = "workflow"
	RoleMonitor  Role = "monitor"
	RoleIgnore   Role = "ignore"
	RoleDrop     Role = ""
)

// Process is occupancy truth from /proc. Overlay never replaces these fields.
type Process struct {
	PID         int32
	PPID        int32
	StartTime   uint64
	Comm        string
	Exe         string
	Cmdline     []string
	CWD         string
	CPUPct      float64
	CPUKnown    bool
	RSS         uint64
	Utime       uint64
	Stime       uint64
	State       byte
	Runtime     Runtime
	Role        Role
	CollapseKey string
	AgentRoot   bool
	Cgroup      string
	NameHint    string
	ModelHint   string // classifier-derived, e.g. gguf basename from llama-server argv
	Tag         string // classifier-derived provenance: "rc" for a Remote Control session
}

// Usage is lifetime token usage for a session, when the runtime exposes it.
// Known=false means the runtime wrote no totals (Grok); never treat the zero
// value as "used nothing".
type Usage struct {
	Input      int64 // uncached input
	CacheRead  int64
	CacheWrite int64
	Output     int64
	Known      bool
}

// Overlay is a cache entry from session files or heartbeat. Nil pointers are absent.
type Overlay struct {
	SessionID        string
	PID              int32
	StartTime        uint64
	Runtime          Runtime
	ProvenName       string
	Project          string
	Model            string
	TokensUsed       *int64
	ContextWindow    *int64
	ContextFill      *float64
	CostUSD          *float64
	CostSource       string // "" = runtime-reported (none do today); "table:builtin" / "table:user" = estimate from price table
	Usage            Usage
	WindowSource     string // "" = runtime file; "table" = price table lookup
	SubagentLive     int
	SubagentDeclared int
	Title            string
	UpdatedAt        time.Time
	SessionPath      string
	OverlayCWD       string
	Heartbeat        bool
	ParentSession    string
	SubagentStatus   string
	SubagentID       string
	SubagentType     string

	// Status is the runtime's own word for the session: busy | idle | wait | error | shell.
	// Empty means the overlay does not know; the spine may still upgrade to busy.
	Status string
	// StartedAt / CompletedAt are set when the overlay knows them (grok subagent meta,
	// claude sidecar startedAt). Zero means unknown; age then comes from /proc.
	StartedAt   time.Time
	CompletedAt time.Time
	// SessionName is the runtime's label for the session (claude sidecar "name",
	// e.g. aegis-48). It is not an identity and never becomes ProvenName.
	SessionName string
	Branch      string
	Effort      string
	Entrypoint  string // runtime's own word: cli | sdk-cli | sdk-ts

	ForkOf    string // parent session id for a fork/clone/fanout
	CapsuleID string
	Worktree  string
	Kind      string   // fork | clone | fanout | slot
	TokPerSec *float64 // nil = unknown; never coerce 0 on first sample
	SlotIndex *int     // llama-server slot; nil = not a slot row
	Dark      bool     // local unit with no pid
}

type Row struct {
	Process     Process
	Overlay     Overlay
	OverlayOK   bool
	OverlayOnly bool
	Children    []Row
}
