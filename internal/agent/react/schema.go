package react

type ParameterSchema struct {
	Type        string
	Description string
	Required    []string
	Properties  map[string]ParameterSchema
	Enum        []string
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
	return out
}

type ToolSpec struct {
	Name        string
	Description string
	Parameters  ParameterSchema
}
