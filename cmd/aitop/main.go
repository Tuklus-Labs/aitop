package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"aitop/internal/act"
	actclaude "aitop/internal/act/claude"
	actcodex "aitop/internal/act/codex"
	actgrok "aitop/internal/act/grok"
	actlocal "aitop/internal/act/local"
	"aitop/internal/overlay/inference"
	"aitop/internal/price"
	"aitop/internal/snapshot"
	"aitop/internal/supervisor"
	"aitop/internal/theme"
	"aitop/internal/types"
	"aitop/internal/ui"
)

type runOptions struct {
	JSON          bool
	Once          bool
	Screenshot    string
	ScreenshotSet bool
	ThemePath     string
	PricesPath    string
	NoPrices      bool
	Interval      time.Duration
}

type runSupervisor interface {
	Context() context.Context
	Go(string, func(context.Context) error) error
	Shutdown() []supervisor.Failure
}

type diagnosticClass string

const (
	diagnosticScopeUsage       = "usage"
	diagnosticScopeDependency  = "dependency"
	diagnosticScopeCapture     = "capture"
	diagnosticScopeJSON        = "json"
	diagnosticScopeScreenshot  = "screenshot"
	diagnosticScopeRegister    = "register"
	diagnosticScopeInteractive = "interactive"
	diagnosticScopeShutdown    = "shutdown"
	diagnosticScopeInternal    = "internal"

	diagnosticTaskFlags       = "flags"
	diagnosticTaskJSON        = "json"
	diagnosticTaskScreenshot  = "screenshot"
	diagnosticTaskInteractive = "interactive"
	diagnosticTaskWrite       = "write"
	diagnosticTaskActor       = "actor"
	diagnosticTaskRun         = "run"
	diagnosticTaskSupervisor  = "supervisor"
	diagnosticTaskFailure     = "failure"

	diagnosticCanceled    diagnosticClass = "canceled"
	diagnosticDeadline    diagnosticClass = "deadline"
	diagnosticIO          diagnosticClass = "io"
	diagnosticUsage       diagnosticClass = "usage"
	diagnosticDependency  diagnosticClass = "dependency"
	diagnosticCapture     diagnosticClass = "capture"
	diagnosticJSON        diagnosticClass = "json"
	diagnosticScreenshot  diagnosticClass = "screenshot"
	diagnosticRegister    diagnosticClass = "register"
	diagnosticInteractive diagnosticClass = "interactive"
	diagnosticShutdown    diagnosticClass = "shutdown"
	diagnosticFailure     diagnosticClass = "failure"
)

type runDeps struct {
	Now              func() time.Time
	CaptureOnce      func(context.Context, runOptions) (*snapshot.Snapshot, error)
	WriteJSON        func(*snapshot.Snapshot, io.Writer, time.Time) error
	ResolveTheme     func(path, configured string) theme.Theme
	RenderScreenshot func(*snapshot.Snapshot, theme.Theme, int, int, time.Time) string
	NewSupervisor    func(context.Context) runSupervisor
	NewActor         func() *act.Actor
	RunInteractive   func(context.Context, runOptions, taskRegistrar, *act.Actor, io.Writer) error
}

var (
	errUsage             = errors.New("usage")
	errMissingDependency = errors.New("missing selected dependency")
	errNilSupervisor     = errors.New("nil supervisor")
	errNilActor          = errors.New("nil actor")
	errInvalidScreenshot = errors.New("invalid screenshot size")
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, productionRunDeps()))
}

func productionRunDeps() runDeps {
	return runDeps{
		Now:          time.Now,
		CaptureOnce:  productionCaptureOnce,
		WriteJSON:    snapshot.WriteJSON,
		ResolveTheme: theme.Resolve,
		RenderScreenshot: func(s *snapshot.Snapshot, th theme.Theme, w, h int, now time.Time) string {
			lipgloss.SetColorProfile(termenv.TrueColor)
			return ui.Render(s, th, w, h, now)
		},
		NewSupervisor: func(ctx context.Context) runSupervisor {
			return supervisor.New(ctx)
		},
		NewActor: func() *act.Actor {
			actor := act.New(map[types.Runtime]act.Adapter{
				types.RuntimeLocal:  actlocal.New(),
				types.RuntimeGrok:   actgrok.New(),
				types.RuntimeClaude: actclaude.New(),
				types.RuntimeCodex:  actcodex.New(),
			})
			actor.CapsuleDir = act.CapsuleRoot()
			actor.ForksDir = act.ForksRoot()
			return actor
		},
		RunInteractive: productionRunInteractive,
	}
}

