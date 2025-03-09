package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/evilvic/ollama-tui/pkg/models"
	"github.com/evilvic/ollama-tui/pkg/utils"
)

const (
	// StateModelSelect is the state for selecting a model
	StateModelSelect = iota
	// StatePrompting is the state for entering a prompt
	StatePrompting
	// StateLoading is the state for loading a response
	StateLoading
)

// Model represents the UI model
type Model struct {
	State              int
	List               list.Model
	Models             []models.Model
	SelectedModel      string
	Input              textarea.Model
	Viewport           viewport.Model
	Spinner            spinner.Model
	Responses          []string
	CurrentPrompt      string
	CurrentResponse    string
	Err                error
	InProgressResponse string
	IsGenerating       bool
	ScreenWidth        int
	ScreenHeight       int
	CancelGenerate     context.CancelFunc
	ViewportFocused    bool
}

// TokenMsg represents a token message
type TokenMsg struct {
	Token string
	Done  bool
}

// FetchModelsMsg represents a fetch models message
type FetchModelsMsg struct {
	Models []models.Model
}

// ErrorMsg represents an error message
type ErrorMsg struct {
	Err error
}

// SetCancelFuncMsg represents a message to set the cancel function
type SetCancelFuncMsg struct {
	Cancel context.CancelFunc
}

// NewModel creates a new UI model
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Available models"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = TitleStyle

	ta := textarea.New()
	ta.Placeholder = "Write your prompt here..."
	ta.Focus()
	ta.CharLimit = 5000
	ta.SetWidth(100)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	vp := viewport.New(0, 0)
	vp.Style = ResponseStyle
	vp.SetContent("Responses will appear here.\n\n")

	return Model{
		State:              StateModelSelect,
		List:               l,
		Spinner:            s,
		Input:              ta,
		Viewport:           vp,
		Responses:          []string{},
		InProgressResponse: "",
		IsGenerating:       false,
		ScreenWidth:        80,
		ScreenHeight:       24,
		ViewportFocused:    false,
	}
}

// Init initializes the UI model
func (m Model) Init() tea.Cmd {
	// Send initial commands to fetch models and start the spinner
	// Also send a WindowSizeMsg to initialize the layout properly
	cmds := []tea.Cmd{
		FetchModelsCmd,
		m.Spinner.Tick,
		tea.EnterAltScreen,
	}

	// Get initial terminal size and add a command to send a window size message
	if width, height, err := term.GetSize(int(0)); err == nil {
		cmds = append(cmds, func() tea.Msg {
			return tea.WindowSizeMsg{Width: width, Height: height}
		})
	} else {
		// Use default size if we can't get the terminal size
		cmds = append(cmds, InitializeWindowSizeCmd)
	}

	return tea.Batch(cmds...)
}

// InitializeWindowSizeCmd is a command to initialize the window size
func InitializeWindowSizeCmd() tea.Msg {
	// Use a reasonable default size that will be updated when the actual window size is detected
	return tea.WindowSizeMsg{Width: 80, Height: 24}
}

// AppLayout returns the layout dimensions for the application
func AppLayout(width, height int, state int) (int, int) {
	if state == StateModelSelect {
		return width, height - 4
	}

	// For chat view, use the full width and height
	return width, height
}

// View renders the UI
func (m Model) View() string {
	switch m.State {
	case StateModelSelect:
		return m.List.View()

	case StatePrompting, StateLoading:
		// Get terminal dimensions
		width := m.ScreenWidth
		height := m.ScreenHeight
		if width <= 0 {
			width = 80 // Default width if not set
		}
		if height <= 0 {
			height = 24 // Default height if not set
		}

		// Create a container for the entire UI
		container := lipgloss.NewStyle().Width(width).Height(height)

		// Title section
		titleView := TitleStyle.Render(fmt.Sprintf("Chat with %s", m.SelectedModel))
		titleHeight := lipgloss.Height(titleView) + 2 // +2 for spacing

		// Input section (fixed at bottom)
		inputStyle := InputBoxStyle.Copy().Width(width - 4)
		if !m.ViewportFocused {
			inputStyle = inputStyle.BorderForeground(lipgloss.Color("#FF5F87"))
		} else {
			inputStyle = inputStyle.BorderForeground(lipgloss.Color("#AFAFAF"))
		}
		inputView := inputStyle.Render(m.Input.View())
		inputHeight := lipgloss.Height(inputView)

		// Status bar (fixed at bottom)
		contextIndicator := ""
		if APIClient.HasContext() {
			contextIndicator = "🔄 Context Active | "
		}
		statusText := fmt.Sprintf(" %s | %sTab: Toggle focus | Ctrl+N: New Chat | Ctrl+C: Exit ", m.SelectedModel, contextIndicator)
		statusView := StatusBarStyle.Copy().Width(width).Render(statusText)
		statusHeight := lipgloss.Height(statusView)

		// Loading indicator
		var loadingView string
		loadingHeight := 0
		if m.State == StateLoading && m.IsGenerating {
			loadingView = fmt.Sprintf("  %s Generating...", m.Spinner.View())
			loadingHeight = 1
		}

		// Calculate viewport height
		// Available height = total height - (title + input + status + loading + spacing)
		viewportHeight := height - titleHeight - inputHeight - statusHeight - loadingHeight - 2
		if viewportHeight < 5 {
			viewportHeight = 5
		}

		// Set viewport style with calculated height
		viewportStyle := ResponseStyle.Copy()
		if m.ViewportFocused {
			viewportStyle = viewportStyle.BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FF5F87"))
		}

		// Ensure viewport has the correct height
		m.Viewport.Height = viewportHeight
		m.Viewport.Width = width - 4

		// Render the viewport
		viewportView := viewportStyle.Render(m.Viewport.View())

		// Build the final view with fixed positions
		var sb strings.Builder

		// Title at the top
		sb.WriteString(titleView)
		sb.WriteString("\n\n")

		// Viewport in the middle (takes available space)
		sb.WriteString(viewportView)
		sb.WriteString("\n")

		// Loading indicator before input
		if loadingView != "" {
			sb.WriteString(loadingView)
			sb.WriteString("\n")
		}

		// Input box fixed at the bottom
		sb.WriteString(inputView)
		sb.WriteString("\n")

		// Status bar at the very bottom
		sb.WriteString(statusView)

		return container.Render(sb.String())

	default:
		return "Unknown state"
	}
}

// UpdateViewportContent updates the viewport content with the current responses
func (m *Model) UpdateViewportContent() {
	var content strings.Builder
	for _, resp := range m.Responses {
		content.WriteString(resp)
		content.WriteString("\n\n")
	}
	m.Viewport.SetContent(content.String())
	m.Viewport.GotoBottom()
}

// AddResponse adds a response to the list of responses
func (m *Model) AddResponse(prompt, response string) {
	m.Responses = append(m.Responses, fmt.Sprintf("Prompt: %s\n\nResponse:\n%s", prompt, response))
	m.UpdateViewportContent()
}

// UpdateResponse updates the last response with new content
func (m *Model) UpdateResponse(prompt, response string) {
	if len(m.Responses) > 0 {
		responseText := response
		if m.ScreenWidth > 10 {
			responseText = utils.WrapText(response, m.ScreenWidth-10)
		}
		m.Responses[len(m.Responses)-1] = fmt.Sprintf("Prompt: %s\n\nResponse:\n%s", prompt, responseText)
		m.UpdateViewportContent()
	}
}
