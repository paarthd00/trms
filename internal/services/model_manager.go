package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ModelState represents the state of a model
type ModelState int

const (
	ModelStateNotInstalled ModelState = iota
	ModelStateDownloading
	ModelStatePartial
	ModelStateComplete
	ModelStateCorrupted
)

// ModelStatus represents detailed model status
type ModelStatus struct {
	Name         string
	State        ModelState
	Size         int64
	Downloaded   int64
	Percent      int
	Error        string
	ManifestPath string
	BlobsPath    string
	Layers       []LayerStatus
}

// LayerStatus represents the status of a model layer
type LayerStatus struct {
	Digest     string
	Size       int64
	Downloaded int64
	Complete   bool
}

// ModelManager manages model states and partial downloads
type ModelManager struct {
	ollamaPath   string
	modelStates  map[string]*ModelStatus
	mu           sync.RWMutex
}

// NewModelManager creates a new model manager
func NewModelManager() *ModelManager {
	home, _ := os.UserHomeDir()
	ollamaPath := filepath.Join(home, ".ollama")
	
	return &ModelManager{
		ollamaPath:  ollamaPath,
		modelStates: make(map[string]*ModelStatus),
	}
}

// ScanModels scans for all models including partial downloads
func (m *ModelManager) ScanModels() (map[string]*ModelStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear existing states
	m.modelStates = make(map[string]*ModelStatus)

	// Scan manifests directory
	manifestsPath := filepath.Join(m.ollamaPath, "models", "manifests")
	if err := m.scanManifests(manifestsPath); err != nil {
		return nil, err
	}

	// Check running models via API
	if err := m.checkRunningModels(); err != nil {
		// Don't fail if API is not available
		fmt.Printf("Warning: Could not check running models: %v\n", err)
	}

	return m.modelStates, nil
}

// scanManifests scans the manifests directory for model information
func (m *ModelManager) scanManifests(manifestsPath string) error {
	// Walk through registry.ollama.ai directory
	registryPath := filepath.Join(manifestsPath, "registry.ollama.ai")
	
	err := filepath.Walk(registryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Look for manifest files
		if !info.IsDir() && strings.HasSuffix(info.Name(), "latest") {
			// Extract model name from path
			relPath, _ := filepath.Rel(registryPath, filepath.Dir(path))
			parts := strings.Split(relPath, string(os.PathSeparator))
			
			if len(parts) >= 2 {
				modelName := parts[1] // library/model format
				
				// Read manifest to get layer information
				manifest, err := m.readManifest(path)
				if err != nil {
					// Create partial status
					m.modelStates[modelName] = &ModelStatus{
						Name:         modelName,
						State:        ModelStateCorrupted,
						Error:        err.Error(),
						ManifestPath: path,
					}
				} else {
					// Check layer status
					status := m.checkModelStatus(modelName, manifest, path)
					m.modelStates[modelName] = status
				}
			}
		}
		return nil
	})

	return err
}

