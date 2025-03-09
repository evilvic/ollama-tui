# Ollama TUI

A terminal user interface (TUI) for interacting with Ollama models, built with Go and the Charm libraries.

![Ollama TUI Screenshot](https://github.com/evilvic/ollama-tui/raw/main/screenshots/ollama-tui-demo.png)

## Features

- Browse and select from available Ollama models
- Interactive chat interface with selected models
- Real-time streaming responses
- Conversation memory (maintains context between prompts)
- Text wrapping for better readability
- Fixed input box at the bottom for a more familiar chat experience
- Keyboard navigation with focus switching between chat history and input
- Cancel generation with Ctrl+C

## Requirements

- Go 1.24 or later
- [Ollama](https://ollama.ai/) running locally on port 11434

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/evilvic/ollama-tui.git
cd ollama-tui

# Build the application
go build -o ollama-tui

# Run the application
./ollama-tui
```

### Using Go Install

```bash
go install github.com/evilvic/ollama-tui@latest
```

## Usage

1. Make sure Ollama is running locally (`ollama serve`)
2. Launch the application: `./ollama-tui`
3. Select a model from the list using arrow keys and press Enter
4. Type your prompt and press Enter to generate a response
5. The model will remember previous interactions automatically
6. Press Ctrl+N to start a new conversation (clears context)
7. Press Ctrl+C to exit the application

## Keyboard Shortcuts

- **Arrow keys**: Navigate through the model list or scroll through responses
- **Tab**: Toggle focus between chat history and input box
- **Enter**: Select a model or send a prompt
- **Ctrl+N**: Start a new conversation (clears context)
- **Page Up/Down**: Scroll through chat history
- **Home/End**: Jump to the beginning/end of chat history
- **Ctrl+C**: Cancel generation or exit the application
- **Esc**: Exit the application

## Dependencies

- [Bubble Tea](https://github.com/charmbracelet/bubbletea): Terminal UI framework
- [Bubbles](https://github.com/charmbracelet/bubbles): UI components for Bubble Tea
- [Lip Gloss](https://github.com/charmbracelet/lipgloss): Style definitions for terminal applications

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. 