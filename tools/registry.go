package tools

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
)

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
func (r *Registry) Execute(ctx context.Context, name string, rawArgs map[string]any) (result string, err error) {
	ctx, end := trace(ctx, "execute", attribute.String("tool.name", name))
	defer end(&err)

	r.mu.RLock()
	entry, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}

	args, verr := Validate(entry.tool.Parameters(), rawArgs)
	if verr != nil {
		err = fmt.Errorf("tool %s: %w", name, verr)
		return "", err
	}

	result, err = entry.tool.Execute(ctx, args)
	return result, err
}

// Subset returns a new Registry containing only the entries named in names.
// Every name must be present; an unknown name returns an error and a nil
// registry. The returned registry shares Entry pointers with the receiver —
// entries are immutable after registration so sharing is safe.
func (r *Registry) Subset(names []string) (*Registry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sub := &Registry{tools: make(map[string]*Entry, len(names))}
	for _, name := range names {
		entry, ok := r.tools[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
		sub.tools[name] = entry
	}
	return sub, nil
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
