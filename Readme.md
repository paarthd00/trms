# Trm Search - Terminal Search & Local AI

A beautiful terminal application that combines web search, command execution, and local AI chat using Ollama - all without any API keys!

![dashboard](images/dashboard.png)

## Features

- 🔍 **Web Search**: Search the web using DuckDuckGo (no API key required)
- 🤖 **Local AI Chat**: Chat with various AI models through Ollama
- 🚀 **Auto-Install**: Ollama installs automatically when needed
- 📦 **Model Management**: Easy model selection and pulling
- 💻 **Command Execution**: Run shell commands directly
- 🎨 **Beautiful TUI**: Smooth terminal UI with Bubble Tea

## Available AI Models

Ollama supports many models including:
- **llama2** - Meta's LLaMA 2 model
- **mistral** - Fast and efficient model
- **codellama** - Specialized for code
- **gemma** - Google's lightweight model
- **neural-chat** - Intel's conversational model
- **starling-lm** - Berkeley's powerful model
- **orca-mini** - Microsoft's reasoning model
- **vicuna** - Fine-tuned LLaMA model
- **phi** - Microsoft's small but capable model

## Installation

```bash
# Clone the repository
git clone https://github.com/paarthd00/trm-search.git
cd trm-search

# Install using the script
./install.sh

# Or build manually
go build -o trms
sudo mv trms /usr/local/bin/
```

## Usage

### Basic Commands

Start the application:
```bash
trms
```

**Available commands:**
- `:s` or `search <query>` - Search the web
- `:ai` or `ai <prompt>` - Chat with AI
- `:models` - Manage AI models
- `:q` or `quit` - Exit
- Any other input runs as shell command

### AI Features

- **Auto-Setup**: Ollama installs automatically on first use
- **Model Selection**: Press `Ctrl+P` in AI mode to switch models
- **Model Pulling**: Select any model to download it automatically
- **No API Keys**: Everything runs locally on your machine

### Keyboard Shortcuts

- `ESC` - Return to main menu
- `Ctrl+C` - Quit application
- `Ctrl+P` - Change AI model (in AI mode)
- `↑/↓` - Navigate lists
- `Enter` - Select/Execute
- `/` - Filter results

## Examples

```bash
# Search the web
:s golang bubble tea tutorial

# Quick AI chat
ai explain kubernetes in simple terms

# Run shell commands
ls -la
docker ps

# Switch AI models
:models
# Then select a model to use
```

## How It Works

1. **First Run**: The app checks if Ollama is installed
2. **Auto-Install**: If not found, it offers to install Ollama
3. **Model Management**: Pull models as needed with `:models`
4. **Local Processing**: All AI runs on your machine - no cloud needed!

## Troubleshooting

**Ollama not starting?**
```bash
# Start Ollama manually
ollama serve

# Pull a model manually
ollama pull llama2
```

**Want to see available models?**
```bash
# In the app
:models

# Or from terminal
ollama list
```

## Development

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling
- [Ollama](https://ollama.ai) - Local AI models

## License

MIT