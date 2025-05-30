package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
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
	copyButtons   []CopyButtonArea
	lastCopied    int // Index of last copied message for feedback
}

// CopyButtonArea defines a clickable copy button area
type CopyButtonArea struct {
	MessageIndex int
	X            int
	Y            int
	Width        int
	Height       int
}

// GetMessages returns the messages (for external access)
func (c *ChatView) GetMessages() []ChatMessage {
	return c.messages
}

// ClearMessages clears all messages from the chat view
func (c *ChatView) ClearMessages() {
	c.messages = []ChatMessage{}
	c.content = ""
	c.updateContent()
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
		viewport:   vp,
		messages:   []ChatMessage{},
		width:      width,
		height:     height,
		lastCopied: -1, // Initialize to invalid index
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
		// Leave room for: header(1) + separator(1) + spacing(1) + input(1) + spacing(1) + status(1) = 6 lines
		c.viewport.Height = msg.Height - 6
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
		// Handle mouse events for selection and copy buttons
		switch msg.Type {
		case tea.MouseWheelUp:
			c.viewport.LineUp(3)
		case tea.MouseWheelDown:
			c.viewport.LineDown(3)
		case tea.MouseLeft:
			// Check if click is on a copy button
			clickY := msg.Y + c.viewport.YOffset
			clickX := msg.X
			
			for _, btn := range c.copyButtons {
				if clickX >= btn.X && clickX < btn.X + btn.Width &&
				   clickY >= btn.Y && clickY < btn.Y + btn.Height {
					// Copy button clicked!
					c.copyMessage(btn.MessageIndex)
					return c, c.ResetCopyFeedback()
				}
			}
			
			// Not a copy button, start text selection
			c.selecting = true
			c.selectStart = clickY
		case tea.MouseRelease:
			if c.selecting {
				c.selectEnd = msg.Y + c.viewport.YOffset
			}
		}
	
	case CopyFeedbackResetMsg:
		// Reset copy feedback
		c.lastCopied = -1
		c.updateContent()
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

	// Just return the main viewport content - no extra status bars
	return c.viewport.View()
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
	
	// Clear copy button areas
	c.copyButtons = []CopyButtonArea{}
	
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
			// Header with Assistant label and copy button
			headerLeft := AssistantLabelStyle.Render("Assistant")
			
			// Show different button style if recently copied
			var copyBtn string
			if c.lastCopied == i {
				copyBtn = CopyButtonCopiedStyle.Render("✓ Copied")
			} else {
				copyBtn = CopyButtonStyle.Render("📋 Copy")
			}
			
			// Calculate positions for copy button tracking
			currentLine := strings.Count(content.String(), "\n")
			btnX := contentWidth - 10 // Position from right
			btnY := currentLine + 1 // Below timestamp
			
			// Track copy button area for click detection
			c.copyButtons = append(c.copyButtons, CopyButtonArea{
				MessageIndex: i,
				X:            btnX,
				Y:            btnY,
				Width:        10,
				Height:       1,
			})
			
			// Create header with copy button aligned to right
			headerPadding := contentWidth - len("Assistant") - 10
			if headerPadding < 1 {
				headerPadding = 1
			}
			header := lipgloss.JoinHorizontal(lipgloss.Left,
				headerLeft,
				strings.Repeat(" ", headerPadding),
				copyBtn,
			)
			content.WriteString(header)
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
	
	// Copy to system clipboard
	text := selectedText.String()
	if err := clipboard.WriteAll(text); err == nil {
		c.clipboardCopy = text // Also store locally for feedback
	}
	
	// Show feedback
	c.selecting = false
}

// CopyLastMessage copies the last assistant message to clipboard
func (c *ChatView) CopyLastMessage() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "assistant" {
			text := c.messages[i].Content
			if err := clipboard.WriteAll(text); err == nil {
				c.clipboardCopy = text // Also store locally for feedback
			}
			return
		}
	}
}

// copyMessage copies a specific message by index
func (c *ChatView) copyMessage(messageIndex int) {
	if messageIndex >= 0 && messageIndex < len(c.messages) {
		text := c.messages[messageIndex].Content
		if err := clipboard.WriteAll(text); err == nil {
			c.clipboardCopy = text
			c.lastCopied = messageIndex
			// Reset the copied state after update to redraw
			c.updateContent()
		}
	}
}

// ResetCopyFeedback resets the copy feedback after a delay
func (c *ChatView) ResetCopyFeedback() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return CopyFeedbackResetMsg{}
	})
}

// CopyFeedbackResetMsg is sent to reset copy feedback
type CopyFeedbackResetMsg struct{}

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
	
	CopyButtonStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Background(lipgloss.Color("237")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Bold(true)
	
	CopyButtonCopiedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")).
		Background(lipgloss.Color("22")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("10")).
		Padding(0, 1).
		Bold(true)
)