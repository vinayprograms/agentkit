package tools

import (
	"context"
	"testing"
)

func TestWhichFindsKnownCommand(t *testing.T) {
	tool := Which()
	args, err := Validate(tool.Parameters(), map[string]any{"command": "ls"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Error("expected path for 'ls', got empty")
	}
}

func TestWhichUnknownCommand(t *testing.T) {
	tool := Which()
	args, err := Validate(tool.Parameters(), map[string]any{"command": "nonexistent_command_xyz_12345"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "" {
		t.Errorf("expected empty for unknown command, got %q", result)
	}
}
