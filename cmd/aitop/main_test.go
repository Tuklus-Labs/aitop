package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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
	renderGraph                                                           atomic.Int32
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
		RenderGraphScreenshot: func(*snapshot.Snapshot, theme.Theme, int, int, time.Time) string {
			spies.renderGraph.Add(1)
			return "GRAPHFRAME"
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
		{"screenshot-graph-now", []string{"--screenshot-graph=120x40"}, func(d *runDeps) { d.Now = nil }, "dependency", "screenshot"},
		{"screenshot-graph-capture", []string{"--screenshot-graph=120x40"}, func(d *runDeps) { d.CaptureOnce = nil }, "dependency", "screenshot"},
		{"screenshot-graph-theme", []string{"--screenshot-graph=120x40"}, func(d *runDeps) { d.ResolveTheme = nil }, "dependency", "screenshot"},
		{"screenshot-graph-render", []string{"--screenshot-graph=120x40"}, func(d *runDeps) { d.RenderGraphScreenshot = nil }, "dependency", "screenshot"},
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
		{"--screenshot-graph="},
		{"--json", "--screenshot-graph="},
		{"--once", "--screenshot-graph="},
		{"--screenshot-graph=", "--json"},
		{"--screenshot-graph=", "--once"},
		{"--screenshot-graph=120x40", "--json"},
		{"--screenshot-graph=120x40", "--once"},
		{"--json", "--screenshot-graph=120x40"},
		{"--once", "--screenshot-graph=120x40"},
		// Two frames, one stdout, no ordering worth defining.
		{"--screenshot=120x40", "--screenshot-graph=120x40"},
		{"--screenshot-graph=120x40", "--screenshot=120x40"},
	}
	for _, args := range cases {
		deps, spies := testRunDeps(t)
		var stdout, stderr bytes.Buffer
		status := run(context.Background(), args, &stdout, &stderr, deps)
		if status != 2 {
			t.Fatalf("run-rejects-screenshot-with-json-or-once rule violated: args=%v status=%d stderr=%q", args, status, stderr.String())
		}
		if spies.capture.Load() != 0 || spies.writeJSON.Load() != 0 || spies.newSup.Load() != 0 || spies.newActor.Load() != 0 || spies.theme.Load() != 0 || spies.render.Load() != 0 || spies.renderGraph.Load() != 0 {
			t.Fatalf("run-rejects-screenshot-with-json-or-once rule violated: args=%v capture=%d write=%d sup=%d actor=%d theme=%d render=%d renderGraph=%d", args, spies.capture.Load(), spies.writeJSON.Load(), spies.newSup.Load(), spies.newActor.Load(), spies.theme.Load(), spies.render.Load(), spies.renderGraph.Load())
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

// TestMain pins the colour profile, for the same reason internal/ui does.
// Under `go test` there is no terminal, so lipgloss detects Ascii and renders
// every style as the bare word: dim text and bright text come out
// byte-identical and any assertion about colour is vacuously true. This
// package's screenshot tests render real frames through the production
// RenderScreenshot dep, which sets TrueColor for the process when it runs --
// so without this pin the answer depends on whether a screenshot test happened
// to run first, which is test-order-dependent colour and worse than either
// profile chosen deliberately.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
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

// spyAttach replaces the production attach for one test and records what it was
// handed. It returns a real shadow so the caller's own lifecycle still runs.
func spyAttach(t *testing.T, fail error) *attachSpy {
	t.Helper()
	spy := &attachSpy{}
	prior := attachGraph
	attachGraph = func(eng *snapshot.Engine, homes snapshot.GraphHomes) (*graph.Shadow, *graph.OccupancyCollector, snapshot.NativeLanes, error) {
		spy.calls++
		spy.homes = homes
		spy.engine = eng
		if fail != nil {
			return nil, nil, nil, fail
		}
		return prior(eng, homes)
	}
	t.Cleanup(func() { attachGraph = prior })
	return spy
}

type attachSpy struct {
	calls  int
	homes  snapshot.GraphHomes
	engine *snapshot.Engine
}

// check asserts the spy saw exactly one attach carrying the engine's own homes.
func (spy *attachSpy) check(t *testing.T, path string) {
	t.Helper()
	if spy.calls != 1 {
		t.Fatalf("production-attaches-the-graph-once rule violated: path=%s calls=%d", path, spy.calls)
	}
	eng := spy.engine
	if eng == nil {
		t.Fatalf("production-attaches-the-running-engine rule violated: path=%s engine=nil", path)
	}
	// Non-empty first: a path that passed GraphHomes{} against an engine whose
	// homes were also empty would satisfy an equality check while registering
	// no native collector at all.
	if eng.ClaudeHome == "" || eng.CodexHome == "" || eng.GrokHome == "" {
		t.Fatalf("production-engine-has-homes rule violated: path=%s claude=%q codex=%q grok=%q", path, eng.ClaudeHome, eng.CodexHome, eng.GrokHome)
	}
	want := snapshot.GraphHomes{Claude: eng.ClaudeHome, Codex: eng.CodexHome, Grok: eng.GrokHome}
	if spy.homes != want {
		t.Fatalf("production-passes-engine-homes-to-the-graph rule violated: path=%s got=%+v want=%+v", path, spy.homes, want)
	}
}

// TestProductionCaptureOncePassesEngineHomes pins the one-shot wiring. The
// native collectors read a runtime's home directory, and productionEngine is
// the only thing that knows where those are; a capture that attached the graph
// without them would produce schema-2 JSON whose graph is occupancy-only, which
// looks exactly like a box with nothing running on it.
func TestProductionCaptureOncePassesEngineHomes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	spy := spyAttach(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := productionCaptureOnce(ctx, runOptions{NoPrices: true}); err != nil {
		t.Fatalf("production-capture-once-succeeds rule violated: err=%v", err)
	}
	spy.check(t, "captureOnce")
	// The homes must be the ones under the HOME this run actually had, not a
	// path baked in at build time.
	if !strings.HasPrefix(spy.homes.Claude, home) {
		t.Fatalf("production-homes-follow-the-environment rule violated: claude=%q home=%q", spy.homes.Claude, home)
	}
}

// TestProductionRunInteractivePassesEngineHomes covers the second attach site.
// Two call sites and one lapse is the shape that leaves the TUI running an
// occupancy-only graph while the one-shot path looks correct, so the interactive
// path is asserted directly rather than assumed to match. The attach is made to
// fail so the run returns before any terminal is touched.
func TestProductionRunInteractivePassesEngineHomes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sentinel := errors.New("attach refused")
	spy := spyAttach(t, sentinel)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := productionRunInteractive(ctx, runOptions{NoPrices: true}, nil, nil, io.Discard)
	if !errors.Is(err, sentinel) {
		t.Fatalf("production-run-interactive-returns-attach-errors rule violated: err=%v", err)
	}
	spy.check(t, "runInteractive")
}

// rowsAtRunProbe is a collector that records whether the engine had rows the
// first time the shadow ran it. It claims no capability of its own beyond
// identity, publishes nothing, and blocks until the context ends, so it changes
// what the graph contains not at all.
type rowsAtRunProbe struct {
	eng       *snapshot.Engine
	ran       atomic.Bool
	sawRows   atomic.Bool
	rowsAtRun atomic.Int64
}

func (p *rowsAtRunProbe) Descriptor() graph.CollectorDescriptor {
	return graph.CollectorDescriptor{
		ID:           graph.SourceID("aitop:test:rows-at-run"),
		Runtime:      types.RuntimeLocal,
		Schemas:      []graph.InputSchema{{Name: "aitop-test", Version: 1}},
		Capabilities: []graph.Capability{graph.CapabilityIdentity},
	}
}

func (p *rowsAtRunProbe) Run(ctx context.Context, _ graph.EventSink) error {
	p.ran.Store(true)
	p.rowsAtRun.Store(int64(len(p.eng.Rows())))
	p.sawRows.Store(p.eng.Snapshot() != nil && len(p.eng.Rows()) > 0)
	<-ctx.Done()
	return ctx.Err()
}

// TestProductionCaptureOnceFillsRowsBeforeRunningCollectors pins the ordering
// the incarnation rule depends on. Every collector's first tick reads the
// engine's rows to decide each node's incarnation: native dates a session by
// its process when the rows bind one and by its invocation when they do not,
// and occupancy always dates it by its process. Run the collectors against an
// empty engine and the two disagree, occupancy's events are rejected on
// identity, and the one-shot emits a graph whose occupancy source is Partial --
// with no next tick to converge on, unlike the interactive path.
//
// The probe answers the question directly rather than by looking for the
// symptom: a gap count depends on what happens to be running on the box, and
// would read clean on a machine with no live claude session at all.
//
// The engine's overlays are injected rather than collected, which is what makes
// the test portable. join.Join emits a row only for a CLASSIFIED agent process,
// and the go test binary matches no classifier entry, so on a box with nothing
// agent-shaped running the engine legitimately holds zero rows and the ordering
// question has no content. An earlier version asserted a bare "the engine had
// rows" against real /proc and passed here only because a live claude session
// happened to exist; anywhere else it would have failed and sent the reader
// hunting an ordering bug that was not there. A dark local unit is the one
// overlay shape join turns into a row with no process behind it, so it gives
// the assertion something to be about on every machine.
func TestProductionCaptureOnceFillsRowsBeforeRunningCollectors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var probe *rowsAtRunProbe
	prior := attachGraph
	attachGraph = func(eng *snapshot.Engine, _ snapshot.GraphHomes) (*graph.Shadow, *graph.OccupancyCollector, snapshot.NativeLanes, error) {
		eng.Overlay = func() ([]types.Overlay, error) {
			return []types.Overlay{{
				Runtime:     types.RuntimeLocal,
				SessionName: "aitop-test-dark-unit",
				Status:      "off",
			}}, nil
		}
		probe = &rowsAtRunProbe{eng: eng}
		occ := graph.NewOccupancyCollector(func() []types.Row { return eng.Rows() })
		shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), occ, probe)
		if err != nil {
			return nil, nil, nil, err
		}
		eng.GraphSnapshot = shadow.Snapshot
		return shadow, occ, nil, nil
	}
	t.Cleanup(func() { attachGraph = prior })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := productionCaptureOnce(ctx, runOptions{NoPrices: true}); err != nil {
		t.Fatalf("production-capture-once-succeeds rule violated: err=%v", err)
	}
	if probe == nil || !probe.ran.Load() {
		t.Fatalf("capture-once-runs-the-collectors rule violated: probe=%v", probe)
	}
	// The canary, and it must be checked FIRST. If the injected unit stops
	// producing a row, the ordering assertion below becomes vacuous and would
	// go on passing forever without ever comparing anything.
	rowsAfter := len(probe.eng.Rows())
	if rowsAfter == 0 {
		t.Fatalf("capture-once-fixture-produces-a-row rule violated: the injected dark local unit yielded no row, so the ordering rule below has nothing to discriminate")
	}
	if !probe.sawRows.Load() {
		t.Fatalf("capture-once-fills-rows-before-running-collectors rule violated: the collectors ran against an engine holding %d rows, and the capture that followed them produced %d", probe.rowsAtRun.Load(), rowsAfter)
	}
}

