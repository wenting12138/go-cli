package domain

import "encoding/json"

const (
	MessageTypeText       = "text"
	MessageTypeToolCall   = "tool_call"
	MessageTypeToolResult = "tool_result"
)

type Message struct {
	Role       string
	Content    string
	Type       string
	ToolName   string
	ToolCallID string
	Args       map[string]any
	Result     string
}

type Session struct {
	ID       string
	Title    string
	Messages []Message
}

func TextMessage(role, content string) Message {
	return Message{
		Role:    role,
		Content: content,
		Type:    MessageTypeText,
	}
}

func ToolCallMessage(toolCallID, toolName string, args map[string]any) Message {
	return Message{
		Role:       "assistant",
		Type:       MessageTypeToolCall,
		ToolName:   toolName,
		ToolCallID: toolCallID,
		Args:       cloneArgs(args),
	}
}

func ToolResultMessage(toolCallID, toolName, result string) Message {
	return Message{
		Role:       "tool",
		Type:       MessageTypeToolResult,
		ToolName:   toolName,
		ToolCallID: toolCallID,
		Result:     result,
	}
}

func (m Message) ModelRole() string {
	switch m.Role {
	case "assistant", "user", "system", "tool":
		return m.Role
	default:
		return "user"
	}
}

func (m Message) ModelContent() string {
	switch m.Type {
	case "", MessageTypeText:
		return m.Content
	case MessageTypeToolCall:
		return mustJSON(map[string]any{
			"type":         MessageTypeToolCall,
			"tool_call_id": m.ToolCallID,
			"tool":         m.ToolName,
			"args":         cloneArgs(m.Args),
		})
	case MessageTypeToolResult:
		return mustJSON(map[string]any{
			"type":         MessageTypeToolResult,
			"tool_call_id": m.ToolCallID,
			"tool":         m.ToolName,
			"result":       m.Result,
		})
	default:
		if m.Content != "" {
			return m.Content
		}
		return mustJSON(map[string]any{
			"type":    m.Type,
			"tool":    m.ToolName,
			"args":    cloneArgs(m.Args),
			"result":  m.Result,
			"content": m.Content,
		})
	}
}

func cloneArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return `{"type":"serialization_error"}`
	}
	return string(data)
}
