package act

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aitop/internal/types"
)

type fakeAdapter struct {
	name         types.Runtime
	forks        atomic.Int32
	kills        atomic.Int32
	err          error
	forkHook     func()
	killHook     func()
	spawnSession string
}

func (f *fakeAdapter) Name() types.Runtime { return f.name }

func (f *fakeAdapter) Fork(context.Context, Target, Capsule, string) (Spawned, error) {
	f.forks.Add(1)
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
	name    types.Runtime
	started chan struct{}
	block   chan struct{}
	forks   atomic.Int32
	kills   atomic.Int32
	once    sync.Once
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
	a.Start()
	t.Cleanup(a.Stop)

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
	a.Start()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		a.Stop()
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
	a.Start()
	t.Cleanup(func() {
		select {
		case <-block:
		default:
			close(block)
		}
		a.Stop()
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
	a.Start()
	t.Cleanup(a.Stop)

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
	a.Start()
	t.Cleanup(func() {
		select {
		case <-hold:
		default:
			close(hold)
		}
		a.Stop()
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
