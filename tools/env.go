package tools

import (
	"context"
	"os"
	"strings"
)

type envTool struct{}

// Env returns a tool that reads environment variables.
func Env() Tool { return &envTool{} }

func (t *envTool) Name() string { return "env" }
func (t *envTool) Description() string {
	return "Read environment variables. Returns a specific variable by name, or lists all non-secret variables."
}
func (t *envTool) Parameters() map[string]Param {
	return map[string]Param{
		"name": {
			Type:        StringParam,
			Description: "Environment variable name to read (optional — omit to list all non-secret vars)",
			Required:    false,
		},
	}
}

func (t *envTool) Execute(ctx context.Context, args Args) (string, error) {
	name := args.StringOr("name", "")

	if name != "" {
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", nil
		}
		if isSensitiveEnvVar(name) {
			return "[redacted]", nil
		}
		return val, nil
	}

	// List all, filtering secrets
	var lines []string
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if isSensitiveEnvVar(parts[0]) {
			continue
		}
		lines = append(lines, e)
	}
	return strings.Join(lines, "\n"), nil
}

// isSensitiveEnvVar returns true for env vars that likely contain secrets.
func isSensitiveEnvVar(name string) bool {
	upper := strings.ToUpper(name)
	sensitive := []string{"KEY", "SECRET", "TOKEN", "PASSWORD", "PASS", "CREDENTIAL", "AUTH", "PRIVATE"}
	for _, s := range sensitive {
		if strings.Contains(upper, s) {
			return true
		}
	}
	return false
}
