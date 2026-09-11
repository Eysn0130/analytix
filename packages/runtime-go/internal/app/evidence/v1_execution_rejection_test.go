package evidence

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
)

func TestEvidenceExecutionBoundariesRejectStructurallyValidV1Authority(t *testing.T) {
	fixture := v1EvidenceExecutionFixture(t)
	assertStructurallyValidV1EvidenceFixture(t, fixture)

	t.Run("normalize MCP outcome", func(t *testing.T) {
		outcome, err := NormalizeMCPToolOutcome(NormalizeMCPToolOutcomeInput{
			Context: fixture.context, Grant: fixture.grant, Call: fixture.call,
			Raw: map[string]any{"executed": true}, At: fixture.now.Add(time.Second),
		})
		if err == nil || outcome.Version != 0 {
			t.Fatalf("V1 audit authority normalized a live tool outcome: outcome=%#v err=%v", outcome, err)
		}
	})

	t.Run("issuer prepare", func(t *testing.T) {
		registry := &v1RejectionRegistrySpy{}
		settlements := &v1RejectionSettlementStoreSpy{}
		issuer := Issuer{
			Registry: registry, SettlementStore: settlements, Authority: newMemoryFinalAuthority(171),
			Now: func() time.Time { return fixture.now.Add(2 * time.Second) },
		}
		prepared, err := issuer.Prepare(context.Background(), fixture.issue)
		if err == nil || prepared.Record.ReceiptID != "" || prepared.Marker.ReceiptID != "" {
			t.Fatalf("V1 audit authority prepared a receipt settlement: prepared=%#v err=%v", prepared, err)
		}
		if registry.calls != 0 || settlements.calls != 0 {
			t.Fatalf("V1 issuer touched private authority stores: registry=%d settlements=%d", registry.calls, settlements.calls)
		}
	})

	t.Run("resolve current evidence", func(t *testing.T) {
		registry := &v1RejectionRegistrySpy{}
		resolved, err := ResolveCurrentEvidence(context.Background(), registry, fixture.context, []string{"evr_legacy-audit-reference"})
		if err == nil || resolved != nil {
			t.Fatalf("V1 audit authority resolved current evidence: resolved=%#v err=%v", resolved, err)
		}
		if registry.calls != 0 {
			t.Fatalf("V1 evidence resolution called the registry %d times", registry.calls)
		}
	})

	t.Run("tool evidence prepare and commit", func(t *testing.T) {
		registry := &v1RejectionRegistrySpy{}
		settlements := &v1RejectionSettlementStoreSpy{}
		reader := &v1RejectionEvidenceReaderSpy{}
		service := ToolEvidenceService{
			Issuer: Issuer{
				Registry: registry, SettlementStore: settlements, Authority: newMemoryFinalAuthority(172),
				Now: func() time.Time { return fixture.now.Add(2 * time.Second) },
			},
			Reader: reader, Now: func() time.Time { return fixture.now.Add(time.Second) },
		}
		input := PrepareToolEvidenceInput{
			Context: fixture.context, Grant: fixture.grant, GrantRegistry: fixture.grantRegistry,
			Call: fixture.call, Outcome: fixture.outcome, ResultItemID: fixture.issue.ResultItemID,
		}
		prepared, eligible, err := service.PrepareCurrentToolEvidence(context.Background(), input)
		if err == nil || !eligible || prepared.Authority.Record.ReceiptID != "" {
			t.Fatalf("V1 audit authority prepared tool evidence: prepared=%#v eligible=%v err=%v", prepared, eligible, err)
		}
		if reader.calls != 0 || registry.calls != 0 || settlements.calls != 0 {
			t.Fatalf("V1 tool preparation crossed a side-effect boundary: reader=%d registry=%d settlements=%d", reader.calls, registry.calls, settlements.calls)
		}

		legacyPrepared := v1RejectionPreparedSettlement(t, fixture, newMemoryFinalAuthority(174))
		legacyMarker, err := domainevidence.NewHostEvidenceSettlementMarker(legacyPrepared)
		if err != nil || domainevidence.ValidateHostEvidenceSettlementMarker(legacyMarker) != nil {
			t.Fatalf("historical V1 fixture did not produce an audit-readable opaque marker: prepared=%#v err=%v", legacyPrepared, err)
		}
		thread := toolEvidenceDurableThread(input, legacyMarker, fixture.now.Add(3*time.Second))
		receipt, err := service.CommitCurrentToolEvidence(context.Background(), CommitToolEvidenceInput{
			Context: fixture.context, Marker: legacyMarker, Thread: thread,
		})
		if err == nil || receipt.ReceiptID != "" {
			t.Fatalf("V1 audit authority committed tool evidence: receipt=%#v err=%v", receipt, err)
		}
		if reader.calls != 0 || registry.calls != 0 || settlements.calls != 0 {
			t.Fatalf("V1 tool commit crossed a side-effect boundary: reader=%d registry=%d settlements=%d", reader.calls, registry.calls, settlements.calls)
		}
	})

	t.Run("claim verifier and registry projection", func(t *testing.T) {
		registry := &v1RejectionRegistrySpy{}
		proposal := domainevidence.ClaimProposal{
			SchemaVersion: domainevidence.ClaimProposalVersion, ProposalID: "proposal-v1-audit",
			ClaimType: domainevidence.ClaimAccount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{
				SubjectID: "entity-a", AccountID: "00123456789012345678", Granularity: "transaction",
			},
			EvidenceIDs: []string{"evr_legacy-audit-reference"}, CounterEvidenceIDs: []string{},
		}
		if _, err := domainevidence.NormalizeClaimProposal(proposal); err != nil {
			t.Fatalf("claim control is structurally invalid: %v", err)
		}
		verifier := ClaimVerifier{
			Registry: registry, Now: func() time.Time { return fixture.now.Add(3 * time.Second) },
			IDGenerator: func() (string, error) { return "claim-v1-audit", nil },
		}
		record, err := verifier.Verify(context.Background(), fixture.context, proposal)
		if err == nil || record.SchemaVersion != 0 {
			t.Fatalf("V1 audit authority verified a claim: record=%#v err=%v", record, err)
		}
		if registry.calls != 0 {
			t.Fatalf("V1 claim verifier called the registry %d times", registry.calls)
		}
		candidates, err := VerifiedPublicationCandidatesFromRegistrySnapshot(
			context.Background(), registry, fixture.context, fixture.now.Add(3*time.Second),
		)
		if err == nil || candidates.Claims != nil || candidates.NoHitReceiptIDs != nil {
			t.Fatalf("V1 audit authority projected publication candidates: candidates=%#v err=%v", candidates, err)
		}
		if registry.calls != 0 {
			t.Fatalf("V1 publication projection called the registry %d times", registry.calls)
		}
	})

	t.Run("final evidence gate", func(t *testing.T) {
		registry := &v1RejectionRegistrySpy{}
		envelope, err := (FinalEvidenceGate{Registry: registry}).Finalize(context.Background(), FinalGateInput{
			Context: fixture.context, TerminalReason: TerminalSuccess, IssuedAt: fixture.now.Add(4 * time.Second),
		})
		if err == nil || envelope.SchemaVersion != 0 {
			t.Fatalf("V1 audit authority entered the final evidence gate: envelope=%#v err=%v", envelope, err)
		}
		if registry.calls != 0 {
			t.Fatalf("V1 final gate called the registry %d times", registry.calls)
		}
	})

	t.Run("case publication finalizer", func(t *testing.T) {
		registry := &v1LockedSnapshotSpy{lockedMemoryEvidenceRegistry: &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}}
		privateStore := &memoryPrivateFinalStore{
			records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
		}
		authority := newMemoryFinalAuthority(173)
		finalizer := NewCasePublicationFinalizerWithAuthority(
			registry, registry, authority, privateStore, newTestFinalPublicationEventIO(), newTestTurnTerminalCoordinator(authority, privateStore),
		)
		result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
			Context: fixture.context, ThreadID: fixture.context.ThreadID, TurnID: fixture.context.TurnID,
			TerminalReason: TerminalSuccess, AcceptedAt: fixture.now.Add(4 * time.Second),
		})
		if err == nil || result.Boundary.Envelope.SchemaVersion != 0 {
			t.Fatalf("V1 audit authority entered case publication finalization: result=%#v err=%v", result, err)
		}
		if registry.snapshotCalls != 0 || privateStore.putCalls != 0 {
			t.Fatalf("V1 publication crossed a locked or private authority boundary: snapshots=%d private=%d", registry.snapshotCalls, privateStore.putCalls)
		}
	})
}

