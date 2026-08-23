package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vinayprograms/agentkit/llm"
)

// summarizerMaxTokens bounds the summarizer's response. Reasoning models
// (e.g. gpt-oss) can spend their entire budget on hidden "thinking" tokens
// before producing any content, so this needs enough headroom above a
// typical thinking burst that some tokens are still left for the answer.
const summarizerMaxTokens = 4000

// ErrEmptySummary is returned by Summarize when the model produced no
// usable content — typically a reasoning model that exhausted its token
// budget on thinking (stop_reason "length") before writing an answer.
// Callers should treat this the same as any other summarization failure
// (e.g. degrade to returning raw content).
var ErrEmptySummary = errors.New("summarizer returned empty content")

// Summarizer extracts information from content.
type Summarizer interface {
	Summarize(ctx context.Context, content, question string) (string, error)
}

// LLMSummarizer implements Summarizer using an LLM model.
type llmSummarizer struct {
	model llm.Model
}

// NewSummarizer creates a Summarizer backed by the given LLM model.
func NewSummarizer(model llm.Model) Summarizer {
	return &llmSummarizer{model: model}
}

// Summarize extracts information from content based on a question.
func (s *llmSummarizer) Summarize(ctx context.Context, content, question string) (string, error) {
	if s.model == nil {
		return "", fmt.Errorf("no LLM configured for summarization")
	}

	prompt := fmt.Sprintf(`Web page content:
---
%s
---

%s

Provide a concise response based only on the content above. In your response:
- Keep the answer focused and relevant to the question
- Use quotation marks for exact language from the content
- Limit quotes to 125 characters maximum
- If the content doesn't contain relevant information, say so
- Be concise but thorough`, content, question)

	req := llm.Prompt(prompt, llm.MaxTokens(summarizerMaxTokens))
	req.Thinking = llm.ThinkingOff // this call wants a direct answer, not reasoning traces

	resp, err := s.model.Chat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarization LLM call failed: %w", err)
	}

	if strings.TrimSpace(resp.Content) == "" {
		return "", fmt.Errorf("%w (stop_reason=%q, thinking_chars=%d)", ErrEmptySummary, resp.StopReason, len(resp.Thinking))
	}

	return resp.Content, nil
}
