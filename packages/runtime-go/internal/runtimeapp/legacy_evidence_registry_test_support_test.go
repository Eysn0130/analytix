package runtimeapp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	casethreadauthoritystore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	"analytix.local/runtime-go/internal/server"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func seedRuntimeLegacyEvidenceRegistryV1(
	t *testing.T,
	root string,
	authority finalauthorityport.Authority,
) (domainsecurity.TurnSecurityContext, domainevidence.EvidenceReceipt) {
	t.Helper()
	store, err := evidenceregistrystore.NewStore(root, authority)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, input := runtimeLegacyRegistryPreparedInputV1(t)
	receipt, err := store.CommitPrepared(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return securityContext, receipt
}

func runtimeLegacyRegistryPreparedInputV1(
	t *testing.T,
) (domainsecurity.TurnSecurityContext, registryport.CommitPreparedInput) {
	t.Helper()
	now := time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)
	workspace := t.TempDir()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-legacy-registry-v1", TurnID: "turn-legacy-registry-v1",
		WorkspaceRealPath: workspace, CaseID: "case-legacy-registry-v1",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("legacy-registry-v1-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("legacy-registry-v1-snapshot"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-manifest")),
		ContextEpoch:       3, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext, runtimeLegacyRegistryPreparedInputForContextV1(t, securityContext)
}

func runtimeLegacyRegistryPreparedInputForContextV1(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
) registryport.CommitPreparedInput {
	return runtimeLegacyRegistryPreparedInputForContextAndLabelV1(t, securityContext, "")
}

func runtimeLegacyRegistryPreparedInputForContextAndLabelV1(t *testing.T, securityContext domainsecurity.TurnSecurityContext, label string) registryport.CommitPreparedInput {
	t.Helper()
	now := time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)
	material, err := json.Marshal(domainevidence.CanonicalEvidenceMaterial{
		SchemaVersion: domainevidence.CanonicalEvidenceVersion,
		Facts: []domainevidence.CanonicalEvidenceFact{{
			FactID: "fact-legacy-registry-v1", ClaimType: domainevidence.ClaimAmount,
			NormalizedPayload: domainevidence.NormalizedClaimPayload{
				SubjectID: "entity-legacy-v1", EntityID: "entity-legacy-v1", AccountID: "00123456789012345678",
				AmountMinor: "4200000", Currency: "CNY", Direction: "out",
				StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z", Granularity: "transaction",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := domainevidence.CanonicalEvidenceBytes(material)
	if err != nil {
		t.Fatal(err)
	}
	serverIdentity, err := domainsecurity.NewVerifiedMCPServerIdentity(
		"analytix_funds", "analytix-fund-analysis", "1.0.0",
		domainsecurity.SHA256Hex([]byte("legacy-registry-v1-server")), 1,
	)
	if err != nil {
		t.Fatal(err)
	}
	settlement := domainevidence.EvidenceSettlementProof{
		SettlementID:         domainsecurity.SHA256Hex([]byte("legacy-registry-v1-settlement" + label)),
		PreparedRecordDigest: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-prepared" + label)),
	}
	draft, err := domainevidence.NewEvidenceReceiptDraft(domainevidence.EvidenceReceiptInput{
		ReceiptID: domainevidence.EvidenceSettlementReceiptID(settlement.SettlementID), Context: securityContext,
		ExecutionGrantID: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-grant")),
		ToolCallID:       toolidentitytest.MustHostToolCallIDV1("legacy-registry-v1-tool-call"),
		ServerIdentity:   serverIdentity, ServerVersion: "1.0.0", ConnectionEpoch: 1,
		ToolName: "mcp__analytix_funds__query", ArgsHash: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-args")),
		ResultHash: domainsecurity.CanonicalJSONHash(canonical), SourceType: "transactions",
		DatasetSnapshotID: securityContext.DatasetSnapshotID, QueryHash: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-query")),
		QueryRange: domainevidence.EvidenceQueryRange{
			EntityIDs: []string{"entity-legacy-v1"}, AccountIDs: []string{"00123456789012345678"},
			Directions: []string{"out"}, StartAt: "2026-01-01T00:00:00Z", EndAt: "2026-01-31T23:59:59Z",
			SourceIDs: []string{"flow-legacy-v1"}, FiltersHash: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-filters")),
		},
		Granularity: "transaction", Currency: "CNY", Timezone: "Asia/Shanghai",
		PaginationCompleteness: domainevidence.PaginationComplete,
		SourceRecordIDs:        []string{"row-legacy-v1"}, RawSHA256: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-raw")),
		TransformationLineage: []domainevidence.TransformationLineageStep{}, PIIClassification: domainevidence.PIIMasked,
		IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return registryport.CommitPreparedInput{
		Context: securityContext, Draft: draft, CanonicalEvidence: canonical,
		SettlementProof: settlement, RegisteredAt: now.Add(time.Minute),
	}
}

func seedRuntimeDurableLegacyRegistryContextV1(
	t *testing.T,
	durableRoot string,
	dataDir string,
	authority finalauthorityport.Authority,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	workspace := t.TempDir()
	durable, err := server.NewTempDurableEventSessionStore(durableRoot)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := durable.CreateThread(map[string]any{"title": "legacy registry V1 restart"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, _ := thread["id"].(string)
	turnID := "turn-legacy-registry-v1"
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: "case-legacy-registry-v1",
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("legacy-registry-v1-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("legacy-registry-v1-snapshot"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("legacy-registry-v1-manifest")),
		ContextEpoch:       3, IssuedAt: time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	contextBody, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := map[string]any{}
	if err := json.Unmarshal(contextBody, &contextRecord); err != nil {
		t.Fatal(err)
	}
	epoch, err := contextepochapp.PrepareTurn(contextepochapp.PrepareTurnInput{
		Thread: map[string]any{}, SecurityContext: securityContext, At: time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := durable.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "securityContext": contextRecord,
		"contextEpochSnapshot": contextepochapp.PublicSnapshot(epoch.State.AcceptedSnapshot), "items": []any{},
	}, "", map[string]any{"securityState": contextRecord, "contextEpochState": contextepochapp.PublicState(epoch.State)}); err != nil {
		t.Fatal(err)
	}
	privateRoot := filepath.Join(dataDir, "private")
	access, err := privatecastest.NewAccessAuthority(privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	caseStore, err := casethreadauthoritystore.NewStore(
		filepath.Join(privateRoot, "case-thread-authority"), access,
	)
	if err != nil {
		t.Fatal(err)
	}
	caseAuthority, err := casethreadapp.NewRegistry(context.Background(), authority, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	if err := caseAuthority.Register(context.Background(), securityContext); err != nil {
		t.Fatal(err)
	}
	if err := caseAuthority.Commit(
		context.Background(), securityContext, epoch.State, time.Date(2026, 8, 24, 3, 1, 0, 0, time.UTC),
	); err != nil {
		t.Fatal(err)
	}
	return securityContext
}
