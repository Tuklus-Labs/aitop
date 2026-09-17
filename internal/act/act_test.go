package act

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tuklus-Labs/aitop/internal/types"
)

type fakeAdapter struct {
	name         types.Runtime
	forks        atomic.Int32
	kills        atomic.Int32
	err          error
	forkHook     func()
	killHook     func()
	spawnSession string
	lastCtx      atomic.Value
	mu           sync.Mutex
	forkKeys     []string
}

func (f *fakeAdapter) Name() types.Runtime { return f.name }

func (f *fakeAdapter) snapshotForkKeys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.forkKeys))
	copy(out, f.forkKeys)
	return out
}

func (f *fakeAdapter) Fork(ctx context.Context, t Target, _ Capsule, _ string) (Spawned, error) {
	f.lastCtx.Store(ctx)
	f.forks.Add(1)
	f.mu.Lock()
	f.forkKeys = append(f.forkKeys, t.Key)
	f.mu.Unlock()
	if f.forkHook != nil {
		f.forkHook()
	}
	return Spawned{SessionID: f.spawnSession}, f.err
}

func (f *fakeAdapter) Clone(context.Context, Target, Capsule) (Spawned, error) {
	return Spawned{}, ErrUnsupported
}

func (f *fakeAdapter) Message(context.Context, Target, string) error { return ErrUnsupported }
func (f *fakeAdapter) Restart(context.Context, Target) error         { return f.err }
func (f *fakeAdapter) Kill(context.Context, Target) error {
	f.kills.Add(1)
	if f.killHook != nil {
		f.killHook()
	}
	return f.err
}
func (f *fakeAdapter) Promote(context.Context, Target, string) error { return ErrUnsupported }
func (f *fakeAdapter) Budget(context.Context, Target, string) error  { return ErrUnsupported }
func (f *fakeAdapter) Merge(context.Context, Target, Target, Target) error {
	return ErrUnsupported
}
func (f *fakeAdapter) Transcript(context.Context, Target) (string, error) {
	return "", ErrUnsupported
}
func (f *fakeAdapter) Fanout(context.Context, Target, int) error { return ErrUnsupported }

type blockingAdapter struct {
	name        types.Runtime
	started     chan struct{}
	killStarted chan struct{}
	block       chan struct{}
	forks       atomic.Int32
	kills       atomic.Int32
	once        sync.Once
	killOnce    sync.Once
}

func (b *blockingAdapter) Name() types.Runtime { return b.name }

func (b *blockingAdapter) wait() {
	if b.block != nil {
		<-b.block
	}
}

func (b *blockingAdapter) signalStarted() {
	b.once.Do(func() {
		if b.started != nil {
			close(b.started)
		}
	})
}

func (b *blockingAdapter) Fork(context.Context, Target, Capsule, string) (Spawned, error) {
	b.forks.Add(1)
	b.signalStarted()
	b.wait()
	return Spawned{}, nil
}

func (b *blockingAdapter) Clone(context.Context, Target, Capsule) (Spawned, error) {
	return Spawned{}, ErrUnsupported
}

func (b *blockingAdapter) Message(context.Context, Target, string) error { return ErrUnsupported }
func (b *blockingAdapter) Restart(context.Context, Target) error         { return ErrUnsupported }
func (b *blockingAdapter) Kill(context.Context, Target) error {
	b.kills.Add(1)
	b.killOnce.Do(func() {
		if b.killStarted != nil {
			close(b.killStarted)
		}
	})
	b.wait()
	return nil
}
func (b *blockingAdapter) Promote(context.Context, Target, string) error { return ErrUnsupported }
func (b *blockingAdapter) Budget(context.Context, Target, string) error  { return ErrUnsupported }
func (b *blockingAdapter) Merge(context.Context, Target, Target, Target) error {
	return ErrUnsupported
}
func (b *blockingAdapter) Transcript(context.Context, Target) (string, error) {
	return "", ErrUnsupported
}
func (b *blockingAdapter) Fanout(context.Context, Target, int) error { return ErrUnsupported }

