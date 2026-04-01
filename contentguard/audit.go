package contentguard

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// AuditTrail manages cryptographic signing of security decisions.
type AuditTrail struct {
	sessionID  string
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
	records    []*Record
}

// NewAuditTrail creates a new audit trail with a fresh Ed25519 keypair.
func NewAuditTrail(sessionID string) (*AuditTrail, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair: %w", err)
	}

	return &AuditTrail{
		sessionID:  sessionID,
		publicKey:  pub,
		privateKey: priv,
		records:    make([]*Record, 0),
	}, nil
}

// PublicKey returns the base64-encoded public key for verification.
func (a *AuditTrail) PublicKey() string {
	return base64.StdEncoding.EncodeToString(a.publicKey)
}

// SessionID returns the session identifier.
func (a *AuditTrail) SessionID() string {
	return a.sessionID
}

// Record represents a signed security decision.
type Record struct {
	SessionID   string    `json:"session_id"`
	Timestamp   time.Time `json:"timestamp"`
	ToolName    string    `json:"tool_name"`
	Verdict     string    `json:"verdict"`
	Rationale   string    `json:"rationale"`
	Findings    []string  `json:"findings"` // summary of each stage finding
	ContentHash string    `json:"content_hash,omitempty"`
	Signature   string    `json:"signature,omitempty"`
}

// RecordDecision creates and signs a decision record.
func (a *AuditTrail) RecordDecision(result *Result) *Record {
	var findings []string
	for _, f := range result.Findings {
		findings = append(findings, fmt.Sprintf("[%s] %s: %s", f.Verdict, f.Source, f.Rationale))
	}

	record := &Record{
		SessionID: a.sessionID,
		Timestamp: time.Now().UTC(),
		ToolName:  result.ToolName,
		Verdict:   string(result.Verdict),
		Rationale: result.Rationale,
		Findings:  findings,
	}

	record.Signature = signRecord(a.privateKey, record)
	a.records = append(a.records, record)

	return record
}

func signRecord(key ed25519.PrivateKey, record *Record) string {
	canonical := canonicalJSON(record)
	hash := sha256.Sum256(canonical)
	sig := ed25519.Sign(key, hash[:])
	return base64.StdEncoding.EncodeToString(sig)
}

func canonicalJSON(record *Record) []byte {
	m := map[string]any{
		"session_id": record.SessionID,
		"timestamp":  record.Timestamp.Format(time.RFC3339Nano),
		"tool_name":  record.ToolName,
		"verdict":    record.Verdict,
		"rationale":  record.Rationale,
		"findings":   record.Findings,
	}
	if record.ContentHash != "" {
		m["content_hash"] = record.ContentHash
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := "{"
	for i, k := range keys {
		if i > 0 {
			result += ","
		}
		v, _ := json.Marshal(m[k])
		result += fmt.Sprintf(`"%s":%s`, k, string(v))
	}
	result += "}"

	return []byte(result)
}

func hashBase64(content string) string {
	hash := sha256.Sum256([]byte(content))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// VerifyRecord verifies a record's signature against the given public key.
func VerifyRecord(record *Record, publicKeyBase64 string) (bool, error) {
	pubKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return false, fmt.Errorf("invalid public key: %w", err)
	}

	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key size: %d", len(pubKeyBytes))
	}

	sigBytes, err := base64.StdEncoding.DecodeString(record.Signature)
	if err != nil {
		return false, fmt.Errorf("invalid signature: %w", err)
	}

	canonical := canonicalJSON(record)
	hash := sha256.Sum256(canonical)

	return ed25519.Verify(ed25519.PublicKey(pubKeyBytes), hash[:], sigBytes), nil
}

// Records returns all recorded decisions.
func (a *AuditTrail) Records() []*Record {
	return a.records
}

// SessionLog represents a complete session security log.
type SessionLog struct {
	SessionID string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
	PublicKey  string    `json:"public_key"`
	Records   []*Record `json:"records"`
}

// ExportLog exports the audit trail as a session log.
func (a *AuditTrail) ExportLog() *SessionLog {
	return &SessionLog{
		SessionID: a.sessionID,
		StartedAt: time.Now().UTC(),
		PublicKey:  a.PublicKey(),
		Records:   a.records,
	}
}

// Close zeros out the private key from memory.
func (a *AuditTrail) Close() {
	for i := range a.privateKey {
		a.privateKey[i] = 0
	}
	a.privateKey = nil
}
