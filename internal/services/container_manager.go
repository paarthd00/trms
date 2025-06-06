package services

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ContainerManager manages Docker containers for TRMS
type ContainerManager struct {
	dependencyManager *DependencyManager
}

// ContainerStatus represents the status of a container
type ContainerStatus struct {
	Name      string
	Status    string
	Health    string
	Ports     []string
	Running   bool
	Exists    bool
}

// NewContainerManager creates a new container manager
func NewContainerManager() *ContainerManager {
	return &ContainerManager{
		dependencyManager: NewDependencyManager(),
	}
}

// StartupCheck performs comprehensive startup checks for all containers
func (cm *ContainerManager) StartupCheck() error {
	fmt.Println("🔍 Performing TRMS startup checks...")

	// Check dependencies first
	if err := cm.ensureDependencies(); err != nil {
		return fmt.Errorf("dependency check failed: %w", err)
	}

	// Check and start PostgreSQL container
	if err := cm.ensurePostgreSQL(); err != nil {
		fmt.Printf("⚠️  PostgreSQL setup failed: %v\n", err)
		fmt.Println("Database features will be limited.")
	}

	// Check and start Ollama container
	if err := cm.ensureOllama(); err != nil {
		fmt.Printf("⚠️  Ollama setup failed: %v\n", err)
		fmt.Println("AI features may be limited.")
	}

	fmt.Println("✅ Startup checks completed!")
	return nil
}

// ensureDependencies ensures all system dependencies are installed
func (cm *ContainerManager) ensureDependencies() error {
	// Check if Docker is installed and running
	if !cm.dependencyManager.IsDockerInstalled() {
		fmt.Println("🐳 Docker not found!")
		if cm.dependencyManager.PromptUserPermission() {
			return cm.dependencyManager.CheckAndInstallDependencies()
		} else {
			return fmt.Errorf("Docker is required for TRMS to function properly")
		}
	}

	if !cm.dependencyManager.IsDockerRunning() {
		fmt.Println("🐳 Starting Docker daemon...")
		return cm.dependencyManager.StartDockerDaemon()
	}

	fmt.Println("✅ Docker is ready")
	return nil
}

// ensurePostgreSQL ensures PostgreSQL container is running
func (cm *ContainerManager) ensurePostgreSQL() error {
	fmt.Println("🐘 Checking PostgreSQL container...")

	status := cm.getContainerStatus("trms-postgres")
	
	// If no specific trms-postgres container, check if there's a compatible one
	if !status.Exists {
		if cm.isPortInUse("5433") && cm.isExistingPostgreSQLCompatible() {
			fmt.Println("✅ Using existing PostgreSQL container on port 5433")
			return nil
		}
		fmt.Println("📥 Creating PostgreSQL container...")
		return cm.createPostgreSQLContainer()
	}

	if !status.Running {
		fmt.Println("🚀 Starting PostgreSQL container...")
		return cm.startContainer("trms-postgres")
	}

	if status.Health != "healthy" && status.Health != "" {
		fmt.Println("⏳ Waiting for PostgreSQL to be healthy...")
		return cm.waitForContainerHealth("trms-postgres", 30*time.Second)
	}

	fmt.Println("✅ PostgreSQL container is ready")
	return nil
}

// ensureOllama ensures Ollama container is running
func (cm *ContainerManager) ensureOllama() error {
	fmt.Println("🤖 Checking Ollama container...")

	status := cm.getContainerStatus("trms-ollama")
	
	if !status.Exists {
		fmt.Println("📥 Creating Ollama container...")
		return cm.createOllamaContainer()
	}

	if !status.Running {
		fmt.Println("🚀 Starting Ollama container...")
		return cm.startContainer("trms-ollama")
	}

	fmt.Println("✅ Ollama container is ready")
	return nil
}

