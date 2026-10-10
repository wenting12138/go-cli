package tools

import (
	"strings"

	"go-cli/internal/agent/react"
	"go-cli/internal/agentmode"
	"go-cli/internal/config"
)

func Build(model config.ModelConfig, mode agentmode.Mode) []react.Tool {
	return []react.Tool{
		TimeNow{},
		ReadFile{},
		GrepFiles{},
		ExecuteShellCommand{},
		WriteTextFile{},
		EditFileLines{},
	}
}

func FilterByName(toolset []react.Tool, allowed []string) []react.Tool {
	if len(allowed) == 0 {
		return toolset
	}

	lookup := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		lookup[trimmed] = struct{}{}
	}
	if len(lookup) == 0 {
		return nil
	}

	filtered := make([]react.Tool, 0, len(toolset))
	for _, tool := range toolset {
		if _, ok := lookup[tool.Name()]; ok {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func Specs(model config.ModelConfig, mode agentmode.Mode) []react.ToolSpec {
	toolset := Build(model, mode)
	specs := make([]react.ToolSpec, 0, len(toolset))
	for _, tool := range toolset {
		specs = append(specs, react.ToolSpec{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  tool.Parameters(),
		})
	}
	return specs
}
