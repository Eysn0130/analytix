package runtimeapp

import (
	"context"
	"errors"
	"strings"

	"analytix.local/runtime-go/internal/adapters/outbound/officeengineassets"
	packagedauthority "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	materializationport "analytix.local/runtime-go/internal/ports/pluginmaterialization"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// A successful real executable inspection selects the private path. Invalid or
// formal packages never fall back to caller-configured development sources.
func newOfficeRuntime(ctx context.Context, config Config, identity identityport.Authority, protected []string) (map[string]adapterport.Adapter, *hostapp.Service, *officeengineassets.PrivateLocal) {
	inspection, err := packagedauthority.InspectCurrentPackageV2(ctx)
	if errors.Is(err, packagedauthority.ErrNotPackagedRuntimeV2) {
		adapters := newOfficeEditingAdapters(ctx, config, identity, protected)
		return adapters, newDevelopmentPackageHost(ctx, config, identity, adapters), nil
	}
	if err != nil || identity == nil || config.Insecure || strings.TrimSpace(config.RuntimeToken) == "" {
		return nil, nil, nil
	}
	assets, err := officeengineassets.OpenPrivateLocal(ctx, inspection)
	if err != nil {
		return nil, nil, nil
	}
	adapters := composeOfficeEditingAdapters(config, identity, protected, assets.Current)
	// Server composition binds its projector/capture to the concrete adapters.
	// The Host uses wrappers around those same pointers, not replacements in
	// the server map, so all finite selection/editing operations remain bound.
	hostedAdapters := qualifyOfficeAdapters(adapters, assets.Current)
	host := composeOfficePackageHost(ctx, config, identity, hostedAdapters, assets.Root(), assets.Current)
	if host == nil || len(adapters) != 3 || !assets.Current(ctx) {
		return nil, nil, nil
	}
	return adapters, host, assets
}

func qualifyOfficeAdapters(adapters map[string]adapterport.Adapter, current func(context.Context) bool) map[string]adapterport.Adapter {
	hosted := make(map[string]adapterport.Adapter, len(adapters))
	for id, adapter := range adapters {
		hosted[id] = qualifiedOfficeAdapter{inner: adapter, current: current}
	}
	return hosted
}

type qualifiedOfficeResolver struct {
	inner   hostapp.ActiveResolver
	current func(context.Context) bool
}

func (r qualifiedOfficeResolver) ResolveActive(ctx context.Context) (materializationport.ResultV1, error) {
	if r.inner == nil || r.current == nil || !r.current(ctx) {
		return materializationport.ResultV1{}, hostapp.ErrUnavailable
	}
	result, err := r.inner.ResolveActive(ctx)
	if err != nil || !r.current(ctx) {
		return materializationport.ResultV1{}, hostapp.ErrUnavailable
	}
	return result, err
}

// Losing package readiness during a call makes the result unconfirmed. A write
// may already have committed: its fixed operation must be queried after repair.
type qualifiedOfficeAdapter struct {
	inner   adapterport.Adapter
	current func(context.Context) bool
}

func (a qualifiedOfficeAdapter) Readiness(ctx context.Context, binding adapterport.Binding) (adapterport.Readiness, error) {
	if a.inner == nil || a.current == nil || !a.current(ctx) {
		return adapterport.Readiness{}, hostapp.ErrUnavailable
	}
	result, err := a.inner.Readiness(ctx, binding)
	if !a.current(ctx) {
		return adapterport.Readiness{}, hostapp.ErrUnavailable
	}
	return result, err
}
func (a qualifiedOfficeAdapter) Invoke(ctx context.Context, call adapterport.Call) (adapterport.Result, error) {
	if a.inner == nil || a.current == nil || !a.current(ctx) {
		return adapterport.Result{}, hostapp.ErrUnavailable
	}
	result, err := a.inner.Invoke(ctx, call)
	if !a.current(ctx) {
		return adapterport.Result{}, hostapp.ErrUnavailable
	}
	return result, err
}
