package evidence

import (
	"context"
	"errors"

	domainevidence "analytix.local/runtime-go/internal/domain/evidence"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type snapshotRegistry struct {
	registry domainevidence.EvidenceReceiptRegistry
}

func newSnapshotRegistry(registry domainevidence.EvidenceReceiptRegistry) registryport.Registry {
	return snapshotRegistry{registry: registry}
}

func (snapshotRegistry) CommitPrepared(context.Context, registryport.CommitPreparedInput) (domainevidence.EvidenceReceipt, error) {
	return domainevidence.EvidenceReceipt{}, errors.New("evidence registry snapshot is read-only")
}

func (registry snapshotRegistry) Resolve(_ context.Context, query registryport.MembershipQuery) (domainevidence.RegisteredEvidence, error) {
	return domainevidence.VerifyEvidenceReceiptMembership(registry.registry, query.Context, query.ReceiptID)
}

func (snapshotRegistry) Revoke(context.Context, registryport.RevokeInput) error {
	return errors.New("evidence registry snapshot is read-only")
}

func (registry snapshotRegistry) Replay(_ context.Context, securityContext domainsecurity.TurnSecurityContext) (domainevidence.EvidenceReceiptRegistry, error) {
	if !domainevidence.EvidenceReceiptRegistryMatchesContext(registry.registry, securityContext) {
		return domainevidence.EvidenceReceiptRegistry{}, errors.New("evidence registry snapshot context is invalid")
	}
	return domainevidence.ParseEvidenceReceiptRegistry(domainevidence.EvidenceReceiptRegistryRecord(registry.registry))
}
