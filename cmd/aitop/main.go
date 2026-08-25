package main

import (
	"flag"
	"fmt"
	"os"
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
	"aitop/internal/theme"
	"aitop/internal/types"
	"aitop/internal/ui"
)

func main() {
	jsonOnce := flag.Bool("json", false, "dump occupancy as JSON and exit")
	once := flag.Bool("once", false, "one sample and exit (same as --json)")
	themePath := flag.String("theme", "", "btop .theme file (default: $AITOP_THEME, then btop.conf color_theme, then built-in nightfable)")
	interval := flag.Duration("interval", 100*time.Millisecond, "proc sample and paint interval")
	screenshot := flag.String("screenshot", "", "render one frame at WxH (e.g. 120x40) to stdout and exit")
	pricesPath := flag.String("prices", "", "price table override (default: $AITOP_PRICES, then ~/.config/aitop/prices.json)")
	noPrices := flag.Bool("no-prices", false, "never estimate cost; COST stays — unless a runtime reports it")
	flag.Parse()

	var prices *price.Table
	if !*noPrices {
		prices = price.Builtin()
		up := *pricesPath
		if up == "" {
			up = price.UserPath()
		}
		if err := prices.LoadUser(up); err != nil {
			fmt.Fprintf(os.Stderr, "aitop: %v (builtin prices still apply)\n", err)
		}
	}

	procRoot, grokHome, claudeHome, codexHome, hbDir, forksDir := snapshot.DefaultHomes()
	eng := &snapshot.Engine{
		ProcRoot:   procRoot,
		GrokHome:   grokHome,
		ClaudeHome: claudeHome,
		CodexHome:  codexHome,
		HBDir:      hbDir,
		ForksDir:   forksDir,
		Interval:   *interval,
		Prices:     prices,
		Inference:  inference.NewPoller(procRoot),
	}
	if *jsonOnce || *once {
		eng.Inference.Poll() // one synchronous probe so locals carry tokens
		eng.RefreshOverlay()
		time.Sleep(200 * time.Millisecond) // second sample so CPU% is known
		eng.RefreshOverlay()
		if err := snapshot.WriteJSON(eng.Snapshot(), os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aitop: json: %v\n", err)
			os.Exit(1)
		}
		return
	}
	th := theme.Resolve(*themePath, "")
	if *screenshot != "" {
		var w, h int
		if _, err := fmt.Sscanf(*screenshot, "%dx%d", &w, &h); err != nil || w < 1 || h < 1 {
			fmt.Fprintf(os.Stderr, "aitop: --screenshot wants WxH, got %q\n", *screenshot)
			os.Exit(2)
		}
		lipgloss.SetColorProfile(termenv.TrueColor) // piped output keeps its ink
		eng.Inference.Poll()
		eng.RefreshOverlay()
		time.Sleep(300 * time.Millisecond)
		eng.RefreshOverlay()
		fmt.Print(ui.Render(eng.Snapshot(), th, w, h, time.Now()))
		return
	}
	src := eng.Start()
	actor := act.New(map[types.Runtime]act.Adapter{
		types.RuntimeLocal:  actlocal.New(),
		types.RuntimeGrok:   actgrok.New(),
		types.RuntimeClaude: actclaude.New(),
		types.RuntimeCodex:  actcodex.New(),
	})
	actor.CapsuleDir = act.CapsuleRoot()
	actor.ForksDir = act.ForksRoot()
	actor.Start()
	defer actor.Stop()
	p := tea.NewProgram(ui.New(src, th, actor.Enqueue), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "aitop: %v\n", err)
		os.Exit(1)
	}
}
