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

// GenerateRequest represents a request to generate text from a model
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// GenerateResponse represents a response from the Ollama API for text generation
type GenerateResponse struct {
	Model     string `json:"model"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
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
