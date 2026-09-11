//go:build !analytix_prod

package runtimeapp

import (
	"net/http"
	"strings"

	"analytix.local/runtime-go/internal/server"
)

type ContractConfig = server.RuntimeServerContractConfig

func NewRuntimeServerContractHandler(config ContractConfig) http.Handler {
	if strings.TrimSpace(config.RuntimeToken) == "" && !config.Insecure {
		config.RuntimeToken = DefaultRuntimeToken
	}
	return NewRuntimeServerHandler(config)
}
