// Package types provides shared types used across agentkit packages.
package types

// ObservationItem represents a stored observation with its metadata.
type ObservationItem struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Category string `json:"category"` // "finding" | "insight" | "lesson"
}

// FILResult holds categorized observation results.
type FILResult struct {
	Findings []string `json:"findings"`
	Insights []string `json:"insights"`
	Lessons  []string `json:"lessons"`
}

// SemanticMemoryResult is a memory with relevance score.
type SemanticMemoryResult struct {
	ID       string  `json:"id"`
	Content  string  `json:"content"`
	Category string  `json:"category"` // "finding" | "insight" | "lesson"
	Score    float32 `json:"score"`
}
