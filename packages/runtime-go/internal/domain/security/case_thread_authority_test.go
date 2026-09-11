package security

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
)

func TestCaseThreadAuthorityRecordBindsTrustedCaseContextStrictly(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	securityContext := mustCaseTurnSecurityContextV2(t, "thread-a", "turn-a", "/cases/a", "case-a", 3, time.Unix(3, 0))
	record, err := NewCaseThreadAuthorityRecord(securityContext, SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil || ValidateCaseThreadAuthorityRecord(record) != nil {
		t.Fatalf("valid case thread authority failed: record=%#v err=%v", record, err)
	}
	if !CaseThreadAuthorityCanAuthorizeExecution(record) {
		t.Fatal("current V2 case authority did not authorize execution")
	}
	body, err := CaseThreadAuthorityRecordBytes(record)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCaseThreadAuthorityRecord(body)
	if err != nil || parsed.RecordDigest != record.RecordDigest || parsed.SecurityContext == nil || *parsed.SecurityContext != securityContext {
		t.Fatalf("case thread authority round trip failed: parsed=%#v err=%v", parsed, err)
	}
	tampered := record
	tamperedContext := *record.SecurityContext
	tamperedContext.CaseID = "case-b"
	tampered.SecurityContext = &tamperedContext
	if ValidateCaseThreadAuthorityRecord(tampered) == nil {
		t.Fatal("case thread authority allowed case tampering")
	}
	if _, err := ParseCaseThreadAuthorityRecord(append(body[:len(body)-1], []byte(`,"unexpected":true}`)...)); err == nil {
		t.Fatal("case thread authority allowed unknown properties")
	}
	unbound := mustGeneralTurnSecurityContextV2(t, "thread-u", "turn-u", "/workspace")
	if _, err := NewCaseThreadAuthorityRecord(unbound, SHA256Hex(publicKey), publicKey, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil {
		t.Fatal("unbound thread received case authority")
	}
}

func TestLegacyV1CaseThreadAuthorityIsAuditOnly(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{17}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	legacy := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread-legacy", TurnID: "turn-legacy", WorkspaceRealPath: "/cases/legacy", CaseID: "case-legacy",
		CaseBindingHash: SHA256Hex([]byte("binding-legacy")), DatasetSnapshotID: "snapshot-legacy",
		SourceManifestHash: SHA256Hex([]byte("manifest-legacy")), ContextEpoch: 5, IssuedAt: time.Unix(5, 0),
	})
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	if _, err := NewCaseThreadAuthorityRecord(legacy, SHA256Hex(publicKey), publicKey, sign); err == nil {
		t.Fatal("new authority constructor accepted a legacy V1 execution context")
	}
	legacyRecord, err := signCaseThreadAuthorityRecord(CaseThreadAuthorityRecord{
		SchemaVersion: CaseThreadAuthorityRecordVersion, AuthorityPurpose: CaseThreadAuthorityPurpose,
		AuthorityAlgorithm: CaseThreadAuthorityAlgorithm, AuthorityKeyID: SHA256Hex(publicKey),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), SecurityContext: &legacy,
	}, sign)
	if err != nil || ValidateCaseThreadAuthorityRecord(legacyRecord) != nil {
		t.Fatalf("legacy signed authority must remain inspectable for audit: record=%#v err=%v", legacyRecord, err)
	}
	if CaseThreadAuthorityCanAuthorizeExecution(legacyRecord) {
		t.Fatal("legacy signed authority was treated as current execution authority")
	}
	body, err := CaseThreadAuthorityRecordBytes(legacyRecord)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCaseThreadAuthorityRecord(body)
	if err != nil || parsed.SecurityContext == nil || parsed.SecurityContext.Version != TurnSecurityContextVersionV1 {
		t.Fatalf("legacy authority audit round trip failed: parsed=%#v err=%v", parsed, err)
	}
}

