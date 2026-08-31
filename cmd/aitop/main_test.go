package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"aitop/internal/act"
	"aitop/internal/graph"
	"aitop/internal/overlay/inference"
	"aitop/internal/snapshot"
	"aitop/internal/supervisor"
	"aitop/internal/theme"
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

const diagnosticSecret = "SECRET-\x01\x07-/tmp/aitop-secret.json-rm-rf-NOW"

type supervisorSpy struct {
	ctx           context.Context
	goes          []string
	goFns         []func(context.Context) error
	goErr         error
	shutdowns     atomic.Int32
	shutdownFails []supervisor.Failure
}

func (s *supervisorSpy) Context() context.Context { return s.ctx }

func (s *supervisorSpy) Go(name string, run func(context.Context) error) error {
	s.goes = append(s.goes, name)
	s.goFns = append(s.goFns, run)
	return s.goErr
}

func (s *supervisorSpy) Shutdown() []supervisor.Failure {
	s.shutdowns.Add(1)
	return append([]supervisor.Failure(nil), s.shutdownFails...)
}

type failWriter struct {
	calls atomic.Int32
	err   error
}

func (w *failWriter) Write(p []byte) (int, error) {
	w.calls.Add(1)
	return 0, w.err
}

type runSpies struct {
	now, capture, writeJSON, theme, render, newSup, newActor, interactive atomic.Int32
	writeJSONNow, renderNow                                               time.Time
	captureErr, writeJSONErr, interactiveErr                              error
	nilSup, nilActor                                                      bool
	sup                                                                   *supervisorSpy
	interactiveCtx                                                        context.Context
	interactiveReg                                                        taskRegistrar
	interactiveWriter                                                     io.Writer
	frozen                                                                time.Time
	snap                                                                  *snapshot.Snapshot
}

func testRunDeps(t *testing.T) (runDeps, *runSpies) {
	t.Helper()
	spies := &runSpies{
		frozen: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
		snap: &snapshot.Snapshot{
			At:   time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC),
			Rows: []types.Row{},
			Graph: &graph.Snapshot{
				At:    time.Date(2026, 8, 26, 12, 0, 0, 1, time.UTC),
				Nodes: []graph.Node{},
				Edges: []graph.Edge{},
				Gaps:  []graph.Gap{},
			},
		},
	}
	deps := runDeps{
		Now: func() time.Time {
			spies.now.Add(1)
			return spies.frozen
		},
		CaptureOnce: func(context.Context, runOptions) (*snapshot.Snapshot, error) {
			spies.capture.Add(1)
			return spies.snap, spies.captureErr
		},
		WriteJSON: func(_ *snapshot.Snapshot, w io.Writer, now time.Time) error {
			spies.writeJSON.Add(1)
			spies.writeJSONNow = now
			if spies.writeJSONErr != nil {
				return spies.writeJSONErr
			}
			_, err := io.WriteString(w, `{"schema":2}`+"\n")
			return err
		},
		ResolveTheme: func(string, string) theme.Theme {
			spies.theme.Add(1)
			return theme.Nightfable()
		},
		RenderScreenshot: func(*snapshot.Snapshot, theme.Theme, int, int, time.Time) string {
			spies.render.Add(1)
			return "FRAME"
		},
		NewSupervisor: func(ctx context.Context) runSupervisor {
			spies.newSup.Add(1)
			if spies.nilSup {
				return nil
			}
			if spies.sup == nil {
				spies.sup = &supervisorSpy{ctx: ctx}
			}
			return spies.sup
		},
		NewActor: func() *act.Actor {
			spies.newActor.Add(1)
			if spies.nilActor {
				return nil
			}
			return act.New(map[types.Runtime]act.Adapter{})
		},
		RunInteractive: func(ctx context.Context, _ runOptions, reg taskRegistrar, _ *act.Actor, w io.Writer) error {
			spies.interactive.Add(1)
			spies.interactiveCtx = ctx
			spies.interactiveReg = reg
			spies.interactiveWriter = w
			return spies.interactiveErr
		},
	}
	return deps, spies
}

