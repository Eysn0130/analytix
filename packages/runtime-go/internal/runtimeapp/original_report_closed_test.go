//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func TestRuntimeClosedPreWitnessReportRejectsUnknownAndErasedFinal(t *testing.T) {
	for _, fault := range []string{"unknown-disposition", "foreign-installation", "foreign-enrollment", "signed-final-erases-prefix"} {
		t.Run(fault, func(t *testing.T) {
			options := runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, preWitnessDisposition: domainpending.StatusFailed}
			if fault == "unknown-disposition" {
				options.preWitnessDisposition = domainpending.StatusOutcomeUnknown
			}
			if strings.HasPrefix(fault, "foreign-") {
				options.foreignReportIdentity = strings.TrimPrefix(fault, "foreign-")
			}
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, options)
			if fault == "signed-final-erases-prefix" {
				interruptRuntimeErasedMaterialsFinalForTestV1(t, fixture)
			}
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			attempts := fixture.witnessAttempts()
			if fault == "unknown-disposition" {
				// UNKNOWN is now a supported preserved prefix, but must never
				// enter this closed/no-hold failure route.
				guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(context.Background(), fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
				if err != nil || guard.publication.closed != nil || guard.publication.deferred == nil || !guard.report.OwnsThread(fixture.attempt.ThreadID) {
					t.Fatal("UNKNOWN acquired closed history or lost its hold", err)
				}
				if err := guard.ValidateSemanticOperationsV1(context.Background(), nil, nil, ""); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || fixture.witnessAttempts() != attempts {
					t.Fatal("UNKNOWN proof changed Original")
				}
				return
			}
			handler, err := NewRuntimeServerHandlerE(fixture.config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("invalid closed prefix allowed startup")
			}
			if fault == "signed-final-erases-prefix" && !strings.Contains(err.Error(), "closed report Original/Final prefix changed") {
				t.Fatalf("signed Final erasure did not reach full Original comparison: %v", err)
			}
			if strings.HasPrefix(fault, "foreign-") && !strings.Contains(err.Error(), "publication attempt installation anchor mismatch") {
				t.Fatalf("closed foreign identity did not reach its independent anchor: %v", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || fixture.witnessAttempts() != attempts {
				t.Fatal("closed prefix refusal wrote managed state or contacted a witness")
			}
		})
	}
}

func TestRuntimeClosedPreWitnessReportAllowsStartupWithoutThreadHold(t *testing.T) {
	for _, cut := range []publicationapp.RestartAttemptStateV1{publicationapp.RestartAttemptReservedV1, publicationapp.RestartAttemptCandidateDurableV1, publicationapp.RestartAttemptMaterialsDurableV1} {
		t.Run(string(cut), func(t *testing.T) {
			testRuntimeClosedPreWitnessStartsV1(t, cut, false, false, domainpending.StatusFailed)
		})
	}
}

func TestRuntimeClosedPreWitnessBeforeResultAllowsStartup(t *testing.T) {
	testRuntimeClosedPreWitnessStartsV1(t, publicationapp.RestartAttemptMaterialsDurableV1, true, false, domainpending.StatusFailed)
}

func TestRuntimeClosedPreWitnessBeforeResultWitnessOutage(t *testing.T) {
	testRuntimeClosedPreWitnessStartsV1(t, publicationapp.RestartAttemptMaterialsDurableV1, true, true, domainpending.StatusFailed)
}

func TestRuntimeClosedCancelledPreWitnessAllowsStartup(t *testing.T) {
	for _, beforeResult := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-cancelled-result", true: "before-result"}[beforeResult], func(t *testing.T) {
			testRuntimeClosedPreWitnessStartsV1(t, publicationapp.RestartAttemptMaterialsDurableV1, beforeResult, true, domainpending.StatusCancelled)
		})
	}
}

