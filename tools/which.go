package tools

import (
	"context"
	"os/exec"
)

type whichTool struct{}

// Which returns a tool that checks if a command exists and returns its path.
func Which() Tool { return &whichTool{} }

func (t *whichTool) Name() string { return "which" }
func (t *whichTool) Description() string {
	return "Check if a command exists and return its path. Returns empty string if not found."
}
func (t *whichTool) Parameters() map[string]Param {
	return map[string]Param{
		"command": {
			Type:        StringParam,
			Description: "Command name to look up",
			Required:    true,
		},
	}
}

func (t *whichTool) Execute(ctx context.Context, args Args) (string, error) {
	cmd, err := args.String("command")
	if err != nil {
		return "", err
	}

	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", nil // Not found is not an error
	}
	return path, nil
}
