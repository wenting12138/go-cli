package app

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/config"
	"go-cli/internal/eventbus"
	"go-cli/internal/modules/shared"
	chatservice "go-cli/internal/services/chat"
)

type pageID string

const (
	pageChat   pageID = "chat"
	pageModels pageID = "models"
	pageResume pageID = "resume"
	pageTools  pageID = "tools"
)

type keyMap struct {
	Send key.Binding
	Back key.Binding
	Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Send, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Send, k.Back},
		{k.Quit},
	}
}

var keys = keyMap{
	Send: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back to chat"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}

var (
	appStyle    = lipgloss.NewStyle().Padding(1, 2)
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
	inactiveBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Foreground(lipgloss.Color("242")).
				Padding(0, 1)
)

type Model struct {
	viewport viewport.Model
	input    textinput.Model
	help     help.Model
	keys     keyMap
	width    int
	height   int
	current  pageID
	pages    map[pageID]shared.Page
	config   config.State
	bus      *eventbus.Bus
	events   <-chan any
	chatSvc  *chatservice.Service
	slash    slashMenu
}

type eventMsg struct {
	event any
}

type slashCommand struct {
	command     string
	description string
}

type slashMenu struct {
	commands []slashCommand
	filtered []slashCommand
	visible  bool
	selected int
}
