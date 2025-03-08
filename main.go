package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

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
type generatedResponseMsg struct{ response string }

const (
	stateModelSelect = iota
	statePrompting
	stateLoading
)

type mainModel struct {
	state           int
	list            list.Model
	models          []Model
	selectedModel   string
	input           textarea.Model
	viewport        viewport.Model
	spinner         spinner.Model
	responses       []string
	currentPrompt   string
	currentResponse string
	err             error
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
		state:    stateModelSelect,
		list:     l,
		spinner:  s,
		input:    ta,
		viewport: vp,
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
			return m, tea.Quit

		case "enter":
			if m.state == stateModelSelect {
				if i, ok := m.list.SelectedItem().(item); ok {
					m.selectedModel = i.name
					m.state = statePrompting
					return m, nil
				}
			} else if m.state == statePrompting {
				if strings.TrimSpace(m.input.Value()) != "" {
					m.currentPrompt = m.input.Value()
					m.input.Reset()
					m.state = stateLoading
					return m, generateResponse(m.selectedModel, m.currentPrompt)
				}
			}
		}

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

	case generatedResponseMsg:
		m.currentResponse = msg.response
		newEntry := fmt.Sprintf("Prompt: %s\n\nResponse:\n%s\n\n---\n", m.currentPrompt, m.currentResponse)
		m.responses = append(m.responses, newEntry)

		// Imprimir para depuración
		fmt.Printf("Recibida respuesta de longitud: %d\n", len(m.currentResponse))
		fmt.Printf("Nueva entrada: %s\n", newEntry)

		// Actualizar contenido del viewport
		content := strings.Join(m.responses, "\n")

		// Forzar la actualización del viewport
		m.viewport = viewport.New(m.viewport.Width, m.viewport.Height)
		m.viewport.Style = responseStyle
		m.viewport.SetContent(content)
		m.viewport.GotoBottom()

		// Debug
		fmt.Printf("Viewport content length: %d\n", len(content))

		m.state = statePrompting
		return m, nil

	case errorMsg:
		m.err = msg.err
		m.state = statePrompting
		return m, nil

	case tea.WindowSizeMsg:
		h, v := appLayout(msg.Width, msg.Height, m.state)
		if m.state == stateModelSelect {
			m.list.SetSize(h, v)
		} else {
			m.input.SetWidth(h)

			// Ajustar mejor el viewport
			m.viewport.Width = h - 4 // Un poco más estrecho que el ancho total

			// Asegurarnos de que el viewport tenga altura suficiente
			viewportHeight := v - 12 // Reservar espacio para título, input y barra de estado
			if viewportHeight < 1 {
				viewportHeight = 1
			}
			m.viewport.Height = viewportHeight

			fmt.Printf("Viewport dimensions: %dx%d\n", m.viewport.Width, m.viewport.Height)
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

// Alternativa: en lugar de usar viewport, simplemente mostramos la última respuesta
func (m mainModel) View() string {
	switch m.state {
	case stateModelSelect:
		return m.list.View()

	case statePrompting:
		var sb strings.Builder
		sb.WriteString(titleStyle.Render(fmt.Sprintf("Chat with %s", m.selectedModel)))
		sb.WriteString("\n\n")

		// Simplemente mostrar la última respuesta si existe
		if len(m.responses) > 0 {
			lastResponse := m.responses[len(m.responses)-1]
			sb.WriteString(responseStyle.Render(lastResponse))
		} else {
			sb.WriteString(responseStyle.Render("No responses yet. Send a prompt to start."))
		}

		sb.WriteString("\n\n")
		sb.WriteString(m.input.View())
		sb.WriteString("\n\n")
		sb.WriteString(statusBarStyle.Render(fmt.Sprintf(" %s | Ctrl+C to exit ", m.selectedModel)))
		return sb.String()

	case stateLoading:
		return fmt.Sprintf("\n\n  %s Model is thinking...\n\n", m.spinner.View())

	default:
		return "Unknown state"
	}
}

func appLayout(width, height int, state int) (int, int) {
	if state == stateModelSelect {
		return width, height - 4
	}
	// Usar casi toda la altura disponible
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

func generateResponse(model, prompt string) tea.Cmd {
	return func() tea.Msg {
		fmt.Printf("Enviando prompt a %s: %s\n", model, prompt)

		reqBody, err := json.Marshal(GenerateRequest{
			Model:  model,
			Prompt: prompt,
		})
		if err != nil {
			return errorMsg{err}
		}

		resp, err := http.Post(ollamaURL+"/api/generate", "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			return errorMsg{err}
		}
		defer resp.Body.Close()

		fmt.Printf("Status: %s\n", resp.Status)

		var fullResponse strings.Builder
		linesProcessed := 0

		scanner := bufio.NewScanner(resp.Body)
		// Aumentar el buffer del scanner para manejar líneas más largas
		const maxCapacity = 1024 * 1024
		buf := make([]byte, maxCapacity)
		scanner.Buffer(buf, maxCapacity)

		for scanner.Scan() {
			line := scanner.Text()
			linesProcessed++

			if line == "" {
				continue
			}

			var genResp GenerateResponse
			if err := json.Unmarshal([]byte(line), &genResp); err != nil {
				fmt.Printf("Error en línea %d: %v\n", linesProcessed, err)
				continue
			}

			fullResponse.WriteString(genResp.Response)
			fmt.Printf("Chunk %d: %s\n", linesProcessed, genResp.Response)

			if genResp.Done {
				break
			}
		}

		if err := scanner.Err(); err != nil {
			return errorMsg{fmt.Errorf("error leyendo respuesta: %v", err)}
		}

		result := fullResponse.String()
		fmt.Printf("Respuesta completa (%d bytes): %s\n", len(result), result)

		if result == "" {
			return errorMsg{fmt.Errorf("respuesta vacía recibida de Ollama")}
		}

		return generatedResponseMsg{response: result}
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error initializing application: %v\n", err)
		os.Exit(1)
	}
}
