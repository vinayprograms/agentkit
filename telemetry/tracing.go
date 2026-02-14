// OpenTelemetry tracing support for distributed agent observability.
package telemetry

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps OpenTelemetry tracing with agent-specific helpers.
type Tracer struct {
	tracer trace.Tracer
	debug  bool // When true, include content in span attributes
}

var (
	globalTracer *Tracer
	tracerMu     sync.RWMutex
)

// SetGlobalTracer sets the global tracer instance.
func SetGlobalTracer(t *Tracer) {
	tracerMu.Lock()
	defer tracerMu.Unlock()
	globalTracer = t
}

// GetTracer returns the global tracer, or a no-op tracer if not set.
func GetTracer() *Tracer {
	tracerMu.RLock()
	defer tracerMu.RUnlock()
	if globalTracer == nil {
		return &Tracer{tracer: trace.NewNoopTracerProvider().Tracer("")}
	}
	return globalTracer
}

// NewTracer creates a new tracer with the given name.
func NewTracer(name string, debug bool) *Tracer {
	return &Tracer{
		tracer: otel.Tracer(name),
		debug:  debug,
	}
}

// SetDebug enables or disables debug mode (content in spans).
func (t *Tracer) SetDebug(debug bool) {
	t.debug = debug
}

// Debug returns whether debug mode is enabled.
func (t *Tracer) Debug() bool {
	return t.debug
}

// StartSpan starts a new span with the given name.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// --- LLM Spans ---

// LLMSpanOptions contains options for LLM call spans.
type LLMSpanOptions struct {
	Model     string
	Provider  string
	TokensIn  int
	TokensOut int
	Prompt    string // Only included if debug=true
	Response  string // Only included if debug=true
	Thinking  string // Only included if debug=true
}

// StartLLMSpan starts a span for an LLM call.
func (t *Tracer) StartLLMSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient))
}

// EndLLMSpan ends an LLM span with attributes.
func (t *Tracer) EndLLMSpan(span trace.Span, opts LLMSpanOptions, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("llm.model", opts.Model),
		attribute.String("llm.provider", opts.Provider),
		attribute.Int("llm.tokens.input", opts.TokensIn),
		attribute.Int("llm.tokens.output", opts.TokensOut),
	}

	if t.debug {
		if opts.Prompt != "" {
			attrs = append(attrs, attribute.String("llm.prompt", truncate(opts.Prompt, 4000)))
		}
		if opts.Response != "" {
			attrs = append(attrs, attribute.String("llm.response", truncate(opts.Response, 4000)))
		}
		if opts.Thinking != "" {
			attrs = append(attrs, attribute.String("llm.thinking", truncate(opts.Thinking, 4000)))
		}
	}

	span.SetAttributes(attrs...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()
}

// --- Tool Spans ---

// ToolSpanOptions contains options for tool execution spans.
type ToolSpanOptions struct {
	Tool   string
	Args   map[string]interface{} // Always included (agent-controlled)
	Result string                 // Only included if debug=true
}

// StartToolSpan starts a span for a tool execution.
func (t *Tracer) StartToolSpan(ctx context.Context, toolName string) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "tool."+toolName, trace.WithSpanKind(trace.SpanKindInternal))
	span.SetAttributes(attribute.String("tool.name", toolName))
	return ctx, span
}

// EndToolSpan ends a tool span with attributes.
func (t *Tracer) EndToolSpan(span trace.Span, opts ToolSpanOptions, err error) {
	// Args are always logged (agent-controlled, not user data)
	for k, v := range opts.Args {
		span.SetAttributes(attribute.String("tool.arg."+k, truncateAny(v, 500)))
	}

	// Result only in debug mode (may contain user data)
	if t.debug && opts.Result != "" {
		span.SetAttributes(attribute.String("tool.result", truncate(opts.Result, 4000)))
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()
}

// --- MCP Spans ---

// MCPSpanOptions contains options for MCP tool call spans.
type MCPSpanOptions struct {
	Server string
	Tool   string
	Args   map[string]interface{}
	Result string // Only included if debug=true
}

// StartMCPSpan starts a span for an MCP tool call.
func (t *Tracer) StartMCPSpan(ctx context.Context, server, tool string) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, "mcp."+server+"."+tool, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(
		attribute.String("mcp.server", server),
		attribute.String("mcp.tool", tool),
	)
	return ctx, span
}

