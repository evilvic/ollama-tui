package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// TitleStyle is the style for titles
	TitleStyle = lipgloss.NewStyle().
			MarginLeft(2).
			Bold(true).
			Foreground(lipgloss.Color("#FF5F87"))

	// ResponseStyle is the style for responses
	ResponseStyle = lipgloss.NewStyle().
			MarginLeft(2).
			MarginRight(2)

	// StatusBarStyle is the style for the status bar
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AFAFAF")).
			Reverse(true)

	// InputBoxStyle is the style for the input box
	InputBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF5F87")).
			Padding(0, 1)

	// ContainerStyle is the style for the container
	ContainerStyle = lipgloss.NewStyle()

	// ChatAreaStyle is the style for the chat area
	ChatAreaStyle = lipgloss.NewStyle()
)
