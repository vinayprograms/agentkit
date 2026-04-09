package tools

import (
	"context"
	"testing"
)

func TestHostname(t *testing.T) {
	tool := Hostname()
	args, err := Validate(tool.Parameters(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Error("expected non-empty hostname")
	}
}
