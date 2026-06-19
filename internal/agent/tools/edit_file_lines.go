package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go-cli/internal/agent/react"
)

type EditFileLines struct{}

func (t EditFileLines) Name() string {
	return "edit_file_lines"
}

func (t EditFileLines) Description() string {
	return "Replaces a contiguous line range in a text file inside the current workspace. The edit semantics are delete the specified lines, then insert the provided replacement lines."
}

func (t EditFileLines) Parameters() react.ParameterSchema {
	return react.ParameterSchema{
		Type:     "object",
		Required: []string{"path", "start_line", "end_line", "new_lines"},
		Properties: map[string]react.ParameterSchema{
			"path": {
				Type:        "string",
				Description: "Relative or absolute file path inside the current workspace.",
			},
			"start_line": {
				Type:        "integer",
				Description: "1-based inclusive start line of the block to replace.",
			},
			"end_line": {
				Type:        "integer",
				Description: "1-based inclusive end line of the block to replace.",
			},
			"new_lines": {
				Type:        "string",
				Description: "Replacement multiline text to insert after deleting the original block. Use an empty string to delete the block without inserting anything.",
			},
		},
		AdditionalProperties: false,
	}
}

func (t EditFileLines) ValidateArgs(args map[string]any) error {
	if err := t.Parameters().Validate(args); err != nil {
		return err
	}

	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path must not be empty")
	}

	startLine := intArg(args, "start_line", 0)
	endLine := intArg(args, "end_line", 0)
	if startLine < 1 {
		return fmt.Errorf("start_line must be at least 1")
	}
	if endLine < startLine {
		return fmt.Errorf("end_line must be greater than or equal to start_line")
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

func (t EditFileLines) Run(ctx context.Context, args map[string]any) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	path, _ := args["path"].(string)
	startLine := intArg(args, "start_line", 0)
	endLine := intArg(args, "end_line", 0)
	newLines, _ := args["new_lines"].(string)

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

	originalText := string(data)
	hasTrailingNewline := strings.HasSuffix(originalText, "\n")
	lines := strings.Split(originalText, "\n")
	if hasTrailingNewline && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}

	if startLine > len(lines) {
		return "", fmt.Errorf("start_line %d is beyond file length %d", startLine, len(lines))
	}
	if endLine > len(lines) {
		return "", fmt.Errorf("end_line %d is beyond file length %d", endLine, len(lines))
	}

	before := append([]string(nil), lines[:startLine-1]...)
	after := append([]string(nil), lines[endLine:]...)
	replacedBlock := append([]string(nil), lines[startLine-1:endLine]...)

	replacement := splitPreserveEmpty(newLines)
	updated := append(before, replacement...)
	updated = append(updated, after...)

	output := strings.Join(updated, "\n")
	if hasTrailingNewline {
		output += "\n"
	}

	if err := os.WriteFile(resolvedPath, []byte(output), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	replacedText := strings.Join(replacedBlock, "\n")

	return fmt.Sprintf(
		"path: %s\nreplaced_lines: %d-%d\ninserted_lines: %d\nold_lines:\n%s",
		resolvedPath,
		startLine,
		endLine,
		len(replacement),
		replacedText,
	), nil
}

func splitPreserveEmpty(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}