// slowNativeLane is a stand-in for a native collector whose first tick is a
// slow disk walk. It publishes one node and only then signals readiness, which
// is the ordering a real lane has and the ordering the one-shot depends on.
type slowNativeLane struct {
	delay   time.Duration
	session string
	ready   chan struct{}
	once    sync.Once
}

func newSlowNativeLane(delay time.Duration, session string) *slowNativeLane {
	return &slowNativeLane{delay: delay, session: session, ready: make(chan struct{})}
}

func (l *slowNativeLane) FirstTick() <-chan struct{} { return l.ready }

// FirstTickNodes reports what Run published, so the one-shot waits for this
// lane's own node instead of for a stretch of quiet.
func (l *slowNativeLane) FirstTickNodes() []graph.NodeID {
	id, err := graph.ClaudeSessionID(l.session)
	if err != nil {
		return nil
	}
	return []graph.NodeID{id}
}

func (l *slowNativeLane) Descriptor() graph.CollectorDescriptor {
	return graph.CollectorDescriptor{
		ID:           graph.SourceID("aitop:test:slow-native-lane"),
		Runtime:      types.RuntimeClaude,
		Schemas:      []graph.InputSchema{{Name: "aitop-test", Version: 1}},
		Capabilities: []graph.Capability{graph.CapabilityIdentity},
	}
}

