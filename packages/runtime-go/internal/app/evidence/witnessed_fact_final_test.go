package evidence

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
	sourceprobeport "analytix.local/runtime-go/internal/ports/sourceprobe"
	caseterminaltest "analytix.local/runtime-go/internal/testsupport/caseturnterminal"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type factFinalSourceLeaseStub struct {
	active         atomic.Bool
	calls          atomic.Int32
	err            error
	corrupt        bool
	beforeCallback func()
}

type factFinalHostAuthoritySpy struct {
	calls atomic.Int32
}

func (spy *factFinalHostAuthoritySpy) WithFreshPublicationSnapshotAuthority(
	context.Context,
	sourceprobeport.PublicationInput,
	func([]domainsecurity.VerifiedSourceProbe, sourceprobeport.HostEvidenceCapability) error,
) error {
	spy.calls.Add(1)
	return errors.New("test host evidence authority must not be used for an ordinary terminal")
}

func (stub *factFinalSourceLeaseStub) WithFreshPublicationSnapshot(
	_ context.Context,
	input sourceprobeport.PublicationInput,
	callback func([]domainsecurity.VerifiedSourceProbe) error,
) error {
	stub.calls.Add(1)
	if stub.err != nil {
		return stub.err
	}
	probes := make([]domainsecurity.VerifiedSourceProbe, 0, len(input.Requirements))
	seen := map[string]bool{}
	for _, requirement := range input.Requirements {
		if seen[requirement.ServerID] {
			continue
		}
		seen[requirement.ServerID] = true
		identity, err := domainsecurity.ParseVerifiedMCPServerIdentity(requirement.ServerIdentity)
		if err != nil {
			return err
		}
		snapshotID := input.Context.DatasetSnapshotID
		if stub.corrupt {
			snapshotID = securitycontexttest.DatasetSnapshotID("fact-final-source-mismatch")
		}
		checkedAt := evidenceIssuerTime().Add(4 * time.Second)
		probe, err := domainsecurity.NewVerifiedSourceProbe(domainsecurity.VerifiedSourceProbeInput{
			ServerID: requirement.ServerID, ServerIdentity: requirement.ServerIdentity,
			ConnectionEpoch:    requirement.ConnectionEpoch,
			CatalogFingerprint: domainsecurity.SHA256Hex([]byte("fact-final-catalog")),
			SpecFingerprint:    domainsecurity.SHA256Hex([]byte("fact-final-spec")),
			ThreadID:           input.Context.ThreadID, TurnID: input.Context.TurnID,
			ContextEpoch: input.Context.ContextEpoch, ContextDigest: input.Context.ContextDigest,
			DatasetSnapshotID: snapshotID, CheckedAt: checkedAt,
			Response: domainsecurity.SourceProbeResponse{
				Version: domainsecurity.SourceProbeVersion, ServerName: identity.ObservedName,
				ServerVersion: identity.ObservedVersion, CaseID: input.Context.CaseID,
				CaseBindingHash: input.Context.CaseBindingHash, DatasetSnapshotID: snapshotID,
				Ready: true, ReadOnly: true, CheckedAt: checkedAt.Format(time.RFC3339Nano),
			},
		})
		if err != nil {
			return err
		}
		probes = append(probes, probe)
	}
	stub.active.Store(true)
	defer stub.active.Store(false)
	if stub.beforeCallback != nil {
		stub.beforeCallback()
	}
	return callback(probes)
}

type factFinalRegistryStub struct {
	*lockedMemoryEvidenceRegistry
	lockDepth       atomic.Int32
	lockCalls       atomic.Int32
	issueCalls      atomic.Int32
	orderViolations atomic.Int32
	source          *factFinalSourceLeaseStub
}

func (registry *factFinalRegistryStub) WithLockedSnapshot(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	callback func(domainevidence.EvidenceReceiptRegistry) error,
) error {
	registry.lockCalls.Add(1)
	registry.lockedMemoryEvidenceRegistry.mu.Lock()
	defer registry.lockedMemoryEvidenceRegistry.mu.Unlock()
	registry.lockDepth.Add(1)
	defer registry.lockDepth.Add(-1)
	snapshot, err := registry.memoryEvidenceRegistry.current(securityContext)
	if err != nil {
		return err
	}
	frozen, err := domainevidence.ParseEvidenceReceiptRegistry(snapshot)
	if err != nil {
		return err
	}
	return callback(frozen)
}

func (registry *factFinalRegistryStub) WithFactFinalWitnessAuthority(
	_ context.Context,
	_ registryport.FactFinalWitnessRequest,
	_ func(registryport.FactFinalWitnessCapability) error,
) error {
	registry.issueCalls.Add(1)
	if registry.lockDepth.Load() != 0 || registry.source == nil || !registry.source.active.Load() {
		registry.orderViolations.Add(1)
	}
	return errors.New("test fact-final witness authority is intentionally unavailable")
}

