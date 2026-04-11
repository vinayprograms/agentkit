package update

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vinayprograms/agentkit/acp/proto/config"
	"github.com/vinayprograms/agentkit/acp/proto/plan"
	"github.com/vinayprograms/agentkit/acp/proto/tool"
)

func TestTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Message", Message, "messageChunk"},
		{"ToolCall", ToolCall, "toolCall"},
		{"Plan", Plan, "planUpdate"},
		{"Config", Config, "configOptionUpdate"},
		{"Commands", Commands, "availableCommandsUpdate"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestUpdateMessageRoundtrip(t *testing.T) {
	u := Update{
		SessionID: "s1",
		Type:      Message,
		Role:      "assistant",
		Chunk:     "Hello, world!",
		Meta:      map[string]any{"seq": 1.0},
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{`"sessionId"`, `"type"`, `"role"`, `"chunk"`, `"_meta"`} {
		if !strings.Contains(raw, key) {
			t.Errorf("JSON missing key %s", key)
		}
	}

	var got Update
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SessionID != "s1" || got.Type != Message || got.Chunk != "Hello, world!" {
		t.Errorf("Message roundtrip mismatch: %+v", got)
	}
}

func TestUpdateToolCallRoundtrip(t *testing.T) {
	u := Update{
		SessionID: "s1",
		Type:      ToolCall,
		ToolCall:  &tool.Call{ID: "tc-1", Status: tool.Running},
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Update
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ToolCall == nil || got.ToolCall.ID != "tc-1" {
		t.Error("ToolCall roundtrip failed")
	}
}

func TestUpdatePlanRoundtrip(t *testing.T) {
	u := Update{
		SessionID: "s1",
		Type:      Plan,
		Plan:      []plan.Step{{Content: "step 1", Priority: plan.High, Status: plan.Pending}},
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Update
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got.Plan) != 1 || got.Plan[0].Content != "step 1" {
		t.Error("Plan roundtrip failed")
	}
}

func TestUpdateConfigRoundtrip(t *testing.T) {
	u := Update{
		SessionID: "s1",
		Type:      Config,
		Setting:   &config.Option{ID: "opt-1", Name: "Mode", Type: "select", Value: "fast"},
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if !strings.Contains(string(data), `"configOption"`) {
		t.Error("JSON should use key configOption")
	}

	var got Update
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Setting == nil || got.Setting.ID != "opt-1" {
		t.Error("Config roundtrip failed")
	}
}

func TestUpdateCommandsRoundtrip(t *testing.T) {
	u := Update{
		SessionID: "s1",
		Type:      Commands,
		Commands:  []config.Command{{Name: "help"}},
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Update
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got.Commands) != 1 || got.Commands[0].Name != "help" {
		t.Error("Commands roundtrip failed")
	}
}

func TestUpdateOmitempty(t *testing.T) {
	u := Update{SessionID: "s1", Type: Message}
	data, _ := json.Marshal(u)
	raw := string(data)

	for _, key := range []string{
		`"role"`, `"chunk"`, `"toolCall"`, `"plan"`,
		`"configOption"`, `"commands"`, `"_meta"`,
	} {
		if strings.Contains(raw, key) {
			t.Errorf("zero-value Update should omit %s, got %s", key, raw)
		}
	}
}
