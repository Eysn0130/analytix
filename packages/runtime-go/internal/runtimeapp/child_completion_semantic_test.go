//go:build darwin || linux

package runtimeapp

import (
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	"analytix.local/runtime-go/internal/jobs"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type runtimeLiveChildVerifierProbeV1 struct {
	calls int
	err   error
}

func (probe *runtimeLiveChildVerifierProbeV1) VerifyStoredChildCompletion(context.Context, domainjob.Record) error {
	probe.calls++
	return probe.err
}

func TestRuntimeStoredChildConstructorRoutesOnlyExactHeldRecordsToOriginalProof(t *testing.T) {
	for _, independent := range []bool{false, true} {
		name := "held"
		if independent {
			name = "independent"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var core *runtimeChildIdentityStartupV1
			if independent {
				core = runtimeIndependentStoredChildFixtureV1(t)
			} else {
				core = runtimeStoredChildCompletionFixtureV1(t, "")
			}
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			input, err := preserved.childConstructorInputV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			failure := errors.New("synthetic live child authority failure")
			live := &runtimeLiveChildVerifierProbeV1{err: failure}
			verifier := preserved.childConstructorVerifierV1(live)
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			manager, err := jobs.NewManagerWithRestartPreservationV1(ctx, filepath.Join(core.roots.DataDir, "child-runs"), verifier, input)
			if independent {
				if !errors.Is(err, failure) || live.calls != 1 || manager != nil {
					t.Fatalf("independent constructor did not retain live authority error: calls=%d err=%v", live.calls, err)
				}
			} else {
				if err != nil || manager == nil || live.calls != 0 {
					t.Fatalf("held constructor borrowed live authority: calls=%d err=%v", live.calls, err)
				}
				if _, err := manager.LockChildRun(core.jobRecords[0].ID); !errors.Is(err, jobs.ErrRestartPreserved) {
					t.Fatalf("historical verification released held execution: %v", err)
				}
				for _, fault := range []string{"changed_record", "unrecorded_job", "cancelled"} {
					record, callCtx := core.jobRecords[0], ctx
					switch fault {
					case "changed_record":
						record.Status = "failed"
					case "unrecorded_job":
						record.ID = "job-99999"
					case "cancelled":
						cancelled, cancel := context.WithCancel(ctx)
						cancel()
						callCtx = cancelled
					}
					if err := verifier.VerifyStoredChildCompletion(callCtx, record); err == nil || fault == "cancelled" && !errors.Is(err, context.Canceled) {
						t.Fatalf("held constructor admitted %s: %v", fault, err)
					}
				}
				if live.calls != 0 {
					t.Fatal("held verification failure fell back to live authority")
				}
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("stored child constructor changed original files or modes")
			}
		})
	}
}

