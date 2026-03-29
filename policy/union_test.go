package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var _ Lookup = (*Policy)(nil)
var _ Lookup = (*Union)(nil)

func TestUnion_Empty(t *testing.T) {
	u := NewUnion()

	// Default is deny — unconfigured tools should be disabled.
	if u.IsToolEnabled("read") {
		t.Error("expected tool disabled by default (default_deny)")
	}
	// GetToolPolicy returns nil for unconfigured tools.
	if u.GetToolPolicy("read") != nil {
		t.Error("expected nil for unconfigured tool")
	}
	dirs := u.GetAllowedDirs()
	if dirs != nil {
		t.Errorf("expected nil allowed dirs, got %v", dirs)
	}
}

func TestUnion_SingleStore(t *testing.T) {
	pol := &Policy{
		DefaultDeny:    true,
		ProtectedFiles: DefaultProtectedFiles(),
		AllowedDirs:    []string{"/workspace", "/tmp"},
		Tools: map[string]*ToolPolicy{
			"bash":  {Deny: []string{"rm"}},
			"write": {Allow: []string{"/workspace/**"}},
		},
	}

	u := NewUnion(pol)

	if !u.IsToolEnabled("bash") {
		t.Error("bash should be enabled")
	}
	if u.IsToolEnabled("glob") {
		t.Error("glob should be disabled with default_deny")
	}

	dirs := u.GetAllowedDirs()
	if len(dirs) != 2 {
		t.Fatalf("expected 2 allowed dirs, got %d", len(dirs))
	}

	tp := u.GetToolPolicy("bash")
	if len(tp.Deny) != 1 || tp.Deny[0] != "rm" {
		t.Errorf("expected bash deny [rm], got %v", tp.Deny)
	}
}

func TestUnion_DefaultDeny_LastWins(t *testing.T) {
	pol1 := &Policy{DefaultDeny: true, Tools: make(map[string]*ToolPolicy)}
	pol2 := &Policy{DefaultDeny: false, Tools: make(map[string]*ToolPolicy)}

	u := NewUnion(pol1, pol2)
	if !u.IsToolEnabled("read") {
		t.Error("expected default_deny=false from last store")
	}

	u2 := NewUnion(pol2, pol1)
	if u2.IsToolEnabled("read") {
		t.Error("expected default_deny=true from last store")
	}
}

func TestUnion_AllowedDirs_Union(t *testing.T) {
	pol1 := &Policy{AllowedDirs: []string{"/workspace", "/tmp"}, Tools: make(map[string]*ToolPolicy)}
	pol2 := &Policy{AllowedDirs: []string{"/tmp", "/data"}, Tools: make(map[string]*ToolPolicy)}

	u := NewUnion(pol1, pol2)
	dirs := u.GetAllowedDirs()

	if len(dirs) != 3 {
		t.Fatalf("expected 3 allowed dirs, got %d: %v", len(dirs), dirs)
	}
	dirSet := make(map[string]bool)
	for _, d := range dirs {
		dirSet[d] = true
	}
	for _, expected := range []string{"/workspace", "/tmp", "/data"} {
		if !dirSet[expected] {
			t.Errorf("expected %s in allowed dirs", expected)
		}
	}
}

func TestUnion_Tools_PerToolOverlay(t *testing.T) {
	pol1 := &Policy{
		Tools: map[string]*ToolPolicy{
			"bash": {Deny: []string{"rm", "curl"}},
			"read": {Allow: []string{"/global/**"}},
		},
	}
	pol2 := &Policy{
		Tools: map[string]*ToolPolicy{
			"bash": {Deny: []string{"wget"}},
		},
	}

	u := NewUnion(pol1, pol2)

	bashTP := u.GetToolPolicy("bash")
	if len(bashTP.Deny) != 1 || bashTP.Deny[0] != "wget" {
		t.Errorf("expected bash deny [wget] from higher-priority store, got %v", bashTP.Deny)
	}

	readTP := u.GetToolPolicy("read")
	if len(readTP.Allow) != 1 || readTP.Allow[0] != "/global/**" {
		t.Errorf("expected read allow [/global/**] from lower-priority store, got %v", readTP.Allow)
	}
}

func TestUnion_MCP_Enabled_LastWins(t *testing.T) {
	pol1 := &Policy{
		Tools: make(map[string]*ToolPolicy),
		MCP:   &MCPPolicy{Enabled: true, Allow: []string{"filesystem:read_file"}},
	}
	pol2 := &Policy{
		Tools: make(map[string]*ToolPolicy),
		MCP:   &MCPPolicy{Enabled: true, Allow: []string{"memory:*"}},
	}

	u := NewUnion(pol1, pol2)
	// Enabled=true from last store, allow everything since both are enabled.
	allowed, _, _ := u.CheckMCPTool("unknown", "anything")
	// Both have allow lists → merged. "unknown:anything" not in merged list.
	if allowed {
		t.Error("expected unknown:anything to be denied with allow list")
	}
}

