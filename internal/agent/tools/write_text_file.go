package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-cli/internal/agent/react"
)

type WriteTextFile struct{}

func (t WriteTextFile) Name() string {
	return "write_text_file"
}

func (t WriteTextFile) Description() string {
	return "Writes UTF-8 text content to a file inside the current workspace. Supports overwrite or append mode."
}

func (t WriteTextFile) Parameters() react.ParameterSchema {
	return react.ParameterSchema{
		Type:     "object",
		Required: []string{"path", "content"},
		Properties: map[string]react.ParameterSchema{
			"path": {
				Type:        "string",
				Description: "Relative or absolute file path inside the current workspace.",
			},
			"content": {
				Type:        "string",
				Description: "Text content to write.",
			},
			"append": {
				Type:        "boolean",
				Description: "If true, append to the file instead of replacing it. Defaults to false.",
			},
		},
		AdditionalProperties: false,
	}
}

func (t WriteTextFile) ValidateArgs(args map[string]any) error {
	if err := t.Parameters().Validate(args); err != nil {
		return err
	}

	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path must not be empty")
	}

	if _, ok := args["content"].(string); !ok {
		return fmt.Errorf("content must be a string")
	}

	workspaceRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}
	if _, err := resolveWorkspacePath(workspaceRoot, path); err != nil {
		return err
	}

	return nil
}

func (t WriteTextFile) Run(ctx context.Context, args map[string]any) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	appendMode, _ := args["append"].(bool)

	workspaceRoot, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	resolvedPath, err := resolveWorkspacePath(workspaceRoot, path)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return "", fmt.Errorf("create parent directories: %w", err)
	}

	var flag int
	if appendMode {
		flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	} else {
		flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}

	file, err := os.OpenFile(resolvedPath, flag, 0o644)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	mode := "overwrite"
	if appendMode {
		mode = "append"
	}

	return fmt.Sprintf("path: %s\nmode: %s\nbytes_written: %d", resolvedPath, mode, len(content)), nil
}
