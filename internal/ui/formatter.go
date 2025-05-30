package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// FormatAIResponse formats AI response text with rich formatting
func FormatAIResponse(content string, maxWidth int) string {
	if content == "" {
		return ""
	}

	if maxWidth <= 0 {
		maxWidth = 80
	}

	formatter := &ResponseFormatter{
		maxWidth: maxWidth,
	}

	return formatter.Format(content)
}

// ResponseFormatter handles text formatting for AI responses
type ResponseFormatter struct {
	maxWidth int
}

// Format processes the entire content
func (f *ResponseFormatter) Format(content string) string {
	var result strings.Builder
	lines := strings.Split(content, "\n")

	inCodeBlock := false
	codeLanguage := ""
	var codeBlockContent strings.Builder

	for i, line := range lines {
		// Handle code blocks
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End code block
				code := codeBlockContent.String()
				result.WriteString(f.formatCodeBlock(code, codeLanguage))
				if i < len(lines)-1 {
					result.WriteString("\n")
				}
				codeBlockContent.Reset()
				inCodeBlock = false
				codeLanguage = ""
			} else {
				// Start code block
				inCodeBlock = true
				if len(line) > 3 {
					codeLanguage = strings.TrimSpace(line[3:])
				}
			}
			continue
		}

		if inCodeBlock {
			codeBlockContent.WriteString(line)
			if i < len(lines)-1 {
				codeBlockContent.WriteString("\n")
			}
		} else {
			// Process line with inline formatting
			formatted := f.formatLine(line)
			result.WriteString(formatted)
			if i < len(lines)-1 {
				result.WriteString("\n")
			}
		}
	}

	// Handle unclosed code block
	if inCodeBlock {
		code := codeBlockContent.String()
		result.WriteString(f.formatCodeBlock(code, codeLanguage))
	}

	return result.String()
}

// formatLine processes a single line with inline formatting
func (f *ResponseFormatter) formatLine(line string) string {
	// Handle headers
	if strings.HasPrefix(line, "# ") {
		heading := strings.TrimPrefix(line, "# ")
		return HeadingStyle.Render(heading)
	}
	if strings.HasPrefix(line, "## ") {
		heading := strings.TrimPrefix(line, "## ")
		return SubheadingStyle.Render(heading)
	}
	if strings.HasPrefix(line, "### ") {
		heading := strings.TrimPrefix(line, "### ")
		return Heading3Style.Render(heading)
	}

	// Handle lists
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		bullet := BulletStyle.Render("•")
		content := line[2:]
		content = f.formatInlineElements(content)
		return bullet + " " + content
	}

	// Handle numbered lists
	if matched, _ := regexp.MatchString(`^\d+\.\s`, line); matched {
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			num := NumberStyle.Render(parts[0])
			content := f.formatInlineElements(parts[1])
			return num + " " + content
		}
	}

	// Handle blockquotes
	if strings.HasPrefix(line, "> ") {
		quote := strings.TrimPrefix(line, "> ")
		quote = f.formatInlineElements(quote)
		return QuoteStyle.Render("│ " + quote)
	}

	// Handle horizontal rules
	if line == "---" || line == "***" || line == "___" {
		return HorizontalRuleStyle.Render(strings.Repeat("─", f.maxWidth))
	}

	// Regular line with inline formatting
	formatted := f.formatInlineElements(line)
	return f.wrapText(formatted, f.maxWidth)
}

// formatInlineElements handles inline formatting like bold, italic, code
func (f *ResponseFormatter) formatInlineElements(text string) string {
	// Order matters: process in specific order to avoid conflicts
	
	// Inline code (backticks)
	text = f.formatInlineCode(text)
	
	// Bold (** or __)
	text = f.formatBold(text)
	
	// Italic (* or _)
	text = f.formatItalic(text)
	
	// Links
	text = f.formatLinks(text)
	
	return text
}

// formatInlineCode formats `code` segments
func (f *ResponseFormatter) formatInlineCode(text string) string {
	re := regexp.MustCompile("`([^`]+)`")
	return re.ReplaceAllStringFunc(text, func(match string) string {
		code := strings.Trim(match, "`")
		return InlineCodeStyle.Render(code)
	})
}

// formatBold formats **bold** and __bold__ text
func (f *ResponseFormatter) formatBold(text string) string {
	// Handle ** first
	re := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	text = re.ReplaceAllStringFunc(text, func(match string) string {
		content := strings.Trim(match, "*")
		return BoldStyle.Render(content)
	})
	
	// Then handle __
	re = regexp.MustCompile(`__([^_]+)__`)
	text = re.ReplaceAllStringFunc(text, func(match string) string {
		content := strings.Trim(match, "_")
		return BoldStyle.Render(content)
	})
	
	return text
}

