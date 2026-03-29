package policy

// Lookup is the interface for policy enforcement.
// Both *Policy and *Union satisfy this interface.
type Lookup interface {
	GetToolPolicy(tool string) *ToolPolicy
	IsToolEnabled(tool string) bool
	CheckPath(tool, path string) (bool, string)
	CheckDomain(tool, domain string) (bool, string)
	CheckMCPTool(server, tool string) (bool, string, string)
	IsProtectedFile(path string) bool
	GetAllowedDirs() []string
}
