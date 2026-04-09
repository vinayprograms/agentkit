package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type diffTool struct{}

// Diff creates a diff tool that compares two files.
func Diff() Tool {
	return &diffTool{}
}

func (t *diffTool) Name() string { return "diff" }

func (t *diffTool) Description() string {
	return "Compare two files and show differences in unified diff format. Useful to verify changes before committing or to understand what changed between versions."
}

func (t *diffTool) Parameters() map[string]Param {
	return map[string]Param{
		"file_a": {
			Type:        StringParam,
			Description: "Path to the first file",
			Required:    true,
		},
		"file_b": {
			Type:        StringParam,
			Description: "Path to the second file",
			Required:    true,
		},
	}
}

func (t *diffTool) Execute(ctx context.Context, args Args) (string, error) {
	fileA, err := args.String("file_a")
	if err != nil {
		return "", err
	}
	fileB, err := args.String("file_b")
	if err != nil {
		return "", err
	}

	contentA, err := os.ReadFile(fileA)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", fileA, err)
	}
	contentB, err := os.ReadFile(fileB)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", fileB, err)
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
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		// Skip matching lines
		if i < len(a) && j < len(b) && a[i] == b[j] {
			i++
			j++
			continue
		}

		// Found a difference - show context
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
