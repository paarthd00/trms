package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	
	"trms/internal/models"
	"trms/internal/services"
)

// ProgressExtras stores additional progress information
type ProgressExtras struct {
	Speed string
	ETA   string
}

// ModelManagerView handles model management UI
type ModelManagerView struct {
	list        list.Model
	viewport    viewport.Model
	width       int
	height      int
	focused     bool
	showDetails bool
	modelStates map[string]*services.ModelStatus
	progressExtras map[string]ProgressExtras // Speed and ETA data
	
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

	// Status icon based on state
	var status string
	switch i.State {
	case services.ModelStateComplete:
		if i.IsCurrent {
			status = "●" // Current model indicator - clean dot
		} else {
			status = "✓" // Clean checkmark
		}
	case services.ModelStateDownloading:
		status = "↓"
	case services.ModelStatePartial:
		status = "⚠"
	case services.ModelStateCorrupted:
		status = "✗"
	default:
		status = "○"
	}

	// Size info 
	sizeInfo := formatBytes(i.Size)

	return fmt.Sprintf("%s %-25s %s", status, i.Name, sizeInfo)
}

func (i ModelManagerItem) Description() string {
	if i.IsHeader || i.IsSeparator {
		return ""
	}

	switch i.State {
	case services.ModelStateComplete:
		if i.IsCurrent {
			return "Currently active model • Press 'i' to reinstall"
		} else {
			return "Ready to use • Press 'i' to reinstall"
		}
	case services.ModelStateDownloading:
		if i.Percent > 0 {
			progressBar := renderProgressBar(i.Percent, 30)
			speedETA := ""
			if i.Speed != "" && i.ETA != "" {
				speedETA = fmt.Sprintf(" • %s • ETA: %s", i.Speed, i.ETA)
			} else if i.Speed != "" {
				speedETA = fmt.Sprintf(" • %s", i.Speed)
			}
			return fmt.Sprintf("Downloading %s %d%%%s", progressBar, i.Percent, speedETA)
		}
		return "Starting download..."
	case services.ModelStatePartial:
		if i.Percent > 0 {
			progressBar := renderProgressBar(i.Percent, 30)
			return fmt.Sprintf("Partial %s %d%% • Press 'r' to resume or 'c' to clean", progressBar, i.Percent)
		}
		return "Partial download • Press 'r' to resume or 'c' to clean"
	case services.ModelStateCorrupted:
		return fmt.Sprintf("Corrupted: %s • Press 'c' to clean", i.Error)
	default:
		return "Available for download • Press 'i' to install"
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
		key.WithKeys("i"),
		key.WithHelp("i", "install/download"),
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
		key.WithKeys("enter"),
		key.WithHelp("Enter", "show info"),
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
				m.navigateUp()
			case key.Matches(msg, modelManagerKeys.Down):
				m.navigateDown()
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
		item := &m.filteredModels[m.selectedIndex]
		// Safety check: don't return headers or separators
		if item.IsHeader || item.IsSeparator {
			return nil
		}
		return item
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
				helpItems = append(helpItems, "i: Install/Download", "Enter: Info")
			case services.ModelStateComplete:
				helpItems = append(helpItems, "d: Delete", "Enter: Info")
			case services.ModelStatePartial:
				helpItems = append(helpItems, "r: Resume", "c: Clean", "Enter: Info")
			case services.ModelStateCorrupted:
				helpItems = append(helpItems, "c: Clean", "Enter: Info")
			case services.ModelStateDownloading:
				helpItems = append(helpItems, "Ctrl+C: Cancel", "Enter: Info")
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
	
	// Organize models into categories by state
	var downloadingModels []ModelManagerItem
	var partialModels []ModelManagerItem
	var installedImageModels []ModelManagerItem
	var installedRegularModels []ModelManagerItem
	
	// First, process all downloaded/downloading models from modelStates
	if m.modelStates != nil {
		for name, status := range m.modelStates {
			// Apply fuzzy search filter
			if m.searchQuery != "" && !fuzzyMatch(strings.ToLower(name), strings.ToLower(m.searchQuery)) {
				continue
			}
			
			// For category filtering, check if this model belongs to the category
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
				// Get speed and ETA from progress extras
				var speed, eta string
				if extras, exists := m.progressExtras[name]; exists {
					speed = extras.Speed
					eta = extras.ETA
				}
				
				item := ModelManagerItem{
					Name:       name,
					State:      status.State,
					Size:       status.Size,
					Downloaded: status.Downloaded,
					Percent:    status.Percent,
					Error:      status.Error,
					Speed:      speed,
					ETA:        eta,
				}
				
				// Categorize by state
				switch status.State {
				case services.ModelStateDownloading:
					downloadingModels = append(downloadingModels, item)
				case services.ModelStatePartial:
					partialModels = append(partialModels, item)
				case services.ModelStateComplete:
					if isImageGenerationModel(name) {
						installedImageModels = append(installedImageModels, item)
					} else {
						installedRegularModels = append(installedRegularModels, item)
					}
				}
				
				processedModels[name] = true
			}
		}
	}
	
	// Then process available models from JSON that haven't been downloaded
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
	
	// Convert available models to items and apply search filter
	var availableModels []ModelManagerItem
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
		
		availableModels = append(availableModels, item)
	}
	
	// Build the final list with clear sections
	
	// 1. Active downloads (highest priority)
	if len(downloadingModels) > 0 {
		allItems = append(allItems, ModelManagerItem{
			Name:     fmt.Sprintf("📥 DOWNLOADING (%d)", len(downloadingModels)),
			IsHeader: true,
		})
		allItems = append(allItems, downloadingModels...)
		allItems = append(allItems, ModelManagerItem{IsSeparator: true})
	}
	
	// 2. Partial downloads
	if len(partialModels) > 0 {
		allItems = append(allItems, ModelManagerItem{
			Name:     fmt.Sprintf("⚠️ PARTIAL DOWNLOADS (%d)", len(partialModels)),
			IsHeader: true,
		})
		allItems = append(allItems, partialModels...)
		allItems = append(allItems, ModelManagerItem{IsSeparator: true})
	}
	
	// 3. Installed image generation models (priority section)
	if len(installedImageModels) > 0 {
		allItems = append(allItems, ModelManagerItem{
			Name:     fmt.Sprintf("🎨 IMAGE GENERATION MODELS (%d)", len(installedImageModels)),
			IsHeader: true,
		})
		allItems = append(allItems, installedImageModels...)
		allItems = append(allItems, ModelManagerItem{IsSeparator: true})
	}
	
	// 4. Other installed models
	if len(installedRegularModels) > 0 {
		allItems = append(allItems, ModelManagerItem{
			Name:     fmt.Sprintf("✅ INSTALLED MODELS (%d)", len(installedRegularModels)),
			IsHeader: true,
		})
		allItems = append(allItems, installedRegularModels...)
		allItems = append(allItems, ModelManagerItem{IsSeparator: true})
	}
	
	// 5. Available models (image generation first, then by category)
	if len(availableModels) > 0 {
		// Separate image generation models from regular models
		var availableImageModels []ModelManagerItem
		var availableRegularModels []ModelManagerItem
		
		for _, item := range availableModels {
			if isImageGenerationModel(item.Name) {
				availableImageModels = append(availableImageModels, item)
			} else {
				availableRegularModels = append(availableRegularModels, item)
			}
		}
		
		// Show available image generation models first
		if len(availableImageModels) > 0 {
			allItems = append(allItems, ModelManagerItem{
				Name:     fmt.Sprintf("🎨 AVAILABLE IMAGE MODELS (%d)", len(availableImageModels)),
				IsHeader: true,
			})
			allItems = append(allItems, availableImageModels...)
			allItems = append(allItems, ModelManagerItem{IsSeparator: true})
		}
		
		// Then show other available models
		if len(availableRegularModels) > 0 {
			if m.currentCategory == "all" {
				// Group by category when showing all
				categoryGroups := make(map[string][]ModelManagerItem)
				
				for _, item := range availableRegularModels {
					// Find the category for this model
					category := "specialized" // default
					for _, model := range m.allModels {
						if model.Name == item.Name && len(model.Tags) > 0 {
							category = model.Tags[0]
							break
						}
					}
					categoryGroups[category] = append(categoryGroups[category], item)
				}
				
				// Add models by category order
				categoryOrder := []string{"general", "small", "coding", "vision", "math", "creative", "multilingual", "embedding", "specialized"}
				for _, categoryKey := range categoryOrder {
					if items, exists := categoryGroups[categoryKey]; exists && len(items) > 0 {
						if category, catExists := m.categories[categoryKey]; catExists {
							allItems = append(allItems, ModelManagerItem{
								Name:     fmt.Sprintf("%s %s (%d)", category.Icon, strings.ToUpper(category.Name), len(items)),
								IsHeader: true,
							})
						}
						allItems = append(allItems, items...)
						allItems = append(allItems, ModelManagerItem{IsSeparator: true})
					}
				}
			} else {
				// Show as a single section when filtering by category
				if category, exists := m.categories[m.currentCategory]; exists {
					allItems = append(allItems, ModelManagerItem{
						Name:     fmt.Sprintf("%s AVAILABLE %s (%d)", category.Icon, strings.ToUpper(category.Name), len(availableRegularModels)),
						IsHeader: true,
					})
				} else {
					allItems = append(allItems, ModelManagerItem{
						Name:     fmt.Sprintf("📦 AVAILABLE MODELS (%d)", len(availableRegularModels)),
						IsHeader: true,
					})
				}
				allItems = append(allItems, availableRegularModels...)
				allItems = append(allItems, ModelManagerItem{IsSeparator: true})
			}
		}
	}
	
	// Remove trailing separator
	if len(allItems) > 0 && allItems[len(allItems)-1].IsSeparator {
		allItems = allItems[:len(allItems)-1]
	}
	
	m.filteredModels = allItems
	
	// Reset selection if out of bounds
	if m.selectedIndex >= len(m.filteredModels) {
		m.selectedIndex = 0
	}
	if len(m.filteredModels) == 0 {
		m.selectedIndex = 0
		return
	}
	
	// Ensure we start on a selectable item (not header or separator)
	if m.selectedIndex < len(m.filteredModels) && 
		(m.filteredModels[m.selectedIndex].IsHeader || m.filteredModels[m.selectedIndex].IsSeparator) {
		// Find the first selectable item
		for i := 0; i < len(m.filteredModels); i++ {
			if !m.filteredModels[i].IsHeader && !m.filteredModels[i].IsSeparator {
				m.selectedIndex = i
				return
			}
		}
		// If no selectable items found, keep current selection
	}
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
	m.UpdateProgressWithStats(modelName, percent, downloaded, total, "", "")
}

