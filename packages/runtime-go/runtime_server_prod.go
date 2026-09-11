//go:build analytix_prod

package runtimego

import (
	"net/http"

	"analytix.local/runtime-go/internal/runtimeapp"
)

type RuntimeServerConfig = runtimeapp.Config

const DefaultRuntimeToken = runtimeapp.DefaultRuntimeToken

func NewRuntimeServerHandler(config RuntimeServerConfig) http.Handler {
	handler, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		panic(err)
	}
	return handler
}

func NewRuntimeServerHandlerE(config RuntimeServerConfig) (http.Handler, error) {
	return runtimeapp.NewRuntimeServerHandlerWithOwnedPersistenceLeaseE(config)
}
