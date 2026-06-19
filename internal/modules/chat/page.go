package chat

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/glamour/v2"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/config"
	"go-cli/internal/modules/chat/domain"
	chatui "go-cli/internal/modules/chat/ui"
	"go-cli/internal/modules/shared"
)

const PageName = "chat"

var (
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("86")).
			PaddingLeft(1)
	assistantStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(lipgloss.Color("63")).
			PaddingLeft(1).
			PaddingLeft(1)
	thinkingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("244")).
			PaddingLeft(1)
	thinkingBoxStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("248")).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("241")).
				Padding(0, 1)
	thinkingTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("246")).
				Bold(true)
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("240")).
			PaddingLeft(1)
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

type Page struct {
	messages         []chatui.Message
	systemPrompt     string
	modelMessages    []domain.Message
	model            config.ModelConfig
	sessionID        string
	streamIndex      int
	thinkingIndex    int
	modelStreamIndex int
	streamRequestID  string
	cachedWidth      int
	cachedContent    string
	frozenRendered   []string
	frozenWidth      int
	dirty            bool
}

func NewPage(model config.ModelConfig) *Page {
	return &Page{
		model:            model,
		streamIndex:      -1,
		thinkingIndex:    -1,
		modelStreamIndex: -1,
		dirty:            true,
		messages: []chatui.Message{
			chatui.TextMessage(chatui.TypeSystem, startupLogo()),
		},
	}
}

func (p *Page) Title() string {
	return "Go CLI Chat"
}

func (p *Page) Placeholder() string {
	return "Type a message or use /resume or /models"
}

func (p *Page) Content(width int) string {
	if !p.dirty && p.cachedWidth == width {
		return p.cachedContent
	}

	if p.frozenWidth != width || len(p.frozenRendered) != len(p.messages) {
		p.rebuildFrozenRendered(width)
	}

	var lines []string
	for i, message := range p.messages {
		if i > 0 && needsReplyDivider(p.messages[i-1], message) {
			lines = append(lines, renderReplyDivider(width))
		}
		lines = append(lines, p.renderMessage(width, i, message))
	}
	p.cachedWidth = width
	p.cachedContent = strings.Join(lines, "\n\n")
	p.dirty = false
	return p.cachedContent
}

func (p *Page) HandleInput(text string) tea.Cmd {
	if p.sessionID == "" {
		p.sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeUser, text))
	p.modelMessages = append(p.modelMessages, domain.TextMessage("user", text))
	p.markDirty()
	return nil
}

func (p *Page) ApplyUpdate(msg shared.PageUpdatedMsg) {
	message, ok := msg.Payload.(chatui.Message)
	if !ok {
		return
	}
	p.messages = append(p.messages, message)
	p.markDirty()
}

func (p *Page) SystemMessage(text string) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, text))
	p.markDirty()
}

func (p *Page) LoadSession(session domain.Session) {
	p.sessionID = session.ID
	p.messages = append([]chatui.Message{
		chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Resumed session %s: %s", session.ID, session.Title)),
	}, toUIMessages(session.Messages)...)
	p.markDirty()
}

func (p *Page) RestoreSession(sessionID, title string, uiMessages []chatui.Message, modelMessages []domain.Message) {
	p.sessionID = sessionID
	p.messages = append([]chatui.Message{
		chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Resumed session %s: %s", sessionID, title)),
	}, chatui.CloneMessages(uiMessages)...)
	p.modelMessages = cloneMessages(modelMessages)
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) SetSystemPrompt(systemPrompt string) {
	p.systemPrompt = strings.TrimSpace(systemPrompt)
}

func (p *Page) SystemPrompt() string {
	return p.systemPrompt
}

func (p *Page) SetModel(model config.ModelConfig) {
	p.model = model
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, fmt.Sprintf("Switched model to %s (%s/%s), context=%s.", model.ID, model.Provider, model.Name, formatContextWindow(model.ContextWindow))))
	p.markDirty()
}

func (p *Page) AppendStream(chunk string) {
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		return
	}
	p.messages[p.streamIndex].Content += chunk
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].Content += chunk
	}
	p.markDirty()
}

func (p *Page) AppendReasoning(chunk string) {
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		return
	}
	p.modelMessages[p.modelStreamIndex].Reasoning += chunk
	p.markDirty()
}

func (p *Page) SetReasoningSignature(signature string) {
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		return
	}
	p.modelMessages[p.modelStreamIndex].ReasoningSignature = signature
	p.markDirty()
}

