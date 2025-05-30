package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
)

type Mode int

const (
	InputMode Mode = iota
	SearchMode
	AIMode
	CommandMode
	ModelSelectMode
	InstallMode
)

type SearchResult struct {
	title       string
	url         string
	description string
}

func (s SearchResult) Title() string       { return s.title }
func (s SearchResult) Description() string { return s.description }
func (s SearchResult) FilterValue() string { return s.title }

type ModelItem struct {
	name     string
	size     string
	installed bool
}

func (m ModelItem) Title() string {
	status := "📦"
	if m.installed {
		status = "✅"
	}
	return fmt.Sprintf("%s %s", status, m.name)
}

func (m ModelItem) Description() string {
	if m.installed {
		return fmt.Sprintf("Installed (%s)", m.size)
	}
	return "Not installed - press Enter to pull"
}

func (m ModelItem) FilterValue() string { return m.name }

type model struct {
	mode           Mode
	input          textinput.Model
	viewport       viewport.Model
	searchResults  []SearchResult
	list           list.Model
	modelList      list.Model
	width          int
	height         int
	commandOutput  string
	err            error
	quitting       bool
	ollama         *OllamaManager
}

type commandFinishedMsg struct {
	output string
	err    error
}

type searchFinishedMsg struct {
	results []SearchResult
	err     error
}

type aiResponseMsg struct {
	response string
	err      error
}

type installOllamaMsg struct{}

type ollamaInstalledMsg struct {
	err error
}

type modelPulledMsg struct {
	model string
	err   error
}

