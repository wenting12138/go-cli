package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/config"
	"go-cli/internal/modules/chat"
	"go-cli/internal/modules/models"
	"go-cli/internal/modules/resume"
	"go-cli/internal/modules/shared"
)

type pageID string

const (
	pageChat   pageID = "chat"
	pageModels pageID = "models"
	pageResume pageID = "resume"
)

type keyMap struct {
	Send key.Binding
	Back key.Binding
	Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Send, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Send, k.Back},
		{k.Quit},
	}
}

var keys = keyMap{
	Send: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back to chat"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

var (
	appStyle    = lipgloss.NewStyle().Padding(1, 2)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
)

type Model struct {
	viewport viewport.Model
	input    textinput.Model
	help     help.Model
	keys     keyMap
	width    int
	height   int
	current  pageID
	pages    map[pageID]shared.Page
	config   config.State
}

func NewModel(cfg config.State) Model {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Type a message or use /resume or /models"
	input.Focus()
	input.CharLimit = 280

	activeModel, _ := cfg.ActiveModel()
	pages := map[pageID]shared.Page{
		pageChat:   chat.NewPage(activeModel),
		pageModels: models.NewPage(cfg.Config.Models, cfg.Config.CurrentModel),
		pageResume: resume.NewPage(),
	}

	return Model{
		viewport: viewport.New(0, 0),
		input:    input,
		help:     help.New(),
		keys:     keys,
		current:  pageChat,
		pages:    pages,
		config:   cfg,
	}
}

func Run() error {
	cfg, err := config.Ensure()
	if err != nil {
		return err
	}

	program := tea.NewProgram(NewModel(cfg), tea.WithAltScreen())
	_, err = program.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	m.syncViewport()
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

		contentWidth := max(20, msg.Width-4)
		headerHeight := lipgloss.Height(m.renderHeader())
		inputHeight := 3
		helpHeight := 1
		viewportHeight := max(5, msg.Height-headerHeight-inputHeight-helpHeight-4)

		m.viewport.Width = contentWidth
		m.viewport.Height = viewportHeight
		m.input.Width = max(10, contentWidth-4)
		m.syncViewport()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Back):
			if m.current != pageChat {
				m.current = pageChat
				m.input.Placeholder = m.currentPage().Placeholder()
				m.syncViewport()
			}
			return m, nil
		case key.Matches(msg, m.keys.Send):
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}

			m.input.SetValue("")
			if strings.HasPrefix(text, "/") {
				return m.handleSlashCommand(text)
			}

			page := m.currentPage()
			cmd := page.HandleInput(text)
			m.syncViewport()
			return m, cmd
		}
	case shared.PageUpdatedMsg:
		if page, ok := m.pages[pageID(msg.Page)]; ok {
			page.ApplyUpdate(msg)
		}
		m.syncViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.viewport.View(),
		inputBoxStyle.Width(m.viewport.Width).Render(m.input.View()),
		helpStyle.Render(m.help.View(m.keys)),
	)

	return appStyle.Render(body)
}

func (m *Model) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(strings.TrimPrefix(text, "/"))
	if len(parts) == 0 {
		return m, nil
	}

	switch parts[0] {
	case "chat":
		m.current = pageChat
		m.input.Placeholder = m.currentPage().Placeholder()
		m.syncViewport()
		return m, nil
	case "models":
		if len(parts) == 1 {
			m.current = pageModels
			m.input.Placeholder = m.currentPage().Placeholder()
			m.syncViewport()
			return m, nil
		}
		return m.handleModelByID(parts[1])
	case "resume":
		if len(parts) == 1 {
			m.current = pageResume
			m.input.Placeholder = m.currentPage().Placeholder()
			m.syncViewport()
			return m, nil
		}
		return m.handleResumeByID(parts[1])
	case "clear":
		activeModel, err := m.config.ActiveModel()
		if err != nil {
			m.currentPage().SystemMessage(err.Error())
			m.syncViewport()
			return m, nil
		}
		m.pages[pageChat] = chat.NewPage(activeModel)
		if m.current == pageChat {
			m.input.Placeholder = m.currentPage().Placeholder()
		}
		m.syncViewport()
		return m, nil
	default:
		if page := m.currentPage(); page != nil {
			page.SystemMessage(fmt.Sprintf("Unknown command: /%s\nAvailable commands: /chat /models /models <id> /resume /resume <id> /clear", parts[0]))
		}
		m.syncViewport()
		return m, nil
	}
}

func (m *Model) handleModelByID(id string) (tea.Model, tea.Cmd) {
	model, err := m.config.SelectModel(id)
	if err != nil {
		m.currentPage().SystemMessage(fmt.Sprintf("%s\nUse /models to view available models.", err.Error()))
		m.syncViewport()
		return m, nil
	}

	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if ok {
		chatPage.SetModel(model)
	}
	modelsPage, ok := m.pages[pageModels].(*models.Page)
	if ok {
		modelsPage.SetCurrentModel(id)
	}

	m.current = pageChat
	m.input.Placeholder = m.currentPage().Placeholder()
	m.syncViewport()
	return m, nil
}

func (m *Model) handleResumeByID(id string) (tea.Model, tea.Cmd) {
	resumePage, ok := m.pages[pageResume].(*resume.Page)
	if !ok {
		return m, nil
	}

	session, found := resumePage.SessionByID(id)
	if !found {
		m.currentPage().SystemMessage(fmt.Sprintf("Session not found: %s\nUse /resume to view available sessions.", id))
		m.syncViewport()
		return m, nil
	}

	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return m, nil
	}

	chatPage.LoadSession(session)
	resumePage.MarkResumed(session.ID)
	m.current = pageChat
	m.input.Placeholder = m.currentPage().Placeholder()
	m.syncViewport()
	return m, nil
}

func (m *Model) syncViewport() {
	page := m.currentPage()
	if page == nil {
		return
	}
	m.input.Placeholder = page.Placeholder()
	m.viewport.SetContent(page.Content(m.viewport.Width))
	m.viewport.GotoTop()
}

func (m Model) renderHeader() string {
	title := "Go CLI"
	if page := m.currentPage(); page != nil {
		title = page.Title()
	}
	return headerStyle.Width(max(20, m.width-4)).Render(" " + title + " ")
}

func (m Model) currentPage() shared.Page {
	return m.pages[m.current]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
