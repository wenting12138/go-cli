package app

import "strings"

func newSlashMenu() slashMenu {
	commands := []slashCommand{
		{command: "/chat", description: "back to chat page"},
		{command: "/plan", description: "enter plan mode"},
		{command: "/approve", description: "exit plan mode"},
		{command: "/reject", description: "reject plan with optional feedback"},
		{command: "/cancel", description: "cancel feedback input"},
		{command: "/models", description: "open model selector"},
		{command: "/resume", description: "open session resume page"},
		{command: "/tools", description: "inspect tool definitions"},
		{command: "/compact", description: "compact model conversation"},
		{command: "/clear", description: "clear current chat"},
	}
	return slashMenu{commands: commands}
}

func (m *Model) updateSlashMenu() {
	items := m.currentSlashItems()
	if len(items) == 0 {
		m.slash.selected = 0
		return
	}
	if m.slash.selected >= len(items) {
		m.slash.selected = 0
	}
	if m.slash.selected < 0 {
		m.slash.selected = 0
	}
}

func (m Model) shouldShowSlashMenu() bool {
	return len(m.currentSlashItems()) > 0
}

func (m Model) currentSlashItems() []slashCommand {
	if m.current != pageChat || m.focus != focusInput {
		return nil
	}

	value := strings.TrimSpace(m.input.Value())
	if value == "" || !strings.HasPrefix(value, "/") || strings.Contains(value, " ") {
		return nil
	}

	query := strings.ToLower(value)
	filtered := make([]slashCommand, 0, len(m.slash.commands))
	for _, cmd := range m.slash.commands {
		if strings.HasPrefix(strings.ToLower(cmd.command), query) {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}

func (s *slashMenu) move(delta int, total int) {
	if total <= 0 {
		s.selected = 0
		return
	}
	s.selected = (s.selected + delta + total) % total
}

func (m Model) selectedSlashCompletion() (string, bool) {
	items := m.currentSlashItems()
	if len(items) == 0 {
		return "", false
	}
	index := m.slash.selected
	if index < 0 || index >= len(items) {
		index = 0
	}
	return items[index].command, true
}

func (s *slashMenu) hide() {
	s.selected = 0
}