func (p *Page) FinishStream() {
	if p.thinkingIndex >= 0 && p.thinkingIndex < len(p.messages) {
		p.messages[p.thinkingIndex].Content = strings.TrimSpace(p.messages[p.thinkingIndex].Content)
	}
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		p.streamIndex = -1
		p.thinkingIndex = -1
		p.modelStreamIndex = -1
		p.streamRequestID = ""
		return
	}
	p.messages[p.streamIndex].Content = strings.TrimSpace(p.messages[p.streamIndex].Content)
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].Content = strings.TrimSpace(p.modelMessages[p.modelStreamIndex].Content)
	}
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) FailStream(err string) {
	if p.thinkingIndex >= 0 && p.thinkingIndex < len(p.messages) && strings.TrimSpace(p.messages[p.thinkingIndex].Content) == "" {
		p.messages = append(p.messages[:p.thinkingIndex], p.messages[p.thinkingIndex+1:]...)
		if p.streamIndex > p.thinkingIndex {
			p.streamIndex--
		}
	}
	if p.streamIndex >= 0 && p.streamIndex < len(p.messages) && p.messages[p.streamIndex].Content == "" {
		p.messages = append(p.messages[:p.streamIndex], p.messages[p.streamIndex+1:]...)
	}
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, err))
	p.markDirty()
}

func (p *Page) SnapshotMessages() []domain.Message {
	return cloneMessages(p.modelMessages)
}

func (p *Page) UISnapshotMessages() []chatui.Message {
	return chatui.CloneMessages(p.messages)
}

func (p *Page) BeginRequest(requestID string) {
	p.streamRequestID = requestID
	p.streamIndex = -1
	p.thinkingIndex = -1
}

func (p *Page) BeginAssistantStream(requestID string) {
	if p.streamRequestID == "" {
		p.streamRequestID = requestID
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeAssistant, ""))
	p.frozenRendered = append(p.frozenRendered, "")
	p.streamIndex = len(p.messages) - 1
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		p.modelMessages = append(p.modelMessages, domain.TextMessage("assistant", ""))
		p.modelStreamIndex = len(p.modelMessages) - 1
	}
	p.streamRequestID = requestID
	p.markDirty()
}

func (p *Page) BeginThinking(requestID string) {
	if p.streamRequestID == "" {
		p.streamRequestID = requestID
	}
	if p.thinkingIndex >= 0 && p.thinkingIndex < len(p.messages) {
		return
	}
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		p.modelMessages = append(p.modelMessages, domain.TextMessage("assistant", ""))
		p.modelStreamIndex = len(p.modelMessages) - 1
	}
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeThinking, ""))
	p.frozenRendered = append(p.frozenRendered, "")
	p.thinkingIndex = len(p.messages) - 1
	p.streamRequestID = requestID
	p.markDirty()
}

func (p *Page) AppendThinking(chunk string) {
	if p.thinkingIndex < 0 || p.thinkingIndex >= len(p.messages) {
		return
	}
	p.messages[p.thinkingIndex].Content += chunk
	p.markDirty()
}

func (p *Page) HasActiveRequest(requestID string) bool {
	return p.streamRequestID != "" && p.streamRequestID == requestID
}

func (p *Page) HasActiveStream(requestID string) bool {
	return p.HasActiveRequest(requestID) && p.streamIndex >= 0
}

func (p *Page) HasPendingStream() bool {
	return p.streamRequestID != ""
}

func (p *Page) ActiveRequestID() string {
	return p.streamRequestID
}

func (p *Page) SessionID() string {
	return p.sessionID
}

func (p *Page) AppendToolCall(toolCallID, toolName string, args map[string]any) {
	p.messages = append(p.messages, chatui.ToolMessage(toolCallID, toolName, formatToolCall(toolName, args), buildToolMeta(toolName, args, "")))
	p.modelMessages = append(p.modelMessages, domain.ToolCallMessage(toolCallID, toolName, args))
	p.markDirty()
}

func (p *Page) AppendToolResult(toolCallID, toolName, result string) {
	p.modelMessages = append(p.modelMessages, domain.ToolResultMessage(toolCallID, toolName, result))
	p.applyToolResultToUI(toolCallID, toolName, result)
	p.markDirty()
}

func (p *Page) ApplyCompact(boundary, summary domain.Message, restoredFiles, preservedTail []domain.Message) {
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, boundary.Content))
	p.messages = append(p.messages, chatui.TextMessage(chatui.TypeSystem, "Compaction summary saved to model context."))
	p.modelMessages = append([]domain.Message{boundary, summary}, cloneMessages(restoredFiles)...)
	p.modelMessages = append(p.modelMessages, cloneMessages(preservedTail)...)
	p.streamIndex = -1
	p.thinkingIndex = -1
	p.modelStreamIndex = -1
	p.streamRequestID = ""
	p.markDirty()
}

