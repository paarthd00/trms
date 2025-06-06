package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"trms/internal/models"
	"trms/internal/services"
)


// CleanModelManagerView is a cleaner, simpler model manager
type CleanModelManagerView struct {
	width         int
	height        int
	models        []*services.CleanModelInfo
	filteredModels []*services.CleanModelInfo
	selectedIndex int
	stateManager  *services.ModelStateManager
	searchQuery   string
	showSearch    bool
	message       string
	messageTime   time.Time
}

// NewCleanModelManagerView creates a new clean model manager view
func NewCleanModelManagerView(width, height int, stateManager *services.ModelStateManager) CleanModelManagerView {
	view := CleanModelManagerView{
		width:        width,
		height:       height,
		stateManager: stateManager,
	}
	
	// Initialize models directly from catalog to avoid loading issues
	view.loadModelsFromCatalog()
	
	return view
}

// loadModelsFromCatalog loads models directly from the catalog
func (m *CleanModelManagerView) loadModelsFromCatalog() {
	m.models = []*services.CleanModelInfo{}
	for _, catalogModel := range models.AllModels {
		m.models = append(m.models, &services.CleanModelInfo{
			Name:        catalogModel.Name,
			State:       services.CleanModelStateNotInstalled,
			Size:        parseSizeUI(catalogModel.Size),
			Description: catalogModel.Description,
		})
	}
	m.filterModels()
}

// Init initializes the view
func (m CleanModelManagerView) Init() tea.Cmd {
	return tea.Batch(
		m.refreshModels(),
		tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return t
		}),
	)
}

// Update handles messages
func (m CleanModelManagerView) Update(msg tea.Msg) (CleanModelManagerView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.moveUp()
		case "down", "j":
			m.moveDown()
		case "enter", "i":
			return m, m.handleInstall()
		case "d":
			return m, m.handleDelete()
		case "c":
			return m, m.handleCancel()
		case "/":
			m.showSearch = true
			m.searchQuery = ""
		case "esc":
			if m.showSearch {
				m.showSearch = false
				m.searchQuery = ""
				m.filterModels()
			}
		case "r":
			return m, m.refreshModels()
		default:
			if m.showSearch {
				m.searchQuery += msg.String()
				m.filterModels()
			}
		}

	case modelRefreshMsg:
		m.models = msg.models
		m.filterModels()
		m.message = fmt.Sprintf("Models refreshed (%d total)", len(m.models))
		m.messageTime = time.Now()

	case modelActionMsg:
		m.message = msg.message
		m.messageTime = time.Now()
		return m, m.refreshModels()

	case time.Time:
		// Clear old messages
		if time.Since(m.messageTime) > 3*time.Second {
			m.message = ""
		}
		// Refresh models to update download progress
		return m, tea.Batch(
			tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return t
			}),
			m.refreshModels(),
		)
	}

	return m, nil
}

// View renders the view
func (m CleanModelManagerView) View() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		MarginBottom(1).
		Render("📦 Model Manager")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Search bar
	if m.showSearch {
		searchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")).
			Bold(true)
		b.WriteString(searchStyle.Render(fmt.Sprintf("Search: %s_", m.searchQuery)))
		b.WriteString("\n\n")
	}

	// Model list
	if len(m.filteredModels) == 0 {
		if len(m.models) == 0 {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("11")).
				Render("Loading models..."))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Render("No models found"))
		}
	} else {
		visibleHeight := m.height - 10 // Account for header, help, etc.
		start := 0
		end := len(m.filteredModels)

		// Scroll to keep selected item visible
		if m.selectedIndex >= visibleHeight {
			start = m.selectedIndex - visibleHeight + 1
		}
		if start+visibleHeight < end {
			end = start + visibleHeight
		}

		for i := start; i < end; i++ {
			b.WriteString(m.renderModelItem(i))
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	// Message
	if m.message != "" {
		b.WriteString("\n\n")
		msgStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)
		b.WriteString(msgStyle.Render(m.message))
	}

	// Help
	b.WriteString("\n\n")
	help := m.renderHelp()
	b.WriteString(help)

	return b.String()
}

