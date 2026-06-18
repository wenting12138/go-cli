package resume

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/modules/shared"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	bodyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

type Page struct {
	sessions     []domain.Session
	lastResumed  string
	systemNotice string
}

func NewPage() *Page {
	return &Page{
		sessions: []domain.Session{
			{
				ID:    "1",
				Title: "Daily Standup Notes",
				Messages: []domain.Message{
					{Role: "user", Content: "Summarize today's engineering standup."},
					{Role: "assistant", Content: "Main blockers were deployment access and flaky integration tests."},
				},
			},
			{
				ID:    "2",
				Title: "Release Checklist",
				Messages: []domain.Message{
					{Role: "user", Content: "Create a release checklist for v1.2.0."},
					{Role: "assistant", Content: "Verify changelog, tag build, run smoke tests, and publish artifacts."},
				},
			},
		},
	}
}

func (p *Page) Title() string {
	return "Resume Session"
}

func (p *Page) Placeholder() string {
	return "Use /resume <id> to restore a session"
}

func (p *Page) Content(width int) string {
	lines := []string{
		titleStyle.Render("Available sessions"),
		bodyStyle.Width(max(20, width)).Render("Use /resume <id> to restore one of the sessions below."),
	}

	for _, session := range p.sessions {
		lines = append(lines, "")
		lines = append(lines, bodyStyle.Width(max(20, width)).Render(fmt.Sprintf("[%s] %s", session.ID, session.Title)))
		lines = append(lines, hintStyle.Width(max(20, width)).Render(preview(session)))
	}

	if p.lastResumed != "" {
		lines = append(lines, "", hintStyle.Render("Last resumed session: "+p.lastResumed))
	}
	if p.systemNotice != "" {
		lines = append(lines, "", hintStyle.Width(max(20, width)).Render(p.systemNotice))
	}

	return strings.Join(lines, "\n")
}

func (p *Page) HandleInput(text string) tea.Cmd {
	p.systemNotice = "This page does not accept direct input. Use /resume <id>."
	return nil
}

func (p *Page) ApplyUpdate(shared.PageUpdatedMsg) {}

func (p *Page) SystemMessage(text string) {
	p.systemNotice = text
}

func (p *Page) SessionByID(id string) (domain.Session, bool) {
	for _, session := range p.sessions {
		if session.ID == id {
			return session, true
		}
	}
	return domain.Session{}, false
}

func (p *Page) MarkResumed(id string) {
	p.lastResumed = id
	p.systemNotice = "Session restored. Returned to chat."
}

func preview(session domain.Session) string {
	if len(session.Messages) == 0 {
		return "No messages."
	}
	first := session.Messages[0].Content
	if len(first) > 56 {
		first = first[:56] + "..."
	}
	return first
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
