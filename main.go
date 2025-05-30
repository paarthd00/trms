package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Mode int

const (
	CommandMode Mode = iota
	ChatMode
	NewChatMode
	ModelManagementMode
	ChatListMode
)

type ModelItem struct {
	name        string
	description string
	size        string
	installed   bool
	isSeparator bool
	isHeader    bool
}

func (m ModelItem) Title() string {
	if m.isHeader {
		return headerStyle.Render(fmt.Sprintf("━━━ %s ━━━", m.name))
	}
	if m.isSeparator {
		return separatorStyle.Render("────────────────────────────────────────")
	}

	status := "📥" // Download icon
	if m.installed {
		status = "✅" // Installed icon
	}
	return fmt.Sprintf("%s %-25s %8s", status, m.name, m.size)
}

func (m ModelItem) Description() string {
	if m.isHeader || m.isSeparator {
		return ""
	}
	return m.description
}

func (m ModelItem) FilterValue() string {
	if m.isHeader || m.isSeparator {
		return ""
	}
	return m.name
}

type model struct {
	mode             Mode
	input            textinput.Model
	viewport         viewport.Model
	modelList        list.Model
	chatList         list.Model
	width            int
	height           int
	commandOutput    string
	err              error
	quitting         bool
	ollama           *OllamaManager
	db               *DatabaseManager
	pullingModel     string
	pullProgress     int
	currentSessionID int
	chatHistory      []Message
	showingModels    bool
	dbSetupProgress  string
	modelProgress    *PullProgress
	showingProgress  bool
	installedModels  []ModelItem
	availableModels  []ModelItem
}

type commandFinishedMsg struct {
	output string
	err    error
}

type aiResponseMsg struct {
	response string
	err      error
}

type installOllamaMsg struct{}

type ollamaInstalledMsg struct {
	err error
}

type setupDatabaseMsg struct{}

type databaseSetupMsg struct {
	err error
}

type modelPulledMsg struct {
	model string
	err   error
}

type modelProgressMsg struct {
	progress *PullProgress
}

type modelDeletedMsg struct {
	model string
	err   error
}

type pullCancelledMsg struct{}

type newChatMsg struct {
	sessionID int
	err       error
}

type tickMsg struct{}

type modelsRefreshedMsg struct {
	err error
}

type chatsRefreshedMsg struct {
	err error
}

type ChatSessionItem struct {
	session ChatSession
}

func (c ChatSessionItem) Title() string {
	return fmt.Sprintf("[%d] %s", c.session.ID, c.session.Name)
}

func (c ChatSessionItem) Description() string {
	return fmt.Sprintf("Model: %s | Updated: %s", c.session.ModelName, c.session.UpdatedAt.Format("Jan 2, 15:04"))
}

