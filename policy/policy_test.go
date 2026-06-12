package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicy_LoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "policy.toml")

	content := `
default_deny = true

[tools.read]
allow = ["$WORKSPACE/**"]
deny = ["~/.ssh/*"]
`
	os.WriteFile(policyPath, []byte(content), 0644)

	pol, err := FromFile(policyPath, tmpDir, "/home/user")
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if !pol.DefaultDeny {
		t.Error("expected default_deny = true")
	}

	if !pol.IsToolEnabled("read") {
		t.Error("expected read to be enabled")
	}

	readPolicy := pol.GetToolPolicy("read")
	// Patterns should be expanded after FromFile.
	if readPolicy.Allow[0] != tmpDir+"/**" {
		t.Errorf("expected expanded allow pattern, got %s", readPolicy.Allow[0])
	}
}

func TestPolicy_Defaults(t *testing.T) {
	pol := New()
	// DefaultDeny is true, so unconfigured tools are disabled.
	if pol.IsToolEnabled("read") {
		t.Error("unconfigured tool should be disabled with default_deny")
	}
	// GetToolPolicy returns nil for unconfigured tools.
	if pol.GetToolPolicy("read") != nil {
		t.Error("expected nil for unconfigured tool")
	}
}

func TestPolicy_ToolEnabled(t *testing.T) {
	pol := New() // DefaultDeny = true
	pol.Tools["read"] = &ToolPolicy{}

	// Listed in [tools] → enabled.
	if !pol.IsToolEnabled("read") {
		t.Error("listed tool should be enabled")
	}
	// Not listed + DefaultDeny → disabled.
	if pol.IsToolEnabled("glob") {
		t.Error("unlisted tool should be disabled with default_deny")
	}
	// Not listed + DefaultDeny = false → enabled.
	pol.DefaultDeny = false
	if !pol.IsToolEnabled("glob") {
		t.Error("unlisted tool should be enabled without default_deny")
	}
}

func TestPolicy_DefaultDeny(t *testing.T) {
	pol := New()
	pol.DefaultDeny = true

	allowed, reason := pol.CheckPath("read", "/etc/passwd")
	if allowed {
		t.Error("should deny /etc/passwd with default_deny")
	}
	if reason == "" {
		t.Error("should have denial reason")
	}
}

func TestPolicy_DenyPatterns(t *testing.T) {
	pol := New()
	pol.Tools["read"] = &ToolPolicy{

		Allow: []string{"**"},
		Deny:  []string{"/home/user/.ssh/*"},
	}

	allowed, _ := pol.CheckPath("read", "/home/user/.ssh/id_rsa")
	if allowed {
		t.Error("should deny .ssh paths")
	}
}

func TestPolicy_AllowPatterns(t *testing.T) {
	pol := New()
	pol.Tools["read"] = &ToolPolicy{

		Allow: []string{"/workspace/**"},
	}

	allowed, _ := pol.CheckPath("read", "/workspace/src/main.go")
	if !allowed {
		t.Error("should allow /workspace/** paths")
	}
}

func TestPolicy_PatternExpansion(t *testing.T) {
	content := `
[tools.write]
allow = ["$WORKSPACE/**"]

[tools.read]
allow = ["~/documents/**"]
`
	pol, err := FromTOML(content, "/my/workspace", "/home/testuser")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// $WORKSPACE should be expanded.
	allowed, _ := pol.CheckPath("write", "/my/workspace/file.txt")
	if !allowed {
		t.Error("should expand $WORKSPACE and allow")
	}
	allowed, _ = pol.CheckPath("write", "/other/path/file.txt")
	if allowed {
		t.Error("should deny paths outside $WORKSPACE")
	}

	// ~ should be expanded.
	allowed, _ = pol.CheckPath("read", "/home/testuser/documents/file.txt")
	if !allowed {
		t.Error("should expand ~ and allow")
	}
}

