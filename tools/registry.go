package tools

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/vinayprograms/agentkit/tools")

// Entry holds a tool prepared for registration, optionally with guards.
type Entry struct {
	tool Tool
}

// New creates an Entry from a Tool, ready for registration.
func New(tool Tool) *Entry {
	return &Entry{tool: tool}
}

// With attaches a Guard to the entry. Multiple guards can be chained.
func (e *Entry) With(g Guard) *Entry {
	e.tool = &guardedTool{inner: e.tool, guard: g}
	return e
}

// guardedTool wraps a Tool with a Guard check before execution.
type guardedTool struct {
	inner Tool
	guard Guard
}

func (g *guardedTool) Name() string                 { return g.inner.Name() }
func (g *guardedTool) Description() string          { return g.inner.Description() }
func (g *guardedTool) Parameters() map[string]Param { return g.inner.Parameters() }

func (g *guardedTool) Execute(ctx context.Context, args Args) (string, error) {
	if err := g.guard.Check(ctx, args); err != nil {
		return "", err
	}
	return g.inner.Execute(ctx, args)
}

// Registry holds registered tools and dispatches execution.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Entry
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*Entry),
	}
}

// Register adds a tool entry to the registry.
func (r *Registry) Register(entry *Entry) error {
	if entry == nil || entry.tool == nil {
		return fmt.Errorf("nil entry")
	}
	name := entry.tool.Name()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}
	r.tools[name] = entry
	return nil
}

// Get returns the tool with the given name, or nil.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if entry, ok := r.tools[name]; ok {
		return entry.tool
	}
	return nil
}

// Has returns true if a tool with the given name is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// Execute validates args and runs the named tool.
func (r *Registry) Execute(ctx context.Context, name string, rawArgs map[string]any) (string, error) {
	ctx, span := tracer.Start(ctx, "tool.execute",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("tool.name", name)),
	)
	defer span.End()

	r.mu.RLock()
	entry, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		err := fmt.Errorf("unknown tool: %s", name)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	args, err := Validate(entry.tool.Parameters(), rawArgs)
	if err != nil {
		err = fmt.Errorf("tool %s: %w", name, err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}

	result, err := entry.tool.Execute(ctx, args)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

// Definitions returns LLM-facing definitions for all registered tools.
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]Definition, 0, len(r.tools))
	for _, entry := range r.tools {
		defs = append(defs, Definition{
			Name:        entry.tool.Name(),
			Description: entry.tool.Description(),
			Parameters:  entry.tool.Parameters(),
		})
	}
	return defs
}