func TestActorForkWritesSidecar(t *testing.T) {
	caps := t.TempDir()
	forksDir := t.TempDir()
	ad := &fakeAdapter{name: types.RuntimeGrok, spawnSession: "C"}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	a.CapsuleDir = caps
	a.ForksDir = forksDir
	runActor(t, a)

	if err := a.Enqueue(Intent{
		Op: OpFork,
		Target: Target{
			Key:       "pid:5:1",
			Runtime:   types.RuntimeGrok,
			SessionID: "P",
			Worktree:  "/tmp/wt",
		},
	}); err != nil {
		t.Fatalf("enqueue-fork violated: %v", err)
	}

	path := filepath.Join(forksDir, "C.json")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var got map[string]string
			if json.Unmarshal(raw, &got) != nil {
				t.Fatalf("fork-sidecar-json violated: %s", raw)
			}
			if got["parent"] != "P" || got["child_session"] != "C" {
				t.Fatalf("fork-sidecar-parent-child violated: %s", raw)
			}
			if got["fork_of"] != "P" || got["kind"] != "fork" || got["worktree"] != "/tmp/wt" {
				t.Fatalf("fork-sidecar-fields violated: %s", raw)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fork-sidecar-written-after-spawn violated: forks=%d result=%v missing %s", ad.forks.Load(), a.LastResult(), path)
}

func TestUnsupportedErrorContainsUnsupported(t *testing.T) {
	if ErrUnsupported == nil || !errors.Is(ErrUnsupported, ErrUnsupported) {
		t.Fatalf("unsupported-is-loud violated: ErrUnsupported=%v", ErrUnsupported)
	}
	if !strings.Contains(ErrUnsupported.Error(), "unsupported") {
		t.Fatalf("unsupported-error-contains-unsupported violated: %q", ErrUnsupported.Error())
	}
}

func TestActorRunsOffCaller(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	ad := &fakeAdapter{
		name: types.RuntimeGrok,
		forkHook: func() {
			close(entered)
			<-release
		},
	}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	runActor(t, a)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})

	done := make(chan error, 1)
	go func() {
		done <- a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "pid:1:1"}})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("enqueue-fork violated: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("actor-runs-intent-off-caller violated: Enqueue blocked on adapter")
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatalf("actor-runs-intent-off-caller violated: forks=%d", ad.forks.Load())
	}
	if ad.forks.Load() != 1 {
		t.Fatalf("actor-runs-intent-off-caller violated: forks=%d", ad.forks.Load())
	}
}

func TestQueueFullDoesNotDropConfirmedKill(t *testing.T) {
	started := make(chan struct{})
	block := make(chan struct{})
	ad := &blockingAdapter{name: types.RuntimeGrok, started: started, block: block}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	a.bound = 1
	runActor(t, a)
	t.Cleanup(func() {
		select {
		case <-block:
		default:
			close(block)
		}
	})

	if err := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "a"}}); err != nil {
		t.Fatalf("enqueue-fork violated: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatalf("queue-full-in-flight-blocked violated: adapter never entered")
	}

	err := a.Enqueue(Intent{Op: OpClone, Target: Target{Runtime: types.RuntimeGrok, Key: "b"}})
	if err == nil {
		t.Fatalf("queue-full-rejects-non-kill violated: err=nil")
	}

	if err := a.Enqueue(Intent{Op: OpKill, Target: Target{Runtime: types.RuntimeGrok, Key: "a"}, Confirmed: true}); err != nil {
		t.Fatalf("confirmed-kill-displaces-or-enqueues violated: %v", err)
	}
}

func TestMessageUnsupportedIsLoudInResult(t *testing.T) {
	ad := &fakeAdapter{name: types.RuntimeGrok}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	runActor(t, a)

	if err := a.Enqueue(Intent{Op: OpMessage, Target: Target{Runtime: types.RuntimeGrok, Key: "pid:1:1"}, Args: "hi"}); err != nil {
		t.Fatalf("enqueue-message violated: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r := a.LastResult(); r != nil && strings.Contains(r.Err, "unsupported") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	got := ""
	if r := a.LastResult(); r != nil {
		got = r.Err
	}
	t.Fatalf("unsupported-is-loud violated: Result.Err=%q", got)
}

func TestSameKeySerializes(t *testing.T) {
	var overlapping atomic.Int32
	entered := make(chan struct{}, 2)
	hold := make(chan struct{})
	ad := &fakeAdapter{
		name: types.RuntimeGrok,
		forkHook: func() {
			if n := overlapping.Add(1); n != 1 {
				t.Errorf("same-key-serializes violated: overlapping=%d", n)
			}
			entered <- struct{}{}
			<-hold
			overlapping.Add(-1)
		},
	}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	runActor(t, a)
	t.Cleanup(func() {
		select {
		case <-hold:
		default:
			close(hold)
		}
	})

	tgt := Target{Runtime: types.RuntimeGrok, Key: "pid:9:9"}
	if err := a.Enqueue(Intent{Op: OpFork, Target: tgt}); err != nil {
		t.Fatalf("enqueue-fork violated: %v", err)
	}
	if err := a.Enqueue(Intent{Op: OpFork, Target: tgt}); err != nil {
		t.Fatalf("enqueue-second-same-key violated: %v", err)
	}

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatalf("same-key-serializes violated: first fork never started")
	}
	select {
	case <-entered:
		t.Fatalf("same-key-serializes violated: second fork started while first in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(hold)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatalf("same-key-serializes violated: second fork never ran")
	}
}

func waitActorRunning(t *testing.T, a *Actor) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		a.mu.Lock()
		running := a.started && !a.stopped
		a.mu.Unlock()
		if running {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("actor-running-state rule violated: started=%t stopped=%t", a.started, a.stopped)
		default:
			runtime.Gosched()
		}
	}
}

