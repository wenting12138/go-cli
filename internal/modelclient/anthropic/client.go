package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"go-cli/internal/config"
	"go-cli/internal/modelclient/protocol"
	"go-cli/internal/modules/chat/domain"
)

const anthropicVersion = "2023-06-01"

type Client struct {
	model  config.ModelConfig
	client *http.Client
}

type request struct {
	Model     string      `json:"model"`
	System    string      `json:"system,omitempty"`
	Messages  []message   `json:"messages"`
	Tools     []tool      `json:"tools,omitempty"`
	MaxTokens int         `json:"max_tokens"`
	Stream    bool        `json:"stream"`
	Metadata  requestMeta `json:"metadata,omitempty"`
}

type requestMeta struct {
	UserID string `json:"user_id,omitempty"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type eventPayload struct {
	Type         string              `json:"type"`
	Index        int                 `json:"index,omitempty"`
	Delta        payloadDelta        `json:"delta,omitempty"`
	ContentBlock payloadContentBlock `json:"content_block,omitempty"`
	Error        *payloadError       `json:"error,omitempty"`
}

type payloadDelta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Signature   string `json:"signature,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type payloadContentBlock struct {
	Type  string         `json:"type,omitempty"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type payloadError struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

type toolUseAccumulator struct {
	ID            string
	Name          string
	ArgumentsJSON strings.Builder
}

func New(model config.ModelConfig) *Client {
	return &Client{
		model:  model,
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) StreamChat(ctx context.Context, chatRequest protocol.ChatRequest, onEvent func(protocol.Event) error) error {
	systemPrompt, messages := toMessages(chatRequest.Messages)
	payload := request{
		Model:     c.model.Name,
		System:    systemPrompt,
		Messages:  messages,
		Tools:     toTools(chatRequest.Tools),
		MaxTokens: defaultMaxTokens(c.model.ContextWindow),
		Stream:    true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal anthropic request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.model.BaseURL, "/")+"/messages", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Accept", "text/event-stream")
	if c.model.APIKey != "" {
		req.Header.Set("x-api-key", c.model.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return decodeError(resp)
	}

	accumulators := map[int]*toolUseAccumulator{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	var eventName string
	var dataLines []string

	flush := func() error {
		if eventName == "" && len(dataLines) == 0 {
			return nil
		}
		err := c.handleEvent(eventName, strings.Join(dataLines, "\n"), accumulators, onEvent)
		eventName = ""
		dataLines = dataLines[:0]
		return err
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read anthropic stream: %w", err)
	}
	if err := flush(); err != nil {
		return err
	}
	return flushToolUses(accumulators, onEvent)
}

func (c *Client) handleEvent(eventName, data string, accumulators map[int]*toolUseAccumulator, onEvent func(protocol.Event) error) error {
	if data == "" || eventName == "ping" {
		return nil
	}

	var parsed eventPayload
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		return fmt.Errorf("parse anthropic stream chunk: %w", err)
	}

	if parsed.Error != nil && parsed.Error.Message != "" {
		return fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}

	switch eventName {
	case "content_block_start":
		if parsed.ContentBlock.Type == "tool_use" {
			acc := &toolUseAccumulator{
				ID:   parsed.ContentBlock.ID,
				Name: parsed.ContentBlock.Name,
			}
			if len(parsed.ContentBlock.Input) > 0 {
				data, err := json.Marshal(parsed.ContentBlock.Input)
				if err != nil {
					return fmt.Errorf("marshal anthropic tool input: %w", err)
				}
				acc.ArgumentsJSON.Write(data)
			}
			accumulators[parsed.Index] = acc
		}
		if parsed.ContentBlock.Type == "redacted_thinking" {
			return onEvent(protocol.Event{
				Type: protocol.EventTypeRedactedThinking,
				Text: "[redacted thinking omitted]",
			})
		}
	case "content_block_delta":
		switch parsed.Delta.Type {
		case "text_delta":
			if parsed.Delta.Text == "" {
				return nil
			}
			return onEvent(protocol.Event{
				Type: protocol.EventTypeTextDelta,
				Text: parsed.Delta.Text,
			})
		case "thinking_delta":
			if parsed.Delta.Thinking == "" {
				return nil
			}
			return onEvent(protocol.Event{
				Type: protocol.EventTypeThinkingDelta,
				Text: parsed.Delta.Thinking,
			})
		case "signature_delta":
			if parsed.Delta.Signature == "" {
				return nil
			}
			return onEvent(protocol.Event{
				Type: protocol.EventTypeRedactedThinking,
				Text: "[redacted thinking omitted]",
			})
		case "input_json_delta":
			acc, ok := accumulators[parsed.Index]
			if !ok {
				acc = &toolUseAccumulator{}
				accumulators[parsed.Index] = acc
			}
			acc.ArgumentsJSON.WriteString(parsed.Delta.PartialJSON)
		}
	case "error":
		if parsed.Error != nil && parsed.Error.Message != "" {
			return fmt.Errorf("anthropic error: %s", parsed.Error.Message)
		}
	case "message_stop":
		return flushToolUses(accumulators, onEvent)
	}

	return nil
}

func decodeError(resp *http.Response) error {
	var parsed struct {
		Type  string `json:"type"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err == nil && parsed.Error != nil && parsed.Error.Message != "" {
		return fmt.Errorf("anthropic error: %s", parsed.Error.Message)
	}
	return fmt.Errorf("anthropic error: %s", resp.Status)
}

