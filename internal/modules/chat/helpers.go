package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"go-cli/internal/modules/chat/domain"
	chatui "go-cli/internal/modules/chat/ui"
)

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func formatContextWindow(size int) string {
	if size%1000 == 0 {
		return fmt.Sprintf("%dk", size/1000)
	}
	return fmt.Sprintf("%.1fk", float64(size)/1000)
}

func (p *Page) SessionTitle() string {
	for _, message := range p.modelMessages {
		if message.Role == "user" && strings.TrimSpace(message.Content) != "" {
			text := strings.TrimSpace(message.Content)
			if len(text) > 48 {
				return text[:48] + "..."
			}
			return text
		}
	}
	return "Untitled Session"
}

func cloneMessages(messages []domain.Message) []domain.Message {
	out := make([]domain.Message, 0, len(messages))
	for _, message := range messages {
		cloned := message
		if len(message.AssistantBlocks) > 0 {
			cloned.AssistantBlocks = cloneAssistantBlocks(message.AssistantBlocks)
		}
		if len(message.Args) > 0 {
			cloned.Args = make(map[string]any, len(message.Args))
			for key, value := range message.Args {
				cloned.Args[key] = value
			}
		}
		out = append(out, cloned)
	}
	return out
}

func cloneAssistantBlocks(blocks []domain.AssistantContentBlock) []domain.AssistantContentBlock {
	out := make([]domain.AssistantContentBlock, 0, len(blocks))
	for _, block := range blocks {
		cloned := block
		if len(block.Input) > 0 {
			cloned.Input = make(map[string]any, len(block.Input))
			for key, value := range block.Input {
				cloned.Input[key] = value
			}
		}
		out = append(out, cloned)
	}
	return out
}

func toUIMessages(messages []domain.Message) []chatui.Message {
	out := make([]chatui.Message, 0, len(messages))
	for _, message := range messages {
		switch message.Type {
		case domain.MessageTypeCompact:
			out = append(out, chatui.TextMessage(chatui.TypeSystem, message.Content))
		case domain.MessageTypeToolCall:
			out = append(out, chatui.ToolMessage(message.ToolCallID, message.ToolName, formatToolCall(message.ToolName, message.Args), buildToolMeta(message.ToolName, message.Args, "")))
		default:
			out = append(out, chatui.TextMessage(toUIType(message.Role), message.Content))
		}
	}
	return out
}

func toUIType(role string) string {
	switch role {
	case "user":
		return chatui.TypeUser
	case "assistant":
		return chatui.TypeAssistant
	default:
		return chatui.TypeSystem
	}
}

func startupLogo() string {
	return strings.TrimSpace(`
   ____        _______ _      ___
  / ___| ___  / / ____| |    |_ _|
 | |  _ / _ \/ / |    | |     | |
 | |_| | (_) / /| |___ | |___  | |
  \____|\___/_/  \____||_____|___|

  /models   choose model
  /resume   restore session
  /tools    inspect tools
`)
}

func needsReplyDivider(prev, current chatui.Message) bool {
	return (prev.Type == chatui.TypeUser && current.Type == chatui.TypeAssistant) ||
		(prev.Type == chatui.TypeAssistant && current.Type == chatui.TypeUser)
}

func renderReplyDivider(width int) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("255")).
		Render(strings.Repeat("─", max(8, width)))
}
