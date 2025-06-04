package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	
	"trms/internal/models"
	"trms/internal/services"
)

// ModelManagerView handles model management UI
type ModelManagerView struct {
	viewport     viewport.Model
	width        int
	height       int
	focused      bool
	showDetails  bool
	modelStates  map[string]*services.ModelStatus
	
	// Filtering and search
	categories         map[string]models.ModelCategory
	categorizedModels  map[string][]models.ModelInfo
	currentCategory    string
	allModels         []models.ModelInfo
	filteredModels    []ModelManagerItem
	selectedIndex     int
	
	// Search input
	searchInput       textinput.Model
	searchMode        bool
	searchQuery       string
}

// ModelManagerItem represents a model in the manager
type ModelManagerItem struct {
	Name        string
	State       services.ModelState
	Size        int64
	Downloaded  int64
	Percent     int
	Error       string
	Speed       string // Download speed like "5.2 MB/s"
	ETA         string // Estimated time remaining like "2m 30s"
	IsHeader    bool
	IsSeparator bool
	IsCurrent   bool   // Whether this is the currently selected model
}

// Implement list.Item interface
func (i ModelManagerItem) Title() string {
	if i.IsHeader {
		return HeaderStyle.Render(i.Name)
	}
	if i.IsSeparator {
		return SeparatorStyle.Render("────────────────────────────────────────")
	}

	// Status icon and progress based on state
	var status, progressBar string
	switch i.State {
	case services.ModelStateComplete:
		if i.IsCurrent {
			status = "●" // Current model indicator - clean dot
		} else {
			status = "✓" // Clean checkmark
		}
	case services.ModelStateDownloading:
		status = fmt.Sprintf("↓ %d%%", i.Percent)
		progressBar = renderProgressBar(i.Percent, 20, "█", "░")
	case services.ModelStatePartial:
		status = fmt.Sprintf("⚠ %d%%", i.Percent)
		progressBar = renderProgressBar(i.Percent, 20, "█", "░")
	case services.ModelStateCorrupted:
		status = "✗"
	default:
		status = "○"
	}

	// Size info with better formatting
	var sizeInfo string
	if i.State == services.ModelStatePartial || i.State == services.ModelStateDownloading {
		sizeInfo = fmt.Sprintf("%s / %s", formatBytes(i.Downloaded), formatBytes(i.Size))
		if progressBar != "" {
			sizeInfo += fmt.Sprintf("\n    %s", progressBar)
		}
	} else {
		sizeInfo = formatBytes(i.Size)
	}

	return fmt.Sprintf("%s %-25s %s", status, i.Name, sizeInfo)
}

