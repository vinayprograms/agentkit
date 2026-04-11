package prompt

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/acp/proto/config"
	"github.com/vinayprograms/agentkit/acp/proto/content"
)

func TestReasonConstants(t *testing.T) {
	tests := []struct {
		name string
		got  Reason
		want string
	}{
		{"EndTurn", EndTurn, "end_turn"},
		{"MaxTokens", MaxTokens, "max_tokens"},
		{"MaxTurns", MaxTurns, "max_turn_requests"},
		{"Refusal", Refusal, "refusal"},
		{"Cancelled", Cancelled, "cancelled"},
	}
	for _, tt := range tests {
		if string(tt.got) != tt.want {
			t.Errorf("Reason %s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestParamsRoundtrip(t *testing.T) {
	p := Params{
		SessionID: "s1",
		Content:   []content.Block{{Type: content.Text, Text: "hello"}},
		Command:   &config.Command{Name: "review", Description: "Review code"},
		Meta:      map[string]any{"k": "v"},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{`"sessionId"`, `"content"`, `"command"`, `"_meta"`} {
		if !strings.Contains(raw, key) {
			t.Errorf("JSON missing key %s", key)
		}
	}

	var got Params
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != "s1" || len(got.Content) != 1 || got.Command == nil {
		t.Errorf("Params roundtrip failed: %+v", got)
	}
}

func TestParamsOmitempty(t *testing.T) {
	p := Params{SessionID: "s1", Content: []content.Block{{Type: content.Text}}}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{`"command"`, `"_meta"`} {
		if strings.Contains(raw, key) {
			t.Errorf("zero-value Params should omit %s, got %s", key, raw)
		}
	}
}

func TestResultRoundtrip(t *testing.T) {
	r := Result{Reason: EndTurn, Meta: map[string]any{"t": 1.0}}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if !strings.Contains(string(data), `"stopReason"`) {
		t.Error("Result JSON should use key stopReason")
	}

	var got Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Reason != EndTurn {
		t.Errorf("Reason = %q, want %q", got.Reason, EndTurn)
	}
}

func TestResultOmitempty(t *testing.T) {
	r := Result{Reason: EndTurn}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), `"_meta"`) {
		t.Error("zero-value Result should omit _meta")
	}
}
