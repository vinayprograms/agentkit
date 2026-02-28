package resume

import (
	"testing"
)

func TestResume_Validate(t *testing.T) {
	tests := []struct {
		name    string
		resume  Resume
		wantErr error
	}{
		{
			name:    "valid",
			resume:  Resume{AgentID: "test-1", Name: "coder"},
			wantErr: nil,
		},
		{
			name:    "missing agent_id",
			resume:  Resume{Name: "coder"},
			wantErr: ErrEmptyAgentID,
		},
		{
			name:    "missing name",
			resume:  Resume{AgentID: "test-1"},
			wantErr: ErrEmptyName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resume.Validate()
			if err != tt.wantErr {
				t.Errorf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestResume_HasCapability(t *testing.T) {
	r := Resume{
		Capabilities: []string{"code.golang", "code.python", "test.unit"},
	}

	tests := []struct {
		query string
		want  bool
	}{
		{"code.golang", true},
		{"code.python", true},
		{"test.unit", true},
		{"code.rust", false},
		{"design.system", false},
		{"code.*", true},         // wildcard
		{"test.*", true},         // wildcard
		{"design.*", false},      // wildcard no match
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := r.HasCapability(tt.query)
			if got != tt.want {
				t.Errorf("HasCapability(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	sim := CosineSimilarity(a, b)
	if sim != 1.0 {
		t.Errorf("identical vectors: got %f, want 1.0", sim)
	}

	// Orthogonal vectors
	a = []float64{1, 0, 0}
	b = []float64{0, 1, 0}
	sim = CosineSimilarity(a, b)
	if sim != 0.0 {
		t.Errorf("orthogonal vectors: got %f, want 0.0", sim)
	}

	// Different lengths
	sim = CosineSimilarity([]float64{1, 0}, []float64{1, 0, 0})
	if sim != 0.0 {
		t.Errorf("different lengths: got %f, want 0.0", sim)
	}

	// Empty
	sim = CosineSimilarity([]float64{}, []float64{})
	if sim != 0.0 {
		t.Errorf("empty vectors: got %f, want 0.0", sim)
	}
}

func TestRankByEmbedding(t *testing.T) {
	query := []float64{1, 0, 0}

	resumes := []Resume{
		{AgentID: "a", Name: "far", Embedding: []float64{0, 1, 0}},
		{AgentID: "b", Name: "close", Embedding: []float64{0.9, 0.1, 0}},
		{AgentID: "c", Name: "exact", Embedding: []float64{1, 0, 0}},
		{AgentID: "d", Name: "no-embedding"},
	}

	matches := RankByEmbedding(query, resumes)

	if len(matches) != 3 {
		t.Fatalf("got %d matches, want 3 (skip no-embedding)", len(matches))
	}

	if matches[0].Resume.AgentID != "c" {
		t.Errorf("first match should be 'exact', got %s", matches[0].Resume.AgentID)
	}
	if matches[1].Resume.AgentID != "b" {
		t.Errorf("second match should be 'close', got %s", matches[1].Resume.AgentID)
	}
	if matches[2].Resume.AgentID != "a" {
		t.Errorf("third match should be 'far', got %s", matches[2].Resume.AgentID)
	}
}

func TestResume_MarshalUnmarshal(t *testing.T) {
	r := Resume{
		AgentID:      "test-1",
		Name:         "coder",
		Description:  "A coding agent",
		Capabilities: []string{"code.golang", "test.unit"},
		Tools:        []string{"bash", "write"},
		Embedding:    []float64{0.1, 0.2, 0.3},
	}

	data, err := r.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.AgentID != r.AgentID {
		t.Errorf("agent_id: got %s, want %s", got.AgentID, r.AgentID)
	}
	if got.Name != r.Name {
		t.Errorf("name: got %s, want %s", got.Name, r.Name)
	}
	if len(got.Capabilities) != len(r.Capabilities) {
		t.Errorf("capabilities: got %d, want %d", len(got.Capabilities), len(r.Capabilities))
	}
	if len(got.Embedding) != len(r.Embedding) {
		t.Errorf("embedding: got %d, want %d", len(got.Embedding), len(r.Embedding))
	}
}