func (c ChatSessionItem) FilterValue() string { return c.session.Name }

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	modelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)

	modeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230")).
			Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// Rich text styles for AI responses
	codeBlockStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(1, 2).
			Margin(1, 0)

	inlineCodeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("238")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	userMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true).
				MarginTop(1).
				MarginBottom(1)

	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Bold(true).
				MarginTop(1).
				MarginBottom(1)

	messageContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				MarginLeft(2)
)

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Type a command or press Tab for AI..."
	ti.Focus()
	ti.CharLimit = 256

	vp := viewport.New(80, 25)
	vp.SetContent("")
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	modelListItems := []list.Item{}
	ml := list.New(modelListItems, list.NewDefaultDelegate(), 80, 20)
	ml.Title = "Available Models (/ to search)"
	ml.SetShowHelp(false)
	ml.SetFilteringEnabled(true)

	chatListItems := []list.Item{}
	cl := list.New(chatListItems, list.NewDefaultDelegate(), 80, 20)
	cl.Title = "Chat Sessions (/ to search)"
	cl.SetShowHelp(false)
	cl.SetFilteringEnabled(true)

	return model{
		mode:             CommandMode,
		input:            ti,
		viewport:         vp,
		modelList:        ml,
		chatList:         cl,
		ollama:           NewOllamaManager(),
		db:               NewDatabaseManager(),
		currentSessionID: 1, // Default session
		chatHistory:      []Message{},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.checkDatabase(),
		m.checkOllama(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 6   // Account for border and padding
		m.viewport.Height = msg.Height - 8 // More space for content
		m.modelList.SetSize(msg.Width, msg.Height-5)
		m.chatList.SetSize(msg.Width, msg.Height-5)

	case tea.KeyMsg:
		// Handle Ollama installation prompt
		if m.commandOutput != "" && strings.Contains(m.commandOutput, "install Ollama") {
			if msg.String() == "y" || msg.String() == "Y" {
				m.viewport.SetContent("Installing Ollama... This may take a few minutes.")
				return m, m.installOllama()
			} else if msg.String() == "n" || msg.String() == "N" {
				m.commandOutput = ""
				m.viewport.SetContent("")
				return m, nil
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyTab:
			// Simple toggle between Command and Chat
			switch m.mode {
			case CommandMode:
				m.mode = ChatMode
				m.input.Placeholder = fmt.Sprintf("Chat with %s (Ctrl+N: new, Ctrl+S: switch, Ctrl+L: manage, Ctrl+H: history)", m.ollama.GetCurrentModel())
				return m, m.loadChatHistory()
			case ChatMode:
				if m.showingModels {
					m.showingModels = false
					m.input.Placeholder = fmt.Sprintf("Chat with %s (Ctrl+N: new, Ctrl+S: switch, Ctrl+L: manage, Ctrl+H: history)", m.ollama.GetCurrentModel())
					m.input.Focus()
				} else {
					m.mode = CommandMode
					m.input.Placeholder = "Type a command or press Tab for chat..."
				}
			}
			m.input.Reset()
			m.input.Focus()
			return m, nil

		case tea.KeyEsc:
			switch m.mode {
			case NewChatMode, ModelManagementMode, ChatListMode:
				m.mode = CommandMode
				m.input.Placeholder = "Type a command or press Tab for chat..."
				m.input.Focus()
			case ChatMode:
				if m.showingModels {
					m.showingModels = false
					m.input.Placeholder = fmt.Sprintf("Chat with %s (Ctrl+N: new, Ctrl+S: switch, Ctrl+L: manage, Ctrl+H: history)", m.ollama.GetCurrentModel())
					m.input.Focus()
				} else {
					m.mode = CommandMode
					m.input.Placeholder = "Type a command or press Tab for chat..."
					m.input.Focus()
				}
			}
			return m, nil

		case tea.KeyCtrlN:
			// Create new chat (Ctrl+N)
			if m.mode == ChatMode {
				m.mode = NewChatMode
				return m, m.refreshInstalledModels()
			}

		case tea.KeyCtrlL:
			// Model management (Ctrl+L) - Changed from Ctrl+M to avoid conflict with Enter
			if m.mode == ChatMode || m.mode == CommandMode {
				m.mode = ModelManagementMode
				return m, m.refreshModels()
			}

		case tea.KeyCtrlH:
			// Chat history (Ctrl+H)
			if m.mode == ChatMode || m.mode == CommandMode {
				m.mode = ChatListMode
				return m, m.refreshChats()
			}

		case tea.KeyCtrlS:
			// Quick model switch in current chat (Ctrl+S)
			if m.mode == ChatMode {
				m.showingModels = true
				return m, m.refreshInstalledModels()
			}

		case tea.KeyCtrlG:
			// Cancel model pull (Ctrl+G for "Go/Cancel")
			if m.mode == ModelManagementMode && m.ollama.IsPulling() {
				m.viewport.SetContent("Cancelling download...")
				return m, m.cancelPull()
			}
		case tea.KeyCtrlD:
			// Delete models (Ctrl+D)
			if m.mode == ModelManagementMode {
				if item, ok := m.modelList.SelectedItem().(ModelItem); ok && item.installed && !item.isHeader && !item.isSeparator {
					// Show confirmation before deletion
					m.viewport.SetContent(fmt.Sprintf("Deleting model '%s'...", item.name))
					return m, m.deleteModel(item.name)
				}
			}
		case tea.KeyEnter:
			switch m.mode {
			case CommandMode:
				input := m.input.Value()
				if input == "" {
					return m, nil
				}
				m.input.Reset()

				// Simple shortcuts
				switch input {
				case "q", "quit", "exit":
					m.quitting = true
					return m, tea.Quit
				case "c", "chat":
					m.mode = ChatMode
					m.input.Placeholder = fmt.Sprintf("Chat with %s (Ctrl+N: new, Ctrl+S: switch, Ctrl+L: manage, Ctrl+H: history)", m.ollama.GetCurrentModel())
					return m, m.loadChatHistory()
				case "stats", "info":
					// Show current chat statistics
					m.viewport.SetContent(m.showChatStats())
					return m, nil
				case "history", "h":
					// Quick access to chat history
					m.mode = ChatListMode
					return m, m.refreshChats()
				case "new":
					// Quick new chat
					m.mode = NewChatMode
					return m, m.refreshInstalledModels()
				default:
					// Execute as shell command
					return m, m.executeCommand(input)
				}

			case NewChatMode:
				// Select model for new chat
				if len(m.ollama.GetModels()) == 0 {
					// No models installed, redirect to model management
					m.mode = ModelManagementMode
					return m, m.refreshModels()
				}
				if item, ok := m.modelList.SelectedItem().(ModelItem); ok && item.installed && !item.isHeader && !item.isSeparator {
					return m, m.createNewChatWithModel(item.name)
				}

			case ModelManagementMode:
				// Download/select models for management
				if item, ok := m.modelList.SelectedItem().(ModelItem); ok && !item.isHeader && !item.isSeparator {
					if !item.installed {
						m.pullingModel = item.name
						return m, m.pullModel(item.name)
					}
				}

			case ChatListMode:
				// Select and load a chat session with full context
				if item, ok := m.chatList.SelectedItem().(ChatSessionItem); ok {
					m.mode = ChatMode
					m.input.Placeholder = fmt.Sprintf("Chat with %s (Ctrl+N: new, Ctrl+S: switch, Ctrl+L: manage, Ctrl+H: history)", item.session.ModelName)
					m.input.Focus()
					return m, m.switchToChatSession(item.session.ID)
				}

			case ChatMode:
				if m.showingModels {
					// Quick model switch for current chat
					if item, ok := m.modelList.SelectedItem().(ModelItem); ok && item.installed && !item.isHeader && !item.isSeparator {
						// Update model for current session in database
						if m.db.IsConnected() {
							m.db.UpdateChatSessionModel(m.currentSessionID, item.name)
						}

						m.ollama.SetCurrentModel(item.name)
						m.showingModels = false
						m.input.Placeholder = fmt.Sprintf("Chat with %s (N: new, S: switch, L: manage, H: history)", item.name)
						m.input.Focus()
					}
				} else {
					// Chat message
					prompt := m.input.Value()
					if prompt == "" {
						return m, nil
					}
					m.input.Reset()

					// Check if Ollama is running
					if !m.ollama.IsRunning() {
						m.viewport.SetContent("Starting Ollama...")
						return m, m.startOllama()
					}

					m.viewport.SetContent("Thinking...")
					return m, m.performAI(prompt)
				}
			}
		}

	case commandFinishedMsg:
		m.commandOutput = msg.output
		if msg.err != nil {
			m.err = msg.err
			m.commandOutput = fmt.Sprintf("Error: %v", msg.err)
		}
		m.viewport.SetContent(m.commandOutput)

	case aiResponseMsg:
		if msg.err != nil {
			m.err = msg.err
			m.viewport.SetContent(fmt.Sprintf("AI error: %v", msg.err))
		} else {
			// Format the AI response with rich text
			formattedResponse := m.formatChatContent(msg.response)
			m.viewport.SetContent(formattedResponse)
		}
		m.viewport.GotoTop()

	case databaseSetupMsg:
		if msg.err != nil {
			m.commandOutput = fmt.Sprintf("Failed to setup database: %v\nSome features will be unavailable.", msg.err)
			m.viewport.SetContent(m.commandOutput)
		} else {
			m.viewport.SetContent("Database setup successful! Connecting...")
			return m, m.connectDatabase()
		}

	case ollamaInstalledMsg:
		if msg.err != nil {
			m.commandOutput = fmt.Sprintf("Failed to install Ollama: %v", msg.err)
			m.viewport.SetContent(m.commandOutput)
		} else {
			m.viewport.SetContent("Ollama installed successfully! Starting service...")
			return m, m.startOllama()
		}

	case modelPulledMsg:
		m.pullingModel = ""
		m.modelProgress = nil
		m.showingProgress = false
		if msg.err != nil {
			if strings.Contains(msg.err.Error(), "cancelled") {
				m.viewport.SetContent("❌ Download was cancelled.")
			} else {
				m.viewport.SetContent(fmt.Sprintf("❌ Failed to download model '%s': %v", msg.model, msg.err))
			}
		} else {
			m.ollama.SetCurrentModel(msg.model)
			m.viewport.SetContent(fmt.Sprintf("✅ Model '%s' downloaded and ready to use!", msg.model))
			return m, m.refreshModels()
		}

	case modelProgressMsg:
		if msg.progress != nil {
			m.modelProgress = msg.progress
			m.showingProgress = true

			// Continue progress updates only if still pulling
			if m.ollama.IsPulling() && msg.progress.Percent < 100 {
				return m, tea.Tick(time.Millisecond*250, func(t time.Time) tea.Msg {
					if progress := m.ollama.GetPullProgress(); progress != nil {
						return modelProgressMsg{progress: progress}
					}
					return nil
				})
			} else if msg.progress.Percent >= 100 {
				// Download completed, clean up progress
				m.showingProgress = false
				m.modelProgress = nil
			}
		}

	case modelDeletedMsg:
		if msg.err != nil {
			// Show specific error messages
			errorMsg := msg.err.Error()
			if strings.Contains(errorMsg, "not found") {
				m.viewport.SetContent(fmt.Sprintf("Model '%s' is not installed or already deleted.", msg.model))
			} else if strings.Contains(errorMsg, "timeout") {
				m.viewport.SetContent(fmt.Sprintf("Delete operation timed out for model '%s'. Please try again.", msg.model))
			} else {
				m.viewport.SetContent(fmt.Sprintf("Failed to delete model '%s': %v", msg.model, msg.err))
			}
		} else {
			m.viewport.SetContent(fmt.Sprintf("✅ Model '%s' deleted successfully!", msg.model))
			// Refresh model list after successful deletion
			return m, m.refreshModels()
		}

	case pullCancelledMsg:
		m.pullingModel = ""
		m.modelProgress = nil
		m.showingProgress = false
		m.viewport.SetContent("❌ Download cancelled by user.")
		// Refresh models to update the list
		return m, m.refreshModels()

	case newChatMsg:
		if msg.err != nil {
			m.viewport.SetContent(fmt.Sprintf("Failed to create new chat: %v", msg.err))
		} else {
			m.currentSessionID = msg.sessionID
			m.mode = ChatMode

			// Get the session to set the correct model
			if session, err := m.db.GetChatSession(msg.sessionID); err == nil {
				m.ollama.SetCurrentModel(session.ModelName)
			}

			m.input.Placeholder = fmt.Sprintf("Chat with %s (N: new, S: switch, L: manage, H: history)", m.ollama.GetCurrentModel())
			m.input.Focus()

			// Load chat history for this session
			return m, m.loadChatHistory()
		}

	case tickMsg:
		if m.pullingModel != "" {
			m.pullProgress++
			spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			spinnerChar := spinner[m.pullProgress%len(spinner)]
			m.viewport.SetContent(fmt.Sprintf("Pulling %s %s\nThis may take several minutes...", m.pullingModel, spinnerChar))

			// Continue the animation
			return m, tea.Tick(time.Millisecond*150, func(t time.Time) tea.Msg {
				return tickMsg{}
			})
		}

	case modelsRefreshedMsg:
		if msg.err == nil {
			m.updateModelList()
		}

	case chatsRefreshedMsg:
		if msg.err == nil {
			m.updateChatList()
		}
	}

	// Update components based on mode
	switch m.mode {
	case CommandMode:
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	case ChatMode:
		if m.showingModels {
			// Don't update input when showing models
			m.modelList, cmd = m.modelList.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			// Update input and viewport for chat
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	case NewChatMode, ModelManagementMode:
		// Only update model list for these modes
		m.modelList, cmd = m.modelList.Update(msg)
		cmds = append(cmds, cmd)
	case ChatListMode:
		// Update chat list for chat history mode
		m.chatList, cmd = m.chatList.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye! 👋\n"
	}

	var s strings.Builder

	// Title
	title := titleStyle.Render("🤖 Terminal AI & Command Runner")
	s.WriteString(title + "\n")

	// Mode indicator
	modes := []string{"Command", "Chat", "New Chat", "Model Management", "Chat History"}
	modeIndicators := []string{}
	for i, modeName := range modes {
		if i == int(m.mode) {
			if m.mode == ChatMode && m.showingModels {
				modeIndicators = append(modeIndicators, modeStyle.Render("Quick Switch"))
			} else {
				modeIndicators = append(modeIndicators, modeStyle.Render(modeName))
			}
		} else {
			modeIndicators = append(modeIndicators, modeName)
		}
	}
	s.WriteString(strings.Join(modeIndicators, " │ ") + "\n")

	// Status indicators
	statusIndicators := []string{}
	if m.dbSetupProgress != "" {
		statusIndicators = append(statusIndicators, m.dbSetupProgress)
	} else if m.db.IsConnected() {
		statusIndicators = append(statusIndicators, "🗄️ DB Ready")
	} else if m.db.IsPostgresRunning() {
		statusIndicators = append(statusIndicators, "🗄️ DB Starting...")
	} else {
		statusIndicators = append(statusIndicators, "🗄️ DB Available")
	}

	if m.ollama.IsRunning() {
		statusIndicators = append(statusIndicators, "🤖 Ollama Ready")
	} else {
		statusIndicators = append(statusIndicators, "🤖 Ollama Available")
	}

	if len(statusIndicators) > 0 {
		s.WriteString(helpStyle.Render(strings.Join(statusIndicators, " │ ")) + "\n")
	}
	s.WriteString("\n")

	// Main content
	switch m.mode {
	case CommandMode:
		s.WriteString(m.input.View() + "\n")
		s.WriteString(helpStyle.Render("Tab: Chat │ Ctrl+L: Models │ Ctrl+H: History │ Enter: Run command │ 'c': Chat │ 'q': Quit") + "\n")
		if m.commandOutput != "" {
			s.WriteString("\n" + m.viewport.View())
		}

	case NewChatMode:
		s.WriteString(titleStyle.Render("Select Model for New Chat") + "\n")
		s.WriteString(m.modelList.View() + "\n")
		if len(m.ollama.GetModels()) == 0 {
			s.WriteString(helpStyle.Render("No models installed. Press ESC → L to download models") + "\n")
		} else {
			s.WriteString(helpStyle.Render("↑/↓: Navigate │ Enter: Create chat with model │ ESC: Back") + "\n")
		}

	case ModelManagementMode:
		s.WriteString(titleStyle.Render("Model Management") + "\n")
		s.WriteString(m.modelList.View() + "\n")

		// Show progress bar if downloading
		if m.showingProgress && m.modelProgress != nil {
			progress := m.modelProgress
			progressBar := m.renderProgressBar(progress.Percent)
			s.WriteString(fmt.Sprintf("\n📥 %s\n", progress.Status))
			s.WriteString(fmt.Sprintf("%s %d%%", progressBar, progress.Percent))
			if progress.Downloaded != "" && progress.Total != "" {
				s.WriteString(fmt.Sprintf(" (%s / %s)", progress.Downloaded, progress.Total))
			}
			s.WriteString("\n")
			s.WriteString(helpStyle.Render("Ctrl+G: Cancel download │ ESC: Back") + "\n")
		} else {
			// Show management options
			if item, ok := m.modelList.SelectedItem().(ModelItem); ok {
				if item.isHeader || item.isSeparator {
					s.WriteString(helpStyle.Render("↑/↓: Navigate │ ESC: Back") + "\n")
				} else if item.installed {
					s.WriteString(helpStyle.Render("↑/↓: Navigate │ Ctrl+D: Delete model │ ESC: Back") + "\n")
				} else {
					s.WriteString(helpStyle.Render("↑/↓: Navigate │ Enter: Download model │ ESC: Back") + "\n")
				}
			} else {
				s.WriteString(helpStyle.Render("↑/↓: Navigate │ Enter: Download │ Ctrl+D: Delete │ ESC: Back") + "\n")
			}
		}
	case ChatListMode:
		s.WriteString(titleStyle.Render("Chat History") + "\n")
		s.WriteString(m.chatList.View() + "\n")
		if len(m.chatList.Items()) == 0 {
			s.WriteString(helpStyle.Render("No chat sessions found") + "\n")
		}
		s.WriteString(helpStyle.Render("↑/↓: Navigate │ Enter: Load chat │ ESC: Back") + "\n")

	case ChatMode:
		if m.showingModels {
			s.WriteString(titleStyle.Render("Quick Model Switch") + "\n")
			s.WriteString(m.modelList.View() + "\n")
			s.WriteString(helpStyle.Render("↑/↓: Navigate │ Enter: Switch model │ ESC: Back to chat") + "\n")
		} else {
			s.WriteString(modelStyle.Render(fmt.Sprintf("Chat #%d | Model: %s | Messages: %d", m.currentSessionID, m.ollama.GetCurrentModel(), len(m.chatHistory))) + "\n")
			s.WriteString(m.input.View() + "\n")
			s.WriteString(helpStyle.Render("Ctrl+N: New chat │ Ctrl+S: Switch model │ Ctrl+L: Manage models │ Ctrl+H: Chat history │ Tab: Commands") + "\n")
			if m.viewport.TotalLineCount() > 0 {
				s.WriteString("\n" + m.viewport.View())
			}
		}
	}

	if m.err != nil {
		s.WriteString(fmt.Sprintf("\nError: %v\n", m.err))
	}

	return s.String()
}

// Enhanced chat context management methods

func (m *model) loadChatHistory() tea.Cmd {
	return func() tea.Msg {
		if !m.db.IsConnected() {
			return aiResponseMsg{
				response: "Database not connected. Chat history unavailable.",
				err:      nil,
			}
		}

		// Load ALL messages for the session to maintain full context
		messages, err := m.db.GetMessages(m.currentSessionID, 0) // 0 = no limit
		if err != nil {
			return aiResponseMsg{
				response: fmt.Sprintf("Error loading chat history: %v", err),
				err:      nil,
			}
		}

		// Store messages in model for context
		m.chatHistory = messages

		// Format chat history for display (show last 20 for UI)
		displayMessages := messages
		if len(messages) > 20 {
			displayMessages = messages[len(messages)-20:] // Show last 20 messages
		}

		if len(displayMessages) == 0 {
			return aiResponseMsg{
				response: "No previous messages in this chat. Start a new conversation!",
				err:      nil,
			}
		}

		formattedHistory := m.formatChatHistoryFromMessages(displayMessages)

		return aiResponseMsg{
			response: formattedHistory,
			err:      nil,
		}
	}
}

func (m *model) formatChatHistoryFromMessages(messages []Message) string {
	if len(messages) == 0 {
		return "No messages to display."
	}

	var content strings.Builder

	// Add context indicator
	totalMessages := len(m.chatHistory)
	displayCount := len(messages)

	if totalMessages > displayCount {
		content.WriteString(helpStyle.Render(fmt.Sprintf("... showing last %d of %d messages ...", displayCount, totalMessages)))
		content.WriteString("\n\n")
	}

	for i, msg := range messages {
		if i > 0 {
			content.WriteString("\n")
		}

		if msg.Role == "user" {
			content.WriteString(userMessageStyle.Render("You:"))
			content.WriteString("\n")
			formattedContent := m.formatChatContent(msg.Content)
			content.WriteString(messageContentStyle.Render(formattedContent))
		} else {
			content.WriteString(assistantMessageStyle.Render("AI:"))
			content.WriteString("\n")
			formattedContent := m.formatChatContent(msg.Content)
			content.WriteString(messageContentStyle.Render(formattedContent))
		}
		content.WriteString("\n")
	}

	return content.String()
}

func (m *model) performAI(prompt string) tea.Cmd {
	return func() tea.Msg {
		// Save user message to database first
		if m.db.IsConnected() {
			userMsg, err := m.db.SaveMessage(m.currentSessionID, "user", prompt)
			if err == nil {
				// Add to in-memory history
				m.chatHistory = append(m.chatHistory, *userMsg)
			}
		}

		// Build conversation context for the AI
		conversationContext := m.buildConversationContext(prompt)

		// Send full context to AI
		response, err := m.ollama.ChatWithContext(conversationContext)

		// Save assistant response to database
		if m.db.IsConnected() && err == nil {
			assistantMsg, saveErr := m.db.SaveMessage(m.currentSessionID, "assistant", response)
			if saveErr == nil {
				// Add to in-memory history
				m.chatHistory = append(m.chatHistory, *assistantMsg)
			}
		}

		return aiResponseMsg{
			response: response,
			err:      err,
		}
	}
}

func (m *model) buildConversationContext(currentPrompt string) string {
	if len(m.chatHistory) == 0 {
		return currentPrompt
	}

	var context strings.Builder

	// Include recent conversation history (last 10 exchanges to avoid token limits)
	recentHistory := m.chatHistory
	if len(m.chatHistory) > 20 { // 20 messages = 10 exchanges
		recentHistory = m.chatHistory[len(m.chatHistory)-20:]
	}

	context.WriteString("Previous conversation:\n")
	for _, msg := range recentHistory {
		if msg.Role == "user" {
			context.WriteString(fmt.Sprintf("User: %s\n", msg.Content))
		} else {
			context.WriteString(fmt.Sprintf("Assistant: %s\n", msg.Content))
		}
	}

	context.WriteString(fmt.Sprintf("\nUser: %s", currentPrompt))

	return context.String()
}

func (m *model) switchToChatSession(sessionID int) tea.Cmd {
	return func() tea.Msg {
		if !m.db.IsConnected() {
			return aiResponseMsg{
				response: "Database not connected",
				err:      fmt.Errorf("database not connected"),
			}
		}

		// Get session details
		session, err := m.db.GetChatSession(sessionID)
		if err != nil {
			return aiResponseMsg{
				response: fmt.Sprintf("Error loading chat session: %v", err),
				err:      err,
			}
		}

		// Update current session
		m.currentSessionID = sessionID
		m.ollama.SetCurrentModel(session.ModelName)

		// Load full chat history for context
		messages, err := m.db.GetMessages(sessionID, 0) // Load ALL messages
		if err != nil {
			return aiResponseMsg{
				response: fmt.Sprintf("Error loading chat history: %v", err),
				err:      err,
			}
		}

		// Store in memory for context
		m.chatHistory = messages

		// Format for display (last 15 messages)
		displayMessages := messages
		if len(messages) > 15 {
			displayMessages = messages[len(messages)-15:]
		}

		var response strings.Builder
		response.WriteString(fmt.Sprintf("📝 Switched to: %s\n", session.Name))
		response.WriteString(fmt.Sprintf("🤖 Model: %s\n", session.ModelName))
		response.WriteString(fmt.Sprintf("📊 Total messages: %d\n\n", len(messages)))

		if len(displayMessages) > 0 {
			response.WriteString("Recent conversation:\n")
			response.WriteString("═══════════════════\n\n")
			response.WriteString(m.formatChatHistoryFromMessages(displayMessages))
		} else {
			response.WriteString("No previous messages. Start a new conversation!")
		}

		return aiResponseMsg{
			response: response.String(),
			err:      nil,
		}
	}
}

func (m *model) createNewChatWithModel(modelName string) tea.Cmd {
	return func() tea.Msg {
		if !m.db.IsConnected() {
			return newChatMsg{sessionID: 0, err: fmt.Errorf("database not connected")}
		}

		// Create more descriptive session name with timestamp
		timestamp := time.Now().Format("Jan 2, 15:04")
		sessionName := fmt.Sprintf("Chat with %s - %s", modelName, timestamp)

		session, err := m.db.CreateChatSession(sessionName, modelName)
		if err != nil {
			return newChatMsg{sessionID: 0, err: err}
		}

		return newChatMsg{sessionID: session.ID, err: nil}
	}
}

func (m *model) showChatStats() string {
	if !m.db.IsConnected() {
		return "Database not connected"
	}

	// Get current session info
	session, err := m.db.GetChatSession(m.currentSessionID)
	if err != nil {
		return fmt.Sprintf("Error getting session info: %v", err)
	}

	messageCount := len(m.chatHistory)
	userMessages := 0
	assistantMessages := 0

	for _, msg := range m.chatHistory {
		if msg.Role == "user" {
			userMessages++
		} else {
			assistantMessages++
		}
	}

	var stats strings.Builder
	stats.WriteString(fmt.Sprintf("📊 Chat Statistics\n"))
	stats.WriteString(fmt.Sprintf("Session: %s\n", session.Name))
	stats.WriteString(fmt.Sprintf("Model: %s\n", session.ModelName))
	stats.WriteString(fmt.Sprintf("Created: %s\n", session.CreatedAt.Format("Jan 2, 2006 at 15:04")))
	stats.WriteString(fmt.Sprintf("Last updated: %s\n", session.UpdatedAt.Format("Jan 2, 2006 at 15:04")))
	stats.WriteString(fmt.Sprintf("Total messages: %d\n", messageCount))
	stats.WriteString(fmt.Sprintf("Your messages: %d\n", userMessages))
	stats.WriteString(fmt.Sprintf("AI responses: %d\n", assistantMessages))

	return stats.String()
}

// Original methods (keeping existing functionality)

func (m *model) checkOllama() tea.Cmd {
	return func() tea.Msg {
		if !m.ollama.IsInstalled() {
			m.commandOutput = "Ollama is not installed. Would you like to install Ollama now? (y/n)"
			m.viewport.SetContent(m.commandOutput)
		}
		return nil
	}
}

func (m *model) installOllama() tea.Cmd {
	return func() tea.Msg {
		err := m.ollama.InstallOllama()
		return ollamaInstalledMsg{err: err}
	}
}

func (m *model) startOllama() tea.Cmd {
	return func() tea.Msg {
		if err := m.ollama.StartService(); err != nil {
			return aiResponseMsg{
				response: "",
				err:      err,
			}
		}

		// Pull default model if no models exist
		m.ollama.RefreshModels()
		if len(m.ollama.GetModels()) == 0 {
			m.ollama.PullModel("llama2")
		}

		return aiResponseMsg{
			response: "Ollama started! You can now chat.",
			err:      nil,
		}
	}
}

func (m *model) refreshModels() tea.Cmd {
	return func() tea.Msg {
		// Fetch both installed and available models
		err1 := m.ollama.RefreshModels()
		err2 := m.ollama.FetchAvailableModels()

		var err error
		if err1 != nil {
			err = err1
		} else if err2 != nil {
			err = err2
		}

		return modelsRefreshedMsg{err: err}
	}
}

func (m *model) updateModelList() {
	if m.mode == NewChatMode || (m.mode == ChatMode && m.showingModels) {
		m.updateInstalledModelList()
	} else {
		m.updateFullModelList()
	}
}

func (m *model) updateInstalledModelList() {
	installedModels := m.ollama.GetModels()
	items := []list.Item{}

	if len(installedModels) == 0 {
		// Show message when no models are installed
		items = append(items, ModelItem{
			name:        "No models installed",
			description: "Go to Model Management (L) to download models",
			size:        "",
			installed:   false,
			isHeader:    true,
		})
	} else {
		// Add header
		items = append(items, ModelItem{
			name:     fmt.Sprintf("INSTALLED MODELS (%d)", len(installedModels)),
			isHeader: true,
		})

		// Add installed models
		for _, model := range installedModels {
			items = append(items, ModelItem{
				name:        model.Name,
				description: "Ready to use",
				size:        model.Size,
				installed:   true,
			})
		}
	}

	m.modelList.SetItems(items)
}

func (m *model) updateFullModelList() {
	installedModels := m.ollama.GetModels()
	installedMap := make(map[string]bool)

	// Create a map of installed models
	for _, model := range installedModels {
		// Store the exact name
		installedMap[model.Name] = true
	}

	items := []list.Item{}
	installedItems := []ModelItem{}
	availableItems := []ModelItem{}

	// Separate installed and available models
	for _, modelInfo := range AllModels {
		installed := false

		// For models without tags (like "llama2"), check if it exists as-is or with :latest
		if !strings.Contains(modelInfo.Name, ":") {
			installed = installedMap[modelInfo.Name] || installedMap[modelInfo.Name+":latest"]
		} else {
			// For models with specific tags (like "llama2:13b"), only check exact match
			installed = installedMap[modelInfo.Name]
		}

		// Create description with tags if available
		description := modelInfo.Description
		if len(modelInfo.Tags) > 0 {
			description += " [" + strings.Join(modelInfo.Tags, ", ") + "]"
		}

		modelItem := ModelItem{
			name:        modelInfo.Name,
			description: description,
			size:        modelInfo.Size,
			installed:   installed,
		}

		if installed {
			installedItems = append(installedItems, modelItem)
		} else {
			availableItems = append(availableItems, modelItem)
		}
	}

	// Build final list with headers and separators
	if len(installedItems) > 0 {
		// Add installed models header
		items = append(items, ModelItem{
			name:     fmt.Sprintf("INSTALLED MODELS (%d)", len(installedItems)),
			isHeader: true,
		})

		// Add installed models
		for _, item := range installedItems {
			items = append(items, item)
		}

		// Add separator
		items = append(items, ModelItem{
			isSeparator: true,
		})
	}

	// Add available models header
	items = append(items, ModelItem{
		name:     fmt.Sprintf("AVAILABLE MODELS (%d)", len(availableItems)),
		isHeader: true,
	})

	// Add available models
	for _, item := range availableItems {
		items = append(items, item)
	}

	// Convert to list.Item interface
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	m.modelList.SetItems(listItems)
}

func (m *model) executeCommand(command string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("bash", "-c", command)
		output, err := cmd.CombinedOutput()
		return commandFinishedMsg{
			output: string(output),
			err:    err,
		}
	}
}

func (m *model) refreshChats() tea.Cmd {
	return func() tea.Msg {
		if !m.db.IsConnected() {
			return chatsRefreshedMsg{err: fmt.Errorf("database not connected")}
		}

		_, err := m.db.GetChatSessions()
		return chatsRefreshedMsg{err: err}
	}
}

func (m *model) updateChatList() {
	if !m.db.IsConnected() {
		return
	}

	sessions, err := m.db.GetChatSessions()
	if err != nil {
		return
	}

	items := []list.Item{}
	for _, session := range sessions {
		items = append(items, ChatSessionItem{session: session})
	}

	m.chatList.SetItems(items)
}

func (m *model) formatChatHistory() string {
	if !m.db.IsConnected() {
		return ""
	}

	messages, err := m.db.GetRecentMessages(m.currentSessionID, 10)
	if err != nil {
		return ""
	}

	var content strings.Builder

	for i, msg := range messages {
		if i > 0 {
			content.WriteString("\n")
		}

		if msg.Role == "user" {
			content.WriteString(userMessageStyle.Render("You:"))
			content.WriteString("\n")
			formattedContent := m.formatChatContent(msg.Content)
			content.WriteString(messageContentStyle.Render(formattedContent))
		} else {
			content.WriteString(assistantMessageStyle.Render("AI:"))
			content.WriteString("\n")
			formattedContent := m.formatChatContent(msg.Content)
			content.WriteString(messageContentStyle.Render(formattedContent))
		}
		content.WriteString("\n")
	}

	return content.String()
}

func (m *model) checkDatabase() tea.Cmd {
	return func() tea.Msg {
		// Check if database is already running
		if m.db.IsPostgresRunning() {
			// Try to connect
			if err := m.db.Connect(); err == nil {
				return nil // Database is ready
			}
		}

		// Check if docker-compose.yml exists
		if _, err := os.Stat("docker-compose.yml"); err != nil {
			m.commandOutput = "Database configuration not found. Some features will be unavailable."
			m.viewport.SetContent(m.commandOutput)
			return nil
		}

		// Check if Docker is available
		if !m.db.IsDockerInstalled() {
			m.commandOutput = "Docker is required for database features but not installed. Some features will be unavailable."
			m.viewport.SetContent(m.commandOutput)
			return nil
		}

		// Automatically setup database
		m.viewport.SetContent("Setting up PostgreSQL database for chat history...")
		return m.setupDatabase()()
	}
}

func (m *model) setupDatabase() tea.Cmd {
	return func() tea.Msg {
		// Update progress in steps
		m.dbSetupProgress = "🗄️ Setting up PostgreSQL..."
		err := m.db.SetupPostgres()
		m.dbSetupProgress = ""
		return databaseSetupMsg{err: err}
	}
}

func (m *model) connectDatabase() tea.Cmd {
	return func() tea.Msg {
		if err := m.db.Connect(); err != nil {
			return databaseSetupMsg{err: err}
		}

		// Load chat history after successful connection
		return m.loadChatHistory()()
	}
}

func (m *model) renderProgressBar(percent int) string {
	width := 40
	filled := int(float64(width) * float64(percent) / 100.0)

	bar := strings.Builder{}
	bar.WriteString("│")

	for i := 0; i < width; i++ {
		if i < filled {
			bar.WriteString("█")
		} else {
			bar.WriteString("░")
		}
	}

	bar.WriteString("│")
	return bar.String()
}

func (m *model) formatChatContent(content string) string {
	if content == "" {
		return ""
	}

	var result strings.Builder
	lines := strings.Split(content, "\n")

	inCodeBlock := false
	var codeBlockContent strings.Builder

	for i, line := range lines {
		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End code block
				result.WriteString(codeBlockStyle.Render(codeBlockContent.String()))
				result.WriteString("\n")
				codeBlockContent.Reset()
				inCodeBlock = false
			} else {
				// Start code block
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			codeBlockContent.WriteString(line)
			if i < len(lines)-1 {
				codeBlockContent.WriteString("\n")
			}
		} else {
			// Handle inline code
			line = m.formatInlineCode(line)

			// Wrap long lines
			wrappedLine := m.wrapText(line, m.viewport.Width-4)
			result.WriteString(wrappedLine)
			if i < len(lines)-1 {
				result.WriteString("\n")
			}
		}
	}

	// Handle unclosed code block
	if inCodeBlock {
		result.WriteString(codeBlockStyle.Render(codeBlockContent.String()))
	}

	return result.String()
}

