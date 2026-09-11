//go:build darwin || linux

package runtimeapp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainpending "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	startupport "analytix.local/runtime-go/internal/ports/startup"
)

func runtimeForeignReportCheckpointForTestV1(t *testing.T, previous domainsecurity.MonotonicHeadCheckpointV1) (domainsecurity.MonotonicHeadCheckpointV1, domainsecurity.MonotonicHeadSignFunc) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sign := func(body []byte) ([]byte, error) { return ed25519.Sign(privateKey, body), nil }
	checkpoint, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: previous.InstallationID, EnrollmentID: previous.EnrollmentID, Namespace: previous.Namespace,
		Generation: previous.Generation, CurrentStateDigest: previous.CurrentStateDigest, PreviousStateDigest: previous.PreviousStateDigest,
		PreviousCheckpointDigest: previous.PreviousCheckpointDigest, FenceNonce: previous.FenceNonce, MutationID: previous.MutationID,
		WitnessKeyID: domainsecurity.SHA256Hex(publicKey), WitnessPublicKey: publicKey,
	}, sign)
	if err != nil || checkpoint.WitnessKeyID == previous.WitnessKeyID {
		t.Fatal("foreign checkpoint construction failed", err)
	}
	return checkpoint, sign
}

func runtimeForeignReportAdvanceReceiptForTestV1(t *testing.T, previous domainsecurity.MonotonicHeadCheckpointV1, request domainsecurity.MonotonicHeadAdvanceRequestV1, sign domainsecurity.MonotonicHeadSignFunc) domainsecurity.MonotonicHeadAdvanceReceiptV1 {
	t.Helper()
	publicKey, err := base64.RawURLEncoding.DecodeString(previous.WitnessPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	next, err := domainsecurity.NewMonotonicHeadCheckpointV1(domainsecurity.MonotonicHeadCheckpointInputV1{
		InstallationID: request.InstallationID, EnrollmentID: request.EnrollmentID, Namespace: request.Namespace,
		Generation: request.NextGeneration, CurrentStateDigest: request.NextStateDigest, PreviousStateDigest: request.ExpectedStateDigest,
		PreviousCheckpointDigest: request.ExpectedCheckpointDigest, FenceNonce: domainsecurity.SHA256Hex([]byte("foreign report next fence")), MutationID: request.MutationID,
		WitnessKeyID: previous.WitnessKeyID, WitnessPublicKey: publicKey,
	}, sign)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainsecurity.NewMonotonicHeadAdvanceReceiptV1(request, next, sign)
	if err != nil || domainsecurity.ValidateMonotonicHeadAdvanceV1(previous, request, receipt) != nil {
		t.Fatal("foreign advance must be internally authentic and exact", err)
	}
	return receipt
}

func TestRuntimeOriginalCommittedReportSettlementRejectsForeignWitness(t *testing.T) {
	testRuntimeOriginalReportRejectsForeignWitnessV1(t, false)
}

func TestRuntimeOriginalUnsettledReportIntentRejectsForeignWitness(t *testing.T) {
	testRuntimeOriginalReportRejectsForeignWitnessV1(t, true)
}

func testRuntimeOriginalReportRejectsForeignWitnessV1(t *testing.T, unsettled bool) {
	t.Helper()
	ctx := context.Background()
	options := runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptCommittedSettlementV1, foreignCommittedWitness: true}
	gate := "committed report settlement lacks independent witness authority"
	if unsettled {
		options = runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptIntentDurableV1, foreignIntentWitness: true}
		gate = "unsettled report intent lacks independent witness authority"
	}
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, options)
	original, final, observation, err := fixture.publication.readV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	associatedOriginal, associatedFinal, _, associatedObservation, err := readRuntimeAssociatedSemanticInventoryV1(ctx, fixture.core, fixture.scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []struct {
		publication, evidence runtimeOriginalSemanticFilesV1
	}{
		{original["report-publication"], associatedOriginal["evidence-authority"]},
		{final["report-publication"], associatedFinal["evidence-authority"]},
	} {
		history, err := prepareRuntimeReportHistoryEndpointV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.core.pendingInventory, endpoint.publication, endpoint.evidence, fixture.core.primaries)
		if err != nil || history == nil || !runtimeReportPlanIsDeferredV1(history.plan) {
			t.Fatal("foreign witness must pass complete domain/current-key endpoint audit", err)
		}
	}
	if err := observation.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := associatedObservation.Revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
	calls := fixture.witnessAttempts()
	handler, err := NewRuntimeServerHandlerE(fixture.config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("foreign witness enabled committed report startup")
	}
	if !strings.Contains(err.Error(), gate) {
		t.Fatal("foreign witness did not reach the independent authority gate", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || fixture.witnessAttempts() != calls {
		t.Fatal("foreign witness refusal changed managed state or contacted witness")
	}
}

func TestRuntimeOriginalUnknownCommittedReportPreservesHistory(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptDeliveryBlockedV1, "")
}

