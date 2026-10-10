package app

import "strings"

func newSlashMenu() slashMenu {
	commands := []slashCommand{
		{command: "/chat", description: "back to chat page"},
		{command: "/models", description: "open model selector"},
		{command: "/resume", description: "open session resume page"},
		{command: "/tools", description: "inspect tool definitions"},
		{command: "/skills", description: "open skill selector"},
		{command: "/skill-clear", description: "clear active and pending skills"},
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
	filtered := make([]slashCommand, 0, len(m.slash.commands)+len(m.skills))
	for _, cmd := range m.slash.commands {
		if strings.HasPrefix(strings.ToLower(cmd.command), query) {
			filtered = append(filtered, cmd)
		}
	}
	for _, skill := range m.skills {
		if !skill.UserInvocable {
			continue
		}
		command := "/" + skill.Name
		if !strings.HasPrefix(strings.ToLower(command), query) {
			continue
		}
		description := "load skill"
		if strings.TrimSpace(skill.Description) != "" {
			description = skill.Description
		}
		filtered = append(filtered, slashCommand{
			command:     command,
			description: description,
		})
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
