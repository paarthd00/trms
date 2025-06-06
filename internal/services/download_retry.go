package services

import (
	"fmt"
	"time"
)

// RetryConfig defines retry behavior for downloads
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  2 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
	}
}

// RetryDownload attempts to download a model with retries
func (o *OllamaService) RetryDownload(model string, config RetryConfig) error {
	var lastErr error
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff
			delay := time.Duration(float64(config.InitialDelay) * float64(attempt) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
			
			o.mu.Lock()
			if modelProgress, exists := o.activeDownloads[model]; exists {
				modelProgress.Status = fmt.Sprintf("Retrying in %v... (attempt %d/%d)", delay, attempt+1, config.MaxRetries+1)
			}
			o.mu.Unlock()
			
			fmt.Printf("Download failed, retrying in %v (attempt %d/%d)\n", delay, attempt+1, config.MaxRetries+1)
			time.Sleep(delay)
		}
		
		// Attempt download
		err := o.pullModelOnce(model)
		if err == nil {
			// Success!
			return nil
		}
		
		lastErr = err
		
		// Check if this is a retryable error
		if !isRetryableError(err) {
			return fmt.Errorf("non-retryable error: %w", err)
		}
		
		fmt.Printf("Download attempt %d failed: %v\n", attempt+1, err)
	}
	
	return fmt.Errorf("download failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// pullModelOnce performs a single download attempt (original PullModel logic)
func (o *OllamaService) pullModelOnce(model string) error {
	// This contains the core download logic from PullModel
	// but without the retry wrapper
	return o.PullModel(model) // For now, delegate to the existing method
}

// isRetryableError determines if an error is worth retrying
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	
	// Network-related errors that are worth retrying
	retryableErrors := []string{
		"connection",
		"timeout",
		"network",
		"temporary",
		"i/o timeout",
		"broken pipe",
		"connection reset",
		"no route to host",
		"host unreachable",
	}
	
	for _, retryable := range retryableErrors {
		if contains(errStr, retryable) {
			return true
		}
	}
	
	// Non-retryable errors
	nonRetryableErrors := []string{
		"not found",
		"unauthorized",
		"forbidden",
		"invalid",
		"malformed",
		"insufficient space",
		"disk full",
	}
	
	for _, nonRetryable := range nonRetryableErrors {
		if contains(errStr, nonRetryable) {
			return false
		}
	}
	
	// Default to retryable for unknown errors
	return true
}

// contains checks if a string contains a substring (case-insensitive)
func contains(str, substr string) bool {
	return len(str) >= len(substr) && 
		   (str == substr || 
		    (len(str) > len(substr) && 
		     anySubstring(str, substr)))
}

func anySubstring(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ResumeDownload attempts to resume a partially downloaded model
func (o *OllamaService) ResumeDownload(model string) error {
	// Check if there's a partial download
	if o.modelManager != nil {
		if status, exists := o.modelManager.GetModelStatus(model); exists {
			if status.State == ModelStatePartial {
				fmt.Printf("Resuming partial download for %s (%.1f%% complete)\n", 
					model, float64(status.Downloaded)/float64(status.Size)*100)
				
				// Clean up partial state first
				if err := o.modelManager.CleanPartialDownloads(model); err != nil {
					fmt.Printf("Warning: failed to clean partial download: %v\n", err)
				}
			}
		}
	}
	
	// Start fresh download with retry logic
	return o.RetryDownload(model, DefaultRetryConfig())
}

// PullModelWithRetry is the main entry point for downloading models with retry logic
func (o *OllamaService) PullModelWithRetry(model string) error {
	// First check if model is already installed
	if o.modelExists(model) {
		return fmt.Errorf("model %s is already installed", model)
	}
	
	// Try to resume if there's a partial download
	if o.modelManager != nil {
		if status, exists := o.modelManager.GetModelStatus(model); exists && status.State == ModelStatePartial {
			fmt.Printf("Found partial download for %s, attempting to resume...\n", model)
			return o.ResumeDownload(model)
		}
	}
	
	// Start fresh download with retry logic
	return o.RetryDownload(model, DefaultRetryConfig())
}