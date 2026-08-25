package codex

import (
	"context"
	"fmt"
	"time"

	"aitop/internal/act"
	"aitop/internal/types"
)

// Adapter runs Codex CLI for cognition fork/clone. Fork is `codex exec`, never
// `codex fork`. Clone is `codex fork <parent-session>`.
type Adapter struct {
	Run      func(name string, args ...string) error
	Worktree func(cwd, id string) (string, error)
	Killer   act.Killer
	Grace    time.Duration
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
	m := model
	if m == "" {
		m = t.Model
	}
	args := []string{"exec", "--cd", wt}
	if m != "" {
		args = append(args, "-m", m)
	}
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

func (a *Adapter) Message(context.Context, act.Target, string) error {
	return fmt.Errorf("%w: codex message", act.ErrUnsupported)
}

func (a *Adapter) Restart(context.Context, act.Target) error {
	return fmt.Errorf("%w: codex restart", act.ErrUnsupported)
}

func (a *Adapter) Promote(context.Context, act.Target, string) error {
	return fmt.Errorf("%w: codex promote", act.ErrUnsupported)
}

func (a *Adapter) Budget(context.Context, act.Target, string) error {
	return fmt.Errorf("%w: codex budget", act.ErrUnsupported)
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

func parentSandbox(t act.Target) []string {
	for _, a := range t.Argv {
		if a == "--dangerously-bypass-approvals-and-sandbox" {
			return []string{a}
		}
	}
	return nil
}
