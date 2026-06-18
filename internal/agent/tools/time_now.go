package tools

import (
	"context"
	"time"
)

type TimeNow struct{}

func (t TimeNow) Name() string {
	return "time_now"
}

func (t TimeNow) Description() string {
	return "Returns the current local time and RFC3339 timestamp. Args are optional."
}

func (t TimeNow) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t TimeNow) ValidateArgs(args map[string]any) error {
	return nil
}

func (t TimeNow) Run(ctx context.Context, args map[string]any) (string, error) {
	now := time.Now()
	return now.Format("2006-01-02 15:04:05 MST") + " | " + now.Format(time.RFC3339), nil
}