func testRuntimeClosedPreWitnessStartsV1(t *testing.T, cut publicationapp.RestartAttemptStateV1, beforeResult, offline bool, disposition string) {
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: cut, preWitnessDisposition: disposition, omitFailedResult: beforeResult})
	if offline && !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("fixture witness was not available to disable")
	}
	history, err := prepareRuntimeOriginalReportHistoryV1(context.Background(), fixture.core, fixture.publication, fixture.advance, fixture.scope)
	if err != nil || history == nil || len(history.plan.Attempts) != 1 || history.plan.Attempts[0].State != publicationapp.RestartAttemptAbortedV1 || history.plan.Attempts[0].Disposition == nil || history.plan.Attempts[0].Disposition.Status != disposition {
		t.Fatalf("native closed stage did not establish an aborted prefix: %v", err)
	}
	if disposition == domainpending.StatusCancelled && history.plan.Attempts[0].Disposition.ReasonCode != "report_stage_cancelled" {
		t.Fatal("cancelled stage lost its signed reason")
	}
	if fixture.scope.OwnsThread(fixture.attempt.ThreadID) {
		t.Fatal("closed report entered whole-thread execution hold")
	}
	workID := fixture.attempt.ReportStageWorkID
	pendingRoot := filepath.Join(fixture.core.roots.DataDir, "private", "pending-work")
	protected := []string{filepath.Join(pendingRoot, "receipts", workID[:2], workID+".json"), filepath.Join(pendingRoot, "dispositions", workID[:2], workID+".json")}
	for _, owner := range runtimePublicationOwnersV1 {
		protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", owner))
	}
	before := startupWholeTreeRecordMapForTest(t, protected...)
	attempts := fixture.witnessAttempts()
	if _, err := prepareRuntimeReportPreservationBeforeRecoveryV1(context.Background(), fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance); err != nil {
		t.Fatal(err)
	}
	if fixture.witnessAttempts() != attempts || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, protected...)) {
		t.Fatal("closed historical preservation had an effect")
	}
	sharedBefore, _ := fixture.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
	riskBefore, _ := fixture.witnessSnapshot(domainenrollment.ThreadRiskNamespaceV1)
	for restart := 0; restart < 2; restart++ {
		handler, err := NewRuntimeServerHandlerE(fixture.config)
		if err != nil {
			t.Fatalf("closed %s blocked native startup %d: %v", cut, restart, err)
		}
		shutdownOwnedRuntimeHandler(t, handler)
		after := startupWholeTreeRecordMapForTest(t, protected...)
		if !reflect.DeepEqual(before, after) {
			changed := []string{}
			for name, entry := range before {
				if other, found := after[name]; !found || !reflect.DeepEqual(entry, other) {
					changed = append(changed, "changed:"+name)
				}
			}
			for name := range after {
				if _, found := before[name]; !found {
					changed = append(changed, "added:"+name)
				}
			}
			sort.Strings(changed)
			t.Fatalf("closed startup delta: witness=%d paths=%v", fixture.witnessAttempts()-attempts, changed)
		}
		shared, _ := fixture.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
		risk, _ := fixture.witnessSnapshot(domainenrollment.ThreadRiskNamespaceV1)
		if shared.AdvanceCalls != sharedBefore.AdvanceCalls || shared.ResolveCalls != sharedBefore.ResolveCalls || risk.AdvanceCalls != riskBefore.AdvanceCalls || risk.ResolveCalls != riskBefore.ResolveCalls || !reflect.DeepEqual(shared.Checkpoint, sharedBefore.Checkpoint) || !reflect.DeepEqual(risk.Checkpoint, riskBefore.Checkpoint) {
			t.Fatal("closed startup advanced, resolved or changed a witness checkpoint")
		}
		observations := shared.ObserveCalls - sharedBefore.ObserveCalls + risk.ObserveCalls - riskBefore.ObserveCalls
		if offline {
			// The fixture rejects at HTTP availability before its successful
			// Observe counter. Exactly one failed attempt creates the fixed
			// first boundary; the second startup must not retry that work.
			if fixture.witnessAttempts()-attempts != 1 || observations != 0 {
				t.Fatalf("offline witness accounting: attempts=%d observations=%d", fixture.witnessAttempts()-attempts, observations)
			}
		} else if fixture.witnessAttempts()-attempts != observations {
			t.Fatalf("online witness accounting: attempts=%d observations=%d", fixture.witnessAttempts()-attempts, observations)
		}
		t.Logf("restart %d independent Core/history witness totals: attempts=%d shared_observe=%d risk_observe=%d", restart, fixture.witnessAttempts()-attempts, shared.ObserveCalls-sharedBefore.ObserveCalls, risk.ObserveCalls-riskBefore.ObserveCalls)
		body, err := os.ReadFile(fixture.primary)
		if err != nil {
			t.Fatal(err)
		}
		primary, err := finalauthority.ParsePrimaryThreadSnapshotV1(context.Background(), fixture.attempt.ThreadID, body)
		if err != nil {
			t.Fatal(err)
		}
		if err := history.validateClosedResultsV1(context.Background(), runtimeReportSemanticPrimariesV1{fixture.attempt.ThreadID: primary}); err != nil {
			t.Fatal(err)
		}
		result, err := runtimeClosedReportResultV1(history.plan.Attempts[0], primary.Thread)
		if err != nil || result == nil {
			t.Fatalf("failed Core grant did not settle: %v", err)
		}
		if disposition == domainpending.StatusCancelled {
			entry := history.plan.Attempts[0]
			expected, valid := executiongrantapp.ClosedReportRestartProjectionV1(entry.Stage, *entry.Disposition)
			actual, err := domaintoolresult.ParsePublicToolResultProjectionV1(result["output"])
			if !valid || err != nil || actual != expected || result["status"] != "failed" {
				t.Fatalf("cancelled result lost its exact projection/lifecycle: %v", err)
			}
		}
	}
}

