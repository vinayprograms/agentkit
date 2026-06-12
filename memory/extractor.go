package memory

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/vinayprograms/agentkit/llm"
)

// Extractor uses an LLM to extract findings, insights, and lessons from text.
// The consumer decides when to call it (e.g., after each agent step) and what
// to do with the results (e.g., store via RememberFIL).
type Extractor struct {
	model llm.Model
}

// NewExtractor creates an extractor backed by the given LLM model.
func NewExtractor(model llm.Model) *Extractor {
	return &Extractor{model: model}
}

// defaultMaxInputChars bounds how much text is sent to the LLM. Input beyond
// this is truncated, matching the legacy extractor's behavior.
const defaultMaxInputChars = 4000

// ExtractOption configures a single Extract call.
type ExtractOption func(*extractOptions)

type extractOptions struct {
	source        string
	maxInputChars int
}

// WithSource labels the text with its origin (e.g. a step name/type), giving
// the LLM context that improves extraction quality. The label is included in
// the prompt but never replaces the text itself.
func WithSource(label string) ExtractOption {
	return func(o *extractOptions) { o.source = label }
}

// WithMaxInputChars overrides the input-truncation bound (default 4000).
// A value <= 0 disables truncation.
func WithMaxInputChars(n int) ExtractOption {
	return func(o *extractOptions) { o.maxInputChars = n }
}

const extractionPrompt = `You are an observation extractor. Given text output, extract:

1. **Findings**: Factual discoveries (facts, data, configurations found)
2. **Insights**: Conclusions, inferences, or decisions made
3. **Lessons**: Learnings that should guide future work (what to do/avoid)

Return a JSON object with these arrays. Be concise - each item should be a single sentence.
Only include meaningful observations. If a category has nothing, return an empty array.

Example:
{"findings": ["The API rate limit is 100 requests per minute"], "insights": ["REST is more suitable than GraphQL"], "lessons": ["Always check rate limits before integration"]}`

// Extract parses text into findings, insights, and lessons.
// Returns nil slices if the text is too short or the LLM can't extract anything.
//
// By default input is truncated to 4000 characters; use WithMaxInputChars to
// change the bound and WithSource to label the text's origin for the LLM.
func (e *Extractor) Extract(ctx context.Context, text string, opts ...ExtractOption) (findings, insights, lessons []string, err error) {
	if e.model == nil {
		return nil, nil, nil, nil
	}

	if len(strings.TrimSpace(text)) < 50 {
		return nil, nil, nil, nil
	}

	o := extractOptions{maxInputChars: defaultMaxInputChars}
	for _, opt := range opts {
		opt(&o)
	}

	if o.maxInputChars > 0 && len(text) > o.maxInputChars {
		text = text[:o.maxInputChars] + "\n... [truncated]"
	}

	userContent := text
	if o.source != "" {
		userContent = "Source: " + o.source + "\n\n" + text
	}

	resp, err := e.model.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: extractionPrompt},
			{Role: "user", Content: userContent},
		},
	})
	if err != nil {
		// Don't fail the caller if extraction fails
		return nil, nil, nil, nil
	}

	f, i, l := parseFIL(resp.Content)
	return f, i, l, nil
}

// parseFIL extracts FIL arrays from an LLM response that may contain JSON
// wrapped in markdown code blocks.
func parseFIL(content string) (findings, insights, lessons []string) {
	content = strings.TrimSpace(content)

	// Strip markdown code block if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		content = strings.Join(jsonLines, "\n")
	}

	// Find JSON object bounds
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, nil, nil
	}
	content = content[start : end+1]

	var result struct {
		Findings []string `json:"findings"`
		Insights []string `json:"insights"`
		Lessons  []string `json:"lessons"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, nil, nil
	}

	return result.Findings, result.Insights, result.Lessons
}