func (p *Page) renderMessage(width int, index int, message chatui.Message) string {
	contentWidth := max(20, width)
	content := lipgloss.NewStyle().Width(contentWidth).Render(message.Content)
	if message.Type == chatui.TypeAssistant {
		renderWidth := max(1, contentWidth-assistantStyle.GetHorizontalFrameSize())
		if index == p.streamIndex {
			content = assistantStyle.Width(renderWidth).Render(message.Content)
		} else if index < len(p.frozenRendered) && p.frozenRendered[index] != "" {
			content = assistantStyle.Width(renderWidth).Render(p.frozenRendered[index])
		} else {
			content = assistantStyle.Width(renderWidth).Render(renderAssistantMarkdown(renderWidth, message.Content))
		}
	}
	if message.Type == chatui.TypeThinking {
		if index == p.thinkingIndex {
			content = thinkingStyle.Width(max(1, contentWidth-thinkingStyle.GetHorizontalFrameSize())).Render(message.Content)
		} else {
			content = thinkingBoxStyle.Width(max(1, contentWidth-thinkingBoxStyle.GetHorizontalFrameSize())).Render(renderThinkingCollapsed(contentWidth, message.Content))
		}
	}
	if message.Type == chatui.TypeTool {
		content = toolBodyStyle.Width(max(1, contentWidth-toolBodyStyle.GetHorizontalFrameSize())).Render(renderToolCard(contentWidth, message))
	}
	if message.Type == chatui.TypeUser {
		content = userStyle.Width(max(1, contentWidth-userStyle.GetHorizontalFrameSize())).Render(message.Content)
	}
	if message.Type == chatui.TypeSystem {
		content = systemStyle.Width(max(1, contentWidth-systemStyle.GetHorizontalFrameSize())).Render(message.Content)
	}

	return content
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatContextWindow(size int) string {
	if size%1000 == 0 {
		return fmt.Sprintf("%dk", size/1000)
	}
	return fmt.Sprintf("%.1fk", float64(size)/1000)
}

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

func formatToolResult(toolCallID, toolName, result string) string {
	return strings.Join([]string{
		fmt.Sprintf("result: %s", toolName),
		fmt.Sprintf("id: %s", toolCallID),
		fmt.Sprintf("content: %s", result),
	}, "\n")
}

func renderAssistantMarkdown(width int, content string) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dracula"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	out, err := renderer.Render(content)
	if err != nil {
		return lipgloss.NewStyle().Width(width).Render(content)
	}

	return strings.TrimRight(out, "\n")
}

func renderToolCard(width int, message chatui.Message) string {
	lines := strings.Split(strings.TrimSpace(message.Content), "\n")
	if len(lines) == 0 {
		return ""
	}

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
		return renderWriteTextFileCard(width, body, extractWriteTextFileContent(lines))
	}
	if message.ToolName == "edit_file_lines" || title == "edit_file_lines" {
		oldLines, _ := message.Meta["old_lines"].(string)
		return renderEditFileLinesCard(width, body, extractToolMetadata(lines, "range:"), oldLines, extractWriteTextFileContent(lines))
	}

	card := []string{
		toolLabelStyle.Render(title),
	}
	if body != "" {
		card = append(card, lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(body))
	}

	return lipgloss.NewStyle().Width(max(1, width-toolBodyStyle.GetHorizontalFrameSize())).Render(strings.Join(card, "\n"))
}

func renderWriteTextFileCard(width int, target, content string) string {
	header := writeFileHeaderStyle.Render("write_text_file")
	if target != "" {
		header = header + "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(target)
	}

	patchWidth := max(1, width-toolBodyStyle.GetHorizontalFrameSize()-writeFilePatchStyle.GetHorizontalFrameSize())
	patch := renderAddedContentPatch(patchWidth, content)

	return lipgloss.NewStyle().
		Width(max(1, width-toolBodyStyle.GetHorizontalFrameSize())).
		Render(header + "\n" + patch)
}

func renderEditFileLinesCard(width int, target, lineRange, oldContent, content string) string {
	header := writeFileHeaderStyle.Render("edit_file_lines")
	if target != "" {
		header += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(target)
	}
	if lineRange != "" {
		header += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("@@ "+lineRange+" @@")
	}

	patchWidth := max(1, width-toolBodyStyle.GetHorizontalFrameSize()-writeFilePatchStyle.GetHorizontalFrameSize())
	patch := renderEditPatch(patchWidth, lineRange, oldContent, content)

	return lipgloss.NewStyle().
		Width(max(1, width-toolBodyStyle.GetHorizontalFrameSize())).
		Render(header + "\n" + patch)
}

func renderAddedContentPatch(width int, content string) string {
	lines := previewContentLines(content, 12)
	if len(lines) == 0 {
		lines = []string{"+ "}
	}

	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		rendered = append(rendered, writeFilePlusStyle.Render("+")+" "+line)
	}

	return writeFilePatchStyle.Width(width).Render(strings.Join(rendered, "\n"))
}