func TestRuntimeOriginalUnsettledReportIntentPreservesMissingSuffix(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptIntentDurableV1, "")
}

func TestRuntimeOriginalCommittedReportSettlementPreservesMissingSuffix(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptCommittedSettlementV1, "")
}

func TestRuntimeOriginalSelectedReportCommitPreservesMissingSuffix(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptCommitSelectionV1, "")
}

func TestRuntimeOriginalDurableReportCommitPreservesMissingSuffix(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptCommitReceiptV1, "")
}

func TestRuntimeOriginalReportDecisionPreservesMissingSuffix(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptDeliveryDecisionV1, "")
}

func TestRuntimeOriginalReportGrantSettlementPreservesMissingSuffix(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptGrantSettlementV1, "")
}

func TestRuntimeOriginalMatchingReportResultPreservesMissingSuffix(t *testing.T) {
	testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t, publicationapp.RestartAttemptDeliveryDecisionV1, "matching")
}

func TestRuntimeOriginalPostWitnessRejectsUnboundCoreResult(t *testing.T) {
	testRuntimeOriginalReportRejectsUnboundCoreResultV1(t, publicationapp.RestartAttemptDeliveryDecisionV1)
}

func TestRuntimeOriginalSettledReportRejectsUnboundCoreResult(t *testing.T) {
	testRuntimeOriginalReportRejectsUnboundCoreResultV1(t, publicationapp.RestartAttemptGrantSettlementV1)
}

