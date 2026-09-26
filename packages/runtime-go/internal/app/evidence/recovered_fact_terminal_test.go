package evidence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	gateprojection "analytix.local/runtime-go/internal/app/gateprojection"
	appturn "analytix.local/runtime-go/internal/app/turn"
	terminalapp "analytix.local/runtime-go/internal/app/turnterminal"
	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

func TestRecoveredFactTerminalUsesOriginalChainAndFreshWitness(t *testing.T) {
	for _, mode := range []string{"valid", "concurrent", "duplicate-member", "revoked", "case-changed", "missing-events", "tampered-events", "mixed-invalid", "revoked-before-activation", "revoked-before-publication", "cancelled-before-publication"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fixture := newConcreteRegistryFinalizerFixture(t)
			if _, err := fixture.persist(ctx); err != nil {
				t.Fatal(err)
			}
			owner := fixture.finalizer.(*casePublicationFinalizer)
			records, err := fixture.privateStore.List(ctx)
			if err != nil || len(records) != 1 {
				t.Fatal("missing real signed original")
			}
			snapshot := func() []byte {
				private, err := fixture.privateStore.List(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				body, err := json.Marshal([]any{private, fixture.store.status, fixture.store.items, fixture.store.fields, fixture.store.events, fixture.store.finishCalls})
				if err != nil {
					t.Fatal(err)
				}
				return body
			}
			before := snapshot()
			reader := acceptedFinalReaderForStore(fixture.securityContext, fixture.store)
			inventory, err := PreflightFinalAuthorityInventory(ctx, reader, reader, fixture.registry, fixture.coordinator.authority, fixture.privateStore)
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := owner.terminalCoordinator.RecoverV1(ctx, terminalapp.RestartRecoveryInputV1{CompletionStore: fixture.store, CASReader: reader, AuditOnlyPrivateInventory: inventory.AuditOnlyPublicWinners})
			if err != nil || len(recovery.FactCandidates) != 1 || len(recovery.Complete) != 0 {
				t.Fatalf("complete fact candidate inventory: %d %v", len(recovery.FactCandidates), err)
			}
			candidate := recovery.FactCandidates[0]
			complete := candidate.Terminal
			original := gateprojection.TerminalCompleteFinalAuthorityV1{PrivateFinal: candidate.PrivateFinal, Intent: complete.Intent, ProviderClosure: complete.ProviderClosure, PublicObservation: complete.PublicObservation, AcceptedFinalDisposition: complete.AcceptedFinalDisposition, TerminalDisposition: complete.TerminalDisposition}
			index := gateprojection.NewTrustedFinalProjectionIndexWithReadback(fixture.coordinator.authority, owner.eventIO.Readback)
			if index.SeedTerminalComplete(ctx, []gateprojection.TerminalCompleteFinalAuthorityV1{original}) == nil {
				t.Fatal("old seed guard was removed")
			}
			batch := []gateprojection.TerminalCompleteFinalAuthorityV1{original}
			if mode == "duplicate-member" {
				batch = append(batch, original)
			}
			if mode == "mixed-invalid" {
				invalid := original
				invalid.TerminalDisposition.DispositionID = "invalid"
				batch = append(batch, invalid)
			}
			if mode == "case-changed" {
				fixture.observer.observation.CaseID = "different-case"
			}
			var calls, acquisitions atomic.Int32
			var revoked atomic.Bool
			validate := func(context.Context, domainevidence.PrivateAcceptedFinalRecord) error {
				call := calls.Add(1)
				if revoked.Load() || mode == "revoked" || (mode == "revoked-before-activation" && call >= 5) {
					return errors.New("synthetic authority revoked")
				}
				return nil
			}
			load := func(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, cap appturn.FactFinalMutationAuthority) ([]map[string]any, error) {
				if mode == "missing-events" {
					return nil, errors.New("synthetic manifest missing")
				}
				events, err := LoadVerifiedAcceptedFinalEventsWithAuthority(ctx, owner.eventIO, fixture.store, record, cap)
				if mode == "tampered-events" && err == nil {
					body, _ := json.Marshal(events)
					var clone []map[string]any
					if json.Unmarshal(body, &clone) != nil || len(clone) == 0 {
						t.Fatal("missing original events")
					}
					clone[0]["type"] = "tampered"
					return clone, nil
				}
				return events, err
			}
			var witness registryport.RecoveredFactFinalWitnessIssuer = recoveryBoundaryWitness{
				inner: fixture.freshRegistryInstance(t), before: func() {
					if acquisitions.Add(1) == 2 {
						revoked.Store(mode == "revoked-before-publication")
						if mode == "cancelled-before-publication" {
							cancel()
						}
					}
				},
			}
			if mode == "concurrent" {
				results := make(chan error, 2)
				for i := 0; i < 2; i++ {
					go func() { results <- index.RestoreFactTerminalBatch(ctx, batch, witness, validate, load) }()
				}
				successes := 0
				for i := 0; i < 2; i++ {
					if <-results == nil {
						successes++
					}
				}
				if successes != 1 || len(index.RecordsForThread(fixture.securityContext.ThreadID)) != 1 {
					t.Fatal("concurrent startup did not admit exactly one original")
				}
				if !bytes.Equal(before, snapshot()) {
					t.Fatal("concurrent recovery mutated original")
				}
				return
			}
			err = index.RestoreFactTerminalBatch(ctx, batch, witness, validate, load)
			_, found := index.Resolve(fixture.securityContext.ThreadID, fixture.securityContext.TurnID)
			if mode == "valid" {
				if err != nil || !found {
					t.Fatalf("fresh recovery failed: %v", err)
				}
				if index.RestoreFactTerminalBatch(ctx, batch, witness, validate, load) == nil || len(index.RecordsForThread(fixture.securityContext.ThreadID)) != 1 {
					t.Fatal("duplicate startup replaced admitted history")
				}
				freshIndex := gateprojection.NewTrustedFinalProjectionIndexWithReadback(fixture.coordinator.authority, owner.eventIO.Readback)
				if err := freshIndex.RestoreFactTerminalBatch(ctx, batch, witness, validate, load); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || found {
				t.Fatalf("failed batch became public: %s", mode)
			}
			after := snapshot()
			if !bytes.Equal(before, after) {
				t.Fatal("recovery mutated durable original")
			}
		})
	}
}

type recoveryBoundaryWitness struct {
	inner  registryport.RecoveredFactFinalWitnessIssuer
	before func()
}

func (w recoveryBoundaryWitness) WithRecoveredFactFinalWitness(ctx context.Context, record domainevidence.PrivateAcceptedFinalRecord, use func(registryport.FactFinalWitnessCapability) error) error {
	w.before()
	return w.inner.WithRecoveredFactFinalWitness(ctx, record, use)
}
