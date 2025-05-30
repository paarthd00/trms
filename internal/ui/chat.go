package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// ChatView represents the chat interface component
type ChatView struct {
	viewport      viewport.Model
	messages      []ChatMessage
	content       string
	width         int
	height        int
	ready         bool
	selecting     bool
	selectStart   int
	selectEnd     int
	clipboardCopy string
}

// GetMessages returns the messages (for external access)
func (c *ChatView) GetMessages() []ChatMessage {
	return c.messages
}

// ChatMessage represents a single message in the chat
type ChatMessage struct {
	Role      string
	Content   string
	Timestamp time.Time
}

// KeyMap defines key bindings for the chat view
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	Copy     key.Binding
	Select   key.Binding
	Search   key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "scroll up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "scroll down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+u"),
		key.WithHelp("PgUp/Ctrl+u", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+d"),
		key.WithHelp("PgDn/Ctrl+d", "page down"),
	),
	Home: key.NewBinding(
		key.WithKeys("home", "g"),
		key.WithHelp("Home/g", "go to top"),
	),
	End: key.NewBinding(
		key.WithKeys("end", "G"),
		key.WithHelp("End/G", "go to bottom"),
	),
	Copy: key.NewBinding(
		key.WithKeys("ctrl+y", "y"),
		key.WithHelp("Ctrl+Y/y", "copy selection"),
	),
	Select: key.NewBinding(
		key.WithKeys("v"),
		key.WithHelp("v", "visual select mode"),
	),
	Search: key.NewBinding(
		key.WithKeys("/", "ctrl+f"),
		key.WithHelp("//Ctrl+F", "search"),
	),
}

// New creates a new ChatView
func NewChatView(width, height int) ChatView {
	vp := viewport.New(width, height)
	vp.SetContent("")
	vp.MouseWheelEnabled = true

	return ChatView{
		viewport: vp,
		messages: []ChatMessage{},
		width:    width,
		height:   height,
	}
}

// Init initializes the chat view
func (c ChatView) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the component
func (c ChatView) Update(msg tea.Msg) (ChatView, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.viewport.Width = msg.Width
		c.viewport.Height = msg.Height - 4 // Leave room for status
		if !c.ready {
			c.ready = true
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Up):
			c.viewport.LineUp(1)
		case key.Matches(msg, keys.Down):
			c.viewport.LineDown(1)
		case key.Matches(msg, keys.PageUp):
			c.viewport.ViewUp()
		case key.Matches(msg, keys.PageDown):
			c.viewport.ViewDown()
		case key.Matches(msg, keys.Home):
			c.viewport.GotoTop()
		case key.Matches(msg, keys.End):
			c.viewport.GotoBottom()
		case key.Matches(msg, keys.Select):
			c.selecting = !c.selecting
			if c.selecting {
				c.selectStart = c.viewport.YOffset
			}
		case key.Matches(msg, keys.Copy):
			if c.selecting {
				// Copy selected text to clipboard
				c.copySelection()
				c.selecting = false
			} else {
				// If not selecting, copy last assistant message
				c.CopyLastMessage()
			}
		}

	case tea.MouseMsg:
		// Handle mouse events for selection
		switch msg.Type {
		case tea.MouseWheelUp:
			c.viewport.LineUp(3)
		case tea.MouseWheelDown:
			c.viewport.LineDown(3)
		case tea.MouseLeft:
			// Start selection
			c.selecting = true
			c.selectStart = msg.Y + c.viewport.YOffset
		case tea.MouseRelease:
			if c.selecting {
				c.selectEnd = msg.Y + c.viewport.YOffset
			}
		}
	}

	// Update viewport
	c.viewport, cmd = c.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

// View renders the chat view
func (c ChatView) View() string {
	if !c.ready {
		return "\n  Initializing chat..."
	}

	// Main viewport
	content := c.viewport.View()

	// Status bar
	statusBar := c.renderStatusBar()

	// Help text
	help := c.renderHelp()

	return lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		statusBar,
		help,
	)
}

// AddMessage adds a new message to the chat
func (c *ChatView) AddMessage(role, content string) {
	msg := ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
	c.messages = append(c.messages, msg)
	c.updateContent()
	c.viewport.GotoBottom()
}

// SetMessages sets all messages at once
func (c *ChatView) SetMessages(messages []ChatMessage) {
	c.messages = messages
	c.updateContent()
}