func runtimeIndependentStoredChildFixtureV1(t *testing.T, faults ...string) *runtimeChildIdentityStartupV1 {
	t.Helper()
	ctx := context.Background()
	fault := "complete"
	if len(faults) == 1 {
		fault = faults[0]
	} else if len(faults) > 1 {
		t.Fatal("conflicting child fixture faults")
	}
	core := runtimeStoredChildCompletionFixtureV1(t, fault)
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(body []byte) ([]byte, error) { return key.Sign(ctx, body) }
	var previous domainpendingwork.PendingWorkReceiptV1
	for _, receipt := range core.pendingInventory.Receipts {
		if receipt.Kind == domainpendingwork.KindReportStage {
			previous = receipt
		}
	}
	if previous.WorkID == "" {
		t.Fatal("fixture has no report scope")
	}
	// Move the synthetic unresolved report to the second reserved child. The
	// completed first child and its parent are then independent of that hold.
	for _, leaf := range []string{"receipts", "dispositions"} {
		if err := os.Remove(filepath.Join(core.roots.DataDir, "private", "pending-work", leaf, previous.WorkID[:2], previous.WorkID+".json")); err != nil {
			t.Fatal(err)
		}
	}
	parentSnapshot, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, core.jobRecords[0].ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	previousContext, err := appturn.FrozenSecurityContextForTurn(parentSnapshot.Thread, core.jobRecords[0].ParentTurnID)
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.Parse(time.RFC3339Nano, previousContext.IssuedAt)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{ThreadID: "thr_durable_701", TurnID: "turn_951", WorkspaceRealPath: previousContext.WorkspaceRealPath, TenantID: previousContext.TenantID, UserID: previousContext.UserID, CaseID: previousContext.CaseID, CaseBindingHash: previousContext.CaseBindingHash, DatasetSnapshotID: previousContext.DatasetSnapshotID, SourceManifestHash: previousContext.SourceManifestHash, ContextEpoch: 1, IssuedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"member": pendingworkapp.ReportStageToolName}
	argsBody, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	entropy := sha256.Sum256([]byte("synthetic-sibling-report-call"))
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	digest := func(label string) string {
		return domainsecurity.SHA256Hex([]byte("synthetic-sibling-report-" + label))
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: frozen, Provider: "synthetic-provider", ServerIdentity: "host:builtin", ToolName: pendingworkapp.ReportStageToolName, ToolCallID: callID, ArgsHash: domainsecurity.CanonicalJSONHash(argsBody), SchemaHash: digest("schema"), ScopeHash: digest("scope"), ApprovalState: "approved", IssuedAt: at, ExpiresAt: at.Add(time.Hour)})
	item := map[string]any{"id": domaintoolcall.ToolCallItemIDV1(frozen.TurnID, callID), "kind": "tool_call", "role": "assistant", "status": "completed", "threadId": frozen.ThreadID, "turnId": frozen.TurnID, "toolName": grant.ToolName, "callId": callID, "arguments": args, "createdAt": grant.IssuedAt, "contextDigest": frozen.ContextDigest, "contextEpoch": float64(frozen.ContextEpoch), "executionGrantId": grant.GrantID, "executionGrant": grant}
	thread := map[string]any{"id": frozen.ThreadID, "securityState": frozen, "turns": []any{map[string]any{"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "running", "securityContext": frozen, "items": []any{item}}}}
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
	member, ok := domainsecurity.ExecutionGrantRegistryEntryByID(registry, grant.GrantID)
	if !ok {
		t.Fatal("sibling report grant missing")
	}
	receipt, err := domainpendingwork.NewPendingWorkReceiptV1(domainpendingwork.ReceiptInputV1{Kind: domainpendingwork.KindReportStage, SecurityContext: frozen, GrantRegistrySequence: registry.Sequence, GrantRegistryDigest: registry.StateDigest, GrantMembers: []domainpendingwork.GrantMemberV1{{Ordinal: 1, GrantID: grant.GrantID, RegistrySequence: member.Sequence, RegistryEntryDigest: member.EntryDigest}}, PayloadHash: digest("payload"), RouteHash: digest("route"), IssuedAt: at.Add(time.Second), ExpiresAt: at.Add(30 * time.Minute), AuthorityKeyID: key.KeyID(), AuthorityPublicKey: key.PublicKey()}, sign)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(core.roots.DurableDir, "threads", frozen.ThreadID, "thread.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	body, err = domainpendingwork.PendingWorkReceiptV1Bytes(receipt)
	if err != nil {
		t.Fatal(err)
	}
	runtimeWritePendingSemanticBodyForTestV1(t, core.roots.DataDir, "receipts", receipt.WorkID, body)
	disposition, err := domainpendingwork.NewPendingWorkDispositionV1(receipt, domainpendingwork.StatusOutcomeUnknown, "report_stage_outcome_unknown_after_restart", at.Add(time.Minute), key.KeyID(), key.PublicKey(), sign)
	if err != nil {
		t.Fatal(err)
	}
	body, err = domainpendingwork.PendingWorkDispositionV1Bytes(disposition)
	if err != nil {
		t.Fatal(err)
	}
	runtimeWritePendingSemanticBodyForTestV1(t, core.roots.DataDir, "dispositions", receipt.WorkID, body)
	fresh, err := runtimeObservePendingCoreForTestV1(t, core)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	if scope.OwnsThread(core.jobRecords[0].ParentThreadID) || scope.OwnsThread(core.jobRecords[0].ChildThreadID) || !scope.OwnsThread(frozen.ThreadID) {
		t.Fatal("completed child is not independent of the report hold")
	}
	return fresh
}

func TestRuntimeStoredChildAuthenticatedRecoveryKeepsOriginalProof(t *testing.T) {
	for _, scenario := range []string{"independent_recovery", "future_context_repair", "applied_event_replacement"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			fault := "complete"
			if scenario == "future_context_repair" {
				fault = "missing_committed_child"
			}
			core := runtimeIndependentStoredChildFixtureV1(t, fault)
			const prefix, tail = "private/aa-before-completion-proof.txt", "private/z-completion-tail.txt"
			if scenario == "independent_recovery" {
				preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
				if err != nil {
					t.Fatal(err)
				}
				runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
					if err := os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("applied independent prefix"), 0o600); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, tail), []byte("recovered independent tail"), 0o600)
				}, prefix, tail, preserved)
			} else if scenario == "future_context_repair" {
				record := core.jobRecords[0]
				primary, err := core.primaries.ReadPrimaryThreadSnapshotV1(ctx, record.ChildThreadID)
				if err != nil {
					t.Fatal(err)
				}
				frozen, err := appturn.FrozenSecurityContextForTurn(primary.Thread, record.ChildTurnID)
				if err != nil {
					t.Fatal(err)
				}
				at, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
				if err != nil {
					t.Fatal(err)
				}
				state, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, at)
				if err != nil {
					t.Fatal(err)
				}
				key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
				if err != nil {
					t.Fatal(err)
				}
				committed, err := domainsecurity.NewCommittedTurnContextAuthorityRecord(frozen, state, at, key.KeyID(), key.PublicKey(), func(body []byte) ([]byte, error) { return key.Sign(ctx, body) })
				if err != nil {
					t.Fatal(err)
				}
				body, err := domainsecurity.CaseThreadAuthorityRecordBytes(committed)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join("private", "case-thread-authority", committed.RecordDigest[:2], committed.RecordDigest+".json")
				runtimePendingSemanticCutForTestV1(t, core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
					if err := os.MkdirAll(filepath.Dir(filepath.Join(stage.DataDir, path)), 0o700); err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(stage.DataDir, path), body, 0o600); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, prefix), []byte("applied independent prefix"), 0o600)
				}, prefix, path)
			} else {
				id := core.jobRecords[0].ChildThreadID
				path := filepath.Join(core.roots.DurableDir, "threads", id, "events.jsonl")
				remaining := filepath.Join(core.roots.DurableDir, "threads", id, "z-completion-tail.txt")
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				after := append([]byte("\t"), body...)
				runtimeSummaryReplacementCutForTestV1(t, core, path, remaining, after, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
					if err := os.WriteFile(filepath.Join(stage.DurableDir, "threads", id, "events.jsonl"), after, 0o600); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DurableDir, "threads", id, "z-completion-tail.txt"), []byte("remaining tail"), 0o600)
				})
			}
			fresh, err := runtimeObservePendingCoreForTestV1(t, core)
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, fresh)
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("original completion observation wrote state")
			}
			if scenario != "independent_recovery" {
				if err == nil {
					t.Fatal("future completion proof replaced original evidence")
				}
				return
			}
			if err != nil {
				t.Fatalf("independent completed-child recovery rejected: %v", err)
			}
			root, err := persistencefs.FreezeRootAuthority(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, preserved).RecoverAuthenticatedExisting(ctx); err != nil {
				t.Fatal(err)
			}
			after := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for path, original := range before {
				if !semanticOriginalRecordUnchangedForTestV1(original, after[path]) {
					t.Fatalf("independent completion recovery changed %s", path)
				}
			}
			body, err := os.ReadFile(filepath.Join(core.roots.DataDir, tail))
			if err != nil || string(body) != "recovered independent tail" {
				t.Fatal("independent completed-child recovery did not finish")
			}
		})
	}
}

