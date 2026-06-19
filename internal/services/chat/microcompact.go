package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"go-cli/internal/modules/chat/domain"
)

type microcompactStats struct {
	InputMessages  int
	OutputMessages int
}

func microcompactMessages(messages []domain.Message) ([]domain.Message, microcompactStats) {
	stats := microcompactStats{InputMessages: len(messages)}
	if len(messages) == 0 {
		return nil, stats
	}

	compacted := make([]domain.Message, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		current := cloneMessage(messages[i])

		if isEmptyTextMessage(current) {
			continue
		}

		if current.Type == domain.MessageTypeToolCall && i+1 < len(messages) {
			next := messages[i+1]
			if next.Type == domain.MessageTypeToolResult &&
				next.ToolCallID == current.ToolCallID &&
				next.ToolName == current.ToolName {
				compacted = append(compacted, domain.TextMessage("system", summarizeToolInteraction(current, next)))
				i++
				continue
			}
		}

		if current.Type == domain.MessageTypeText &&
			current.Role == "system" &&
			len(compacted) > 0 &&
			compacted[len(compacted)-1].Type == domain.MessageTypeText &&
			compacted[len(compacted)-1].Role == "system" {
			prev := compacted[len(compacted)-1]
			prev.Content = strings.TrimSpace(prev.Content + "\n" + current.Content)
			compacted[len(compacted)-1] = prev
			continue
		}

		compacted = append(compacted, current)
	}

	stats.OutputMessages = len(compacted)
	return compacted, stats
}

func summarizeToolInteraction(call, result domain.Message) string {
	args := "{}"
	if len(call.Args) > 0 {
		if data, err := json.Marshal(call.Args); err == nil {
			args = string(data)
		}
	}

	resultText := strings.TrimSpace(result.Result)
	if len(resultText) > 320 {
		resultText = resultText[:320] + "..."
	}

	return fmt.Sprintf(
		"Tool interaction summary:\nname: %s\nargs: %s\nresult: %s",
		call.ToolName,
		args,
		resultText,
	)
}

func isEmptyTextMessage(message domain.Message) bool {
	return message.Type == domain.MessageTypeText && strings.TrimSpace(message.Content) == ""
}

func cloneMessage(message domain.Message) domain.Message {
	cloned := message
	if len(message.Args) > 0 {
		cloned.Args = make(map[string]any, len(message.Args))
		for key, value := range message.Args {
			cloned.Args[key] = value
		}
	}
	return cloned
}
