package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

const (
	ollamaURL = "http://localhost:11434"
)

var (
	titleStyle     = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color("#FF5F87"))
	responseStyle  = lipgloss.NewStyle().MarginLeft(2).MarginRight(2)
	statusBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AFAFAF")).Reverse(true)
	inputBoxStyle  = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FF5F87")).Padding(0, 1)

	// Add a container style for the entire UI
	containerStyle = lipgloss.NewStyle()

	// Add a style for the chat area (viewport)
	chatAreaStyle = lipgloss.NewStyle()
)

type Model struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	Digest  string `json:"digest"`
	Details struct {
		Family  string `json:"family"`
		Format  string `json:"format"`
		Context int    `json:"context"`
	} `json:"details"`
}

type ModelListResponse struct {
	Models []Model `json:"models"`
}

type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type GenerateResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
}

type item struct {
	name    string
	details string
}

func (i item) Title() string       { return i.name }
func (i item) Description() string { return i.details }
func (i item) FilterValue() string { return i.name }

type fetchModelsMsg struct{ models []Model }
type errorMsg struct{ err error }
type tokenMsg struct {
	token string
	done  bool
}

const (
	stateModelSelect = iota
	statePrompting
	stateLoading
)

type mainModel struct {
	state              int
	list               list.Model
	models             []Model
	selectedModel      string
	input              textarea.Model
	viewport           viewport.Model
	spinner            spinner.Model
	responses          []string
	currentPrompt      string
	currentResponse    string
	err                error
	inProgressResponse string
	isGenerating       bool
	screenWidth        int
	screenHeight       int
	cancelGenerate     context.CancelFunc
	viewportFocused    bool
}

func wrapText(text string, width int) string {
	if width <= 10 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result []string

	for _, line := range lines {
		if len(line) <= width {
			result = append(result, line)
			continue
		}

		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}

		currentLine := words[0]
		currentWidth := len(words[0])

		for i := 1; i < len(words); i++ {
			word := words[i]
			if currentWidth+1+len(word) > width {
				result = append(result, currentLine)
				currentLine = word
				currentWidth = len(word)
			} else {
				currentLine += " " + word
				currentWidth += 1 + len(word)
			}
		}

		if currentLine != "" {
			result = append(result, currentLine)
		}
	}

	return strings.Join(result, "\n")
}

func initialModel() mainModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Available models"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = titleStyle

	ta := textarea.New()
	ta.Placeholder = "Write your prompt here..."
	ta.Focus()
	ta.CharLimit = 5000
	ta.SetWidth(100)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	vp := viewport.New(0, 0)
	vp.Style = responseStyle
	vp.SetContent("Responses will appear here.\n\n")

	return mainModel{
		state:              stateModelSelect,
		list:               l,
		spinner:            s,
		input:              ta,
		viewport:           vp,
		responses:          []string{},
		inProgressResponse: "",
		isGenerating:       false,
		screenWidth:        80,
		screenHeight:       24,
		viewportFocused:    false,
	}
}

func (m mainModel) Init() tea.Cmd {
	// Send initial commands to fetch models and start the spinner
	// Also send a WindowSizeMsg to initialize the layout properly
	cmds := []tea.Cmd{
		fetchModels(),
		m.spinner.Tick,
		tea.EnterAltScreen,
	}

	// Get initial terminal size and add a command to send a window size message
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		cmds = append(cmds, func() tea.Msg {
			return tea.WindowSizeMsg{Width: width, Height: height}
		})
	} else {
		// Use default size if we can't get the terminal size
		cmds = append(cmds, initializeWindowSize)
	}

	return tea.Batch(cmds...)
}

