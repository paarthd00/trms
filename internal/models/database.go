package models

// DatabaseManager interface defines database operations
type DatabaseManager interface {
	// Connection management
	Connect() error
	Disconnect() error
	IsConnected() bool
	IsPostgresRunning() bool
	IsDockerInstalled() bool
	SetupPostgres() error

	// Chat session operations
	CreateChatSession(name, modelName string) (*ChatSession, error)
	GetChatSession(sessionID int) (*ChatSession, error)
	GetChatSessions() ([]ChatSession, error)
	UpdateChatSessionModel(sessionID int, modelName string) error

	// Message operations
	SaveMessage(sessionID int, role, content string) (*Message, error)
	GetMessages(sessionID, limit int) ([]Message, error)
	GetRecentMessages(sessionID, count int) ([]Message, error)
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}