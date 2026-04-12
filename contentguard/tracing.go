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

// event records a point-in-time marker on the current span.
// Use for decision points and state transitions that don't warrant a full span.
func event(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	otrace.SpanFromContext(ctx).AddEvent(name, otrace.WithAttributes(attrs...))
}

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
