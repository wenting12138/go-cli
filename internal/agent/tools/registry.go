package tools

import (
	"go-cli/internal/agent/react"
	"go-cli/internal/config"
)

func Build(model config.ModelConfig) []react.Tool {
	return []react.Tool{
		TimeNow{},
		ActiveModel{Model: model},
		ReadFile{},
		GrepFiles{},
		ExecuteShellCommand{},
		WriteTextFile{},
		EditFileLines{},
	}
}

func Specs(model config.ModelConfig) []react.ToolSpec {
	toolset := Build(model)
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
