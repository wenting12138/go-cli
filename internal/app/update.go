package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"go-cli/internal/agent/react"
	agenttools "go-cli/internal/agent/tools"
	"go-cli/internal/agentmode"
	"go-cli/internal/config"
	"go-cli/internal/events"
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat"
	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/modules/models"
	"go-cli/internal/modules/resume"
	"go-cli/internal/modules/shared"
	skillspage "go-cli/internal/modules/skills"
	toolspage "go-cli/internal/modules/tools"
	"go-cli/internal/sessionstore"
	skillstore "go-cli/internal/skills"
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
		shouldSync := m.handleEvent(msg.event)
		if shouldSync {
			m.syncViewport()
		}
		return m, tea.Batch(waitEventCmd(m.events), m.streamFlushCmd())
	case streamFlushTickMsg:
		if !m.streamFlushPending {
			return m, nil
		}
		m.streamFlushPending = false
		m.syncViewport()
		return m, m.streamFlushCmd()
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
	case msg.String() == "ctrl+c":
		return m, tea.Quit
	case key.Matches(msg, m.keys.Quit) && m.focus != focusInput:
		return m, tea.Quit
	case key.Matches(msg, m.keys.Focus):
		m.toggleFocus()
		return m, nil
	case key.Matches(msg, m.keys.Back):
		if interrupted := m.interruptActiveStream(); interrupted {
			m.syncViewport()
			return m, nil
		}
		return m.handleBackKey()
	case m.current == pageModels:
		return m.handleModelsKey(msg)
	case m.current == pageResume:
		return m.handleResumeKey(msg)
	case m.current == pageSkills:
		return m.handleSkillsKey(msg)
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
			if line, ok := m.viewportLineAtMouse(msg); ok && chatPage.ToggleToolCardAtLine(line) {
				m.persistActiveChat()
				m.syncViewport()
				return m, nil
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
	if modelsPage.IsRenaming() {
		switch msg.String() {
		case "esc":
			modelsPage.CancelRename()
			m.syncViewport()
			return m, nil
		case "enter":
			return m.handleRenameModel(modelsPage.SelectedModelID(), modelsPage.RenameValue())
		}
		cmd := modelsPage.Update(msg)
		m.syncViewport()
		return m, cmd
	}
	if msg.String() == "r" {
		if modelsPage.StartRename() {
			m.syncViewport()
		}
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

func (m Model) handleSkillsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	skillsPage, ok := m.pages[pageSkills].(*skillspage.Page)
	if !ok {
		return m, nil
	}
	if msg.String() == "r" {
		return m.refreshSkillsPage()
	}
	if key.Matches(msg, m.keys.Send) {
		return m.handleSkillByName(skillsPage.SelectedSkillName())
	}
	cmd := skillsPage.Update(msg)
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
	m.lastMetrics = protocol.ResponseMetrics{}
	activeModel, err := m.config.ActiveModel()
	if err != nil {
		chatPage.FailStream(err.Error())
		m.syncViewport()
		return m, nil
	}
	turnSkill := chatPage.TurnSkill()
	if turnSkill == nil {
		if discovered, ok := skillstore.Discover(m.skills, text); ok {
			chatPage.ActivateSkill(discovered)
			chatPage.SystemMessage(fmt.Sprintf("Auto discovered skill %s.", discovered.Name))
			if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok {
				*toolsPage = *toolspage.NewPage(m.currentToolSpecs(activeModel))
			}
			if skillsPage, ok := m.pages[pageSkills].(*skillspage.Page); ok {
				m.syncSkillsPageState(skillsPage)
			}
			turnSkill = chatPage.TurnSkill()
		}
	}
	systemPrompt, err := m.chatSvc.BuildSystemPromptForSkill(activeModel, m.mode, turnSkill)
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
	if err := m.chatSvc.StartStreamWithSkill(activeModel, m.mode, chatPage.SnapshotMessages(), turnSkill); err != nil {
		chatPage.FailStream(fmt.Sprintf("model request failed: %s", err))
		m.syncViewport()
		return m, nil
	}
	chatPage.ConsumePendingForkSkill()
	m.persistActiveChat()
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
	case "skills":
		if len(parts) == 1 {
			m.current = pageSkills
			m.input.Placeholder = m.currentPage().Placeholder()
			m.syncViewport()
			return m, nil
		}
		if len(parts) == 2 && parts[1] == "clear" {
			return m.handleClearSkillCommand()
		}
		return m.handleSkillByName(parts[1])
	case "skill-clear":
		return m.handleClearSkillCommand()
	case "compact":
		return m.handleCompactCommand()
	case "clear":
		return m.handleClearCommand()
	default:
		if model, cmd, ok := m.tryHandleSkillAlias(parts[0]); ok {
			return model, cmd
		}
		if page := m.currentPage(); page != nil {
			page.SystemMessage(fmt.Sprintf("Unknown command: /%s\nAvailable commands: /chat /models /models <id> /resume /resume <id> /tools /skills /skills <name> /skills clear /skill-clear /compact /clear /<skill-name>", parts[0]))
		}
		m.syncViewport()
		return m, nil
	}
}

