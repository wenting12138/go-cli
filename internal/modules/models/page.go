package models

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/config"
	"go-cli/internal/modules/shared"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	bodyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

type Page struct {
	models       []config.ModelConfig
	currentModel string
	systemNotice string
}

func NewPage(models []config.ModelConfig, currentModel string) *Page {
	return &Page{
		models:       append([]config.ModelConfig(nil), models...),
		currentModel: currentModel,
	}
}

func (p *Page) Title() string {
	return "Models"
}

func (p *Page) Placeholder() string {
	return "Use /models <id> to switch model"
}

func (p *Page) Content(width int) string {
	lines := []string{
		titleStyle.Render("Available models"),
		bodyStyle.Width(max(20, width)).Render("Use /models <id> to switch the active model."),
	}

	for _, model := range p.models {
		lines = append(lines, "")
		prefix := " "
		if model.ID == p.currentModel {
			prefix = "*"
		}
		lines = append(lines, bodyStyle.Width(max(20, width)).Render(
			fmt.Sprintf("%s [%s] %s/%s", prefix, model.ID, model.Provider, model.Name),
		))
		lines = append(lines, hintStyle.Width(max(20, width)).Render(
			fmt.Sprintf("context=%d max_tokens=%d", model.ContextWindow, model.MaxTokens),
		))
	}

	if p.systemNotice != "" {
		lines = append(lines, "", hintStyle.Width(max(20, width)).Render(p.systemNotice))
	}

	return strings.Join(lines, "\n")
}

func (p *Page) HandleInput(text string) tea.Cmd {
	p.systemNotice = "This page does not accept direct input. Use /models <id>."
	return nil
}

func (p *Page) ApplyUpdate(shared.PageUpdatedMsg) {}

func (p *Page) SystemMessage(text string) {
	p.systemNotice = text
}

func (p *Page) SetCurrentModel(id string) {
	p.currentModel = id
	p.systemNotice = "Model switched. Returned to chat."
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
