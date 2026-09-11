package contextepoch

import (
	"testing"
	"time"

	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestSecurityBindingEntryBindsV2PublicationAuthority(t *testing.T) {
	at := time.Date(2026, 7, 12, 23, 0, 0, 0, time.UTC)
	general := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-binding-entry", TurnID: "turn-general", WorkspaceRealPath: "/workspace", ContextEpoch: 2, IssuedAt: at,
	})
	boundary := newContextEpochBoundaryTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: general.ThreadID, TurnID: "turn-boundary", WorkspaceRealPath: general.WorkspaceRealPath,
		ContextEpoch: general.ContextEpoch, IssuedAt: at.Add(time.Second),
	}, domainsecurity.PublicationBlockerCaseBindingMissing)
	generalEntry := SecurityBindingEntry(general)
	boundaryEntry := SecurityBindingEntry(boundary)
	if generalEntry.Digest == boundaryEntry.Digest {
		t.Fatal("security binding entry dropped risk policy, observation, disposition, or blocker authority")
	}
	derived, err := securityContextAtEpoch(boundary, boundary.ContextEpoch+1, at.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !domainsecurity.TurnSecurityContextIsBoundaryOnly(derived) || derived.PublicationPolicy != boundary.PublicationPolicy {
		t.Fatalf("boundary derivation downgraded or replaced publication policy: %#v", derived)
	}
}

func TestPrepareTurnRejectsLegacyV1AuditContext(t *testing.T) {
	legacy := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-legacy-audit", TurnID: "turn-legacy", WorkspaceRealPath: "/workspace", IssuedAt: time.Now().UTC(),
	})
	if _, err := PrepareTurn(PrepareTurnInput{Thread: map[string]any{}, SecurityContext: legacy, At: time.Now().UTC()}); err == nil {
		t.Fatal("legacy V1 context entered the accepted context epoch")
	}
}

func TestCompactionPreservesCasePolicyAndRejectsBoundary(t *testing.T) {
	at := time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC)
	caseContext := newContextEpochTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-case-compact", TurnID: "turn-case", WorkspaceRealPath: "/workspace/case",
		CaseID: "case-1234", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-case")),
		DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("snapshot-case"), SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-case")),
		ContextEpoch: 3, IssuedAt: at,
	})
	state, err := BootstrapState(caseContext.ThreadID, caseContext.ContextEpoch, []domaincontextepoch.SourceEntry{SecurityBindingEntry(caseContext)}, at)
	if err != nil {
		t.Fatal(err)
	}
	thread := map[string]any{
		"id": caseContext.ThreadID, "securityState": turnSecurityRecordForTest(caseContext), "contextEpochState": PublicState(state),
	}
	nextTurns := []any{map[string]any{
		"id":    "turn-case-compact",
		"items": []any{map[string]any{"kind": "compaction", "auto": true}},
	}}
	compacted, err := PrepareAndAttachCompactionAuthority(
		thread, nextTurns, caseContext, caseContext.ThreadID, "turn-case-compact", domainsecurity.SHA256Hex([]byte("recovery")), at.Add(time.Second), true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !domainsecurity.TurnSecurityContextAllowsCaseEvidence(compacted.SecurityContext) ||
		compacted.SecurityContext.PublicationPolicy != caseContext.PublicationPolicy {
		t.Fatalf("case compaction lost final-gate policy: %#v", compacted)
	}
	if auto, err := CompactionAutoMode(compacted.State); err != nil || !auto {
		t.Fatalf("case compaction did not bind its automatic mode: auto=%t err=%v", auto, err)
	}
	tampered := compacted.State
	for index := range tampered.Registry {
		if tampered.Registry[index].SourceID == CompactionModeSourceID {
			sequence := tampered.Registry[index].Sequence
			tampered.Registry[index] = compactionModeSourceEntry(false)
			tampered.Registry[index].Sequence = sequence
		}
	}
	tampered = domaincontextepoch.SealState(tampered)
	if _, err := CompactionAutoMode(tampered); err == nil {
		t.Fatal("registry-only compaction mode tampering was accepted")
	}
	manual := tampered
	manual.AcceptedSnapshot.RegistryDigest = domaincontextepoch.RegistryDigest(manual.Registry)
	manual.AcceptedSnapshot.Sources = sourceSnapshots(manual.Registry)
	manual.AcceptedSnapshot = domaincontextepoch.SealSnapshot(manual.AcceptedSnapshot)
	manual = domaincontextepoch.SealState(manual)
	if auto, err := CompactionAutoMode(manual); err != nil || auto {
		t.Fatalf("manual compaction mode was not parsed exactly: auto=%t err=%v", auto, err)
	}
	missing := compacted.State
	missing.Registry = nil
	missing = domaincontextepoch.SealState(missing)
	if _, err := CompactionAutoMode(missing); err == nil {
		t.Fatal("missing compaction mode authority was accepted")
	}
	boundary := newContextEpochBoundaryTSCV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-boundary-compact", TurnID: "turn-boundary", WorkspaceRealPath: "/workspace/boundary",
		ContextEpoch: 1, IssuedAt: at,
	}, domainsecurity.PublicationBlockerCaseBindingMissing)
	if _, err := PrepareAndAttachCompactionAuthority(map[string]any{
		"id": boundary.ThreadID, "securityState": turnSecurityRecordForTest(boundary),
	}, []any{map[string]any{"id": "turn-never"}}, boundary, boundary.ThreadID, "turn-never", "", at, false); err == nil {
		t.Fatal("boundary-only context authorized compaction")
	}
}