func (i ModelManagerItem) Description() string {
	if i.IsHeader || i.IsSeparator {
		return ""
	}

	switch i.State {
	case services.ModelStateComplete:
		if i.IsCurrent {
			return "Currently active model"
		} else {
			return "Ready to use • Press Enter to switch"
		}
	case services.ModelStateDownloading:
		desc := fmt.Sprintf("Downloading %d%%", i.Percent)
		if i.Speed != "" && i.ETA != "" {
			desc += fmt.Sprintf(" • %s • ETA: %s", i.Speed, i.ETA)
		}
		return desc
	case services.ModelStatePartial:
		return fmt.Sprintf("Partial download %d%% • Press Enter to resume or 'c' to clean", i.Percent)
	case services.ModelStateCorrupted:
		return fmt.Sprintf("Corrupted: %s • Press 'c' to clean", i.Error)
	default:
		return "Available for download • Press Enter to start"
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
	Download     key.Binding
	Delete       key.Binding
	Resume       key.Binding
	Clean        key.Binding
	Refresh      key.Binding
	Details      key.Binding
	Cancel       key.Binding
	Search       key.Binding
	ExitSearch   key.Binding
	NextCategory key.Binding
	PrevCategory key.Binding
	Up           key.Binding
	Down         key.Binding
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
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	ExitSearch: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("ESC", "exit search"),
	),
	NextCategory: key.NewBinding(
		key.WithKeys("tab", "right"),
		key.WithHelp("Tab/→", "next category"),
	),
	PrevCategory: key.NewBinding(
		key.WithKeys("shift+tab", "left"),
		key.WithHelp("Shift+Tab/←", "prev category"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
}

// NewModelManagerView creates a new model manager view
func NewModelManagerView(width, height int) ModelManagerView {
	vp := viewport.New(width, height/3)
	vp.SetContent("")

	// Initialize search input
	searchInput := textinput.New()
	searchInput.Placeholder = "Search models..."
	searchInput.CharLimit = 50
	searchInput.Width = width - 20

	// Initialize category data
	categorizedModels, categories, _ := models.GetModelsByCategory()
	allModels, _ := models.LoadModelsFromJSON()

	m := ModelManagerView{
		viewport:          vp,
		width:             width,
		height:            height,
		categories:        categories,
		categorizedModels: categorizedModels,
		currentCategory:   "all",
		allModels:         allModels,
		searchInput:       searchInput,
		selectedIndex:     0,
	}

	// Initialize filtered models
	m.updateFilteredModels()
	return m
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
		m.searchInput.Width = msg.Width - 20
		if m.showDetails {
			m.viewport.Width = msg.Width
			m.viewport.Height = (msg.Height-6)/3
		}

	case tea.KeyMsg:
		if m.searchMode {
			// Handle search mode keys
			switch {
			case key.Matches(msg, modelManagerKeys.ExitSearch):
				m.searchMode = false
				m.searchInput.Blur()
				m.searchQuery = ""
				m.searchInput.SetValue("")
				m.updateFilteredModels()
			case msg.Type == tea.KeyEnter:
				m.searchMode = false
				m.searchInput.Blur()
				m.searchQuery = m.searchInput.Value()
				m.updateFilteredModels()
			default:
				m.searchInput, cmd = m.searchInput.Update(msg)
				cmds = append(cmds, cmd)
				// Update search query in real-time
				m.searchQuery = m.searchInput.Value()
				m.updateFilteredModels()
			}
		} else {
			// Handle normal mode keys
			switch {
			case key.Matches(msg, modelManagerKeys.Search):
				m.searchMode = true
				m.searchInput.Focus()
			case key.Matches(msg, modelManagerKeys.NextCategory):
				m.nextCategory()
				m.updateFilteredModels()
			case key.Matches(msg, modelManagerKeys.PrevCategory):
				m.prevCategory()
				m.updateFilteredModels()
			case key.Matches(msg, modelManagerKeys.Up):
				if m.selectedIndex > 0 {
					m.selectedIndex--
				}
			case key.Matches(msg, modelManagerKeys.Down):
				if m.selectedIndex < len(m.filteredModels)-1 {
					m.selectedIndex++
				}
			case key.Matches(msg, modelManagerKeys.Details):
				m.showDetails = !m.showDetails
				if m.showDetails {
					m.updateDetails()
				}
			case key.Matches(msg, modelManagerKeys.Refresh):
				return m, m.refreshModels()
			}
		}

	default:
		// Handle other messages
	}

	// Update viewport if showing details
	if m.showDetails {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// GetSelectedModel returns the currently selected model
func (m *ModelManagerView) GetSelectedModel() *ModelManagerItem {
	if m.selectedIndex >= 0 && m.selectedIndex < len(m.filteredModels) {
		return &m.filteredModels[m.selectedIndex]
	}
	return nil
}

// View renders the view
func (m ModelManagerView) View() string {
	var content strings.Builder

	// Category selector and search bar
	header := m.renderHeader()
	content.WriteString(header)
	content.WriteString("\n")

	// Model list
	modelList := m.renderModelList()
	content.WriteString(modelList)

	// Details view if enabled
	if m.showDetails {
		content.WriteString("\n")
		content.WriteString(
			lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("238")).
				Render(m.viewport.View()))
	}

	// Help bar
	help := m.renderHelp()
	content.WriteString("\n")
	content.WriteString(help)

	return content.String()
}

// SetModelStates updates the model states
func (m *ModelManagerView) SetModelStates(states map[string]*services.ModelStatus, availableModels []models.ModelInfo) {
	m.modelStates = states
	m.allModels = availableModels
	m.updateFilteredModels()
}

// updateDetails updates the detail view
func (m *ModelManagerView) updateDetails() {
	selected := m.GetSelectedModel()
	if selected == nil || selected.IsHeader || selected.IsSeparator {
		return
	}

	// Get detailed state
	if state, exists := m.modelStates[selected.Name]; exists {
		var details strings.Builder
		details.WriteString(fmt.Sprintf("Model: %s\n", selected.Name))
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

	if m.searchMode {
		helpItems = append(helpItems, "ESC: Exit search", "Enter: Apply filter")
	} else {
		selected := m.GetSelectedModel()
		if selected != nil && !selected.IsHeader && !selected.IsSeparator {
			switch selected.State {
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
		
		helpItems = append(helpItems, "/: Search", "Tab/Shift+Tab: Category", "↑/↓: Navigate")
		if m.currentCategory != "all" {
			if category, exists := m.categories[m.currentCategory]; exists {
				helpItems = append(helpItems, fmt.Sprintf("Filter: %s %s", category.Icon, category.Name))
			}
		}
	}

	return ModelHelpStyle.Render(strings.Join(helpItems, " • "))
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

// nextCategory cycles through categories
func (m *ModelManagerView) nextCategory() {
	categories := []string{"all", "general", "small", "coding", "vision", "image-generation", "math", "creative", "multilingual", "embedding", "specialized"}
	
	currentIndex := 0
	for i, cat := range categories {
		if cat == m.currentCategory {
			currentIndex = i
			break
		}
	}
	
	nextIndex := (currentIndex + 1) % len(categories)
	m.currentCategory = categories[nextIndex]
	m.selectedIndex = 0 // Reset selection
}

// prevCategory cycles backwards through categories
func (m *ModelManagerView) prevCategory() {
	categories := []string{"all", "general", "small", "coding", "vision", "image-generation", "math", "creative", "multilingual", "embedding", "specialized"}
	
	currentIndex := 0
	for i, cat := range categories {
		if cat == m.currentCategory {
			currentIndex = i
			break
		}
	}
	
	prevIndex := (currentIndex - 1 + len(categories)) % len(categories)
	m.currentCategory = categories[prevIndex]
	m.selectedIndex = 0 // Reset selection
}

// updateFilteredModels updates the filtered model list based on category and search
func (m *ModelManagerView) updateFilteredModels() {
	var allItems []ModelManagerItem
	processedModels := make(map[string]bool)
	
	// First, add all downloaded/downloading models from modelStates
	if m.modelStates != nil {
		for name, status := range m.modelStates {
			// Apply fuzzy search filter
			if m.searchQuery != "" && !fuzzyMatch(strings.ToLower(name), strings.ToLower(m.searchQuery)) {
				continue
			}
			
			// For category filtering, we need to check if this model belongs to the category
			shouldInclude := m.currentCategory == "all"
			if !shouldInclude {
				// Check if the model is in the current category from JSON
				if categoryModels, exists := m.categorizedModels[m.currentCategory]; exists {
					for _, model := range categoryModels {
						if model.Name == name {
							shouldInclude = true
							break
						}
					}
				}
				
				// Also check if it's in allModels with matching category tag
				if !shouldInclude {
					for _, model := range m.allModels {
						if model.Name == name && len(model.Tags) > 0 && model.Tags[0] == m.currentCategory {
							shouldInclude = true
							break
						}
					}
				}
			}
			
			if shouldInclude {
				item := ModelManagerItem{
					Name:       name,
					State:      status.State,
					Size:       status.Size,
					Downloaded: status.Downloaded,
					Percent:    status.Percent,
					Error:      status.Error,
				}
				allItems = append(allItems, item)
				processedModels[name] = true
			}
		}
	}
	
	// Then add available models from JSON that haven't been processed yet
	var modelsToShow []models.ModelInfo
	if m.currentCategory == "all" {
		modelsToShow = m.allModels
	} else {
		if categoryModels, exists := m.categorizedModels[m.currentCategory]; exists {
			modelsToShow = categoryModels
		}
		
		// Also include any models from allModels that match the category
		for _, model := range m.allModels {
			if len(model.Tags) > 0 && model.Tags[0] == m.currentCategory {
				found := false
				for _, existing := range modelsToShow {
					if existing.Name == model.Name {
						found = true
						break
					}
				}
				if !found {
					modelsToShow = append(modelsToShow, model)
				}
			}
		}
	}
	
	// Convert to ModelManagerItems and apply search filter
	for _, model := range modelsToShow {
		// Skip if already processed from modelStates
		if processedModels[model.Name] {
			continue
		}
		
		// Skip headers and separators
		if len(model.Tags) > 0 && (model.Tags[0] == "header" || model.Tags[0] == "separator") {
			continue
		}
		
		// Apply fuzzy search filter
		if m.searchQuery != "" && !fuzzyMatch(strings.ToLower(model.Name), strings.ToLower(m.searchQuery)) {
			continue
		}
		
		size := parseSize(model.Size)
		item := ModelManagerItem{
			Name:  model.Name,
			State: services.ModelStateNotInstalled,
			Size:  size,
		}
		
		allItems = append(allItems, item)
	}
	
	// Sort by priority: downloading > partial > installed > available
	sort.Slice(allItems, func(i, j int) bool {
		priority := func(state services.ModelState) int {
			switch state {
			case services.ModelStateDownloading:
				return 0
			case services.ModelStatePartial:
				return 1
			case services.ModelStateComplete:
				return 2
			default:
				return 3
			}
		}
		return priority(allItems[i].State) < priority(allItems[j].State)
	})
	
	m.filteredModels = allItems
	
	// Reset selection if out of bounds
	if m.selectedIndex >= len(m.filteredModels) {
		m.selectedIndex = 0
	}
	if len(m.filteredModels) == 0 {
		m.selectedIndex = 0
	}
}





// renderProgressBar creates a visual progress bar
func renderProgressBar(percent, width int, filled, empty string) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	
	filledWidth := (percent * width) / 100
	emptyWidth := width - filledWidth
	
	bar := strings.Repeat(filled, filledWidth) + strings.Repeat(empty, emptyWidth)
	return fmt.Sprintf("[%s] %d%%", bar, percent)
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

// UpdateProgress updates the progress for a specific model
func (m *ModelManagerView) UpdateProgress(modelName string, percent int, downloaded, total string) {
	// Update the model states map if we have it
	if m.modelStates != nil {
		if status, exists := m.modelStates[modelName]; exists {
			status.Percent = percent
			status.State = services.ModelStateDownloading
		}
	}
	
	// Update filtered models
	m.updateFilteredModels()
}

// SetModelDownloading marks a model as starting to download
func (m *ModelManagerView) SetModelDownloading(modelName string) {
	// Update the model states map if we have it
	if m.modelStates != nil {
		if status, exists := m.modelStates[modelName]; exists {
			status.State = services.ModelStateDownloading
			status.Percent = 0
		}
	}
	
	// Update filtered models
	m.updateFilteredModels()
}

// SetCurrentModel marks a model as the currently active one
func (m *ModelManagerView) SetCurrentModel(currentModelName string) {
	for i := range m.filteredModels {
		m.filteredModels[i].IsCurrent = (m.filteredModels[i].Name == currentModelName)
	}
}

// Additional styles needed
var (
	HeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true).
		Padding(0, 1)

	SeparatorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))
		
	ModelHelpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))
		
	CategoryStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("12")).
		Bold(true).
		Padding(0, 1)
		
	SelectedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("63")).
		Bold(true).
		Padding(0, 1)
		
	SearchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")).
		Bold(true)
)

