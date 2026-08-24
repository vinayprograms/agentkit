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

// extractTool is the structured-decision tool Extract asks the model to
// call: findings/insights/lessons as explicit tool-call arguments instead
// of a JSON object scraped out of prose (which may or may not be wrapped in
// a ``` fence, and which a chain-of-thought preamble can defeat either way
// — see the parseFIL doc comment and REPORT.md bug #11).
var extractTool = llm.ToolDef{
	Name:        "extract",
	Description: "Report the findings, insights, and lessons extracted from the text.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"findings": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Factual discoveries"},
			"insights": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Conclusions or decisions made"},
			"lessons":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Learnings that should guide future work"},
		},
	},
}

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

	d, err := llm.Ask(ctx, e.model, extractionPrompt+"\n\n"+userContent, extractTool, parseFILFallback)
	if err != nil {
		// Don't fail the caller if extraction fails
		return nil, nil, nil, nil
	}

	f, i, l := filFromDecision(d)
	return f, i, l, nil
}

// filFromDecision converts an llm.Decision from the extract tool (or its
// prose fallback) into findings/insights/lessons slices.
func filFromDecision(d *llm.Decision) (findings, insights, lessons []string) {
	if d.Args == nil {
		return nil, nil, nil
	}
	return stringSlice(d.Args["findings"]), stringSlice(d.Args["insights"]), stringSlice(d.Args["lessons"])
}

// stringSlice coerces a tool-call argument value (decoded from JSON as
// []interface{}) into a []string, skipping non-string elements.
func stringSlice(v any) []string {
	items, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseFILFallback adapts parseFIL to llm.ParseFallback: it's the prose
// fallback llm.Ask uses when the model answers in text instead of calling
// extractTool.
func parseFILFallback(content string) (map[string]any, bool) {
	f, i, l := parseFIL(content)
	if f == nil && i == nil && l == nil {
		return nil, false
	}
	args := map[string]any{}
	if f != nil {
		args["findings"] = toAnySlice(f)
	}
	if i != nil {
		args["insights"] = toAnySlice(i)
	}
	if l != nil {
		args["lessons"] = toAnySlice(l)
	}
	return args, true
}

func toAnySlice(s []string) []interface{} {
	out := make([]interface{}, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// parseFIL extracts FIL arrays from an LLM response that may contain JSON
// wrapped in markdown code blocks. It survives as the prose fallback for
// providers/models that can't honor ToolChoice, or that answer in text
// anyway — the primary path is the structured extract tool call above.
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
