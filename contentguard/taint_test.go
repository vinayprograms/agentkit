package contentguard

import (
	"testing"
)

func TestNewBlock_EnforcesInvariants(t *testing.T) {
	tests := []struct {
		name         string
		trust        TrustLevel
		typ          ContentKind
		mutable      bool
		wantType     ContentKind
		wantMutable  bool
	}{
		{
			name:        "trusted instruction immutable - unchanged",
			trust:       Trusted,
			typ:         Instruction,
			mutable:     false,
			wantType:    Instruction,
			wantMutable: false,
		},
		{
			name:        "vetted instruction mutable - unchanged",
			trust:       Vetted,
			typ:         Instruction,
			mutable:     true,
			wantType:    Instruction,
			wantMutable: true,
		},
		{
			name:        "untrusted instruction - forced to data",
			trust:       Untrusted,
			typ:         Instruction,
			mutable:     false,
			wantType:    Data,
			wantMutable: true,
		},
		{
			name:        "untrusted immutable - forced to mutable",
			trust:       Untrusted,
			typ:         Data,
			mutable:     false,
			wantType:    Data,
			wantMutable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taint := newTaint("test", tt.trust, tt.typ, tt.mutable, "content", "test")

			if taint.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", taint.Type, tt.wantType)
			}

			if taint.Mutable != tt.wantMutable {
				t.Errorf("Mutable = %v, want %v", taint.Mutable, tt.wantMutable)
			}
		})
	}
}

func TestBlock_CanOverride(t *testing.T) {
	immutable := newTaint("sys", Trusted, Instruction, false, "system", "")
	mutableTrusted := newTaint("commit", Trusted, Instruction, true, "commit", "")
	vetted := newTaint("goal", Vetted, Instruction, true, "goal", "")
	untrusted := newTaint("file", Untrusted, Data, true, "file content", "")

	tests := []struct {
		name   string
		a      *Taint
		b      *Taint
		canA   bool
	}{
		{"untrusted cannot override immutable", untrusted, immutable, false},
		{"untrusted cannot override mutable trusted", untrusted, mutableTrusted, false},
		{"untrusted cannot override vetted", untrusted, vetted, false},
		{"vetted cannot override immutable", vetted, immutable, false},
		{"trusted can override mutable vetted", mutableTrusted, vetted, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.CanOverride(tt.b); got != tt.canA {
				t.Errorf("a.CanOverride(b) = %v, want %v", got, tt.canA)
			}
		})
	}
}

func TestPropagatedTrust(t *testing.T) {
	tests := []struct {
		a, b TrustLevel
		want TrustLevel
	}{
		{Trusted, Trusted, Trusted},
		{Trusted, Vetted, Vetted},
		{Trusted, Untrusted, Untrusted},
		{Vetted, Untrusted, Untrusted},
		{Vetted, Vetted, Vetted},
	}

	for _, tt := range tests {
		t.Run(string(tt.a)+"+"+string(tt.b), func(t *testing.T) {
			if got := PropagatedTrust(tt.a, tt.b); got != tt.want {
				t.Errorf("PropagatedTrust(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
