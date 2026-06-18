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
)

type StreamStartedMsg struct {
	Events <-chan tea.Msg
}

type StreamChunkMsg struct {
	Chunk string
}

type StreamErrorMsg struct {
	Err string
}

type StreamDoneMsg struct{}

func ReplyCmd(messages []domain.Message, model config.ModelConfig) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 32)

		go func() {
			defer close(events)

			client, err := modelclient.New(model)
			if err != nil {
				events <- StreamErrorMsg{Err: fmt.Sprintf("model request failed: %s", err)}
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			err = client.StreamChat(ctx, messages, func(chunk string) error {
				if chunk == "" {
					return nil
				}
				events <- StreamChunkMsg{Chunk: chunk}
				return nil
			})
			if err != nil {
				events <- StreamErrorMsg{Err: fmt.Sprintf("model request failed: %s", err)}
				return
			}

			events <- StreamDoneMsg{}
		}()

		return StreamStartedMsg{Events: events}
	}
}

func WaitStreamCmd(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return StreamDoneMsg{}
		}
		return msg
	}
}

func TrimReply(content string) string {
	return strings.TrimSpace(content)
}
