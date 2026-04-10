package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Definition.JSONSchema
// ---------------------------------------------------------------------------

func TestDefinition_JSONSchema_EmptyParams(t *testing.T) {
	d := Definition{
		Name:       "test",
		Parameters: nil,
	}
	schema := d.JSONSchema()

	if schema["type"] != "object" {
		t.Errorf("expected type=object, got %v", schema["type"])
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to be map[string]any, got %T", schema["properties"])
	}
	if len(props) != 0 {
		t.Errorf("expected empty properties, got %d", len(props))
	}
	if _, exists := schema["required"]; exists {
		t.Error("expected no required key for empty params")
	}
}

func TestDefinition_JSONSchema_EmptyParamsMap(t *testing.T) {
	d := Definition{
		Name:       "test",
		Parameters: map[string]Param{},
	}
	schema := d.JSONSchema()
	props := schema["properties"].(map[string]any)
	if len(props) != 0 {
		t.Errorf("expected empty properties, got %d", len(props))
	}
}

func TestDefinition_JSONSchema_RequiredAndOptional(t *testing.T) {
	d := Definition{
		Name: "test",
		Parameters: map[string]Param{
			"name": {Type: StringParam, Description: "user name", Required: true},
			"age":  {Type: IntParam, Description: "user age", Required: false},
		},
	}
	schema := d.JSONSchema()

	props := schema["properties"].(map[string]any)
	if len(props) != 2 {
		t.Fatalf("expected 2 properties, got %d", len(props))
	}

	nameProp := props["name"].(map[string]any)
	if nameProp["type"] != "string" {
		t.Errorf("name type: got %v, want string", nameProp["type"])
	}
	if nameProp["description"] != "user name" {
		t.Errorf("name description: got %v", nameProp["description"])
	}

	ageProp := props["age"].(map[string]any)
	if ageProp["type"] != "integer" {
		t.Errorf("age type: got %v, want integer", ageProp["type"])
	}

	req, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("expected required to be []string, got %T", schema["required"])
	}
	if len(req) != 1 || req[0] != "name" {
		t.Errorf("expected required=[name], got %v", req)
	}
}

func TestDefinition_JSONSchema_WithEnum(t *testing.T) {
	d := Definition{
		Name: "test",
		Parameters: map[string]Param{
			"color": {
				Type:        StringParam,
				Description: "pick a color",
				Enum:        []string{"red", "green", "blue"},
				Required:    true,
			},
		},
	}
	schema := d.JSONSchema()

	props := schema["properties"].(map[string]any)
	colorProp := props["color"].(map[string]any)
	enumVal, ok := colorProp["enum"].([]string)
	if !ok {
		t.Fatalf("expected enum to be []string, got %T", colorProp["enum"])
	}
	if len(enumVal) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(enumVal))
	}
}

func TestDefinition_JSONSchema_NoEnumWhenEmpty(t *testing.T) {
	d := Definition{
		Name: "test",
		Parameters: map[string]Param{
			"x": {Type: StringParam, Description: "desc"},
		},
	}
	schema := d.JSONSchema()
	props := schema["properties"].(map[string]any)
	xProp := props["x"].(map[string]any)
	if _, exists := xProp["enum"]; exists {
		t.Error("expected no enum key when Enum is nil")
	}
}