// renderModelItem renders a single model item
func (m CleanModelManagerView) renderModelItem(index int) string {
	model := m.filteredModels[index]
	selected := index == m.selectedIndex

	// Base style
	var style lipgloss.Style
	if selected {
		style = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("15")).
			Bold(true).
			PaddingLeft(1).
			PaddingRight(1)
	} else {
		style = lipgloss.NewStyle().
			PaddingLeft(3)
	}

	// Status icon
	var statusIcon string
	var statusColor string
	switch model.State {
	case services.CleanModelStateInstalled:
		statusIcon = "✅"
		statusColor = "10"
	case services.CleanModelStateDownloading:
		statusIcon = "⬇️"
		statusColor = "11"
	case services.CleanModelStateNotInstalled:
		statusIcon = "○"
		statusColor = "8"
	default:
		statusIcon = "?"
		statusColor = "7"
	}

	// Model name and size
	modelName := model.Name
	sizeStr := ""
	if model.Size > 0 {
		sizeStr = formatBytes(model.Size)
	}

	// Build the line
	var line strings.Builder
	if selected {
		line.WriteString("▶ ")
	}
	
	line.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(statusColor)).
		Render(statusIcon))
	line.WriteString(" ")
	line.WriteString(modelName)
	
	if sizeStr != "" {
		line.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Render(fmt.Sprintf(" (%s)", sizeStr)))
	}

	// Add progress bar if downloading
	if model.State == services.CleanModelStateDownloading && model.DownloadProgress != nil {
		progress := model.DownloadProgress
		line.WriteString("\n")
		
		// Progress bar
		progressBar := m.renderProgressBar(progress.Progress)
		line.WriteString("   ")
		line.WriteString(progressBar)
		
		// Speed and ETA
		if progress.Speed != "" {
			line.WriteString(fmt.Sprintf(" %s", progress.Speed))
		}
		if progress.ETA != "" {
			line.WriteString(fmt.Sprintf(" • ETA: %s", progress.ETA))
		}
	}

	return style.Render(line.String())
}

// renderProgressBar renders a clean progress bar
func (m CleanModelManagerView) renderProgressBar(percent float64) string {
	width := 30
	filled := int(float64(width) * percent / 100)
	empty := width - filled

	bar := "["
	bar += strings.Repeat("=", filled)
	if filled < width {
		bar += ">"
		bar += strings.Repeat("-", empty-1)
	}
	bar += "]"

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Render(fmt.Sprintf("%s %3.0f%%", bar, percent))
}

