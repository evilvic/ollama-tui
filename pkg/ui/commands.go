package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/evilvic/ollama-tui/pkg/api"
)

var (
	// TokenChan is a channel for token messages
	TokenChan chan TokenMsg
	// APIClient is the API client
	APIClient *api.Client
)

// Initialize the token channel
func init() {
	TokenChan = make(chan TokenMsg, 100)
	APIClient = api.NewClient("")
}

// FetchModelsCmd fetches the list of available models for the specified provider
func FetchModelsCmd(provider string) tea.Cmd {
	return func() tea.Msg {
		// Create a new API client for the selected provider
		APIClient = api.NewClient(provider)

		models, err := APIClient.FetchModels()
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return FetchModelsMsg{Models: models}
	}
}

// ListenForTokensCmd listens for token messages
func ListenForTokensCmd() tea.Cmd {
	return func() tea.Msg {
		return <-TokenChan
	}
}

// StartGenerateResponseCmd starts generating a response
func StartGenerateResponseCmd(model, prompt string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())

		cmds := []tea.Cmd{
			func() tea.Msg {
				return SetCancelFuncMsg{Cancel: cancel}
			},
		}

		go generateResponseAsync(ctx, model, prompt, func(token string, done bool) {
			TokenChan <- TokenMsg{Token: token, Done: done}
		})

		cmds = append(cmds, ListenForTokensCmd())
		return tea.Batch(cmds...)()
	}
}

// generateResponseAsync generates a response asynchronously
func generateResponseAsync(ctx context.Context, model, prompt string, callback func(string, bool)) {
	err := APIClient.GenerateResponse(ctx, model, prompt, callback)
	if err != nil {
		callback("", true)
	}
}
