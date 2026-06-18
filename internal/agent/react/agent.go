package react

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go-cli/internal/modelclient"
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat/domain"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() ParameterSchema
	ValidateArgs(args map[string]any) error
	Run(ctx context.Context, args map[string]any) (string, error)
}

type Hooks struct {
	OnToolStart  func(name, toolCallID string, args map[string]any)
	OnToolResult func(name, toolCallID, result string)
	OnFinalStart func()
	OnFinalChunk func(chunk string)
}

type Agent struct {
	client   modelclient.Client
	tools    map[string]Tool
	maxSteps int
}

type collectResult struct {
	TextChunks []string
	ToolCalls  []protocol.ToolCall
}

func New(client modelclient.Client, tools []Tool) *Agent {
	indexed := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		indexed[tool.Name()] = tool
	}

	return &Agent{
		client:   client,
		tools:    indexed,
		maxSteps: 6,
	}
}

func (a *Agent) Run(ctx context.Context, history []domain.Message, hooks Hooks) error {
	messages := append([]domain.Message{
		domain.TextMessage("system", a.systemPrompt()),
	}, history...)

	for step := 0; step < a.maxSteps; step++ {
		result, err := a.collect(ctx, messages)
		if err != nil {
			return err
		}

		if len(result.ToolCalls) == 0 {
			final := strings.TrimSpace(strings.Join(result.TextChunks, ""))
			if final == "" {
				return fmt.Errorf("model returned no text and no tool calls")
			}

			messages = append(messages, domain.TextMessage("assistant", final))
			if hooks.OnFinalStart != nil {
				hooks.OnFinalStart()
			}
			if hooks.OnFinalChunk != nil {
				for _, chunk := range result.TextChunks {
					if chunk == "" {
						continue
					}
					hooks.OnFinalChunk(chunk)
				}
			}
			return nil
		}

		if text := strings.TrimSpace(strings.Join(result.TextChunks, "")); text != "" {
			messages = append(messages, domain.TextMessage("assistant", text))
		}

		for index, toolCall := range result.ToolCalls {
			toolCallID := toolCall.ID
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("call-%d-%d", step+1, index+1)
			}
			args := cloneArgs(toolCall.Arguments)

			messages = append(messages, domain.ToolCallMessage(toolCallID, toolCall.Name, args))

			tool, exists := a.tools[toolCall.Name]
			if !exists {
				messages = append(messages, domain.ToolResultMessage(toolCallID, toolCall.Name, fmt.Sprintf("tool %q not found", toolCall.Name)))
				continue
			}

			if hooks.OnToolStart != nil {
				hooks.OnToolStart(tool.Name(), toolCallID, args)
			}

			if err := tool.ValidateArgs(args); err != nil {
				resultText := fmt.Sprintf("tool args error: %s", err)
				if hooks.OnToolResult != nil {
					hooks.OnToolResult(tool.Name(), toolCallID, resultText)
				}
				messages = append(messages, domain.ToolResultMessage(toolCallID, tool.Name(), resultText))
				continue
			}

			toolResult, err := tool.Run(ctx, args)
			if err != nil {
				toolResult = fmt.Sprintf("tool error: %s", err)
			}

			if hooks.OnToolResult != nil {
				hooks.OnToolResult(tool.Name(), toolCallID, toolResult)
			}

			messages = append(messages, domain.ToolResultMessage(toolCallID, tool.Name(), toolResult))
		}
	}

	return fmt.Errorf("agent exceeded max steps without final answer")
}

func (a *Agent) collect(ctx context.Context, messages []domain.Message) (collectResult, error) {
	result := collectResult{}
	request := protocol.ChatRequest{
		Messages: messages,
		Tools:    a.toolDefinitions(),
	}

	err := a.client.StreamChat(ctx, request, func(event protocol.Event) error {
		switch event.Type {
		case protocol.EventTypeTextDelta:
			result.TextChunks = append(result.TextChunks, event.Text)
		case protocol.EventTypeToolCall:
			result.ToolCalls = append(result.ToolCalls, protocol.ToolCall{
				ID:        event.ToolCall.ID,
				Name:      event.ToolCall.Name,
				Arguments: cloneArgs(event.ToolCall.Arguments),
			})
		}
		return nil
	})
	if err != nil {
		return collectResult{}, err
	}
	return result, nil
}

func (a *Agent) systemPrompt() string {
	return strings.TrimSpace(`You are a helpful CLI assistant.
Use the available tools when they are needed.
Keep answers concise and useful.`)
}

func (a *Agent) toolDefinitions() []protocol.ToolDefinition {
	names := make([]string, 0, len(a.tools))
	for name := range a.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	definitions := make([]protocol.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := a.tools[name]
		definitions = append(definitions, protocol.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters().ToMap(),
		})
	}
	return definitions
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
