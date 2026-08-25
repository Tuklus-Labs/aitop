package claude

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aitop/internal/act"
	"aitop/internal/types"
)

// Adapter runs Claude CLI for cognition fork/clone. Fork is a new session plus
// capsule prompt, never --fork-session. Clone is --resume --fork-session.
type Adapter struct {
	Run      func(name string, args ...string) error
	Worktree func(cwd, id string) (string, error)
	Killer   act.Killer
	Grace    time.Duration
	prefs    act.Prefs
}

func New() *Adapter {
	return &Adapter{
		Run:      act.StartCLI,
		Worktree: act.GitWorktree,
		Killer:   act.ProcKiller{},
		Grace:    act.KillGrace,
	}
}

func (a *Adapter) Name() types.Runtime { return types.RuntimeClaude }

func (a *Adapter) Fork(ctx context.Context, t act.Target, cap act.Capsule, model string) (act.Spawned, error) {
	if err := ctx.Err(); err != nil {
		return act.Spawned{}, err
	}
	wt, id, err := act.PrepareWorktree(a.Worktree, t, cap)
	if err != nil {
		return act.Spawned{}, err
	}
	session, err := act.ChildSession(cap)
	if err != nil {
		return act.Spawned{}, err
	}
	m := a.modelFor(t, model)
	args := []string{
		"-p",
		"--worktree", act.WorktreeName(id),
		"--session-id", session,
	}
	if m != "" {
		args = append(args, "--model", m)
	}
	if e := a.budgetFor(t); e != "" {
		args = append(args, "--effort", e)
	}
	args = append(args, "--permission-mode", permissionMode(t), act.CapsulePrompt(cap))
	if err := a.run("claude", args...); err != nil {
		return act.Spawned{}, err
	}
	return act.Spawned{SessionID: session, Worktree: wt, CapsuleID: cap.ID}, nil
}

func (a *Adapter) Clone(ctx context.Context, t act.Target, cap act.Capsule) (act.Spawned, error) {
	if err := ctx.Err(); err != nil {
		return act.Spawned{}, err
	}
	wt, id, err := act.PrepareWorktree(a.Worktree, t, cap)
	if err != nil {
		return act.Spawned{}, err
	}
	session, err := act.ChildSession(cap)
	if err != nil {
		return act.Spawned{}, err
	}
	args := []string{
		"-p",
		"--resume", t.SessionID,
		"--fork-session",
		"--worktree", act.WorktreeName(id),
		"--session-id", session,
	}
	if e := a.budgetFor(t); e != "" {
		args = append(args, "--effort", e)
	}
	if err := a.run("claude", args...); err != nil {
		return act.Spawned{}, err
	}
	return act.Spawned{SessionID: session, Worktree: wt, CapsuleID: cap.ID}, nil
}

func (a *Adapter) Kill(_ context.Context, t act.Target) error {
	k := a.Killer
	if k == nil {
		k = act.ProcKiller{}
	}
	return act.Kill(k, t, a.Grace)
}

func (a *Adapter) Message(context.Context, act.Target, string) error {
	return fmt.Errorf("%w: claude message", act.ErrUnsupported)
}

func (a *Adapter) Restart(context.Context, act.Target) error {
	return fmt.Errorf("%w: claude restart", act.ErrUnsupported)
}

func (a *Adapter) Promote(_ context.Context, t act.Target, spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" || act.SessionKey(t) == "" {
		return fmt.Errorf("%w: claude promote", act.ErrUnsupported)
	}
	a.prefs.SetModel(t, spec)
	return nil
}

func (a *Adapter) Budget(_ context.Context, t act.Target, spec string) error {
	spec = strings.TrimSpace(spec)
	if !claudeEffort(spec) || act.SessionKey(t) == "" {
		return fmt.Errorf("%w: claude budget wants low|medium|high|max", act.ErrUnsupported)
	}
	a.prefs.SetBudget(t, spec)
	return nil
}

func (a *Adapter) Merge(context.Context, act.Target, act.Target, act.Target) error {
	return fmt.Errorf("%w: claude merge", act.ErrUnsupported)
}

func (a *Adapter) Transcript(context.Context, act.Target) (string, error) {
	return "", fmt.Errorf("%w: claude transcript", act.ErrUnsupported)
}

func (a *Adapter) Fanout(context.Context, act.Target, int) error {
	return fmt.Errorf("%w: claude fanout", act.ErrUnsupported)
}

func (a *Adapter) run(name string, args ...string) error {
	if a.Run == nil {
		return fmt.Errorf("claude: Run not configured")
	}
	return a.Run(name, args...)
}

func (a *Adapter) modelFor(t act.Target, model string) string {
	fb := model
	if fb == "" {
		fb = t.Model
	}
	return a.prefs.Model(t, fb)
}

func (a *Adapter) budgetFor(t act.Target) string {
	return a.prefs.Budget(t)
}

func claudeEffort(spec string) bool {
	switch spec {
	case "low", "medium", "high", "max":
		return true
	}
	return false
}

func permissionMode(t act.Target) string {
	allowed := map[string]bool{
		"acceptEdits":       true,
		"auto":              true,
		"bypassPermissions": true,
		"manual":            true,
		"dontAsk":           true,
		"plan":              true,
	}
	for i, a := range t.Argv {
		switch {
		case a == "--permission-mode" && i+1 < len(t.Argv) && allowed[t.Argv[i+1]]:
			return t.Argv[i+1]
		case strings.HasPrefix(a, "--permission-mode="):
			v := strings.TrimPrefix(a, "--permission-mode=")
			if allowed[v] {
				return v
			}
		}
	}
	return "bypassPermissions"
}
