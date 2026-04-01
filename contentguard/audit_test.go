package contentguard

import (
	"encoding/base64"
	"testing"
)

func TestAuditTrail_KeyGeneration(t *testing.T) {
	trail, err := NewAuditTrail("test-session")
	if err != nil {
		t.Fatalf("failed to create audit trail: %v", err)
	}
	defer trail.Close()

	if trail.PublicKey() == "" {
		t.Error("expected non-empty public key")
	}
	if trail.SessionID() != "test-session" {
		t.Errorf("expected session ID 'test-session', got %q", trail.SessionID())
	}
}

func TestAuditTrail_RecordAndVerify(t *testing.T) {
	trail, _ := NewAuditTrail("test-session")
	defer trail.Close()

	result := &Result{
		Verdict:   Deny,
		Rationale: "injection detected",
		ToolName:  "bash",
		Findings: []*Finding{
			{Verdict: Escalate, Rationale: "high_risk_tool:bash", Source: "deterministic"},
			{Verdict: Deny, Rationale: "injection detected", Source: "reviewer"},
		},
	}

	record := trail.RecordDecision(result)

	if record.Verdict != string(Deny) {
		t.Errorf("expected verdict 'deny', got %q", record.Verdict)
	}
	if record.Signature == "" {
		t.Error("expected non-empty signature")
	}
	if len(record.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(record.Findings))
	}

	valid, err := VerifyRecord(record, trail.PublicKey())
	if err != nil {
		t.Fatalf("verification error: %v", err)
	}
	if !valid {
		t.Error("expected valid signature")
	}
}

func TestAuditTrail_TamperedRecord(t *testing.T) {
	trail, _ := NewAuditTrail("test-session")
	defer trail.Close()

	record := trail.RecordDecision(&Result{
		Verdict: Allow, ToolName: "read",
		Findings: []*Finding{{Verdict: Allow, Source: "deterministic"}},
	})

	record.Verdict = string(Deny)

	valid, _ := VerifyRecord(record, trail.PublicKey())
	if valid {
		t.Error("expected invalid signature after tampering")
	}
}

func TestAuditTrail_ExportLog(t *testing.T) {
	trail, _ := NewAuditTrail("test-session")
	defer trail.Close()

	trail.RecordDecision(&Result{
		Verdict: Allow, ToolName: "read",
		Findings: []*Finding{{Verdict: Allow, Source: "deterministic"}},
	})

	log := trail.ExportLog()
	if log.SessionID != "test-session" {
		t.Errorf("expected session 'test-session', got %q", log.SessionID)
	}
	if len(log.Records) != 1 {
		t.Errorf("expected 1 record, got %d", len(log.Records))
	}
}

func TestVerifyRecord_InvalidPublicKey(t *testing.T) {
	_, err := VerifyRecord(&Record{Signature: "abc"}, "not-valid!!!")
	if err == nil {
		t.Error("expected error for invalid public key")
	}
}

func TestVerifyRecord_WrongKeySize(t *testing.T) {
	small := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := VerifyRecord(&Record{Signature: "abc"}, small)
	if err == nil {
		t.Error("expected error for wrong key size")
	}
}
