package models

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// Mode represents the current application mode
type Mode int

const (
	ChatMode Mode = iota
	NewChatMode
	ModelManagementMode
	ChatListMode
	ModelSelectionMode
	ConfirmationMode
	ModelInfoMode
	CategorySelectionMode
	ImageGenerationMode
)

// AppModel is the main application model
type AppModel struct {
	// Current mode
	Mode Mode

	// UI Components
	Input     textinput.Model
	ModelList list.Model
	ChatList  list.Model

	// Window dimensions
	Width  int
	Height int

	// State
	CurrentSessionID int
	ChatHistory      []Message
	Quitting         bool
	Error            error
	
	// Confirmation dialog
	ConfirmationAction string
	ConfirmationTarget string
	
	// Model info display
	ModelInfoData interface{}
	ModelInfoTarget string

	// Managers
	Ollama *OllamaManager
	DB     *DatabaseManager

	// Progress tracking
	PullingModel    string
	ModelProgress   *PullProgress
	ShowingProgress bool
	ShowingModels   bool

	// Command output
	CommandOutput   string
	DBSetupProgress string
	
	// Debug mode
	DebugMode       bool
}

// Message represents a chat message
type Message struct {
	ID        int
	SessionID int
	Role      string
	Content   string
	CreatedAt time.Time
}

// ChatSession represents a chat session
type ChatSession struct {
	ID        int
	Name      string
	ModelName string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ModelItem represents a model in the list
type ModelItem struct {
	Name        string
	Desc        string // Renamed to avoid conflict with Description() method
	Size        string
	Installed   bool
	IsPulling   bool
	PullPercent int
	IsSeparator bool
	IsHeader    bool
}

// PullProgress tracks model download progress
type PullProgress struct {
	Model      string
	Status     string
	Percent    int
	Downloaded string
	Total      string
	Speed      string // Download speed like "5.2 MB/s"
	ETA        string // Estimated time remaining like "2m 30s"
}

// OllamaModel represents an Ollama model
type OllamaModel struct {
	Name string
	Size string
}

// Messages for tea.Model updates
type (
	// Command execution
	CommandFinishedMsg struct {
		Output string
		Err    error
	}

	// AI responses
	AIResponseMsg struct {
		Response string
		Err      error
	}

	// Database operations
	DatabaseSetupMsg struct {
		Err error
	}

	// Model operations
	ModelPulledMsg struct {
		Model string
		Err   error
	}

	ModelProgressMsg struct {
		Progress *PullProgress
	}

	ModelDeletedMsg struct {
		Model string
		Err   error
	}

	ModelsRefreshedMsg struct {
		Err error
	}

	// Chat operations
	NewChatMsg struct {
		SessionID int
		Err       error
	}

	ChatsRefreshedMsg struct {
		Err error
	}

	// System operations
	OllamaInstalledMsg struct {
		Err error
	}

	PullCancelledMsg struct{}
	TickMsg          struct{}
	ProgressTickMsg  struct{
		Model string
	}
	
	ModelInfoMsg struct{
		ModelName string
		Info      interface{}
		Err       error
	}
	
	ShowCopyHistoryMsg struct{}
	
	DownloadStartedMsg struct{
		Model string
	}
)

// Init initializes the application model
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.CheckDatabase(),
		m.CheckOllama(),
	)
}

// Placeholder methods - these would contain the actual logic
func (m AppModel) CheckDatabase() tea.Cmd {
	return nil
}

func (m AppModel) CheckOllama() tea.Cmd {
	return nil
}

// ModelInfo contains information about available models
type ModelInfo struct {
	Name        string
	Description string
	Size        string
	MemoryGB    float64  // Memory required in GB
	Tags        []string
}

