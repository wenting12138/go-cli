package chat

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/modules/chat/domain"
	chatui "go-cli/internal/modules/chat/ui"
)

var (
	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("244")).
			PaddingLeft(1)
	thinkingBoxStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("248")).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("241")).
				Padding(0, 1)
	thinkingTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("246")).
				Bold(true)
)

func (p *Page) AppendReasoning(chunk string) {
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		return
	}
	p.modelMessages[p.modelStreamIndex].Reasoning += chunk
	p.markDirty()
}

func (p *Page) SetReasoningSignature(signature string) {
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		return
	}
	p.modelMessages[p.modelStreamIndex].ReasoningSignature = signature
	p.markDirty()
}

func (p *Page) BeginThinking(requestID string) {
	if p.streamRequestID == "" {
		p.streamRequestID = requestID
	}
	if p.thinkingIndex >= 0 && p.thinkingIndex < len(p.messages) {
		p.messages[p.thinkingIndex].Content = strings.TrimSpace(p.messages[p.thinkingIndex].Content)
		p.thinkingIndex = -1
	}
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		current := p.modelMessages[p.modelStreamIndex]
		if strings.TrimSpace(current.Content) != "" || strings.TrimSpace(current.Reasoning) != "" {
			p.modelStreamIndex = -1
		}
	}
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		p.modelMessages = append(p.modelMessages, domain.TextMessage("assistant", ""))
		p.modelStreamIndex = len(p.modelMessages) - 1
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeThinking, ""))
	p.frozenRendered = append(p.frozenRendered, "")
	p.thinkingIndex = len(p.messages) - 1
	p.streamRequestID = requestID
	p.markDirty()
}

func (p *Page) AppendThinking(chunk string) {
	if p.thinkingIndex < 0 || p.thinkingIndex >= len(p.messages) {
		return
	}
	p.messages[p.thinkingIndex].Content += chunk
	p.markDirty()
}

func renderThinkingCollapsed(width int, content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return thinkingTitleStyle.Render("Thinking")
	}

	lines := strings.Split(trimmed, "\n")
	preview := make([]string, 0, 4)
	preview = append(preview, thinkingTitleStyle.Render("Thinking"))

	lineLimit := 3
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		preview = append(preview, line)
		if len(preview) > lineLimit {
			break
		}
	}

	if len(preview) == 1 {
		preview = append(preview, trimmed)
	}

	if len(preview) > lineLimit+1 {
		preview = preview[:lineLimit+1]
	}

	if len(lines) > lineLimit {
		preview = append(preview, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("..."))
	}

	body := strings.Join(preview, "\n")
	return lipgloss.NewStyle().Width(max(1, width-thinkingBoxStyle.GetHorizontalFrameSize())).Render(body)
}
