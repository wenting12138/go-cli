package tools

import (
	"context"
	"reflect"
	"testing"

	"go-cli/internal/agent/react"
)

type testTool struct {
	name string
}

func (t testTool) Name() string { return t.name }

func (t testTool) Description() string { return t.name + " description" }

func (t testTool) Parameters() react.ParameterSchema { return react.ParameterSchema{Type: "object"} }

func (t testTool) ValidateArgs(args map[string]any) error { return nil }

func (t testTool) Run(ctx context.Context, args map[string]any) (string, error) { return "", nil }

func TestFilterByName(t *testing.T) {
	t.Parallel()

	toolset := []react.Tool{
		testTool{name: "read_file"},
		testTool{name: "grep_files"},
		testTool{name: "execute_shell_command"},
	}

	filtered := FilterByName(toolset, []string{"grep_files", "execute_shell_command"})

	got := make([]string, 0, len(filtered))
	for _, tool := range filtered {
		got = append(got, tool.Name())
	}

	want := []string{"grep_files", "execute_shell_command"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected filtered tools %v, got %v", want, got)
	}
}

func TestFilterByNameIgnoresDuplicatesAndWhitespace(t *testing.T) {
	t.Parallel()

	toolset := []react.Tool{
		testTool{name: "read_file"},
		testTool{name: "grep_files"},
	}

	filtered := FilterByName(toolset, []string{" grep_files ", "grep_files", ""})

	if len(filtered) != 1 || filtered[0].Name() != "grep_files" {
		t.Fatalf("expected only grep_files, got %#v", filtered)
	}
}