// UpdateProgressWithStats updates the progress for a specific model with speed and ETA
func (m *ModelManagerView) UpdateProgressWithStats(modelName string, percent int, downloaded, total, speed, eta string) {
	// Update or create the model states map entry
	if m.modelStates == nil {
		m.modelStates = make(map[string]*services.ModelStatus)
	}
	
	if status, exists := m.modelStates[modelName]; exists {
		status.Percent = percent
		status.State = services.ModelStateDownloading
		if downloaded != "" {
			status.Downloaded = parseSize(downloaded)
		}
		if total != "" {
			status.Size = parseSize(total)
		}
	} else {
		// Create new status entry for models that aren't in the map yet
		downloadedBytes := int64(0)
		totalBytes := int64(0)
		if downloaded != "" {
			downloadedBytes = parseSize(downloaded)
		}
		if total != "" {
			totalBytes = parseSize(total)
		}
		
		m.modelStates[modelName] = &services.ModelStatus{
			State:      services.ModelStateDownloading,
			Percent:    percent,
			Downloaded: downloadedBytes,
			Size:       totalBytes,
		}
	}
	
	// Store speed and ETA separately for this model
	if m.progressExtras == nil {
		m.progressExtras = make(map[string]ProgressExtras)
	}
	m.progressExtras[modelName] = ProgressExtras{
		Speed: speed,
		ETA:   eta,
	}
	
	// Update filtered models
	m.updateFilteredModels()
}