func (m *Model) handleClearSkillCommand() (tea.Model, tea.Cmd) {
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return m, nil
	}
	if chatPage.HasPendingStream() {
		chatPage.SystemMessage("wait for the current response to finish")
		m.syncViewport()
		return m, nil
	}
	if !chatPage.ClearSkills() {
		chatPage.SystemMessage("no active skill to clear")
		m.syncViewport()
		return m, nil
	}
	if activeModel, activeErr := m.config.ActiveModel(); activeErr == nil {
		if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok {
			*toolsPage = *toolspage.NewPage(m.currentToolSpecs(activeModel))
		}
	}
	if skillsPage, ok := m.pages[pageSkills].(*skillspage.Page); ok {
		m.syncSkillsPageState(skillsPage)
	}
	m.persistActiveChat()
	m.current = pageChat
	m.input.Placeholder = m.currentPage().Placeholder()
	m.syncViewport()
	return m, nil
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
		systemPrompt, promptErr := m.chatSvc.BuildSystemPromptForSkill(activeModel, m.mode, chatPage.TurnSkill())
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
	if m.current == pageChat {
		m.input.Placeholder = m.currentPage().Placeholder()
	}
	if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok {
		*toolsPage = *toolspage.NewPage(m.currentToolSpecs(activeModel))
	}
	if _, ok := m.pages[pageSkills].(*skillspage.Page); ok {
		_, _ = m.refreshSkillsPage()
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
		*toolsPage = *toolspage.NewPage(m.currentToolSpecs(model))
	}

	m.current = pageChat
	m.input.Placeholder = m.currentPage().Placeholder()
	m.syncViewport()
	return m, nil
}

func (m *Model) handleRenameModel(id, name string) (tea.Model, tea.Cmd) {
	modelsPage, ok := m.pages[pageModels].(*models.Page)
	if !ok {
		return m, nil
	}

	model, err := m.config.RenameModel(id, name)
	if err != nil {
		modelsPage.SystemMessage(err.Error())
		m.syncViewport()
		return m, nil
	}

	modelsPage.FinishRename()
	modelsPage.SetModelsSelected(m.config.Config.Models, m.config.Config.CurrentModel, model.ID)
	modelsPage.SystemMessage(fmt.Sprintf("Updated model to id=%s name=%s.", model.ID, model.Name))

	if chatPage, ok := m.pages[pageChat].(*chat.Page); ok && model.ID == m.config.Config.CurrentModel {
		chatPage.RefreshModel(model)
	}
	if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok && model.ID == m.config.Config.CurrentModel {
		*toolsPage = *toolspage.NewPage(m.currentToolSpecs(model))
	}

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

	chatPage.RestoreSessionState(session.ID, session.Title, session.UIMessages, session.ModelMessages, session.ActiveSkill, session.PendingSkill)
	chatPage.SetSystemPrompt(session.SystemPrompt)
	m.mode = agentmode.Chat
	if activeModel, activeErr := m.config.ActiveModel(); activeErr == nil {
		if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok {
			*toolsPage = *toolspage.NewPage(m.currentToolSpecs(activeModel))
		}
	}
	_, _ = m.refreshSkillsPage()
	resumePage.MarkResumed(session.ID)
	m.current = pageChat
	m.input.Placeholder = m.currentPage().Placeholder()
	m.syncViewport()
	return m, nil
}

func (m *Model) handleSkillByName(name string) (tea.Model, tea.Cmd) {
	var (
		skill skillstore.Skill
		found bool
	)
	for _, candidate := range m.skills {
		if strings.EqualFold(candidate.Name, name) {
			skill = candidate
			found = true
			break
		}
	}
	if !found {
		workingDir, err := os.Getwd()
		if err != nil {
			m.currentPage().SystemMessage(err.Error())
			m.syncViewport()
			return m, nil
		}
		loaded, err := skillstore.Find(workingDir, name)
		if err != nil {
			m.currentPage().SystemMessage(fmt.Sprintf("%s\nUse /skills to view available skills.", err.Error()))
			m.syncViewport()
			return m, nil
		}
		skill = loaded
	}

	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return m, nil
	}
	chatPage.ActivateSkill(skill)
	if activeModel, activeErr := m.config.ActiveModel(); activeErr == nil {
		if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok {
			*toolsPage = *toolspage.NewPage(m.currentToolSpecs(activeModel))
		}
	}
	if skillsPage, ok := m.pages[pageSkills].(*skillspage.Page); ok {
		m.syncSkillsPageState(skillsPage)
	}
	m.persistActiveChat()
	m.current = pageChat
	m.input.Placeholder = m.currentPage().Placeholder()
	m.syncViewport()
	return m, nil
}

