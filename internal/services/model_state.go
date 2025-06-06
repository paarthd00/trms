package services

import (
	"fmt"
	"sync"
	"time"
	
	"trms/internal/models"
)

// CleanModelState represents the simple state of a model
type CleanModelState string

const (
	CleanModelStateNotInstalled CleanModelState = "not_installed"
	CleanModelStateDownloading  CleanModelState = "downloading"
	CleanModelStateInstalled    CleanModelState = "installed"
	CleanModelStateUpdating     CleanModelState = "updating"
)

// CleanModelInfo represents comprehensive information about a model
type CleanModelInfo struct {
	Name          string
	State         CleanModelState
	Size          int64
	InstalledSize int64
	Description   string
	LastUsed      time.Time
	InstallDate   time.Time
	
	// Download info (only valid when downloading)
	DownloadProgress *DownloadProgress
}

// ModelStateManager manages the state of all models
type ModelStateManager struct {
	mu              sync.RWMutex
	models          map[string]*CleanModelInfo
	downloadManager *DownloadManager
	ollamaService   *OllamaService
}

// NewModelStateManager creates a new model state manager
func NewModelStateManager(downloadManager *DownloadManager, ollamaService *OllamaService) *ModelStateManager {
	return &ModelStateManager{
		models:          make(map[string]*CleanModelInfo),
		downloadManager: downloadManager,
		ollamaService:   ollamaService,
	}
}

// RefreshStates refreshes the state of all models
func (msm *ModelStateManager) RefreshStates() error {
	msm.mu.Lock()
	defer msm.mu.Unlock()

	// Clear existing states
	msm.models = make(map[string]*CleanModelInfo)

	// Get installed models from Ollama
	installedModels, err := msm.downloadManager.GetInstalledModels()
	if err != nil {
		// If Ollama is not running, just continue with empty installed list
		installedModels = []string{}
	}

	// Create a map for quick lookup
	installedMap := make(map[string]bool)
	for _, model := range installedModels {
		installedMap[model] = true
	}

	// Add all available models from our catalog
	for _, catalogModel := range models.AllModels {
		info := &CleanModelInfo{
			Name:        catalogModel.Name,
			State:       CleanModelStateNotInstalled,
			Size:        parseSize(catalogModel.Size),
			Description: catalogModel.Description,
		}

		// Check if installed
		if installedMap[catalogModel.Name] {
			info.State = CleanModelStateInstalled
			info.InstallDate = time.Now() // We don't track actual install date yet
			info.InstalledSize = info.Size // Approximate
		}

		msm.models[catalogModel.Name] = info
	}

	// Add any installed models not in our catalog
	for _, installedModel := range installedModels {
		if _, exists := msm.models[installedModel]; !exists {
			msm.models[installedModel] = &CleanModelInfo{
				Name:          installedModel,
				State:         CleanModelStateInstalled,
				Description:   "Custom or external model",
				InstallDate:   time.Now(),
			}
		}
	}

	// Update states based on active downloads
	activeDownloads := msm.downloadManager.GetAllProgress()
	for modelName, progress := range activeDownloads {
		if model, exists := msm.models[modelName]; exists {
			switch progress.State {
			case DownloadStateDownloading, DownloadStateQueued:
				model.State = CleanModelStateDownloading
				model.DownloadProgress = progress
			case DownloadStateCompleted:
				model.State = CleanModelStateInstalled
				model.InstallDate = time.Now()
				model.DownloadProgress = nil
			}
		}
	}

	return nil
}

// GetModelInfo returns information about a specific model
func (msm *ModelStateManager) GetModelInfo(modelName string) *CleanModelInfo {
	msm.mu.RLock()
	defer msm.mu.RUnlock()

	if info, exists := msm.models[modelName]; exists {
		// Return a copy
		modelCopy := *info
		
		// Update download progress if downloading
		if info.State == CleanModelStateDownloading {
			if progress := msm.downloadManager.GetProgress(info.Name); progress != nil {
				modelCopy.DownloadProgress = progress
			}
		}
		
		return &modelCopy
	}

	return nil
}

// GetAllModels returns all models with their current states
func (msm *ModelStateManager) GetAllModels() []*CleanModelInfo {
	msm.mu.RLock()
	defer msm.mu.RUnlock()

	var result []*CleanModelInfo
	for _, info := range msm.models {
		modelCopy := *info
		
		// Update download progress if downloading
		if info.State == CleanModelStateDownloading {
			if progress := msm.downloadManager.GetProgress(info.Name); progress != nil {
				modelCopy.DownloadProgress = progress
			}
		}
		
		result = append(result, &modelCopy)
	}

	return result
}

// GetModelsByState returns models in a specific state
func (msm *ModelStateManager) GetModelsByState(state CleanModelState) []*CleanModelInfo {
	msm.mu.RLock()
	defer msm.mu.RUnlock()

	var result []*CleanModelInfo
	for _, info := range msm.models {
		if info.State == state {
			modelCopy := *info
			
			// Update download progress if downloading
			if info.State == CleanModelStateDownloading {
				if progress := msm.downloadManager.GetProgress(info.Name); progress != nil {
					modelCopy.DownloadProgress = progress
				}
			}
			
			result = append(result, &modelCopy)
		}
	}

	return result
}

// StartDownload initiates a model download
func (msm *ModelStateManager) StartDownload(modelName string) error {
	// Start the download
	if err := msm.downloadManager.StartDownload(modelName); err != nil {
		return err
	}

	// Update state
	msm.mu.Lock()
	if info, exists := msm.models[modelName]; exists {
		info.State = CleanModelStateDownloading
	}
	msm.mu.Unlock()

	return nil
}

// CancelDownload cancels a model download
func (msm *ModelStateManager) CancelDownload(modelName string) error {
	// Cancel the download
	if err := msm.downloadManager.CancelDownload(modelName); err != nil {
		return err
	}

	// Update state
	msm.mu.Lock()
	if info, exists := msm.models[modelName]; exists {
		info.State = CleanModelStateNotInstalled
		info.DownloadProgress = nil
	}
	msm.mu.Unlock()

	return nil
}

// DeleteModel deletes an installed model
func (msm *ModelStateManager) DeleteModel(modelName string) error {
	// Delete via Ollama
	if err := msm.ollamaService.DeleteModel(modelName); err != nil {
		return err
	}

	// Update state
	msm.mu.Lock()
	if info, exists := msm.models[modelName]; exists {
		info.State = CleanModelStateNotInstalled
		info.InstallDate = time.Time{}
		info.InstalledSize = 0
	}
	msm.mu.Unlock()

	return nil
}

// SetModelUsed marks a model as recently used
func (msm *ModelStateManager) SetModelUsed(modelName string) {
	msm.mu.Lock()
	defer msm.mu.Unlock()

	if info, exists := msm.models[modelName]; exists {
		info.LastUsed = time.Now()
	}
}

// parseSize converts a size string like "3.8GB" to bytes
func parseSize(sizeStr string) int64 {
	if sizeStr == "" {
		return 0
	}

	var num float64
	var unit string
	fmt.Sscanf(sizeStr, "%f%s", &num, &unit)

	multiplier := int64(1)
	switch unit {
	case "KB", "kb":
		multiplier = 1024
	case "MB", "mb":
		multiplier = 1024 * 1024
	case "GB", "gb":
		multiplier = 1024 * 1024 * 1024
	case "TB", "tb":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(num * float64(multiplier))
}