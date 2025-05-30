package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	
	"trms/internal/models"
	"trms/internal/services"
)

// ModelManagerView handles model management UI
type ModelManagerView struct {
	list         list.Model
	viewport     viewport.Model
	width        int
	height       int
	focused      bool
	showDetails  bool
	modelStates  map[string]*services.ModelStatus
}

// ModelManagerItem represents a model in the manager
type ModelManagerItem struct {
	Name        string
	State       services.ModelState
	Size        int64
	Downloaded  int64
	Percent     int
	Error       string
	IsHeader    bool
	IsSeparator bool
}

// Implement list.Item interface
func (i ModelManagerItem) Title() string {
	if i.IsHeader {
		return HeaderStyle.Render(i.Name)
	}
	if i.IsSeparator {
		return SeparatorStyle.Render("────────────────────────────────────────")
	}

	// Status icon based on state
	var status string
	switch i.State {
	case services.ModelStateComplete:
		status = "✅"
	case services.ModelStateDownloading:
		status = fmt.Sprintf("⏬ %d%%", i.Percent)
	case services.ModelStatePartial:
		status = fmt.Sprintf("⚠️  %d%%", i.Percent)
	case services.ModelStateCorrupted:
		status = "❌"
	default:
		status = "📥"
	}

	// Size info
	sizeInfo := formatBytes(i.Size)
	if i.State == services.ModelStatePartial || i.State == services.ModelStateDownloading {
		sizeInfo = fmt.Sprintf("%s / %s", formatBytes(i.Downloaded), formatBytes(i.Size))
	}

	return fmt.Sprintf("%s %-25s %s", status, i.Name, sizeInfo)
}

func (i ModelManagerItem) Description() string {
	if i.IsHeader || i.IsSeparator {
		return ""
	}

	switch i.State {
	case services.ModelStateComplete:
		return "Ready to use"
	case services.ModelStateDownloading:
		return "Currently downloading..."
	case services.ModelStatePartial:
		return "Partial download - press 'r' to resume or 'd' to delete"
	case services.ModelStateCorrupted:
		return fmt.Sprintf("Corrupted: %s - press 'd' to clean", i.Error)
	default:
		return "Not installed - press Enter to download"
	}
}

func (i ModelManagerItem) FilterValue() string {
	if i.IsHeader || i.IsSeparator {
		return ""
	}
	return i.Name
}

// ModelManagerKeys defines key bindings
type modelManagerKeyMap struct {
	Download key.Binding
	Delete   key.Binding
	Resume   key.Binding
	Clean    key.Binding
	Refresh  key.Binding
	Details  key.Binding
	Cancel   key.Binding
}

var modelManagerKeys = modelManagerKeyMap{
	Download: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("Enter", "download"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d", "delete"),
		key.WithHelp("d", "delete model"),
	),
	Resume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume download"),
	),
	Clean: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clean partial"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("R", "f5"),
		key.WithHelp("R/F5", "refresh"),
	),
	Details: key.NewBinding(
		key.WithKeys("i"),
		key.WithHelp("i", "show details"),
	),
	Cancel: key.NewBinding(
		key.WithKeys("ctrl+c", "ctrl+g"),
		key.WithHelp("Ctrl+C", "cancel download"),
	),
}

// NewModelManagerView creates a new model manager view
func NewModelManagerView(width, height int) ModelManagerView {
	items := []list.Item{}
	l := list.New(items, list.NewDefaultDelegate(), width, height-4)
	l.Title = "Model Management"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = HeaderStyle
	l.Styles.TitleBar = lipgloss.NewStyle().Background(lipgloss.Color("235"))

	vp := viewport.New(width, height/3)
	vp.SetContent("")

	return ModelManagerView{
		list:     l,
		viewport: vp,
		width:    width,
		height:   height,
	}
}

