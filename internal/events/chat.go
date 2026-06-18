package events

type ChatStreamStarted struct {
	RequestID string
}

type ChatResponseStarted struct {
	RequestID string
}

type ChatStreamChunk struct {
	RequestID string
	Chunk     string
}

type ChatStreamFailed struct {
	RequestID string
	Err       string
}

type ChatStreamCompleted struct {
	RequestID string
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
	Result     string
}
