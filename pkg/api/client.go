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
)

// Client represents an Ollama API client
type Client struct {
	BaseURL string
	client  *http.Client
}

// NewClient creates a new Ollama API client
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultOllamaURL
	}

	return &Client{
		BaseURL: baseURL,
		client:  &http.Client{},
	}
}

// FetchModels fetches the list of available models from the Ollama API
func (c *Client) FetchModels() ([]models.Model, error) {
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

// GenerateResponse generates a response from a model
func (c *Client) GenerateResponse(ctx context.Context, model, prompt string, callback func(string, bool)) error {
	reqBody, err := json.Marshal(models.GenerateRequest{
		Model:  model,
		Prompt: prompt,
		Stream: true,
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
