package evidence

import (
	"context"
	"errors"
	"testing"
	"time"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type originalSettlementHistoryObserverV2 struct {
	input    PreservedEvidenceSettlementInventoryV1
	registry *memoryEvidenceRegistry
}

func (observer *originalSettlementHistoryObserverV2) ObserveOriginalEvidenceSettlementInventoryV1(context.Context) (PreservedEvidenceSettlementInventoryV1, error) {
	// Deliberately share the source maps: preflight must freeze its own copy.
	return observer.input, nil
}

func (observer *originalSettlementHistoryObserverV2) ObserveRestartEvidenceRegistryInventoryV2(ctx context.Context, contexts []domainsecurity.TurnSecurityContext) ([]registryport.InventoryRecord, *registryport.OriginalHistoryV2, error) {
	records, err := observer.registry.ListRegistries(ctx, contexts)
	return records, observer.input.RegistryHistoryV2, err
}

func originalSettlementHistoryFixtureV2(t *testing.T) (Issuer, IssueEvidenceInput, acceptedFinalPublicReaderStub, *originalSettlementHistoryObserverV2) {
	t.Helper()
	issuer, input := sourceBoundV2IssueFixture(t, "entity-a", "entity-a")
	prepared := prepareSignedSettlementWithoutIssuerSourceChecks(t, issuer, input)
	if err := issuer.SettlementStore.PutPreparedIfAbsent(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := domainevidence.NewEvidenceReceiptRegistry(input.Context)
	if err != nil {
		t.Fatal(err)
	}
	registry, _, err = domainevidence.RegisterEvidenceReceipt(registry, prepared.ReceiptDraft, prepared.CanonicalEvidence,
		domainevidence.EvidenceSettlementProof{SettlementID: prepared.SettlementID, PreparedRecordDigest: prepared.RecordDigest}, evidenceIssuerTime().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	history := &registryport.OriginalHistoryV2{
		Indexes:  map[string]domainevidence.EvidenceRegistryAuthorityIndexV2{},
		Capsules: map[string]domainevidence.EvidenceRegistryAuthorityCapsule{},
	}
	sign := func(body []byte) ([]byte, error) { return issuer.Authority.Sign(context.Background(), body) }
	for i := 0; i < 2; i++ {
		if i == 1 {
			registry, err = domainevidence.RevokeEvidenceReceipt(registry, prepared.ReceiptID, "source_changed", evidenceIssuerTime().Add(2*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
		}
		capsule, err := domainevidence.NewEvidenceRegistryAuthorityCapsule(input.Context, registry, issuer.Authority.KeyID(), issuer.Authority.PublicKey(), sign)
		if err != nil {
			t.Fatal(err)
		}
		history.Capsules[capsule.RecordDigest] = capsule
	}
	return issuer, input, settlementReader(input, marker), &originalSettlementHistoryObserverV2{
		input: PreservedEvidenceSettlementInventoryV1{RegistryHistoryV2: history}, registry: issuer.Registry.(*memoryEvidenceRegistry),
	}
}

func TestOriginalSettlementHistoryV2SharedPrefixDoesNotRepair(t *testing.T) {
	issuer, _, reader, observer := originalSettlementHistoryFixtureV2(t)
	inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), reader, issuer, observer)
	if err != nil || len(inventory.Pending) != 0 || len(inventory.Quarantined) != 1 || len(inventory.Committed) != 0 || len(inventory.Preserved) != 0 {
		t.Fatalf("complete unselected history acquired current classification: err=%v pending=%d quarantined=%d", err, len(inventory.Pending), len(inventory.Quarantined))
	}
	if err := ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(context.Background(), reader, issuer, inventory, observer); err != nil || observer.registry.commitCalls != 0 {
		t.Fatalf("history apply mutated current authority: commits=%d err=%v", observer.registry.commitCalls, err)
	}
}

func TestOriginalSettlementHistoryV2RequiresCompleteNonHeldSupport(t *testing.T) {
	for _, missing := range []string{"grant", "marker", "prepared"} {
		t.Run(missing, func(t *testing.T) {
			issuer, input, reader, observer := originalSettlementHistoryFixtureV2(t)
			turn := reader.threads[input.Context.ThreadID]["turns"].([]any)[0].(map[string]any)
			items := turn["items"].([]any)
			switch missing {
			case "grant":
				turn["items"] = items[1:]
			case "marker":
				delete(items[1].(map[string]any), "hostEvidenceSettlement")
			case "prepared":
				issuer.SettlementStore.(*memoryEvidenceSettlementStore).records = map[string]domainevidence.PreparedEvidenceSettlement{}
			}
			if _, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), reader, issuer, observer); err == nil || observer.registry.commitCalls != 0 {
				t.Fatalf("incomplete non-held history accepted: missing=%s err=%v", missing, err)
			}
		})
	}
}

func addIndependentSettlementToHistoryFixtureV2(t *testing.T, issuer Issuer, source IssueEvidenceInput, reader acceptedFinalPublicReaderStub) {
	t.Helper()
	frozen := newEvidenceCaseContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-independent", TurnID: "turn-independent", WorkspaceRealPath: source.Context.WorkspaceRealPath,
		CaseID: source.Context.CaseID, CaseBindingHash: source.Context.CaseBindingHash, DatasetSnapshotID: source.Context.DatasetSnapshotID,
		SourceManifestHash: source.Context.SourceManifestHash, ContextEpoch: source.Context.ContextEpoch, IssuedAt: evidenceIssuerTime(),
	})
	_, input := evidenceIssuerFixtureForContext(t, evidenceIssuerTime(), frozen)
	input.RawResult, input.Material = source.RawResult, source.Material
	prepared := prepareSignedSettlementWithoutIssuerSourceChecks(t, issuer, input)
	// A bare DSV2 probe is readable history, never a startup repair lease.
	// The independent record must retain the same classification without V2
	// original history; live issuance needs the separate Host capability.
	if err := domainevidence.ValidatePreparedEvidenceSettlementForExecution(prepared); err == nil {
		t.Fatal("independent bare probe unexpectedly acquired startup execution authority")
	}
	if err := issuer.SettlementStore.PutPreparedIfAbsent(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	marker, err := domainevidence.NewHostEvidenceSettlementMarker(prepared)
	if err != nil {
		t.Fatal(err)
	}
	reader.threads[frozen.ThreadID] = settlementReader(input, marker).threads[frozen.ThreadID]
}