// Helper function to send an initial window size message
func initializeWindowSize() tea.Msg {
	// Use a reasonable default size that will be updated when the actual window size is detected
	return tea.WindowSizeMsg{Width: 80, Height: 24}
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.isGenerating && m.cancelGenerate != nil {
				m.cancelGenerate()
			}
			return m, tea.Quit

		case "tab":
			if m.state == statePrompting {
				m.viewportFocused = !m.viewportFocused
				if m.viewportFocused {
					m.input.Blur()
				} else {
					m.input.Focus()
				}
				return m, nil
			}

		case "enter":
			if m.state == stateModelSelect {
				if i, ok := m.list.SelectedItem().(item); ok {
					m.selectedModel = i.name
					m.state = statePrompting

					// Return a batch of commands:
					// 1. Clear the screen for a fresh start
					// 2. Send a window size message to initialize the layout
					return m, tea.Batch(
						tea.ClearScreen,
						func() tea.Msg {
							return tea.WindowSizeMsg{
								Width:  m.screenWidth,
								Height: m.screenHeight,
							}
						},
					)
				}
			}
			if m.state == statePrompting {
				if strings.TrimSpace(m.input.Value()) != "" {
					if m.isGenerating && m.cancelGenerate != nil {
						m.cancelGenerate()
					}

					m.currentPrompt = m.input.Value()
					m.input.Reset()
					m.state = stateLoading
					m.isGenerating = true
					m.inProgressResponse = ""

					m.responses = append(m.responses, fmt.Sprintf("Prompt: %s\n\nResponse:\n", m.currentPrompt))

					// Update viewport content with the new prompt
					var content strings.Builder
					for _, resp := range m.responses {
						content.WriteString(resp)
						content.WriteString("\n\n")
					}
					m.viewport.SetContent(content.String())
					m.viewport.GotoBottom()

					return m, startGenerateResponse(m.selectedModel, m.currentPrompt)
				}
			}
		}

	case setCancelFuncMsg:
		m.cancelGenerate = msg.cancel
		return m, nil

	case fetchModelsMsg:
		items := []list.Item{}
		for _, model := range msg.models {
			items = append(items, item{
				name:    model.Name,
				details: fmt.Sprintf("Family: %s, Context: %d", model.Details.Family, model.Details.Context),
			})
		}
		m.list.SetItems(items)
		m.models = msg.models
		return m, nil

	case tokenMsg:
		if msg.done && !m.isGenerating {
			return m, nil
		}

		m.inProgressResponse += msg.token

		if len(m.responses) > 0 {
			// Format the response text with proper wrapping
			responseText := m.inProgressResponse
			if m.screenWidth > 10 {
				responseText = wrapText(responseText, m.screenWidth-10)
			}

			// Update the last response with the new content
			m.responses[len(m.responses)-1] = fmt.Sprintf("Prompt: %s\n\nResponse:\n%s", m.currentPrompt, responseText)

			// Update viewport content with all responses
			var content strings.Builder
			for _, resp := range m.responses {
				content.WriteString(resp)
				content.WriteString("\n\n")
			}

			// Set the content and scroll to bottom
			m.viewport.SetContent(content.String())
			m.viewport.GotoBottom()
		}

		if msg.done {
			m.currentResponse = m.inProgressResponse
			m.isGenerating = false
			m.state = statePrompting
			m.cancelGenerate = nil

			// Make sure we update the viewport one last time
			var content strings.Builder
			for _, resp := range m.responses {
				content.WriteString(resp)
				content.WriteString("\n\n")
			}
			m.viewport.SetContent(content.String())
			m.viewport.GotoBottom()

			return m, nil
		}

		return m, listenForTokens()

	case errorMsg:
		m.err = msg.err
		m.isGenerating = false
		m.state = statePrompting
		m.cancelGenerate = nil
		return m, nil

	case tea.WindowSizeMsg:
		m.screenWidth = msg.Width
		m.screenHeight = msg.Height

		h, v := appLayout(msg.Width, msg.Height, m.state)
		if m.state == stateModelSelect {
			m.list.SetSize(h, v)
			return m, nil
		}

		// For chat view, update the layout
		// Fixed input height (3 lines + borders)
		inputHeight := 5

		// Status bar height
		statusBarHeight := 1

		// Title height (including spacing)
		titleHeight := 3

		// Loading indicator height
		loadingHeight := 0
		if m.state == stateLoading && m.isGenerating {
			loadingHeight = 1
		}

		// Set input width to full width minus margins
		m.input.SetWidth(h - 4)

		// Viewport takes the remaining height
		// Total height minus fixed elements and spacing
		viewportHeight := v - inputHeight - statusBarHeight - titleHeight - loadingHeight - 3
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		m.viewport.Height = viewportHeight
		m.viewport.Width = h - 4

		// Update content wrapping based on new width
		if len(m.responses) > 0 {
			var content strings.Builder
			for i, resp := range m.responses {
				// For the last response that's in progress, rewrap it
				if i == len(m.responses)-1 && len(m.inProgressResponse) > 0 {
					responseText := wrapText(m.inProgressResponse, h-10)
					content.WriteString(fmt.Sprintf("Prompt: %s\n\nResponse:\n%s", m.currentPrompt, responseText))
				} else {
					content.WriteString(resp)
				}
				content.WriteString("\n\n")
			}
			m.viewport.SetContent(content.String())
			m.viewport.GotoBottom()
		} else {
			m.viewport.SetContent("No responses yet. Send a prompt to start.\n\n")
		}

		// Force a redraw to ensure the layout is correct
		return m, tea.ClearScreen

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	switch m.state {
	case stateModelSelect:
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	case statePrompting:
		if m.viewportFocused {
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)

			// These keys should be handled by the viewport even when input is focused
			switch msg := msg.(type) {
			case tea.KeyMsg:
				switch msg.String() {
				case "pgup", "pgdown", "home", "end":
					m.viewport, cmd = m.viewport.Update(msg)
					cmds = append(cmds, cmd)
				}
			}
		}
	case stateLoading:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m mainModel) View() string {
	switch m.state {
	case stateModelSelect:
		return m.list.View()

	case statePrompting, stateLoading:
		// Get terminal dimensions
		width := m.screenWidth
		height := m.screenHeight
		if width <= 0 {
			width = 80 // Default width if not set
		}
		if height <= 0 {
			height = 24 // Default height if not set
		}

		// Create a container for the entire UI
		container := lipgloss.NewStyle().Width(width).Height(height)

		// Title section
		titleView := titleStyle.Render(fmt.Sprintf("Chat with %s", m.selectedModel))
		titleHeight := lipgloss.Height(titleView) + 2 // +2 for spacing

		// Input section (fixed at bottom)
		inputStyle := inputBoxStyle.Copy().Width(width - 4)
		if !m.viewportFocused {
			inputStyle = inputStyle.BorderForeground(lipgloss.Color("#FF5F87"))
		} else {
			inputStyle = inputStyle.BorderForeground(lipgloss.Color("#AFAFAF"))
		}
		inputView := inputStyle.Render(m.input.View())
		inputHeight := lipgloss.Height(inputView)

		// Status bar (fixed at bottom)
		statusText := fmt.Sprintf(" %s | Tab: Toggle focus | Ctrl+C: Exit ", m.selectedModel)
		statusView := statusBarStyle.Copy().Width(width).Render(statusText)
		statusHeight := lipgloss.Height(statusView)

		// Loading indicator
		var loadingView string
		loadingHeight := 0
		if m.state == stateLoading && m.isGenerating {
			loadingView = fmt.Sprintf("  %s Generating...", m.spinner.View())
			loadingHeight = 1
		}

		// Calculate viewport height
		// Available height = total height - (title + input + status + loading + spacing)
		viewportHeight := height - titleHeight - inputHeight - statusHeight - loadingHeight - 2
		if viewportHeight < 5 {
			viewportHeight = 5
		}

		// Set viewport style with calculated height
		viewportStyle := responseStyle.Copy()
		if m.viewportFocused {
			viewportStyle = viewportStyle.BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#FF5F87"))
		}

		// Ensure viewport has the correct height
		m.viewport.Height = viewportHeight
		m.viewport.Width = width - 4

		// Render the viewport
		viewportView := viewportStyle.Render(m.viewport.View())

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

func appLayout(width, height int, state int) (int, int) {
	if state == stateModelSelect {
		return width, height - 4
	}

	// For chat view, use the full width and height
	return width, height
}

func fetchModels() tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(ollamaURL + "/api/tags")
		if err != nil {
			return errorMsg{err}
		}
		defer resp.Body.Close()

		var modelList ModelListResponse
		if err := json.NewDecoder(resp.Body).Decode(&modelList); err != nil {
			return errorMsg{err}
		}

		return fetchModelsMsg{models: modelList.Models}
	}
}

