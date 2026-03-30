// Package security provides prompt injection defense through trust-tagged content taints,
// tiered verification, and cryptographic audit trails.
package contentguard

import (
	"crypto/sha256"
	"encoding/hex"
)

// TrustLevel represents the origin-based authenticity of content.
type TrustLevel string

const (
	// Trusted is for framework-generated content (system prompt, supervisor messages).
	Trusted TrustLevel = "trusted"
	// Vetted is for human-authored content (Agentfile goals, signed packages).
	Vetted TrustLevel = "vetted"
	// Untrusted is for external content (tool results, file reads, web fetches).
	Untrusted TrustLevel = "untrusted"
)

// ContentKind represents how content should be interpreted.
type ContentKind string

const (
	// Instruction means content contains executable instructions.
	Instruction ContentKind = "instruction"
	// Data means content is data only, never to be interpreted as instructions.
	Data ContentKind = "data"
)

// Taint represents a piece of content with security metadata.
type Taint struct {
	// ID is a unique identifier for taint tracking.
	ID string `json:"id"`

	// Trust indicates who created the content.
	Trust TrustLevel `json:"trust"`

	// Type indicates how to interpret the content.
	Type ContentKind `json:"type"`

	// Mutable indicates whether later content can override this taint.
	// Immutable taints have precedence immunity.
	Mutable bool `json:"mutable"`

	// Content is the actual text content.
	Content string `json:"content"`

	// ContentHash is SHA256 hash of content for de-duplication.
	ContentHash string `json:"content_hash,omitempty"`

	// Source describes where the content came from (for debugging).
	Source string `json:"source,omitempty"`

	// AgentContext identifies which agent/sub-agent created this taint.
	// Used to filter taints during security checks in multi-agent scenarios.
	AgentContext string `json:"agent_context,omitempty"`

	// TaintedBy lists IDs of taints that influenced this taint.
	TaintedBy []string `json:"tainted_by,omitempty"`

	// CreatedAtSeq is the session event sequence when this taint was created.
	// Used to correlate taints with session events in forensic analysis.
	CreatedAtSeq uint64 `json:"created_at_seq,omitempty"`

	// DedupeHit indicates this taint was reused from a previous identical content.
	DedupeHit bool `json:"dedupe_hit,omitempty"`
}

// computeHash returns SHA256 hash of content as hex string.
func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// newTaint creates a taint with the given properties.
// It enforces security invariants:
// - Untrusted content is always type=data
// - Untrusted content is always mutable=true
func newTaint(id string, trust TrustLevel, typ ContentKind, mutable bool, content, source string) *Taint {
	// Enforce invariants
	if trust == Untrusted {
		typ = Data     // Untrusted content is NEVER instruction
		mutable = true     // Untrusted content cannot claim precedence immunity
	}

	return &Taint{
		ID:      id,
		Trust:        trust,
		Type:         typ,
		Mutable:      mutable,
		Content:      content,
		ContentHash:  computeHash(content),
		Source:       source,
	}
}

// IsInstruction returns true if this taint contains executable instructions.
func (t *Taint) IsInstruction() bool {
	return t.Type == Instruction
}

// IsData returns true if this taint contains data only.
func (t *Taint) IsData() bool {
	return t.Type == Data
}

// IsImmutable returns true if this taint has precedence immunity.
func (t *Taint) IsImmutable() bool {
	return !t.Mutable
}

// CanOverride returns true if this taint can override the other taint.
// Higher trust + immutable beats lower trust + mutable.
func (t *Taint) CanOverride(other *Taint) bool {
	// Immutable taints cannot be overridden
	if other.IsImmutable() {
		return false
	}

	// Lower or equal trust cannot override higher trust
	if t.trustRank() <= other.trustRank() {
		return false
	}

	return true
}

// trustRank returns a numeric rank for trust level comparison.
func (t *Taint) trustRank() int {
	switch t.Trust {
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

// PropagatedTrust returns the trust level when combining this taint with another.
// The result is the lowest (least trusted) of the two.
func PropagatedTrust(a, b TrustLevel) TrustLevel {
	rankA := trustLevelRank(a)
	rankB := trustLevelRank(b)
	if rankA < rankB {
		return a
	}
	return b
}

func trustLevelRank(t TrustLevel) int {
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
