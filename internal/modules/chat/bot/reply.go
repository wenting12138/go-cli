package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-cli/internal/config"
	"go-cli/internal/modelclient"
	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/modules/shared"
)

const pageName = "chat"

func ReplyCmd(messages []domain.Message, model config.ModelConfig) tea.Cmd {
	return func() tea.Msg {
		client, err := modelclient.New(model)
		if err != nil {
			return failureMessage(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		reply, err := client.Chat(ctx, messages)
		if err != nil {
			return failureMessage(err)
		}

		return shared.PageUpdatedMsg{
			Page: pageName,
			Payload: domain.Message{
				Role:    "assistant",
				Content: strings.TrimSpace(reply),
			},
		}
	}
}

func failureMessage(err error) tea.Msg {
	return shared.PageUpdatedMsg{
		Page: pageName,
		Payload: domain.Message{
			Role:    "system",
			Content: fmt.Sprintf("model request failed: %s", err),
		},
	}
}
