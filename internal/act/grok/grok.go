package grok

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aitop/internal/act"
	"aitop/internal/types"
)

// Adapter runs Grok CLI for cognition fork/clone. Fork is a new session plus
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

func (a *Adapter) Name() types.Runtime { return types.RuntimeGrok }

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
	// --prompt-file is itself headless. Do not pass -p/--single: that flag
	// consumes the next token as the prompt, so `-p --cwd` would prompt `--cwd`.
	args := []string{
		"--cwd", wt,
		"--session-id", session,
	}
	if m != "" {
		args = append(args, "-m", m)
	}
	if n := a.budgetFor(t); n != "" {
		args = append(args, "--max-turns", n)
	}
	args = append(args, "--always-approve", "--prompt-file", act.PromptFile(cap))
	if err := a.run("grok", args...); err != nil {
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
	args := []string{
		"--resume", t.SessionID,
		"--fork-session",
		"--session-id", session,
		"--cwd", wt,
		"--always-approve",
	}
	if err := a.run("grok", args...); err != nil {
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
	return fmt.Errorf("%w: grok message", act.ErrUnsupported)
}

func (a *Adapter) Restart(context.Context, act.Target) error {
	return fmt.Errorf("%w: grok restart", act.ErrUnsupported)
}

func (a *Adapter) Promote(_ context.Context, t act.Target, spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" || act.SessionKey(t) == "" {
		return fmt.Errorf("%w: grok promote", act.ErrUnsupported)
	}
	a.prefs.SetModel(t, spec)
	return nil
}

func (a *Adapter) Budget(_ context.Context, t act.Target, spec string) error {
	spec = strings.TrimSpace(spec)
	n, err := strconv.Atoi(spec)
	if err != nil || n < 1 || act.SessionKey(t) == "" {
		return fmt.Errorf("%w: grok budget wants integer max-turns", act.ErrUnsupported)
	}
	a.prefs.SetBudget(t, strconv.Itoa(n))
	return nil
}

func (a *Adapter) Merge(context.Context, act.Target, act.Target, act.Target) error {
	return fmt.Errorf("%w: grok merge", act.ErrUnsupported)
}

func (a *Adapter) Transcript(context.Context, act.Target) (string, error) {
	return "", fmt.Errorf("%w: grok transcript", act.ErrUnsupported)
}

func (a *Adapter) Fanout(context.Context, act.Target, int) error {
	return fmt.Errorf("%w: grok fanout", act.ErrUnsupported)
}

func (a *Adapter) run(name string, args ...string) error {
	if a.Run == nil {
		return fmt.Errorf("grok: Run not configured")
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
