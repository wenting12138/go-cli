package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	agenttools "go-cli/internal/agent/tools"
	"go-cli/internal/agentmode"
	"go-cli/internal/events"
	"go-cli/internal/modules/chat"
	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/modules/models"
	"go-cli/internal/modules/resume"
	"go-cli/internal/modules/shared"
	toolspage "go-cli/internal/modules/tools"
	"go-cli/internal/sessionstore"
)

const autoCompactThresholdRatio = 0.70
const autoCompactMinimumMessages = 8

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.updateLayout()
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
	m.updateLayout()
	m.syncViewport()
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
	case m.current == pageResume:
		return m.handleResumeKey(msg)
	case m.shouldShowSlashMenu():
		if handledModel, handledCmd, ok := m.handleSlashMenuKey(msg); ok {
			return handledModel, handledCmd
		}
	case m.focus == focusViewport:
		if key.Matches(msg, m.keys.ToggleTool) {
			if chatPage, ok := m.pages[pageChat].(*chat.Page); ok && m.current == pageChat {
				if chatPage.ToggleLastToolCard() {
					m.persistActiveChat()
					m.syncViewport()
					return m, nil
				}
			}
		}
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
	m.updateLayout()
	m.syncViewport()
	return m, cmd
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.current == pageChat && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if chatPage, ok := m.pages[pageChat].(*chat.Page); ok {
			if line, ok := m.viewportLineAtMouse(msg); ok {
				switch chatPage.ToolActionAtLine(line) {
				case chat.ToolActionPlanApprove:
					m.approvePlan()
					m.persistActiveChat()
					m.syncViewport()
					return m, nil
				case chat.ToolActionPlanReject:
					if m.pendingPlanApproval {
						m.beginRejectFeedback()
						m.syncViewport()
						return m, nil
					}
				}
				if chatPage.ToggleToolCardAtLine(line) {
					m.persistActiveChat()
					m.syncViewport()
					return m, nil
				}
			}
		}
	}
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
	if m.shouldShowSlashMenu() {
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

func (m Model) handleResumeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	resumePage, ok := m.pages[pageResume].(*resume.Page)
	if !ok {
		return m, nil
	}
	if key.Matches(msg, m.keys.Send) {
		return m.handleResumeByID(resumePage.SelectedSessionID())
	}
	cmd := resumePage.Update(msg)
	m.syncViewport()
	return m, cmd
}

