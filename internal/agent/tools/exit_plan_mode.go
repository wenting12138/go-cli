package tools

import (
	"context"

	"go-cli/internal/agent/react"
)

type ExitPlanMode struct{}

func (t ExitPlanMode) Name() string {
	return "exit_plan_mode"
}

func (t ExitPlanMode) Description() string {
	return "Use this when the implementation plan is ready for user approval. This ends the current planning turn and asks the user to review the saved plan."
}

func (t ExitPlanMode) Parameters() react.ParameterSchema {
	return react.ParameterSchema{
		Type:                 "object",
		AdditionalProperties: false,
	}
}

func (t ExitPlanMode) ValidateArgs(args map[string]any) error {
	return t.Parameters().Validate(args)
}

func (t ExitPlanMode) Run(context.Context, map[string]any) (string, error) {
	return "plan ready for approval", nil
}

func (t ExitPlanMode) InterruptsTurn() bool {
	return true
}