func TestRuntimeClosedReportSemanticCandidatePreservesOriginalFailure(t *testing.T) {
	for _, fault := range []string{"harmless-title", "reclassify-failure", "oversized-original-result", "add-exact-restart-result", "add-private-body", "add-wrong-role", "post-intent-erased-result"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			options := runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, preWitnessDisposition: domainpending.StatusFailed, omitFailedResult: strings.HasPrefix(fault, "add-")}
			if fault == "post-intent-erased-result" {
				options = runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptDeliveryDecisionV1, restartStatus: domainpending.StatusFailed}
			}
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, options)
			if fault == "oversized-original-result" {
				body, err := os.ReadFile(fixture.primary)
				if err != nil {
					t.Fatal(err)
				}
				var primary map[string]any
				if err := json.Unmarshal(body, &primary); err != nil {
					t.Fatal(err)
				}
				resultID := domaintoolresult.ToolResultItemIDV1(fixture.attempt.TurnID, fixture.attempt.ToolCallID)
				for _, rawTurn := range primary["turns"].([]any) {
					for _, rawItem := range rawTurn.(map[string]any)["items"].([]any) {
						item := rawItem.(map[string]any)
						if item["id"] == resultID {
							item["legacyNote"] = strings.Repeat("x", (1<<20)+1)
						}
					}
				}
				body, err = json.Marshal(primary)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(fixture.primary, body, 0600); err != nil {
					t.Fatal(err)
				}
				fixture.refresh(t)
			}
			originalFiles := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			originalCalls := fixture.witnessAttempts()
			preserved, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
			if fault == "oversized-original-result" {
				// The existing Core result verifier rejects this record before
				// a closed historical digest can be established. It cannot be
				// treated as an accepted Original or an absent result.
				if err == nil || !strings.Contains(err.Error(), "report stage settled grant lacks an exact durable result") {
					t.Fatalf("oversized original bypassed its Core verifier: %v", err)
				}
				if fixture.witnessAttempts() != originalCalls || !reflect.DeepEqual(originalFiles, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
					t.Fatal("unverifiable original changed state")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			calls := fixture.witnessAttempts()
			snapshot, err := persistencefs.NewStartupSnapshotReader(fixture.core.roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("closed semantic " + fault))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			root, err := persistencefs.FreezeRootAuthority(fixture.core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(fixture.core.roots)
			if err != nil {
				t.Fatal(err)
			}
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(fixture.core.roots, root, journal, preserved)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				path := filepath.Join(stage.DurableDir, "threads", fixture.attempt.ThreadID, "thread.json")
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				var primary map[string]any
				if err := json.Unmarshal(body, &primary); err != nil {
					return err
				}
				primary["title"] = "ordinary metadata update"
				if fault == "post-intent-erased-result" {
					resultID := domaintoolresult.ToolResultItemIDV1(fixture.attempt.TurnID, fixture.attempt.ToolCallID)
					removed := false
					for _, raw := range primary["turns"].([]any) {
						turn := raw.(map[string]any)
						items := turn["items"].([]any)
						for i, item := range items {
							if item.(map[string]any)["id"] == resultID {
								turn["items"] = append(items[:i], items[i+1:]...)
								removed = true
								break
							}
						}
					}
					if !removed {
						return errors.New("actual failed Core result was absent before mutation")
					}
				}
				if strings.HasPrefix(fault, "add-") {
					entry := preserved.history.plan.Attempts[0]
					projection, valid := executiongrantapp.ClosedReportRestartProjectionV1(entry.Stage, *entry.Disposition)
					if !valid {
						return errors.New("closed fixture disposition invalid")
					}
					at, err := time.Parse(time.RFC3339Nano, entry.Disposition.DisposedAt)
					if err != nil {
						return err
					}
					stamp := at.Add(time.Second).Format(time.RFC3339Nano)
					records, err := toolcatalogapp.SettleToolResult(toolcatalogapp.ToolResultInput{
						ThreadID: entry.Stage.Context.ThreadID, TurnID: entry.Stage.Context.TurnID, CreatedAt: stamp, FinishedAt: stamp,
						Call:       domainmodel.ToolCall{ID: entry.Attempt.ToolCallID, Name: "stage_case_report", Arguments: json.RawMessage(`{}`)},
						Projection: projection, IsError: true, ContextDigest: entry.Stage.Context.ContextDigest, ContextEpoch: entry.Stage.Context.ContextEpoch, ExecutionGrantID: entry.Stage.GrantMembers[0].GrantID,
					})
					if err != nil {
						return err
					}
					if fault == "add-private-body" {
						records.ResultItem["content"] = "synthetic-private-body"
					}
					if fault == "add-wrong-role" {
						records.ResultItem["role"] = "assistant"
					}
					for _, rawTurn := range primary["turns"].([]any) {
						turn := rawTurn.(map[string]any)
						if turn["id"] == entry.Stage.Context.TurnID {
							turn["items"] = append(turn["items"].([]any), records.ResultItem)
						}
					}
				}
				if fault == "reclassify-failure" {
					result, err := runtimeClosedReportResultV1(preserved.history.plan.Attempts[0], primary)
					if err != nil {
						return err
					}
					result["output"] = domaintoolresult.PublicToolResultProjectionRecordV1(toolcatalogapp.BuildPublicToolResultProjectionV1("stage_case_report", map[string]any{"code": "tool_cancelled"}, true))
					// Replace only this exact result; the original issued grant
					// prefix and all identity/time fields are unchanged.
					for _, rawTurn := range primary["turns"].([]any) {
						turn := rawTurn.(map[string]any)
						for index, rawItem := range turn["items"].([]any) {
							if item := rawItem.(map[string]any); item["id"] == result["id"] {
								turn["items"].([]any)[index] = result
							}
						}
					}
				}
				body, err = json.Marshal(primary)
				if err != nil {
					return err
				}
				if err := os.WriteFile(path, body, 0600); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(stage.DataDir, "private", "a-closed-result-marker.bin"), []byte("independent marker"), 0600)
			})
			if err != nil {
				t.Fatal(err)
			}
			applyErr := prepared.Apply(ctx)
			if err := prepared.Close(); err != nil {
				t.Fatal(err)
			}
			if fault == "harmless-title" || fault == "add-exact-restart-result" {
				if applyErr != nil {
					t.Fatal(applyErr)
				}
			} else {
				want := "closed report original Core result changed"
				if strings.HasPrefix(fault, "add-") {
					want = "closed report added an unauthorized Core restart result"
				}
				if fault == "add-wrong-role" {
					want = "report stage settled grant lacks an exact durable result"
				}
				if fault == "post-intent-erased-result" {
					want = "closed post-intent report requires its actual failed Core result"
				}
				if applyErr == nil || !strings.Contains(applyErr.Error(), want) {
					t.Fatalf("failure reclassification was not refused by original digest: %v", applyErr)
				}
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
					t.Fatal("rejected signed candidate changed managed state")
				}
			}
			if fixture.witnessAttempts() != calls {
				t.Fatal("closed semantic validation contacted witness")
			}
		})
	}
}

