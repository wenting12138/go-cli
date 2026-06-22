package chat

import (
	"fmt"
	"strings"
	"time"

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
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			BorderLeft(true).
			BorderForeground(lipgloss.Color("240")).
			PaddingLeft(1)
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
	cachedPrefix     renderCache
	frozenRendered   []string
	frozenWidth      int
	toolRenderCache  map[int]toolRenderCache
	toolHits         []toolHit
	dirty            bool
}

type renderCache struct {
	width     int
	tailStart int
	content   string
	lineCount int
	toolHits  []toolHit
	dirty     bool
}

type toolHit struct {
	messageIndex int
	toolName     string
	startLine    int
	endLine      int
}

type toolRenderCache struct {
	width    int
	key      string
	rendered string
}

type ToolAction string

const (
	ToolActionNone ToolAction = ""
)

func NewPage(model config.ModelConfig) *Page {
	return &Page{
		model:            model,
		streamIndex:      -1,
		thinkingIndex:    -1,
		modelStreamIndex: -1,
		dirty:            true,
		cachedPrefix: renderCache{
			tailStart: -1,
			dirty:     true,
		},
		toolRenderCache: map[int]toolRenderCache{},
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

	if tailStart, ok := p.activeTailStart(); ok {
		p.renderWithActiveTail(width, tailStart)
	} else {
		p.renderAll(width)
	}
	p.cachedWidth = width
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

func (p *Page) RefreshModel(model config.ModelConfig) {
	p.model = model
}

func (p *Page) AppendStream(chunk string) {
	if p.streamIndex < 0 || p.streamIndex >= len(p.messages) {
		return
	}
	p.messages[p.streamIndex].Content += chunk
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].Content += chunk
	}
	p.markTailDirty()
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
	p.freezeAssistantMessage(p.streamIndex)
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
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		current := p.modelMessages[p.modelStreamIndex]
		if strings.TrimSpace(current.Content) != "" || strings.TrimSpace(current.Reasoning) != "" {
			p.modelStreamIndex = -1
		}
	}
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		p.modelMessages = append(p.modelMessages, domain.TextMessage("assistant", ""))
		p.modelStreamIndex = len(p.modelMessages) - 1
	}
	p.streamRequestID = requestID
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

func (p *Page) EnsureSessionID() string {
	if p.sessionID == "" {
		p.sessionID = fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return p.sessionID
}

func (p *Page) AppendToolCall(toolCallID, toolName string, args map[string]any) {
	p.messages = append(p.messages, chatui.ToolMessage(toolCallID, toolName, formatToolCall(toolName, args), buildToolMeta(toolName, args, "")))
	p.modelMessages = append(p.modelMessages, domain.ToolCallMessage(toolCallID, toolName, args))
	p.modelStreamIndex = -1
	p.markDirty()
}

func (p *Page) AppendToolResult(toolCallID, toolName, result string) {
	p.modelMessages = append(p.modelMessages, domain.ToolResultMessage(toolCallID, toolName, result))
	p.applyToolResultToUI(toolCallID, toolName, result)
	p.markDirty()
}

func (p *Page) ApplyAssistantBlocks(blocks []domain.AssistantContentBlock) {
	if len(blocks) == 0 {
		return
	}
	if p.modelStreamIndex < 0 || p.modelStreamIndex >= len(p.modelMessages) {
		p.modelMessages = append(p.modelMessages, domain.TextMessage("assistant", ""))
		p.modelStreamIndex = len(p.modelMessages) - 1
	}
	if p.modelStreamIndex >= 0 && p.modelStreamIndex < len(p.modelMessages) {
		p.modelMessages[p.modelStreamIndex].AssistantBlocks = cloneAssistantBlocks(blocks)
		return
	}
	for i := len(p.modelMessages) - 1; i >= 0; i-- {
		if p.modelMessages[i].Role != "assistant" || p.modelMessages[i].Type != domain.MessageTypeText {
			continue
		}
		p.modelMessages[i].AssistantBlocks = cloneAssistantBlocks(blocks)
		return
	}
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
		content = toolBodyStyle.Width(max(1, contentWidth-toolBodyStyle.GetHorizontalFrameSize())).Render(p.renderToolCardCached(contentWidth, index, message))
	}
	if message.Type == chatui.TypeUser {
		content = userStyle.Width(max(1, contentWidth-userStyle.GetHorizontalFrameSize())).Render(message.Content)
	}
	if message.Type == chatui.TypeSystem {
		content = systemStyle.Width(max(1, contentWidth-systemStyle.GetHorizontalFrameSize())).Render(message.Content)
	}

	return content
}

func (p *Page) markDirty() {
	p.dirty = true
	p.cachedPrefix.dirty = true
}

func (p *Page) markTailDirty() {
	p.dirty = true
}

