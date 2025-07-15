package main

import (
	"fmt"
	"os"

	"github.com/LFroesch/logdog/internal/logdog"
	"github.com/LFroesch/logdog/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	logdog.Info("Starting Logdog...")
	p := tea.NewProgram(tui.NewModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