func (registry *factFinalRegistryStub) revokeLocked(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	receiptID string,
) error {
	registry.lockedMemoryEvidenceRegistry.mu.Lock()
	defer registry.lockedMemoryEvidenceRegistry.mu.Unlock()
	return registry.memoryEvidenceRegistry.Revoke(ctx, registryport.RevokeInput{
		Context: securityContext, ReceiptID: receiptID, ReasonCode: "source_registry_changed",
		RevokedAt: time.Now().UTC(),
	})
}

type factFinalBoundaryFixture struct {
	finalizer    CasePublicationFinalizer
	registry     *factFinalRegistryStub
	source       *factFinalSourceLeaseStub
	privateStore *memoryPrivateFinalStore
	store        *caseTerminalStoreStub
	input        IssueEvidenceInput
	receipt      domainevidence.EvidenceReceipt
}

func newFactFinalBoundaryFixture(
	t *testing.T,
	source *factFinalSourceLeaseStub,
) factFinalBoundaryFixture {
	t.Helper()
	issuer, input, _ := claimEvidenceFixture(
		t, domainevidence.ClaimAmount, amountClaimPayload(), "transactions", domainevidence.PaginationComplete,
	)
	receipt := seedPreauthorizedRegistryForGateUnitTest(t, issuer, input)
	memoryRegistry := issuer.Registry.(*memoryEvidenceRegistry)
	registry := &factFinalRegistryStub{
		lockedMemoryEvidenceRegistry: &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: memoryRegistry},
		source:                       source,
	}
	authority := newMemoryFinalAuthority(0x67)
	privateStore := &memoryPrivateFinalStore{
		records:      map[string]domainevidence.PrivateAcceptedFinalRecord{},
		dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
	}
	coordinator, err := caseterminaltest.NewInMemoryCoordinatorV1(authority, privateStore)
	if err != nil {
		t.Fatal(err)
	}
	eventIO := newTestFinalPublicationEventIO()
	finalizer := NewCasePublicationFinalizerWithPublicationSnapshots(
		registry, registry, authority, privateStore, eventIO, coordinator, source,
	)
	return factFinalBoundaryFixture{
		finalizer: finalizer, registry: registry, source: source, privateStore: privateStore,
		store: &caseTerminalStoreStub{}, input: input, receipt: receipt,
	}
}

func persistFactFinalBoundaryForTest(
	ctx context.Context,
	fixture factFinalBoundaryFixture,
) (PersistCaseBoundaryResult, error) {
	return fixture.finalizer.PersistBoundary(ctx, PersistCaseBoundaryInput{
		Store: fixture.store, Context: fixture.input.Context, TerminalReason: TerminalSuccess,
		ThreadID: fixture.input.Context.ThreadID, TurnID: fixture.input.Context.TurnID,
		AcceptedAt: evidenceIssuerTime().Add(10 * time.Second),
	})
}

func assertPublicationSnapshotBoundaryOnly(
	t *testing.T,
	fixture factFinalBoundaryFixture,
	result PersistCaseBoundaryResult,
	err error,
) {
	t.Helper()
	if err != nil || !result.Persistence.Changed ||
		result.Boundary.Envelope.Variant != domainevidence.NeedsEvidenceAnswer ||
		result.Boundary.Envelope.Blocker != "publication_snapshot_not_fresh" ||
		len(result.Boundary.Envelope.Claims) != 0 ||
		result.Persistence.AcceptedFinal.FactFinalWitnessAdmission != nil ||
		result.Persistence.AcceptedFinal.PublicationSnapshotProofDigest != "" {
		t.Fatalf("fact-final authority failure escaped boundary-only: result=%#v err=%v", result, err)
	}
	records, listErr := fixture.privateStore.List(context.Background())
	if listErr != nil || len(records) != 1 ||
		records[0].Envelope.Variant != domainevidence.NeedsEvidenceAnswer ||
		len(records[0].Envelope.Claims) != 0 ||
		records[0].PublicationSnapshotProof != nil ||
		records[0].AcceptedFinal.FactFinalWitnessAdmission != nil {
		t.Fatalf("fact-final authority failure persisted non-boundary state: records=%#v err=%v", records, listErr)
	}
}

func TestFactFinalPublicationReleasesRegistryLockBeforeSourceLeaseAndFailsClosedWithoutHostSnapshotAuthority(t *testing.T) {
	source := &factFinalSourceLeaseStub{}
	fixture := newFactFinalBoundaryFixture(t, source)
	source.beforeCallback = func() {
		if fixture.registry.lockDepth.Load() != 0 {
			fixture.registry.orderViolations.Add(1)
		}
	}
	type outcome struct {
		result PersistCaseBoundaryResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := persistFactFinalBoundaryForTest(context.Background(), fixture)
		done <- outcome{result: result, err: err}
	}()
	var completed outcome
	select {
	case completed = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fact-final source lease deadlocked with the registry snapshot lock")
	}
	assertPublicationSnapshotBoundaryOnly(t, fixture, completed.result, completed.err)
	if source.calls.Load() != 1 || fixture.registry.issueCalls.Load() != 0 ||
		fixture.registry.orderViolations.Load() != 0 || fixture.registry.lockCalls.Load() != 2 {
		t.Fatalf("fact-final fail-closed ordering drifted: source=%d witness=%d ordering=%d registryLocks=%d",
			source.calls.Load(), fixture.registry.issueCalls.Load(), fixture.registry.orderViolations.Load(),
			fixture.registry.lockCalls.Load())
	}
}