// readManifest reads and parses a manifest file
func (m *ModelManager) readManifest(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// checkModelStatus checks the download status of a model
func (m *ModelManager) checkModelStatus(modelName string, manifest map[string]interface{}, manifestPath string) *ModelStatus {
	status := &ModelStatus{
		Name:         modelName,
		ManifestPath: manifestPath,
		BlobsPath:    filepath.Join(m.ollamaPath, "models", "blobs"),
		Layers:       []LayerStatus{},
	}

	// Extract layers from manifest
	if layers, ok := manifest["layers"].([]interface{}); ok {
		totalSize := int64(0)
		downloadedSize := int64(0)
		allComplete := true

		for _, layer := range layers {
			if layerMap, ok := layer.(map[string]interface{}); ok {
				layerStatus := m.checkLayerStatus(layerMap, status.BlobsPath)
				status.Layers = append(status.Layers, layerStatus)
				
				totalSize += layerStatus.Size
				downloadedSize += layerStatus.Downloaded
				
				if !layerStatus.Complete {
					allComplete = false
				}
			}
		}

		status.Size = totalSize
		status.Downloaded = downloadedSize
		
		if totalSize > 0 {
			status.Percent = int(float64(downloadedSize) / float64(totalSize) * 100)
		}

		// Determine state
		if allComplete && downloadedSize == totalSize {
			status.State = ModelStateComplete
		} else if downloadedSize > 0 {
			status.State = ModelStatePartial
		} else {
			status.State = ModelStateNotInstalled
		}
	}

	return status
}

// checkLayerStatus checks the status of a single layer
func (m *ModelManager) checkLayerStatus(layer map[string]interface{}, blobsPath string) LayerStatus {
	status := LayerStatus{}

	// Extract digest
	if digest, ok := layer["digest"].(string); ok {
		status.Digest = digest
		
		// Extract size
		if size, ok := layer["size"].(float64); ok {
			status.Size = int64(size)
		}

		// Check if blob exists
		blobPath := filepath.Join(blobsPath, strings.Replace(digest, ":", "-", 1))
		if info, err := os.Stat(blobPath); err == nil {
			status.Downloaded = info.Size()
			status.Complete = status.Downloaded == status.Size
		}
	}

	return status
}

// checkRunningModels checks models via Ollama API
func (m *ModelManager) checkRunningModels() error {
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

	// Update states for running models
	for _, model := range result.Models {
		if existing, ok := m.modelStates[model.Name]; ok {
			// Update existing entry
			existing.State = ModelStateComplete
			existing.Size = model.Size
			existing.Downloaded = model.Size
			existing.Percent = 100
		} else {
			// Add new entry
			m.modelStates[model.Name] = &ModelStatus{
				Name:       model.Name,
				State:      ModelStateComplete,
				Size:       model.Size,
				Downloaded: model.Size,
				Percent:    100,
			}
		}
	}

	return nil
}

// CleanPartialDownloads removes partial downloads for a model
func (m *ModelManager) CleanPartialDownloads(modelName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, ok := m.modelStates[modelName]
	if !ok {
		return fmt.Errorf("model %s not found", modelName)
	}

	// Remove manifest
	if status.ManifestPath != "" {
		if err := os.Remove(status.ManifestPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove manifest: %w", err)
		}
	}

	// Remove blobs
	for _, layer := range status.Layers {
		if layer.Downloaded > 0 && !layer.Complete {
			blobPath := filepath.Join(status.BlobsPath, strings.Replace(layer.Digest, ":", "-", 1))
			if err := os.Remove(blobPath); err != nil && !os.IsNotExist(err) {
				fmt.Printf("Warning: failed to remove blob %s: %v\n", layer.Digest, err)
			}
		}
	}

	// Remove from state
	delete(m.modelStates, modelName)

	return nil
}

// GetModelStatus returns the status of a specific model
func (m *ModelManager) GetModelStatus(modelName string) (*ModelStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	status, ok := m.modelStates[modelName]
	return status, ok
}

// GetAllStatuses returns all model statuses
func (m *ModelManager) GetAllStatuses() map[string]*ModelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]*ModelStatus)
	for k, v := range m.modelStates {
		result[k] = v
	}
	return result
}

// ResumePull attempts to resume a partial download
func (m *ModelManager) ResumePull(modelName string) error {
	// This would integrate with Ollama's pull mechanism
	// For now, we'll just trigger a normal pull which should resume
	return fmt.Errorf("resume not implemented - use normal pull")
}

// GetDownloadQueue returns models currently being downloaded
func (m *ModelManager) GetDownloadQueue() []*ModelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var queue []*ModelStatus
	for _, status := range m.modelStates {
		if status.State == ModelStateDownloading {
			queue = append(queue, status)
		}
	}
	return queue
}

// GetPartialDownloads returns models with partial downloads
func (m *ModelManager) GetPartialDownloads() []*ModelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var partials []*ModelStatus
	for _, status := range m.modelStates {
		if status.State == ModelStatePartial {
			partials = append(partials, status)
		}
	}
	return partials
}