type v1LockedSnapshotSpy struct {
	*lockedMemoryEvidenceRegistry
	snapshotCalls int
}

func (registry *v1LockedSnapshotSpy) WithLockedSnapshot(ctx context.Context, securityContext domainsecurity.TurnSecurityContext, callback func(domainevidence.EvidenceReceiptRegistry) error) error {
	registry.snapshotCalls++
	return registry.lockedMemoryEvidenceRegistry.WithLockedSnapshot(ctx, securityContext, callback)
}

func TestEvidenceDomainConstructorsRejectStructurallyValidV1Authority(t *testing.T) {
	fixture := v1EvidenceExecutionFixture(t)
	assertStructurallyValidV1EvidenceFixture(t, fixture)
	preparedAt := fixture.now.Add(2 * time.Second)
	canonical, err := domainevidence.CanonicalEvidenceBytes(fixture.issue.Material.CanonicalEvidence)
	if err != nil {
		t.Fatal(err)
	}

	draftInput := evidenceReceiptDraftInput(fixture.issue, canonical, "pending", preparedAt)
	if draft, err := domainevidence.NewEvidenceReceiptDraft(draftInput); err == nil || draft.ReceiptID != "" {
		t.Fatalf("V1 audit authority constructed a new receipt draft: draft=%#v err=%v", draft, err)
	}

	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyDraft := v1RejectionReceiptDraft(draftInput)
	settlementInput := domainevidence.PreparedEvidenceSettlementInput{
		Context: fixture.context, Grant: fixture.grant,
		ActiveGrantRegistrySequence: fixture.grantRegistry.Sequence,
		ActiveGrantRegistryDigest:   fixture.grantRegistry.StateDigest,
		SourceProbe:                 fixture.probe, ToolOutcome: fixture.outcome,
		RawResult: fixture.issue.RawResult.RawResult, CanonicalEvidence: canonical,
		ReceiptDraft: legacyDraft, QueryHash: fixture.issue.Material.QueryHash,
		ResultItemID: fixture.issue.ResultItemID, PreparedAt: preparedAt,
		AuthorityKeyID: domainsecurity.SHA256Hex(publicKey), AuthorityPublicKey: publicKey,
	}
	settlementID := domainevidence.ComputeEvidenceSettlementID(settlementInput)
	legacyDraft.ReceiptID = domainevidence.EvidenceSettlementReceiptID(settlementID)
	legacyDraft = v1RejectionSealReceiptDraft(legacyDraft)
	settlementInput.ReceiptDraft = legacyDraft
	if domainevidence.ValidateEvidenceReceiptDraft(legacyDraft) != nil ||
		domainevidence.ComputeEvidenceSettlementID(settlementInput) != settlementID ||
		legacyDraft.ReceiptID != domainevidence.EvidenceSettlementReceiptID(settlementID) {
		t.Fatalf("legacy receipt control is not structurally valid: draft=%#v", legacyDraft)
	}
	if record, err := domainevidence.NewPreparedEvidenceSettlement(settlementInput, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	}); err == nil || record.ReceiptID != "" {
		t.Fatalf("V1 audit authority constructed a prepared settlement: record=%#v err=%v", record, err)
	}

	registry, err := domainevidence.NewEvidenceReceiptRegistry(fixture.context)
	if err != nil {
		t.Fatalf("structurally valid V1 audit registry could not be built: %v", err)
	}
	registry, sealedReceipt, err := domainevidence.RegisterEvidenceReceipt(
		registry, legacyDraft, canonical,
		domainevidence.EvidenceSettlementProof{
			SettlementID: settlementID, PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("v1-audit-prepared-record")),
		},
		preparedAt,
	)
	if err != nil || domainevidence.ValidateEvidenceReceipt(sealedReceipt) != nil {
		t.Fatalf("V1 audit publication fixture is invalid: receipt=%#v err=%v", sealedReceipt, err)
	}
	head, err := domainevidence.NewEvidenceRegistryHead(registry)
	if err != nil || domainevidence.ValidateEvidenceRegistryHead(head) != nil {
		t.Fatalf("V1 audit registry head is invalid: head=%#v err=%v", head, err)
	}
	proof, err := domainevidence.NewPublicationSnapshotProof(domainevidence.PublicationSnapshotProofInput{
		Context: fixture.context, RegistryHead: head, EvidenceReceiptIDs: []string{sealedReceipt.ReceiptID},
		Sources: []domainevidence.PublicationSourceSnapshot{{
			ReceiptID: sealedReceipt.ReceiptID, ServerID: fixture.probe.ServerID,
			ServerIdentity: fixture.probe.ServerIdentity, ServerVersion: fixture.issue.Material.ServerVersion,
			ConnectionEpoch: fixture.probe.ConnectionEpoch, ToolName: fixture.grant.ToolName,
			DatasetSnapshotID:  fixture.context.DatasetSnapshotID,
			CatalogFingerprint: fixture.probe.CatalogFingerprint, SpecFingerprint: fixture.probe.SpecFingerprint,
			ProbeDigest: fixture.probe.ProbeDigest, CheckedAt: fixture.probe.CheckedAt,
		}},
		CheckedAt: preparedAt.Add(time.Second),
	})
	if err == nil || proof.ProofDigest != "" {
		t.Fatalf("V1 audit authority constructed a publication proof: proof=%#v err=%v", proof, err)
	}
}

