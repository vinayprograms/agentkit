package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type patchTool struct{}

// Patch creates a patch tool that applies unified diffs to files.
func Patch() Tool {
	return &patchTool{}
}

func (t *patchTool) Name() string { return "patch" }

func (t *patchTool) Description() string {
	return "Apply a unified diff patch to a file. The patch should use standard unified diff format with --- and +++ headers."
}

func (t *patchTool) Parameters() map[string]Param {
	return map[string]Param{
		"path": {
			Type:        StringParam,
			Description: "Path to the file to patch",
			Required:    true,
		},
		"patch": {
			Type:        StringParam,
			Description: "Unified diff content to apply",
			Required:    true,
		},
	}
}

func (t *patchTool) Execute(ctx context.Context, args Args) (string, error) {
	path, err := args.String("path")
	if err != nil {
		return "", err
	}
	patchContent, err := args.String("patch")
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	result, err := applyPatch(lines, patchContent)
	if err != nil {
		return "", fmt.Errorf("patch failed: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, []byte(strings.Join(result, "\n")), info.Mode()); err != nil {
		return "", fmt.Errorf("failed to write patched file: %w", err)
	}

	return fmt.Sprintf("Patched %s successfully", path), nil
}

// applyPatch applies a unified diff to lines.
func applyPatch(original []string, patch string) ([]string, error) {
	result := make([]string, len(original))
	copy(result, original)

	patchLines := strings.Split(patch, "\n")
	offset := 0 // Track line offset from insertions/deletions

	for i := 0; i < len(patchLines); i++ {
		line := patchLines[i]

		// Skip headers
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			continue
		}

		// Parse hunk header
		if strings.HasPrefix(line, "@@") {
			var startA, countA int
			fmt.Sscanf(line, "@@ -%d,%d", &startA, &countA)
			startA-- // Convert to 0-indexed

			// Collect hunk changes
			var newLines []string
			pos := startA + offset
			removeCount := 0

			for i++; i < len(patchLines); i++ {
				hl := patchLines[i]
				if strings.HasPrefix(hl, "@@") || hl == "" && i == len(patchLines)-1 {
					i-- // Reprocess this line
					break
				}

				if strings.HasPrefix(hl, " ") {
					newLines = append(newLines, hl[1:])
				} else if strings.HasPrefix(hl, "-") {
					removeCount++
				} else if strings.HasPrefix(hl, "+") {
					newLines = append(newLines, hl[1:])
				}
			}

			// Apply: remove old lines, insert new
			if pos < 0 {
				pos = 0
			}

			// Simple replacement: remove from pos to pos+countA, insert newLines
			if pos+countA <= len(result) {
				before := make([]string, pos)
				copy(before, result[:pos])
				after := result[pos+countA:]
				result = append(append(before, newLines...), after...)
				offset += len(newLines) - countA
			}
		}
	}

	return result, nil
}
