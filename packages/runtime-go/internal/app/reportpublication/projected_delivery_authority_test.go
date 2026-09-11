package reportpublication

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	domainauthorityadvance "analytix.local/runtime-go/internal/domain/authorityadvance"
	domainpublication "analytix.local/runtime-go/internal/domain/reportpublication"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	evidenceauthorityport "analytix.local/runtime-go/internal/ports/evidenceauthority"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	publicationport "analytix.local/runtime-go/internal/ports/reportpublication"
)

func TestProjectedDeliveryAuthorityRequiresCompleteCurrentProjectedGraph(t *testing.T) {
	authority, selector, projection, fixture, current := newProjectedDeliveryAuthorityFixtureV1(t)
	fixture.evidence.mu.Lock()
	evidenceCallsBefore := fixture.evidence.calls
	fixture.evidence.mu.Unlock()
	fixture.delivery.mu.Lock()
	outcomeReadsBefore := fixture.delivery.resolveCalls
	fixture.delivery.mu.Unlock()
	resolved, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector)
	if err != nil || !reflect.DeepEqual(resolved, projection) {
		t.Fatalf("complete projected graph was not resolved: resolved=%#v err=%v", resolved, err)
	}
	if current.calls != 2 {
		t.Fatalf("authority did not bracket its targeted reads with current-context checks: current=%d", current.calls)
	}
	fixture.evidence.mu.Lock()
	evidenceCalls := fixture.evidence.calls - evidenceCallsBefore
	fixture.evidence.mu.Unlock()
	fixture.delivery.mu.Lock()
	outcomeReads := fixture.delivery.resolveCalls - outcomeReadsBefore
	fixture.delivery.mu.Unlock()
	if evidenceCalls != 1 || outcomeReads != 2 {
		t.Fatalf("current authority split its witnessed snapshot or outcome readback: evidence=%d outcomes=%d", evidenceCalls, outcomeReads)
	}

	fixture.stages.mu.Lock()
	fixture.stages.trustedResultItem = nil
	fixture.stages.mu.Unlock()
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("projected graph without its durable thread result was accepted: %v", err)
	}
}

func TestProjectedDeliveryAuthorityVerifiesHistoricalGraphWithoutGrantingCurrentAuthority(t *testing.T) {
	currentAuthority, selector, projection, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	historicalConfig := currentAuthority.config
	authority, err := NewHistoricalProjectedDeliveryAuthorityV1(historicalConfig)
	if err != nil {
		t.Fatal(err)
	}
	fixture.pii.mu.Lock()
	fixture.pii.failWitnessedAt = 1
	fixture.pii.mu.Unlock()
	historical := historicalProjectedSelectorFromCurrentV1(selector)
	resolved, err := authority.ResolveTrustedHistoricalProjectedDelivery(context.Background(), historical)
	if err != nil || !reflect.DeepEqual(resolved, projection) {
		t.Fatalf("historical projected graph was not verified independently of current authority: resolved=%#v err=%v", resolved, err)
	}
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryUnavailable) {
		t.Fatalf("historical-only authority gained current release authority: %v", err)
	}
	fixture.artifacts.mu.Lock()
	fixture.artifacts.records[fixture.input.TargetIdentityDigest] = []byte("historically corrupted artifact")
	fixture.artifacts.mu.Unlock()
	if _, err := authority.ResolveTrustedHistoricalProjectedDelivery(context.Background(), historical); !errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("historical verifier accepted a corrupted durable artifact graph: %v", err)
	}
}

