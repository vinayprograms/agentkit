package tools

import (
	"context"
	"os"
)

type pwdTool struct{}

// Pwd returns a tool that prints the current working directory.
func Pwd() Tool { return &pwdTool{} }

func (t *pwdTool) Name() string        { return "pwd" }
func (t *pwdTool) Description() string { return "Print the current working directory." }
func (t *pwdTool) Parameters() map[string]Param {
	return map[string]Param{}
}

func (t *pwdTool) Execute(ctx context.Context, args Args) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return dir, nil
}