// AllModels contains information about all available Ollama models  
var AllModels = []ModelInfo{
	// Small models (good for systems with <8GB RAM)
	{Name: "phi", Description: "Microsoft's Phi-2 - Very efficient", Size: "1.7GB", MemoryGB: 3.0, Tags: []string{"small", "efficient", "recommended"}},
	{Name: "orca-mini", Description: "Smaller Orca model - Good for limited RAM", Size: "1.9GB", MemoryGB: 3.5, Tags: []string{"small", "3b", "recommended"}},
	{Name: "tinyllama", Description: "Tiny but capable model", Size: "637MB", MemoryGB: 1.5, Tags: []string{"tiny", "1b", "recommended"}},
	{Name: "phi3", Description: "Microsoft's Phi-3 compact model", Size: "2.3GB", MemoryGB: 4.0, Tags: []string{"small", "efficient", "recommended"}},
	{Name: "phi3.5", Description: "Microsoft's Phi-3.5 small but powerful model", Size: "2.2GB", MemoryGB: 4.0, Tags: []string{"small", "efficient", "recommended"}},
	{Name: "qwen2.5:0.5b", Description: "Qwen 2.5 0.5B parameter model", Size: "394MB", MemoryGB: 2.0, Tags: []string{"tiny", "0.5b", "recommended"}},
	{Name: "qwen2.5:1.5b", Description: "Qwen 2.5 1.5B parameter model", Size: "934MB", MemoryGB: 2.0, Tags: []string{"small", "1.5b", "recommended"}},
	{Name: "llama3.2:1b", Description: "Llama 3.2 1B parameter model", Size: "1.3GB", MemoryGB: 3.0, Tags: []string{"small", "1b", "recommended"}},
	{Name: "llama3.2:3b", Description: "Llama 3.2 3B parameter model", Size: "2.0GB", MemoryGB: 4.0, Tags: []string{"small", "3b", "recommended"}},
	{Name: "moondream", Description: "Small vision language model", Size: "829MB", MemoryGB: 2.0, Tags: []string{"vision", "small", "recommended"}},
	
	// Medium models (8-16GB RAM)
	{Name: "llama3.2", Description: "Meta's latest Llama 3.2 model", Size: "2.0GB", MemoryGB: 4.0, Tags: []string{"general", "3b", "popular"}},
	{Name: "llama3.1", Description: "Meta's Llama 3.1 model", Size: "4.7GB", MemoryGB: 8.0, Tags: []string{"general", "8b", "popular"}},
	{Name: "llama3", Description: "Meta's Llama 3 base model", Size: "4.7GB", MemoryGB: 8.0, Tags: []string{"general", "8b", "popular"}},
	{Name: "llama2", Description: "Meta's Llama 2 base model", Size: "3.8GB", MemoryGB: 8.0, Tags: []string{"general", "7b"}},
	{Name: "mistral", Description: "Mistral AI's flagship model", Size: "4.1GB", MemoryGB: 8.0, Tags: []string{"general", "7b", "popular"}},
	{Name: "qwen2.5", Description: "Alibaba's Qwen 2.5 model", Size: "4.4GB", MemoryGB: 8.0, Tags: []string{"general", "7b", "popular"}},
	{Name: "qwen2", Description: "Alibaba's Qwen 2.0 model", Size: "4.4GB", MemoryGB: 8.0, Tags: []string{"general", "7b"}},
	{Name: "gemma2", Description: "Google's Gemma 2 model", Size: "5.4GB", MemoryGB: 10.0, Tags: []string{"general", "9b"}},
	{Name: "gemma", Description: "Google's Gemma model", Size: "5.0GB", MemoryGB: 10.0, Tags: []string{"general", "7b"}},
	{Name: "neural-chat", Description: "Intel's Neural Chat", Size: "4.1GB", MemoryGB: 8.0, Tags: []string{"chat", "7b"}},
	{Name: "starling-lm", Description: "Berkeley's Starling", Size: "4.1GB", MemoryGB: 8.0, Tags: []string{"chat", "7b"}},
	{Name: "vicuna", Description: "Berkeley's Vicuna model", Size: "3.8GB", MemoryGB: 8.0, Tags: []string{"chat", "7b"}},
	{Name: "zephyr", Description: "HuggingFace's Zephyr model", Size: "4.1GB", MemoryGB: 8.0, Tags: []string{"chat", "7b"}},
	{Name: "orca2", Description: "Microsoft's Orca 2 model", Size: "3.8GB", MemoryGB: 8.0, Tags: []string{"chat", "7b"}},
	{Name: "openchat", Description: "OpenChat conversation model", Size: "4.1GB", MemoryGB: 8.0, Tags: []string{"chat", "7b"}},
	{Name: "llava", Description: "Large Language and Vision Assistant", Size: "4.5GB", MemoryGB: 8.0, Tags: []string{"vision", "7b"}},
	{Name: "llava-llama3", Description: "LLaVA based on Llama 3", Size: "5.5GB", MemoryGB: 10.0, Tags: []string{"vision", "8b"}},
	{Name: "llava-phi3", Description: "LLaVA based on Phi-3", Size: "2.9GB", MemoryGB: 6.0, Tags: []string{"vision", "3b"}},
	{Name: "bakllava", Description: "BakLLaVA vision model", Size: "4.4GB", MemoryGB: 8.0, Tags: []string{"vision", "7b"}},
	
	// Code models
	{Name: "codellama", Description: "Meta's Code Llama for programming", Size: "3.8GB", MemoryGB: 8.0, Tags: []string{"code", "7b", "recommended"}},
	{Name: "codegemma", Description: "Google's CodeGemma for code generation", Size: "5.0GB", MemoryGB: 10.0, Tags: []string{"code", "7b"}},
	{Name: "deepseek-coder", Description: "DeepSeek's specialized coding model", Size: "6.4GB", MemoryGB: 12.0, Tags: []string{"code", "6.7b"}},
	{Name: "starcoder2", Description: "Hugging Face's StarCoder 2", Size: "4.0GB", MemoryGB: 8.0, Tags: []string{"code", "7b"}},
	{Name: "magicoder", Description: "OSS-Instruct trained coding model", Size: "6.7GB", MemoryGB: 12.0, Tags: []string{"code", "7b"}},
	{Name: "stable-code", Description: "Stability AI's code completion model", Size: "1.6GB", MemoryGB: 4.0, Tags: []string{"code", "3b"}},
	
	// Math models
	{Name: "mathstral", Description: "Mistral's specialized math model", Size: "4.1GB", MemoryGB: 8.0, Tags: []string{"math", "7b"}},
	{Name: "qwen2-math", Description: "Qwen2 specialized for mathematics", Size: "4.4GB", MemoryGB: 8.0, Tags: []string{"math", "7b"}},
	{Name: "deepseek-math", Description: "DeepSeek's mathematical reasoning model", Size: "6.4GB", MemoryGB: 12.0, Tags: []string{"math", "7b"}},
	
	// Large models (16-32GB RAM)
	{Name: "llama2:13b", Description: "Larger Llama 2 model", Size: "7.3GB", MemoryGB: 13.0, Tags: []string{"general", "13b"}},
	{Name: "codellama:13b", Description: "Larger code model", Size: "7.3GB", MemoryGB: 13.0, Tags: []string{"code", "13b"}},
	{Name: "llama3.2-vision", Description: "Llama 3.2 with vision capabilities", Size: "7.9GB", MemoryGB: 16.0, Tags: []string{"vision", "11b"}},
	
	// Very large models (32GB+ RAM)
	{Name: "mixtral", Description: "Mixtral 8x7B MoE model", Size: "26GB", MemoryGB: 48.0, Tags: []string{"general", "moe", "large"}},
	{Name: "codellama:34b", Description: "Largest code model", Size: "19GB", MemoryGB: 35.0, Tags: []string{"code", "34b", "large"}},
	{Name: "llama2:70b", Description: "Largest Llama 2 model", Size: "39GB", MemoryGB: 70.0, Tags: []string{"general", "70b", "large"}},
	{Name: "command-r", Description: "Cohere's multilingual model", Size: "20GB", MemoryGB: 35.0, Tags: []string{"general", "35b", "large"}},
	{Name: "command-r-plus", Description: "Cohere's advanced multilingual model", Size: "59GB", MemoryGB: 128.0, Tags: []string{"general", "104b", "large"}},
	
	// Embedding models
	{Name: "nomic-embed-text", Description: "Text embedding model by Nomic AI", Size: "274MB", MemoryGB: 1.0, Tags: []string{"embedding", "small"}},
	{Name: "mxbai-embed-large", Description: "Large embedding model by MixedBread AI", Size: "669MB", MemoryGB: 2.0, Tags: []string{"embedding", "large"}},
	{Name: "all-minilm", Description: "Compact sentence embedding model", Size: "46MB", MemoryGB: 1.0, Tags: []string{"embedding", "tiny"}},
	{Name: "snowflake-arctic-embed", Description: "Snowflake's Arctic embedding model", Size: "669MB", MemoryGB: 2.0, Tags: []string{"embedding", "large"}},
}