// getContainerStatus gets the status of a container
func (cm *ContainerManager) getContainerStatus(containerName string) ContainerStatus {
	status := ContainerStatus{Name: containerName}

	// Check if container exists
	cmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return status
	}

	if strings.TrimSpace(string(output)) == containerName {
		status.Exists = true

		// Get detailed status
		cmd = exec.Command("docker", "inspect", containerName)
		output, err = cmd.Output()
		if err != nil {
			return status
		}

		var containers []map[string]interface{}
		if err := json.Unmarshal(output, &containers); err != nil || len(containers) == 0 {
			return status
		}

		container := containers[0]

		// Check running status
		if state, ok := container["State"].(map[string]interface{}); ok {
			if running, ok := state["Running"].(bool); ok {
				status.Running = running
			}
			if statusStr, ok := state["Status"].(string); ok {
				status.Status = statusStr
			}
			if health, ok := state["Health"].(map[string]interface{}); ok {
				if healthStatus, ok := health["Status"].(string); ok {
					status.Health = healthStatus
				}
			}
		}

		// Get port mappings
		if networkSettings, ok := container["NetworkSettings"].(map[string]interface{}); ok {
			if ports, ok := networkSettings["Ports"].(map[string]interface{}); ok {
				for port, bindings := range ports {
					if bindingList, ok := bindings.([]interface{}); ok && len(bindingList) > 0 {
						if binding, ok := bindingList[0].(map[string]interface{}); ok {
							if hostPort, ok := binding["HostPort"].(string); ok {
								status.Ports = append(status.Ports, fmt.Sprintf("%s:%s", hostPort, port))
							}
						}
					}
				}
			}
		}
	}

	return status
}

// createPostgreSQLContainer creates a PostgreSQL container using docker-compose
func (cm *ContainerManager) createPostgreSQLContainer() error {
	// Check if there's already a postgres container running on the same port
	if cm.isPortInUse("5433") {
		fmt.Println("Port 5433 is already in use by another container")
		// Try to use the existing container if it's compatible
		if cm.isExistingPostgreSQLCompatible() {
			fmt.Println("Using existing PostgreSQL container")
			return nil
		}
		// Otherwise use a different port
		return cm.createPostgreSQLWithAlternativePort()
	}

	// Check if docker-compose.yml exists
	if _, err := os.Stat("docker-compose.yml"); os.IsNotExist(err) {
		// Create docker-compose.yml if it doesn't exist
		if err := cm.createDockerComposeFile(); err != nil {
			return fmt.Errorf("failed to create docker-compose.yml: %w", err)
		}
	}

	// Start PostgreSQL using docker-compose
	cmd := exec.Command("docker-compose", "up", "-d", "postgres")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		// Fallback to docker run if docker-compose fails
		return cm.createPostgreSQLContainerDirectly()
	}

	// Wait for container to be healthy
	return cm.waitForContainerHealth("trms-postgres", 60*time.Second)
}

// createPostgreSQLContainerDirectly creates PostgreSQL container using docker run
func (cm *ContainerManager) createPostgreSQLContainerDirectly() error {
	// Create init.sql if it doesn't exist
	if err := cm.ensureInitSQL(); err != nil {
		return fmt.Errorf("failed to create init.sql: %w", err)
	}

	// Get current directory for volume mounting
	currentDir, _ := os.Getwd()
	initSQLPath := filepath.Join(currentDir, "init.sql")

	cmd := exec.Command("docker", "run", "-d",
		"--name", "trms-postgres",
		"-e", "POSTGRES_DB=trms",
		"-e", "POSTGRES_USER=trms",
		"-e", "POSTGRES_PASSWORD=trms_password",
		"-p", "5433:5432",
		"-v", "trms_postgres_data:/var/lib/postgresql/data",
		"-v", fmt.Sprintf("%s:/docker-entrypoint-initdb.d/init.sql", initSQLPath),
		"--health-cmd", "pg_isready -U trms -d trms",
		"--health-interval", "5s",
		"--health-timeout", "5s",
		"--health-retries", "5",
		"pgvector/pgvector:pg16")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL container: %w", err)
	}

	return cm.waitForContainerHealth("trms-postgres", 60*time.Second)
}

// createOllamaContainer creates an Ollama container
func (cm *ContainerManager) createOllamaContainer() error {
	cmd := exec.Command("docker", "run", "-d",
		"--name", "trms-ollama",
		"-p", "11434:11434",
		"-v", "trms_ollama:/root/.ollama",
		"--restart", "unless-stopped",
		"ollama/ollama")

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Ollama container: %w", err)
	}

	// Wait a bit for Ollama to start
	time.Sleep(10 * time.Second)
	
	fmt.Println("✅ Ollama container created and started")
	return nil
}

