package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSpawn_Single(t *testing.T) {
	var capturedRole, capturedTask string
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		capturedRole = role
		capturedTask = task
		return "sub-agent output", nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role": "researcher",
		"task": "find information",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRole != "researcher" {
		t.Errorf("expected role 'researcher', got %s", capturedRole)
	}
	if capturedTask != "find information" {
		t.Errorf("expected task, got %s", capturedTask)
	}
	if !strings.Contains(result, "sub-agent output") {
		t.Errorf("expected output in result, got %s", result)
	}
}

func TestSpawn_SingleWithOutputs(t *testing.T) {
	var capturedOutputs []string
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		capturedOutputs = outputs
		return `{"events":["e1"]}`, nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role":    "researcher",
		"task":    "find events",
		"outputs": []any{"events", "dates"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedOutputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(capturedOutputs))
	}
}

func TestSpawn_SingleMissingRole(t *testing.T) {
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "", nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"task": "do stuff",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error for missing role")
	}
}

func TestSpawn_NoSpawner(t *testing.T) {
	tool := Spawn(nil)
	args, _ := Validate(tool.Parameters(), map[string]any{
		"role": "test",
		"task": "test",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when spawner not configured")
	}
}

func TestSpawn_SpawnerError(t *testing.T) {
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "", fmt.Errorf("spawner exploded")
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role": "test",
		"task": "test",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil || !strings.Contains(err.Error(), "spawner exploded") {
		t.Errorf("expected spawner error, got %v", err)
	}
}

func TestSpawn_Multiple(t *testing.T) {
	var mu = &struct{ count int }{}
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		mu.count++
		return fmt.Sprintf("result from %s", role), nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"agents": []any{
			map[string]any{"role": "researcher", "task": "find stuff"},
			map[string]any{"role": "analyst", "task": "analyze stuff"},
		},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "researcher") {
		t.Error("expected researcher in result")
	}
	if !strings.Contains(result, "analyst") {
		t.Error("expected analyst in result")
	}
}

func TestSpawn_MultipleEmpty(t *testing.T) {
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "", nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"agents": []any{},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "No agents specified." {
		t.Errorf("expected 'No agents specified.', got %q", result)
	}
}

func TestSpawn_MultipleWithError(t *testing.T) {
	tool := Spawn(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		if role == "failing" {
			return "", fmt.Errorf("agent failed")
		}
		return "ok", nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"agents": []any{
			map[string]any{"role": "working", "task": "work"},
			map[string]any{"role": "failing", "task": "fail"},
		},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Error: agent failed") {
		t.Errorf("expected error in result, got %q", result)
	}
	if !strings.Contains(result, "ok") {
		t.Errorf("expected success in result, got %q", result)
	}
}

func TestSpawn_NameAndDescription(t *testing.T) {
	tool := Spawn(nil)
	if tool.Name() != "spawn_agent" {
		t.Errorf("expected 'spawn_agent', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}
}
