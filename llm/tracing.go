package llm

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
)

const scope = "llm"

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/llm")

// trace starts a client span scoped to the llm package.
// Returns the derived context and a cleanup function to defer.
func trace(ctx context.Context, op string, attrs ...attribute.KeyValue) (context.Context, func(*error)) {
	ctx, span := tracer.Start(ctx, scope+"."+op,
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(attrs...),
	)
	return ctx, func(errp *error) {
		if errp != nil && *errp != nil {
			span.RecordError(*errp)
			span.SetStatus(codes.Error, (*errp).Error())
		}
		span.End()
	}
}

// tracedModel wraps a Model to emit a span per Chat call.
type tracedModel struct {
	inner    Model
	provider string
	model    string
}

func instrument(inner Model, provider, model string) Model {
	return &tracedModel{inner: inner, provider: provider, model: model}
}

func (t *tracedModel) Chat(ctx context.Context, req ChatRequest) (resp *ChatResponse, err error) {
	ctx, end := trace(ctx, "chat",
		attribute.String("llm.provider", t.provider),
		attribute.String("llm.model", t.model),
	)
	defer end(&err)

	resp, err = t.inner.Chat(ctx, req)
	if err == nil && resp != nil {
		otrace.SpanFromContext(ctx).SetAttributes(
			attribute.Int("llm.tokens.input", resp.InputTokens),
			attribute.Int("llm.tokens.output", resp.OutputTokens),
		)
	}
	return resp, err
}
