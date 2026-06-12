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
	mu      sync.RWMutex
	spawner SpawnFunc
}

// Spawn creates a tool that spawns sub-agents.
// Accepts an array of agent specs. One spec = single agent. Multiple = parallel execution.
func Spawn(spawner SpawnFunc) Tool {
	return &spawnTool{spawner: spawner}
}

// SpawnBinder is a spawn tool whose SpawnFunc can be supplied after the tool is
// constructed and registered. This resolves the chicken-and-egg case where the
// natural spawner closes over the executor that owns the tool registry: build
// the binder, register binder.Tool(), then Bind the spawner once the executor
// exists. Calling the tool before Bind returns an error.
type SpawnBinder struct {
	tool *spawnTool
}

// NewSpawnBinder returns a binder whose tool is ready to register but not yet
// wired to a spawner.
func NewSpawnBinder() *SpawnBinder {
	return &SpawnBinder{tool: &spawnTool{}}
}

// Tool returns the registrable spawn tool.
func (b *SpawnBinder) Tool() Tool { return b.tool }

// Bind sets (or replaces) the SpawnFunc the tool delegates to. Safe to call
// after the tool has been registered.
func (b *SpawnBinder) Bind(spawner SpawnFunc) {
	b.tool.mu.Lock()
	defer b.tool.mu.Unlock()
	b.tool.spawner = spawner
}

func (t *spawnTool) spawnFunc() SpawnFunc {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.spawner
}

func (t *spawnTool) Name() string { return "spawn_agents" }

func (t *spawnTool) Description() string {
	return `Spawn sub-agents to handle tasks. Pass one or more agent specs.

Parameters:
  - agents (required): Array of agent specs, each with:
    - role (required): Name/role for the sub-agent (e.g., "researcher", "critic")
    - task (required): Task description
    - outputs (optional): List of field names for structured JSON response

Single agent:
  spawn_agents(agents: [{role: "researcher", task: "Find key events"}])

Multiple agents (run in parallel):
  spawn_agents(agents: [
    {role: "researcher", task: "Find historical context"},
    {role: "analyst", task: "Analyze current trends"}
  ])`
}

func (t *spawnTool) Parameters() map[string]Param {
	return map[string]Param{
		"agents": {
			Type:        ArrayParam,
			Description: "Array of agent specs (each has role, task, optional outputs)",
			Required:    true,
		},
	}
}

type agentSpec struct {
	role    string
	task    string
	outputs []string
}

type agentResult struct {
	index  int
	result string
	err    error
}

func (t *spawnTool) Execute(ctx context.Context, args Args) (string, error) {
	spawner := t.spawnFunc()
	if spawner == nil {
		return "", fmt.Errorf("spawn_agents not available (no spawner configured)")
	}

	agentsRaw, ok := args.values["agents"].([]any)
	if !ok {
		return "", fmt.Errorf("agents must be an array")
	}
	if len(agentsRaw) == 0 {
		return "No agents specified.", nil
	}

	// Parse specs
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
			result, err := spawner(ctx, s.role, s.task, s.outputs)
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

	// Single agent — return result directly
	if len(collected) == 1 {
		if collected[0].err != nil {
			return "", collected[0].err
		}
		return collected[0].result, nil
	}

	// Multiple agents — format with headers
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
