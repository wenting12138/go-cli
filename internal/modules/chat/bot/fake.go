package bot

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-cli/internal/modules/chat/domain"
)

func FakeReplyCmd(userText string, wrap func(domain.Message) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(250 * time.Millisecond)
		return wrap(domain.Message{
			Role: "assistant",
			Content: fmt.Sprintf(
				"You said: %q\n\nThis is a placeholder reply. Replace FakeReplyCmd with your real business logic or API call.",
				userText,
			),
		})
	}
}