func TestRuntimeClosedCompletedReportPrefixesAllowStartup(t *testing.T) {
	for _, cut := range []publicationapp.RestartAttemptStateV1{publicationapp.RestartAttemptStageDispositionV1, publicationapp.RestartAttemptStageCompletionV1} {
		t.Run(string(cut), func(t *testing.T) {
			ctx := context.Background()
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: cut})
			history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
			if err != nil || history == nil || len(history.plan.Attempts) != 1 {
				t.Fatal("native completed prefix full preflight failed", err)
			}
			entry := history.plan.Attempts[0]
			if entry.State != cut || entry.Disposition == nil || entry.Disposition.Status != domainpending.StatusCompleted || entry.Decision == nil || entry.GrantSettlement == nil ||
				(entry.StageCompletion != nil) != (cut == publicationapp.RestartAttemptStageCompletionV1) || entry.DeliveryOutcome != nil {
				t.Fatal("fixture did not stop at exact completed prefix")
			}
			if fixture.scope.OwnsThread(entry.Stage.Context.ThreadID) {
				t.Fatal("completed prefix held the whole thread")
			}
			if !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
				t.Fatal("could not disable witness")
			}
			workID := entry.Stage.WorkID
			protected := []string{filepath.Join(fixture.core.roots.DataDir, "private", "authority-advance")}
			for _, leaf := range []string{"receipts", "dispositions"} {
				protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", "pending-work", leaf, workID[:2], workID+".json"))
			}
			for _, owner := range runtimePublicationOwnersV1 {
				protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", owner))
			}
			before := startupWholeTreeRecordMapForTest(t, protected...)
			calls := fixture.witnessAttempts()
			for restart := 0; restart < 2; restart++ {
				whole := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
				handler, err := NewRuntimeServerHandlerE(fixture.config)
				if err != nil {
					if !reflect.DeepEqual(whole, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || calls != fixture.witnessAttempts() {
						t.Fatalf("completed prefix refusal: managedChanged=%t witnessRequests=%d error=%v", !reflect.DeepEqual(whole, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)), fixture.witnessAttempts()-calls, err)
					}
					t.Fatalf("completed prefix blocked startup %d: %v", restart, err)
				}
				shutdownOwnedRuntimeHandler(t, handler)
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, protected...)) {
					t.Fatal("completed prefix changed its original authority or missing suffix")
				}
				if history.semantic == nil {
					t.Fatal("completed prefix lacks semantic history qualification")
				}
				if err := history.semantic.withSemanticCandidateV1(ctx, nil, nil, "", nil); err != nil {
					t.Fatal("completed result lost exact historical graph", err)
				}
				if restart == 1 && fixture.witnessAttempts() != calls {
					t.Fatal("completed prefix retried witness on restart")
				}
				calls = fixture.witnessAttempts()
			}
		})
	}
}
