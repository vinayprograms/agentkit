package tools

import (
	"context"
	"os/user"
)

type whoamiTool struct{}

// Whoami returns a tool that prints the current user name.
func Whoami() Tool { return &whoamiTool{} }

func (t *whoamiTool) Name() string        { return "whoami" }
func (t *whoamiTool) Description() string { return "Print the current user name." }
func (t *whoamiTool) Parameters() map[string]Param {
	return map[string]Param{}
}

func (t *whoamiTool) Execute(ctx context.Context, args Args) (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.Username, nil
}
