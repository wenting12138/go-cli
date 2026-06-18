package tools

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/agent/react"
	"go-cli/internal/modules/shared"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	nameStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	bodyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	codeStyle  = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
)

type Page struct {
	specs        []react.ToolSpec
	systemNotice string
}

func NewPage(specs []react.ToolSpec) *Page {
	return &Page{specs: append([]react.ToolSpec(nil), specs...)}
}

func (p *Page) Title() string {
	return "Tools"
}

func (p *Page) Placeholder() string {
	return "Use /chat to return"
}

func (p *Page) Content(width int) string {
	textWidth := max(20, width)
	codeWidth := max(20, width-4)

	lines := []string{
		titleStyle.Render("Available tools"),
		hintStyle.Width(textWidth).Render("These are the tool definitions currently exposed to the model."),
	}

	for _, spec := range p.specs {
		lines = append(lines, "")
		lines = append(lines, nameStyle.Render(spec.Name))
		lines = append(lines, bodyStyle.Width(textWidth).Render(spec.Description))
		lines = append(lines, codeStyle.Width(codeWidth).Render(formatSchema(spec.Parameters)))
	}

	if p.systemNotice != "" {
		lines = append(lines, "", hintStyle.Width(textWidth).Render(p.systemNotice))
	}

	return strings.Join(lines, "\n")
}

func (p *Page) HandleInput(text string) tea.Cmd {
	p.systemNotice = "This page is read-only. Use /chat to return."
	return nil
}

func (p *Page) ApplyUpdate(shared.PageUpdatedMsg) {}

func (p *Page) SystemMessage(text string) {
	p.systemNotice = text
}

func formatSchema(schema react.ParameterSchema) string {
	data, err := json.MarshalIndent(schema.ToMap(), "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
