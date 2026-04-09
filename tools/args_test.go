package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

func TestValidate_StringParam(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]Param
		raw     map[string]any
		wantErr string
	}{
		{
			name:   "valid string",
			params: map[string]Param{"name": {Type: StringParam, Required: true}},
			raw:    map[string]any{"name": "alice"},
		},
		{
			name:    "required missing",
			params:  map[string]Param{"name": {Type: StringParam, Required: true}},
			raw:     map[string]any{},
			wantErr: "is required",
		},
		{
			name:    "required nil value",
			params:  map[string]Param{"name": {Type: StringParam, Required: true}},
			raw:     map[string]any{"name": nil},
			wantErr: "is required",
		},
		{
			name:    "wrong type",
			params:  map[string]Param{"name": {Type: StringParam, Required: true}},
			raw:     map[string]any{"name": 123},
			wantErr: "must be a string",
		},
		{
			name:   "optional missing is ok",
			params: map[string]Param{"name": {Type: StringParam, Required: false}},
			raw:    map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate(tt.params, tt.raw)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidate_IntParam(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		wantErr string
	}{
		{"int value", map[string]any{"n": 42}, ""},
		{"float64 value", map[string]any{"n": float64(3.14)}, ""},
		{"json.Number value", map[string]any{"n": json.Number("7")}, ""},
		{"wrong type string", map[string]any{"n": "abc"}, "must be a number"},
		{"wrong type bool", map[string]any{"n": true}, "must be a number"},
	}

	params := map[string]Param{"n": {Type: IntParam, Required: true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate(params, tt.raw)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q missing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidate_BoolParam(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		wantErr string
	}{
		{"true", map[string]any{"flag": true}, ""},
		{"false", map[string]any{"flag": false}, ""},
		{"wrong type", map[string]any{"flag": "yes"}, "must be a boolean"},
	}

	params := map[string]Param{"flag": {Type: BoolParam, Required: true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate(params, tt.raw)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q missing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidate_ArrayParam(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		wantErr string
	}{
		{"[]string", map[string]any{"items": []string{"a", "b"}}, ""},
		{"[]any", map[string]any{"items": []any{"a", "b"}}, ""},
		{"wrong type string", map[string]any{"items": "not-array"}, "must be an array"},
		{"wrong type int", map[string]any{"items": 42}, "must be an array"},
	}

	params := map[string]Param{"items": {Type: ArrayParam, Required: true}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate(params, tt.raw)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q missing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidate_NilParams(t *testing.T) {
	// nil params map means no validation constraints
	args, err := Validate(nil, map[string]any{"anything": "goes"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !args.Has("anything") {
		t.Error("expected raw values to be accessible")
	}
}

func TestValidate_NilRaw(t *testing.T) {
	// optional param with nil raw should pass
	_, err := Validate(map[string]Param{"x": {Type: StringParam}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_RequiredNilRaw(t *testing.T) {
	_, err := Validate(map[string]Param{"x": {Type: StringParam, Required: true}}, nil)
	if err == nil {
		t.Fatal("expected error for required param with nil raw")
	}
}

// ---------------------------------------------------------------------------
// String / StringOr
// ---------------------------------------------------------------------------

func TestArgs_String(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]any
		key     string
		want    string
		wantErr string
	}{
		{"present", map[string]any{"k": "hello"}, "k", "hello", ""},
		{"missing", map[string]any{}, "k", "", "is required"},
		{"wrong type", map[string]any{"k": 42}, "k", "", "must be a string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			got, err := a.String(tt.key)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArgs_StringOr(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
		key    string
		def    string
		want   string
	}{
		{"present", map[string]any{"k": "val"}, "k", "def", "val"},
		{"missing", map[string]any{}, "k", "def", "def"},
		{"wrong type returns default", map[string]any{"k": 123}, "k", "def", "def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			if got := a.StringOr(tt.key, tt.def); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Int / IntOr
// ---------------------------------------------------------------------------

func TestArgs_Int(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]any
		key     string
		want    int
		wantErr string
	}{
		{"int value", map[string]any{"n": 42}, "n", 42, ""},
		{"float64 value", map[string]any{"n": float64(10)}, "n", 10, ""},
		{"json.Number", map[string]any{"n": json.Number("99")}, "n", 99, ""},
		{"missing", map[string]any{}, "n", 0, "is required"},
		{"wrong type", map[string]any{"n": "text"}, "n", 0, "must be a number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			got, err := a.Int(tt.key)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestArgs_IntOr(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
		key    string
		def    int
		want   int
	}{
		{"int present", map[string]any{"n": 5}, "n", 0, 5},
		{"float64 present", map[string]any{"n": float64(7)}, "n", 0, 7},
		{"json.Number present", map[string]any{"n": json.Number("11")}, "n", 0, 11},
		{"missing", map[string]any{}, "n", 42, 42},
		{"wrong type returns default", map[string]any{"n": "abc"}, "n", 42, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			if got := a.IntOr(tt.key, tt.def); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Float / FloatOr
// ---------------------------------------------------------------------------

func TestArgs_Float(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]any
		key     string
		want    float64
		wantErr string
	}{
		{"float64 value", map[string]any{"f": 3.14}, "f", 3.14, ""},
		{"int value", map[string]any{"f": 10}, "f", 10.0, ""},
		{"json.Number", map[string]any{"f": json.Number("2.718")}, "f", 2.718, ""},
		{"missing", map[string]any{}, "f", 0, "is required"},
		{"wrong type", map[string]any{"f": "text"}, "f", 0, "must be a number"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			got, err := a.Float(tt.key)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

func TestArgs_FloatOr(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
		key    string
		def    float64
		want   float64
	}{
		{"float64 present", map[string]any{"f": 1.5}, "f", 0, 1.5},
		{"int present", map[string]any{"f": 3}, "f", 0, 3.0},
		{"json.Number present", map[string]any{"f": json.Number("9.9")}, "f", 0, 9.9},
		{"missing", map[string]any{}, "f", 7.7, 7.7},
		{"wrong type returns default", map[string]any{"f": true}, "f", 7.7, 7.7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			if got := a.FloatOr(tt.key, tt.def); got != tt.want {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Bool / BoolOr
// ---------------------------------------------------------------------------

func TestArgs_Bool(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]any
		key     string
		want    bool
		wantErr string
	}{
		{"true", map[string]any{"b": true}, "b", true, ""},
		{"false", map[string]any{"b": false}, "b", false, ""},
		{"missing", map[string]any{}, "b", false, "is required"},
		{"wrong type", map[string]any{"b": "yes"}, "b", false, "must be a boolean"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			got, err := a.Bool(tt.key)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArgs_BoolOr(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
		key    string
		def    bool
		want   bool
	}{
		{"present true", map[string]any{"b": true}, "b", false, true},
		{"present false", map[string]any{"b": false}, "b", true, false},
		{"missing returns default true", map[string]any{}, "b", true, true},
		{"missing returns default false", map[string]any{}, "b", false, false},
		{"wrong type returns default", map[string]any{"b": 1}, "b", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			if got := a.BoolOr(tt.key, tt.def); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// StringSlice / StringSliceOr
// ---------------------------------------------------------------------------

func TestArgs_StringSlice(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]any
		key     string
		want    []string
		wantErr string
	}{
		{
			name:   "[]string",
			values: map[string]any{"s": []string{"a", "b"}},
			key:    "s",
			want:   []string{"a", "b"},
		},
		{
			name:   "[]any with strings",
			values: map[string]any{"s": []any{"x", "y", "z"}},
			key:    "s",
			want:   []string{"x", "y", "z"},
		},
		{
			name:    "[]any with non-string element",
			values:  map[string]any{"s": []any{"a", 42}},
			key:     "s",
			wantErr: "must be a string",
		},
		{
			name:    "missing",
			values:  map[string]any{},
			key:     "s",
			wantErr: "is required",
		},
		{
			name:    "wrong type entirely",
			values:  map[string]any{"s": "not-a-slice"},
			key:     "s",
			wantErr: "must be an array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			got, err := a.StringSlice(tt.key)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got len %d, want len %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestArgs_StringSliceOr(t *testing.T) {
	def := []string{"default"}
	tests := []struct {
		name   string
		values map[string]any
		key    string
		def    []string
		want   []string
	}{
		{"present []string", map[string]any{"s": []string{"a"}}, "s", def, []string{"a"}},
		{"present []any", map[string]any{"s": []any{"b"}}, "s", def, []string{"b"}},
		{"missing", map[string]any{}, "s", def, def},
		{"bad element returns default", map[string]any{"s": []any{1}}, "s", def, def},
		{"wrong type returns default", map[string]any{"s": 99}, "s", def, def},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{values: tt.values}
			got := a.StringSliceOr(tt.key, tt.def)
			if len(got) != len(tt.want) {
				t.Fatalf("got len %d, want len %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Has
// ---------------------------------------------------------------------------

func TestArgs_Has(t *testing.T) {
	a := Args{values: map[string]any{"present": "val"}}
	if !a.Has("present") {
		t.Error("expected Has to return true for present key")
	}
	if a.Has("absent") {
		t.Error("expected Has to return false for absent key")
	}
}

// ---------------------------------------------------------------------------
// toStringSlice (white-box)
// ---------------------------------------------------------------------------

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    []string
		wantErr string
	}{
		{"[]string", []string{"a", "b"}, []string{"a", "b"}, ""},
		{"[]any strings", []any{"c", "d"}, []string{"c", "d"}, ""},
		{"[]any mixed", []any{"a", 1}, nil, "must be a string"},
		{"int", 42, nil, "must be an array"},
		{"empty []string", []string{}, []string{}, ""},
		{"empty []any", []any{}, []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toStringSlice(tt.input, "key")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got len %d, want len %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
