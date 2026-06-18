package bot

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-cli/internal/config"
	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/modules/shared"
)

const pageName = "chat"

func FakeReplyCmd(userText string, model config.ModelConfig) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(250 * time.Millisecond)
		return shared.PageUpdatedMsg{
			Page: pageName,
			Payload: domain.Message{
				Role: "assistant",
				Content: fmt.Sprintf(
					"You said: %q\n\nCurrent model config: %s/%s context_window=%d max_tokens=%d\n\nThis is a placeholder reply. Replace FakeReplyCmd with your real business logic or API call.",
					userText,
					model.Provider,
					model.Name,
					model.ContextWindow,
					model.MaxTokens,
				),
			},
		}
	}
}
