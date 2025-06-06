package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DownloadState represents the state of a model download
type DownloadState int

const (
	DownloadStateIdle DownloadState = iota
	DownloadStateQueued
	DownloadStateDownloading
	DownloadStateCompleted
	DownloadStateFailed
	DownloadStateCancelled
)

func (ds DownloadState) String() string {
	switch ds {
	case DownloadStateIdle:
		return "idle"
	case DownloadStateQueued:
		return "queued"
	case DownloadStateDownloading:
		return "downloading"
	case DownloadStateCompleted:
		return "completed"
	case DownloadStateFailed:
		return "failed"
	case DownloadStateCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// DownloadProgress represents the current progress of a download
type DownloadProgress struct {
	ModelName      string
	State          DownloadState
	Progress       float64 // 0-100
	CurrentBytes   int64
	TotalBytes     int64
	Speed          string // e.g., "5.2 MB/s"
	ETA            string // e.g., "2m 30s"
	Error          string
	StartTime      time.Time
	LastUpdate     time.Time
}

// DownloadManager handles all model downloads
type DownloadManager struct {
	mu              sync.RWMutex
	downloads       map[string]*DownloadProgress
	activeDownloads map[string]*exec.Cmd
	ollamaService   *OllamaService
}

// NewDownloadManager creates a new download manager
func NewDownloadManager(ollamaService *OllamaService) *DownloadManager {
	return &DownloadManager{
		downloads:       make(map[string]*DownloadProgress),
		activeDownloads: make(map[string]*exec.Cmd),
		ollamaService:   ollamaService,
	}
}

// StartDownload starts downloading a model
func (dm *DownloadManager) StartDownload(modelName string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Check if already downloading
	if progress, exists := dm.downloads[modelName]; exists {
		if progress.State == DownloadStateDownloading || progress.State == DownloadStateQueued {
			return fmt.Errorf("model %s is already downloading", modelName)
		}
	}

	// Initialize download progress
	dm.downloads[modelName] = &DownloadProgress{
		ModelName:  modelName,
		State:      DownloadStateQueued,
		Progress:   0,
		StartTime:  time.Now(),
		LastUpdate: time.Now(),
	}

	// Start the download in a goroutine
	go dm.executeDownload(modelName)

	return nil
}

// executeDownload runs the actual ollama pull command
func (dm *DownloadManager) executeDownload(modelName string) {
	dm.mu.Lock()
	progress := dm.downloads[modelName]
	progress.State = DownloadStateDownloading
	dm.mu.Unlock()

	// Run ollama pull
	cmd := exec.Command("ollama", "pull", modelName)
	
	// Store the command for potential cancellation
	dm.mu.Lock()
	dm.activeDownloads[modelName] = cmd
	dm.mu.Unlock()

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		dm.setDownloadError(modelName, fmt.Errorf("failed to create stdout pipe: %w", err))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		dm.setDownloadError(modelName, fmt.Errorf("failed to create stderr pipe: %w", err))
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		dm.setDownloadError(modelName, fmt.Errorf("failed to start download: %w", err))
		return
	}

	// Read progress from stdout
	go dm.readProgress(modelName, stdout)
	
	// Read errors from stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		var errorMsg strings.Builder
		for scanner.Scan() {
			errorMsg.WriteString(scanner.Text() + "\n")
		}
		if errorMsg.Len() > 0 {
			dm.mu.Lock()
			if p, exists := dm.downloads[modelName]; exists {
				p.Error = errorMsg.String()
			}
			dm.mu.Unlock()
		}
	}()

	// Wait for command to complete
	err = cmd.Wait()
	
	dm.mu.Lock()
	delete(dm.activeDownloads, modelName)
	
	if progress, exists := dm.downloads[modelName]; exists {
		if err != nil {
			if progress.State != DownloadStateCancelled {
				progress.State = DownloadStateFailed
				progress.Error = err.Error()
			}
		} else if progress.State != DownloadStateCancelled {
			progress.State = DownloadStateCompleted
			progress.Progress = 100
		}
		progress.LastUpdate = time.Now()
	}
	dm.mu.Unlock()
}

// readProgress reads and parses progress updates from ollama
func (dm *DownloadManager) readProgress(modelName string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	
	for scanner.Scan() {
		line := scanner.Text()
		
		// Parse different ollama output formats
		if strings.Contains(line, "pulling") {
			dm.updateProgress(modelName, 0, 0, 0, "Initializing download...")
		} else if strings.Contains(line, "%") {
			// Parse progress line
			dm.parseProgressLine(modelName, line)
		} else if strings.Contains(line, "success") || strings.Contains(line, "complete") {
			dm.updateProgress(modelName, 100, 0, 0, "Download complete")
		}
	}
}

