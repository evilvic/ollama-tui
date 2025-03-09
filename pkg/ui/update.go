package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/evilvic/ollama-tui/pkg/models"
	"github.com/evilvic/ollama-tui/pkg/utils"
)

// Update updates the UI model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.IsGenerating && m.CancelGenerate != nil {
				m.CancelGenerate()
			}
			return m, tea.Quit

		case "tab":
			if m.State == StatePrompting {
				m.ViewportFocused = !m.ViewportFocused
				if m.ViewportFocused {
					m.Input.Blur()
				} else {
					m.Input.Focus()
				}
				return m, nil
			}

		case "ctrl+n":
			// Clear conversation context and start a new chat
			if m.State == StatePrompting {
				APIClient.ClearContext()
				return m, tea.Batch(
					tea.ClearScreen,
					func() tea.Msg {
						return tea.WindowSizeMsg{
							Width:  m.ScreenWidth,
							Height: m.ScreenHeight,
						}
					},
				)
			}

		case "enter":
			if m.State == StateProviderSelect {
				if i, ok := m.ProviderList.SelectedItem().(models.ListItem); ok {
					m.SelectedProvider = i.Name
					m.State = StateModelSelect

					// Return a batch of commands:
					// 1. Clear the screen for a fresh start
					// 2. Send a window size message to initialize the layout
					// 3. Fetch models for the selected provider
					return m, tea.Batch(
						tea.ClearScreen,
						func() tea.Msg {
							return tea.WindowSizeMsg{
								Width:  m.ScreenWidth,
								Height: m.ScreenHeight,
							}
						},
						FetchModelsCmd,
					)
				}
			}

			if m.State == StateModelSelect {
				if i, ok := m.List.SelectedItem().(models.ListItem); ok {
					m.SelectedModel = i.Name
					m.State = StatePrompting

					// Return a batch of commands:
					// 1. Clear the screen for a fresh start
					// 2. Send a window size message to initialize the layout
					return m, tea.Batch(
						tea.ClearScreen,
						func() tea.Msg {
							return tea.WindowSizeMsg{
								Width:  m.ScreenWidth,
								Height: m.ScreenHeight,
							}
						},
					)
				}
			}
			if m.State == StatePrompting {
				if strings.TrimSpace(m.Input.Value()) != "" {
					if m.IsGenerating && m.CancelGenerate != nil {
						m.CancelGenerate()
					}

					m.CurrentPrompt = m.Input.Value()
					m.Input.Reset()
					m.State = StateLoading
					m.IsGenerating = true
					m.InProgressResponse = ""

					m.Responses = append(m.Responses, fmt.Sprintf("Prompt: %s\n\nResponse:\n", m.CurrentPrompt))

					// Update viewport content with the new prompt
					m.UpdateViewportContent()

					return m, StartGenerateResponseCmd(m.SelectedModel, m.CurrentPrompt)
				}
			}
		}

	case SetCancelFuncMsg:
		m.CancelGenerate = msg.Cancel
		return m, nil

	case FetchModelsMsg:
		items := []list.Item{}
		for _, model := range msg.Models {
			items = append(items, models.ListItem{
				Name:    model.Name,
				Details: fmt.Sprintf("Family: %s, Context: %d", model.Details.Family, model.Details.Context),
			})
		}
		m.List.SetItems(items)
		m.Models = msg.Models
		return m, nil

	case TokenMsg:
		if msg.Done && !m.IsGenerating {
			return m, nil
		}

		m.InProgressResponse += msg.Token

		// Update the response with the new token
		m.UpdateResponse(m.CurrentPrompt, m.InProgressResponse)

		if msg.Done {
			m.CurrentResponse = m.InProgressResponse
			m.IsGenerating = false
			m.State = StatePrompting
			m.CancelGenerate = nil

			// Make sure we update the viewport one last time
			m.UpdateViewportContent()

			return m, nil
		}

		return m, ListenForTokensCmd()

	case ErrorMsg:
		m.Err = msg.Err
		m.IsGenerating = false
		m.State = StatePrompting
		m.CancelGenerate = nil
		return m, nil

	case tea.WindowSizeMsg:
		m.ScreenWidth = msg.Width
		m.ScreenHeight = msg.Height

		h, v := AppLayout(msg.Width, msg.Height, m.State)
		if m.State == StateProviderSelect {
			m.ProviderList.SetSize(h, v)
			return m, nil
		} else if m.State == StateModelSelect {
			m.List.SetSize(h, v)
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
		if m.State == StateLoading && m.IsGenerating {
			loadingHeight = 1
		}

		// Set input width to full width minus margins
		m.Input.SetWidth(h - 4)

		// Viewport takes the remaining height
		// Total height minus fixed elements and spacing
		viewportHeight := v - inputHeight - statusBarHeight - titleHeight - loadingHeight - 3
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		m.Viewport.Height = viewportHeight
		m.Viewport.Width = h - 4

		// Update content wrapping based on new width
		if len(m.Responses) > 0 {
			var content strings.Builder
			for i, resp := range m.Responses {
				// For the last response that's in progress, rewrap it
				if i == len(m.Responses)-1 && len(m.InProgressResponse) > 0 {
					responseText := utils.WrapText(m.InProgressResponse, h-10)
					content.WriteString(fmt.Sprintf("Prompt: %s\n\nResponse:\n%s", m.CurrentPrompt, responseText))
				} else {
					content.WriteString(resp)
				}
				content.WriteString("\n\n")
			}
			m.Viewport.SetContent(content.String())
			m.Viewport.GotoBottom()
		} else {
			m.Viewport.SetContent("No responses yet. Send a prompt to start.\n\n")
		}

		// Force a redraw to ensure the layout is correct
		return m, tea.ClearScreen

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd
	}

	// Handle other messages
	switch m.State {
	case StateProviderSelect:
		var cmd tea.Cmd
		m.ProviderList, cmd = m.ProviderList.Update(msg)
		cmds = append(cmds, cmd)

	case StateModelSelect:
		var cmd tea.Cmd
		m.List, cmd = m.List.Update(msg)
		cmds = append(cmds, cmd)

	case StatePrompting:
		if !m.ViewportFocused {
			var cmd tea.Cmd
			m.Input, cmd = m.Input.Update(msg)
			cmds = append(cmds, cmd)

			// These keys should be handled by the viewport even when input is focused
			switch msg := msg.(type) {
			case tea.KeyMsg:
				switch msg.String() {
				case "pgup", "pgdown", "home", "end":
					m.Viewport, cmd = m.Viewport.Update(msg)
					cmds = append(cmds, cmd)
				}
			}
		} else {
			var cmd tea.Cmd
			m.Viewport, cmd = m.Viewport.Update(msg)
			cmds = append(cmds, cmd)
		}

	case StateLoading:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}