func (p *Page) ToggleToolCardAtLine(line int) bool {
	for _, hit := range p.toolHits {
		if line < hit.startLine || line > hit.endLine {
			continue
		}
		if hit.messageIndex < 0 || hit.messageIndex >= len(p.messages) {
			return false
		}
		if p.messages[hit.messageIndex].Meta == nil {
			p.messages[hit.messageIndex].Meta = map[string]any{}
		}
		expanded, _ := p.messages[hit.messageIndex].Meta["expanded"].(bool)
		p.messages[hit.messageIndex].Meta["expanded"] = !expanded
		p.markDirty()
		return true
	}
	return false
}

func (p *Page) ToolNameAtLine(line int) string {
	for _, hit := range p.toolHits {
		if line >= hit.startLine && line <= hit.endLine {
			return hit.toolName
		}
	}
	return ""
}

func (p *Page) ToolActionAtLine(line int) ToolAction {
	return ToolActionNone
}

func (p *Page) activeTailStart() (int, bool) {
	start := len(p.messages)
	if p.thinkingIndex >= 0 && p.thinkingIndex < start {
		start = p.thinkingIndex
	}
	if p.streamIndex >= 0 && p.streamIndex < start {
		start = p.streamIndex
	}
	if start >= len(p.messages) {
		return 0, false
	}
	return start, true
}

func (p *Page) renderAll(width int) {
	content, _, hits := p.renderRange(width, 0, len(p.messages), 0)
	p.cachedContent = content
	p.toolHits = hits
	p.cachedPrefix = renderCache{
		tailStart: -1,
		dirty:     true,
	}
}

func (p *Page) renderWithActiveTail(width, tailStart int) {
	if p.cachedPrefix.width != width || p.cachedPrefix.tailStart != tailStart || p.cachedPrefix.dirty {
		content, lineCount, hits := p.renderRange(width, 0, tailStart, 0)
		p.cachedPrefix.width = width
		p.cachedPrefix.tailStart = tailStart
		p.cachedPrefix.content = content
		p.cachedPrefix.lineCount = lineCount
		p.cachedPrefix.toolHits = hits
		p.cachedPrefix.dirty = false
	}

	tailContent, _, tailHits := p.renderRange(width, tailStart, len(p.messages), p.cachedPrefix.lineCount)
	switch {
	case p.cachedPrefix.content == "":
		p.cachedContent = tailContent
	case tailContent == "":
		p.cachedContent = p.cachedPrefix.content
	default:
		p.cachedContent = p.cachedPrefix.content + "\n\n" + tailContent
	}

	p.toolHits = append(make([]toolHit, 0, len(p.cachedPrefix.toolHits)+len(tailHits)), p.cachedPrefix.toolHits...)
	p.toolHits = append(p.toolHits, tailHits...)
}

func (p *Page) renderRange(width, start, end, initialLineOffset int) (string, int, []toolHit) {
	if start < 0 {
		start = 0
	}
	if end > len(p.messages) {
		end = len(p.messages)
	}
	if start >= end {
		return "", initialLineOffset, nil
	}

	lines := make([]string, 0, end-start)
	hits := make([]toolHit, 0)
	lineOffset := initialLineOffset
	hasPrior := start > 0

	for i := start; i < end; i++ {
		message := p.messages[i]
		if i > 0 && needsReplyDivider(p.messages[i-1], message) {
			divider := renderReplyDivider(width)
			if len(lines) > 0 || hasPrior {
				lineOffset++
			}
			lines = append(lines, divider)
			lineOffset += lipgloss.Height(divider)
			hasPrior = true
		}

		rendered := p.renderMessage(width, i, message)
		if len(lines) > 0 || hasPrior {
			lineOffset++
		}
		startLine := lineOffset
		lines = append(lines, rendered)
		lineOffset += lipgloss.Height(rendered)
		hasPrior = true

		if message.Type == chatui.TypeTool {
			hits = append(hits, toolHit{
				messageIndex: i,
				toolName:     message.ToolName,
				startLine:    startLine,
				endLine:      lineOffset - 1,
			})
		}
	}

	return strings.Join(lines, "\n\n"), lineOffset, hits
}

func (p *Page) renderToolCardCached(width, index int, message chatui.Message) string {
	cacheKey := toolRenderCacheKey(message)
	if cached, ok := p.toolRenderCache[index]; ok && cached.width == width && cached.key == cacheKey {
		return cached.rendered
	}

	rendered := renderToolCard(width, message)
	p.toolRenderCache[index] = toolRenderCache{
		width:    width,
		key:      cacheKey,
		rendered: rendered,
	}
	return rendered
}

func toolRenderCacheKey(message chatui.Message) string {
	oldLines := ""
	if message.Meta != nil {
		oldLines, _ = message.Meta["old_lines"].(string)
	}
	return fmt.Sprintf(
		"%s|%s|%s|%t|%s",
		message.ToolCallID,
		message.ToolName,
		message.Content,
		isToolExpanded(message),
		oldLines,
	)
}
