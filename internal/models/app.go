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
	CommandMode Mode = iota
	ChatMode
	NewChatMode
	ModelManagementMode
	ChatListMode
	ModelSelectionMode
	ConfirmationMode
	ModelInfoMode
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
	
	// Medium models (8-16GB RAM)
	{Name: "llama2", Description: "Meta's Llama 2 base model", Size: "3.8GB", MemoryGB: 7.0, Tags: []string{"general", "7b"}},
	{Name: "mistral", Description: "Mistral 7B - Fast and efficient", Size: "4.1GB", MemoryGB: 7.5, Tags: []string{"general", "7b", "popular"}},
	{Name: "neural-chat", Description: "Intel's Neural Chat", Size: "4.1GB", MemoryGB: 7.5, Tags: []string{"chat", "7b"}},
	{Name: "starling-lm", Description: "Berkeley's Starling", Size: "4.1GB", MemoryGB: 7.5, Tags: []string{"chat", "7b"}},
	{Name: "vicuna", Description: "Vicuna chat model", Size: "3.8GB", MemoryGB: 7.0, Tags: []string{"chat", "7b"}},
	{Name: "zephyr", Description: "HuggingFace's Zephyr", Size: "4.1GB", MemoryGB: 7.5, Tags: []string{"chat", "7b"}},
	{Name: "codellama", Description: "Code-focused Llama model", Size: "3.8GB", MemoryGB: 7.0, Tags: []string{"code", "7b"}},
	
	// Large models (16-32GB RAM)
	{Name: "llama2:13b", Description: "Larger Llama 2 model", Size: "7.3GB", MemoryGB: 13.0, Tags: []string{"general", "13b"}},
	{Name: "codellama:13b", Description: "Larger code model", Size: "7.3GB", MemoryGB: 13.0, Tags: []string{"code", "13b"}},
	
	// Very large models (32GB+ RAM)
	{Name: "mixtral", Description: "Mixtral 8x7B MoE - Requires 32GB+", Size: "26GB", MemoryGB: 48.0, Tags: []string{"general", "moe", "large"}},
	{Name: "codellama:34b", Description: "Largest code model", Size: "19GB", MemoryGB: 35.0, Tags: []string{"code", "34b", "large"}},
	{Name: "llama2:70b", Description: "Largest Llama 2 model", Size: "39GB", MemoryGB: 70.0, Tags: []string{"general", "70b", "large"}},
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