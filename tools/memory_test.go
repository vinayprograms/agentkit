package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/types"
)

// mockSemanticMemory implements SemanticMemory for testing.
type mockSemanticMemory struct {
	items []types.ObservationItem
	idSeq int
}

func newMockSemanticMemory() *mockSemanticMemory {
	return &mockSemanticMemory{}
}

func (m *mockSemanticMemory) RememberFIL(ctx context.Context, findings, insights, lessons []string, source string) ([]string, error) {
	var ids []string
	for _, f := range findings {
		m.idSeq++
		id := fmt.Sprintf("f-%d", m.idSeq)
		m.items = append(m.items, types.ObservationItem{ID: id, Content: f, Category: "finding"})
		ids = append(ids, id)
	}
	for _, i := range insights {
		m.idSeq++
		id := fmt.Sprintf("i-%d", m.idSeq)
		m.items = append(m.items, types.ObservationItem{ID: id, Content: i, Category: "insight"})
		ids = append(ids, id)
	}
	for _, l := range lessons {
		m.idSeq++
		id := fmt.Sprintf("l-%d", m.idSeq)
		m.items = append(m.items, types.ObservationItem{ID: id, Content: l, Category: "lesson"})
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *mockSemanticMemory) RetrieveByID(ctx context.Context, id string) (*types.ObservationItem, error) {
	for _, item := range m.items {
		if item.ID == id {
			return &item, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (m *mockSemanticMemory) RecallFIL(ctx context.Context, query string, limitPerCategory int) (*types.FILResult, error) {
	result := &types.FILResult{}
	q := strings.ToLower(query)
	for _, item := range m.items {
		if !strings.Contains(strings.ToLower(item.Content), q) {
			continue
		}
		switch item.Category {
		case "finding":
			if len(result.Findings) < limitPerCategory {
				result.Findings = append(result.Findings, item.Content)
			}
		case "insight":
			if len(result.Insights) < limitPerCategory {
				result.Insights = append(result.Insights, item.Content)
			}
		case "lesson":
			if len(result.Lessons) < limitPerCategory {
				result.Lessons = append(result.Lessons, item.Content)
			}
		}
	}
	return result, nil
}

func (m *mockSemanticMemory) Recall(ctx context.Context, query string, limit int) ([]types.SemanticMemoryResult, error) {
	var results []types.SemanticMemoryResult
	q := strings.ToLower(query)
	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.Content), q) {
			results = append(results, types.SemanticMemoryResult{
				ID:       item.ID,
				Content:  item.Content,
				Category: item.Category,
				Score:    1.0,
			})
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

func TestRemember_StoreFIL(t *testing.T) {
	mem := newMockSemanticMemory()
	tool := Remember(mem)

	args, err := Validate(tool.Parameters(), map[string]any{
		"findings": []any{"Database uses PostgreSQL"},
		"insights": []any{"PostgreSQL chosen for JSON support"},
		"lessons":  []any{"Always check rate limits first"},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Stored 3 observations") {
		t.Errorf("expected 'Stored 3 observations' in result, got %q", result)
	}
	if !strings.Contains(result, "f-1") {
		t.Errorf("expected finding ID in result, got %q", result)
	}
	if len(mem.items) != 3 {
		t.Errorf("expected 3 items stored, got %d", len(mem.items))
	}
}

func TestRemember_EmptyLists(t *testing.T) {
	mem := newMockSemanticMemory()
	tool := Remember(mem)

	// All empty -- should error
	args, err := Validate(tool.Parameters(), map[string]any{
		"findings": []any{},
		"insights": []any{},
		"lessons":  []any{},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("expected error when all lists are empty")
	}
}

func TestRemember_PartialLists(t *testing.T) {
	mem := newMockSemanticMemory()
	tool := Remember(mem)

	// Only findings provided
	args, err := Validate(tool.Parameters(), map[string]any{
		"findings": []any{"fact one", "fact two"},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Stored 2 observations") {
		t.Errorf("expected 'Stored 2 observations', got %q", result)
	}
}

func TestRecall_ReturnsCategories(t *testing.T) {
	mem := newMockSemanticMemory()
	// Pre-populate memory
	mem.RememberFIL(context.Background(),
		[]string{"PostgreSQL has great JSON support"},
		[]string{"We chose PostgreSQL for its JSON features"},
		[]string{"Always benchmark PostgreSQL JSON queries"},
		"test",
	)

	tool := Recall(mem)
	args, err := Validate(tool.Parameters(), map[string]any{"query": "postgresql"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "Findings:") {
		t.Errorf("expected Findings section, got %q", result)
	}
	if !strings.Contains(result, "Insights:") {
		t.Errorf("expected Insights section, got %q", result)
	}
	if !strings.Contains(result, "Lessons:") {
		t.Errorf("expected Lessons section, got %q", result)
	}
}

func TestRecall_NoResults(t *testing.T) {
	mem := newMockSemanticMemory()
	tool := Recall(mem)

	args, err := Validate(tool.Parameters(), map[string]any{"query": "nonexistent"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	if result != "No relevant memories found." {
		t.Errorf("expected 'No relevant memories found.', got %q", result)
	}
}

func TestRecall_WithLimit(t *testing.T) {
	mem := newMockSemanticMemory()
	// Store many findings with "api" in them
	mem.RememberFIL(context.Background(),
		[]string{"api fact 1", "api fact 2", "api fact 3"},
		nil, nil, "test",
	)

	tool := Recall(mem)
	args, err := Validate(tool.Parameters(), map[string]any{"query": "api", "limit": 2})
	if err != nil {
		t.Fatal(err)
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	// Count findings lines
	findingsCount := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- api fact") {
			findingsCount++
		}
	}
	if findingsCount > 2 {
		t.Errorf("expected at most 2 findings (limit=2), got %d", findingsCount)
	}
}
