package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evilvic/ollama-tui/pkg/ui"
)

func main() {
	// Use the full terminal screen and enable mouse support
	p := tea.NewProgram(
		ui.NewModel(),
		tea.WithAltScreen(),       // Use the alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run the program
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error initializing application: %v\n", err)
		os.Exit(1)
	}
}
