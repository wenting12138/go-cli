package chat

import (
	"context"
	"fmt"
	"time"

	"go-cli/internal/agent/react"
	"go-cli/internal/agent/tools"
	"go-cli/internal/config"
	"go-cli/internal/eventbus"
	"go-cli/internal/events"
	"go-cli/internal/modelclient"
	"go-cli/internal/modules/chat/domain"
)

type Service struct {
	bus *eventbus.Bus
}

func New(bus *eventbus.Bus) *Service {
	return &Service{bus: bus}
}

func (s *Service) StartStream(model config.ModelConfig, messages []domain.Message) error {
	client, err := modelclient.New(model)
	if err != nil {
		return err
	}
	agent := react.New(client, tools.Build(model))

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