func (m *Model) refreshSkillsPage() (tea.Model, tea.Cmd) {
	skillsPage, ok := m.pages[pageSkills].(*skillspage.Page)
	if !ok {
		return m, nil
	}
	workingDir, err := os.Getwd()
	if err != nil {
		skillsPage.SystemMessage(err.Error())
		m.syncViewport()
		return m, nil
	}
	skills, err := skillstore.Load(workingDir)
	if err != nil {
		skillsPage.SystemMessage(err.Error())
		m.syncViewport()
		return m, nil
	}
	m.skills = skills
	skillsPage.SetSkills(skills)
	m.syncSkillsPageState(skillsPage)
	skillsPage.SystemMessage(fmt.Sprintf("Reloaded %d skills.", len(skills)))
	m.syncViewport()
	return m, nil
}

func (m *Model) handleEvent(event any) bool {
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return false
	}

	switch e := event.(type) {
	case events.ChatStreamStarted:
		chatPage.BeginRequest(e.RequestID)
		m.streamFlushPending = false
		return true
	case events.ChatAssistantBlocks:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.ApplyAssistantBlocks(e.Blocks)
		}
		return true
	case events.ChatThinkingStarted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.BeginThinking(e.RequestID)
		}
		return true
	case events.ChatThinkingChunk:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.AppendThinking(e.Chunk)
			chatPage.AppendReasoning(e.Chunk)
			m.streamFlushPending = true
		}
		return false
	case events.ChatThinkingSignature:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.SetReasoningSignature(e.Signature)
		}
		return true
	case events.ChatResponseStarted:
		chatPage.BeginAssistantStream(e.RequestID)
		return true
	case events.ChatStreamChunk:
		if chatPage.HasActiveStream(e.RequestID) {
			chatPage.AppendStream(e.Chunk)
			m.streamFlushPending = true
		}
		return false
	case events.ChatToolCallStarted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.AppendToolCall(e.ToolCallID, e.Tool, e.Args)
			m.persistActiveChat()
		}
		return true
	case events.ChatToolCallCompleted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.AppendToolResult(e.ToolCallID, e.Tool, e.Result)
			m.persistActiveChat()
		}
		return true
	case events.ChatStreamFailed:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.FailStream(e.Err)
			m.streamFlushPending = false
			m.persistActiveChat()
		}
		return true
	case events.ChatStreamCompleted:
		if chatPage.HasActiveRequest(e.RequestID) {
			chatPage.FinishStream()
			m.streamFlushPending = false
			m.lastMetrics = e.Metrics
			m.persistActiveChat()
		}
		return true
	}
	return false
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
		ActiveSkill:   chatPage.ActiveSkill(),
		PendingSkill:  chatPage.PendingForkSkill(),
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

func (m *Model) interruptActiveStream() bool {
	if m.current != pageChat {
		return false
	}
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok || !chatPage.HasPendingStream() {
		return false
	}
	requestID := chatPage.ActiveRequestID()
	if requestID == "" {
		return false
	}
	return m.chatSvc.Cancel(requestID)
}

func (m Model) streamFlushCmd() tea.Cmd {
	if !m.streamFlushPending {
		return nil
	}
	return streamFlushTickCmd()
}

func (m *Model) tryHandleSkillAlias(name string) (tea.Model, tea.Cmd, bool) {
	for _, skill := range m.skills {
		if !strings.EqualFold(skill.Name, name) {
			continue
		}
		model, cmd := m.handleSkillByName(skill.Name)
		return model, cmd, true
	}
	return m, nil, false
}

func (m Model) syncSkillsPageState(skillsPage *skillspage.Page) {
	if skillsPage == nil {
		return
	}
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		skillsPage.SetActiveSkills("", "")
		return
	}
	active := ""
	pending := ""
	if skill := chatPage.ActiveSkill(); skill != nil {
		active = skill.Name
	}
	if skill := chatPage.PendingForkSkill(); skill != nil {
		pending = skill.Name
	}
	skillsPage.SetActiveSkills(active, pending)
}

func (m Model) currentToolSpecs(model config.ModelConfig) []react.ToolSpec {
	toolset := agenttools.Build(model, m.mode)
	if chatPage, ok := m.pages[pageChat].(*chat.Page); ok {
		if skill := chatPage.TurnSkill(); skill != nil && len(skill.AllowedTools) > 0 {
			toolset = agenttools.FilterByName(toolset, skill.AllowedTools)
		}
	}
	specs := make([]react.ToolSpec, 0, len(toolset))
	for _, tool := range toolset {
		specs = append(specs, react.ToolSpec{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return specs
}
