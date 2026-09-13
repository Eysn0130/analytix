//go:build darwin || linux

package runtimeapp

import (
	authoritycompositionfixture "analytix.local/runtime-go/internal/formalauthority"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	pendingapp "analytix.local/runtime-go/internal/app/pendingwork"
	publicationapp "analytix.local/runtime-go/internal/app/reportpublication"
	domainenrollment "analytix.local/runtime-go/internal/domain/authorityenrollment"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstartup "analytix.local/runtime-go/internal/domain/startup"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
	startupport "analytix.local/runtime-go/internal/ports/startup"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

// This proves the Original composition boundary for a real unresolved Core
// stage. Its reserved attempt has no publication suffix; it is not evidence
// of a completed report, stored witness replay, or runtime activation.
func TestRuntimeOriginalReportHistoryRetainsReservedCore(t *testing.T) {
	ctx := context.Background()
	fixture := newRuntimeOriginalReservedReportHistoryFixtureV1(t)
	core, publication, advance, scope := fixture.core, fixture.publication, fixture.advance, fixture.scope
	roots, primary, attempt := core.roots, fixture.primary, fixture.attempt
	before := startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)
	history, err := prepareRuntimeOriginalReportHistoryV1(ctx, core, publication, advance, scope)
	if err != nil {
		t.Fatal(err)
	}
	if history == nil || len(history.plan.Attempts) != 1 || history.plan.Attempts[0].Attempt.AttemptID != attempt.AttemptID || history.plan.Attempts[0].DeliveryOutcome != nil {
		t.Fatal("complete reserved Original inventory was not retained")
	}
	if err := history.revalidate(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, roots.DataDir, roots.DurableDir)) {
		t.Fatal("Original history observation changed managed state")
	}
	selector := publicationport.ProjectedDeliverySelectorV1{SecurityContext: scope.Contexts()[0], DeliveryID: domainsecurity.SHA256Hex([]byte("not issued delivery")), OutcomeRecordDigest: domainsecurity.SHA256Hex([]byte("not issued outcome")), PublicationCommitDigest: domainsecurity.SHA256Hex([]byte("not issued commit"))}
	if _, err := history.authority.ResolveCurrentProjectedDelivery(ctx, selector); !errors.Is(err, publicationapp.ErrProjectedDeliveryUnavailable) {
		t.Fatalf("historical reader exposed current authority: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if result, err := prepareRuntimeOriginalReportHistoryV1(cancelled, core, publication, advance, scope); result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled history returned authority: %v", err)
	}
	trust := core.originalRegistryTrust
	core.originalRegistryTrust = nil
	if result, err := prepareRuntimeOriginalReportHistoryV1(ctx, core, publication, advance, scope); result != nil || err == nil {
		t.Fatal("missing independent enrollment returned history")
	}
	core.originalRegistryTrust = trust
	original, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary, append(original, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := history.revalidate(ctx); err == nil {
		t.Fatal("history survived original primary drift")
	}
}

type runtimeOriginalReservedReportHistoryFixtureV1 struct {
	config              Config
	core                *runtimeChildIdentityStartupV1
	publication         *runtimePublicationSemanticPreservationV1
	advance             *runtimeAuthorityAdvanceStartupV2
	scope               *pendingapp.ReportRestartScopeV1
	primary             string
	attempt             domainpublication.PublicationAttemptV1
	witnessAttempts     func() int
	witnessSnapshot     func(string) (authoritycompositionfixture.WitnessSnapshot, bool)
	setWitnessAvailable func(string, bool) bool
}

