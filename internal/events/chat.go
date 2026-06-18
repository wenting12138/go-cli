package events

type ChatStreamStarted struct {
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
