// Package policy provides security policy loading and enforcement.
package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// defaultProtectedFiles are files that agents cannot modify (security-critical).
var defaultProtectedFiles = []string{
	"agent.toml",
	"credentials.toml",
	"policy.toml",
}

// DefaultProtectedFiles returns a copy of the default protected file list.
func DefaultProtectedFiles() []string {
	out := make([]string, len(defaultProtectedFiles))
	copy(out, defaultProtectedFiles)
	return out
}

// Policy represents the security policy for the agent.
type Policy struct {
	DefaultDeny    bool                   `toml:"default_deny"`
	ProtectedFiles []string               `toml:"-"`
	AllowedDirs    []string               `toml:"allowed_dirs"`
	Tools          map[string]*ToolPolicy `toml:"tools"`
	MCP            *MCPPolicy             `toml:"mcp"`
	Content        *Content               `toml:"content"`
}

// ToolPolicy represents the policy for a specific tool.
// A tool listed in [tools] is enabled. Unlisted tools are controlled by DefaultDeny.
// Allow and Deny are generic string patterns — each tool interprets them
// for its resource type (filesystem paths, domains, etc.).
type ToolPolicy struct {
	Allow   []string `toml:"allow"`
	Deny    []string `toml:"deny"`
	Sandbox string   `toml:"sandbox"`
	Timeout int      `toml:"timeout"`
}

// MCPPolicy controls MCP tool access.
type MCPPolicy struct {
	Enabled bool     `toml:"enabled"` // Universal enable/disable switch for MCP tools.
	Allow   []string `toml:"allow"`   // Allow patterns for MCP tools, in the form "server:tool" with optional wildcards.
}

// Content holds content-related policy sections.
type Content struct {
	Security *ContentSecurity `toml:"security"`
}

// ContentSecurity defines patterns and keywords for detecting suspicious content.
type ContentSecurity struct {
	Patterns []string `toml:"patterns"`
	Keywords []string `toml:"keywords"`
}

// New creates a secure default policy (deny by default).
// Call ExpandPatterns after loading to resolve $WORKSPACE and ~ in patterns.
func New() *Policy {
	return &Policy{
		DefaultDeny:    true,
		ProtectedFiles: DefaultProtectedFiles(),
		Tools:          make(map[string]*ToolPolicy),
		Content:        &Content{Security: &ContentSecurity{}},
	}
}

// FromFile creates a policy from a TOML file.
// workspace and homeDir are used to expand $WORKSPACE and ~ in patterns.
func FromFile(path, workspace, homeDir string) (*Policy, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file: %w", err)
	}
	return FromTOML(string(content), workspace, homeDir)
}

// FromTOML creates a policy from TOML content.
// workspace and homeDir are used to expand $WORKSPACE and ~ in patterns.
// Unrecognized keys are silently ignored.
func FromTOML(content, workspace, homeDir string) (*Policy, error) {
	pol := New()
	if _, err := toml.Decode(content, pol); err != nil {
		return nil, fmt.Errorf("failed to parse policy: %w", err)
	}
	pol.expandPatterns(workspace, homeDir)
	return pol, nil
}

// expandPatterns resolves $WORKSPACE and ~ placeholders in AllowedDirs
// and all tool Allow/Deny patterns.
func (p *Policy) expandPatterns(workspace, homeDir string) {
	p.AllowedDirs = expandSlice(p.AllowedDirs, workspace, homeDir)
	for _, tp := range p.Tools {
		tp.Allow = expandSlice(tp.Allow, workspace, homeDir)
		tp.Deny = expandSlice(tp.Deny, workspace, homeDir)
	}
}

func expandSlice(patterns []string, workspace, homeDir string) []string {
	for i, pattern := range patterns {
		if strings.HasPrefix(pattern, "$WORKSPACE") {
			pattern = strings.Replace(pattern, "$WORKSPACE", workspace, 1)
		}
		if strings.HasPrefix(pattern, "~") {
			pattern = strings.Replace(pattern, "~", homeDir, 1)
		}
		patterns[i] = pattern
	}
	return patterns
}

// GetToolPolicy returns the policy for a tool, or nil if not configured.
func (p *Policy) GetToolPolicy(tool string) *ToolPolicy {
	if p == nil || p.Tools == nil {
		return nil
	}
	return p.Tools[tool] // nil if not in map
}

// IsToolEnabled checks if a tool is enabled.
// A tool is enabled if it's listed in [tools], or if DefaultDeny is false.
func (p *Policy) IsToolEnabled(tool string) bool {
	if p == nil {
		return true
	}
	if _, listed := p.Tools[tool]; listed {
		return true
	}
	return !p.DefaultDeny
}

// IsProtectedFile checks if a path refers to a protected config file.
// Resolves symlinks to prevent bypass attacks.
//
// Protected entries that are bare filenames (e.g., "agent.toml") match by basename.
// Entries containing a path separator (e.g., "configs/my-config.toml") are resolved
// to absolute paths relative to cwd and compared as full paths.
func (p *Policy) IsProtectedFile(path string) bool {
	realPath := resolvePath(path)

	for _, protected := range p.ProtectedFiles {
		if strings.Contains(protected, string(filepath.Separator)) || strings.Contains(protected, "/") {
			// Path entry — resolve and compare full paths.
			if resolvePath(protected) == realPath {
				return true
			}
		} else {
			// Bare filename — compare basenames.
			if filepath.Base(realPath) == protected {
				return true
			}
		}
	}
	return false
}