func assertDiagnosticLine(t *testing.T, line, scope, task, class, secret string) {
	t.Helper()
	line = strings.TrimSuffix(line, "\n")
	want := fmt.Sprintf("aitop scope=%s task=%s class=%s", scope, task, class)
	if line != want {
		t.Fatalf("run-diagnostic-redaction rule violated: line=%q want=%q", line, want)
	}
	if len(line) > 256 {
		t.Fatalf("run-diagnostic-redaction rule violated: bytes=%d want<=256 line=%q", len(line), line)
	}
	for _, r := range line {
		if r > unicode.MaxASCII || unicode.IsControl(r) {
			t.Fatalf("run-diagnostic-redaction rule violated: non-ascii-or-control rune=%U line=%q", r, line)
		}
	}
	if secret != "" && strings.Contains(line, secret) {
		t.Fatalf("run-diagnostic-redaction rule violated: secret leaked line=%q", line)
	}
	if strings.Contains(strings.ToLower(line), "secret") {
		t.Fatalf("run-diagnostic-redaction rule violated: secret token leaked line=%q", line)
	}
}

func TestCaptureFailureEmitsNoSuccessfulDocument(t *testing.T) {
	deps, spies := testRunDeps(t)
	spies.captureErr = errors.New(diagnosticSecret)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if status == 0 || stdout.Len() != 0 {
		t.Fatalf("capture-failure-emits-no-successful-document rule violated: status=%d stdout=%q writeJSON=%d", status, stdout.Bytes(), spies.writeJSON.Load())
	}
	if spies.writeJSON.Load() != 0 {
		t.Fatalf("capture-failure-emits-no-successful-document rule violated: WriteJSON calls=%d", spies.writeJSON.Load())
	}
}

func TestWriterSinkFailureNoRetryAndNonzeroExit(t *testing.T) {
	deps, spies := testRunDeps(t)
	var writes atomic.Int32
	deps.WriteJSON = func(_ *snapshot.Snapshot, w io.Writer, _ time.Time) error {
		writes.Add(1)
		_, _ = w.Write([]byte("{"))
		return errors.New(diagnosticSecret)
	}
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if status == 0 || writes.Load() != 1 {
		t.Fatalf("writer-sink-failure-no-retry-and-nonzero-exit rule violated: status=%d writes=%d stdout=%q", status, writes.Load(), stdout.Bytes())
	}
	_ = spies
}

func TestRunRejectsMissingSelectedDependency(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		mut   func(*runDeps)
		scope string
		task  string
	}{
		{"json-now", []string{"--json"}, func(d *runDeps) { d.Now = nil }, "dependency", "json"},
		{"json-capture", []string{"--json"}, func(d *runDeps) { d.CaptureOnce = nil }, "dependency", "json"},
		{"json-write", []string{"--json"}, func(d *runDeps) { d.WriteJSON = nil }, "dependency", "json"},
		{"screenshot-now", []string{"--screenshot=120x40"}, func(d *runDeps) { d.Now = nil }, "dependency", "screenshot"},
		{"screenshot-capture", []string{"--screenshot=120x40"}, func(d *runDeps) { d.CaptureOnce = nil }, "dependency", "screenshot"},
		{"screenshot-theme", []string{"--screenshot=120x40"}, func(d *runDeps) { d.ResolveTheme = nil }, "dependency", "screenshot"},
		{"screenshot-render", []string{"--screenshot=120x40"}, func(d *runDeps) { d.RenderScreenshot = nil }, "dependency", "screenshot"},
		{"interactive-supervisor", nil, func(d *runDeps) { d.NewSupervisor = nil }, "dependency", "interactive"},
		{"interactive-actor", nil, func(d *runDeps) { d.NewActor = nil }, "dependency", "interactive"},
		{"interactive-run", nil, func(d *runDeps) { d.RunInteractive = nil }, "dependency", "interactive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps, spies := testRunDeps(t)
			tc.mut(&deps)
			var stdout, stderr bytes.Buffer
			status := run(context.Background(), tc.args, &stdout, &stderr, deps)
			if status == 0 {
				t.Fatalf("run-rejects-missing-selected-dependency rule violated: case=%s status=0 capture=%d", tc.name, spies.capture.Load())
			}
			line := strings.TrimSpace(stderr.String())
			assertDiagnosticLine(t, line, tc.scope, tc.task, "dependency", "")
		})
	}
}

