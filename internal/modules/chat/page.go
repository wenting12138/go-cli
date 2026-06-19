package chat

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/config"
	"go-cli/internal/modules/chat/domain"
	chatui "go-cli/internal/modules/chat/ui"
	"go-cli/internal/modules/shared"
)

const PageName = "chat"

var (
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("86")).
			PaddingLeft(1)
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("240")).
			PaddingLeft(1)
)

type Page struct {
	messages         []chatui.Message
	systemPrompt     string
	modelMessages    []domain.Message
	model            config.ModelConfig
	sessionID        string
	streamIndex      int
	thinkingIndex    int
	modelStreamIndex int
	streamRequestID  string
	cachedWidth      int
	cachedContent    string
	frozenRendered   []string
	frozenWidth      int
	dirty            bool
}

func NewPage(model config.ModelConfig) *Page {
	return &Page{
		model:            model,
		streamIndex:      -1,
		thinkingIndex:    -1,
		modelStreamIndex: -1,
		dirty:            true,
		messages: []chatui.Message{
			chatui.TextMessage(chatui.TypeSystem, startupLogo()),
		},
	}
}

func (p *Page) Title() string {
	return "Go CLI Chat"
}

func (p *Page) Placeholder() string {
	return "Type a message or use /resume or /models"
}

func (p *Page) Content(width int) string {
	if !p.dirty && p.cachedWidth == width {
		return p.cachedContent
	}

	if p.frozenWidth != width || len(p.frozenRendered) != len(p.messages) {
		p.rebuildFrozenRendered(width)
	}

	var lines []string
	for i, message := range p.messages {
		if i > 0 && needsReplyDivider(p.messages[i-1], message) {
			lines = append(lines, renderReplyDivider(width))
		}
		lines = append(lines, p.renderMessage(width, i, message))
	}
	p.cachedWidth = width
	p.cachedContent = strings.Join(lines, "\n\n")
	p.dirty = false
	return p.cachedContent
}

func (p *Page) HandleInput(text string) tea.Cmd {
	if p.sessionID == "" {
		p.sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeUser, text))
	p.modelMessages = append(p.modelMessages, domain.TextMessage("user", text))
	p.markDirty()
	return nil
}

func (p *Page) ApplyUpdate(msg shared.PageUpdatedMsg) {
	message, ok := msg.Payload.(chatui.Message)
	if !ok {
		return
	}
	p.messages = append(p.messages, message)
	p.markDirty()
}

func (p *Page) SystemMessage(text string) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, text))
	p.markDirty()
}

func (p *Page) LoadSession(session domain.Session) {
	p.sessionID = session.ID
	p.messages = append([]chatui.Message{
		chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Resumed session %s: %s", session.ID, session.Title)),
	}, toUIMessages(session.Messages)...)
	p.markDirty()
}

func (p *Page) RestoreSession(sessionID, title string, uiMessages []chatui.Message, modelMessages []domain.Message) {
	p.sessionID = sessionID
	p.messages = append([]chatui.Message{
		chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Resumed session %s: %s", sessionID, title)),
	}, chatui.CloneMessages(uiMessages)...)
	p.modelMessages = cloneMessages(modelMessages)
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) SetSystemPrompt(systemPrompt string) {
	p.systemPrompt = strings.TrimSpace(systemPrompt)
}

func (p *Page) SystemPrompt() string {
	return p.systemPrompt
}

func (p *Page) SetModel(model config.ModelConfig) {
	p.model = model
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Switched model to %s (%s/%s), context=%s.", model.ID, model.Provider, model.Name, formatContextWindow(model.ContextWindow))))
	p.markDirty()
}

func (p *Page) AppendStream(chunk string) {
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		return
	}
	p.messages[p.streamIndex].Content += chunk
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].Content += chunk
	}
	p.markDirty()
}

func (p *Page) FinishStream() {
	if p.thinkingIndex >= 0 && p.thinkingIndex < len(p.messages) {
		p.messages[p.thinkingIndex].Content = strings.TrimSpace(p.messages[p.thinkingIndex].Content)
	}
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		p.streamIndex = -1
		p.thinkingIndex = -1
		p.modelStreamIndex = -1
		p.streamRequestID = ""
		return
	}
	p.messages[p.streamIndex].Content = strings.TrimSpace(p.messages[p.streamIndex].Content)
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].Content = strings.TrimSpace(p.modelMessages[p.modelStreamIndex].Content)
	}
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) FailStream(err string) {
	if p.thinkingIndex >= 0 && p.thinkingIndex < len(p.messages) && strings.TrimSpace(p.messages[p.thinkingIndex].Content) == "" {
		p.messages = append(p.messages[:p.thinkingIndex], p.messages[p.thinkingIndex+1:]...)
		if p.streamIndex > p.thinkingIndex {
			p.streamIndex--
		}
	}
	if p.streamIndex >= 0 && p.streamIndex < len(p.messages) && p.messages[p.streamIndex].Content == "" {
		p.messages = append(p.messages[:p.streamIndex], p.messages[p.streamIndex+1:]...)
	}
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, err))
	p.markDirty()
}

