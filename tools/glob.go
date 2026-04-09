package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type globTool struct {
	workspace string
}

// Glob creates a tool that finds files matching a glob pattern within the given workspace.
func Glob(workspace string) Tool {
	return &globTool{workspace: workspace}
}

func (t *globTool) Name() string { return "glob" }

func (t *globTool) Description() string {
	return "Find files matching a glob pattern. Supports * (any chars in one dir), ? (one char), and ** (recursive across directories). Examples: '*.go', 'src/**/*.ts', 'config/*.{json,toml}'."
}

func (t *globTool) Parameters() map[string]Param {
	return map[string]Param{
		"pattern": {
			Type:        StringParam,
			Description: "Glob pattern (e.g., *.go, **/*.txt)",
			Required:    true,
		},
	}
}

func (t *globTool) Execute(ctx context.Context, args Args) (string, error) {
	pattern, err := args.String("pattern")
	if err != nil {
		return "", err
	}

	// If pattern is relative, anchor it to workspace
	if t.workspace != "" && !filepath.IsAbs(pattern) {
		pattern = filepath.Join(t.workspace, pattern)
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid pattern: %w", err)
	}

	if len(matches) == 0 {
		return "No matches found.", nil
	}

	var sb strings.Builder
	for i, m := range matches {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(m)
	}
	return sb.String(), nil
}
