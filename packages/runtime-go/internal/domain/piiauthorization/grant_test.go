package piiauthorization

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	testsecurity "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const piiGrantTestAccount = "6222020202020202020"

func TestPIIProjectionGrantBindsExactCaseClaimContentTargetAndApprovalWithoutRawPII(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	grant, err := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePIIProjectionGrantForBindingsV1(
		grant, fixture.input.SecurityContext, fixture.input.ClaimLedgerDigest, fixture.input.ProjectionRulesetHash,
		fixture.input.ProjectedContentSHA256, fixture.input.PreservedControlledFieldCount, fixture.input.TargetIdentityDigest,
	); err != nil {
		t.Fatal(err)
	}
	body, err := PIIProjectionGrantV1Bytes(grant)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(piiGrantTestAccount)) {
		t.Fatalf("PII grant leaked the raw account: %s", body)
	}
	if !bytes.Contains(body, []byte(domainsecurity.SHA256Hex([]byte(piiGrantTestAccount)))) {
		t.Fatalf("PII grant lost the exact field hash: %s", body)
	}
	parsed, err := ParsePIIProjectionGrantV1(body)
	if err != nil || parsed.RecordDigest != grant.RecordDigest || parsed.ApprovalScopeDigest != grant.ApprovalScopeDigest {
		t.Fatalf("PII grant strict roundtrip failed: parsed=%#v err=%v", parsed, err)
	}
	keyID, publicKey, signature, err := PIIProjectionGrantAuthorityMaterialV1(parsed)
	if err != nil || keyID != domainsecurity.SHA256Hex(publicKey) ||
		!ed25519.Verify(publicKey, PIIProjectionGrantSigningBytesV1(parsed), signature) {
		t.Fatalf("PII grant authority material is invalid: key=%s err=%v", keyID, err)
	}
}

func TestPIIProjectionGrantRejectsCrossContextContentTargetAndExpiredScope(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	grant, err := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	otherContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-other", TurnID: "turn-other", WorkspaceRealPath: "/workspace/other", CaseID: "case-other",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("other-binding")), ContextEpoch: 2, IssuedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		context domainsecurity.TurnSecurityContext
		ledger  string
		rules   string
		content string
		count   uint64
		target  string
	}{
		{"cross context", otherContext, fixture.input.ClaimLedgerDigest, fixture.input.ProjectionRulesetHash, fixture.input.ProjectedContentSHA256, 1, fixture.input.TargetIdentityDigest},
		{"claim ledger", fixture.input.SecurityContext, domainsecurity.SHA256Hex([]byte("other-ledger")), fixture.input.ProjectionRulesetHash, fixture.input.ProjectedContentSHA256, 1, fixture.input.TargetIdentityDigest},
		{"rules", fixture.input.SecurityContext, fixture.input.ClaimLedgerDigest, domainsecurity.SHA256Hex([]byte("other-rules")), fixture.input.ProjectedContentSHA256, 1, fixture.input.TargetIdentityDigest},
		{"content", fixture.input.SecurityContext, fixture.input.ClaimLedgerDigest, fixture.input.ProjectionRulesetHash, domainsecurity.SHA256Hex([]byte("other-content")), 1, fixture.input.TargetIdentityDigest},
		{"field count", fixture.input.SecurityContext, fixture.input.ClaimLedgerDigest, fixture.input.ProjectionRulesetHash, fixture.input.ProjectedContentSHA256, 2, fixture.input.TargetIdentityDigest},
		{"target", fixture.input.SecurityContext, fixture.input.ClaimLedgerDigest, fixture.input.ProjectionRulesetHash, fixture.input.ProjectedContentSHA256, 1, domainsecurity.SHA256Hex([]byte("other-target"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if ValidatePIIProjectionGrantForBindingsV1(grant, test.context, test.ledger, test.rules, test.content, test.count, test.target) == nil {
				t.Fatal("PII grant accepted a mismatched binding")
			}
		})
	}
	tooLong := fixture.input
	tooLong.ExpiresAt = tooLong.IssuedAt.Add(PIIProjectionGrantMaxTTL + time.Nanosecond)
	if _, err := NewPIIProjectionGrantV1(tooLong, fixture.sign); err == nil {
		t.Fatal("PII grant accepted an overlong authorization window")
	}
}