func (m Model) handleSlashMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	total := len(m.currentSlashItems())
	switch msg.String() {
	case "up":
		m.slash.move(-1, total)
		return m, nil, true
	case "down":
		m.slash.move(1, total)
		return m, nil, true
	case "tab":
		if completion, ok := m.selectedSlashCompletion(); ok {
			m.input.SetValue(completion + " ")
			m.input.CursorEnd()
			m.slash.hide()
			return m, nil, true
		}
	case "enter":
		if completion, ok := m.selectedSlashCompletion(); ok {
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
	if m.rejectFeedbackMode {
		m.rejectPlan(text)
		m.persistActiveChat()
		m.syncViewport()
		return m, nil
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
	activeModel, err := m.config.ActiveModel()
	if err != nil {
		chatPage.FailStream(err.Error())
		m.syncViewport()
		return m, nil
	}
	systemPrompt, err := m.chatSvc.BuildSystemPrompt(activeModel, m.mode)
	if err == nil {
		chatPage.SetSystemPrompt(systemPrompt)
	}
	if m.mode == agentmode.Chat && shouldAutoCompact(chatPage.SystemPrompt(), chatPage.SnapshotMessages(), activeModel.ContextWindow) {
		result, compactErr := m.chatSvc.Compact(activeModel, chatPage.SystemPrompt(), chatPage.SnapshotMessages())
		if compactErr == nil {
			chatPage.ApplyCompact(result.Boundary, result.Summary, result.RestoredFiles, result.PreservedTail)
			chatPage.SystemMessage("Auto compacted context before sending.")
		} else {
			chatPage.SystemMessage(fmt.Sprintf("Auto compact skipped: %s", compactErr))
		}
	}
	m.persistActiveChat()
	if err := m.chatSvc.StartStream(activeModel, m.mode, chatPage.SnapshotMessages()); err != nil {
		chatPage.FailStream(fmt.Sprintf("model request failed: %s", err))
		m.syncViewport()
		return m, nil
	}
	m.syncViewport()
	return m, nil
}

func shouldAutoCompact(systemPrompt string, messages []domain.Message, contextWindow int) bool {
	if contextWindow <= 0 || len(messages) < autoCompactMinimumMessages {
		return false
	}
	used := estimateContextUsage(systemPrompt, messages)
	return float64(used) >= float64(contextWindow)*autoCompactThresholdRatio
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
	case "plan":
		m.current = pageChat
		m.enterPlanMode()
		m.input.Placeholder = m.currentPage().Placeholder()
		if len(parts) == 1 {
			m.showCurrentPlan()
			m.persistActiveChat()
			m.syncViewport()
			return m, nil
		}
		return m.handleChatInput(strings.Join(parts[1:], " "))
	case "approve":
		if !m.pendingPlanApproval {
			if page := m.currentPage(); page != nil {
				page.SystemMessage("No plan is waiting for approval.")
			}
			m.syncViewport()
			return m, nil
		}
		m.approvePlan()
		m.persistActiveChat()
		m.syncViewport()
		return m, nil
	case "reject":
		if !m.pendingPlanApproval && !m.rejectFeedbackMode {
			if page := m.currentPage(); page != nil {
				page.SystemMessage("No plan is waiting for rejection.")
			}
			m.syncViewport()
			return m, nil
		}
		m.current = pageChat
		feedback := strings.TrimSpace(strings.Join(parts[1:], " "))
		if feedback == "" {
			m.beginRejectFeedback()
			m.input.Placeholder = m.currentPage().Placeholder()
			m.syncViewport()
			return m, nil
		}
		m.rejectPlan(feedback)
		m.persistActiveChat()
		m.syncViewport()
		return m, nil
	case "cancel":
		if m.rejectFeedbackMode {
			m.cancelRejectFeedback()
			m.syncViewport()
			return m, nil
		}
		if page := m.currentPage(); page != nil {
			page.SystemMessage("Nothing to cancel.")
		}
		m.persistActiveChat()
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
	case "compact":
		return m.handleCompactCommand()
	case "clear":
		return m.handleClearCommand()
	default:
		if page := m.currentPage(); page != nil {
			page.SystemMessage(fmt.Sprintf("Unknown command: /%s\nAvailable commands: /chat /plan /approve /reject [feedback] /cancel /models /models <id> /resume /resume <id> /tools /compact /clear", parts[0]))
		}
		m.syncViewport()
		return m, nil
	}
}

func (m *Model) handleCompactCommand() (tea.Model, tea.Cmd) {
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return m, nil
	}
	if chatPage.HasPendingStream() {
		chatPage.SystemMessage("wait for the current response to finish")
		m.syncViewport()
		return m, nil
	}

	activeModel, err := m.config.ActiveModel()
	if err != nil {
		chatPage.SystemMessage(err.Error())
		m.syncViewport()
		return m, nil
	}

	if chatPage.SystemPrompt() == "" {
		systemPrompt, promptErr := m.chatSvc.BuildSystemPrompt(activeModel, m.mode)
		if promptErr == nil {
			chatPage.SetSystemPrompt(systemPrompt)
		}
	}

	result, err := m.chatSvc.Compact(activeModel, chatPage.SystemPrompt(), chatPage.SnapshotMessages())
	if err != nil {
		chatPage.SystemMessage(err.Error())
		m.syncViewport()
		return m, nil
	}

	chatPage.ApplyCompact(result.Boundary, result.Summary, result.RestoredFiles, result.PreservedTail)
	m.persistActiveChat()
	m.syncViewport()
	return m, nil
}

