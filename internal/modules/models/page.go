package models

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"go-cli/internal/config"
	"go-cli/internal/modules/shared"
)

type item struct {
	model config.ModelConfig
}

func (i item) Title() string {
	return fmt.Sprintf("%s/%s", i.model.Provider, i.model.Name)
}

func (i item) Description() string {
	return fmt.Sprintf("id=%s  context=%d  max_tokens=%d", i.model.ID, i.model.ContextWindow, i.model.MaxTokens)
}

func (i item) FilterValue() string {
	return i.model.ID
}

type Page struct {
	list         list.Model
	currentModel string
	systemNotice string
}

func NewPage(models []config.ModelConfig, currentModel string) *Page {
	items := make([]list.Item, 0, len(models))
	selectedIndex := 0
	for i, model := range models {
		items = append(items, item{model: model})
		if model.ID == currentModel {
			selectedIndex = i
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	l := list.New(items, delegate, 0, 0)
	l.Title = "Available models"
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.Select(selectedIndex)

	return &Page{
		list:         l,
		currentModel: currentModel,
	}
}

func (p *Page) Title() string {
	return "Models"
}

func (p *Page) Placeholder() string {
	return "Use arrows to select, Enter to switch"
}

func (p *Page) Content(width int) string {
	return p.list.View()
}

func (p *Page) HandleInput(text string) tea.Cmd {
	p.systemNotice = "Use arrow keys and Enter to switch model."
	return nil
}

func (p *Page) ApplyUpdate(shared.PageUpdatedMsg) {}

func (p *Page) SystemMessage(text string) {
	p.systemNotice = text
	p.list.NewStatusMessage(text)
}

func (p *Page) SetCurrentModel(id string) {
	p.currentModel = id
	p.systemNotice = "Model switched. Returned to chat."
	p.list.NewStatusMessage(p.systemNotice)

	for i, entry := range p.list.Items() {
		listItem, ok := entry.(item)
		if ok && listItem.model.ID == id {
			p.list.Select(i)
			return
		}
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

func (p *Page) SelectedModelID() string {
	selected, ok := p.list.SelectedItem().(item)
	if !ok {
		return ""
	}
	return selected.model.ID
}