func TestProjectedDeliveryAuthorityRejectsMissingMismatchedAndIncompleteOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *reportPublicationFixture, *publicationport.ProjectedDeliverySelectorV1)
		want   error
	}{
		{
			name: "missing outcome",
			mutate: func(_ *testing.T, fixture *reportPublicationFixture, _ *publicationport.ProjectedDeliverySelectorV1) {
				fixture.delivery.outcome = domainpublication.ReportDeliveryOutcomeV1{}
				fixture.delivery.projection = domainpublication.ReportDeliveryProjectionV1{}
			},
			want: ErrProjectedDeliveryNotFound,
		},
		{
			name: "mismatched outcome digest",
			mutate: func(_ *testing.T, _ *reportPublicationFixture, selector *publicationport.ProjectedDeliverySelectorV1) {
				selector.OutcomeRecordDigest = domainsecurity.SHA256Hex([]byte("another projected outcome"))
			},
			want: ErrProjectedDeliveryIntegrity,
		},
		{
			name: "missing completed pending disposition",
			mutate: func(_ *testing.T, fixture *reportPublicationFixture, _ *publicationport.ProjectedDeliverySelectorV1) {
				fixture.stages.disposition = nil
			},
			want: ErrProjectedDeliveryIntegrity,
		},
		{
			name: "missing stage completion",
			mutate: func(_ *testing.T, fixture *reportPublicationFixture, _ *publicationport.ProjectedDeliverySelectorV1) {
				fixture.stageCompletions.records = map[string]domainpublication.ReportStageCompletionV1{}
			},
			want: ErrProjectedDeliveryIntegrity,
		},
		{
			name: "missing delivery decision",
			mutate: func(_ *testing.T, fixture *reportPublicationFixture, _ *publicationport.ProjectedDeliverySelectorV1) {
				fixture.decisions.records = map[string]domainpublication.ReportDeliveryDecisionV1{}
			},
			want: ErrProjectedDeliveryIntegrity,
		},
		{
			name: "missing grant settlement",
			mutate: func(_ *testing.T, fixture *reportPublicationFixture, _ *publicationport.ProjectedDeliverySelectorV1) {
				fixture.grantSettlements.records = map[string]domainpublication.ReportGrantSettlementV1{}
			},
			want: ErrProjectedDeliveryIntegrity,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
			test.mutate(t, fixture, &selector)
			if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, test.want) {
				t.Fatalf("unsafe projected graph result: got=%v want=%v", err, test.want)
			}
		})
	}
}

func TestProjectedDeliveryAuthorityRejectsOutcomeChangedAfterGraphRead(t *testing.T) {
	authority, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	authority.config.DeliveryOutcomes = &sequencedProjectedOutcomeResolverV1{
		first:     fixture.delivery.outcome,
		secondErr: publicationport.ErrNotFound,
	}
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("changed stable outcome view authorized release: %v", err)
	}
}

func TestProjectedDeliveryAuthorityLinearizesUseInsideWitnessedSnapshot(t *testing.T) {
	authority, selector, projection, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	fixture.evidence.mu.Lock()
	evidenceCallsBefore := fixture.evidence.calls
	fixture.evidence.mu.Unlock()
	fixture.delivery.mu.Lock()
	outcomeReadsBefore := fixture.delivery.resolveCalls
	fixture.delivery.mu.Unlock()
	useCalls := 0
	err := authority.WithCurrentProjectedDelivery(context.Background(), selector, func(candidate domainpublication.ReportDeliveryProjectionV1) error {
		useCalls++
		if !reflect.DeepEqual(candidate, projection) {
			t.Fatalf("linearized use received another projection: %#v", candidate)
		}
		if fixture.evidence.mu.TryLock() {
			fixture.evidence.mu.Unlock()
			t.Fatal("linearized use ran after the witnessed snapshot was released")
		}
		return nil
	})
	if err != nil || useCalls != 1 {
		t.Fatalf("linearized projected use failed: calls=%d err=%v", useCalls, err)
	}
	fixture.evidence.mu.Lock()
	evidenceCalls := fixture.evidence.calls - evidenceCallsBefore
	fixture.evidence.mu.Unlock()
	fixture.delivery.mu.Lock()
	outcomeReads := fixture.delivery.resolveCalls - outcomeReadsBefore
	fixture.delivery.mu.Unlock()
	if evidenceCalls != 1 || outcomeReads != 3 {
		t.Fatalf("linearized use did not hold one snapshot and bracket use with outcome reads: evidence=%d outcomes=%d", evidenceCalls, outcomeReads)
	}
}

