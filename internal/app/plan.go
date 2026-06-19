package app

import (
	"fmt"
	"strings"

	agenttools "go-cli/internal/agent/tools"
	"go-cli/internal/agentmode"
	"go-cli/internal/modules/chat"
	toolspage "go-cli/internal/modules/tools"
)

func (m *Model) enterPlanMode() {
	if m.mode == agentmode.Plan {
		return
	}
	m.prePlanMode = m.mode
	m.mode = agentmode.Plan
	m.pendingPlanApproval = false
	m.rejectFeedbackMode = false
	m.refreshToolSpecs()
	if chatPage, ok := m.pages[pageChat].(*chat.Page); ok {
		sessionID := chatPage.EnsureSessionID()
		path := m.planPath(sessionID)
		chatPage.SystemMessage(fmt.Sprintf("Plan mode enabled. Read-only tools active. Plan file: %s", path))
	}
}

func (m *Model) exitPlanMode() {
	if m.mode != agentmode.Plan {
		return
	}
	next := m.prePlanMode
	if next == "" {
		next = agentmode.Chat
	}
	m.mode = next
	m.prePlanMode = ""
	m.pendingPlanApproval = false
	m.rejectFeedbackMode = false
	m.refreshToolSpecs()
	if chatPage, ok := m.pages[pageChat].(*chat.Page); ok {
		chatPage.SystemMessage("Plan mode exited. Returned to chat mode.")
	}
}

func (m *Model) approvePlan() {
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if ok && m.planStore != nil {
		sessionID := chatPage.SessionID()
		if sessionID != "" {
			path, content, err := m.loadPlan(sessionID)
			if err == nil {
				chatPage.AppendApprovedPlan(path, content)
			} else {
				chatPage.SystemMessage(err.Error())
			}
		}
	}
	m.exitPlanMode()
}

func (m *Model) showCurrentPlan() {
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return
	}
	sessionID := chatPage.EnsureSessionID()
	path, content, err := m.loadPlan(sessionID)
	if err != nil {
		chatPage.SystemMessage(err.Error())
		return
	}
	if strings.TrimSpace(content) == "" {
		chatPage.SystemMessage(fmt.Sprintf("No plan saved yet. Plan file: %s", path))
		return
	}
	chatPage.SystemMessage(fmt.Sprintf("Current plan (%s):\n\n%s", path, strings.TrimSpace(content)))
}

func (m *Model) refreshToolSpecs() {
	model, err := m.config.ActiveModel()
	if err != nil {
		return
	}
	if toolsPage, ok := m.pages[pageTools].(*toolspage.Page); ok {
		*toolsPage = *toolspage.NewPage(agenttools.Specs(model, m.mode))
	}
}

func (m *Model) saveCurrentPlan() {
	if m.mode != agentmode.Plan || m.planStore == nil {
		return
	}
	chatPage, ok := m.pages[pageChat].(*chat.Page)
	if !ok {
		return
	}
	sessionID := chatPage.SessionID()
	if sessionID == "" {
		return
	}
	messages := chatPage.SnapshotMessages()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(messages[i].Content)
		if content == "" {
			continue
		}
		_, _ = m.planStore.Save(sessionID, content)
		return
	}
}

func (m *Model) markPlanReady() {
	m.pendingPlanApproval = true
	m.rejectFeedbackMode = false
}

func (m *Model) rejectPlan(feedback string) {
	if m.mode != agentmode.Plan {
		return
	}
	m.pendingPlanApproval = false
	m.rejectFeedbackMode = false
	if chatPage, ok := m.pages[pageChat].(*chat.Page); ok {
		if strings.TrimSpace(feedback) == "" {
			chatPage.SystemMessage("Plan rejected. Add feedback and ask for a revised plan.")
			return
		}
		chatPage.HandleInput("Please revise the plan based on this feedback:\n" + strings.TrimSpace(feedback))
	}
}

func (m *Model) beginRejectFeedback() {
	if m.mode != agentmode.Plan || !m.pendingPlanApproval {
		return
	}
	m.rejectFeedbackMode = true
	m.pendingPlanApproval = false
}

func (m *Model) cancelRejectFeedback() {
	if !m.rejectFeedbackMode {
		return
	}
	m.rejectFeedbackMode = false
	m.pendingPlanApproval = true
}

func (m *Model) planPath(sessionID string) string {
	if m.planStore == nil {
		return ""
	}
	return m.planStore.Path(sessionID)
}

func (m *Model) loadPlan(sessionID string) (string, string, error) {
	if m.planStore == nil {
		return "", "", fmt.Errorf("plan store unavailable")
	}
	return m.planStore.Load(sessionID)
}
