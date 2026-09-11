//go:build darwin || linux

package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

// All records use one independently enrolled installation. Two report turns
// share a primary; the other completed thread remains independently usable.
func runtimeMultiReportInventoryFixtureV1(t *testing.T) []*runtimeOriginalReservedReportHistoryFixtureV1 {
	return runtimeReportInventorySamplesV1(t, []runtimeReportInventorySampleV1{
		{"thread-inventory-independent", "turn-independent", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptStageDispositionV1}},
		{"thread-inventory-shared", "turn-closed", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptStageCompletionV1}},
		{"thread-inventory-shared", "turn-open", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptCommittedSettlementV1}},
	})
}

type runtimeReportInventorySampleV1 struct {
	thread, turn string
	options      runtimeOriginalReportHistoryFixtureOptionsV1
}

func runtimeReportInventorySamplesV1(t *testing.T, samples []runtimeReportInventorySampleV1) []*runtimeOriginalReservedReportHistoryFixtureV1 {
	t.Helper()
	witness, config := runtimeWitnessedRegistryConfigV2(t)
	workspace := t.TempDir()
	var fixtures []*runtimeOriginalReservedReportHistoryFixtureV1
	for i, sample := range samples {
		frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: sample.thread, TurnID: sample.turn, WorkspaceRealPath: workspace,
			CaseID: "case-inventory", CaseBindingHash: domainsecurity.SHA256Hex([]byte("inventory-binding")), ContextEpoch: 1,
			IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		sample.options.existingHead = i != 0
		fixtures = append(fixtures, newRuntimeOriginalReportAtInstallationFixtureV1(t, sample.options, witness, config, frozen))
	}
	return fixtures
}

func runtimeVariedReportInventoryFixtureV1(t *testing.T) []*runtimeOriginalReservedReportHistoryFixtureV1 {
	return runtimeReportInventorySamplesV1(t, []runtimeReportInventorySampleV1{
		{"thread-inventory-independent", "turn-failed", runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptReservedV1, preWitnessDisposition: domainpending.StatusFailed, omitFailedResult: true}},
		{"thread-inventory-shared", "turn-failed-held", runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, preWitnessDisposition: domainpending.StatusFailed, omitFailedResult: true}},
		{"thread-inventory-shared", "turn-delivered", runtimeOriginalReportHistoryFixtureOptionsV1{controlled: true}},
		{"thread-inventory-shared", "turn-open", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptCommittedSettlementV1}},
		{"thread-inventory-other-open", "turn-intent", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptIntentDurableV1}},
	})
}

func TestRuntimeOriginalInventoryPartitionEdges(t *testing.T) {
	for _, allOpen := range []bool{true, false} {
		label := "closed-only"
		options := runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptStageDispositionV1}
		if allOpen {
			label = "open-only"
			options.postWitnessCut = publicationapp.RestartAttemptCommittedSettlementV1
		}
		t.Run(label, func(t *testing.T) {
			fixtures := runtimeReportInventorySamplesV1(t, []runtimeReportInventorySampleV1{{"thread-pair", "turn-one", options}, {"thread-pair", "turn-two", options}})
			last := fixtures[1]
			guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(context.Background(), last.core, last.core.access, last.publication.installation, last.advance)
			if err != nil {
				t.Fatal(err)
			}
			if guard.history == nil || guard.history.mixedClosed == nil || (guard.publication.deferred != nil) != allOpen || guard.report.OwnsThread(last.attempt.ThreadID) != allOpen {
				t.Fatal("partition lost an empty/nonempty scope boundary")
			}
			if err := guard.ValidateSemanticOperationsV1(context.Background(), nil, nil, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeOriginalVariedInventorySemanticGuards(t *testing.T) {
	fixtures := runtimeVariedReportInventoryFixtureV1(t)
	last := fixtures[len(fixtures)-1]
	ctx := context.Background()
	core := last.core
	guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, core, core.access, last.publication.installation, last.advance)
	if err != nil {
		t.Fatal(err)
	}
	if guard.history == nil || len(guard.history.plan.Attempts) != 5 || len(guard.history.mixedClosed.Attempts) != 3 || len(guard.report.ThreadIDs()) != 2 || !guard.history.controlled.hasRecords {
		t.Fatal("varied inventory lost its complete denominator")
	}
	if err := guard.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"held-closed-primary", "controlled-record", "independent-primary"} {
		t.Run(fault, func(t *testing.T) {
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			calls := last.witnessAttempts()
			snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("inventory semantic/" + fault))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			root, err := persistencefs.FreezeRootAuthority(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(core.roots)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, guard).Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if fault == "controlled-record" {
					for name := range guard.history.controlled.owners["controlled-artifact-access-v2"] {
						return os.Remove(filepath.Join(stage.DataDir, "private", "controlled-artifact-access-v2", name))
					}
					return errors.New("controlled original denominator empty")
				}
				fixture := fixtures[0]
				if fault == "held-closed-primary" {
					fixture = fixtures[1]
				}
				relative, err := filepath.Rel(core.roots.DurableDir, fixture.primary)
				if err != nil {
					return err
				}
				target := filepath.Join(stage.DurableDir, relative)
				body, err := os.ReadFile(target)
				if err != nil {
					return err
				}
				var primary map[string]any
				if err := json.Unmarshal(body, &primary); err != nil {
					return err
				}
				primary["title"] = "ordinary inventory metadata"
				body, err = json.Marshal(primary)
				if err != nil {
					return err
				}
				return os.WriteFile(target, body, 0600)
			})
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			err = prepared.Apply(ctx)
			if fault == "independent-primary" {
				if err != nil {
					t.Fatal("independent ordinary metadata refused", err)
				}
				if err := guard.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
				t.Fatal("invalid candidate applied or changed Original", err)
			}
			if calls != last.witnessAttempts() {
				t.Fatal("semantic proof contacted witness")
			}
		})
	}
}

