package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	
	"trms/internal/ui"
	"trms/internal/models"
	"trms/internal/services"
)

// Application is the main TUI application
type Application struct {
	model               models.AppModel
	chatView            ui.ChatView
	modernChatView      ui.ModernChatView
	modelManagerView    ui.ModelManagerView
	cleanModelManager   ui.CleanModelManagerView
	imageGeneratorView  ui.ImageGeneratorView
	width               int
	height              int
	ollama              *services.OllamaService
	db                  *services.DatabaseService
	systemInfo          *services.SystemInfo
	imageGenerator      *services.ImageGeneratorService
	mcpBridge           *services.MCPBridge
	useModernUI         bool
	useCleanModels      bool
}

// New creates a new Application instance
func New() *Application {
	// Create input field
	ti := textinput.New()
	ti.Placeholder = "Type your message..."
	ti.Focus()
	ti.CharLimit = 256
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Create chat view with initial size (will be updated on WindowSizeMsg)
	chatView := ui.NewChatView(80, 24)
	modernChatView := ui.NewModernChatView(80, 24)

	// Create model manager view with initial size
	modelManagerView := ui.NewModelManagerView(80, 24)

	// Create image generator view with initial size
	imageGeneratorView := ui.NewImageGeneratorView(80, 24)

	// Create services
	containerManager := services.NewContainerManager()
	ollamaService := services.NewOllamaService()
	dbService := services.NewDatabaseService()
	imageGeneratorService := services.NewImageGeneratorService()
	mcpBridge := services.NewMCPBridge(ollamaService)
	
	// Create clean model manager
	cleanModelManager := ui.NewCleanModelManagerView(80, 24, ollamaService.GetStateManager())
	
	// Perform startup checks and container management
	if err := containerManager.StartupCheck(); err != nil {
		fmt.Printf("Startup check failed: %v\n", err)
		fmt.Println("Some features may be limited. Continuing...")
	}
	
	// Try to connect to database if it's running
	if dbService.IsPostgresRunning() {
		if err := dbService.Connect(); err != nil {
			fmt.Printf("Warning: Failed to connect to database: %v\n", err)
		}
	}

	// Get system info
	systemInfo, _ := services.GetSystemInfo()

	app := &Application{
		model: models.AppModel{
			Mode:             models.ChatMode,
			Input:            ti,
			CurrentSessionID: 1,
			ChatHistory:      []models.Message{},
			Width:            80,
			Height:           25,
		},
		chatView:           chatView,
		modernChatView:     modernChatView,
		modelManagerView:   modelManagerView,
		cleanModelManager:  cleanModelManager,
		imageGeneratorView: imageGeneratorView,
		width:              80,
		height:             25,
		ollama:             ollamaService,
		db:                 dbService,
		systemInfo:         systemInfo,
		imageGenerator:     imageGeneratorService,
		mcpBridge:          mcpBridge,
		useModernUI:        true, // Enable modern UI by default
		useCleanModels:     true, // Use clean model manager by default
	}

	return app
}

// Init initializes the application
func (app *Application) Init() tea.Cmd {
	var cmds []tea.Cmd
	
	cmds = append(cmds, textinput.Blink)
	cmds = append(cmds, tea.EnterAltScreen)
	cmds = append(cmds, app.checkOllama())
	
	// Initialize clean model manager if using clean models
	if app.useCleanModels {
		cmds = append(cmds, app.cleanModelManager.Init())
	}
	
	return tea.Batch(cmds...)
}

