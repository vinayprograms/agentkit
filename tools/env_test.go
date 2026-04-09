package tools

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestEnvListAll(t *testing.T) {
	tool := Env()
	args, err := Validate(tool.Parameters(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result == "" {
		t.Error("expected non-empty env list")
	}

	// Should contain at least PATH (or similar common var)
	if !strings.Contains(result, "=") {
		t.Error("expected KEY=VALUE format in output")
	}
}

func TestEnvSpecificVar(t *testing.T) {
	tool := Env()

	os.Setenv("AGENTKIT_TEST_VAR", "hello123")
	defer os.Unsetenv("AGENTKIT_TEST_VAR")

	args, err := Validate(tool.Parameters(), map[string]any{"name": "AGENTKIT_TEST_VAR"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "hello123" {
		t.Errorf("got %q, want %q", result, "hello123")
	}
}

func TestEnvMissingVar(t *testing.T) {
	tool := Env()
	args, err := Validate(tool.Parameters(), map[string]any{"name": "AGENTKIT_NONEXISTENT_VAR_XYZ"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "" {
		t.Errorf("expected empty for missing var, got %q", result)
	}
}

func TestEnvSensitiveRedacted(t *testing.T) {
	tool := Env()

	os.Setenv("MY_SECRET_KEY", "supersecret")
	defer os.Unsetenv("MY_SECRET_KEY")

	args, err := Validate(tool.Parameters(), map[string]any{"name": "MY_SECRET_KEY"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "[redacted]" {
		t.Errorf("expected [redacted], got %q", result)
	}
}

func TestEnvSensitiveFilteredFromList(t *testing.T) {
	tool := Env()

	os.Setenv("MY_API_TOKEN", "shouldnotappear")
	defer os.Unsetenv("MY_API_TOKEN")

	args, err := Validate(tool.Parameters(), nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "MY_API_TOKEN") {
		t.Error("sensitive var MY_API_TOKEN should be filtered from list")
	}
}
