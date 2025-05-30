package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
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
	currentModel  string
	models        []OllamaModel
	pullContext   context.Context
	pullCancel    context.CancelFunc
	pullProgress  *PullProgress
}

type PullProgress struct {
	Model       string
	Status      string
	Percent     int
	Downloaded  string
	Total       string
	Speed       string
}

func NewOllamaManager() *OllamaManager {
	return &OllamaManager{
		currentModel: "", // Will be set to first available model
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
		// Keep the full model name including tags
		om.models = append(om.models, OllamaModel{
			Name:     m.Name,
			Size:     formatBytes(m.Size),
			Modified: m.Modified,
			Digest:   m.Digest,
		})
	}
	
	// Set current model to first available if none is set
	if om.currentModel == "" && len(om.models) > 0 {
		om.currentModel = om.models[0].Name
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
	if om.currentModel == "" {
		return "No model selected"
	}
	return om.currentModel
}

func (om *OllamaManager) PullModel(model string) error {
	// Create cancellable context
	om.pullContext, om.pullCancel = context.WithCancel(context.Background())
	om.pullProgress = &PullProgress{
		Model:   model,
		Status:  "Starting download...",
		Percent: 0,
	}

	cmd := exec.CommandContext(om.pullContext, "ollama", "pull", model)
	
	// Get stdout pipe for progress tracking
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Parse progress output
	scanner := bufio.NewScanner(stdout)
	progressRegex := regexp.MustCompile(`(\d+\.?\d*)\s*([KMGT]?B)\s*/\s*(\d+\.?\d*)\s*([KMGT]?B)\s*(\d+%)`)
	
	for scanner.Scan() {
		line := scanner.Text()
		
		if matches := progressRegex.FindStringSubmatch(line); matches != nil {
			downloaded := matches[1] + " " + matches[2]
			total := matches[3] + " " + matches[4]
			percentStr := matches[5]
			
			// Parse percentage
			percentStr = strings.TrimSuffix(percentStr, "%")
			if percent, err := strconv.Atoi(percentStr); err == nil {
				om.pullProgress.Downloaded = downloaded
				om.pullProgress.Total = total
				om.pullProgress.Percent = percent
				om.pullProgress.Status = fmt.Sprintf("Downloading %s", model)
			}
		} else if strings.Contains(line, "pulling") {
			om.pullProgress.Status = "Pulling manifest..."
		} else if strings.Contains(line, "verifying") {
			om.pullProgress.Status = "Verifying sha256..."
		} else if strings.Contains(line, "writing") {
			om.pullProgress.Status = "Writing manifest..."
		} else if strings.Contains(line, "success") {
			om.pullProgress.Status = "Download complete!"
			om.pullProgress.Percent = 100
		}
	}

	err = cmd.Wait()
	
	// Clean up
	om.pullProgress = nil
	om.pullCancel = nil
	
	return err
}

func (om *OllamaManager) CancelPull() error {
	if om.pullCancel != nil {
		om.pullCancel()
		om.pullProgress = nil
		return nil
	}
	return fmt.Errorf("no active pull to cancel")
}

func (om *OllamaManager) GetPullProgress() *PullProgress {
	return om.pullProgress
}

func (om *OllamaManager) IsPulling() bool {
	return om.pullProgress != nil
}

func (om *OllamaManager) DeleteModel(model string) error {
	cmd := exec.Command("ollama", "rm", model)
	return cmd.Run()
}

func (om *OllamaManager) GetModelSize(model string) (string, error) {
	// Get detailed model info
	cmd := exec.Command("ollama", "show", model)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	// Parse size from output - this is a simple approach
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Size") || strings.Contains(line, "size") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[len(parts)-1], nil
			}
		}
	}
	
	return "Unknown", nil
}

