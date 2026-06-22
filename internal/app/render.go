package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat"
	"go-cli/internal/modules/chat/domain"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	sections := []string{
		m.renderHeader(),
		m.renderViewport(),
	}

	if m.current == pageChat {
		sections = append(sections, m.renderSlashMenu())
	}

	sections = append(sections, m.renderBottomPanel())
	sections = append(sections, m.renderModelStatus())
	sections = append(sections, m.renderHelp())

	body := lipgloss.JoinVertical(lipgloss.Left, sections...)

	return appStyle.Render(body)
}

func (m Model) renderHeader() string {
	title := "Go CLI"
	if page := m.currentPage(); page != nil {
		title = page.Title()
	}
	return headerStyle.Width(max(20, m.width-4)).Render(" " + title + " ")
}

func (m Model) renderViewport() string {
	content := m.viewport.View()
	scrollbar := m.renderScrollbar()
	if scrollbar == "" {
		return content
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, content, scrollbar)
}

func (m Model) renderModelStatus() string {
	width := m.contentWidth()
	model, err := m.config.ActiveModel()
	if err != nil {
		return statusStyle.Width(width).Render("model: unavailable")
	}

	sessionID := "-"
	usedContext := 0
	if chatPage, ok := m.pages[pageChat].(*chat.Page); ok && chatPage.SessionID() != "" {
		sessionID = chatPage.SessionID()
		usedContext = estimateContextUsage(chatPage.SystemPrompt(), chatPage.SnapshotMessages())
	}

	line := fmt.Sprintf(
		"session: %s  mode: %s  model: %s  used: %s/%s  focus: %s",
		sessionID,
		m.mode,
		model.ID,
		formatContextWindow(usedContext),
		formatContextWindow(model.ContextWindow),
		m.focus,
	)
	if metrics := formatMetrics(m.lastMetrics); metrics != "" {
		line += "  " + metrics
	}
	return statusStyle.Width(width).Render(line)
}

func (m Model) renderHelp() string {
	return helpStyle.Width(m.contentWidth()).Render(m.help.View(m.keys))
}

func (m Model) renderBottomPanel() string {
	width := m.contentWidth()
	style := inputBoxStyle
	if m.focus != focusInput {
		style = inactiveBoxStyle
	}
	inputBox := style.Width(max(1, width-style.GetHorizontalFrameSize())).Render(m.input.View())
	if m.current != pageChat || !m.hasTypingIndicator() {
		return inputBox
	}
	return lipgloss.JoinVertical(lipgloss.Left, inputBox, m.renderTypingIndicator())
}

func (m Model) renderTypingIndicator() string {
	dots := []string{"●", "●", "●"}
	parts := make([]string, 0, len(dots))
	active := m.typingIndicatorFrame % len(dots)
	for i, dot := range dots {
		style := typingIndicatorStyle
		if i == active {
			style = typingIndicatorActiveStyle
		}
		parts = append(parts, style.Render(dot))
	}
	return lipgloss.NewStyle().
		Width(m.contentWidth()).
		PaddingLeft(2).
		Render(strings.Join(parts, "  "))
}

func (m Model) renderSlashMenu() string {
	items := m.currentSlashItems()
	if len(items) == 0 {
		return ""
	}

	width := m.contentWidth()
	var lines []string
	selected := m.slash.selected
	if selected < 0 || selected >= len(items) {
		selected = 0
	}
	for i, item := range items {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s", prefix, item.command, item.description))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(max(1, width-4)).
		Render(strings.Join(lines, "\n"))
}

func (m Model) renderSlashMenuHeight(width int) int {
	if len(m.currentSlashItems()) == 0 {
		return 0
	}
	return lipgloss.Height(m.renderSlashMenu())
}

func (m Model) renderScrollbar() string {
	height := m.viewport.Height
	if height <= 0 {
		return ""
	}

	total := m.viewport.TotalLineCount()
	if total <= height {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color("238")).
			Render(strings.Repeat(" \n", max(0, height-1)) + " ")
	}

	track := make([]string, height)
	for i := range track {
		track[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render("│")
	}

	thumbSize := max(1, height*height/total)
	maxOffset := total - height
	thumbStart := 0
	if maxOffset > 0 {
		thumbStart = m.viewport.YOffset * (height - thumbSize) / maxOffset
	}

	for i := thumbStart; i < thumbStart+thumbSize && i < len(track); i++ {
		track[i] = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Render("█")
	}

	return strings.Join(track, "\n")
}

func estimateContextUsage(systemPrompt string, messages []domain.Message) int {
	total := roughTokens(systemPrompt)
	for _, message := range messages {
		total += roughTokens(message.ModelRole())
		total += roughTokens(message.ModelContent())
	}
	return total
}

func roughTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	tokens := runes / 4
	if runes%4 != 0 {
		tokens++
	}
	if tokens < 1 {
		return 1
	}
	return tokens
}

func formatMetrics(metrics protocol.ResponseMetrics) string {
	parts := make([]string, 0, 4)
	if metrics.FirstTokenDuration > 0 {
		parts = append(parts, "first="+formatDuration(metrics.FirstTokenDuration))
	}
	if metrics.TotalDuration > 0 {
		parts = append(parts, "total="+formatDuration(metrics.TotalDuration))
	}
	if metrics.PromptEvalDuration > 0 || metrics.PromptEvalCount > 0 {
		part := "prompt=" + formatDuration(metrics.PromptEvalDuration)
		if metrics.PromptEvalCount > 0 {
			part += fmt.Sprintf("/%d", metrics.PromptEvalCount)
		}
		parts = append(parts, part)
	}
	if metrics.EvalDuration > 0 || metrics.EvalCount > 0 {
		part := "eval=" + formatDuration(metrics.EvalDuration)
		if metrics.EvalCount > 0 {
			part += fmt.Sprintf("/%d", metrics.EvalCount)
		}
		parts = append(parts, part)
	}
	if metrics.LoadDuration > 0 {
		parts = append(parts, "load="+formatDuration(metrics.LoadDuration))
	}
	return strings.Join(parts, "  ")
}

func formatDuration(nanos int64) string {
	if nanos <= 0 {
		return "0ms"
	}
	ms := nanos / 1_000_000
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.2fs", float64(nanos)/1_000_000_000)
}

func (m Model) viewportLineAtMouse(msg tea.MouseMsg) (int, bool) {
	const appPaddingTop = 1
	const appPaddingLeft = 2

	viewportTop := appPaddingTop + lipgloss.Height(m.renderHeader())
	viewportBottom := viewportTop + m.viewport.Height
	if msg.Y < viewportTop || msg.Y >= viewportBottom {
		return 0, false
	}

	viewportLeft := appPaddingLeft
	viewportRight := viewportLeft + m.viewport.Width
	if msg.X < viewportLeft || msg.X >= viewportRight {
		return 0, false
	}

	return m.viewport.YOffset + (msg.Y - viewportTop), true
}
