package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

type runtimeOwnedResourceCloserV1 interface {
	Close() error
}

// runtimeOwnedResourceHandlerV1 keeps a borrowed private-state resource open
// until the server has stopped admissions and drained all owned operations.
// Failed drains or closes retain ownership so Shutdown can safely retry.
type runtimeOwnedResourceHandlerV1 struct {
	http.Handler
	resource runtimeOwnedResourceCloserV1

	mu               sync.Mutex
	innerComplete    bool
	shutdownComplete bool
	shutdownErr      error
}

func bindRuntimeOwnedResourceV1(
	handler http.Handler,
	resource runtimeOwnedResourceCloserV1,
) (http.Handler, error) {
	if handler == nil || resource == nil {
		return nil, errors.New("runtime owned resource is unavailable")
	}
	return &runtimeOwnedResourceHandlerV1{Handler: handler, resource: resource}, nil
}

func (handler *runtimeOwnedResourceHandlerV1) Shutdown(ctx context.Context) error {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if handler.shutdownComplete {
		return handler.shutdownErr
	}
	if !handler.innerComplete {
		if lifecycle, ok := handler.Handler.(interface{ Shutdown(context.Context) error }); ok {
			if err := lifecycle.Shutdown(ctx); err != nil {
				return err
			}
		}
		handler.innerComplete = true
	}
	if err := handler.resource.Close(); err != nil {
		return err
	}
	handler.shutdownComplete = true
	handler.shutdownErr = nil
	return nil
}
