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
	// StateProviderSelect is the state for selecting a provider
	StateProviderSelect = iota
	// StateAPIKeyInput is the state for entering an API key
	StateAPIKeyInput
	// StateModelSelect is the state for selecting a model
	StateModelSelect
	// StatePrompting is the state for entering a prompt
	StatePrompting
	// StateLoading is the state for loading a response
	StateLoading
)

// Model represents the UI model
type Model struct {
	State              int
	ProviderList       list.Model
	List               list.Model
	Models             []models.Model
	SelectedProvider   string
	SelectedModel      string
	Input              textarea.Model
	APIKeyInput        textarea.Model
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

	// Provider list
	pl := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	pl.Title = "Available providers"
	pl.SetShowStatusBar(false)
	pl.SetFilteringEnabled(false)
	pl.Styles.Title = TitleStyle

	// Add Ollama as the only provider for now
	pl.SetItems([]list.Item{
		models.ListItem{
			Name:    "ollama",
			Details: "Local LLM server",
		},
		models.ListItem{
			Name:    "openai",
			Details: "OpenAI API",
		},
	})

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

	// API Key input
	apiKeyInput := textarea.New()
	apiKeyInput.Placeholder = "Enter your OpenAI API key..."
	apiKeyInput.Focus()
	apiKeyInput.CharLimit = 100
	apiKeyInput.SetWidth(100)
	apiKeyInput.SetHeight(3)
	apiKeyInput.ShowLineNumbers = false

	vp := viewport.New(0, 0)
	vp.Style = ResponseStyle
	vp.SetContent("Responses will appear here.\n\n")

	return Model{
		State:              StateProviderSelect,
		ProviderList:       pl,
		List:               l,
		Spinner:            s,
		Input:              ta,
		APIKeyInput:        apiKeyInput,
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
	// Send initial commands to start the spinner and enter alt screen
	// We'll fetch models after provider selection
	cmds := []tea.Cmd{
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
	if state == StateProviderSelect || state == StateModelSelect || state == StateAPIKeyInput {
		return width, height - 4
	}

	// For chat view, use the full width and height
	return width, height
}

// View renders the UI
func (m Model) View() string {
	switch m.State {
	case StateProviderSelect:
		return m.ProviderList.View()

	case StateAPIKeyInput:
		// Create a container for the API key input
		width := m.ScreenWidth
		height := m.ScreenHeight

		// Title
		titleView := TitleStyle.Render("OpenAI API Key Required")

		// Instructions
		instructions := "Please enter your OpenAI API key to continue.\nYou can find your API key at https://platform.openai.com/api-keys\n\nPress Enter to continue or Esc to go back."
		instructionsView := lipgloss.NewStyle().
			Width(width-4).
			Padding(1, 0, 1, 0).
			Render(instructions)

		// Input
		inputStyle := InputBoxStyle.Copy().Width(width - 4)
		inputView := inputStyle.Render(m.APIKeyInput.View())

		// Combine views
		content := lipgloss.JoinVertical(
			lipgloss.Left,
			titleView,
			"\n",
			instructionsView,
			"\n",
			inputView,
		)

		return lipgloss.Place(
			width,
			height,
			lipgloss.Center,
			lipgloss.Center,
			content,
		)

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
