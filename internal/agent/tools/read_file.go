package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-cli/internal/agent/react"
)

type ReadFile struct{}

func (t ReadFile) Name() string {
	return "read_file"
}

func (t ReadFile) Description() string {
	return "Reads a text file from the current workspace. Supports optional line slicing."
}

func (t ReadFile) Parameters() react.ParameterSchema {
	return react.ParameterSchema{
		Type:     "object",
		Required: []string{"path"},
		Properties: map[string]react.ParameterSchema{
			"path": {
				Type:        "string",
				Description: "Relative or absolute file path inside the current workspace.",
			},
			"start_line": {
				Type:        "integer",
				Description: "Optional 1-based start line. Defaults to 1.",
			},
			"max_lines": {
				Type:        "integer",
				Description: "Optional maximum number of lines to return. Defaults to 80.",
			},
		},
		AdditionalProperties: false,
	}
}

func (t ReadFile) ValidateArgs(args map[string]any) error {
	if err := t.Parameters().Validate(args); err != nil {
		return err
	}

	startLine := intArg(args, "start_line", 1)
	if startLine < 1 {
		return fmt.Errorf("start_line must be at least 1")
	}

	maxLines := intArg(args, "max_lines", 80)
	if maxLines < 1 {
		return fmt.Errorf("max_lines must be at least 1")
	}
	if maxLines > 200 {
		return fmt.Errorf("max_lines must be at most 200")
	}

	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path must not be empty")
	}

	return nil
}

func (t ReadFile) Run(ctx context.Context, args map[string]any) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	path, _ := args["path"].(string)
	startLine := intArg(args, "start_line", 1)
	maxLines := intArg(args, "max_lines", 80)

	workspaceRoot, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	resolvedPath, err := resolveWorkspacePath(workspaceRoot, path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	startIndex := startLine - 1
	if startIndex >= len(lines) {
		return "", fmt.Errorf("start_line %d is beyond file length %d", startLine, len(lines))
	}

	endIndex := startIndex + maxLines
	if endIndex > len(lines) {
		endIndex = len(lines)
	}

	var output []string
	output = append(output, fmt.Sprintf("path: %s", resolvedPath))
	output = append(output, fmt.Sprintf("lines: %d-%d", startIndex+1, endIndex))
	for i := startIndex; i < endIndex; i++ {
		output = append(output, fmt.Sprintf("%d: %s", i+1, lines[i]))
	}

	return strings.Join(output, "\n"), nil
}

func resolveWorkspacePath(root, input string) (string, error) {
	cleanInput := filepath.Clean(strings.TrimSpace(input))
	var resolved string
	if filepath.IsAbs(cleanInput) {
		resolved = cleanInput
	} else {
		resolved = filepath.Join(root, cleanInput)
	}

	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	absolutePath, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	rel, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil {
		return "", fmt.Errorf("check path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the current workspace", input)
	}

	return absolutePath, nil
}

func intArg(args map[string]any, key string, fallback int) int {
	value, ok := args[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}
