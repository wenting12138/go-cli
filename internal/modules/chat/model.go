package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/modules/chat/bot"
	"go-cli/internal/modules/chat/domain"
)

type replyMsg struct {
	message domain.Message
}

type keyMap struct {
	Send  key.Binding
	Clear key.Binding
	Quit  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Send, k.Clear, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Send, k.Clear},
		{k.Quit},
	}
}

var keys = keyMap{
	Send: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send"),
	),
	Clear: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "clear chat"),
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
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true)
	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Bold(true)
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
)

type Model struct {
	viewport viewport.Model
	input    textinput.Model
	help     help.Model
	keys     keyMap
	messages []domain.Message
	width    int
	height   int
}

func NewModel() Model {
	input := textinput.New()
	input.Placeholder = "Type a message..."
	input.Prompt = "> "
	input.Focus()
	input.CharLimit = 280

	messages := []domain.Message{
		{Role: "system", Content: "Chat CLI ready. Type a message and press Enter."},
		{Role: "assistant", Content: "Hello. I can echo your message and show a simple scripted reply."},
	}

	return Model{
		viewport: viewport.New(0, 0),
		input:    input,
		help:     help.New(),
		keys:     keys,
		messages: messages,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width

		contentWidth := max(20, msg.Width-4)
		headerHeight := lipgloss.Height(m.renderHeader())
		inputHeight := 3
		helpHeight := 1
		viewportHeight := max(5, msg.Height-headerHeight-inputHeight-helpHeight-4)

		m.viewport.Width = contentWidth
		m.viewport.Height = viewportHeight
		m.input.Width = max(10, contentWidth-4)
		m.syncViewport()
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Clear):
			m.messages = []domain.Message{
				{Role: "system", Content: "Chat history cleared."},
			}
			m.syncViewport()
			return m, nil
		case key.Matches(msg, m.keys.Send):
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}

			m.messages = append(m.messages, domain.Message{Role: "user", Content: text})
			m.input.SetValue("")
			m.syncViewport()
			return m, bot.FakeReplyCmd(text, func(message domain.Message) tea.Msg {
				return replyMsg{message: message}
			})
		}
	case replyMsg:
		m.messages = append(m.messages, msg.message)
		m.syncViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	body := lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.viewport.View(),
		inputBoxStyle.Width(m.viewport.Width).Render(m.input.View()),
		systemStyle.Render(m.help.View(m.keys)),
	)

	return appStyle.Render(body)
}

func (m *Model) syncViewport() {
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m Model) renderHeader() string {
	return headerStyle.Width(max(20, m.width-4)).Render(" Go CLI Chat ")
}

func (m Model) renderMessages() string {
	var lines []string
	for _, message := range m.messages {
		lines = append(lines, m.renderMessage(message))
	}
	return strings.Join(lines, "\n\n")
}

func (m Model) renderMessage(message domain.Message) string {
	label := systemStyle.Render("system")
	switch message.Role {
	case "user":
		label = userStyle.Render("you")
	case "assistant":
		label = assistantStyle.Render("assistant")
	}

	content := lipgloss.NewStyle().
		Width(max(20, m.viewport.Width)).
		Render(message.Content)

	return fmt.Sprintf("%s\n%s", label, content)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
