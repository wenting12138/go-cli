package chat

import (
	"context"
	"fmt"
	"time"

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

	requestID := fmt.Sprintf("chat-%d", time.Now().UnixNano())
	s.bus.Publish(events.ChatStreamStarted{RequestID: requestID})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		err := client.StreamChat(ctx, messages, func(chunk string) error {
			if chunk == "" {
				return nil
			}
			s.bus.Publish(events.ChatStreamChunk{
				RequestID: requestID,
				Chunk:     chunk,
			})
			return nil
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
