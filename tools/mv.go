package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type mvTool struct {
	workspace string
}

// Mv creates a tool that moves or renames files/directories within the given workspace.
func Mv(workspace string) Tool {
	return &mvTool{workspace: workspace}
}

func (t *mvTool) Name() string { return "mv" }

func (t *mvTool) Description() string {
	return "Move or rename a file or directory."
}

func (t *mvTool) Parameters() map[string]Param {
	return map[string]Param{
		"source": {
			Type:        StringParam,
			Description: "Source path",
			Required:    true,
		},
		"destination": {
			Type:        StringParam,
			Description: "Destination path",
			Required:    true,
		},
	}
}

func (t *mvTool) Execute(ctx context.Context, args Args) (string, error) {
	src, err := args.String("source")
	if err != nil {
		return "", err
	}
	dst, err := args.String("destination")
	if err != nil {
		return "", err
	}

	src, err = t.resolve(src)
	if err != nil {
		return "", fmt.Errorf("source: %w", err)
	}
	dst, err = t.resolve(dst)
	if err != nil {
		return "", fmt.Errorf("destination: %w", err)
	}

	if err := os.Rename(src, dst); err != nil {
		return "", fmt.Errorf("failed to move: %w", err)
	}

	return fmt.Sprintf("Moved %s → %s", src, dst), nil
}

func (t *mvTool) resolve(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	path = filepath.Clean(path)

	if t.workspace != "" && !strings.HasPrefix(path, filepath.Clean(t.workspace)+string(filepath.Separator)) && path != filepath.Clean(t.workspace) {
		return "", fmt.Errorf("path %s is outside workspace %s", path, t.workspace)
	}
	return path, nil
}
