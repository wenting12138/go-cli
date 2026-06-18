package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	"go-cli/internal/agent/react"
)

type ExecuteShellCommand struct{}

func (t ExecuteShellCommand) Name() string {
	return "execute_shell_command"
}

func (t ExecuteShellCommand) Description() string {
	return "Executes a shell command in the current workspace and returns the working directory, exit code, stdout, and stderr."
}

func (t ExecuteShellCommand) Parameters() react.ParameterSchema {
	return react.ParameterSchema{
		Type:     "object",
		Required: []string{"command"},
		Properties: map[string]react.ParameterSchema{
			"command": {
				Type:        "string",
				Description: "Shell command to execute.",
			},
			"path": {
				Type:        "string",
				Description: "Optional relative or absolute working directory inside the current workspace.",
			},
			"timeout_seconds": {
				Type:        "integer",
				Description: "Optional timeout in seconds. Defaults to 30 and is capped at 300.",
			},
		},
		AdditionalProperties: false,
	}
}

func (t ExecuteShellCommand) ValidateArgs(args map[string]any) error {
	if err := t.Parameters().Validate(args); err != nil {
		return err
	}

	command, _ := args["command"].(string)
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command must not be empty")
	}

	timeoutSeconds := intArg(args, "timeout_seconds", 30)
	if timeoutSeconds < 1 {
		return fmt.Errorf("timeout_seconds must be at least 1")
	}
	if timeoutSeconds > 300 {
		return fmt.Errorf("timeout_seconds must be at most 300")
	}

	path, _ := args["path"].(string)
	if strings.TrimSpace(path) != "" {
		workspaceRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve workspace root: %w", err)
		}
		if _, err := resolveWorkspacePath(workspaceRoot, path); err != nil {
			return err
		}
	}

	return nil
}

func (t ExecuteShellCommand) Run(ctx context.Context, args map[string]any) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	commandText, _ := args["command"].(string)
	timeoutSeconds := intArg(args, "timeout_seconds", 30)

	workspaceRoot, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	workingDir := workspaceRoot
	if path, _ := args["path"].(string); strings.TrimSpace(path) != "" {
		workingDir, err = resolveWorkspacePath(workspaceRoot, path)
		if err != nil {
			return "", err
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	command := shellCommand(runCtx, commandText)
	command.Dir = workingDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Run()

	exitCode := 0
	timedOut := false

	if err != nil {
		switch {
		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			timedOut = true
			exitCode = -1
		case errors.Is(err, context.Canceled):
			return "", err
		default:
			exitCode = exitCodeFromError(err)
			if exitCode == 0 {
				return "", fmt.Errorf("run command: %w", err)
			}
		}
	}

	return formatShellResult(workingDir, exitCode, timedOut, stdout.String(), stderr.String()), nil
}

func shellCommand(ctx context.Context, commandText string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", commandText)
	}
	return exec.CommandContext(ctx, "sh", "-lc", commandText)
}

func exitCodeFromError(err error) int {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0
	}

	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
		return status.ExitStatus()
	}
	return exitErr.ExitCode()
}

func formatShellResult(cwd string, exitCode int, timedOut bool, stdout, stderr string) string {
	lines := []string{
		fmt.Sprintf("cwd: %s", cwd),
		fmt.Sprintf("exit_code: %d", exitCode),
		fmt.Sprintf("timed_out: %t", timedOut),
		"stdout:",
		strings.TrimRight(stdout, "\n"),
		"stderr:",
		strings.TrimRight(stderr, "\n"),
	}
	return strings.Join(lines, "\n")
}
