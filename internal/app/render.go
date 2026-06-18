package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	sections = append(sections, helpStyle.Render(m.help.View(m.keys)))

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
	model, err := m.config.ActiveModel()
	if err != nil {
		return statusStyle.Render("model: unavailable")
	}

	return statusStyle.Render(
		fmt.Sprintf("model: %s  context: %s  focus: %s", model.ID, formatContextWindow(model.ContextWindow), m.focus),
	)
}

func (m Model) renderBottomPanel() string {
	width := m.contentWidth()
	style := inputBoxStyle
	if m.focus != focusInput {
		style = inactiveBoxStyle
	}
	return style.Width(width).Render(m.input.View())
}

func (m Model) renderSlashMenu() string {
	if !m.slash.visible || len(m.slash.filtered) == 0 {
		return ""
	}

	width := m.contentWidth()
	var lines []string
	for i, item := range m.slash.filtered {
		prefix := "  "
		if i == m.slash.selected {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s", prefix, item.command, item.description))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(lines, "\n"))
}

func (m Model) renderSlashMenuHeight(width int) int {
	if !m.slash.visible || len(m.slash.filtered) == 0 {
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
