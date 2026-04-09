package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestSpawnAgents_MultipleAgents(t *testing.T) {
	spawner := func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return fmt.Sprintf("result from %s", role), nil
	}

	tool := SpawnAgents(spawner)
	args, err := Validate(tool.Parameters(), map[string]any{
		"agents": []any{
			map[string]any{"role": "researcher", "task": "find historical context"},
			map[string]any{"role": "analyst", "task": "analyze trends"},
			map[string]any{"role": "critic", "task": "identify weaknesses"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "result from researcher") {
		t.Errorf("expected researcher result, got %q", result)
	}
	if !strings.Contains(result, "result from analyst") {
		t.Errorf("expected analyst result, got %q", result)
	}
	if !strings.Contains(result, "result from critic") {
		t.Errorf("expected critic result, got %q", result)
	}
}

func TestSpawnAgents_MissingSpawner(t *testing.T) {
	tool := SpawnAgents(nil)
	args, err := Validate(tool.Parameters(), map[string]any{
		"agents": []any{
			map[string]any{"role": "researcher", "task": "do work"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when spawner is nil")
	}
	if !strings.Contains(err.Error(), "no spawner configured") {
		t.Errorf("expected 'no spawner configured' error, got %q", err.Error())
	}
}

func TestSpawnAgents_EmptyAgentsList(t *testing.T) {
	spawner := func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "result", nil
	}

	tool := SpawnAgents(spawner)
	args, err := Validate(tool.Parameters(), map[string]any{
		"agents": []any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "No agents specified." {
		t.Errorf("expected 'No agents specified.', got %q", result)
	}
}

func TestSpawnAgents_AgentError(t *testing.T) {
	spawner := func(ctx context.Context, role, task string, outputs []string) (string, error) {
		if role == "failing" {
			return "", fmt.Errorf("agent failed")
		}
		return "ok from " + role, nil
	}

	tool := SpawnAgents(spawner)
	args, err := Validate(tool.Parameters(), map[string]any{
		"agents": []any{
			map[string]any{"role": "working", "task": "do work"},
			map[string]any{"role": "failing", "task": "will fail"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	// Should contain success for working agent and error for failing agent
	if !strings.Contains(result, "ok from working") {
		t.Errorf("expected success from working agent, got %q", result)
	}
	if !strings.Contains(result, "Error: agent failed") {
		t.Errorf("expected error from failing agent, got %q", result)
	}
}

func TestSpawnAgents_WithOutputs(t *testing.T) {
	var capturedOutputs []string
	spawner := func(ctx context.Context, role, task string, outputs []string) (string, error) {
		capturedOutputs = outputs
		return `{"events": ["event1"], "dates": ["2024"]}`, nil
	}

	tool := SpawnAgents(spawner)
	args, err := Validate(tool.Parameters(), map[string]any{
		"agents": []any{
			map[string]any{
				"role":    "researcher",
				"task":    "find events",
				"outputs": []any{"events", "dates"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if len(capturedOutputs) != 2 || capturedOutputs[0] != "events" || capturedOutputs[1] != "dates" {
		t.Errorf("expected outputs [events, dates], got %v", capturedOutputs)
	}
}