func testRuntimeOriginalReportRejectsUnboundCoreResultV1(t *testing.T, cut publicationapp.RestartAttemptStateV1) {
	t.Helper()
	for _, result := range []string{"foreign-admission", "missing-admission"} {
		t.Run(result, func(t *testing.T) {
			ctx := context.Background()
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{
				postWitnessCut: cut, postWitnessCoreResult: result,
			})
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			calls := fixture.witnessAttempts()
			_, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
			if err == nil || !strings.Contains(err.Error(), "original report Core result does not match its delivery decision") {
				t.Fatal("unbound Core result acquired preservation qualification", err)
			}
			handler, err := NewRuntimeServerHandlerE(fixture.config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("unbound Core result allowed startup")
			}
			if !strings.Contains(err.Error(), "original report Core result does not match its delivery decision") {
				t.Fatal("full root did not enforce the exact result qualification", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || calls != fixture.witnessAttempts() {
				t.Fatal("settled Core qualification refusal changed Original or contacted witness")
			}
		})
	}
}

func TestRuntimeOriginalPostWitnessActiveGrantQualification(t *testing.T) {
	for _, cut := range []publicationapp.RestartAttemptStateV1{
		publicationapp.RestartAttemptDeliveryBlockedV1, publicationapp.RestartAttemptCommittedSettlementV1, publicationapp.RestartAttemptCommitSelectionV1,
		publicationapp.RestartAttemptCommitReceiptV1, publicationapp.RestartAttemptDeliveryDecisionV1,
	} {
		t.Run(string(cut), func(t *testing.T) {
			prefixCut := cut
			if cut == publicationapp.RestartAttemptDeliveryBlockedV1 {
				prefixCut = publicationapp.RestartAttemptCommittedSettlementV1
			}
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: prefixCut, unknownAfterCommit: cut == publicationapp.RestartAttemptDeliveryBlockedV1})
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			calls := fixture.witnessAttempts()
			guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(context.Background(), fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
			if err != nil || guard.publication == nil || guard.publication.deferred == nil || len(guard.publication.deferred.plan.Attempts) != 1 || guard.publication.deferred.plan.Attempts[0].State != cut {
				t.Fatal("complete native before-result prefix lost its qualification", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || calls != fixture.witnessAttempts() {
				t.Fatal("before-result qualification changed Original or contacted witness")
			}
		})
	}
}

func testRuntimeOriginalPostWitnessReportPreservesMissingSuffixV1(t *testing.T, cut publicationapp.RestartAttemptStateV1, coreResult string) {
	t.Helper()
	ctx := context.Background()
	unknown := cut == publicationapp.RestartAttemptDeliveryBlockedV1
	prefixCut := cut
	if unknown {
		prefixCut = publicationapp.RestartAttemptCommittedSettlementV1
	}
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: prefixCut, postWitnessCoreResult: coreResult, unknownAfterCommit: unknown})
	history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
	if err != nil || history == nil || len(history.plan.Attempts) != 1 {
		t.Fatalf("complete Original/Final committed settlement preflight failed: %v", err)
	}
	entry := history.plan.Attempts[0]
	if entry.State != cut || entry.Intent == nil || (entry.Settlement != nil) != (prefixCut != publicationapp.RestartAttemptIntentDurableV1) || entry.Candidate == nil || entry.Index == nil ||
		(entry.Selection != nil) != (prefixCut != publicationapp.RestartAttemptIntentDurableV1 && prefixCut != publicationapp.RestartAttemptCommittedSettlementV1) ||
		(entry.Commit != nil) != (prefixCut == publicationapp.RestartAttemptCommitReceiptV1 || prefixCut == publicationapp.RestartAttemptDeliveryDecisionV1 || prefixCut == publicationapp.RestartAttemptGrantSettlementV1) ||
		(entry.Decision != nil) != (prefixCut == publicationapp.RestartAttemptDeliveryDecisionV1 || prefixCut == publicationapp.RestartAttemptGrantSettlementV1) || (entry.Disposition != nil) != unknown || (entry.GrantSettlement != nil) != (prefixCut == publicationapp.RestartAttemptGrantSettlementV1) || entry.StageCompletion != nil || entry.DeliveryOutcome != nil {
		t.Fatal("fixture did not retain the exact post-witness prefix")
	}
	if unknown && (entry.Disposition.Status != domainpending.StatusOutcomeUnknown || entry.Disposition.ReasonCode != "report_stage_outcome_unknown_after_restart") {
		t.Fatal("native UNKNOWN disposition changed")
	}

	if !fixture.scope.OwnsThread(entry.Stage.Context.ThreadID) {
		t.Fatal("unresolved committed report lost its preservation scope")
	}
	if !fixture.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("could not disable fixture witness")
	}
	workID := entry.Stage.WorkID
	pendingRoot := filepath.Join(fixture.core.roots.DataDir, "private", "pending-work")
	protected := []string{fixture.primary, filepath.Join(pendingRoot, "receipts", workID[:2], workID+".json"), filepath.Join(fixture.core.roots.DataDir, "private", "authority-advance"), filepath.Join(fixture.core.roots.DataDir, "private", "evidence-authority")}
	for _, owner := range runtimePublicationOwnersV1 {
		protected = append(protected, filepath.Join(fixture.core.roots.DataDir, "private", owner))
	}
	if unknown {
		protected = append(protected, filepath.Join(pendingRoot, "dispositions", workID[:2], workID+".json"))
	}
	before := startupWholeTreeRecordMapForTest(t, protected...)
	calls := fixture.witnessAttempts()
	for restart := 0; restart < 2; restart++ {
		whole := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
		handler, err := NewRuntimeServerHandlerE(fixture.config)
		if err != nil {
			if !reflect.DeepEqual(whole, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || fixture.witnessAttempts() != calls {
				t.Fatal("committed settlement refusal wrote or contacted witness", err)
			}
			t.Fatalf("committed settlement blocked native startup %d: %v", restart, err)
		}
		shutdownOwnedRuntimeHandler(t, handler)
		if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, protected...)) || fixture.witnessAttempts() != calls {
			t.Fatal("committed history restart changed Original or retried authority")
		}
		if _, err := os.Lstat(filepath.Join(pendingRoot, "dispositions", workID[:2], workID+".json")); !unknown && !os.IsNotExist(err) {
			t.Fatal("committed settlement invented a pending disposition")
		}
	}
}

