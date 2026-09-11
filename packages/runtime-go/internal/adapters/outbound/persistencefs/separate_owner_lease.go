package persistencefs

import (
	"context"
	"errors"
)

func validateSeparateOwnerDisjointFromManaged(
	managed *RootAuthority,
	candidate *SeparateOwnerRootAuthority,
	existing map[string]*SeparateOwnerRootAuthority,
) error {
	if managed == nil || candidate == nil {
		return errors.New("separate-owner disjoint authority is unavailable")
	}
	candidateRoot, candidatePresent := candidate.rootBinding()
	if !candidatePresent {
		return nil
	}
	managed.mu.Lock()
	for _, capability := range managed.capabilities {
		if capability.RootIdentity != "" && capability.RootIdentity == candidateRoot.Identity {
			managed.mu.Unlock()
			return errors.New("separate-owner root aliases a managed persistence root")
		}
	}
	managed.mu.Unlock()
	for _, authority := range existing {
		root, present := authority.rootBinding()
		if present && root.Identity == candidateRoot.Identity {
			return errors.New("separate-owner roots alias the same filesystem object")
		}
	}
	return nil
}

func (lease *CompositeLease) validateSeparateOwnersLocked() error {
	if lease == nil || lease.separateOwners == nil || !separateOwnerIsSHA256(lease.separateDigest) ||
		separateOwnerAuthoritiesDigest(lease.separateOwners) != lease.separateDigest {
		return errors.New("separate-owner lease authority is invalid")
	}
	for _, authority := range lease.separateOwners {
		if err := authority.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (lease *CompositeLease) FrozenSeparateOwnerAuthority(root string) (*SeparateOwnerRootAuthority, bool) {
	if lease == nil {
		return nil, false
	}
	canonical, err := canonicalPathWithoutCreate(root)
	if err != nil {
		return nil, false
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if !lease.liveLocked() {
		return nil, false
	}
	authority, ok := lease.separateOwners[canonicalPathKey(canonical)]
	return authority, ok
}

func (lease *CompositeLease) WithSeparateOwnerRootAccess(
	ctx context.Context,
	authority *SeparateOwnerRootAuthority,
	access func(*SeparateOwnerRootAccess) error,
) error {
	if lease == nil || authority == nil || access == nil {
		return errors.New("separate-owner access lease is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	lease.mu.Lock()
	lease.ensureAccessCondLocked()
	registered, ok := lease.separateOwners[canonicalPathKey(authority.Root())]
	if !ok || registered != authority || !lease.liveLocked() {
		lease.mu.Unlock()
		return errors.New("separate-owner access is outside a live persistence lease")
	}
	lease.activeAccesses++
	lease.mu.Unlock()
	defer func() {
		lease.mu.Lock()
		lease.activeAccesses--
		lease.accessCond.Broadcast()
		lease.mu.Unlock()
	}()
	validate := func() error {
		lease.mu.Lock()
		defer lease.mu.Unlock()
		registered, ok := lease.separateOwners[canonicalPathKey(authority.Root())]
		if !ok || registered != authority || !lease.liveLocked() {
			return errors.New("separate-owner lease or authority is no longer live")
		}
		return nil
	}
	rootAccess, err := openSeparateOwnerRootAccess(authority, validate)
	if err != nil {
		return err
	}
	accessErr := access(rootAccess)
	closeErr := rootAccess.Close()
	validateErr := validate()
	return errors.Join(accessErr, closeErr, validateErr)
}

func openSeparateOwnerRootAccess(
	authority *SeparateOwnerRootAuthority,
	validate func() error,
) (*SeparateOwnerRootAccess, error) {
	if authority == nil || validate == nil || validate() != nil || authority.Validate() != nil {
		return nil, errors.New("separate-owner root access authority is invalid")
	}
	if !authority.RootExists() {
		return &SeparateOwnerRootAccess{authority: authority}, nil
	}
	directory, state, err := platformOpenSeparateOwnerRootDirectory(authority, validate)
	if err != nil {
		return nil, err
	}
	return &SeparateOwnerRootAccess{
		authority: authority,
		root: &SeparateOwnerDirectory{
			directory: directory, state: state, requireProtected: false, validateLease: validate,
		},
		present: true,
	}, nil
}