func TestPIIProjectionGrantStrictParsingAndSignatureFailClosed(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	grant, err := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := PIIProjectionGrantV1Bytes(grant)
	unknown := append([]byte(nil), body[:len(body)-1]...)
	unknown = append(unknown, []byte(`,"modelSafeToAnswer":true}`)...)
	if _, err := ParsePIIProjectionGrantV1(unknown); err == nil {
		t.Fatal("PII grant accepted an unknown model-controlled property")
	}
	if _, err := ParsePIIProjectionGrantV1(append(append([]byte(nil), body...), '\n')); err == nil {
		t.Fatal("PII grant accepted noncanonical trailing bytes")
	}
	tampered := grant
	tampered.TargetIdentityDigest = domainsecurity.SHA256Hex([]byte("attacker-target"))
	tampered.ApprovalScopeDigest = approvalScopeDigestV1(tampered)
	tampered.GrantID = piiProjectionGrantIDV1(tampered)
	tampered.RecordDigest = piiProjectionGrantRecordDigestV1(tampered)
	if ValidatePIIProjectionGrantV1(tampered) == nil {
		t.Fatal("PII grant accepted a target mutation without a host signature")
	}
	duplicate := fixture.input
	duplicate.FieldBindings = append(duplicate.FieldBindings, duplicate.FieldBindings[0])
	duplicate.PreservedControlledFieldCount = 2
	if _, err := NewPIIProjectionGrantV1(duplicate, fixture.sign); err == nil {
		t.Fatal("PII grant accepted a duplicated controlled field")
	}
}

func TestPIIProjectionApprovalScopeIsDeterministicAndRecordIdentityIndependent(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	left, err := ApprovalScopeDigestV1(ApprovalScopeInputFromGrantInputV1(fixture.input))
	if err != nil {
		t.Fatal(err)
	}
	right, err := ApprovalScopeDigestV1(ApprovalScopeInputFromGrantInputV1(fixture.input))
	if err != nil || left != right {
		t.Fatalf("approval scope is not deterministic: left=%s right=%s err=%v", left, right, err)
	}
	changed := fixture.input
	changed.ApprovalID = "appr_controlled_pii_87654321"
	same, err := ApprovalScopeDigestV1(ApprovalScopeInputFromGrantInputV1(changed))
	if err != nil || same != left {
		t.Fatalf("approval record identity created a circular semantic scope: left=%s same=%s err=%v", left, same, err)
	}
	changed.ProjectedContentSHA256 = domainsecurity.SHA256Hex([]byte("different-report"))
	different, err := ApprovalScopeDigestV1(ApprovalScopeInputFromGrantInputV1(changed))
	if err != nil || different == left {
		t.Fatalf("approval scope did not bind report content: left=%s different=%s err=%v", left, different, err)
	}
	changed = fixture.input
	changed.AllowedAccessActions = []string{ControlledArtifactAccessActionDisplayV1}
	different, err = ApprovalScopeDigestV1(ApprovalScopeInputFromGrantInputV1(changed))
	if err != nil || different == left {
		t.Fatalf("approval scope did not bind display/export rights: left=%s different=%s err=%v", left, different, err)
	}
	changed = fixture.input
	changed.RetentionPolicyDigest = domainsecurity.SHA256Hex([]byte("different-retention-policy"))
	different, err = ApprovalScopeDigestV1(ApprovalScopeInputFromGrantInputV1(changed))
	if err != nil || different == left {
		t.Fatalf("approval scope did not bind retention policy: left=%s different=%s err=%v", left, different, err)
	}
}

