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
)

const (
	ollamaURL = "http://localhost:11434"
)

var (
	titleStyle     = lipgloss.NewStyle().MarginLeft(2).Bold(true).Foreground(lipgloss.Color("#FF5F87"))
	responseStyle  = lipgloss.NewStyle().MarginLeft(2).MarginRight(2)
	statusBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AFAFAF")).Reverse(true)
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
	cancelGenerate     context.CancelFunc
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

	vp := viewport.New(0, 0)
	vp.Style = responseStyle
	vp.SetContent("Respuestas aparecerán aquí.\n\n")

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
	}
}

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(
		fetchModels(),
		m.spinner.Tick,
	)
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

		case "enter":
			if m.state == stateModelSelect {
				if i, ok := m.list.SelectedItem().(item); ok {
					m.selectedModel = i.name
					m.state = statePrompting
					return m, nil
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
			responseText := m.inProgressResponse
			if m.screenWidth > 10 {
				responseText = wrapText(responseText, m.screenWidth-10)
			}

			m.responses[len(m.responses)-1] = fmt.Sprintf("Prompt: %s\n\nResponse:\n%s", m.currentPrompt, responseText)
		}

		if msg.done {
			m.currentResponse = m.inProgressResponse
			m.isGenerating = false
			m.state = statePrompting
			m.cancelGenerate = nil
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

		h, v := appLayout(msg.Width, msg.Height, m.state)
		if m.state == stateModelSelect {
			m.list.SetSize(h, v)
		} else {
			m.input.SetWidth(h)

			m.viewport.Width = h - 4

			viewportHeight := v - 12
			if viewportHeight < 1 {
				viewportHeight = 1
			}
			m.viewport.Height = viewportHeight

			if len(m.responses) > 0 && len(m.inProgressResponse) > 0 {
				responseText := wrapText(m.inProgressResponse, msg.Width-10)
				m.responses[len(m.responses)-1] = fmt.Sprintf("Prompt: %s\n\nResponse:\n%s", m.currentPrompt, responseText)
			}
		}
		return m, nil

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
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
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
		var sb strings.Builder
		sb.WriteString(titleStyle.Render(fmt.Sprintf("Chat with %s", m.selectedModel)))
		sb.WriteString("\n\n")

		if len(m.responses) > 0 {
			for _, resp := range m.responses {
				sb.WriteString(responseStyle.Render(resp))
				sb.WriteString("\n\n")
			}
		} else {
			sb.WriteString(responseStyle.Render("No responses yet. Send a prompt to start."))
			sb.WriteString("\n\n")
		}

		if m.state == stateLoading && m.isGenerating {
			sb.WriteString(fmt.Sprintf("  %s Generating...\n\n", m.spinner.View()))
		}

		sb.WriteString(m.input.View())
		sb.WriteString("\n\n")
		sb.WriteString(statusBarStyle.Render(fmt.Sprintf(" %s | Ctrl+C to exit ", m.selectedModel)))
		return sb.String()

	default:
		return "Unknown state"
	}
}

func appLayout(width, height int, state int) (int, int) {
	if state == stateModelSelect {
		return width, height - 4
	}
	return width, height - 2
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

	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error initializing application: %v\n", err)
		os.Exit(1)
	}
}
