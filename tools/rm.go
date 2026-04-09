package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type rmTool struct {
	workspace string
}

// Rm creates a tool that deletes files/directories within the given workspace.
func Rm(workspace string) Tool {
	return &rmTool{workspace: workspace}
}

func (t *rmTool) Name() string { return "rm" }

func (t *rmTool) Description() string {
	return "Delete a file or empty directory. Use recursive=true for non-empty directories."
}

func (t *rmTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Path to delete",
			Required:    true,
		},
		"recursive": {
			Type:        BoolParam,
			Description: "Delete directories and contents recursively",
		},
	}
}

func (t *rmTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}

	path, err = t.resolve(path)
	if err != nil {
		return "", err
	}

	recursive := args.BoolOr("recursive", false)

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("path not found: %w", err)
	}

	if info.IsDir() && !recursive {
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("directory not empty (use recursive=true): %w", err)
		}
	} else if recursive {
		if err := os.RemoveAll(path); err != nil {
			return "", fmt.Errorf("failed to delete: %w", err)
		}
	} else {
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("failed to delete: %w", err)
		}
	}

	return fmt.Sprintf("Deleted: %s", path), nil
}

func (t *rmTool) resolve(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	path = filepath.Clean(path)

	if t.workspace != "" && !strings.HasPrefix(path, filepath.Clean(t.workspace)+string(filepath.Separator)) && path != filepath.Clean(t.workspace) {
		return "", fmt.Errorf("path %s is outside workspace %s", path, t.workspace)
	}
	return path, nil
}