func TestRuntimeOriginalCommittedReportSettlementRejectsSemanticAdvanceChanges(t *testing.T) {
	for _, attack := range []string{"frozen-unknown-disposition-removal", "signed-final-only-unknown-disposition", "frozen-intent-removal", "signed-final-only-intent", "frozen-settlement-removal", "signed-final-only-settlement", "frozen-observation-removal", "signed-final-only-selection", "signed-final-only-observation", "frozen-commit-removal", "signed-final-only-commit", "frozen-decision-removal", "signed-final-only-decision", "frozen-grant-settlement-removal", "signed-final-only-grant-settlement", "frozen-stage-completion-removal", "signed-final-only-stage-completion"} {
		t.Run(attack, func(t *testing.T) {
			ctx := context.Background()
			unknownCut := strings.Contains(attack, "unknown-disposition")
			intentCut := attack == "frozen-intent-removal" || attack == "signed-final-only-intent"
			completionCut := attack == "frozen-stage-completion-removal" || attack == "signed-final-only-stage-completion"
			grantCut := attack == "frozen-grant-settlement-removal" || attack == "signed-final-only-grant-settlement"
			commitCut := attack == "frozen-commit-removal" || attack == "signed-final-only-commit"
			decisionCut := attack == "frozen-decision-removal" || attack == "signed-final-only-decision"
			selected := strings.Contains(attack, "observation") || strings.HasSuffix(attack, "selection") || commitCut || decisionCut || grantCut || completionCut
			finalOnly := strings.HasPrefix(attack, "signed-final-only-")
			cutState := publicationapp.RestartAttemptCommittedSettlementV1
			if selected {
				cutState = publicationapp.RestartAttemptCommitSelectionV1
			}
			if commitCut {
				cutState = publicationapp.RestartAttemptCommitReceiptV1
			}
			if decisionCut {
				cutState = publicationapp.RestartAttemptDeliveryDecisionV1
			}
			if grantCut {
				cutState = publicationapp.RestartAttemptGrantSettlementV1
			}
			if completionCut {
				cutState = publicationapp.RestartAttemptStageCompletionV1
			}
			if intentCut {
				cutState = publicationapp.RestartAttemptIntentDurableV1
			}
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: cutState, unknownAfterCommit: unknownCut})
			history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
			if err != nil || history == nil || (history.deferred == nil && !history.closedCompletedPrefix) {
				t.Fatal("native committed settlement did not qualify", err)
			}
			mutationID := history.plan.Attempts[0].Intent.MutationID
			intentRelative := filepath.Join("private", "authority-advance", "v2", "intents", mutationID[:2], mutationID+".json")
			settlementRelative := filepath.Join("private", "authority-advance", "v2", "settlements", mutationID[:2], mutationID+".json")
			relatives := []string{intentRelative, settlementRelative}
			removeRelative := settlementRelative
			originalGate := "original authority advance journal was changed by semantic startup"
			frozenGate := "deferred report authority advance cannot be changed by semantic startup"
			prefixState := publicationapp.RestartAttemptMaterialsDurableV1
			if intentCut {
				relatives = []string{intentRelative}
				removeRelative = intentRelative
			}
			if selected {
				selection := history.plan.Attempts[0].Selection
				if completionCut {
					id := history.plan.Attempts[0].StageCompletion.CompletionID
					removeRelative = filepath.Join("private", "report-publication", "stage-completions", id[:2], id+".json")
					originalGate = "closed completed report Original/Final prefix changed"
					frozenGate = "original preserved publication bytes, mode or absence changed"
				} else if grantCut {
					id := history.plan.Attempts[0].GrantSettlement.SettlementID
					removeRelative = filepath.Join("private", "report-publication", "grant-settlements", id[:2], id+".json")
					originalGate = "deferred report Original/Final prefix changed"
					frozenGate = "original preserved publication bytes, mode or absence changed"
				} else if decisionCut {
					id := history.plan.Attempts[0].Decision.DecisionID
					removeRelative = filepath.Join("private", "report-publication", "delivery-decisions", id[:2], id+".json")
					originalGate = "deferred report Original/Final prefix changed"
					frozenGate = "original preserved publication bytes, mode or absence changed"
				} else if commitCut {
					id := history.plan.Attempts[0].Commit.RecordDigest
					removeRelative = filepath.Join("private", "report-publication", "commit-receipts", id[:2], id+".json")
					originalGate = "deferred report Original/Final prefix changed"
					frozenGate = "original preserved publication bytes, mode or absence changed"
				} else if strings.Contains(attack, "observation") {
					id := selection.ObservationDigest
					removeRelative = filepath.Join("private", "evidence-authority", "observations", id[:2], id+".json")
					originalGate = "original report selection lacks its exact stored witness exchange"
					frozenGate = "original associated bytes, mode or presence changed"
				} else {
					id := selection.SelectionID
					removeRelative = filepath.Join("private", "report-publication", "commit-selections", id[:2], id+".json")
					originalGate = "deferred report Original/Final prefix changed"
				}
				relatives = []string{removeRelative}
				prefixState = publicationapp.RestartAttemptCommittedSettlementV1
				if commitCut {
					prefixState = publicationapp.RestartAttemptCommitSelectionV1
				}
				if decisionCut {
					prefixState = publicationapp.RestartAttemptCommitReceiptV1
				}
				if grantCut {
					prefixState = publicationapp.RestartAttemptDeliveryDecisionV1
				}
				if completionCut {
					prefixState = publicationapp.RestartAttemptStageDispositionV1
				}
			}
			if unknownCut {
				id := history.plan.Attempts[0].Stage.WorkID
				removeRelative = filepath.Join("private", "pending-work", "dispositions", id[:2], id+".json")
				relatives = []string{removeRelative}
				prefixState = publicationapp.RestartAttemptCommittedSettlementV1
				originalGate = "report restart unresolved authority changed"
				frozenGate = pendingworkapp.ErrRestartPreserved.Error()
			}
			originalRecords := map[string][]byte{}
			for _, relative := range relatives {
				body, err := os.ReadFile(filepath.Join(fixture.core.roots.DataDir, relative))
				if err != nil {
					t.Fatal(err)
				}
				originalRecords[relative] = body
				if finalOnly {
					if err := os.Remove(filepath.Join(fixture.core.roots.DataDir, relative)); err != nil {
						t.Fatal(err)
					}
				}
			}
			guard := runtimeReportRestartPreservationV1{}
			if !finalOnly {
				guard, err = prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
				if err != nil || guard.publication == nil || !guard.publication.recoveryDeferredV1() || (history.deferred != nil && guard.durable == nil) {
					t.Fatal("removal fixture requires the full native deferred preservation guard", err)
				}
				if err := guard.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
					t.Fatal("complete unmodified Original must pass the native guard", err)
				}
			}
			if finalOnly {
				fixture.refresh(t)
				prefix, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
				if attack == "signed-final-only-observation" {
					if err == nil || prefix != nil || !strings.Contains(err.Error(), originalGate) {
						t.Fatal("missing Original exchange did not reach its exact historical gate", err)
					}
				} else if err != nil || prefix == nil || len(prefix.plan.Attempts) != 1 || prefix.plan.Attempts[0].State != prefixState {
					t.Fatal("Final-only fixture must start with its authentic earlier Original cut", err)
				}
				guard = runtimeReportRestartPreservationV1{}
			}
			roots := fixture.core.roots
			key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
			if err != nil || key.KeyID() != fixture.core.verification.KeyID() {
				t.Fatal("signed program requires the current installation key", err)
			}
			before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			calls := fixture.witnessAttempts()
			snapshot, err := persistencefs.NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("committed settlement semantic " + attack))
			baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
			if err != nil {
				t.Fatal(err)
			}
			root, err := persistencefs.FreezeRootAuthority(roots)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
			if err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join("private", "a-committed-settlement-marker.bin")
			builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, guard)
			prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if !finalOnly {
					if err := os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("independent-marker"), 0600); err != nil {
						return err
					}
					return os.Remove(filepath.Join(stage.DataDir, removeRelative))
				}
				for relative, body := range originalRecords {
					if err := os.WriteFile(filepath.Join(stage.DataDir, relative), body, 0600); err != nil {
						return err
					}
				}
				return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("signed-prefix-marker"), 0600)
			})
			if err != nil {
				t.Fatal("semantic candidate did not prepare", err)
			}
			defer prepared.Close()
			if !finalOnly {
				if err := prepared.Apply(ctx); err == nil || !strings.Contains(err.Error(), frozenGate) {
					t.Fatal("actual guarded Apply did not enforce the frozen authority-advance boundary", err)
				}
			} else {
				cancelled, cancel := context.WithCancel(ctx)
				defer cancel()
				cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: filepath.Join(roots.DataDir, marker), intentPath: filepath.Join(roots.DataDir, relatives[0])}
				if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
					t.Fatal("signed program did not stop before adding advance history", err)
				}
				for relative := range originalRecords {
					if _, err := os.Lstat(filepath.Join(roots.DataDir, relative)); !os.IsNotExist(err) {
						t.Fatal("Final-only advance history already became physical Original")
					}
				}
				if !unknownCut {
					fixture.refresh(t)
				}
				before = startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
				handler, err := NewRuntimeServerHandlerE(fixture.config)
				if err == nil {
					shutdownOwnedRuntimeHandler(t, handler)
					t.Fatal("signed Final supplied Original committed settlement authority")
				}
				if !strings.Contains(err.Error(), originalGate) {
					t.Fatal("Final-only history did not reach the Original authority gate", err)
				}
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) || fixture.witnessAttempts() != calls {
				t.Fatal("advance preservation refusal wrote managed state or contacted witness")
			}
		})
	}
}

