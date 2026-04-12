package rpc

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
)

const scope = "rpc"

// Span kinds exposed to call sites — avoids leaking OTel vocabulary.
const (
	client = otrace.SpanKindClient
	server = otrace.SpanKindServer
)

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/acp/internal/rpc")

// event records a point-in-time marker on the current span.
// Use for decision points and state transitions that don't warrant a full span.
func event(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	otrace.SpanFromContext(ctx).AddEvent(name, otrace.WithAttributes(attrs...))
}

// trace starts a span scoped to the rpc package. The kind argument chooses
// client (outgoing call) or server (incoming request) span kind.
func trace(ctx context.Context, kind otrace.SpanKind, op string, attrs ...attribute.KeyValue) (context.Context, func(*error)) {
	ctx, span := tracer.Start(ctx, scope+"."+op,
		otrace.WithSpanKind(kind),
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