// Update handles all messages and updates
func (app *Application) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		app.width = msg.Width
		app.height = msg.Height
		app.model.Width = msg.Width
		app.model.Height = msg.Height

		// Update input field width to be responsive
		inputWidth := msg.Width - 8 // Account for borders and padding
		if inputWidth < 20 {
			inputWidth = 20
		}
		if inputWidth > 120 {
			inputWidth = 120
		}
		app.model.Input.Width = inputWidth

		// Update chat view size
		if app.useModernUI {
			app.modernChatView, cmd = app.modernChatView.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			app.chatView, cmd = app.chatView.Update(msg)
			cmds = append(cmds, cmd)
		}
		
		// Update model manager view size
		app.modelManagerView, cmd = app.modelManagerView.Update(msg)
		cmds = append(cmds, cmd)
		app.cleanModelManager, cmd = app.cleanModelManager.Update(msg)
		cmds = append(cmds, cmd)
		
		// Update image generator view size
		app.imageGeneratorView, cmd = app.imageGeneratorView.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		// First, handle mode-specific keyboard updates for navigation
		// This ensures arrow keys and other navigation keys are properly forwarded
		switch app.model.Mode {
		case models.ChatMode:
			app.chatView, cmd = app.chatView.Update(msg)
			cmds = append(cmds, cmd)
		case models.ModelManagementMode, models.ModelSelectionMode, models.CategorySelectionMode:
			if app.useCleanModels {
				app.cleanModelManager, cmd = app.cleanModelManager.Update(msg)
				cmds = append(cmds, cmd)
			} else {
				app.modelManagerView, cmd = app.modelManagerView.Update(msg)
				cmds = append(cmds, cmd)
			}
		case models.ImageGenerationMode:
			app.imageGeneratorView, cmd = app.imageGeneratorView.Update(msg)
			cmds = append(cmds, cmd)
		case models.ConfirmationMode:
			// Handle confirmation dialog keys
			switch msg.String() {
			case "y", "Y":
				return app, app.handleConfirmedAction()
			case "n", "N", "esc":
				app.model.Mode = models.ModelManagementMode
				return app, nil
			}
		}

		// Then handle global key bindings
		switch msg.Type {
		case tea.KeyCtrlC:
			app.model.Quitting = true
			return app, tea.Quit


		case tea.KeyEsc:
			// Handle escape for different modes
			switch app.model.Mode {
			case models.ModelManagementMode, models.ChatListMode, models.ImageGenerationMode:
				app.model.Mode = models.ChatMode
				currentModel := app.ollama.GetCurrentModel()
				if currentModel == "No model selected" || currentModel == "" {
					app.model.Input.Placeholder = "Message assistant... (No model selected)"
				} else {
					app.model.Input.Placeholder = fmt.Sprintf("Chat with %s", currentModel)
				}
				app.model.Input.Focus()
			case models.ModelSelectionMode:
				app.model.Mode = models.ChatMode
				currentModel := app.ollama.GetCurrentModel()
				if currentModel == "No model selected" || currentModel == "" {
					app.model.Input.Placeholder = "Message assistant... (No model selected)"
				} else {
					app.model.Input.Placeholder = fmt.Sprintf("Chat with %s", currentModel)
				}
				app.model.Input.Focus()
			case models.ModelInfoMode:
				app.model.Mode = models.ModelManagementMode
				return app, nil
			}
			return app, nil

		case tea.KeyEnter:
			// Handle enter based on mode
			switch app.model.Mode {
			case models.ChatMode:
				prompt := app.model.Input.Value()
				if prompt != "" {
					app.model.Input.Reset()
					// Check if it's a command (starts with /)
					if strings.HasPrefix(prompt, "/") {
						return app, app.handleSlashCommand(prompt[1:]) // Remove the / prefix
					}
					// Add message to chat view
					app.chatView.AddMessage("user", prompt)
					// Save user message to database
					app.db.SaveMessage(app.model.CurrentSessionID, "user", prompt)
					// Add thinking indicator
					app.chatView.AddMessage("assistant", "Thinking...")
					// Send to Ollama for real response
					return app, app.performAI(prompt)
				}
			case models.ModelManagementMode:
				// Show model info
				return app, app.showModelInfo()
			case models.ModelSelectionMode:
				// Select model for current chat
				return app, app.handleModelSelection()
			case models.ChatListMode:
				// Switch to selected chat
				return app, app.handleChatSelection()
			}
		
		default:
			// Handle special key combinations
			switch msg.String() {
			case "ctrl+m":
				app.model.Mode = models.ModelManagementMode
				return app, app.refreshModelStates()
			case "ctrl+n":
				// New chat (from chat mode)
				if app.model.Mode == models.ChatMode {
					return app, app.createNewChat()
				}
			case "ctrl+s":
				// Switch model in current chat
				if app.model.Mode == models.ChatMode {
					app.model.Mode = models.ModelSelectionMode
					return app, app.refreshModelStates()
				}
			case "ctrl+h":
				// Show chat history
				if app.model.Mode == models.ChatMode {
					return app, app.showChatHistory()
				}
			}
			
			// Handle other keys for model management
			if app.model.Mode == models.ModelManagementMode || app.model.Mode == models.ModelSelectionMode {
				switch msg.String() {
				case "d", "delete":
					return app, app.showDeleteConfirmation()
				case "c":
					return app, app.handleModelAction("clean")
				case "r":
					return app, app.handleModelAction("restart")
				case "i":
					return app, app.handleModelAction("download")
				}
			}
		}

	case tea.MouseMsg:
		if app.model.Mode == models.ChatMode {
			app.chatView, cmd = app.chatView.Update(msg)
			cmds = append(cmds, cmd)
		}

		// Handle Ollama responses
		case models.AIResponseMsg:
		// Remove the "Thinking..." message by getting all messages except the last one if it's "Thinking..."
		messages := app.chatView.GetMessages()
		if len(messages) > 0 {
			lastMsg := messages[len(messages)-1]
			if lastMsg.Role == "assistant" && lastMsg.Content == "Thinking..." {
				// Clear and re-add all messages except the last one
				app.chatView.SetMessages(messages[:len(messages)-1])
			}
		}
		
		if msg.Err != nil {
			// Show error in chat
			app.chatView.AddMessage("system", fmt.Sprintf("Error: %v", msg.Err))
		} else if msg.Response != "" {
			// Add AI response to chat only if there's actual content
			app.chatView.AddMessage("assistant", msg.Response)
			// Save AI response to database
			app.db.SaveMessage(app.model.CurrentSessionID, "assistant", msg.Response)
		}
		
	case models.ModelsRefreshedMsg:
		// Models have been refreshed, update the view if in model management mode
		if app.model.Mode == models.ModelManagementMode && msg.Err == nil {
			// The model states are already updated in the view
		}
		
	case models.ModelDeletedMsg:
		if msg.Err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("Failed to delete model %s: %v", msg.Model, msg.Err))
		} else {
			app.chatView.AddMessage("system", fmt.Sprintf("Model %s deleted successfully!", msg.Model))
		}
		// Refresh model states
		return app, app.refreshModelStates()
		
	case models.NewChatMsg:
		if msg.Err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("Failed to create new chat: %v", msg.Err))
		} else {
			// Silent new chat creation - just switch to new session
			app.model.CurrentSessionID = msg.SessionID
		}
		
	case models.ModelProgressMsg:
		// Skip updating model manager view - we only want bottom indicator
		if msg.Progress != nil {
			// Force a UI refresh by returning a nil command (this triggers re-render)
			return app, nil
		}
		
	case models.ModelPulledMsg:
		if msg.Err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("Failed to download model %s: %v", msg.Model, msg.Err))
		} else {
			app.chatView.AddMessage("system", fmt.Sprintf("✅ Model %s downloaded successfully!", msg.Model))
		}
		// Refresh model states to show the new model
		return app, app.refreshModelStates()
		
	case models.ProgressTickMsg:
		// Check progress for the specific model
		progress := app.ollama.GetProgress(msg.Model)
		if progress != nil {
			// Update model manager view with progress
			if !app.useCleanModels {
				app.modelManagerView.UpdateProgressWithStats(
					msg.Model, 
					progress.Percent, 
					progress.Downloaded, 
					progress.Total,
					progress.Speed,
					progress.ETA,
				)
			}
			// Clean model manager handles progress automatically via state manager
			
			// Check if download is complete
			if progress.Percent >= 100 || progress.Status == "Download complete" {
				// Download complete - refresh model states immediately (no duplicate message)
				return app, app.refreshModelStates()
			} else if strings.Contains(progress.Status, "failed") || strings.Contains(progress.Status, "error") {
				// Download failed - show error and stop tracking
				app.chatView.AddMessage("system", fmt.Sprintf("Download failed for %s: %s", msg.Model, progress.Status))
				return app, app.refreshModelStates()
			} else {
				// Continue ticking if still downloading and force UI refresh
				return app, tea.Batch(
					app.startProgressTracking(msg.Model),
					func() tea.Msg { return nil }, // Force refresh
				)
			}
		} else {
			// No progress found - check if Ollama is running
			if !app.ollama.IsRunning() {
				app.chatView.AddMessage("system", fmt.Sprintf("Cannot download %s: Ollama is not running", msg.Model))
				return app, app.refreshModelStates()
			}
			// Continue tracking for a bit longer in case download is starting
			return app, app.startProgressTracking(msg.Model)
		}
		
	case models.ModelInfoMsg:
		if msg.Err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("Failed to get model info: %v", msg.Err))
			app.model.Mode = models.ModelManagementMode
		} else {
			// Store model info and switch to info mode
			app.model.ModelInfoData = msg.Info
			app.model.ModelInfoTarget = msg.ModelName
			app.model.Mode = models.ModelInfoMode
		}
		
	case ui.ShowCopyHistoryMsg:
		// Show copy history as a system message
		if app.model.Mode == models.ChatMode {
			historyText := app.chatView.FormatCopyHistory()
			app.chatView.AddMessage("system", historyText)
		}
		
	case ui.ImageGenerationStartedMsg:
		// Handle image generation start
		return app, app.generateImage(msg.Model, msg.Prompt, msg.Parameters)
		
	case ui.ImageGeneratedMsg:
		// Pass to image generator view
		app.imageGeneratorView, cmd = app.imageGeneratorView.Update(msg)
		cmds = append(cmds, cmd)
		
	case ui.ImageSavedMsg:
		// Show save result in chat
		if msg.Err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("Failed to save image: %v", msg.Err))
		} else {
			app.chatView.AddMessage("system", fmt.Sprintf("Image saved to: %s", msg.Path))
		}
		
	}

	// Update input
	app.model.Input, cmd = app.model.Input.Update(msg)
	cmds = append(cmds, cmd)

	return app, tea.Batch(cmds...)
}

