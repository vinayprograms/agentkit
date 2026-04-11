package content

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Text", Text, "text"},
		{"Image", Image, "image"},
		{"Audio", Audio, "audio"},
		{"Resource", Resource, "resource"},
		{"Link", Link, "resource_link"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestBlockRoundtrip(t *testing.T) {
	block := Block{
		Type:        Text,
		Text:        "hello",
		Data:        "base64data",
		MimeType:    "image/png",
		Embedded:    &Embedded{URI: "file:///a.txt", MimeType: "text/plain", Text: "content", Data: "b64"},
		URI:         "https://example.com",
		Name:        "example",
		Description: "a link",
		Annotations: map[string]any{"key": "val"},
		Meta:        map[string]any{"debug": true},
	}

	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	// Verify JSON keys.
	for _, key := range []string{
		`"type"`, `"text"`, `"data"`, `"mimeType"`, `"resource"`,
		`"uri"`, `"name"`, `"description"`, `"annotations"`, `"_meta"`,
	} {
		if !strings.Contains(raw, key) {
			t.Errorf("JSON missing key %s", key)
		}
	}

	var got Block
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Type != block.Type {
		t.Errorf("Type = %q, want %q", got.Type, block.Type)
	}
	if got.Text != block.Text {
		t.Errorf("Text = %q, want %q", got.Text, block.Text)
	}
	if got.Embedded == nil || got.Embedded.URI != "file:///a.txt" {
		t.Errorf("Embedded roundtrip failed")
	}
}

func TestBlockOmitempty(t *testing.T) {
	block := Block{Type: Text}

	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{
		`"text":`, `"data":`, `"mimeType":`, `"resource":`,
		`"uri":`, `"name":`, `"description":`, `"annotations":`, `"_meta":`,
	} {
		if strings.Contains(raw, key) {
			t.Errorf("zero-value Block should omit %s, got %s", key, raw)
		}
	}
}

func TestEmbeddedRoundtrip(t *testing.T) {
	e := Embedded{URI: "file:///b.txt", MimeType: "text/plain", Text: "hello", Data: "abc"}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got Embedded
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got != e {
		t.Errorf("Embedded roundtrip: got %+v, want %+v", got, e)
	}
}

func TestEmbeddedOmitempty(t *testing.T) {
	e := Embedded{URI: "file:///c.txt"}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	raw := string(data)

	for _, key := range []string{`"mimeType"`, `"text"`, `"data"`} {
		if strings.Contains(raw, key) {
			t.Errorf("zero-value Embedded should omit %s, got %s", key, raw)
		}
	}
	if !strings.Contains(raw, `"uri"`) {
		t.Error("Embedded must always contain uri")
	}
}
