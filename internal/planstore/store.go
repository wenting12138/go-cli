package planstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	rootDirName = ".go-cli"
	plansDir    = "plans"
)

type Store struct {
	root string
}

func New() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	root := filepath.Join(home, rootDirName, plansDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create plans dir: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) Path(sessionID string) string {
	return filepath.Join(s.root, sanitizeSessionID(sessionID)+".md")
}

func (s *Store) Save(sessionID, content string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", nil
	}
	path := s.Path(sessionID)
	data := []byte(strings.TrimSpace(content))
	if len(data) > 0 {
		data = append(data, '\n')
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

func (s *Store) Load(sessionID string) (string, string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", "", nil
	}
	path := s.Path(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, "", nil
		}
		return path, "", fmt.Errorf("read %s: %w", path, err)
	}
	return path, string(data), nil
}

func sanitizeSessionID(sessionID string) string {
	replacer := strings.NewReplacer("\\", "-", "/", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	value := strings.TrimSpace(replacer.Replace(sessionID))
	if value == "" {
		return "plan"
	}
	return value
}