func flushToolUses(accumulators map[int]*toolUseAccumulator, onEvent func(protocol.Event) error) error {
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
			return fmt.Errorf("anthropic returned incomplete tool use at index %d", index)
		}
		args, err := parseArguments(acc.ArgumentsJSON.String())
		if err != nil {
			return fmt.Errorf("parse anthropic tool use input: %w", err)
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
		delete(accumulators, index)
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

func toMessages(messages []domain.Message) (string, []message) {
	systemParts := make([]string, 0, 4)
	out := make([]message, 0, len(messages))

	for _, entry := range messages {
		switch entry.Type {
		case domain.MessageTypeToolCall:
			out = append(out, message{
				Role: "assistant",
				Content: []contentBlock{
					{
						Type:  "tool_use",
						ID:    entry.ToolCallID,
						Name:  entry.ToolName,
						Input: cloneMap(entry.Args),
					},
				},
			})
		case domain.MessageTypeToolResult:
			out = append(out, message{
				Role: "user",
				Content: []contentBlock{
					{
						Type:      "tool_result",
						ToolUseID: entry.ToolCallID,
						Content:   entry.Result,
					},
				},
			})
		default:
			if entry.ModelRole() == "system" {
				if strings.TrimSpace(entry.Content) != "" {
					systemParts = append(systemParts, entry.Content)
				}
				continue
			}
			out = append(out, message{
				Role: entry.ModelRole(),
				Content: []contentBlock{
					{
						Type: "text",
						Text: entry.Content,
					},
				},
			})
		}
	}

	return strings.TrimSpace(strings.Join(systemParts, "\n\n")), out
}

func toTools(definitions []protocol.ToolDefinition) []tool {
	out := make([]tool, 0, len(definitions))
	for _, definition := range definitions {
		out = append(out, tool{
			Name:        definition.Name,
			Description: definition.Description,
			InputSchema: cloneMap(definition.Parameters),
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

func defaultMaxTokens(contextWindow int) int {
	if contextWindow <= 0 {
		return 4096
	}
	if contextWindow < 8192 {
		if contextWindow/2 < 1024 {
			return 1024
		}
		return contextWindow / 2
	}
	if contextWindow/8 < 4096 {
		return 4096
	}
	if contextWindow/8 > 8192 {
		return 8192
	}
	return contextWindow / 8
}
