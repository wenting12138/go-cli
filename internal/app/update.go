package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	agenttools "go-cli/internal/agent/tools"
	"go-cli/internal/events"
	"go-cli/internal/modules/chat"
	"go-cli/internal/modules/models"
	"go-cli/internal/modules/resume"
	"go-cli/internal/modules/shared"
	toolspage "go-cli/internal/modules/tools"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

		contentWidth := m.contentWidth()
		m.input.Width = max(10, contentWidth-4)
		m.viewport.Width = max(18, contentWidth-2)
		if modelsPage, ok := m.pages[pageModels].(*models.Page); ok {
			modelsPage.SetSize(contentWidth, m.viewport.Height)
		}

		headerHeight := lipgloss.Height(m.renderHeader())
		inputHeight := lipgloss.Height(m.renderBottomPanel())
		slashHeight := m.renderSlashMenuHeight(contentWidth)
		statusHeight := lipgloss.Height(m.renderModelStatus())
		helpHeight := lipgloss.Height(helpStyle.Render(m.help.View(m.keys)))
		viewportHeight := max(5, msg.Height-headerHeight-inputHeight-slashHeight-statusHeight-helpHeight-4)

		m.viewport.Height = viewportHeight
		if modelsPage, ok := m.pages[pageModels].(*models.Page); ok {
			modelsPage.SetSize(contentWidth, viewportHeight)
		}
		m.syncViewport()
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
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

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Focus):
		m.toggleFocus()
		return m, nil
	case key.Matches(msg, m.keys.Back):
		return m.handleBackKey()
	case m.current == pageModels:
		return m.handleModelsKey(msg)
	case m.slash.visible:
		if handledModel, handledCmd, ok := m.handleSlashMenuKey(msg); ok {
			return handledModel, handledCmd
		}
	case m.focus == focusViewport:
		m.viewport, _ = m.viewport.Update(msg)
		return m, nil
	case key.Matches(msg, m.keys.Send):
		return m.handleSendKey()
	}

	var cmd tea.Cmd
	if m.focus == focusInput {
		m.input, cmd = m.input.Update(msg)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	m.updateSlashMenu()
	return m, cmd
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) toggleFocus() {
	if m.focus == focusInput {
		m.focus = focusViewport
		m.input.Blur()
	} else {
		m.focus = focusInput
		m.input.Focus()
	}
	m.syncViewport()
}

func (m Model) handleBackKey() (tea.Model, tea.Cmd) {
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
}

func (m Model) handleModelsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
}

func (m Model) handleSlashMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.String() {
	case "up":
		m.slash.move(-1)
		return m, nil, true
	case "down":
		m.slash.move(1)
		return m, nil, true
	case "tab":
		if completion, ok := m.slash.selectedCompletion(); ok {
			m.input.SetValue(completion + " ")
			m.input.CursorEnd()
			m.slash.hide()
			return m, nil, true
		}
	case "enter":
		if completion, ok := m.slash.selectedCompletion(); ok {
			m.input.SetValue("")
			m.slash.hide()
			model, cmd := m.handleSlashCommand(completion)
			return model, cmd, true
		}
	}
	return m, nil, false
}

func (m Model) handleSendKey() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}

	m.input.SetValue("")
	if strings.HasPrefix(text, "/") {
		return m.handleSlashCommand(text)
	}

	if m.current == pageChat {
		return m.handleChatInput(text)
	}

	page := m.currentPage()
	cmd := page.HandleInput(text)
	m.syncViewport()
	return m, cmd
}

func (m Model) handleChatInput(text string) (tea.Model, tea.Cmd) {
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
	case "tools":
		m.current = pageTools
		m.input.Placeholder = m.currentPage().Placeholder()
		m.syncViewport()
		return m, nil
	case "clear":
		return m.handleClearCommand()
	default:
		if page := m.currentPage(); page != nil {
			page.SystemMessage(fmt.Sprintf("Unknown command: /%s\nAvailable commands: /chat /models /models <id> /resume /resume <id> /tools /clear", parts[0]))
		}
		m.syncViewport()
		return m, nil
	}
}

func (m *Model) handleClearCommand() (tea.Model, tea.Cmd) {
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
}

func (m *Model) handleModelByID(id string) (tea.Model, tea.Cmd) {
	model, err := m.config.SelectModel(id)
	if err != nil {
		m.currentPage().SystemMessage(fmt.Sprintf("%s\nUse /models to view available models.", err.Error()))
		m.syncViewport()
		return m, nil
	}

	if chatPage, ok := m.pages[pageChat].(*chat.Page); ok {
		chatPage.SetModel(model)
	}
	if modelsPage, ok := m.pages[pageModels].(*models.Page); ok {
		modelsPage.SetCurrentModel(id)
	}
	if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok {
		*toolsPage = *toolspage.NewPage(agenttools.Specs(model))
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