// startContainer starts a stopped container
func (cm *ContainerManager) startContainer(containerName string) error {
	cmd := exec.Command("docker", "start", containerName)
	return cmd.Run()
}

// waitForContainerHealth waits for a container to become healthy
func (cm *ContainerManager) waitForContainerHealth(containerName string, timeout time.Duration) error {
	start := time.Now()
	
	for time.Since(start) < timeout {
		status := cm.getContainerStatus(containerName)
		
		if status.Health == "healthy" || (status.Health == "" && status.Running) {
			return nil
		}
		
		if status.Health == "unhealthy" {
			return fmt.Errorf("container %s is unhealthy", containerName)
		}
		
		fmt.Printf("⏳ Waiting for %s to be ready... (%s)\n", containerName, status.Status)
		time.Sleep(3 * time.Second)
	}
	
	return fmt.Errorf("timeout waiting for container %s to be healthy", containerName)
}

// createDockerComposeFile creates a docker-compose.yml file
func (cm *ContainerManager) createDockerComposeFile() error {
	composeContent := `version: '3.8'

services:
  postgres:
    container_name: trms-postgres
    image: pgvector/pgvector:pg16
    environment:
      POSTGRES_DB: trms
      POSTGRES_USER: trms
      POSTGRES_PASSWORD: trms_password
    ports:
      - "5433:5432"
    volumes:
      - trms_postgres_data:/var/lib/postgresql/data
      - ./init.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U trms -d trms"]
      interval: 5s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  ollama:
    container_name: trms-ollama
    image: ollama/ollama
    ports:
      - "11434:11434"
    volumes:
      - trms_ollama:/root/.ollama
    restart: unless-stopped

volumes:
  trms_postgres_data:
  trms_ollama:
`

	return os.WriteFile("docker-compose.yml", []byte(composeContent), 0644)
}

// ensureInitSQL creates init.sql if it doesn't exist
func (cm *ContainerManager) ensureInitSQL() error {
	if _, err := os.Stat("init.sql"); os.IsNotExist(err) {
		initSQLContent := `-- TRMS Database Schema
CREATE EXTENSION IF NOT EXISTS vector;

-- Chat sessions table
CREATE TABLE IF NOT EXISTS chat_sessions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    model VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Messages table
CREATE TABLE IF NOT EXISTS messages (
    id SERIAL PRIMARY KEY,
    session_id INTEGER REFERENCES chat_sessions(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Model metadata table
CREATE TABLE IF NOT EXISTS model_metadata (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    size_bytes BIGINT,
    download_date TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    usage_count INTEGER DEFAULT 0
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_created_at ON chat_sessions(created_at);

-- Insert default session if none exists
INSERT INTO chat_sessions (name, model) 
SELECT 'Default Chat', 'phi' 
WHERE NOT EXISTS (SELECT 1 FROM chat_sessions);
`

		return os.WriteFile("init.sql", []byte(initSQLContent), 0644)
	}
	return nil
}

// GetContainerLogs gets logs from a container
func (cm *ContainerManager) GetContainerLogs(containerName string, lines int) (string, error) {
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName)
	output, err := cmd.Output()
	return string(output), err
}

// RestartContainer restarts a container
func (cm *ContainerManager) RestartContainer(containerName string) error {
	cmd := exec.Command("docker", "restart", containerName)
	return cmd.Run()
}

// StopContainer stops a container
func (cm *ContainerManager) StopContainer(containerName string) error {
	cmd := exec.Command("docker", "stop", containerName)
	return cmd.Run()
}

// RemoveContainer removes a container
func (cm *ContainerManager) RemoveContainer(containerName string) error {
	// Stop first
	cm.StopContainer(containerName)
	
	// Then remove
	cmd := exec.Command("docker", "rm", containerName)
	return cmd.Run()
}

// GetAllContainerStatuses gets status of all TRMS containers
func (cm *ContainerManager) GetAllContainerStatuses() map[string]ContainerStatus {
	containers := map[string]ContainerStatus{
		"postgres": cm.getContainerStatus("trms-postgres"),
		"ollama":   cm.getContainerStatus("trms-ollama"),
	}
	return containers
}

