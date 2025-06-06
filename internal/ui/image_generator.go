package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ImageGeneratorView handles image generation UI
type ImageGeneratorView struct {
	promptInput     textinput.Model
	viewport        viewport.Model
	width           int
	height          int
	focused         bool
	
	// Image generation state
	currentModel    string
	isGenerating    bool
	generatedImages []GeneratedImage
	selectedImage   int
	
	// Settings
	steps          int
	guidance       float64
	width_setting  int
	height_setting int
}

// GeneratedImage represents a generated image
type GeneratedImage struct {
	Path       string
	Prompt     string
	Model      string
	Timestamp  time.Time
	Parameters ImageGenerationParams
}

// ImageGenerationParams holds image generation parameters for UI
type ImageGenerationParams struct {
	Steps     int     `json:"steps"`
	Guidance  float64 `json:"guidance"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
}

// ImageGeneratorKeys defines key bindings for image generation
type imageGeneratorKeyMap struct {
	Generate     key.Binding
	Save         key.Binding
	Delete       key.Binding
	NextImage    key.Binding
	PrevImage    key.Binding
	ShowSettings key.Binding
	Back         key.Binding
}

var imageGeneratorKeys = imageGeneratorKeyMap{
	Generate: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("Enter", "generate image"),
	),
	Save: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "save image"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete image"),
	),
	NextImage: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "next image"),
	),
	PrevImage: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "prev image"),
	),
	ShowSettings: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("Tab", "settings"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("ESC", "back"),
	),
}

// NewImageGeneratorView creates a new image generator view
func NewImageGeneratorView(width, height int) ImageGeneratorView {
	promptInput := textinput.New()
	promptInput.Placeholder = "Enter your image prompt..."
	promptInput.CharLimit = 500
	promptInput.Width = width - 4
	promptInput.Focus()

	vp := viewport.New(width, height-6)
	vp.SetContent("")

	return ImageGeneratorView{
		promptInput:    promptInput,
		viewport:       vp,
		width:          width,
		height:         height,
		generatedImages: []GeneratedImage{},
		selectedImage:  0,
		// Default settings
		steps:          20,
		guidance:       7.5,
		width_setting:  512,
		height_setting: 512,
	}
}

// Init initializes the view
func (m ImageGeneratorView) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages
func (m ImageGeneratorView) Update(msg tea.Msg) (ImageGeneratorView, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.promptInput.Width = msg.Width - 4
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, imageGeneratorKeys.Generate):
			if m.promptInput.Value() != "" && !m.isGenerating {
				return m, m.generateImage()
			}
		case key.Matches(msg, imageGeneratorKeys.Save):
			if len(m.generatedImages) > 0 {
				return m, m.saveCurrentImage()
			}
		case key.Matches(msg, imageGeneratorKeys.Delete):
			if len(m.generatedImages) > 0 {
				m.deleteCurrentImage()
				m.updateViewport()
			}
		case key.Matches(msg, imageGeneratorKeys.NextImage):
			if len(m.generatedImages) > 0 {
				m.selectedImage = (m.selectedImage + 1) % len(m.generatedImages)
				m.updateViewport()
			}
		case key.Matches(msg, imageGeneratorKeys.PrevImage):
			if len(m.generatedImages) > 0 {
				m.selectedImage = (m.selectedImage - 1 + len(m.generatedImages)) % len(m.generatedImages)
				m.updateViewport()
			}
		default:
			m.promptInput, cmd = m.promptInput.Update(msg)
			cmds = append(cmds, cmd)
		}

	case ImageGeneratedMsg:
		m.isGenerating = false
		if msg.Err == nil {
			// Add generated image to list
			m.generatedImages = append(m.generatedImages, GeneratedImage{
				Path:      msg.ImagePath,
				Prompt:    m.promptInput.Value(),
				Model:     m.currentModel,
				Timestamp: time.Now(),
				Parameters: ImageGenerationParams{
					Steps:    m.steps,
					Guidance: m.guidance,
					Width:    m.width_setting,
					Height:   m.height_setting,
				},
			})
			m.selectedImage = len(m.generatedImages) - 1
			m.updateViewport()
		}

	case ImageGenerationStartedMsg:
		m.isGenerating = true
		m.updateViewport()
	}

	// Update viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the view
func (m ImageGeneratorView) View() string {
	var content strings.Builder

	// Header
	content.WriteString(m.renderHeader())
	content.WriteString("\n\n")

	// Prompt input
	content.WriteString("Prompt: ")
	content.WriteString(m.promptInput.View())
	content.WriteString("\n\n")

	// Generation status or image display
	if m.isGenerating {
		content.WriteString("🎨 Generating image...")
	} else if len(m.generatedImages) > 0 {
		content.WriteString(m.renderImageGallery())
	} else {
		content.WriteString("No images generated yet. Enter a prompt and press Enter to generate.")
	}

	content.WriteString("\n\n")
	
	// Settings panel
	content.WriteString(m.renderSettings())
	content.WriteString("\n")

	// Help
	content.WriteString(m.renderHelp())

	return content.String()
}

// SetCurrentModel sets the current image generation model
func (m *ImageGeneratorView) SetCurrentModel(model string) {
	m.currentModel = model
}

// renderHeader renders the header with current model info
func (m ImageGeneratorView) renderHeader() string {
	if m.currentModel == "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true).
			Render("🎨 Image Generator - No model selected")
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true).
		Render(fmt.Sprintf("🎨 Image Generator - %s", m.currentModel))
}

// renderImageGallery renders the image gallery
func (m ImageGeneratorView) renderImageGallery() string {
	if len(m.generatedImages) == 0 {
		return ""
	}

	var content strings.Builder
	current := m.generatedImages[m.selectedImage]

	// Image counter
	content.WriteString(fmt.Sprintf("Image %d of %d\n\n", m.selectedImage+1, len(m.generatedImages)))

	// Image info
	infoStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238")).
		Padding(1)

	info := fmt.Sprintf("📁 Path: %s\n"+
		"💭 Prompt: %s\n"+
		"🤖 Model: %s\n"+
		"⏰ Generated: %s\n"+
		"⚙️  Settings: %dx%d, %d steps, guidance %.1f",
		current.Path,
		current.Prompt,
		current.Model,
		current.Timestamp.Format("2006-01-02 15:04:05"),
		current.Parameters.Width,
		current.Parameters.Height,
		current.Parameters.Steps,
		current.Parameters.Guidance)

	content.WriteString(infoStyle.Render(info))

	return content.String()
}

// renderSettings renders the settings panel
func (m ImageGeneratorView) renderSettings() string {
	settingsStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1)

	settings := fmt.Sprintf("Settings: %dx%d | Steps: %d | Guidance: %.1f",
		m.width_setting, m.height_setting, m.steps, m.guidance)

	return settingsStyle.Render(settings)
}

// renderHelp renders the help text
func (m ImageGeneratorView) renderHelp() string {
	var helpItems []string

	if m.currentModel == "" {
		helpItems = append(helpItems, "Select an image generation model first")
	} else if m.isGenerating {
		helpItems = append(helpItems, "Generating...")
	} else {
		helpItems = append(helpItems, "Enter: Generate")
		if len(m.generatedImages) > 0 {
			helpItems = append(helpItems, "←/→: Navigate", "s: Save", "d: Delete")
		}
	}

	helpItems = append(helpItems, "Tab: Settings", "ESC: Back")

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Render(strings.Join(helpItems, " • "))
}

// generateImage generates an image using the current model and prompt
func (m *ImageGeneratorView) generateImage() tea.Cmd {
	return func() tea.Msg {
		// Return a message to start generation
		return ImageGenerationStartedMsg{
			Model:  m.currentModel,
			Prompt: m.promptInput.Value(),
			Parameters: ImageGenerationParams{
				Steps:    m.steps,
				Guidance: m.guidance,
				Width:    m.width_setting,
				Height:   m.height_setting,
			},
		}
	}
}

// saveCurrentImage saves the current image to downloads
func (m *ImageGeneratorView) saveCurrentImage() tea.Cmd {
	if len(m.generatedImages) == 0 {
		return nil
	}

	current := m.generatedImages[m.selectedImage]
	return func() tea.Msg {
		// Create downloads directory if it doesn't exist
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ImageSavedMsg{Err: err}
		}

		downloadsDir := filepath.Join(homeDir, "Downloads", "trms-images")
		err = os.MkdirAll(downloadsDir, 0755)
		if err != nil {
			return ImageSavedMsg{Err: err}
		}

		// Generate unique filename
		timestamp := current.Timestamp.Format("2006-01-02_15-04-05")
		filename := fmt.Sprintf("%s_%s.png", timestamp, strings.ReplaceAll(current.Model, ":", "_"))
		destPath := filepath.Join(downloadsDir, filename)

		// Copy file
		err = copyFile(current.Path, destPath)
		if err != nil {
			return ImageSavedMsg{Err: err}
		}

		return ImageSavedMsg{Path: destPath}
	}
}

// deleteCurrentImage deletes the current image
func (m *ImageGeneratorView) deleteCurrentImage() {
	if len(m.generatedImages) == 0 {
		return
	}

	// Remove file
	current := m.generatedImages[m.selectedImage]
	os.Remove(current.Path)

	// Remove from list
	m.generatedImages = append(m.generatedImages[:m.selectedImage], m.generatedImages[m.selectedImage+1:]...)

	// Adjust selection
	if m.selectedImage >= len(m.generatedImages) && len(m.generatedImages) > 0 {
		m.selectedImage = len(m.generatedImages) - 1
	}
}

// updateViewport updates the viewport content
func (m *ImageGeneratorView) updateViewport() {
	// This could be used for additional content if needed
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}

// Message types for image generation
type ImageGenerationStartedMsg struct {
	Model      string
	Prompt     string
	Parameters ImageGenerationParams
}

type ImageGeneratedMsg struct {
	ImagePath string
	Err       error
}

type ImageSavedMsg struct {
	Path string
	Err  error
}