func TestRuntimeStoredChildCandidateKeepsCompleteOriginalDependencies(t *testing.T) {
	core := runtimeIndependentStoredChildFixtureV1(t)
	ctx := context.Background()
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	finals, _, _, err := readRuntimeAcceptedFinalSemanticInventoryV1(ctx, core, preserved.report)
	if err != nil {
		t.Fatal(err)
	}
	operations := []domainstartup.SemanticStartupOperationV1{}
	for id := range finals.records {
		for _, leaf := range []string{"records", "dispositions"} {
			body, exists, err := finals.bodyV1(leaf, id)
			if err != nil || !exists {
				t.Fatal("complete fixture lacks original accepted pair")
			}
			operations = append(operations, domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationRemoveFile, Path: acceptedFinalSemanticPathV1(leaf, id), Before: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}, After: domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}})
		}
	}
	digest := domainsecurity.SHA256Hex([]byte("synthetic-candidate-completion-closure"))
	plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, operations)
	if err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	if err := preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, nil, ""); err == nil {
		t.Error("candidate removed a completed child's original authority while retaining its job")
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("candidate completion proof wrote state")
	}
}

func TestRuntimeStoredChildCandidateKeepsPublicCompletionProof(t *testing.T) {
	for _, scenario := range []string{"remove_events", "alter_events", "drop_parent_grant", "independent_title", "independent_file", "event_format", "spaced_child_id", "foreign_sibling_thread"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			core := runtimeIndependentStoredChildFixtureV1(t)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil {
				t.Fatal(err)
			}
			record := core.jobRecords[0]
			path := "durable/threads/" + record.ChildThreadID + "/events.jsonl"
			if scenario == "drop_parent_grant" {
				path = "durable/threads/" + record.ParentThreadID + "/thread.json"
			}
			if scenario == "independent_title" || scenario == "spaced_child_id" || scenario == "foreign_sibling_thread" {
				path = "durable/threads/" + record.ChildThreadID + "/thread.json"
			}
			if scenario == "independent_file" {
				path = "data/private/independent-child-proof.txt"
			}
			state := func(body []byte) domainstartup.SemanticEntryStateV1 {
				return domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeFile, Mode: 0o600, Size: int64(len(body)), SHA256: domainsecurity.SHA256Hex(body)}
			}
			beforeState := domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
			var body []byte
			if scenario != "independent_file" {
				body, err = os.ReadFile(filepath.Join(core.roots.DurableDir, path[len("durable/"):]))
				if err != nil {
					t.Fatal(err)
				}
				beforeState = state(body)
			}
			switch scenario {
			case "event_format":
				body = append([]byte("\t"), bytes.ReplaceAll(body[:len(body)-1], []byte("\n"), []byte("\n\t"))...)
				body = append(body, '\n')
			case "alter_events":
				lines := bytes.Split(body, []byte("\n"))
				var event map[string]any
				if err := json.Unmarshal(lines[0], &event); err != nil {
					t.Fatal(err)
				}
				event["timestamp"] = "2026-09-07T04:00:00Z"
				lines[0], err = json.Marshal(event)
				if err != nil {
					t.Fatal(err)
				}
				body = bytes.Join(lines, []byte("\n"))
			case "drop_parent_grant", "independent_title", "spaced_child_id", "foreign_sibling_thread":
				var thread map[string]any
				if err := json.Unmarshal(body, &thread); err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "drop_parent_grant":
					thread["turns"].([]any)[0].(map[string]any)["items"] = []any{}
				case "spaced_child_id":
					thread["id"] = " " + thread["id"].(string) + " "
				case "foreign_sibling_thread":
					thread["turns"] = append(thread["turns"].([]any), map[string]any{"id": "turn-sibling", "threadId": "thread-foreign", "status": "running", "items": []any{}})
				case "independent_title":
					thread["title"] = "independent title"
				}
				body, err = json.Marshal(thread)
				if err != nil {
					t.Fatal(err)
				}
			case "independent_file":
				body = []byte("independent progress")
			}
			operation := domainstartup.SemanticStartupOperationV1{Kind: domainstartup.SemanticOperationInstallFile, Path: path, Before: beforeState, After: state(body)}
			if scenario == "remove_events" {
				operation.Kind = domainstartup.SemanticOperationRemoveFile
				operation.After = domainstartup.SemanticEntryStateV1{Type: domainstartup.ManagedEntryTypeAbsent}
			}
			digest := domainsecurity.SHA256Hex([]byte("synthetic-candidate-public-completion"))
			plan, err := domainstartup.NewSemanticStartupPlanV1(digest, digest, digest, []domainstartup.SemanticStartupOperationV1{operation})
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			err = preserved.ValidateSemanticOperationsV1(ctx, plan.Operations, func(domainstartup.SemanticStartupOperationV1) ([]byte, error) { return body, nil }, "")
			if scenario == "independent_title" || scenario == "independent_file" || scenario == "event_format" {
				if err != nil {
					t.Fatalf("candidate blocked independent progress: %v", err)
				}
			} else if err == nil {
				t.Error("candidate replaced original public completion proof")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("candidate public proof wrote state")
			}
		})
	}
}

