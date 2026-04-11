package acp

// Priority indicates the importance of a plan entry.
type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

// PlanStatus tracks plan entry progress.
type PlanStatus string

const (
	PlanPending PlanStatus = "pending"
	PlanRunning PlanStatus = "in_progress"
	PlanDone    PlanStatus = "completed"
)

// PlanEntry is a single step in the agent's execution plan.
type PlanEntry struct {
	Content  string     `json:"content"`
	Priority Priority   `json:"priority"`
	Status   PlanStatus `json:"status"`
	Meta     Meta       `json:"_meta,omitempty"`
}
