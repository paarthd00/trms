package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageGeneratorService handles image generation through Ollama
type ImageGeneratorService struct {
	baseURL    string
	tempDir    string
	httpClient *http.Client
}

// GenerationParameters holds image generation parameters
type GenerationParameters struct {
	Steps     int     `json:"steps"`
	Guidance  float64 `json:"guidance"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
}

// NewImageGeneratorService creates a new image generator service
func NewImageGeneratorService() *ImageGeneratorService {
	// Create temp directory for generated images
	tempDir := filepath.Join(os.TempDir(), "trms-images")
	os.MkdirAll(tempDir, 0755)

	return &ImageGeneratorService{
		baseURL: "http://localhost:11434",
		tempDir: tempDir,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // Image generation can take a while
		},
	}
}

// GenerateImageRequest represents the request for image generation
type GenerateImageRequest struct {
	Model   string                    `json:"model"`
	Prompt  string                    `json:"prompt"`
	Options map[string]interface{}    `json:"options,omitempty"`
	Format  string                    `json:"format,omitempty"`
	Stream  bool                      `json:"stream"`
}

// GenerateImageResponse represents the response from image generation
type GenerateImageResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Response  string    `json:"response,omitempty"`
	Images    []string  `json:"images,omitempty"` // Base64 encoded images
	Done      bool      `json:"done"`
}

// GenerateImage generates an image using the specified model and parameters
func (s *ImageGeneratorService) GenerateImage(model, prompt string, params GenerationParameters) (string, error) {
	// Prepare the request
	options := map[string]interface{}{
		"num_inference_steps": params.Steps,
		"guidance_scale":      params.Guidance,
		"width":              params.Width,
		"height":             params.Height,
	}

	req := GenerateImageRequest{
		Model:   model,
		Prompt:  prompt,
		Options: options,
		Format:  "json",
		Stream:  false,
	}

	// Convert to JSON
	jsonData, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	// Make the request
	resp, err := s.httpClient.Post(
		s.baseURL+"/api/generate",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var genResp GenerateImageResponse
	decoder := json.NewDecoder(resp.Body)
	
	// For streaming responses, we might get multiple JSON objects
	for decoder.More() {
		err = decoder.Decode(&genResp)
		if err != nil {
			return "", fmt.Errorf("failed to decode response: %v", err)
		}
		
		// If we have images and it's done, break
		if len(genResp.Images) > 0 && genResp.Done {
			break
		}
	}

	if len(genResp.Images) == 0 {
		return "", fmt.Errorf("no images returned from model")
	}

	// Save the first image (models typically return one image)
	imagePath, err := s.saveBase64Image(genResp.Images[0], model, prompt)
	if err != nil {
		return "", fmt.Errorf("failed to save image: %v", err)
	}

	return imagePath, nil
}

// saveBase64Image saves a base64 encoded image to a file
func (s *ImageGeneratorService) saveBase64Image(base64Data, model, prompt string) (string, error) {
	// Decode base64
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 image: %v", err)
	}

	// Generate filename
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.png", timestamp, sanitizeFilename(model))
	imagePath := filepath.Join(s.tempDir, filename)

	// Write file
	err = os.WriteFile(imagePath, imageData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write image file: %v", err)
	}

	return imagePath, nil
}

// sanitizeFilename removes invalid characters from filename
func sanitizeFilename(name string) string {
	// Replace invalid characters with underscores
	invalid := []string{":", "/", "\\", "?", "*", "<", ">", "|", "\""}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	return result
}

// IsImageGenerationModel checks if a model is for image generation
func (s *ImageGeneratorService) IsImageGenerationModel(modelName string) bool {
	imageModels := []string{
		"stable-diffusion",
		"flux",
		"flux-schnell", 
		"sdxl",
		"playground-v2.5",
		"dreamshaper",
	}
	
	for _, imgModel := range imageModels {
		if strings.Contains(strings.ToLower(modelName), strings.ToLower(imgModel)) {
			return true
		}
	}
	return false
}

// GetImageGenerationModels returns a list of available image generation models
func (s *ImageGeneratorService) GetImageGenerationModels() []string {
	return []string{
		"stable-diffusion",
		"flux",
		"flux-schnell", 
		"sdxl",
		"playground-v2.5",
		"dreamshaper",
	}
}

// CleanupTempImages removes old temporary images
func (s *ImageGeneratorService) CleanupTempImages(olderThan time.Duration) error {
	entries, err := os.ReadDir(s.tempDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-olderThan)
	
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(s.tempDir, entry.Name()))
		}
	}
	
	return nil
}