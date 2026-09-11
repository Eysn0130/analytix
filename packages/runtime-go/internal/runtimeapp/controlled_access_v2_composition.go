package runtimeapp

import "errors"

type controlledAccessCompositionStateV2 struct {
	HasDurableRecords            bool
	HistoricalAuthorityAvailable bool
	LiveServiceReady             bool
}

// validateControlledAccessCompositionV2 prevents structural V2 records or a
// configured desktop transport from being mistaken for trusted historical or
// live release authority. Runtime activation remains possible for an empty V2
// store, but the capability stays disabled until the complete authority graph
// is composed.
func validateControlledAccessCompositionV2(state controlledAccessCompositionStateV2) error {
	if state.LiveServiceReady && !state.HistoricalAuthorityAvailable {
		return errors.New("controlled artifact V2 live service lacks historical authority")
	}
	if state.HasDurableRecords && !state.HistoricalAuthorityAvailable {
		return errors.New("controlled artifact V2 records require trusted historical authority")
	}
	return nil
}
