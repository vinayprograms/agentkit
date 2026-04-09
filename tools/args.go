package tools

import (
	"encoding/json"
	"fmt"
)

// Args provides typed access to validated tool arguments.
type Args struct {
	values map[string]any
}

// Validate checks raw args against param definitions and returns typed Args.
func Validate(params map[string]Param, raw map[string]any) (Args, error) {
	for name, p := range params {
		v, exists := raw[name]
		if !exists || v == nil {
			if p.Required {
				return Args{}, fmt.Errorf("%s is required", name)
			}
			continue
		}

		switch p.Type {
		case StringParam:
			if _, ok := v.(string); !ok {
				return Args{}, fmt.Errorf("%s must be a string, got %T", name, v)
			}
		case IntParam:
			switch v.(type) {
			case int, float64, json.Number:
			default:
				return Args{}, fmt.Errorf("%s must be a number, got %T", name, v)
			}
		case BoolParam:
			if _, ok := v.(bool); !ok {
				return Args{}, fmt.Errorf("%s must be a boolean, got %T", name, v)
			}
		case ArrayParam:
			switch v.(type) {
			case []string, []any:
			default:
				return Args{}, fmt.Errorf("%s must be an array, got %T", name, v)
			}
		}
	}

	return Args{values: raw}, nil
}

// String gets a required string argument.
func (a Args) String(key string) (string, error) {
	v, ok := a.values[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %T", key, v)
	}
	return s, nil
}

// StringOr gets an optional string argument with a default.
func (a Args) StringOr(key, defaultVal string) string {
	v, ok := a.values[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// Int gets a required integer argument.
// Handles both int and float64 (JSON numbers decode as float64).
func (a Args) Int(key string) (int, error) {
	v, ok := a.values[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case float64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("%s must be a number, got %T", key, v)
	}
}

// IntOr gets an optional integer argument with a default.
func (a Args) IntOr(key string, defaultVal int) int {
	v, ok := a.values[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return defaultVal
	}
}

// Float gets a required float64 argument.
func (a Args) Float(key string) (float64, error) {
	v, ok := a.values[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case json.Number:
		return n.Float64()
	default:
		return 0, fmt.Errorf("%s must be a number, got %T", key, v)
	}
}

// FloatOr gets an optional float64 argument with a default.
func (a Args) FloatOr(key string, defaultVal float64) float64 {
	v, ok := a.values[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return defaultVal
	}
}

// Bool gets a required boolean argument.
func (a Args) Bool(key string) (bool, error) {
	v, ok := a.values[key]
	if !ok {
		return false, fmt.Errorf("%s is required", key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean, got %T", key, v)
	}
	return b, nil
}

// BoolOr gets an optional boolean argument with a default.
func (a Args) BoolOr(key string, defaultVal bool) bool {
	v, ok := a.values[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// StringSlice gets a required string slice argument.
// Handles []any (JSON arrays decode as []any).
func (a Args) StringSlice(key string) ([]string, error) {
	v, ok := a.values[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	return toStringSlice(v, key)
}

// StringSliceOr gets an optional string slice argument with a default.
func (a Args) StringSliceOr(key string, defaultVal []string) []string {
	v, ok := a.values[key]
	if !ok {
		return defaultVal
	}
	result, err := toStringSlice(v, key)
	if err != nil {
		return defaultVal
	}
	return result
}

func toStringSlice(v any, key string) ([]string, error) {
	switch arr := v.(type) {
	case []string:
		return arr, nil
	case []any:
		result := make([]string, 0, len(arr))
		for i, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] must be a string, got %T", key, i, item)
			}
			result = append(result, s)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%s must be an array, got %T", key, v)
	}
}

// Has returns true if the key exists in the arguments.
func (a Args) Has(key string) bool {
	_, ok := a.values[key]
	return ok
}
