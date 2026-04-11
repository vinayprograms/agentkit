package acp

// Info identifies an agent or host implementation.
type Info struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// Meta carries extensibility data on protocol types.
// Reserved keys: "traceparent", "tracestate", "baggage" (W3C trace context).
type Meta map[string]any