func TestDefinition_JSONSchema_MixedTypes(t *testing.T) {
	d := Definition{
		Name: "mixed",
		Parameters: map[string]Param{
			"query":   {Type: StringParam, Description: "search query", Required: true},
			"limit":   {Type: IntParam, Description: "max results", Required: false},
			"verbose": {Type: BoolParam, Description: "verbose output", Required: true},
			"tags":    {Type: ArrayParam, Description: "tag list", Required: false},
		},
	}
	schema := d.JSONSchema()

	props := schema["properties"].(map[string]any)
	if len(props) != 4 {
		t.Fatalf("expected 4 properties, got %d", len(props))
	}

	// Check types
	expectedTypes := map[string]string{
		"query": "string", "limit": "integer", "verbose": "boolean", "tags": "array",
	}
	for name, wantType := range expectedTypes {
		p := props[name].(map[string]any)
		if p["type"] != wantType {
			t.Errorf("%s type: got %v, want %s", name, p["type"], wantType)
		}
	}

	// Check required
	req := schema["required"].([]string)
	sort.Strings(req)
	if len(req) != 2 {
		t.Fatalf("expected 2 required, got %d", len(req))
	}
	if req[0] != "query" && req[1] != "query" {
		t.Errorf("expected query in required, got %v", req)
	}
}

// ---------------------------------------------------------------------------
// guardedTool
// ---------------------------------------------------------------------------

// fakeTool is a minimal Tool for testing guards.
type fakeTool struct {
	executed bool
}

func (f *fakeTool) Name() string                 { return "fake" }
func (f *fakeTool) Description() string          { return "a fake tool" }
func (f *fakeTool) Parameters() map[string]Param { return nil }
func (f *fakeTool) Execute(ctx context.Context, args Args) (string, error) {
	f.executed = true
	return "executed", nil
}

// allowGuard always allows execution.
type allowGuard struct{}

func (g *allowGuard) Check(ctx context.Context, args Args) error { return nil }

// denyGuard always blocks execution.
type denyGuard struct{ msg string }

func (g *denyGuard) Check(ctx context.Context, args Args) error {
	return fmt.Errorf("%s", g.msg)
}

// trackGuard records whether it was called.
type trackGuard struct {
	called bool
	allow  bool
}

func (g *trackGuard) Check(ctx context.Context, args Args) error {
	g.called = true
	if !g.allow {
		return fmt.Errorf("denied")
	}
	return nil
}