// SetModelDownloading marks a model as starting to download
func (m *ModelManagerView) SetModelDownloading(modelName string) {
	// Update or create the model states map entry
	if m.modelStates == nil {
		m.modelStates = make(map[string]*services.ModelStatus)
	}
	
	if status, exists := m.modelStates[modelName]; exists {
		status.State = services.ModelStateDownloading
		status.Percent = 0
	} else {
		// Find the model size from allModels for initial display
		var modelSize int64 = 0
		for _, model := range m.allModels {
			if model.Name == modelName {
				modelSize = parseSize(model.Size)
				break
			}
		}
		
		// Create new status entry for models that aren't in the map yet
		m.modelStates[modelName] = &services.ModelStatus{
			State:      services.ModelStateDownloading,
			Percent:    0,
			Downloaded: 0,
			Size:       modelSize,
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
		
		// Add description if selected OR if it's downloading/partial (for progress bars)
		if (i == m.selectedIndex || item.State == services.ModelStateDownloading || item.State == services.ModelStatePartial) && item.Description() != "" {
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

// renderProgressBar renders a visual progress bar
func renderProgressBar(percent int, width int) string {
	if percent < 0 {
		percent = 0
	} else if percent > 100 {
		percent = 100
	}
	
	filled := int(float64(width) * float64(percent) / 100.0)
	empty := width - filled
	
	bar := "["
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}
	bar += "]"
	
	return bar
}

// isImageGenerationModel checks if a model is for image generation
func isImageGenerationModel(modelName string) bool {
	imageModels := []string{
		"stable-diffusion",
		"flux",
		"flux-schnell", 
		"sdxl",
		"playground-v2.5",
		"dreamshaper",
	}
	
	modelLower := strings.ToLower(modelName)
	for _, imgModel := range imageModels {
		if strings.Contains(modelLower, strings.ToLower(imgModel)) {
			return true
		}
	}
	return false
}

// navigateUp moves selection up, skipping headers and separators
func (m *ModelManagerView) navigateUp() {
	if len(m.filteredModels) == 0 {
		return
	}
	
	// Start from current position and move up
	newIndex := m.selectedIndex - 1
	
	// Find the previous selectable item
	for newIndex >= 0 {
		if !m.filteredModels[newIndex].IsHeader && !m.filteredModels[newIndex].IsSeparator {
			m.selectedIndex = newIndex
			return
		}
		newIndex--
	}
	
	// If we can't find a selectable item above, wrap to the bottom
	for i := len(m.filteredModels) - 1; i > m.selectedIndex; i-- {
		if !m.filteredModels[i].IsHeader && !m.filteredModels[i].IsSeparator {
			m.selectedIndex = i
			return
		}
	}
}

// navigateDown moves selection down, skipping headers and separators
func (m *ModelManagerView) navigateDown() {
	if len(m.filteredModels) == 0 {
		return
	}
	
	// Start from current position and move down
	newIndex := m.selectedIndex + 1
	
	// Find the next selectable item
	for newIndex < len(m.filteredModels) {
		if !m.filteredModels[newIndex].IsHeader && !m.filteredModels[newIndex].IsSeparator {
			m.selectedIndex = newIndex
			return
		}
		newIndex++
	}
	
	// If we can't find a selectable item below, wrap to the top
	for i := 0; i < m.selectedIndex; i++ {
		if !m.filteredModels[i].IsHeader && !m.filteredModels[i].IsSeparator {
			m.selectedIndex = i
			return
		}
	}
}