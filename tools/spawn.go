package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// SpawnFunc is the function signature for spawning dynamic sub-agents.
// outputs is optional -- when provided, the sub-agent returns structured JSON.
type SpawnFunc func(ctx context.Context, role, task string, outputs []string) (string, error)

// --- SpawnAgent Tool ---

type spawnAgentTool struct {
	spawner SpawnFunc
}

// SpawnAgent creates a tool that spawns a single sub-agent to handle a task.
func SpawnAgent(spawner SpawnFunc) Tool {
	return &spawnAgentTool{spawner: spawner}
}

func (t *spawnAgentTool) Name() string { return "spawn_agent" }

func (t *spawnAgentTool) Description() string {
	return `Spawn a sub-agent to handle a specific task.

Parameters:
  - role (required): Name/role for the sub-agent (e.g., "researcher", "critic")
  - task (required): Task description for the sub-agent
  - outputs (optional): List of field names for structured output. When provided,
    the sub-agent returns a JSON object with these fields. Use when you need
    specific data to process further. Omit for freeform text responses.

Returns:
  - If outputs specified: JSON object with declared fields
  - If outputs omitted: Plain text response

Example:
  spawn_agent(role: "researcher", task: "Find key events", outputs: ["events", "dates"])
  → {"events": [...], "dates": [...]}`
}

func (t *spawnAgentTool) Parameters() map[string]Param {
	return map[string]Param{
		"role": {
			Type:        StringParam,
			Description: "The role/persona for the sub-agent (e.g., 'researcher', 'critic', 'analyst')",
			Required:    true,
		},
		"task": {
			Type:        StringParam,
			Description: "The specific task for the sub-agent to complete",
			Required:    true,
		},
		"outputs": {
			Type:        ArrayParam,
			Description: "Optional list of output field names for structured JSON response",
		},
	}
}

func (t *spawnAgentTool) Execute(ctx context.Context, args Args) (string, error) {
	role, err := args.String("role")
	if err != nil {
		return "", err
	}
	task, err := args.String("task")
	if err != nil {
		return "", err
	}

	outputs := args.StringSliceOr("outputs", nil)

	if t.spawner == nil {
		return "", fmt.Errorf("spawn_agent not available (no spawner configured)")
	}

	return t.spawner(ctx, role, task, outputs)
}

// --- SpawnAgents Tool ---

type spawnAgentsTool struct {
	spawner SpawnFunc
}

// SpawnAgents creates a tool that spawns multiple sub-agents in parallel.
func SpawnAgents(spawner SpawnFunc) Tool {
	return &spawnAgentsTool{spawner: spawner}
}

func (t *spawnAgentsTool) Name() string { return "spawn_agents" }

func (t *spawnAgentsTool) Description() string {
	return `Spawn multiple sub-agents in parallel.

Parameters:
  - agents (required): Array of agent specs, each with:
    - role (required): Name/role for the sub-agent
    - task (required): Task description
    - outputs (optional): List of field names for structured output

Returns: Array of results in same order as input agents.

Example:
  spawn_agents(agents: [
    {role: "researcher", task: "Find historical context"},
    {role: "analyst", task: "Analyze current trends"},
    {role: "critic", task: "Identify weaknesses"}
  ])
  → ["Historical context...", "Current trends...", "Weaknesses..."]`
}

func (t *spawnAgentsTool) Parameters() map[string]Param {
	return map[string]Param{
		"agents": {
			Type:        ArrayParam,
			Description: "Array of agent specifications to run in parallel",
			Required:    true,
		},
	}
}

// agentResult holds the result of a parallel agent spawn.
type agentResult struct {
	index  int
	result string
	err    error
}

func (t *spawnAgentsTool) Execute(ctx context.Context, args Args) (string, error) {
	if t.spawner == nil {
		return "", fmt.Errorf("spawn_agents not available (no spawner configured)")
	}

	// Get raw agents array — args validated as ArrayParam so we reach into the values.
	agentsRaw, ok := args.values["agents"].([]any)
	if !ok {
		return "", fmt.Errorf("agents array is required")
	}

	if len(agentsRaw) == 0 {
		return "No agents specified.", nil
	}

	// Parse agent specs
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
		agent, err := Validate(map[string]Param{
			"role": {Type: StringParam, Required: true},
			"task": {Type: StringParam, Required: true},
			"outputs": {Type: ArrayParam},
		}, agentMap)
		if err != nil {
			return "", fmt.Errorf("agent[%d]: %w", i, err)
		}
		role, _ := agent.String("role")
		task, _ := agent.String("task")
		outputs := agent.StringSliceOr("outputs", nil)
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

	// Build text output
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
