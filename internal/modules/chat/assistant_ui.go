package chat

import (
	"strings"
	"sync"

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
	assistantRendererMu    sync.Mutex
	assistantRendererCache = map[int]*glamour.TermRenderer{}
)

func renderAssistantMarkdown(width int, content string) string {
	renderer, err := assistantRenderer(width)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	out, err := renderer.Render(content)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	return strings.TrimRight(out, "\n")
}

func assistantRenderer(width int) (*glamour.TermRenderer, error) {
	assistantRendererMu.Lock()
	defer assistantRendererMu.Unlock()

	if renderer, ok := assistantRendererCache[width]; ok {
		return renderer, nil
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dracula"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	assistantRendererCache[width] = renderer
	return renderer, nil
}

func (p *Page) rebuildFrozenRendered(width int) {
	p.frozenWidth = width
	p.frozenRendered = make([]string, len(p.messages))
	contentWidth := max(20, width)
	renderWidth := max(1, contentWidth-assistantStyle.GetHorizontalFrameSize())

	for i, message := range p.messages {
		if message.Type != chatui.TypeAssistant || i == p.streamIndex {
			continue
		}
		p.frozenRendered[i] = renderAssistantMarkdown(renderWidth, message.Content)
	}
}

func (p *Page) freezeAssistantMessage(index int) {
	if index < 0 || index >= len(p.messages) {
		return
	}
	if p.messages[index].Type != chatui.TypeAssistant || p.frozenWidth <= 0 {
		return
	}
	if len(p.frozenRendered) != len(p.messages) {
		p.frozenRendered = make([]string, len(p.messages))
	}
	contentWidth := max(20, p.frozenWidth)
	renderWidth := max(1, contentWidth-assistantStyle.GetHorizontalFrameSize())
	p.frozenRendered[index] = renderAssistantMarkdown(renderWidth, p.messages[index].Content)
}
