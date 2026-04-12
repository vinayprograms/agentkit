package memory

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/memory")

// startSpan starts an internal span for a memory store op and returns the
// new ctx and an end func that records err status before ending the span.
func startSpan(ctx context.Context, name, store string, attrs ...attribute.KeyValue) (context.Context, func(*error)) {
	all := make([]attribute.KeyValue, 0, len(attrs)+1)
	all = append(all, attribute.String("memory.store", store))
	all = append(all, attrs...)
	ctx, span := tracer.Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(all...),
	)
	return ctx, func(errp *error) {
		if errp != nil && *errp != nil {
			span.RecordError(*errp)
			span.SetStatus(codes.Error, (*errp).Error())
		}
		span.End()
	}
}