// View renders the application
func (app *Application) View() string {
	if app.model.Quitting {
		return "Goodbye! 👋\n"
	}

	var content string

	// Render based on mode
	switch app.model.Mode {
	case models.ChatMode:
		content = app.renderChatMode()
	case models.ModelManagementMode:
		content = app.renderModelManagementMode()
	case models.ModelSelectionMode:
		content = app.renderModelSelectionMode()
	case models.CategorySelectionMode:
		content = app.renderCategorySelectionMode()
	case models.ChatListMode:
		content = app.renderChatListMode()
	case models.ConfirmationMode:
		content = app.renderConfirmationMode()
	case models.ModelInfoMode:
		content = app.renderModelInfoMode()
	case models.ImageGenerationMode:
		content = app.renderImageGenerationMode()
	}

	return content
}

// renderSeparator creates a responsive separator line
func (app *Application) renderSeparator() string {
	width := app.width
	if width > 100 {
		width = 100
	}
	if width < 20 {
		width = 20
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render(strings.Repeat("─", width))
}


// renderChatMode renders the chat mode interface
func (app *Application) renderChatMode() string {
	var s string

	// Simple header without model name
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true).
		Render("TRMS")
	s += header + "\n"
	
	// Subtle separator
	s += app.renderSeparator() + "\n\n"

	// Chat view
	s += app.chatView.View() + "\n"

	// Clean input area
	currentModel := app.ollama.GetCurrentModel()
	if currentModel == "" || currentModel == "No model selected" {
		app.model.Input.Placeholder = "Select a model first (Ctrl+S)"
	} else {
		app.model.Input.Placeholder = "Type your message..."
	}
	
	// Input with clean styling
	inputArea := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(app.model.Input.View())
	s += inputArea + "\n"

	// Bottom status bar with model indicator and help
	var modelStatus string
	if currentModel == "" || currentModel == "No model selected" {
		modelStatus = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("No model selected")
	} else {
		modelStatus = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Render("● " + currentModel)
	}
	
	// Responsive help text based on screen width
	var helpText string
	if app.width > 120 {
		helpText = "/help: Commands • Ctrl+M: Models • Ctrl+S: Switch Model • Click 📋 to copy • Ctrl+P: History"
	} else if app.width > 80 {
		helpText = "/help: Commands • Ctrl+M: Models • Ctrl+S: Switch • 📋 Copy • Ctrl+P: History"
	} else {
		helpText = "/help • Ctrl+M: Models • Ctrl+S: Switch • 📋 Copy"
	}
	
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render(helpText)
	
	statusBar := lipgloss.JoinHorizontal(lipgloss.Left,
		modelStatus,
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Render(" │ "),
		help,
	)
	s += "\n" + statusBar

	return s
}

// renderModelManagementMode renders the model management interface
func (app *Application) renderModelManagementMode() string {
	var s string

	// Clean header with memory info
	availableMem := services.FormatMemory(app.systemInfo.AvailableMemory)
	totalMem := services.FormatMemory(app.systemInfo.TotalMemory)
	
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true).
			Render("TRMS"),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render(" │ "),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Render("Model Manager"),
	)
	
	memInfo := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Render(fmt.Sprintf(" • %s / %s", availableMem, totalMem))
	
	s += header + memInfo + "\n"
	
	// Subtle separator
	s += app.renderSeparator() + "\n\n"

	// Memory warning if needed
	availableGB := float64(app.systemInfo.AvailableMemory) / (1024 * 1024 * 1024)
	if availableGB < 8 {
		warning := lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")).
			Render("⚠ Low memory - consider: phi, orca-mini, tinyllama")
		s += warning + "\n\n"
	}

	// Model manager view
	if app.useCleanModels {
		s += app.cleanModelManager.View()
		return s
	}
	
	s += app.modelManagerView.View() + "\n"

	// Clean help
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render("Enter: Download • Tab/T: Categories • d: Delete • c: Clean • r: Restart • i: Info • ESC: Back")
	s += help

	// Simple bottom download status - check directly from Ollama
	activeDownloads := app.ollama.GetActiveDownloads()
	if len(activeDownloads) > 0 {
		for model, progress := range activeDownloads {
			downloadStatus := fmt.Sprintf("%s (%d%%)", model, progress.Percent)
			s += "\n\n" + lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")).
				Bold(true).
				Render("⬇ Downloading: " + downloadStatus)
			break // Only show the first one
		}
	}

	return s
}

// renderModelSelectionMode renders the model selection interface for chat
func (app *Application) renderModelSelectionMode() string {
	var s string

	// Clean header with current context
	currentModel := app.ollama.GetCurrentModel()
	var modelDisplay string
	if currentModel == "" || currentModel == "No model selected" {
		modelDisplay = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Render("No model")
	} else {
		modelDisplay = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true).
			Render(currentModel)
	}
	
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true).
			Render("TRMS"),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render(" │ "),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Render("Switch Model"),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render(" │ "),
		modelDisplay,
	)
	s += header + "\n"
	
	// Subtle separator
	s += app.renderSeparator() + "\n\n"

	// Model manager view
	s += app.modelManagerView.View() + "\n"

	// Clean help
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render("Enter: Switch • d: Delete • c: Clean • i: Info • ESC: Back to Chat")
	s += help

	return s
}

// renderCategorySelectionMode renders the category selection interface
func (app *Application) renderCategorySelectionMode() string {
	var s string

	// Clean header
	header := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true).
			Render("TRMS"),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render(" │ "),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Render("📂 Model Categories"),
	)
	s += header + "\n"
	
	// Subtle separator
	s += app.renderSeparator() + "\n\n"

	// Model manager view (showing categories)
	s += app.modelManagerView.View() + "\n"

	// Clean help
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render("Enter: Select Category • Tab/T: Back to Models • ↑/↓: Navigate • ESC: Back")
	s += help

	return s
}

