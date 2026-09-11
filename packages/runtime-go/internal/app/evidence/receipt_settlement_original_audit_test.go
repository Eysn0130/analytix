package evidence

import (
	"context"
	"errors"
	"reflect"
	"testing"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

func TestOriginalSettlementAuditIncludesMarkerBeforeCapsule(t *testing.T) {
	for _, cut := range []string{"prepared-only", "marker-before-capsule", "all-capsules"} {
		t.Run(cut, func(t *testing.T) {
			issuer, input, reader, observer := originalSettlementHistoryFixtureV2(t)
			prepared, err := issuer.SettlementStore.ListPrepared(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			history := observer.input.RegistryHistoryV2
			if cut != "all-capsules" {
				history = &registryport.OriginalHistoryV2{
					Indexes:  map[string]domainevidence.EvidenceRegistryAuthorityIndexV2{},
					Capsules: map[string]domainevidence.EvidenceRegistryAuthorityCapsule{},
				}
			}
			if cut == "prepared-only" {
				reader = settlementReaderWithoutResult(input)
			}
			if err := ValidateOriginalEvidenceSettlementInventoryV1(context.Background(), reader, issuer.Authority, prepared, history); err != nil {
				t.Fatal(err)
			}
			inventory, err := inspectEvidenceSettlementInventoryV1(context.Background(), reader, Issuer{Authority: issuer.Authority}, nil, nil,
				&originalEvidenceSettlementAuditV1{prepared: prepared, history: history})
			if err != nil || !reflect.DeepEqual(inventory, EvidenceSettlementInventory{}) || observer.registry.commitCalls != 0 {
				t.Fatalf("historical audit returned authority: err=%v", err)
			}
		})
	}
}

func TestOriginalSettlementAuditRejectsIncompleteSupport(t *testing.T) {
	for _, fault := range []string{"grant-before-capsule", "result-before-capsule", "missing-marker", "missing-prepared", "duplicate-prepared", "signature", "missing-primary", "nil-prepared", "nil-history", "nil-indexes", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			issuer, input, reader, observer := originalSettlementHistoryFixtureV2(t)
			prepared, err := issuer.SettlementStore.ListPrepared(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			history := observer.input.RegistryHistoryV2
			turn := reader.threads[input.Context.ThreadID]["turns"].([]any)[0].(map[string]any)
			items := turn["items"].([]any)
			ctx := context.Background()
			switch fault {
			case "grant-before-capsule", "result-before-capsule":
				history.Capsules = map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}
				if fault == "grant-before-capsule" {
					turn["items"] = items[1:]
				} else {
					items[1].(map[string]any)["callId"] = "foreign-call"
				}
			case "missing-marker":
				delete(items[1].(map[string]any), "hostEvidenceSettlement")
			case "missing-prepared":
				prepared = []domainevidence.PreparedEvidenceSettlement{}
			case "duplicate-prepared":
				prepared = append(prepared, prepared[0])
			case "signature":
				prepared[0].AuthoritySignature = "invalid"
			case "missing-primary":
				delete(reader.threads, input.Context.ThreadID)
				history.Capsules = map[string]domainevidence.EvidenceRegistryAuthorityCapsule{}
			case "nil-prepared":
				prepared = nil
			case "nil-history":
				history = nil
			case "nil-indexes":
				history.Indexes = nil
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			err = ValidateOriginalEvidenceSettlementInventoryV1(ctx, reader, issuer.Authority, prepared, history)
			if err == nil || fault == "cancelled" && !errors.Is(err, context.Canceled) || observer.registry.commitCalls != 0 {
				t.Fatalf("incomplete Original audit accepted: err=%v", err)
			}
		})
	}
}
