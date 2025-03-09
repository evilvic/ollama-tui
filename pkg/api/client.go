package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/evilvic/ollama-tui/pkg/models"
)

const (
	// DefaultOllamaURL is the default URL for the Ollama API
	DefaultOllamaURL = "http://localhost:11434"
	DefaultOpenAIURL = "https://api.openai.com/v1"
)

// Client represents an Ollama API client
type Client struct {
	BaseURL string
	client  *http.Client
	context []int
}

// NewClient creates a new Ollama API client
func NewClient(provider string) *Client {
	var baseURL string
	switch provider {
	case "openai":
		baseURL = DefaultOpenAIURL
	case "ollama":
		baseURL = DefaultOllamaURL
	default:
		baseURL = DefaultOllamaURL
	}

	return &Client{
		BaseURL: baseURL,
		client:  &http.Client{},
	}
}

// FetchModels fetches the list of available models based on the provider
func (c *Client) FetchModels() ([]models.Model, error) {
	// Check if the URL contains "openai" to determine if it's OpenAI
	if c.BaseURL == DefaultOpenAIURL {
		// For OpenAI, return a predefined list of models
		// In a real implementation, you would call the OpenAI API to get the models
		return []models.Model{
			{
				Name: "gpt-3.5-turbo",
				Details: struct {
					Family  string `json:"family"`
					Format  string `json:"format"`
					Context int    `json:"context"`
				}{
					Family:  "GPT-3.5",
					Format:  "Chat",
					Context: 4096,
				},
			},
			{
				Name: "gpt-4",
				Details: struct {
					Family  string `json:"family"`
					Format  string `json:"format"`
					Context int    `json:"context"`
				}{
					Family:  "GPT-4",
					Format:  "Chat",
					Context: 8192,
				},
			},
			{
				Name: "gpt-4-turbo",
				Details: struct {
					Family  string `json:"family"`
					Format  string `json:"format"`
					Context int    `json:"context"`
				}{
					Family:  "GPT-4",
					Format:  "Chat",
					Context: 128000,
				},
			},
		}, nil
	}

	// For Ollama, use the existing implementation
	resp, err := c.client.Get(c.BaseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	var modelList models.ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelList); err != nil {
		return nil, fmt.Errorf("failed to decode model list: %w", err)
	}

	return modelList.Models, nil
}

// ClearContext clears the conversation context
func (c *Client) ClearContext() {
	c.context = nil
}

// HasContext returns true if the client has a conversation context
func (c *Client) HasContext() bool {
	return c.context != nil && len(c.context) > 0
}

// GenerateResponse generates a response from a model
func (c *Client) GenerateResponse(ctx context.Context, model, prompt string, callback func(string, bool)) error {
	// Create the request with context if available
	reqBody, err := json.Marshal(models.GenerateRequest{
		Model:   model,
		Prompt:  prompt,
		Stream:  true,
		Context: c.context,
	})

	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/generate", bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var mu sync.Mutex
	scanner := bufio.NewScanner(resp.Body)
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			callback("", true)
			return nil
		default:
			line := scanner.Text()
			if line == "" {
				continue
			}

			var genResp models.GenerateResponse
			if err := json.Unmarshal([]byte(line), &genResp); err != nil {
				continue
			}

			mu.Lock()
			if genResp.Response != "" {
				callback(genResp.Response, genResp.Done)
			}

			// Save the context for future requests
			if genResp.Context != nil && len(genResp.Context) > 0 {
				c.context = genResp.Context
			}

			if genResp.Done {
				callback("", true)
				mu.Unlock()
				return nil
			}
			mu.Unlock()
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	callback("", true)
	return nil
}
