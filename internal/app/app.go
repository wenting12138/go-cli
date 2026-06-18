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
	"go-cli/internal/eventbus"
	"go-cli/internal/events"
	"go-cli/internal/modules/chat"
	"go-cli/internal/modules/models"
	"go-cli/internal/modules/resume"
	"go-cli/internal/modules/shared"
	chatservice "go-cli/internal/services/chat"
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
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
	inactiveBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Foreground(lipgloss.Color("242")).
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
	bus      *eventbus.Bus
	events   <-chan any
	chatSvc  *chatservice.Service
	slash    slashMenu
}

func NewModel(cfg config.State) Model {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Type a message or use /resume or /models"
	input.Focus()
	input.CharLimit = 280

	activeModel, _ := cfg.ActiveModel()
	bus := eventbus.New()
	pages := map[pageID]shared.Page{
		pageChat:   chat.NewPage(activeModel),
		pageModels: models.NewPage(cfg.Config.Models, cfg.Config.CurrentModel),
		pageResume: resume.NewPage(),
	}
	events, _ := bus.Subscribe(128)

	return Model{
		viewport: viewport.New(0, 0),
		input:    input,
		help:     help.New(),
		keys:     keys,
		current:  pageChat,
		pages:    pages,
		config:   cfg,
		bus:      bus,
		events:   events,
		chatSvc:  chatservice.New(bus),
		slash:    newSlashMenu(),
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
	return tea.Batch(textinput.Blink, waitEventCmd(m.events))
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
		slashHeight := m.renderSlashMenuHeight(contentWidth)
		statusHeight := 1
		helpHeight := 1
		viewportHeight := max(5, msg.Height-headerHeight-inputHeight-slashHeight-statusHeight-helpHeight-4)

		m.viewport.Width = contentWidth
		m.viewport.Height = viewportHeight
		m.input.Width = max(10, contentWidth-4)
		if modelsPage, ok := m.pages[pageModels].(*models.Page); ok {
			modelsPage.SetSize(contentWidth, viewportHeight)
		}
		m.syncViewport()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Back):
			if m.slash.visible {
				m.slash.hide()
				m.syncViewport()
				return m, nil
			}
			if m.current != pageChat {
				m.current = pageChat
				m.input.Placeholder = m.currentPage().Placeholder()
				m.syncViewport()
			}
			return m, nil
		case m.current == pageModels:
			modelsPage, ok := m.pages[pageModels].(*models.Page)
			if !ok {
				return m, nil
			}
			if key.Matches(msg, m.keys.Send) {
				return m.handleModelByID(modelsPage.SelectedModelID())
			}
			cmd := modelsPage.Update(msg)
			m.syncViewport()
			return m, cmd
		case m.slash.visible:
			switch msg.String() {
			case "up":
				m.slash.move(-1)
				return m, nil
			case "down":
				m.slash.move(1)
				return m, nil
			case "tab":
				if completion, ok := m.slash.selectedCompletion(); ok {
					m.input.SetValue(completion + " ")
					m.input.CursorEnd()
					m.slash.hide()
					return m, nil
				}
			case "enter":
				if completion, ok := m.slash.selectedCompletion(); ok {
					m.input.SetValue("")
					m.slash.hide()
					return m.handleSlashCommand(completion)
				}
			}
		case key.Matches(msg, m.keys.Send):
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}

			m.input.SetValue("")
			if strings.HasPrefix(text, "/") {
				return m.handleSlashCommand(text)
			}

			if m.current == pageChat {
				chatPage, ok := m.pages[pageChat].(*chat.Page)
				if !ok {
					return m, nil
				}
				if chatPage.HasPendingStream() {
					chatPage.SystemMessage("wait for the current response to finish")
					m.syncViewport()
					return m, nil
				}

				chatPage.HandleInput(text)
				messages := chatPage.SnapshotMessages()
				activeModel, err := m.config.ActiveModel()
				if err != nil {
					chatPage.FailStream(err.Error())
					m.syncViewport()
					return m, nil
				}
				if err := m.chatSvc.StartStream(activeModel, messages); err != nil {
					chatPage.FailStream(fmt.Sprintf("model request failed: %s", err))
					m.syncViewport()
					return m, nil
				}
				m.syncViewport()
				return m, nil
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
	case eventMsg:
		m.handleEvent(msg.event)
		m.syncViewport()
		return m, waitEventCmd(m.events)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateSlashMenu()
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	sections := []string{
		m.renderHeader(),
		m.viewport.View(),
		m.renderBottomPanel(),
	}

	if m.current == pageChat {
		sections = append(sections, m.renderSlashMenu())
	}

	sections = append(sections, m.renderModelStatus())
	sections = append(sections, helpStyle.Render(m.help.View(m.keys)))

	body := lipgloss.JoinVertical(lipgloss.Left, sections...)

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
	m.updateSlashMenu()
	m.viewport.SetContent(page.Content(m.viewport.Width))
	m.viewport.GotoTop()
}