func TestRunUsesInjectedClockAndBranchDependencies(t *testing.T) {
	deps, spies := testRunDeps(t)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if status != 0 || spies.now.Load() == 0 || spies.capture.Load() != 1 || spies.writeJSON.Load() != 1 {
		t.Fatalf("run-uses-injected-clock-and-branch-dependencies rule violated: status=%d now=%d capture=%d write=%d", status, spies.now.Load(), spies.capture.Load(), spies.writeJSON.Load())
	}
	if !spies.writeJSONNow.Equal(spies.frozen) {
		t.Fatalf("run-uses-injected-clock-and-branch-dependencies rule violated: writeNow=%s want=%s", spies.writeJSONNow, spies.frozen)
	}
	if spies.newSup.Load() != 0 || spies.newActor.Load() != 0 {
		t.Fatalf("run-uses-injected-clock-and-branch-dependencies rule violated: supervisor=%d actor=%d", spies.newSup.Load(), spies.newActor.Load())
	}

	deps, spies = testRunDeps(t)
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), []string{"--screenshot=80x24"}, &stdout, &stderr, deps)
	if status != 0 || spies.now.Load() == 0 || spies.capture.Load() != 1 || spies.theme.Load() != 1 || spies.render.Load() != 1 {
		t.Fatalf("run-uses-injected-clock-and-branch-dependencies rule violated: screenshot status=%d now=%d capture=%d theme=%d render=%d stderr=%q", status, spies.now.Load(), spies.capture.Load(), spies.theme.Load(), spies.render.Load(), stderr.String())
	}
}

func TestRunRegistersActorWithSupervisor(t *testing.T) {
	deps, spies := testRunDeps(t)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), nil, &stdout, &stderr, deps)
	if status != 0 {
		t.Fatalf("run-registers-actor-with-supervisor rule violated: status=%d stderr=%q", status, stderr.String())
	}
	if spies.sup == nil || len(spies.sup.goes) != 1 || spies.sup.goes[0] != "actor" || spies.sup.goFns[0] == nil {
		goes := []string{}
		if spies.sup != nil {
			goes = spies.sup.goes
		}
		t.Fatalf("run-registers-actor-with-supervisor rule violated: goes=%v want=[actor]", goes)
	}
}

func TestRunInteractiveUsesSupervisorContext(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	supCtx, stop := context.WithCancel(parent)
	defer stop()
	deps, spies := testRunDeps(t)
	spies.sup = &supervisorSpy{ctx: supCtx}
	var stdout, stderr bytes.Buffer
	status := run(parent, nil, &stdout, &stderr, deps)
	if status != 0 {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: status=%d stderr=%q", status, stderr.String())
	}
	if spies.interactiveCtx != supCtx {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: ctx=%v want supervisor context parent=%v", spies.interactiveCtx, parent)
	}
	if spies.interactiveWriter != &stdout {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: writer=%T want stdout-only extra=stderr", spies.interactiveWriter)
	}
	if spies.interactiveReg == nil {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: registrar=<nil>")
	}
	if err := spies.interactiveReg.Go("delegate", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: delegated Go err=%v", err)
	}
	if len(spies.sup.goes) < 2 || spies.sup.goes[len(spies.sup.goes)-1] != "delegate" {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: goes=%v want delegate via spy", spies.sup.goes)
	}
	if _, ok := spies.interactiveReg.(interface{ Shutdown() []supervisor.Failure }); ok {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: registrar exposes Shutdown type=%T", spies.interactiveReg)
	}
	if _, ok := spies.interactiveReg.(runSupervisor); ok {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: registrar implements runSupervisor type=%T", spies.interactiveReg)
	}
	if _, ok := spies.interactiveReg.(taskRegistrarFunc); !ok {
		t.Fatalf("run-interactive-uses-supervisor-context rule violated: dynamic type=%T want=taskRegistrarFunc", spies.interactiveReg)
	}
}

