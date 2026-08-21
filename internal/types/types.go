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
}

type Row struct {
	Process     Process
	Overlay     Overlay
	OverlayOK   bool
	OverlayOnly bool
	Children    []Row
}