// renderChatListMode renders the chat history browser
func (app *Application) renderChatListMode() string {
	var s string

	// Title
	title := titleStyle.Render("⚡ Chat History")
	s += title + "\n\n"

	// Current session info
	sessionInfo := helpStyle.Render(fmt.Sprintf("Current Session: %d", app.model.CurrentSessionID))
	s += sessionInfo + "\n\n"

	// TODO: Implement chat list view
	s += "Chat history browser coming soon...\n"
	s += "Use Ctrl+N to create a new chat\n"
	s += "Press ESC to return to chat mode\n"

	return s
}


// handleSlashCommand processes slash commands from chat mode
func (app *Application) handleSlashCommand(input string) tea.Cmd {
	switch input {
	case "q", "quit", "exit":
		return tea.Quit
	case "models", "m":
		app.model.Mode = models.ModelManagementMode
		return app.refreshModelStates()
	case "image", "img", "generate":
		app.model.Mode = models.ImageGenerationMode
		// Set current model if it's an image generation model
		currentModel := app.ollama.GetCurrentModel()
		if app.imageGenerator.IsImageGenerationModel(currentModel) {
			app.imageGeneratorView.SetCurrentModel(currentModel)
		}
		return nil
	case "install-ollama":
		// Install Ollama directly from TRMS
		return app.installOllama()
	case "status":
		// Show container status
		return app.showContainerStatus()
	case "restart-containers":
		// Restart all containers
		return app.restartContainers()
	case "logs":
		// Show container logs
		return app.showContainerLogs()
	case "reset":
		// Reset containers
		return app.resetContainers()
	case "deps", "dependencies":
		// Check dependencies
		return app.checkDependencies()
	case "help", "h", "?":
		// Show help
		return app.showHelp()
	default:
		// Show unknown command message
		app.chatView.AddMessage("system", fmt.Sprintf("Unknown command: '/%s'. Type '/help' for available commands.", input))
		return nil
	}
}

// performAI sends a message to Ollama and gets response
func (app *Application) performAI(prompt string) tea.Cmd {
	return func() tea.Msg {
		// Check if Ollama is running
		if !app.ollama.IsRunning() {
			return models.AIResponseMsg{
				Response: "",
				Err:      fmt.Errorf("Ollama is not running. Please start Ollama first"),
			}
		}

		// Check if model is selected
		currentModel := app.ollama.GetCurrentModel()
		if currentModel == "No model selected" || currentModel == "" {
			return models.AIResponseMsg{
				Response: "",
				Err:      fmt.Errorf("No model selected. Please run 'ollama pull phi' or another small model first"),
			}
		}

		// Send to Ollama
		response, err := app.ollama.Chat(prompt)
		return models.AIResponseMsg{
			Response: response,
			Err:      err,
		}
	}
}

// performAIWithTools sends a message through MCP bridge with tool support
func (app *Application) performAIWithTools(prompt string) tea.Cmd {
	return func() tea.Msg {
		// Check if Ollama is running
		if !app.ollama.IsRunning() {
			return models.AIResponseMsg{
				Response: "",
				Err:      fmt.Errorf("Ollama is not running. Please start Ollama first"),
			}
		}

		// Check if model is selected
		currentModel := app.ollama.GetCurrentModel()
		if currentModel == "No model selected" || currentModel == "" {
			return models.AIResponseMsg{
				Response: "",
				Err:      fmt.Errorf("No model selected. Please run 'ollama pull phi' or another small model first"),
			}
		}

		// Use MCP bridge for tool-enabled responses if available
		if app.mcpBridge != nil {
			mcpResponse, err := app.mcpBridge.ChatWithTools(prompt)
			if err != nil {
				return models.AIResponseMsg{
					Response: "",
					Err:      err,
				}
			}
			
			// Convert MCP response to our message format
			var toolCalls []models.MCPToolCall
			for _, tc := range mcpResponse.ToolCalls {
				toolCalls = append(toolCalls, models.MCPToolCall{
					ID:     tc.ID,
					Name:   tc.Name,
					Args:   tc.Args,
					Result: tc.Result,
					Error:  tc.Error,
				})
			}
			
			return models.MCPResponseMsg{
				Response: &models.MCPResponse{
					Content:   mcpResponse.Content,
					ToolCalls: toolCalls,
				},
				Err: nil,
			}
		}

		// Fallback to regular Ollama chat
		response, err := app.ollama.Chat(prompt)
		return models.AIResponseMsg{
			Response: response,
			Err:      err,
		}
	}
}

// checkOllama checks if Ollama is installed and running
func (app *Application) checkOllama() tea.Cmd {
	return func() tea.Msg {
		if !app.ollama.IsInstalled() {
			// Provide clear installation instructions
			app.chatView.AddMessage("system", `⚠️  Ollama is not installed

To install Ollama on Linux, run:
curl -fsSL https://ollama.com/install.sh | sh

For other platforms, visit: https://ollama.com/download

After installation, restart TRMS to use chat features.`)
		} else if !app.ollama.IsRunning() {
			// Try to start Ollama
			app.chatView.AddMessage("system", "Starting Ollama service...")
			if err := app.ollama.StartService(); err != nil {
				app.chatView.AddMessage("system", fmt.Sprintf(`⚠️  Ollama is installed but not running

Failed to start automatically: %v

Try starting manually with:
ollama serve

Then restart TRMS.`, err))
			} else {
				// Refresh models after starting
				app.ollama.RefreshModels()
				app.chatView.AddMessage("system", "✅ Ollama service started successfully!")
			}
		} else {
			// Ollama is running, refresh models
			app.ollama.RefreshModels()
			
			// Check if no models are installed and auto-switch to model manager
			installedModels := app.ollama.GetModels()
			if len(installedModels) == 0 {
				// No models installed, switch to model manager
				app.model.Mode = models.ModelManagementMode
				app.chatView.AddMessage("system", "👋 Welcome to TRMS! No models found. Please download a model to start chatting.")
			}
		}
		return nil
	}
}


// Professional styles
var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		MarginBottom(1)
		
	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		MarginBottom(1)

	modeStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("15")).
		Padding(0, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))

	successStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Bold(true)

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("9")).
		Bold(true)

	subtleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
)