func renderEditPatch(width int, lineRange, oldContent, content string) string {
	rendered := make([]string, 0, 16)
	oldLines := previewContentLines(oldContent, 8)
	if len(oldLines) == 0 && lineRange != "" {
		rendered = append(rendered, editFileDeleteStyle.Render("-")+" lines "+lineRange)
	}
	for _, line := range oldLines {
		rendered = append(rendered, editFileDeleteStyle.Render("-")+" "+line)
	}

	lines := previewContentLines(content, 12)
	for _, line := range lines {
		rendered = append(rendered, writeFilePlusStyle.Render("+")+" "+line)
	}

	if len(rendered) == 0 {
		rendered = append(rendered, writeFilePlusStyle.Render("+")+" ")
	}

	return writeFilePatchStyle.Width(width).Render(strings.Join(rendered, "\n"))
}

func previewContentLines(content string, maxLines int) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return nil
	}

	rawLines := strings.Split(content, "\n")
	if len(rawLines) > maxLines {
		rawLines = append(rawLines[:maxLines], "...")
	}
	return rawLines
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
		"path": stringArg(args, "path"),
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

func renderThinkingCollapsed(width int, content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return thinkingTitleStyle.Render("Thinking")
	}

	lines := strings.Split(trimmed, "\n")
	preview := make([]string, 0, 4)
	preview = append(preview, thinkingTitleStyle.Render("Thinking"))

	lineLimit := 3
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		preview = append(preview, line)
		if len(preview) > lineLimit {
			break
		}
	}

	if len(preview) == 1 {
		preview = append(preview, trimmed)
	}

	if len(preview) > lineLimit+1 {
		preview = preview[:lineLimit+1]
	}

	if len(lines) > lineLimit {
		preview = append(preview, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("..."))
	}

	body := strings.Join(preview, "\n")
	return lipgloss.NewStyle().Width(max(1, width-thinkingBoxStyle.GetHorizontalFrameSize())).Render(body)
}

func (p *Page) SessionTitle() string {
	for _, message := range p.modelMessages {
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			text := strings.TrimSpace(message.Content)
			if len(text) > 48 {
				return text[:48] + "..."
			}
			return text
		}
	}
	return "Untitled Session"
}

func cloneMessages(messages []domain.Message) []domain.Message {
	out := make([]domain.Message, 0, len(messages))
	for _, message := range messages {
		cloned := message
		if len(message.Args) > 0 {
			cloned.Args = make(map[string]any, len(message.Args))
			for key, value := range message.Args {
				cloned.Args[key] = value
			}
		}
		out = append(out, cloned)
	}
	return out
}

func toUIMessages(messages []domain.Message) []chatui.Message {
	out := make([]chatui.Message, 0, len(messages))
	for _, message := range messages {
		switch message.Type {
		case domain.MessageTypeCompact:
			out = append(out, chatui.TextMessage(chatui.TypeSystem, message.Content))
		case domain.MessageTypeToolCall:
			out = append(out, chatui.ToolMessage(message.ToolCallID, message.ToolName, formatToolCall(message.ToolName, message.Args), buildToolMeta(message.ToolName, message.Args, "")))
		default:
			out = append(out, chatui.TextMessage(toUIType(message.Role), message.Content))
		}
	}
	return out
}

func toUIType(role string) string {
	switch role {
	case "user":
		return chatui.TypeUser
	case "assistant":
		return chatui.TypeAssistant
	default:
		return chatui.TypeSystem
	}
}

func (p *Page) markDirty() {
	p.dirty = true
}

func (p *Page) rebuildFrozenRendered(width int) {
	p.frozenWidth = width
	p.frozenRendered = make([]string, len(p.messages))
	contentWidth := max(20, width)

	for i, message := range p.messages {
		if message.Type != chatui.TypeAssistant || i == p.streamIndex {
			continue
		}
		p.frozenRendered[i] = renderAssistantMarkdown(contentWidth, message.Content)
	}
}

func startupLogo() string {
	return strings.TrimSpace(`
   ____        _______ _      ___
  / ___| ___  / / ____| |    |_ _|
 | |  _ / _ \/ / |    | |     | |
 | |_| | (_) / /| |___ | |___  | |
  \____|\___/_/  \____||_____|___|

  /models   choose model
  /resume   restore session
  /tools    inspect tools
`)
}

func needsReplyDivider(prev, current chatui.Message) bool {
	return (prev.Type == chatui.TypeUser && current.Type == chatui.TypeAssistant) ||
		(prev.Type == chatui.TypeAssistant && current.Type == chatui.TypeUser)
}

func renderReplyDivider(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Render(strings.Repeat("─", max(8, width)))
}
