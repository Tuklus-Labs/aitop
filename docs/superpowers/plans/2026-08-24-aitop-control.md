# aitop control Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make aitop a controller: highlight a row and fork/clone/message/restart/kill/promote/budget/open logs, including a packing-gated llama-server fanout, with split-view merge of cognition forks.

**Architecture:** Occupancy clocks stay (paint, /proc, overlay, inference poller). A fourth Actor goroutine consumes intents the TUI enqueues; adapters (Grok, Claude, Codex, Local) exec off-tick. Paint never opens JSONL or signals. Join gains a locals-only dark-roster root exception. Spec: `docs/superpowers/specs/2026-08-24-aitop-control-design.md`.

**Tech Stack:** Go 1.26, bubbletea, existing aitop dual-index. Tests: `go test ./...`. Instruments follow `~/.claude/STYLE.md` and `aitop/STYLE.md`. TDD. Loud `Fatalf` names the invariant. Do not rewrite `hermes-*.service`. Do not SIGKILL from a key.

**Work from:** `/home/aegis/Projects/aitop` branch `control`. Do not commit to `main`.

House: no Rust, no em dashes in new prose, no `sudo`. `systemctl --user` only. Deep-tests: append Control rows to `tests/RISK_MODEL.md` and PF-C* to `tests/SABOTAGE_LOG.md` as each slice lands; do not claim a slice green without a planted-failure watch.

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/types/types.go` | Overlay fields: ForkOf, CapsuleID, Worktree, Kind, TokPerSec, SlotIndex, Dark |
| `internal/act/act.go` | Intent, Actor, queue, dispatch |
| `internal/act/capsule.go` | Write capsule.json + capsule.md |
| `internal/act/worktree.go` | `git worktree add` helper |
| `internal/act/pack.go` | VRAM/RAM packing gate |
| `internal/act/port.go` | Next free port 8180-8399 |
| `internal/act/kill.go` | SIGINT then SIGTERM, pid-reuse, never SIGKILL |
| `internal/act/adapter.go` | Adapter interface, ErrUnsupported |
| `internal/act/grok/grok.go` | Grok argv for fork/clone/resume |
| `internal/act/claude/claude.go` | Claude argv |
| `internal/act/codex/codex.go` | Codex argv |
| `internal/act/local/local.go` | systemd-run fanout, systemctl stop/restart, template-protect |
| `internal/overlay/forks/forks.go` | Read `$XDG_RUNTIME_DIR/aitop/forks/*.json` |
| `internal/overlay/local/local.go` | Dark roster from unit ExecStart |
| `internal/overlay/inference/inference.go` | Per-slot child overlays + tok/s |
| `internal/join/join.go` | Dark local roots; nest by parent key |
| `internal/ui/model.go` | Keys, confirm, prompt, enqueue |
| `internal/ui/view.go` | Footer, T/S column, split, pending |
| `cmd/aitop/main.go` | Start Actor, pass enqueue into ui.New |
| `README.md` | Keys table |

---

### Task 1: Overlay fields

**Files:**
- Modify: `internal/types/types.go`
- Modify: `internal/types/types_test.go` (create if assertions belong there; else add to existing)
- Test: `internal/types/types_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/types/types_test.go`:

```go
func TestOverlayControlFieldsZeroMeanAbsent(t *testing.T) {
	var o Overlay
	if o.ForkOf != "" || o.CapsuleID != "" || o.Worktree != "" || o.Kind != "" || o.TokPerSec != nil || o.SlotIndex != nil || o.Dark {
		t.Fatalf("control-fields-zero-mean-absent violated: %+v", o)
	}
}
```

- [ ] **Step 2: Run it, expect FAIL** (`undefined: o.ForkOf` etc.)

Run: `go test ./internal/types/ -count=1 -run TestOverlayControlFieldsZeroMeanAbsent`

- [ ] **Step 3: Add fields to Overlay**

After `Entrypoint`:

```go
	ForkOf     string   // parent session id for a fork/clone/fanout
	CapsuleID  string
	Worktree   string
	Kind       string   // fork | clone | fanout | slot
	TokPerSec  *float64 // nil = unknown; never coerce 0 on first sample
	SlotIndex  *int     // llama-server slot; nil = not a slot row
	Dark       bool     // local unit with no pid
```

- [ ] **Step 4: `go test ./internal/types/ -count=1` PASS**

- [ ] **Step 5: Commit** `types: overlay fields for fork, fanout, slots, tok/s`

---

### Task 2: Adapter interface, Intent, Actor queue

**Files:**
- Create: `internal/act/adapter.go`, `internal/act/act.go`, `internal/act/act_test.go`
- Modify: `tests/RISK_MODEL.md` (add Control section stub that Task 2 fills: actor never on paint path, unsupported is loud, queue bound)

- [ ] **Step 1: Write failing tests** in `internal/act/act_test.go`

```go
package act

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"aitop/internal/types"
)

type fakeAdapter struct {
	name     types.Runtime
	forks    atomic.Int32
	lastArgs []string
	err      error
}

func (f *fakeAdapter) Name() types.Runtime { return f.name }
func (f *fakeAdapter) Fork(context.Context, Target, Capsule, string) (Spawned, error) {
	f.forks.Add(1)
	return Spawned{}, f.err
}
func (f *fakeAdapter) Clone(context.Context, Target, Capsule) (Spawned, error) {
	return Spawned{}, ErrUnsupported
}
func (f *fakeAdapter) Message(context.Context, Target, string) error { return ErrUnsupported }
func (f *fakeAdapter) Restart(context.Context, Target) error         { return f.err }
func (f *fakeAdapter) Kill(context.Context, Target) error            { return f.err }
func (f *fakeAdapter) Promote(context.Context, Target, string) error { return ErrUnsupported }
func (f *fakeAdapter) Budget(context.Context, Target, string) error  { return ErrUnsupported }
func (f *fakeAdapter) Merge(context.Context, Target, Target, Target) error {
	return ErrUnsupported
}
func (f *fakeAdapter) Transcript(context.Context, Target) (string, error) {
	return "", ErrUnsupported
}
func (f *fakeAdapter) Fanout(context.Context, Target, int) error { return ErrUnsupported }

func TestUnsupportedErrorContainsUnsupported(t *testing.T) {
	if ErrUnsupported == nil || !errors.Is(ErrUnsupported, ErrUnsupported) {
		t.Fatalf("unsupported-is-loud violated: ErrUnsupported=%v", ErrUnsupported)
	}
	if !containsUnsupported(ErrUnsupported.Error()) {
		t.Fatalf("unsupported-error-contains-unsupported violated: %q", ErrUnsupported.Error())
	}
}

func containsUnsupported(s string) bool {
	return len(s) >= 11 && (s == "unsupported" || len(s) > 11 && (containsUnsupportedWord(s)))
}
func containsUnsupportedWord(s string) bool {
	for i := 0; i+11 <= len(s); i++ {
		if s[i:i+11] == "unsupported" {
			return true
		}
	}
	return false
}

func TestActorRunsOffCaller(t *testing.T) {
	ad := &fakeAdapter{name: types.RuntimeGrok}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	a.Start()
	t.Cleanup(a.Stop)
	if err := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "pid:1:1"}}); err != nil {
		t.Fatalf("enqueue-fork violated: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ad.forks.Load() == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("actor-runs-intent-off-caller violated: forks=%d", ad.forks.Load())
}

func TestQueueFullDoesNotDropConfirmedKill(t *testing.T) {
	block := make(chan struct{})
	ad := &blockingAdapter{block: block} // implement Adapter; Kill waits on block
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	a.bound = 1
	a.Start()
	t.Cleanup(func() { close(block); a.Stop() })
	_ = a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "a"}})
	err := a.Enqueue(Intent{Op: OpClone, Target: Target{Runtime: types.RuntimeGrok, Key: "b"}})
	if err == nil {
		t.Fatalf("queue-full-rejects-non-kill violated: err=nil")
	}
	if err := a.Enqueue(Intent{Op: OpKill, Target: Target{Runtime: types.RuntimeGrok, Key: "a"}, Confirmed: true}); err != nil {
		t.Fatalf("confirmed-kill-displaces-or-enqueues violated: %v", err)
	}
}
```

`blockingAdapter` must implement Adapter; Kill/Fork wait on `block`. Keep it in the test file.

Also test: `Enqueue` of OpMessage to a runtime with only ErrUnsupported records a Result whose error string contains `unsupported`.

- [ ] **Step 2: Run, expect FAIL** (`go test ./internal/act/ -count=1`)

- [ ] **Step 3: Implement** `adapter.go` + `act.go`

```go
var ErrUnsupported = errors.New("unsupported")

type Op string
const (
	OpFork Op = "fork"
	OpClone Op = "clone"
	OpMessage Op = "message"
	OpRestart Op = "restart"
	OpKill Op = "kill"
	OpPromote Op = "promote"
	OpBudget Op = "budget"
	OpMerge Op = "merge"
	OpFanout Op = "fanout"
	OpTranscript Op = "transcript"
)

type Intent struct {
	Op        Op
	Target    Target
	Args      string // prompt text, model id, "ctx,np", merge winner key
	N         int    // fanout count
	Confirmed bool
	Parent    Target // merge
	Winner    Target
	Loser     Target
}

type Result struct {
	Key    string
	Op     Op
	Err    string
	At     time.Time
}

type Actor struct {
	adapters map[types.Runtime]Adapter
	ch       chan Intent
	bound    int
	results  atomic.Pointer[Result] // last result, footer reads
	run      func(Intent)           // tests spy; nil = real dispatch
	cancel   context.CancelFunc
}

func New(ads map[types.Runtime]Adapter) *Actor {
	return &Actor{adapters: ads, bound: 16, ch: make(chan Intent, 16)}
}
```

`Start` launches one goroutine reading `ch`. Same `Target.Key` serializes (in-flight map + wait). Cap 4 in flight. `Enqueue`: if channel full and Op==OpKill && Confirmed, try to drop one non-kill from a side buffer or use a dedicated kill slot of size 1. Simplest correct implementation: `bound` is cap of `ch`; on full, if kill+confirmed, spawn a one-shot goroutine for that kill (still serialize per key via mutex map). Document that in a comment: confirmed kill bypasses the bounded data queue.

Dispatch switch calls the adapter. Store Result.

Capsule type stub:

```go
type Capsule struct {
	Schema int    `json:"schema"`
	ID     string `json:"id"`
	Kind   string `json:"kind"`
}
```

Full capsule lands in Task 8. Stub is enough to compile Fork/Clone signatures.

- [ ] **Step 4: `go test ./internal/act/ -count=1` PASS**

- [ ] **Step 5: Commit** `act: intent queue, adapter interface, unsupported is loud`

---

### Task 3: Kill: SIGINT, grace, SIGTERM, never SIGKILL, pid-reuse, refuse self

**Files:**
- Create: `internal/act/kill.go`, `internal/act/kill_test.go`
- Modify: `internal/act/act.go` dispatch OpKill through Killer

- [ ] **Step 1: Failing tests**

```go
func TestKillSendsSIGINTFirstNeverSIGKILL(t *testing.T) {
	var got []syscall.Signal
	k := &spyKiller{pid: 9, start: 11, self: 1, signal: func(pid int32, start uint64, sig syscall.Signal) error {
		got = append(got, sig)
		return nil
	}}
	if err := Kill(k, Target{PID: 9, StartTime: 11}, 0); err != nil { // grace 0 so SIGTERM follows in test
		t.Fatalf("kill-returns: %v", err)
	}
	if len(got) < 1 || got[0] != syscall.SIGINT {
		t.Fatalf("kill-sends-sigint-first violated: %v", got)
	}
	for _, s := range got {
		if s == syscall.SIGKILL {
			t.Fatalf("kill-never-sends-sigkill violated: %v", got)
		}
	}
	if len(got) < 2 || got[1] != syscall.SIGTERM {
		t.Fatalf("kill-escalates-to-sigterm violated: %v", got)
	}
}

func TestKillPidReuseDoesNotSignal(t *testing.T) {
	called := 0
	k := &spyKiller{pid: 9, start: 99, self: 1, signal: func(int32, uint64, syscall.Signal) error {
		called++
		return nil
	}}
	err := Kill(k, Target{PID: 9, StartTime: 11}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "pid-reuse") {
		t.Fatalf("kill-pid-reuse-is-loud violated: err=%v", err)
	}
	if called != 0 {
		t.Fatalf("kill-pid-reuse-does-not-signal violated: called=%d", called)
	}
}

func TestKillRefusesSelf(t *testing.T) {
	called := 0
	k := &spyKiller{pid: 7, start: 1, self: 7, signal: func(int32, uint64, syscall.Signal) error {
		called++
		return nil
	}}
	err := Kill(k, Target{PID: 7, StartTime: 1}, time.Second)
	if err == nil || called != 0 {
		t.Fatalf("kill-refuses-self violated: err=%v called=%d", err, called)
	}
}
```

`spyKiller` implements:

```go
type Killer interface {
	LiveStart(pid int32) (start uint64, ok bool) // ok=false means process gone
	SelfPID() int32
	Signal(pid int32, sig syscall.Signal) error
}
```

Pid-reuse: LiveStart returns start 99 when target.StartTime is 11.

- [ ] **Step 2: FAIL** `go test ./internal/act/ -count=1 -run TestKill`

- [ ] **Step 3: Implement `kill.go`.** Production Killer reads `/proc/<pid>/stat` starttime (same parse as proc package). Signal via `syscall.Kill`. Grace default 5s; tests pass 0. After SIGINT, if still live with same starttime, SIGTERM. Never SIGKILL.

- [ ] **Step 4: PASS**

- [ ] **Step 5: Commit** `act: kill is SIGINT then SIGTERM, never SIGKILL, pid-reuse is loud`

Plant PF-C6 and PF-C7 in `tests/SABOTAGE_LOG.md` (watch the tests fail if you invert SIGINT/SIGKILL or drop the starttime check). Record the plant.

---

### Task 4: Key remap, confirm, enqueue (no exec on tick)

**Files:**
- Modify: `internal/ui/model.go`, `internal/ui/view.go`, `internal/ui/view_test.go`
- Modify: `cmd/aitop/main.go` to construct Actor and pass `Enqueue` into `ui.New`

`c` and `r` currently sort. `k` currently moves up. Spec: `k` kill, `c` clone, `r` restart, sort under `s` then letter.

- [ ] **Step 1: Extend `TestKeysSortFilterAndExpand`** (or new `TestActionKeysEnqueueAndConfirm`)

Must assert:

1. After window+tick, key `k` does **not** move cursor up. It enters confirm mode. View contains `kill` and `y/N`.
2. Key `c` does not change sort to CPU; it enqueues clone only after we decide clone needs no confirm (spec: clone does not confirm; kill does). So `c` enqueues OpClone.
3. Sort: `s` then `c` sorts CPU (existing first-row assertion can reuse fixture).
4. Tick does not increment a spy exec/enqueue counter. Enqueue spy increments only on confirmed kill / on clone key.

Wire `Model` with `enqueue func(act.Intent) error` (nil-safe). Tests set a recording func.

Also update `TestKeysSortFilterAndExpand` because `n` still sorts name (spec keeps `s` then `n`, so `n` alone must NOT sort). **This will fail the existing test.** Update that test to send `s` then `n`. Same for any test that sends `c`/`r` as sort.

Footer must contain `k kill` not `k` as move.

- [ ] **Step 2: FAIL** (`sort-by-name` will fail when `n` does nothing; new kill test fails)

- [ ] **Step 3: Implement keys** in `model.go`:

| key | action |
|-----|--------|
| `k` | confirm kill (not move) |
| `j` / down / up / arrows / g/G / pg | move (up is arrows and `ctrl+u` only; no vim `k`) |
| `c` | enqueue clone |
| `r` | confirm restart |
| `f` | enqueue fork (local later prompts N; agents fork immediately) |
| `m` `p` `b` | prompt line (can stub enqueue until Task 12; still enter prompt mode) |
| `s` | sortPrefix=true; next `c r t a n $` sorts |
| `R` | reverse (unchanged) |
| `v` | mark (stub ok) |
| Enter | expand/collapse still; leaf opens detail until Task 11 pager |

Confirm: `y` enqueues with Confirmed true; anything else cancels. Esc cancels.

`ui.New(src, theme, enqueue)`. `cmd/aitop` creates actor, `ui.New(..., actor.Enqueue)`. `--json`/`--screenshot` do not start actor.

- [ ] **Step 4: `go test ./internal/ui/ ./internal/act/ ./cmd/aitop/ -count=1` PASS**

- [ ] **Step 5: Commit** `ui: btop action keys; k is kill; sort lives under s`

Plant PF-C1: spy on enqueue during tickMsg; count stays 0.

---

### Task 5: Dark roster + join exception

**Files:**
- Modify: `internal/join/join.go`, `internal/join/join_test.go`
- Modify: `internal/overlay/local/local.go`, `internal/overlay/local/local_test.go`

Keep `TestOverlayOnlyPIDDoesNotCreateRootRow` green (ghost pid 99 still not a root). Add:

```go
func TestDarkLocalUnitIsRootOff(t *testing.T) {
	rows := Join(nil, []types.Overlay{{
		Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "off", Dark: true, Title: "Iris: Qwen3.8",
	}})
	if len(rows) != 1 || !rows[0].OverlayOnly || rows[0].Overlay.SessionName != "hermes-qwen38" || rows[0].Overlay.Status != "off" {
		t.Fatalf("dark-local-unit-is-root-off violated: %+v", rows)
	}
}

func TestLivePidJoinsDarkUnitNotDuplicate(t *testing.T) {
	spine := []types.Process{{PID: 10, StartTime: 1, Comm: "llama-server", AgentRoot: true, Role: types.RoleSidecar, Runtime: types.RuntimeLocal}}
	ov := []types.Overlay{
		{Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "off", Dark: true},
		{PID: 10, StartTime: 1, Runtime: types.RuntimeLocal, SessionName: "hermes-qwen38", Status: "idle"},
	}
	rows := Join(spine, ov)
	if len(rows) != 1 {
		t.Fatalf("live-pid-joins-dark-unit-not-duplicate violated: n=%d", len(rows))
	}
	if rows[0].Overlay.Dark || rows[0].Overlay.Status != "idle" || rows[0].Process.PID != 10 {
		t.Fatalf("live-overlay-wins-dark-row violated: %+v", rows[0])
	}
}

func TestGrokOverlayWithoutPidStillNotRoot(t *testing.T) {
	rows := Join(nil, []types.Overlay{{Runtime: types.RuntimeGrok, SessionID: "x", ParentSession: ""}})
	if len(rows) != 0 {
		t.Fatalf("grok-overlay-without-pid-is-not-root violated: n=%d", len(rows))
	}
}
```

Join change: overlays with `Runtime==Local && SessionName!="" && PID==0` are candidate dark roots. After pid loop, emit a root for each dark unit that did not merge onto a live row with the same SessionName.

`local.Collect`: also scan unit files (no pid). If ExecStart contains `llama-server` or `vllm`, emit Dark overlay. Existing pid path unchanged. Fixture: temp unit dir with a hermes-qwen38.service copy of the real ExecStart line.

- [ ] FAIL, implement, PASS, commit `join+local: dark unit roster, live pid wins, grok stays live-only`

Plant PF-C8.

---

### Task 6: Slot children + tok/s

**Files:**
- Modify: `internal/overlay/inference/inference.go`, `inference_test.go`
- Modify: `internal/join/join.go` (nest ParentSession == parent key)
- Modify: `internal/ui/view.go` (T/S column drop after COST), `view_test.go`
- Modify: `internal/present` if status/age need slot rows

Parent key helper (put in `join` or `types`):

```go
func ParentKey(o types.Overlay) string {
	if o.SessionID != "" {
		return o.SessionID
	}
	if o.Runtime == types.RuntimeLocal && o.SessionName != "" {
		return "local:" + o.SessionName
	}
	if o.PID != 0 {
		return "local-pid:" + strconv.Itoa(int(o.PID))
	}
	return ""
}
```

Inference: for each slot, emit overlay `{ParentSession: ParentKey(server), Kind: "slot", SlotIndex: &i, Status: busy/idle, TokensUsed, ContextWindow, TokPerSec}`. Server overlay still has SubagentLive/Declared.

Tok/s: keep last n_decoded per (pid,slot) on the poller; delta/dt; first sample nil.

Join: orphans nest when `c.ParentSession == ParentKey(parent.Overlay)` OR `== parent.Overlay.SessionID` (existing).

Test: slot with ParentSession `local:qwen38` nests under a live local row with SessionName qwen38; does not become a root.

Column T/S in view; drop order after COST. Update `TestColumnDropOrderIsCostCtxTokFirst` to Cost, T/S, Ctx, Tok... or keep COST first then T/S then CTX. Spec: drop T/S after COST. So drop order: COST=1, T/S=2, CTX=3, TOK=4, ... shift the rest.

- [ ] Commit `inference+join+ui: slot children, tok/s column`

Plant PF-C9.

---

### Task 7: Local fanout + packing gate + template protect

**Files:**
- Create: `internal/act/pack.go`, `internal/act/port.go`, `internal/act/local/local.go`, tests
- Modify: Actor dispatch OpFanout / OpClone for RuntimeLocal

Packing tests (no exec):

```go
func TestPackingGateRefusesSecondGPUHeavy(t *testing.T) {
	vram := func() (used, total uint64, err error) { return 23000 << 20, 24576 << 20, nil }
	err := Pack(PackInput{
		VRAM: vram,
		GPUHeavy: true,
		LiveRSS: 20 << 30,
		Watchdog: 0.85,
	})
	if err == nil || !strings.Contains(err.Error(), "vram") {
		t.Fatalf("packing-gate-refuses-second-gpu-heavy violated: %v", err)
	}
}
```

Fanout exec spy:

```go
func TestLocalCloneDoesNotTouchHermesUnitFile(t *testing.T) {
	var writes []string
	var argv [][]string
	l := local.Adapter{
		Run: func(name string, args ...string) error {
			argv = append(argv, append([]string{name}, args...))
			return nil
		},
		WriteFile: func(path string, _ []byte) error { writes = append(writes, path); return nil },
		UsedPorts: func() []int { return []int{8193} },
		BindOK:    func(int) bool { return true },
		Pack:      func(local.PackInput) error { return nil },
	}
	_, err := l.Clone(context.Background(), act.Target{
		Unit: "hermes-qwen38",
		Overlay: types.Overlay{ /* argv via a field: put template argv on Target */ },
	}, act.Capsule{Kind: "fanout"})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	for _, w := range writes {
		if strings.Contains(w, "hermes-") && strings.Contains(w, "systemd") {
			t.Fatalf("local-clone-does-not-rewrite-hermes-unit violated: wrote %s", w)
		}
	}
	joined := strings.Join(argv[0], " ")
	if !strings.Contains(joined, "systemd-run") {
		t.Fatalf("local-clone-uses-systemd-run violated: %s", joined)
	}
	if strings.Contains(joined, "--port 8193") || strings.Contains(joined, "--port=8193") {
		t.Fatalf("local-clone-gets-new-port violated: %s", joined)
	}
}

