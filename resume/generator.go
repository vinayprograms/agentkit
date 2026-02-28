package resume

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// LLM is a minimal interface for generating resume content.
// Typically backed by a small/cheap model (e.g., Haiku).
type LLM interface {
	// Complete sends a prompt and returns the response text.
	Complete(ctx context.Context, prompt string) (string, error)
}

// AgentfileInfo contains parsed Agentfile data needed for resume generation.
type AgentfileInfo struct {
	Name    string
	Goals   []GoalInfo
	Inputs  []InputInfo
	Outputs []string
}

// GoalInfo describes a goal from the Agentfile.
type GoalInfo struct {
	Name        string
	Description string
	Outputs     []string
}

// InputInfo describes an input from the Agentfile.
type InputInfo struct {
	Name     string
	Default  *string
}

// GenerateFromAgentfile analyzes an Agentfile and produces a resume.
// Uses the small LLM to infer description and capabilities.
func GenerateFromAgentfile(ctx context.Context, agentID string, af AgentfileInfo, tools []string, llm LLM) (*Resume, error) {
	// Build prompt for capability inference
	prompt := buildInferencePrompt(af, tools)

	resp, err := llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm inference failed: %w", err)
	}

	// Parse LLM response
	var inferred inferredResume
	if err := json.Unmarshal([]byte(resp), &inferred); err != nil {
		return nil, fmt.Errorf("failed to parse llm response: %w (response: %s)", err, resp)
	}

	// Build inputs
	inputs := make([]FieldInfo, len(af.Inputs))
	for i, inp := range af.Inputs {
		inputs[i] = FieldInfo{
			Name:     inp.Name,
			Required: inp.Default == nil,
		}
		if inp.Default != nil {
			inputs[i].Default = *inp.Default
		}
	}

	// Build outputs
	var outputs []FieldInfo
	for _, name := range af.Outputs {
		outputs = append(outputs, FieldInfo{Name: name})
	}

	return &Resume{
		AgentID:      agentID,
		Name:         af.Name,
		Description:  inferred.Description,
		Capabilities: inferred.Capabilities,
		Tools:        tools,
		Inputs:       inputs,
		Outputs:      outputs,
		CreatedAt:    time.Now(),
	}, nil
}

// inferredResume is the expected JSON structure from the LLM.
type inferredResume struct {
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
}

// buildInferencePrompt creates the prompt for capability inference.
func buildInferencePrompt(af AgentfileInfo, tools []string) string {
	prompt := `Analyze this agent definition and respond with JSON only (no markdown, no explanation).

Agent name: ` + af.Name + `

Goals:
`
	for _, g := range af.Goals {
		prompt += fmt.Sprintf("- %s: %s\n", g.Name, g.Description)
	}

	if len(tools) > 0 {
		prompt += "\nAvailable tools:\n"
		for _, t := range tools {
			prompt += "- " + t + "\n"
		}
	}

	prompt += `
Respond with this exact JSON structure:
{
  "description": "One sentence describing what this agent does",
  "capabilities": ["dot.separated.tags"]
}

Rules for capabilities:
- Use dot-separated hierarchy: code.golang, test.unit, design.system, ops.deploy
- First level is the domain: code, test, design, ops, data, security, docs
- Second level is the specialization
- Include 2-5 capabilities
- Be specific based on the goals and tools`

	return prompt
}