func TestUnion_MCP_Allow_Union(t *testing.T) {
	pol1 := &Policy{
		Tools: make(map[string]*ToolPolicy),
		MCP:   &MCPPolicy{Enabled: true, Allow: []string{"filesystem:read_file", "memory:*"}},
	}
	pol2 := &Policy{
		Tools: make(map[string]*ToolPolicy),
		MCP:   &MCPPolicy{Enabled: true, Allow: []string{"memory:*", "git:status"}},
	}

	u := NewUnion(pol1, pol2)

	for _, tc := range []struct{ server, tool string }{
		{"filesystem", "read_file"},
		{"memory", "store"},
		{"git", "status"},
	} {
		allowed, reason, _ := u.CheckMCPTool(tc.server, tc.tool)
		if !allowed {
			t.Errorf("expected %s:%s to be allowed, got denied: %s", tc.server, tc.tool, reason)
		}
	}

	allowed, _, _ := u.CheckMCPTool("filesystem", "write_file")
	if allowed {
		t.Error("expected filesystem:write_file to be denied")
	}
}

func TestUnion_MCP_OneNil(t *testing.T) {
	pol1 := &Policy{Tools: make(map[string]*ToolPolicy)}
	pol2 := &Policy{
		Tools: make(map[string]*ToolPolicy),
		MCP:   &MCPPolicy{Enabled: true, Allow: []string{"filesystem:read_file"}},
	}

	u := NewUnion(pol1, pol2)

	allowed, _, _ := u.CheckMCPTool("filesystem", "read_file")
	if !allowed {
		t.Error("expected filesystem:read_file to be allowed")
	}
	allowed, _, _ = u.CheckMCPTool("filesystem", "write_file")
	if allowed {
		t.Error("expected filesystem:write_file to be denied")
	}
}

func TestUnion_ContentSecurity_Union(t *testing.T) {
	pol1 := &Policy{
		Tools: make(map[string]*ToolPolicy),
		Content: &Content{Security: &ContentSecurity{
			Patterns: []string{"exfil:send.*external"},
			Keywords: []string{"secret", "api_key"},
		}},
	}
	pol2 := &Policy{
		Tools: make(map[string]*ToolPolicy),
		Content: &Content{Security: &ContentSecurity{
			Patterns: []string{"exfil:send.*external", "inject:drop.*table"},
			Keywords: []string{"password"},
		}},
	}

	u := NewUnion(pol1, pol2)
	merged := u.Merged()

	if len(merged.Content.Security.Patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d: %v", len(merged.Content.Security.Patterns), merged.Content.Security.Patterns)
	}
	if len(merged.Content.Security.Keywords) != 3 {
		t.Fatalf("expected 3 keywords, got %d: %v", len(merged.Content.Security.Keywords), merged.Content.Security.Keywords)
	}
}

func TestUnion_ContentSecurity_OneNil(t *testing.T) {
	pol1 := &Policy{Tools: make(map[string]*ToolPolicy)}
	pol2 := &Policy{
		Tools:   make(map[string]*ToolPolicy),
		Content: &Content{Security: &ContentSecurity{Keywords: []string{"secret"}}},
	}

	u := NewUnion(pol1, pol2)
	merged := u.Merged()

	if merged.Content == nil || merged.Content.Security == nil {
		t.Fatal("expected non-nil content.security")
	}
	if len(merged.Content.Security.Keywords) != 1 {
		t.Errorf("expected 1 keyword, got %d", len(merged.Content.Security.Keywords))
	}
}

func TestUnion_Refresh(t *testing.T) {
	pol1 := &Policy{DefaultDeny: false, Tools: make(map[string]*ToolPolicy)}

	u := NewUnion(pol1)

	if !u.IsToolEnabled("read") {
		t.Error("expected read enabled before refresh")
	}

	pol1.DefaultDeny = true

	if !u.IsToolEnabled("read") {
		t.Error("expected read enabled from cache")
	}

	u.Refresh()

	if u.IsToolEnabled("read") {
		t.Error("expected read disabled after refresh")
	}
}

func TestUnion_CheckPath_MergedPolicy(t *testing.T) {
	pol1 := &Policy{
		ProtectedFiles: DefaultProtectedFiles(),
		Tools: map[string]*ToolPolicy{
			"read": {
				
				Allow:   []string{"/workspace/**"},
				Deny:    []string{"/workspace/.ssh/*"},
			},
		},
	}

	u := NewUnion(pol1)

	ok, _ := u.CheckPath("read", "/workspace/src/main.go")
	if !ok {
		t.Error("expected /workspace/src/main.go to be allowed")
	}

	ok, _ = u.CheckPath("read", "/workspace/.ssh/id_rsa")
	if ok {
		t.Error("expected .ssh/id_rsa to be denied")
	}
}

