package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	chatui "go-cli/internal/modules/chat/ui"
)

var (
	toolHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244"))
	toolLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)
	toolBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	writeFileHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Bold(true)
	writeFilePatchStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("22")).
				Foreground(lipgloss.Color("255")).
				Padding(0, 1)
	writeFilePlusStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("48")).
				Bold(true)
	editFileDeleteStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("203")).
				Bold(true)
)

const toolPreviewLineLimit = 12

func formatArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	data, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func formatToolCall(toolName string, args map[string]any) string {
	if toolName == "write_text_file" {
		path := stringArg(args, "path")
		content := stringArg(args, "content")
		return strings.Join([]string{
			fmt.Sprintf("tool: %s", toolName),
			fmt.Sprintf("target: %s", path),
			"content:",
			content,
		}, "\n")
	}
	if toolName == "edit_file_lines" {
		path := stringArg(args, "path")
		newLines := stringArg(args, "new_lines")
		return strings.Join([]string{
			fmt.Sprintf("tool: %s", toolName),
			fmt.Sprintf("target: %s", path),
			fmt.Sprintf("range: %d-%d", intArg(args, "start_line"), intArg(args, "end_line")),
			"content:",
			newLines,
		}, "\n")
	}

	target := formatToolTarget(toolName, args)
	if target == "" {
		return fmt.Sprintf("tool: %s", toolName)
	}
	return strings.Join([]string{
		fmt.Sprintf("tool: %s", toolName),
		fmt.Sprintf("target: %s", target),
	}, "\n")
}

func renderToolCard(width int, message chatui.Message) string {
	lines := strings.Split(strings.TrimSpace(message.Content), "\n")
	if len(lines) == 0 {
		return ""
	}
	expanded := isToolExpanded(message)

	title := "Tool"
	body := ""
	if name := strings.TrimSpace(strings.TrimPrefix(lines[0], "tool:")); name != "" && name != lines[0] {
		title = name
	} else {
		body = strings.TrimSpace(lines[0])
	}

	if len(lines) > 1 {
		target := strings.TrimSpace(strings.TrimPrefix(lines[1], "target:"))
		if target != "" && target != lines[1] {
			body = target
		}
	}

	if message.ToolName == "write_text_file" || title == "write_text_file" {
		return renderWriteTextFileCard(width, body, extractWriteTextFileContent(lines), expanded)
	}
	if message.ToolName == "edit_file_lines" || title == "edit_file_lines" {
		oldLines, _ := message.Meta["old_lines"].(string)
		return renderEditFileLinesCard(width, body, extractToolMetadata(lines, "range:"), oldLines, extractWriteTextFileContent(lines), expanded)
	}

	card := []string{
		toolLabelStyle.Render(title),
	}
	if body != "" {
		card = append(card, lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(body))
	}

	return lipgloss.NewStyle().Width(max(1, width-toolBodyStyle.GetHorizontalFrameSize())).Render(strings.Join(card, "\n"))
}

func renderWriteTextFileCard(width int, target, content string, expanded bool) string {
	header := writeFileHeaderStyle.Render("write_text_file")
	if target != "" {
		header = header + "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(target)
	}

	patchWidth := max(1, width-toolBodyStyle.GetHorizontalFrameSize()-writeFilePatchStyle.GetHorizontalFrameSize())
	patch, hint := renderAddedContentPatch(patchWidth, content, expanded)

	body := header + "\n" + patch
	if hint != "" {
		body += "\n" + toolHintStyle.Render(hint)
	}

	return lipgloss.NewStyle().
		Width(max(1, width-toolBodyStyle.GetHorizontalFrameSize())).
		Render(body)
}

func renderEditFileLinesCard(width int, target, lineRange, oldContent, content string, expanded bool) string {
	header := writeFileHeaderStyle.Render("edit_file_lines")
	if target != "" {
		header += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(target)
	}
	if lineRange != "" {
		header += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("lines "+lineRange)
	}

	patchWidth := max(1, width-toolBodyStyle.GetHorizontalFrameSize()-writeFilePatchStyle.GetHorizontalFrameSize())
	patch, hint := renderEditPatch(patchWidth, lineRange, oldContent, content, expanded)

	body := header + "\n" + patch
	if hint != "" {
		body += "\n" + toolHintStyle.Render(hint)
	}

	return lipgloss.NewStyle().
		Width(max(1, width-toolBodyStyle.GetHorizontalFrameSize())).
		Render(body)
}

func renderAddedContentPatch(width int, content string, expanded bool) (string, string) {
	lines, truncated := contentLines(content, expanded, toolPreviewLineLimit)
	if len(lines) == 0 {
		lines = []string{"+ "}
	}

	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered = append(rendered, renderPatchEntry(width, "+", writeFilePlusStyle, line))
	}

	return strings.Join(rendered, "\n"), expandHint(truncated, expanded)
}

