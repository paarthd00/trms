package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ModelCategory represents a category of models
type ModelCategory struct {
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
}

// ModelEntry represents a model from the JSON file
type ModelEntry struct {
	Name        string  `json:"name"`
	Size        string  `json:"size"`
	MemoryGB    float64 `json:"memory_gb"`
	Description string  `json:"description"`
}

// ModelDatabase represents the complete model database
type ModelDatabase struct {
	Models     map[string][]ModelEntry       `json:"models"`
	Categories map[string]ModelCategory `json:"categories"`
}

// LoadModelsFromJSON loads models from the JSON file
func LoadModelsFromJSON() ([]ModelInfo, error) {
	// Get the executable directory
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)
	
	// Try different possible locations for the JSON file
	possiblePaths := []string{
		filepath.Join(execDir, "models.json"),
		"models.json",
		"./models.json",
		"/home/p/Projects/trms/models.json", // Development path
	}
	
	var jsonFile string
	var data []byte
	
	for _, path := range possiblePaths {
		if data, err = os.ReadFile(path); err == nil {
			jsonFile = path
			break
		}
	}
	
	if data == nil {
		// Fall back to the embedded models if JSON file not found
		fmt.Printf("Warning: models.json not found, using built-in model list\n")
		return AllModels, nil
	}
	
	fmt.Printf("Loading models from: %s\n", jsonFile)
	
	var db ModelDatabase
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("failed to parse models.json: %v", err)
	}
	
	// Convert to ModelInfo slice
	var models []ModelInfo
	
	// Process each category
	for categoryKey, categoryModels := range db.Models {
		category := db.Categories[categoryKey]
		
		// Add category header
		models = append(models, ModelInfo{
			Name:        fmt.Sprintf("%s %s", category.Icon, category.Name),
			Description: category.Description,
			Size:        "",
			MemoryGB:    0,
			Tags:        []string{"header", categoryKey},
		})
		
		// Add models in this category
		for _, model := range categoryModels {
			tags := []string{categoryKey}
			
			// Add size-based tags
			if model.MemoryGB <= 4 {
				tags = append(tags, "small", "recommended")
			} else if model.MemoryGB <= 8 {
				tags = append(tags, "medium")
			} else if model.MemoryGB <= 16 {
				tags = append(tags, "large")
			} else {
				tags = append(tags, "xlarge")
			}
			
			// Add special tags based on name
			if contains(model.Name, []string{"phi", "tinyllama", "orca-mini"}) {
				tags = append(tags, "efficient")
			}
			if contains(model.Name, []string{"llama", "mistral", "qwen"}) {
				tags = append(tags, "popular")
			}
			
			models = append(models, ModelInfo{
				Name:        model.Name,
				Description: model.Description,
				Size:        model.Size,
				MemoryGB:    model.MemoryGB,
				Tags:        tags,
			})
		}
		
		// Add separator after each category (except the last one)
		models = append(models, ModelInfo{
			Name:        "separator",
			Description: "",
			Size:        "",
			MemoryGB:    0,
			Tags:        []string{"separator"},
		})
	}
	
	fmt.Printf("Loaded %d models across %d categories\n", 
		len(models)-len(db.Categories)-len(db.Categories), // Subtract headers and separators
		len(db.Categories))
	
	return models, nil
}

// GetModelsByCategory returns models grouped by category
func GetModelsByCategory() (map[string][]ModelInfo, map[string]ModelCategory, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)
	
	possiblePaths := []string{
		filepath.Join(execDir, "models.json"),
		"models.json",
		"./models.json",
		"/home/p/Projects/trms/models.json",
	}
	
	var data []byte
	for _, path := range possiblePaths {
		if data, err = os.ReadFile(path); err == nil {
			break
		}
	}
	
	if data == nil {
		return nil, nil, fmt.Errorf("models.json not found")
	}
	
	var db ModelDatabase
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, nil, fmt.Errorf("failed to parse models.json: %v", err)
	}
	
	// Convert to our format
	categories := make(map[string][]ModelInfo)
	for categoryKey, categoryModels := range db.Models {
		var models []ModelInfo
		for _, model := range categoryModels {
			models = append(models, ModelInfo{
				Name:        model.Name,
				Description: model.Description,
				Size:        model.Size,
				MemoryGB:    model.MemoryGB,
				Tags:        []string{categoryKey},
			})
		}
		categories[categoryKey] = models
	}
	
	return categories, db.Categories, nil
}

// contains checks if a string contains any of the substrings
func contains(str string, substrs []string) bool {
	for _, substr := range substrs {
		if len(str) >= len(substr) {
			for i := 0; i <= len(str)-len(substr); i++ {
				if str[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// Initialize models on package load
func init() {
	// Use hardcoded models for now to ensure compatibility
	// if loadedModels, err := LoadModelsFromJSON(); err == nil {
	//	AllModels = loadedModels
	// }
}