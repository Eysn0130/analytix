//go:build !analytix_prod

package runtimego

import server "analytix.local/runtime-go/internal/server"

// Compatibility shim: durable event/session store behavior lives in
// internal/server. Root wrappers keep old tests package-local.
type durableJSONLDiagnostic = server.DurableJSONLDiagnostic
type durableLoadEventsResult = server.DurableLoadEventsResult
type tempDurableEventSessionStore = server.DurableEventSessionStore

func newTempDurableEventSessionStore(root string) (*tempDurableEventSessionStore, error) {
	return server.NewTempDurableEventSessionStore(root)
}

func newCandidateDurableEventSessionStore(root string) (*tempDurableEventSessionStore, error) {
	return server.NewCandidateDurableEventSessionStore(root)
}
