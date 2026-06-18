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
	"go-cli/internal/modules/chat/domain"
)

type Client struct {
	model  config.ModelConfig
	client *http.Client
}

type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	Options  options   `json:"options,omitempty"`
}

type options struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type response struct {
	Message message `json:"message"`
	Error   string  `json:"error,omitempty"`
}

func New(model config.ModelConfig) *Client {
	return &Client{
		model:  model,
		client: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) StreamChat(ctx context.Context, messages []domain.Message, onChunk func(string) error) error {
	payload := request{
		Model:    c.model.Name,
		Messages: toMessages(messages),
		Stream:   true,
		Options: options{
			NumPredict: c.model.MaxTokens,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.model.BaseURL, "/")+"/api/chat", bytes.NewReader(body))
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
		if parsed.Message.Content == "" {
			continue
		}
		if err := onChunk(parsed.Message.Content); err != nil {
			return err
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