func TestRunInteractiveAlwaysShutsDownOnce(t *testing.T) {
	deps, spies := testRunDeps(t)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), nil, &stdout, &stderr, deps)
	if status != 0 || spies.sup == nil || spies.sup.shutdowns.Load() != 1 {
		n := int32(-1)
		if spies.sup != nil {
			n = spies.sup.shutdowns.Load()
		}
		t.Fatalf("run-interactive-always-shuts-down-once rule violated: status=%d shutdowns=%d", status, n)
	}

	t.Run("engine-wait-after-cancel", func(t *testing.T) {
		gctx := gateCancelContext{done: make(chan struct{})}
		var n atomic.Int32
		finished := make(chan struct{})
		reapEngineOnCancel(gctx, func() {
			n.Add(1)
			close(finished)
		})
		if n.Load() != 0 {
			t.Fatalf("run-interactive-always-shuts-down-once rule violated: engine wait invoked before cancel n=%d", n.Load())
		}
		close(gctx.done)
		<-finished
		if n.Load() != 1 {
			t.Fatalf("run-interactive-always-shuts-down-once rule violated: engine wait not invoked after cancel n=%d", n.Load())
		}
	})
}

type gateCancelContext struct {
	done chan struct{}
}

func (g gateCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (g gateCancelContext) Done() <-chan struct{}       { return g.done }
func (g gateCancelContext) Err() error {
	select {
	case <-g.done:
		return context.Canceled
	default:
		return nil
	}
}
func (g gateCancelContext) Value(any) any { return nil }

func TestRunInteractiveRejectsNilSupervisorOrActor(t *testing.T) {
	deps, spies := testRunDeps(t)
	spies.nilSup = true
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-interactive-rejects-nil-supervisor-or-actor rule violated: nil supervisor status=0")
	}
	if spies.sup != nil && spies.sup.shutdowns.Load() != 0 {
		t.Fatalf("run-interactive-rejects-nil-supervisor-or-actor rule violated: nil supervisor shutdowns=%d", spies.sup.shutdowns.Load())
	}

	deps, spies = testRunDeps(t)
	spies.nilActor = true
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 || spies.sup == nil || spies.sup.shutdowns.Load() != 1 {
		n := int32(-1)
		if spies.sup != nil {
			n = spies.sup.shutdowns.Load()
		}
		t.Fatalf("run-interactive-rejects-nil-supervisor-or-actor rule violated: nil actor status=%d shutdowns=%d", status, n)
	}
}

func TestRunInteractiveGoFailureReturnsNonzeroAndShutsDown(t *testing.T) {
	deps, spies := testRunDeps(t)
	spies.sup = &supervisorSpy{ctx: context.Background(), goErr: errors.New(diagnosticSecret)}
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 || spies.sup.shutdowns.Load() != 1 || spies.interactive.Load() != 0 {
		t.Fatalf("run-interactive-go-failure-returns-nonzero-and-shuts-down rule violated: status=%d shutdowns=%d interactive=%d", status, spies.sup.shutdowns.Load(), spies.interactive.Load())
	}
}

func TestRunInteractiveErrorReturnsNonzeroAndShutsDown(t *testing.T) {
	deps, spies := testRunDeps(t)
	spies.interactiveErr = errors.New(diagnosticSecret)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 || spies.sup == nil || spies.sup.shutdowns.Load() != 1 {
		n := int32(-1)
		if spies.sup != nil {
			n = spies.sup.shutdowns.Load()
		}
		t.Fatalf("run-interactive-error-returns-nonzero-and-shuts-down rule violated: status=%d shutdowns=%d", status, n)
	}
}