func TestOriginalSettlementHistoryV2PreservesIndependentClassificationAndRejectsDrift(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(map[bool]string{false: "independent classification", true: "history drift"}[drift], func(t *testing.T) {
			issuer, input, reader, observer := originalSettlementHistoryFixtureV2(t)
			addIndependentSettlementToHistoryFixtureV2(t, issuer, input, reader)
			inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), reader, issuer, observer)
			if err != nil || len(inventory.Pending) != 0 || len(inventory.Quarantined) != 2 || len(inventory.Preserved) != 0 {
				t.Fatalf("mixed preflight: pending=%d quarantined=%d err=%v", len(inventory.Pending), len(inventory.Quarantined), err)
			}
			independentReader := acceptedFinalPublicReaderStub{threads: map[string]map[string]any{"thread-independent": reader.threads["thread-independent"]}}
			independentIssuer := issuer
			independentIssuer.SettlementStore = &memoryEvidenceSettlementStore{records: map[string]domainevidence.PreparedEvidenceSettlement{}}
			for _, prepared := range inventory.Quarantined {
				if prepared.SecurityContext.ThreadID == "thread-independent" {
					if err := independentIssuer.SettlementStore.PutPreparedIfAbsent(context.Background(), prepared); err != nil {
						t.Fatal(err)
					}
				}
			}
			baseline, err := PreflightEvidenceSettlementInventory(context.Background(), independentReader, independentIssuer)
			if err != nil || len(baseline.Quarantined) != 1 || len(baseline.Pending) != 0 || len(baseline.Preserved) != 0 {
				t.Fatalf("independent baseline classification drifted: %v", err)
			}
			if drift {
				for digest := range observer.input.RegistryHistoryV2.Capsules {
					delete(observer.input.RegistryHistoryV2.Capsules, digest)
					break
				}
			}
			err = ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(context.Background(), reader, issuer, inventory, observer)
			if drift {
				if err == nil || observer.registry.commitCalls != 0 {
					t.Fatalf("mutable source changed frozen history before independent commit: commits=%d err=%v", observer.registry.commitCalls, err)
				}
			} else if err != nil || observer.registry.commitCalls != 0 {
				t.Fatalf("independent audit classification acquired repair: commits=%d err=%v", observer.registry.commitCalls, err)
			}
		})
	}
}

func TestOriginalSettlementHistoryV2SourceRequiresPreservation(t *testing.T) {
	issuer, _, reader, observer := originalSettlementHistoryFixtureV2(t)
	issuer.Registry = &unavailableSettlementRegistryV1{memoryEvidenceRegistry: observer.registry}
	if _, err := preflightEvidenceSettlementInventoryV1(context.Background(), reader, issuer, nil, observer); err == nil {
		t.Fatal("unavailable registry swallowed original history without preservation")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("history cancelled")
	cancel(cause)
	if _, err := PreflightEvidenceSettlementInventoryWithPreservationV1(ctx, reader, issuer, observer); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled history preflight continued: %v", err)
	}
}
