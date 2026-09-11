package evidence

import (
	"context"
	"errors"
	"testing"

	evidenceauthorityapp "analytix.local/runtime-go/internal/app/evidenceauthority"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	monotonicheadport "analytix.local/runtime-go/internal/ports/monotonichead"
)

type observeFailureLockedRegistryV1 struct {
	*lockedMemoryEvidenceRegistry
	err           error
	afterCallback bool
	cancel        context.CancelFunc
	calls         int
}

func (registry *observeFailureLockedRegistryV1) WithLockedSnapshot(ctx context.Context, frozen domainsecurity.TurnSecurityContext, use func(domainevidence.EvidenceReceiptRegistry) error) error {
	registry.calls++
	if registry.afterCallback {
		if err := registry.lockedMemoryEvidenceRegistry.WithLockedSnapshot(ctx, frozen, use); err != nil {
			return err
		}
	}
	if registry.cancel != nil {
		registry.cancel()
	}
	return registry.err
}

func TestObserveUnavailableBoundaryRequiresPreCallbackPureFailure(t *testing.T) {
	marked := errors.Join(evidenceauthorityapp.ErrWitnessObserveUnavailableV1, monotonicheadport.ErrUnavailable)
	for _, name := range []string{"pure-observe", "bare-unavailable", "mixed-physical", "after-callback", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			_, input := evidenceIssuerFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			registry := &observeFailureLockedRegistryV1{lockedMemoryEvidenceRegistry: &lockedMemoryEvidenceRegistry{memoryEvidenceRegistry: &memoryEvidenceRegistry{}}, err: marked}
			if name == "bare-unavailable" {
				registry.err = monotonicheadport.ErrUnavailable
			}
			if name == "mixed-physical" {
				registry.err = errors.Join(marked, errors.New("physical corruption"))
			}
			if name == "after-callback" {
				registry.afterCallback = true
			}
			if name == "cancelled" {
				registry.cancel = cancel
			}
			privateStore := &memoryPrivateFinalStore{records: map[string]domainevidence.PrivateAcceptedFinalRecord{}, dispositions: map[string]domainevidence.AcceptedFinalDispositionRecord{}}
			authority := newMemoryFinalAuthority(1)
			finalizer := NewCasePublicationFinalizerWithHostEvidenceAuthority(registry, registry, authority, privateStore, newTestFinalPublicationEventIO(), newTestTurnTerminalCoordinator(authority, privateStore), nil, nil)
			result, err := finalizer.PersistBoundary(ctx, PersistCaseBoundaryInput{Store: &caseTerminalStoreStub{}, Context: input.Context, TerminalReason: TerminalSuccess, ThreadID: input.Context.ThreadID, TurnID: input.Context.TurnID, AcceptedAt: evidenceIssuerTime()})
			records, listErr := privateStore.List(context.Background())
			if listErr != nil || registry.calls != 1 {
				t.Fatalf("snapshot was retried: %v", listErr)
			}
			if name == "pure-observe" {
				if err != nil || !result.Persistence.Changed || result.Boundary.Envelope.Blocker != caseEvidenceAuthorityUnavailableBlockerV1 || len(records) != 1 {
					t.Fatalf("fixed boundary was not committed: %v", err)
				}
				if records[0].RegistryHead.Sequence != 0 || records[0].AcceptedFinal.FactFinalWitnessAdmission != nil || records[0].PublicationSnapshotProof != nil || len(records[0].Envelope.Claims) != 0 || len(records[0].Envelope.EvidenceReceiptIDs) != 0 {
					t.Fatal("unavailable boundary acquired evidence")
				}
			} else {
				if err == nil {
					t.Fatal("nonavailability error became successful fallback")
				}
				if name == "after-callback" {
					if len(records) != 1 || records[0].Envelope.Blocker == caseEvidenceAuthorityUnavailableBlockerV1 {
						t.Fatal("callback outcome was reminted as unavailable")
					}
				} else if len(records) != 0 {
					t.Fatal("refusal wrote a private final")
				}
				if name == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation was lost")
				}
			}
		})
	}
}
