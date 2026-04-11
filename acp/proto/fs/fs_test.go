package fs

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadParamsRoundtrip(t *testing.T) {
	p := ReadParams{Path: "/src/main.go", Line: 10, Limit: 20}
	data, _ := json.Marshal(p)

	var got ReadParams
	json.Unmarshal(data, &got)
	if got.Path != "/src/main.go" || got.Line != 10 || got.Limit != 20 {
		t.Errorf("roundtrip: %+v", got)
	}
}

func TestReadParamsOmitempty(t *testing.T) {
	p := ReadParams{Path: "/a"}
	data, _ := json.Marshal(p)
	raw := string(data)
	for _, key := range []string{`"line":`, `"limit":`, `"_meta":`} {
		if strings.Contains(raw, key) {
			t.Errorf("should omit %s, got %s", key, raw)
		}
	}
}

func TestWriteParamsRoundtrip(t *testing.T) {
	p := WriteParams{Path: "/a.txt", Content: "hello"}
	data, _ := json.Marshal(p)

	var got WriteParams
	json.Unmarshal(data, &got)
	if got.Path != "/a.txt" || got.Content != "hello" {
		t.Errorf("roundtrip: %+v", got)
	}
}

func TestReadResultRoundtrip(t *testing.T) {
	r := ReadResult{Content: "package main"}
	data, _ := json.Marshal(r)

	var got ReadResult
	json.Unmarshal(data, &got)
	if got.Content != "package main" {
		t.Errorf("roundtrip: %+v", got)
	}
}
