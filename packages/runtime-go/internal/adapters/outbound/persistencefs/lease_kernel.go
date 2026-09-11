package persistencefs

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"sync"
)

type kernelRangeSpec struct {
	offset    int64
	exclusive bool
}

// platformKernelArbiter locks deterministic byte ranges on an OS-owned kernel
// object. Its authority is not addressed through a user-writable pathname, so
// replacing managed roots, lockfiles, or their parent directories cannot
// create a second writer.
type platformKernelArbiter interface {
	Acquire([]kernelRangeSpec) error
	Release(kernelRangeSpec) error
	Validate() error
	Close() error
}

type kernelScopeLease struct {
	id uint64
}

type kernelScopeRecord struct {
	paths  []compositeLeaseSpec
	ranges []kernelRangeSpec
}

type kernelRangeHold struct {
	exclusive bool
	refs      int
}

var persistenceKernelScopes = struct {
	sync.Mutex
	next    uint64
	arbiter platformKernelArbiter
	leases  map[uint64]kernelScopeRecord
	held    map[int64]kernelRangeHold
}{leases: map[uint64]kernelScopeRecord{}, held: map[int64]kernelRangeHold{}}

func acquireKernelScopeLease(roots RootSet) (*kernelScopeLease, error) {
	return acquireKernelScopeLeaseForSpecs(compositeLeaseSpecs(roots))
}

func acquireKernelScopeLeaseForSpecs(paths []compositeLeaseSpec) (*kernelScopeLease, error) {
	ranges := kernelRangesForSpecs(paths)
	persistenceKernelScopes.Lock()
	defer persistenceKernelScopes.Unlock()
	for _, held := range persistenceKernelScopes.leases {
		if kernelScopeSpecsConflict(paths, held.paths) {
			return nil, ErrPersistenceInUse
		}
	}
	for _, spec := range ranges {
		if held, ok := persistenceKernelScopes.held[spec.offset]; ok && (spec.exclusive || held.exclusive) {
			return nil, ErrPersistenceInUse
		}
	}
	if persistenceKernelScopes.arbiter == nil {
		arbiter, err := acquirePlatformKernelArbiter()
		if err != nil {
			return nil, err
		}
		persistenceKernelScopes.arbiter = arbiter
	}
	newRanges := make([]kernelRangeSpec, 0, len(ranges))
	for _, spec := range ranges {
		if _, ok := persistenceKernelScopes.held[spec.offset]; !ok {
			newRanges = append(newRanges, spec)
		}
	}
	if err := persistenceKernelScopes.arbiter.Acquire(newRanges); err != nil {
		if len(persistenceKernelScopes.leases) == 0 {
			_ = persistenceKernelScopes.arbiter.Close()
			persistenceKernelScopes.arbiter = nil
		}
		return nil, err
	}
	for _, spec := range ranges {
		hold := persistenceKernelScopes.held[spec.offset]
		hold.refs++
		hold.exclusive = hold.exclusive || spec.exclusive
		persistenceKernelScopes.held[spec.offset] = hold
	}
	persistenceKernelScopes.next++
	id := persistenceKernelScopes.next
	persistenceKernelScopes.leases[id] = kernelScopeRecord{paths: append([]compositeLeaseSpec(nil), paths...), ranges: append([]kernelRangeSpec(nil), ranges...)}
	return &kernelScopeLease{id: id}, nil
}

func kernelRangesForSpecs(specs []compositeLeaseSpec) []kernelRangeSpec {
	byOffset := make(map[int64]bool, len(specs))
	for _, spec := range specs {
		digest := sha256.Sum256([]byte(spec.path))
		offset := int64(binary.BigEndian.Uint64(digest[:8]) & uint64(^uint64(0)>>1))
		byOffset[offset] = byOffset[offset] || spec.exclusive
	}
	offsets := make([]int64, 0, len(byOffset))
	for offset := range byOffset {
		offsets = append(offsets, offset)
	}
	sort.Slice(offsets, func(left int, right int) bool { return offsets[left] < offsets[right] })
	ranges := make([]kernelRangeSpec, 0, len(offsets))
	for _, offset := range offsets {
		ranges = append(ranges, kernelRangeSpec{offset: offset, exclusive: byOffset[offset]})
	}
	return ranges
}

func kernelScopeSpecsConflict(left []compositeLeaseSpec, right []compositeLeaseSpec) bool {
	rightModes := make(map[string]bool, len(right))
	for _, spec := range right {
		rightModes[spec.path] = spec.exclusive
	}
	for _, spec := range left {
		if exclusive, ok := rightModes[spec.path]; ok && (spec.exclusive || exclusive) {
			return true
		}
	}
	return false
}

func (lease *kernelScopeLease) Validate() error {
	if lease == nil || lease.id == 0 {
		return errors.New("persistence kernel scope lease is unavailable")
	}
	persistenceKernelScopes.Lock()
	defer persistenceKernelScopes.Unlock()
	if _, ok := persistenceKernelScopes.leases[lease.id]; !ok || persistenceKernelScopes.arbiter == nil {
		return errors.New("persistence kernel scope lease is no longer active")
	}
	return persistenceKernelScopes.arbiter.Validate()
}

func (lease *kernelScopeLease) Close() error {
	if lease == nil || lease.id == 0 {
		return nil
	}
	persistenceKernelScopes.Lock()
	defer persistenceKernelScopes.Unlock()
	record, ok := persistenceKernelScopes.leases[lease.id]
	if !ok {
		lease.id = 0
		return nil
	}
	remaining := append([]kernelRangeSpec(nil), record.ranges...)
	for len(remaining) > 0 {
		spec := remaining[0]
		hold := persistenceKernelScopes.held[spec.offset]
		if hold.refs > 1 {
			hold.refs--
			persistenceKernelScopes.held[spec.offset] = hold
		} else {
			if persistenceKernelScopes.arbiter == nil {
				return errors.New("persistence kernel arbiter disappeared before release")
			}
			if err := persistenceKernelScopes.arbiter.Release(spec); err != nil {
				record.ranges = remaining
				persistenceKernelScopes.leases[lease.id] = record
				return err
			}
			delete(persistenceKernelScopes.held, spec.offset)
		}
		remaining = remaining[1:]
		record.ranges = remaining
		persistenceKernelScopes.leases[lease.id] = record
	}
	delete(persistenceKernelScopes.leases, lease.id)
	lease.id = 0
	if len(persistenceKernelScopes.leases) != 0 {
		return nil
	}
	arbiter := persistenceKernelScopes.arbiter
	persistenceKernelScopes.arbiter = nil
	return arbiter.Close()
}
