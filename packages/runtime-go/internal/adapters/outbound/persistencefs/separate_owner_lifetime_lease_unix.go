//go:build darwin || linux

package persistencefs

import (
	"context"
	"errors"
	"sync"
)

// SeparateOwnerLifetimeLease keeps one separately owned root pinned and
// exclusively locked without acquiring the Go DataDir or durable-root lease.
// The caller must supply an existing private root; this lease never creates it.
type SeparateOwnerLifetimeLease struct {
	mu      sync.Mutex
	cond    *sync.Cond
	active  int
	closing bool
	closed  bool
	root    *SeparateOwnerRootAuthority
	kernel  *kernelScopeLease
	scope   platformScopeLease
}

func AcquireSeparateOwnerLifetimeLease(root string) (*SeparateOwnerLifetimeLease, error) {
	if err := rejectSeparateOwnerTerminalReparse(root); err != nil {
		return nil, err
	}
	canonical, err := canonicalPathWithoutCreate(root)
	if err != nil {
		return nil, err
	}
	specs := compositeLeaseSpecsForPaths([]string{canonical})
	lease := &SeparateOwnerLifetimeLease{}
	lease.cond = sync.NewCond(&lease.mu)
	lease.kernel, err = acquireKernelScopeLeaseForSpecs(specs)
	if err != nil {
		return nil, err
	}
	lease.scope, err = acquirePlatformScopeLeaseForSpecs(specs)
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	lease.root, err = freezeSeparateOwnerRootAuthority(canonical)
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	if !lease.root.RootExists() || !lease.liveLocked() {
		return nil, errors.Join(errors.New("separate-owner lifetime root is unavailable"), lease.Close())
	}
	return lease, nil
}

func (lease *SeparateOwnerLifetimeLease) liveLocked() bool {
	return lease != nil && !lease.closing && !lease.closed && lease.root != nil &&
		lease.root.Validate() == nil && lease.kernel != nil && lease.kernel.Validate() == nil &&
		lease.scope != nil && lease.scope.Validate() == nil
}

func (lease *SeparateOwnerLifetimeLease) WithRootAccess(ctx context.Context, access func(*SeparateOwnerRootAccess) error) error {
	if lease == nil || access == nil {
		return errors.New("separate-owner lifetime access is unavailable")
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	lease.mu.Lock()
	if !lease.liveLocked() {
		lease.mu.Unlock()
		return errors.New("separate-owner lifetime lease is not live")
	}
	lease.active++
	authority := lease.root
	lease.mu.Unlock()
	defer func() {
		lease.mu.Lock()
		lease.active--
		lease.cond.Broadcast()
		lease.mu.Unlock()
	}()
	validate := func() error {
		lease.mu.Lock()
		defer lease.mu.Unlock()
		if lease.root != authority || !lease.liveLocked() {
			return errors.New("separate-owner lifetime authority changed")
		}
		return nil
	}
	opened, err := openSeparateOwnerRootAccess(authority, validate)
	if err != nil {
		return err
	}
	accessErr := access(opened)
	return errors.Join(accessErr, opened.Close(), validate())
}

func (lease *SeparateOwnerLifetimeLease) Close() error {
	if lease == nil {
		return nil
	}
	lease.mu.Lock()
	defer lease.mu.Unlock()
	if lease.closed {
		return nil
	}
	lease.closing = true
	for lease.active > 0 {
		lease.cond.Wait()
	}
	if lease.kernel != nil {
		if err := lease.kernel.Close(); err != nil {
			return err
		}
		lease.kernel = nil
	}
	var closeErr error
	if lease.scope != nil {
		closeErr = errors.Join(closeErr, lease.scope.Close())
		lease.scope = nil
	}
	if lease.root != nil {
		closeErr = errors.Join(closeErr, lease.root.close())
		lease.root = nil
	}
	lease.closed = true
	return closeErr
}