func (m *Model) handleEvent(event any) {
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return
	}

	switch e := event.(type) {
	case events.ChatStreamStarted:
	case events.ChatResponseStarted:
		chatPage.BeginAssistantStream(e.RequestID)
	case events.ChatStreamChunk:
		if chatPage.HasActiveStream(e.RequestID) {
			chatPage.AppendStream(e.Chunk)
		}
	case events.ChatToolCallStarted:
		if chatPage.HasActiveStream(e.RequestID) {
			chatPage.AppendToolCall(e.ToolCallID, e.Tool, e.Args)
		}
	case events.ChatToolCallCompleted:
		if chatPage.HasActiveStream(e.RequestID) {
			chatPage.AppendToolResult(e.ToolCallID, e.Tool, e.Result)
		}
	case events.ChatStreamFailed:
		if chatPage.HasActiveStream(e.RequestID) {
			chatPage.FailStream(e.Err)
		}
	case events.ChatStreamCompleted:
		if chatPage.HasActiveStream(e.RequestID) {
			chatPage.FinishStream()
		}
	}
}

func (m Model) renderHeader() string {
	title := "Go CLI"
	if page := m.currentPage(); page != nil {
		title = page.Title()
	}
	return headerStyle.Width(max(20, m.width-4)).Render(" " + title + " ")
}

func (m Model) renderModelStatus() string {
	model, err := m.config.ActiveModel()
	if err != nil {
		return statusStyle.Render("model: unavailable")
	}

	return statusStyle.Render(
		fmt.Sprintf("model: %s  context: %s", model.ID, formatContextWindow(model.ContextWindow)),
	)
}

func (m Model) renderBottomPanel() string {
	width := m.viewport.Width
	if width <= 0 {
		width = max(20, m.width-4)
	}

	if m.current == pageChat {
		return inputBoxStyle.Width(width).Render(m.input.View())
	}

	return inactiveBoxStyle.Width(width).Render("commands are available on the chat page")
}

func (m Model) renderSlashMenu() string {
	if !m.slash.visible || len(m.slash.filtered) == 0 {
		return ""
	}

	width := max(20, m.viewport.Width)
	var lines []string
	for i, item := range m.slash.filtered {
		prefix := "  "
		if i == m.slash.selected {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s", prefix, item.command, item.description))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(lines, "\n"))
}

func (m Model) renderSlashMenuHeight(width int) int {
	if !m.slash.visible || len(m.slash.filtered) == 0 {
		return 0
	}
	return lipgloss.Height(m.renderSlashMenu())
}

func (m Model) currentPage() shared.Page {
	return m.pages[m.current]
}

func formatContextWindow(size int) string {
	if size%1000 == 0 {
		return fmt.Sprintf("%dk", size/1000)
	}
	return fmt.Sprintf("%.1fk", float64(size)/1000)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type eventMsg struct {
	event any
}

func waitEventCmd(events <-chan any) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return eventMsg{event: event}
	}
}

type slashCommand struct {
	command     string
	description string
}

type slashMenu struct {
	commands []slashCommand
	filtered []slashCommand
	visible  bool
	selected int
}

func newSlashMenu() slashMenu {
	commands := []slashCommand{
		{command: "/chat", description: "back to chat page"},
		{command: "/models", description: "open model selector"},
		{command: "/resume", description: "open session resume page"},
		{command: "/clear", description: "clear current chat"},
	}
	return slashMenu{commands: commands}
}

func (m *Model) updateSlashMenu() {
	value := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(value, "/") {
		m.slash.hide()
		return
	}
	if strings.Contains(value, " ") {
		m.slash.hide()
		return
	}

	query := strings.ToLower(value)
	var filtered []slashCommand
	for _, cmd := range m.slash.commands {
		if strings.HasPrefix(strings.ToLower(cmd.command), query) {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		m.slash.hide()
		return
	}

	m.slash.filtered = filtered
	m.slash.visible = true
	if m.slash.selected >= len(filtered) {
		m.slash.selected = 0
	}
}

func (s *slashMenu) move(delta int) {
	if !s.visible || len(s.filtered) == 0 {
		return
	}
	s.selected = (s.selected + delta + len(s.filtered)) % len(s.filtered)
}

func (s *slashMenu) selectedCompletion() (string, bool) {
	if !s.visible || len(s.filtered) == 0 {
		return "", false
	}
	return s.filtered[s.selected].command, true
}

func (s *slashMenu) hide() {
	s.filtered = nil
	s.visible = false
	s.selected = 0
}
