package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/evilvic/ollama-tui/pkg/models"
)

const (
	DefaultOllamaURL = "http://localhost:11434"
	DefaultOpenAIURL = "https://api.openai.com/v1"
)

type Client struct {
	BaseURL string
	APIKey  string
	client  *http.Client
	context []int
}

func NewClient(provider string, apiKey string) *Client {
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
		APIKey:  apiKey,
		client:  &http.Client{},
	}
}

func (c *Client) FetchModels() ([]models.Model, error) {
	// Create a log file
	logFile, err := os.OpenFile("openai_api.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return getHardcodedOpenAIModels(), nil
	}
	defer logFile.Close()

	logger := log.New(logFile, "", log.LstdFlags)

	if c.BaseURL == DefaultOpenAIURL {
		logger.Println("Fetching OpenAI models from API...")

		// Create a request to the OpenAI API
		req, err := http.NewRequest("GET", c.BaseURL+"/models", nil)
		if err != nil {
			logger.Printf("Error creating request: %v\n", err)
			return getHardcodedOpenAIModels(), nil
		}

		// Add the API key to the request header
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")

		// Log a masked version of the API key for debugging
		maskedKey := "****"
		if len(c.APIKey) > 4 {
			maskedKey = c.APIKey[:4] + "..." + c.APIKey[len(c.APIKey)-4:]
		}
		logger.Printf("Sending request to %s with API key: %s (length: %d)\n",
			c.BaseURL+"/models", maskedKey, len(c.APIKey))

		// Send the request
		resp, err := c.client.Do(req)
		if err != nil {
			logger.Printf("Error sending request: %v\n", err)
			return getHardcodedOpenAIModels(), nil
		}
		defer resp.Body.Close()

		logger.Printf("Response status code: %d\n", resp.StatusCode)

		// Check for error status codes
		if resp.StatusCode != http.StatusOK {
			// Read the response body to get error details
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Printf("Error reading error response body: %v\n", err)
				return getHardcodedOpenAIModels(), nil
			}

			logger.Printf("Error response body: %s\n", string(bodyBytes))

			if resp.StatusCode == 401 {
				logger.Println("Authentication error: The API key is invalid or missing.")
				logger.Printf("API Key format check: starts with 'sk-'? %v, length > 20? %v\n",
					strings.HasPrefix(c.APIKey, "sk-"), len(c.APIKey) > 20)
			}

			return getHardcodedOpenAIModels(), nil
		}

		// Read the response body for debugging
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Printf("Error reading response body: %v\n", err)
			return getHardcodedOpenAIModels(), nil
		}

		// Log the response body
		logger.Printf("Response body: %s\n", string(bodyBytes))

		// Create a new reader from the bytes for JSON decoding
		respBodyReader := bytes.NewReader(bodyBytes)

		// Decode the response
		var openAIResp models.OpenAIModelResponse
		if err := json.NewDecoder(respBodyReader).Decode(&openAIResp); err != nil {
			logger.Printf("Error decoding response: %v\n", err)
			return getHardcodedOpenAIModels(), nil
		}

		logger.Printf("Decoded %d models from API\n", len(openAIResp.Data))

		// Convert OpenAI models to our internal model format
		result := make([]models.Model, 0, len(openAIResp.Data))
		for _, m := range openAIResp.Data {
			logger.Printf("Processing model: %s\n", m.ID)
			model := models.Model{
				Name: m.ID,
				Details: struct {
					Family  string `json:"family"`
					Format  string `json:"format"`
					Context int    `json:"context"`
				}{
					Family:  "OpenAI",
					Format:  "Chat",
					Context: 4096, // Default context size
				},
			}
			result = append(result, model)
		}

		// Ensure we have at least some models
		if len(result) == 0 {
			logger.Println("No models found in API response, using hardcoded models")
			return getHardcodedOpenAIModels(), nil
		}

		logger.Printf("Returning %d models from API\n", len(result))
		return result, nil
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

// getHardcodedOpenAIModels returns a list of hardcoded OpenAI models
func getHardcodedOpenAIModels() []models.Model {
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
	}
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
