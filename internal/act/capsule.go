package act

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CapsuleSchema = 1

type CapsuleParent struct {
	Runtime   string `json:"runtime"`
	SessionID string `json:"session_id"`
	PID       int32  `json:"pid,omitempty"`
	StartTime uint64 `json:"starttime,omitempty"`
	Model     string `json:"model"`
	CWD       string `json:"cwd"`
	Title     string `json:"title"`
	Project   string `json:"project,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Effort    string `json:"effort,omitempty"`
}

type CapsuleTask struct {
	Title     string   `json:"title"`
	Todos     []string `json:"todos,omitempty"`
	LastTools []string `json:"last_tools,omitempty"`
	Head      string   `json:"head,omitempty"`
}

type CapsuleChild struct {
	Runtime   string `json:"runtime"`
	Model     string `json:"model"`
	CWD       string `json:"cwd"`
	SessionID string `json:"session_id"`
}

type Capsule struct {
	Schema    int           `json:"schema"`
	ID        string        `json:"id"`
	Kind      string        `json:"kind"`
	CreatedAt string        `json:"created_at,omitempty"`
	Parent    CapsuleParent `json:"parent"`
	Task      CapsuleTask   `json:"task"`
	Ancestry  []string      `json:"ancestry,omitempty"`
	Child     CapsuleChild  `json:"child"`
	Dir       string        `json:"-"` // written directory (root/id); adapters read capsule.md here
}

// CapsuleRoot is $AITOP_CAPSULE_ROOT, else $XDG_RUNTIME_DIR/aitop/capsule.
func CapsuleRoot() string {
	if v := os.Getenv("AITOP_CAPSULE_ROOT"); v != "" {
		return v
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "aitop", "capsule")
}

// Write places capsule.json and capsule.md under root/id. path is that directory.
func Write(root string, cap Capsule) (path string, err error) {
	if root == "" {
		return "", fmt.Errorf("capsule: empty root")
	}
	if cap.ID == "" {
		return "", fmt.Errorf("capsule: empty id")
	}
	if cap.ID != filepath.Base(cap.ID) || cap.ID == "." || cap.ID == ".." {
		return "", fmt.Errorf("capsule: invalid id %q", cap.ID)
	}
	if cap.Schema == 0 {
		cap.Schema = CapsuleSchema
	}
	if cap.CreatedAt == "" {
		cap.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	dir := filepath.Join(root, cap.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	raw, err := json.Marshal(cap)
	if err != nil {
		return "", err
	}
	js := filepath.Join(dir, "capsule.json")
	if err := os.WriteFile(js, append(raw, '\n'), 0o644); err != nil {
		return "", err
	}
	md := filepath.Join(dir, "capsule.md")
	if err := os.WriteFile(md, []byte(renderMarkdown(cap)), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

func (a *Actor) writeCapsule(in Intent) (Capsule, error) {
	if a == nil || a.CapsuleDir == "" {
		return Capsule{}, nil
	}
	cap, _, err := PrepareFork(a.CapsuleDir, in)
	return cap, err
}

// PrepareFork fills a Capsule from Intent, writes it under root, and returns
// the written capsule plus the directory. The caller then Forks.
func PrepareFork(root string, in Intent) (Capsule, string, error) {
	cap := capsuleFromIntent(in)
	if cap.ID == "" {
		id, err := newCapsuleID()
		if err != nil {
			return Capsule{}, "", err
		}
		cap.ID = id
	}
	path, err := Write(root, cap)
	cap.Dir = path
	return cap, path, err
}

// PromptFile is root/id/capsule.md when Dir is set, else capsule.md.
func PromptFile(cap Capsule) string {
	if cap.Dir != "" {
		return filepath.Join(cap.Dir, "capsule.md")
	}
	return "capsule.md"
}

// CapsulePrompt is the child's first prompt: capsule.md if present, else the
// short branch directive. Used when the CLI takes a prompt argument, not a file.
func CapsulePrompt(cap Capsule) string {
	if b, err := os.ReadFile(PromptFile(cap)); err == nil && len(b) > 0 {
		return string(b)
	}
	if cap.Kind == "clone" {
		return "You are a cloned branch. The parent is still running.\n"
	}
	return "You are a branch. The parent is still running.\n"
}

// ChildSession is a preallocated child UUID, or a new one.
func ChildSession(cap Capsule) (string, error) {
	if cap.Child.SessionID != "" {
		return cap.Child.SessionID, nil
	}
	return NewSessionID()
}

// ShortForkID is 8 hex chars for aitop-fork-<id> names.
func ShortForkID(cap Capsule) string {
	raw := strings.ReplaceAll(cap.ID, "-", "")
	if len(raw) >= 8 {
		return strings.ToLower(raw[:8])
	}
	raw = strings.ReplaceAll(cap.Child.SessionID, "-", "")
	if len(raw) >= 8 {
		return strings.ToLower(raw[:8])
	}
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

// NewSessionID returns a random UUID v4 string.
func NewSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("session id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func capsuleFromIntent(in Intent) Capsule {
	kind := "fork"
	switch in.Op {
	case OpClone:
		kind = "clone"
	case OpFanout:
		kind = "fanout"
	}
	parent := CapsuleParent{
		Runtime:   string(in.Target.Runtime),
		SessionID: in.Target.SessionID,
		PID:       in.Target.PID,
		StartTime: in.Target.StartTime,
		Model:     firstNonEmpty(in.Target.Model, in.Target.Overlay.Model),
		CWD:       firstNonEmpty(in.Target.CWD, in.Target.Overlay.OverlayCWD),
		Title:     in.Target.Overlay.Title,
		Project:   in.Target.Overlay.Project,
		Branch:    in.Target.Overlay.Branch,
		Effort:    in.Target.Overlay.Effort,
	}
	childModel := parent.Model
	if in.Op == OpFork && in.Args != "" {
		childModel = in.Args
	}
	childCWD := firstNonEmpty(in.Target.Worktree, parent.CWD)
	var ancestry []string
	if in.Target.SessionID != "" {
		ancestry = []string{in.Target.SessionID}
	}
	return Capsule{
		Schema:    CapsuleSchema,
		Kind:      kind,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Parent:    parent,
		Task:      CapsuleTask{Title: parent.Title},
		Ancestry:  ancestry,
		Child: CapsuleChild{
			Runtime: parent.Runtime,
			Model:   childModel,
			CWD:     childCWD,
		},
	}
}

func newCapsuleID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("capsule id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func renderMarkdown(cap Capsule) string {
	var b strings.Builder
	if cap.Kind == "clone" {
		b.WriteString("You are a cloned branch. The parent is still running.\n")
	} else {
		b.WriteString("You are a branch. The parent is still running.\n")
	}
	b.WriteString("Do not claim you are the parent. Keep going.\n\n")

	cwd := firstNonEmpty(cap.Child.CWD, cap.Parent.CWD)
	model := firstNonEmpty(cap.Child.Model, cap.Parent.Model)
	title := firstNonEmpty(cap.Task.Title, cap.Parent.Title)

	b.WriteString("- ancestry: ")
	if len(cap.Ancestry) == 0 {
		b.WriteString("(none)\n")
	} else {
		b.WriteString(strings.Join(cap.Ancestry, ", "))
		b.WriteString("\n")
	}
	b.WriteString("- cwd: ")
	b.WriteString(cwd)
	b.WriteString("\n")
	b.WriteString("- model: ")
	b.WriteString(model)
	b.WriteString("\n")
	b.WriteString("- title: ")
	b.WriteString(title)
	b.WriteString("\n")

	if len(cap.Task.LastTools) > 0 {
		b.WriteString("\n## last tools\n")
		for _, tool := range cap.Task.LastTools {
			b.WriteString("- ")
			b.WriteString(tool)
			b.WriteString("\n")
		}
	}
	if len(cap.Task.Todos) > 0 {
		b.WriteString("\n## todos\n")
		for _, todo := range cap.Task.Todos {
			b.WriteString("- ")
			b.WriteString(todo)
			b.WriteString("\n")
		}
	}
	if cap.Task.Head != "" {
		b.WriteString("\n## head\n")
		b.WriteString(cap.Task.Head)
		b.WriteString("\n")
	}
	return b.String()
}
