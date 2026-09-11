package runtimeapp

import "testing"

func TestControlledAccessV2RecordsWithoutHistoricalAuthorityFailStartup(t *testing.T) {
	for _, state := range []controlledAccessCompositionStateV2{
		{HasDurableRecords: true},
		{LiveServiceReady: true},
		{HasDurableRecords: true, LiveServiceReady: true},
	} {
		if err := validateControlledAccessCompositionV2(state); err == nil {
			t.Fatalf("unsafe controlled access V2 composition was accepted: %#v", state)
		}
	}
}

func TestEmptyControlledAccessV2InventoryDoesNotEnableService(t *testing.T) {
	if err := validateControlledAccessCompositionV2(controlledAccessCompositionStateV2{}); err != nil {
		t.Fatalf("empty disabled controlled access V2 composition was rejected: %v", err)
	}
	if err := validateControlledAccessCompositionV2(controlledAccessCompositionStateV2{
		HasDurableRecords: true, HistoricalAuthorityAvailable: true,
	}); err != nil {
		t.Fatalf("trusted historical-only controlled access V2 composition was rejected: %v", err)
	}
}