func TestRunInteractiveShutdownFailuresReturnNonzero(t *testing.T) {
	deps, spies := testRunDeps(t)
	spies.sup = &supervisorSpy{ctx: context.Background(), shutdownFails: []supervisor.Failure{{Name: "actor", Err: errors.New("first-secret")}, {Name: "other", Err: errors.New("second-secret")}}}
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-interactive-shutdown-failures-return-nonzero rule violated: status=0 lines=%q", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("run-interactive-shutdown-failures-return-nonzero rule violated: lines=%v want=2 shutdown diagnostics", lines)
	}

	deps, spies = testRunDeps(t)
	spies.interactiveErr = errors.New("interactive-secret")
	spies.sup = &supervisorSpy{ctx: context.Background(), shutdownFails: []supervisor.Failure{{Name: "actor", Err: errors.New("shutdown-secret")}}}
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), nil, &stdout, &stderr, deps)
	lines = strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if status == 0 || len(lines) != 2 {
		t.Fatalf("run-interactive-shutdown-failures-return-nonzero rule violated: dual status=%d lines=%v", status, lines)
	}
}

func TestSafeDiagnosticClassificationPrecedence(t *testing.T) {
	valid := [][2]string{
		{"usage", "flags"},
		{"dependency", "json"},
		{"dependency", "screenshot"},
		{"dependency", "interactive"},
		{"capture", "json"},
		{"capture", "screenshot"},
		{"json", "write"},
		{"screenshot", "write"},
		{"register", "actor"},
		{"interactive", "run"},
		{"shutdown", "supervisor"},
	}
	fallback := map[[2]string]string{
		{"usage", "flags"}:            "usage",
		{"dependency", "json"}:        "dependency",
		{"dependency", "screenshot"}:  "dependency",
		{"dependency", "interactive"}: "dependency",
		{"capture", "json"}:           "capture",
		{"capture", "screenshot"}:     "capture",
		{"json", "write"}:             "json",
		{"screenshot", "write"}:       "screenshot",
		{"register", "actor"}:         "register",
		{"interactive", "run"}:        "interactive",
		{"shutdown", "supervisor"}:    "shutdown",
	}
	if got := safeDiagnostic("usage", "flags", nil); got != "" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: nil valid pair got=%q", got)
	}
	canceled := fmt.Errorf("wrap: %w", context.Canceled)
	deadline := fmt.Errorf("wrap: %w", context.DeadlineExceeded)
	short := fmt.Errorf("wrap: %w", io.ErrShortWrite)
	joinedAll := errors.Join(context.Canceled, context.DeadlineExceeded, io.ErrShortWrite)
	joinedDeadlineShort := errors.Join(context.DeadlineExceeded, io.ErrShortWrite)
	arbitrary := errors.New("arbitrary-secret")
	for _, pair := range valid {
		scope, task := pair[0], pair[1]
		if got := safeDiagnostic(scope, task, canceled); got != fmt.Sprintf("aitop scope=%s task=%s class=canceled", scope, task) {
			t.Fatalf("safe-diagnostic-classification-precedence rule violated: pair=%s/%s canceled got=%q", scope, task, got)
		}
		if got := safeDiagnostic(scope, task, deadline); got != fmt.Sprintf("aitop scope=%s task=%s class=deadline", scope, task) {
			t.Fatalf("safe-diagnostic-classification-precedence rule violated: pair=%s/%s deadline got=%q", scope, task, got)
		}
		if got := safeDiagnostic(scope, task, short); got != fmt.Sprintf("aitop scope=%s task=%s class=io", scope, task) {
			t.Fatalf("safe-diagnostic-classification-precedence rule violated: pair=%s/%s short-write got=%q", scope, task, got)
		}
		if got := safeDiagnostic(scope, task, joinedAll); got != fmt.Sprintf("aitop scope=%s task=%s class=canceled", scope, task) {
			t.Fatalf("safe-diagnostic-classification-precedence rule violated: pair=%s/%s joined-all got=%q", scope, task, got)
		}
		if got := safeDiagnostic(scope, task, joinedDeadlineShort); got != fmt.Sprintf("aitop scope=%s task=%s class=deadline", scope, task) {
			t.Fatalf("safe-diagnostic-classification-precedence rule violated: pair=%s/%s joined-deadline-short got=%q", scope, task, got)
		}
		want := fallback[pair]
		if got := safeDiagnostic(scope, task, arbitrary); got != fmt.Sprintf("aitop scope=%s task=%s class=%s", scope, task, want) {
			t.Fatalf("safe-diagnostic-classification-precedence rule violated: pair=%s/%s arbitrary got=%q want class=%s", scope, task, got, want)
		}
	}
	if got := safeDiagnostic("shutdown", "supervisor", arbitrary); got != "aitop scope=shutdown task=supervisor class=shutdown" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: supervisor fallback got=%q", got)
	}
	if got := safeDiagnostic("nope", "also", nil); got != "" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: nil invalid pair got=%q", got)
	}
	if got := safeDiagnostic("nope", "also", canceled); got != "aitop scope=internal task=failure class=canceled" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: invalid canceled got=%q", got)
	}
	if got := safeDiagnostic("nope", "also", deadline); got != "aitop scope=internal task=failure class=deadline" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: invalid deadline got=%q", got)
	}
	if got := safeDiagnostic("nope", "also", short); got != "aitop scope=internal task=failure class=io" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: invalid io got=%q", got)
	}
	if got := safeDiagnostic("nope", "also", joinedAll); got != "aitop scope=internal task=failure class=canceled" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: invalid joined-all got=%q", got)
	}
	if got := safeDiagnostic("nope", "also", joinedDeadlineShort); got != "aitop scope=internal task=failure class=deadline" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: invalid joined-deadline-short got=%q", got)
	}
	if got := safeDiagnostic("nope", "also", arbitrary); got != "aitop scope=internal task=failure class=failure" {
		t.Fatalf("safe-diagnostic-classification-precedence rule violated: invalid arbitrary got=%q", got)
	}
}