func TestFactFinalPublicationRegistryDriftConcurrentlyReacquiresCurrentBoundarySnapshot(t *testing.T) {
	source := &factFinalSourceLeaseStub{}
	fixture := newFactFinalBoundaryFixture(t, source)
	leaseEntered := make(chan struct{})
	releaseLease := make(chan struct{})
	source.beforeCallback = func() {
		if fixture.registry.lockDepth.Load() != 0 {
			fixture.registry.orderViolations.Add(1)
		}
		close(leaseEntered)
		<-releaseLease
	}
	type outcome struct {
		result PersistCaseBoundaryResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := persistFactFinalBoundaryForTest(context.Background(), fixture)
		done <- outcome{result: result, err: err}
	}()
	select {
	case <-leaseEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("source lease did not start after preliminary registry snapshot")
	}
	if err := fixture.registry.revokeLocked(
		context.Background(), fixture.input.Context, fixture.receipt.ReceiptID,
	); err != nil {
		t.Fatal(err)
	}
	close(releaseLease)
	var completed outcome
	select {
	case completed = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent registry drift deadlocked fact-final downgrade")
	}
	assertPublicationSnapshotBoundaryOnly(t, fixture, completed.result, completed.err)
	if fixture.registry.issueCalls.Load() != 0 ||
		fixture.registry.orderViolations.Load() != 0 ||
		fixture.registry.lockCalls.Load() != 2 {
		t.Fatalf("registry drift did not use released preliminary plus fresh boundary snapshots: witness=%d ordering=%d locks=%d",
			fixture.registry.issueCalls.Load(), fixture.registry.orderViolations.Load(), fixture.registry.lockCalls.Load())
	}
}

func TestFactFinalPublicationUnavailableAuthorityIsBoundaryOnly(t *testing.T) {
	for name, testCase := range map[string]struct {
		configure           func(*factFinalBoundaryFixture)
		expectedSourceCalls int32
	}{
		"source unavailable": {
			configure: func(fixture *factFinalBoundaryFixture) {
				fixture.source.err = errors.New("test source unavailable")
			},
			expectedSourceCalls: 1,
		},
		"source mismatch": {
			configure: func(fixture *factFinalBoundaryFixture) {
				fixture.source.corrupt = true
			},
			expectedSourceCalls: 1,
		},
		"source authority missing": {
			configure: func(fixture *factFinalBoundaryFixture) {
				fixture.finalizer.(*casePublicationFinalizer).publicationSnapshots = nil
			},
			expectedSourceCalls: 0,
		},
		"witness authority missing": {
			configure: func(fixture *factFinalBoundaryFixture) {
				fixture.finalizer.(*casePublicationFinalizer).factFinalWitnesses = nil
			},
			expectedSourceCalls: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newFactFinalBoundaryFixture(t, &factFinalSourceLeaseStub{})
			testCase.configure(&fixture)
			result, err := persistFactFinalBoundaryForTest(context.Background(), fixture)
			assertPublicationSnapshotBoundaryOnly(t, fixture, result, err)
			if fixture.source.calls.Load() != testCase.expectedSourceCalls ||
				fixture.registry.issueCalls.Load() != 0 ||
				fixture.registry.lockCalls.Load() != 2 {
				t.Fatalf("unexpected fail-closed calls: source=%d witness=%d locks=%d",
					fixture.source.calls.Load(), fixture.registry.issueCalls.Load(), fixture.registry.lockCalls.Load())
			}
		})
	}
}

func TestOrdinaryCaseTerminalDoesNotRequireHostDSV2Authority(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	registry := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}
	privateStore := &memoryPrivateFinalStore{
		records:      map[string]domainevidence.PrivateAcceptedFinalRecord{},
		dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
	}
	authority := newMemoryFinalAuthority(0x68)
	coordinator, err := caseterminaltest.NewInMemoryCoordinatorV1(authority, privateStore)
	if err != nil {
		t.Fatal(err)
	}
	hostAuthority := &factFinalHostAuthoritySpy{}
	finalizer := NewCasePublicationFinalizerWithHostEvidenceAuthority(
		registry,
		registry,
		authority,
		privateStore,
		newTestFinalPublicationEventIO(),
		coordinator,
		hostAuthority,
		nil,
	)
	slot, err := domainordinaryresult.NewResultSlotV1("The ordinary terminal completed.")
	if err != nil {
		t.Fatal(err)
	}
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		OrdinaryResult: &slot, CaseSlotIntent: CaseSlotNotRequestedV1,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID,
		AcceptedAt: evidenceIssuerTime().Add(10 * time.Second),
	})
	if err != nil || !result.Persistence.Changed ||
		result.Boundary.Envelope.Variant != domainevidence.GeneralGuidanceAnswer ||
		result.Boundary.Text != slot.Text || hostAuthority.calls.Load() != 0 {
		t.Fatalf("ordinary terminal entered host DSV2 authority: result=%#v calls=%d err=%v", result, hostAuthority.calls.Load(), err)
	}
}
