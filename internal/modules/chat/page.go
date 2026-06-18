package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/glamour/v2"
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
	assistantStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(lipgloss.Color("63")).
			PaddingLeft(1).
			PaddingLeft(1)
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("240")).
			PaddingLeft(1)
	toolLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)
	toolBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
)

type Page struct {
	messages         []chatui.Message
	systemPrompt     string
	modelMessages    []domain.Message
	model            config.ModelConfig
	sessionID        string
	streamIndex      int
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
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		p.streamIndex = -1
		p.modelStreamIndex = -1
		p.streamRequestID = ""
		return
	}
	p.messages[p.streamIndex].Content = strings.TrimSpace(p.messages[p.streamIndex].Content)
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].Content = strings.TrimSpace(p.modelMessages[p.modelStreamIndex].Content)
	}
	p.streamIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) FailStream(err string) {
	if p.streamIndex >= 0 && p.streamIndex < len(p.messages) && p.messages[p.streamIndex].Content == "" {
		p.messages = append(p.messages[:p.streamIndex], p.messages[p.streamIndex+1:]...)
	}
	p.streamIndex = -1
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
}

func (p *Page) BeginAssistantStream(requestID string) {
	if p.streamRequestID == "" {
		p.streamRequestID = requestID
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeAssistant, ""))
	p.modelMessages = append(p.modelMessages, domain.TextMessage("assistant", ""))
	p.frozenRendered = append(p.frozenRendered, "")
	p.streamIndex = len(p.messages) - 1
	p.modelStreamIndex = len(p.modelMessages) - 1
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
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeTool, formatToolCall(toolCallID, toolName, args)))
	p.modelMessages = append(p.modelMessages, domain.ToolCallMessage(toolCallID, toolName, args))
	p.markDirty()
}

func (p *Page) AppendToolResult(toolCallID, toolName, result string) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeTool, formatToolResult(toolCallID, toolName, result)))
	p.modelMessages = append(p.modelMessages, domain.ToolResultMessage(toolCallID, toolName, result))
	p.markDirty()
}

func (p *Page) ApplyCompact(boundary, summary domain.Message, preservedTail []domain.Message) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, boundary.Content))
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, "Compaction summary saved to model context."))
	p.modelMessages = append([]domain.Message{boundary, summary}, cloneMessages(preservedTail)...)
	p.streamIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) renderMessage(width int, index int, message chatui.Message) string {
	contentWidth := max(20, width)
	content := lipgloss.NewStyle().Width(contentWidth).Render(message.Content)
	if message.Type == chatui.TypeAssistant {
		if index == p.streamIndex {
			content = assistantStyle.Width(contentWidth - assistantStyle.GetHorizontalFrameSize()).Render(message.Content)
		} else if index < len(p.frozenRendered) && p.frozenRendered[index] != "" {
			content = assistantStyle.Width(contentWidth - assistantStyle.GetHorizontalFrameSize()).Render(p.frozenRendered[index])
		} else {
			content = assistantStyle.Width(contentWidth - assistantStyle.GetHorizontalFrameSize()).Render(renderAssistantMarkdown(contentWidth, message.Content))
		}
	}
	if message.Type == chatui.TypeTool {
		content = toolBodyStyle.Width(max(1, contentWidth-toolBodyStyle.GetHorizontalFrameSize())).Render(message.Content)
	}
	if message.Type == chatui.TypeUser {
		content = userStyle.Width(max(1, contentWidth-userStyle.GetHorizontalFrameSize())).Render(message.Content)
	}
	if message.Type == chatui.TypeSystem {
		content = systemStyle.Width(max(1, contentWidth-systemStyle.GetHorizontalFrameSize())).Render(message.Content)
	}

	return content
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatContextWindow(size int) string {
	if size%1000 == 0 {
		return fmt.Sprintf("%dk", size/1000)
	}
	return fmt.Sprintf("%.1fk", float64(size)/1000)
}

func formatArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func formatToolCall(toolCallID, toolName string, args map[string]any) string {
	return strings.Join([]string{
		fmt.Sprintf("call: %s", toolName),
		fmt.Sprintf("id: %s", toolCallID),
		fmt.Sprintf("args: %s", formatArgs(args)),
	}, "\n")
}

func formatToolResult(toolCallID, toolName, result string) string {
	return strings.Join([]string{
		fmt.Sprintf("result: %s", toolName),
		fmt.Sprintf("id: %s", toolCallID),
		fmt.Sprintf("content: %s", result),
	}, "\n")
}

func renderAssistantMarkdown(width int, content string) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	out, err := renderer.Render(content)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	return strings.TrimRight(out, "\n")
}

func (p *Page) SessionTitle() string {
	for _, message := range p.modelMessages {
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			text := strings.TrimSpace(message.Content)
			if len(text) > 48 {
				return text[:48] + "..."
			}
			return text
		}
	}
	return "Untitled Session"
}

func cloneMessages(messages []domain.Message) []domain.Message {
	out := make([]domain.Message, 0, len(messages))
	for _, message := range messages {
		cloned := message
		if len(message.Args) > 0 {
			cloned.Args = make(map[string]any, len(message.Args))
			for key, value := range message.Args {
				cloned.Args[key] = value
			}
		}
		out = append(out, cloned)
	}
	return out
}

func toUIMessages(messages []domain.Message) []chatui.Message {
	out := make([]chatui.Message, 0, len(messages))
	for _, message := range messages {
		switch message.Type {
		case domain.MessageTypeCompact:
			out = append(out, chatui.TextMessage(chatui.TypeSystem, message.Content))
		case domain.MessageTypeToolCall:
			out = append(out, chatui.TextMessage(chatui.TypeTool, formatToolCall(message.ToolCallID, message.ToolName, message.Args)))
		case domain.MessageTypeToolResult:
			out = append(out, chatui.TextMessage(chatui.TypeTool, formatToolResult(message.ToolCallID, message.ToolName, message.Result)))
		default:
			out = append(out, chatui.TextMessage(toUIType(message.Role), message.Content))
		}
	}
	return out
}

func toUIType(role string) string {
	switch role {
	case "user":
		return chatui.TypeUser
	case "assistant":
		return chatui.TypeAssistant
	default:
		return chatui.TypeSystem
	}
}

func (p *Page) markDirty() {
	p.dirty = true
}

func (p *Page) rebuildFrozenRendered(width int) {
	p.frozenWidth = width
	p.frozenRendered = make([]string, len(p.messages))
	contentWidth := max(20, width)

	for i, message := range p.messages {
		if message.Type != chatui.TypeAssistant || i == p.streamIndex {
			continue
		}
		p.frozenRendered[i] = renderAssistantMarkdown(contentWidth, message.Content)
	}
}

func startupLogo() string {
	return strings.TrimSpace(`
   ____        _______ _      ___
  / ___| ___  / / ____| |    |_ _|
 | |  _ / _ \/ / |    | |     | |
 | |_| | (_) / /| |___ | |___  | |
  \____|\___/_/  \____||_____|___|

  /models   choose model
  /resume   restore session
  /tools    inspect tools
`)
}

func needsReplyDivider(prev, current chatui.Message) bool {
	return (prev.Type == chatui.TypeUser && current.Type == chatui.TypeAssistant) ||
		(prev.Type == chatui.TypeAssistant && current.Type == chatui.TypeUser)
}

func renderReplyDivider(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Render(strings.Repeat("─", max(8, width)))
}
