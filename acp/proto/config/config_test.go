package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCategoryConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Category
		want string
	}{
		{"Mode", Mode, "mode"},
		{"Model", Model, "model"},
		{"Thought", Thought, "thought_level"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Category %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestOptionRoundtrip(t *testing.T) {
	o := Option{
		ID:       "opt-1",
		Name:     "Mode",
		Category: Mode,
		Type:     "select",
		Value:    "fast",
		Choices:  []Choice{{Value: "fast", Label: "Fast"}, {Value: "careful", Label: "Careful"}},
		Meta:     map[string]any{"v": 1.0},
	}

	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{
		`"id"`, `"name"`, `"category"`, `"type"`, `"value"`, `"choices"`, `"_meta"`,
	} {
		if !strings.Contains(raw, key) {
			t.Errorf("JSON missing key %s", key)
		}
	}

	var got Option
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != o.ID || got.Category != Mode || len(got.Choices) != 2 {
		t.Errorf("Option roundtrip mismatch: %+v", got)
	}
}

func TestOptionOmitempty(t *testing.T) {
	o := Option{ID: "opt-2", Name: "X", Type: "select", Value: "a"}
	data, _ := json.Marshal(o)
	raw := string(data)

	for _, key := range []string{`"category"`, `"choices"`, `"_meta"`} {
		if strings.Contains(raw, key) {
			t.Errorf("zero-value Option should omit %s, got %s", key, raw)
		}
	}
}

func TestSetParamsRoundtrip(t *testing.T) {
	p := SetParams{SessionID: "s1", OptionID: "opt-1", Value: "careful", Meta: map[string]any{"a": "b"}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got SetParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != "s1" || got.OptionID != "opt-1" || got.Value != "careful" {
		t.Errorf("SetParams roundtrip mismatch: %+v", got)
	}
}

func TestSetResultOmitempty(t *testing.T) {
	r := SetResult{}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), `"_meta"`) {
		t.Error("zero-value SetResult should omit _meta")
	}
}

func TestModeParamsRoundtrip(t *testing.T) {
	p := ModeParams{SessionID: "s1", Mode: "fast", Meta: map[string]any{"x": 1.0}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got ModeParams
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != "s1" || got.Mode != "fast" {
		t.Errorf("ModeParams roundtrip mismatch: %+v", got)
	}
}

func TestCommandRoundtrip(t *testing.T) {
	c := Command{Name: "review", Description: "Review code", InputHint: "PR number", Input: "123"}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{`"name"`, `"description"`, `"inputHint"`, `"input"`} {
		if !strings.Contains(raw, key) {
			t.Errorf("JSON missing key %s", key)
		}
	}

	var got Command
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != c {
		t.Errorf("Command roundtrip mismatch: got %+v, want %+v", got, c)
	}
}

func TestCommandOmitempty(t *testing.T) {
	c := Command{Name: "help"}
	data, _ := json.Marshal(c)
	raw := string(data)

	for _, key := range []string{`"description"`, `"inputHint"`, `"input"`} {
		if strings.Contains(raw, key) {
			t.Errorf("zero-value Command should omit %s, got %s", key, raw)
		}
	}
}
