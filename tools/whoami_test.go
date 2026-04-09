package tools

import (
	"context"
	"testing"
)

func TestWhoami(t *testing.T) {
	tool := Whoami()
	args, err := Validate(tool.Parameters(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Error("expected non-empty username")
	}
}
