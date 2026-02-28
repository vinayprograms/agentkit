package resume

import (
	"encoding/json"
	"errors"
	"math"
	"time"
)

// Common errors.
var (
	ErrEmptyAgentID = errors.New("agent_id is required")
	ErrEmptyName    = errors.New("name is required")
)

// Resume describes an agent's capabilities, derived from its Agentfile.
type Resume struct {
	// AgentID uniquely identifies the agent instance.
	AgentID string `json:"agent_id"`

	// Name is the workflow name from the Agentfile.
	Name string `json:"name"`

	// Description is a natural language summary of what the agent does.
	Description string `json:"description"`

	// Capabilities are dot-separated hierarchical tags (e.g., "code.golang").
	Capabilities []string `json:"capabilities"`

	// Tools lists the tools available to this agent.
	Tools []string `json:"tools,omitempty"`

	// Inputs describes the agent's expected input parameters.
	Inputs []FieldInfo `json:"inputs,omitempty"`

	// Outputs describes what the agent produces.
	Outputs []FieldInfo `json:"outputs,omitempty"`

	// Embedding is the vector representation of this resume for semantic matching.
	Embedding []float64 `json:"embedding,omitempty"`

	// CreatedAt is when the resume was generated.
	CreatedAt time.Time `json:"created_at"`
}

// FieldInfo describes an input or output field.
type FieldInfo struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

// Validate checks if the resume has required fields.
func (r *Resume) Validate() error {
	if r.AgentID == "" {
		return ErrEmptyAgentID
	}
	if r.Name == "" {
		return ErrEmptyName
	}
	return nil
}

// Marshal serializes the resume to JSON.
func (r *Resume) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

// Unmarshal deserializes a resume from JSON.
func Unmarshal(data []byte) (*Resume, error) {
	var r Resume
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// HasCapability checks if the resume includes a capability.
// Supports exact match and wildcard prefix (e.g., "code.*" matches "code.golang").
func (r *Resume) HasCapability(query string) bool {
	// Check for wildcard
	if len(query) > 2 && query[len(query)-2:] == ".*" {
		prefix := query[:len(query)-1] // "code.*" -> "code."
		for _, cap := range r.Capabilities {
			if len(cap) >= len(prefix) && cap[:len(prefix)] == prefix {
				return true
			}
		}
		return false
	}

	// Exact match
	for _, cap := range r.Capabilities {
		if cap == query {
			return true
		}
	}
	return false
}

// ToText returns a natural language representation of the resume
// suitable for embedding.
func (r *Resume) ToText() string {
	text := r.Name + ": " + r.Description
	if len(r.Capabilities) > 0 {
		text += " Capabilities:"
		for _, cap := range r.Capabilities {
			text += " " + cap
		}
	}
	if len(r.Tools) > 0 {
		text += " Tools:"
		for _, tool := range r.Tools {
			text += " " + tool
		}
	}
	return text
}

// CosineSimilarity computes similarity between two embedding vectors.
// Returns a value between -1 and 1 (1 = identical, 0 = unrelated).
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

// RankByEmbedding ranks resumes by cosine similarity to a query embedding.
// Returns resumes sorted by similarity (highest first) with their scores.
func RankByEmbedding(query []float64, resumes []Resume) []Match {
	matches := make([]Match, 0, len(resumes))
	for _, r := range resumes {
		if len(r.Embedding) == 0 {
			continue
		}
		score := CosineSimilarity(query, r.Embedding)
		matches = append(matches, Match{Resume: r, Score: score})
	}

	// Sort by score descending
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].Score > matches[j-1].Score; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	return matches
}

// Match pairs a resume with a similarity score.
type Match struct {
	Resume Resume  `json:"resume"`
	Score  float64 `json:"score"`
}
