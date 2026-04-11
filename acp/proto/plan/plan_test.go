package plan

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Status
		want string
	}{
		{"Pending", Pending, "pending"},
		{"Running", Running, "in_progress"},
		{"Done", Done, "completed"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Status %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestPriorityConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Priority
		want string
	}{
		{"High", High, "high"},
		{"Medium", Medium, "medium"},
		{"Low", Low, "low"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Priority %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestStepRoundtrip(t *testing.T) {
	s := Step{
		Content:  "Run tests",
		Priority: High,
		Status:   Running,
		Meta:     map[string]any{"order": 1.0},
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{`"content"`, `"priority"`, `"status"`, `"_meta"`} {
		if !strings.Contains(raw, key) {
			t.Errorf("JSON missing key %s", key)
		}
	}

	var got Step
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Content != s.Content || got.Priority != High || got.Status != Running {
		t.Errorf("Step roundtrip mismatch: %+v", got)
	}
}

func TestStepOmitempty(t *testing.T) {
	s := Step{Content: "x", Priority: Low, Status: Pending}
	data, _ := json.Marshal(s)
	if strings.Contains(string(data), `"_meta"`) {
		t.Error("zero-value Step should omit _meta")
	}
}
