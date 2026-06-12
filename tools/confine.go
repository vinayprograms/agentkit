package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// confine resolves path against the workspace and verifies it stays within the
// workspace or one of the extra allowed roots.
//
// Relative paths are joined to the workspace. The workspace remains the default
// (and sole, when no extra roots are given) confinement root; extra roots are
// additive directories the consumer explicitly permits. An empty workspace
// disables confinement entirely.
func confine(path, workspace string, extraRoots []string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	path = filepath.Clean(path)

	if workspace == "" {
		return path, nil
	}

	roots := make([]string, 0, len(extraRoots)+1)
	roots = append(roots, workspace)
	roots = append(roots, extraRoots...)

	for _, root := range roots {
		if root == "" {
			continue
		}
		r := filepath.Clean(root)
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			return path, nil
		}
	}

	return "", fmt.Errorf("path %s is outside workspace %s", path, workspace)
}
