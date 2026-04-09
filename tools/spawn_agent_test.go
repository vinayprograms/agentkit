package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSpawnAgent_Basic(t *testing.T) {
	tool := SpawnAgent(nil)

	if tool.Name() != "spawn_agent" {
		t.Errorf("expected name 'spawn_agent', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}

	params := tool.Parameters()
	if _, ok := params["role"]; !ok {
		t.Error("expected 'role' parameter")
	}
	if _, ok := params["task"]; !ok {
		t.Error("expected 'task' parameter")
	}
}

func TestSpawnAgent_RequiresSpawner(t *testing.T) {
	tool := SpawnAgent(nil)

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role": "researcher",
		"task": "test task",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when spawner not configured")
	}
}

func TestSpawnAgent_RequiresRole(t *testing.T) {
	tool := SpawnAgent(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "result", nil
	})

	_, err := Validate(tool.Parameters(), map[string]any{
		"task": "test task",
	})
	if err == nil {
		t.Error("expected validation error when role missing")
	}
}

func TestSpawnAgent_ExecutesSpawner(t *testing.T) {
	var capturedRole, capturedTask string
	tool := SpawnAgent(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		capturedRole = role
		capturedTask = task
		return "sub-agent output", nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role": "researcher",
		"task": "find information about X",
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRole != "researcher" {
		t.Errorf("expected role 'researcher', got %s", capturedRole)
	}
	if capturedTask != "find information about X" {
		t.Errorf("expected task 'find information about X', got %s", capturedTask)
	}
	if !strings.Contains(result, "sub-agent output") {
		t.Errorf("expected 'sub-agent output' in result, got %s", result)
	}
}

func TestSpawnAgent_WithOutputs(t *testing.T) {
	var capturedOutputs []string
	tool := SpawnAgent(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		capturedOutputs = outputs
		return "result", nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role":    "researcher",
		"task":    "find info",
		"outputs": []any{"findings", "sources"},
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedOutputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(capturedOutputs))
	}
}

func TestSpawnAgent_SpawnerReturnsError(t *testing.T) {
	tool := SpawnAgent(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "", fmt.Errorf("spawner exploded")
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role": "researcher",
		"task": "do stuff",
	})
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error from spawner")
	}
	if !strings.Contains(err.Error(), "spawner exploded") {
		t.Errorf("expected 'spawner exploded' error, got %q", err.Error())
	}
}

func TestSpawnAgent_WithOutputsPassedToSpawner(t *testing.T) {
	var capturedOutputs []string
	tool := SpawnAgent(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		capturedOutputs = outputs
		return `{"events":["e1"],"dates":["d1"]}`, nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role":    "researcher",
		"task":    "find events",
		"outputs": []any{"events", "dates"},
	})
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedOutputs) != 2 || capturedOutputs[0] != "events" || capturedOutputs[1] != "dates" {
		t.Errorf("expected [events, dates], got %v", capturedOutputs)
	}
	if !strings.Contains(result, "events") {
		t.Errorf("expected result to contain 'events', got %q", result)
	}
}

func TestSpawnAgent_NoOutputsIsNil(t *testing.T) {
	var capturedOutputs []string
	called := false
	tool := SpawnAgent(func(ctx context.Context, role, task string, outputs []string) (string, error) {
		called = true
		capturedOutputs = outputs
		return "done", nil
	})

	args, _ := Validate(tool.Parameters(), map[string]any{
		"role": "worker",
		"task": "do work",
	})
	_, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("spawner should have been called")
	}
	if capturedOutputs != nil {
		t.Errorf("expected nil outputs when not provided, got %v", capturedOutputs)
	}
}