func TestPolicy_GlobPatterns(t *testing.T) {
	pol := New()
	pol.Tools["read"] = &ToolPolicy{

		Allow: []string{"/workspace/*.go"},
	}

	allowed, _ := pol.CheckPath("read", "/workspace/main.go")
	if !allowed {
		t.Error("should match *.go pattern")
	}

	allowed, _ = pol.CheckPath("read", "/workspace/main.txt")
	if allowed {
		t.Error("should not match .txt files")
	}
}

func TestPolicy_RecursiveGlob(t *testing.T) {
	pol := New()
	pol.Tools["read"] = &ToolPolicy{

		Allow: []string{"/workspace/**"},
	}

	allowed, _ := pol.CheckPath("read", "/workspace/a/b/c/d/file.go")
	if !allowed {
		t.Error("** should match any depth")
	}

	pol.Tools["read"].Allow = []string{"/workspace/*"}
	allowed, _ = pol.CheckPath("read", "/workspace/a/b/file.go")
	if allowed {
		t.Error("* should not match across directories")
	}
}

func TestPolicy_WebDomains(t *testing.T) {
	pol := New()
	pol.Tools["web_fetch"] = &ToolPolicy{

		Allow: []string{"github.com", "*.google.com"},
	}

	allowed, _ := pol.CheckDomain("web_fetch", "github.com")
	if !allowed {
		t.Error("should allow github.com")
	}

	allowed, _ = pol.CheckDomain("web_fetch", "docs.google.com")
	if !allowed {
		t.Error("should allow *.google.com")
	}

	allowed, _ = pol.CheckDomain("web_fetch", "evil.com")
	if allowed {
		t.Error("should block evil.com")
	}
}

func TestPolicy_ProtectedFiles(t *testing.T) {
	pol := New()

	tests := []struct {
		path      string
		protected bool
	}{
		{"agent.toml", true},
		{"policy.toml", true},
		{"credentials.toml", true},
		{"/workspace/agent.toml", true},
		{"/workspace/policy.toml", true},
		{"/home/user/.config/grid/credentials.toml", true},
		{"README.md", false},
		{"/workspace/src/main.go", false},
		{"/workspace/config.toml", false},
	}

	for _, tt := range tests {
		result := pol.IsProtectedFile(tt.path)
		if result != tt.protected {
			t.Errorf("IsProtectedFile(%q) = %v, want %v", tt.path, result, tt.protected)
		}
	}
}

func TestPolicy_WriteBlocksProtectedFiles(t *testing.T) {
	pol := New()
	pol.Tools["write"] = &ToolPolicy{

		Allow: []string{"**"},
	}

	allowed, reason := pol.CheckPath("write", "/workspace/agent.toml")
	if allowed {
		t.Error("should block write to agent.toml")
	}
	if reason == "" {
		t.Error("should have denial reason for protected file")
	}

	allowed, _ = pol.CheckPath("write", "/workspace/src/main.go")
	if !allowed {
		t.Error("should allow write to normal files")
	}
}

func TestPolicy_EditBlocksProtectedFiles(t *testing.T) {
	pol := New()
	pol.Tools["edit"] = &ToolPolicy{

		Allow: []string{"**"},
	}

	allowed, _ := pol.CheckPath("edit", "policy.toml")
	if allowed {
		t.Error("should block edit to policy.toml")
	}

	allowed, _ = pol.CheckPath("edit", "credentials.toml")
	if allowed {
		t.Error("should block edit to credentials.toml")
	}
}

func TestPolicy_SymlinkBypass(t *testing.T) {
	tmpDir := t.TempDir()
	pol := New()

	credPath := filepath.Join(tmpDir, "credentials.toml")
	os.WriteFile(credPath, []byte("test"), 0600)

	linkPath := filepath.Join(tmpDir, "innocent.txt")
	os.Symlink(credPath, linkPath)

	if !pol.IsProtectedFile(linkPath) {
		t.Error("symlink to credentials.toml should be protected")
	}
	if !pol.IsProtectedFile(credPath) {
		t.Error("direct credentials.toml should be protected")
	}
}