func TestProjectedDeliveryAuthorityFailsClosedWhenOutcomeChangesAfterLinearizedUse(t *testing.T) {
	authority, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	authority.config.DeliveryOutcomes = &threeReadProjectedOutcomeResolverV1{
		outcome: fixture.delivery.outcome, thirdErr: publicationport.ErrNotFound,
	}
	useCalls := 0
	err := authority.WithCurrentProjectedDelivery(context.Background(), selector, func(domainpublication.ReportDeliveryProjectionV1) error {
		useCalls++
		return nil
	})
	if useCalls != 1 || !errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("outcome change after linearized side effect was accepted: calls=%d err=%v", useCalls, err)
	}
}

func TestProjectedDeliveryAuthorityReturnsLinearizedUseFailureWithoutReclassifyingIt(t *testing.T) {
	authority, selector, _, _, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	useFailure := errors.New("trusted controlled sink rejected release")
	err := authority.WithCurrentProjectedDelivery(context.Background(), selector, func(domainpublication.ReportDeliveryProjectionV1) error {
		return useFailure
	})
	if !errors.Is(err, useFailure) || errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("trusted-use failure was hidden behind graph classification: %v", err)
	}
}

func TestProjectedDeliveryAuthorityFailureClassesAreClosedAndExclusive(t *testing.T) {
	t.Run("dependency cancellation", func(t *testing.T) {
		authority, selector, _, _, _ := newProjectedDeliveryAuthorityFixtureV1(t)
		authority.config.DeliveryOutcomes = projectedOutcomeFailureStubV1{err: context.Canceled}
		_, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector)
		assertProjectedDeliveryFailureClassV1(t, err, ErrProjectedDeliveryCancelled)
		if errors.Is(err, ErrProjectedDeliveryIntegrity) {
			t.Fatalf("dependency cancellation retained an integrity classification: %v", err)
		}
	})

	t.Run("witness unavailable", func(t *testing.T) {
		authority, selector, _, _, _ := newProjectedDeliveryAuthorityFixtureV1(t)
		authority.config.Evidence = projectedWitnessFailureStubV1{err: errors.New("witness store unavailable")}
		_, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector)
		assertProjectedDeliveryFailureClassV1(t, err, ErrProjectedDeliveryUnavailable)
	})
}

func TestProjectedDeliveryAuthorityReleasesPartialEffectLeaseOnAcquireFailure(t *testing.T) {
	authority, selector, _, _, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	releases := 0
	authority.config.AcquireContextEffect = func(
		ctx context.Context,
		_ domainsecurity.TurnSecurityContext,
	) (context.Context, func(), error) {
		return ctx, func() { releases++ }, errors.New("effect binding changed")
	}
	_, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector)
	if !errors.Is(err, ErrProjectedDeliveryStale) || releases != 1 {
		t.Fatalf("partial effect lease leaked or changed classification: releases=%d err=%v", releases, err)
	}
}

func TestProjectedDeliveryAuthorityRejectsRejectedWinnerAndStaleContext(t *testing.T) {
	authority, selector, _, fixture, current := newProjectedDeliveryAuthorityFixtureV1(t)
	entry := fixtureProjectedEntryV1(t, fixture)
	rejection, err := domainpublication.NewReportDeliveryRejectionV1(
		domainpublication.ReportDeliveryRejectionInputV1{
			Decision: *entry.Decision, GrantSettlement: *entry.GrantSettlement,
			StageReceipt: entry.Stage, StageDisposition: *entry.Disposition, StageCompletion: *entry.StageCompletion,
			ReasonCode:                    domainpublication.ReportDeliveryRejectionEvidenceChangedV1,
			WitnessObservationDigest:      domainsecurity.SHA256Hex([]byte("projected authority changed witness")),
			EvidenceAuthorityBundleDigest: domainsecurity.SHA256Hex([]byte("projected authority changed bundle")),
			EvidenceRegistryIndexDigest:   domainsecurity.SHA256Hex([]byte("projected authority changed index")),
			EvidenceRegistrySequence:      entry.Decision.EvidenceRegistrySequence + 1,
			EvidenceRegistryStateDigest:   domainsecurity.SHA256Hex([]byte("projected authority changed state")),
			AuthorityKeyID:                fixture.authority.keyID,
			AuthorityPublicKey:            fixture.authority.publicKey,
		},
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := domainpublication.RejectedReportDeliveryOutcomeV1(rejection)
	if err != nil {
		t.Fatal(err)
	}
	fixture.delivery.outcome = rejected
	fixture.delivery.projection = domainpublication.ReportDeliveryProjectionV1{}
	selector.OutcomeRecordDigest = rejection.RecordDigest
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryRejected) {
		t.Fatalf("rejected winner authorized release: %v", err)
	}

	authority, selector, _, _, current = newProjectedDeliveryAuthorityFixtureV1(t)
	current.failAt = 2
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryStale) {
		t.Fatalf("context change during graph verification authorized release: %v", err)
	}
}

