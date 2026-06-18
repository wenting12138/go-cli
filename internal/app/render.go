package app

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

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

	return statusStyle.Width(width).Render(
		fmt.Sprintf(
			"session: %s  model: %s  used: %s/%s  focus: %s",
			sessionID,
			model.ID,
			formatContextWindow(usedContext),
			formatContextWindow(model.ContextWindow),
			m.focus,
		),
	)
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
	return style.Width(max(1, width-style.GetHorizontalFrameSize())).Render(m.input.View())
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