func TestRunFlagAndDependencyDiagnosticsAreRedacted(t *testing.T) {
	deps, _ := testRunDeps(t)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), []string{"--not-a-flag", diagnosticSecret, "--theme", diagnosticSecret}, &stdout, &stderr, deps)
	if status != 2 {
		t.Fatalf("run-flag-and-dependency-diagnostics-are-redacted rule violated: parser status=%d stderr=%q", status, stderr.String())
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "usage", "flags", "usage", diagnosticSecret)
	if strings.Contains(stderr.String(), diagnosticSecret) || strings.Contains(stderr.String(), "--not-a-flag") {
		t.Fatalf("run-flag-and-dependency-diagnostics-are-redacted rule violated: parser leaked stderr=%q", stderr.String())
	}

	deps, _ = testRunDeps(t)
	deps.WriteJSON = nil
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), []string{"--json", "--prices", diagnosticSecret}, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-flag-and-dependency-diagnostics-are-redacted rule violated: missing write status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "dependency", "json", "dependency", diagnosticSecret)

	deps, spies := testRunDeps(t)
	spies.nilSup = true
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-flag-and-dependency-diagnostics-are-redacted rule violated: nil supervisor status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "dependency", "interactive", "dependency", diagnosticSecret)

	deps, spies = testRunDeps(t)
	spies.nilActor = true
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-flag-and-dependency-diagnostics-are-redacted rule violated: nil actor status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "dependency", "interactive", "dependency", diagnosticSecret)

	deps, _ = testRunDeps(t)
	deps.WriteJSON = nil
	fw := &failWriter{err: errors.New(diagnosticSecret)}
	status = run(context.Background(), []string{"--json"}, io.Discard, fw, deps)
	if status == 0 || fw.calls.Load() != 1 {
		t.Fatalf("run-flag-and-dependency-diagnostics-are-redacted rule violated: stderr fail status=%d writes=%d", status, fw.calls.Load())
	}
}

func TestRunCaptureAndJSONDiagnosticsAreRedacted(t *testing.T) {
	deps, spies := testRunDeps(t)
	spies.captureErr = errors.New(diagnosticSecret)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-capture-and-json-diagnostics-are-redacted rule violated: capture status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "capture", "json", "capture", diagnosticSecret)

	deps, spies = testRunDeps(t)
	spies.writeJSONErr = errors.New(diagnosticSecret)
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-capture-and-json-diagnostics-are-redacted rule violated: json write status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "json", "write", "json", diagnosticSecret)
}

