// Package tools provides the tool registry and built-in tools.
package tools

import (
	"encoding/json"
	"fmt"
)

// Args wraps tool arguments with typed accessor methods.
// Eliminates repetitive type assertions and improves error messages.
type Args map[string]interface{}

// String gets a required string argument.
func (a Args) String(key string) (string, error) {
	v, ok := a[key]
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
	v, ok := a[key]
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
	v, ok := a[key]
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
	v, ok := a[key]
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
	v, ok := a[key]
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
	v, ok := a[key]
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
	v, ok := a[key]
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
	v, ok := a[key]
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
// Handles []interface{} (JSON arrays decode as []interface{}).
func (a Args) StringSlice(key string) ([]string, error) {
	v, ok := a[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	return toStringSlice(v, key)
}

// StringSliceOr gets an optional string slice argument with a default.
func (a Args) StringSliceOr(key string, defaultVal []string) []string {
	v, ok := a[key]
	if !ok {
		return defaultVal
	}
	result, err := toStringSlice(v, key)
	if err != nil {
		return defaultVal
	}
	return result
}

// toStringSlice converts an interface{} to []string.
func toStringSlice(v interface{}, key string) ([]string, error) {
	switch arr := v.(type) {
	case []string:
		return arr, nil
	case []interface{}:
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
	_, ok := a[key]
	return ok
}

// Raw returns the raw value for a key, or nil if not present.
func (a Args) Raw(key string) interface{} {
	return a[key]
}

// filterNonEmpty returns a slice with empty strings removed.
func filterNonEmpty(slice []string) []string {
	if slice == nil {
		return nil
	}
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}
