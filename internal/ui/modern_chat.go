package ui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModernChatView represents a Claude-like modern chat interface
type ModernChatView struct {
	viewport      viewport.Model
	messages      []ModernMessage
	content       string
	width         int
	height        int
	ready         bool
	copyButtons   []CopyButtonArea
	lastCopied    int
	copyHistory   []CopyHistoryItem
	thinking      bool
}

// ModernMessage represents a message with enhanced capabilities
type ModernMessage struct {
	Role        string
	Content     string
	Timestamp   time.Time
	ToolCalls   []ToolCall
	Attachments []Attachment
	IsStreaming bool
}

// ToolCall represents a function/tool call
type ToolCall struct {
	ID       string
	Name     string
	Args     map[string]interface{}
	Result   string
	Status   string // "calling", "success", "error"
	Error    string
}

// Attachment represents file attachments
type Attachment struct {
	Type     string // "file", "image", "code"
	Name     string
	Path     string
	Size     int64
	MimeType string
}

// NewModernChatView creates a new modern chat view
func NewModernChatView(width, height int) ModernChatView {
	vp := viewport.New(width, height-6) // Leave space for input and status
	vp.SetContent("")
	vp.MouseWheelEnabled = true

	return ModernChatView{
		viewport:    vp,
		messages:    []ModernMessage{},
		width:       width,
		height:      height,
		lastCopied:  -1,
		copyHistory: []CopyHistoryItem{},
	}
}

// Init initializes the modern chat view
func (c ModernChatView) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the component
func (c ModernChatView) Update(msg tea.Msg) (ModernChatView, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.width = msg.Width
		c.height = msg.Height
		c.viewport.Width = msg.Width
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
		case key.Matches(msg, keys.Copy):
			c.CopyLastMessage()
		}

	case tea.MouseMsg:
		switch msg.Type {
		case tea.MouseWheelUp:
			c.viewport.LineUp(3)
		case tea.MouseWheelDown:
			c.viewport.LineDown(3)
		case tea.MouseLeft:
			// Handle copy button clicks
			viewportY := msg.Y - 3
			clickY := viewportY + c.viewport.YOffset
			clickX := msg.X
			
			for _, btn := range c.copyButtons {
				if clickX >= btn.X && clickX < btn.X + btn.Width &&
				   clickY >= btn.Y && clickY < btn.Y + btn.Height {
					c.copyMessage(btn.MessageIndex)
					return c, c.ResetCopyFeedback()
				}
			}
		}
	
	case CopyFeedbackResetMsg:
		c.lastCopied = -1
		c.updateContent()
	}

	// Update viewport
	c.viewport, cmd = c.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

// View renders the modern chat view
func (c ModernChatView) View() string {
	if !c.ready {
		return "\n  Initializing chat..."
	}

	return c.viewport.View()
}

// AddMessage adds a new message to the chat
func (c *ModernChatView) AddMessage(role, content string) {
	msg := ModernMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
	c.messages = append(c.messages, msg)
	c.updateContent()
	c.viewport.GotoBottom()
}

// AddMessageWithTools adds a message with tool calls
func (c *ModernChatView) AddMessageWithTools(role, content string, toolCalls []ToolCall) {
	msg := ModernMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
		ToolCalls: toolCalls,
	}
	c.messages = append(c.messages, msg)
	c.updateContent()
	c.viewport.GotoBottom()
}

// SetThinking sets the thinking state
func (c *ModernChatView) SetThinking(thinking bool) {
	c.thinking = thinking
	c.updateContent()
}

// updateContent updates the viewport content with modern styling
func (c *ModernChatView) updateContent() {
	var content strings.Builder
	c.copyButtons = []CopyButtonArea{}
	
	// Calculate content width (centered, max 800px like Claude)
	maxWidth := 800
	if c.width < maxWidth+40 {
		maxWidth = c.width - 40
	}
	if maxWidth < 400 {
		maxWidth = 400
	}
	
	// Add some top padding
	content.WriteString("\n")
	
	for i, msg := range c.messages {
		if i > 0 {
			content.WriteString("\n\n")
		}
		
		c.renderMessage(&content, msg, i, maxWidth)
	}
	
	// Add thinking indicator if needed
	if c.thinking {
		content.WriteString("\n\n")
		c.renderThinkingIndicator(&content, maxWidth)
	}
	
	// Add bottom padding
	content.WriteString("\n\n")
	
	c.content = content.String()
	c.viewport.SetContent(c.content)
}

// renderMessage renders a single message with modern styling
func (c *ModernChatView) renderMessage(content *strings.Builder, msg ModernMessage, index int, maxWidth int) {
	switch msg.Role {
	case "user":
		c.renderUserMessage(content, msg, maxWidth)
	case "assistant":
		c.renderAssistantMessage(content, msg, index, maxWidth)
	case "system":
		c.renderSystemMessage(content, msg, maxWidth)
	}
}

