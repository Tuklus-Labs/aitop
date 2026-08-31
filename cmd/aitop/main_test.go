package main

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aitop/internal/act"
	"aitop/internal/overlay/inference"
	"aitop/internal/snapshot"
	"aitop/internal/supervisor"
	"aitop/internal/types"
)

type registrarSpy struct {
	name string
	run  func(context.Context) error
	err  error
	n    atomic.Int32
}

func (s *registrarSpy) Go(name string, run func(context.Context) error) error {
	s.n.Add(1)
	s.name = name
	s.run = run
	return s.err
}

func TestRegisterActorWithSupervisor(t *testing.T) {
	if err := registerActor(nil, act.New(nil)); err == nil {
		t.Fatalf("register-actor-with-supervisor rule violated: nil registrar err=%v", err)
	}
	spy := &registrarSpy{}
	if err := registerActor(spy, nil); err == nil {
		t.Fatalf("register-actor-with-supervisor rule violated: nil actor err=%v spyCalls=%d", err, spy.n.Load())
	}
	if spy.n.Load() != 0 {
		t.Fatalf("register-actor-with-supervisor rule violated: nil actor still called Go calls=%d", spy.n.Load())
	}

	wantErr := errors.New("register-actor-go-secret")
	spy = &registrarSpy{err: wantErr}
	actor := act.New(map[types.Runtime]act.Adapter{})
	got := registerActor(spy, actor)
	if got != wantErr {
		t.Fatalf("register-actor-with-supervisor rule violated: Go error not returned got=%v want=%v", got, wantErr)
	}
	if spy.n.Load() != 1 || spy.name != "actor" || spy.run == nil {
		t.Fatalf("register-actor-with-supervisor rule violated: Go spy name=%q calls=%d runNil=%t want name=actor calls=1", spy.name, spy.n.Load(), spy.run == nil)
	}

	var _ taskRegistrar = taskRegistrarFunc(nil)
	reg := taskRegistrarFunc(spy.Go)
	var iface taskRegistrar = reg
	if _, ok := iface.(interface{ Shutdown() []supervisor.Failure }); ok {
		t.Fatalf("register-actor-with-supervisor rule violated: taskRegistrarFunc exposes Shutdown type=%s", reflect.TypeOf(iface))
	}
	if _, ok := iface.(interface{ Context() context.Context }); ok {
		t.Fatalf("register-actor-with-supervisor rule violated: taskRegistrarFunc exposes Context type=%s", reflect.TypeOf(iface))
	}
	if _, ok := iface.(interface{ Failures() []supervisor.Failure }); ok {
		t.Fatalf("register-actor-with-supervisor rule violated: taskRegistrarFunc exposes Failures type=%s", reflect.TypeOf(iface))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sup := supervisor.New(ctx)
	live := act.New(map[types.Runtime]act.Adapter{})
	if err := registerActor(sup, live); err != nil {
		t.Fatalf("register-actor-with-supervisor rule violated: concrete Supervisor register err=%v", err)
	}
	sup.Shutdown()
}

func TestRuntimeTasksNoLeakAfterTwentyCycles(t *testing.T) {
	procRoot := t.TempDir()
	runCycle := func(t *testing.T, i int) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sup := supervisor.New(ctx)
		poller := inference.NewPoller(procRoot)
		poller.Discover = func(string) []inference.Server { return nil }
		poller.Interval = time.Hour
		eng := &snapshot.Engine{
			ProcRoot:  procRoot,
			Interval:  time.Hour,
			Inference: poller,
		}
		eng.Start(sup.Context())
		actor := act.New(map[types.Runtime]act.Adapter{})
		if err := registerActor(sup, actor); err != nil {
			t.Fatalf("runtime-tasks-no-leak-after-twenty-cycles rule violated: cycle=%d register err=%v", i, err)
		}
		sup.Shutdown()
		eng.Wait()
		poller.Wait()
	}

	runCycle(t, -1)
	for i := 0; i < 20; i++ {
		runCycle(t, i)
	}
	after := leftoverRuntimeStacks()
	if len(after) != 0 {
		t.Fatalf("runtime-tasks-no-leak-after-twenty-cycles rule violated: leftover after=20 count=%d stacks=%s", len(after), strings.Join(after, "\n---\n"))
	}
}

func leftoverRuntimeStacks() []string {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	chunks := strings.Split(string(buf[:n]), "\n\n")
	var out []string
	for _, chunk := range chunks {
		if strings.Contains(chunk, "_test.go") || strings.Contains(chunk, "testing.tRunner") {
			continue
		}
		if strings.Contains(chunk, "aitop/internal/supervisor") ||
			strings.Contains(chunk, "aitop/internal/snapshot") ||
			strings.Contains(chunk, "aitop/internal/overlay/inference") ||
			strings.Contains(chunk, "aitop/internal/act.(*Actor)") {
			out = append(out, chunk)
		}
	}
	return out
}
