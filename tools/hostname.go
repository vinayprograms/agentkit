package tools

import (
	"context"
	"os"
)

type hostnameTool struct{}

// Hostname returns a tool that prints the system hostname.
func Hostname() Tool { return &hostnameTool{} }

func (t *hostnameTool) Name() string        { return "hostname" }
func (t *hostnameTool) Description() string { return "Print the system hostname." }
func (t *hostnameTool) Parameters() map[string]Param {
	return map[string]Param{}
}

func (t *hostnameTool) Execute(ctx context.Context, args Args) (string, error) {
	name, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return name, nil
}
