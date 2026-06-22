package chat

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go-cli/internal/agent/react"
	"go-cli/internal/agent/tools"
	"go-cli/internal/agentmode"
	"go-cli/internal/config"
	"go-cli/internal/eventbus"
	"go-cli/internal/events"
	"go-cli/internal/modelclient"
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/prompt"
)

type Service struct {
	bus           *eventbus.Bus
	promptBuilder *prompt.Builder
	mu            sync.Mutex
	active        map[string]context.CancelFunc
}

type StreamMetrics struct {
	StartedAt       time.Time
	FirstOutputAt   time.Time
	CompletedAt     time.Time
	ProviderMetrics protocol.ResponseMetrics
}

func New(bus *eventbus.Bus) *Service {
	return &Service{
		bus:           bus,
		promptBuilder: prompt.NewBuilder(),
		active:        make(map[string]context.CancelFunc),
	}
}

func (s *Service) StartStream(model config.ModelConfig, mode agentmode.Mode, messages []domain.Message) error {
	client, err := modelclient.New(model)
	if err != nil {
		return err
	}
	systemPrompt, err := s.BuildSystemPrompt(model, mode)
	if err != nil {
		return err
	}
	agent := react.New(client, model, tools.Build(model, mode), s.promptBuilder, systemPrompt)

	requestID := fmt.Sprintf("chat-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	s.storeCancel(requestID, cancel)
	s.bus.Publish(events.ChatStreamStarted{RequestID: requestID})

	go func() {
		defer s.clearCancel(requestID)
		defer cancel()
		toolArgsByID := make(map[string]map[string]any)
		metrics := StreamMetrics{
			StartedAt: time.Now(),
		}
		markFirstOutput := func() {
			if metrics.FirstOutputAt.IsZero() {
				metrics.FirstOutputAt = time.Now()
			}
		}

		err := agent.Run(ctx, messages, react.Hooks{
			OnAssistantBlocks: func(blocks []domain.AssistantContentBlock) {
				s.bus.Publish(events.ChatAssistantBlocks{
					RequestID: requestID,
					Blocks:    cloneAssistantBlocks(blocks),
				})
			},
			OnToolStart: func(name, toolCallID string, args map[string]any) {
				markFirstOutput()
				toolArgsByID[toolCallID] = cloneArgs(args)
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
					Args:       cloneArgs(toolArgsByID[toolCallID]),
					Result:     result,
				})
			},
			OnThinkingStart: func() {
				markFirstOutput()
				s.bus.Publish(events.ChatThinkingStarted{RequestID: requestID})
			},
			OnThinkingChunk: func(chunk string) {
				s.bus.Publish(events.ChatThinkingChunk{
					RequestID: requestID,
					Chunk:     chunk,
				})
			},
			OnThinkingSignature: func(signature string) {
				s.bus.Publish(events.ChatThinkingSignature{
					RequestID: requestID,
					Signature: signature,
				})
			},
			OnFinalStart: func() {
				markFirstOutput()
				s.bus.Publish(events.ChatResponseStarted{RequestID: requestID})
			},
			OnFinalChunk: func(chunk string) {
				s.bus.Publish(events.ChatStreamChunk{
					RequestID: requestID,
					Chunk:     chunk,
				})
			},
			OnMetrics: func(providerMetrics protocol.ResponseMetrics) {
				metrics.ProviderMetrics = providerMetrics
			},
		})
		if err != nil {
			if ctx.Err() == context.Canceled {
				s.bus.Publish(events.ChatStreamFailed{
					RequestID: requestID,
					Err:       "response interrupted",
				})
				return
			}
			s.bus.Publish(events.ChatStreamFailed{
				RequestID: requestID,
				Err:       fmt.Sprintf("model request failed: %s", err),
			})
			return
		}

		metrics.CompletedAt = time.Now()
		s.bus.Publish(events.ChatStreamCompleted{
			RequestID: requestID,
			Metrics:   s.buildResponseMetrics(metrics),
		})
	}()

	return nil
}

func (s *Service) buildResponseMetrics(metrics StreamMetrics) protocol.ResponseMetrics {
	out := metrics.ProviderMetrics
	if !metrics.FirstOutputAt.IsZero() && !metrics.StartedAt.IsZero() {
		out.FirstTokenDuration = metrics.FirstOutputAt.Sub(metrics.StartedAt).Nanoseconds()
	}
	if !metrics.CompletedAt.IsZero() && !metrics.StartedAt.IsZero() && out.TotalDuration <= 0 {
		out.TotalDuration = metrics.CompletedAt.Sub(metrics.StartedAt).Nanoseconds()
	}
	return out
}

func (s *Service) Cancel(requestID string) bool {
	s.mu.Lock()
	cancel, ok := s.active[requestID]
	s.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}

func (s *Service) BuildSystemPrompt(model config.ModelConfig, mode agentmode.Mode) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return s.promptBuilder.Build(prompt.Input{
		Model:      model,
		Mode:       mode,
		WorkingDir: workingDir,
		Tools:      react.ToolDefinitions(tools.Build(model, mode)),
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

func (s *Service) storeCancel(requestID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[requestID] = cancel
}

func (s *Service) clearCancel(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, requestID)
}
