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
		m.viewport.View(),
		m.renderBottomPanel(),
	}

	if m.current == pageChat {
		sections = append(sections, m.renderSlashMenu())
	}

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

func (m Model) renderModelStatus() string {
	model, err := m.config.ActiveModel()
	if err != nil {
		return statusStyle.Render("model: unavailable")
	}

	return statusStyle.Render(
		fmt.Sprintf("model: %s  context: %s", model.ID, formatContextWindow(model.ContextWindow)),
	)
}

func (m Model) renderBottomPanel() string {
	width := m.viewport.Width
	if width <= 0 {
		width = max(20, m.width-4)
	}

	if m.current == pageChat {
		return inputBoxStyle.Width(width).Render(m.input.View())
	}

	return inactiveBoxStyle.Width(width).Render("commands are available on the chat page")
}

func (m Model) renderSlashMenu() string {
	if !m.slash.visible || len(m.slash.filtered) == 0 {
		return ""
	}

	width := max(20, m.viewport.Width)
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