// EndMCPSpan ends an MCP span with attributes.
func (t *Tracer) EndMCPSpan(span trace.Span, opts MCPSpanOptions, err error) {
	for k, v := range opts.Args {
		span.SetAttributes(attribute.String("mcp.arg."+k, truncateAny(v, 500)))
	}

	if t.debug && opts.Result != "" {
		span.SetAttributes(attribute.String("mcp.result", truncate(opts.Result, 4000)))
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()
}

// --- Security Spans ---

// SecuritySpanOptions contains options for security check spans.
type SecuritySpanOptions struct {
	CheckPath string   // static, static→triage, static→triage→supervisor
	Verdict   string   // allow, deny, modify
	BlockID   string
	Flags     []string
	Prompt    string // Only in debug mode
	Response  string // Only in debug mode
}

// StartSecuritySpan starts a span for a security check.
func (t *Tracer) StartSecuritySpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "security."+name, trace.WithSpanKind(trace.SpanKindInternal))
}

// EndSecuritySpan ends a security span with attributes.
func (t *Tracer) EndSecuritySpan(span trace.Span, opts SecuritySpanOptions, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("security.check_path", opts.CheckPath),
		attribute.String("security.verdict", opts.Verdict),
	}

	if opts.BlockID != "" {
		attrs = append(attrs, attribute.String("security.block_id", opts.BlockID))
	}
	if len(opts.Flags) > 0 {
		attrs = append(attrs, attribute.StringSlice("security.flags", opts.Flags))
	}

	if t.debug {
		if opts.Prompt != "" {
			attrs = append(attrs, attribute.String("security.prompt", truncate(opts.Prompt, 2000)))
		}
		if opts.Response != "" {
			attrs = append(attrs, attribute.String("security.response", truncate(opts.Response, 2000)))
		}
	}

	span.SetAttributes(attrs...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()
}

// --- Policy Spans ---

// PolicySpanOptions contains options for policy check spans.
type PolicySpanOptions struct {
	Step    string // deterministic, llm
	Allowed bool
	Reason  string
	Command string // Only in debug mode
}

// StartPolicySpan starts a span for a policy check.
func (t *Tracer) StartPolicySpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, "policy."+name, trace.WithSpanKind(trace.SpanKindInternal))
}

// EndPolicySpan ends a policy span with attributes.
func (t *Tracer) EndPolicySpan(span trace.Span, opts PolicySpanOptions, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("policy.step", opts.Step),
		attribute.Bool("policy.allowed", opts.Allowed),
	}

	if opts.Reason != "" {
		attrs = append(attrs, attribute.String("policy.reason", opts.Reason))
	}

	if t.debug && opts.Command != "" {
		attrs = append(attrs, attribute.String("policy.command", truncate(opts.Command, 1000)))
	}

	span.SetAttributes(attrs...)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}

	span.End()
}

// --- Context Propagation ---

// InjectContext injects trace context into a carrier for cross-process propagation.
func InjectContext(ctx context.Context, carrier propagation.TextMapCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// ExtractContext extracts trace context from a carrier.
func ExtractContext(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// MapCarrier is a simple map-based TextMapCarrier for context propagation.
type MapCarrier map[string]string

func (c MapCarrier) Get(key string) string {
	return c[key]
}

func (c MapCarrier) Set(key, value string) {
	c[key] = value
}

func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// --- Helpers ---

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func truncateAny(v interface{}, maxLen int) string {
	switch val := v.(type) {
	case string:
		return truncate(val, maxLen)
	default:
		s := string(mustMarshal(v))
		return truncate(s, maxLen)
	}
}

func mustMarshal(v interface{}) []byte {
	// Simple JSON-like representation without importing encoding/json
	// to avoid circular deps. For complex types, just stringify.
	switch val := v.(type) {
	case string:
		return []byte(val)
	case []byte:
		return val
	case int, int64, float64, bool:
		return []byte(string(rune(val.(int))))
	default:
		return []byte("<complex>")
	}
}
