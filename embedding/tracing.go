package embedding

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/embedding")

type tracedEmbedder struct {
	inner    Embedder
	provider string
	model    string
}

func withTracing(inner Embedder, provider, model string) Embedder {
	if inner == nil {
		return nil
	}
	return &tracedEmbedder{inner: inner, provider: provider, model: model}
}

func (t *tracedEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	ctx, span := tracer.Start(ctx, "embedding.embed",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("embedding.provider", t.provider),
			attribute.String("embedding.model", t.model),
		),
	)
	defer span.End()

	vec, err := t.inner.Embed(ctx, text)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return vec, err
	}
	span.SetAttributes(attribute.Int("embedding.dimensions", len(vec)))
	return vec, nil
}