// renderHelp renders the help text
func (m CleanModelManagerView) renderHelp() string {
	var helpItems []string

	if m.showSearch {
		helpItems = []string{"Type to search", "ESC: Cancel", "Enter: Select"}
	} else {
		selected := m.getSelectedModel()
		if selected != nil {
			switch selected.State {
			case services.CleanModelStateNotInstalled:
				helpItems = []string{"Enter/i: Install", "↑↓: Navigate", "/: Search"}
			case services.CleanModelStateInstalled:
				helpItems = []string{"Enter: Use Model", "d: Delete", "↑↓: Navigate"}
			case services.CleanModelStateDownloading:
				helpItems = []string{"c: Cancel", "↑↓: Navigate"}
			}
		} else {
			helpItems = []string{"↑↓: Navigate", "/: Search", "r: Refresh"}
		}
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(strings.Join(helpItems, " • "))
}

// Helper methods

func (m *CleanModelManagerView) moveUp() {
	if m.selectedIndex > 0 {
		m.selectedIndex--
	}
}

func (m *CleanModelManagerView) moveDown() {
	if m.selectedIndex < len(m.filteredModels)-1 {
		m.selectedIndex++
	}
}

func (m *CleanModelManagerView) getSelectedModel() *services.CleanModelInfo {
	if m.selectedIndex >= 0 && m.selectedIndex < len(m.filteredModels) {
		return m.filteredModels[m.selectedIndex]
	}
	return nil
}

func (m *CleanModelManagerView) filterModels() {
	if m.searchQuery == "" {
		m.filteredModels = m.models
	} else {
		m.filteredModels = []*services.CleanModelInfo{}
		query := strings.ToLower(m.searchQuery)
		for _, model := range m.models {
			if strings.Contains(strings.ToLower(model.Name), query) ||
				strings.Contains(strings.ToLower(model.Description), query) {
				m.filteredModels = append(m.filteredModels, model)
			}
		}
	}

	// Sort models: downloading first, then installed, then not installed
	sort.Slice(m.filteredModels, func(i, j int) bool {
		mi, mj := m.filteredModels[i], m.filteredModels[j]
		
		// Downloading always first
		if mi.State == services.CleanModelStateDownloading && mj.State != services.CleanModelStateDownloading {
			return true
		}
		if mi.State != services.CleanModelStateDownloading && mj.State == services.CleanModelStateDownloading {
			return false
		}
		
		// Then installed
		if mi.State == services.CleanModelStateInstalled && mj.State == services.CleanModelStateNotInstalled {
			return true
		}
		if mi.State == services.CleanModelStateNotInstalled && mj.State == services.CleanModelStateInstalled {
			return false
		}
		
		// Finally by name
		return mi.Name < mj.Name
	})

	// Adjust selected index
	if m.selectedIndex >= len(m.filteredModels) {
		m.selectedIndex = len(m.filteredModels) - 1
	}
	if m.selectedIndex < 0 && len(m.filteredModels) > 0 {
		m.selectedIndex = 0
	}
}

// Commands

func (m *CleanModelManagerView) refreshModels() tea.Cmd {
	return func() tea.Msg {
		// Create models directly from catalog - this ensures models are always available
		var catalogModels []*services.CleanModelInfo
		for _, catalogModel := range models.AllModels {
			catalogModels = append(catalogModels, &services.CleanModelInfo{
				Name:        catalogModel.Name,
				State:       services.CleanModelStateNotInstalled,
				Size:        parseSizeUI(catalogModel.Size),
				Description: catalogModel.Description,
			})
		}
		return modelRefreshMsg{models: catalogModels}
	}
}

func (m *CleanModelManagerView) handleInstall() tea.Cmd {
	selected := m.getSelectedModel()
	if selected == nil {
		return nil
	}

	switch selected.State {
	case services.CleanModelStateNotInstalled:
		return func() tea.Msg {
			err := m.stateManager.StartDownload(selected.Name)
			if err != nil {
				return modelActionMsg{
					message: fmt.Sprintf("Failed to start download: %v", err),
				}
			}
			return modelActionMsg{
				message: fmt.Sprintf("Starting download of %s...", selected.Name),
			}
		}
	case services.CleanModelStateInstalled:
		// Switch to this model
		return func() tea.Msg {
			// This would be handled by the parent
			return modelActionMsg{
				message: fmt.Sprintf("Switched to %s", selected.Name),
			}
		}
	}

	return nil
}

func (m *CleanModelManagerView) handleDelete() tea.Cmd {
	selected := m.getSelectedModel()
	if selected == nil || selected.State != services.CleanModelStateInstalled {
		return nil
	}

	return func() tea.Msg {
		err := m.stateManager.DeleteModel(selected.Name)
		if err != nil {
			return modelActionMsg{
				message: fmt.Sprintf("Failed to delete: %v", err),
			}
		}
		return modelActionMsg{
			message: fmt.Sprintf("Deleted %s", selected.Name),
		}
	}
}

func (m *CleanModelManagerView) handleCancel() tea.Cmd {
	selected := m.getSelectedModel()
	if selected == nil || selected.State != services.CleanModelStateDownloading {
		return nil
	}

	return func() tea.Msg {
		err := m.stateManager.CancelDownload(selected.Name)
		if err != nil {
			return modelActionMsg{
				message: fmt.Sprintf("Failed to cancel: %v", err),
			}
		}
		return modelActionMsg{
			message: fmt.Sprintf("Cancelled download of %s", selected.Name),
		}
	}
}

// Message types

type modelRefreshMsg struct {
	models []*services.CleanModelInfo
}

type modelActionMsg struct {
	message string
}

// GetSelectedModelName returns the name of the selected model
func (m *CleanModelManagerView) GetSelectedModelName() string {
	selected := m.getSelectedModel()
	if selected != nil {
		return selected.Name
	}
	return ""
}

// IsModelInstalled checks if the selected model is installed
func (m *CleanModelManagerView) IsModelInstalled() bool {
	selected := m.getSelectedModel()
	return selected != nil && selected.State == services.CleanModelStateInstalled
}

// parseSizeUI converts a size string like "3.8GB" to bytes
func parseSizeUI(sizeStr string) int64 {
	if sizeStr == "" {
		return 0
	}

	var num float64
	var unit string
	fmt.Sscanf(sizeStr, "%f%s", &num, &unit)

	multiplier := int64(1)
	switch unit {
	case "KB", "kb":
		multiplier = 1024
	case "MB", "mb":
		multiplier = 1024 * 1024
	case "GB", "gb":
		multiplier = 1024 * 1024 * 1024
	case "TB", "tb":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(num * float64(multiplier))
}


// GetSelectedModel returns the currently selected model (compatible with old interface)
func (m *CleanModelManagerView) GetSelectedModel() *ModelManagerItem {
	selected := m.getSelectedModel()
	if selected == nil {
		return nil
	}

	// Convert CleanModelInfo to ModelManagerItem for compatibility
	var state services.ModelState
	switch selected.State {
	case services.CleanModelStateInstalled:
		state = services.ModelStateComplete
	case services.CleanModelStateDownloading:
		state = services.ModelStateDownloading
	case services.CleanModelStateNotInstalled:
		state = services.ModelStateNotInstalled
	default:
		state = services.ModelStateNotInstalled
	}

	return &ModelManagerItem{
		Name:        selected.Name,
		State:       state,
		Size:        selected.Size,
		IsHeader:    false,
		IsSeparator: false,
	}
}

// SetModelStates updates the model states (compatible with old interface)
func (m *CleanModelManagerView) SetModelStates(states map[string]*services.ModelStatus, enrichedModels []models.ModelInfo) {
	// This is handled automatically by the state manager refresh
	// The state manager will be updated externally
}

// SetCurrentModel sets the current model (compatible with old interface)
func (m *CleanModelManagerView) SetCurrentModel(modelName string) {
	// This is handled by the parent application
}

// UpdateProgressWithStats updates progress for a specific model (compatible with old interface)
func (m *CleanModelManagerView) UpdateProgressWithStats(modelName string, percent int, downloaded, total int64, speed, eta string) {
	// Progress is handled automatically by the state manager
}

// SetModelDownloading marks a model as downloading (compatible with old interface)
func (m *CleanModelManagerView) SetModelDownloading(modelName string) {
	// This is handled automatically by the state manager
}