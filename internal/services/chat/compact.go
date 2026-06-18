package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-cli/internal/config"
	"go-cli/internal/modelclient"
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat/domain"
)

const preservedTailMessages = 6

type CompactResult struct {
	Boundary      domain.Message
	Summary       domain.Message
	PreservedTail []domain.Message
}

func (s *Service) Compact(model config.ModelConfig, systemPrompt string, messages []domain.Message) (CompactResult, error) {
	if len(messages) <= preservedTailMessages {
		return CompactResult{}, fmt.Errorf("not enough messages to compact")
	}

	client, err := modelclient.New(model)
	if err != nil {
		return CompactResult{}, err
	}

	summaryInput := cloneDomainMessages(messages[:len(messages)-preservedTailMessages])
	preservedTail := cloneDomainMessages(messages[len(messages)-preservedTailMessages:])

	requestMessages := append(summaryInput, domain.TextMessage("user", compactPrompt()))
	if strings.TrimSpace(systemPrompt) != "" {
		requestMessages = append([]domain.Message{
			domain.TextMessage("system", systemPrompt),
			domain.TextMessage("system", compactSystemInstruction()),
		}, requestMessages...)
	} else {
		requestMessages = append([]domain.Message{
			domain.TextMessage("system", compactSystemInstruction()),
		}, requestMessages...)
	}

	var chunks []string
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	err = client.StreamChat(ctx, protocol.ChatRequest{
		Messages: requestMessages,
	}, func(event protocol.Event) error {
		if event.Type == protocol.EventTypeTextDelta && event.Text != "" {
			chunks = append(chunks, event.Text)
		}
		return nil
	})
	if err != nil {
		return CompactResult{}, fmt.Errorf("compact request failed: %w", err)
	}

	summary := strings.TrimSpace(strings.Join(chunks, ""))
	if summary == "" {
		return CompactResult{}, fmt.Errorf("compact returned empty summary")
	}

	return CompactResult{
		Boundary:      domain.CompactBoundaryMessage(fmt.Sprintf("Conversation compacted on %s.", time.Now().Format("2006-01-02 15:04:05"))),
		Summary:       domain.TextMessage("system", compactSummaryMessage(summary)),
		PreservedTail: preservedTail,
	}, nil
}

func compactSystemInstruction() string {
	return strings.TrimSpace(`
You are summarizing a CLI coding session for context compaction.
Do not call tools.
Return plain text only.
Focus on user intent, files touched, key decisions, unresolved issues, and the exact current state of work.
`)
}

func compactPrompt() string {
	return strings.TrimSpace(`
Summarize the conversation above so a coding agent can continue work without losing context.

Include:
1. The user's current goal
2. Important files, commands, and tool results
3. Decisions already made
4. Bugs, errors, or constraints discovered
5. Exact pending work and the most recent state
`)
}

func compactSummaryMessage(summary string) string {
	return strings.TrimSpace("Compacted conversation summary:\n\n" + summary)
}

func cloneDomainMessages(messages []domain.Message) []domain.Message {
	out := make([]domain.Message, 0, len(messages))
	for _, message := range messages {
		cloned := message
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
