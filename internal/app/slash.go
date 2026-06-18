package app

import "strings"

func newSlashMenu() slashMenu {
	commands := []slashCommand{
		{command: "/chat", description: "back to chat page"},
		{command: "/models", description: "open model selector"},
		{command: "/resume", description: "open session resume page"},
		{command: "/tools", description: "inspect tool definitions"},
		{command: "/clear", description: "clear current chat"},
	}
	return slashMenu{commands: commands}
}

func (m *Model) updateSlashMenu() {
	value := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(value, "/") {
		m.slash.hide()
		return
	}
	if strings.Contains(value, " ") {
		m.slash.hide()
		return
	}

	query := strings.ToLower(value)
	var filtered []slashCommand
	for _, cmd := range m.slash.commands {
		if strings.HasPrefix(strings.ToLower(cmd.command), query) {
			filtered = append(filtered, cmd)
		}
	}
	if len(filtered) == 0 {
		m.slash.hide()
		return
	}

	m.slash.filtered = filtered
	m.slash.visible = true
	if m.slash.selected >= len(filtered) {
		m.slash.selected = 0
	}
}

func (s *slashMenu) move(delta int) {
	if !s.visible || len(s.filtered) == 0 {
		return
	}
	s.selected = (s.selected + delta + len(s.filtered)) % len(s.filtered)
}

func (s *slashMenu) selectedCompletion() (string, bool) {
	if !s.visible || len(s.filtered) == 0 {
		return "", false
	}
	return s.filtered[s.selected].command, true
}

func (s *slashMenu) hide() {
	s.filtered = nil
	s.visible = false
	s.selected = 0
}
