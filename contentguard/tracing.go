package contentguard

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
)

const scope = "contentguard"

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/contentguard")

// trace starts an internal span scoped to the contentguard package.
func trace(ctx context.Context, op string, attrs ...attribute.KeyValue) (context.Context, func(*error)) {
	ctx, span := tracer.Start(ctx, scope+"."+op,
		otrace.WithSpanKind(otrace.SpanKindInternal),
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