func (m *Model) handleClearCommand() (tea.Model, tea.Cmd) {
	activeModel, err := m.config.ActiveModel()
	if err != nil {
		m.currentPage().SystemMessage(err.Error())
		m.syncViewport()
		return m, nil
	}
	m.pages[pageChat] = chat.NewPage(activeModel)
	m.mode = agentmode.Chat
	m.prePlanMode = ""
	m.pendingPlanApproval = false
	m.rejectFeedbackMode = false
	if m.current == pageChat {
		m.input.Placeholder = m.currentPage().Placeholder()
	}
	m.refreshToolSpecs()
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
		*toolsPage = *toolspage.NewPage(agenttools.Specs(model, m.mode))
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

	if !resumePage.HasSession(id) {
		m.currentPage().SystemMessage(fmt.Sprintf("Session not found: %s\nUse /resume to view available sessions.", id))
		m.syncViewport()
		return m, nil
	}
	if m.store == nil {
		m.currentPage().SystemMessage("session store unavailable")
		m.syncViewport()
		return m, nil
	}

	session, err := m.store.Load(id)
	if err != nil {
		m.currentPage().SystemMessage(err.Error())
		m.syncViewport()
		return m, nil
	}

	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return m, nil
	}

	chatPage.RestoreSession(session.ID, session.Title, session.UIMessages, session.ModelMessages)
	chatPage.SetSystemPrompt(session.SystemPrompt)
	m.mode = session.Mode
	if m.mode == "" {
		m.mode = agentmode.Chat
	}
	m.prePlanMode = session.PrePlanMode
	m.pendingPlanApproval = false
	m.rejectFeedbackMode = false
	m.refreshToolSpecs()
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
		chatPage.BeginRequest(e.RequestID)
	case events.ChatThinkingStarted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.BeginThinking(e.RequestID)
		}
	case events.ChatThinkingChunk:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.AppendThinking(e.Chunk)
			chatPage.AppendReasoning(e.Chunk)
		}
	case events.ChatThinkingSignature:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.SetReasoningSignature(e.Signature)
		}
	case events.ChatResponseStarted:
		chatPage.BeginAssistantStream(e.RequestID)
	case events.ChatStreamChunk:
		if chatPage.HasActiveStream(e.RequestID) {
			chatPage.AppendStream(e.Chunk)
		}
	case events.ChatToolCallStarted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.AppendToolCall(e.ToolCallID, e.Tool, e.Args)
			if e.Tool == "exit_plan_mode" {
				m.markPlanReady()
			}
			m.persistActiveChat()
		}
	case events.ChatToolCallCompleted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.AppendToolResult(e.ToolCallID, e.Tool, e.Result)
			m.persistActiveChat()
		}
	case events.ChatStreamFailed:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.FailStream(e.Err)
			m.persistActiveChat()
		}
	case events.ChatStreamCompleted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.FinishStream()
			m.saveCurrentPlan()
			m.persistActiveChat()
		}
	}
}

func (m *Model) persistActiveChat() {
	if m.store == nil {
		return
	}

	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok || chatPage.SessionID() == "" {
		return
	}

	_ = m.store.Save(sessionstore.SessionData{
		ID:            chatPage.SessionID(),
		Title:         chatPage.SessionTitle(),
		UIMessages:    chatPage.UISnapshotMessages(),
		SystemPrompt:  chatPage.SystemPrompt(),
		ModelMessages: chatPage.SnapshotMessages(),
		Mode:          m.mode,
		PrePlanMode:   m.prePlanMode,
		PlanPath:      m.planPath(chatPage.SessionID()),
		UpdatedAt:     time.Now(),
	})
	m.refreshResumePage()
}

func (m *Model) refreshResumePage() {
	if m.store == nil {
		return
	}
	resumePage, ok := m.pages[pageResume].(*resume.Page)
	if !ok {
		return
	}
	summaries, err := m.store.List()
	if err != nil {
		resumePage.SystemMessage(err.Error())
		return
	}
	resumePage.SetSessions(summaries)
}