// Init initializes the view
func (m ModelManagerView) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m ModelManagerView) Update(msg tea.Msg) (ModelManagerView, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
		if m.showDetails {
			m.list.SetSize(msg.Width, (msg.Height-4)*2/3)
			m.viewport.Width = msg.Width
			m.viewport.Height = (msg.Height-4)/3
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, modelManagerKeys.Details):
			m.showDetails = !m.showDetails
			if m.showDetails {
				m.updateDetails()
			}
		case key.Matches(msg, modelManagerKeys.Refresh):
			// Trigger refresh
			return m, m.refreshModels()
		}
	}

	// Update list
	m.list, cmd = m.list.Update(msg)
	cmds = append(cmds, cmd)

	// Update viewport if showing details
	if m.showDetails {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// GetList returns the internal list for accessing selected items
func (m *ModelManagerView) GetList() list.Model {
	return m.list
}

// View renders the view
func (m ModelManagerView) View() string {
	var content string

	if m.showDetails {
		// Split view
		content = m.list.View() + "\n" + 
			lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("238")).
				Render(m.viewport.View())
	} else {
		content = m.list.View()
	}

	// Help bar
	help := m.renderHelp()

	return content + "\n" + help
}

// SetModelStates updates the model states
func (m *ModelManagerView) SetModelStates(states map[string]*services.ModelStatus, availableModels []models.ModelInfo) {
	m.modelStates = states
	
	items := []list.Item{}
	
	// Categories
	var complete []ModelManagerItem
	var downloading []ModelManagerItem
	var partial []ModelManagerItem
	var corrupted []ModelManagerItem
	var notInstalled []ModelManagerItem

	// Categorize existing states
	for name, state := range states {
		item := ModelManagerItem{
			Name:       name,
			State:      state.State,
			Size:       state.Size,
			Downloaded: state.Downloaded,
			Percent:    state.Percent,
			Error:      state.Error,
		}

		switch state.State {
		case services.ModelStateComplete:
			complete = append(complete, item)
		case services.ModelStateDownloading:
			downloading = append(downloading, item)
		case services.ModelStatePartial:
			partial = append(partial, item)
		case services.ModelStateCorrupted:
			corrupted = append(corrupted, item)
		}
	}

	// Add available models not in states
	for _, model := range availableModels {
		if _, exists := states[model.Name]; !exists {
			// Parse size string to bytes
			size := parseSize(model.Size)
			notInstalled = append(notInstalled, ModelManagerItem{
				Name:  model.Name,
				State: services.ModelStateNotInstalled,
				Size:  size,
			})
		}
	}

	// Build final list
	if len(downloading) > 0 {
		items = append(items, ModelManagerItem{
			Name:     fmt.Sprintf("DOWNLOADING (%d)", len(downloading)),
			IsHeader: true,
		})
		for _, item := range downloading {
			items = append(items, item)
		}
		items = append(items, ModelManagerItem{IsSeparator: true})
	}

	if len(partial) > 0 {
		items = append(items, ModelManagerItem{
			Name:     fmt.Sprintf("PARTIAL DOWNLOADS (%d)", len(partial)),
			IsHeader: true,
		})
		for _, item := range partial {
			items = append(items, item)
		}
		items = append(items, ModelManagerItem{IsSeparator: true})
	}

	if len(corrupted) > 0 {
		items = append(items, ModelManagerItem{
			Name:     fmt.Sprintf("CORRUPTED (%d)", len(corrupted)),
			IsHeader: true,
		})
		for _, item := range corrupted {
			items = append(items, item)
		}
		items = append(items, ModelManagerItem{IsSeparator: true})
	}

	if len(complete) > 0 {
		items = append(items, ModelManagerItem{
			Name:     fmt.Sprintf("INSTALLED (%d)", len(complete)),
			IsHeader: true,
		})
		for _, item := range complete {
			items = append(items, item)
		}
		items = append(items, ModelManagerItem{IsSeparator: true})
	}

	if len(notInstalled) > 0 {
		items = append(items, ModelManagerItem{
			Name:     fmt.Sprintf("AVAILABLE (%d)", len(notInstalled)),
			IsHeader: true,
		})
		for _, item := range notInstalled {
			items = append(items, item)
		}
	}

	m.list.SetItems(items)
}

