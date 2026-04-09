package tools

import (
	"context"
	"os"
	"testing"
)

func TestPwd(t *testing.T) {
	tool := Pwd()
	args, err := Validate(tool.Parameters(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}

	// Should match os.Getwd
	wd, _ := os.Getwd()
	if result != wd {
		t.Errorf("got %q, want %q", result, wd)
	}
}