// refreshModelStates refreshes the model states including partial downloads
func (app *Application) refreshModelStates() tea.Cmd {
	return func() tea.Msg {
		// Force refresh of installed models first
		app.ollama.RefreshModels()
		
		// Get all model states including partials
		states, err := app.ollama.GetModelStates()
		if err != nil {
			return models.ModelsRefreshedMsg{Err: err}
		}

		// Update the model manager view with memory requirements
		enrichedModels := make([]models.ModelInfo, len(models.AllModels))
		copy(enrichedModels, models.AllModels)
		
		// Add memory check to descriptions
		for i := range enrichedModels {
			canRun, reason := services.CanRunModel(enrichedModels[i].MemoryGB, app.systemInfo)
			if !canRun {
				enrichedModels[i].Description = fmt.Sprintf("⚠️  %s - %s", reason, enrichedModels[i].Description)
			}
		}

		if app.useCleanModels {
			// For clean model manager, refresh the state manager
			app.ollama.GetStateManager().RefreshStates()
		} else {
			app.modelManagerView.SetModelStates(states, enrichedModels)
			
			// Update current model indicator
			currentModel := app.ollama.GetCurrentModel()
			if currentModel != "" && currentModel != "No model selected" {
				app.modelManagerView.SetCurrentModel(currentModel)
			}
		}

		return models.ModelsRefreshedMsg{Err: nil}
	}
}

// handleModelAction handles model management actions
func (app *Application) handleModelAction(action string) tea.Cmd {
	return func() tea.Msg {
		// Get selected item from model manager
		var selected *ui.ModelManagerItem
		if app.useCleanModels {
			selected = app.cleanModelManager.GetSelectedModel()
		} else {
			selected = app.modelManagerView.GetSelectedModel()
		}
		
		if selected == nil {
			return models.AIResponseMsg{
				Response: "",
				Err:      fmt.Errorf("No valid model selected. Please select a model to %s", action),
			}
		}

		item := *selected
		// This check should now be redundant due to GetSelectedModel safety check
		if item.IsHeader || item.IsSeparator {
			return models.AIResponseMsg{
				Response: "",
				Err:      fmt.Errorf("Cannot %s a header or separator. Please select a model", action),
			}
		}

		switch action {
		case "download":
			// Check if Ollama is running
			if !app.ollama.IsRunning() {
				return models.AIResponseMsg{
					Response: "",
					Err:      fmt.Errorf("Ollama is not running. Please start Ollama first with 'ollama serve'"),
				}
			}
			
			// Check if already downloading
			if item.State == services.ModelStateDownloading {
				return models.AIResponseMsg{
					Response: "",
					Err:      fmt.Errorf("Model %s is already downloading", item.Name),
				}
			}
			
			// If model is already installed, switch to it instead of downloading
			if item.State == services.ModelStateComplete {
				// In model management mode, switch to the model silently
				app.ollama.SetCurrentModel(item.Name)
				return app.refreshModelStates()()
			}
			
			// Check memory requirements
			for _, model := range models.AllModels {
				if model.Name == item.Name {
					canRun, reason := services.CanRunModel(model.MemoryGB, app.systemInfo)
					if !canRun {
						return models.AIResponseMsg{
							Response: "",
							Err:      fmt.Errorf(reason),
						}
					}
					break
				}
			}
			// Skip updating model manager UI - we only want bottom indicator
			
			// Show download starting message
			app.chatView.AddMessage("system", fmt.Sprintf("Starting download of %s...", item.Name))
			
			// Return command that starts the download (progress tracking will start after download setup)
			return app.startModelDownload(item.Name)

		case "delete":
			if item.State == services.ModelStateComplete {
				// Delete complete model via Ollama
				err := app.ollama.DeleteModel(item.Name)
				if err != nil {
					return models.ModelDeletedMsg{Model: item.Name, Err: err}
				}
			}
			return app.refreshModelStates()()

		case "clean":
			if item.State == services.ModelStatePartial || item.State == services.ModelStateCorrupted {
				// Clean partial download
				err := app.ollama.CleanPartialDownload(item.Name)
				if err != nil {
					return models.AIResponseMsg{
						Response: "",
						Err:      fmt.Errorf("Failed to clean partial download: %v", err),
					}
				}
			}
			return app.refreshModelStates()()
			
		case "restart":
			// Restart download for partial models
			if item.State == services.ModelStatePartial || item.State == services.ModelStateCorrupted {
				// Clean first, then restart download
				app.ollama.CleanPartialDownload(item.Name)
				err := app.ollama.PullModel(item.Name)
				if err != nil {
					return models.AIResponseMsg{
						Response: "",
						Err:      fmt.Errorf("Failed to restart download: %v", err),
					}
				}
			}
			return app.refreshModelStates()()
		}

		return nil
	}
}

// createNewChat creates a new chat session
func (app *Application) createNewChat() tea.Cmd {
	return func() tea.Msg {
		// Create a new chat session with default name and current model
		currentModel := app.ollama.GetCurrentModel()
		if currentModel == "No model selected" || currentModel == "" {
			currentModel = "default"
		}
		
		sessionName := fmt.Sprintf("Chat %s", time.Now().Format("Jan 2 15:04"))
		session, err := app.db.CreateChatSession(sessionName, currentModel)
		if err != nil {
			return models.NewChatMsg{SessionID: 0, Err: err}
		}
		
		// Switch to the new session
		app.model.CurrentSessionID = session.ID
		app.chatView.ClearMessages()
		
		return models.NewChatMsg{SessionID: session.ID, Err: nil}
	}
}

// showModelSelection shows model selection for current chat
func (app *Application) showModelSelection() tea.Cmd {
	return func() tea.Msg {
		app.model.Mode = models.ModelSelectionMode
		// Refresh model states and return the message
		return app.refreshModelStates()()
	}
}

// showChatHistory shows the chat history browser
func (app *Application) showChatHistory() tea.Cmd {
	return func() tea.Msg {
		app.model.Mode = models.ChatListMode
		return app.refreshChatSessions()()
	}
}

// refreshChatSessions refreshes the list of chat sessions
func (app *Application) refreshChatSessions() tea.Cmd {
	return func() tea.Msg {
		_, err := app.db.GetChatSessions()
		if err != nil {
			return models.ChatsRefreshedMsg{Err: err}
		}
		
		// Update the chat list view with sessions
		// This would need to be implemented in the UI layer
		
		return models.ChatsRefreshedMsg{Err: nil}
	}
}

// switchModelInChat switches the model for the current chat session
func (app *Application) switchModelInChat(modelName string) tea.Cmd {
	return func() tea.Msg {
		// Switch back to chat mode first
		app.model.Mode = models.ChatMode
		
		// Update the current session's model
		if err := app.db.UpdateChatSessionModel(app.model.CurrentSessionID, modelName); err != nil {
			return models.AIResponseMsg{Response: "", Err: fmt.Errorf("Failed to update session model: %v", err)}
		}
		
		// No system message - clean switching
		
		// Update Ollama's current model
		app.ollama.SetCurrentModel(modelName)
		
		// Update input placeholder
		app.model.Input.Placeholder = fmt.Sprintf("Chat with %s", modelName)
		app.model.Input.Focus()
		
		return nil
	}
}