func TestBudgetOnHermesTemplateRefuses(t *testing.T) {
	l := local.Adapter{Run: func(string, ...string) error { t.Fatalf("budget-on-hermes-does-not-exec violated"); return nil }}
	err := l.Budget(context.Background(), act.Target{Unit: "hermes-qwen38"}, "131072,4")
	if err == nil || !strings.Contains(err.Error(), "clone") {
		t.Fatalf("budget-on-hermes-template-refuses violated: %v", err)
	}
}
```

Need template argv on Target. Add `Target.Argv []string` and `Target.TemplatePort int`. Overlay collectors later fill this; for Task 7 tests pass argv in.

Port picker: 8180-8399, skip used, bind-check injected.

systemd-run `--user --unit=aitop-fanout-<id> --property=Restart=no` then the llama-server argv with replaced `--port` and `--slot-save-path`.

Kill local with Unit set: `systemctl --user stop <unit>.service` (add `.service` if missing). Inject Run.

- [ ] Commit `act/local: packing-gated fanout via systemd-run; hermes templates are read-only`

Plant PF-C3, C4, C5, C15.

---

### Task 8: Capsule writer

**Files:**
- Create: `internal/act/capsule.go`, `internal/act/capsule_test.go`

Write to a temp dir in tests (`t.TempDir()` as XDG_RUNTIME_DIR).

```go
func TestCapsuleWrittenBeforeCallerContinues(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, Capsule{
		Schema: 1, ID: "01TEST", Kind: "fork",
		Parent: CapsuleParent{Runtime: "grok", SessionID: "p", Model: "grok-4.6", CWD: "/home/aegis/Projects/aitop", Title: "x"},
		Child:  CapsuleChild{Runtime: "grok", SessionID: "c", CWD: "/tmp/wt"},
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(path, "capsule.json"))
	md, _ := os.ReadFile(filepath.Join(path, "capsule.md"))
	if !bytes.Contains(raw, []byte(`"schema":1`)) || !bytes.Contains(md, []byte("You are a branch")) {
		t.Fatalf("capsule-json-and-md-exist violated json=%s md=%s", raw, md)
	}
	if bytes.Contains(md, []byte("wait for the parent")) {
		t.Fatalf("capsule-must-not-tell-child-to-wait violated")
	}
}
```

`capsule.md` must include: you are a branch, parent still running, ancestry, cwd, model, title. `task.head` omitted when empty.

- [ ] Commit `act: capsule json+md; child exists without waiting on parent`

Plant PF-C10 (ordering: Actor.Fork writes capsule, then calls adapter.Fork; test with a fake adapter that checks the file exists).

---

### Task 9: Grok, Claude, Codex fork vs clone argv

**Files:**
- Create: `internal/act/grok/grok.go` + test, `claude/claude.go` + test, `codex/codex.go` + test
- Create: `internal/act/worktree.go` (git helper, injected Run)

Adapters do not exec in tests: `Run func(name string, args ...string) error` captures argv.

```go
func TestGrokForkIsNotForkSession(t *testing.T) {
	var argv []string
	g := grok.Adapter{Run: func(name string, args ...string) error {
		argv = append([]string{name}, args...)
		return nil
	}, Worktree: func(string) (string, error) { return "/tmp/wt", nil }}
	_, err := g.Fork(ctx, target, cap, "grok-4.6")
	if err != nil { t.Fatal(err) }
	s := strings.Join(argv, " ")
	if strings.Contains(s, "--fork-session") || strings.Contains(s, "--resume") {
		t.Fatalf("fork-argv-is-not-clone violated: %s", s)
	}
	if !strings.Contains(s, "-p") || !strings.Contains(s, "--prompt-file") || !strings.Contains(s, "--session-id") {
		t.Fatalf("fork-argv-missing-new-session violated: %s", s)
	}
}

