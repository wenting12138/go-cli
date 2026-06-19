package tools

import (
	"go-cli/internal/agent/react"
	"go-cli/internal/agentmode"
	"go-cli/internal/config"
)

func Build(model config.ModelConfig, mode agentmode.Mode) []react.Tool {
	if mode == agentmode.Plan {
		return []react.Tool{
			TimeNow{},
			ReadFile{},
			GrepFiles{},
			ExitPlanMode{},
		}
	}

	return []react.Tool{
		TimeNow{},
		ReadFile{},
		GrepFiles{},
		ExecuteShellCommand{},
		WriteTextFile{},
		EditFileLines{},
	}
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
