package skills

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/modules/shared"
	skillstore "go-cli/internal/skills"
)

type item struct {
	skill skillstore.Skill
	badge string
}

func (i item) Title() string {
	if i.badge == "" {
		return i.skill.Name
	}
	return fmt.Sprintf("%s [%s]", i.skill.Name, i.badge)
}

func (i item) Description() string {
	parts := []string{fmt.Sprintf("[%s]", i.skill.Scope), i.skill.Context}
	if len(i.skill.AllowedTools) > 0 {
		parts = append(parts, fmt.Sprintf("%d tools", len(i.skill.AllowedTools)))
	} else {
		parts = append(parts, "all tools")
	}
	meta := strings.Join(parts, "  ")
	if i.skill.Description == "" {
		return meta
	}
	return fmt.Sprintf("%s  %s", meta, i.skill.Description)
}

func (i item) FilterValue() string {
	return i.skill.Name
}

type Page struct {
	list         list.Model
	systemNotice string
	width        int
	height       int
	activeSkill  string
	pendingSkill string
}

func NewPage(skills []skillstore.Skill) *Page {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	l := list.New(nil, delegate, 0, 0)
	l.Title = "Available skills"
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)

	page := &Page{list: l}
	page.SetSkills(skills)
	return page
}

func (p *Page) Title() string {
	return "Skills"
}

func (p *Page) Placeholder() string {
	return "Use arrows to select, Enter to load, r to refresh"
}

func (p *Page) Content(width int) string {
	detail := p.renderSelectedSkill(width)
	if detail == "" {
		return p.list.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, p.list.View(), "", detail)
}

func (p *Page) HandleInput(text string) tea.Cmd {
	p.systemNotice = "Use arrow keys and Enter to load a skill."
	p.list.NewStatusMessage(p.systemNotice)
	return nil
}

func (p *Page) ApplyUpdate(shared.PageUpdatedMsg) {}

func (p *Page) SystemMessage(text string) {
	p.systemNotice = text
	p.list.NewStatusMessage(text)
}

func (p *Page) SetSkills(skills []skillstore.Skill) {
	items := make([]list.Item, 0, len(skills))
	selectedName := p.SelectedSkillName()
	selectedIndex := 0
	for i, skill := range skills {
		items = append(items, item{skill: skill, badge: p.skillBadge(skill.Name)})
		if skill.Name == selectedName {
			selectedIndex = i
		}
	}
	p.list.SetItems(items)
	if len(items) > 0 {
		p.list.Select(selectedIndex)
	}
}

func (p *Page) SetActiveSkills(activeSkill, pendingSkill string) {
	p.activeSkill = strings.TrimSpace(activeSkill)
	p.pendingSkill = strings.TrimSpace(pendingSkill)
	currentItems := p.list.Items()
	if len(currentItems) == 0 {
		return
	}
	refreshed := make([]skillstore.Skill, 0, len(currentItems))
	for _, raw := range currentItems {
		entry, ok := raw.(item)
		if !ok {
			continue
		}
		refreshed = append(refreshed, entry.skill)
	}
	p.SetSkills(refreshed)
}

func (p *Page) SetSize(width, height int) {
	p.width = width
	p.height = height
	listHeight := height - 9
	if listHeight < 6 {
		listHeight = height
	}
	p.list.SetSize(width, listHeight)
}

func (p *Page) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return cmd
}

func (p *Page) SelectedSkillName() string {
	selected, ok := p.list.SelectedItem().(item)
	if !ok {
		return ""
	}
	return selected.skill.Name
}

func (p *Page) renderSelectedSkill(width int) string {
	selected, ok := p.list.SelectedItem().(item)
	if !ok {
		return ""
	}

	skill := selected.skill
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")).Render(skill.Name),
		fmt.Sprintf("scope: %s  context: %s", skill.Scope, skill.Context),
	}
	if badge := p.skillBadge(skill.Name); badge != "" {
		lines = append(lines, "status: "+badge)
	}
	if strings.TrimSpace(skill.WhenToUse) != "" {
		lines = append(lines, "when: "+skill.WhenToUse)
	}
	if len(skill.AllowedTools) > 0 {
		lines = append(lines, "allowed tools: "+strings.Join(skill.AllowedTools, ", "))
	} else {
		lines = append(lines, "allowed tools: all")
	}
	if strings.TrimSpace(skill.Path) != "" {
		lines = append(lines, "path: "+skill.Path)
	}

	cardWidth := width - 4
	if cardWidth < 20 {
		cardWidth = 20
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(cardWidth).
		Render(strings.Join(lines, "\n"))
}

func (p *Page) skillBadge(name string) string {
	switch {
	case p.pendingSkill != "" && strings.EqualFold(p.pendingSkill, name):
		return "pending fork"
	case p.activeSkill != "" && strings.EqualFold(p.activeSkill, name):
		return "active"
	default:
		return ""
	}
}