func TestPolicy_ProtectedFiles_PathEntry(t *testing.T) {
	tmpDir := t.TempDir()
	pol := New()

	// Add a path-based entry (as --config / --policy would provide).
	configPath := filepath.Join(tmpDir, "my-custom-config.toml")
	os.WriteFile(configPath, []byte("test"), 0600)
	pol.ProtectedFiles = append(pol.ProtectedFiles, configPath)

	// The exact file should be protected.
	if !pol.IsProtectedFile(configPath) {
		t.Error("custom config should be protected via path match")
	}

	// A different file with the same basename in another dir should NOT match.
	otherDir := t.TempDir()
	otherPath := filepath.Join(otherDir, "my-custom-config.toml")
	os.WriteFile(otherPath, []byte("other"), 0600)
	if pol.IsProtectedFile(otherPath) {
		t.Error("same basename in different dir should not match path-based entry")
	}

	// Default basename entries should still work.
	if !pol.IsProtectedFile("agent.toml") {
		t.Error("agent.toml should still be protected via basename match")
	}
}

func TestPolicy_MCPToolNotConfigured(t *testing.T) {
	pol := New()

	allowed, _, warning := pol.CheckMCPTool("filesystem", "read_file")
	if !allowed {
		t.Error("should allow MCP tool when policy not configured")
	}
	if warning == "" {
		t.Error("should warn when MCP policy not configured")
	}
}

func TestPolicy_MCPToolEnabled(t *testing.T) {
	pol := New()
	pol.MCP = &MCPPolicy{Enabled: true}

	allowed, _, warning := pol.CheckMCPTool("filesystem", "read_file")
	if !allowed {
		t.Error("should allow when enabled with no allow list")
	}
	if warning != "" {
		t.Error("should not warn when policy is configured")
	}
}

func TestPolicy_MCPToolDisabled(t *testing.T) {
	pol := New()
	pol.MCP = &MCPPolicy{Enabled: false}

	allowed, reason, _ := pol.CheckMCPTool("filesystem", "read_file")
	if allowed {
		t.Error("should deny when MCP is disabled")
	}
	if reason == "" {
		t.Error("should have denial reason")
	}
}

func TestPolicy_MCPToolWithAllowList(t *testing.T) {
	pol := New()
	pol.MCP = &MCPPolicy{
		Enabled: true,
		Allow:   []string{"filesystem:read_file", "memory:*"},
	}

	allowed, _, _ := pol.CheckMCPTool("filesystem", "read_file")
	if !allowed {
		t.Error("should allow filesystem:read_file")
	}

	allowed, _, _ = pol.CheckMCPTool("memory", "store")
	if !allowed {
		t.Error("should allow memory:* pattern")
	}

	allowed, reason, _ := pol.CheckMCPTool("filesystem", "write_file")
	if allowed {
		t.Error("should deny filesystem:write_file")
	}
	if reason == "" {
		t.Error("should have denial reason")
	}
}

func TestPolicy_MCPToolParsing(t *testing.T) {
	content := `
default_deny = true

[mcp]
enabled = true
allow = ["filesystem:read_file", "memory:*"]
`
	pol, err := FromTOML(content, "", "")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if pol.MCP == nil {
		t.Fatal("expected MCP policy to be parsed")
	}
	if !pol.MCP.Enabled {
		t.Error("expected MCP enabled = true")
	}
	if len(pol.MCP.Allow) != 2 {
		t.Errorf("expected 2 allowed tools, got %d", len(pol.MCP.Allow))
	}
}

func TestParse_UnknownKeysIgnored(t *testing.T) {
	content := `
default_deny = true
bogus_key = "wat"
workspace = "/some/path"
`
	pol, err := FromTOML(content, "", "")
	if err != nil {
		t.Fatalf("expected unknown keys to be ignored, got: %v", err)
	}
	if !pol.DefaultDeny {
		t.Error("expected default_deny = true")
	}
}