// resolvePath returns the real absolute path, resolving symlinks where possible.
func resolvePath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		dir := filepath.Dir(absPath)
		if realDir, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(realDir, filepath.Base(absPath))
		}
		return absPath
	}
	return realPath
}

// CheckPath checks if a path is allowed for a tool.
func (p *Policy) CheckPath(tool, path string) (bool, string) {
	if !p.IsToolEnabled(tool) {
		return false, fmt.Sprintf("tool %s is disabled", tool)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	if tool == "write" || tool == "edit" {
		if p.IsProtectedFile(absPath) {
			return false, fmt.Sprintf("path %s is a protected config file", path)
		}
	}

	if len(p.AllowedDirs) > 0 {
		if !p.isWithinAllowedDirs(absPath) {
			return false, fmt.Sprintf("path %s is outside allowed directories", path)
		}
	}

	tp := p.GetToolPolicy(tool)
	if tp == nil {
		return true, "" // enabled but no restrictions configured
	}

	for _, pattern := range tp.Deny {
		if matchPath(pattern, absPath) {
			return false, fmt.Sprintf("path %s matches deny pattern %s", path, pattern)
		}
	}

	if len(tp.Allow) > 0 {
		for _, pattern := range tp.Allow {
			if matchPath(pattern, absPath) {
				return true, ""
			}
		}
		return false, fmt.Sprintf("path %s not in allow list", path)
	}

	return true, ""
}

// isWithinAllowedDirs checks if an absolute path falls within any allowed directory.
func (p *Policy) isWithinAllowedDirs(absPath string) bool {
	for _, dir := range p.AllowedDirs {
		dirAbs, err := filepath.Abs(dir)
		if err != nil {
			dirAbs = dir
		}
		if absPath == dirAbs || strings.HasPrefix(absPath, dirAbs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// GetAllowedDirs returns the allowed directories.
func (p *Policy) GetAllowedDirs() []string {
	return p.AllowedDirs
}

// CheckDomain checks if a domain is allowed for web tools.
func (p *Policy) CheckDomain(tool, domain string) (bool, string) {
	if !p.IsToolEnabled(tool) {
		return false, fmt.Sprintf("tool %s is disabled", tool)
	}
	tp := p.GetToolPolicy(tool)

	if tp == nil || len(tp.Allow) == 0 {
		return true, ""
	}

	for _, pattern := range tp.Allow {
		if matchDomain(pattern, domain) {
			return true, ""
		}
	}

	return false, fmt.Sprintf("domain %s not in allow list", domain)
}

// CheckMCPTool checks if an MCP tool is allowed.
// Returns (allowed, reason, warning).
// Warning is non-empty if MCP policy is not configured.
func (p *Policy) CheckMCPTool(server, tool string) (bool, string, string) {
	if p.MCP == nil {
		return true, "", "MCP policy not configured - all MCP tools allowed. Set [mcp] in policy.toml for production."
	}

	if !p.MCP.Enabled {
		toolSpec := fmt.Sprintf("%s:%s", server, tool)
		return false, fmt.Sprintf("MCP is disabled, tool %s blocked", toolSpec), ""
	}

	// Enabled with no allow list — allow everything.
	if len(p.MCP.Allow) == 0 {
		return true, "", ""
	}

	toolSpec := fmt.Sprintf("%s:%s", server, tool)
	for _, pattern := range p.MCP.Allow {
		if matchMCPTool(pattern, server, tool) {
			return true, "", ""
		}
	}

	return false, fmt.Sprintf("MCP tool %s not in allow list", toolSpec), ""
}

// matchMCPTool matches "server:tool" against a pattern.
func matchMCPTool(pattern, server, tool string) bool {
	parts := strings.SplitN(pattern, ":", 2)
	if len(parts) != 2 {
		return pattern == server+":"+tool
	}
	serverPattern, toolPattern := parts[0], parts[1]
	return (serverPattern == "*" || serverPattern == server) &&
		(toolPattern == "*" || toolPattern == tool)
}

// matchPath matches a path against a glob pattern.
func matchPath(pattern, path string) bool {
	pattern = filepath.Clean(pattern)
	path = filepath.Clean(path)

	if strings.Contains(pattern, "**") {
		parts := strings.SplitN(pattern, "**", 2)
		prefix := strings.TrimSuffix(parts[0], string(filepath.Separator))
		suffix := ""
		if len(parts) > 1 {
			suffix = strings.TrimPrefix(parts[1], string(filepath.Separator))
		}

		if prefix != "" && !strings.HasPrefix(path, prefix) {
			return false
		}

		remaining := path
		if prefix != "" {
			remaining = strings.TrimPrefix(path, prefix)
			remaining = strings.TrimPrefix(remaining, string(filepath.Separator))
		}

		if suffix == "" {
			return true
		}
		if strings.HasSuffix(remaining, suffix) {
			return true
		}
		matched, _ := filepath.Match(suffix, filepath.Base(remaining))
		return matched
	}

	matched, _ := filepath.Match(pattern, path)
	return matched
}

// matchDomain matches a domain against a pattern.
func matchDomain(pattern, domain string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(domain, suffix) || domain == strings.TrimPrefix(suffix, ".")
	}
	return pattern == domain
}
