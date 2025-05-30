package services

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"trms/internal/models"
)

// OllamaService implements the OllamaManager interface
type OllamaService struct {
	installedModels []models.OllamaModel
	currentModel    string
	mu              sync.Mutex
	pullProgress    *models.PullProgress
	isPulling       bool
	cancelPull      chan bool
	activeDownloads map[string]*models.PullProgress
	modelManager    *ModelManager
}

// NewOllamaService creates a new OllamaService instance
func NewOllamaService() *OllamaService {
	return &OllamaService{
		installedModels: []models.OllamaModel{},
		activeDownloads: make(map[string]*models.PullProgress),
		cancelPull:      make(chan bool, 1),
		modelManager:    NewModelManager(),
	}
}

// IsInstalled checks if Ollama is installed
func (o *OllamaService) IsInstalled() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}

// InstallOllama installs Ollama
func (o *OllamaService) InstallOllama() error {
	// Download and run the install script
	cmd := exec.Command("bash", "-c", "curl -fsSL https://ollama.ai/install.sh | sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// IsRunning checks if Ollama service is running
func (o *OllamaService) IsRunning() bool {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// StartService starts the Ollama service
func (o *OllamaService) StartService() error {
	cmd := exec.Command("ollama", "serve")
	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait for service to be ready
	for i := 0; i < 30; i++ {
		if o.IsRunning() {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout waiting for Ollama service to start")
}

// GetModels returns installed models
func (o *OllamaService) GetModels() []models.OllamaModel {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.installedModels
}

// RefreshModels refreshes the list of installed models
func (o *OllamaService) RefreshModels() error {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	o.installedModels = []models.OllamaModel{}
	for _, m := range result.Models {
		sizeStr := formatBytes(m.Size)
		o.installedModels = append(o.installedModels, models.OllamaModel{
			Name: m.Name,
			Size: sizeStr,
		})

		// Set current model if none selected
		if o.currentModel == "" && len(o.installedModels) > 0 {
			o.currentModel = o.installedModels[0].Name
		}
	}

	return nil
}

// FetchAvailableModels fetches available models (not implemented for local)
func (o *OllamaService) FetchAvailableModels() error {
	// This would typically fetch from Ollama's model registry
	// For now, we use the predefined list in models.AllModels
	return nil
}

// GetCurrentModel returns the current model
func (o *OllamaService) GetCurrentModel() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.currentModel == "" {
		return "No model selected"
	}
	return o.currentModel
}

// SetCurrentModel sets the current model
func (o *OllamaService) SetCurrentModel(model string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.currentModel = model
}

// PullModel pulls a model from Ollama
func (o *OllamaService) PullModel(model string) error {
	o.mu.Lock()
	o.isPulling = true
	o.pullProgress = &models.PullProgress{
		Model:   model,
		Status:  "Starting download...",
		Percent: 0,
	}
	o.activeDownloads[model] = o.pullProgress
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.isPulling = false
		delete(o.activeDownloads, model)
		o.mu.Unlock()
	}()

	// Create pull request
	reqBody := map[string]string{"name": model}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", "http://localhost:11434/api/pull", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 0} // No timeout for downloads
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read streaming response
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-o.cancelPull:
			return fmt.Errorf("download cancelled")
		default:
			var progress struct {
				Status     string `json:"status"`
				Digest     string `json:"digest"`
				Total      int64  `json:"total"`
				Completed  int64  `json:"completed"`
				Percent    int    `json:"percent"`
				Downloaded string `json:"downloaded"`
			}

			if err := json.Unmarshal(scanner.Bytes(), &progress); err != nil {
				continue
			}

			o.mu.Lock()
			if o.pullProgress != nil {
				o.pullProgress.Status = progress.Status
				if progress.Total > 0 {
					o.pullProgress.Percent = int(float64(progress.Completed) / float64(progress.Total) * 100)
					o.pullProgress.Downloaded = formatBytes(progress.Completed)
					o.pullProgress.Total = formatBytes(progress.Total)
				}
			}
			o.mu.Unlock()
		}
	}

	// Refresh models after successful pull
	o.RefreshModels()
	return scanner.Err()
}

// DeleteModel deletes a model
func (o *OllamaService) DeleteModel(model string) error {
	reqBody := map[string]string{"name": model}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("DELETE", "http://localhost:11434/api/delete", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete model: %s", string(body))
	}

	// Refresh models after deletion
	o.RefreshModels()
	return nil
}

// IsPulling returns if a model is being pulled
func (o *OllamaService) IsPulling() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.isPulling
}

// CancelPull cancels the current pull operation
func (o *OllamaService) CancelPull() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	if o.isPulling {
		select {
		case o.cancelPull <- true:
		default:
		}
	}
	return nil
}

// GetPullProgress returns the current pull progress
func (o *OllamaService) GetPullProgress() *models.PullProgress {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.pullProgress
}

// GetActiveDownloads returns all active downloads
func (o *OllamaService) GetActiveDownloads() map[string]*models.PullProgress {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	downloads := make(map[string]*models.PullProgress)
	for k, v := range o.activeDownloads {
		downloads[k] = v
	}
	return downloads
}

// Chat sends a chat message to the current model
func (o *OllamaService) Chat(prompt string) (string, error) {
	return o.ChatWithContext(prompt)
}

// ChatWithContext sends a chat message with context
func (o *OllamaService) ChatWithContext(context string) (string, error) {
	o.mu.Lock()
	model := o.currentModel
	o.mu.Unlock()

	if model == "" || model == "No model selected" {
		return "", fmt.Errorf("no model selected")
	}

	// Use streaming to get full response
	reqBody := map[string]interface{}{
		"model":  model,
		"prompt": context,
		"stream": true,  // Enable streaming for complete responses
		"options": map[string]interface{}{
			"num_predict": 2048,  // Increase token limit
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "http://localhost:11434/api/generate", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 300 * time.Second} // 5 minute timeout for long responses
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Read streaming response
	scanner := bufio.NewScanner(resp.Body)
	var fullResponse strings.Builder
	
	for scanner.Scan() {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
			Error    string `json:"error"`
		}
		
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue // Skip malformed chunks
		}
		
		if chunk.Error != "" {
			return "", fmt.Errorf(chunk.Error)
		}
		
		fullResponse.WriteString(chunk.Response)
		
		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading response: %v", err)
	}

	return fullResponse.String(), nil
}

// formatBytes formats bytes to human readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Helper to check if model exists
func (o *OllamaService) modelExists(name string) bool {
	for _, m := range o.installedModels {
		if m.Name == name || strings.HasPrefix(m.Name, name+":") {
			return true
		}
	}
	return false
}

// GetModelStates returns all model states including partial downloads
func (o *OllamaService) GetModelStates() (map[string]*ModelStatus, error) {
	return o.modelManager.ScanModels()
}

// CleanPartialDownload removes partial download for a model
func (o *OllamaService) CleanPartialDownload(modelName string) error {
	return o.modelManager.CleanPartialDownloads(modelName)
}

// GetPartialDownloads returns models with partial downloads
func (o *OllamaService) GetPartialDownloads() ([]*ModelStatus, error) {
	if _, err := o.modelManager.ScanModels(); err != nil {
		return nil, err
	}
	return o.modelManager.GetPartialDownloads(), nil
}

// GetDownloadQueue returns models currently being downloaded
func (o *OllamaService) GetDownloadQueue() []*ModelStatus {
	return o.modelManager.GetDownloadQueue()
}