func TestPolicy_AllowedDirs_FromTOML(t *testing.T) {
	content := `
default_deny = false
allowed_dirs = ["/workspace", "/tmp"]

[tools.read]
`
	pol, err := FromTOML(content, "", "")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(pol.AllowedDirs) != 2 {
		t.Fatalf("expected 2 allowed_dirs, got %d", len(pol.AllowedDirs))
	}
}

func TestPolicy_AllowedDirs_CheckPath(t *testing.T) {
	pol := &Policy{
		AllowedDirs: []string{"/workspace", "/tmp"},
		Tools:       make(map[string]*ToolPolicy),
	}
	pol.Tools["read"] = &ToolPolicy{}
	pol.Tools["write"] = &ToolPolicy{}

	ok, _ := pol.CheckPath("read", "/workspace/file.txt")
	if !ok {
		t.Error("expected /workspace/file.txt to be allowed")
	}

	ok, _ = pol.CheckPath("read", "/tmp/data.json")
	if !ok {
		t.Error("expected /tmp/data.json to be allowed")
	}

	ok, reason := pol.CheckPath("read", "/etc/passwd")
	if ok {
		t.Error("expected /etc/passwd to be denied")
	}
	if !strings.Contains(reason, "outside allowed directories") {
		t.Errorf("expected 'outside allowed directories' reason, got: %s", reason)
	}
}

func TestPolicy_AllowedDirs_WithExpansion(t *testing.T) {
	content := `
allowed_dirs = ["$WORKSPACE", "/tmp"]

[tools.read]
`
	pol, err := FromTOML(content, "/home/user/project", "")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	ok, _ := pol.CheckPath("read", "/home/user/project/src/main.go")
	if !ok {
		t.Error("expected $WORKSPACE path to be allowed after expansion")
	}

	ok, _ = pol.CheckPath("read", "/etc/hosts")
	if ok {
		t.Error("expected /etc/hosts to be denied")
	}
}

func TestPolicy_AllowedDirs_Empty_NoRestriction(t *testing.T) {
	pol := &Policy{
		AllowedDirs: nil,
		Tools:       make(map[string]*ToolPolicy),
	}
	pol.Tools["read"] = &ToolPolicy{}

	ok, _ := pol.CheckPath("read", "/etc/passwd")
	if !ok {
		t.Error("with no allowed_dirs, paths should not be restricted by directory")
	}
}

func TestPolicy_GetAllowedDirs(t *testing.T) {
	pol := &Policy{
		AllowedDirs: []string{"/workspace", "/tmp"},
	}
	dirs := pol.GetAllowedDirs()
	if len(dirs) != 2 || dirs[0] != "/workspace" || dirs[1] != "/tmp" {
		t.Errorf("expected [/workspace, /tmp], got %v", dirs)
	}

	// Empty
	pol2 := &Policy{}
	dirs2 := pol2.GetAllowedDirs()
	if dirs2 != nil {
		t.Errorf("expected nil, got %v", dirs2)
	}
}

func TestParse_AllowedDirs(t *testing.T) {
	content := `
default_deny = false
allowed_dirs = ["/workspace"]
`
	pol, err := FromTOML(content, "", "")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(pol.AllowedDirs) != 1 || pol.AllowedDirs[0] != "/workspace" {
		t.Errorf("expected allowed_dirs [/workspace], got %v", pol.AllowedDirs)
	}
}

// Error paths