// Implement list.Item interface for ModelItem
func (m ModelItem) Title() string {
	if m.IsHeader {
		return m.Name
	}
	if m.IsSeparator {
		return "────────────────────────────────────────"
	}

	status := "📥" // Download icon
	if m.Installed {
		status = "✅" // Installed icon
	} else if m.IsPulling {
		status = "⏬" // Downloading icon
	}
	return status + " " + m.Name + " " + m.Size
}

func (m ModelItem) Description() string {
	if m.IsHeader || m.IsSeparator {
		return ""
	}
	if m.IsPulling {
		return fmt.Sprintf("Downloading... %d%%", m.PullPercent)
	}
	return m.Desc
}

func (m ModelItem) FilterValue() string {
	if m.IsHeader || m.IsSeparator {
		return ""
	}
	return m.Name
}

// ChatSessionItem wraps a ChatSession for the list
type ChatSessionItem struct {
	Session ChatSession
}

func (c ChatSessionItem) Title() string {
	return fmt.Sprintf("[%d] %s", c.Session.ID, c.Session.Name)
}

func (c ChatSessionItem) Description() string {
	return "Model: " + c.Session.ModelName + " | Updated: " + c.Session.UpdatedAt.Format("Jan 2, 15:04")
}

func (c ChatSessionItem) FilterValue() string {
	return c.Session.Name
}