// HealthCheck performs a health check on all containers
func (cm *ContainerManager) HealthCheck() error {
	statuses := cm.GetAllContainerStatuses()
	
	var errors []string
	
	for name, status := range statuses {
		if !status.Exists {
			errors = append(errors, fmt.Sprintf("%s container does not exist", name))
			continue
		}
		
		if !status.Running {
			errors = append(errors, fmt.Sprintf("%s container is not running", name))
			continue
		}
		
		if status.Health == "unhealthy" {
			errors = append(errors, fmt.Sprintf("%s container is unhealthy", name))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("health check failed: %s", strings.Join(errors, "; "))
	}
	
	return nil
}

// isPortInUse checks if a port is already in use by any container
func (cm *ContainerManager) isPortInUse(port string) bool {
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("publish=%s", port), "--format", "{{.Names}}")
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) != ""
}

// isExistingPostgreSQLCompatible checks if the existing PostgreSQL container is compatible
func (cm *ContainerManager) isExistingPostgreSQLCompatible() bool {
	// Check if there's a container on port 5433 with the right database
	cmd := exec.Command("docker", "ps", "--filter", "publish=5433", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	
	containerName := strings.TrimSpace(string(output))
	if containerName == "" {
		return false
	}
	
	// Try to connect to see if it has the right database/user
	// For now, assume it's compatible if it exists
	return true
}

// createPostgreSQLWithAlternativePort creates PostgreSQL on an alternative port
func (cm *ContainerManager) createPostgreSQLWithAlternativePort() error {
	// Find an available port starting from 5434
	availablePort := cm.findAvailablePort(5434)
	
	fmt.Printf("Creating PostgreSQL container on port %d\n", availablePort)
	
	// Create init.sql if it doesn't exist
	if err := cm.ensureInitSQL(); err != nil {
		return fmt.Errorf("failed to create init.sql: %w", err)
	}

	// Get current directory for volume mounting
	currentDir, _ := os.Getwd()
	initSQLPath := filepath.Join(currentDir, "init.sql")

	cmd := exec.Command("docker", "run", "-d",
		"--name", "trms-postgres",
		"-e", "POSTGRES_DB=trms",
		"-e", "POSTGRES_USER=trms",
		"-e", "POSTGRES_PASSWORD=trms_password",
		"-p", fmt.Sprintf("%d:5432", availablePort),
		"-v", "trms_postgres_data:/var/lib/postgresql/data",
		"-v", fmt.Sprintf("%s:/docker-entrypoint-initdb.d/init.sql", initSQLPath),
		"--health-cmd", "pg_isready -U trms -d trms",
		"--health-interval", "5s",
		"--health-timeout", "5s",
		"--health-retries", "5",
		"pgvector/pgvector:pg16")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL container: %w", err)
	}

	fmt.Printf("PostgreSQL container created on port %d\n", availablePort)
	return cm.waitForContainerHealth("trms-postgres", 60*time.Second)
}

// findAvailablePort finds an available port starting from the given port
func (cm *ContainerManager) findAvailablePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		if !cm.isPortInUse(fmt.Sprintf("%d", port)) {
			return port
		}
	}
	return startPort // Fallback
}

// ShowStatus displays status of all containers
func (cm *ContainerManager) ShowStatus() {
	fmt.Println("\n🐳 TRMS Container Status:")
	fmt.Println(strings.Repeat("=", 50))
	
	statuses := cm.GetAllContainerStatuses()
	
	for name, status := range statuses {
		fmt.Printf("\n📦 %s:\n", strings.ToUpper(name))
		fmt.Printf("  Exists: %v\n", status.Exists)
		fmt.Printf("  Running: %v\n", status.Running)
		fmt.Printf("  Status: %s\n", status.Status)
		if status.Health != "" {
			fmt.Printf("  Health: %s\n", status.Health)
		}
		if len(status.Ports) > 0 {
			fmt.Printf("  Ports: %s\n", strings.Join(status.Ports, ", "))
		}
	}
	
	fmt.Println()
}
