// Tracing wrapper for LLM providers.
package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/vinayprograms/agentkit/telemetry"
)

// tracingModel wraps a Provider with OpenTelemetry tracing.
type tracingModel struct {
	provider     Model
	serviceName string
}

// WithTracing wraps a provider with tracing instrumentation.
func WithTracing(p Model, serviceName string) Model {
	return &tracingModel{
		provider:     p,
		serviceName: serviceName,
	}
}

// Chat implements Provider with tracing.
func (tp *tracingModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	tracer := telemetry.GetTracer()

	ctx, span := tracer.StartLLMSpan(ctx, "llm.chat")

	resp, err := tp.provider.Chat(ctx, req)

	// Build span options
	opts := telemetry.LLMSpanOptions{
		Provider: tp.serviceName,
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
