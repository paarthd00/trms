package services

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"time"

	_ "github.com/lib/pq"
	"trms/internal/models"
)

// DatabaseService implements the DatabaseManager interface
type DatabaseService struct {
	db       *sql.DB
	config   models.DatabaseConfig
	isConnected bool
}

// NewDatabaseService creates a new DatabaseService instance
func NewDatabaseService() *DatabaseService {
	return &DatabaseService{
		config: models.DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			User:     "trmuser",
			Password: "trmpass",
			Database: "trmdb",
		},
	}
}

// Connect connects to the database
func (d *DatabaseService) Connect() error {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		d.config.Host, d.config.Port, d.config.User, d.config.Password, d.config.Database)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return err
	}

	d.db = db
	d.isConnected = true

	// Initialize schema
	return d.initSchema()
}

// Disconnect closes the database connection
func (d *DatabaseService) Disconnect() error {
	if d.db != nil {
		err := d.db.Close()
		d.isConnected = false
		return err
	}
	return nil
}

// IsConnected returns if database is connected
func (d *DatabaseService) IsConnected() bool {
	return d.isConnected && d.db != nil
}

// IsPostgresRunning checks if PostgreSQL is running
func (d *DatabaseService) IsPostgresRunning() bool {
	cmd := exec.Command("docker", "ps", "--filter", "name=trms-postgres", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(output) > 0 && string(output) != ""
}

// IsDockerInstalled checks if Docker is installed
func (d *DatabaseService) IsDockerInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// SetupPostgres sets up PostgreSQL using docker-compose
func (d *DatabaseService) SetupPostgres() error {
	// Check if docker-compose.yml exists
	if _, err := os.Stat("docker-compose.yml"); err != nil {
		return fmt.Errorf("docker-compose.yml not found")
	}

	// Start PostgreSQL
	cmd := exec.Command("docker-compose", "up", "-d", "postgres")
	if err := cmd.Run(); err != nil {
		return err
	}

	// Wait for PostgreSQL to be ready
	for i := 0; i < 30; i++ {
		if d.IsPostgresRunning() {
			time.Sleep(2 * time.Second) // Extra time for initialization
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout waiting for PostgreSQL to start")
}

// initSchema initializes the database schema
func (d *DatabaseService) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS chat_sessions (
		id SERIAL PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		model_name VARCHAR(100) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id SERIAL PRIMARY KEY,
		session_id INTEGER REFERENCES chat_sessions(id) ON DELETE CASCADE,
		role VARCHAR(20) NOT NULL,
		content TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
	`

	_, err := d.db.Exec(schema)
	return err
}

// CreateChatSession creates a new chat session
func (d *DatabaseService) CreateChatSession(name, modelName string) (*models.ChatSession, error) {
	var session models.ChatSession
	err := d.db.QueryRow(
		"INSERT INTO chat_sessions (name, model_name) VALUES ($1, $2) RETURNING id, name, model_name, created_at, updated_at",
		name, modelName,
	).Scan(&session.ID, &session.Name, &session.ModelName, &session.CreatedAt, &session.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetChatSession retrieves a chat session by ID
func (d *DatabaseService) GetChatSession(sessionID int) (*models.ChatSession, error) {
	var session models.ChatSession
	err := d.db.QueryRow(
		"SELECT id, name, model_name, created_at, updated_at FROM chat_sessions WHERE id = $1",
		sessionID,
	).Scan(&session.ID, &session.Name, &session.ModelName, &session.CreatedAt, &session.UpdatedAt)

	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetChatSessions retrieves all chat sessions
func (d *DatabaseService) GetChatSessions() ([]models.ChatSession, error) {
	rows, err := d.db.Query(
		"SELECT id, name, model_name, created_at, updated_at FROM chat_sessions ORDER BY updated_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []models.ChatSession
	for rows.Next() {
		var session models.ChatSession
		if err := rows.Scan(&session.ID, &session.Name, &session.ModelName, &session.CreatedAt, &session.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

// UpdateChatSessionModel updates the model for a chat session
func (d *DatabaseService) UpdateChatSessionModel(sessionID int, modelName string) error {
	_, err := d.db.Exec(
		"UPDATE chat_sessions SET model_name = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		modelName, sessionID,
	)
	return err
}

// SaveMessage saves a message to the database
func (d *DatabaseService) SaveMessage(sessionID int, role, content string) (*models.Message, error) {
	var msg models.Message
	err := d.db.QueryRow(
		"INSERT INTO messages (session_id, role, content) VALUES ($1, $2, $3) RETURNING id, session_id, role, content, created_at",
		sessionID, role, content,
	).Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt)

	if err != nil {
		return nil, err
	}

	// Update session's updated_at
	d.db.Exec("UPDATE chat_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = $1", sessionID)

	return &msg, nil
}

// GetMessages retrieves messages for a session
func (d *DatabaseService) GetMessages(sessionID, limit int) ([]models.Message, error) {
	query := "SELECT id, session_id, role, content, created_at FROM messages WHERE session_id = $1 ORDER BY created_at"
	args := []interface{}{sessionID}

	if limit > 0 {
		query += " LIMIT $2"
		args = append(args, limit)
	}

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

// GetRecentMessages retrieves recent messages for a session
func (d *DatabaseService) GetRecentMessages(sessionID, count int) ([]models.Message, error) {
	// Get the most recent messages
	rows, err := d.db.Query(`
		SELECT id, session_id, role, content, created_at 
		FROM messages 
		WHERE session_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2
	`, sessionID, count)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		if err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, rows.Err()
}