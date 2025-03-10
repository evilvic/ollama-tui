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

	// OpenAI conversation history
	openAIMessages []models.ChatMessage
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
		BaseURL:        baseURL,
		APIKey:         apiKey,
		client:         &http.Client{},
		openAIMessages: []models.ChatMessage{},
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
	c.openAIMessages = nil
}

// HasContext returns true if the client has a conversation context
func (c *Client) HasContext() bool {
	return (c.context != nil && len(c.context) > 0) || (c.openAIMessages != nil && len(c.openAIMessages) > 0)
}

// GenerateResponse generates a response from a model
func (c *Client) GenerateResponse(ctx context.Context, model, prompt string, callback func(string, bool)) error {
	// Create a log file for debugging
	logFile, err := os.OpenFile("api_response.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		defer logFile.Close()
		logger := log.New(logFile, "", log.LstdFlags)
		logger.Printf("Generating response for model: %s, prompt: %s\n", model, prompt)
		logger.Printf("Using provider: %s\n", c.BaseURL)
	}

	// Handle OpenAI API
	if c.BaseURL == DefaultOpenAIURL {
		return c.generateOpenAIResponse(ctx, model, prompt, callback)
	}

	// Handle Ollama API (existing implementation)
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

// generateOpenAIResponse generates a response using the OpenAI API
func (c *Client) generateOpenAIResponse(ctx context.Context, model, prompt string, callback func(string, bool)) error {
	// Create a log file for debugging
	logFile, err := os.OpenFile("openai_chat.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		defer logFile.Close()
		logger := log.New(logFile, "", log.LstdFlags)
		logger.Printf("Generating OpenAI response for model: %s, prompt: %s\n", model, prompt)
		logger.Printf("Conversation history: %d messages\n", len(c.openAIMessages))
	}

	// Create a logger function for convenience
	logMessage := func(format string, args ...interface{}) {
		if logFile != nil {
			logger := log.New(logFile, "", log.LstdFlags)
			logger.Printf(format, args...)
		}
	}

	// Create messages array
	var messages []models.ChatMessage

	// If we have conversation history, use it
	if c.openAIMessages != nil && len(c.openAIMessages) > 0 {
		messages = append(messages, c.openAIMessages...)
	}

	// Add the new user message
	userMessage := models.ChatMessage{
		Role:    "user",
		Content: prompt,
	}
	messages = append(messages, userMessage)

	// Create the request
	chatReq := models.OpenAIChatRequest{
		Model:       model,
		Messages:    messages,
		Stream:      true,
		Temperature: 0.7,
	}

	// Marshal the request to JSON
	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		logMessage("Error marshaling request: %v", err)
		return fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	logMessage("Request body: %s", string(reqBody))

	// Create the HTTP request - Fix the URL by using the correct path
	chatCompletionsURL := c.BaseURL + "/chat/completions"
	logMessage("Using URL: %s", chatCompletionsURL)

	req, err := http.NewRequestWithContext(ctx, "POST", chatCompletionsURL, bytes.NewBuffer(reqBody))
	if err != nil {
		logMessage("Error creating request: %v", err)
		return fmt.Errorf("failed to create OpenAI request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	logMessage("Sending request to %s with API key length: %d", chatCompletionsURL, len(c.APIKey))

	// Send the request
	resp, err := c.client.Do(req)
	if err != nil {
		logMessage("Error sending request: %v", err)
		return fmt.Errorf("failed to send OpenAI request: %w", err)
	}
	defer resp.Body.Close()

	logMessage("Response status code: %d", resp.StatusCode)

	// Check for error status codes
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logMessage("Error response body: %s", string(bodyBytes))
		return fmt.Errorf("OpenAI API returned status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Process the streaming response
	reader := bufio.NewReader(resp.Body)

	// Store the assistant's response
	var assistantResponse strings.Builder

	logMessage("Starting to read response stream")

	for {
		select {
		case <-ctx.Done():
			logMessage("Context cancelled")
			callback("", true)
			return nil
		default:
			// Read a line from the response
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					logMessage("End of response stream (EOF)")
					// Add the assistant's message to the conversation history
					if assistantResponse.Len() > 0 {
						c.openAIMessages = append(c.openAIMessages, userMessage)
						c.openAIMessages = append(c.openAIMessages, models.ChatMessage{
							Role:    "assistant",
							Content: assistantResponse.String(),
						})
						logMessage("Added conversation history. Total messages: %d", len(c.openAIMessages))
					} else {
						logMessage("No assistant response received")
					}
					callback("", true)
					return nil
				}
				logMessage("Error reading response: %v", err)
				return fmt.Errorf("error reading OpenAI response: %w", err)
			}

			logMessage("Received line: %s", line)

			// Skip empty lines and "data: [DONE]"
			line = strings.TrimSpace(line)
			if line == "" {
				logMessage("Empty line, skipping")
				continue
			}

			if line == "data: [DONE]" {
				logMessage("Received DONE signal")
				// If we're done, add the messages to the conversation history
				if assistantResponse.Len() > 0 {
					c.openAIMessages = append(c.openAIMessages, userMessage)
					c.openAIMessages = append(c.openAIMessages, models.ChatMessage{
						Role:    "assistant",
						Content: assistantResponse.String(),
					})
					logMessage("Added conversation history. Total messages: %d", len(c.openAIMessages))
				} else {
					logMessage("No assistant response received at DONE signal")
				}
				callback("", true)
				return nil
			}

			// Remove "data: " prefix
			if strings.HasPrefix(line, "data: ") {
				line = strings.TrimPrefix(line, "data: ")
				logMessage("Trimmed data prefix: %s", line)
			} else {
				logMessage("Line doesn't have data prefix, skipping: %s", line)
				continue
			}

			// Parse the JSON
			var streamResp models.OpenAIChatStreamResponse
			if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
				logMessage("Error parsing JSON: %v, line: %s", err, line)
				continue
			}

			logMessage("Parsed stream response: %+v", streamResp)

			// Process the choices
			if len(streamResp.Choices) > 0 {
				choice := streamResp.Choices[0]
				logMessage("Processing choice: %+v", choice)

				// Check if this is the end of the response
				if choice.FinishReason != nil {
					logMessage("Finish reason: %v", *choice.FinishReason)
					// Add the assistant's message to the conversation history
					if assistantResponse.Len() > 0 {
						c.openAIMessages = append(c.openAIMessages, userMessage)
						c.openAIMessages = append(c.openAIMessages, models.ChatMessage{
							Role:    "assistant",
							Content: assistantResponse.String(),
						})
						logMessage("Added conversation history. Total messages: %d", len(c.openAIMessages))
					} else {
						logMessage("No assistant response received at finish")
					}
					callback("", true)
					return nil
				}

				// Send the content
				if choice.Delta.Content != "" {
					logMessage("Delta content: %s", choice.Delta.Content)
					assistantResponse.WriteString(choice.Delta.Content)
					callback(choice.Delta.Content, false)
				} else if choice.Delta.Role != "" {
					logMessage("Delta role: %s", choice.Delta.Role)
				} else {
					logMessage("Empty delta")
				}
			} else {
				logMessage("No choices in response")
			}
		}
	}
}