// updateContent updates the viewport content from messages
func (c *ChatView) updateContent() {
	var content strings.Builder
	
	// Calculate proper width for content
	contentWidth := c.width - 8
	if contentWidth < 40 {
		contentWidth = 40
	}
	if contentWidth > 100 {
		contentWidth = 100
	}
	
	for i, msg := range c.messages {
		if i > 0 {
			content.WriteString("\n\n")
		}
		
		// Timestamp
		timestamp := msg.Timestamp.Format("15:04")
		content.WriteString(TimestampStyle.Render(timestamp))
		content.WriteString("\n")
		
		// Role and content
		switch msg.Role {
		case "user":
			content.WriteString(UserLabelStyle.Render("You"))
			content.WriteString("\n")
			// Wrap user input
			wrapped := wrapText(msg.Content, contentWidth-4)
			content.WriteString(UserBubbleStyle.MaxWidth(contentWidth).Render(wrapped))
			
		case "assistant":
			content.WriteString(AssistantLabelStyle.Render("Assistant"))
			content.WriteString("\n")
			// Format and wrap AI response
			formatted := FormatAIResponse(msg.Content, contentWidth-4)
			content.WriteString(AssistantBubbleStyle.MaxWidth(contentWidth).Render(formatted))
			
		case "system":
			content.WriteString(SystemLabelStyle.Render("System"))
			content.WriteString("\n")
			wrapped := wrapText(msg.Content, contentWidth-4)
			content.WriteString(SystemBubbleStyle.MaxWidth(contentWidth).Render(wrapped))
		}
	}
	
	c.content = content.String()
	c.viewport.SetContent(c.content)
}

// wrapText wraps plain text to fit within the specified width
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	lines := strings.Split(text, "\n")
	
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		
		// Handle empty lines
		if line == "" {
			continue
		}
		
		// Wrap long lines
		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}
		
		currentLine := ""
		currentLength := 0
		
		for _, word := range words {
			wordLength := len(word)
			
			if currentLength > 0 && currentLength+1+wordLength > width {
				result.WriteString(currentLine)
				result.WriteString("\n")
				currentLine = word
				currentLength = wordLength
			} else {
				if currentLength > 0 {
					currentLine += " "
					currentLength++
				}
				currentLine += word
				currentLength += wordLength
			}
		}
		
		if currentLine != "" {
			result.WriteString(currentLine)
		}
	}
	
	return result.String()
}

// renderStatusBar renders the status bar
func (c ChatView) renderStatusBar() string {
	// Scroll percentage
	scrollPercent := 0
	if c.viewport.TotalLineCount() > 0 {
		scrollPercent = int(float64(c.viewport.YOffset) / float64(c.viewport.TotalLineCount()) * 100)
	}
	
	status := fmt.Sprintf(" %d messages | %d%% ", len(c.messages), scrollPercent)
	
	if c.selecting {
		status += "| VISUAL MODE "
	}
	
	if c.clipboardCopy != "" {
		status += "| Copied! "
	}
	
	width := c.width
	if width < 40 {
		width = 40
	}
	
	return StatusBarStyle.Width(width).Render(status)
}

// renderHelp renders the help text
func (c ChatView) renderHelp() string {
	helpItems := []string{
		"↑/↓ Scroll",
		"PgUp/PgDn Page",
		"g/G Top/Bottom",
		"v Select",
		"Ctrl+Y Copy",
		"/ Search",
	}
	
	return HelpStyle.Render(strings.Join(helpItems, " • "))
}

// copySelection copies the selected text
func (c *ChatView) copySelection() {
	if !c.selecting || c.selectStart == c.selectEnd {
		return
	}
	
	// Get the content between selection points
	lines := strings.Split(c.content, "\n")
	
	startLine := c.selectStart
	endLine := c.selectEnd
	if startLine > endLine {
		startLine, endLine = endLine, startLine
	}
	
	// Ensure bounds
	if startLine < 0 {
		startLine = 0
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}
	
	// Extract selected lines
	var selectedText strings.Builder
	for i := startLine; i <= endLine && i < len(lines); i++ {
		if i > startLine {
			selectedText.WriteString("\n")
		}
		// Strip ANSI codes from the copied text
		clean := stripANSI(lines[i])
		selectedText.WriteString(clean)
	}
	
	// Copy to clipboard (using a simple implementation)
	// In production, you'd use github.com/atotto/clipboard
	c.clipboardCopy = selectedText.String()
	
	// Show feedback
	c.selecting = false
}

// CopyLastMessage copies the last assistant message to clipboard
func (c *ChatView) CopyLastMessage() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "assistant" {
			c.clipboardCopy = c.messages[i].Content
			return
		}
	}
}


// SearchMessages searches through messages
func (c *ChatView) SearchMessages(query string) []int {
	var matches []int
	
	searchContent := make([]string, len(c.messages))
	for i, msg := range c.messages {
		searchContent[i] = msg.Content
	}
	
	results := fuzzy.Find(query, searchContent)
	for _, r := range results {
		matches = append(matches, r.Index)
	}
	
	return matches
}

// Styles for the chat view
var (
	TimestampStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true)
		
	UserLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("99")).
		Bold(true).
		MarginLeft(2)
		
	AssistantLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)
		
	SystemLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true)
		
	UserBubbleStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("99")).
		Foreground(lipgloss.Color("230")).
		Padding(1, 2).
		MarginLeft(4).
		MarginRight(2)
		
	AssistantBubbleStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("252")).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(1, 2).
		MarginRight(4)
		
	SystemBubbleStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("52")).
		Foreground(lipgloss.Color("214")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("214")).
		Padding(1, 2).
		MarginLeft(2).
		MarginRight(2)
		
	StatusBarStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("245")).
		Padding(0, 1)
		
	HelpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
)