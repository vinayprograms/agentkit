// Package plan defines execution plan steps sent via session updates.
package plan

// Status tracks step progress.
type Status string

const (
	Pending Status = "pending"
	Running Status = "in_progress"
	Done    Status = "completed"
)

// Priority indicates the importance of a step.
type Priority string

const (
	High   Priority = "high"
	Medium Priority = "medium"
	Low    Priority = "low"
)

// Step is a single entry in the agent's execution plan.
type Step struct {
	Content  string         `json:"content"`
	Priority Priority       `json:"priority"`
	Status   Status         `json:"status"`
	Meta     map[string]any `json:"_meta,omitempty"`
}
