# TRMS - Terminal Resource Management Studio

A powerful terminal-based AI chat application with local model management using Ollama.

## Features

- 🤖 **Local AI Chat** - Chat with AI models running locally via Ollama
- 📦 **Model Management** - Browse, download, and manage AI models with real-time progress
- 💾 **Chat History** - Persistent chat sessions stored in PostgreSQL
- 📜 **Session Management** - Create, switch between, and manage multiple chat sessions
- 📊 **Progress Tracking** - Real-time download progress with speed and ETA
- 🎨 **Beautiful TUI** - Clean terminal interface with intuitive navigation
- 🔄 **Auto-start Services** - Automatically starts Docker container and Ollama when needed
- 🧹 **Download Management** - Resume interrupted downloads, clean partial files

## Requirements

- Go 1.19+
- Docker and Docker Compose
- Ollama

## Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/trms.git
cd trms

# Build the application
go build -o trms cmd/trms/main.go

# Run the application
./trms
```

The application will automatically:
1. Check if Docker is installed
2. Start the PostgreSQL container if not running
3. Initialize the database schema
4. Check if Ollama is running and attempt to start it

## Usage

### Key Bindings

#### Global
- **Tab** - Switch between Command and Chat modes
- **Ctrl+M** - Open model management
- **Ctrl+C** - Quit application
- **ESC** - Return to command mode

#### Chat Mode
- **↑/↓** - Scroll through chat history
- **v** - Select text
- **Ctrl+Y** - Copy selected text
- **g/G** - Jump to top/bottom of chat
- **Enter** - Send message
- **Ctrl+N** - Create new chat session
- **Ctrl+S** - Switch model for current chat
- **Ctrl+H** - View chat history

#### Model Management
- **↑/↓** - Navigate through model list
- **Enter** - Download selected model
- **d** - Delete installed model
- **c** - Clean partial downloads
- **r** - Restart failed downloads
- **i** - View detailed model information
- **ESC** - Return to previous mode

### Command Mode

Type commands or shortcuts:
- `chat` or `c` - Switch to chat mode
- `models` or `m` - Open model management
- `quit`, `q`, or `exit` - Exit application
- Any other text - Execute as shell command

### Chat Mode

- Chat with your selected AI model
- All conversations are automatically saved to the database
- Chat history persists between sessions
- The model name is displayed in the mode indicator
- If no model is selected, you'll be prompted to download one

**Chat Session Management:**
- **Create New Chat** (Ctrl+N): Start a fresh conversation with timestamp
- **Switch Models** (Ctrl+S): Change models mid-conversation with context preserved
- **View History** (Ctrl+H): Browse and switch between previous chat sessions
- **Model Changes**: System messages track when models are switched within a chat
- **Current Model Display**: Always shows which model is active in chat header

### Model Management

The model manager provides comprehensive model control:

**Features:**
- Browse 20+ available Ollama models
- Real-time system memory display
- Memory requirement warnings for each model
- Model status indicators:
  - 🔴 Currently active model
  - ✅ Installed and ready to use
  - 📥 Available for download
  - ⚠️ Partial download (can be cleaned)
  - 🔄 Currently downloading

**Downloading Models:**
- Press **Enter** to download a model (or switch to it if already installed)
- Real-time progress bar with:
  - Download percentage
  - Current/Total size (e.g., 1.2GB/2.0GB)
  - Download speed
  - Time remaining estimate
- Downloads can be resumed if interrupted
- Multiple models can be queued for download
- Progress tracking for each model in queue

**Managing Models:**
- **d** - Delete an installed model completely
- **c** - Clean partial/corrupted downloads
- **r** - Restart failed or interrupted downloads
- **i** - View detailed model information (size, parameters, license, etc.)
- **ESC** - Cancel current operation and return
- **Model Selection**: Direct integration with chat mode for quick switching

**Memory Management:**
- Automatically checks if you have enough memory to run a model
- Shows warnings for models that exceed available memory
- Recommends smaller models if system resources are limited

## Database Schema

The application uses PostgreSQL with the following schema:
- **chat_sessions** - Stores chat session metadata
- **messages** - Stores all chat messages with timestamps
- **model_feedback** - For future model performance tracking
- **pgvector** - Enabled for future semantic search capabilities

## Configuration

Database configuration (automatically managed):
- Host: localhost
- Port: 5433 (mapped from container's 5432)
- Database: trms
- User: trms
- Password: trms_password

## Troubleshooting

**Database not starting?**
- Ensure Docker is installed and running
- Check if port 5433 is available
- The app will show connection errors but continue to run

**Ollama not working?**
- The app will try to start Ollama automatically
- If it fails, start Ollama manually: `ollama serve`
- Check available models: `ollama list`

**Model download fails?**
- Check available disk space
- Ensure you have sufficient memory for the model
- Try cleaning partial downloads with 'c' in model management

## License

MIT