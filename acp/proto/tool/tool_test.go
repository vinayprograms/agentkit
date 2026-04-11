package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/acp/proto/content"
)

func TestKindConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Kind
		want string
	}{
		{"Read", Read, "read"},
		{"Edit", Edit, "edit"},
		{"Delete", Delete, "delete"},
		{"Move", Move, "move"},
		{"Search", Search, "search"},
		{"Execute", Execute, "execute"},
		{"Think", Think, "think"},
		{"Fetch", Fetch, "fetch"},
		{"Other", Other, "other"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Kind %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestStatusConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Status
		want string
	}{
		{"Pending", Pending, "pending"},
		{"Running", Running, "in_progress"},
		{"Done", Done, "completed"},
		{"Failed", Failed, "failed"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Status %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestDecisionConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Decision
		want string
	}{
		{"AllowOnce", AllowOnce, "allow_once"},
		{"AllowAlways", AllowAlways, "allow_always"},
		{"RejectOnce", RejectOnce, "reject_once"},
		{"RejectAlways", RejectAlways, "reject_always"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Decision %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestCallRoundtrip(t *testing.T) {
	c := Call{
		ID:     "call-1",
		Title:  "Read file",
		Kind:   Read,
		Status: Running,
		Input:  `{"path":"/tmp/x"}`,
		Output: []content.Block{{Type: content.Text, Text: "file content"}},
		Location: &Location{
			Path: "/tmp/x",
			Line: 42,
		},
		TerminalID: "term-1",
		Diff:       &Diff{OldText: "old", NewText: "new"},
		Meta:       map[string]any{"k": "v"},
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{
		`"id"`, `"title"`, `"kind"`, `"status"`, `"input"`,
		`"output"`, `"location"`, `"terminalId"`, `"diff"`, `"_meta"`,
	} {
		if !strings.Contains(raw, key) {
			t.Errorf("JSON missing key %s", key)
		}
	}

	var got Call
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != c.ID || got.Kind != c.Kind || got.Status != c.Status {
		t.Errorf("Call roundtrip mismatch: got ID=%q Kind=%q Status=%q", got.ID, got.Kind, got.Status)
	}
	if got.Location == nil || got.Location.Line != 42 {
		t.Error("Location roundtrip failed")
	}
	if got.Diff == nil || got.Diff.OldText != "old" {
		t.Error("Diff roundtrip failed")
	}
}

func TestCallOmitempty(t *testing.T) {
	c := Call{ID: "call-2", Status: Pending}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{
		`"title"`, `"kind"`, `"input"`, `"output"`,
		`"location"`, `"terminalId"`, `"diff"`, `"_meta"`,
	} {
		if strings.Contains(raw, key) {
			t.Errorf("zero-value Call should omit %s, got %s", key, raw)
		}
	}
}

func TestPermissionRoundtrip(t *testing.T) {
	p := Permission{
		SessionID: "s1",
		ToolCall:  Call{ID: "c1", Status: Pending},
		Meta:      map[string]any{"x": 1.0},
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Permission
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != "s1" || got.ToolCall.ID != "c1" {
		t.Errorf("Permission roundtrip failed: %+v", got)
	}
}

func TestApprovalRoundtrip(t *testing.T) {
	a := Approval{Decision: AllowOnce, Meta: map[string]any{"y": 2.0}}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Approval
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Decision != AllowOnce {
		t.Errorf("Decision = %q, want %q", got.Decision, AllowOnce)
	}
}
