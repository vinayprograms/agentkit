package agent

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
)

const scope = "acp.agent"

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/acp/agent")

// trace starts a server span scoped to acp.agent. The agent handles
// incoming requests from the host, so all spans are server kind.
func trace(ctx context.Context, op string, attrs ...attribute.KeyValue) (context.Context, func(*error)) {
	ctx, span := tracer.Start(ctx, scope+"."+op,
		otrace.WithSpanKind(otrace.SpanKindServer),
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