func newRuntimeOriginalReservedReportHistoryFixtureV1(t *testing.T) *runtimeOriginalReservedReportHistoryFixtureV1 {
	t.Helper()
	return newRuntimeOriginalReportHistoryWithOptionsFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{preWitnessCut: publicationapp.RestartAttemptReservedV1})
}
func (fixture *runtimeOriginalReservedReportHistoryFixtureV1) refresh(t *testing.T) {
	t.Helper()
	if err := fixture.refreshE(); err != nil {
		t.Fatal(err)
	}
}
func (fixture *runtimeOriginalReservedReportHistoryFixtureV1) refreshE() error {
	ctx := context.Background()
	rootAuthority, err := persistencefs.FreezeRootAuthority(fixture.core.roots)
	if err != nil {
		return err
	}
	installation, err := loadRuntimeOptionalDomainInstallation(ctx, fixture.config, fixture.core.roots, rootAuthority)
	if err != nil {
		return err
	}
	core, err := prepareRuntimeChildIdentityStartupV1(ctx, fixture.core.roots, rootAuthority, fixture.core.access, installation)
	if err != nil {
		return err
	}
	core.originalRegistryTrust, err = prepareRuntimeOriginalRegistryTrustV2(ctx, fixture.config, core.verification)
	if err != nil {
		return err
	}
	publication, err := prepareRuntimePublicationSemanticPreservationV1(ctx, core, installation)
	if err != nil {
		return err
	}
	advance, err := prepareRuntimeAuthorityAdvanceStartupV2(ctx, fixture.core.roots, rootAuthority, fixture.core.access, installation)
	if err != nil {
		return err
	}
	scope, err := prepareRuntimeReportRestartScopeV1(ctx, core)
	if err != nil {
		return err
	}

	fixture.core, fixture.publication, fixture.advance, fixture.scope = core, publication, advance, scope
	return nil
}

func TestRuntimeOriginalReportHistoryRejectsSignedFinalOrphan(t *testing.T) {
	ctx := context.Background()
	fixture := newRuntimeOriginalReservedReportHistoryFixtureV1(t)
	core := fixture.core
	key, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(core.roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json"), true)
	if err != nil {
		t.Fatal(err)
	}
	_, orphan := runtimeReservedReportAttemptFixtureV1(t, key)
	body, err := domainpublication.PublicationAttemptV1Bytes(orphan)
	if err != nil {
		t.Fatal(err)
	}
	relative := filepath.Join("private", "report-publication", "attempts", orphan.AttemptID[:2], orphan.AttemptID+".json")
	marker := filepath.Join("private", "a-original-report-final-marker.bin")
	snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	digest := domainsecurity.SHA256Hex([]byte("Original report Final orphan negative"))
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
	builder := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, runtimeReportRestartPreservationV1{})
	prepared, err := builder.Prepare(ctx, baseline, digest, func(_ context.Context, stage startupport.PersistenceRootsV1) error {
		target := filepath.Join(stage.DataDir, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("signed-prefix-marker"), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	cancelled, cancel := context.WithCancel(ctx)
	defer cancel()
	cut := cancelAfterTerminalDispositionContextV1{Context: cancelled, cancel: cancel, dispositionPath: filepath.Join(core.roots.DataDir, marker), intentPath: filepath.Join(core.roots.DataDir, relative)}
	if err := prepared.Apply(cut); !errors.Is(err, context.Canceled) {
		t.Fatalf("signed Final fixture did not interrupt: %v", err)
	}
	if _, err := os.Stat(cut.intentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Final orphan was already physically installed")
	}
	fixture.refresh(t)
	before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
	history, err := prepareRuntimeOriginalReportHistoryV1(ctx, fixture.core, fixture.publication, fixture.advance, fixture.scope)
	if err == nil || history != nil {
		t.Fatal("signed Final orphan returned usable report history")
	}
	if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
		t.Fatal("Final rejection changed physical Original")
	}
}

func newRuntimeOriginalMixedClosedOpenFixtureV1(t *testing.T) (*runtimeOriginalReservedReportHistoryFixtureV1, *runtimeOriginalReservedReportHistoryFixtureV1) {
	t.Helper()
	ctx := context.Background()
	witness, config := runtimeWitnessedRegistryConfigV2(t)
	contextFor := func(label string) domainsecurity.TurnSecurityContext {
		frozen, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
			ThreadID: "thread-mixed-" + label, TurnID: "turn-mixed-" + label, WorkspaceRealPath: workspacetest.New(t),
			CaseID: "case-mixed-" + label, CaseBindingHash: domainsecurity.SHA256Hex([]byte("mixed-binding-" + label)), ContextEpoch: 1,
			IssuedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatal(err)
		}
		return frozen
	}
	closed := newRuntimeOriginalReportAtInstallationFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptStageDispositionV1}, witness, config, contextFor("closed"))
	closedBody, err := os.ReadFile(closed.primary)
	if err != nil {
		t.Fatal(err)
	}
	closedRecords, _, _, err := closed.publication.readV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	open := newRuntimeOriginalReportAtInstallationFixtureV1(t, runtimeOriginalReportHistoryFixtureOptionsV1{postWitnessCut: publicationapp.RestartAttemptCommittedSettlementV1, existingHead: true}, witness, config, contextFor("open"))
	retainedBody, err := os.ReadFile(closed.primary)
	if err != nil || !reflect.DeepEqual(closedBody, retainedBody) {
		t.Fatal("second report changed closed Core", err)
	}
	allRecords, _, _, err := open.publication.readV1(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for owner, files := range closedRecords {
		for name, entry := range runtimeReportCommittedFilesV1(files) {
			if !reflect.DeepEqual(entry, allRecords[owner][name]) {
				t.Fatal("second report changed an original closed record")
			}
		}
	}
	return closed, open
}