func newContextEpochTSCV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	if input.TenantID == "" {
		input.TenantID = domainsecurity.LocalTenantID
	}
	if input.UserID == "" {
		input.UserID = domainsecurity.LocalUserID
	}
	if input.ContextEpoch == 0 {
		input.ContextEpoch = 1
	}
	if input.CaseID == "" || input.CaseID == domainsecurity.UnboundCaseID {
		input.CaseID = domainsecurity.UnboundCaseID
		input.CaseBindingHash = domainsecurity.UnboundCaseBindingHash(input.WorkspaceRealPath)
		input.DatasetSnapshotID = domainsecurity.NoDatasetSnapshotID
		input.PublicationPolicy = contextEpochPolicyForTest(t, input, domainsecurity.PublicationDispositionGeneralOutput)
	} else {
		if input.DatasetSnapshotID == "" {
			input.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID(input.ThreadID + ":" + input.TurnID)
		} else if len(input.DatasetSnapshotID) < len(domainsecurity.DatasetSnapshotIDPrefixV1)+64 ||
			input.DatasetSnapshotID[:len(domainsecurity.DatasetSnapshotIDPrefixV1)] != domainsecurity.DatasetSnapshotIDPrefixV1 ||
			!domainsecurity.IsSHA256Hex(input.DatasetSnapshotID[len(domainsecurity.DatasetSnapshotIDPrefixV1):]) {
			input.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID(input.DatasetSnapshotID)
		}
		input.PublicationPolicy = contextEpochPolicyForTest(t, input, domainsecurity.PublicationDispositionCaseEvidenceGate)
	}
	if input.SourceManifestHash == "" {
		input.SourceManifestHash = domainsecurity.EmptySourceManifestHash
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(
		input.ThreadID, input.WorkspaceRealPath, input.PublicationPolicy.RiskClass, input.PublicationPolicy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	input.RiskAuthorityBinding = binding
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func newContextEpochBoundaryTSCV2(t *testing.T, input domainsecurity.TurnSecurityContextInput, blocker string) domainsecurity.TurnSecurityContext {
	t.Helper()
	input.TenantID = domainsecurity.LocalTenantID
	input.UserID = domainsecurity.LocalUserID
	if input.ContextEpoch == 0 {
		input.ContextEpoch = 1
	}
	input.CaseID = domainsecurity.UnboundCaseID
	input.CaseBindingHash = domainsecurity.UnboundCaseBindingHash(input.WorkspaceRealPath)
	input.DatasetSnapshotID = domainsecurity.NoDatasetSnapshotID
	input.SourceManifestHash = domainsecurity.EmptySourceManifestHash
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("risk:case:" + input.ThreadID)),
		RiskClass:              domainsecurity.RiskClassCase, Disposition: domainsecurity.PublicationDispositionCaseBoundaryOnly,
		CaseBindingState:         domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("observation:missing:" + input.WorkspaceRealPath)),
		BlockerCode:              blocker,
	})
	if err != nil {
		t.Fatal(err)
	}
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = domainsecurity.NewQuarantinedRiskAuthorityBindingV1()
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func contextEpochPolicyForTest(t *testing.T, input domainsecurity.TurnSecurityContextInput, disposition string) domainsecurity.TurnPublicationPolicyV1 {
	t.Helper()
	riskClass := domainsecurity.RiskClassGeneral
	bindingState := domainsecurity.CaseBindingStateMissing
	if disposition == domainsecurity.PublicationDispositionCaseEvidenceGate {
		riskClass = domainsecurity.RiskClassCase
		bindingState = domainsecurity.CaseBindingStateValid
	}
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("risk:" + riskClass + ":" + input.ThreadID + ":" + input.WorkspaceRealPath)),
		RiskClass:              riskClass, Disposition: disposition, CaseBindingState: bindingState,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("observation:" + input.WorkspaceRealPath + ":" + input.CaseID + ":" + input.CaseBindingHash)),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}