func TestRunScreenshotWriterDiagnosticsAreRedacted(t *testing.T) {
	deps, _ := testRunDeps(t)
	fw := &failWriter{err: errors.New(diagnosticSecret)}
	var stderr bytes.Buffer
	status := run(context.Background(), []string{"--screenshot=80x24"}, fw, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-screenshot-writer-diagnostics-are-redacted rule violated: status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "screenshot", "write", "screenshot", diagnosticSecret)
	if fw.calls.Load() != 1 {
		t.Fatalf("run-screenshot-writer-diagnostics-are-redacted rule violated: stdout writes=%d", fw.calls.Load())
	}
}

func TestRunRegisterInteractiveShutdownDiagnosticsAreRedacted(t *testing.T) {
	deps, spies := testRunDeps(t)
	spies.sup = &supervisorSpy{ctx: context.Background(), goErr: errors.New(diagnosticSecret)}
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-register-interactive-shutdown-diagnostics-are-redacted rule violated: register status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "register", "actor", "register", diagnosticSecret)

	deps, spies = testRunDeps(t)
	spies.interactiveErr = errors.New(diagnosticSecret)
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), nil, &stdout, &stderr, deps)
	if status == 0 {
		t.Fatalf("run-register-interactive-shutdown-diagnostics-are-redacted rule violated: interactive status=0")
	}
	assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "interactive", "run", "interactive", diagnosticSecret)

	deps, spies = testRunDeps(t)
	spies.sup = &supervisorSpy{ctx: context.Background(), shutdownFails: []supervisor.Failure{
		{Name: "actor", Err: errors.New(diagnosticSecret + "-one")},
		{Name: "other", Err: errors.New(diagnosticSecret + "-two")},
	}}
	stdout.Reset()
	stderr.Reset()
	status = run(context.Background(), nil, &stdout, &stderr, deps)
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if status == 0 || len(lines) != 2 {
		t.Fatalf("run-register-interactive-shutdown-diagnostics-are-redacted rule violated: shutdown status=%d lines=%v", status, lines)
	}
	for _, line := range lines {
		assertDiagnosticLine(t, line, "shutdown", "supervisor", "shutdown", diagnosticSecret)
	}
}

func TestRunOnceAliasesJSON(t *testing.T) {
	for _, args := range [][]string{{"--once"}, {"--json", "--once"}} {
		deps, spies := testRunDeps(t)
		var stdout, stderr bytes.Buffer
		status := run(context.Background(), args, &stdout, &stderr, deps)
		if status != 0 || spies.capture.Load() != 1 || spies.writeJSON.Load() != 1 || spies.interactive.Load() != 0 {
			t.Fatalf("run-once-aliases-json rule violated: args=%v status=%d capture=%d write=%d interactive=%d", args, status, spies.capture.Load(), spies.writeJSON.Load(), spies.interactive.Load())
		}
	}
}

func TestRunRejectsScreenshotWithJSONOrOnce(t *testing.T) {
	cases := [][]string{
		{"--screenshot="},
		{"--json", "--screenshot="},
		{"--once", "--screenshot="},
		{"--screenshot=", "--json"},
		{"--screenshot=", "--once"},
		{"--screenshot=120x40", "--json"},
		{"--screenshot=120x40", "--once"},
		{"--json", "--screenshot=120x40"},
		{"--once", "--screenshot=120x40"},
	}
	for _, args := range cases {
		deps, spies := testRunDeps(t)
		var stdout, stderr bytes.Buffer
		status := run(context.Background(), args, &stdout, &stderr, deps)
		if status != 2 {
			t.Fatalf("run-rejects-screenshot-with-json-or-once rule violated: args=%v status=%d stderr=%q", args, status, stderr.String())
		}
		if spies.capture.Load() != 0 || spies.writeJSON.Load() != 0 || spies.newSup.Load() != 0 || spies.newActor.Load() != 0 || spies.theme.Load() != 0 || spies.render.Load() != 0 {
			t.Fatalf("run-rejects-screenshot-with-json-or-once rule violated: args=%v capture=%d write=%d sup=%d actor=%d theme=%d render=%d", args, spies.capture.Load(), spies.writeJSON.Load(), spies.newSup.Load(), spies.newActor.Load(), spies.theme.Load(), spies.render.Load())
		}
		assertDiagnosticLine(t, strings.TrimSpace(stderr.String()), "usage", "flags", "usage", "")
	}
}

