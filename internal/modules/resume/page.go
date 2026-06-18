package resume

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"go-cli/internal/modules/shared"
	"go-cli/internal/sessionstore"
)

type item struct {
	summary sessionstore.Summary
}

func (i item) Title() string {
	return fmt.Sprintf("[%s] %s", i.summary.ID, i.summary.Title)
}

func (i item) Description() string {
	return i.summary.Preview
}

func (i item) FilterValue() string {
	return i.summary.ID
}

type Page struct {
	list         list.Model
	lastResumed  string
	systemNotice string
}

func NewPage(summaries []sessionstore.Summary) *Page {
	items := make([]list.Item, 0, len(summaries))
	for _, summary := range summaries {
		items = append(items, item{summary: summary})
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	l := list.New(items, delegate, 0, 0)
	l.Title = "Available sessions"
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)

	return &Page{list: l}
}

func (p *Page) Title() string {
	return "Resume Session"
}

func (p *Page) Placeholder() string {
	return "Use arrows to select, Enter to restore"
}

func (p *Page) Content(width int) string {
	return p.list.View()
}

func (p *Page) HandleInput(text string) tea.Cmd {
	p.systemNotice = "Use arrow keys and Enter to restore a session."
	p.list.NewStatusMessage(p.systemNotice)
	return nil
}

func (p *Page) ApplyUpdate(shared.PageUpdatedMsg) {}

func (p *Page) SystemMessage(text string) {
	p.systemNotice = text
	p.list.NewStatusMessage(text)
}

func (p *Page) HasSession(id string) bool {
	for _, entry := range p.list.Items() {
		listItem, ok := entry.(item)
		if ok && listItem.summary.ID == id {
			return true
		}
	}
	return false
}

func (p *Page) MarkResumed(id string) {
	p.lastResumed = id
	p.systemNotice = "Session restored. Returned to chat."
	p.list.NewStatusMessage(p.systemNotice)

	for i, entry := range p.list.Items() {
		listItem, ok := entry.(item)
		if ok && listItem.summary.ID == id {
			p.list.Select(i)
			return
		}
	}
}

func (p *Page) SetSessions(summaries []sessionstore.Summary) {
	items := make([]list.Item, 0, len(summaries))
	selectedID := p.SelectedSessionID()
	selectedIndex := 0

	for i, summary := range summaries {
		items = append(items, item{summary: summary})
		if summary.ID == selectedID {
			selectedIndex = i
		}
	}

	p.list.SetItems(items)
	if len(items) > 0 {
		p.list.Select(selectedIndex)
	}
}

func (p *Page) SetSize(width, height int) {
	p.list.SetSize(width, height)
}

func (p *Page) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return cmd
}

func (p *Page) SelectedSessionID() string {
	selected, ok := p.list.SelectedItem().(item)
	if !ok {
		return ""
	}
	return selected.summary.ID
}
