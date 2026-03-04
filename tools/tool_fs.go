package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vinayprograms/agentkit/policy"
)

// ── mkdir ──

type mkdirTool struct {
	policy *policy.Policy
}

func (t *mkdirTool) Name() string { return "mkdir" }

func (t *mkdirTool) Description() string {
	return "Create a directory (and parent directories if needed)."
}

func (t *mkdirTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Directory path to create",
			},
		},
		"required": []string{"path"},
	}
}

func (t *mkdirTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	path, err := args.String("path")
	if err != nil {
		return nil, err
	}

	allowed, reason := t.policy.CheckPath(t.Name(), path)
	if !allowed {
		return nil, fmt.Errorf("policy denied: %s", reason)
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	return fmt.Sprintf("Created directory: %s", path), nil
}

// ── mv ──

type mvTool struct {
	policy *policy.Policy
}

func (t *mvTool) Name() string { return "mv" }

func (t *mvTool) Description() string {
	return "Move or rename a file or directory."
}

func (t *mvTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Source path",
			},
			"destination": map[string]interface{}{
				"type":        "string",
				"description": "Destination path",
			},
		},
		"required": []string{"source", "destination"},
	}
}

func (t *mvTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	src, err := args.String("source")
	if err != nil {
		return nil, err
	}
	dst, err := args.String("destination")
	if err != nil {
		return nil, err
	}

	// Check both source and destination
	if allowed, reason := t.policy.CheckPath(t.Name(), src); !allowed {
		return nil, fmt.Errorf("policy denied (source): %s", reason)
	}
	if allowed, reason := t.policy.CheckPath(t.Name(), dst); !allowed {
		return nil, fmt.Errorf("policy denied (destination): %s", reason)
	}

	if err := os.Rename(src, dst); err != nil {
		return nil, fmt.Errorf("failed to move: %w", err)
	}

	return fmt.Sprintf("Moved %s → %s", src, dst), nil
}

// ── cp ──

type cpTool struct {
	policy *policy.Policy
}

func (t *cpTool) Name() string { return "cp" }

func (t *cpTool) Description() string {
	return "Copy a file. For directories, copies recursively."
}

func (t *cpTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"source": map[string]interface{}{
				"type":        "string",
				"description": "Source file or directory path",
			},
			"destination": map[string]interface{}{
				"type":        "string",
				"description": "Destination path",
			},
		},
		"required": []string{"source", "destination"},
	}
}

func (t *cpTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	src, err := args.String("source")
	if err != nil {
		return nil, err
	}
	dst, err := args.String("destination")
	if err != nil {
		return nil, err
	}

	if allowed, reason := t.policy.CheckPath(t.Name(), src); !allowed {
		return nil, fmt.Errorf("policy denied (source): %s", reason)
	}
	if allowed, reason := t.policy.CheckPath(t.Name(), dst); !allowed {
		return nil, fmt.Errorf("policy denied (destination): %s", reason)
	}

	info, err := os.Stat(src)
	if err != nil {
		return nil, fmt.Errorf("source not found: %w", err)
	}

	if info.IsDir() {
		if err := copyDir(src, dst); err != nil {
			return nil, err
		}
	} else {
		if err := copyFile(src, dst); err != nil {
			return nil, err
		}
	}

	return fmt.Sprintf("Copied %s → %s", src, dst), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		return copyFile(path, target)
	})
}

// ── rm ──

type rmTool struct {
	policy *policy.Policy
}

func (t *rmTool) Name() string { return "rm" }

func (t *rmTool) Description() string {
	return "Delete a file or empty directory. Use recursive=true for non-empty directories."
}

func (t *rmTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to delete",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "Delete directories and contents recursively",
			},
		},
		"required": []string{"path"},
	}
}

func (t *rmTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	path, err := args.String("path")
	if err != nil {
		return nil, err
	}

	allowed, reason := t.policy.CheckPath(t.Name(), path)
	if !allowed {
		return nil, fmt.Errorf("policy denied: %s", reason)
	}

	recursive, _ := args.Bool("recursive")

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("path not found: %w", err)
	}

	if info.IsDir() && !recursive {
		// Only remove empty directories without recursive flag
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("directory not empty (use recursive=true): %w", err)
		}
	} else if recursive {
		if err := os.RemoveAll(path); err != nil {
			return nil, fmt.Errorf("failed to delete: %w", err)
		}
	} else {
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("failed to delete: %w", err)
		}
	}

	return fmt.Sprintf("Deleted: %s", path), nil
}

// ── head ──

type headTool struct {
	policy *policy.Policy
}

func (t *headTool) Name() string { return "head" }

func (t *headTool) Description() string {
	return "Read the first N lines of a file (default 10)."
}

func (t *headTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file",
			},
			"lines": map[string]interface{}{
				"type":        "integer",
				"description": "Number of lines to read (default 10)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *headTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	path, err := args.String("path")
	if err != nil {
		return nil, err
	}

	allowed, reason := t.policy.CheckPath(t.Name(), path)
	if !allowed {
		return nil, fmt.Errorf("policy denied: %s", reason)
	}

	n := 10
	if v, err := args.Int("lines"); err == nil {
		n = v
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.SplitN(string(content), "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}

	return strings.Join(lines, "\n"), nil
}

// ── tail ──

type tailTool struct {
	policy *policy.Policy
}

func (t *tailTool) Name() string { return "tail" }

func (t *tailTool) Description() string {
	return "Read the last N lines of a file (default 10)."
}

func (t *tailTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file",
			},
			"lines": map[string]interface{}{
				"type":        "integer",
				"description": "Number of lines to read (default 10)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *tailTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	path, err := args.String("path")
	if err != nil {
		return nil, err
	}

	allowed, reason := t.policy.CheckPath(t.Name(), path)
	if !allowed {
		return nil, fmt.Errorf("policy denied: %s", reason)
	}

	n := 10
	if v, err := args.Int("lines"); err == nil {
		n = v
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	allLines := strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	start := len(allLines) - n
	if start < 0 {
		start = 0
	}

	return strings.Join(allLines[start:], "\n"), nil
}
