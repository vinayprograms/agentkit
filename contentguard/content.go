// Package contentguard provides prompt injection defense through tracked content
// and staged verification.
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
