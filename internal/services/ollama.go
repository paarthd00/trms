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
	downloadStats   map[string]*DownloadStats
}

// DownloadStats tracks download statistics for speed/ETA calculation
type DownloadStats struct {
	StartTime    time.Time
	LastUpdate   time.Time
	LastBytes    int64
	TotalBytes   int64
	Speed        string
	ETA          string
}

// NewOllamaService creates a new OllamaService instance
func NewOllamaService() *OllamaService {
	return &OllamaService{
		installedModels: []models.OllamaModel{},
		activeDownloads: make(map[string]*models.PullProgress),
		cancelPull:      make(chan bool, 1),
		modelManager:    NewModelManager(),
		downloadStats:   make(map[string]*DownloadStats),
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

// GetProgress returns the current progress for a model download
func (o *OllamaService) GetProgress(model string) *models.PullProgress {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	if progress, exists := o.activeDownloads[model]; exists {
		// Get speed and ETA from download stats
		var speed, eta string
		if stats, statsExist := o.downloadStats[model]; statsExist {
			speed = stats.Speed
			eta = stats.ETA
		}
		
		// Return a copy to avoid race conditions
		result := &models.PullProgress{
			Model:      progress.Model,
			Status:     progress.Status,
			Percent:    progress.Percent,
			Downloaded: progress.Downloaded,
			Total:      progress.Total,
		}
		
		// Add speed and ETA to the status for display
		if speed != "" && eta != "" {
			result.Status = fmt.Sprintf("%s • %s • ETA: %s", progress.Status, speed, eta)
		} else if speed != "" {
			result.Status = fmt.Sprintf("%s • %s", progress.Status, speed)
		}
		
		return result
	}
	return nil
}

// GetActiveDownloads returns all currently active downloads
func (o *OllamaService) GetActiveDownloads() map[string]*models.PullProgress {
	o.mu.Lock()
	defer o.mu.Unlock()
	
	// Return a copy to avoid race conditions
	activeDownloads := make(map[string]*models.PullProgress)
	for model, progress := range o.activeDownloads {
		activeDownloads[model] = &models.PullProgress{
			Model:      progress.Model,
			Status:     progress.Status,
			Percent:    progress.Percent,
			Downloaded: progress.Downloaded,
			Total:      progress.Total,
		}
	}
	return activeDownloads
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
	
	// Initialize download stats
	o.downloadStats[model] = &DownloadStats{
		StartTime:  time.Now(),
		LastUpdate: time.Now(),
		LastBytes:  0,
		TotalBytes: 0,
	}
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
				
				// Handle both formats: numeric bytes and string format
				if progress.Total > 0 && progress.Completed > 0 {
					// Direct numeric values from API
					o.pullProgress.Percent = int(float64(progress.Completed) / float64(progress.Total) * 100)
					o.pullProgress.Downloaded = formatBytes(progress.Completed)
					o.pullProgress.Total = formatBytes(progress.Total)
					
					// Calculate speed and ETA
					if stats, exists := o.downloadStats[model]; exists {
						now := time.Now()
						timeDiff := now.Sub(stats.LastUpdate).Seconds()
						
						if timeDiff > 0 {
							bytesDiff := progress.Completed - stats.LastBytes
							if bytesDiff > 0 {
								speedBytesPerSec := float64(bytesDiff) / timeDiff
								stats.Speed = formatBytes(int64(speedBytesPerSec)) + "/s"
								
								// Calculate ETA
								remaining := progress.Total - progress.Completed
								if speedBytesPerSec > 0 {
									etaSeconds := float64(remaining) / speedBytesPerSec
									stats.ETA = formatDuration(time.Duration(etaSeconds) * time.Second)
								}
							}
						}
						
						stats.LastUpdate = now
						stats.LastBytes = progress.Completed
						stats.TotalBytes = progress.Total
					}
				} else if progress.Downloaded != "" {
					// String format from API (e.g., "1.2 GB")
					o.pullProgress.Downloaded = progress.Downloaded
					if o.pullProgress.Total == "" && progress.Total > 0 {
						o.pullProgress.Total = formatBytes(progress.Total)
					}
				}
				
				// Update percent if available
				if progress.Percent > 0 {
					o.pullProgress.Percent = progress.Percent
				}
			}
			o.mu.Unlock()
		}
	}

	// Mark download as complete and clean up
	o.mu.Lock()
	if o.pullProgress != nil {
		o.pullProgress.Status = "Download complete"
		o.pullProgress.Percent = 100
	}
	delete(o.activeDownloads, model)
	delete(o.downloadStats, model)
	o.isPulling = false
	o.mu.Unlock()
	
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

// formatDuration formats a duration to human readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// ModelInfo represents detailed information about a model
type ModelInfo struct {
	Name         string                 `json:"name"`
	ModifiedAt   string                 `json:"modified_at"`
	Size         int64                  `json:"size"`
	Digest       string                 `json:"digest"`
	Details      ModelDetails           `json:"details"`
	License      string                 `json:"license"`
	ModelFile    string                 `json:"modelfile"`
	Parameters   map[string]interface{} `json:"parameters"`
	Template     string                 `json:"template"`
}

// ModelDetails represents model configuration details
type ModelDetails struct {
	Format            string `json:"format"`
	Family            string `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string `json:"parameter_size"`
	QuantizationLevel string `json:"quantization_level"`
}

// GetModelInfo retrieves detailed information about a model using ollama show
func (o *OllamaService) GetModelInfo(modelName string) (*ModelInfo, error) {
	// Use ollama show command to get model info
	cmd := exec.Command("ollama", "show", modelName, "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get model info: %v", err)
	}

	var modelInfo ModelInfo
	if err := json.Unmarshal(output, &modelInfo); err != nil {
		return nil, fmt.Errorf("failed to parse model info: %v", err)
	}

	return &modelInfo, nil
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