func TestRuntimeOriginalMixedClosedAndOpenReportHistory(t *testing.T) {
	ctx := context.Background()
	closed, open := newRuntimeOriginalMixedClosedOpenFixtureV1(t)
	config := open.config
	history, err := prepareRuntimeOriginalReportHistoryV1(ctx, open.core, open.publication, open.advance, open.scope)
	if err != nil || history == nil || len(history.plan.Attempts) != 2 {
		t.Fatal("complete mixed Original/Final graph failed", err)
	}
	states := map[string]publicationapp.RestartAttemptStateV1{}
	for _, entry := range history.plan.Attempts {
		states[entry.Stage.Context.ThreadID] = entry.State
	}
	if states[closed.attempt.ThreadID] != publicationapp.RestartAttemptStageDispositionV1 || states[open.attempt.ThreadID] != publicationapp.RestartAttemptCommittedSettlementV1 {
		t.Fatal("mixed fixture lost exact native cuts")
	}
	if open.scope.OwnsThread(closed.attempt.ThreadID) || !open.scope.OwnsThread(open.attempt.ThreadID) || len(open.scope.ThreadIDs()) != 1 {
		t.Fatal("mixed Core hold scope is incorrect")
	}
	if !open.setWitnessAvailable(domainenrollment.SharedEvidenceNamespaceV1, false) {
		t.Fatal("could not disable witness")
	}

	guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, open.core, open.core.access, open.publication.installation, open.advance)
	if err != nil || guard.history == nil || guard.history.mixedClosed == nil || guard.publication.deferred == nil || guard.publication.closed == nil {
		t.Fatal("mixed history lost a proof branch", err)
	}
	protected := []string{open.primary, filepath.Join(open.core.roots.DataDir, "private", "authority-advance")}
	for _, owner := range runtimePublicationOwnersV1 {
		protected = append(protected, filepath.Join(open.core.roots.DataDir, "private", owner))
	}
	for _, fixture := range []*runtimeOriginalReservedReportHistoryFixtureV1{closed, open} {
		id := fixture.attempt.ReportStageWorkID
		protected = append(protected, filepath.Join(open.core.roots.DataDir, "private", "pending-work", "receipts", id[:2], id+".json"))
	}
	id := closed.attempt.ReportStageWorkID
	protected = append(protected, filepath.Join(open.core.roots.DataDir, "private", "pending-work", "dispositions", id[:2], id+".json"))
	frozen := startupWholeTreeRecordMapForTest(t, protected...)
	calls := open.witnessAttempts()
	sharedBefore, _ := open.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
	riskBefore, _ := open.witnessSnapshot(domainenrollment.ThreadRiskNamespaceV1)
	for restart := 0; restart < 2; restart++ {
		before := startupWholeTreeRecordMapForTest(t, open.core.roots.DataDir, open.core.roots.DurableDir)
		handler, err := NewRuntimeServerHandlerE(config)
		if err != nil {
			if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, open.core.roots.DataDir, open.core.roots.DurableDir)) || calls != open.witnessAttempts() {
				t.Fatal("mixed refusal changed managed records or called witness", err)
			}
			t.Fatal("mixed closed/open reports blocked native startup", err)
		}
		shutdownOwnedRuntimeHandler(t, handler)
		if !reflect.DeepEqual(frozen, startupWholeTreeRecordMapForTest(t, protected...)) {
			t.Fatalf("mixed startup changed protected history: restart=%d witnessAttempts=%d", restart, open.witnessAttempts()-calls)
		}
		shared, _ := open.witnessSnapshot(domainenrollment.SharedEvidenceNamespaceV1)
		risk, _ := open.witnessSnapshot(domainenrollment.ThreadRiskNamespaceV1)
		if shared.AdvanceCalls != sharedBefore.AdvanceCalls || shared.ResolveCalls != sharedBefore.ResolveCalls || shared.ObserveCalls != sharedBefore.ObserveCalls ||
			risk.AdvanceCalls != riskBefore.AdvanceCalls || risk.ResolveCalls != riskBefore.ResolveCalls || risk.ObserveCalls != riskBefore.ObserveCalls ||
			!reflect.DeepEqual(shared.Checkpoint, sharedBefore.Checkpoint) || !reflect.DeepEqual(risk.Checkpoint, riskBefore.Checkpoint) {
			t.Fatal("mixed startup changed successful witness counters or checkpoints")
		}
		// As in the closed single-thread fixture, the first fixed non-fact
		// boundary may encounter the offline witness. The second startup must
		// not repeat it. Request counters alone do not identify its endpoint.
		if open.witnessAttempts()-calls != 1 {
			t.Fatalf("mixed first-boundary failed request count: restart=%d attempts=%d", restart, open.witnessAttempts()-calls)
		}
		t.Logf("mixed restart %d: protected records unchanged, failed witness attempts=%d, successful counters/checkpoints unchanged", restart, open.witnessAttempts()-calls)
		id := open.attempt.ReportStageWorkID
		if _, err := os.Lstat(filepath.Join(open.core.roots.DataDir, "private", "pending-work", "dispositions", id[:2], id+".json")); !os.IsNotExist(err) {
			t.Fatal("mixed startup invented open disposition")
		}
		if err := guard.history.semantic.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
			t.Fatal("mixed startup changed exact Core/report graph", err)
		}
	}
}

