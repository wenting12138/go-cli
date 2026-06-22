package events

import (
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat/domain"
)

type ChatStreamStarted struct {
	RequestID string
}

type ChatAssistantBlocks struct {
	RequestID string
	Blocks    []domain.AssistantContentBlock
}

type ChatResponseStarted struct {
	RequestID string
}

type ChatStreamChunk struct {
	RequestID string
	Chunk     string
}

type ChatThinkingStarted struct {
	RequestID string
}

type ChatThinkingChunk struct {
	RequestID string
	Chunk     string
}

type ChatThinkingSignature struct {
	RequestID string
	Signature string
}

type ChatStreamFailed struct {
	RequestID string
	Err       string
}

type ChatStreamCompleted struct {
	RequestID string
	Metrics   protocol.ResponseMetrics
}

type ChatToolCallStarted struct {
	RequestID  string
	Tool       string
	ToolCallID string
	Args       map[string]any
}

type ChatToolCallCompleted struct {
	RequestID  string
	Tool       string
	ToolCallID string
	Args       map[string]any
	Result     string
}