// updateDetails updates the detail view
func (m *ModelManagerView) updateDetails() {
	selected := m.list.SelectedItem()
	if selected == nil {
		return
	}

	item, ok := selected.(ModelManagerItem)
	if !ok || item.IsHeader || item.IsSeparator {
		return
	}

	// Get detailed state
	if state, exists := m.modelStates[item.Name]; exists {
		var details strings.Builder
		details.WriteString(fmt.Sprintf("Model: %s\n", item.Name))
		details.WriteString(fmt.Sprintf("State: %s\n", stateString(state.State)))
		details.WriteString(fmt.Sprintf("Size: %s\n", formatBytes(state.Size)))
		details.WriteString(fmt.Sprintf("Downloaded: %s (%d%%)\n", formatBytes(state.Downloaded), state.Percent))
		
		if state.Error != "" {
			details.WriteString(fmt.Sprintf("Error: %s\n", state.Error))
		}

		if len(state.Layers) > 0 {
			details.WriteString(fmt.Sprintf("\nLayers (%d):\n", len(state.Layers)))
			for i, layer := range state.Layers {
				status := "❌"
				if layer.Complete {
					status = "✅"
				} else if layer.Downloaded > 0 {
					status = "⚠️"
				}
				details.WriteString(fmt.Sprintf("  %d. %s %s (%s/%s)\n", 
					i+1, status, 
					layer.Digest[:12],
					formatBytes(layer.Downloaded),
					formatBytes(layer.Size),
				))
			}
		}

		m.viewport.SetContent(details.String())
	}
}

// refreshModels triggers a model refresh
func (m *ModelManagerView) refreshModels() tea.Cmd {
	return func() tea.Msg {
		// This would trigger a refresh in the main app
		return models.ModelsRefreshedMsg{Err: nil}
	}
}

// renderHelp renders the help bar
func (m ModelManagerView) renderHelp() string {
	var helpItems []string

	selected := m.list.SelectedItem()
	if item, ok := selected.(ModelManagerItem); ok && !item.IsHeader && !item.IsSeparator {
		switch item.State {
		case services.ModelStateNotInstalled:
			helpItems = append(helpItems, "Enter: Download")
		case services.ModelStateComplete:
			helpItems = append(helpItems, "d: Delete")
		case services.ModelStatePartial:
			helpItems = append(helpItems, "r: Resume", "c: Clean")
		case services.ModelStateCorrupted:
			helpItems = append(helpItems, "c: Clean")
		case services.ModelStateDownloading:
			helpItems = append(helpItems, "Ctrl+C: Cancel")
		}
	}

	helpItems = append(helpItems, "i: Details", "R: Refresh", "↑/↓: Navigate", "ESC: Back")

	return HelpStyle.Render(strings.Join(helpItems, " • "))
}

// Helper functions
func stateString(state services.ModelState) string {
	switch state {
	case services.ModelStateNotInstalled:
		return "Not Installed"
	case services.ModelStateDownloading:
		return "Downloading"
	case services.ModelStatePartial:
		return "Partial Download"
	case services.ModelStateComplete:
		return "Complete"
	case services.ModelStateCorrupted:
		return "Corrupted"
	default:
		return "Unknown"
	}
}

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

func parseSize(sizeStr string) int64 {
	// Simple parser for sizes like "3.8GB", "400MB"
	sizeStr = strings.TrimSpace(sizeStr)
	if sizeStr == "" {
		return 0
	}

	// Extract number and unit
	var num float64
	var unit string
	fmt.Sscanf(sizeStr, "%f%s", &num, &unit)

	multiplier := int64(1)
	switch strings.ToUpper(unit) {
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	case "TB":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(num * float64(multiplier))
}

// Additional styles needed
var (
	HeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("213")).
		Bold(true)

	SeparatorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("238"))
)