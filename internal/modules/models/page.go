package models

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	return fmt.Sprintf("id=%s  context=%s", i.model.ID, formatContextWindow(i.model.ContextWindow))
}

func (i item) FilterValue() string {
	return i.model.ID
}

type Page struct {
	list         list.Model
	models       []config.ModelConfig
	currentModel string
	systemNotice string
	renaming     bool
	renameInput  textinput.Model
	width        int
	height       int
}

func NewPage(models []config.ModelConfig, currentModel string) *Page {
	input := textinput.New()
	input.Prompt = "name> "
	input.CharLimit = 256

	p := &Page{
		models:       cloneModels(models),
		currentModel: currentModel,
		renameInput:  input,
	}
	p.rebuildList(models, currentModel, currentModel)
	return p
}

func (p *Page) rebuildList(models []config.ModelConfig, currentModel, selectedModelID string) {
	items := make([]list.Item, 0, len(models))
	selectedIndex := 0
	for i, model := range models {
		items = append(items, item{model: model})
		if model.ID == selectedModelID || (selectedModelID == "" && model.ID == currentModel) {
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
	p.list = l
	p.models = cloneModels(models)
	p.currentModel = currentModel
	p.applyListSize()
}

func (p *Page) Title() string {
	return "Models"
}

func (p *Page) Placeholder() string {
	if p.renaming {
		return "Enter to save model name, Esc to cancel"
	}
	return "Use arrows to select, Enter to switch, r to rename"
}

func (p *Page) Content(width int) string {
	content := p.list.View()
	if !p.renaming {
		return content
	}

	editor := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Render(p.renameInput.View())
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("244")).
		Render("Rename selected model. Enter saves, Esc cancels.")
	return strings.Join([]string{content, editor, hint}, "\n\n")
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

func (p *Page) SetModels(models []config.ModelConfig, currentModel string) {
	p.SetModelsSelected(models, currentModel, p.SelectedModelID())
}

func (p *Page) SetModelsSelected(models []config.ModelConfig, currentModel, selectedModelID string) {
	p.rebuildList(models, currentModel, selectedModelID)
}

func (p *Page) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.renameInput.Width = max(12, width-12)
	p.applyListSize()
}

func (p *Page) Update(msg tea.Msg) tea.Cmd {
	if p.renaming {
		var cmd tea.Cmd
		p.renameInput, cmd = p.renameInput.Update(msg)
		return cmd
	}
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

func (p *Page) SelectedModel() (config.ModelConfig, bool) {
	selected, ok := p.list.SelectedItem().(item)
	if !ok {
		return config.ModelConfig{}, false
	}
	return selected.model, true
}

func (p *Page) StartRename() bool {
	selected, ok := p.SelectedModel()
	if !ok {
		return false
	}
	p.renaming = true
	p.renameInput.SetValue(selected.Name)
	p.renameInput.CursorEnd()
	p.renameInput.Focus()
	p.applyListSize()
	p.systemNotice = fmt.Sprintf("Editing model name for %s", selected.ID)
	p.list.NewStatusMessage(p.systemNotice)
	return true
}

func (p *Page) CancelRename() {
	p.renaming = false
	p.renameInput.Blur()
	p.renameInput.SetValue("")
	p.applyListSize()
	p.systemNotice = "Rename cancelled."
	p.list.NewStatusMessage(p.systemNotice)
}

func (p *Page) IsRenaming() bool {
	return p.renaming
}

func (p *Page) RenameValue() string {
	return strings.TrimSpace(p.renameInput.Value())
}

func (p *Page) FinishRename() {
	p.renaming = false
	p.renameInput.Blur()
	p.renameInput.SetValue("")
	p.applyListSize()
}

func (p *Page) applyListSize() {
	if p.width <= 0 || p.height <= 0 {
		return
	}
	height := p.height
	if p.renaming {
		height -= p.renameSectionHeight()
	}
	if height < 1 {
		height = 1
	}
	p.list.SetSize(p.width, height)
}

func (p *Page) renameSectionHeight() int {
	editor := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Render(p.renameInput.View())
	hint := "Rename selected model. Enter saves, Esc cancels."
	return lipgloss.Height(editor) + lipgloss.Height(hint) + 2
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cloneModels(models []config.ModelConfig) []config.ModelConfig {
	out := make([]config.ModelConfig, 0, len(models))
	for _, model := range models {
		out = append(out, model)
	}
	return out
}

func formatContextWindow(size int) string {
	if size%1000 == 0 {
		return fmt.Sprintf("%dk", size/1000)
	}
	return fmt.Sprintf("%.1fk", float64(size)/1000)
}
