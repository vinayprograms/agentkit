package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
)

// ── pwd ──

type pwdTool struct{}

func (t *pwdTool) Name() string        { return "pwd" }
func (t *pwdTool) Description() string { return "Print the current working directory." }
func (t *pwdTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *pwdTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return dir, nil
}

// ── hostname ──

type hostnameTool struct{}

func (t *hostnameTool) Name() string        { return "hostname" }
func (t *hostnameTool) Description() string { return "Print the system hostname." }
func (t *hostnameTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *hostnameTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	name, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return name, nil
}

// ── whoami ──

type whoamiTool struct{}

func (t *whoamiTool) Name() string        { return "whoami" }
func (t *whoamiTool) Description() string { return "Print the current user name." }
func (t *whoamiTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *whoamiTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return u.Username, nil
}

// ── env ──

type envTool struct{}

func (t *envTool) Name() string { return "env" }
func (t *envTool) Description() string {
	return "Read environment variables. Returns a specific variable by name, or lists all non-secret variables."
}
func (t *envTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Environment variable name to read (optional — omit to list all non-secret vars)",
			},
		},
	}
}

func (t *envTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	name, _ := args.String("name")

	if name != "" {
		val, ok := os.LookupEnv(name)
		if !ok {
			return "", nil
		}
		// Block reading secret-looking vars
		if isSensitiveEnvVar(name) {
			return "[redacted]", nil
		}
		return val, nil
	}

	// List all, filtering secrets
	var lines []string
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if isSensitiveEnvVar(parts[0]) {
			continue
		}
		lines = append(lines, e)
	}
	return strings.Join(lines, "\n"), nil
}

// isSensitiveEnvVar returns true for env vars that likely contain secrets.
func isSensitiveEnvVar(name string) bool {
	upper := strings.ToUpper(name)
	sensitive := []string{"KEY", "SECRET", "TOKEN", "PASSWORD", "PASS", "CREDENTIAL", "AUTH", "PRIVATE"}
	for _, s := range sensitive {
		if strings.Contains(upper, s) {
			return true
		}
	}
	return false
}

// ── which ──

type whichTool struct{}

func (t *whichTool) Name() string { return "which" }
func (t *whichTool) Description() string {
	return "Check if a command exists and return its path. Returns empty string if not found."
}
func (t *whichTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Command name to look up",
			},
		},
		"required": []string{"command"},
	}
}

func (t *whichTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	cmd, err := args.String("command")
	if err != nil {
		return nil, err
	}

	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", nil // Not found — not an error
	}
	return path, nil
}

// ── sysinfo ──

type sysinfoTool struct{}

func (t *sysinfoTool) Name() string { return "sysinfo" }
func (t *sysinfoTool) Description() string {
	return "Print system information: OS, architecture, CPU count, hostname, working directory."
}
func (t *sysinfoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *sysinfoTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	hostname, _ := os.Hostname()
	cwd, _ := os.Getwd()
	u, _ := user.Current()

	username := ""
	if u != nil {
		username = u.Username
	}

	return strings.Join([]string{
		"os: " + runtime.GOOS,
		"arch: " + runtime.GOARCH,
		fmt.Sprintf("cpus: %d", runtime.NumCPU()),
		"hostname: " + hostname,
		"user: " + username,
		"cwd: " + cwd,
	}, "\n"), nil
}

// ── date ──

type dateTool struct{}

func (t *dateTool) Name() string { return "now" }
func (t *dateTool) Description() string {
	return "Get the current date and time. Returns ISO 8601 datetime, date, time, day of week, and Unix timestamp. Use this whenever you need today's date or the current time."
}
func (t *dateTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"timezone": map[string]interface{}{
				"type":        "string",
				"description": "IANA timezone (e.g., 'America/New_York', 'UTC'). Defaults to system local time.",
			},
		},
	}
}

func (t *dateTool) Execute(ctx context.Context, rawArgs map[string]interface{}) (interface{}, error) {
	args := Args(rawArgs)
	now := time.Now()

	if tz, err := args.String("timezone"); err == nil && tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone %q: %w", tz, err)
		}
		now = now.In(loc)
	}

	return strings.Join([]string{
		"datetime: " + now.Format(time.RFC3339),
		"date: " + now.Format("2006-01-02"),
		"time: " + now.Format("15:04:05"),
		"day: " + now.Weekday().String(),
		fmt.Sprintf("unix: %d", now.Unix()),
		"timezone: " + now.Location().String(),
	}, "\n"), nil
}
