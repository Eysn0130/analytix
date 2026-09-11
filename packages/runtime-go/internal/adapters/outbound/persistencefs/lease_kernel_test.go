package persistencefs

import (
	"errors"
	"testing"
)

type failingKernelArbiter struct {
	failOffset int64
	fail       bool
	releases   map[int64]int
	closed     int
}

func (arbiter *failingKernelArbiter) Acquire([]kernelRangeSpec) error { return nil }

func (arbiter *failingKernelArbiter) Release(spec kernelRangeSpec) error {
	if arbiter.releases == nil {
		arbiter.releases = map[int64]int{}
	}
	arbiter.releases[spec.offset]++
	if arbiter.fail && spec.offset == arbiter.failOffset {
		return errors.New("injected kernel range release failure")
	}
	return nil
}

func (arbiter *failingKernelArbiter) Validate() error { return nil }
func (arbiter *failingKernelArbiter) Close() error {
	arbiter.closed++
	return nil
}

type countingScopeLease struct{ closed int }

func (*countingScopeLease) Validate() error { return nil }
func (lease *countingScopeLease) Close() error {
	lease.closed++
	return nil
}

func TestKernelScopeCloseFailureRetainsExactReleaseProgress(t *testing.T) {
	shared := kernelRangeSpec{offset: 11}
	unique := kernelRangeSpec{offset: 29, exclusive: true}
	arbiter := &failingKernelArbiter{failOffset: unique.offset, fail: true}
	persistenceKernelScopes.Lock()
	if len(persistenceKernelScopes.leases) != 0 || len(persistenceKernelScopes.held) != 0 || persistenceKernelScopes.arbiter != nil {
		persistenceKernelScopes.Unlock()
		t.Fatal("kernel scope manager was not idle before failure test")
	}
	persistenceKernelScopes.arbiter = arbiter
	persistenceKernelScopes.leases[1] = kernelScopeRecord{ranges: []kernelRangeSpec{shared, unique}}
	persistenceKernelScopes.leases[2] = kernelScopeRecord{ranges: []kernelRangeSpec{shared}}
	persistenceKernelScopes.held[shared.offset] = kernelRangeHold{refs: 2}
	persistenceKernelScopes.held[unique.offset] = kernelRangeHold{refs: 1, exclusive: true}
	persistenceKernelScopes.Unlock()
	t.Cleanup(func() {
		persistenceKernelScopes.Lock()
		persistenceKernelScopes.arbiter = nil
		persistenceKernelScopes.leases = map[uint64]kernelScopeRecord{}
		persistenceKernelScopes.held = map[int64]kernelRangeHold{}
		persistenceKernelScopes.Unlock()
	})

	scope := &countingScopeLease{}
	composite := &CompositeLease{kernel: &kernelScopeLease{id: 1}, scope: scope}
	if err := composite.Close(); err == nil {
		t.Fatal("injected kernel release failure was ignored")
	}
	if composite.closed || composite.kernel == nil || scope.closed != 0 {
		t.Fatal("failed kernel release disposed secondary authority or prevented retry")
	}
	persistenceKernelScopes.Lock()
	record := persistenceKernelScopes.leases[1]
	sharedHold := persistenceKernelScopes.held[shared.offset]
	_, uniqueHeld := persistenceKernelScopes.held[unique.offset]
	persistenceKernelScopes.Unlock()
	if len(record.ranges) != 1 || record.ranges[0].offset != unique.offset || sharedHold.refs != 1 || !uniqueHeld {
		t.Fatalf("failed close lost exact release progress: record=%#v shared=%#v uniqueHeld=%v", record, sharedHold, uniqueHeld)
	}

	arbiter.fail = false
	if err := composite.Close(); err != nil {
		t.Fatalf("retry close: %v", err)
	}
	if !composite.closed || composite.kernel != nil || scope.closed != 1 {
		t.Fatal("successful retry did not finish composite disposal exactly once")
	}
	if arbiter.releases[shared.offset] != 0 || arbiter.releases[unique.offset] != 2 {
		t.Fatalf("retry repeated a released shared range: %#v", arbiter.releases)
	}
	second := &kernelScopeLease{id: 2}
	if err := second.Close(); err != nil {
		t.Fatalf("release remaining shared owner: %v", err)
	}
	if arbiter.releases[shared.offset] != 1 || arbiter.closed != 1 {
		t.Fatalf("last owner did not release and close once: releases=%#v closed=%d", arbiter.releases, arbiter.closed)
	}
}
