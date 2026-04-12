package embedding

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
)

const scope = "embedding"

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/embedding")

// trace starts a client span scoped to the embedding package.
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

// tracedEmbedder wraps an Embedder to emit a span per Embed call.
type tracedEmbedder struct {
	inner    Embedder
	provider string
	model    string
}

func instrument(inner Embedder, provider, model string) Embedder {
	if inner == nil {
		return nil
	}
	return &tracedEmbedder{inner: inner, provider: provider, model: model}
}

func (t *tracedEmbedder) Embed(ctx context.Context, text string) (vec []float64, err error) {
	ctx, end := trace(ctx, "embed",
		attribute.String("embedding.provider", t.provider),
		attribute.String("embedding.model", t.model),
	)
	defer end(&err)

	vec, err = t.inner.Embed(ctx, text)
	if err == nil {
		otrace.SpanFromContext(ctx).SetAttributes(attribute.Int("embedding.dimensions", len(vec)))
	}
	return vec, err
}