var tokenChan chan tokenMsg

func init() {
	tokenChan = make(chan tokenMsg, 100)
}

type setCancelFuncMsg struct {
	cancel context.CancelFunc
}

func listenForTokens() tea.Cmd {
	return func() tea.Msg {
		return <-tokenChan
	}
}

func startGenerateResponse(model, prompt string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())

		cmds := []tea.Cmd{
			func() tea.Msg {
				return setCancelFuncMsg{cancel: cancel}
			},
		}

		go generateResponseAsync(ctx, model, prompt, func(token string, done bool) {
			tokenChan <- tokenMsg{token: token, done: done}
		})

		cmds = append(cmds, listenForTokens())
		return tea.Batch(cmds...)()
	}
}

func generateResponseAsync(ctx context.Context, model, prompt string, callback func(string, bool)) {
	var mu sync.Mutex

	reqBody, err := json.Marshal(GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: true,
	})

	if err != nil {
		callback("", true)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "POST", ollamaURL+"/api/generate", bytes.NewBuffer(reqBody))
	if err != nil {
		callback("", true)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		callback("", true)
		return
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			callback("", true)
			return
		default:
			line := scanner.Text()
			if line == "" {
				continue
			}

			var genResp GenerateResponse
			if err := json.Unmarshal([]byte(line), &genResp); err != nil {
				continue
			}

			mu.Lock()
			if genResp.Response != "" {
				callback(genResp.Response, genResp.Done)
			}

			if genResp.Done {
				callback("", true)
				mu.Unlock()
				return
			}
			mu.Unlock()
		}
	}

	if err := scanner.Err(); err != nil {
		callback("", true)
	}

	callback("", true)
}

func main() {
	if tokenChan == nil {
		tokenChan = make(chan tokenMsg, 100)
	}

	// Use the full terminal screen and enable mouse support
	p := tea.NewProgram(
		initialModel(),
		tea.WithAltScreen(),       // Use the alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	// Run the program without trying to send messages before it starts
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error initializing application: %v\n", err)
		os.Exit(1)
	}
}