// attachGraph is the production graph attach, indirected so a test can see the
// homes the run paths pass it. The engine's homes and the graph's homes are two
// separate arguments to two separate subsystems, and nothing but this seam can
// tell "wired to the engine's homes" from "wired to some homes".
var attachGraph = snapshot.AttachGraph

// nativeFirstTickBudget bounds how long a one-shot waits for the native lanes'
// first disk walk, in total across every lane. A one-shot has no next tick: it
// samples the graph once and exits, so a lane still walking a session store
// when the sample is taken is simply missing from the output, with gaps=0 and
// nothing else to say so. The wait is bounded rather than blocking because a
// pathological store must cost a late graph, never a hung command.
const nativeFirstTickBudget = 2 * time.Second

// productionGraphHomes is the one place the engine's homes become the graph's.
// Both run paths go through it so a home added to the engine cannot reach the
// overlay collectors while silently missing the native ones.
func productionGraphHomes(eng *snapshot.Engine) snapshot.GraphHomes {
	if eng == nil {
		return snapshot.GraphHomes{}
	}
	return snapshot.GraphHomes{Claude: eng.ClaudeHome, Codex: eng.CodexHome, Grok: eng.GrokHome}
}

func productionEngine(opt runOptions) *snapshot.Engine {
	var prices *price.Table
	if !opt.NoPrices {
		prices = price.Builtin()
		up := opt.PricesPath
		if up == "" {
			up = price.UserPath()
		}
		_ = prices.LoadUser(up)
	}
	procRoot, grokHome, claudeHome, codexHome, hbDir, forksDir := snapshot.DefaultHomes()
	return &snapshot.Engine{
		ProcRoot:   procRoot,
		GrokHome:   grokHome,
		ClaudeHome: claudeHome,
		CodexHome:  codexHome,
		HBDir:      hbDir,
		ForksDir:   forksDir,
		Interval:   opt.Interval,
		Prices:     prices,
		Inference:  inference.NewPoller(procRoot),
	}
}

func reapEngineOnCancel(ctx context.Context, wait func()) {
	if ctx == nil || wait == nil {
		return
	}
	go func() {
		<-ctx.Done()
		wait()
	}()
}

