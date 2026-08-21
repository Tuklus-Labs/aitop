package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"aitop/internal/snapshot"
	"aitop/internal/ui"
)

func main() {
	jsonOnce := flag.Bool("json", false, "dump occupancy as JSON and exit")
	once := flag.Bool("once", false, "one sample and exit")
	flag.Parse()

	procRoot, grokHome, claudeHome := snapshot.DefaultHomes()
	eng := &snapshot.Engine{ProcRoot: procRoot, GrokHome: grokHome, ClaudeHome: claudeHome}
	if *jsonOnce || *once {
		eng.RefreshOverlay()
		if err := snapshot.WriteJSON(eng.Rows(), os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "aitop: json: %v\n", err)
			os.Exit(1)
		}
		return
	}
	box := eng.Start()
	p := tea.NewProgram(ui.New(box), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "aitop: %v\n", err)
		os.Exit(1)
	}
}
