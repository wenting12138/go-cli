package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go-cli/internal/config"
	"go-cli/internal/modules/chat/domain"
)

type Client struct {
	model  config.ModelConfig
	client *http.Client
}

type request struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	Stream    bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type response struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func New(model config.ModelConfig) *Client {
	return &Client{
		model:  model,
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) Chat(ctx context.Context, messages []domain.Message) (string, error) {
	payload := request{
		Model:     c.model.Name,
		Messages:  toMessages(messages),
		MaxTokens: c.model.MaxTokens,
		Stream:    false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.model.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build openai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.model.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.model.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read openai response: %w", err)
	}

	var parsed response
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("parse openai response: %w", err)
	}
	if resp.StatusCode >= 400 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("openai error: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("openai error: %s", resp.Status)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response contained no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func toMessages(messages []domain.Message) []message {
	out := make([]message, 0, len(messages))
	for _, entry := range messages {
		out = append(out, message{
			Role:    normalizeRole(entry.Role),
			Content: entry.Content,
		})
	}
	return out
}

func normalizeRole(role string) string {
	switch role {
	case "assistant", "user", "system":
		return role
	default:
		return "user"
	}
}
