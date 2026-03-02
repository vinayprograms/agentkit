package embedding

import "testing"

func TestNewNoneProvider(t *testing.T) {
	e, err := New(Config{Provider: "none"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e != nil {
		t.Fatal("expected nil embedder for 'none' provider")
	}
}

func TestNewEmptyProvider(t *testing.T) {
	e, err := New(Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e != nil {
		t.Fatal("expected nil embedder for empty provider")
	}
}

func TestNewUnknownProvider(t *testing.T) {
	_, err := New(Config{Provider: "banana"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestNewOpenAIMissingKey(t *testing.T) {
	_, err := New(Config{Provider: "openai"})
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestNewOpenAIWithKey(t *testing.T) {
	e, err := New(Config{Provider: "openai", APIKey: "sk-test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected non-nil embedder")
	}
}

func TestNewGoogleMissingKey(t *testing.T) {
	_, err := New(Config{Provider: "google"})
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
}

func TestNewGoogleWithKey(t *testing.T) {
	e, err := New(Config{Provider: "google", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected non-nil embedder")
	}
}

func TestNewOpenAICompatMissingBaseURL(t *testing.T) {
	_, err := New(Config{Provider: "openai-compat", Model: "nomic-embed-text"})
	if err == nil {
		t.Fatal("expected error for missing base_url")
	}
}

func TestNewOpenAICompatMissingModel(t *testing.T) {
	_, err := New(Config{Provider: "openai-compat", BaseURL: "http://localhost:11434/v1"})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestNewOpenAICompat(t *testing.T) {
	e, err := New(Config{
		Provider: "openai-compat",
		BaseURL:  "http://localhost:11434/v1",
		Model:    "nomic-embed-text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected non-nil embedder")
	}
}

func TestProviderCaseInsensitive(t *testing.T) {
	e, err := New(Config{Provider: "OpenAI", APIKey: "sk-test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e == nil {
		t.Fatal("expected non-nil embedder")
	}
}
