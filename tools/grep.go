package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// grepMatch represents a single grep match result.
type grepMatch struct {
	file    string
	line    int
	content string
}

type grepTool struct {
	workspace  string
	extraRoots []string
}

// Grep creates a tool that searches file contents by regex within the given workspace.
func Grep(workspace string, extraRoots ...string) Tool {
	return &grepTool{workspace: workspace, extraRoots: extraRoots}
}

func (t *grepTool) Name() string { return "grep" }

func (t *grepTool) Description() string {
	return "Search for a regex pattern in files. When given a directory, recursively searches all files. Returns file path, line number, and matching line for each hit. Prefer this over bash+grep — it respects workspace boundaries and returns structured results."
}

func (t *grepTool) Parameters() map[string]Param {
	return map[string]Param{
		"pattern": {
			Type:        StringParam,
			Description: "Regex pattern to search for",
			Required:    true,
		},
		"path": {
			Type:        StringParam,
			Description: "File or directory to search",
			Required:    true,
		},
	}
}

func (t *grepTool) Execute(ctx context.Context, args Args) (string, error) {
	pattern, err := args.String("pattern")
	if err != nil {
		return "", err
	}
	path, err := args.String("path")
	if err != nil {
		return "", err
	}

	path, err = t.resolve(path)
	if err != nil {
		return "", err
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}

	var matches []grepMatch

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("path not found: %w", err)
	}

	if info.IsDir() {
		err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors
			}
			if info.IsDir() {
				return nil
			}
			fileMatches, _ := grepFile(re, p)
			matches = append(matches, fileMatches...)
			return nil
		})
		if err != nil {
			return "", err
		}
	} else {
		matches, err = grepFile(re, path)
		if err != nil {
			return "", err
		}
	}

	if len(matches) == 0 {
		return "No matches found.", nil
	}

	var sb strings.Builder
	for i, m := range matches {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "%s:%d: %s", m.file, m.line, m.content)
	}
	return sb.String(), nil
}

func grepFile(re *regexp.Regexp, path string) ([]grepMatch, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var matches []grepMatch
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if re.MatchString(line) {
			matches = append(matches, grepMatch{
				file:    path,
				line:    i + 1,
				content: line,
			})
		}
	}
	return matches, nil
}

func (t *grepTool) resolve(path string) (string, error) {
	return confine(path, t.workspace, t.extraRoots)
}
