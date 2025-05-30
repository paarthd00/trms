package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type OllamaModel struct {
	Name       string
	Size       string
	Modified   time.Time
	Digest     string
}

type OllamaManager struct {
	currentModel string
	models       []OllamaModel
}

func NewOllamaManager() *OllamaManager {
	return &OllamaManager{
		currentModel: "llama2",
		models:       []OllamaModel{},
	}
}

func (om *OllamaManager) IsInstalled() bool {
	cmd := exec.Command("which", "ollama")
	err := cmd.Run()
	return err == nil
}

func (om *OllamaManager) IsRunning() bool {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func (om *OllamaManager) InstallOllama() error {
	// Download and run the install script
	cmd := exec.Command("sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
	cmd.Stdout = nil
	cmd.Stderr = nil
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Ollama: %w", err)
	}
	
	return nil
}

func (om *OllamaManager) StartService() error {
	cmd := exec.Command("ollama", "serve")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Ollama: %w", err)
	}
	
	// Give it a moment to start
	time.Sleep(2 * time.Second)
	
	if !om.IsRunning() {
		return fmt.Errorf("Ollama failed to start")
	}
	
	return nil
}

func (om *OllamaManager) RefreshModels() error {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return fmt.Errorf("failed to get models: %w", err)
	}
	defer resp.Body.Close()
	
	var data struct {
		Models []struct {
			Name       string    `json:"name"`
			Size       int64     `json:"size"`
			Modified   time.Time `json:"modified_at"`
			Digest     string    `json:"digest"`
		} `json:"models"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to parse models: %w", err)
	}
	
	om.models = []OllamaModel{}
	for _, m := range data.Models {
		// Skip duplicate model names (different tags)
		baseName := strings.Split(m.Name, ":")[0]
		exists := false
		for _, existing := range om.models {
			if existing.Name == baseName {
				exists = true
				break
			}
		}
		if !exists {
			om.models = append(om.models, OllamaModel{
				Name:     baseName,
				Size:     formatBytes(m.Size),
				Modified: m.Modified,
				Digest:   m.Digest,
			})
		}
	}
	
	return nil
}

func (om *OllamaManager) GetModels() []OllamaModel {
	return om.models
}

func (om *OllamaManager) SetCurrentModel(model string) {
	om.currentModel = model
}

func (om *OllamaManager) GetCurrentModel() string {
	return om.currentModel
}

func (om *OllamaManager) PullModel(model string) error {
	cmd := exec.Command("ollama", "pull", model)
	return cmd.Run()
}

func (om *OllamaManager) Chat(prompt string) (string, error) {
	payload := map[string]interface{}{
		"model":  om.currentModel,
		"prompt": prompt,
		"stream": false,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("Ollama not running: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != "" {
		// Check if model needs to be pulled
		if strings.Contains(result.Error, "model") && strings.Contains(result.Error, "not found") {
			return fmt.Sprintf("Model '%s' not found. Pull it with: ollama pull %s", om.currentModel, om.currentModel), nil
		}
		return "", fmt.Errorf("Ollama error: %s", result.Error)
	}

	if result.Response == "" {
		return "No response from Ollama", nil
	}

	return result.Response, nil
}

// Popular Ollama models
var PopularModels = []string{
	"llama2",
	"mistral",
	"codellama",
	"gemma",
	"neural-chat",
	"starling-lm",
	"orca-mini",
	"vicuna",
	"llava",
	"phi",
}

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