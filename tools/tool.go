package tools

import "context"

// ParamType constrains tool parameter types.
type ParamType string

const (
	StringParam ParamType = "string"
	IntParam    ParamType = "integer"
	BoolParam   ParamType = "boolean"
	ArrayParam  ParamType = "array"
)

// Param defines a single tool parameter.
type Param struct {
	Type        ParamType
	Description string
	Enum        []string // optional, for constrained values
	Required    bool
}

// Tool is something an LLM can call.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]Param
	Execute(ctx context.Context, args Args) (string, error)
}

// Guard checks whether a tool call should proceed.
type Guard interface {
	Check(ctx context.Context, args Args) error
}

// CredentialProvider provides API keys for tools.
type CredentialProvider interface {
	GetAPIKey(provider string) string
}

// Definition is the LLM-facing tool description.
type Definition struct {
	Name        string
	Description string
	Parameters  map[string]Param
}

// JSONSchema converts Parameters to a JSON Schema object suitable for LLM APIs.
func (d Definition) JSONSchema() map[string]any {
	if len(d.Parameters) == 0 {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	properties := make(map[string]any, len(d.Parameters))
	var required []string

	for name, p := range d.Parameters {
		prop := map[string]any{
			"type":        string(p.Type),
			"description": p.Description,
		}
		if len(p.Enum) > 0 {
			prop["enum"] = p.Enum
		}
		properties[name] = prop

		if p.Required {
			required = append(required, name)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
