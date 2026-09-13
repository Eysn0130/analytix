package runtimeapp

import (
	telemetrystore "analytix.local/runtime-go/internal/adapters/outbound/cachetelemetrystore"
	casestore "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	terminalstore "analytix.local/runtime-go/internal/adapters/outbound/turnterminalstore"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	appturn "analytix.local/runtime-go/internal/app/turn"
	contracts "analytix.local/runtime-go/internal/contracts"
	domaintelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	domainturnterminal "analytix.local/runtime-go/internal/domain/turnterminal"
	"analytix.local/runtime-go/internal/jobs"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	terminaltest "analytix.local/runtime-go/internal/testsupport/turnterminal"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func runtimeReportPreservationFixtureV1(t *testing.T, legacy, unknown, missingGrant bool) (*runtimeChildIdentityStartupV1, string) {
	t.Helper()
	roots, err := persistencefs.ResolveRootSet(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return runtimeReportPreservationAtRootsFixtureV1(t, roots, legacy, unknown, missingGrant)
}

func runtimeReportPreservationAtRootsFixtureV1(t *testing.T, roots persistencefs.RootSet, legacy, unknown, missingGrant bool, supplied ...domainsecurity.TurnSecurityContext) (*runtimeChildIdentityStartupV1, string) {
	t.Helper()
	ctx := context.Background()
	authority, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(filepath.Join(roots.DataDir, "private"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thread-report-held", TurnID: "turn-report-held", WorkspaceRealPath: workspacetest.New(t), CaseID: "case-report", CaseBindingHash: domainsecurity.SHA256Hex([]byte("report-binding")), ContextEpoch: 1, IssuedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(supplied) > 1 {
		t.Fatal("ambiguous original held context fixture")
	}
	if len(supplied) == 1 {
		frozen = supplied[0]
	}
	arguments := map[string]any{"member": pendingworkapp.ReportStageToolName}
	argumentBytes, _ := json.Marshal(arguments)
	entropy := sha256.Sum256([]byte("synthetic-root-report-call"))
	if len(supplied) == 1 {
		entropy = sha256.Sum256([]byte("synthetic-root-report-call/" + frozen.ContextDigest))
	}
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: frozen, Provider: "synthetic-provider", ServerIdentity: "host:builtin", ToolName: pendingworkapp.ReportStageToolName, ToolCallID: callID, ArgsHash: domainsecurity.CanonicalJSONHash(argumentBytes), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err := domainsecurity.ValidateExecutionGrant(grant); err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": domaintoolcall.ToolCallItemIDV1(frozen.TurnID, grant.ToolCallID), "kind": "tool_call", "role": "assistant", "status": "completed", "threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": grant.ToolCallID, "arguments": arguments, "createdAt": grant.IssuedAt, "contextDigest": frozen.ContextDigest, "contextEpoch": float64(frozen.ContextEpoch), "executionGrantId": grant.GrantID, "executionGrant": grant}
	thread := map[string]any{"id": frozen.ThreadID, "securityState": frozen, "turns": []any{map[string]any{"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "running", "securityContext": frozen, "items": []any{item}}}}
	// Re-read the actual JSON representation used by the original primary.
	body, _ := json.Marshal(thread)
	if err := json.Unmarshal(body, &thread); err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(frozen.ThreadID, thread, frozen.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !ok {
		t.Fatal("fixture lacks original grant")
	}
	sign := func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) }
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{Kind: domainpendingwork.KindReportStage, SecurityContext: frozen, GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest, GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest}}, PayloadHash: domainsecurity.SHA256Hex([]byte("synthetic-report-payload")), RouteHash: domainsecurity.SHA256Hex([]byte("synthetic-report-route")), IssuedAt: now.Add(time.Second), ExpiresAt: now.Add(30 * time.Minute), AuthorityKeyID: authority.KeyID(), AuthorityPublicKey: authority.PublicKey()}, sign)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaf := range []string{"receipts", "dispositions"} {
		cas, err := finalauthority.OpenSecurePrivateCASWithAccessAuthority(filepath.Join(roots.DataDir, "private", "pending-work", leaf), 1<<20, access)
		if err != nil {
			t.Fatal(err)
		}
		if leaf == "receipts" {
			body, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if err := cas.PutIfAbsent(ctx, receipt.WorkID, body); err != nil {
				t.Fatal(err)
			}
		}
		if leaf == "dispositions" && unknown {
			disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusOutcomeUnknown, "report_stage_outcome_unknown_after_restart", now.Add(time.Minute), authority.KeyID(), authority.PublicKey(), sign)
			if err != nil {
				t.Fatal(err)
			}
			body, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
			if err != nil {
				t.Fatal(err)
			}
			if err := cas.PutIfAbsent(ctx, receipt.WorkID, body); err != nil {
				t.Fatal(err)
			}
		}
		if err := cas.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if missingGrant {
		thread["turns"].([]any)[0].(map[string]any)["items"] = []any{}
		body, _ = json.Marshal(thread)
	}
	primaryRoot := roots.DurableDir
	if legacy {
		primaryRoot = filepath.Join(primaryRoot, "runtime-go")
	}
	primaryPath := filepath.Join(primaryRoot, "threads", frozen.ThreadID, "thread.json")
	if len(supplied) == 1 {
		if previous, readErr := os.ReadFile(primaryPath); readErr == nil {
			var original map[string]any
			if err := json.Unmarshal(previous, &original); err != nil {
				t.Fatal(err)
			}
			for _, raw := range original["turns"].([]any) {
				if raw.(map[string]any)["id"] == frozen.TurnID {
					t.Fatal("duplicate original fixture turn")
				}
			}
			original["turns"] = append(original["turns"].([]any), thread["turns"].([]any)...)
			original["securityState"] = frozen
			body, err = json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
		} else if !os.IsNotExist(readErr) {
			t.Fatal(readErr)
		}
	}
	if err := os.MkdirAll(filepath.Dir(primaryPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	rootAuthority, err := persistencefs.FreezeRootAuthority(roots)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareRuntimeChildIdentityStartupV1(ctx, roots, rootAuthority, access, nil)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, primaryPath
}

func runtimeStoredChildCompletionFixtureV1(t *testing.T, fault string) *runtimeChildIdentityStartupV1 {
	t.Helper()
	ctx := context.Background()
	core, _ := runtimeChildRestartScopeFixtureV1(t, false, false)
	record := core.jobRecords[0]
	parentSnapshot, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	childSnapshot, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, record.ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := appturn.FrozenSecurityContextForTurn(parentSnapshot.Thread, record.ParentTurnID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := appturn.FrozenSecurityContextForTurn(childSnapshot.Thread, record.ChildTurnID)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, child.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(body []byte) ([]byte, error) { return key.Sign(ctx, body) }
	caseStore, err := casestore.NewStore(filepath.Join(core.roots.DataDir, "private", "case-thread-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := casethreadapp.NewRegistry(ctx, key, caseStore)
	if err != nil {
		t.Fatal(err)
	}
	for _, frozen := range []domainsecurity.TurnSecurityContext{parent, child} {
		if err := registry.Register(ctx, frozen); err != nil {
			t.Fatal(err)
		}
		if fault == "missing_committed_child" && frozen == child {
			continue
		}
		state, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Commit(ctx, frozen, state, at); err != nil {
			t.Fatal(err)
		}
	}
	fixture, err := terminaltest.NewFixtureWithAuthorityV1(child, at.Add(10*time.Second), key.PublicKey(), sign)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := appturn.BuildAcceptedFinalPublicationPlan(fixture.PrivateFinal.AcceptedFinal, fixture.PrivateFinal.RenderedText, fixture.PrivateFinal.PublicationIntent)
	if err != nil {
		t.Fatal(err)
	}
	turn := childSnapshot.Thread["turns"].([]any)[0].(map[string]any)
	for field, value := range plan.TurnFields {
		turn[field] = value
	}
	turn["acceptedFinal"], turn["items"] = fixture.PrivateFinal.AcceptedFinal, plan.TurnItems
	turn["status"] = fixture.PrivateFinal.PublicationIntent.TerminalStatus
	body, err := json.Marshal(childSnapshot.Thread)
	if err != nil {
		t.Fatal(err)
	}
	childDir := filepath.Join(core.roots.DurableDir, "threads", child.ThreadID)
	if err := os.WriteFile(filepath.Join(childDir, "thread.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	events := []map[string]any{}
	var log bytes.Buffer
	for index, event := range plan.Events {
		value := contracts.CloneMap(event.Draft)
		value["seq"] = index + 1
		events = append(events, value)
		if err := json.NewEncoder(&log).Encode(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := appturn.ValidateAcceptedFinalDurableReadbackV1(events, events, false); err != nil {
		t.Fatal(err)
	}
	if fault != "missing_events" {
		if fault == "altered_event" {
			events[0]["timestamp"] = at.Add(time.Hour).Format(time.RFC3339Nano)
			log.Reset()
			for _, event := range events {
				if err := json.NewEncoder(&log).Encode(event); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := os.WriteFile(filepath.Join(childDir, "events.jsonl"), log.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := finalauthority.NewAcceptedFinalCASReader(core.roots.DurableDir)
	if err != nil {
		t.Fatal(err)
	}
	observations, err := reader.ReadAcceptedFinalCASObservations(ctx, child.ThreadID, []string{child.TurnID})
	if err != nil {
		t.Fatal(err)
	}
	bindingObserver := core.verification.(interface {
		ObserveProviderTurnBindingHMACV1(context.Context, domainsecurity.TurnSecurityContext) (string, error)
	})
	hmac, err := bindingObserver.ObserveProviderTurnBindingHMACV1(ctx, child)
	if err != nil {
		t.Fatal(err)
	}
	closedAt := at.Add(12 * time.Second)
	closure, err := domaintelemetry.NewProviderTurnClosureV1(domaintelemetry.ProviderTurnClosureInputV1{TurnBindingHMAC: hmac, Intents: []domaintelemetry.ProviderAttemptIntentV1{}, Settlements: []domaintelemetry.ProviderAttemptSettlementV1{}, TerminalReasonCode: fixture.Intent.TerminalReasonCode, ClosedAt: closedAt, AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey()}, sign)
	if err != nil {
		t.Fatal(err)
	}
	disposition, err := domainevidence.NewAcceptedFinalDispositionRecordV2(domainevidence.AcceptedFinalDispositionInput{AcceptedFinal: fixture.PrivateFinal.AcceptedFinal, State: domainevidence.AcceptedFinalCommitted, EventManifestDigest: plan.EventManifestDigest, DecidedAt: closedAt, AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey()}, observations[child.TurnID], domainevidence.AcceptedFinalDecisionSamePublicWinner, sign)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := domainturnterminal.NewTurnTerminalDispositionV1(domainturnterminal.TurnTerminalDispositionInputV1{Intent: fixture.Intent, ProviderClosure: closure, AcceptedFinalDisposition: disposition, AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey()}, sign)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture must satisfy the real production terminal-complete index.
	index := gateprojection.NewTrustedFinalProjectionIndex(key)
	if err := index.SeedTerminalComplete(ctx, []gateprojection.TerminalCompleteFinalAuthorityV1{{PrivateFinal: fixture.PrivateFinal, Intent: fixture.Intent, ProviderClosure: closure, PublicObservation: observations[child.TurnID], AcceptedFinalDisposition: disposition, TerminalDisposition: terminal}}); err != nil {
		t.Fatal(err)
	}
	finals, err := finalauthority.NewPrivateStore(filepath.Join(core.roots.DataDir, "private", "accepted-finals"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := finals.PutIfAbsent(ctx, fixture.PrivateFinal); err != nil {
		t.Fatal(err)
	}
	if err := finals.PutDispositionIfAbsent(ctx, disposition); err != nil {
		t.Fatal(err)
	}
	terminals, err := terminalstore.NewStore(filepath.Join(core.roots.DataDir, "private", "turn-terminal-authority"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := terminals.PutIntentIfAbsent(ctx, fixture.Intent); err != nil {
		t.Fatal(err)
	}
	if err := terminals.PutDispositionIfAbsent(ctx, terminal); err != nil {
		t.Fatal(err)
	}
	telemetry, err := telemetrystore.NewStore(filepath.Join(core.roots.DataDir, "private", "provider-cache-telemetry"), core.access)
	if err != nil {
		t.Fatal(err)
	}
	if err := telemetry.PutClosureIfAbsent(ctx, closure); err != nil {
		t.Fatal(err)
	}
	receiptKey := key
	if fault == "foreign_receipt" {
		receiptKey, err = finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "key.json"), false)
		if err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := domainjob.NewChildCompletionReceiptV1(domainjob.ChildCompletionReceiptInputV1{ChildRunID: record.ID, SecurityBinding: record.SecurityBinding, ParentContext: parent, ChildContext: child, AcceptedFinal: fixture.PrivateFinal.AcceptedFinal, CanContinueParent: false, IssuedAt: at.Add(time.Minute), AuthorityKeyID: receiptKey.KeyID(), AuthorityPublicKey: receiptKey.PublicKey()}, func(body []byte) ([]byte, error) { return receiptKey.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	record.Status, record.ChildCompletionReceipt = "completed", &receipt
	body, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core.roots.DataDir, "child-runs", record.ID+".json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	return fresh
}

func runtimePreparedSettlementForSemanticTestV1(t *testing.T, securityContext domainsecurity.TurnSecurityContext, authority authorityport.Authority, prefix ...domainsecurity.ExecutionGrantRegistry) domainevidence.PreparedEvidenceSettlement {
	t.Helper()
	return runtimePreparedSettlementWithDraftForSemanticTestV1(t, securityContext, authority, nil, prefix...)
}

func runtimePreparedSettlementWithDraftForSemanticTestV1(t *testing.T, securityContext domainsecurity.TurnSecurityContext, authority authorityport.Authority, prepareDraft func(*domainevidence.EvidenceReceiptInput) ([]byte, []byte), prefix ...domainsecurity.ExecutionGrantRegistry) domainevidence.PreparedEvidenceSettlement {
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
	if len(prefix) > 1 {
		t.Fatal("ambiguous actual evidence grant prefix")
	}
	before := domainsecurity.NewExecutionGrantRegistry(securityContext.ThreadID)
	if len(prefix) == 1 {
		before = prefix[0]
	}
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
	if prepareDraft != nil {
		raw, canonical = prepareDraft(&draftInput)
		draftInput.RawSHA256 = domainsecurity.SHA256Hex(raw)
		draftInput.ResultHash = domainsecurity.CanonicalJSONHash(canonical)
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

func runtimeChildRestartScopeFixtureV1(t *testing.T, legacy, wrongIssuedPrefix bool) (*runtimeChildIdentityStartupV1, []domainpendingwork.ChildProducerTargetV1) {
	t.Helper()
	ctx := context.Background()
	core, primaryPath := runtimeReportPreservationFixtureV1(t, legacy, true, false)
	parent, err := os.ReadFile(primaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var thread map[string]any
	if err := json.Unmarshal(parent, &thread); err != nil {
		t.Fatal(err)
	}
	turn := thread["turns"].([]any)[0].(map[string]any)
	frozen, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	targets := []domainpendingwork.ChildProducerTargetV1{
		{Ordinal: 1, JobID: "job-900", ChildThreadID: "thr_durable_700", ChildTurnID: "turn_950"},
		{Ordinal: 2, JobID: "job-901", ChildThreadID: "thr_durable_701", ChildTurnID: "turn_951"},
	}
	args := map[string]any{"tasks": []any{map[string]any{"prompt": "synthetic one"}, map[string]any{"prompt": "synthetic two"}}}
	argsBody, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	entropy := sha256.Sum256([]byte("synthetic-original-child-scope-call"))
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	digest := func(label string) string { return domainsecurity.SHA256Hex([]byte("synthetic-child-scope-" + label)) }
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: frozen, Provider: "synthetic-provider", ServerIdentity: "host:builtin", ToolName: "parallel_tasks", ToolCallID: callID, ArgsHash: domainsecurity.CanonicalJSONHash(argsBody), SchemaHash: digest("schema"), ScopeHash: digest("scope"), ApprovalState: "approved", IssuedAt: at.Add(time.Second), ExpiresAt: at.Add(time.Hour)})
	binding, err := domainjob.NewSecurityBinding(frozen, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	item := map[string]any{"id": domaintoolcall.ToolCallItemIDV1(frozen.TurnID, callID), "kind": "tool_call", "role": "assistant", "status": "running", "threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": callID, "arguments": args, "createdAt": grant.IssuedAt, "contextDigest": frozen.ContextDigest, "contextEpoch": float64(frozen.ContextEpoch), "executionGrantId": grant.GrantID, "executionGrant": grant}
	turn["items"] = append(turn["items"].([]any), item)
	body, err := json.Marshal(thread)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &thread); err != nil {
		t.Fatal(err)
	}
	registry, err := executiongrantapp.RegistryFromThread(frozen.ThreadID, thread, frozen.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !ok {
		t.Fatal("original child grant is absent")
	}
	if err := os.WriteFile(primaryPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	registryDigest := registry.StateDigest
	if wrongIssuedPrefix {
		registryDigest = digest("wrong-issued-prefix")
	}
	sign := func(body []byte) ([]byte, error) { return key.Sign(ctx, body) }
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{Kind: domainpendingwork.KindSideEffectIntent, SecurityContext: frozen, GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registryDigest, GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: entry.Sequence, RegistryEntryDigest: entry.EntryDigest}}, PayloadHash: digest("payload"), RouteHash: digest("route"), IssuedAt: at.Add(2 * time.Second), ExpiresAt: at.Add(time.Minute), AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey(), ChildProducer: &domainpendingwork.ChildProducerV1{ParentBindingDigest: binding.BindingDigest, Children: targets}}, sign)
	if err != nil {
		t.Fatal(err)
	}
	receiptBody, err := domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	runtimeWritePendingSemanticBodyForTestV1(t, core.roots.DataDir, "receipts", receipt.WorkID, receiptBody)
	// A closed parent receipt still protects every allocated child identity.
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusCancelled, "tool_call_cancelled", at.Add(time.Minute), key.KeyID(), key.PublicKey(), sign)
	if err != nil {
		t.Fatal(err)
	}
	dispositionBody, err := domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	runtimeWritePendingSemanticBodyForTestV1(t, core.roots.DataDir, "dispositions", receipt.WorkID, dispositionBody)
	child, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: targets[0].ChildThreadID, TurnID: targets[0].ChildTurnID, WorkspaceRealPath: frozen.WorkspaceRealPath, TenantID: frozen.TenantID, UserID: frozen.UserID, CaseID: frozen.CaseID, CaseBindingHash: frozen.CaseBindingHash, DatasetSnapshotID: frozen.DatasetSnapshotID, SourceManifestHash: frozen.SourceManifestHash, ContextEpoch: 1, IssuedAt: at.Add(3 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	childThread := map[string]any{"id": child.ThreadID, "securityState": child, "turns": []any{map[string]any{"id": child.TurnID, "threadId": child.ThreadID, "status": "running", "securityContext": child, "items": []any{}}}}
	childBody, err := json.Marshal(childThread)
	if err != nil {
		t.Fatal(err)
	}
	childPath := filepath.Join(filepath.Dir(filepath.Dir(primaryPath)), child.ThreadID, "thread.json")
	if err := os.MkdirAll(filepath.Dir(childPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, childBody, 0o600); err != nil {
		t.Fatal(err)
	}
	producerRoot := t.TempDir()
	producer, err := jobs.NewManagerWithChildCompletionVerifier(producerRoot, nil, domainpendingwork.ChildIdentityFloorsV1{JobSequence: 899})
	if err != nil {
		t.Fatal(err)
	}
	request := jobs.StartRequest{ParentGoalID: "goal-child-scope", Kind: "subagent", Status: "queued", ParentThreadID: binding.ParentThreadID, ParentTurnID: binding.ParentTurnID, ParentToolCallID: binding.ParentToolCallID, SecurityBinding: binding, ChildThreadID: child.ThreadID, ChildTurnID: child.TurnID, Workspace: frozen.WorkspaceRealPath}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &request)
	record, err := producer.StartChildRun(request)
	if err != nil || record.ID != targets[0].JobID {
		t.Fatalf("synthetic child producer failed: %v", err)
	}
	jobBody, err := os.ReadFile(filepath.Join(producerRoot, record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(core.roots.DataDir, "child-runs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(core.roots.DataDir, "child-runs", record.ID+".json"), jobBody, 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	return fresh, targets
}

func runtimeWritePendingSemanticBodyForTestV1(t *testing.T, data, leaf, id string, body []byte) {
	t.Helper()
	path := filepath.Join(data, "private", "pending-work", leaf, id[:2], id+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runtimeObservePendingCoreForTestV1(t *testing.T, core *runtimeChildIdentityStartupV1) (*runtimeChildIdentityStartupV1, error) {
	t.Helper()
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	root, err := persistencefs.FreezeRootAuthority(core.roots)
	if err != nil {
		t.Fatal(err)
	}
	next, err := prepareRuntimeChildIdentityStartupV1(context.Background(), core.roots, root, core.access, nil)
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("fresh pending Core observation changed original state")
	}
	return next, err
}