type modelsRefreshedMsg struct {
	err error
}

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
)

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "Enter command..."
	ti.Focus()
	ti.CharLimit = 256

	vp := viewport.New(80, 20)
	vp.SetContent("")

	listItems := []list.Item{}
	l := list.New(listItems, list.NewDefaultDelegate(), 80, 20)
	l.Title = "Search Results"
	l.SetShowHelp(false)

	modelListItems := []list.Item{}
	ml := list.New(modelListItems, list.NewDefaultDelegate(), 80, 20)
	ml.Title = "Ollama Models"
	ml.SetShowHelp(false)

	return model{
		mode:      InputMode,
		input:     ti,
		viewport:  vp,
		list:      l,
		modelList: ml,
		ollama:    NewOllamaManager(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
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
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 4
		m.list.SetSize(msg.Width, msg.Height-4)
		m.modelList.SetSize(msg.Width, msg.Height-4)

	case tea.KeyMsg:
		// Handle Ollama installation prompt
		if m.mode == InstallMode {
			content := m.viewport.View()
			if strings.Contains(content, "install Ollama") {
				if msg.String() == "y" || msg.String() == "Y" {
					m.viewport.SetContent("Installing Ollama... This may take a few minutes.")
					return m, m.installOllama()
				} else if msg.String() == "n" || msg.String() == "N" {
					m.mode = InputMode
					m.viewport.SetContent("")
					return m, nil
				}
			}
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyCtrlP:
			if m.mode == AIMode {
				return m, m.refreshModels()
			}

		case tea.KeyEsc:
			if m.mode != InputMode {
				m.mode = InputMode
				m.input.Reset()
				m.input.Focus()
				m.input.Placeholder = "Enter command..."
				return m, nil
			}

		case tea.KeyEnter:
			switch m.mode {
			case InputMode:
				input := m.input.Value()
				m.input.Reset()

				switch {
				case input == ":s" || strings.HasPrefix(input, "search "):
					m.mode = SearchMode
					if strings.HasPrefix(input, "search ") {
						query := strings.TrimPrefix(input, "search ")
						return m, m.performSearch(query)
					}
					m.input.Placeholder = "Enter search query..."
					return m, nil

				case input == ":ai" || strings.HasPrefix(input, "ai "):
					if !m.ollama.IsRunning() {
						m.viewport.SetContent("Ollama is not running. Trying to start it...")
						return m, m.startOllama()
					}
					
					m.mode = AIMode
					if strings.HasPrefix(input, "ai ") {
						query := strings.TrimPrefix(input, "ai ")
						m.viewport.SetContent("Thinking...")
						return m, m.performAI(query)
					}
					m.input.Placeholder = fmt.Sprintf("Chat with %s...", m.ollama.GetCurrentModel())
					return m, nil

				case input == ":models":
					return m, m.refreshModels()

				case input == ":q" || input == "quit":
					m.quitting = true
					return m, tea.Quit

				default:
					m.mode = CommandMode
					return m, m.executeCommand(input)
				}

			case SearchMode:
				if m.list.FilterState() == list.Unfiltered && m.list.SelectedItem() != nil {
					// Selecting a search result
					if i, ok := m.list.SelectedItem().(SearchResult); ok {
						browser.OpenURL(i.url)
					}
				} else {
					// Performing a new search
					query := m.input.Value()
					m.input.Reset()
					return m, m.performSearch(query)
				}

			case AIMode:
				prompt := m.input.Value()
				if prompt == "" {
					return m, nil
				}
				m.input.Reset()
				m.viewport.SetContent("Thinking...")
				return m, m.performAI(prompt)

			case ModelSelectMode:
				if item, ok := m.modelList.SelectedItem().(ModelItem); ok {
					if !item.installed {
						m.viewport.SetContent(fmt.Sprintf("Pulling %s model...", item.name))
						return m, m.pullModel(item.name)
					} else {
						m.ollama.SetCurrentModel(item.name)
						m.mode = AIMode
						m.input.Placeholder = fmt.Sprintf("Chat with %s...", item.name)
					}
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
		m.mode = InputMode
		m.input.Placeholder = "Enter command..."

	case searchFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.viewport.SetContent(fmt.Sprintf("Search error: %v", msg.err))
			m.mode = InputMode
		} else {
			items := make([]list.Item, len(msg.results))
			for i, r := range msg.results {
				items[i] = r
			}
			m.list.SetItems(items)
			m.searchResults = msg.results
		}

	case aiResponseMsg:
		if msg.err != nil {
			m.err = msg.err
			m.viewport.SetContent(fmt.Sprintf("AI error: %v", msg.err))
		} else {
			m.viewport.SetContent(msg.response)
		}
		m.viewport.GotoTop()
		m.mode = AIMode
		m.input.Placeholder = fmt.Sprintf("Chat with %s...", m.ollama.GetCurrentModel())
		m.input.Focus()

	case ollamaInstalledMsg:
		if msg.err != nil {
			m.viewport.SetContent(fmt.Sprintf("Failed to install Ollama: %v", msg.err))
			m.mode = InputMode
		} else {
			m.viewport.SetContent("Ollama installed successfully! Starting service...")
			return m, m.startOllama()
		}

	case modelPulledMsg:
		if msg.err != nil {
			m.viewport.SetContent(fmt.Sprintf("Failed to pull model: %v", msg.err))
		} else {
			m.ollama.SetCurrentModel(msg.model)
			m.viewport.SetContent(fmt.Sprintf("Model %s pulled successfully!", msg.model))
			return m, m.refreshModels()
		}

	case modelsRefreshedMsg:
		if msg.err == nil {
			m.updateModelList()
		}
		if m.mode != ModelSelectMode {
			m.mode = AIMode
			m.input.Placeholder = fmt.Sprintf("Chat with %s...", m.ollama.GetCurrentModel())
		}
	}

	// Update components based on mode
	switch m.mode {
	case InputMode, SearchMode, AIMode:
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)
		if m.mode == AIMode {
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	case CommandMode, InstallMode:
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	case ModelSelectMode:
		m.modelList, cmd = m.modelList.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.mode == SearchMode && len(m.searchResults) > 0 {
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if m.quitting {
		return "Thanks for using Trm Search!\n"
	}

	var s strings.Builder

	title := titleStyle.Render("🔍 Trm Search - Terminal Search & Local AI")
	s.WriteString(title + "\n")

	switch m.mode {
	case InputMode:
		help := helpStyle.Render("Commands: :s (search) | :ai (AI chat) | :models (manage models) | :q (quit) | or type any shell command")
		s.WriteString(m.input.View() + "\n")
		s.WriteString(help + "\n")
		if m.commandOutput != "" {
			s.WriteString("\n" + m.viewport.View())
		}

	case SearchMode:
		if len(m.searchResults) == 0 {
			s.WriteString(m.input.View() + "\n")
			s.WriteString(helpStyle.Render("Enter search query and press Enter (ESC to go back)") + "\n")
		} else {
			s.WriteString(m.list.View() + "\n")
			s.WriteString(helpStyle.Render("↑/↓: Navigate | Enter: Open in browser | /: Filter | ESC: Back") + "\n")
		}

	case AIMode:
		s.WriteString(modelStyle.Render(fmt.Sprintf("Model: %s", m.ollama.GetCurrentModel())) + "\n")
		s.WriteString(m.input.View() + "\n")
		s.WriteString(helpStyle.Render("Enter prompt (ESC: back | Ctrl+P: change model)") + "\n")
		
		if m.viewport.TotalLineCount() > 0 {
			s.WriteString("\n" + m.viewport.View())
		}

	case ModelSelectMode:
		s.WriteString(m.modelList.View() + "\n")
		s.WriteString(helpStyle.Render("↑/↓: Navigate | Enter: Select/Pull model | ESC: Back") + "\n")

	case CommandMode:
		s.WriteString("Executing command...\n")
		s.WriteString(m.viewport.View())

	case InstallMode:
		s.WriteString(m.viewport.View() + "\n")
	}

	if m.err != nil {
		s.WriteString(fmt.Sprintf("\nError: %v\n", m.err))
	}

	return s.String()
}

func (m *model) checkOllama() tea.Cmd {
	return func() tea.Msg {
		if !m.ollama.IsInstalled() {
			m.mode = InstallMode
			m.viewport.SetContent("Ollama is not installed. Would you like to install Ollama now? (y/n)")
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
			response: "Ollama started successfully! You can now chat.",
			err:      nil,
		}
	}
}

func (m *model) refreshModels() tea.Cmd {
	return func() tea.Msg {
		m.mode = ModelSelectMode
		err := m.ollama.RefreshModels()
		return modelsRefreshedMsg{err: err}
	}
}

func (m *model) updateModelList() {
	installedModels := m.ollama.GetModels()
	installedMap := make(map[string]string)
	
	for _, model := range installedModels {
		installedMap[model.Name] = model.Size
	}
	
	items := []list.Item{}
	
	// Show installed models first
	for _, model := range installedModels {
		items = append(items, ModelItem{
			name:      model.Name,
			size:      model.Size,
			installed: true,
		})
	}
	
	// Show popular models that aren't installed
	for _, modelName := range PopularModels {
		if _, exists := installedMap[modelName]; !exists {
			items = append(items, ModelItem{
				name:      modelName,
				size:      "N/A",
				installed: false,
			})
		}
	}
	
	m.modelList.SetItems(items)
}

func (m *model) pullModel(modelName string) tea.Cmd {
	return func() tea.Msg {
		err := m.ollama.PullModel(modelName)
		return modelPulledMsg{
			model: modelName,
			err:   err,
		}
	}
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

func (m *model) performSearch(query string) tea.Cmd {
	return func() tea.Msg {
		results, err := duckDuckGoSearch(query)
		return searchFinishedMsg{
			results: results,
			err:     err,
		}
	}
}

func (m *model) performAI(prompt string) tea.Cmd {
	return func() tea.Msg {
		response, err := m.ollama.Chat(prompt)
		return aiResponseMsg{
			response: response,
			err:      err,
		}
	}
}

func duckDuckGoSearch(query string) ([]SearchResult, error) {
	escapedQuery := url.QueryEscape(query)
	searchURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", escapedQuery)

	resp, err := http.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		RelatedTopics []interface{} `json:"RelatedTopics"`
		Results []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Results"`
		AbstractText string `json:"AbstractText"`
		AbstractURL  string `json:"AbstractURL"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	var results []SearchResult

	if data.AbstractText != "" && data.AbstractURL != "" {
		results = append(results, SearchResult{
			title:       "Summary",
			url:         data.AbstractURL,
			description: data.AbstractText,
		})
	}

	for _, r := range data.Results {
		if r.Text != "" && r.FirstURL != "" {
			results = append(results, SearchResult{
				title:       r.Text,
				url:         r.FirstURL,
				description: r.Text,
			})
		}
	}

	for _, topic := range data.RelatedTopics {
		if topicMap, ok := topic.(map[string]interface{}); ok {
			if text, ok := topicMap["Text"].(string); ok && text != "" {
				if url, ok := topicMap["FirstURL"].(string); ok && url != "" {
					results = append(results, SearchResult{
						title:       text,
						url:         url,
						description: text,
					})
					if len(results) >= 10 {
						break
					}
				}
			}
		}
	}

	if len(results) == 0 {
		results = append(results, SearchResult{
			title:       "No results found",
			url:         fmt.Sprintf("https://duckduckgo.com/?q=%s", escapedQuery),
			description: "Try searching on DuckDuckGo directly",
		})
	}

	return results, nil
}

func main() {
	flag.Parse()

	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		log.Fatal(err)
	}
}