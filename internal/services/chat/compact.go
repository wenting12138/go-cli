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
	RestoredFiles []domain.Message
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

	summaryInput, stats := microcompactMessages(messages[:len(messages)-preservedTailMessages])
	preservedTail := cloneDomainMessages(messages[len(messages)-preservedTailMessages:])
	restoredFiles := buildReadFileRestoration(messages[:len(messages)-preservedTailMessages], preservedTail)

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
		Boundary: domain.CompactBoundaryMessage(fmt.Sprintf(
			"Conversation compacted on %s. summarized=%d microcompact=%d->%d restored_files=%d preserved_tail=%d",
			time.Now().Format("2006-01-02 15:04:05"),
			len(summaryInput),
			stats.InputMessages,
			stats.OutputMessages,
			len(restoredFiles),
			len(preservedTail),
		)),
		Summary:       domain.TextMessage("system", compactSummaryMessage(summary)),
		RestoredFiles: restoredFiles,
		PreservedTail: preservedTail,
	}, nil
}

func compactSystemInstruction() string {
	return strings.TrimSpace(`
You are summarizing a CLI coding session for context compaction.
Do not call tools.
Return plain text only.
Focus on user intent, files touched, key decisions, unresolved issues, and the exact current state of work.
Your response must follow the requested structure exactly.
`)
}

func compactPrompt() string {
	return strings.TrimSpace(`
Summarize the conversation above so a coding agent can continue work without losing context.

Use exactly this structure:

CURRENT GOAL
- one or more bullets

USER CONSTRAINTS
- one or more bullets

KEY FILES
- <path>: why it matters

IMPORTANT COMMANDS AND TOOLS
- command/tool + result

DECISIONS MADE
- one or more bullets

ERRORS AND FIXES
- error -> fix

PENDING WORK
- one or more bullets

LATEST STATE
- describe precisely what was happening immediately before compaction

NEXT STEP
- the exact next action the agent should take

Rules:
- Be concrete. Prefer exact file paths, command names, and decisions.
- If a section has no content, write "- none".
- Do not add any section outside this template.
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

func buildReadFileRestoration(olderMessages, preservedTail []domain.Message) []domain.Message {
	preservedPaths := make(map[string]struct{})
	for _, entry := range preservedTail {
		if entry.Type == domain.MessageTypeToolCall && entry.ToolName == "read_file" {
			if path := readFilePath(entry.Args); path != "" {
				preservedPaths[path] = struct{}{}
			}
		}
	}

	type restoredFile struct {
		path    string
		summary string
	}

	var ordered []restoredFile
	seen := make(map[string]struct{})
	for i := len(olderMessages) - 2; i >= 0; i-- {
		call := olderMessages[i]
		result := olderMessages[i+1]
		if call.Type != domain.MessageTypeToolCall || call.ToolName != "read_file" {
			continue
		}
		if result.Type != domain.MessageTypeToolResult || result.ToolCallID != call.ToolCallID {
			continue
		}

		path := readFilePath(call.Args)
		if path == "" {
			continue
		}
		if _, ok := preservedPaths[path]; ok {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		ordered = append(ordered, restoredFile{
			path:    path,
			summary: summarizeReadFileResult(result.Result),
		})
		if len(ordered) == 3 {
			break
		}
	}

	out := make([]domain.Message, 0, len(ordered))
	for i := len(ordered) - 1; i >= 0; i-- {
		entry := ordered[i]
		out = append(out, domain.TextMessage("system", fmt.Sprintf(
			"Restored read_file context:\npath: %s\n%s",
			entry.path,
			entry.summary,
		)))
	}
	return out
}

func readFilePath(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	path, _ := args["path"].(string)
	return strings.TrimSpace(path)
}

func summarizeReadFileResult(result string) string {
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) == 0 {
		return "summary: empty result"
	}

	var kept []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "path: ") || strings.HasPrefix(line, "lines: ") {
			kept = append(kept, line)
			continue
		}
		if len(kept) >= 8 {
			break
		}
		kept = append(kept, line)
	}

	if len(kept) == 0 {
		return "summary: no retained lines"
	}
	return strings.Join(kept, "\n")
}
