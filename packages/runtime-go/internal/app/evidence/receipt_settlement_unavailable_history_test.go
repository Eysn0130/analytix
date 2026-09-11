package evidence

import (
	"context"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type unavailableOriginalSettlementObserverV1 struct {
	*originalSettlementHistoryObserverV2
}

func (o unavailableOriginalSettlementObserverV1) ObserveRestartEvidenceRegistryObservationV1(context.Context, []domainsecurity.TurnSecurityContext) (registryport.OriginalObservationV1, error) {
	return registryport.OriginalObservationV1{Unavailable: o.input.RegistryUnavailable}, nil
}

func TestUnavailableOriginalSettlementRegistryAuditsMarkersWithoutRepair(t *testing.T) {
	for _, scenario := range []string{"nonheld", "held", "missing-grant", "bad-result", "missing-prepared", "live-authority", "digest-drift"} {
		t.Run(scenario, func(t *testing.T) {
			issuer, input, reader, source := originalSettlementHistoryFixtureV2(t)
			source.input.RegistryHistoryV2 = nil
			if scenario == "held" {
				source.input = originalSettlementObserverForTestV1(t, issuer, input, reader).input
				reader = acceptedFinalPublicReaderStub{threads: map[string]map[string]any{}}
			}
			source.input.RegistryUnavailable = &registryport.UnavailableOriginalInventoryV1{InventoryDigest: domainsecurity.SHA256Hex([]byte("complete frozen raw owner"))}
			observer := unavailableOriginalSettlementObserverV1{source}
			if scenario != "live-authority" {
				issuer.Registry = &unavailableSettlementRegistryV1{memoryEvidenceRegistry: source.registry}
			}
			if scenario == "missing-grant" || scenario == "bad-result" {
				turn := reader.threads[input.Context.ThreadID]["turns"].([]any)[0].(map[string]any)
				items := turn["items"].([]any)
				if scenario == "missing-grant" {
					turn["items"] = items[1:]
				} else {
					delete(items[1].(map[string]any), "finishedAt")
				}
			}
			if scenario == "missing-prepared" {
				for id := range issuer.SettlementStore.(*memoryEvidenceSettlementStore).records {
					delete(issuer.SettlementStore.(*memoryEvidenceSettlementStore).records, id)
				}
			}
			inventory, err := PreflightEvidenceSettlementInventoryWithPreservationV1(context.Background(), reader, issuer, observer)
			if scenario == "missing-grant" || scenario == "bad-result" || scenario == "missing-prepared" || scenario == "live-authority" {
				if err == nil || source.registry.commitCalls != 0 {
					t.Fatalf("unavailable registry bypassed original marker/authority audit: %v", err)
				}
				return
			}
			if err != nil || len(inventory.Pending)+len(inventory.Committed)+len(inventory.Abandoned) != 0 || len(inventory.Preserved)+len(inventory.Quarantined) != 1 {
				t.Fatalf("unavailable registry acquired repair classification: %v", err)
			}
			if scenario == "held" && len(inventory.Preserved) != 1 || scenario != "held" && len(inventory.Quarantined) != 1 {
				t.Fatal("unavailable registry changed original hold scope")
			}
			if scenario == "digest-drift" {
				source.input.RegistryUnavailable.InventoryDigest = domainsecurity.SHA256Hex([]byte("different raw owner"))
			}
			err = ApplyEvidenceSettlementReconciliationInventoryWithPreservationV1(context.Background(), reader, issuer, inventory, observer)
			if scenario == "digest-drift" && err == nil || scenario != "digest-drift" && err != nil || source.registry.commitCalls != 0 {
				t.Fatalf("unavailable raw owner drift/apply: commits=%d err=%v", source.registry.commitCalls, err)
			}
		})
	}
}