func TestGuardedTool_AllowsExecution(t *testing.T) {
	inner := &fakeTool{}
	gt := &guardedTool{inner: inner, guard: &allowGuard{}}

	result, err := gt.Execute(context.Background(), Args{values: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "executed" {
		t.Errorf("got %q, want %q", result, "executed")
	}
	if !inner.executed {
		t.Error("expected inner tool to be executed")
	}
}

func TestGuardedTool_BlocksExecution(t *testing.T) {
	inner := &fakeTool{}
	gt := &guardedTool{inner: inner, guard: &denyGuard{msg: "forbidden"}}

	_, err := gt.Execute(context.Background(), Args{values: map[string]any{}})
	if err == nil {
		t.Fatal("expected error from guard")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("error %q does not contain 'forbidden'", err)
	}
	if inner.executed {
		t.Error("inner tool should NOT have been executed when guard blocks")
	}
}

func TestGuardedTool_DelegatesMetadata(t *testing.T) {
	inner := &fakeTool{}
	gt := &guardedTool{inner: inner, guard: &allowGuard{}}

	if gt.Name() != "fake" {
		t.Errorf("Name: got %q, want %q", gt.Name(), "fake")
	}
	if gt.Description() != "a fake tool" {
		t.Errorf("Description: got %q", gt.Description())
	}
	if gt.Parameters() != nil {
		t.Errorf("Parameters: expected nil, got %v", gt.Parameters())
	}
}

func TestGuardedTool_MultipleGuardsChained(t *testing.T) {
	inner := &fakeTool{}

	g1 := &trackGuard{allow: true}
	g2 := &trackGuard{allow: true}

	// Chain: wrap inner with g1, then wrap that with g2.
	// With() nesting: g2.Check runs first (outer), then g1.Check (inner).
	wrapped := &guardedTool{inner: &guardedTool{inner: inner, guard: g1}, guard: g2}

	result, err := wrapped.Execute(context.Background(), Args{values: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "executed" {
		t.Errorf("got %q, want %q", result, "executed")
	}
	if !g1.called {
		t.Error("expected inner guard to be called")
	}
	if !g2.called {
		t.Error("expected outer guard to be called")
	}
}

func TestGuardedTool_ChainedFirstGuardBlocks(t *testing.T) {
	inner := &fakeTool{}

	outerGuard := &denyGuard{msg: "outer blocked"}
	innerGuard := &trackGuard{allow: true}

	wrapped := &guardedTool{inner: &guardedTool{inner: inner, guard: innerGuard}, guard: outerGuard}

	_, err := wrapped.Execute(context.Background(), Args{values: map[string]any{}})
	if err == nil {
		t.Fatal("expected error from outer guard")
	}
	if !strings.Contains(err.Error(), "outer blocked") {
		t.Errorf("expected 'outer blocked' error, got %v", err)
	}
	// Inner guard should NOT have been reached
	if innerGuard.called {
		t.Error("inner guard should not be called when outer guard blocks")
	}
	if inner.executed {
		t.Error("inner tool should not be executed when outer guard blocks")
	}
}

func TestGuardedTool_ChainedSecondGuardBlocks(t *testing.T) {
	inner := &fakeTool{}

	outerGuard := &allowGuard{}
	innerGuard := &denyGuard{msg: "inner blocked"}

	wrapped := &guardedTool{inner: &guardedTool{inner: inner, guard: innerGuard}, guard: outerGuard}

	_, err := wrapped.Execute(context.Background(), Args{values: map[string]any{}})
	if err == nil {
		t.Fatal("expected error from inner guard")
	}
	if !strings.Contains(err.Error(), "inner blocked") {
		t.Errorf("expected 'inner blocked' error, got %v", err)
	}
	if inner.executed {
		t.Error("inner tool should not be executed when inner guard blocks")
	}
}

// TestAllTools_NameAndDescription registers all available tools and verifies
// every one has a non-empty Name() and Description().
func TestAllTools_NameAndDescription(t *testing.T) {
	dir := t.TempDir()
	store := NewInMemoryStore()
	spawner := func(ctx context.Context, role, task string, outputs []string) (string, error) {
		return "ok", nil
	}

	allTools := []Tool{
		Bash(dir),
		Read(dir),
		Write(dir),
		Edit(dir),
		Glob(dir),
		Grep(dir),
		Ls(dir),
		Tree(dir),
		Head(dir),
		Tail(dir),
		Diff(),
		Pwd(),
		Hostname(),
		Whoami(),
		Env(),
		Which(),
		Sysinfo(),
		Datetime(),
		Mkdir(dir),
		Mv(dir),
		Cp(dir),
		Rm(dir),
		Git(dir),
		Patch(),
		Spawn(spawner),
		ScratchpadRead(store, false),
		ScratchpadWrite(store, false),
		ScratchpadList(store, false),
		ScratchpadSearch(store, false),
		ScratchpadRead(store, true),
		ScratchpadWrite(store, true),
		ScratchpadList(store, true),
		ScratchpadSearch(store, true),
		Search(nil),
		Fetch(nil),
		Remember(newMockSemanticMemory()),
		Recall(newMockSemanticMemory()),
	}

	reg := NewRegistry()
	for _, tool := range allTools {
		err := reg.Register(New(tool))
		if err != nil {
			// Some tools may have duplicate names (persistent vs ephemeral scratchpad)
			// so skip duplicates
			continue
		}
	}

	defs := reg.Definitions()
	if len(defs) == 0 {
		t.Fatal("expected at least one tool definition")
	}

	for _, d := range defs {
		if d.Name == "" {
			t.Error("found tool with empty Name()")
		}
		if d.Description == "" {
			t.Errorf("tool %q has empty Description()", d.Name)
		}
	}

	// Also verify that each original tool's Name() and Description() are non-empty
	for _, tool := range allTools {
		if tool.Name() == "" {
			t.Error("found tool with empty Name()")
		}
		if tool.Description() == "" {
			t.Errorf("tool %q has empty Description()", tool.Name())
		}
	}
}
