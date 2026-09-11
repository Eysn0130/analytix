package evidencesettlement

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPrivateEvidenceSettlementStoreRejectsLegacySnapshotBeforePersistence(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	record := preparedStoreRecord(t, 3*time.Minute)
	legacySnapshotID := domainsecurity.DatasetSnapshotIDPrefixV1 + domainsecurity.SHA256Hex([]byte("settlement-store-legacy"))
	issuedAt, err := time.Parse(time.RFC3339Nano, record.SecurityContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	record.SecurityContext = domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: record.SecurityContext.ThreadID, TurnID: record.SecurityContext.TurnID,
		WorkspaceRealPath: record.SecurityContext.WorkspaceRealPath,
		TenantID:          record.SecurityContext.TenantID, UserID: record.SecurityContext.UserID,
		CaseID: record.SecurityContext.CaseID, CaseBindingHash: record.SecurityContext.CaseBindingHash,
		DatasetSnapshotID: legacySnapshotID, SourceManifestHash: record.SecurityContext.SourceManifestHash,
		ContextEpoch: record.SecurityContext.ContextEpoch, IssuedAt: issuedAt,
	})
	record.SourceProbe.DatasetSnapshotID = legacySnapshotID
	if !strings.HasPrefix(record.SecurityContext.DatasetSnapshotID, domainsecurity.DatasetSnapshotIDPrefixV1) ||
		domainsecurity.DatasetSnapshotFactAuthorityBlocker(record.SecurityContext.DatasetSnapshotID) != domainsecurity.SourceProbeBlockerDatasetSnapshotAuthorityUnavailable ||
		domainsecurity.SourceProbeCanAuthorizeFacts(record.SourceProbe) {
		t.Fatalf("fixture is not an audit-only legacy settlement: %#v", record.SecurityContext)
	}
	// Positive settlement persistence intentionally remains quarantined until a
	// real registry/content-root/producer-witness-backed DSV2 fixture exists.
	if err := store.PutPreparedIfAbsent(context.Background(), record); err == nil || err.Error() != "prepared evidence settlement is invalid" {
		t.Fatalf("legacy settlement reached private persistence: %v", err)
	}
	path := store.recordPath(record.SettlementID)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected legacy settlement created private state: %v", err)
	}
	restarted, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if records, err := restarted.ListPrepared(context.Background()); err != nil || len(records) != 0 {
		t.Fatalf("prepared settlement inventory mismatch: records=%#v err=%v", records, err)
	}
	if hasRecords, err := restarted.HasRecords(context.Background()); err != nil || hasRecords {
		t.Fatalf("rejected legacy settlement remained visible after restart: has=%v err=%v", hasRecords, err)
	}
}

func TestBareDSV2PreparedSettlementCannotUseLegacyStore(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record := preparedStoreRecord(t, 3*time.Minute)
	if domainsecurity.SourceProbeCanAuthorizeFacts(record.SourceProbe) {
		t.Fatal("fixture unexpectedly became bare DSV2 execution authority")
	}
	if err := store.PutPreparedIfAbsent(context.Background(), record); err == nil {
		t.Fatal("legacy store accepted a bare DSV2 prepared settlement")
	}
}

func TestPreparedSettlementInventoryPreservesDirectoriesAndExactAddress(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "read keeps directory mode", true: "extra address nesting"}[nested], func(t *testing.T) {
			store, err := NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			record := preparedStoreRecord(t, 3*time.Minute)
			body, err := domainevidence.PreparedEvidenceSettlementBytes(record)
			if err != nil {
				t.Fatal(err)
			}
			path := store.recordPath(record.SettlementID)
			if nested {
				path = filepath.Join(store.prepared, "extra", record.SettlementID[:2], record.SettlementID+".json")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" {
				if err := os.Chmod(filepath.Dir(path), 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(filepath.Dir(path), 0o700) })
			}
			before, err := os.Stat(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			records, readErr := store.ListPrepared(context.Background())
			after, err := os.Stat(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if before.Mode() != after.Mode() {
				t.Fatalf("inventory changed directory mode: before=%04o after=%04o", before.Mode().Perm(), after.Mode().Perm())
			}
			if nested && readErr == nil {
				t.Fatal("inventory accepted a prepared record outside its exact owner address")
			}
			if !nested && (readErr != nil || len(records) != 1) {
				t.Fatalf("original inventory unavailable: count=%d err=%v", len(records), readErr)
			}
		})
	}
}

func TestPrivateEvidenceSettlementInventoryFailsClosedOnUnknownSymlinkAndCorruption(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(store.prepared, "unknown.tmp")
	if err := os.WriteFile(unknown, []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListPrepared(context.Background()); err == nil {
		t.Fatal("unknown crash residue was silently ignored")
	}
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(store.prepared, "link")
		if err := os.Symlink(filepath.Dir(store.prepared), link); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ListPrepared(context.Background()); err == nil {
			t.Fatal("symlink in private settlement inventory was accepted")
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
	settlementID := domainsecurity.SHA256Hex([]byte("corrupt-empty-store-record"))
	corruptPath := store.recordPath(settlementID)
	if err := os.MkdirAll(filepath.Dir(corruptPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corruptPath, []byte(`{"schemaVersion":1`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolvePrepared(context.Background(), settlementID); err == nil {
		t.Fatal("truncated prepared settlement was accepted")
	}
}

func preparedStoreRecord(t *testing.T, preparedOffset time.Duration) domainevidence.PreparedEvidenceSettlement {
	return preparedStoreRecordForSuffixV1(t, preparedOffset, "")
}

func preparedStoreRecordForSuffixV1(t *testing.T, preparedOffset time.Duration, suffix string) domainevidence.PreparedEvidenceSettlement {
	t.Helper()
	base := time.Date(2026, 7, 10, 16, 0, 0, 0, time.UTC)
	policyDigest := domainsecurity.SHA256Hex([]byte("evidence-settlement-store-risk-policy"))
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: policyDigest, RiskClass: domainsecurity.RiskClassCase,
		Disposition: domainsecurity.PublicationDispositionCaseEvidenceGate, CaseBindingState: domainsecurity.CaseBindingStateValid,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte("evidence-settlement-store-binding-observation")),
		BlockerCode:              domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding("thread-store"+suffix, "/workspace", domainsecurity.RiskClassCase, policyDigest)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-store" + suffix, TurnID: "turn-store" + suffix, WorkspaceRealPath: "/workspace", CaseID: "case-store",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding")), DatasetSnapshotID: securitycontexttest.DatasetSnapshotID("settlement-store"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest")), ContextEpoch: 2, IssuedAt: base,
		PublicationPolicy: policy, RiskAuthorityBinding: binding,
	})
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
	grantRegistry, err := domainsecurity.RegisterExecutionGrant(domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID), securityContext.ThreadID, grant, base.Add(time.Minute))
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
	preparedAt := base.Add(preparedOffset)
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
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = 7
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	input.AuthorityKeyID = domainsecurity.SHA256Hex(publicKey)
	input.AuthorityPublicKey = publicKey
	record, err := domainevidence.NewPreparedEvidenceSettlement(input, func(message []byte) ([]byte, error) {
		return ed25519.Sign(privateKey, message), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}