func TestUnion_CheckDomain_MergedPolicy(t *testing.T) {
	pol := &Policy{
		Tools: map[string]*ToolPolicy{
			"web_fetch": {
				
				Allow:   []string{"github.com", "*.google.com"},
			},
		},
	}

	u := NewUnion(pol)

	ok, _ := u.CheckDomain("web_fetch", "github.com")
	if !ok {
		t.Error("expected github.com to be allowed")
	}
	ok, _ = u.CheckDomain("web_fetch", "evil.com")
	if ok {
		t.Error("expected evil.com to be denied")
	}
}

func TestUnion_IsProtectedFile(t *testing.T) {
	u := NewUnion(&Policy{
		ProtectedFiles: DefaultProtectedFiles(),
		Tools:          make(map[string]*ToolPolicy),
	})

	if !u.IsProtectedFile("policy.toml") {
		t.Error("expected policy.toml to be protected")
	}
	if u.IsProtectedFile("README.md") {
		t.Error("expected README.md to not be protected")
	}
}

func TestUnion_ToolPolicy_PerToolOverlay_Allow(t *testing.T) {
	pol1 := &Policy{
		Tools: map[string]*ToolPolicy{
			"read": {Allow: []string{"/global/**", "/tmp/**"}},
		},
	}
	pol2 := &Policy{
		Tools: map[string]*ToolPolicy{
			"read": {Allow: []string{"/project/**"}},
		},
	}

	u := NewUnion(pol1, pol2)
	tp := u.GetToolPolicy("read")

	if len(tp.Allow) != 1 {
		t.Fatalf("expected 1 allow entry, got %d: %v", len(tp.Allow), tp.Allow)
	}
	if tp.Allow[0] != "/project/**" {
		t.Errorf("expected pol2 allow [/project/**], got %v", tp.Allow)
	}
}

func TestUnion_FromFiles(t *testing.T) {
	tmpDir := t.TempDir()

	globalPath := filepath.Join(tmpDir, "global-policy.toml")
	globalContent := `
default_deny = true

[tools.read]
allow = ["/global/**"]
`
	os.WriteFile(globalPath, []byte(globalContent), 0644)

	projectPath := filepath.Join(tmpDir, "project-policy.toml")
	projectContent := `
[tools.bash]

[tools.write]
allow = ["/project/**"]
`
	os.WriteFile(projectPath, []byte(projectContent), 0644)

	global, err := FromFile(globalPath, "", "")
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	project, err := FromFile(projectPath, "", "")
	if err != nil {
		t.Fatalf("load project: %v", err)
	}

	u := NewUnion(global, project)

	if !u.IsToolEnabled("bash") {
		t.Error("expected bash enabled from project policy")
	}

	if !u.IsToolEnabled("read") {
		t.Error("expected read enabled from global policy")
	}

	if !u.IsToolEnabled("write") {
		t.Error("expected write enabled from project policy")
	}

	// glob is not mentioned in either file. With default_deny=true (default),
	// unconfigured tools are disabled.
	if u.IsToolEnabled("glob") {
		t.Error("expected glob disabled since default_deny is true")
	}
}

func TestUnion_FromFiles_DefaultDenyOverride(t *testing.T) {
	tmpDir := t.TempDir()

	globalPath := filepath.Join(tmpDir, "global-policy.toml")
	os.WriteFile(globalPath, []byte(`default_deny = false`), 0644)

	projectPath := filepath.Join(tmpDir, "project-policy.toml")
	os.WriteFile(projectPath, []byte(`default_deny = true`), 0644)

	global, _ := FromFile(globalPath, "", "")
	project, _ := FromFile(projectPath, "", "")

	u := NewUnion(global, project)

	if u.IsToolEnabled("read") {
		t.Error("expected read disabled with default_deny=true from project")
	}
}

func TestUnion_NestedUnion(t *testing.T) {
	inner := NewUnion(&Policy{
		DefaultDeny: true,
		AllowedDirs: []string{"/inner"},
		Tools: map[string]*ToolPolicy{
			"bash": {Deny: []string{"rm"}},
		},
	})

	outer := NewUnion(inner, &Policy{
		AllowedDirs: []string{"/outer"},
		Tools:       make(map[string]*ToolPolicy),
	})

	dirs := outer.GetAllowedDirs()
	dirSet := make(map[string]bool)
	for _, d := range dirs {
		dirSet[d] = true
	}
	if !dirSet["/inner"] || !dirSet["/outer"] {
		t.Errorf("expected both /inner and /outer in dirs, got %v", dirs)
	}

	if !outer.IsToolEnabled("bash") {
		t.Error("expected bash enabled from inner union")
	}
}

func TestUnion_CheckPath_ProtectedFile_Write(t *testing.T) {
	tmpDir := t.TempDir()

	pol := &Policy{
		ProtectedFiles: DefaultProtectedFiles(),
		Tools: map[string]*ToolPolicy{
			"write": {Allow: []string{"**"}},
		},
	}

	u := NewUnion(pol)

	ok, reason := u.CheckPath("write", filepath.Join(tmpDir, "policy.toml"))
	if ok {
		t.Error("expected write to policy.toml to be blocked")
	}
	if !strings.Contains(reason, "protected") {
		t.Errorf("expected 'protected' in reason, got: %s", reason)
	}
}
