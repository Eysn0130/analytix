package evidence

import (
	"context"
	"errors"
	"testing"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestFailedAcceptedFinalEventBundleNeverActivatesPublicProjection(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	registry := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}
	authority := newMemoryFinalAuthority(9)
	privateStore := &memoryPrivateFinalStore{
		records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
	}
	eventIO := newTestFinalPublicationEventIO()
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback)
	eventIO.AppendEvents = func(context.Context, appturn.AcceptedFinalCompletionStore, []map[string]any) ([]map[string]any, error) {
		return nil, errors.New("atomic bundle write failed before commit")
	}
	finalizer := NewCasePublicationFinalizerWithPublicationSnapshots(registry, registry, authority, privateStore, eventIO, newTestTurnTerminalCoordinator(authority, privateStore), nil, index)
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err == nil || !result.Persistence.Changed {
		t.Fatalf("failed event publication did not preserve the quarantined CAS result: result=%#v err=%v", result, err)
	}
	callbackCalls := 0
	available, useErr := result.UseCaseLongitudinalAcceptedSlotsV1(func(
		domainsecurity.TurnSecurityContext,
		string,
		string,
		string,
		domainevidence.AcceptedEntitySlotBindingV1,
	) error {
		callbackCalls++
		return nil
	})
	if useErr != nil || available || callbackCalls != 0 {
		t.Fatalf("failed event publication exposed longitudinal slot authority: available=%t calls=%d err=%v", available, callbackCalls, useErr)
	}
	digest := result.Persistence.AcceptedFinal.RecordDigest
	if _, ok := index.Resolve(input.Context.ThreadID, input.Context.TurnID); ok {
		t.Fatal("failed event bundle activated the thread snapshot projection")
	}
	if _, _, ok := index.ResolveForEvent(input.Context.ThreadID, input.Context.TurnID); ok {
		t.Fatal("failed event bundle left provisional live-event authority behind")
	}
	disposition, resolveErr := privateStore.ResolveDisposition(context.Background(), digest)
	if resolveErr != nil || disposition.State != domainevidence.AcceptedFinalCommitted {
		t.Fatalf("quarantined public CAS lost deterministic restart authority: disposition=%#v err=%v", disposition, resolveErr)
	}
}

func TestCommittedProductionFinalizationCarriesClosedLongitudinalSlotAuthorityV1(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	finalizer, _, _, _ := newTestCasePublicationFinalizer()
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: &caseTerminalStoreStub{}, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	available, err := result.UseCaseLongitudinalAcceptedSlotsV1(func(
		securityContext domainsecurity.TurnSecurityContext,
		acceptedFinalDigest string,
		dispositionDigest string,
		finalGateVersion string,
		slot domainevidence.AcceptedEntitySlotBindingV1,
	) error {
		t.Fatal("boundary-only final unexpectedly produced an eligible accepted slot")
		return nil
	})
	if err != nil || !available {
		t.Fatalf("committed production finalization omitted its closed longitudinal authority: available=%t err=%v", available, err)
	}
}

func TestFailedTrustedProjectionStageNeverExposesLongitudinalSlotAuthorityV1(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	registry := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}
	authority := newMemoryFinalAuthority(12)
	privateStore := &memoryPrivateFinalStore{
		records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
	}
	eventIO := newTestFinalPublicationEventIO()
	wrongProjectionAuthority := newMemoryFinalAuthority(13)
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(wrongProjectionAuthority, eventIO.Readback)
	finalizer := NewCasePublicationFinalizerWithPublicationSnapshots(
		registry, registry, authority, privateStore, eventIO,
		newTestTurnTerminalCoordinator(authority, privateStore), nil, index,
	)
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: &caseTerminalStoreStub{}, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err == nil || !result.Persistence.Changed {
		t.Fatalf("failed trusted projection stage did not preserve its quarantined CAS result: result=%#v err=%v", result, err)
	}
	callbackCalls := 0
	available, useErr := result.UseCaseLongitudinalAcceptedSlotsV1(func(
		domainsecurity.TurnSecurityContext,
		string,
		string,
		string,
		domainevidence.AcceptedEntitySlotBindingV1,
	) error {
		callbackCalls++
		return nil
	})
	if useErr != nil || available || callbackCalls != 0 {
		t.Fatalf("failed trusted projection stage exposed longitudinal slot authority: available=%t calls=%d err=%v", available, callbackCalls, useErr)
	}
}