// parseProgressLine parses a progress line from ollama output
func (dm *DownloadManager) parseProgressLine(modelName string, line string) {
	// Ollama typically outputs lines like:
	// "pulling manifest"
	// "pulling sha256:abc123... 100% 45 MB/152 MB 5.2 MB/s 20s"
	
	// Try to extract percentage, sizes, and speed
	parts := strings.Fields(line)
	
	var percent float64
	var currentMB, totalMB float64
	var speed string
	
	for i, part := range parts {
		// Look for percentage
		if strings.HasSuffix(part, "%") {
			fmt.Sscanf(part, "%f%%", &percent)
		}
		
		// Look for size info (e.g., "45 MB/152 MB")
		if i < len(parts)-1 && parts[i+1] == "MB" {
			fmt.Sscanf(part, "%f", &currentMB)
			
			// Check for total size
			if i+2 < len(parts) && strings.HasPrefix(parts[i+2], "/") {
				if i+3 < len(parts) {
					totalStr := strings.TrimPrefix(parts[i+2], "/")
					fmt.Sscanf(totalStr, "%f", &totalMB)
				}
			}
		}
		
		// Look for speed (e.g., "5.2 MB/s")
		if strings.HasSuffix(part, "/s") {
			if i > 0 {
				speed = parts[i-1] + " " + part
			}
		}
	}
	
	// Convert MB to bytes
	currentBytes := int64(currentMB * 1024 * 1024)
	totalBytes := int64(totalMB * 1024 * 1024)
	
	// Calculate ETA if we have speed
	eta := ""
	if speed != "" && totalBytes > currentBytes {
		remaining := totalBytes - currentBytes
		// Parse speed to calculate ETA
		var speedVal float64
		var speedUnit string
		fmt.Sscanf(speed, "%f %s", &speedVal, &speedUnit)
		
		if speedUnit == "MB/s" && speedVal > 0 {
			remainingMB := float64(remaining) / (1024 * 1024)
			secondsRemaining := remainingMB / speedVal
			eta = formatDuration(time.Duration(secondsRemaining) * time.Second)
		}
	}
	
	dm.updateProgress(modelName, percent, currentBytes, totalBytes, line)
	
	// Update speed and ETA
	dm.mu.Lock()
	if progress, exists := dm.downloads[modelName]; exists {
		if speed != "" {
			progress.Speed = speed
		}
		if eta != "" {
			progress.ETA = eta
		}
	}
	dm.mu.Unlock()
}

// updateProgress updates the progress for a model
func (dm *DownloadManager) updateProgress(modelName string, percent float64, current, total int64, status string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if progress, exists := dm.downloads[modelName]; exists {
		progress.Progress = percent
		progress.CurrentBytes = current
		progress.TotalBytes = total
		progress.LastUpdate = time.Now()
		
		// Calculate speed if we have byte information
		if current > 0 && progress.StartTime.Unix() > 0 {
			elapsed := time.Since(progress.StartTime).Seconds()
			if elapsed > 0 {
				bytesPerSecond := float64(current) / elapsed
				progress.Speed = formatSpeed(bytesPerSecond)
				
				// Calculate ETA
				if total > current && bytesPerSecond > 0 {
					remainingBytes := total - current
					remainingSeconds := float64(remainingBytes) / bytesPerSecond
					progress.ETA = formatDuration(time.Duration(remainingSeconds) * time.Second)
				}
			}
		}
	}
}

// CancelDownload cancels an active download
func (dm *DownloadManager) CancelDownload(modelName string) error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if cmd, exists := dm.activeDownloads[modelName]; exists {
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to cancel download: %w", err)
		}
		
		if progress, exists := dm.downloads[modelName]; exists {
			progress.State = DownloadStateCancelled
			progress.LastUpdate = time.Now()
		}
		
		delete(dm.activeDownloads, modelName)
	}

	return nil
}

// GetProgress returns the current progress for a model
func (dm *DownloadManager) GetProgress(modelName string) *DownloadProgress {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if progress, exists := dm.downloads[modelName]; exists {
		// Return a copy to avoid race conditions
		p := *progress
		return &p
	}

	return nil
}

// GetAllProgress returns progress for all downloads
func (dm *DownloadManager) GetAllProgress() map[string]*DownloadProgress {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]*DownloadProgress)
	for name, progress := range dm.downloads {
		p := *progress
		result[name] = &p
	}

	return result
}

// GetActiveDownloads returns only active downloads
func (dm *DownloadManager) GetActiveDownloads() map[string]*DownloadProgress {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make(map[string]*DownloadProgress)
	for name, progress := range dm.downloads {
		if progress.State == DownloadStateDownloading || progress.State == DownloadStateQueued {
			p := *progress
			result[name] = &p
		}
	}

	return result
}

// ClearCompleted removes completed downloads from tracking
func (dm *DownloadManager) ClearCompleted() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	for name, progress := range dm.downloads {
		if progress.State == DownloadStateCompleted {
			delete(dm.downloads, name)
		}
	}
}

// setDownloadError sets an error for a download
func (dm *DownloadManager) setDownloadError(modelName string, err error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if progress, exists := dm.downloads[modelName]; exists {
		progress.State = DownloadStateFailed
		progress.Error = err.Error()
		progress.LastUpdate = time.Now()
	}
}

// Helper functions

func formatSpeed(bytesPerSecond float64) string {
	if bytesPerSecond < 1024 {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	} else if bytesPerSecond < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSecond/1024)
	} else if bytesPerSecond < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB/s", bytesPerSecond/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB/s", bytesPerSecond/(1024*1024*1024))
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dh", hours)
}

// GetInstalledModels returns a list of installed models from Ollama
func (dm *DownloadManager) GetInstalledModels() ([]string, error) {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range result.Models {
		models = append(models, m.Name)
	}

	return models, nil
}