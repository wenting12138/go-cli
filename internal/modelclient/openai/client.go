package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"go-cli/internal/config"
	"go-cli/internal/modelclient/endpointutil"
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat/domain"
)

type Client struct {
	model  config.ModelConfig
	client *http.Client
}

type request struct {
	Model      string    `json:"model"`
	Messages   []message `json:"messages"`
	Tools      []tool    `json:"tools,omitempty"`
	ToolChoice string    `json:"tool_choice,omitempty"`
	Stream     bool      `json:"stream"`
}

type message struct {
	Role             string            `json:"role"`
	Content          string            `json:"content,omitempty"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	ToolCalls        []messageToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string            `json:"tool_call_id,omitempty"`
}

type tool struct {
	Type     string       `json:"type"`
	Function functionSpec `json:"function"`
}

type functionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type messageToolCall struct {
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type,omitempty"`
	Function messageFunction `json:"function"`
}

type messageFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type response struct {
	Choices []struct {
		Message message `json:"message,omitempty"`
		Delta   struct {
			Content          string          `json:"content,omitempty"`
			ReasoningContent string          `json:"reasoning_content,omitempty"`
			ToolCalls        []deltaToolCall `json:"tool_calls,omitempty"`
		} `json:"delta,omitempty"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type deltaToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type toolCallAccumulator struct {
	ID            string
	Name          string
	ArgumentsJSON strings.Builder
}

func New(model config.ModelConfig) *Client {
	return &Client{
		model:  model,
		client: &http.Client{},
	}
}

func (c *Client) StreamChat(ctx context.Context, chatRequest protocol.ChatRequest, onEvent func(protocol.Event) error) error {
	payload := request{
		Model:    c.model.Name,
		Messages: toMessages(chatRequest.Messages),
		Tools:    toTools(chatRequest.Tools),
		Stream:   true,
	}
	if len(payload.Tools) > 0 {
		payload.ToolChoice = "auto"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal openai request: %w", err)
	}

	endpoint := endpointutil.Resolve(c.model.BaseURL, "/chat/completions", c.model.ShouldCompleteBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.model.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.model.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var parsed response
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err == nil && parsed.Error != nil && parsed.Error.Message != "" {
			return fmt.Errorf("openai error: %s", parsed.Error.Message)
		}
		return fmt.Errorf("openai error: %s", resp.Status)
	}

	accumulators := map[int]*toolCallAccumulator{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return flushToolCalls(accumulators, onEvent)
		}

		var parsed response
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			return fmt.Errorf("parse openai stream chunk: %w", err)
		}
		for _, choice := range parsed.Choices {
			if choice.Delta.ReasoningContent != "" {
				if err := onEvent(protocol.Event{
					Type: protocol.EventTypeThinkingDelta,
					Text: choice.Delta.ReasoningContent,
				}); err != nil {
					return err
				}
			}
			if choice.Delta.Content != "" {
				if err := onEvent(protocol.Event{
					Type: protocol.EventTypeTextDelta,
					Text: choice.Delta.Content,
				}); err != nil {
					return err
				}
			}

			for _, toolDelta := range choice.Delta.ToolCalls {
				acc, ok := accumulators[toolDelta.Index]
				if !ok {
					acc = &toolCallAccumulator{}
					accumulators[toolDelta.Index] = acc
				}
				if toolDelta.ID != "" {
					acc.ID = toolDelta.ID
				}
				if toolDelta.Function.Name != "" {
					acc.Name = toolDelta.Function.Name
				}
				if toolDelta.Function.Arguments != "" {
					acc.ArgumentsJSON.WriteString(toolDelta.Function.Arguments)
				}
			}

			if len(choice.Message.ToolCalls) > 0 {
				for index, toolCall := range choice.Message.ToolCalls {
					acc := &toolCallAccumulator{
						ID:   toolCall.ID,
						Name: toolCall.Function.Name,
					}
					acc.ArgumentsJSON.WriteString(toolCall.Function.Arguments)
					accumulators[index] = acc
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read openai stream: %w", err)
	}
	return flushToolCalls(accumulators, onEvent)
}

func flushToolCalls(accumulators map[int]*toolCallAccumulator, onEvent func(protocol.Event) error) error {
	if len(accumulators) == 0 {
		return nil
	}

	indexes := make([]int, 0, len(accumulators))
	for index := range accumulators {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	for _, index := range indexes {
		acc := accumulators[index]
		if acc == nil || acc.Name == "" {
			return fmt.Errorf("openai returned incomplete tool call at index %d", index)
		}
		args, err := parseArguments(acc.ArgumentsJSON.String())
		if err != nil {
			return fmt.Errorf("parse openai tool call arguments: %w", err)
		}
		if err := onEvent(protocol.Event{
			Type: protocol.EventTypeToolCall,
			ToolCall: protocol.ToolCall{
				ID:        acc.ID,
				Name:      acc.Name,
				Arguments: args,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func parseArguments(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	if args == nil {
		return map[string]any{}, nil
	}
	return args, nil
}

func toMessages(messages []domain.Message) []message {
	out := make([]message, 0, len(messages))
	for _, entry := range messages {
		switch entry.Type {
		case domain.MessageTypeToolCall:
			out = append(out, message{
				Role: "assistant",
				ToolCalls: []messageToolCall{
					{
						ID:   entry.ToolCallID,
						Type: "function",
						Function: messageFunction{
							Name:      entry.ToolName,
							Arguments: mustJSON(entry.Args),
						},
					},
				},
			})
		case domain.MessageTypeToolResult:
			out = append(out, message{
				Role:       "tool",
				Content:    entry.Result,
				ToolCallID: entry.ToolCallID,
			})
		default:
			out = append(out, message{
				Role:             entry.ModelRole(),
				Content:          entry.Content,
				ReasoningContent: entry.Reasoning,
			})
		}
	}
	return out
}

func toTools(definitions []protocol.ToolDefinition) []tool {
	out := make([]tool, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, tool{
			Type: "function",
			Function: functionSpec{
				Name:        definition.Name,
				Description: definition.Description,
				Parameters:  cloneMap(definition.Parameters),
			},
		})
	}
	return out
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func mustJSON(v any) string {
	if v == nil {
		return "{}"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}
