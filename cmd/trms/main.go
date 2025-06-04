package main

import (
	"flag"
	"fmt"
	"os"
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
	model            models.AppModel
	chatView         ui.ChatView
	modelManagerView ui.ModelManagerView
	width            int
	height           int
	ollama           *services.OllamaService
	db               *services.DatabaseService
	systemInfo       *services.SystemInfo
}

// New creates a new Application instance
func New() *Application {
	// Create input field
	ti := textinput.New()
	ti.Placeholder = "Type a command..."
	ti.Focus()
	ti.CharLimit = 256
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Create chat view with initial size (will be updated on WindowSizeMsg)
	chatView := ui.NewChatView(80, 24)

	// Create model manager view with initial size
	modelManagerView := ui.NewModelManagerView(80, 24)

	// Create services
	ollamaService := services.NewOllamaService()
	dbService := services.NewDatabaseService()
	
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
			Mode:             models.CommandMode,
			Input:            ti,
			CurrentSessionID: 1,
			ChatHistory:      []models.Message{},
			Width:            80,
			Height:           25,
		},
		chatView:         chatView,
		modelManagerView: modelManagerView,
		width:            80,
		height:           25,
		ollama:           ollamaService,
		db:               dbService,
		systemInfo:       systemInfo,
	}

	return app
}

// Init initializes the application
func (app *Application) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		tea.EnterAltScreen,
		app.checkOllama(),
	)
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
		app.chatView, cmd = app.chatView.Update(msg)
		cmds = append(cmds, cmd)
		
		// Update model manager view size
		app.modelManagerView, cmd = app.modelManagerView.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		// First, handle mode-specific keyboard updates for navigation
		// This ensures arrow keys and other navigation keys are properly forwarded
		switch app.model.Mode {
		case models.ChatMode:
			app.chatView, cmd = app.chatView.Update(msg)
			cmds = append(cmds, cmd)
		case models.ModelManagementMode, models.ModelSelectionMode, models.CategorySelectionMode:
			app.modelManagerView, cmd = app.modelManagerView.Update(msg)
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

		case tea.KeyTab:
			// Toggle between modes
			if app.model.Mode == models.CommandMode {
				app.model.Mode = models.ChatMode
				currentModel := app.ollama.GetCurrentModel()
				if currentModel == "No model selected" || currentModel == "" {
					app.model.Input.Placeholder = "Message assistant... (No model selected)"
				} else {
					app.model.Input.Placeholder = fmt.Sprintf("Chat with %s", currentModel)
				}
			} else if app.model.Mode == models.ChatMode {
				app.model.Mode = models.CommandMode
				app.model.Input.Placeholder = "Type a command..."
			}
			app.model.Input.Focus()
			return app, nil

		case tea.KeyEsc:
			// Handle escape for different modes
			switch app.model.Mode {
			case models.ChatMode, models.ModelManagementMode, models.ChatListMode:
				app.model.Mode = models.CommandMode
				app.model.Input.Placeholder = "Type a command..."
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
			case models.CommandMode:
				input := app.model.Input.Value()
				if input != "" {
					app.model.Input.Reset()
					return app, app.handleCommand(input)
				}
			case models.ChatMode:
				prompt := app.model.Input.Value()
				if prompt != "" {
					app.model.Input.Reset()
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
				// Download model
				return app, app.handleModelAction("download")
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
				if app.model.Mode == models.ChatMode || app.model.Mode == models.CommandMode {
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
				case "i", "info":
					return app, app.showModelInfo()
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
		// Update progress in model manager view
		if msg.Progress != nil {
			// Update the model manager with progress
			app.modelManagerView.UpdateProgress(msg.Progress.Model, msg.Progress.Percent, msg.Progress.Downloaded, msg.Progress.Total)
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
			// Update progress in UI
			app.modelManagerView.UpdateProgress(msg.Model, progress.Percent, progress.Downloaded, progress.Total)
			
			// Check if download is complete
			if progress.Percent >= 100 || progress.Status == "Download complete" {
				// Download complete - refresh model states immediately (no duplicate message)
				return app, app.refreshModelStates()
			} else {
				// Continue ticking if still downloading
				return app, app.startProgressTracking(msg.Model)
			}
		} else {
			// No progress found - model might be complete or failed
			// Refresh to get current state
			return app, app.refreshModelStates()
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
	case models.CommandMode:
		content = app.renderCommandMode()
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

// renderCommandMode renders the command mode interface
func (app *Application) renderCommandMode() string {
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
			Render("Command Mode"),
	)
	s += header + "\n"
	
	// Subtle separator
	s += app.renderSeparator() + "\n\n"

	// Clean input area with prompt
	inputArea := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true).
			Render("$ "),
		app.model.Input.View(),
	)
	
	// Wrap input in clean border
	styledInput := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(inputArea)
	s += styledInput + "\n\n"

	// Clean help
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render("Tab: Chat Mode • Ctrl+M: Model Manager • 'help' for commands")
	s += help

	return s
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
		helpText = "Tab: Command Mode • Ctrl+S: Switch Model • Click 📋 to copy • Ctrl+P: History"
	} else if app.width > 80 {
		helpText = "Tab: Command • Ctrl+S: Switch • 📋 Copy • Ctrl+P: History"
	} else {
		helpText = "Tab: Cmd • Ctrl+S: Switch • 📋 Copy"
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
	s += app.modelManagerView.View() + "\n"

	// Download queue status
	queueInfo := app.getQueueStatus()
	if queueInfo != "" {
		queue := lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Render("📥 " + queueInfo)
		s += "\n" + queue + "\n"
	}

	// Clean help
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render("Enter: Download • Tab/T: Categories • d: Delete • c: Clean • r: Restart • i: Info • ESC: Back")
	s += help

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
	s += "Press ESC to return to command mode\n"

	return s
}

// handleCommand processes command input
func (app *Application) handleCommand(input string) tea.Cmd {
	switch input {
	case "q", "quit", "exit":
		return tea.Quit
	case "chat", "c":
		app.model.Mode = models.ChatMode
		currentModel := app.ollama.GetCurrentModel()
		if currentModel == "No model selected" || currentModel == "" {
			app.model.Input.Placeholder = "Message assistant... (No model selected)"
		} else {
			app.model.Input.Placeholder = fmt.Sprintf("Chat with %s", currentModel)
		}
		return nil
	case "models", "m":
		app.model.Mode = models.ModelManagementMode
		return app.refreshModelStates()
	case "install-ollama":
		// Install Ollama directly from TRMS
		return app.installOllama()
	case "help", "h", "?":
		// Show help
		return app.showHelp()
	default:
		// Show unknown command message
		app.chatView.AddMessage("system", fmt.Sprintf("Unknown command: '%s'. Type 'help' for available commands.", input))
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
			// Don't add a message here - it's running fine
		}
		return nil
	}
}

// checkAndStartDatabase checks if the database container is running and starts it if needed
func checkAndStartDatabase() error {
	db := services.NewDatabaseService()
	
	// Check if Docker is installed
	if !db.IsDockerInstalled() {
		return fmt.Errorf("Docker is not installed. Please install Docker to use database features")
	}
	
	// Check if PostgreSQL container is running
	if !db.IsPostgresRunning() {
		fmt.Println("PostgreSQL container is not running. Starting it now...")
		if err := db.SetupPostgres(); err != nil {
			return fmt.Errorf("failed to start PostgreSQL container: %v", err)
		}
		fmt.Println("PostgreSQL container started successfully.")
	}
	
	// Connect to the database
	fmt.Println("Connecting to database...")
	if err := db.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	fmt.Println("Database connected successfully.")
	
	// Keep the connection for later use
	db.Disconnect() // Disconnect for now, will reconnect when needed
	
	return nil
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

		app.modelManagerView.SetModelStates(states, enrichedModels)
		
		// Update current model indicator
		currentModel := app.ollama.GetCurrentModel()
		if currentModel != "" && currentModel != "No model selected" {
			app.modelManagerView.SetCurrentModel(currentModel)
		}

		return models.ModelsRefreshedMsg{Err: nil}
	}
}

