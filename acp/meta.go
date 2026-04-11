package acp

// Meta carries extensibility data on protocol types.
// Reserved keys: "traceparent", "tracestate", "baggage" (W3C trace context).
type Meta map[string]any