func TestRuntimeVariedInventoryOrdinaryHTTPWitnessOutage(t *testing.T) {
	fixtures := runtimeVariedReportInventoryFixtureV1(t)
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, true, "", fixtures[0])
}

func TestRuntimeDisposedInventoryOrdinaryHTTPWitnessOutage(t *testing.T) {
	fixtures := runtimeReportInventorySamplesV1(t, []runtimeReportInventorySampleV1{
		{"thread-disposed-independent", "turn-failed", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptDeliveryDecisionV1, restartStatus: domainpending.StatusFailed}},
		{"thread-disposed-held", "turn-unknown", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptDeliveryDecisionV1, restartStatus: domainpending.StatusOutcomeUnknown}},
	})
	testRuntimeClosedReportSameThreadOrdinaryHTTPV1(t, true, "", fixtures[0])
}

func TestRuntimeOriginalDisposedPrefixRejectsUnknownSettledCore(t *testing.T) {
	ctx := context.Background()
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptDeliveryDecisionV1, restartStatus: domainpending.StatusFailed})
	stage := fixture.core.pendingInventory.Receipts[0]
	original := fixture.core.pendingInventory.Dispositions[stage.WorkID]
	at, err := time.Parse(time.RFC3339Nano, original.DisposedAt)
	if err != nil {
		t.Fatal(err)
	}
	authority := fixture.publication.installation
	changed, err := domainpending.NewPendingWorkDispositionV1(stage, domainpending.StatusOutcomeUnknown, "report_stage_outcome_unknown_after_restart", at, authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainpending.PendingWorkDispositionV1Bytes(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.core.roots.DataDir, "private", "pending-work", "dispositions", stage.WorkID[:2], stage.WorkID+".json"), body, 0600); err != nil {
		t.Fatal(err)
	}
	fixture.refresh(t)
	before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
	calls := fixture.witnessAttempts()
	_, err = prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
	if err == nil || !strings.Contains(err.Error(), "UNKNOWN") {
		t.Fatal("signed UNKNOWN bypassed its actual Core result", err)
	}
	if calls != fixture.witnessAttempts() || !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) {
		t.Fatal("invalid disposed prefix changed Original or contacted witness")
	}
}

func TestRuntimeOriginalDisposedPrefixProof(t *testing.T) {
	for _, sample := range []struct {
		name    string
		options runtimeOriginalReportHistoryFixtureOptionsV1
	}{
		{"unknown-before-intent", runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptMaterialsDurableV1, restartStatus: domainpending.StatusOutcomeUnknown}},
		{"unknown-after-decision", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptDeliveryDecisionV1, restartStatus: domainpending.StatusOutcomeUnknown}},
		{"failed-after-decision", runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptDeliveryDecisionV1, restartStatus: domainpending.StatusFailed}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, sample.options)
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			calls := fixture.witnessAttempts()
			guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(context.Background(), fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
			if err != nil {
				t.Fatal("actual disposed prefix remained a global gate", err)
			}
			unknown := sample.options.restartStatus == domainpending.StatusOutcomeUnknown
			if guard.report.OwnsThread(fixture.attempt.ThreadID) != unknown || (guard.publication.deferred != nil) != unknown || (guard.publication.closed != nil) == unknown {
				t.Fatal("disposed prefix scope classification changed")
			}
			if err := guard.ValidateSemanticOperationsV1(context.Background(), nil, nil, ""); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || calls != fixture.witnessAttempts() {
				t.Fatal("disposed prefix proof changed Original or contacted witness")
			}
		})
	}
}

func TestRuntimeOriginalCompleteInventoryScope(t *testing.T) {
	fixtures := runtimeMultiReportInventoryFixtureV1(t)
	last := fixtures[len(fixtures)-1]
	history, err := prepareRuntimeOriginalReportHistoryV1(context.Background(), last.core, last.publication, last.advance, last.scope)
	if err != nil || history == nil || len(history.plan.Attempts) != 3 {
		t.Fatal("complete native inventory failed", err)
	}
	if !last.scope.OwnsThread(fixtures[1].attempt.ThreadID) || !last.scope.OwnsThread(last.attempt.ThreadID) || last.scope.OwnsThread(fixtures[0].attempt.ThreadID) || len(last.scope.ThreadIDs()) != 1 {
		t.Fatal("whole-thread unresolved scope union is incorrect")
	}
	guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(context.Background(), last.core, last.core.access, last.publication.installation, last.advance)
	if err != nil || guard.history == nil || guard.publication.deferred == nil || guard.publication.closed == nil {
		t.Fatal("complete inventory lost its open or closed proof", err)
	}
}
