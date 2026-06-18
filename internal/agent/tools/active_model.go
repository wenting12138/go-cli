package tools

import (
	"context"
	"fmt"

	"go-cli/internal/agent/react"
	"go-cli/internal/config"
)

type ActiveModel struct {
	Model config.ModelConfig
}

func (t ActiveModel) Name() string {
	return "active_model"
}

func (t ActiveModel) Description() string {
	return "Returns the current model id, provider, model name, and context window."
}

func (t ActiveModel) Parameters() react.ParameterSchema {
	return react.ParameterSchema{
		Type:                 "object",
		Properties:           map[string]react.ParameterSchema{},
		AdditionalProperties: false,
	}
}

func (t ActiveModel) ValidateArgs(args map[string]any) error {
	return t.Parameters().Validate(args)
}

func (t ActiveModel) Run(ctx context.Context, args map[string]any) (string, error) {
	return fmt.Sprintf(
		"id=%s provider=%s name=%s context_window=%d",
		t.Model.ID,
		t.Model.Provider,
		t.Model.Name,
		t.Model.ContextWindow,
	), nil
}
