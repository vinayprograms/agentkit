package policy

import "maps"

// Union merges multiple policies into one, with later policies taking priority
// on conflicts (last wins). The merge happens once, at construction.
//
// Union implements Lookup, so a merged policy is used for enforcement exactly
// like a single *Policy.
type Union struct {
	merged *Policy
}

// NewUnion merges policies with priority ordering: the last policy passed wins
// on conflicts. Scalar fields (DefaultDeny, MCP.Enabled) are last-wins;
// AllowedDirs, ProtectedFiles, MCP allow-lists, and content patterns/keywords
// are deduplicated unions; Tools are overlaid per name.
func NewUnion(policies ...*Policy) *Union {
	return &Union{merged: build(policies)}
}

func (u *Union) GetToolPolicy(tool string) *ToolPolicy { return u.merged.GetToolPolicy(tool) }

func (u *Union) IsToolEnabled(tool string) bool { return u.merged.IsToolEnabled(tool) }

func (u *Union) CheckPath(tool, path string) (bool, string) { return u.merged.CheckPath(tool, path) }

func (u *Union) CheckDomain(tool, domain string) (bool, string) {
	return u.merged.CheckDomain(tool, domain)
}

func (u *Union) CheckMCPTool(server, tool string) (bool, string, string) {
	return u.merged.CheckMCPTool(server, tool)
}

func (u *Union) IsProtectedFile(path string) bool { return u.merged.IsProtectedFile(path) }

func (u *Union) GetAllowedDirs() []string { return u.merged.GetAllowedDirs() }

// build folds policies into one, last-wins on conflicts.
func build(policies []*Policy) *Policy {
	result := New()

	for _, pol := range policies {
		if pol == nil {
			continue
		}

		// DefaultDeny: last-wins.
		result.DefaultDeny = pol.DefaultDeny

		// AllowedDirs / ProtectedFiles: deduplicated union.
		result.AllowedDirs = dedupeUnion(result.AllowedDirs, pol.AllowedDirs)
		result.ProtectedFiles = dedupeUnion(result.ProtectedFiles, pol.ProtectedFiles)

		// Tools: per-tool overlay, last-wins per tool name.
		maps.Copy(result.Tools, pol.Tools)

		// MCP: Enabled last-wins, Allow deduplicated union.
		if pol.MCP != nil {
			if result.MCP == nil {
				result.MCP = &MCPPolicy{}
			}
			result.MCP.Enabled = pol.MCP.Enabled
			result.MCP.Allow = dedupeUnion(result.MCP.Allow, pol.MCP.Allow)
		}

		// Content.Security: Patterns and Keywords deduplicated union.
		if pol.Content != nil && pol.Content.Security != nil {
			if result.Content == nil {
				result.Content = &Content{}
			}
			if result.Content.Security == nil {
				result.Content.Security = &ContentSecurity{}
			}
			result.Content.Security.Patterns = dedupeUnion(result.Content.Security.Patterns, pol.Content.Security.Patterns)
			result.Content.Security.Keywords = dedupeUnion(result.Content.Security.Keywords, pol.Content.Security.Keywords)
		}
	}

	return result
}

// dedupeUnion merges two string slices, removing duplicates.
func dedupeUnion(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(a)+len(b))
	var result []string

	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}