// renderHeader renders the category selector and search bar
func (m ModelManagerView) renderHeader() string {
	var header strings.Builder
	
	// Category selector with wrapping
	categories := []string{"all", "general", "small", "coding", "vision", "image-generation", "math", "creative", "multilingual", "embedding", "specialized"}
	var categoryButtons []string
	
	for _, cat := range categories {
		var style lipgloss.Style
		var text string
		
		if cat == "all" {
			text = "📋 All"
		} else if category, exists := m.categories[cat]; exists {
			text = fmt.Sprintf("%s %s", category.Icon, category.Name)
		} else {
			text = strings.Title(cat)
		}
		
		if cat == m.currentCategory {
			style = SelectedStyle
		} else {
			style = CategoryStyle
		}
		
		categoryButtons = append(categoryButtons, style.Render(text))
	}
	
	// Wrap categories to fit within terminal width
	header.WriteString("Categories: ")
	
	maxWidth := m.width - 12 // Account for "Categories: " prefix
	currentLine := ""
	lines := []string{}
	
	for i, button := range categoryButtons {
		testLine := currentLine
		if testLine != "" {
			testLine += " " + button
		} else {
			testLine = button
		}
		
		if lipgloss.Width(testLine) <= maxWidth {
			currentLine = testLine
		} else {
			// Start a new line
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = button
		}
		
		// Add the last line
		if i == len(categoryButtons)-1 && currentLine != "" {
			lines = append(lines, currentLine)
		}
	}
	
	// Join lines with proper indentation
	for i, line := range lines {
		if i == 0 {
			header.WriteString(line)
		} else {
			header.WriteString("\n            ") // Indent to align with "Categories: "
			header.WriteString(line)
		}
	}
	
	// Search bar
	if m.searchMode {
		header.WriteString("\n")
		header.WriteString(SearchStyle.Render("Search: "))
		header.WriteString(m.searchInput.View())
	} else if m.searchQuery != "" {
		header.WriteString(fmt.Sprintf(" | 🔍 Search: %s", m.searchQuery))
	}
	
	return header.String()
}

