package app

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/agentmode"
	"go-cli/internal/config"
	"go-cli/internal/eventbus"
	"go-cli/internal/modules/shared"
	chatservice "go-cli/internal/services/chat"
	"go-cli/internal/sessionstore"
)

type pageID string
type focusMode string

const (
	pageChat   pageID = "chat"
	pageModels pageID = "models"
	pageResume pageID = "resume"
	pageTools  pageID = "tools"

	focusInput    focusMode = "input"
	focusViewport focusMode = "viewport"
)

type keyMap struct {
	Send       key.Binding
	Back       key.Binding
	Quit       key.Binding
	Focus      key.Binding
	ToggleTool key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Send, k.Focus, k.ToggleTool, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Send, k.Focus, k.ToggleTool, k.Back},
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
	Focus: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "toggle focus"),
	),
	ToggleTool: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "expand tool"),
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
	typingIndicatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("239"))
	typingIndicatorActiveStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("252"))
)

type Model struct {
	viewport             viewport.Model
	viewportContent      string
	viewportContentPage  pageID
	viewportContentWidth int
	input                textinput.Model
	help                 help.Model
	keys                 keyMap
	width                int
	height               int
	current              pageID
	pages                map[pageID]shared.Page
	config               config.State
	bus                  *eventbus.Bus
	events               <-chan any
	chatSvc              *chatservice.Service
	slash                slashMenu
	focus                focusMode
	store                *sessionstore.Store
	mode                 agentmode.Mode
	typingIndicatorFrame int
}

type eventMsg struct {
	event any
}

type animationTickMsg struct{}

type slashCommand struct {
	command     string
	description string
}

type slashMenu struct {
	commands []slashCommand
	selected int
}
