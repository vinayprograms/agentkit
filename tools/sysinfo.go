package tools

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"
)

type sysinfoTool struct{}

// Sysinfo returns a tool that prints system information.
func Sysinfo() Tool { return &sysinfoTool{} }

func (t *sysinfoTool) Name() string { return "sysinfo" }
func (t *sysinfoTool) Description() string {
	return "Print system information: OS, architecture, CPU count, hostname, working directory."
}
func (t *sysinfoTool) Parameters() map[string]Param {
	return map[string]Param{}
}

func (t *sysinfoTool) Execute(ctx context.Context, args Args) (string, error) {
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
