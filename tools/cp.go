package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type cpTool struct {
	workspace  string
	extraRoots []string
}

// Cp creates a tool that copies files/directories within the given workspace.
func Cp(workspace string, extraRoots ...string) Tool {
	return &cpTool{workspace: workspace, extraRoots: extraRoots}
}

func (t *cpTool) Name() string { return "cp" }

func (t *cpTool) Description() string {
	return "Copy a file. For directories, copies recursively."
}

func (t *cpTool) Parameters() map[string]Param {
	return map[string]Param{
		"source": {
			Type:        StringParam,
			Description: "Source file or directory path",
			Required:    true,
		},
		"destination": {
			Type:        StringParam,
			Description: "Destination path",
			Required:    true,
		},
	}
}

func (t *cpTool) Execute(ctx context.Context, args Args) (string, error) {
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

	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("source not found: %w", err)
	}

	if info.IsDir() {
		if err := copyDir(src, dst); err != nil {
			return "", err
		}
	} else {
		if err := copyFile(src, dst); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("Copied %s → %s", src, dst), nil
}

func (t *cpTool) resolve(path string) (string, error) {
	return confine(path, t.workspace, t.extraRoots)
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