func TestProjectedDeliveryAuthorityRejectsIncompleteWitnessAndArtifactGraph(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*reportPublicationFixture)
	}{
		{
			name: "missing authority intent",
			mutate: func(fixture *reportPublicationFixture) {
				fixture.coordinator.mu.Lock()
				fixture.coordinator.intent = domainauthorityadvance.MonotonicAdvanceIntentV2{}
				fixture.coordinator.mu.Unlock()
			},
		},
		{
			name: "missing authority settlement",
			mutate: func(fixture *reportPublicationFixture) {
				fixture.coordinator.mu.Lock()
				fixture.coordinator.settlement = domainauthorityadvance.MonotonicAdvanceSettlementV2{}
				fixture.coordinator.mu.Unlock()
			},
		},
		{
			name: "missing witness observation",
			mutate: func(fixture *reportPublicationFixture) {
				fixture.observations.mu.Lock()
				fixture.observations.records = map[string]evidenceauthorityport.ObservationBundle{}
				fixture.observations.mu.Unlock()
			},
		},
		{
			name: "artifact hash mismatch",
			mutate: func(fixture *reportPublicationFixture) {
				fixture.artifacts.mu.Lock()
				fixture.artifacts.records[fixture.input.TargetIdentityDigest] = []byte("tampered report artifact")
				fixture.artifacts.mu.Unlock()
			},
		},
		{
			name: "untrusted completed stage",
			mutate: func(fixture *reportPublicationFixture) {
				fixture.stages.mu.Lock()
				value := *fixture.stages.disposition
				value.AuthoritySignature = "tampered"
				fixture.stages.disposition = &value
				fixture.stages.mu.Unlock()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authority, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
			test.mutate(fixture)
			if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryIntegrity) {
				t.Fatalf("incomplete exact delivery graph acquired current authority: %v", err)
			}
		})
	}
}

func TestProjectedDeliveryAuthorityRejectsEvidenceRevokedAfterProjection(t *testing.T) {
	authority, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	revoked := fixture.revokedSnapshotAfterPublication(t)
	fixture.evidence.mu.Lock()
	fixture.evidence.third = revoked
	fixture.evidence.headReader = nil
	fixture.evidence.mu.Unlock()
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("revoked evidence retained live projected authority: %v", err)
	}
}

func TestProjectedDeliveryAuthorityRejectsCommitOutsideCurrentWitnessAncestry(t *testing.T) {
	authority, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureV1(t)
	fixture.evidence.mu.Lock()
	fixture.evidence.third = fixture.initialSnapshot
	fixture.evidence.headReader = nil
	fixture.evidence.mu.Unlock()
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("commit outside the current witnessed ancestry retained live authority: %v", err)
	}
}

func TestProjectedDeliveryAuthorityRejectsControlledPIIRevokedAfterProjection(t *testing.T) {
	authority, selector, _, fixture, _ := newProjectedDeliveryAuthorityFixtureForProjectionV1(
		t, domainpublication.PIIProjectionControlledFull,
	)
	fixture.pii.mu.Lock()
	fixture.pii.failWitnessedAt = fixture.pii.witnessedCalls + 1
	fixture.pii.mu.Unlock()
	if _, err := authority.ResolveCurrentProjectedDelivery(context.Background(), selector); !errors.Is(err, ErrProjectedDeliveryIntegrity) {
		t.Fatalf("revoked controlled PII grant retained live projected authority: %v", err)
	}
}

