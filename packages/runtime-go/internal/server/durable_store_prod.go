//go:build analytix_prod

package server

func validateDurableStoreRoot(_ string, _ durableStoreMode) error {
	return nil
}

func isCandidateDurableStoreMode(_ durableStoreMode) bool {
	return false
}
