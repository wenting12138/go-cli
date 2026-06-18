package ui

type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

const (
	TypeSystem    = "system"
	TypeUser      = "user"
	TypeAssistant = "assistant"
	TypeTool      = "tool"
)

func TextMessage(kind, content string) Message {
	return Message{
		Type:    kind,
		Content: content,
	}
}

func CloneMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}