func TestV1PreparedEvidenceSettlementIsQuarantinedOnRestart(t *testing.T) {
	fixture := v1EvidenceExecutionFixture(t)
	authority := newMemoryFinalAuthority(173)
	prepared := v1RejectionPreparedSettlement(t, fixture, authority)
	if domainevidence.ValidatePreparedEvidenceSettlement(prepared) != nil {
		t.Fatal("V1 prepared settlement control is not structurally valid")
	}
	if domainevidence.ValidatePreparedEvidenceSettlementForExecution(prepared) == nil {
		t.Fatal("V1 prepared settlement unexpectedly has current execution authority")
	}
	registry := &memoryEvidenceRegistry{}
	issuer := Issuer{
		Registry: registry,
		SettlementStore: &memoryEvidenceSettlementStore{records: map[string]domainevidence.PreparedEvidenceSettlement{
			prepared.SettlementID: prepared,
		}},
		Authority: authority,
	}
	reader := settlementReaderWithoutResult(fixture.issue)
	inventory, err := PreflightEvidenceSettlementInventory(context.Background(), reader, issuer)
	if err != nil || len(inventory.Quarantined) != 1 || len(inventory.Pending) != 0 ||
		len(inventory.Committed) != 0 || len(inventory.Abandoned) != 0 {
		t.Fatalf("V1 prepared settlement was not isolated as audit-only: inventory=%#v err=%v", inventory, err)
	}
	if err := ApplyEvidenceSettlementReconciliationInventory(context.Background(), reader, issuer, inventory); err != nil {
		t.Fatal(err)
	}
	if registry.commitCalls != 0 || registry.initialized {
		t.Fatalf("V1 quarantine mutated the evidence registry: calls=%d initialized=%v", registry.commitCalls, registry.initialized)
	}
}