func TestRuntimeOriginalMatchingReportResultRejectsSemanticChanges(t *testing.T) {
	for _, attack := range []string{"frozen-result-removal", "signed-final-only-result"} {
		t.Run(attack, func(t *testing.T) {
			ctx := context.Background()
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{
				postWitnessCut: publicationapp.RestartAttemptDeliveryDecisionV1, postWitnessCoreResult: "matching",
			})
			history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
			if err != nil || history == nil || history.deferred == nil {
				t.Fatal("matching Original result did not qualify", err)
			}
			decision := history.plan.Attempts[0].Decision
			matchingBody, err := os.ReadFile(fixture.primary)
			if err != nil {
				t.Fatal(err)
			}
			var primary map[string]any
			if err := json.Unmarshal(matchingBody, &primary); err != nil {
				t.Fatal(err)
			}
			removeRuntimeOriginalReportResultForTestV1(t, primary, domaintoolresult.ToolResultItemIDV1(decision.Context.TurnID, decision.ToolCallID))
			beforeResultBody, err := json.Marshal(primary)
			if err != nil {
				t.Fatal(err)
			}
			relative, err := filepath.Rel(fixture.core.roots.DurableDir, fixture.primary)
			if err != nil {
				t.Fatal(err)
			}
			finalOnly := attack == "signed-final-only-result"
			if finalOnly {
				if err := os.WriteFile(fixture.primary, beforeResultBody, 0600); err != nil {
					t.Fatal(err)
				}
				fixture.refresh(t)
			}
			guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, fixture.core, fixture.core.access, fixture.publication.installation, fixture.advance)
			if err != nil || guard.publication == nil || guard.publication.deferred == nil || guard.durable == nil {
				t.Fatal("unmodified exact Original requires the full native guard", err)
			}
			if err := guard.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
				t.Fatal(err)
			}
			roots := fixture.core.roots
			marker := filepath.Join("private", "a-report-result-marker.bin")
			before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
			calls := fixture.witnessAttempts()
			if finalOnly {
				tail := filepath.Join(filepath.Dir(relative), "z-result-tail.bin")
				absent, err := filepath.Rel(roots.DataDir, filepath.Join(roots.DurableDir, tail))
				if err != nil {
					t.Fatal(err)
				}
				runtimePendingSemanticCutForTestV1(t, fixture.core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
					if err := os.WriteFile(filepath.Join(stage.DurableDir, relative), matchingBody, 0600); err != nil {
						return err
					}
					if err := os.WriteFile(filepath.Join(stage.DurableDir, tail), []byte("remaining result tail"), 0600); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("signed result prefix"), 0600)
				}, marker, absent)
				if body, err := os.ReadFile(fixture.primary); err != nil || !reflect.DeepEqual(body, beforeResultBody) {
					t.Fatal("signed Final result already became physical Original", err)
				}
				before = startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
				handler, err := NewRuntimeServerHandlerE(fixture.config)
				if err == nil {
					shutdownOwnedRuntimeHandler(t, handler)
					t.Fatal("signed Final result supplied Original authority")
				}
				if !errors.Is(err, pendingworkapp.ErrRestartPreserved) {
					t.Fatal("signed Final result missed the Original held-primary gate", err)
				}
			} else {
				snapshot, err := persistencefs.NewStartupSnapshotReader(roots).CaptureManagedSnapshotV1(ctx)
				if err != nil {
					t.Fatal(err)
				}
				digest := domainsecurity.SHA256Hex([]byte("matching report result semantic removal"))
				baseline, err := domainstartup.NewReadOnlyStartupBaselineV1(snapshot, digest, time.Unix(1, 0).UTC())
				if err != nil {
					t.Fatal(err)
				}
				root, err := persistencefs.FreezeRootAuthority(roots)
				if err != nil {
					t.Fatal(err)
				}
				journal, err := persistencefs.FreezeJournalNamespaceAuthorityForRoots(roots)
				if err != nil {
					t.Fatal(err)
				}
				builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(roots, root, journal, guard)
				prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
					if err := os.WriteFile(filepath.Join(stage.DurableDir, relative), beforeResultBody, 0600); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("independent result marker"), 0600)
				})
				if err != nil {
					t.Fatal(err)
				}
				defer prepared.Close()
				if err := prepared.Apply(ctx); !errors.Is(err, eventlog.ErrRestartPreserved) {
					t.Fatal("actual full guarded Apply did not preserve the Original result", err)
				}
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) || calls != fixture.witnessAttempts() {
				t.Fatal("result preservation refusal changed managed state or contacted witness")
			}
		})
	}
}

