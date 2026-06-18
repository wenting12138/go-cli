package protocol

import "go-cli/internal/modules/chat/domain"

type EventType string

const (
	EventTypeTextDelta        EventType = "text_delta"
	EventTypeThinkingDelta    EventType = "thinking_delta"
	EventTypeRedactedThinking EventType = "redacted_thinking"
	EventTypeToolCall         EventType = "tool_call"
)

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type Event struct {
	Type     EventType
	Text     string
	ToolCall ToolCall
}

type ChatRequest struct {
	Messages []domain.Message
	Tools    []ToolDefinition
}