func newProjectedDeliveryAuthorityFixtureV1(
	t *testing.T,
) (*ProjectedDeliveryAuthorityV1, publicationport.ProjectedDeliverySelectorV1, domainpublication.ReportDeliveryProjectionV1, *reportPublicationFixture, *projectedCurrentContextStubV1) {
	return newProjectedDeliveryAuthorityFixtureForProjectionV1(t, domainpublication.PIIProjectionOrdinaryMasked)
}

func newProjectedDeliveryAuthorityFixtureForProjectionV1(
	t *testing.T,
	projectionClass string,
) (*ProjectedDeliveryAuthorityV1, publicationport.ProjectedDeliverySelectorV1, domainpublication.ReportDeliveryProjectionV1, *reportPublicationFixture, *projectedCurrentContextStubV1) {
	t.Helper()
	fixture := newReportPublicationFixture(t, projectionClass)
	published, err := fixture.service.Publish(context.Background(), fixture.input)
	if err != nil {
		t.Fatal(err)
	}
	settlement := fixture.installDurableReportGrantSettlement(t, published.Decision)
	completion := fixture.installDurableReportStageCompletion(t, published.Decision, settlement)
	projection, err := domainpublication.NewReportDeliveryProjectionV1(
		domainpublication.ReportDeliveryProjectionInputV1{
			Decision: published.Decision, GrantSettlement: settlement,
			StageReceipt: fixture.stages.receipt, StageDisposition: *fixture.stages.disposition, StageCompletion: completion,
			AuthorityKeyID: fixture.authority.keyID, AuthorityPublicKey: fixture.authority.publicKey,
		},
		func(message []byte) ([]byte, error) { return fixture.authority.Sign(context.Background(), message) },
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := domainpublication.ProjectedReportDeliveryOutcomeV1(projection)
	if err != nil {
		t.Fatal(err)
	}
	fixture.delivery.outcome = outcome
	fixture.delivery.projection = projection
	current := &projectedCurrentContextStubV1{current: fixture.input.PendingToolCall.SecurityContext}
	bundle := fixture.initialSnapshot.Head.Bundle
	authority, err := NewProjectedDeliveryAuthorityV1(ProjectedDeliveryAuthorityConfigV1{
		InstallationID: bundle.InstallationID, EnrollmentID: bundle.EnrollmentID,
		WitnessKeyID: domainsecurity.SHA256Hex(fixture.coordinator.witnessPublic), WitnessKey: fixture.coordinator.witnessPublic,
		Attempts: fixture.attempts, Receipts: fixture.receipts, Indexes: fixture.indexes,
		Selections: fixture.selections, Commits: fixture.commits, Decisions: fixture.decisions,
		GrantSettlements: fixture.grantSettlements, StageCompletions: fixture.stageCompletions,
		DeliveryOutcomes: fixture.delivery, Ledgers: fixture.ledgers, Projections: fixture.projections,
		Inspections: fixture.inspections, Intents: fixture.coordinator, Settlements: fixture.coordinator,
		Bundles: fixture.bundles, Observations: fixture.observations, Artifacts: fixture.artifacts,
		ControlledMetadata: fixture.artifacts, Pending: fixture.stages,
		Evidence: fixture.evidence, PIIAuthority: fixture.pii,
		Authority: fixture.authority, ValidateCurrent: current.Validate, AcquireContextEffect: fixture.effectGate.AcquireEffect,
	})
	if err != nil {
		t.Fatal(err)
	}
	return authority, publicationport.ProjectedDeliverySelectorV1{
		SecurityContext: fixture.input.PendingToolCall.SecurityContext,
		DeliveryID:      projection.DeliveryID, OutcomeRecordDigest: projection.RecordDigest,
		PublicationCommitDigest: projection.CommitRecordDigest,
	}, projection, fixture, current
}

func fixtureProjectedEntryV1(t *testing.T, fixture *reportPublicationFixture) RestartAttemptV1 {
	t.Helper()
	plan, err := fixture.restartPlan()
	if err != nil || len(plan.Attempts) != 1 {
		t.Fatalf("resolve projected fixture plan: plan=%#v err=%v", plan, err)
	}
	return plan.Attempts[0]
}

func historicalProjectedSelectorFromCurrentV1(
	selector publicationport.ProjectedDeliverySelectorV1,
) publicationport.HistoricalProjectedDeliverySelectorV1 {
	securityContext := selector.SecurityContext
	return publicationport.HistoricalProjectedDeliverySelectorV1{
		DeliveryID: selector.DeliveryID, OutcomeRecordDigest: selector.OutcomeRecordDigest,
		PublicationCommitDigest: selector.PublicationCommitDigest,
		ThreadID:                securityContext.ThreadID, TurnID: securityContext.TurnID,
		ContextDigest: securityContext.ContextDigest, CaseBindingHash: securityContext.CaseBindingHash,
		ContextEpoch: securityContext.ContextEpoch, DatasetSnapshotID: securityContext.DatasetSnapshotID,
		SourceManifestHash: securityContext.SourceManifestHash,
	}
}

type projectedCurrentContextStubV1 struct {
	mu      sync.Mutex
	current domainsecurity.TurnSecurityContext
	failAt  int
	calls   int
}

type sequencedProjectedOutcomeResolverV1 struct {
	mu        sync.Mutex
	calls     int
	first     domainpublication.ReportDeliveryOutcomeV1
	second    domainpublication.ReportDeliveryOutcomeV1
	secondErr error
}

type threeReadProjectedOutcomeResolverV1 struct {
	mu       sync.Mutex
	calls    int
	outcome  domainpublication.ReportDeliveryOutcomeV1
	thirdErr error
}

type projectedOutcomeFailureStubV1 struct {
	err error
}

type projectedWitnessFailureStubV1 struct {
	err error
}

func (resolver *sequencedProjectedOutcomeResolverV1) ResolveOutcome(
	context.Context,
	string,
) (domainpublication.ReportDeliveryOutcomeV1, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.calls++
	if resolver.calls == 1 {
		return resolver.first, nil
	}
	return resolver.second, resolver.secondErr
}

func (resolver *threeReadProjectedOutcomeResolverV1) ResolveOutcome(
	context.Context,
	string,
) (domainpublication.ReportDeliveryOutcomeV1, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.calls++
	if resolver.calls == 3 {
		return domainpublication.ReportDeliveryOutcomeV1{}, resolver.thirdErr
	}
	return resolver.outcome, nil
}

func (stub projectedOutcomeFailureStubV1) ResolveOutcome(
	context.Context,
	string,
) (domainpublication.ReportDeliveryOutcomeV1, error) {
	return domainpublication.ReportDeliveryOutcomeV1{}, stub.err
}

func (stub projectedWitnessFailureStubV1) WithWitnessedSnapshot(
	context.Context,
	domainsecurity.TurnSecurityContext,
	func(registryport.WitnessedSnapshot) error,
) error {
	return stub.err
}

func assertProjectedDeliveryFailureClassV1(t *testing.T, err error, want error) {
	t.Helper()
	classes := []error{
		ErrProjectedDeliveryUnavailable, ErrProjectedDeliveryInvalid, ErrProjectedDeliveryStale,
		ErrProjectedDeliveryNotFound, ErrProjectedDeliveryRejected, ErrProjectedDeliveryIntegrity,
		ErrProjectedDeliveryCancelled,
	}
	matches := 0
	for _, class := range classes {
		if errors.Is(err, class) {
			matches++
		}
	}
	if !errors.Is(err, want) || matches != 1 {
		t.Fatalf("projected delivery error class is not closed and exclusive: got=%v want=%v matches=%d", err, want, matches)
	}
}

func (stub *projectedCurrentContextStubV1) Validate(_ context.Context, candidate domainsecurity.TurnSecurityContext) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.calls++
	if candidate != stub.current || stub.failAt > 0 && stub.calls >= stub.failAt {
		return errors.New("projected delivery context changed")
	}
	return nil
}
