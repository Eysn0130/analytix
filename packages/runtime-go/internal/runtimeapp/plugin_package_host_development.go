package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	packagedauthority "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	pluginauthority "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationauthority"
	pluginstore "analytix.local/runtime-go/internal/adapters/outbound/pluginmaterializationfs"
	materializationapp "analytix.local/runtime-go/internal/app/pluginmaterialization"
	hostapp "analytix.local/runtime-go/internal/app/pluginpackagehost"
	domainplugin "analytix.local/runtime-go/internal/domain/pluginmaterialization"
	domainpackage "analytix.local/runtime-go/internal/domain/pluginpackage"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

// Source experiments are additive and unavailable in a packaged executable even
// if it was built without the production tag. No caller can supply registrations.
func newDevelopmentPackageHost(ctx context.Context, config Config, identity identityport.Authority, adapters map[string]adapterport.Adapter) (*hostapp.Service, int) {
	if config.DevelopmentPluginSourceRoot == "" {
		return nil, 0
	}
	if identity == nil {
		return nil, 1
	}
	if _, err := packagedauthority.InspectCurrentPackageV2(ctx); !errors.Is(err, packagedauthority.ErrNotPackagedRuntimeV2) {
		return nil, 1
	}
	root, err := canonicalBundledFundsDirectoryV1(config.DevelopmentPluginSourceRoot)
	if err != nil {
		return nil, 1
	}
	descriptors := officePackageDescriptors(root, nil)
	descriptors["analytix-canvas"] = staticEditorPackageDescriptor{root: root}
	return composeStaticEditorPackageHost(ctx, config, identity, adapters, descriptors)
}

// Source attribution/materialization is shared, but execution admission belongs
// to the caller. A private package supplies its sealed, continuously checked root.
func composeOfficePackageHost(ctx context.Context, config Config, identity identityport.Authority, adapters map[string]adapterport.Adapter, root string, current func(context.Context) bool) (*hostapp.Service, int) {
	return composeStaticEditorPackageHost(ctx, config, identity, adapters, officePackageDescriptors(root, current))
}

type staticEditorPackageDescriptor struct {
	root    string
	current func(context.Context) bool
}

func officePackageDescriptors(root string, current func(context.Context) bool) map[string]staticEditorPackageDescriptor {
	result := make(map[string]staticEditorPackageDescriptor)
	for _, id := range []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations"} {
		result[id] = staticEditorPackageDescriptor{root: root, current: current}
	}
	return result
}

// One Host and one materialization authority own all fixed editor packages.
// Each descriptor retains its own continuously checked source admission.
func composeStaticEditorPackageHost(ctx context.Context, config Config, identity identityport.Authority, adapters map[string]adapterport.Adapter, descriptors map[string]staticEditorPackageDescriptor) (*hostapp.Service, int) {
	if identity == nil {
		return nil, len(descriptors)
	}
	dataDir, runtimeHome, err := bundledFundsRuntimeRootsV1(config.DataDir)
	if err != nil {
		return nil, len(descriptors)
	}
	// Inspect before enrolling anything. These are the only source packages this
	// executable knows how to compose; descriptors contain no executable scripts.
	ids := []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations", "analytix-canvas"}
	validationErrors := 0
	bindings := make(map[string]materializationapp.DevelopmentSourceBindingV1)
	registrations := make(map[string]domainpackage.DevelopmentSourceRegistrationV1)
	for _, id := range ids {
		descriptor, exists := descriptors[id]
		if !exists {
			continue
		}
		if descriptor.root == "" || (descriptor.current != nil && !descriptor.current(ctx)) {
			validationErrors++
			continue
		}
		source := filepath.Join(descriptor.root, "plugins", id)
		observed, err := pluginstore.InspectDevelopmentSourceTreeV1(ctx, source)
		if err != nil {
			validationErrors++
			continue
		}
		registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
		if err != nil || registration.Identity.PackageID != id {
			validationErrors++
			continue
		}
		binding, err := materializationapp.NewDevelopmentSourceBindingV1(registration, source, domainplugin.TargetV1{Platform: runtime.GOOS, Arch: runtime.GOARCH})
		if err != nil {
			validationErrors++
			continue
		}
		bindings[id], registrations[id] = binding, registration
	}
	if len(bindings) == 0 {
		return nil, validationErrors
	}
	_, stateErr := os.Lstat(filepath.Join(runtimeHome, ".state", "bundled-plugin-materialization", "v1"))
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return nil, len(descriptors)
	}
	_, cacheErr := os.Lstat(filepath.Join(runtimeHome, "plugins", "cache", "analytix-hub"))
	if cacheErr != nil && !errors.Is(cacheErr, os.ErrNotExist) {
		return nil, len(descriptors)
	}
	authority, err := pluginauthority.OpenOrCreateV1(dataDir, stateErr == nil || cacheErr == nil)
	if err != nil {
		return nil, len(descriptors)
	}
	var hosted []hostapp.Registration
	for _, id := range ids {
		binding, exists := bindings[id]
		if !exists {
			continue
		}
		store, err := pluginstore.NewPackageStoreV1(runtimeHome, id, nil)
		if err != nil {
			validationErrors++
			continue
		}
		service, err := materializationapp.NewDevelopmentSourceServiceV1(store, authority, binding, time.Now)
		if err != nil {
			validationErrors++
			continue
		}
		if _, err := service.ResolveActive(ctx); err != nil {
			intent, err := binding.NewIntentV1(time.Now().UTC())
			if err != nil {
				validationErrors++
				continue
			}
			intent, err = store.PrepareDevelopmentIntentV1(ctx, intent)
			if err != nil {
				validationErrors++
				continue
			}
			if _, err := service.Materialize(ctx, intent); err != nil {
				validationErrors++
				continue
			}
		}
		registration := registrations[id]
		var skillReader adapterport.SkillReader
		if registration.StaticEditorSkillSHA256V1() != "" {
			skillReader = pluginstore.StaticEditorSkillReader{Store: store, Authority: authority, SHA256: registration.StaticEditorSkillSHA256V1()}
		}
		var resolver hostapp.ActiveResolver = service
		current := descriptors[id].current
		if current != nil {
			resolver = qualifiedOfficeResolver{inner: service, current: current}
		}
		hosted = append(hosted, hostapp.Registration{Identity: registration.Identity,
			SourceRegistrationSHA256: domainpackage.DevelopmentSourceRegistrationSHA256V1(registration),
			Materialization:          resolver, State: store, Adapter: adapters[id], SkillReader: skillReader})
	}
	host, err := hostapp.New(identity, authority, hosted, time.Now)
	if err != nil {
		return nil, len(descriptors)
	}
	for _, registration := range hosted {
		current := descriptors[registration.Identity.PackageID].current
		if current != nil && !current(ctx) {
			return nil, len(descriptors)
		}
	}
	return host, validationErrors
}