func TestAcceptedFinalReadbackFailureNeverPublishesStagedEvents(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	registry := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}
	authority := newMemoryFinalAuthority(10)
	privateStore := &memoryPrivateFinalStore{
		records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
	}
	eventIO := newTestFinalPublicationEventIO()
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback)
	loadEvents, appendEvents := eventIO.LoadEvents, eventIO.AppendEvents
	appended := false
	eventIO.LoadEvents = func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, threadID string) ([]map[string]any, error) {
		if appended {
			return nil, errors.New("post-append readback failed")
		}
		return loadEvents(ctx, store, threadID)
	}
	eventIO.AppendEvents = func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, events []map[string]any) ([]map[string]any, error) {
		written, err := appendEvents(ctx, store, events)
		if err == nil {
			appended = true
		}
		return written, err
	}
	publishCalls := 0
	originalActivateAndPublish := eventIO.ActivateAndPublishEvents
	eventIO.ActivateAndPublishEvents = func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, events []map[string]any, seal domainevent.AcceptedFinalDeliverySealV1, activate func() error) error {
		publishCalls++
		return originalActivateAndPublish(ctx, store, events, seal, activate)
	}
	finalizer := NewCasePublicationFinalizerWithPublicationSnapshots(registry, registry, authority, privateStore, eventIO, newTestTurnTerminalCoordinator(authority, privateStore), nil, index)
	store := &caseTerminalStoreStub{}
	result, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	})
	if err == nil || !appended || publishCalls != 0 {
		t.Fatalf("post-append verification failure reached live publication: appended=%t publishCalls=%d err=%v", appended, publishCalls, err)
	}
	if _, ok := index.Resolve(input.Context.ThreadID, input.Context.TurnID); ok {
		t.Fatalf("post-append verification failure activated snapshot authority: %#v", result)
	}
}

func TestAcceptedFinalLivePublishRequiresActiveProjection(t *testing.T) {
	_, input := evidenceIssuerFixture(t)
	registry := &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}
	authority := newMemoryFinalAuthority(11)
	privateStore := &memoryPrivateFinalStore{
		records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{},
	}
	eventIO := newTestFinalPublicationEventIO()
	index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(authority, eventIO.Readback)
	publishCalls := 0
	originalActivateAndPublish := eventIO.ActivateAndPublishEvents
	eventIO.ActivateAndPublishEvents = func(ctx context.Context, store appturn.AcceptedFinalCompletionStore, events []map[string]any, seal domainevent.AcceptedFinalDeliverySealV1, activate func() error) error {
		publishCalls++
		return originalActivateAndPublish(ctx, store, events, seal, func() error {
			if err := activate(); err != nil {
				return err
			}
			if _, _, ok := index.ResolveForEvent(input.Context.ThreadID, input.Context.TurnID); !ok {
				return errors.New("live publish ran before projection activation")
			}
			return nil
		})
	}
	finalizer := NewCasePublicationFinalizerWithPublicationSnapshots(
		registry, registry, authority, privateStore, eventIO,
		newTestTurnTerminalCoordinator(authority, privateStore), nil, index,
	)
	store := &caseTerminalStoreStub{}
	if _, err := finalizer.PersistBoundary(context.Background(), PersistCaseBoundaryInput{
		Store: store, Context: input.Context, TerminalReason: TerminalSuccess,
		ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime(),
	}); err != nil {
		t.Fatal(err)
	}
	if publishCalls != 1 {
		t.Fatalf("live publish calls = %d, want 1", publishCalls)
	}
}
