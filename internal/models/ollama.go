package models

// OllamaManager interface defines Ollama operations
type OllamaManager interface {
	// Service management
	IsInstalled() bool
	InstallOllama() error
	IsRunning() bool
	StartService() error

	// Model management
	GetModels() []OllamaModel
	RefreshModels() error
	FetchAvailableModels() error
	GetCurrentModel() string
	SetCurrentModel(model string)

	// Model operations
	PullModel(model string) error
	DeleteModel(model string) error
	IsPulling() bool
	CancelPull() error
	GetPullProgress() *PullProgress
	GetActiveDownloads() map[string]*PullProgress

	// Chat operations
	Chat(prompt string) (string, error)
	ChatWithContext(context string) (string, error)
}