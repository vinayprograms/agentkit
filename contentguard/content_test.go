package contentguard

import (
	"testing"
)

// trustRank returns a numeric rank for trust level comparison.
func trustRank(t Trust) int {
	switch t {
	case Trusted:
		return 3
	case Vetted:
		return 2
	case Untrusted:
		return 1
	default:
		return 0
	}
}

// canOverride returns true if content a can override content b.
func canOverride(a, b *Content) bool {
	if !b.Mutable {
		return false
	}
	return trustRank(a.Trust) > trustRank(b.Trust)
}

// propagatedTrust returns the lowest trust of two levels.
func propagatedTrust(a, b Trust) Trust {
	if trustRank(a) < trustRank(b) {
		return a
	}
	return b
}

func TestNewContent_EnforcesInvariants(t *testing.T) {
	tests := []struct {
		name        string
		trust       Trust
		kind        Kind
		mutable     bool
		wantKind    Kind
		wantMutable bool
	}{
		{
			name:        "trusted instruction immutable - unchanged",
			trust:       Trusted,
			kind:        Instruction,
			mutable:     false,
			wantKind:    Instruction,
			wantMutable: false,
		},
		{
			name:        "vetted instruction mutable - unchanged",
			trust:       Vetted,
			kind:        Instruction,
			mutable:     true,
			wantKind:    Instruction,
			wantMutable: true,
		},
		{
			name:        "untrusted instruction - forced to data",
			trust:       Untrusted,
			kind:        Instruction,
			mutable:     false,
			wantKind:    Data,
			wantMutable: true,
		},
		{
			name:        "untrusted immutable - forced to mutable",
			trust:       Untrusted,
			kind:        Data,
			mutable:     false,
			wantKind:    Data,
			wantMutable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newContent("test", tt.trust, tt.kind, tt.mutable, "content", "test")

			if c.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", c.Kind, tt.wantKind)
			}

			if c.Mutable != tt.wantMutable {
				t.Errorf("Mutable = %v, want %v", c.Mutable, tt.wantMutable)
			}
		})
	}
}

func TestCanOverride(t *testing.T) {
	immutable := newContent("sys", Trusted, Instruction, false, "system", "")
	mutableTrusted := newContent("commit", Trusted, Instruction, true, "commit", "")
	vetted := newContent("goal", Vetted, Instruction, true, "goal", "")
	untrusted := newContent("file", Untrusted, Data, true, "file content", "")

	tests := []struct {
		name string
		a    *Content
		b    *Content
		canA bool
	}{
		{"untrusted cannot override immutable", untrusted, immutable, false},
		{"untrusted cannot override mutable trusted", untrusted, mutableTrusted, false},
		{"untrusted cannot override vetted", untrusted, vetted, false},
		{"vetted cannot override immutable", vetted, immutable, false},
		{"trusted can override mutable vetted", mutableTrusted, vetted, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canOverride(tt.a, tt.b); got != tt.canA {
				t.Errorf("canOverride(a, b) = %v, want %v", got, tt.canA)
			}
		})
	}
}

func TestPropagatedTrust(t *testing.T) {
	tests := []struct {
		a, b Trust
		want Trust
	}{
		{Trusted, Trusted, Trusted},
		{Trusted, Vetted, Vetted},
		{Trusted, Untrusted, Untrusted},
		{Vetted, Untrusted, Untrusted},
		{Vetted, Vetted, Vetted},
	}

	for _, tt := range tests {
		t.Run(string(tt.a)+"+"+string(tt.b), func(t *testing.T) {
			if got := propagatedTrust(tt.a, tt.b); got != tt.want {
				t.Errorf("propagatedTrust(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestContent_Kind(t *testing.T) {
	instr := newContent("t1", Trusted, Instruction, false, "do this", "system")
	if instr.Kind != Instruction {
		t.Errorf("expected Instruction, got %v", instr.Kind)
	}

	data := newContent("t2", Untrusted, Data, true, "content", "web_fetch")
	if data.Kind != Data {
		t.Errorf("expected Data, got %v", data.Kind)
	}
}
