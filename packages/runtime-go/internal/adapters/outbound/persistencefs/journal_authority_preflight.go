package persistencefs

import (
	"context"
	"errors"
	"sync"
)

// JournalAuthorityPreflightOwnerV1 exposes only a host-derived digest and an
// exact revalidation operation. Implementations must be read-only and cover a
// complete security-relevant owner inventory.
type JournalAuthorityPreflightOwnerV1 interface {
	ObserveJournalAuthorityPreflightV1(context.Context) (string, error)
	ValidateJournalAuthorityPreflightV1(context.Context, string) error
}

// PreparedJournalAuthorityBootstrapV1 records the trusted host composition's
// complete owner set and its revalidated read-only fixed point before authority
// namespace bootstrap. Architecture tests restrict its production call sites.
type PreparedJournalAuthorityBootstrapV1 struct {
	mu           sync.Mutex
	lease        *CompositeLease
	owners       []JournalAuthorityPreflightOwnerV1
	observations []string
	created      bool
}

func PrepareJournalAuthorityBootstrapV1(
	ctx context.Context,
	lease *CompositeLease,
	owners ...JournalAuthorityPreflightOwnerV1,
) (*PreparedJournalAuthorityBootstrapV1, error) {
	if lease == nil || len(owners) == 0 {
		return nil, errors.New("journal authority preflight owners are unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	observations := make([]string, 0, len(owners))
	for _, owner := range owners {
		if owner == nil {
			return nil, errors.New("journal authority preflight owner is unavailable")
		}
		digest, err := owner.ObserveJournalAuthorityPreflightV1(ctx)
		if err != nil || !separateOwnerIsSHA256(digest) {
			return nil, errors.New("journal authority preflight observation failed")
		}
		observations = append(observations, digest)
	}
	prepared := &PreparedJournalAuthorityBootstrapV1{
		lease: lease, owners: append([]JournalAuthorityPreflightOwnerV1(nil), owners...),
		observations: observations,
	}
	if err := prepared.validate(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

// BindExistingV1 freezes an existing namespace without creating it. It is
// safe during phase-zero discovery and retains the same owner fixed point.
func (prepared *PreparedJournalAuthorityBootstrapV1) BindExistingV1(
	ctx context.Context,
) (*JournalNamespaceAuthority, bool, error) {
	if prepared == nil {
		return nil, false, errors.New("journal authority preflight is unavailable")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if err := prepared.validate(ctx); err != nil {
		return nil, false, err
	}
	authority, present, err := prepared.lease.openExistingJournalAuthorityCandidateV1()
	if err != nil {
		return nil, false, err
	}
	if !present {
		if err := prepared.validate(ctx); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	}
	committed, err := prepared.lease.commitJournalAuthorityCandidateV1(authority, func() error {
		return prepared.validateOwners(ctx)
	})
	if err != nil {
		return nil, false, err
	}
	return committed, true, nil
}

// CreateV1 is the only production bootstrap path exposed by CompositeLease.
// It revalidates the complete owner set immediately before and after the
// explicit namespace creation boundary.
func (prepared *PreparedJournalAuthorityBootstrapV1) CreateV1(
	ctx context.Context,
) (*JournalNamespaceAuthority, error) {
	if prepared == nil {
		return nil, errors.New("journal authority preflight is unavailable")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.created {
		return nil, errors.New("journal authority bootstrap was already consumed")
	}
	if err := prepared.validate(ctx); err != nil {
		return nil, err
	}
	authority, err := prepared.lease.createJournalAuthorityCandidateAfterPreflightV1()
	if err != nil {
		return nil, err
	}
	prepared.created = true
	committed, err := prepared.lease.commitJournalAuthorityCandidateV1(authority, func() error {
		return prepared.validateOwners(ctx)
	})
	if err != nil {
		return nil, err
	}
	return committed, nil
}

func (prepared *PreparedJournalAuthorityBootstrapV1) validate(ctx context.Context) error {
	if prepared == nil || prepared.lease == nil || len(prepared.owners) == 0 ||
		len(prepared.owners) != len(prepared.observations) {
		return errors.New("journal authority preflight is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if _, held := prepared.lease.FrozenRoots(); !held {
		return errors.New("journal authority preflight lease changed")
	}
	return prepared.validateOwners(ctx)
}

func (prepared *PreparedJournalAuthorityBootstrapV1) validateOwners(ctx context.Context) error {
	if prepared == nil || len(prepared.owners) == 0 || len(prepared.owners) != len(prepared.observations) {
		return errors.New("journal authority preflight is invalid")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	for index, owner := range prepared.owners {
		if owner == nil || !separateOwnerIsSHA256(prepared.observations[index]) ||
			owner.ValidateJournalAuthorityPreflightV1(ctx, prepared.observations[index]) != nil {
			return errors.New("journal authority preflight owner changed")
		}
	}
	return nil
}