// handleModelAction handles model management actions
func (app *Application) handleModelAction(action string) tea.Cmd {
	return func() tea.Msg {
		// Get selected item from model manager
		selected := app.modelManagerView.GetSelectedModel()
		if selected == nil {
			return nil
		}

		item := *selected
		if item.IsHeader || item.IsSeparator {
			return nil
		}

		switch action {
		case "download":
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
			// Start pulling the model in background
			go func(modelName string) {
				err := app.ollama.PullModel(modelName)
				// Note: In a proper implementation, we'd need to send this error
				// back to the main UI thread using a channel or similar mechanism
				_ = err
			}(item.Name)
			
			// Immediately update the UI to show download starting
			app.modelManagerView.SetModelDownloading(item.Name)
			
			// Start progress tracking
			return app.startProgressTracking(item.Name)()

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
	selected := app.modelManagerView.GetSelectedModel()
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

// startProgressTracking starts tracking progress for a model download
func (app *Application) startProgressTracking(modelName string) tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
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
		selected := app.modelManagerView.GetSelectedModel()
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
		selected := app.modelManagerView.GetSelectedModel()
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

🔧 Navigation:
  chat, c          - Enter chat mode
  models, m        - Open model manager (90+ models available!)
  Tab              - Toggle between command and chat mode
  Ctrl+C           - Quit

💬 Chat Mode:
  Ctrl+S           - Switch model
  Ctrl+N           - New chat
  Ctrl+H           - Show chat history
  Ctrl+P           - Show copy history

🤖 Model Manager (90+ Models Available):
  ⚡ Lightweight    - phi, tinyllama, orca-mini (2-4GB RAM)
  🤖 General       - llama3.2, mistral, qwen2.5 (4-8GB RAM)  
  💻 Programming   - codellama, deepseek-coder, magicoder
  👁️ Vision        - llava, moondream, llama3.2-vision
  🧮 Mathematics   - mathstral, qwen2-math, deepseek-math
  🌍 Multilingual  - aya (101 languages), command-r
  🔗 Embeddings    - nomic-embed-text, mxbai-embed-large
  ✍️ Creative      - dolphin models, wizard-vicuna
  🎯 Specialized   - Medical, SQL, and domain-specific models

📋 Model Manager Keys:
  Enter            - Download/select model
  d                - Delete model
  c                - Clean partial download
  r                - Resume/restart download
  i                - Show model info
  R                - Refresh model list

🚀 Quick Start:
  install-ollama   - Install Ollama (Linux: curl -fsSL ollama.com/install.sh | sh)
  help, h, ?       - Show this help

All models are organized by category and memory requirements!`

		app.chatView.AddMessage("system", helpText)
		return nil
	}
}

func main() {
	flag.Parse()

	// Check and start database before starting the application
	if err := checkAndStartDatabase(); err != nil {
		fmt.Printf("Database initialization error: %v\n", err)
		fmt.Println("The application will continue without database features.")
		// Continue running the app even if database fails
	}

	// Create and run the application
	app := New()
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseAllMotion())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}