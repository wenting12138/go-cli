package chat

import (
	"strings"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/lipgloss"

	chatui "go-cli/internal/modules/chat/ui"
)

var (
	assistantStyle = lipgloss.NewStyle().
		BorderLeft(true).
		BorderForeground(lipgloss.Color("63")).
		PaddingLeft(1).
		PaddingLeft(1)
)

func renderAssistantMarkdown(width int, content string) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dracula"),
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
