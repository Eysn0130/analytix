package runtimeapp

import (
	"errors"
	"sync"
)

var errControlledPIIResourceOwnerClosedV1 = errors.New("controlled PII resource owner is closed")

type controlledPIIResourceCloserV1 interface {
	Close() error
}

// controlledPIIResourceOwnerV1 is the sole startup owner for the private PII
// grant, report-publication, and controlled-access store aggregates. Resources
// are borrowed by validation services and released in reverse construction
// order on every failure, quarantine completion, or shutdown path.
type controlledPIIResourceOwnerV1 struct {
	mu        sync.Mutex
	closeOnce sync.Once
	closed    bool
	resources []controlledPIIResourceCloserV1
	closeErr  error
}

func (owner *controlledPIIResourceOwnerV1) Add(resource controlledPIIResourceCloserV1) error {
	if owner == nil || resource == nil {
		return errControlledPIIResourceOwnerClosedV1
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.closed {
		return errControlledPIIResourceOwnerClosedV1
	}
	owner.resources = append(owner.resources, resource)
	return nil
}

func (owner *controlledPIIResourceOwnerV1) Close() error {
	if owner == nil {
		return nil
	}
	owner.closeOnce.Do(func() {
		owner.mu.Lock()
		owner.closed = true
		resources := append([]controlledPIIResourceCloserV1(nil), owner.resources...)
		owner.resources = nil
		owner.mu.Unlock()
		for index := len(resources) - 1; index >= 0; index-- {
			owner.closeErr = errors.Join(owner.closeErr, resources[index].Close())
		}
	})
	return owner.closeErr
}
