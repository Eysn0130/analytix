//go:build !analytix_prod

package server

import (
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	"fmt"
	"path/filepath"
	"strings"
)

const durableStoreModeCandidate durableStoreMode = "candidate-durable-event-session-store"

func NewCandidateDurableEventSessionStore(root string, floors ...domainpendingwork.ChildIdentityFloorsV1) (*DurableEventSessionStore, error) {
	return newDurableEventSessionStore(root, durableStoreModeCandidate, false, floors...)
}

func validateDurableStoreRoot(path string, mode durableStoreMode) error {
	if mode == durableStoreModeCandidate && !isCandidateDurableRoot(path) {
		return fmt.Errorf("candidate durable root must contain analytix-go-runtime-candidate marker: %s", path)
	}
	return nil
}

func isCandidateDurableStoreMode(mode durableStoreMode) bool {
	return mode == durableStoreModeCandidate
}

func isCandidateDurableRoot(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "analytix-go-runtime-candidate")
}