// handleModelSelection handles model selection in chat mode
func (app *Application) handleModelSelection() tea.Cmd {
	// Get selected item from model manager
	var selected *ui.ModelManagerItem
	if app.useCleanModels {
		selected = app.cleanModelManager.GetSelectedModel()
	} else {
		selected = app.modelManagerView.GetSelectedModel()
	}
	if selected == nil {
		return nil
	}

	item := *selected
	if item.IsHeader || item.IsSeparator {
		return nil
	}

	// Only allow switching to installed models
	if item.State != services.ModelStateComplete {
		return func() tea.Msg {
			return models.AIResponseMsg{Response: "", Err: fmt.Errorf("Model %s is not installed. Please download it first", item.Name)}
		}
	}

	// Switch model in current chat and return to chat mode
	return app.switchModelInChat(item.Name)
}

// handleChatSelection handles chat selection from chat history
func (app *Application) handleChatSelection() tea.Cmd {
	return func() tea.Msg {
		// This would handle selecting a chat from the list
		// For now, just return to chat mode
		app.model.Mode = models.ChatMode
		currentModel := app.ollama.GetCurrentModel()
		if currentModel == "No model selected" || currentModel == "" {
			app.model.Input.Placeholder = "Message assistant... (No model selected)"
		} else {
			app.model.Input.Placeholder = fmt.Sprintf("Chat with %s", currentModel)
		}
		return nil
	}
}

// getQueueStatus returns information about the download queue
func (app *Application) getQueueStatus() string {
	// Get active downloads from Ollama service
	activeDownloads := app.ollama.GetActiveDownloads()
	if len(activeDownloads) == 0 {
		return ""
	}
	
	if len(activeDownloads) == 1 {
		for model, progress := range activeDownloads {
			return fmt.Sprintf("%s (%d%%)", model, progress.Percent)
		}
	}
	
	return fmt.Sprintf("%d models downloading", len(activeDownloads))
}

// startModelDownload starts downloading a model
func (app *Application) startModelDownload(modelName string) tea.Cmd {
	// Mark model as downloading in UI immediately (only for old model manager)
	if !app.useCleanModels {
		app.modelManagerView.SetModelDownloading(modelName)
	}
	// Clean model manager handles this automatically via download manager
	
	// Start the download
	if app.useCleanModels {
		// Use the new download manager
		err := app.ollama.GetDownloadManager().StartDownload(modelName)
		if err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("Failed to start download: %v", err))
		}
		// Progress tracking is handled automatically by the download manager
		return nil
	} else {
		// Use the old download method
		go func() {
			err := app.ollama.PullModel(modelName)
			if err != nil {
				// Error will be shown through progress tracking
			}
		}()
		
		// Start progress tracking immediately
		return app.startProgressTracking(modelName)
	}
}

// startProgressTracking starts tracking progress for a model download
func (app *Application) startProgressTracking(modelName string) tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return models.ProgressTickMsg{Model: modelName}
	})
}

// checkProgress checks the current progress of a model download
func (app *Application) checkProgress(modelName string) tea.Cmd {
	return func() tea.Msg {
		// Get current progress from Ollama service
		progress := app.ollama.GetProgress(modelName)
		if progress != nil {
			return models.ModelProgressMsg{Progress: progress}
		}
		return nil
	}
}

// showDeleteConfirmation shows confirmation dialog for model deletion
func (app *Application) showDeleteConfirmation() tea.Cmd {
	return func() tea.Msg {
		// Get selected item
		var selected *ui.ModelManagerItem
		if app.useCleanModels {
			selected = app.cleanModelManager.GetSelectedModel()
		} else {
			selected = app.modelManagerView.GetSelectedModel()
		}
		if selected == nil {
			return nil
		}

		item := *selected
		if item.IsHeader || item.IsSeparator {
			return nil
		}

		// Only allow deleting complete models
		if item.State != services.ModelStateComplete {
			return models.AIResponseMsg{Response: "", Err: fmt.Errorf("Cannot delete model %s - not fully downloaded", item.Name)}
		}

		// Set up confirmation dialog
		app.model.Mode = models.ConfirmationMode
		app.model.ConfirmationAction = "delete"
		app.model.ConfirmationTarget = item.Name

		return nil
	}
}

// handleConfirmedAction handles the confirmed action
func (app *Application) handleConfirmedAction() tea.Cmd {
	return func() tea.Msg {
		switch app.model.ConfirmationAction {
		case "delete":
			// Delete the model
			err := app.ollama.DeleteModel(app.model.ConfirmationTarget)
			if err != nil {
				app.model.Mode = models.ModelManagementMode
				return models.ModelDeletedMsg{Model: app.model.ConfirmationTarget, Err: err}
			}
			app.model.Mode = models.ModelManagementMode
			return app.refreshModelStates()()
		}
		
		app.model.Mode = models.ModelManagementMode
		return nil
	}
}

// renderConfirmationMode renders the confirmation dialog
func (app *Application) renderConfirmationMode() string {
	var s string

	// Title
	title := titleStyle.Render("⚠️  Confirmation Required")
	s += title + "\n\n"

	// Confirmation message
	switch app.model.ConfirmationAction {
	case "delete":
		s += fmt.Sprintf("Are you sure you want to delete model '%s'?\n", app.model.ConfirmationTarget)
		s += "This action cannot be undone.\n\n"
	}

	// Options
	s += helpStyle.Render("Press 'y' to confirm, 'n' or ESC to cancel")

	return s
}

// formatBytes converts bytes to human readable format
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

// showModelInfo shows detailed information about the selected model
func (app *Application) showModelInfo() tea.Cmd {
	return func() tea.Msg {
		// Get selected item
		var selected *ui.ModelManagerItem
		if app.useCleanModels {
			selected = app.cleanModelManager.GetSelectedModel()
		} else {
			selected = app.modelManagerView.GetSelectedModel()
		}
		if selected == nil {
			return nil
		}

		item := *selected
		if item.IsHeader || item.IsSeparator {
			return nil
		}

		// Only show info for installed models
		if item.State != services.ModelStateComplete {
			return models.ModelInfoMsg{
				ModelName: item.Name,
				Info:      nil,
				Err:       fmt.Errorf("Model %s is not installed. Install it first to view details", item.Name),
			}
		}

		// Get model info using ollama show
		modelInfo, err := app.ollama.GetModelInfo(item.Name)
		return models.ModelInfoMsg{
			ModelName: item.Name,
			Info:      modelInfo,
			Err:       err,
		}
	}
}