func (m *model) formatInlineCode(text string) string {
	// Simple inline code detection with backticks
	re := regexp.MustCompile("`([^`]+)`")
	return re.ReplaceAllStringFunc(text, func(match string) string {
		code := strings.Trim(match, "`")
		return inlineCodeStyle.Render(code)
	})
}

func (m *model) wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var result strings.Builder
	var currentLine strings.Builder
	currentLength := 0

	for _, word := range words {
		wordLength := utf8.RuneCountInString(word)

		// If adding this word would exceed the width, start a new line
		if currentLength > 0 && currentLength+1+wordLength > width {
			result.WriteString(currentLine.String())
			result.WriteString("\n")
			currentLine.Reset()
			currentLength = 0
		}

		if currentLength > 0 {
			currentLine.WriteString(" ")
			currentLength++
		}

		currentLine.WriteString(word)
		currentLength += wordLength
	}

	if currentLine.Len() > 0 {
		result.WriteString(currentLine.String())
	}

	return result.String()
}

func (m *model) pullModel(modelName string) tea.Cmd {
	return tea.Batch(
		// Start progress tracking immediately
		func() tea.Msg {
			return modelProgressMsg{progress: &PullProgress{
				Model:   modelName,
				Status:  "Initializing download...",
				Percent: 0,
			}}
		},
		// Start the actual download in background
		func() tea.Msg {
			// Set up progress tracking
			m.ollama.pullProgress = &PullProgress{
				Model:   modelName,
				Status:  "Starting download...",
				Percent: 0,
			}

			// Start progress updates
			go func() {
				ticker := time.NewTicker(500 * time.Millisecond)
				defer ticker.Stop()

				for {
					select {
					case <-ticker.C:
						if progress := m.ollama.GetPullProgress(); progress != nil {
							// Send progress update
							if m.ollama.IsPulling() {
								// Continue sending progress updates
								continue
							}
						}
						return
					}
				}
			}()

			// Do the actual pull
			err := m.ollama.PullModel(modelName)
			return modelPulledMsg{
				model: modelName,
				err:   err,
			}
		},
		// Start periodic progress updates
		tea.Tick(time.Millisecond*250, func(t time.Time) tea.Msg {
			if progress := m.ollama.GetPullProgress(); progress != nil && m.ollama.IsPulling() {
				return modelProgressMsg{progress: progress}
			}
			return nil
		}),
	)
}

func (m *model) cancelPull() tea.Cmd {
	return func() tea.Msg {
		err := m.ollama.CancelPull()
		if err != nil {
			// Still show cancellation message even if there's an error
			return pullCancelledMsg{}
		}
		return pullCancelledMsg{}
	}
}
func (m *model) deleteModel(modelName string) tea.Cmd {
	return func() tea.Msg {
		err := m.ollama.DeleteModel(modelName)
		return modelDeletedMsg{
			model: modelName,
			err:   err,
		}
	}
}
func (m *model) refreshInstalledModels() tea.Cmd {
	return func() tea.Msg {
		// Refresh both installed and available models
		err1 := m.ollama.RefreshModels()
		err2 := m.ollama.FetchAvailableModels()

		var err error
		if err1 != nil {
			err = err1
		} else if err2 != nil {
			err = err2
		}

		return modelsRefreshedMsg{err: err}
	}
}

func main() {
	flag.Parse()

	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		log.Fatal(err)
	}
}