type v1EvidenceFixture struct {
	now           time.Time
	context       domainsecurity.TurnSecurityContext
	grant         domainsecurity.ExecutionGrant
	grantRegistry domainsecurity.ExecutionGrantRegistry
	call          domainmodel.ToolCall
	outcome       domainevidence.ToolOutcome
	probe         domainsecurity.VerifiedSourceProbe
	issue         IssueEvidenceInput
}

func v1EvidenceExecutionFixture(t *testing.T) v1EvidenceFixture {
	t.Helper()
	now := evidenceIssuerTime()
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-v1-audit", TurnID: "turn-v1-audit", WorkspaceRealPath: "/workspace",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, CaseID: "case-v1-audit",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("binding-v1-audit")),
		DatasetSnapshotID:  domainsecurity.DatasetSnapshotIDPrefixV1 + domainsecurity.SHA256Hex([]byte("v1-audit")),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-v1-audit")), ContextEpoch: 9, IssuedAt: now,
	})
	arguments := json.RawMessage(`{"table_name":"analysis_txn_detail_idx"}`)
	call := domainmodel.ToolCall{ID: evidenceTestHostToolCallID(t, "v1-audit"), Name: fundsCountEvidenceCanonicalTool, Arguments: arguments}
	identity := evidenceTestVerifiedMCPIdentity(t, "analytix_funds", "analytix_funds", fundsCountEvidenceServerVersion, 9)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider-v1-audit", ServerIdentity: identity,
		ToolName: call.Name, ToolCallID: call.ID, ConnectionEpoch: 9,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema-v1-audit")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope-v1-audit")), ReadOnly: true,
		ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	grantRegistry, err := domainsecurity.RegisterExecutionGrant(
		domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID), securityContext.ThreadID, grant, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: call.Name, ToolCallID: call.ID, ContextDigest: securityContext.ContextDigest,
		ExecutionGrantID: grant.GrantID, CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ServerIdentity: grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess,
		IssuedAt: now.Add(time.Second),
	})
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: identity, ConnectionEpoch: 9,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog-v1-audit")),
		SpecFingerprint:    domainsecurity.SHA256Hex([]byte("spec-v1-audit")),
		ThreadID:           securityContext.ThreadID, TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID, CheckedAt: now,
		Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: fundsCountEvidenceServerVersion,
			CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash,
			DatasetSnapshotID: securityContext.DatasetSnapshotID, Ready: true, ReadOnly: true,
			CheckedAt: now.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, issue := evidenceIssuerFixture(t)
	issue.Context = securityContext
	issue.Grant = grant
	issue.GrantRegistry = grantRegistry
	issue.Outcome = outcome
	issue.SourceProbe = probe
	issue.Material.ServerVersion = fundsCountEvidenceServerVersion
	issue.ResultItemID = domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, call.ID)
	issue.IssuedAt = now.Add(2 * time.Second)
	return v1EvidenceFixture{
		now: now, context: securityContext, grant: grant, grantRegistry: grantRegistry,
		call: call, outcome: outcome, probe: probe, issue: issue,
	}
}

