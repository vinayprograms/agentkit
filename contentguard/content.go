// Package contentguard provides prompt injection defense through tracked content
// and staged verification.
//
// # Telemetry
//
// Each Finding carries a Latency for the stage that produced it (populated by
// the LLM-backed screener and reviewer stages). Result.Related lists the
// untrusted content blocks that were in scope for a checked call, so consumers
// can propagate taint into the resulting tool-result block.
//
// # User trust
//
// The legacy UserTrust verification knob is intentionally not carried over.
// Trust here is origin-based (see Trust) and fixed per content block; there is
// no global per-user trust dial. Consumers that need user-level gating should
// implement it at the call site (e.g. by choosing which stages to run, or by
// adjusting the Config.Context passed to stages).
package contentguard

import (
	"crypto/sha256"
	"encoding/hex"
)

// Trust represents the origin-based authenticity of content.
type Trust string

const (
	// Trusted is for framework-generated content (system prompt, supervisor messages).
	Trusted Trust = "trusted"
	// Vetted is for human-authored content (Agentfile goals, signed packages).
	Vetted Trust = "vetted"
	// Untrusted is for external content (tool results, file reads, web fetches).
	Untrusted Trust = "untrusted"
)

// Kind represents how content should be interpreted.
type Kind string

const (
	// Instruction means content contains executable instructions.
	Instruction Kind = "instruction"
	// Data means content is data only, never to be interpreted as instructions.
	Data Kind = "data"
)

// Content represents a piece of tracked content with security metadata.
type Content struct {
	ID      string
	Trust   Trust
	Kind    Kind
	Mutable bool
	Text    string
	Source  string
	Origins []*Content // parent content that influenced this
}

// computeHash returns SHA256 hash of text as hex string.
func computeHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// newContent creates tracked content with the given properties.
// It enforces security invariants:
// - Untrusted content is always kind=Data
// - Untrusted content is always mutable=true
func newContent(id string, trust Trust, kind Kind, mutable bool, text, source string) *Content {
	if trust == Untrusted {
		kind = Data
		mutable = true
	}

	return &Content{
		ID:      id,
		Trust:   trust,
		Kind:    kind,
		Mutable: mutable,
		Text:    text,
		Source:  source,
	}
}