func (om *OllamaManager) FetchAvailableModels() error {
	// Use comprehensive fallback models list with latest models
	AllModels = []AvailableModel{
		// Latest and most popular models
		{"llama3.2", "Meta's latest Llama 3.2 model", "2.0GB", []string{"text", "chat"}, "2024-09-01"},
		{"llama3.2:1b", "Llama 3.2 1B - Ultra efficient", "1.3GB", []string{"text", "chat"}, "2024-09-01"},
		{"llama3.2:3b", "Llama 3.2 3B - Balanced performance", "2.0GB", []string{"text", "chat"}, "2024-09-01"},
		{"llama3.1", "Meta's Llama 3.1 - Enhanced reasoning", "4.7GB", []string{"text", "chat"}, "2024-07-01"},
		{"llama3.1:8b", "Llama 3.1 8B - Standard size", "4.7GB", []string{"text", "chat"}, "2024-07-01"},
		{"llama3.1:70b", "Llama 3.1 70B - Most capable", "40GB", []string{"text", "chat"}, "2024-07-01"},
		{"llama3.1:405b", "Llama 3.1 405B - Flagship model", "231GB", []string{"text", "chat"}, "2024-07-01"},
		{"llama3", "Meta's Llama 3 model", "4.7GB", []string{"text", "chat"}, "2024-04-01"},
		{"llama3:8b", "Llama 3 8B", "4.7GB", []string{"text", "chat"}, "2024-04-01"},
		{"llama3:70b", "Llama 3 70B", "40GB", []string{"text", "chat"}, "2024-04-01"},
		{"llama2", "Meta's LLaMA 2 - General purpose", "3.8GB", []string{"text", "chat"}, "2024-01-01"},
		{"llama2:7b", "LLaMA 2 7B", "3.8GB", []string{"text", "chat"}, "2024-01-01"},
		{"llama2:13b", "LLaMA 2 13B", "7.4GB", []string{"text", "chat"}, "2024-01-01"},
		{"llama2:70b", "LLaMA 2 70B", "39GB", []string{"text", "chat"}, "2024-01-01"},
		
		// Qwen models (Alibaba)
		{"qwen2.5", "Qwen 2.5 - Latest from Alibaba", "4.1GB", []string{"text", "chat", "multilingual"}, "2024-09-01"},
		{"qwen2.5:7b", "Qwen 2.5 7B", "4.1GB", []string{"text", "chat", "multilingual"}, "2024-09-01"},
		{"qwen2.5:14b", "Qwen 2.5 14B", "8.0GB", []string{"text", "chat", "multilingual"}, "2024-09-01"},
		{"qwen2.5:32b", "Qwen 2.5 32B", "18GB", []string{"text", "chat", "multilingual"}, "2024-09-01"},
		{"qwen2.5:72b", "Qwen 2.5 72B", "41GB", []string{"text", "chat", "multilingual"}, "2024-09-01"},
		{"qwen2", "Qwen 2 - Multilingual excellence", "4.1GB", []string{"text", "chat", "multilingual"}, "2024-06-01"},
		{"qwen2:0.5b", "Qwen 2 0.5B - Ultra compact", "395MB", []string{"text", "chat"}, "2024-06-01"},
		{"qwen2:1.5b", "Qwen 2 1.5B - Efficient", "934MB", []string{"text", "chat"}, "2024-06-01"},
		{"qwen2:7b", "Qwen 2 7B", "4.1GB", []string{"text", "chat", "multilingual"}, "2024-06-01"},
		{"qwen2:72b", "Qwen 2 72B", "41GB", []string{"text", "chat", "multilingual"}, "2024-06-01"},
		
		// Mistral models
		{"mistral-nemo", "Mistral Nemo - 12B parameter model", "6.8GB", []string{"text", "chat"}, "2024-07-01"},
		{"mistral", "Mistral 7B - Fast and efficient", "4.1GB", []string{"text", "chat"}, "2024-01-01"},
		{"mistral:7b", "Mistral 7B", "4.1GB", []string{"text", "chat"}, "2024-01-01"},
		{"mixtral", "Mixtral 8x7B - Mixture of experts", "26GB", []string{"text", "chat"}, "2024-01-01"},
		{"mixtral:8x7b", "Mixtral 8x7B", "26GB", []string{"text", "chat"}, "2024-01-01"},
		{"mixtral:8x22b", "Mixtral 8x22B - Largest Mixtral", "80GB", []string{"text", "chat"}, "2024-04-01"},
		
		// Gemma models (Google)
		{"gemma2", "Google's Gemma 2 - Enhanced", "4.8GB", []string{"text", "chat"}, "2024-06-01"},
		{"gemma2:2b", "Gemma 2 2B", "1.6GB", []string{"text", "chat"}, "2024-06-01"},
		{"gemma2:9b", "Gemma 2 9B", "5.4GB", []string{"text", "chat"}, "2024-06-01"},
		{"gemma2:27b", "Gemma 2 27B", "16GB", []string{"text", "chat"}, "2024-06-01"},
		{"gemma", "Google's original Gemma", "4.8GB", []string{"text", "chat"}, "2024-02-01"},
		{"gemma:2b", "Gemma 2B", "1.4GB", []string{"text", "chat"}, "2024-02-01"},
		{"gemma:7b", "Gemma 7B", "4.8GB", []string{"text", "chat"}, "2024-02-01"},
		{"codegemma", "CodeGemma - Code specialist", "5.0GB", []string{"code"}, "2024-04-01"},
		{"codegemma:2b", "CodeGemma 2B", "1.6GB", []string{"code"}, "2024-04-01"},
		{"codegemma:7b", "CodeGemma 7B", "5.0GB", []string{"code"}, "2024-04-01"},
		
		// Microsoft Phi models
		{"phi3", "Microsoft's Phi-3 - Compact power", "2.2GB", []string{"text", "chat"}, "2024-04-01"},
		{"phi3:3.8b", "Phi-3 Mini", "2.2GB", []string{"text", "chat"}, "2024-04-01"},
		{"phi3:14b", "Phi-3 Medium", "7.9GB", []string{"text", "chat"}, "2024-04-01"},
		{"phi3.5", "Phi-3.5 - Latest version", "2.2GB", []string{"text", "chat"}, "2024-08-01"},
		{"phi", "Microsoft's original Phi", "1.6GB", []string{"text", "chat"}, "2024-01-01"},
		
		// Code-specialized models
		{"codellama", "Code Llama - Code generation", "3.8GB", []string{"code"}, "2024-01-01"},
		{"codellama:7b", "Code Llama 7B", "3.8GB", []string{"code"}, "2024-01-01"},
		{"codellama:13b", "Code Llama 13B", "7.4GB", []string{"code"}, "2024-01-01"},
		{"codellama:34b", "Code Llama 34B", "19GB", []string{"code"}, "2024-01-01"},
		{"deepseek-coder", "DeepSeek Coder - Advanced coding", "4.1GB", []string{"code"}, "2024-01-01"},
		{"deepseek-coder:1.3b", "DeepSeek Coder 1.3B", "776MB", []string{"code"}, "2024-01-01"},
		{"deepseek-coder:6.7b", "DeepSeek Coder 6.7B", "4.1GB", []string{"code"}, "2024-01-01"},
		{"deepseek-coder:33b", "DeepSeek Coder 33B", "19GB", []string{"code"}, "2024-01-01"},
		{"starcoder2", "StarCoder2 - GitHub Copilot base", "4.1GB", []string{"code"}, "2024-02-01"},
		{"starcoder2:3b", "StarCoder2 3B", "1.7GB", []string{"code"}, "2024-02-01"},
		{"starcoder2:7b", "StarCoder2 7B", "4.1GB", []string{"code"}, "2024-02-01"},
		{"starcoder2:15b", "StarCoder2 15B", "8.7GB", []string{"code"}, "2024-02-01"},
		
		// Vision models
		{"llava", "LLaVA - Vision and language", "4.5GB", []string{"vision", "multimodal"}, "2024-01-01"},
		{"llava:7b", "LLaVA 7B", "4.5GB", []string{"vision", "multimodal"}, "2024-01-01"},
		{"llava:13b", "LLaVA 13B", "8.0GB", []string{"vision", "multimodal"}, "2024-01-01"},
		{"llava:34b", "LLaVA 34B", "20GB", []string{"vision", "multimodal"}, "2024-01-01"},
		{"llava-llama3", "LLaVA with Llama 3", "5.5GB", []string{"vision", "multimodal"}, "2024-04-01"},
		{"llava-phi3", "LLaVA with Phi-3", "3.2GB", []string{"vision", "multimodal"}, "2024-05-01"},
		{"moondream", "Moondream - Tiny vision model", "829MB", []string{"vision", "multimodal"}, "2024-01-01"},
		{"bakllava", "BakLLaVA - Enhanced vision", "4.5GB", []string{"vision", "multimodal"}, "2024-01-01"},
		
		// Specialized and fine-tuned models
		{"dolphin-mistral", "Dolphin Mistral - Uncensored", "4.1GB", []string{"text", "chat"}, "2024-01-01"},
		{"dolphin-llama3", "Dolphin Llama 3 - Uncensored", "4.7GB", []string{"text", "chat"}, "2024-04-01"},
		{"dolphin-phi", "Dolphin Phi - Uncensored compact", "1.6GB", []string{"text", "chat"}, "2024-01-01"},
		{"nous-hermes2", "Nous Hermes 2 - Enhanced reasoning", "4.1GB", []string{"chat", "instruct"}, "2024-01-01"},
		{"nous-hermes2:10.7b", "Nous Hermes 2 10.7B", "6.1GB", []string{"chat", "instruct"}, "2024-01-01"},
		{"nous-hermes2:34b", "Nous Hermes 2 34B", "19GB", []string{"chat", "instruct"}, "2024-01-01"},
		{"openhermes", "OpenHermes - Instruction tuned", "4.1GB", []string{"chat", "instruct"}, "2024-01-01"},
		{"neural-chat", "Intel's Neural Chat", "4.1GB", []string{"chat"}, "2024-01-01"},
		{"starling-lm", "Starling LM - Berkeley RLHF", "4.1GB", []string{"chat"}, "2024-01-01"},
		{"zephyr", "Zephyr - HuggingFace DPO", "4.1GB", []string{"chat"}, "2024-01-01"},
		{"openchat", "OpenChat - ChatGPT alternative", "4.1GB", []string{"chat"}, "2024-01-01"},
		{"openchat:7b", "OpenChat 7B", "4.1GB", []string{"chat"}, "2024-01-01"},
		{"vicuna", "Vicuna - Conversation specialist", "3.8GB", []string{"chat"}, "2024-01-01"},
		{"orca-mini", "Orca Mini - Reasoning focused", "1.9GB", []string{"text", "chat"}, "2024-01-01"},
		{"wizardlm2", "WizardLM 2 - Instruction following", "4.1GB", []string{"chat", "instruct"}, "2024-01-01"},
		{"wizardlm2:7b", "WizardLM 2 7B", "4.1GB", []string{"chat", "instruct"}, "2024-01-01"},
		
		// Compact models
		{"tinyllama", "TinyLlama - 1.1B parameters", "638MB", []string{"text"}, "2024-01-01"},
		{"tinydolphin", "TinyDolphin - Compact uncensored", "636MB", []string{"text", "chat"}, "2024-01-01"},
		
		// Enterprise and specialized
		{"command-r", "Command R - Cohere's model", "20GB", []string{"text", "chat", "rag"}, "2024-03-01"},
		{"command-r-plus", "Command R+ - Enhanced version", "52GB", []string{"text", "chat", "rag"}, "2024-04-01"},
		{"dbrx", "DBRX - Databricks MoE", "70GB", []string{"text", "chat"}, "2024-03-01"},
		{"arctic", "Snowflake Arctic - Efficient MoE", "17GB", []string{"text", "chat"}, "2024-04-01"},
		{"solar", "Solar - SOLAR 10.7B", "6.1GB", []string{"text", "chat"}, "2024-01-01"},
		{"yi", "Yi - 01.ai multilingual", "4.1GB", []string{"text", "chat", "multilingual"}, "2024-01-01"},
		{"yi:6b", "Yi 6B", "3.5GB", []string{"text", "chat", "multilingual"}, "2024-01-01"},
		{"yi:9b", "Yi 9B", "5.1GB", []string{"text", "chat", "multilingual"}, "2024-01-01"},
		{"yi:34b", "Yi 34B", "19GB", []string{"text", "chat", "multilingual"}, "2024-01-01"},
		{"internlm2", "InternLM 2 - Shanghai AI Lab", "4.1GB", []string{"text", "chat"}, "2024-01-01"},
		{"internlm2:7b", "InternLM 2 7B", "4.1GB", []string{"text", "chat"}, "2024-01-01"},
		{"internlm2:20b", "InternLM 2 20B", "11GB", []string{"text", "chat"}, "2024-01-01"},
		
		// Embedding models
		{"nomic-embed-text", "Nomic Embed Text - Embeddings", "274MB", []string{"embedding"}, "2024-01-01"},
		{"all-minilm", "All-MiniLM-L6-v2 - Sentence embeddings", "91MB", []string{"embedding"}, "2024-01-01"},
		{"mxbai-embed-large", "MixedBread AI Large Embeddings", "334MB", []string{"embedding"}, "2024-01-01"},
		
		// Math and reasoning specialists
		{"mathstral", "Mathstral - Mistral's math model", "4.1GB", []string{"math", "reasoning"}, "2024-07-01"},
		{"wizard-math", "WizardMath - Mathematical reasoning", "3.8GB", []string{"math", "reasoning"}, "2024-01-01"},
		{"deepseek-math", "DeepSeek Math - Mathematical problem solving", "4.1GB", []string{"math", "reasoning"}, "2024-01-01"},
	}
	return nil
}

// Enhanced chat method with context support
func (om *OllamaManager) ChatWithContext(fullContext string) (string, error) {
	payload := map[string]interface{}{
		"model":  om.currentModel,
		"prompt": fullContext,
		"stream": false,
		"options": map[string]interface{}{
			"temperature": 0.7,
			"top_p":       0.9,
			"top_k":       40,
		},
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

// Enhanced Chat method that uses ChatWithContext
func (om *OllamaManager) Chat(prompt string) (string, error) {
	// For simple prompts without context, use the enhanced method
	return om.ChatWithContext(prompt)
}

type AvailableModel struct {
	Name        string
	Description string
	Size        string
	Tags        []string
	UpdatedAt   string
}

var AllModels []AvailableModel

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
