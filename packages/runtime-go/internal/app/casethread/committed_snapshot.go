package casethread

import (
	"context"
	"errors"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// VerifiedCommittedContextInventoryV1 is an immutable historical observation.
// It exposes no Register, Commit, migration, execution, or current authority.
type VerifiedCommittedContextInventoryV1 struct {
	contexts map[string]domainsecurity.TurnSecurityContext
}

// VerifyContextRecordSignaturesV1 verifies every physical member without
// claiming its lineage graph is complete. An interrupted-transaction caller
// must separately validate the original and complete candidate inventories.
func VerifyContextRecordSignaturesV1(ctx context.Context, records []domainsecurity.CaseThreadAuthorityRecord, authority authorityport.Authority) error {
	if ctx == nil || authority == nil {
		return errors.New("case context signature observation is unavailable")
	}
	registry := newRegistry(authority, nil)
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := registry.verifyTrusted(ctx, record); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func VerifyCommittedContextInventoryV1(ctx context.Context, records []domainsecurity.CaseThreadAuthorityRecord, authority authorityport.Authority) (*VerifiedCommittedContextInventoryV1, error) {
	if ctx == nil || authority == nil {
		return nil, errors.New("committed case context observation is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := domainsecurity.ValidateCaseThreadAuthorityInventoryV1(records); err != nil {
		return nil, err
	}
	// Use the full existing current-key/conflict owner. This registry remains
	// local to validation and is never returned as an execution capability.
	registry := newRegistry(authority, nil)
	if err := registry.addInventory(ctx, records); err != nil {
		return nil, err
	}
	result := &VerifiedCommittedContextInventoryV1{contexts: map[string]domainsecurity.TurnSecurityContext{}}
	for key, record := range registry.committed {
		if record.SecurityContext == nil || record.CommittedTurnState == nil {
			return nil, errors.New("committed case context record is incomplete")
		}
		result.contexts[key] = *record.SecurityContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (inventory *VerifiedCommittedContextInventoryV1) CommittedContextV1(threadID, turnID string) (domainsecurity.TurnSecurityContext, bool) {
	if inventory == nil {
		return domainsecurity.TurnSecurityContext{}, false
	}
	frozen, found := inventory.contexts[committedTurnKey(threadID, turnID)]
	return frozen, found
}