// renderUserMessage renders a user message with modern styling
func (c *ModernChatView) renderUserMessage(content *strings.Builder, msg ModernMessage, maxWidth int) {
	// Center the content
	leftPadding := (c.width - maxWidth) / 2
	if leftPadding < 0 {
		leftPadding = 2
	}
	
	padding := strings.Repeat(" ", leftPadding)
	
	// User avatar and content
	userStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#2563eb")).
		Foreground(lipgloss.Color("#ffffff")).
		Padding(1, 2).
		MarginBottom(1).
		MaxWidth(maxWidth - 8)
	
	wrappedContent := wrapText(msg.Content, maxWidth-12)
	
	content.WriteString(padding)
	content.WriteString(userStyle.Render("👤 " + wrappedContent))
}

// renderAssistantMessage renders an assistant message with modern styling
func (c *ModernChatView) renderAssistantMessage(content *strings.Builder, msg ModernMessage, index int, maxWidth int) {
	leftPadding := (c.width - maxWidth) / 2
	if leftPadding < 0 {
		leftPadding = 2
	}
	padding := strings.Repeat(" ", leftPadding)
	
	// Assistant header with copy button
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#059669")).
		Bold(true).
		MarginBottom(1)
	
	showCopyButton := msg.Content != "Thinking..." && msg.Content != ""
	header := "🤖 Assistant"
	
	if showCopyButton {
		currentLine := strings.Count(content.String(), "\n")
		btnX := leftPadding + maxWidth - 12
		btnY := currentLine + 1
		
		c.copyButtons = append(c.copyButtons, CopyButtonArea{
			MessageIndex: index,
			X:            btnX,
			Y:            btnY,
			Width:        10,
			Height:       1,
		})
		
		var copyBtn string
		if c.lastCopied == index {
			copyBtn = ModernCopyButtonCopiedStyle.Render("✓ Copied")
		} else {
			copyBtn = ModernCopyButtonStyle.Render("📋 Copy")
		}
		
		headerPadding := maxWidth - len(header) - 12
		if headerPadding < 1 {
			headerPadding = 1
		}
		
		fullHeader := lipgloss.JoinHorizontal(lipgloss.Left,
			headerStyle.Render(header),
			strings.Repeat(" ", headerPadding),
			copyBtn,
		)
		content.WriteString(padding)
		content.WriteString(fullHeader)
	} else {
		content.WriteString(padding)
		content.WriteString(headerStyle.Render(header))
	}
	content.WriteString("\n")
	
	// Render tool calls if any
	for _, toolCall := range msg.ToolCalls {
		c.renderToolCall(content, toolCall, padding, maxWidth)
	}
	
	// Assistant content
	if msg.Content != "" && msg.Content != "Thinking..." {
		assistantStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#f9fafb")).
			Foreground(lipgloss.Color("#111827")).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#e5e7eb")).
			Padding(1, 2).
			MarginBottom(1).
			MaxWidth(maxWidth - 8)
		
		formattedContent := c.FormatAssistantContent(msg.Content, maxWidth-12)
		
		content.WriteString(padding)
		content.WriteString(assistantStyle.Render(formattedContent))
	}
}

// renderSystemMessage renders a system message
func (c *ModernChatView) renderSystemMessage(content *strings.Builder, msg ModernMessage, maxWidth int) {
	leftPadding := (c.width - maxWidth) / 2
	if leftPadding < 0 {
		leftPadding = 2
	}
	padding := strings.Repeat(" ", leftPadding)
	
	systemStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#fef3c7")).
		Foreground(lipgloss.Color("#92400e")).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#f59e0b")).
		Padding(1, 2).
		MarginBottom(1).
		MaxWidth(maxWidth - 8)
	
	wrappedContent := wrapText(msg.Content, maxWidth-12)
	
	content.WriteString(padding)
	content.WriteString(systemStyle.Render("⚙️ " + wrappedContent))
}

// renderToolCall renders a tool call with modern styling
func (c *ModernChatView) renderToolCall(content *strings.Builder, toolCall ToolCall, padding string, maxWidth int) {
	var statusColor string
	var statusIcon string
	
	switch toolCall.Status {
	case "calling":
		statusColor = "#3b82f6"
		statusIcon = "⏳"
	case "success":
		statusColor = "#10b981"
		statusIcon = "✅"
	case "error":
		statusColor = "#ef4444"
		statusIcon = "❌"
	default:
		statusColor = "#6b7280"
		statusIcon = "🔧"
	}
	
	toolStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#f3f4f6")).
		Foreground(lipgloss.Color("#374151")).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(statusColor)).
		Padding(1, 2).
		MarginBottom(1).
		MaxWidth(maxWidth - 8)
	
	toolContent := fmt.Sprintf("%s Using %s", statusIcon, toolCall.Name)
	if toolCall.Status == "error" && toolCall.Error != "" {
		toolContent += fmt.Sprintf("\nError: %s", toolCall.Error)
	} else if toolCall.Result != "" {
		// Truncate long results
		result := toolCall.Result
		if len(result) > 200 {
			result = result[:197] + "..."
		}
		toolContent += fmt.Sprintf("\nResult: %s", result)
	}
	
	content.WriteString(padding)
	content.WriteString(toolStyle.Render(toolContent))
	content.WriteString("\n")
}

