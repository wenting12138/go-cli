package modelclient

import (
	"context"
	"fmt"
	"strings"

	"go-cli/internal/config"
	"go-cli/internal/modelclient/ollama"
	"go-cli/internal/modelclient/openai"
	"go-cli/internal/modules/chat/domain"
)

type Client interface {
	StreamChat(ctx context.Context, messages []domain.Message, onChunk func(string) error) error
}

func New(model config.ModelConfig) (Client, error) {
	switch strings.ToLower(model.Provider) {
	case "openai", "openai-compatible":
		return openai.New(model), nil
	case "ollama":
		return ollama.New(model), nil
	default:
		return nil, fmt.Errorf("unsupported model provider %q", model.Provider)
	}
}
