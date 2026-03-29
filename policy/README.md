# policy

Security policy loading and enforcement for agent tools, MCP access, and filesystem boundaries.

## Usage

```go
// Load from a TOML file. Workspace and homeDir expand $WORKSPACE and ~ in patterns.
pol, err := policy.FromFile("policy.toml", workspace, homeDir)
if err != nil { ... }

// Check if a tool is enabled (listed in [tools] or DefaultDeny is false).
if pol.IsToolEnabled("bash") { ... }

// Check filesystem path access.
ok, reason := pol.CheckPath("read", "/workspace/file.go")

// Check domain access for web tools.
ok, reason := pol.CheckDomain("web_fetch", "github.com")

// Check MCP tool access.
ok, reason, warning := pol.CheckMCPTool("filesystem", "read_file")

// Check if a file is protected from modification.
if pol.IsProtectedFile("agent.toml") { ... }

// Get allowed directories.
dirs := pol.GetAllowedDirs()
```

## Configuration (policy.toml)

```toml
default_deny = true
allowed_dirs = ["$WORKSPACE", "/tmp"]

[tools.read]
allow = ["$WORKSPACE/**"]
deny = ["$WORKSPACE/.secrets/**"]

[tools.write]
allow = ["$WORKSPACE/**"]

[tools.bash]
sandbox = "none"
timeout = 300

[tools.web_fetch]
allow = ["github.com", "*.google.com"]

[mcp]
enabled = true
allow = ["filesystem:read_file", "memory:*"]

[content.security]
patterns = ["exfil_curl:(?i)curl\\s+.*(-d|--data)"]
keywords = ["confidential"]
```

### Top-level fields

| Field | Type | Description |
|---|---|---|
| `default_deny` | bool | If true, tools not listed in `[tools]` are disabled. Default: true. |
| `allowed_dirs` | []string | Universal filesystem boundary. Paths outside these are denied. Supports `$WORKSPACE` and `~`. |

### Tool sections (`[tools.<name>]`)

A tool listed under `[tools]` is enabled. Unlisted tools are controlled by `default_deny`.

| Field | Type | Description |
|---|---|---|
| `allow` | []string | Allow patterns. Interpreted by the tool (path globs, domain patterns, etc.). |
| `deny` | []string | Deny patterns. Deny wins over allow. |
| `sandbox` | string | Bash only: `"none"`, `"bwrap"`, or `"docker"`. |
| `timeout` | int | Max seconds per invocation. |

### MCP section (`[mcp]`)

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable/disable all MCP tools. |
| `allow` | []string | Patterns in `"server:tool"` format. Supports `*` wildcards. |

### Content security (`[content.security]`)

| Field | Type | Description |
|---|---|---|
| `patterns` | []string | Additional regex patterns in `"name:regex"` format. |
| `keywords` | []string | Additional sensitive keywords (case-insensitive). |

## Merging Policies

Use `Union` to compose multiple policies with priority ordering (last wins):

```go
global, _ := policy.FromFile("global-policy.toml", ws, home)
project, _ := policy.FromFile("project-policy.toml", ws, home)

u := policy.NewUnion(global, project)
// project overrides global for DefaultDeny and per-tool config.
// AllowedDirs, ProtectedFiles, MCP Allow, and content security patterns/keywords
// are merged (deduplicated union).

ok, reason := u.CheckPath("read", path)
```

Call `u.Refresh()` to invalidate the cached merge after underlying policies change.

## Protected Files

Default protected files (`agent.toml`, `credentials.toml`, `policy.toml`) cannot be modified via write/edit tools. Append runtime paths for custom config files:

```go
pol.ProtectedFiles = append(pol.ProtectedFiles, configPath, policyPath)
```

Path-based entries (containing `/`) match by full resolved path. Bare filenames match by basename. Symlinks are resolved to prevent bypass.

## Lookup Interface

Both `*Policy` and `*Union` satisfy the `Lookup` interface:

```go
type Lookup interface {
    GetToolPolicy(tool string) *ToolPolicy
    IsToolEnabled(tool string) bool
    CheckPath(tool, path string) (bool, string)
    CheckDomain(tool, domain string) (bool, string)
    CheckMCPTool(server, tool string) (bool, string, string)
    IsProtectedFile(path string) bool
    GetAllowedDirs() []string
}
```