func (p *Page) SnapshotMessages() []domain.Message {
	return cloneMessages(p.modelMessages)
}

func (p *Page) UISnapshotMessages() []chatui.Message {
	return chatui.CloneMessages(p.messages)
}

func (p *Page) BeginRequest(requestID string) {
	p.streamRequestID = requestID
	p.streamIndex = -1
	p.thinkingIndex = -1
}

func (p *Page) BeginAssistantStream(requestID string) {
	if p.streamRequestID == "" {
		p.streamRequestID = requestID
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeAssistant, ""))
	p.frozenRendered = append(p.frozenRendered, "")
	p.streamIndex = len(p.messages) - 1
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		p.modelMessages = append(p.modelMessages, domain.TextMessage("assistant", ""))
		p.modelStreamIndex = len(p.modelMessages) - 1
	}
	p.streamRequestID = requestID
	p.markDirty()
}

func (p *Page) HasActiveRequest(requestID string) bool {
	return p.streamRequestID != "" && p.streamRequestID == requestID
}

func (p *Page) HasActiveStream(requestID string) bool {
	return p.HasActiveRequest(requestID) && p.streamIndex >= 0
}

func (p *Page) HasPendingStream() bool {
	return p.streamRequestID != ""
}

func (p *Page) ActiveRequestID() string {
	return p.streamRequestID
}

func (p *Page) SessionID() string {
	return p.sessionID
}

func (p *Page) AppendToolCall(toolCallID, toolName string, args map[string]any) {
	p.messages = append(p.messages, chatui.ToolMessage(toolCallID, toolName, formatToolCall(toolName, args), buildToolMeta(toolName, args, "")))
	p.modelMessages = append(p.modelMessages, domain.ToolCallMessage(toolCallID, toolName, args))
	p.markDirty()
}

func (p *Page) AppendToolResult(toolCallID, toolName, result string) {
	p.modelMessages = append(p.modelMessages, domain.ToolResultMessage(toolCallID, toolName, result))
	p.applyToolResultToUI(toolCallID, toolName, result)
	p.markDirty()
}

func (p *Page) ApplyCompact(boundary, summary domain.Message, restoredFiles, preservedTail []domain.Message) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, boundary.Content))
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, "Compaction summary saved to model context."))
	p.modelMessages = append([]domain.Message{boundary, summary}, cloneMessages(restoredFiles)...)
	p.modelMessages = append(p.modelMessages, cloneMessages(preservedTail)...)
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) renderMessage(width int, index int, message chatui.Message) string {
	contentWidth := max(20, width)
	content := lipgloss.NewStyle().Width(contentWidth).Render(message.Content)
	if message.Type == chatui.TypeAssistant {
		renderWidth := max(1, contentWidth-assistantStyle.GetHorizontalFrameSize())
		if index == p.streamIndex {
			content = assistantStyle.Width(renderWidth).Render(message.Content)
		} else if index < len(p.frozenRendered) && p.frozenRendered[index] != "" {
			content = assistantStyle.Width(renderWidth).Render(p.frozenRendered[index])
		} else {
			content = assistantStyle.Width(renderWidth).Render(renderAssistantMarkdown(renderWidth, message.Content))
		}
	}
	if message.Type == chatui.TypeThinking {
		if index == p.thinkingIndex {
			content = thinkingStyle.Width(max(1, contentWidth-thinkingStyle.GetHorizontalFrameSize())).Render(message.Content)
		} else {
			content = thinkingBoxStyle.Width(max(1, contentWidth-thinkingBoxStyle.GetHorizontalFrameSize())).Render(renderThinkingCollapsed(contentWidth, message.Content))
		}
	}
	if message.Type == chatui.TypeTool {
		content = toolBodyStyle.Width(max(1, contentWidth-toolBodyStyle.GetHorizontalFrameSize())).Render(renderToolCard(contentWidth, message))
	}
	if message.Type == chatui.TypeUser {
		content = userStyle.Width(max(1, contentWidth-userStyle.GetHorizontalFrameSize())).Render(message.Content)
	}
	if message.Type == chatui.TypeSystem {
		content = systemStyle.Width(max(1, contentWidth-systemStyle.GetHorizontalFrameSize())).Render(message.Content)
	}

	return content
}

func (p *Page) markDirty() {
	p.dirty = true
}
