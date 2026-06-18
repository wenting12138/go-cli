package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"go-cli/internal/modules/chat"
)

func Run() error {
	program := tea.NewProgram(chat.NewModel(), tea.WithAltScreen())
	_, err := program.Run()
	return err
}
