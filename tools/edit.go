package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type editTool struct {
	workspace string
}

// Edit creates a tool that performs string replacement in files within the given workspace.
func Edit(workspace string) Tool {
	return &editTool{workspace: workspace}
}

func (t *editTool) Name() string { return "edit" }

func (t *editTool) Description() string {
	return "Find and replace text in a file. The old text must match exactly (including whitespace and newlines). Supports multiline matches. The old text must appear exactly once in the file — if ambiguous, include more surrounding context to make it unique."
}

func (t *editTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Path to the file to edit",
			Required:    true,
		},
		"old": {
			Type:        StringParam,
			Description: "Text to find (exact match)",
			Required:    true,
		},
		"new": {
			Type:        StringParam,
			Description: "Text to replace with",
			Required:    true,
		},
	}
}

func (t *editTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}
	old, err := args.String("old")
	if err != nil {
		return "", err
	}
	newText, err := args.String("new")
	if err != nil {
		return "", err
	}

	safePath, err := t.resolve(path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	oldContent := string(content)
	if !strings.Contains(oldContent, old) {
		return "", fmt.Errorf("pattern not found in file")
	}

	newContent := strings.Replace(oldContent, old, newText, 1)
	if err := os.WriteFile(safePath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return "ok", nil
}

func (t *editTool) resolve(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	path = filepath.Clean(path)

	if t.workspace != "" {
		ws := filepath.Clean(t.workspace)
		if !strings.HasPrefix(path, ws+string(filepath.Separator)) && path != ws {
			return "", fmt.Errorf("path %s is outside workspace %s", path, t.workspace)
		}
	}
	return path, nil
}