func (l *slowNativeLane) Run(ctx context.Context, sink graph.EventSink) error {
	select {
	case <-time.After(l.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	now := time.Now()
	actor, err := graph.ClaudeSessionID(l.session)
	if err != nil {
		return err
	}
	inc, err := graph.InvocationIncarnation(types.RuntimeClaude, l.session)
	if err != nil {
		return err
	}
	_, _ = sink.Publish(graph.Event{
		Schema: 1,
		Source: graph.EventSource{
			Ref: graph.SourceRef{
				ID:          graph.SourceID("aitop:test:slow-native-lane"),
				Runtime:     types.RuntimeClaude,
				Incarnation: graph.SourceIncarnationID(1),
				Authority:   graph.AuthorityNative,
			},
			Mode: graph.SourceImmutable,
		},
		ID:               graph.ImmutableEventID(types.RuntimeClaude, "slow-native-lane-record", "/aitop/test/slow-native-lane"),
		ReceivedAt:       now,
		Kind:             graph.EventNodeObserved,
		Actor:            actor,
		ActorIncarnation: inc,
		Data:             graph.NodeObserved{Runtime: types.RuntimeClaude, Role: types.RolePrimary},
	})
	l.once.Do(func() { close(l.ready) })
	<-ctx.Done()
	return ctx.Err()
}

// attachWithSlowLane replaces the production attach with one that registers a
// single slow native lane beside the real occupancy collector.
func attachWithSlowLane(t *testing.T, lane *slowNativeLane) {
	t.Helper()
	prior := attachGraph
	attachGraph = func(eng *snapshot.Engine, _ snapshot.GraphHomes) (*graph.Shadow, *graph.OccupancyCollector, snapshot.NativeLanes, error) {
		occ := graph.NewOccupancyCollector(func() []types.Row { return eng.Rows() })
		shadow, err := graph.NewShadow(graph.DefaultReconcileConfig(), graph.DefaultStoreConfig(), occ, lane)
		if err != nil {
			return nil, nil, nil, err
		}
		eng.GraphSnapshot = shadow.Snapshot
		return shadow, occ, snapshot.NativeLanes{lane}, nil
	}
	t.Cleanup(func() { attachGraph = prior })
}

func graphHasNode(snap *snapshot.Snapshot, id graph.NodeID) bool {
	if snap == nil || snap.Graph == nil {
		return false
	}
	for _, node := range snap.Graph.Nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

// TestProductionCaptureOnceWaitsForANativeLaneUnderTheCap is the readiness
// half. The occupancy wait is satisfied by any node and occupancy publishes one
// immediately, so before this wait existed a one-shot could sample the graph
// while a native lane was still walking a session store and emit an
// occupancy-only graph with gaps=0 and nothing to say anything was missed. A
// thin graph and a quiet box looked identical.
func TestProductionCaptureOnceWaitsForANativeLaneUnderTheCap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const session = "84a06b9a-0873-4655-9eb9-d7cf99554b05"
	// Well past the 500ms occupancy wait, well under the 2s native cap: the
	// window where the old code was wrong and the new code has to be right.
	lane := newSlowNativeLane(900*time.Millisecond, session)
	attachWithSlowLane(t, lane)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snap, err := productionCaptureOnce(ctx, runOptions{NoPrices: true})
	if err != nil {
		t.Fatalf("production-capture-once-succeeds rule violated: err=%v", err)
	}
	id, err := graph.ClaudeSessionID(session)
	if err != nil {
		t.Fatalf("capture-once-test-fixture-session-id rule violated: err=%v", err)
	}
	if !graphHasNode(snap, id) {
		nodes := 0
		if snap != nil && snap.Graph != nil {
			nodes = len(snap.Graph.Nodes)
		}
		t.Fatalf("capture-once-waits-for-a-native-lane-under-the-cap rule violated: %s absent from a graph of %d nodes after a %s lane", id, nodes, lane.delay)
	}
}

// TestProductionCaptureOnceReturnsAtTheCapForASlowerLane is the bounded half.
// A pathological store must cost a late graph, never a hung command, so the
// one-shot gives up and emits what it has rather than waiting on a lane that
// may never come in.
func TestProductionCaptureOnceReturnsAtTheCapForASlowerLane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const session = "8ad6c366-70de-429c-98bf-a1c350d6ac31"
	lane := newSlowNativeLane(time.Hour, session)
	attachWithSlowLane(t, lane)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	started := time.Now()
	snap, err := productionCaptureOnce(ctx, runOptions{NoPrices: true})
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("production-capture-once-succeeds rule violated: err=%v", err)
	}
	id, err := graph.ClaudeSessionID(session)
	if err != nil {
		t.Fatalf("capture-once-test-fixture-session-id rule violated: err=%v", err)
	}
	if graphHasNode(snap, id) {
		t.Fatalf("capture-once-test-lane-is-slower-than-the-cap rule violated: %s present, so the fixture never exercised the bound", id)
	}
	// Bounded, not blocking. The budget plus the occupancy wait plus the drain
	// is the whole cost; the ceiling is loose because this asserts the absence
	// of an unbounded wait, not a performance target.
	if elapsed > 15*time.Second {
		t.Fatalf("capture-once-returns-at-the-cap rule violated: elapsed=%s for a lane that never comes in, budget=%s", elapsed, nativeFirstTickBudget)
	}
	// The one-shot still emits everything that DID arrive, so giving up on a
	// lane costs that lane's evidence and nothing else.
	if snap == nil || snap.Graph == nil {
		t.Fatalf("capture-once-still-emits-a-graph rule violated: snap=%v", snap)
	}
}

