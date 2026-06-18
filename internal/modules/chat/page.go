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
			Bold(true)
	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
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
	modelMessages    []domain.Message
	model            config.ModelConfig
	sessionID        string
	streamIndex      int
	modelStreamIndex int
	streamRequestID  string
}

func NewPage(model config.ModelConfig) *Page {
	return &Page{
		model:            model,
		streamIndex:      -1,
		modelStreamIndex: -1,
		messages: []chatui.Message{
			chatui.TextMessage(chatui.TypeSystem, "Chat CLI ready. Type a message or use /resume or /models."),
			chatui.TextMessage(chatui.TypeAssistant, fmt.Sprintf("This is the main conversation page. Active model: %s/%s, context=%s. Use /resume to open the session recovery page.", model.Provider, model.Name, formatContextWindow(model.ContextWindow))),
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
	var lines []string
	for i, message := range p.messages {
		lines = append(lines, p.renderMessage(width, i, message))
	}
	return strings.Join(lines, "\n\n")
}

func (p *Page) HandleInput(text string) tea.Cmd {
	if p.sessionID == "" {
		p.sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeUser, text))
	p.modelMessages = append(p.modelMessages, domain.TextMessage("user", text))
	return nil
}

func (p *Page) ApplyUpdate(msg shared.PageUpdatedMsg) {
	message, ok := msg.Payload.(chatui.Message)
	if !ok {
		return
	}
	p.messages = append(p.messages, message)
}

func (p *Page) SystemMessage(text string) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, text))
}

func (p *Page) LoadSession(session domain.Session) {
	p.sessionID = session.ID
	p.messages = append([]chatui.Message{
		chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Resumed session %s: %s", session.ID, session.Title)),
	}, toUIMessages(session.Messages)...)
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
}

func (p *Page) SetModel(model config.ModelConfig) {
	p.model = model
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Switched model to %s (%s/%s), context=%s.", model.ID, model.Provider, model.Name, formatContextWindow(model.ContextWindow))))
}

func (p *Page) AppendStream(chunk string) {
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		return
	}
	p.messages[p.streamIndex].Content += chunk
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].Content += chunk
	}
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
}

func (p *Page) FailStream(err string) {
	if p.streamIndex >= 0 && p.streamIndex < len(p.messages) && p.messages[p.streamIndex].Content == "" {
		p.messages = append(p.messages[:p.streamIndex], p.messages[p.streamIndex+1:]...)
	}
	p.streamIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, err))
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
	p.streamIndex = len(p.messages) - 1
	p.modelStreamIndex = len(p.modelMessages) - 1
	p.streamRequestID = requestID
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
}

func (p *Page) AppendToolResult(toolCallID, toolName, result string) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeTool, formatToolResult(toolCallID, toolName, result)))
	p.modelMessages = append(p.modelMessages, domain.ToolResultMessage(toolCallID, toolName, result))
}

func (p *Page) renderMessage(width int, index int, message chatui.Message) string {
	label := systemStyle.Render("system")
	switch message.Type {
	case chatui.TypeUser:
		label = userStyle.Render("you")
	case chatui.TypeAssistant:
		label = assistantStyle.Render("assistant")
	case chatui.TypeTool:
		label = toolLabelStyle.Render("tool")
	}

	contentWidth := max(20, width)
	content := lipgloss.NewStyle().Width(contentWidth).Render(message.Content)
	if message.Type == chatui.TypeAssistant && index != p.streamIndex {
		content = renderAssistantMarkdown(contentWidth, message.Content)
	}
	if message.Type == chatui.TypeTool {
		content = toolBodyStyle.Width(max(1, contentWidth-toolBodyStyle.GetHorizontalFrameSize())).Render(message.Content)
	}

	return fmt.Sprintf("%s\n%s", label, content)
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