func productionCaptureOnce(ctx context.Context, opt runOptions) (*snapshot.Snapshot, error) {
	eng := productionEngine(opt)
	shadow, occ, lanes, err := attachGraph(eng, productionGraphHomes(eng))
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Rows BEFORE collectors. Every collector's first tick reads the engine's
	// current rows, and that is where a session's process binding lives: a
	// native collector that ticks against an empty engine dates its nodes by
	// their invocation incarnation, and occupancy's process-incarnation events
	// for the same sessions are then rejected on identity. That is the
	// incarnation fight the shared incarnation rule exists to prevent, and it
	// costs the one-shot its occupancy rows for good, because a one-shot has no
	// next tick to converge on. Measured on this box: 3 collision gaps and a
	// Partial occupancy source, every run, until the capture moved up here.
	snap, err := eng.CaptureOnce(ctx)
	if err != nil {
		return snap, err
	}
	done := make(chan error, 1)
	go func() { done <- shadow.Run(runCtx) }()
	occ.Notify()
	rows := []types.Row(nil)
	if snap != nil {
		rows = snap.Rows
	}
	snapshot.WaitOccupancyGraph(shadow, rows, 500*time.Millisecond)
	// Occupancy's wait is satisfied by any node, and occupancy publishes one
	// well before a native lane has finished walking a home, so it says nothing
	// about whether the native evidence is in. This is the wait that does.
	lanes.WaitNativeEvidence(shadow, nativeFirstTickBudget)
	if snap != nil {
		snap.Graph = shadow.Snapshot()
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	return snap, nil
}

func productionRunInteractive(ctx context.Context, opt runOptions, reg taskRegistrar, actor *act.Actor, out io.Writer) error {
	eng := productionEngine(opt)
	// The interactive path takes no lanes: it repaints every 2 s, so a lane
	// still walking on the first frame is in by the next one. Only a one-shot
	// has to wait, because it has no second frame.
	shadow, _, _, err := attachGraph(eng, productionGraphHomes(eng))
	if err != nil {
		return err
	}
	// Rows before collectors, for the reason recorded in productionCaptureOnce.
	// Start refreshes the overlay synchronously before it returns, so by the
	// time the shadow runs every collector's first tick sees the engine's rows.
	// The interactive path would converge on its own two seconds later; the
	// ordering is the same here so that a session's incarnation does not move
	// under the graph in the first frames anyone actually looks at.
	src := eng.Start(ctx)
	reapEngineOnCancel(ctx, eng.Wait)
	if reg != nil {
		if err := reg.Go("graph", shadow.Run); err != nil {
			return err
		}
	} else {
		go func() { _ = shadow.Run(ctx) }()
	}
	th := theme.Resolve(opt.ThemePath, "")
	p := tea.NewProgram(
		ui.New(src, th, actor.Enqueue),
		tea.WithAltScreen(),
		tea.WithOutput(out),
		tea.WithContext(ctx),
	)
	_, err = p.Run()
	return err
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps runDeps) int {
	opt, err := parseRunOptions(args)
	if err != nil {
		writeDiag(stderr, diagnosticScopeUsage, diagnosticTaskFlags, err)
		return 2
	}
	if opt.ScreenshotSet && (opt.Screenshot == "" || opt.JSON || opt.Once) {
		writeDiag(stderr, diagnosticScopeUsage, diagnosticTaskFlags, errUsage)
		return 2
	}
	if opt.JSON || opt.Once {
		return runJSON(ctx, stdout, stderr, opt, deps)
	}
	if opt.ScreenshotSet {
		return runScreenshot(ctx, stdout, stderr, opt, deps)
	}
	return runInteractive(ctx, stdout, stderr, opt, deps)
}

func parseRunOptions(args []string) (runOptions, error) {
	fs := flag.NewFlagSet("aitop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOnce := fs.Bool("json", false, "dump occupancy as JSON and exit")
	once := fs.Bool("once", false, "one sample and exit (same as --json)")
	themePath := fs.String("theme", "", "btop .theme file")
	interval := fs.Duration("interval", 100*time.Millisecond, "proc sample and paint interval")
	screenshot := fs.String("screenshot", "", "render one frame at WxH to stdout and exit")
	pricesPath := fs.String("prices", "", "price table override")
	noPrices := fs.Bool("no-prices", false, "never estimate cost")
	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}
	opt := runOptions{
		JSON:       *jsonOnce,
		Once:       *once,
		Screenshot: *screenshot,
		ThemePath:  *themePath,
		PricesPath: *pricesPath,
		NoPrices:   *noPrices,
		Interval:   *interval,
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "screenshot" {
			opt.ScreenshotSet = true
		}
	})
	return opt, nil
}

func runJSON(ctx context.Context, stdout, stderr io.Writer, opt runOptions, deps runDeps) int {
	if deps.Now == nil || deps.CaptureOnce == nil || deps.WriteJSON == nil {
		writeDiag(stderr, diagnosticScopeDependency, diagnosticTaskJSON, errMissingDependency)
		return 1
	}
	now := deps.Now()
	snap, err := deps.CaptureOnce(ctx, opt)
	if err != nil {
		writeDiag(stderr, diagnosticScopeCapture, diagnosticTaskJSON, err)
		return 1
	}
	if err := deps.WriteJSON(snap, stdout, now); err != nil {
		writeDiag(stderr, diagnosticScopeJSON, diagnosticTaskWrite, err)
		return 1
	}
	return 0
}

func runScreenshot(ctx context.Context, stdout, stderr io.Writer, opt runOptions, deps runDeps) int {
	var width, height int
	if _, err := fmt.Sscanf(opt.Screenshot, "%dx%d", &width, &height); err != nil || width < 1 || height < 1 {
		writeDiag(stderr, diagnosticScopeUsage, diagnosticTaskFlags, errInvalidScreenshot)
		return 2
	}
	if deps.Now == nil || deps.CaptureOnce == nil || deps.ResolveTheme == nil || deps.RenderScreenshot == nil {
		writeDiag(stderr, diagnosticScopeDependency, diagnosticTaskScreenshot, errMissingDependency)
		return 1
	}
	now := deps.Now()
	snap, err := deps.CaptureOnce(ctx, opt)
	if err != nil {
		writeDiag(stderr, diagnosticScopeCapture, diagnosticTaskScreenshot, err)
		return 1
	}
	th := deps.ResolveTheme(opt.ThemePath, "")
	frame := deps.RenderScreenshot(snap, th, width, height, now)
	n, err := io.WriteString(stdout, frame)
	if err != nil || n != len(frame) {
		if err == nil {
			err = io.ErrShortWrite
		}
		writeDiag(stderr, diagnosticScopeScreenshot, diagnosticTaskWrite, err)
		return 1
	}
	return 0
}

