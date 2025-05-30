# Trm - Terminal AI & Command Runner

A beautiful terminal application that combines command execution with local AI chat using Ollama models - no API keys required!

![dashboard](images/dashboard.png)

## Features

- 🤖 **Local AI Chat**: Chat with 20+ AI models through Ollama
- 📦 **Model Browser**: Browse and pull models with a beautiful interface
- 💻 **Command Execution**: Run shell commands directly
- 🗄️ **Persistent History**: Automatic PostgreSQL setup for chat history
- 🚀 **Auto-Setup**: Ollama and PostgreSQL install automatically when needed
- ⌨️ **Simple Navigation**: Just press Tab to switch modes
- 🎨 **Beautiful TUI**: Clean interface with mode and status indicators

## Keyboard Navigation

- **Tab**: Toggle between Command and Chat modes
- **Ctrl+N**: Create new chat (from chat mode)
- **Ctrl+S**: Quick model switch (in active chat)
- **Ctrl+M**: Model management (download/delete models)
- **Ctrl+G**: Cancel download (during model pull)
- **Ctrl+D**: Delete model (in model management)
- **Enter**: Execute command / Send message / Select model
- **ESC**: Return to command mode
- **Ctrl+C**: Quit

## Quick Commands

In command mode, type:
- `c` or `chat` - Switch to chat mode
- `q` or `quit` - Exit
- Any other text runs as a shell command

## Available AI Models

The model browser shows 100+ models including latest releases:

| Model | Description | Size |
|-------|-------------|------|
| **llama3.2** | Meta's latest Llama 3.2 | 2.0GB |
| **qwen2.5** | Alibaba's latest multilingual | 4.1GB |
| **mistral-nemo** | Mistral's 12B model | 6.8GB |
| **gemma2** | Google's enhanced Gemma 2 | 4.8GB |
| **phi3** | Microsoft's compact power | 2.2GB |
| **codellama** | Code generation specialist | 3.8GB |
| **deepseek-coder** | Advanced coding model | 4.1GB |
| **llava-llama3** | Vision + language model | 5.5GB |

And 100+ more! Browse all models by pressing 'M' in chat mode.

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

Start the application:
```bash
trms
```

### Workflow Examples

**Create New Chat:**
1. Press Tab to enter chat mode, then 'N' for new chat
2. Select from installed models (or press ESC → M to download models first)
3. Start chatting with your chosen model
4. All conversation history is automatically saved

**Quick Model Switch:**
1. In an active chat, press 'S'
2. Select a different installed model
3. Continue the conversation with the new model

**Download Models:**
1. Press 'M' for model management
2. Browse 100+ available models with fuzzy search
3. Press Enter to download, watch real-time progress
4. Press 'C' to cancel, 'D' to delete installed models

**Run Commands:**
1. Just type any command in command mode
2. Press Enter to execute

## Enhanced Model Management

**🚀 Real-time Progress Tracking:**
- Visual progress bars during downloads
- Real-time speed and size information
- Status updates (downloading, verifying, complete)

**⚡ Download Control:**
- **Cancel**: Press 'C' to cancel active downloads
- **Resume**: Restart interrupted downloads
- **Multiple**: Queue multiple model downloads

**🗑️ Model Management:**
- **Delete**: Press 'D' to remove installed models
- **Status**: Clear icons (✅ installed, 📥 available)
- **Sizes**: Accurate model sizes displayed

**🔍 Smart Search:**
- Fuzzy search across 100+ models
- Filter by type: `code`, `vision`, `chat`, `math`
- Sort by size, popularity, or update date

## How It Works

1. **First Run**: Checks if PostgreSQL database and Ollama are available
2. **Auto-Setup**: Offers to setup database and install Ollama if needed
3. **Chat History**: All conversations automatically saved to database
4. **Model Management**: Pull models on-demand
5. **Local Processing**: Everything runs on your machine

## Troubleshooting

**Ollama not starting?**
```bash
# Start manually
ollama serve

# Check installed models
ollama list
```

**Want a specific model?**
Just open the model browser (type `m`) and select it!

## Development

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling
- [Ollama](https://ollama.ai) - Local AI models

## License

MIT