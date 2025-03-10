package models

// Model represents an Ollama model
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

// ModelListResponse represents the response from the Ollama API for listing models
type ModelListResponse struct {
	Models []Model `json:"models"`
}

// OpenAIModelResponse represents the response from the OpenAI API for listing models
type OpenAIModelResponse struct {
	Data   []OpenAIModel `json:"data"`
	Object string        `json:"object"`
}

// OpenAIModel represents a model from the OpenAI API
type OpenAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// OpenAIChatRequest represents a request to the OpenAI chat completions API
type OpenAIChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// OpenAIChatResponse represents a response from the OpenAI chat completions API
type OpenAIChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

// OpenAIChatStreamResponse represents a streaming response from the OpenAI chat completions API
type OpenAIChatStreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

// Choice represents a choice in an OpenAI chat completion response
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// StreamChoice represents a choice in an OpenAI streaming chat completion response
type StreamChoice struct {
	Index        int     `json:"index"`
	Delta        Delta   `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// Delta represents the delta in a streaming response
type Delta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// GenerateRequest represents a request to generate text from a model
type GenerateRequest struct {
	Model    string        `json:"model"`
	Prompt   string        `json:"prompt"`
	Stream   bool          `json:"stream"`
	Context  []int         `json:"context,omitempty"`
	Messages []ChatMessage `json:"messages,omitempty"`
}

// ChatMessage represents a message in a chat conversation
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// GenerateResponse represents a response from the Ollama API for text generation
type GenerateResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
	Context   []int  `json:"context,omitempty"`
}

// ListItem represents an item in the model selection list
type ListItem struct {
	Name    string
	Details string
}

// Title returns the name of the model for the list item
func (i ListItem) Title() string { return i.Name }

// Description returns the details of the model for the list item
func (i ListItem) Description() string { return i.Details }

// FilterValue returns the value to use for filtering the list
func (i ListItem) FilterValue() string { return i.Name }
