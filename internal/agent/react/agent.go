package react

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"go-cli/internal/config"
	"go-cli/internal/modelclient"
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/prompt"
)

type Tool interface {
	Name() string
	Description() string
	Parameters() ParameterSchema
	ValidateArgs(args map[string]any) error
	Run(ctx context.Context, args map[string]any) (string, error)
}

type InterruptingTool interface {
	InterruptsTurn() bool
}

type Hooks struct {
	OnToolStart         func(name, toolCallID string, args map[string]any)
	OnToolResult        func(name, toolCallID, result string)
	OnAssistantBlocks   func(blocks []domain.AssistantContentBlock)
	OnThinkingStart     func()
	OnThinkingChunk     func(chunk string)
	OnThinkingSignature func(signature string)
	OnFinalStart        func()
	OnFinalChunk        func(chunk string)
	OnMetrics           func(metrics protocol.ResponseMetrics)
}

type Agent struct {
	client        modelclient.Client
	model         config.ModelConfig
	tools         map[string]Tool
	promptBuilder *prompt.Builder
	staticPrompt  string
	maxSteps      int
}

type collectResult struct {
	TextChunks      []string
	ToolCalls       []protocol.ToolCall
	AssistantBlocks []domain.AssistantContentBlock
	Metrics         protocol.ResponseMetrics
}

func New(client modelclient.Client, model config.ModelConfig, tools []Tool, promptBuilder *prompt.Builder, systemPrompt string) *Agent {
	indexed := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		indexed[tool.Name()] = tool
	}
	if promptBuilder == nil {
		promptBuilder = prompt.NewBuilder()
	}

	return &Agent{
		client:        client,
		model:         model,
		tools:         indexed,
		promptBuilder: promptBuilder,
		staticPrompt:  systemPrompt,
		maxSteps:      100,
	}
}

func (a *Agent) Run(ctx context.Context, history []domain.Message, hooks Hooks) error {
	systemPrompt, err := a.systemPrompt()
	if err != nil {
		return err
	}
	messages := append([]domain.Message{
		domain.TextMessage("system", systemPrompt),
	}, history...)

	for step := 0; step < a.maxSteps; step++ {
		streamStarted := false
		result, err := a.collect(ctx, messages, func() {
			if hooks.OnThinkingStart != nil {
				hooks.OnThinkingStart()
			}
		}, func(chunk string) {
			if hooks.OnThinkingChunk != nil {
				hooks.OnThinkingChunk(chunk)
			}
		}, func(signature string) {
			if hooks.OnThinkingSignature != nil {
				hooks.OnThinkingSignature(signature)
			}
		}, func(chunk string) {
			if hooks.OnFinalStart != nil && !streamStarted {
				hooks.OnFinalStart()
				streamStarted = true
			}
			if hooks.OnFinalChunk != nil {
				hooks.OnFinalChunk(chunk)
			}
		}, func(metrics protocol.ResponseMetrics) {
			if hooks.OnMetrics != nil {
				hooks.OnMetrics(metrics)
			}
		})
		if err != nil {
			return err
		}
		if hooks.OnAssistantBlocks != nil && len(result.AssistantBlocks) > 0 {
			hooks.OnAssistantBlocks(cloneAssistantBlocks(result.AssistantBlocks))
		}

		if len(result.ToolCalls) == 0 {
			final := strings.TrimSpace(strings.Join(result.TextChunks, ""))
			if final == "" {
				return fmt.Errorf("model returned no text and no tool calls")
			}

			messages = append(messages, assistantMessage(final, result.AssistantBlocks))
			if hooks.OnFinalStart != nil && !streamStarted {
				hooks.OnFinalStart()
			}
			if hooks.OnFinalChunk != nil && !streamStarted {
				for _, chunk := range result.TextChunks {
					if chunk == "" {
						continue
					}
					hooks.OnFinalChunk(chunk)
				}
			}
			return nil
		}

		assistantText := strings.TrimSpace(strings.Join(result.TextChunks, ""))
		if assistantText != "" || len(result.AssistantBlocks) > 0 {
			messages = append(messages, assistantMessage(assistantText, result.AssistantBlocks))
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

			if interrupting, ok := tool.(InterruptingTool); ok && interrupting.InterruptsTurn() {
				return nil
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

func (a *Agent) collect(ctx context.Context, messages []domain.Message, onThinkingStart func(), onThinkingChunk func(string), onThinkingSignature func(string), onTextChunk func(string), onMetrics func(protocol.ResponseMetrics)) (collectResult, error) {
	result := collectResult{}
	thinkingStarted := false
	request := protocol.ChatRequest{
		Messages: messages,
		Tools:    a.toolDefinitions(),
	}

	err := a.client.StreamChat(ctx, request, func(event protocol.Event) error {
		switch event.Type {
		case protocol.EventTypeTextDelta:
			result.TextChunks = append(result.TextChunks, event.Text)
			if onTextChunk != nil && event.Text != "" {
				onTextChunk(event.Text)
			}
		case protocol.EventTypeThinkingDelta, protocol.EventTypeRedactedThinking:
			if onThinkingStart != nil && !thinkingStarted {
				onThinkingStart()
				thinkingStarted = true
			}
			if onThinkingChunk != nil && event.Text != "" {
				onThinkingChunk(event.Text)
			}
		case protocol.EventTypeThinkingSignature:
			if onThinkingSignature != nil && event.Text != "" {
				onThinkingSignature(event.Text)
			}
		case protocol.EventTypeToolCall:
			result.ToolCalls = append(result.ToolCalls, protocol.ToolCall{
				ID:        event.ToolCall.ID,
				Name:      event.ToolCall.Name,
				Arguments: cloneArgs(event.ToolCall.Arguments),
			})
		case protocol.EventTypeAssistantBlocks:
			result.AssistantBlocks = cloneAssistantBlocks(event.AssistantBlocks)
		case protocol.EventTypeMetrics:
			result.Metrics = event.Metrics
			if onMetrics != nil {
				onMetrics(event.Metrics)
			}
		}
		return nil
	})
	if err != nil {
		return collectResult{}, err
	}
	return result, nil
}

func (a *Agent) systemPrompt() (string, error) {
	if strings.TrimSpace(a.staticPrompt) != "" {
		return a.staticPrompt, nil
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return a.promptBuilder.Build(prompt.Input{
		Model:      a.model,
		Mode:       "",
		WorkingDir: workingDir,
		Tools:      a.toolDefinitions(),
	})
}

func (a *Agent) toolDefinitions() []protocol.ToolDefinition {
	return ToolDefinitions(mapsToSlice(a.tools))
}

func ToolDefinitions(tools []Tool) []protocol.ToolDefinition {
	indexed := make(map[string]Tool, len(tools))
	for _, tool := range tools {
		indexed[tool.Name()] = tool
	}

	return toolDefinitionsFromMap(indexed)
}

func toolDefinitionsFromMap(tools map[string]Tool) []protocol.ToolDefinition {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	definitions := make([]protocol.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := tools[name]
		definitions = append(definitions, protocol.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters().ToMap(),
		})
	}
	return definitions
}

func mapsToSlice(indexed map[string]Tool) []Tool {
	names := make([]string, 0, len(indexed))
	for name := range indexed {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]Tool, 0, len(names))
	for _, name := range names {
		out = append(out, indexed[name])
	}
	return out
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

func assistantMessage(content string, blocks []domain.AssistantContentBlock) domain.Message {
	message := domain.TextMessage("assistant", content)
	if len(blocks) > 0 {
		message.AssistantBlocks = cloneAssistantBlocks(blocks)
	}
	return message
}