// renderModelInfoMode renders the model information display
func (app *Application) renderModelInfoMode() string {
	var s string

	// Title
	title := titleStyle.Render(fmt.Sprintf("📋 Model Information: %s", app.model.ModelInfoTarget))
	s += title + "\n\n"

	if app.model.ModelInfoData == nil {
		s += "No model information available.\n"
		s += helpStyle.Render("Press ESC to return to model management")
		return s
	}

	// Cast to ModelInfo
	if modelInfo, ok := app.model.ModelInfoData.(*services.ModelInfo); ok {
		// Basic Information
		s += modeStyle.Render("Basic Information") + "\n"
		s += fmt.Sprintf("Name:         %s\n", modelInfo.Name)
		s += fmt.Sprintf("Size:         %s\n", formatBytes(modelInfo.Size))
		s += fmt.Sprintf("Modified:     %s\n", modelInfo.ModifiedAt)
		s += fmt.Sprintf("Format:       %s\n", modelInfo.Details.Format)
		s += "\n"

		// Model Details
		s += modeStyle.Render("Model Details") + "\n"
		s += fmt.Sprintf("Family:       %s\n", modelInfo.Details.Family)
		s += fmt.Sprintf("Parameters:   %s\n", modelInfo.Details.ParameterSize)
		if modelInfo.Details.QuantizationLevel != "" {
			s += fmt.Sprintf("Quantization: %s\n", modelInfo.Details.QuantizationLevel)
		}
		s += "\n"

		// Parameters (if available)
		if len(modelInfo.Parameters) > 0 {
			s += modeStyle.Render("Parameters") + "\n"
			for key, value := range modelInfo.Parameters {
				s += fmt.Sprintf("%-15s %v\n", key+":", value)
			}
			s += "\n"
		}

		// License (if available)
		if modelInfo.License != "" {
			s += modeStyle.Render("License") + "\n"
			// Truncate license if too long
			license := modelInfo.License
			if len(license) > 200 {
				license = license[:200] + "..."
			}
			s += license + "\n\n"
		}
	} else {
		s += "Failed to parse model information.\n\n"
	}

	// Help
	s += helpStyle.Render("Press ESC to return to model management")

	return s
}

// renderImageGenerationMode renders the image generation interface
func (app *Application) renderImageGenerationMode() string {
	return app.imageGeneratorView.View()
}

// generateImage generates an image using the specified model and parameters
func (app *Application) generateImage(model, prompt string, params ui.ImageGenerationParams) tea.Cmd {
	return func() tea.Msg {
		// Convert UI params to service params
		serviceParams := services.GenerationParameters{
			Steps:    params.Steps,
			Guidance: params.Guidance,
			Width:    params.Width,
			Height:   params.Height,
		}
		
		// Generate image using the service
		imagePath, err := app.imageGenerator.GenerateImage(model, prompt, serviceParams)
		
		return ui.ImageGeneratedMsg{
			ImagePath: imagePath,
			Err:       err,
		}
	}
}

// installOllama installs Ollama on the system
func (app *Application) installOllama() tea.Cmd {
	return func() tea.Msg {
		if app.ollama.IsInstalled() {
			app.chatView.AddMessage("system", "✅ Ollama is already installed!")
			return nil
		}

		app.chatView.AddMessage("system", "Installing Ollama... This may take a few minutes.")
		
		// Run the installation
		if err := app.ollama.InstallOllama(); err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf(`❌ Failed to install Ollama: %v

To install manually on Linux, run:
curl -fsSL https://ollama.com/install.sh | sh

For other platforms, visit: https://ollama.com/download`, err))
		} else {
			app.chatView.AddMessage("system", "✅ Ollama installed successfully! Starting service...")
			// Try to start the service
			if err := app.ollama.StartService(); err != nil {
				app.chatView.AddMessage("system", fmt.Sprintf("⚠️  Installed but failed to start: %v\n\nTry: ollama serve", err))
			} else {
				app.chatView.AddMessage("system", "✅ Ollama is now running! You can start downloading models.")
			}
		}
		return nil
	}
}

// showHelp displays available commands
func (app *Application) showHelp() tea.Cmd {
	return func() tea.Msg {
		helpText := `📚 TRMS - Terminal Resource Management Studio

🔧 Slash Commands (type in chat):
  /models, /m         - Open model manager (90+ models available!)
  /image, /img        - Enter image generation mode
  /install-ollama     - Install Ollama
  /status             - Show container status
  /restart-containers - Restart all containers
  /logs               - Show container logs
  /reset              - Reset all containers and volumes
  /deps               - Check system dependencies
  /quit, /q           - Exit application
  /help, /h           - Show this help

💬 Chat Mode (default):
  Ctrl+M           - Model manager shortcut
  Ctrl+S           - Switch model
  Ctrl+N           - New chat
  Ctrl+H           - Show chat history
  Ctrl+P           - Show copy history
  Ctrl+C           - Quit

🤖 Model Manager (90+ Models Available):
  ⚡ Lightweight    - phi, tinyllama, orca-mini (2-4GB RAM)
  🤖 General       - llama3.2, mistral, qwen2.5 (4-8GB RAM)  
  💻 Programming   - codellama, deepseek-coder, magicoder
  👁️ Vision        - llava, moondream, llama3.2-vision
  🎨 Image Gen     - stable-diffusion, flux, sdxl, playground-v2.5
  🧮 Mathematics   - mathstral, qwen2-math, deepseek-math
  🌍 Multilingual  - aya (101 languages), command-r
  🔗 Embeddings    - nomic-embed-text, mxbai-embed-large
  ✍️ Creative      - dolphin models, wizard-vicuna
  🎯 Specialized   - Medical, SQL, and domain-specific models

📋 Model Manager Keys:
  i                - Install/download model
  Enter            - Show model information
  d                - Delete model
  c                - Clean partial download
  r                - Resume/restart download
  R                - Refresh model list

🎨 Image Generation:
  Enter            - Generate image from prompt
  s                - Save current image to Downloads
  d                - Delete current image
  ←/→              - Navigate between images
  ESC              - Return to chat mode

🐳 Container Management:
  All services run in Docker containers for better isolation and management
  PostgreSQL: Database for chat history and metadata
  Ollama: AI model serving engine
  
🚀 Quick Start:
  /install-ollama  - Install Ollama (Linux: curl -fsSL ollama.com/install.sh | sh)
  /status          - Check system status
  /help            - Show this help

All models are organized by category and memory requirements!`

		app.chatView.AddMessage("system", helpText)
		return nil
	}
}

