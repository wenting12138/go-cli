package sessionstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-cli/internal/modules/chat/domain"
	"go-cli/internal/modules/chat/ui"
)

const (
	rootDirName = ".go-cli"
	sessionsDir = "sessions"
	uiFileName  = "ui.json"
	llmFileName = "model.json"
)

type Summary struct {
	ID        string
	Title     string
	Preview   string
	UpdatedAt time.Time
}

type SessionData struct {
	ID            string
	Title         string
	UIMessages    []ui.Message
	SystemPrompt  string
	ModelMessages []domain.Message
	UpdatedAt     time.Time
}

type Store struct {
	root string
}

type uiFile struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	UpdatedAt time.Time    `json:"updated_at"`
	Messages  []ui.Message `json:"messages"`
}

type modelFile struct {
	ID           string           `json:"id"`
	Title        string           `json:"title"`
	UpdatedAt    time.Time        `json:"updated_at"`
	SystemPrompt string           `json:"system_prompt,omitempty"`
	Messages     []domain.Message `json:"messages"`
}

func New() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	root := filepath.Join(home, rootDirName, sessionsDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) Save(data SessionData) error {
	if strings.TrimSpace(data.ID) == "" {
		return nil
	}

	if data.UpdatedAt.IsZero() {
		data.UpdatedAt = time.Now()
	}

	dir := filepath.Join(s.root, data.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}

	uiPayload := uiFile{
		ID:        data.ID,
		Title:     data.Title,
		UpdatedAt: data.UpdatedAt,
		Messages:  ui.CloneMessages(data.UIMessages),
	}
	modelPayload := modelFile{
		ID:           data.ID,
		Title:        data.Title,
		UpdatedAt:    data.UpdatedAt,
		SystemPrompt: data.SystemPrompt,
		Messages:     cloneMessages(data.ModelMessages),
	}

	if err := writeJSON(filepath.Join(dir, uiFileName), uiPayload); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, llmFileName), modelPayload); err != nil {
		return err
	}
	return nil
}

func (s *Store) Load(sessionID string) (SessionData, error) {
	dir := filepath.Join(s.root, sessionID)

	var uiPayload uiFile
	if err := readJSON(filepath.Join(dir, uiFileName), &uiPayload); err != nil {
		return SessionData{}, err
	}

	var modelPayload modelFile
	if err := readJSON(filepath.Join(dir, llmFileName), &modelPayload); err != nil {
		return SessionData{}, err
	}

	return SessionData{
		ID:            uiPayload.ID,
		Title:         uiPayload.Title,
		UIMessages:    ui.CloneMessages(uiPayload.Messages),
		SystemPrompt:  modelPayload.SystemPrompt,
		ModelMessages: cloneMessages(modelPayload.Messages),
		UpdatedAt:     uiPayload.UpdatedAt,
	}, nil
}

func (s *Store) List() ([]Summary, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		var payload uiFile
		if err := readJSON(filepath.Join(s.root, entry.Name(), uiFileName), &payload); err != nil {
			continue
		}

		summaries = append(summaries, Summary{
			ID:        payload.ID,
			Title:     payload.Title,
			Preview:   preview(payload.Messages),
			UpdatedAt: payload.UpdatedAt,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	return summaries, nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func cloneMessages(messages []domain.Message) []domain.Message {
	out := make([]domain.Message, 0, len(messages))
	for _, message := range messages {
		cloned := message
		if len(message.AssistantBlocks) > 0 {
			cloned.AssistantBlocks = cloneAssistantBlocks(message.AssistantBlocks)
		}
		if len(message.Args) > 0 {
			cloned.Args = make(map[string]any, len(message.Args))
			for key, value := range message.Args {
				cloned.Args[key] = value
			}
		}
		out = append(out, cloned)
	}
	return out
}

func cloneAssistantBlocks(blocks []domain.AssistantContentBlock) []domain.AssistantContentBlock {
	out := make([]domain.AssistantContentBlock, 0, len(blocks))
	for _, block := range blocks {
		cloned := block
		if len(block.Input) > 0 {
			cloned.Input = make(map[string]any, len(block.Input))
			for key, value := range block.Input {
				cloned.Input[key] = value
			}
		}
		out = append(out, cloned)
	}
	return out
}

func preview(messages []ui.Message) string {
	for _, message := range messages {
		if message.Type == ui.TypeUser && strings.TrimSpace(message.Content) != "" {
			text := strings.TrimSpace(message.Content)
			if len(text) > 56 {
				return text[:56] + "..."
			}
			return text
		}
	}
	return "No messages."
}