func assertStructurallyValidV1EvidenceFixture(t *testing.T, fixture v1EvidenceFixture) {
	t.Helper()
	if fixture.context.Version != domainsecurity.TurnSecurityContextVersionV1 ||
		domainsecurity.ValidateTurnSecurityContext(fixture.context) != nil {
		t.Fatalf("V1 audit context is not structurally valid: %#v", fixture.context)
	}
	if domainsecurity.ValidateExecutionGrant(fixture.grant) != nil ||
		fixture.grant.TurnID != fixture.context.TurnID || fixture.grant.ContextDigest != fixture.context.ContextDigest {
		t.Fatalf("matching V1 execution grant is not structurally valid: %#v", fixture.grant)
	}
	if domainsecurity.ValidateExecutionGrantRegistry(fixture.grantRegistry) != nil ||
		domainsecurity.VerifyExecutionGrantMembership(
			fixture.grantRegistry, fixture.context.ThreadID, fixture.context.TurnID, fixture.grant, domainsecurity.GrantRegistryActive,
		) != nil {
		t.Fatalf("matching V1 grant registry is not structurally valid: %#v", fixture.grantRegistry)
	}
	if domainevidence.ValidateToolOutcome(fixture.outcome) != nil ||
		fixture.outcome.ContextDigest != fixture.context.ContextDigest || fixture.outcome.ExecutionGrantID != fixture.grant.GrantID ||
		fixture.outcome.ToolName != fixture.grant.ToolName || fixture.outcome.ToolCallID != fixture.grant.ToolCallID ||
		fixture.outcome.CaseID != fixture.context.CaseID || fixture.outcome.ContextEpoch != fixture.context.ContextEpoch ||
		fixture.outcome.DatasetSnapshotID != fixture.context.DatasetSnapshotID || fixture.outcome.ServerIdentity != fixture.grant.ServerIdentity {
		t.Fatalf("matching V1 tool outcome is not structurally valid: %#v", fixture.outcome)
	}
	if domainsecurity.ValidateVerifiedSourceProbe(fixture.probe) != nil ||
		fixture.probe.ThreadID != fixture.context.ThreadID || fixture.probe.TurnID != fixture.context.TurnID ||
		fixture.probe.ProbeContextDigest != fixture.context.ContextDigest || fixture.probe.CaseID != fixture.context.CaseID ||
		fixture.probe.CaseBindingHash != fixture.context.CaseBindingHash || fixture.probe.ContextEpoch != fixture.context.ContextEpoch ||
		fixture.probe.DatasetSnapshotID != fixture.context.DatasetSnapshotID || fixture.probe.ServerIdentity != fixture.grant.ServerIdentity ||
		fixture.probe.ConnectionEpoch != fixture.grant.ConnectionEpoch {
		t.Fatalf("matching V1 source probe is not structurally valid: %#v", fixture.probe)
	}
	if !domainmcp.ValidLosslessToolResult(fixture.issue.RawResult) ||
		domainsecurity.SHA256Hex(fixture.issue.RawResult.RawResult) != fixture.issue.RawResult.RawSHA256 {
		t.Fatalf("V1 fixture raw result is not structurally valid: %#v", fixture.issue.RawResult)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(fixture.issue.Material.CanonicalEvidence)
	if err != nil || domainsecurity.CanonicalJSONHash(canonical) == "" ||
		len(fixture.issue.Material.TransformationLineage) == 0 ||
		fixture.issue.Material.TransformationLineage[0].InputHash != fixture.issue.RawResult.RawSHA256 ||
		fixture.issue.Material.TransformationLineage[len(fixture.issue.Material.TransformationLineage)-1].OutputHash != domainsecurity.CanonicalJSONHash(canonical) {
		t.Fatalf("V1 fixture evidence material is not structurally valid: material=%#v err=%v", fixture.issue.Material, err)
	}
	if domainsecurity.ValidateTurnSecurityContextForExecution(fixture.context) == nil ||
		domainsecurity.ValidateExecutionGrantForContext(fixture.grant, fixture.context) == nil {
		t.Fatal("structurally valid V1 audit authority unexpectedly became live execution authority")
	}
}

func v1RejectionReceiptDraft(input domainevidence.EvidenceReceiptInput) domainevidence.EvidenceReceipt {
	receipt := domainevidence.EvidenceReceipt{
		SchemaVersion: domainevidence.EvidenceReceiptVersion, ReceiptID: input.ReceiptID,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, CaseID: input.Context.CaseID,
		CaseBindingHash: input.Context.CaseBindingHash, ContextEpoch: input.Context.ContextEpoch,
		ContextDigest: input.Context.ContextDigest, ExecutionGrantID: input.ExecutionGrantID, ToolCallID: input.ToolCallID,
		ServerIdentity: input.ServerIdentity, ServerVersion: input.ServerVersion, ConnectionEpoch: input.ConnectionEpoch,
		ToolName: input.ToolName, ArgsHash: input.ArgsHash, ResultHash: input.ResultHash, SourceType: input.SourceType,
		DatasetSnapshotID: input.DatasetSnapshotID, QueryHash: input.QueryHash, QueryRange: input.QueryRange,
		Granularity: input.Granularity, Currency: input.Currency, Timezone: input.Timezone,
		PaginationCompleteness: input.PaginationCompleteness,
		SourceRecordIDs:        append([]string(nil), input.SourceRecordIDs...), RawSHA256: input.RawSHA256,
		TransformationLineage: append([]domainevidence.TransformationLineageStep(nil), input.TransformationLineage...),
		PIIClassification:     input.PIIClassification, IssuedAt: input.IssuedAt.UTC().Format(time.RFC3339Nano),
	}
	return v1RejectionSealReceiptDraft(receipt)
}

func v1RejectionSealReceiptDraft(receipt domainevidence.EvidenceReceipt) domainevidence.EvidenceReceipt {
	receipt.ReceiptDigest = ""
	receipt.RegistrySequence = 0
	receipt.PreviousRegistryDigest = ""
	receipt.RegistryIntegrityProof = ""
	body, _ := json.Marshal(receipt)
	receipt.ReceiptDigest = domainsecurity.SHA256Hex(body)
	return receipt
}

func v1RejectionPreparedSettlement(t *testing.T, fixture v1EvidenceFixture, authority *memoryFinalAuthority) domainevidence.PreparedEvidenceSettlement {
	t.Helper()
	preparedAt := fixture.now.Add(2 * time.Second)
	canonical, err := domainevidence.CanonicalEvidenceBytes(fixture.issue.Material.CanonicalEvidence)
	if err != nil {
		t.Fatal(err)
	}
	draftInput := evidenceReceiptDraftInput(fixture.issue, canonical, "pending", preparedAt)
	draft := v1RejectionReceiptDraft(draftInput)
	input := domainevidence.PreparedEvidenceSettlementInput{
		Context: fixture.context, Grant: fixture.grant,
		ActiveGrantRegistrySequence: fixture.grantRegistry.Sequence,
		ActiveGrantRegistryDigest:   fixture.grantRegistry.StateDigest,
		SourceProbe:                 fixture.probe, ToolOutcome: fixture.outcome,
		RawResult: fixture.issue.RawResult.RawResult, CanonicalEvidence: canonical,
		ReceiptDraft: draft, QueryHash: fixture.issue.Material.QueryHash,
		ResultItemID: fixture.issue.ResultItemID, PreparedAt: preparedAt,
		AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey(),
	}
	settlementID := domainevidence.ComputeEvidenceSettlementID(input)
	draft.ReceiptID = domainevidence.EvidenceSettlementReceiptID(settlementID)
	draft = v1RejectionSealReceiptDraft(draft)
	record := domainevidence.PreparedEvidenceSettlement{
		SchemaVersion: domainevidence.PreparedEvidenceSettlementVersion, Purpose: domainevidence.EvidenceSettlementPurpose,
		AuthorityAlgorithm: domainevidence.AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: authority.KeyID(),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(authority.PublicKey()),
		SettlementID:       settlementID, ReceiptID: domainevidence.EvidenceSettlementReceiptID(settlementID),
		SecurityContext: fixture.context, ExecutionGrant: fixture.grant,
		ActiveGrantRegistrySequence: fixture.grantRegistry.Sequence,
		ActiveGrantRegistryDigest:   fixture.grantRegistry.StateDigest,
		SourceProbe:                 fixture.probe, ToolOutcome: fixture.outcome,
		RawResultBase64: base64.RawStdEncoding.EncodeToString(fixture.issue.RawResult.RawResult),
		RawSHA256:       fixture.issue.RawResult.RawSHA256, CanonicalEvidence: canonical,
		ReceiptDraft: draft, QueryHash: fixture.issue.Material.QueryHash,
		ResultItemID: fixture.issue.ResultItemID, PreparedAt: preparedAt.Format(time.RFC3339Nano),
	}
	signature, err := authority.Sign(context.Background(), domainevidence.EvidenceSettlementSigningBytes(record))
	if err != nil {
		t.Fatal(err)
	}
	record.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	record.RecordDigest = domainsecurity.SHA256Hex(body)
	if err := domainevidence.ValidatePreparedEvidenceSettlement(record); err != nil {
		t.Fatalf("could not build structurally valid V1 prepared settlement: %v", err)
	}
	return record
}

type v1RejectionRegistrySpy struct {
	calls int
}

func (spy *v1RejectionRegistrySpy) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	spy.calls++
	return domainevidence.EvidenceReceipt{}, errors.New("unexpected V1 registry commit")
}

