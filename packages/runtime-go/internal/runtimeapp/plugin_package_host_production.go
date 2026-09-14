//go:build analytix_prod

package runtimeapp

import (
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
	"context"
)

func newDevelopmentPackageHost(context.Context, Config, identityport.Authority, map[string]adapterport.Adapter) *hostapp.Service {
	return nil
}
