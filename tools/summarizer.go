package tools

import (
	"context"
	"fmt"

	"github.com/vinayprograms/agentkit/llm"
)

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

	resp, err := s.model.Chat(ctx, llm.Prompt(prompt, llm.MaxTokens(1000)))
	if err != nil {
		return "", fmt.Errorf("summarization LLM call failed: %w", err)
	}

	return resp.Content, nil
}
