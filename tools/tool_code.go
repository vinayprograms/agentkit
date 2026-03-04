package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vinayprograms/agentkit/policy"
)

// ── diff ──

type diffTool struct {
	policy *policy.Policy
}

func (t *diffTool) Name() string { return "diff" }

func (t *diffTool) Description() string {
	return "Compare two files and show differences in unified diff format."
}

func (t *diffTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"file_a": map[string]interface{}{
				"type":        "string",
				"description": "Path to the first file",
			},
			"file_b": map[string]interface{}{
				"type":        "string",
				"description": "Path to the second file",
			},
		},
		"required": []string{"file_a", "file_b"},
	}
}

func (t *diffTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	fileA, err := args.String("file_a")
	if err != nil {
		return nil, err
	}
	fileB, err := args.String("file_b")
	if err != nil {
		return nil, err
	}

	if allowed, reason := t.policy.CheckPath(t.Name(), fileA); !allowed {
		return nil, fmt.Errorf("policy denied (file_a): %s", reason)
	}
	if allowed, reason := t.policy.CheckPath(t.Name(), fileB); !allowed {
		return nil, fmt.Errorf("policy denied (file_b): %s", reason)
	}

	contentA, err := os.ReadFile(fileA)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", fileA, err)
	}
	contentB, err := os.ReadFile(fileB)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", fileB, err)
	}

	linesA := strings.Split(string(contentA), "\n")
	linesB := strings.Split(string(contentB), "\n")

	diff := unifiedDiff(fileA, fileB, linesA, linesB)
	if diff == "" {
		return "Files are identical", nil
	}

	return diff, nil
}

// unifiedDiff produces a simple unified diff output.
func unifiedDiff(nameA, nameB string, a, b []string) string {
	var result strings.Builder

	// Simple line-by-line comparison with context
	result.WriteString(fmt.Sprintf("--- %s\n", nameA))
	result.WriteString(fmt.Sprintf("+++ %s\n", nameB))

	// Find differing regions
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	i, j := 0, 0
	for i < len(a) || j < len(b) {
		// Skip matching lines
		if i < len(a) && j < len(b) && a[i] == b[j] {
			i++
			j++
			continue
		}

		// Found a difference — show context
		ctxStart := i - 3
		if ctxStart < 0 {
			ctxStart = 0
		}

		// Find end of differing region
		diffEndA, diffEndB := i, j
		for diffEndA < len(a) || diffEndB < len(b) {
			if diffEndA < len(a) && diffEndB < len(b) && a[diffEndA] == b[diffEndB] {
				// Check if we have enough matching context to end the hunk
				match := 0
				for diffEndA+match < len(a) && diffEndB+match < len(b) && a[diffEndA+match] == b[diffEndB+match] {
					match++
					if match >= 3 {
						break
					}
				}
				if match >= 3 {
					break
				}
			}
			if diffEndA < len(a) {
				diffEndA++
			}
			if diffEndB < len(b) {
				diffEndB++
			}
		}

		ctxEnd := diffEndA + 3
		if ctxEnd > len(a) {
			ctxEnd = len(a)
		}

		result.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", ctxStart+1, ctxEnd-ctxStart, ctxStart+1, diffEndB+(ctxEnd-diffEndA)-ctxStart))

		// Context before
		for k := ctxStart; k < i; k++ {
			result.WriteString(" " + a[k] + "\n")
		}

		// Removed lines
		for k := i; k < diffEndA; k++ {
			result.WriteString("-" + a[k] + "\n")
		}

		// Added lines
		for k := j; k < diffEndB; k++ {
			result.WriteString("+" + b[k] + "\n")
		}

		// Context after
		for k := diffEndA; k < ctxEnd; k++ {
			result.WriteString(" " + a[k] + "\n")
		}

		i = ctxEnd
		j = diffEndB + (ctxEnd - diffEndA)
	}

	return result.String()
}

// ── tree ──

type treeTool struct {
	policy *policy.Policy
}

func (t *treeTool) Name() string { return "tree" }

func (t *treeTool) Description() string {
	return "Show directory structure as a tree. Default depth is 3."
}

func (t *treeTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Root directory path",
			},
			"depth": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum depth to traverse (default 3)",
			},
		},
		"required": []string{"path"},
	}
}

func (t *treeTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	path, err := args.String("path")
	if err != nil {
		return nil, err
	}

	allowed, reason := t.policy.CheckPath(t.Name(), path)
	if !allowed {
		return nil, fmt.Errorf("policy denied: %s", reason)
	}

	maxDepth := 3
	if v, err := args.Int("depth"); err == nil {
		maxDepth = v
	}

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

// ── patch ──

type patchTool struct {
	policy *policy.Policy
}

func (t *patchTool) Name() string { return "patch" }

func (t *patchTool) Description() string {
	return "Apply a unified diff patch to a file. The patch should use standard unified diff format with --- and +++ headers."
}

func (t *patchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to the file to patch",
			},
			"patch": map[string]interface{}{
				"type":        "string",
				"description": "Unified diff content to apply",
			},
		},
		"required": []string{"path", "patch"},
	}
}

func (t *patchTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	path, err := args.String("path")
	if err != nil {
		return nil, err
	}
	patchContent, err := args.String("patch")
	if err != nil {
		return nil, err
	}

	allowed, reason := t.policy.CheckPath(t.Name(), path)
	if !allowed {
		return nil, fmt.Errorf("policy denied: %s", reason)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	result, err := applyPatch(lines, patchContent)
	if err != nil {
		return nil, fmt.Errorf("patch failed: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(path, []byte(strings.Join(result, "\n")), info.Mode()); err != nil {
		return nil, fmt.Errorf("failed to write patched file: %w", err)
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
			end := pos + removeCount + (countA - removeCount) // context lines
			if end > len(result) {
				end = len(result)
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