// renderThinkingIndicator renders a thinking indicator
func (c *ModernChatView) renderThinkingIndicator(content *strings.Builder, maxWidth int) {
	leftPadding := (c.width - maxWidth) / 2
	if leftPadding < 0 {
		leftPadding = 2
	}
	padding := strings.Repeat(" ", leftPadding)
	
	thinkingStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#f3f4f6")).
		Foreground(lipgloss.Color("#6b7280")).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#d1d5db")).
		Padding(1, 2).
		Italic(true)
	
	content.WriteString(padding)
	content.WriteString(thinkingStyle.Render("🤔 Thinking..."))
}

// FormatAssistantContent formats assistant content with syntax highlighting and structure
func (c *ModernChatView) FormatAssistantContent(content string, maxWidth int) string {
	// Handle code blocks
	codeBlockRegex := regexp.MustCompile("```([a-zA-Z]*)\n(.*?)```")
	content = codeBlockRegex.ReplaceAllStringFunc(content, func(match string) string {
		parts := codeBlockRegex.FindStringSubmatch(match)
		if len(parts) >= 3 {
			language := parts[1]
			code := parts[2]
			return c.renderCodeBlock(code, language, maxWidth)
		}
		return match
	})
	
	// Handle inline code
	inlineCodeRegex := regexp.MustCompile("`([^`]+)`")
	content = inlineCodeRegex.ReplaceAllStringFunc(content, func(match string) string {
		code := strings.Trim(match, "`")
		return ModernInlineCodeStyle.Render(code)
	})
	
	// Handle bold text
	boldRegex := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	content = boldRegex.ReplaceAllStringFunc(content, func(match string) string {
		text := strings.Trim(match, "*")
		return BoldTextStyle.Render(text)
	})
	
	// Handle bullet points
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") || strings.HasPrefix(strings.TrimSpace(line), "* ") {
			lines[i] = "  • " + strings.TrimSpace(line)[2:]
		}
	}
	
	return wrapText(strings.Join(lines, "\n"), maxWidth)
}

// renderCodeBlock renders a code block with syntax highlighting
func (c *ModernChatView) renderCodeBlock(code, language string, maxWidth int) string {
	codeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1f2937")).
		Foreground(lipgloss.Color("#f9fafb")).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1).
		MaxWidth(maxWidth - 4)
	
	header := ""
	if language != "" {
		header = fmt.Sprintf("📄 %s\n", language)
	}
	
	return codeStyle.Render(header + code)
}

// CopyLastMessage copies the last assistant message
func (c *ModernChatView) CopyLastMessage() {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == "assistant" {
			text := c.messages[i].Content
			if err := clipboard.WriteAll(text); err == nil {
				c.addToCopyHistory(text, "assistant")
			}
			return
		}
	}
}

// copyMessage copies a specific message by index
func (c *ModernChatView) copyMessage(messageIndex int) {
	if messageIndex >= 0 && messageIndex < len(c.messages) {
		msg := c.messages[messageIndex]
		if err := clipboard.WriteAll(msg.Content); err == nil {
			c.lastCopied = messageIndex
			c.addToCopyHistory(msg.Content, msg.Role)
			c.updateContent()
		}
	}
}

// ResetCopyFeedback resets copy feedback
func (c *ModernChatView) ResetCopyFeedback() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return CopyFeedbackResetMsg{}
	})
}

// addToCopyHistory adds to copy history
func (c *ModernChatView) addToCopyHistory(content, source string) {
	item := CopyHistoryItem{
		Content:   content,
		Timestamp: time.Now(),
		Source:    source,
	}
	
	c.copyHistory = append([]CopyHistoryItem{item}, c.copyHistory...)
	
	if len(c.copyHistory) > 10 {
		c.copyHistory = c.copyHistory[:10]
	}
}

// GetMessages returns messages in the old format for compatibility
func (c *ModernChatView) GetMessages() []ChatMessage {
	var messages []ChatMessage
	for _, msg := range c.messages {
		messages = append(messages, ChatMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		})
	}
	return messages
}

// SetMessages sets messages from old format
func (c *ModernChatView) SetMessages(messages []ChatMessage) {
	c.messages = []ModernMessage{}
	for _, msg := range messages {
		c.messages = append(c.messages, ModernMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
		})
	}
	c.updateContent()
}

// ClearMessages clears all messages
func (c *ModernChatView) ClearMessages() {
	c.messages = []ModernMessage{}
	c.updateContent()
}

// Modern styles
var (
	ModernCopyButtonStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#3b82f6")).
		Background(lipgloss.Color("#eff6ff")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#93c5fd")).
		Padding(0, 1).
		Bold(true)

	ModernCopyButtonCopiedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10b981")).
		Background(lipgloss.Color("#ecfdf5")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6ee7b7")).
		Padding(0, 1).
		Bold(true)

	ModernInlineCodeStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#f3f4f6")).
		Foreground(lipgloss.Color("#374151")).
		Padding(0, 1)

	BoldTextStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#111827"))
)