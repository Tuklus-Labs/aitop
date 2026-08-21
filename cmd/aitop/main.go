package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"aitop/internal/snapshot"
	"aitop/internal/theme"
	"aitop/internal/ui"
)

func main() {
	jsonOnce := flag.Bool("json", false, "dump occupancy as JSON and exit")
	once := flag.Bool("once", false, "one sample and exit (same as --json)")
	themePath := flag.String("theme", "", "btop .theme file (default: $AITOP_THEME, then btop.conf color_theme, then built-in nightfable)")
	interval := flag.Duration("interval", 100*time.Millisecond, "proc sample and paint interval")
	screenshot := flag.String("screenshot", "", "render one frame at WxH (e.g. 120x40) to stdout and exit")
	flag.Parse()

	procRoot, grokHome, claudeHome, codexHome, hbDir := snapshot.DefaultHomes()
	eng := &snapshot.Engine{
		ProcRoot:   procRoot,
		GrokHome:   grokHome,
		ClaudeHome: claudeHome,
		CodexHome:  codexHome,
		HBDir:      hbDir,
		Interval:   *interval,
	}
	if *jsonOnce || *once {
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
		eng.RefreshOverlay()
		time.Sleep(300 * time.Millisecond)
		eng.RefreshOverlay()
		fmt.Print(ui.Render(eng.Snapshot(), th, w, h, time.Now()))
		return
	}
	src := eng.Start()
	p := tea.NewProgram(ui.New(src, th), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "aitop: %v\n", err)
		os.Exit(1)
	}
}