// formatItalic formats *italic* and _italic_ text
func (f *ResponseFormatter) formatItalic(text string) string {
	// Handle single * (but not **)
	re := regexp.MustCompile(`(?:^|[^*])\*([^*\n]+)\*(?:[^*]|$)`)
	text = re.ReplaceAllStringFunc(text, func(match string) string {
		// Extract content between single asterisks
		trimmed := strings.Trim(match, " ")
		if strings.HasPrefix(trimmed, "*") && strings.HasSuffix(trimmed, "*") {
			content := trimmed[1 : len(trimmed)-1]
			prefix := ""
			suffix := ""
			
			if !strings.HasPrefix(match, "*") {
				prefix = match[0:1]
			}
			if !strings.HasSuffix(match, "*") {
				suffix = match[len(match)-1:]
			}
			
			return prefix + ItalicStyle.Render(content) + suffix
		}
		return match
	})
	
	// Handle single _ (but not __)
	re = regexp.MustCompile(`(?:^|[^_])_([^_\n]+)_(?:[^_]|$)`)
	text = re.ReplaceAllStringFunc(text, func(match string) string {
		trimmed := strings.Trim(match, " ")
		if strings.HasPrefix(trimmed, "_") && strings.HasSuffix(trimmed, "_") {
			content := trimmed[1 : len(trimmed)-1]
			prefix := ""
			suffix := ""
			
			if !strings.HasPrefix(match, "_") {
				prefix = match[0:1]
			}
			if !strings.HasSuffix(match, "_") {
				suffix = match[len(match)-1:]
			}
			
			return prefix + ItalicStyle.Render(content) + suffix
		}
		return match
	})
	
	return text
}

// formatLinks formats [text](url) links
func (f *ResponseFormatter) formatLinks(text string) string {
	re := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	return re.ReplaceAllStringFunc(text, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) >= 3 {
			linkText := parts[1]
			url := parts[2]
			return LinkStyle.Render(fmt.Sprintf("%s (%s)", linkText, url))
		}
		return match
	})
}

// formatCodeBlock formats a code block with syntax highlighting hints
func (f *ResponseFormatter) formatCodeBlock(code, language string) string {
	var result strings.Builder
	
	// Language label
	if language != "" {
		label := LanguageLabelStyle.Render(language)
		result.WriteString(label)
		result.WriteString("\n")
	}
	
	// Code block
	codeFormatted := CodeBlockStyle.Width(f.maxWidth - 4).Render(code)
	result.WriteString(codeFormatted)
	
	return result.String()
}

// wrapText wraps text to fit within the specified width
func (f *ResponseFormatter) wrapText(text string, width int) string {
	if width <= 0 || width > 200 {
		width = 80
	}

	// Handle pre-formatted text (don't wrap)
	if strings.Contains(text, "\t") || strings.HasPrefix(strings.TrimSpace(text), "    ") {
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
		// Strip ANSI codes for accurate length calculation
		wordLength := utf8.RuneCountInString(stripANSI(word))

		// Check if adding this word would exceed width
		needNewLine := false
		if currentLength > 0 {
			// Account for space between words
			if currentLength + 1 + wordLength > width {
				needNewLine = true
			}
		} else if wordLength > width {
			// Word is longer than entire width, force break it
			for i, r := range word {
				if i > 0 && i % width == 0 {
					result.WriteString("\n")
				}
				result.WriteRune(r)
			}
			continue
		}

		if needNewLine {
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

// stripANSI removes ANSI escape codes for accurate length calculation
func stripANSI(text string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(text, "")
}

// Formatting styles
var (
	HeadingStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		MarginTop(1).
		MarginBottom(1)

	SubheadingStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("213"))

	Heading3Style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	BoldStyle = lipgloss.NewStyle().
		Bold(true)

	ItalicStyle = lipgloss.NewStyle().
		Italic(true)

	InlineCodeStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("239")).
		Foreground(lipgloss.Color("213")).
		Padding(0, 1)

	CodeBlockStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("237")).
		Foreground(lipgloss.Color("252")).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(1, 2)

	LanguageLabelStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("239")).
		Foreground(lipgloss.Color("213")).
		Padding(0, 1).
		Bold(true)

	LinkStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).
		Underline(true)

	BulletStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("86"))

	NumberStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)

	QuoteStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true).
		MarginLeft(2)

	HorizontalRuleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("238"))
)