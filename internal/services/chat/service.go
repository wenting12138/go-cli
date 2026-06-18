package chat

import (
	"context"
	"fmt"
	"os"
	"time"

	"go-cli/internal/agent/react"
	"go-cli/internal/agent/tools"
	"go-cli/internal/config"
	"go-cli/internal/eventbus"
	"go-cli/internal/events"
	"go-cli/internal/modelclient"
	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/prompt"
)

type Service struct {
	bus           *eventbus.Bus
	promptBuilder *prompt.Builder
}

func New(bus *eventbus.Bus) *Service {
	return &Service{
		bus:           bus,
		promptBuilder: prompt.NewBuilder(),
	}
}

func (s *Service) StartStream(model config.ModelConfig, messages []domain.Message) error {
	client, err := modelclient.New(model)
	if err != nil {
		return err
	}
	agent := react.New(client, model, tools.Build(model), s.promptBuilder)

	requestID := fmt.Sprintf("chat-%d", time.Now().UnixNano())
	s.bus.Publish(events.ChatStreamStarted{RequestID: requestID})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		err := agent.Run(ctx, messages, react.Hooks{
			OnToolStart: func(name, toolCallID string, args map[string]any) {
				s.bus.Publish(events.ChatToolCallStarted{
					RequestID:  requestID,
					Tool:       name,
					ToolCallID: toolCallID,
					Args:       cloneArgs(args),
				})
			},
			OnToolResult: func(name, toolCallID, result string) {
				s.bus.Publish(events.ChatToolCallCompleted{
					RequestID:  requestID,
					Tool:       name,
					ToolCallID: toolCallID,
					Result:     result,
				})
			},
			OnThinkingStart: func() {
				s.bus.Publish(events.ChatThinkingStarted{RequestID: requestID})
			},
			OnThinkingChunk: func(chunk string) {
				s.bus.Publish(events.ChatThinkingChunk{
					RequestID: requestID,
					Chunk:     chunk,
				})
			},
			OnFinalStart: func() {
				s.bus.Publish(events.ChatResponseStarted{RequestID: requestID})
			},
			OnFinalChunk: func(chunk string) {
				s.bus.Publish(events.ChatStreamChunk{
					RequestID: requestID,
					Chunk:     chunk,
				})
			},
		})
		if err != nil {
			s.bus.Publish(events.ChatStreamFailed{
				RequestID: requestID,
				Err:       fmt.Sprintf("model request failed: %s", err),
			})
			return
		}

		s.bus.Publish(events.ChatStreamCompleted{RequestID: requestID})
	}()

	return nil
}

func (s *Service) BuildSystemPrompt(model config.ModelConfig) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return s.promptBuilder.Build(prompt.Input{
		Model:      model,
		WorkingDir: workingDir,
		Tools:      react.ToolDefinitions(tools.Build(model)),
	})
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
