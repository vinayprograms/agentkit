package tools

import (
	"context"
	"fmt"
	"os"
)

type readTool struct {
	workspace  string
	extraRoots []string
}

// Read creates a tool that reads file contents within the given workspace.
func Read(workspace string, extraRoots ...string) Tool {
	return &readTool{workspace: workspace, extraRoots: extraRoots}
}

func (t *readTool) Name() string { return "read" }

func (t *readTool) Description() string {
	return "Read the contents of a file. Returns the full file content. For large files, use head/tail to read specific portions, or grep to find specific content without reading everything."
}

func (t *readTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Path to the file to read",
			Required:    true,
		},
	}
}

func (t *readTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}

	path, err = t.resolve(path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

func (t *readTool) resolve(path string) (string, error) {
	return confine(path, t.workspace, t.extraRoots)
}
