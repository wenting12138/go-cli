package shared

import tea "github.com/charmbracelet/bubbletea"

type Page interface {
	Title() string
	Placeholder() string
	Content(width int) string
	HandleInput(text string) tea.Cmd
	ApplyUpdate(PageUpdatedMsg)
	SystemMessage(text string)
}

type PageUpdatedMsg struct {
	Page    string
	Payload any
}

type ModelSelectedMsg struct {
	ModelID string
}
