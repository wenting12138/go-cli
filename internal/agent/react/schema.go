package react

import (
	"fmt"
	"sort"
)

type ParameterSchema struct {
	Type                 string
	Description          string
	Required             []string
	Properties           map[string]ParameterSchema
	Enum                 []string
	AdditionalProperties bool
}

func (s ParameterSchema) ToMap() map[string]any {
	out := map[string]any{}
	if s.Type != "" {
		out["type"] = s.Type
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if len(s.Required) > 0 {
		required := make([]string, len(s.Required))
		copy(required, s.Required)
		out["required"] = required
	}
	if len(s.Enum) > 0 {
		enum := make([]string, len(s.Enum))
		copy(enum, s.Enum)
		out["enum"] = enum
	}
	if len(s.Properties) > 0 {
		properties := make(map[string]any, len(s.Properties))
		for name, property := range s.Properties {
			properties[name] = property.ToMap()
		}
		out["properties"] = properties
	}
	if s.Type == "object" {
		out["additionalProperties"] = s.AdditionalProperties
	}
	return out
}

func (s ParameterSchema) Validate(value any) error {
	switch s.Type {
	case "", "any":
		return nil
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object, got %s", valueType(value))
		}
		return s.validateObject(obj)
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string, got %s", valueType(value))
		}
		return s.validateEnum(text)
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %s", valueType(value))
		}
		return nil
	case "number":
		if !isNumber(value) {
			return fmt.Errorf("expected number, got %s", valueType(value))
		}
		return nil
	case "integer":
		if !isInteger(value) {
			return fmt.Errorf("expected integer, got %s", valueType(value))
		}
		return nil
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("expected array, got %s", valueType(value))
		}
		return nil
	default:
		return fmt.Errorf("unsupported schema type %q", s.Type)
	}
}

func (s ParameterSchema) validateObject(obj map[string]any) error {
	for _, name := range s.Required {
		if _, ok := obj[name]; !ok {
			return fmt.Errorf("missing required field %q", name)
		}
	}

	if !s.AdditionalProperties {
		for name := range obj {
			if _, ok := s.Properties[name]; !ok {
				return fmt.Errorf("unknown field %q", name)
			}
		}
	}

	propertyNames := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		propertyNames = append(propertyNames, name)
	}
	sort.Strings(propertyNames)

	for _, name := range propertyNames {
		property := s.Properties[name]
		value, ok := obj[name]
		if !ok {
			continue
		}
		if err := property.Validate(value); err != nil {
			return fmt.Errorf("field %q: %w", name, err)
		}
	}

	return nil
}

func (s ParameterSchema) validateEnum(text string) error {
	if len(s.Enum) == 0 {
		return nil
	}
	for _, candidate := range s.Enum {
		if text == candidate {
			return nil
		}
	}
	return fmt.Errorf("must be one of %v", s.Enum)
}

func valueType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64:
		return "integer"
	case uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "number"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	default:
		return false
	}
}

func isInteger(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

type ToolSpec struct {
	Name        string
	Description string
	Parameters  ParameterSchema
}
