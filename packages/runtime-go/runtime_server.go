//go:build !analytix_prod

package runtimego

import (
	"net/http"

	"analytix.local/runtime-go/internal/runtimeapp"
)

// Compatibility shim: the production runtime server lives in internal/server.
// Root exports remain available for existing contract tests.
type RuntimeServerConfig = runtimeapp.Config
type RuntimeServerContractConfig = runtimeapp.ContractConfig

const DefaultRuntimeToken = runtimeapp.DefaultRuntimeToken

func NewRuntimeServerHandler(config RuntimeServerConfig) http.Handler {
	return runtimeapp.NewRuntimeServerHandler(config)
}

func NewRuntimeServerContractHandler(config RuntimeServerContractConfig) http.Handler {
	return runtimeapp.NewRuntimeServerContractHandler(config)
}