func TestCaseBoundaryAuthorityIsDurableButCannotAuthorizeExecution(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{19}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	workspace := "/cases/boundary"
	policy := mustTurnPublicationPolicyV1(t, TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: SHA256Hex([]byte("case-policy:boundary")), RiskClass: RiskClassCase,
		Disposition: PublicationDispositionCaseBoundaryOnly, CaseBindingState: CaseBindingStateUnreadable,
		BindingObservationDigest: SHA256Hex([]byte("unreadable-binding")), BlockerCode: PublicationBlockerCaseBindingUnreadable,
	})
	context, err := NewTurnSecurityContextV2(TurnSecurityContextInput{
		ThreadID: "thread-boundary-authority", TurnID: "turn-boundary-authority", WorkspaceRealPath: workspace,
		TenantID: LocalTenantID, UserID: LocalUserID, CaseID: UnboundCaseID,
		CaseBindingHash: UnboundCaseBindingHash(workspace), DatasetSnapshotID: NoDatasetSnapshotID,
		SourceManifestHash: EmptySourceManifestHash, ContextEpoch: 6, IssuedAt: time.Unix(6, 0), PublicationPolicy: policy,
		RiskAuthorityBinding: NewQuarantinedRiskAuthorityBindingV1(),
	})
	if err != nil {
		t.Fatal(err)
	}
	sign := func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil }
	record, err := NewCaseThreadAuthorityRecord(context, SHA256Hex(publicKey), publicKey, sign)
	if err != nil || ValidateCaseThreadAuthorityRecord(record) != nil {
		t.Fatalf("boundary-only authority was not durable: record=%#v err=%v", record, err)
	}
	if CaseThreadAuthorityCanAuthorizeExecution(record) {
		t.Fatal("boundary-only authority authorized provider or tool execution")
	}
	snapshot := domaincontextepoch.SealSnapshot(domaincontextepoch.Snapshot{
		ThreadID: context.ThreadID, Epoch: context.ContextEpoch,
		RegistryDigest: domaincontextepoch.RegistryDigest(nil), AcceptedAt: time.Unix(6, 0).UTC().Format(time.RFC3339Nano),
	})
	state := domaincontextepoch.SealState(domaincontextepoch.State{ThreadID: context.ThreadID, AcceptedSnapshot: snapshot})
	committed, err := NewCommittedTurnContextAuthorityRecord(
		context, state, time.Unix(7, 0), SHA256Hex(publicKey), publicKey, sign,
	)
	if err != nil || ValidateCaseThreadAuthorityRecord(committed) != nil || !CaseThreadAuthorityIsCommittedContext(committed) {
		t.Fatalf("boundary-only committed authority failed: record=%#v err=%v", committed, err)
	}
	if CaseThreadAuthorityCanAuthorizeExecution(committed) {
		t.Fatal("committed boundary-only authority authorized execution")
	}
}

func TestCommittedTurnContextAuthorityBindsExactEpochState(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	securityContext := mustCaseTurnSecurityContextV2(t, "thread-commit", "turn-commit", "/cases/commit", "case-commit", 2, time.Unix(2, 0))
	snapshot := domaincontextepoch.SealSnapshot(domaincontextepoch.Snapshot{
		ThreadID: securityContext.ThreadID, Epoch: securityContext.ContextEpoch,
		RegistryDigest: domaincontextepoch.RegistryDigest(nil), AcceptedAt: time.Unix(2, 0).UTC().Format(time.RFC3339Nano),
	})
	state := domaincontextepoch.SealState(domaincontextepoch.State{ThreadID: securityContext.ThreadID, AcceptedSnapshot: snapshot})
	if err := domaincontextepoch.ValidateState(state); err != nil {
		t.Fatalf("committed context epoch fixture is invalid: %v", err)
	}
	record, err := NewCommittedTurnContextAuthorityRecord(securityContext, state, time.Unix(3, 0), SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil })
	if err != nil || ValidateCaseThreadAuthorityRecord(record) != nil || !CaseThreadAuthorityIsCommittedContext(record) {
		t.Fatalf("valid committed turn context authority failed: record=%#v err=%v", record, err)
	}
	body, err := CaseThreadAuthorityRecordBytes(record)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCaseThreadAuthorityRecord(body)
	if err != nil || parsed.CommittedTurnState == nil || parsed.CommittedTurnState.ContextEpochState.StateDigest != state.StateDigest {
		t.Fatalf("committed turn context round trip failed: parsed=%#v err=%v", parsed, err)
	}
	tampered := record
	tampered.CommittedTurnState = &CommittedTurnState{ContextEpochState: state, CommittedAt: time.Unix(4, 0).UTC().Format(time.RFC3339Nano)}
	if ValidateCaseThreadAuthorityRecord(tampered) == nil {
		t.Fatal("committed turn context allowed timestamp tampering")
	}
}

func TestCaseThreadLineageAuthorityBindsParentChildAndOperation(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	record, err := NewCaseThreadLineageAuthorityRecord(
		"thread-child", "thread-parent", SHA256Hex([]byte("parent-record")), "fork", SHA256Hex(publicKey), publicKey,
		func(message []byte) ([]byte, error) { return ed25519.Sign(privateKey, message), nil },
	)
	if err != nil || ValidateCaseThreadAuthorityRecord(record) != nil || !CaseThreadAuthorityIsLineage(record) ||
		CaseThreadAuthorityThreadID(record) != "thread-child" {
		t.Fatalf("valid lineage authority failed: record=%#v err=%v", record, err)
	}
	body, err := CaseThreadAuthorityRecordBytes(record)
	if err != nil {
		t.Fatal(err)
	}
	if parsed, parseErr := ParseCaseThreadAuthorityRecord(body); parseErr != nil || parsed.RecordDigest != record.RecordDigest {
		t.Fatalf("lineage authority round trip failed: parsed=%#v err=%v", parsed, parseErr)
	}
	tampered := record
	tampered.Derivation = "resume"
	if ValidateCaseThreadAuthorityRecord(tampered) == nil {
		t.Fatal("lineage authority allowed derivation tampering")
	}
	mixed := record
	mixed.SecurityContext = &TurnSecurityContext{}
	if ValidateCaseThreadAuthorityRecord(mixed) == nil {
		t.Fatal("lineage authority allowed mixed context and lineage variants")
	}
}
