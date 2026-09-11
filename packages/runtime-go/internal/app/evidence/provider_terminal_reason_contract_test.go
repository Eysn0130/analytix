package evidence

import (
	"testing"

	domaincachetelemetry "analytix.local/runtime-go/internal/domain/cachetelemetry"
)

func TestProviderTurnTerminalReasonsExactlyMatchFinalEvidenceGate(t *testing.T) {
	providerReasons := domaincachetelemetry.AllProviderTurnTerminalReasonsV1()
	gateReasons := AllTerminalReasons()
	if len(providerReasons) != len(gateReasons) {
		t.Fatalf("provider closure and final gate terminal taxonomies differ: provider=%d gate=%d", len(providerReasons), len(gateReasons))
	}
	providerSet := make(map[string]struct{}, len(providerReasons))
	for _, reason := range providerReasons {
		providerSet[string(reason)] = struct{}{}
	}
	for _, reason := range gateReasons {
		if _, exists := providerSet[string(reason)]; !exists {
			t.Fatalf("final gate terminal reason %q is not representable in the provider closure", reason)
		}
		delete(providerSet, string(reason))
	}
	if len(providerSet) != 0 {
		t.Fatalf("provider closure has terminal reasons unknown to the final gate: %#v", providerSet)
	}
}
