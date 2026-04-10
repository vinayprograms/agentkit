package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// SpawnFunc is the callback that the consumer provides to handle sub-agent creation.
//
// The tools package does not know how to create agents — that's the consumer's
// responsibility. The consumer implements SpawnFunc to wire in their agent runtime:
// creating the sub-agent, setting up its context, running it, and collecting results.
//
// Parameters:
//   - ctx: context for cancellation and timeouts
//   - role: the persona/role for the sub-agent (e.g., "researcher", "critic")
//   - task: the specific task description for the sub-agent
//   - outputs: optional field names for structured JSON output; nil means freeform text
//
// Returns the sub-agent's response as a string, or an error if spawning/execution failed.
type SpawnFunc func(ctx context.Context, role, task string, outputs []string) (string, error)

type spawnTool struct {
	spawner SpawnFunc
}

// Spawn creates a tool that spawns sub-agents.
// Handles both single agent (role + task) and multiple agents (agents array).
// When multiple agents are specified, they run in parallel.
func Spawn(spawner SpawnFunc) Tool {
	return &spawnTool{spawner: spawner}
}

func (t *spawnTool) Name() string { return "spawn_agent" }

func (t *spawnTool) Description() string {
	return `Spawn one or more sub-agents to handle tasks.

For a single agent:
  spawn_agent(role: "researcher", task: "Find key events", outputs: ["events", "dates"])

For multiple agents in parallel:
  spawn_agent(agents: [
    {role: "researcher", task: "Find historical context"},
    {role: "analyst", task: "Analyze current trends"}
  ])

Parameters:
  Single agent mode:
  - role (required): Name/role for the sub-agent
  - task (required): Task description
  - outputs (optional): List of field names for structured JSON response

  Multi-agent mode:
  - agents (required): Array of agent specs, each with role, task, and optional outputs

Returns text response (single) or numbered results (multiple).`
}

func (t *spawnTool) Parameters() map[string]Param {
	return map[string]Param{
		"role": {
			Type:        StringParam,
			Description: "Role for a single sub-agent (e.g., 'researcher', 'critic')",
		},
		"task": {
			Type:        StringParam,
			Description: "Task for a single sub-agent",
		},
		"outputs": {
			Type:        ArrayParam,
			Description: "Optional output field names for structured JSON response",
		},
		"agents": {
			Type:        ArrayParam,
			Description: "Array of agent specs for parallel execution (each has role, task, optional outputs)",
		},
	}
}

func (t *spawnTool) Execute(ctx context.Context, args Args) (string, error) {
	if t.spawner == nil {
		return "", fmt.Errorf("spawn_agent not available (no spawner configured)")
	}

	// Multi-agent mode: agents array takes precedence
	if args.Has("agents") {
		return t.spawnMultiple(ctx, args)
	}

	// Single-agent mode
	return t.spawnSingle(ctx, args)
}

func (t *spawnTool) spawnSingle(ctx context.Context, args Args) (string, error) {
	role, err := args.String("role")
	if err != nil {
		return "", fmt.Errorf("single agent requires 'role': %w", err)
	}
	task, err := args.String("task")
	if err != nil {
		return "", fmt.Errorf("single agent requires 'task': %w", err)
	}
	outputs := args.StringSliceOr("outputs", nil)

	return t.spawner(ctx, role, task, outputs)
}

type agentResult struct {
	index  int
	result string
	err    error
}

func (t *spawnTool) spawnMultiple(ctx context.Context, args Args) (string, error) {
	agentsRaw, ok := args.values["agents"].([]any)
	if !ok {
		return "", fmt.Errorf("agents must be an array")
	}
	if len(agentsRaw) == 0 {
		return "No agents specified.", nil
	}

	type agentSpec struct {
		role    string
		task    string
		outputs []string
	}

	specs := make([]agentSpec, 0, len(agentsRaw))
	for i, a := range agentsRaw {
		agentMap, ok := a.(map[string]any)
		if !ok {
			return "", fmt.Errorf("agent[%d]: invalid format", i)
		}
		specArgs, err := Validate(map[string]Param{
			"role":    {Type: StringParam, Required: true},
			"task":    {Type: StringParam, Required: true},
			"outputs": {Type: ArrayParam},
		}, agentMap)
		if err != nil {
			return "", fmt.Errorf("agent[%d]: %w", i, err)
		}
		role, _ := specArgs.String("role")
		task, _ := specArgs.String("task")
		outputs := specArgs.StringSliceOr("outputs", nil)
		specs = append(specs, agentSpec{role: role, task: task, outputs: outputs})
	}

	// Run all agents in parallel
	results := make(chan agentResult, len(specs))
	var wg sync.WaitGroup

	for i, spec := range specs {
		wg.Add(1)
		go func(idx int, s agentSpec) {
			defer wg.Done()
			result, err := t.spawner(ctx, s.role, s.task, s.outputs)
			results <- agentResult{index: idx, result: result, err: err}
		}(i, spec)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results in order
	collected := make([]agentResult, len(specs))
	for r := range results {
		collected[r.index] = r
	}

	var sb strings.Builder
	for i, r := range collected {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		fmt.Fprintf(&sb, "Agent %d (%s):\n", i+1, specs[i].role)
		if r.err != nil {
			fmt.Fprintf(&sb, "Error: %v", r.err)
		} else {
			sb.WriteString(r.result)
		}
	}
	return sb.String(), nil
}