func runInteractive(ctx context.Context, stdout, stderr io.Writer, opt runOptions, deps runDeps) int {
	if deps.NewSupervisor == nil || deps.NewActor == nil || deps.RunInteractive == nil {
		writeDiag(stderr, diagnosticScopeDependency, diagnosticTaskInteractive, errMissingDependency)
		return 1
	}
	sup := deps.NewSupervisor(ctx)
	if sup == nil {
		writeDiag(stderr, diagnosticScopeDependency, diagnosticTaskInteractive, errNilSupervisor)
		return 1
	}
	actor := deps.NewActor()
	if actor == nil {
		writeDiag(stderr, diagnosticScopeDependency, diagnosticTaskInteractive, errNilActor)
		reportShutdown(stderr, sup.Shutdown())
		return 1
	}
	if err := registerActor(taskRegistrarFunc(sup.Go), actor); err != nil {
		writeDiag(stderr, diagnosticScopeRegister, diagnosticTaskActor, err)
		reportShutdown(stderr, sup.Shutdown())
		return 1
	}
	runErr := deps.RunInteractive(sup.Context(), opt, taskRegistrarFunc(sup.Go), actor, stdout)
	failures := sup.Shutdown()
	if runErr != nil {
		writeDiag(stderr, diagnosticScopeInteractive, diagnosticTaskRun, runErr)
	}
	reportShutdown(stderr, failures)
	if runErr != nil || len(failures) > 0 {
		return 1
	}
	return 0
}

func reportShutdown(stderr io.Writer, failures []supervisor.Failure) {
	for _, failure := range failures {
		writeDiag(stderr, diagnosticScopeShutdown, diagnosticTaskSupervisor, failure.Err)
	}
}

func writeDiag(stderr io.Writer, scope, task string, err error) {
	line := safeDiagnostic(scope, task, err)
	if line == "" {
		return
	}
	_, _ = fmt.Fprintln(stderr, line)
}

func safeDiagnostic(scope, task string, err error) string {
	normalizedScope, normalizedTask, fallback := normalizeDiagnosticPair(scope, task)
	if err == nil {
		return ""
	}
	class := fallback
	switch {
	case errors.Is(err, context.Canceled):
		class = diagnosticCanceled
	case errors.Is(err, context.DeadlineExceeded):
		class = diagnosticDeadline
	case errors.Is(err, io.ErrShortWrite):
		class = diagnosticIO
	}
	return fmt.Sprintf("aitop scope=%s task=%s class=%s", normalizedScope, normalizedTask, class)
}

func normalizeDiagnosticPair(scope, task string) (string, string, diagnosticClass) {
	switch scope + "/" + task {
	case "usage/flags":
		return diagnosticScopeUsage, diagnosticTaskFlags, diagnosticUsage
	case "dependency/json":
		return diagnosticScopeDependency, diagnosticTaskJSON, diagnosticDependency
	case "dependency/screenshot":
		return diagnosticScopeDependency, diagnosticTaskScreenshot, diagnosticDependency
	case "dependency/interactive":
		return diagnosticScopeDependency, diagnosticTaskInteractive, diagnosticDependency
	case "capture/json":
		return diagnosticScopeCapture, diagnosticTaskJSON, diagnosticCapture
	case "capture/screenshot":
		return diagnosticScopeCapture, diagnosticTaskScreenshot, diagnosticCapture
	case "json/write":
		return diagnosticScopeJSON, diagnosticTaskWrite, diagnosticJSON
	case "screenshot/write":
		return diagnosticScopeScreenshot, diagnosticTaskWrite, diagnosticScreenshot
	case "register/actor":
		return diagnosticScopeRegister, diagnosticTaskActor, diagnosticRegister
	case "interactive/run":
		return diagnosticScopeInteractive, diagnosticTaskRun, diagnosticInteractive
	case "shutdown/supervisor":
		return diagnosticScopeShutdown, diagnosticTaskSupervisor, diagnosticShutdown
	default:
		return diagnosticScopeInternal, diagnosticTaskFailure, diagnosticFailure
	}
}

type taskRegistrar interface {
	Go(string, func(context.Context) error) error
}

type taskRegistrarFunc func(string, func(context.Context) error) error

func (f taskRegistrarFunc) Go(name string, run func(context.Context) error) error {
	return f(name, run)
}

var errNilActorRegistrar = errors.New("register-actor nil registrar or actor rule violated")

func registerActor(reg taskRegistrar, actor *act.Actor) error {
	if reg == nil || actor == nil {
		return errNilActorRegistrar
	}
	return reg.Go("actor", actor.Run)
}
