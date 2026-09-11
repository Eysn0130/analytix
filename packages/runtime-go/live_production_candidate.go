//go:build !analytix_prod

package runtimego

import livelocal "analytix.local/runtime-go/internal/conformance/livelocal"

// Compatibility shim: production-candidate contract routes live in
// internal/conformance/livelocal. Root wrappers keep the previous package-local test API.
const liveProductionCandidatePrefix = livelocal.LiveProductionCandidatePrefix

type liveProductionCandidateHandler = livelocal.LiveProductionCandidateHandler

func newLiveProductionCandidateHandler(runtimeToken string, store *tempDurableEventSessionStore) *liveProductionCandidateHandler {
	return livelocal.NewLiveProductionCandidateHandler(runtimeToken, store)
}
