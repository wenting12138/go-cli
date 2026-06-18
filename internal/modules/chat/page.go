package chat

import (
	"fmt"
	"strings"

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
)

type Page struct {
	messages        []domain.Message
	model           config.ModelConfig
	streamIndex     int
	streamRequestID string
}

func NewPage(model config.ModelConfig) *Page {
	return &Page{
		model:       model,
		streamIndex: -1,
		messages: []domain.Message{
			{Role: "system", Content: "Chat CLI ready. Type a message or use /resume or /models."},
			{Role: "assistant", Content: fmt.Sprintf("This is the main conversation page. Active model: %s/%s, context=%d, max_tokens=%d. Use /resume to open the session recovery page.", model.Provider, model.Name, model.ContextWindow, model.MaxTokens)},
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
	p.messages = append(p.messages, domain.Message{Role: "user", Content: text})
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
	p.messages = append(p.messages, domain.Message{Role: "system", Content: text})
}

func (p *Page) LoadSession(session domain.Session) {
	p.messages = append([]domain.Message{
		{Role: "system", Content: fmt.Sprintf("Resumed session %s: %s", session.ID, session.Title)},
	}, session.Messages...)
}

func (p *Page) SetModel(model config.ModelConfig) {
	p.model = model
	p.messages = append(p.messages, domain.Message{
		Role:    "system",
		Content: fmt.Sprintf("Switched model to %s (%s/%s), context=%d, max_tokens=%d.", model.ID, model.Provider, model.Name, model.ContextWindow, model.MaxTokens),
	})
}

func (p *Page) AppendStream(chunk string) {
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		return
	}
	p.messages[p.streamIndex].Content += chunk
}

func (p *Page) FinishStream() {
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		p.streamIndex = -1
		p.streamRequestID = ""
		return
	}
	p.messages[p.streamIndex].Content = strings.TrimSpace(p.messages[p.streamIndex].Content)
	p.streamIndex = -1
	p.streamRequestID = ""
}

func (p *Page) FailStream(err string) {
	if p.streamIndex >= 0 && p.streamIndex < len(p.messages) && p.messages[p.streamIndex].Content == "" {
		p.messages = append(p.messages[:p.streamIndex], p.messages[p.streamIndex+1:]...)
	}
	p.streamIndex = -1
	p.streamRequestID = ""
	p.messages = append(p.messages, domain.Message{
		Role:    "system",
		Content: err,
	})
}

func (p *Page) SnapshotMessages() []domain.Message {
	return append([]domain.Message(nil), p.messages...)
}

func (p *Page) BeginAssistantStream(requestID string) {
	p.messages = append(p.messages, domain.Message{Role: "assistant", Content: ""})
	p.streamIndex = len(p.messages) - 1
	p.streamRequestID = requestID
}

func (p *Page) HasActiveStream(requestID string) bool {
	return p.streamRequestID != "" && p.streamRequestID == requestID
}

func (p *Page) HasPendingStream() bool {
	return p.streamRequestID != ""
}

func (p *Page) ActiveRequestID() string {
	return p.streamRequestID
}

func renderMessage(width int, message domain.Message) string {
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