// TestNativeFirstTickBudgetIsTwoSeconds pins the cap itself. Both tests above
// are written around it and neither would notice it drifting: the fast lane
// would still land and the slow one would still be dropped if the budget grew
// to a minute, and a one-shot that takes a minute is a hung command.
func TestNativeFirstTickBudgetIsTwoSeconds(t *testing.T) {
	if nativeFirstTickBudget != 2*time.Second {
		t.Fatalf("native-first-tick-budget-is-two-seconds rule violated: budget=%s", nativeFirstTickBudget)
	}
}

// --- Task 7: --screenshot-graph --------------------------------------------

// graphFrameDeps wires the PRODUCTION renderers against a fixed snapshot. The
// spy deps in testRunDeps hand back the string "FRAME", which proves routing
// and says nothing about what was drawn; this test's whole claim is about what
// lands on stdout, so the render funcs have to be the ones main() installs.
func graphFrameDeps(snap *snapshot.Snapshot, captures *atomic.Int32) runDeps {
	deps := productionRunDeps()
	deps.Now = func() time.Time { return time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC) }
	deps.ResolveTheme = func(string, string) theme.Theme { return theme.Nightfable() }
	deps.CaptureOnce = func(context.Context, runOptions) (*snapshot.Snapshot, error) {
		captures.Add(1)
		return snap, nil
	}
	return deps
}