func TestRuntimeOriginalMixedReportSemanticPreservation(t *testing.T) {
	for _, fault := range []string{"harmless-closed-primary", "closed-disposition", "closed-result", "held-primary", "signed-final-closed-disposition"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			closed, open := newRuntimeOriginalMixedClosedOpenFixtureV1(t)
			core := open.core
			guard, err := prepareRuntimeReportPreservationBeforeRecoveryV1(ctx, core, core.access, open.publication.installation, open.advance)
			if err != nil || guard.history == nil || guard.publication.deferred == nil {
				t.Fatal("mixed guard unavailable", err)
			}
			if err := guard.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
				t.Fatal("original mixed guard failed", err)
			}
			id := closed.attempt.ReportStageWorkID
			disposition := filepath.Join("private", "pending-work", "dispositions", id[:2], id+".json")
			marker := filepath.Join("private", "a-mixed-guard-marker.bin")
			tail := filepath.Join("private", "z-mixed-guard-tail.bin")
			mutation := func(_ context.Context, stage startupport.PersistenceRootsV1) error {
				if err := os.WriteFile(filepath.Join(stage.DataDir, marker), []byte("independent marker"), 0600); err != nil {
					return err
				}
				if fault == "closed-disposition" || fault == "signed-final-closed-disposition" {
					if err := os.Remove(filepath.Join(stage.DataDir, disposition)); err != nil {
						return err
					}
					return os.WriteFile(filepath.Join(stage.DataDir, tail), []byte("later sentinel"), 0600)
				}
				fixture := closed
				if fault == "held-primary" {
					fixture = open
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
				primary["title"] = "ordinary metadata"
				if fault == "closed-result" {
					removed := 0
					for _, raw := range primary["turns"].([]any) {
						turn := raw.(map[string]any)
						var retained []any
						for _, rawItem := range turn["items"].([]any) {
							item := rawItem.(map[string]any)
							if item["kind"] == "tool_result" && item["callId"] == closed.attempt.ToolCallID {
								removed++
								continue
							}
							retained = append(retained, item)
						}
						turn["items"] = retained
					}
					if removed != 1 {
						return errors.New("fixture did not remove exactly one report result")
					}
				}
				body, err = json.Marshal(primary)
				if err != nil {
					return err
				}
				return os.WriteFile(target, body, 0600)
			}
			if fault == "signed-final-closed-disposition" {
				original, err := os.ReadFile(filepath.Join(core.roots.DataDir, disposition))
				if err != nil {
					t.Fatal(err)
				}
				runtimePendingSemanticCutForTestV1(t, core, mutation, marker, tail)
				current, err := os.ReadFile(filepath.Join(core.roots.DataDir, disposition))
				if err != nil || !reflect.DeepEqual(original, current) {
					t.Fatal("Final cut erased Original", err)
				}
				before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
				calls := open.witnessAttempts()
				handler, err := NewRuntimeServerHandlerE(open.config)
				if err == nil {
					shutdownOwnedRuntimeHandler(t, handler)
					t.Fatal("signed Final erased closed disposition")
				}
				if !strings.Contains(err.Error(), "report restart unresolved inventory changed") {
					t.Fatal("mixed Final erasure missed complete Core denominator", err)
				}
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) || calls != open.witnessAttempts() {
					t.Fatal("mixed Final refusal changed state")
				}
				return
			}
			before := startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)
			calls := open.witnessAttempts()
			snapshot, err := persistencefs.NewStartupSnapshotReader(core.roots).CaptureManagedSnapshotV1(ctx)
			if err != nil {
				t.Fatal(err)
			}
			digest := domainsecurity.SHA256Hex([]byte("mixed report semantic/" + fault))
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
			prepared, err := persistencefs.NewSemanticPlanBuilderWithRestartPreservationV1(core.roots, root, journal, guard).Prepare(ctx, baseline, digest, mutation)
			if err != nil {
				t.Fatal("mixed candidate did not prepare", err)
			}
			defer prepared.Close()
			err = prepared.Apply(ctx)
			if fault == "harmless-closed-primary" {
				if err != nil {
					t.Fatal("closed primary harmless rewrite blocked", err)
				}
				if err := guard.ValidateSemanticOperationsV1(ctx, nil, nil, ""); err != nil {
					t.Fatal("mixed graph lost after harmless closed rewrite", err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid mixed candidate applied")
				}
				t.Logf("mixed candidate refused before writes: %v", err)
				if !reflect.DeepEqual(before, startupWholeTreeRecordMapForTest(t, core.roots.DataDir, core.roots.DurableDir)) {
					t.Fatal("mixed refusal changed Original")
				}
			}
			if calls != open.witnessAttempts() {
				t.Fatal("mixed semantic candidate contacted witness")
			}
		})
	}
}
