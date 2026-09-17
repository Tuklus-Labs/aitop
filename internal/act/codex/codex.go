package codex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/act"
	"github.com/Tuklus-Labs/aitop/internal/types"
)

// Adapter runs Codex CLI for cognition fork/clone. Fork is `codex exec`, never
// `codex fork`. Clone is `codex fork <parent-session>`.
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

func (a *Adapter) Name() types.Runtime { return types.RuntimeCodex }

func (a *Adapter) Fork(ctx context.Context, t act.Target, cap act.Capsule, model string) (act.Spawned, error) {
	if err := ctx.Err(); err != nil {
		return act.Spawned{}, err
	}
	wt, _, err := act.PrepareWorktree(a.Worktree, t, cap)
	if err != nil {
		return act.Spawned{}, err
	}
	session, err := act.ChildSession(cap)
	if err != nil {
		return act.Spawned{}, err
	}
	m := a.modelFor(t, model)
	args := []string{"exec", "--cd", wt}
	if m != "" {
		args = append(args, "-m", m)
	}
	args = append(args, a.budgetArgs(t)...)
	args = append(args, parentSandbox(t)...)
	args = append(args, act.CapsulePrompt(cap))
	if err := a.run("codex", args...); err != nil {
		return act.Spawned{}, err
	}
	return act.Spawned{SessionID: session, Worktree: wt, CapsuleID: cap.ID}, nil
}

func (a *Adapter) Clone(ctx context.Context, t act.Target, cap act.Capsule) (act.Spawned, error) {
	if err := ctx.Err(); err != nil {
		return act.Spawned{}, err
	}
	wt, _, err := act.PrepareWorktree(a.Worktree, t, cap)
	if err != nil {
		return act.Spawned{}, err
	}
	session, err := act.ChildSession(cap)
	if err != nil {
		return act.Spawned{}, err
	}
	args := []string{"fork", t.SessionID, "--cd", wt}
	if err := a.run("codex", args...); err != nil {
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

func (a *Adapter) Message(ctx context.Context, t act.Target, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.SessionID == "" {
		return fmt.Errorf("codex message: empty session")
	}
	return a.run("codex", "queue", "--thread", t.SessionID, "--message", text)
}

func (a *Adapter) Restart(context.Context, act.Target) error {
	return fmt.Errorf("%w: codex restart", act.ErrUnsupported)
}

func (a *Adapter) Promote(_ context.Context, t act.Target, spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" || act.SessionKey(t) == "" {
		return fmt.Errorf("%w: codex promote", act.ErrUnsupported)
	}
	a.prefs.SetModel(t, spec)
	return nil
}

func (a *Adapter) Budget(_ context.Context, t act.Target, spec string) error {
	spec = strings.TrimSpace(spec)
	if !codexEffort(spec) || act.SessionKey(t) == "" {
		return fmt.Errorf("%w: codex budget wants model_reasoning_effort", act.ErrUnsupported)
	}
	a.prefs.SetBudget(t, spec)
	return nil
}

func (a *Adapter) Merge(context.Context, act.Target, act.Target, act.Target) error {
	return fmt.Errorf("%w: codex merge", act.ErrUnsupported)
}

func (a *Adapter) Transcript(context.Context, act.Target) (string, error) {
	return "", fmt.Errorf("%w: codex transcript", act.ErrUnsupported)
}

func (a *Adapter) Fanout(context.Context, act.Target, int) error {
	return fmt.Errorf("%w: codex fanout", act.ErrUnsupported)
}

func (a *Adapter) run(name string, args ...string) error {
	if a.Run == nil {
		return fmt.Errorf("codex: Run not configured")
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

func (a *Adapter) budgetArgs(t act.Target) []string {
	e := a.prefs.Budget(t)
	if e == "" {
		return nil
	}
	return []string{"-c", fmt.Sprintf("model_reasoning_effort=%q", e)}
}

func codexEffort(spec string) bool {
	switch spec {
	case "minimal", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

func parentSandbox(t act.Target) []string {
	for _, a := range t.Argv {
		if a == "--dangerously-bypass-approvals-and-sandbox" {
			return []string{a}
		}
	}
	return nil
}
