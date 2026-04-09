package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dirEntry represents a directory entry (unexported).
type dirEntry struct {
	name  string
	isDir bool
	size  int64
}

type lsTool struct {
	workspace string
}

// Ls creates a tool that lists directory contents within the given workspace.
func Ls(workspace string) Tool {
	return &lsTool{workspace: workspace}
}

func (t *lsTool) Name() string { return "ls" }

func (t *lsTool) Description() string {
	return "List directory contents. Returns name, type (file/dir), and size for each entry. Non-recursive — shows only immediate children. Use tree for recursive structure."
}

func (t *lsTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Directory path to list",
			Required:    true,
		},
	}
}

func (t *lsTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}

	path, err = t.resolve(path)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var result []dirEntry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, dirEntry{
			name:  e.Name(),
			isDir: e.IsDir(),
			size:  info.Size(),
		})
	}

	if len(result) == 0 {
		return "Empty directory.", nil
	}

	var sb strings.Builder
	for i, entry := range result {
		if i > 0 {
			sb.WriteByte('\n')
		}
		kind := "file"
		if entry.isDir {
			kind = "dir"
		}
		fmt.Fprintf(&sb, "%s\t%s\t%d", entry.name, kind, entry.size)
	}
	return sb.String(), nil
}

func (t *lsTool) resolve(path string) (string, error) {
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
