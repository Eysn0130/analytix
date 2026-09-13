//go:build darwin || linux

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	authorityanchorenv "analytix.local/runtime-go/internal/adapters/outbound/authorityanchorenv"
	authoritycredentialsfs "analytix.local/runtime-go/internal/adapters/outbound/authoritycredentialsfs"
	authoritymanifestfs "analytix.local/runtime-go/internal/adapters/outbound/authoritymanifestfs"
	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	evidenceauthoritystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceauthority"
	registrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	monotonicheadprojection "analytix.local/runtime-go/internal/adapters/outbound/monotonicheadprojection"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	registryapp "analytix.local/runtime-go/internal/app/evidenceregistry"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	authorityfixture "analytix.local/runtime-go/internal/formalauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	"analytix.local/runtime-go/internal/server"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// Provision one synthetic historical registration through the actual signed
// settlement, registry service, private CAS, and enrolled mTLS witness. This
// establishes a committed local Registry for the CLI outage probe; it is not
// native dataset admission, Provider execution, or Final Gate acceptance.
func runtimePrivateFrameSeedCommittedRegistryV1(t *testing.T, fixture *authorityfixture.Service, durableRoot, workspace string) {
	t.Helper()
	ctx := context.Background()
	privateRoot := filepath.Join(fixture.DataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	anchor := authorityanchorenv.Source{Lookup: func(name string) (string, bool) {
		return fixture.AnchorEnvelope, name == authorityanchorenv.AnchorEnvelopeV1Variable
	}}
	anchored, err := (authoritymanifestfs.Reader{Root: fixture.ManifestRoot, Name: "manifest.json", Anchor: anchor}).LoadAnchoredV2(ctx)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := domainenrollment.ProjectAnchoredManifestForNamespaceV2(anchored, domainenrollment.SharedEvidenceNamespaceV1)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := (authoritycredentialsfs.Loader{ProfileRoot: fixture.CredentialProfileRoot, BundleRoot: fixture.CredentialBundleRoot, Anchor: anchor}).LoadCurrent(ctx, anchored)
	if err != nil {
		t.Fatal(err)
	}
	witnessKey, err := base64.RawURLEncoding.DecodeString(projection.Enrollment.WitnessPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	bundles, err := evidenceauthoritystore.NewBundleStore(filepath.Join(privateRoot, "evidence-authority", "bundles"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer bundles.Close()
	observations, err := evidenceauthoritystore.NewObservationStore(filepath.Join(privateRoot, "evidence-authority", "observations"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer observations.Close()
	floor, err := monotonicheadprojection.New(monotonicheadprojection.Config{Root: filepath.Join(privateRoot, "evidence-checkpoint-floor"), InstallationID: projection.InstallationID, EnrollmentID: projection.Enrollment.EnrollmentID, Namespace: projection.Enrollment.Namespace, WitnessKeyID: projection.Enrollment.WitnessKeyID, WitnessPublicKey: witnessKey, InitialCheckpoint: projection.Enrollment.InitialCheckpoint})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := evidenceauthoritystore.NewProjection(filepath.Join(privateRoot, "evidence-authority-projection"), bundles)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := evidenceauthorityapp.New(evidenceauthorityapp.Config{InstallationID: fixture.InstallationID, EnrollmentID: projection.Enrollment.EnrollmentID, Authority: fixture.Authority, WitnessKeyID: projection.Enrollment.WitnessKeyID, WitnessPublicKey: witnessKey, Witness: credentials.SharedEvidence, CheckpointFloor: floor, Bundles: bundles, Observations: observations, Projection: projected, Genesis: evidenceauthorityapp.Genesis{DatasetSnapshotIndexDigest: domainsecurity.DatasetSnapshotIndexGenesisDigestV1(), EvidenceRegistryIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(), PublicationIndexDigest: domainpublication.PublicationIndexGenesisDigestV1()}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	indexes, err := registrystore.NewAuthorityIndexStoreV2(filepath.Join(privateRoot, "evidence-registry", "indexes"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer indexes.Close()
	capsules, err := registrystore.NewAuthorityCapsuleStoreV2(filepath.Join(privateRoot, "evidence-registry", "capsules"), access)
	if err != nil {
		t.Fatal(err)
	}
	defer capsules.Close()
	registry, err := registryapp.New(registryapp.Config{InstallationID: fixture.InstallationID, EnrollmentID: projection.Enrollment.EnrollmentID, Authority: fixture.Authority, WitnessKeyID: projection.Enrollment.WitnessKeyID, WitnessKey: witnessKey, Coordinator: coordinator, WitnessChain: coordinator, Indexes: indexes, Capsules: capsules})
	if err != nil {
		t.Fatal(err)
	}
	durable, err := server.NewProductionDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := durable.CreateThread(map[string]any{"title": "synthetic committed Registry history"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	observation, err := (filestore.CaseBindingReader{}).Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: threadID, TurnID: "turn-private-frame-registry", WorkspaceRealPath: observation.WorkspaceRealPath, CaseID: observation.CaseID, CaseBindingHash: observation.CaseBindingHash, ContextEpoch: 1, IssuedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	prepared := runtimePrivateFramePreparedRegistrySettlementV1(t, frozen, fixture.Authority)
	// Install canonical signed historical preparation, as the runtime original
	// registry preservation fixtures do. The live execution writer deliberately
	// rejects this legacy probe; no current Host capability is claimed here.
	preparedBody, err := domainevidence.PreparedEvidenceSettlementBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	preparedPath := filepath.Join(privateRoot, "evidence-settlements", "prepared", prepared.SettlementID[:2], prepared.SettlementID+".json")
	if err := os.MkdirAll(filepath.Dir(preparedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preparedPath, preparedBody, 0o600); err != nil {
		t.Fatal(err)
	}
	clear(preparedBody)
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil {
		t.Fatal(err)
	}
	preparedAt, err := time.Parse(time.RFC3339Nano, prepared.PreparedAt)
	if err != nil {
		t.Fatal(err)
	}
	input := registryport.CommitPreparedInput{Context: frozen, Draft: prepared.ReceiptDraft, CanonicalEvidence: prepared.CanonicalEvidence, SettlementProof: domainevidence.EvidenceSettlementProof{SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest}, RegisteredAt: preparedAt.Add(time.Minute)}
	receipt, err := registry.CommitPrepared(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := registry.Replay(ctx, frozen)
	if err != nil {
		t.Fatal(err)
	}
	issue, found, err := domainevidence.ResolvePreparedSettlementIssue(replayed, prepared)
	if err != nil || !found || issue.ReceiptID != receipt.ReceiptID {
		t.Fatal("real registry commit did not read back the exact prepared settlement")
	}
	head, err := coordinator.ObserveFresh(ctx)
	if err != nil || head.Bundle.EvidenceRegistryCount != 1 {
		t.Fatal("registry fixture did not commit exactly one witnessed registration")
	}
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{Thread: map[string]any{}, SecurityContext: frozen, At: at})
	if err != nil {
		t.Fatal(err)
	}
	grant := prepared.ExecutionGrant
	call := map[string]any{"id": domaintoolcall.ToolCallItemIDV1(frozen.TurnID, grant.ToolCallID), "kind": "tool_call", "role": "assistant", "status": "completed", "threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID, "contextDigest": frozen.ContextDigest, "contextEpoch": frozen.ContextEpoch, "executionGrantId": grant.GrantID, "executionGrant": grant, "arguments": domaintoolcall.WithheldArgumentsProjectionV1(), "createdAt": grant.IssuedAt}
	result := map[string]any{"id": prepared.ResultItemID, "kind": "tool_result", "role": "tool", "status": "completed", "threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID, "contextDigest": frozen.ContextDigest, "contextEpoch": frozen.ContextEpoch, "executionGrantId": grant.GrantID, "createdAt": input.RegisteredAt.Format(time.RFC3339Nano), "finishedAt": input.RegisteredAt.Format(time.RFC3339Nano), "isError": false, "hostEvidenceSettlement": marker}
	if err := durable.AppendTurnToThread(threadID, map[string]any{"id": frozen.TurnID, "threadId": threadID, "status": "running", "securityContext": frozen, "contextEpochSnapshot": contextepochapp.PublicSnapshot(epoch.State.AcceptedSnapshot), "items": []any{call, result}}, "", map[string]any{"securityState": frozen, "contextEpochState": contextepochapp.PublicState(epoch.State)}); err != nil {
		t.Fatal(err)
	}
	caseStore, err := casestore.NewStore(filepath.Join(privateRoot, "case-thread-authority"), access)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := casethreadapp.NewRegistry(ctx, fixture.Authority, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	if err := cases.Register(ctx, frozen); err != nil {
		t.Fatal(err)
	}
	if err := cases.Commit(ctx, frozen, epoch.State, at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func runtimePrivateFramePreparedRegistrySettlementV1(t *testing.T, securityContext domainsecurity.TurnSecurityContext, authority authorityport.Authority) domainevidence.PreparedEvidenceSettlement {
	t.Helper()
	base, err := time.Parse(time.RFC3339Nano, securityContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	toolName := "mcp__analytix_funds__count_case_rows"
	entropy := sha256.Sum256([]byte("analytix.evidence-settlement-store-test-tool-call/v1\x00legacy"))
	toolCallID, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity("analytix_funds", "analytix_funds", "0.16.16", domainsecurity.SHA256Hex([]byte("settlement-store-test-instance")), 2)
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: serverIdentity,
		ToolName: toolName, ToolCallID: toolCallID, ConnectionEpoch: 2,
		ArgsHash: domainsecurity.SHA256Hex([]byte("args")), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: base.Add(time.Minute), ExpiresAt: base.Add(10 * time.Minute),
	})
	before := domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID)
	grantRegistry, err := domainsecurity.RegisterExecutionGrant(before, securityContext.ThreadID, grant, base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
		ServerID: "analytix_funds", ServerIdentity: serverIdentity, ConnectionEpoch: 2,
		CatalogFingerprint: domainsecurity.SHA256Hex([]byte("catalog")), SpecFingerprint: domainsecurity.SHA256Hex([]byte("spec")),
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ContextEpoch: securityContext.ContextEpoch,
		ContextDigest: securityContext.ContextDigest, DatasetSnapshotID: securityContext.DatasetSnapshotID, CheckedAt: base, Response: domainsecurity.SourceProbeResponse{
			Version: domainsecurity.SourceProbeVersion, ServerName: "analytix_funds", ServerVersion: "0.16.16",
			CaseID: securityContext.CaseID, CaseBindingHash: securityContext.CaseBindingHash, DatasetSnapshotID: securityContext.DatasetSnapshotID,
			Ready: true, ReadOnly: true, CheckedAt: base.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	outcome := domainevidence.NewToolOutcome(domainevidence.ToolOutcomeInput{
		ToolName: toolName, ToolCallID: grant.ToolCallID, ContextDigest: securityContext.ContextDigest,
		ExecutionGrantID: grant.GrantID, CaseID: securityContext.CaseID, ContextEpoch: securityContext.ContextEpoch,
		DatasetSnapshotID: securityContext.DatasetSnapshotID, ServerIdentity: grant.ServerIdentity,
		TransportStatus: domainevidence.TransportSuccess, SemanticStatus: domainevidence.SemanticSuccess, IssuedAt: base.Add(2 * time.Minute),
	})
	canonical, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-count", ClaimType: domainevidence.ClaimCount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{
				SubjectID: "dataset:analysis_txn_detail_idx", EntityID: "dataset:analysis_txn_detail_idx",
				Count: "3", Granularity: "dataset_table_rows",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	preparedAt := base.Add(3 * time.Minute)
	queryHash := domainsecurity.SHA256Hex([]byte("query"))
	raw := []byte(`{"row_count":3}`)
	draftInput := domainevidence.EvidenceReceiptInput{
		ReceiptID: "pending", Context: securityContext, ExecutionGrantID: grant.GrantID, ToolCallID: grant.ToolCallID,
		ServerIdentity: serverIdentity, ServerVersion: "0.16.16", ConnectionEpoch: 2, ToolName: toolName,
		ArgsHash: grant.ArgsHash, ResultHash: domainsecurity.CanonicalJSONHash(canonical),
		SourceType: domainevidence.SourceTypeTransactionDatasetInventory, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		QueryHash: queryHash, QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"dataset:analysis_txn_detail_idx"}, AccountIDs: []string{}, Directions: []string{},
			SourceIDs: []string{"analysis_txn_detail_idx@snapshot-store"}, FiltersHash: domainsecurity.SHA256Hex([]byte("filters")),
		},
		Granularity: "dataset_table_rows", Timezone: "Asia/Shanghai", PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs: []string{"analysis_txn_detail_idx@snapshot-store"}, RawSHA256: domainsecurity.SHA256Hex(raw),
		TransformationLineage: []domainevidence.TransformationLineageStep{}, PIIClassification: domainevidence.PIINone, IssuedAt: preparedAt,
	}
	provisional, err := domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	input := domainevidence.PreparedEvidenceSettlementInput{
		Context: securityContext, Grant: grant, ActiveGrantRegistrySequence: grantRegistry.Sequence,
		ActiveGrantRegistryDigest: grantRegistry.StateDigest, SourceProbe: probe, ToolOutcome: outcome,
		RawResult: raw, CanonicalEvidence: canonical, ReceiptDraft: provisional, QueryHash: queryHash,
		ResultItemID: domaintoolresult.ToolResultItemIDV1(securityContext.TurnID, grant.ToolCallID), PreparedAt: preparedAt,
	}
	settlementID := domainevidence.ComputeEvidenceSettlementID(input)
	draftInput.ReceiptID = domainevidence.EvidenceSettlementReceiptID(settlementID)
	input.ReceiptDraft, err = domainevidence.NewEvidenceReceiptDraft(draftInput)
	if err != nil {
		t.Fatal(err)
	}
	input.AuthorityKeyID = authority.KeyID()
	input.AuthorityPublicKey = authority.PublicKey()
	record, err := domainevidence.NewPreparedEvidenceSettlement(input, func(body []byte) ([]byte, error) { return authority.Sign(context.Background(), body) })
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func runtimePrivateFrameAuthorityRootV1(t *testing.T) string {
	t.Helper()
	root := strings.TrimSpace(os.Getenv("ANALYTIX_TEST_PROFILE_ROOT"))
	if root == "" {
		// This ancestor becomes the public workspace; avoid numeric TempDir IDs.
		return workspacetest.New(t)
	}
	absolute, err := filepath.Abs(root)
	if err != nil || filepath.Clean(absolute) != absolute {
		t.Fatalf("isolated witnessed-registry test profile root is invalid: %v", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("isolated witnessed-registry test profile root is unsafe: info=%#v err=%v", info, err)
	}
	created, err := os.MkdirTemp(absolute, "private-frame-authority-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(created, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(created); err != nil {
			t.Errorf("cleanup isolated witnessed-registry test profile: %v", err)
		}
	})
	return created
}
