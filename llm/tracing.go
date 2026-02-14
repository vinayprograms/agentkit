// Tracing wrapper for LLM providers.
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/vinayprograms/agentkit/telemetry"
)

// TracingProvider wraps a Provider with OpenTelemetry tracing.
type TracingProvider struct {
	provider     Provider
	providerName string
}

// WithTracing wraps a provider with tracing instrumentation.
func WithTracing(p Provider, providerName string) Provider {
	return &TracingProvider{
		provider:     p,
		providerName: providerName,
	}
}

// Chat implements Provider with tracing.
func (tp *TracingProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	tracer := telemetry.GetTracer()

	ctx, span := tracer.StartLLMSpan(ctx, "llm.chat")

	resp, err := tp.provider.Chat(ctx, req)

	// Build span options
	opts := telemetry.LLMSpanOptions{
		Provider: tp.providerName,
	}

	if resp != nil {
		opts.Model = resp.Model
		opts.TokensIn = resp.InputTokens
		opts.TokensOut = resp.OutputTokens
		opts.Response = resp.Content
		opts.Thinking = resp.Thinking
	}

	// Build prompt from messages (only used if debug is enabled)
	if tracer.Debug() {
		var parts []string
		for _, msg := range req.Messages {
			parts = append(parts, fmt.Sprintf("[%s] %s", msg.Role, msg.Content))
		}
		opts.Prompt = strings.Join(parts, "\n")
	}

	tracer.EndLLMSpan(span, opts, err)

	return resp, err
}
