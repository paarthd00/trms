package main

import (
	"flag"
	"fmt"
	"os"

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

	// Create chat view
	chatView := ui.NewChatView(80, 25)

	// Create model manager view
	modelManagerView := ui.NewModelManagerView(80, 25)

	// Create services
	ollamaService := services.NewOllamaService()
	dbService := services.NewDatabaseService()

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

		// Update chat view size
		app.chatView, cmd = app.chatView.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
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
			case models.ChatMode, models.ModelManagementMode:
				app.model.Mode = models.CommandMode
				app.model.Input.Placeholder = "Type a command..."
				app.model.Input.Focus()
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
					// Add thinking indicator
					app.chatView.AddMessage("assistant", "Thinking...")
					// Send to Ollama for real response
					return app, app.performAI(prompt)
				}
			case models.ModelManagementMode:
				// Download model
				return app, app.handleModelAction("download")
			}
		
		default:
			// Handle Ctrl+M for model management
			if msg.String() == "ctrl+m" {
				app.model.Mode = models.ModelManagementMode
				return app, app.refreshModelStates()
			}
			
			// Handle other keys for model management
			if app.model.Mode == models.ModelManagementMode {
				switch msg.String() {
				case "d", "delete":
					return app, app.handleModelAction("delete")
				case "c":
					return app, app.handleModelAction("clean")
				}
			}
		}

		// Mode-specific updates
		switch app.model.Mode {
		case models.ChatMode:
			// If in chat mode, also update chat view for scrolling
			app.chatView, cmd = app.chatView.Update(msg)
			cmds = append(cmds, cmd)
		case models.ModelManagementMode:
			// Update model manager view
			app.modelManagerView, cmd = app.modelManagerView.Update(msg)
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
		} else {
			// Add AI response to chat
			app.chatView.AddMessage("assistant", msg.Response)
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
	}

	return content
}

// renderCommandMode renders the command mode interface
func (app *Application) renderCommandMode() string {
	var s string

	// Title
	title := titleStyle.Render("⚡ Terminal AI Studio")
	s += title + "\n\n"

	// Mode indicator
	mode := modeStyle.Render("Command Mode")
	s += mode + "\n\n"

	// Input
	s += "$ " + app.model.Input.View() + "\n\n"

	// Help
	help := helpStyle.Render("Tab: Chat • Ctrl+M: Models • Ctrl+C: Quit • Enter: Execute • Type 'models' or 'm'")
	s += help

	return s
}

// renderChatMode renders the chat mode interface
func (app *Application) renderChatMode() string {
	var s string

	// Title
	title := titleStyle.Render("⚡ Terminal AI Studio")
	s += title + "\n"

	// Mode indicator with model info
	currentModel := app.ollama.GetCurrentModel()
	mode := modeStyle.Render("Chat Mode - " + currentModel)
	s += mode + "\n\n"

	// Chat view
	s += app.chatView.View() + "\n"

	// Input
	s += "› " + app.model.Input.View() + "\n"

	// Help
	help := helpStyle.Render("Tab: Command • Esc: Back • ↑/↓: Scroll • v: Select • Ctrl+Y: Copy • g/G: Top/Bottom")
	s += help

	return s
}

// renderModelManagementMode renders the model management interface
func (app *Application) renderModelManagementMode() string {
	var s string

	// Title with system info
	availableMem := services.FormatMemory(app.systemInfo.AvailableMemory)
	totalMem := services.FormatMemory(app.systemInfo.TotalMemory)
	title := titleStyle.Render("⚡ Model Management")
	memInfo := helpStyle.Render(fmt.Sprintf("System Memory: %s / %s available", availableMem, totalMem))
	s += title + "  " + memInfo + "\n\n"

	// Recommendations based on available memory
	availableGB := float64(app.systemInfo.AvailableMemory) / (1024 * 1024 * 1024)
	if availableGB < 8 {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("⚠️  Limited memory - recommended models: phi, orca-mini, tinyllama") + "\n\n"
	}

	// Model manager view
	s += app.modelManagerView.View()

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
	default:
		// Execute as shell command
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
			app.model.CommandOutput = "Ollama is not installed. Please install Ollama to use chat features."
		} else if !app.ollama.IsRunning() {
			// Try to start Ollama
			if err := app.ollama.StartService(); err != nil {
				app.model.CommandOutput = "Ollama is installed but not running. Failed to start: " + err.Error()
			} else {
				// Refresh models after starting
				app.ollama.RefreshModels()
			}
		} else {
			// Ollama is running, refresh models
			app.ollama.RefreshModels()
		}
		return nil
	}
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		MarginBottom(1)

	modeStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("99")).
		Foreground(lipgloss.Color("230")).
		Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
)

// refreshModelStates refreshes the model states including partial downloads
func (app *Application) refreshModelStates() tea.Cmd {
	return func() tea.Msg {
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

		return models.ModelsRefreshedMsg{Err: nil}
	}
}

// handleModelAction handles model management actions
func (app *Application) handleModelAction(action string) tea.Cmd {
	return func() tea.Msg {
		// Get selected item from model manager
		list := app.modelManagerView.GetList()
		selected := list.SelectedItem()
		if selected == nil {
			return nil
		}

		item, ok := selected.(ui.ModelManagerItem)
		if !ok || item.IsHeader || item.IsSeparator {
			return nil
		}

		switch action {
		case "download":
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
			// Pull the model
			err := app.ollama.PullModel(item.Name)
			if err != nil {
				return models.AIResponseMsg{
					Response: "",
					Err:      err,
				}
			}
			return app.refreshModelStates()()

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
		}

		return nil
	}
}

func main() {
	flag.Parse()

	// Create and run the application
	app := New()
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseAllMotion())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}