const (
	graphFrameName = "gate-probe-79"
	// The requested size lives in one place, and the flag argument is built
	// from it, so "I asked for WxH" and "I got WxH" cannot drift apart.
	graphFrameW = 150
	graphFrameH = 42
)

func graphFrameSnapshot(nodes ...graph.Node) *snapshot.Snapshot {
	at := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	return &snapshot.Snapshot{
		At:    at,
		Rows:  []types.Row{},
		Graph: &graph.Snapshot{At: at, Nodes: nodes, Edges: []graph.Edge{}, Gaps: []graph.Gap{}},
	}
}

// graphFrameLinesWith counts LINES carrying a needle, not occurrences. Every
// frame's header carries the canary once, so the empty state is a second line
// rather than a first one, and a count separates "the pane said it is empty"
// from "the pane drew rows" without the test having to know where the header
// ends. Locating the body relative to the pane's own border tab would chain
// this assertion to the border assertion, and one plant would then move both.
func graphFrameLinesWith(frame, needle string) int {
	n := 0
	for _, l := range strings.Split(strings.TrimRight(frame, "\n"), "\n") {
		if strings.Contains(l, needle) {
			n++
		}
	}
	return n
}

func TestScreenshotGraphRendersTheGraphPane(t *testing.T) {
	populated := graphFrameSnapshot(graph.Node{
		ID: graph.NodeID("claude:session:62fee278"), Runtime: types.RuntimeClaude,
		Role: types.RolePrimary, ProvenName: graphFrameName, Model: "claude-fable-5",
		Project: "aitop", State: graph.NodeState{Value: graph.StateActive},
	})
	empty := graphFrameSnapshot()

	for _, tc := range []struct {
		name       string
		snap       *snapshot.Snapshot
		wantName   int
		wantCanary int
	}{
		// A populated pane names its nodes and must NOT fall back to the empty
		// state; an empty one says so out loud rather than painting blank, which
		// is byte-identical to a dead collector. One canary line is the header's,
		// which every frame carries; the second is the empty state itself.
		{"nodes", populated, 1, 1},
		{"empty", empty, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captures atomic.Int32
			deps := graphFrameDeps(tc.snap, &captures)
			var stdout, stderr bytes.Buffer
			size := fmt.Sprintf("%dx%d", graphFrameW, graphFrameH)
			status := run(context.Background(), []string{"--screenshot-graph=" + size}, &stdout, &stderr, deps)
			if status != 0 || captures.Load() != 1 {
				t.Fatalf("screenshot-graph-renders-the-graph-pane rule violated: status=%d captures=%d stderr=%q",
					status, captures.Load(), stderr.String())
			}
			frame := ansi.Strip(stdout.String())
			lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
			if len(lines) != graphFrameH {
				t.Fatalf("screenshot-graph-renders-the-requested-size rule violated: %d lines want %d", len(lines), graphFrameH)
			}
			// Height alone is not size, and asserting only the line count let a
			// hardcoded WIDTH through the entire suite: a 42-line frame is 42
			// lines at any width, the spy renderers discard their int arguments,
			// and internal/ui's geometry test calls RenderGraph directly rather
			// than through runScreenshot. Cell width, not byte length, because
			// the pane is drawn in box glyphs.
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != graphFrameW {
					t.Fatalf("screenshot-graph-renders-the-requested-size rule violated: line %d is %d cells wide, want %d: %q",
						i, w, graphFrameW, l)
				}
			}
			// The pane's own border tab. "2 graph" also appears in the TABLE's key
			// row, so a frame-wide scan for the bare word would certify the wrong
			// view; the tab glyphs are what only this pane's border carries.
			if !strings.Contains(frame, "┤ graph ├") {
				t.Fatalf("screenshot-graph-renders-the-graph-pane rule violated: no pane border tab in\n%s", frame)
			}
			if got := graphFrameLinesWith(frame, graphFrameName); got != tc.wantName {
				t.Fatalf("screenshot-graph-draws-the-nodes-it-was-given rule violated: %d lines name %q want %d:\n%s",
					got, graphFrameName, tc.wantName, frame)
			}
			if got := graphFrameLinesWith(frame, snapshot.Canary); got != tc.wantCanary {
				t.Fatalf("screenshot-graph-empty-is-not-quiet rule violated: %d lines carry %s want %d:\n%s",
					got, snapshot.Canary, tc.wantCanary, frame)
			}
		})
	}

	// The negative half. Without it "renders the graph pane" is unmeasured: a
	// --screenshot-graph wired to ui.Render would still exit 0, still write 42
	// lines, and still name the node, because the table draws it too.
	t.Run("table flag is untouched", func(t *testing.T) {
		var captures atomic.Int32
		deps := graphFrameDeps(populated, &captures)
		var stdout, stderr bytes.Buffer
		size := fmt.Sprintf("%dx%d", graphFrameW, graphFrameH)
		status := run(context.Background(), []string{"--screenshot=" + size}, &stdout, &stderr, deps)
		if status != 0 {
			t.Fatalf("screenshot-graph-leaves-the-table-flag-alone rule violated: status=%d stderr=%q", status, stderr.String())
		}
		frame := ansi.Strip(stdout.String())
		if strings.Contains(frame, "┤ graph ├") {
			t.Fatalf("screenshot-graph-leaves-the-table-flag-alone rule violated: --screenshot drew the graph pane:\n%s", frame)
		}
		// The older flag gets the same size assertions as the newer one. This
		// subtest already renders a real frame through the production deps, so
		// the hole the graph flag had -- a hardcoded width passing the entire
		// suite, because a frame of the right HEIGHT is the right height at any
		// width -- was open here too and nothing else closes it: the spy
		// renderers discard their int arguments, and internal/ui's geometry
		// tests call Render directly rather than through runScreenshot.
		lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
		if len(lines) != graphFrameH {
			t.Fatalf("screenshot-renders-the-requested-size rule violated: %d lines want %d", len(lines), graphFrameH)
		}
		// Cell width, not byte length: the table is drawn in box glyphs.
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != graphFrameW {
				t.Fatalf("screenshot-renders-the-requested-size rule violated: line %d is %d cells wide, want %d: %q",
					i, w, graphFrameW, l)
			}
		}
	})
}