// showContainerStatus shows the status of all containers
func (app *Application) showContainerStatus() tea.Cmd {
	return func() tea.Msg {
		containerManager := services.NewContainerManager()
		statuses := containerManager.GetAllContainerStatuses()
		
		statusText := "🐳 Container Status:\n\n"
		
		for name, status := range statuses {
			emoji := "❌"
			if status.Running {
				emoji = "✅"
			} else if status.Exists {
				emoji = "⏸️"
			}
			
			statusText += fmt.Sprintf("%s %s:\n", emoji, strings.ToUpper(name))
			statusText += fmt.Sprintf("  Exists: %v\n", status.Exists)
			statusText += fmt.Sprintf("  Running: %v\n", status.Running)
			if status.Status != "" {
				statusText += fmt.Sprintf("  Status: %s\n", status.Status)
			}
			if status.Health != "" {
				statusText += fmt.Sprintf("  Health: %s\n", status.Health)
			}
			if len(status.Ports) > 0 {
				statusText += fmt.Sprintf("  Ports: %s\n", strings.Join(status.Ports, ", "))
			}
			statusText += "\n"
		}
		
		app.chatView.AddMessage("system", statusText)
		return nil
	}
}

// restartContainers restarts all containers
func (app *Application) restartContainers() tea.Cmd {
	return func() tea.Msg {
		containerManager := services.NewContainerManager()
		
		app.chatView.AddMessage("system", "🔄 Restarting all containers...")
		
		// Restart containers
		if err := containerManager.RestartContainer("trms-postgres"); err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("❌ Failed to restart PostgreSQL: %v", err))
		} else {
			app.chatView.AddMessage("system", "✅ PostgreSQL restarted")
		}
		
		if err := containerManager.RestartContainer("trms-ollama"); err != nil {
			app.chatView.AddMessage("system", fmt.Sprintf("❌ Failed to restart Ollama: %v", err))
		} else {
			app.chatView.AddMessage("system", "✅ Ollama restarted")
		}
		
		return nil
	}
}

// showContainerLogs shows logs from containers
func (app *Application) showContainerLogs() tea.Cmd {
	return func() tea.Msg {
		containerManager := services.NewContainerManager()
		
		// Show PostgreSQL logs
		if logs, err := containerManager.GetContainerLogs("trms-postgres", 10); err == nil {
			app.chatView.AddMessage("system", fmt.Sprintf("📋 PostgreSQL Logs (last 10 lines):\n%s", logs))
		} else {
			app.chatView.AddMessage("system", fmt.Sprintf("❌ Failed to get PostgreSQL logs: %v", err))
		}
		
		// Show Ollama logs
		if logs, err := containerManager.GetContainerLogs("trms-ollama", 10); err == nil {
			app.chatView.AddMessage("system", fmt.Sprintf("📋 Ollama Logs (last 10 lines):\n%s", logs))
		} else {
			app.chatView.AddMessage("system", fmt.Sprintf("❌ Failed to get Ollama logs: %v", err))
		}
		
		return nil
	}
}

// resetContainers resets all containers and volumes
func (app *Application) resetContainers() tea.Cmd {
	return func() tea.Msg {
		containerManager := services.NewContainerManager()
		
		app.chatView.AddMessage("system", "🔄 Resetting all containers and volumes...")
		app.chatView.AddMessage("system", "⚠️  This will delete all chat history and downloaded models!")
		
		// Remove containers
		containerManager.RemoveContainer("trms-postgres")
		containerManager.RemoveContainer("trms-ollama")
		
		// Remove volumes
		exec.Command("docker", "volume", "rm", "trms_postgres_data").Run()
		exec.Command("docker", "volume", "rm", "trms_ollama").Run()
		
		app.chatView.AddMessage("system", "✅ Containers and volumes reset successfully!")
		app.chatView.AddMessage("system", "Use /restart-containers to recreate them.")
		
		return nil
	}
}

// checkDependencies checks system dependencies
func (app *Application) checkDependencies() tea.Cmd {
	return func() tea.Msg {
		depManager := services.NewDependencyManager()
		
		app.chatView.AddMessage("system", "🔍 Checking system dependencies...")
		
		// Check Docker
		if depManager.IsDockerInstalled() {
			if depManager.IsDockerRunning() {
				app.chatView.AddMessage("system", "✅ Docker: Installed and running")
			} else {
				app.chatView.AddMessage("system", "⚠️  Docker: Installed but not running")
			}
		} else {
			app.chatView.AddMessage("system", "❌ Docker: Not installed")
		}
		
		// Check other dependencies
		deps := map[string]func() bool{
			"curl": depManager.IsCurlInstalled,
			"git":  depManager.IsGitInstalled,
		}
		
		for name, checker := range deps {
			if checker() {
				app.chatView.AddMessage("system", fmt.Sprintf("✅ %s: Installed", name))
			} else {
				app.chatView.AddMessage("system", fmt.Sprintf("❌ %s: Not installed", name))
			}
		}
		
		app.chatView.AddMessage("system", "\nUse /deps to install missing dependencies")
		
		return nil
	}
}

func main() {
	var (
		showVersion    = flag.Bool("version", false, "Show version information")
		showStatus     = flag.Bool("status", false, "Show container status and exit")
		skipChecks     = flag.Bool("skip-checks", false, "Skip startup dependency checks")
		resetContainers = flag.Bool("reset", false, "Reset all containers and volumes")
		debugMode      = flag.Bool("debug", false, "Enable debug mode")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("TRMS (Terminal Resource Management Studio) v1.0.0")
		fmt.Println("Enhanced with dependency management and containerization")
		return
	}

	containerManager := services.NewContainerManager()

	if *showStatus {
		containerManager.ShowStatus()
		return
	}

	if *resetContainers {
		fmt.Println("🔄 Resetting all TRMS containers...")
		containerManager.RemoveContainer("trms-postgres")
		containerManager.RemoveContainer("trms-ollama")
		
		// Remove volumes
		exec.Command("docker", "volume", "rm", "trms_postgres_data").Run()
		exec.Command("docker", "volume", "rm", "trms_ollama").Run()
		
		fmt.Println("✅ Containers and volumes reset successfully!")
		return
	}

	if !*skipChecks {
		// Perform startup checks
		if err := containerManager.StartupCheck(); err != nil {
			fmt.Printf("Startup check failed: %v\n", err)
			fmt.Println("Use --skip-checks to bypass this check.")
			fmt.Println("Some features may be limited. Continuing...")
		}
	}

	// Create and run the application
	app := New()
	
	if *debugMode {
		fmt.Println("🐛 Debug mode enabled")
		app.model.DebugMode = true
	}

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseAllMotion())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}