func TestFromFile_NonexistentFile(t *testing.T) {
	_, err := FromFile("/nonexistent/policy.toml", "", "")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestFromTOML_InvalidTOML(t *testing.T) {
	_, err := FromTOML("[broken", "", "")
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

// Nil policy safety

func TestGetToolPolicy_NilPolicy(t *testing.T) {
	var p *Policy
	tp := p.GetToolPolicy("read")
	if tp != nil {
		t.Error("expected nil from nil policy")
	}
}

func TestIsToolEnabled_NilPolicy(t *testing.T) {
	var p *Policy
	if !p.IsToolEnabled("read") {
		t.Error("nil policy should allow all tools")
	}
}

// CheckPath edge cases

func TestCheckPath_EnabledByDefaultDenyFalse_NoToolConfig(t *testing.T) {
	pol := New()
	pol.DefaultDeny = false
	// "read" not in Tools map, but DefaultDeny=false → enabled, no restrictions.
	ok, _ := pol.CheckPath("read", "/any/path")
	if !ok {
		t.Error("expected path allowed when tool enabled with no restrictions")
	}
}

func TestCheckPath_DisabledTool(t *testing.T) {
	pol := New() // DefaultDeny = true
	// "read" not in Tools → disabled.
	ok, reason := pol.CheckPath("read", "/any/path")
	if ok {
		t.Error("expected path denied for disabled tool")
	}
	if reason == "" {
		t.Error("expected denial reason")
	}
}

// CheckDomain edge cases

func TestCheckDomain_DisabledTool(t *testing.T) {
	pol := New() // DefaultDeny = true
	ok, reason := pol.CheckDomain("web_fetch", "github.com")
	if ok {
		t.Error("expected domain denied for disabled tool")
	}
	if reason == "" {
		t.Error("expected denial reason")
	}
}

func TestCheckDomain_EnabledNoConfig(t *testing.T) {
	pol := New()
	pol.DefaultDeny = false
	// web_fetch not in Tools, but DefaultDeny=false → enabled, no restrictions.
	ok, _ := pol.CheckDomain("web_fetch", "anything.com")
	if !ok {
		t.Error("expected domain allowed when no restrictions configured")
	}
}

func TestCheckDomain_WildcardAll(t *testing.T) {
	pol := New()
	pol.Tools["web_fetch"] = &ToolPolicy{
		Allow: []string{"*"},
	}
	ok, _ := pol.CheckDomain("web_fetch", "anything.com")
	if !ok {
		t.Error("expected * to allow all domains")
	}
}

// matchMCPTool edge cases

func TestCheckMCPTool_MalformedPattern(t *testing.T) {
	pol := New()
	pol.MCP = &MCPPolicy{
		Enabled: true,
		Allow:   []string{"no-colon-pattern"},
	}
	// "no-colon-pattern" doesn't match "server:tool" format.
	ok, _, _ := pol.CheckMCPTool("server", "tool")
	if ok {
		t.Error("expected malformed pattern to not match")
	}
}

// matchPath edge cases

func TestCheckPath_RecursiveGlobWithSuffix(t *testing.T) {
	pol := New()
	pol.Tools["read"] = &ToolPolicy{
		Allow: []string{"/workspace/**/*.go"},
	}

	ok, _ := pol.CheckPath("read", "/workspace/src/main.go")
	if !ok {
		t.Error("expected **/*.go to match nested .go file")
	}

	ok, _ = pol.CheckPath("read", "/workspace/src/main.txt")
	if ok {
		t.Error("expected **/*.go to not match .txt file")
	}
}

func TestCheckPath_RecursiveGlobSuffixInPath(t *testing.T) {
	pol := New()
	pol.Tools["read"] = &ToolPolicy{
		Allow: []string{"/workspace/**/test"},
	}

	ok, _ := pol.CheckPath("read", "/workspace/a/b/test")
	if !ok {
		t.Error("expected **/test to match nested path ending in test")
	}
}

func TestFromTOMLWithUnknownKeys(t *testing.T) {
	content := `
default_deny = true
rate_limit = 100

[tools.read]
allow = ["$WORKSPACE/**"]

[mcp]
enabled = true
default_deny = false
`
	pol, unknown, err := FromTOMLWithUnknownKeys(content, "/work", "/home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pol.IsToolEnabled("read") {
		t.Error("read should be enabled")
	}
	joined := strings.Join(unknown, ",")
	if !strings.Contains(joined, "rate_limit") || !strings.Contains(joined, "mcp.default_deny") {
		t.Errorf("expected legacy keys reported, got %v", unknown)
	}
}