// renderModelList renders the filtered model list
func (m ModelManagerView) renderModelList() string {
	if len(m.filteredModels) == 0 {
		if m.searchQuery != "" {
			return "\nNo models found matching your search criteria."
		}
		return "\nNo models available in this category."
	}
	
	var content strings.Builder
	visibleHeight := m.height - 6 // Account for header and help
	if m.showDetails {
		visibleHeight = (m.height - 6) * 2 / 3
	}
	
	// Calculate visible range
	start := 0
	end := len(m.filteredModels)
	
	if end > visibleHeight {
		// Center the selected item
		start = m.selectedIndex - visibleHeight/2
		if start < 0 {
			start = 0
		}
		end = start + visibleHeight
		if end > len(m.filteredModels) {
			end = len(m.filteredModels)
			start = end - visibleHeight
			if start < 0 {
				start = 0
			}
		}
	}
	
	for i := start; i < end; i++ {
		item := m.filteredModels[i]
		
		var line string
		if i == m.selectedIndex {
			line = SelectedStyle.Render(fmt.Sprintf("▶ %s", item.Title()))
		} else {
			line = fmt.Sprintf("  %s", item.Title())
		}
		
		content.WriteString(line)
		if i < end-1 {
			content.WriteString("\n")
		}
		
		// Add description if selected
		if i == m.selectedIndex && item.Description() != "" {
			content.WriteString("\n")
			content.WriteString(fmt.Sprintf("    %s", item.Description()))
		}
	}
	
	return content.String()
}

// fuzzyMatch implements simple fuzzy matching
func fuzzyMatch(text, pattern string) bool {
	if pattern == "" {
		return true
	}
	
	textRunes := []rune(text)
	patternRunes := []rune(pattern)
	
	if len(patternRunes) > len(textRunes) {
		return false
	}
	
	textIndex := 0
	for _, patternRune := range patternRunes {
		found := false
		for textIndex < len(textRunes) {
			if textRunes[textIndex] == patternRune {
				found = true
				textIndex++
				break
			}
			textIndex++
		}
		if !found {
			return false
		}
	}
	
	return true
}