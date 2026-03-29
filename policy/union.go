package policy

// Union merges multiple Lookup stores with priority ordering.
// The last store passed to NewUnion has the highest priority.
// The merged policy is built lazily on first access and cached.
// Call Refresh to invalidate the cache after underlying stores change.
type Union struct {
	stores []Lookup
	merged *Policy
}

// NewUnion creates a Union from multiple Lookup stores.
// Priority: last store wins (same convention as credentials.UnionStore).
func NewUnion(stores ...Lookup) *Union {
	return &Union{stores: stores}
}

// Refresh invalidates the cached merged policy, forcing a rebuild on the next call.
func (u *Union) Refresh() {
	u.merged = nil
}

// Merged returns the lazily-built merged policy. Exposed for consumers
// that need direct access to the merged *Policy (e.g., BashChecker).
func (u *Union) Merged() *Policy {
	if u.merged == nil {
		u.merged = u.build()
	}
	return u.merged
}

func (u *Union) GetToolPolicy(tool string) *ToolPolicy {
	return u.Merged().GetToolPolicy(tool)
}

func (u *Union) IsToolEnabled(tool string) bool {
	return u.Merged().IsToolEnabled(tool)
}

func (u *Union) CheckPath(tool, path string) (bool, string) {
	return u.Merged().CheckPath(tool, path)
}

func (u *Union) CheckDomain(tool, domain string) (bool, string) {
	return u.Merged().CheckDomain(tool, domain)
}

func (u *Union) CheckMCPTool(server, tool string) (bool, string, string) {
	return u.Merged().CheckMCPTool(server, tool)
}

func (u *Union) IsProtectedFile(path string) bool {
	return u.Merged().IsProtectedFile(path)
}

func (u *Union) GetAllowedDirs() []string {
	return u.Merged().GetAllowedDirs()
}

// build creates a merged *Policy from all stores.
func (u *Union) build() *Policy {
	if len(u.stores) == 0 {
		return New()
	}

	result := New()

	for _, store := range u.stores {
		pol, ok := store.(*Policy)
		if !ok {
			// If the store is a *Union, get its merged policy.
			if union, ok := store.(*Union); ok {
				pol = union.Merged()
			} else {
				continue
			}
		}

		// DefaultDeny: last-wins.
		result.DefaultDeny = pol.DefaultDeny

		// AllowedDirs: deduplicated union.
		result.AllowedDirs = dedupeUnion(result.AllowedDirs, pol.AllowedDirs)

		// ProtectedFiles: deduplicated union.
		result.ProtectedFiles = dedupeUnion(result.ProtectedFiles, pol.ProtectedFiles)

		// Tools: per-tool overlay, last-wins per tool name.
		for name, tp := range pol.Tools {
			result.Tools[name] = tp
		}

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
