package runtimeapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	casethreadauthority "analytix.local/runtime-go/internal/adapters/outbound/casethreadauthority"
	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
)

func TestCheckpointOriginalCaseObservationRejectsBoolOnlyAuthority(t *testing.T) {
	frozen, _ := operationRecoverySecurity(t, time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), t.TempDir(), "write_file", []byte(`{"path":"a.txt","content":"synthetic"}`))
	authority := checkpointRecoveryCaseAuthorityStub{caseThreads: map[string]bool{frozen.ThreadID: true}, contexts: map[string]bool{frozen.ContextDigest: true}}
	thread := map[string]any{"id": frozen.ThreadID, "turns": []any{map[string]any{"id": frozen.TurnID, "threadId": frozen.ThreadID, "securityContext": frozen}}}
	if err := validateCheckpointRecoveryThreadWithOriginalObservationV1(context.Background(), thread, authority, frozen, true); err == nil {
		t.Fatal("bool-only case authority bypassed original held inventory observation")
	}
	if err := validateCheckpointRecoveryThread(thread, authority, frozen); err != nil {
		t.Fatalf("ordinary existing context gate changed: %v", err)
	}
}

func TestRuntimeCheckpointRecoveryPreservesReportScopeAndSettlesIndependentGroup(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "legacy"}[legacy], func(t *testing.T) {
			ctx := context.Background()
			core, primary := runtimeReportPreservationFixtureV1(t, legacy, true, false)
			preserved, err := prepareRuntimeReportRestartPreservationV1(ctx, core)
			if err != nil || preserved.report == nil {
				t.Fatalf("root original report scope unavailable: %v", err)
			}
			frozen := preserved.report.Contexts()[0]
			access, err := privatecastest.NewAccessAuthority(filepath.Join(core.roots.DataDir, "private"))
			if err != nil {
				t.Fatal(err)
			}
			checkpointRoot := filepath.Join(core.roots.DataDir, "private", "checkpoint-authority")
			store, err := checkpointauthority.NewStoreContext(ctx, checkpointRoot, access)
			if err != nil {
				t.Fatal(err)
			}
			service := checkpointapp.OperationService{Authority: checkpointapp.SnapshotAuthority{Store: store}, Observer: filestore.CheckpointOperationObserver{}}
			now, err := time.Parse(time.RFC3339Nano, frozen.IssuedAt)
			if err != nil {
				t.Fatal(err)
			}
			arguments := []byte(`{"path":"a.txt","content":"after"}`)
			begin := func(frozen domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, label string) checkpointapp.OperationDraft {
				t.Helper()
				target := filepath.Join(frozen.WorkspaceRealPath, "a.txt")
				if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
					t.Fatal(err)
				}
				draft, err := service.Begin(ctx, checkpointapp.BeginOperationInput{
					SecurityContext: frozen, ExecutionGrant: grant,
					CheckpointID: domaincheckpointref.RuntimeID(label), SourceWorkspaceCheckpointID: label,
					Workspace: frozen.WorkspaceRealPath, ToolName: "write_file", ArgumentsJSON: arguments,
					Paths:     []checkpointapp.OperationPathRequest{{ResolvedPath: target, ArgumentKey: "path", RequestedPath: "a.txt", Role: "target", ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash("after")}},
					CreatedAt: now.Add(time.Second),
				})
				if err != nil {
					t.Fatal(err)
				}
				return draft
			}
			_, heldGrant := operationRecoverySecurity(t, now, frozen.WorkspaceRealPath, "write_file", arguments)
			heldGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
				Context: frozen, Provider: "synthetic-provider", ServerIdentity: "host:builtin", ToolName: "write_file", ToolCallID: heldGrant.ToolCallID,
				ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
				ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
			})
			held := begin(frozen, heldGrant, "held-checkpoint")
			independentContext, independentGrant := operationRecoverySecurity(t, now, t.TempDir(), "write_file", arguments)
			independent := begin(independentContext, independentGrant, "independent-checkpoint")
			heldID := held.AuthorityIntent.OperationGroupID
			heldPath := filepath.Join(checkpointRoot, "operation-group-intents-v2", heldID[:2], heldID+".json")
			before := startupWholeTreeRecordMapForTest(t, filepath.Dir(primary), frozen.WorkspaceRealPath)
			heldBody, err := os.ReadFile(heldPath)
			if err != nil {
				t.Fatal(err)
			}
			for iteration := range 2 {
				prepared, delta, err := recoverAuthenticatedCheckpointOperationsBeforeSemanticBaselineV1(ctx, persistencefs.NewStartupSnapshotReader(core.roots), core.roots.DataDir, access, nil, now.Add(2*time.Second), preserved.report)
				if err != nil || delta.HasChanges() != (iteration == 0) {
					t.Fatalf("root checkpoint recovery iteration=%d changed=%v err=%v", iteration, delta.HasChanges(), err)
				}
				if err := prepared.Verify(ctx, delta); err != nil {
					t.Fatal(err)
				}
				states, err := service.Authority.OperationGroups(ctx)
				if err != nil || len(states) != 2 {
					t.Fatalf("complete group inventory unavailable: count=%d err=%v", len(states), err)
				}
				for _, state := range states {
					if state.Intent.OperationGroupID == heldID && state.Terminal != nil {
						t.Fatal("held original group acquired a recovery terminal")
					}
					if state.Intent.OperationGroupID == independent.AuthorityIntent.OperationGroupID && (state.Terminal == nil || state.Terminal.Status != "no_effect") {
						t.Fatal("independent group did not complete its normal no-effect recovery")
					}
				}
				current, err := os.ReadFile(heldPath)
				if err != nil || string(current) != string(heldBody) || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, filepath.Dir(primary), frozen.WorkspaceRealPath)) {
					t.Fatal("held original authority, primary or workspace changed")
				}
			}
			// A separate, valid capture crash cut: the original held operation
			// has a durable completed terminal, but its capture event is absent.
			// The unscoped producer is used only to prepare this synthetic fixture.
			if err := os.WriteFile(filepath.Join(frozen.WorkspaceRealPath, "a.txt"), []byte("after"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Settle(ctx, held, true, now.Add(3*time.Second)); err != nil {
				t.Fatal(err)
			}
			original, err := preserved.report.ReadPrimaryThreadSnapshotV1(ctx, frozen.ThreadID)
			if err != nil {
				t.Fatal(err)
			}
			caseAuthority := checkpointRecoveryCaseAuthorityStub{caseThreads: map[string]bool{frozen.ThreadID: true}, contexts: map[string]bool{frozen.ContextDigest: true}}
			caseStore, err := casethreadauthority.NewStore(filepath.Join(core.roots.DataDir, "private", "case-thread-authority"), core.access)
			if err != nil {
				t.Fatal(err)
			}
			key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil {
				t.Fatal(err)
			}
			originalRegistry, err := casethreadapp.NewRegistry(ctx, key, caseStore)
			if err != nil {
				t.Fatal(err)
			}
			if err := originalRegistry.Register(ctx, frozen); err != nil {
				t.Fatal(err)
			}
			epoch, err := contextepochapp.BootstrapState(frozen.ThreadID, frozen.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(frozen)}, now)
			if err != nil {
				t.Fatal(err)
			}
			if err := originalRegistry.Commit(ctx, frozen, epoch, now.Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := originalRegistry.PreserveRestartScopeV1(ctx, preserved.report.ThreadIDs()); err != nil {
				t.Fatal(err)
			}
			if originalRegistry.ContainsContext(frozen) || originalRegistry.CanExecute(frozen.ThreadID) {
				t.Fatal("actual held registry retained execution or publication authority")
			}
			beforeCapture := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			for _, allowWrite := range []bool{false, true} {
				for name, observed := range map[string]checkpointRecoveryCaseAuthority{"fixture": caseAuthority, "actual_preserved_registry": originalRegistry} {
					events := newCheckpointRecoveryMemoryEvents(t, independentContext)
					events.threads[frozen.ThreadID] = original.Thread
					err := reconcileCheckpointOperationAuditsWithPreservationV1(ctx, allowWrite, service.Authority, events, observed, preserved)
					if name == "fixture" {
						if err == nil {
							t.Error("bool-only fixture bypassed original authority observation")
						}
					} else if err != nil {
						t.Errorf("held missing capture blocked original observation (%s, write=%v): %v", name, allowWrite, err)
					}
					if events.recordCalls != 0 || len(events.events[frozen.ThreadID]) != 0 {
						t.Error("held missing capture was synthesized into event history")
					}
					if !reflect.DeepEqual(beforeCapture, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
						t.Fatal("held capture observation changed original durable bytes")
					}
				}
			}
			// Give the independent group a missing capture too, so every held
			// rejection must precede an otherwise permitted independent write.
			independentCapture := begin(independentContext, independentGrant, "independent-capture")
			if err := os.WriteFile(filepath.Join(independentContext.WorkspaceRealPath, "a.txt"), []byte("after"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Settle(ctx, independentCapture, true, now.Add(4*time.Second)); err != nil {
				t.Fatal(err)
			}
			inventory, err := service.Authority.CapturedOperationInventoryForPreservedContextV1(ctx, frozen, held.AuthorityIntent.CheckpointID)
			if err != nil || len(inventory.Audits) != 1 {
				t.Fatalf("original held capture authority unavailable: %v", err)
			}
			projected, err := turnapp.SanitizeGenericCaseEventPublication(original.Thread, inventory.Audits[0].Event)
			if err != nil {
				t.Fatal(err)
			}
			for _, scenario := range []string{"exact", "duplicate", "different_payload", "unknown_marker", "missing_case_context"} {
				t.Run(scenario, func(t *testing.T) {
					body, _ := json.Marshal(projected)
					row := map[string]any{}
					if err := json.Unmarshal(body, &row); err != nil {
						t.Fatal(err)
					}
					row["seq"] = 1
					row["timestamp"] = now.Add(5 * time.Second).Format(time.RFC3339Nano)
					checkpoint := row["checkpoint"].(map[string]any)
					if scenario == "different_payload" {
						checkpoint["unrecognizedMetadata"] = "synthetic-conflict"
					}
					if scenario == "unknown_marker" {
						checkpoint["captureEventId"] = domaincheckpointref.CaptureEventID(domainsecurity.SHA256Hex([]byte("unrelated-frontier")))
					}
					checkpoint["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(checkpoint)
					body, _ = json.Marshal(row)
					body = append(body, '\n')
					if scenario == "duplicate" {
						row["seq"] = 2
						duplicate, _ := json.Marshal(row)
						body = append(append(body, duplicate...), '\n')
					}
					if err := os.WriteFile(filepath.Join(filepath.Dir(primary), "events.jsonl"), body, 0o600); err != nil {
						t.Fatal(err)
					}
					candidate := preserved
					candidate.durable, err = eventlog.PrepareSemanticRestartPreservationV1(ctx, core.roots.DurableDir, filepath.Join(core.roots.DurableDir, "thread_summaries.jsonl"), preserved.report.ThreadIDs())
					if err != nil {
						t.Fatalf("original event fixture was not independently valid: %v", err)
					}
					before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
					var currentAuthority checkpointRecoveryCaseAuthority = originalRegistry
					if scenario == "missing_case_context" {
						currentAuthority = checkpointRecoveryCaseAuthorityStub{caseThreads: caseAuthority.caseThreads, contexts: map[string]bool{}}
					}
					// Deliberately omit the held ID from the live store. Its full
					// original-family inventory must still be checked.
					events := newCheckpointRecoveryMemoryEvents(t, independentContext)
					err := reconcileCheckpointOperationAuditsWithPreservationV1(ctx, true, service.Authority, events, currentAuthority, candidate)
					if scenario == "exact" {
						if err != nil || events.recordCalls != 1 || len(events.events[independentContext.ThreadID]) != 1 {
							t.Fatalf("exact held capture prevented independent reconciliation: calls=%d err=%v", events.recordCalls, err)
						}
					} else if err == nil || events.recordCalls != 0 {
						t.Fatalf("invalid held capture crossed independent write boundary: calls=%d err=%v", events.recordCalls, err)
					}
					if len(events.events[frozen.ThreadID]) != 0 || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
						t.Fatal("original held capture inventory was rewritten")
					}
				})
			}
		})
	}
}
