package pendingwork

import (
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPendingWorkReceiptRoundTripStrictJSONAndSignatureTamper(t *testing.T) {
	input, privateKey := receiptFixture(t, KindProviderContinuation)
	receipt := mustReceipt(t, input, privateKey)
	body, err := PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePendingWorkReceiptV1(body)
	if err != nil || parsed.ReceiptID != receipt.ReceiptID {
		t.Fatalf("pending work receipt round trip failed: parsed=%#v err=%v", parsed, err)
	}
	_, publicKey, signature, err := PendingWorkReceiptV1AuthorityMaterial(parsed)
	if err != nil || !ed25519.Verify(publicKey, PendingWorkReceiptV1SigningBytes(parsed), signature) {
		t.Fatalf("pending work receipt signature failed: %v", err)
	}

	unknown := strings.TrimSuffix(string(body), "}") + `,"prompt":"never persist me"}`
	if _, err := ParsePendingWorkReceiptV1([]byte(unknown)); err == nil {
		t.Fatal("unknown pending work receipt field was accepted")
	}
	duplicate := strings.Replace(string(body), `"ordinal":1`, `"ordinal":1,"ordinal":1`, 1)
	if _, err := ParsePendingWorkReceiptV1([]byte(duplicate)); err == nil {
		t.Fatal("duplicate nested pending work receipt field was accepted")
	}
	tampered := receipt
	tampered.GrantMembers[0].ResultItemDigest = domainsecurity.SHA256Hex([]byte("other durable result"))
	if err := ValidatePendingWorkReceiptV1(tampered); err == nil {
		t.Fatal("tampered durable result digest was accepted")
	}
}

func TestPendingWorkIDIsDeterministicAndBindsOrderedAuthority(t *testing.T) {
	input, privateKey := receiptFixture(t, KindToolBatch)
	firstID, err := ComputePendingWorkIDV1(input)
	if err != nil {
		t.Fatal(err)
	}
	input.IssuedAt = input.IssuedAt.Add(time.Hour)
	input.ExpiresAt = input.ExpiresAt.Add(time.Hour)
	secondID, err := ComputePendingWorkIDV1(input)
	if err != nil || firstID != secondID {
		t.Fatalf("logical pending work ID changed with receipt timing: first=%s second=%s err=%v", firstID, secondID, err)
	}
	secondReceipt := mustReceipt(t, input, privateKey)
	if secondReceipt.WorkID != firstID {
		t.Fatalf("receipt did not retain deterministic work ID: %s", secondReceipt.WorkID)
	}

	changed := input
	changed.PayloadHash = domainsecurity.SHA256Hex([]byte("changed sanitized payload"))
	changedID, err := ComputePendingWorkIDV1(changed)
	if err != nil || changedID == firstID {
		t.Fatalf("payload drift did not change pending work ID: changed=%s err=%v", changedID, err)
	}
	changed = input
	changed.RouteHash = domainsecurity.SHA256Hex([]byte("changed route"))
	changedID, err = ComputePendingWorkIDV1(changed)
	if err != nil || changedID == firstID {
		t.Fatalf("route drift did not change pending work ID: changed=%s err=%v", changedID, err)
	}
	changed = input
	changed.GrantMembers = append([]GrantMemberV1(nil), input.GrantMembers...)
	changed.GrantMembers[0].GrantID = domainsecurity.SHA256Hex([]byte("different grant"))
	changedID, err = ComputePendingWorkIDV1(changed)
	if err != nil || changedID == firstID {
		t.Fatalf("ordered grant drift did not change pending work ID: changed=%s err=%v", changedID, err)
	}
}

func TestApprovedDispatchWorkIDIgnoresLaterRegistryHeadButBindsEffect(t *testing.T) {
	input, _ := receiptFixture(t, KindApprovedToolDispatch)
	input.GrantMembers = append([]GrantMemberV1(nil), input.GrantMembers[:1]...)
	firstID, err := ComputePendingWorkIDV1(input)
	if err != nil {
		t.Fatal(err)
	}
	changedHead := input
	changedHead.GrantRegistrySequence++
	changedHead.GrantRegistryDigest = domainsecurity.SHA256Hex([]byte("later registry head"))
	secondID, err := ComputePendingWorkIDV1(changedHead)
	if err != nil || secondID != firstID {
		t.Fatalf("later registry head changed approved effect identity: first=%s second=%s err=%v", firstID, secondID, err)
	}
	changedEffect := input
	changedEffect.GrantMembers = append([]GrantMemberV1(nil), input.GrantMembers...)
	changedEffect.GrantMembers[0].RegistryEntryDigest = domainsecurity.SHA256Hex([]byte("different approved member"))
	thirdID, err := ComputePendingWorkIDV1(changedEffect)
	if err != nil || thirdID == firstID {
		t.Fatalf("approved member drift retained effect identity: changed=%s err=%v", thirdID, err)
	}
}

func TestSideEffectIntentWorkIDBindsOnlyFrozenContextAndSemanticPayload(t *testing.T) {
	input, _ := receiptFixture(t, KindSideEffectIntent)
	input.GrantMembers = append([]GrantMemberV1(nil), input.GrantMembers[:1]...)
	firstID, err := ComputePendingWorkIDV1(input)
	if err != nil {
		t.Fatal(err)
	}

	changedExecution := input
	changedExecution.GrantRegistrySequence++
	changedExecution.GrantRegistryDigest = domainsecurity.SHA256Hex([]byte("later registry head"))
	changedExecution.RouteHash = domainsecurity.SHA256Hex([]byte("reconnected route"))
	changedExecution.GrantMembers = append([]GrantMemberV1(nil), input.GrantMembers...)
	changedExecution.GrantMembers[0].GrantID = domainsecurity.SHA256Hex([]byte("new call and grant"))
	changedExecution.GrantMembers[0].RegistrySequence++
	changedExecution.GrantMembers[0].RegistryEntryDigest = domainsecurity.SHA256Hex([]byte("new registry member"))
	secondID, err := ComputePendingWorkIDV1(changedExecution)
	if err != nil || secondID != firstID {
		t.Fatalf("execution identity bypassed semantic side-effect identity: first=%s second=%s err=%v", firstID, secondID, err)
	}

	changedPayload := input
	changedPayload.PayloadHash = domainsecurity.SHA256Hex([]byte("different semantic arguments"))
	thirdID, err := ComputePendingWorkIDV1(changedPayload)
	if err != nil || thirdID == firstID {
		t.Fatalf("different semantic payload retained side-effect identity: changed=%s err=%v", thirdID, err)
	}

	changedContext := input
	contextIssuedAt, err := time.Parse(time.RFC3339Nano, input.SecurityContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	changedContext.SecurityContext, err = securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.SecurityContext.ThreadID, TurnID: input.SecurityContext.TurnID,
		WorkspaceRealPath: input.SecurityContext.WorkspaceRealPath, TenantID: input.SecurityContext.TenantID,
		UserID: input.SecurityContext.UserID, CaseID: input.SecurityContext.CaseID,
		CaseBindingHash: input.SecurityContext.CaseBindingHash, DatasetSnapshotID: input.SecurityContext.DatasetSnapshotID,
		SourceManifestHash: input.SecurityContext.SourceManifestHash, ContextEpoch: input.SecurityContext.ContextEpoch + 1,
		IssuedAt: contextIssuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	fourthID, err := ComputePendingWorkIDV1(changedContext)
	if err != nil || fourthID == firstID {
		t.Fatalf("different frozen context retained side-effect identity: changed=%s err=%v", fourthID, err)
	}
}

func TestApprovedDispatchDispositionRequiresCanonicalOutcomeReason(t *testing.T) {
	input, privateKey := receiptFixture(t, KindApprovedToolDispatch)
	input.GrantMembers = append([]GrantMemberV1(nil), input.GrantMembers[:1]...)
	receipt := mustReceipt(t, input, privateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	for _, test := range []struct{ status, reason string }{
		{StatusOutcomeUnknown, "restart_invalid"},
		{StatusFailed, "tool_outcome_unknown_after_restart"},
		{StatusCompleted, "completed"},
	} {
		if _, err := NewPendingWorkDispositionV1(
			receipt, test.status, test.reason, input.IssuedAt.Add(time.Minute),
			domainsecurity.SHA256Hex(publicKey), publicKey,
			func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
		); err == nil {
			t.Fatalf("approved dispatch accepted status/reason mismatch: %#v", test)
		}
	}
}

func TestPendingWorkKindSpecificResultBindingFailsClosed(t *testing.T) {
	provider, _ := receiptFixture(t, KindProviderContinuation)
	provider.GrantMembers[0].ResultItemDigest = ""
	if _, err := ComputePendingWorkIDV1(provider); err == nil {
		t.Fatal("provider continuation without exact durable result digest was accepted")
	}
	provider, _ = receiptFixture(t, KindProviderContinuation)
	provider.GrantMembers = nil
	if _, err := ComputePendingWorkIDV1(provider); err == nil {
		t.Fatal("zero-member provider continuation was accepted")
	}
	report, _ := receiptFixture(t, KindReportStage)
	report.GrantMembers[0].ResultItemID = "item-untrusted"
	report.GrantMembers[0].ResultItemDigest = domainsecurity.SHA256Hex([]byte("untrusted"))
	if _, err := ComputePendingWorkIDV1(report); err == nil {
		t.Fatal("report stage with provider result authority was accepted")
	}
	batch, _ := receiptFixture(t, KindToolBatch)
	batch.GrantMembers[0].ResultItemID = "item-untrusted"
	batch.GrantMembers[0].ResultItemDigest = domainsecurity.SHA256Hex([]byte("untrusted"))
	if _, err := ComputePendingWorkIDV1(batch); err == nil {
		t.Fatal("tool batch with provider result authority was accepted")
	}
	batch, _ = receiptFixture(t, KindToolBatch)
	batch.GrantMembers[1].Ordinal = 1
	if _, err := ComputePendingWorkIDV1(batch); err == nil {
		t.Fatal("duplicate ordered grant ordinal was accepted")
	}
}

func TestPendingWorkDispositionRoundTripPurposeSeparationAndExactReceipt(t *testing.T) {
	input, privateKey := receiptFixture(t, KindReportStage)
	receipt := mustReceipt(t, input, privateKey)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	disposition, err := NewPendingWorkDispositionV1(
		receipt, StatusCompleted, "report_stage_completed", input.IssuedAt.Add(time.Minute),
		domainsecurity.SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := PendingWorkDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParsePendingWorkDispositionV1(body)
	if err != nil || ValidatePendingWorkDispositionForReceiptV1(parsed, receipt) != nil {
		t.Fatalf("pending work disposition round trip failed: parsed=%#v err=%v", parsed, err)
	}
	_, _, signature, _ := PendingWorkDispositionV1AuthorityMaterial(parsed)
	if ed25519.Verify(publicKey, PendingWorkReceiptV1SigningBytes(receipt), signature) {
		t.Fatal("receipt and disposition signatures were not purpose-separated")
	}
	otherInput := input
	otherInput.PayloadHash = domainsecurity.SHA256Hex([]byte("different report payload"))
	otherReceipt := mustReceipt(t, otherInput, privateKey)
	if err := ValidatePendingWorkDispositionForReceiptV1(disposition, otherReceipt); err == nil {
		t.Fatal("disposition was accepted for another receipt")
	}
	unknown := strings.TrimSuffix(string(body), "}") + `,"reasoning":"secret"}`
	if _, err := ParsePendingWorkDispositionV1([]byte(unknown)); err == nil {
		t.Fatal("unknown pending work disposition field was accepted")
	}
}

func TestPendingWorkReceiptDoesNotPersistRawPromptPIIOrReasoning(t *testing.T) {
	input, privateKey := receiptFixture(t, KindToolBatch)
	receipt := mustReceipt(t, input, privateKey)
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"/Users/alice/private-case", "tenant-sensitive", "user-sensitive", "raw-case-sensitive",
		"full bank account 6222020000000000000", "private reasoning chain",
	} {
		if bytesContains(body, forbidden) {
			t.Fatalf("pending work receipt persisted forbidden raw content %q: %s", forbidden, body)
		}
	}
	for _, forbiddenField := range []string{`"prompt"`, `"reasoning"`, `"payload"`, `"caseId"`, `"workspaceRealPath"`, `"userId"`, `"tenantId"`} {
		if bytesContains(body, forbiddenField) {
			t.Fatalf("pending work receipt exposed forbidden field %s: %s", forbiddenField, body)
		}
	}
}

func TestV1PendingWorkCannotBeIssuedButRemainsAuditParseable(t *testing.T) {
	input, privateKey := receiptFixture(t, KindToolBatch)
	input.SecurityContext = domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.SecurityContext.ThreadID, TurnID: input.SecurityContext.TurnID,
		WorkspaceRealPath: input.SecurityContext.WorkspaceRealPath, ContextEpoch: input.SecurityContext.ContextEpoch,
		IssuedAt: input.IssuedAt.Add(-time.Minute),
	})
	if err := domainsecurity.ValidateTurnSecurityContext(input.SecurityContext); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPendingWorkReceiptV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil {
		t.Fatal("audit-only V1 context issued executable pending work")
	}
}

func TestOrdinaryPendingWorkCanBindWitnessedCaseBoundary(t *testing.T) {
	input, privateKey := receiptFixture(t, KindToolBatch)
	issuedAt, err := time.Parse(time.RFC3339Nano, input.SecurityContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := securitycontexttest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: input.SecurityContext.ThreadID, TurnID: input.SecurityContext.TurnID,
		WorkspaceRealPath: input.SecurityContext.WorkspaceRealPath,
		ContextEpoch:      input.SecurityContext.ContextEpoch, IssuedAt: issuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		boundary.ThreadID,
		boundary.WorkspaceRealPath,
		domainsecurity.RiskClassCase,
		boundary.PublicationPolicy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err = domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: boundary.ThreadID, TurnID: boundary.TurnID, WorkspaceRealPath: boundary.WorkspaceRealPath,
		TenantID: boundary.TenantID, UserID: boundary.UserID, CaseID: boundary.CaseID,
		CaseBindingHash: boundary.CaseBindingHash, DatasetSnapshotID: boundary.DatasetSnapshotID,
		SourceManifestHash: boundary.SourceManifestHash, ContextEpoch: boundary.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: boundary.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.SecurityContext = boundary
	if _, err := NewPendingWorkReceiptV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err != nil {
		t.Fatalf("witnessed case boundary disabled an ordinary pending-work receipt: %v", err)
	}
}

func receiptFixture(t *testing.T, kind string) (ReceiptInputV1, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 123).UTC()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-safe", TurnID: "turn-safe", WorkspaceRealPath: "/Users/alice/private-case",
		TenantID: "tenant-sensitive", UserID: "user-sensitive", CaseID: "raw-case-sensitive",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-opaque-v2"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest")), ContextEpoch: 7, IssuedAt: now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	members := []GrantMemberV1{
		{Ordinal: 1, GrantID: domainsecurity.SHA256Hex([]byte("grant-1")), RegistrySequence: 2, RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry-2"))},
		{Ordinal: 2, GrantID: domainsecurity.SHA256Hex([]byte("grant-2")), RegistrySequence: 4, RegistryEntryDigest: domainsecurity.SHA256Hex([]byte("entry-4"))},
	}
	if kind == KindProviderContinuation {
		for index := range members {
			members[index].ResultItemID = "item-result-" + string(rune('a'+index))
			members[index].ResultItemDigest = domainsecurity.SHA256Hex([]byte("durable-result-" + members[index].ResultItemID))
		}
	}
	return ReceiptInputV1{
		Kind: kind, SecurityContext: securityContext, GrantRegistrySequence: 5,
		GrantRegistryDigest: domainsecurity.SHA256Hex([]byte("registry-state-5")), GrantMembers: members,
		PayloadHash: domainsecurity.SHA256Hex([]byte("sanitized request shape; full bank account 6222020000000000000 is not stored")),
		RouteHash:   domainsecurity.SHA256Hex([]byte("provider route")), IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute),
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}, privateKey
}

func mustReceipt(t *testing.T, input ReceiptInputV1, privateKey ed25519.PrivateKey) PendingWorkReceiptV1 {
	t.Helper()
	receipt, err := NewPendingWorkReceiptV1(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func bytesContains(body []byte, value string) bool {
	return strings.Contains(string(body), value)
}
