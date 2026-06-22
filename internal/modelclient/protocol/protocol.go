package protocol

import "go-cli/internal/modules/chat/domain"

type EventType string

const (
	EventTypeTextDelta         EventType = "text_delta"
	EventTypeThinkingDelta     EventType = "thinking_delta"
	EventTypeThinkingSignature EventType = "thinking_signature"
	EventTypeRedactedThinking  EventType = "redacted_thinking"
	EventTypeToolCall          EventType = "tool_call"
	EventTypeAssistantBlocks   EventType = "assistant_blocks"
	EventTypeMetrics           EventType = "metrics"
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

type ResponseMetrics struct {
	FirstTokenDuration int64
	TotalDuration      int64
	LoadDuration       int64
	PromptEvalCount    int
	PromptEvalDuration int64
	EvalCount          int
	EvalDuration       int64
}

type Event struct {
	Type            EventType
	Text            string
	ToolCall        ToolCall
	AssistantBlocks []domain.AssistantContentBlock
	Metrics         ResponseMetrics
}

type ChatRequest struct {
	Messages []domain.Message
	Tools    []ToolDefinition
}