func TestRuntimeOriginalUnsettledIntentRequiresOriginalNextBundle(t *testing.T) {
	testRuntimeOriginalReportRequiresNextBundleV1(t, false)
}

func TestRuntimeOriginalUnknownCommittedRequiresOriginalNextBundle(t *testing.T) {
	testRuntimeOriginalReportRequiresNextBundleV1(t, true)
}

func testRuntimeOriginalReportRequiresNextBundleV1(t *testing.T, unknown bool) {
	for _, finalOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-next-bundle", true: "signed-final-only-next-bundle"}[finalOnly], func(t *testing.T) {
			cut := publicationapp.RestartAttemptIntentDurableV1
			if unknown {
				cut = publicationapp.RestartAttemptCommittedSettlementV1
			}
			fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: cut, unknownAfterCommit: unknown})
			id := fixture.attempt.NextEvidenceBundleDigest
			relative := filepath.Join("private", "evidence-authority", "bundles", id[:2], id+".json")
			target := filepath.Join(fixture.core.roots.DataDir, relative)
			body, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			fixture.refresh(t)
			if finalOnly {
				marker := filepath.Join("private", "a-unsettled-intent-marker.bin")
				runtimePendingSemanticCutForTestV1(t, fixture.core, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
					if err := os.WriteFile(filepath.Join(stage.DataDir, relative), body, 0600); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("independent intent prefix"), 0600)
				}, marker, relative)
			}
			before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
			calls := fixture.witnessAttempts()
			handler, err := NewRuntimeServerHandlerE(fixture.config)
			if err == nil {
				shutdownOwnedRuntimeHandler(t, handler)
				t.Fatal("missing Original next bundle acquired intent qualification")
			}
			if !strings.Contains(err.Error(), "report intent lacks its exact Original transition bundles") {
				t.Fatal("next bundle did not reach its exact Original gate", err)
			}
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || calls != fixture.witnessAttempts() {
				t.Fatal("next-bundle refusal changed managed state or contacted witness")
			}
		})
	}
}

