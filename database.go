package main

import (
	"database/sql"
	"fmt"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type ChatSession struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	ModelName string    `json:"model_name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID        int       `json:"id"`
	SessionID int       `json:"session_id"`
	Role      string    `json:"role"` // 'user' or 'assistant'
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type DatabaseManager struct {
	db *sql.DB
}

func (dm *DatabaseManager) IsDockerInstalled() bool {
	cmd := exec.Command("which", "docker")
	err := cmd.Run()
	return err == nil
}

func (dm *DatabaseManager) IsDockerComposeInstalled() bool {
	cmd := exec.Command("docker", "compose", "version")
	err := cmd.Run()
	return err == nil
}

func (dm *DatabaseManager) IsPostgresRunning() bool {
	cmd := exec.Command("docker", "compose", "ps", "--services", "--filter", "status=running")
	cmd.Dir = "."
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "postgres")
}

func (dm *DatabaseManager) SetupPostgres() error {
	// Check if Docker is installed
	if !dm.IsDockerInstalled() {
		return fmt.Errorf("Docker is required but not installed. Please install Docker first")
	}

	// Check if Docker Compose is available
	if !dm.IsDockerComposeInstalled() {
		return fmt.Errorf("Docker Compose is required but not available")
	}

	// Pull the PostgreSQL image first (this shows download progress)
	pullCmd := exec.Command("docker", "compose", "pull", "postgres")
	pullCmd.Dir = "."
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("failed to pull PostgreSQL image: %w", err)
	}

	// Start PostgreSQL container
	cmd := exec.Command("docker", "compose", "up", "-d", "postgres")
	cmd.Dir = "."
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start PostgreSQL container: %w", err)
	}

	// Wait for PostgreSQL to be ready
	return dm.waitForPostgres()
}

func (dm *DatabaseManager) waitForPostgres() error {
	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		cmd := exec.Command("docker", "compose", "exec", "-T", "postgres", "pg_isready", "-U", "trms", "-d", "trms")
		cmd.Dir = "."
		if err := cmd.Run(); err == nil {
			// Give it one more second for full initialization
			time.Sleep(1 * time.Second)
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("PostgreSQL failed to start within 30 seconds")
}

func (dm *DatabaseManager) StopPostgres() error {
	cmd := exec.Command("docker", "compose", "down")
	cmd.Dir = "."
	return cmd.Run()
}

func NewDatabaseManager() *DatabaseManager {
	return &DatabaseManager{db: nil}
}

func (dm *DatabaseManager) Connect() error {
	connStr := "host=localhost port=5433 user=trms password=trms_password dbname=trms sslmode=disable"
	
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	
	// Test connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("database not available: %w", err)
	}
	
	dm.db = db
	return nil
}

func (dm *DatabaseManager) IsConnected() bool {
	return dm.db != nil
}

func (dm *DatabaseManager) Close() error {
	if dm.db != nil {
		return dm.db.Close()
	}
	return nil
}

// Chat Session Management
func (dm *DatabaseManager) CreateChatSession(name, modelName string) (*ChatSession, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	query := `
		INSERT INTO chat_sessions (name, model_name) 
		VALUES ($1, $2) 
		RETURNING id, name, model_name, created_at, updated_at`
	
	session := &ChatSession{}
	err := dm.db.QueryRow(query, name, modelName).Scan(
		&session.ID, &session.Name, &session.ModelName, 
		&session.CreatedAt, &session.UpdatedAt,
	)
	
	return session, err
}

func (dm *DatabaseManager) GetChatSessions() ([]ChatSession, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	query := `
		SELECT id, name, model_name, created_at, updated_at 
		FROM chat_sessions 
		ORDER BY updated_at DESC`
	
	rows, err := dm.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var sessions []ChatSession
	for rows.Next() {
		var session ChatSession
		err := rows.Scan(&session.ID, &session.Name, &session.ModelName, 
			&session.CreatedAt, &session.UpdatedAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	
	return sessions, nil
}

func (dm *DatabaseManager) GetChatSession(id int) (*ChatSession, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	query := `
		SELECT id, name, model_name, created_at, updated_at 
		FROM chat_sessions 
		WHERE id = $1`
	
	session := &ChatSession{}
	err := dm.db.QueryRow(query, id).Scan(
		&session.ID, &session.Name, &session.ModelName, 
		&session.CreatedAt, &session.UpdatedAt,
	)
	
	return session, err
}

func (dm *DatabaseManager) UpdateChatSession(id int, name string) error {
	if !dm.IsConnected() {
		return fmt.Errorf("database not connected")
	}
	
	query := `
		UPDATE chat_sessions 
		SET name = $1, updated_at = CURRENT_TIMESTAMP 
		WHERE id = $2`
	
	_, err := dm.db.Exec(query, name, id)
	return err
}

func (dm *DatabaseManager) UpdateChatSessionModel(id int, modelName string) error {
	if !dm.IsConnected() {
		return fmt.Errorf("database not connected")
	}
	
	query := `
		UPDATE chat_sessions 
		SET model_name = $1, updated_at = CURRENT_TIMESTAMP 
		WHERE id = $2`
	
	_, err := dm.db.Exec(query, modelName, id)
	return err
}

func (dm *DatabaseManager) DeleteChatSession(id int) error {
	if !dm.IsConnected() {
		return fmt.Errorf("database not connected")
	}
	
	query := `DELETE FROM chat_sessions WHERE id = $1`
	_, err := dm.db.Exec(query, id)
	return err
}

// Message Management
func (dm *DatabaseManager) SaveMessage(sessionID int, role, content string) (*Message, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	// Update session's updated_at timestamp
	dm.db.Exec("UPDATE chat_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = $1", sessionID)
	
	query := `
		INSERT INTO messages (session_id, role, content) 
		VALUES ($1, $2, $3) 
		RETURNING id, session_id, role, content, created_at`
	
	message := &Message{}
	err := dm.db.QueryRow(query, sessionID, role, content).Scan(
		&message.ID, &message.SessionID, &message.Role, 
		&message.Content, &message.CreatedAt,
	)
	
	return message, err
}

func (dm *DatabaseManager) GetMessages(sessionID int, limit int) ([]Message, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	query := `
		SELECT id, session_id, role, content, created_at 
		FROM messages 
		WHERE session_id = $1 
		ORDER BY created_at ASC`
	
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	
	rows, err := dm.db.Query(query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []Message
	for rows.Next() {
		var message Message
		err := rows.Scan(&message.ID, &message.SessionID, &message.Role, 
			&message.Content, &message.CreatedAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	
	return messages, nil
}

func (dm *DatabaseManager) GetRecentMessages(sessionID int, limit int) ([]Message, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	query := `
		SELECT id, session_id, role, content, created_at 
		FROM messages 
		WHERE session_id = $1 
		ORDER BY created_at DESC 
		LIMIT $2`
	
	rows, err := dm.db.Query(query, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []Message
	for rows.Next() {
		var message Message
		err := rows.Scan(&message.ID, &message.SessionID, &message.Role, 
			&message.Content, &message.CreatedAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	
	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	
	return messages, nil
}

// Vector operations (for future model improvement)
func (dm *DatabaseManager) SaveMessageWithEmbedding(sessionID int, role, content string, embedding []float32) (*Message, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	// Convert float32 slice to PostgreSQL vector format
	vectorStr := "["
	for i, val := range embedding {
		if i > 0 {
			vectorStr += ","
		}
		vectorStr += fmt.Sprintf("%f", val)
	}
	vectorStr += "]"
	
	query := `
		INSERT INTO messages (session_id, role, content, embedding) 
		VALUES ($1, $2, $3, $4::vector) 
		RETURNING id, session_id, role, content, created_at`
	
	message := &Message{}
	err := dm.db.QueryRow(query, sessionID, role, content, vectorStr).Scan(
		&message.ID, &message.SessionID, &message.Role, 
		&message.Content, &message.CreatedAt,
	)
	
	return message, err
}

func (dm *DatabaseManager) FindSimilarMessages(embedding []float32, limit int) ([]Message, error) {
	if !dm.IsConnected() {
		return nil, fmt.Errorf("database not connected")
	}
	
	// Convert float32 slice to PostgreSQL vector format
	vectorStr := "["
	for i, val := range embedding {
		if i > 0 {
			vectorStr += ","
		}
		vectorStr += fmt.Sprintf("%f", val)
	}
	vectorStr += "]"
	
	query := `
		SELECT id, session_id, role, content, created_at 
		FROM messages 
		WHERE embedding IS NOT NULL 
		ORDER BY embedding <=> $1::vector 
		LIMIT $2`
	
	rows, err := dm.db.Query(query, vectorStr, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var messages []Message
	for rows.Next() {
		var message Message
		err := rows.Scan(&message.ID, &message.SessionID, &message.Role, 
			&message.Content, &message.CreatedAt)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	
	return messages, nil
}