func (spy *v1RejectionRegistrySpy) Resolve(context.Context, registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	spy.calls++
	return domainevidence.RegisteredEvidence{}, errors.New("unexpected V1 registry resolve")
}

func (spy *v1RejectionRegistrySpy) Revoke(context.Context, registryport.RevokeInput) error {
	spy.calls++
	return errors.New("unexpected V1 registry revoke")
}

func (spy *v1RejectionRegistrySpy) Replay(context.Context, domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	spy.calls++
	return domainevidence.EvidenceReceiptRegistry{}, errors.New("unexpected V1 registry replay")
}

type v1RejectionSettlementStoreSpy struct {
	calls int
}

func (spy *v1RejectionSettlementStoreSpy) PutPreparedIfAbsent(context.Context, domainevidence.PreparedEvidenceSettlement) error {
	spy.calls++
	return errors.New("unexpected V1 settlement write")
}

func (spy *v1RejectionSettlementStoreSpy) ResolvePrepared(context.Context, string) (domainevidence.PreparedEvidenceSettlement, error) {
	spy.calls++
	return domainevidence.PreparedEvidenceSettlement{}, errors.New("unexpected V1 settlement read")
}

func (spy *v1RejectionSettlementStoreSpy) ListPrepared(context.Context) ([]domainevidence.PreparedEvidenceSettlement, error) {
	spy.calls++
	return nil, errors.New("unexpected V1 settlement list")
}

func (spy *v1RejectionSettlementStoreSpy) HasRecords(context.Context) (bool, error) {
	spy.calls++
	return false, errors.New("unexpected V1 settlement inventory")
}

type v1RejectionEvidenceReaderSpy struct {
	calls int
}

func (spy *v1RejectionEvidenceReaderSpy) WithCurrentEvidenceRead(
	_ context.Context,
	_ sourceprobeport.EvidenceReadInput,
	_ func(domainsecurity.VerifiedSourceProbe, domainmcp.LosslessToolResult) error,
) error {
	spy.calls++
	return errors.New("unexpected V1 evidence read")
}
