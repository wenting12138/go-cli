package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/config"
	"go-cli/internal/modules/chat/domain"
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
	messages         []domain.Message
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
		messages: []domain.Message{
			domain.TextMessage("system", "Chat CLI ready. Type a message or use /resume or /models."),
			domain.TextMessage("assistant", fmt.Sprintf("This is the main conversation page. Active model: %s/%s, context=%s. Use /resume to open the session recovery page.", model.Provider, model.Name, formatContextWindow(model.ContextWindow))),
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
	for _, message := range p.messages {
		lines = append(lines, renderMessage(width, message))
	}
	return strings.Join(lines, "\n\n")
}

func (p *Page) HandleInput(text string) tea.Cmd {
	if p.sessionID == "" {
		p.sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	message := domain.TextMessage("user", text)
	p.messages = append(p.messages, message)
	p.modelMessages = append(p.modelMessages, message)
	return nil
}

func (p *Page) ApplyUpdate(msg shared.PageUpdatedMsg) {
	message, ok := msg.Payload.(domain.Message)
	if !ok {
		return
	}
	p.messages = append(p.messages, message)
}

func (p *Page) SystemMessage(text string) {
	p.messages = append(p.messages, domain.TextMessage("system", text))
}

func (p *Page) LoadSession(session domain.Session) {
	p.sessionID = session.ID
	p.messages = append([]domain.Message{
		domain.TextMessage("system", fmt.Sprintf("Resumed session %s: %s", session.ID, session.Title)),
	}, session.Messages...)
}

func (p *Page) RestoreSession(sessionID, title string, uiMessages, modelMessages []domain.Message) {
	p.sessionID = sessionID
	p.messages = append([]domain.Message{
		domain.TextMessage("system", fmt.Sprintf("Resumed session %s: %s", sessionID, title)),
	}, cloneMessages(uiMessages)...)
	p.modelMessages = cloneMessages(modelMessages)
	p.streamIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
}

func (p *Page) SetModel(model config.ModelConfig) {
	p.model = model
	p.messages = append(p.messages, domain.Message{
		Role:    "system",
		Type:    domain.MessageTypeText,
		Content: fmt.Sprintf("Switched model to %s (%s/%s), context=%s.", model.ID, model.Provider, model.Name, formatContextWindow(model.ContextWindow)),
	})
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
	p.messages = append(p.messages, domain.Message{
		Role:    "system",
		Type:    domain.MessageTypeText,
		Content: err,
	})
}

func (p *Page) SnapshotMessages() []domain.Message {
	return cloneMessages(p.modelMessages)
}

func (p *Page) UISnapshotMessages() []domain.Message {
	return cloneMessages(p.messages)
}

func (p *Page) BeginRequest(requestID string) {
	p.streamRequestID = requestID
	p.streamIndex = -1
}

func (p *Page) BeginAssistantStream(requestID string) {
	if p.streamRequestID == "" {
		p.streamRequestID = requestID
	}
	message := domain.TextMessage("assistant", "")
	p.messages = append(p.messages, message)
	p.modelMessages = append(p.modelMessages, message)
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
	message := domain.ToolCallMessage(toolCallID, toolName, args)
	p.messages = append(p.messages, message)
	p.modelMessages = append(p.modelMessages, message)
}

func (p *Page) AppendToolResult(toolCallID, toolName, result string) {
	uiMessage := domain.ToolResultMessage(toolCallID, toolName, result)
	p.messages = append(p.messages, uiMessage)
	p.modelMessages = append(p.modelMessages, uiMessage)
}

func renderMessage(width int, message domain.Message) string {
	switch message.Type {
	case domain.MessageTypeToolCall:
		return renderToolCall(width, message)
	case domain.MessageTypeToolResult:
		return renderToolResult(width, message)
	}

	label := systemStyle.Render("system")
	switch message.Role {
	case "user":
		label = userStyle.Render("you")
	case "assistant":
		label = assistantStyle.Render("assistant")
	}

	content := lipgloss.NewStyle().
		Width(max(20, width)).
		Render(message.Content)

	return fmt.Sprintf("%s\n%s", label, content)
}

func renderToolCall(width int, message domain.Message) string {
	body := []string{
		fmt.Sprintf("name: %s", message.ToolName),
		fmt.Sprintf("id: %s", message.ToolCallID),
		fmt.Sprintf("args: %s", formatArgs(message.Args)),
	}
	return fmt.Sprintf(
		"%s\n%s",
		toolLabelStyle.Render("tool call"),
		toolBodyStyle.Width(max(20, width)).Render(strings.Join(body, "\n")),
	)
}

func renderToolResult(width int, message domain.Message) string {
	body := []string{
		fmt.Sprintf("name: %s", message.ToolName),
		fmt.Sprintf("id: %s", message.ToolCallID),
		fmt.Sprintf("result: %s", message.Result),
	}
	return fmt.Sprintf(
		"%s\n%s",
		toolLabelStyle.Render("tool result"),
		toolBodyStyle.Width(max(20, width)).Render(strings.Join(body, "\n")),
	)
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