type childSemanticSeedFailureV1 struct {
	authorityport.Authority
	calls, failAt, signs int
	failure              error
}

func (authority *childSemanticSeedFailureV1) VerifyTrusted(ctx context.Context, keyID string, publicKey, body, signature []byte) error {
	authority.calls++
	if authority.calls == authority.failAt {
		return authority.failure
	}
	return authority.Authority.VerifyTrusted(ctx, keyID, publicKey, body, signature)
}

func (authority *childSemanticSeedFailureV1) Sign(context.Context, []byte) ([]byte, error) {
	authority.signs++
	return nil, errors.New("historical child observation attempted signing")
}

func (authority *childSemanticSeedFailureV1) ObserveProviderTurnBindingHMACV1(ctx context.Context, frozen domainsecurity.TurnSecurityContext) (string, error) {
	return authority.Authority.(interface {
		ObserveProviderTurnBindingHMACV1(context.Context, domainsecurity.TurnSecurityContext) (string, error)
	}).ObserveProviderTurnBindingHMACV1(ctx, frozen)
}

func TestRuntimeStoredChildVerificationPreservesSeedErrors(t *testing.T) {
	ctx := context.Background()
	core := runtimeStoredChildCompletionFixtureV1(t, "complete")
	preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
	if err != nil {
		t.Fatal(err)
	}
	verify := func(authority *childSemanticSeedFailureV1) error {
		copy := *core
		copy.verification = authority
		candidate := preserved
		candidate.core = &copy
		return (runtimeOriginalStoredChildVerifierV1{preserved: candidate}).VerifyStoredChildCompletion(ctx, core.jobRecords[0])
	}
	baseline := &childSemanticSeedFailureV1{Authority: core.verification}
	if err := verify(baseline); err != nil || baseline.signs != 0 || baseline.calls < 8 {
		t.Fatalf("healthy historical verification is unavailable: calls=%d signs=%d err=%v", baseline.calls, baseline.signs, err)
	}
	// The final eight signatures are the real terminal-complete seed's five
	// records, followed by stored completion's R/D/receipt signatures.
	for _, sentinel := range []error{errors.New("synthetic seed verification I/O failure"), context.Canceled} {
		for offset := 0; offset < 5; offset++ {
			authority := &childSemanticSeedFailureV1{Authority: core.verification, failAt: baseline.calls - 7 + offset, failure: sentinel}
			if err := verify(authority); !errors.Is(err, sentinel) {
				t.Errorf("terminal seed lost original verification error at %d: %v", offset, err)
			}
			if authority.signs != 0 {
				t.Fatal("historical child verification signed new authority")
			}
		}
	}
}

func TestRuntimeStoredChildCompletionRequiresOriginalPublicAndCommittedAuthority(t *testing.T) {
	for _, fault := range []string{"complete", "missing_committed_child", "missing_events", "altered_event", "foreign_receipt"} {
		t.Run(fault, func(t *testing.T) {
			core := runtimeStoredChildCompletionFixtureV1(t, fault)
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			_, err := prepareRuntimeReportRestartPreservationV1(context.Background(), core)
			if fault == "complete" {
				if err != nil {
					t.Fatalf("complete original stored child proof rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("incomplete stored child proof became trusted")
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("historical completion verification wrote original state")
			}
		})
	}
}
