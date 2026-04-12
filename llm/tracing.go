package llm

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/llm")

// tracedModel wraps a Model to emit an OTel span per Chat call.
type tracedModel struct {
	inner    Model
	provider string
	model    string
}

func withTracing(inner Model, provider, model string) Model {
	return &tracedModel{inner: inner, provider: provider, model: model}
}

func (t *tracedModel) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	ctx, span := tracer.Start(ctx, "llm.chat",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("llm.provider", t.provider),
			attribute.String("llm.model", t.model),
		),
	)
	defer span.End()

	resp, err := t.inner.Chat(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return resp, err
	}
	if resp != nil {
		span.SetAttributes(
			attribute.Int("llm.tokens.input", resp.InputTokens),
			attribute.Int("llm.tokens.output", resp.OutputTokens),
		)
	}
	return resp, nil
}
