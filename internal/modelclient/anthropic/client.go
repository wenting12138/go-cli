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
	"go-cli/internal/modelclient/endpointutil"
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
	Thinking  string         `json:"thinking,omitempty"`
	Signature string         `json:"signature,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
	Data      string         `json:"data,omitempty"`
}

func (b contentBlock) MarshalJSON() ([]byte, error) {
	payload := map[string]any{
		"type": b.Type,
	}

	switch b.Type {
	case "text":
		if b.Text != "" {
			payload["text"] = b.Text
		}
	case "thinking":
		if b.Thinking != "" {
			payload["thinking"] = b.Thinking
		}
		if b.Signature != "" {
			payload["signature"] = b.Signature
		}
	case "tool_use":
		if b.ID != "" {
			payload["id"] = b.ID
		}
		if b.Name != "" {
			payload["name"] = b.Name
		}
		if b.Input == nil {
			payload["input"] = map[string]any{}
		} else {
			payload["input"] = b.Input
		}
	case "tool_result":
		if b.ToolUseID != "" {
			payload["tool_use_id"] = b.ToolUseID
		}
		payload["content"] = b.Content
	case "redacted_thinking":
		if b.Data != "" {
			payload["data"] = b.Data
		}
	default:
		if b.Text != "" {
			payload["text"] = b.Text
		}
		if b.ID != "" {
			payload["id"] = b.ID
		}
		if b.Name != "" {
			payload["name"] = b.Name
		}
		if b.Input != nil {
			payload["input"] = b.Input
		}
		if b.ToolUseID != "" {
			payload["tool_use_id"] = b.ToolUseID
		}
		if b.Content != "" {
			payload["content"] = b.Content
		}
		if b.Data != "" {
			payload["data"] = b.Data
		}
		if b.Thinking != "" {
			payload["thinking"] = b.Thinking
		}
		if b.Signature != "" {
			payload["signature"] = b.Signature
		}
	}

	return json.Marshal(payload)
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
	Type      string         `json:"type,omitempty"`
	Text      string         `json:"text,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	Signature string         `json:"signature,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Data      string         `json:"data,omitempty"`
}

type payloadError struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message,omitempty"`
}