func startActorRun(t *testing.T, a *Actor, ctx context.Context) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	waitActorRunning(t, a)
	return done
}

func runActor(t *testing.T, a *Actor) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := startActorRun(t, a, ctx)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			a.mu.Lock()
			started, stopped := a.started, a.stopped
			a.mu.Unlock()
			t.Errorf("actor-run-cleanup rule violated: Run did not return timeout=5s started=%t stopped=%t", started, stopped)
		}
	})
}

func TestActorRunRejectsSecondRun(t *testing.T) {
	ad := &fakeAdapter{name: types.RuntimeGrok}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	ctx, cancel := context.WithCancel(context.Background())
	first := startActorRun(t, a, ctx)
	second := a.Run(context.Background())
	if second == nil {
		t.Fatalf("actor-run-rejects-second-run rule violated: second Run err=%v want non-nil first-running=true", second)
	}
	cancel()
	select {
	case err := <-first:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("actor-run-rejects-second-run rule violated: first Run err=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("actor-run-rejects-second-run rule violated: first Run did not return after cancel timeout=5s second=%v", second)
	}
	third := a.Run(context.Background())
	if third == nil {
		t.Fatalf("actor-run-rejects-second-run rule violated: third Run after stop err=%v want non-nil", third)
	}
}

func TestActorCancellationStopsEnqueueAndDrainsQueued(t *testing.T) {
	t.Run("in-flight-and-late-enqueue", func(t *testing.T) {
		block := make(chan struct{})
		var entered atomic.Int32
		allEntered := make(chan struct{})
		ad := &fakeAdapter{
			name: types.RuntimeGrok,
			forkHook: func() {
				if entered.Add(1) == 4 {
					close(allEntered)
				}
				<-block
			},
		}
		a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
		a.bound = 6
		ctx, cancel := context.WithCancel(context.Background())
		done := startActorRun(t, a, ctx)
		for i := 0; i < 4; i++ {
			in := Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "in-flight:" + string(rune('a'+i))}}
			if err := a.Enqueue(in); err != nil {
				t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: in-flight enqueue i=%d err=%v", i, err)
			}
		}
		select {
		case <-allEntered:
		case <-time.After(5 * time.Second):
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: in-flight adapters never entered forks=%d", ad.forks.Load())
		}
		for i := 0; i < 2; i++ {
			in := Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "queued:" + string(rune('a'+i))}}
			if err := a.Enqueue(in); err != nil {
				t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued enqueue i=%d err=%v", i, err)
			}
		}
		cancel()
		stopDeadline := time.After(5 * time.Second)
		for {
			a.mu.Lock()
			stopping := a.stopping
			a.mu.Unlock()
			if stopping {
				break
			}
			select {
			case <-stopDeadline:
				t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: stopping never set after cancel forks=%d", ad.forks.Load())
			default:
				runtime.Gosched()
			}
		}
		late := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "late"}})
		if late == nil {
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: Enqueue succeeded after cancel forks=%d", ad.forks.Load())
		}
		close(block)
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: Run err=%v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: Run did not return after drain timeout=5s forks=%d stopping=%t", ad.forks.Load(), a.stopping)
		}
		if got := ad.forks.Load(); got != 4 {
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued work dispatched forks=%d want=4", got)
		}
	})

	t.Run("queued-only-not-dispatched", func(t *testing.T) {
		prev := runtime.GOMAXPROCS(1)
		t.Cleanup(func() { runtime.GOMAXPROCS(prev) })
		ad := &fakeAdapter{name: types.RuntimeGrok}
		a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
		a.bound = 8
		ctx, cancel := context.WithCancel(context.Background())
		done := startActorRun(t, a, ctx)
		keys := []string{"queued:a", "queued:b", "queued:c", "queued:d"}
		for i, key := range keys {
			if err := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: key}}); err != nil {
				t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued-only enqueue i=%d key=%s err=%v", i, key, err)
			}
		}
		if got := ad.forks.Load(); got != 0 {
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued-only adapters entered before cancel forks=%d keys=%v", got, ad.snapshotForkKeys())
		}
		cancel()
		late := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "late"}})
		if late == nil {
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued-only Enqueue succeeded after cancel forks=%d ctxErr=%v stopping=%t", ad.forks.Load(), a.ctx.Err(), a.stopping)
		}
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued-only Run err=%v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued-only Run did not return after drain timeout=5s forks=%d keys=%v", ad.forks.Load(), ad.snapshotForkKeys())
		}
		if got := ad.forks.Load(); got != 0 {
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued work dispatched forks=%d want=0 keys=%v", got, ad.snapshotForkKeys())
		}
		if r := a.LastResult(); r != nil && r.Err == "" && r.Op == OpFork {
			t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued key got successful fork LastResult key=%s op=%s err=%q forks=%d", r.Key, r.Op, r.Err, ad.forks.Load())
		}
		for _, key := range keys {
			for _, got := range ad.snapshotForkKeys() {
				if got == key {
					t.Fatalf("actor-cancellation-stops-enqueue-and-drains-queued rule violated: queued key dispatched after cancel key=%s keys=%v", key, ad.snapshotForkKeys())
				}
			}
		}
	})
}

