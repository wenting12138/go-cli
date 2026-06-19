package ui

type Message struct {
	Type       string         `json:"type"`
	Content    string         `json:"content"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
}

const (
	TypeSystem    = "system"
	TypeUser      = "user"
	TypeAssistant = "assistant"
	TypeThinking  = "thinking"
	TypeTool      = "tool"
)

func TextMessage(kind, content string) Message {
	return Message{
		Type:    kind,
		Content: content,
	}
}

func ToolMessage(toolCallID, toolName, content string, meta map[string]any) Message {
	return Message{
		Type:       TypeTool,
		Content:    content,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Meta:       cloneMeta(meta),
	}
}

func CloneMessages(messages []Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		cloned := message
		cloned.Meta = cloneMeta(message.Meta)
		out = append(out, cloned)
	}
	return out
}

func cloneMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(meta))
	for key, value := range meta {
		cloned[key] = value
	}
	return cloned
}
