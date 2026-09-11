//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestRuntimeOriginalV2RegistryHeldPrepareUsesAnchoredHistoricalInventory(t *testing.T) {
	ctx := context.Background()
	fixture, config := runtimeWitnessedRegistryConfigV2(t)
	config.ProductionDurableRoot = ""
	config.DurableTempDir = t.TempDir()
	roots, err := persistencefs.ResolveRootSet(config.DataDir, config.DurableTempDir)
	if err != nil {
		t.Fatal(err)
	}
	core, primaryPath := runtimeReportPreservationAtRootsFixtureV1(t, roots, false, true, false)
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	enrolled, configured, err := loadRuntimeSharedEvidenceEnrollmentV2(ctx, config, authority)
	if err != nil || !configured {
		t.Fatalf("fixture lacks independent installation enrollment: %v", err)
	}
	held := scope.Contexts()[0]
	frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: held.ThreadID, TurnID: "turn-original-v2-registry", WorkspaceRealPath: held.WorkspaceRealPath,
		CaseID: held.CaseID, CaseBindingHash: held.CaseBindingHash, ContextEpoch: held.ContextEpoch,
		DatasetSnapshotID: held.DatasetSnapshotID, SourceManifestHash: held.SourceManifestHash,
		IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	preparedRecord := runtimePreparedSettlementForSemanticTestV1(t, frozen, authority)
	preparedBody, err := domainevidence.PreparedEvidenceSettlementBytes(preparedRecord)
	if err != nil {
		t.Fatal(err)
	}
	preparedPath := filepath.Join(roots.DataDir, "private", "evidence-settlements", "prepared", preparedRecord.SettlementID[:2], preparedRecord.SettlementID+".json")
	if err := os.MkdirAll(filepath.Dir(preparedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preparedPath, preparedBody, 0o600); err != nil {
		t.Fatal(err)
	}
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(preparedRecord)
	if err != nil {
		t.Fatal(err)
	}
	preparedAt, err := time.Parse(time.RFC3339Nano, preparedRecord.PreparedAt)
	if err != nil {
		t.Fatal(err)
	}
	settledAt := preparedAt.Add(time.Minute)
	grant := preparedRecord.ExecutionGrant
	call := map[string]any{
		"id": domaintoolcall.ToolCallItemIDV1(frozen.TurnID, grant.ToolCallID), "kind": "tool_call", "role": "assistant", "status": "completed",
		"threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID,
		"contextDigest": frozen.ContextDigest, "contextEpoch": frozen.ContextEpoch, "executionGrantId": grant.GrantID,
		"executionGrant": grant, "arguments": domaintoolcall.WithheldArgumentsProjectionV1(), "createdAt": grant.IssuedAt,
	}
	result := map[string]any{
		"id": preparedRecord.ResultItemID, "kind": "tool_result", "role": "tool", "status": "completed",
		"threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID,
		"contextDigest": frozen.ContextDigest, "contextEpoch": frozen.ContextEpoch, "executionGrantId": grant.GrantID,
		"createdAt": settledAt, "finishedAt": settledAt, "isError": false, "hostEvidenceSettlement": marker,
	}
	primaryBody, err := os.ReadFile(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var primary map[string]any
	if err := json.Unmarshal(primaryBody, &primary); err != nil {
		t.Fatal(err)
	}
	primary["turns"] = append(primary["turns"].([]any), map[string]any{
		// This interruption occurred after the tool result and before final
		// publication; a completed case turn would require accepted-final proof.
		"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "running", "securityContext": frozen, "items": []any{call, result},
	})
	primaryBody, err = json.Marshal(primary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryPath, primaryBody, 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(frozen)
	if err != nil {
		t.Fatal(err)
	}
	registry, _, err = domainevidence.RegisterEvidenceReceipt(registry, preparedRecord.ReceiptDraft, preparedRecord.CanonicalEvidence, domainevidence.EvidenceSettlementProof{SettlementID: preparedRecord.SettlementID, PreparedRecordDigest: preparedRecord.RecordDigest}, settledAt)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
	capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(frozen, registry, authority.KeyID(), authority.PublicKey(), sign)
	if err != nil {
		t.Fatal(err)
	}
	index, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{
		InstallationID: enrolled.projection.InstallationID, EnrollmentID: enrolled.projection.Enrollment.EnrollmentID,
		Generation: 1, PreviousIndexDigest: domainevidence.EvidenceRegistryAuthorityIndexGenesisDigestV2(),
		MutationID: domainsecurity.SHA256Hex([]byte("original-v2-local-history")),
	}, capsule, authority.KeyID(), authority.PublicKey(), sign)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(roots.DataDir, "private", "evidence-registry")
	capsules, err := evidenceregistrystore.NewAuthorityCapsuleStoreV2(filepath.Join(root, "capsules"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := evidenceregistrystore.NewAuthorityIndexStoreV2(filepath.Join(root, "indexes"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := capsules.PutIfAbsent(ctx, capsule); err != nil {
		t.Fatal(err)
	}
	if err := indexes.PutIfAbsent(ctx, index); err != nil {
		t.Fatal(err)
	}
	// Both retries are real local candidates; neither a shared generation nor
	// a stored signed capsule identifies which append a witness selected.
	revoked, err := domainevidence.RevokeEvidenceReceipt(registry, preparedRecord.ReceiptID, "source_changed", settledAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	revokedCapsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(frozen, revoked, authority.KeyID(), authority.PublicKey(), sign)
	if err != nil {
		t.Fatal(err)
	}
	if err := capsules.PutIfAbsent(ctx, revokedCapsule); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"original-revoke-attempt", "original-revoke-retry"} {
		candidate, err := domainevidence.NewEvidenceRegistryAuthorityIndexV2(domainevidence.EvidenceRegistryAuthorityIndexInputV2{
			InstallationID: enrolled.projection.InstallationID, EnrollmentID: enrolled.projection.Enrollment.EnrollmentID,
			Generation: 2, PreviousIndexDigest: index.IndexDigest, MutationID: domainsecurity.SHA256Hex([]byte(label)),
		}, revokedCapsule, authority.KeyID(), authority.PublicKey(), sign)
		if err != nil {
			t.Fatal(err)
		}
		if err := indexes.PutIfAbsent(ctx, candidate); err != nil {
			t.Fatal(err)
		}
	}
	before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
	attempts := fixture.TotalAttempts()
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	prepared, prepareErr := PrepareRuntimeServerStartupWithPersistenceLeaseE(config, lease)
	if fixture.TotalAttempts() != attempts {
		t.Error("original V2 observation acquired fresh witness authority")
	}
	after := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
	for name, record := range before {
		if !semanticOriginalRecordUnchangedForTestV1(record, after[name]) {
			t.Errorf("original V2 preparation changed original record %s", name)
		}
	}
	if !reflect.DeepEqual(before, after) && prepareErr != nil {
		// Composite-journal bootstrap is allowed before the reported defect;
		// every pre-existing entry remains checked above.
		t.Log("failed preparation added startup-owned journal state")
	}
	if prepareErr != nil {
		t.Fatalf("anchored original V2 local history blocked held startup: %v", prepareErr)
	}
	assertOriginal := func() {
		t.Helper()
		after := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
		for name, record := range before {
			if !semanticOriginalRecordUnchangedForTestV1(record, after[name]) {
				t.Fatalf("original V2 startup changed original record %s", name)
			}
		}
		if fixture.TotalAttempts() != attempts {
			t.Fatal("original V2 startup acquired fresh witness authority")
		}
	}
	handler, err := prepared.Activate(config)
	if err != nil {
		t.Fatalf("original V2 held startup activation: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, handler)
	assertOriginal()
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("original V2 held startup fresh restart: %v", err)
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	assertOriginal()
}