func TestPIIProjectionGrantRejectsInvalidAccessAndRetentionAuthority(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	invalidAction := fixture.input
	invalidAction.AllowedAccessActions = []string{"download"}
	if _, err := NewPIIProjectionGrantV1(invalidAction, fixture.sign); err == nil {
		t.Fatal("PII grant accepted an unknown access action")
	}
	duplicateAction := fixture.input
	duplicateAction.AllowedAccessActions = []string{
		ControlledArtifactAccessActionDisplayV1,
		ControlledArtifactAccessActionDisplayV1,
	}
	if _, err := NewPIIProjectionGrantV1(duplicateAction, fixture.sign); err == nil {
		t.Fatal("PII grant accepted duplicate access rights")
	}
	shortRetention := fixture.input
	shortRetention.RetentionUntil = shortRetention.ExpiresAt.Add(-time.Nanosecond)
	if _, err := NewPIIProjectionGrantV1(shortRetention, fixture.sign); err == nil {
		t.Fatal("PII grant accepted retention ending before access authority")
	}
}

type piiGrantFixture struct {
	now        time.Time
	input      GrantInputV1
	privateKey ed25519.PrivateKey
}

func newPIIGrantFixture(t *testing.T) piiGrantFixture {
	t.Helper()
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	securityContext, err := testsecurity.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-pii-grant", TurnID: "turn-pii-grant", WorkspaceRealPath: "/workspace/pii-grant",
		CaseID: "case-pii-grant", CaseBindingHash: domainsecurity.SHA256Hex([]byte("pii-grant-binding")), ContextEpoch: 2, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x63}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	evidenceID := "evr_" + domainsecurity.SHA256Hex([]byte("pii-grant-evidence"))
	input := GrantInputV1{
		SecurityContext: securityContext, RequesterUserID: securityContext.UserID, DisclosurePurpose: DisclosurePurposeCaseReportV1,
		ClaimLedgerDigest: domainsecurity.SHA256Hex([]byte("pii-grant-ledger")),
		FieldBindings: []FieldBindingV1{{
			PIIClass: PIIClassFinancialAccountV1, ClaimID: "claim-account", ClaimRecordDigest: domainsecurity.SHA256Hex([]byte("claim-account-record")),
			ClaimType: domainevidence.ClaimAccount, FieldName: "accountId", ValueSHA256: domainsecurity.SHA256Hex([]byte(piiGrantTestAccount)),
			EvidenceReceiptIDs: []string{evidenceID},
		}},
		ProjectionRulesetHash:  domainsecurity.SHA256Hex([]byte("pii-grant-rules")),
		ProjectedContentSHA256: domainsecurity.SHA256Hex([]byte("controlled report with exact account")), PreservedControlledFieldCount: 1,
		TargetIdentityDigest:  domainsecurity.SHA256Hex([]byte("pii-grant-target")),
		AllowedAccessActions:  []string{ControlledArtifactAccessActionDisplayV1, ControlledArtifactAccessActionExportV1},
		AccessPolicyDigest:    domainsecurity.SHA256Hex([]byte("pii-grant-access-policy")),
		RetentionPolicyDigest: domainsecurity.SHA256Hex([]byte("pii-grant-retention-policy")),
		RetentionUntil:        now.Add(24 * time.Hour),
		ApprovalID:            "appr_controlled_pii_12345678", ApprovalRecordDigest: domainsecurity.SHA256Hex([]byte("pii-grant-approval-record")),
		IssuedAt: now.Add(time.Minute), ExpiresAt: now.Add(10 * time.Minute), AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}
	return piiGrantFixture{now: now, input: input, privateKey: privateKey}
}

func (fixture piiGrantFixture) sign(message []byte) ([]byte, error) {
	return ed25519.Sign(fixture.privateKey, message), nil
}

func TestPIIProjectionGrantCanonicalJSONContainsNoUnexpectedWhitespace(t *testing.T) {
	fixture := newPIIGrantFixture(t)
	grant, err := NewPIIProjectionGrantV1(fixture.input, fixture.sign)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(grant)
	if strings.Contains(string(body), "\n") || strings.Contains(string(body), "  ") {
		t.Fatalf("grant JSON is not compact canonical encoding: %s", body)
	}
}
