package terminal

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCreateRoundtrip(t *testing.T) {
	c := Create{
		Command: "go", Args: []string{"test"}, Cwd: "/project",
		Env: map[string]string{"GOFLAGS": "-v"}, OutputLimit: 1000,
	}
	data, _ := json.Marshal(c)

	var got Create
	json.Unmarshal(data, &got)
	if got.Command != "go" || len(got.Args) != 1 || got.Cwd != "/project" || got.OutputLimit != 1000 {
		t.Errorf("roundtrip: %+v", got)
	}
}

func TestCreateOmitempty(t *testing.T) {
	c := Create{Command: "ls"}
	data, _ := json.Marshal(c)
	raw := string(data)
	for _, key := range []string{`"args":`, `"cwd":`, `"env":`, `"outputLimit":`} {
		if strings.Contains(raw, key) {
			t.Errorf("should omit %s, got %s", key, raw)
		}
	}
}

func TestCreatedRoundtrip(t *testing.T) {
	c := Created{TerminalID: "t-1"}
	data, _ := json.Marshal(c)

	var got Created
	json.Unmarshal(data, &got)
	if got.TerminalID != "t-1" {
		t.Errorf("roundtrip: %+v", got)
	}
}

func TestRefRoundtrip(t *testing.T) {
	r := Ref{TerminalID: "t-2"}
	data, _ := json.Marshal(r)

	if !strings.Contains(string(data), `"terminalId":"t-2"`) {
		t.Errorf("expected terminalId in JSON: %s", data)
	}
}

func TestResultRoundtrip(t *testing.T) {
	r := Result{ExitCode: 1, Output: "error"}
	data, _ := json.Marshal(r)

	var got Result
	json.Unmarshal(data, &got)
	if got.ExitCode != 1 || got.Output != "error" {
		t.Errorf("roundtrip: %+v", got)
	}
}
