package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Tools    []tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role      string            `json:"role"`
	Content   string            `json:"content,omitempty"`
	ToolCalls []messageToolCall `json:"tool_calls,omitempty"`
	Name      string            `json:"name,omitempty"`
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
	Function toolFunction `json:"function"`
}

type toolFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type response struct {
	Message            message `json:"message"`
	Error              string  `json:"error,omitempty"`
	Done               bool    `json:"done,omitempty"`
	TotalDuration      int64   `json:"total_duration,omitempty"`
	LoadDuration       int64   `json:"load_duration,omitempty"`
	PromptEvalCount    int     `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64   `json:"prompt_eval_duration,omitempty"`
	EvalCount          int     `json:"eval_count,omitempty"`
	EvalDuration       int64   `json:"eval_duration,omitempty"`
}

func New(model config.ModelConfig) *Client {
	return &Client{
		model:  model,
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) StreamChat(ctx context.Context, chatRequest protocol.ChatRequest, onEvent func(protocol.Event) error) error {
	payload := request{
		Model:    c.model.Name,
		Messages: toMessages(chatRequest.Messages),
		Tools:    toTools(chatRequest.Tools),
		Stream:   true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ollama request: %w", err)
	}

	endpoint := endpointutil.Resolve(c.model.BaseURL, "/api/chat", c.model.ShouldCompleteBaseURL())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.model.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.model.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var parsed response
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err == nil && parsed.Error != "" {
			return fmt.Errorf("ollama error: %s", parsed.Error)
		}
		return fmt.Errorf("ollama error: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var parsed response
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			return fmt.Errorf("parse ollama stream chunk: %w", err)
		}
		if parsed.Error != "" {
			return fmt.Errorf("ollama error: %s", parsed.Error)
		}

		if parsed.Message.Content != "" {
			if err := onEvent(protocol.Event{
				Type: protocol.EventTypeTextDelta,
				Text: parsed.Message.Content,
			}); err != nil {
				return err
			}
		}

		for index, toolCall := range parsed.Message.ToolCalls {
			if toolCall.Function.Name == "" {
				return fmt.Errorf("ollama returned incomplete tool call at index %d", index)
			}
			if err := onEvent(protocol.Event{
				Type: protocol.EventTypeToolCall,
				ToolCall: protocol.ToolCall{
					ID:        fmt.Sprintf("call-%d", index+1),
					Name:      toolCall.Function.Name,
					Arguments: cloneMap(toolCall.Function.Arguments),
				},
			}); err != nil {
				return err
			}
		}

		if parsed.Done {
			if err := onEvent(protocol.Event{
				Type: protocol.EventTypeMetrics,
				Metrics: protocol.ResponseMetrics{
					TotalDuration:      parsed.TotalDuration,
					LoadDuration:       parsed.LoadDuration,
					PromptEvalCount:    parsed.PromptEvalCount,
					PromptEvalDuration: parsed.PromptEvalDuration,
					EvalCount:          parsed.EvalCount,
					EvalDuration:       parsed.EvalDuration,
				},
			}); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read ollama stream: %w", err)
	}
	return nil
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
						Function: toolFunction{
							Name:      entry.ToolName,
							Arguments: cloneMap(entry.Args),
						},
					},
				},
			})
		case domain.MessageTypeToolResult:
			out = append(out, message{
				Role:    "tool",
				Content: entry.Result,
				Name:    entry.ToolName,
			})
		default:
			out = append(out, message{
				Role:    entry.ModelRole(),
				Content: entry.Content,
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
