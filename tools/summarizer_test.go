package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewSummarizer constructor
// ---------------------------------------------------------------------------

func TestNewSummarizer(t *testing.T) {
	mock := &mockLLM{response: "ok"}
	s := NewSummarizer(mock)
	if s == nil {
		t.Fatal("expected non-nil summarizer")
	}
}

func TestNewSummarizer_NilModel(t *testing.T) {
	s := NewSummarizer(nil)
	if s == nil {
		t.Fatal("expected non-nil summarizer even with nil model")
	}
}

// ---------------------------------------------------------------------------
// Summarize with mock LLM
// ---------------------------------------------------------------------------

func TestSummarize_Success(t *testing.T) {
	mock := &mockLLM{response: "The page describes Go as a compiled language."}
	s := NewSummarizer(mock)

	answer, err := s.Summarize(context.Background(), "Go is a compiled language.", "What is Go?")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if answer != "The page describes Go as a compiled language." {
		t.Errorf("unexpected answer: %q", answer)
	}
}

// ---------------------------------------------------------------------------
// Summarize with nil model — should error
// ---------------------------------------------------------------------------

func TestSummarize_NilModel(t *testing.T) {
	s := NewSummarizer(nil)

	_, err := s.Summarize(context.Background(), "content", "question")
	if err == nil {
		t.Fatal("expected error for nil model")
	}
	if !strings.Contains(err.Error(), "no LLM configured") {
		t.Errorf("expected 'no LLM configured' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Summarize when LLM returns error
// ---------------------------------------------------------------------------

func TestSummarize_LLMError(t *testing.T) {
	mock := &mockLLM{err: fmt.Errorf("connection refused")}
	s := NewSummarizer(mock)

	_, err := s.Summarize(context.Background(), "content", "question")
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected error to contain 'connection refused', got: %v", err)
	}
	if !strings.Contains(err.Error(), "summarization LLM call failed") {
		t.Errorf("expected wrapped error message, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Summarize with empty content
// ---------------------------------------------------------------------------

func TestSummarize_EmptyContent(t *testing.T) {
	mock := &mockLLM{response: "The page contains no relevant information."}
	s := NewSummarizer(mock)

	answer, err := s.Summarize(context.Background(), "", "What is this about?")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if answer != "The page contains no relevant information." {
		t.Errorf("unexpected answer: %q", answer)
	}
}