func TestGrokCloneUsesForkSession(t *testing.T) {
	// --resume and --fork-session required
}
```

Same pattern for Claude (`--fork-session` only on Clone; Fork has `--worktree` and `--permission-mode`). Codex: Fork is `codex exec --cd`; Clone is `codex fork`.

Worktree helper: `git worktree add <repo>/.worktrees/aitop-fork-<id> -b aitop-fork-<id>`. If git fails, adapter returns error and does not exec the runtime.

If cwd is not a git repo, Fork/Clone return an error containing `worktree`.

- [ ] Commit `act: grok/claude/codex fork is new session; clone is --fork-session`

Plant PF-C2.

---

### Task 10: Fork sidecar + nest by session not ppid

**Files:**
- Create: `internal/overlay/forks/forks.go`, `forks_test.go`
- Modify: `internal/snapshot/engine.go` collectOverlays to include forks.Collect
- Modify: `internal/join/join_test.go`

Sidecar `$DIR/forks/<child>.json`:

```json
{"parent":"P","fork_of":"P","kind":"fork","capsule_id":"01","worktree":"/tmp/wt","child_session":"C"}
```

Collect emits overlay SessionID=C, ParentSession=P, ForkOf=P, Kind=fork, Worktree=..., CapsuleID=..., PID=0.

Join: existing orphan nest by ParentSession. Test: parent grok pid 5 session P, child overlay session C parent P, ppid would be 1 if it had a pid; even OverlayOnly child nests under P.

```go
func TestForkChildNestsBySessionNotPPID(t *testing.T) {
	spine := []types.Process{{PID: 5, StartTime: 1, AgentRoot: true, Role: types.RolePrimary, Runtime: types.RuntimeGrok}}
	ov := []types.Overlay{
		{PID: 5, StartTime: 1, SessionID: "P", Runtime: types.RuntimeGrok},
		{SessionID: "C", ParentSession: "P", ForkOf: "P", Kind: "fork", Worktree: "/tmp/wt"},
	}
	rows := Join(spine, ov)
	if len(rows) != 1 || len(rows[0].Children) != 1 || rows[0].Children[0].Overlay.SessionID != "C" {
		t.Fatalf("fork-child-nests-by-session-not-ppid violated: %+v", rows)
	}
}
```

Actor after successful Fork writes the sidecar (inject dir).

- [ ] Commit `overlay: fork sidecar join nests by session`

Plant PF-C11.

---

### Task 11: Enter pager, mark, split, merge

**Files:**
- Modify: `internal/ui/model.go`, `view.go`, `view_test.go`
- Create: `internal/act/merge.go`, `internal/act/merge_test.go`
- Transcript cache: overlay clock or a small `internal/act` reader that the UI only displays. Paint reads `atomic.Pointer[[]string]` per session. A goroutine in Actor or overlay fills it. Simplest: Actor.Transcript returns a path; a `tailer` goroutine in snapshot/engine reads off overlay clock into `Snapshot.Logs map[string][]string`. UI paints that. **Do not read the file in `View()` or `Update(tickMsg)`.**

Test PF-C1 still: tick does not open files (existing overlay spy).

Pager: Enter on a leaf with SessionPath or logs entry toggles `modePager`. `q` returns. View shows cached lines.

`v` marks `markKey`. Second mark of same ancestry + Enter -> `modeSplit` left=parent right=other. Width < 120: footer `unsupported: split needs 120` and stay in pager.

Merge tests:

```go
func TestMergeConflictDoesNotAbort(t *testing.T) {
	var argv [][]string
	m := Merger{Run: func(name string, args ...string) error {
		argv = append(argv, append([]string{name}, args...))
		if name == "git" && len(args) > 0 && args[0] == "merge" {
			return errors.New("conflict")
		}
		return nil
	}}
	err := m.Merge(parent, winner, loser)
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("merge-conflict-is-loud violated: %v", err)
	}
	for _, a := range argv {
		s := strings.Join(a, " ")
		if strings.Contains(s, "--abort") {
			t.Fatalf("merge-conflict-does-not-abort violated: %s", s)
		}
	}
}
```

Clean merge: `git -C <parent cwd> merge --no-ff -m "aitop merge <winner>" <winner branch>`. Then Message parent; if unsupported, write `aitop-merge-<id>.md` into parent session dir (inject). Loser sidecar status=merged.

`M` in split + confirm `y` enqueues OpMerge.

- [ ] Commit `ui+act: transcript pager, split view, merge without abort`

Plant PF-C12.

---

### Task 12: Agent message/promote/budget + README + canary

**Files:**
- Modify adapters for Message/Promote/Budget
- Modify `internal/ui` prompt line already stubbed: wire args into Intent.Args
- Modify `README.md` keys table
- Modify `tests/SABOTAGE_LOG.md`, `tests/RISK_MODEL.md` remaining rows
- `go test ./...`

Codex Message: argv `codex queue --thread <id> --message <text>`. Grok/Claude Message returns ErrUnsupported (test contains `unsupported`).

Promote: next Restart/Fork uses `-m`. Store override in Actor map keyed by session; Restart argv includes it.

Budget: Claude `--effort`. Grok `--max-turns` if Args is an int, else unsupported. Codex `-c` reasoning.

README keys: replace sort `c r t a n` with actions + `s` sort. Document local fanout and packing.

`--json` includes new fields when set (zero absent). Canary still present. Empty snapshot still canary (existing test).

- [ ] `go test ./... -count=1` PASS
- [ ] Commit `act+ui: message/promote/budget; README keys; control sabotage rows`

Plant PF-C13, PF-C14.

---

## Sabotage plants (do not skip)

Each PF-C* is in the spec. The task that implements the behavior writes the SABOTAGE_LOG row in the same commit, having watched the test fail on a plant in that session (invert the production line, run, restore, run).

## Notes for implementers

- Match existing assertion style: `t.Fatalf("kebab-invariant violated: ...")`.
- Do not start slice 4 (Task 8+) before Task 4 is green. Spec: a cognition fork while `k` still means up will kill things.
- `cmd/aitop` screenshot/json paths must not Start() the Actor.
- No live `grok -p` in unit tests. Argv capture only.
- Live systemctl tests FAIL by default; SKIP with `AITOP_LIVE_SYSTEMCTL=0` named opt-out, or run only if `AITOP_LIVE_SYSTEMCTL=1`. Default is skip-absent-not-pass: if the env is unset, skip with `t.Skip("AITOP_LIVE_SYSTEMCTL unset")` only for tests that would talk to systemd; unit tests using injected Run never skip.
- VRAM reader production: try `rocm-smi --showmeminfo vram --csv` parse; if it fails, packing gate errors closed (refuse spawn) rather than open. Tests inject.

## Self-review (plan vs spec)

| Spec slice | Tasks |
|------------|-------|
| Actor + adapters + 100ms isolation | 2, 4 |
| Kill/restart confirm, SIGINT/TERM, pid-reuse, self | 3, 4 |
| Key remap btop grammar | 4 |
| Dark roster join exception | 5 |
| Slots as children, tok/s, parent key | 6 |
| Local fanout, packing, template protect, ports | 7 |
| Capsule hybrid, spawn does not wait | 8, 9 |
| Agent f vs c argv, worktree | 9 |
| Sidecar join by session | 10 |
| Enter, split, merge no abort | 11 |
| m/p/b, unsupported loud, README, canary | 12 |
| Sequence 1..6 | Tasks 1-4, 5-6, 7, 8-10, 11, 12 |
