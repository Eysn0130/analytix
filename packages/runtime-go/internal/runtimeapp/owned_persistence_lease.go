package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"sync"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

// NewRuntimeServerHandlerWithOwnedPersistenceLeaseE is the safe embedded
// production factory. The returned handler owns the composite lease until its
// Shutdown method is called or the process exits.
func NewRuntimeServerHandlerWithOwnedPersistenceLeaseE(config Config) (http.Handler, error) {
	lease, err := AcquireRuntimePersistenceLease(config)
	if err != nil {
		return nil, err
	}
	handler, err := NewRuntimeServerHandlerWithPersistenceLeaseE(config, lease)
	if err != nil {
		return nil, errors.Join(err, lease.Close())
	}
	return &ownedPersistenceLeaseHandler{Handler: handler, lease: lease}, nil
}

type ownedPersistenceLeaseHandler struct {
	http.Handler
	lease            *persistencefs.CompositeLease
	shutdownMu       sync.Mutex
	shutdownComplete bool
	shutdownErr      error
}

func (handler *ownedPersistenceLeaseHandler) Shutdown(ctx context.Context) error {
	handler.shutdownMu.Lock()
	defer handler.shutdownMu.Unlock()
	if handler.shutdownComplete {
		return handler.shutdownErr
	}
	if lifecycle, ok := handler.Handler.(interface{ Shutdown(context.Context) error }); ok {
		if err := lifecycle.Shutdown(ctx); err != nil {
			// A failed lifecycle drain means managed writers or jobs may still
			// hold authority. Keep the persistence lease and allow a later call
			// with a fresh context to retry the drain.
			return err
		}
	}
	handler.shutdownErr = handler.lease.Close()
	handler.shutdownComplete = true
	return handler.shutdownErr
}

func (handler *ownedPersistenceLeaseHandler) FinalPublicationAuthorityIdentityV1() (FinalPublicationAuthorityIdentityV1, error) {
	source, ok := handler.Handler.(FinalPublicationAuthorityIdentitySourceV1)
	if !ok {
		return FinalPublicationAuthorityIdentityV1{}, errors.New("final publication authority identity is unavailable")
	}
	return source.FinalPublicationAuthorityIdentityV1()
}
