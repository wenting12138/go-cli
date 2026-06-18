package app

import (
	"fmt"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	agenttools "go-cli/internal/agent/tools"
	"go-cli/internal/config"
	"go-cli/internal/eventbus"
	"go-cli/internal/modules/chat"
	"go-cli/internal/modules/models"
	"go-cli/internal/modules/resume"
	"go-cli/internal/modules/shared"
	toolspage "go-cli/internal/modules/tools"
	chatservice "go-cli/internal/services/chat"
	"go-cli/internal/sessionstore"
)

func NewModel(cfg config.State) Model {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Type a message or use /resume or /models or /tools"
	input.Focus()
	input.CharLimit = 280

	activeModel, _ := cfg.ActiveModel()
	store, _ := sessionstore.New()
	summaries := []sessionstore.Summary{}
	if store != nil {
		summaries, _ = store.List()
	}
	bus := eventbus.New()
	pages := map[pageID]shared.Page{
		pageChat:   chat.NewPage(activeModel),
		pageModels: models.NewPage(cfg.Config.Models, cfg.Config.CurrentModel),
		pageResume: resume.NewPage(summaries),
		pageTools:  toolspage.NewPage(agenttools.Specs(activeModel)),
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
		focus:    focusInput,
		store:    store,
	}
}

func Run() error {
	cfg, err := config.Ensure()
	if err != nil {
		return err
	}

	program := tea.NewProgram(NewModel(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = program.Run()
	return err
}

func (m Model) Init() tea.Cmd {
	m.syncViewport()
	return tea.Batch(textinput.Blink, waitEventCmd(m.events))
}

func (m *Model) syncViewport() {
	page := m.currentPage()
	if page == nil {
		return
	}

	currentOffset := m.viewport.YOffset
	wasAtBottom := m.viewport.AtBottom()

	m.input.Placeholder = page.Placeholder()
	m.updateSlashMenu()
	m.viewport.SetContent(page.Content(m.viewport.Width))
	if m.current == pageChat && wasAtBottom {
		m.viewport.GotoBottom()
		return
	}

	maxOffset := m.viewport.TotalLineCount() - m.viewport.Height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if currentOffset > maxOffset {
		currentOffset = maxOffset
	}
	if currentOffset < 0 {
		currentOffset = 0
	}
	m.viewport.YOffset = currentOffset
}

func (m *Model) updateLayout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	contentWidth := m.contentWidth()
	m.input.Width = max(10, contentWidth-4)
	m.viewport.Width = max(18, contentWidth-2)

	headerHeight := lipgloss.Height(m.renderHeader())
	inputHeight := lipgloss.Height(m.renderBottomPanel())
	slashHeight := m.renderSlashMenuHeight(contentWidth)
	statusHeight := lipgloss.Height(m.renderModelStatus())
	helpHeight := lipgloss.Height(helpStyle.Render(m.help.View(m.keys)))
	viewportHeight := max(5, m.height-headerHeight-inputHeight-slashHeight-statusHeight-helpHeight-4)
	m.viewport.Height = viewportHeight

	if modelsPage, ok := m.pages[pageModels].(*models.Page); ok {
		modelsPage.SetSize(contentWidth, viewportHeight)
	}
	if resumePage, ok := m.pages[pageResume].(*resume.Page); ok {
		resumePage.SetSize(contentWidth, viewportHeight)
	}
}

func (m Model) currentPage() shared.Page {
	return m.pages[m.current]
}

func (m Model) contentWidth() int {
	return max(20, m.width-4)
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

func waitEventCmd(events <-chan any) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return nil
		}
		return eventMsg{event: event}
	}
}