func TestActorCancellationWaitsForInFlightAndConfirmedKill(t *testing.T) {
	block := make(chan struct{})
	ad := &blockingAdapter{
		name:        types.RuntimeGrok,
		started:     make(chan struct{}),
		killStarted: make(chan struct{}),
		block:       block,
	}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	ctx, cancel := context.WithCancel(context.Background())
	done := startActorRun(t, a, ctx)
	if err := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "fork"}}); err != nil {
		t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: fork enqueue err=%v", err)
	}
	select {
	case <-ad.started:
	case <-time.After(5 * time.Second):
		t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: fork never entered timeout=5s forks=%d", ad.forks.Load())
	}
	if err := a.Enqueue(Intent{Op: OpKill, Target: Target{Runtime: types.RuntimeGrok, Key: "kill"}, Confirmed: true}); err != nil {
		t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: confirmed kill enqueue err=%v", err)
	}
	select {
	case <-ad.killStarted:
	case <-time.After(5 * time.Second):
		t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: confirmed kill never entered timeout=5s forks=%d kills=%d", ad.forks.Load(), ad.kills.Load())
	}
	cancel()
	stopDeadline := time.After(5 * time.Second)
	for {
		if err := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "post-cancel"}}); err != nil {
			break
		}
		select {
		case <-stopDeadline:
			t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: Enqueue still accepted after cancel forks=%d kills=%d", ad.forks.Load(), ad.kills.Load())
		default:
			runtime.Gosched()
		}
	}
	for i := 0; i < 10000; i++ {
		select {
		case err := <-done:
			t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: Run returned before workers finished err=%v forks=%d kills=%d sched=%d", err, ad.forks.Load(), ad.kills.Load(), i)
		default:
			runtime.Gosched()
		}
	}
	close(block)
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: Run err=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("actor-cancellation-waits-for-in-flight-and-confirmed-kill rule violated: Run did not return after workers released timeout=5s forks=%d kills=%d", ad.forks.Load(), ad.kills.Load())
	}
}

func TestActorAdapterReceivesRunContext(t *testing.T) {
	entered := make(chan struct{})
	ad := &fakeAdapter{
		name: types.RuntimeGrok,
		forkHook: func() {
			close(entered)
		},
	}
	a := New(map[types.Runtime]Adapter{types.RuntimeGrok: ad})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startActorRun(t, a, ctx)
	if err := a.Enqueue(Intent{Op: OpFork, Target: Target{Runtime: types.RuntimeGrok, Key: "ctx"}}); err != nil {
		t.Fatalf("actor-adapter-receives-run-context rule violated: enqueue err=%v", err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("actor-adapter-receives-run-context rule violated: adapter never entered timeout=5s forks=%d", ad.forks.Load())
	}
	gotCtx, _ := ad.lastCtx.Load().(context.Context)
	if gotCtx == nil || gotCtx != ctx {
		t.Fatalf("actor-adapter-receives-run-context rule violated: got=%p want=%p background=%p", gotCtx, ctx, context.Background())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("actor-adapter-receives-run-context rule violated: Run did not return timeout=5s got=%p want=%p", gotCtx, ctx)
	}
}