func TestRuntimeOriginalUnknownCommittedRejectsSignedFinalDispositionChange(t *testing.T) {
	ctx := context.Background()
	fixture := newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptCommittedSettlementV1, unknownAfterCommit: true})
	stage := fixture.core.pendingInventory.Receipts[0]
	original := fixture.core.pendingInventory.Dispositions[stage.WorkID]
	at, err := time.Parse(time.RFC3339Nano, original.DisposedAt)
	if err != nil {
		t.Fatal(err)
	}
	authority := fixture.publication.installation
	changed, err := domainpending.NewPendingWorkDispositionV1(stage, original.Status, original.ReasonCode, at.Add(time.Second), authority.KeyID(), authority.PublicKey(), func(body []byte) ([]byte, error) { return authority.Sign(ctx, body) })
	if err != nil {
		t.Fatal(err)
	}
	body, err := domainpending.PendingWorkDispositionV1Bytes(changed)
	if err != nil {
		t.Fatal(err)
	}
	relative := filepath.Join("private", "pending-work", "dispositions", stage.WorkID[:2], stage.WorkID+".json")
	originalBody, err := os.ReadFile(filepath.Join(fixture.core.roots.DataDir, relative))
	if err != nil {
		t.Fatal(err)
	}
	tail := filepath.Join("private", "z-unknown-disposition-tail.bin")
	marker := filepath.Join("private", "a-unknown-disposition-marker.bin")
	runtimePendingSemanticCutForTestV1(t, fixture.core, func(_ context.Context, staged startupport.PersistenceRootsV1) error {
		if err := os.WriteFile(filepath.Join(staged.DataDir, relative), body, 0600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(staged.DataDir, tail), []byte("later sentinel"), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(staged.DataDir, marker), []byte("independent marker"), 0600)
	}, marker, tail)
	actualBody, err := os.ReadFile(filepath.Join(fixture.core.roots.DataDir, relative))
	if err != nil || !reflect.DeepEqual(actualBody, originalBody) {
		t.Fatal("signed Final replacement already changed Original", err)
	}
	before := startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)
	calls := fixture.witnessAttempts()
	handler, err := NewRuntimeServerHandlerE(fixture.config)
	if err == nil {
		shutdownOwnedRuntimeHandler(t, handler)
		t.Fatal("signed Final changed original UNKNOWN")
	}
	if !strings.Contains(err.Error(), "report restart unresolved authority changed") {
		t.Fatal("UNKNOWN change missed exact Original gate", err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, fixture.core.roots.DataDir, fixture.core.roots.DurableDir)) || fixture.witnessAttempts() != calls {
		t.Fatal("UNKNOWN refusal changed Original or contacted witness")
	}
}