func TestJSONDoesNotStartActor(t *testing.T) {
	deps, spies := testRunDeps(t)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), []string{"--json"}, &stdout, &stderr, deps)
	if status != 0 || spies.newActor.Load() != 0 || spies.newSup.Load() != 0 || spies.interactive.Load() != 0 {
		t.Fatalf("json-does-not-start-actor rule violated: status=%d actor=%d sup=%d interactive=%d", status, spies.newActor.Load(), spies.newSup.Load(), spies.interactive.Load())
	}
}

func TestScreenshotDoesNotStartActor(t *testing.T) {
	deps, spies := testRunDeps(t)
	var stdout, stderr bytes.Buffer
	status := run(context.Background(), []string{"--screenshot=80x24"}, &stdout, &stderr, deps)
	if status != 0 || spies.newActor.Load() != 0 || spies.newSup.Load() != 0 || spies.interactive.Load() != 0 {
		t.Fatalf("screenshot-does-not-start-actor rule violated: status=%d actor=%d sup=%d interactive=%d stderr=%q", status, spies.newActor.Load(), spies.newSup.Load(), spies.interactive.Load(), stderr.String())
	}
}

func TestProductionCaptureOncePublishesGraph(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	snap, err := productionCaptureOnce(ctx, runOptions{Interval: time.Millisecond, NoPrices: true})
	if err != nil {
		t.Fatalf("production-capture-once-publishes-graph rule violated: err=%v", err)
	}
	if snap == nil || snap.Graph == nil {
		t.Fatalf("production-capture-once-publishes-graph rule violated: snap=%v graph=%v", snap, snap)
	}
	if snap.Graph.Nodes == nil || snap.Graph.Edges == nil || snap.Graph.Gaps == nil {
		t.Fatalf("production-capture-once-publishes-graph rule violated: nodesNil=%t edgesNil=%t gapsNil=%t", snap.Graph.Nodes == nil, snap.Graph.Edges == nil, snap.Graph.Gaps == nil)
	}
	if occupancyMappableRows(snap.Rows) > 0 && len(snap.Graph.Nodes) == 0 {
		t.Fatalf("production-capture-once-publishes-graph rule violated: mappableRows=%d graphNodes=0", occupancyMappableRows(snap.Rows))
	}
	var buf bytes.Buffer
	if err := snapshot.WriteJSON(snap, &buf, snap.At); err != nil {
		states := map[string]int{}
		for _, n := range snap.Graph.Nodes {
			states[string(n.State.Value)]++
		}
		t.Fatalf("production-capture-once-write-json rule violated: err=%v nodes=%d rows=%d states=%v", err, len(snap.Graph.Nodes), len(snap.Rows), states)
	}
}

func TestProductionRunInteractiveRegistersGraph(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reg := &registrarSpy{}
	actor := act.New(nil)
	done := make(chan error, 1)
	go func() {
		done <- productionRunInteractive(ctx, runOptions{NoPrices: true, Interval: time.Hour}, reg, actor, io.Discard)
	}()
	select {
	case err := <-done:
		if reg.n.Load() != 1 || reg.name != "graph" || reg.run == nil {
			t.Fatalf("production-run-interactive-registers-graph rule violated: n=%d name=%s runNil=%t err=%v", reg.n.Load(), reg.name, reg.run == nil, err)
		}
	case <-time.After(8 * time.Second):
		t.Fatalf("production-run-interactive-registers-graph rule violated: tea did not return on canceled context")
	}
}

func occupancyMappableRows(rows []types.Row) int {
	n := 0
	for _, ev := range graph.OccupancyEventsFromRows(rows, time.Unix(1, 0).UTC()) {
		if ev.Kind == graph.EventNodeObserved {
			n++
		}
	}
	return n
}
