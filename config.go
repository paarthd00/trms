package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	// Database settings
	Database struct {
		AutoSetup bool   `json:"auto_setup"`
		Host      string `json:"host"`
		Port      int    `json:"port"`
		User      string `json:"user"`
		Password  string `json:"password"`
		DBName    string `json:"dbname"`
	} `json:"database"`

	// Ollama settings
	Ollama struct {
		AutoInstall    bool   `json:"auto_install"`
		Host          string `json:"host"`
		Port          int    `json:"port"`
		DefaultModel  string `json:"default_model"`
		AutoPull      bool   `json:"auto_pull_default"`
	} `json:"ollama"`

	// UI settings
	UI struct {
		Theme             string `json:"theme"`
		ShowLineNumbers   bool   `json:"show_line_numbers"`
		WrapText          bool   `json:"wrap_text"`
		MaxChatHistory    int    `json:"max_chat_history"`
		ProgressBarWidth  int    `json:"progress_bar_width"`
	} `json:"ui"`

	// Keybindings (allow customization)
	Keybindings struct {
		NewChat       string `json:"new_chat"`
		SwitchModel   string `json:"switch_model"`
		ModelManager  string `json:"model_manager"`
		CancelAction  string `json:"cancel_action"`
		DeleteModel   string `json:"delete_model"`
	} `json:"keybindings"`

	// Advanced settings
	Advanced struct {
		LogLevel        string `json:"log_level"`
		SaveChatHistory bool   `json:"save_chat_history"`
		MaxRetries      int    `json:"max_retries"`
		Timeout         int    `json:"timeout_seconds"`
	} `json:"advanced"`
}

type ConfigManager struct {
	config     *Config
	configPath string
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		config:     getDefaultConfig(),
		configPath: getConfigPath(),
	}
}

func getDefaultConfig() *Config {
	config := &Config{}
	
	// Database defaults
	config.Database.AutoSetup = true
	config.Database.Host = "localhost"
	config.Database.Port = 5433
	config.Database.User = "trms"
	config.Database.Password = "trms_password"
	config.Database.DBName = "trms"

	// Ollama defaults
	config.Ollama.AutoInstall = true
	config.Ollama.Host = "localhost"
	config.Ollama.Port = 11434
	config.Ollama.DefaultModel = "" // Will be set to first available
	config.Ollama.AutoPull = false

	// UI defaults
	config.UI.Theme = "default"
	config.UI.ShowLineNumbers = false
	config.UI.WrapText = true
	config.UI.MaxChatHistory = 50
	config.UI.ProgressBarWidth = 40

	// Keybinding defaults
	config.Keybindings.NewChat = "ctrl+n"
	config.Keybindings.SwitchModel = "ctrl+s"
	config.Keybindings.ModelManager = "ctrl+m"
	config.Keybindings.CancelAction = "ctrl+g"
	config.Keybindings.DeleteModel = "ctrl+d"

	// Advanced defaults
	config.Advanced.LogLevel = "info"
	config.Advanced.SaveChatHistory = true
	config.Advanced.MaxRetries = 3
	config.Advanced.Timeout = 30

	return config
}

func getConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./trms-config.json"
	}
	
	configDir := filepath.Join(homeDir, ".config", "trms")
	os.MkdirAll(configDir, 0755)
	return filepath.Join(configDir, "config.json")
}

func (cm *ConfigManager) LoadConfig() error {
	// Check if config file exists
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		// Create default config file
		return cm.SaveConfig()
	}

	// Read existing config
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	var loadedConfig Config
	if err := json.Unmarshal(data, &loadedConfig); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	// Merge with defaults (in case new fields were added)
	cm.mergeWithDefaults(&loadedConfig)
	cm.config = &loadedConfig

	return nil
}

func (cm *ConfigManager) SaveConfig() error {
	// Create config directory if it doesn't exist
	configDir := filepath.Dir(cm.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(cm.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (cm *ConfigManager) mergeWithDefaults(loaded *Config) {
	defaults := getDefaultConfig()
	
	// Fill in any missing values with defaults
	if loaded.Database.Host == "" {
		loaded.Database.Host = defaults.Database.Host
	}
	if loaded.Database.Port == 0 {
		loaded.Database.Port = defaults.Database.Port
	}
	if loaded.Ollama.Host == "" {
		loaded.Ollama.Host = defaults.Ollama.Host
	}
	if loaded.Ollama.Port == 0 {
		loaded.Ollama.Port = defaults.Ollama.Port
	}
	if loaded.UI.MaxChatHistory == 0 {
		loaded.UI.MaxChatHistory = defaults.UI.MaxChatHistory
	}
	if loaded.UI.ProgressBarWidth == 0 {
		loaded.UI.ProgressBarWidth = defaults.UI.ProgressBarWidth
	}
	if loaded.Advanced.MaxRetries == 0 {
		loaded.Advanced.MaxRetries = defaults.Advanced.MaxRetries
	}
	if loaded.Advanced.Timeout == 0 {
		loaded.Advanced.Timeout = defaults.Advanced.Timeout
	}
}

func (cm *ConfigManager) GetConfig() *Config {
	return cm.config
}

func (cm *ConfigManager) UpdateConfig(key string, value interface{}) error {
	// Simple key-value updates for common settings
	switch key {
	case "database.auto_setup":
		if v, ok := value.(bool); ok {
			cm.config.Database.AutoSetup = v
		}
	case "ollama.auto_install":
		if v, ok := value.(bool); ok {
			cm.config.Ollama.AutoInstall = v
		}
	case "ollama.default_model":
		if v, ok := value.(string); ok {
			cm.config.Ollama.DefaultModel = v
		}
	case "ui.theme":
		if v, ok := value.(string); ok {
			cm.config.UI.Theme = v
		}
	case "ui.wrap_text":
		if v, ok := value.(bool); ok {
			cm.config.UI.WrapText = v
		}
	case "ui.max_chat_history":
		if v, ok := value.(int); ok {
			cm.config.UI.MaxChatHistory = v
		}
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	return cm.SaveConfig()
}

func (cm *ConfigManager) GetConfigPath() string {
	return cm.configPath
}

func (cm *ConfigManager) ResetToDefaults() error {
	cm.config = getDefaultConfig()
	return cm.SaveConfig()
}