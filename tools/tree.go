package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type treeTool struct {
	workspace string
}

// Tree creates a tree tool that shows directory structure.
// If workspace is non-empty, it is used as the default root path.
func Tree(workspace string) Tool {
	return &treeTool{workspace: workspace}
}

func (t *treeTool) Name() string { return "tree" }

func (t *treeTool) Description() string {
	return "Show directory structure as a tree. Default depth is 3. Use depth > 3 for deep exploration. Good first step to orient in an unfamiliar codebase before reading specific files."
}

func (t *treeTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Root directory path",
			Required:    true,
		},
		"depth": {
			Type:        IntParam,
			Description: "Maximum depth to traverse (default 3)",
		},
	}
}

func (t *treeTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}

	maxDepth := args.IntOr("depth", 3)

	var result strings.Builder
	result.WriteString(path + "\n")

	buildTree(&result, path, "", 0, maxDepth)

	return result.String(), nil
}

func buildTree(w *strings.Builder, dir, prefix string, depth, maxDepth int) {
	if depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Filter out hidden files
	var visible []os.DirEntry
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			visible = append(visible, e)
		}
	}

	for i, entry := range visible {
		isLast := i == len(visible)-1
		connector := "├── "
		childPrefix := "│   "
		if isLast {
			connector = "└── "
			childPrefix = "    "
		}

		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}

		w.WriteString(prefix + connector + name + "\n")

		if entry.IsDir() {
			buildTree(w, filepath.Join(dir, entry.Name()), prefix+childPrefix, depth+1, maxDepth)
		}
	}
}