func renderEditPatch(width int, lineRange, oldContent, content string, expanded bool) (string, string) {
	rendered := make([]string, 0, 16)
	oldLines, oldTruncated := contentLines(oldContent, expanded, toolPreviewLineLimit)
	if len(oldLines) == 0 && lineRange != "" {
		rendered = append(rendered, renderPatchEntry(width, "-", editFileDeleteStyle, "lines "+lineRange))
	}
	for _, line := range oldLines {
		rendered = append(rendered, renderPatchEntry(width, "-", editFileDeleteStyle, line))
	}

	lines, newTruncated := contentLines(content, expanded, toolPreviewLineLimit)
	for _, line := range lines {
		rendered = append(rendered, renderPatchEntry(width, "+", writeFilePlusStyle, line))
	}

	if len(rendered) == 0 {
		rendered = append(rendered, renderPatchEntry(width, "+", writeFilePlusStyle, ""))
	}

	return strings.Join(rendered, "\n"), expandHint(oldTruncated || newTruncated, expanded)
}

func renderPatchEntry(width int, marker string, markerStyle lipgloss.Style, content string) string {
	if width <= 0 {
		width = 1
	}

	lineText := marker + " " + content
	visibleWidth := lipgloss.Width(lineText)
	if visibleWidth < width {
		lineText += strings.Repeat(" ", width-visibleWidth)
	}

	markerFG := "38;5;48"
	if marker == "-" {
		markerFG = "38;5;203"
	}

	if lineText == "" {
		lineText = " "
	}

	return fmt.Sprintf(
		"\x1b[48;5;22;%s;1m%s\x1b[48;5;22;38;5;255m%s\x1b[0m",
		markerFG,
		string([]rune(lineText)[0]),
		string([]rune(lineText)[1:]),
	)
}

func contentLines(content string, expanded bool, lineLimit int) ([]string, bool) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return nil, false
	}

	lines := strings.Split(content, "\n")
	if expanded || lineLimit <= 0 || len(lines) <= lineLimit {
		return lines, false
	}
	return lines[:lineLimit], true
}

func expandHint(truncated, expanded bool) string {
	if truncated {
		return "Press e in viewport to expand"
	}
	if expanded {
		return "Press e in viewport to collapse"
	}
	return ""
}

func extractWriteTextFileContent(lines []string) string {
	for i, line := range lines {
		if strings.TrimSpace(line) == "content:" {
			if i+1 >= len(lines) {
				return ""
			}
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return ""
}

func extractOldLines(result string) string {
	parts := strings.Split(result, "\nold_lines:\n")
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimRight(parts[1], "\n")
}

func extractToolMetadata(lines []string, prefix string) string {
	for _, line := range lines {
		if value := strings.TrimSpace(strings.TrimPrefix(line, prefix)); value != "" && value != line {
			return value
		}
	}
	return ""
}

func buildToolMeta(toolName string, args map[string]any, oldLines string) map[string]any {
	meta := map[string]any{
		"path":     stringArg(args, "path"),
		"expanded": false,
	}
	if toolName == "edit_file_lines" {
		meta["start_line"] = intArg(args, "start_line")
		meta["end_line"] = intArg(args, "end_line")
		if oldLines != "" {
			meta["old_lines"] = oldLines
		}
	}
	if toolName == "write_text_file" {
		meta["content"] = stringArg(args, "content")
	}
	return meta
}

func isToolExpanded(message chatui.Message) bool {
	if message.Meta == nil {
		return false
	}
	expanded, _ := message.Meta["expanded"].(bool)
	return expanded
}

func (p *Page) applyToolResultToUI(toolCallID, toolName, result string) {
	if toolName != "edit_file_lines" {
		return
	}
	oldLines := extractOldLines(result)
	if oldLines == "" {
		return
	}
	for i := len(p.messages) - 1; i >= 0; i-- {
		if p.messages[i].Type != chatui.TypeTool || p.messages[i].ToolCallID != toolCallID {
			continue
		}
		if p.messages[i].Meta == nil {
			p.messages[i].Meta = map[string]any{}
		}
		p.messages[i].Meta["old_lines"] = oldLines
		return
	}
}

func (p *Page) ToggleLastToolCard() bool {
	for i := len(p.messages) - 1; i >= 0; i-- {
		if p.messages[i].Type != chatui.TypeTool {
			continue
		}
		return p.toggleToolCardAtIndex(i)
	}
	return false
}

func (p *Page) toggleToolCardAtIndex(index int) bool {
	if index < 0 || index >= len(p.messages) {
		return false
	}
	if p.messages[index].Meta == nil {
		p.messages[index].Meta = map[string]any{}
	}
	expanded, _ := p.messages[index].Meta["expanded"].(bool)
	p.messages[index].Meta["expanded"] = !expanded
	p.markDirty()
	return true
}

func intArg(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func formatToolTarget(toolName string, args map[string]any) string {
	switch toolName {
	case "read_file", "write_text_file", "edit_file_lines":
		return stringArg(args, "path")
	case "grep_files":
		path := stringArg(args, "path")
		pattern := stringArg(args, "pattern")
		if path != "" && pattern != "" {
			return path + " :: " + pattern
		}
		if path != "" {
			return path
		}
		return pattern
	case "execute_shell_command":
		path := stringArg(args, "path")
		command := stringArg(args, "command")
		if path != "" && command != "" {
			return path + " :: " + command
		}
		if command != "" {
			return command
		}
		return path
	case "active_model":
		return "current model"
	case "time_now":
		return "local clock"
	default:
		if path := stringArg(args, "path"); path != "" {
			return path
		}
		if name := stringArg(args, "file"); name != "" {
			return name
		}
		return ""
	}
}

func stringArg(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}