type toolUseAccumulator struct {
	Type          string
	Text          strings.Builder
	Thinking      strings.Builder
	Signature     string
	ID            string
	Name          string
	ArgumentsJSON strings.Builder
	Data          string
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

	endpoint := endpointutil.Resolve(c.model.BaseURL, "/messages", c.model.ShouldCompleteBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
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

	if eventName == "" {
		eventName = parsed.Type
	}

	switch eventName {
	case "content_block_start":
		acc := &toolUseAccumulator{
			Type:      parsed.ContentBlock.Type,
			Signature: parsed.ContentBlock.Signature,
			ID:        parsed.ContentBlock.ID,
			Name:      parsed.ContentBlock.Name,
			Data:      parsed.ContentBlock.Data,
		}
		if parsed.ContentBlock.Text != "" {
			acc.Text.WriteString(parsed.ContentBlock.Text)
		}
		if parsed.ContentBlock.Thinking != "" {
			acc.Thinking.WriteString(parsed.ContentBlock.Thinking)
		}
		if len(parsed.ContentBlock.Input) > 0 {
			data, err := json.Marshal(parsed.ContentBlock.Input)
			if err != nil {
				return fmt.Errorf("marshal anthropic tool input: %w", err)
			}
			acc.ArgumentsJSON.Write(data)
		}
		accumulators[parsed.Index] = acc
		switch parsed.ContentBlock.Type {
		case "text":
			if parsed.ContentBlock.Text != "" {
				return onEvent(protocol.Event{
					Type: protocol.EventTypeTextDelta,
					Text: parsed.ContentBlock.Text,
				})
			}
		case "thinking":
			if parsed.ContentBlock.Thinking != "" {
				return onEvent(protocol.Event{
					Type: protocol.EventTypeThinkingDelta,
					Text: parsed.ContentBlock.Thinking,
				})
			}
		case "redacted_thinking":
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
			acc, ok := accumulators[parsed.Index]
			if !ok {
				acc = &toolUseAccumulator{Type: "text"}
				accumulators[parsed.Index] = acc
			}
			acc.Text.WriteString(parsed.Delta.Text)
			return onEvent(protocol.Event{
				Type: protocol.EventTypeTextDelta,
				Text: parsed.Delta.Text,
			})
		case "thinking_delta":
			if parsed.Delta.Thinking == "" {
				return nil
			}
			acc, ok := accumulators[parsed.Index]
			if !ok {
				acc = &toolUseAccumulator{Type: "thinking"}
				accumulators[parsed.Index] = acc
			}
			acc.Thinking.WriteString(parsed.Delta.Thinking)
			return onEvent(protocol.Event{
				Type: protocol.EventTypeThinkingDelta,
				Text: parsed.Delta.Thinking,
			})
		case "signature_delta":
			if parsed.Delta.Signature == "" {
				return nil
			}
			acc, ok := accumulators[parsed.Index]
			if !ok {
				acc = &toolUseAccumulator{Type: "thinking"}
				accumulators[parsed.Index] = acc
			}
			acc.Signature = parsed.Delta.Signature
			return onEvent(protocol.Event{
				Type: protocol.EventTypeThinkingSignature,
				Text: parsed.Delta.Signature,
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
		if err := onEvent(protocol.Event{
			Type:            protocol.EventTypeAssistantBlocks,
			AssistantBlocks: assistantBlocksFromAccumulators(accumulators),
		}); err != nil {
			return err
		}
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
		if acc == nil || acc.Type != "tool_use" {
			continue
		}
		if acc.Name == "" {
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
	}
	return nil
}

func assistantBlocksFromAccumulators(accumulators map[int]*toolUseAccumulator) []domain.AssistantContentBlock {
	if len(accumulators) == 0 {
		return nil
	}

	indexes := make([]int, 0, len(accumulators))
	for index := range accumulators {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	blocks := make([]domain.AssistantContentBlock, 0, len(indexes))
	for _, index := range indexes {
		acc := accumulators[index]
		if acc == nil || acc.Type == "" {
			continue
		}

		block := domain.AssistantContentBlock{
			Type:      acc.Type,
			Text:      acc.Text.String(),
			Thinking:  acc.Thinking.String(),
			Signature: acc.Signature,
			ID:        acc.ID,
			Name:      acc.Name,
			Data:      acc.Data,
		}
		if acc.Type == "tool_use" {
			if args, err := parseArguments(acc.ArgumentsJSON.String()); err == nil {
				block.Input = args
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
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
	var pendingAssistant []contentBlock
	skipToolCalls := map[string]struct{}{}

	flushAssistant := func() {
		if len(pendingAssistant) == 0 {
			return
		}
		out = append(out, message{
			Role:    "assistant",
			Content: cloneContentBlocks(pendingAssistant),
		})
		pendingAssistant = nil
	}

	for _, entry := range messages {
		if entry.Type == domain.MessageTypeToolCall {
			if _, ok := skipToolCalls[entry.ToolCallID]; ok {
				delete(skipToolCalls, entry.ToolCallID)
				continue
			}
		}

		switch entry.Type {
		case domain.MessageTypeToolCall:
			pendingAssistant = append(pendingAssistant, contentBlock{
				Type:  "tool_use",
				ID:    entry.ToolCallID,
				Name:  entry.ToolName,
				Input: cloneMap(entry.Args),
			})
		case domain.MessageTypeToolResult:
			flushAssistant()
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
				flushAssistant()
				if strings.TrimSpace(entry.Content) != "" {
					systemParts = append(systemParts, entry.Content)
				}
				continue
			}
			if entry.ModelRole() != "assistant" {
				flushAssistant()
			}
			if entry.ModelRole() == "assistant" && len(entry.AssistantBlocks) > 0 {
				for _, block := range toContentBlocks(entry.AssistantBlocks) {
					pendingAssistant = append(pendingAssistant, block)
					if block.Type == "tool_use" && block.ID != "" {
						skipToolCalls[block.ID] = struct{}{}
					}
				}
				continue
			}
			blocks := make([]contentBlock, 0, 2)
			if entry.ModelRole() == "assistant" && strings.TrimSpace(entry.Reasoning) != "" {
				blocks = append(blocks, contentBlock{
					Type:      "thinking",
					Thinking:  entry.Reasoning,
					Signature: entry.ReasoningSignature,
				})
			}
			if entry.Content != "" || len(blocks) == 0 {
				blocks = append(blocks, contentBlock{
					Type: "text",
					Text: entry.Content,
				})
			}
			if entry.ModelRole() == "assistant" {
				pendingAssistant = append(pendingAssistant, blocks...)
				continue
			}
			out = append(out, message{
				Role:    entry.ModelRole(),
				Content: blocks,
			})
		}
	}
	flushAssistant()

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

func cloneContentBlocks(input []contentBlock) []contentBlock {
	if len(input) == 0 {
		return nil
	}
	cloned := make([]contentBlock, 0, len(input))
	for _, block := range input {
		next := block
		if block.Input != nil {
			next.Input = cloneMap(block.Input)
		}
		cloned = append(cloned, next)
	}
	return cloned
}

func toContentBlocks(blocks []domain.AssistantContentBlock) []contentBlock {
	out := make([]contentBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, contentBlock{
			Type:      block.Type,
			Text:      block.Text,
			Thinking:  block.Thinking,
			Signature: block.Signature,
			ID:        block.ID,
			Name:      block.Name,
			Input:     cloneMap(block.Input),
			ToolUseID: block.ToolUseID,
			Content:   block.Content,
			Data:      block.Data,
		})
	}
	return out
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
