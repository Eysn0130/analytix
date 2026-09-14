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
func newDevelopmentPackageHost(ctx context.Context, config Config, identity identityport.Authority, adapters map[string]adapterport.Adapter) *hostapp.Service {
	if config.DevelopmentPluginSourceRoot == "" || identity == nil {
		return nil
	}
	if _, err := packagedauthority.InspectCurrentPackageV2(ctx); !errors.Is(err, packagedauthority.ErrNotPackagedRuntimeV2) {
		return nil
	}
	root, err := canonicalBundledFundsDirectoryV1(config.DevelopmentPluginSourceRoot)
	if err != nil {
		return nil
	}
	dataDir, runtimeHome, err := bundledFundsRuntimeRootsV1(config.DataDir)
	if err != nil {
		return nil
	}
	// Inspect before enrolling anything. These are the only source packages this
	// executable knows how to compose; descriptors contain no executable scripts.
	ids := []string{"analytix-documents", "analytix-spreadsheets", "analytix-presentations"}
	bindings := make(map[string]materializationapp.DevelopmentSourceBindingV1)
	registrations := make(map[string]domainpackage.DevelopmentSourceRegistrationV1)
	for _, id := range ids {
		source := filepath.Join(root, "plugins", id)
		observed, err := pluginstore.InspectDevelopmentSourceTreeV1(ctx, source)
		if err != nil {
			continue
		}
		registration, err := domainpackage.ParseDevelopmentSourceRegistrationV1([]byte(observed.SourceRegistrationJSON))
		if err != nil || registration.Identity.PackageID != id {
			continue
		}
		binding, err := materializationapp.NewDevelopmentSourceBindingV1(registration, source, domainplugin.TargetV1{Platform: runtime.GOOS, Arch: runtime.GOARCH})
		if err != nil {
			continue
		}
		bindings[id], registrations[id] = binding, registration
	}
	if len(bindings) == 0 {
		return nil
	}
	_, stateErr := os.Lstat(filepath.Join(runtimeHome, ".state", "bundled-plugin-materialization", "v1"))
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return nil
	}
	_, cacheErr := os.Lstat(filepath.Join(runtimeHome, "plugins", "cache", "analytix-hub"))
	if cacheErr != nil && !errors.Is(cacheErr, os.ErrNotExist) {
		return nil
	}
	authority, err := pluginauthority.OpenOrCreateV1(dataDir, stateErr == nil || cacheErr == nil)
	if err != nil {
		return nil
	}
	var hosted []hostapp.Registration
	for _, id := range ids {
		binding, exists := bindings[id]
		if !exists {
			continue
		}
		store, err := pluginstore.NewPackageStoreV1(runtimeHome, id, nil)
		if err != nil {
			continue
		}
		service, err := materializationapp.NewDevelopmentSourceServiceV1(store, authority, binding, time.Now)
		if err != nil {
			continue
		}
		if _, err := service.ResolveActive(ctx); err != nil {
			intent, err := binding.NewIntentV1(time.Now().UTC())
			if err != nil {
				continue
			}
			intent, err = store.PrepareDevelopmentIntentV1(ctx, intent)
			if err != nil {
				continue
			}
			if _, err := service.Materialize(ctx, intent); err != nil {
				continue
			}
		}
		registration := registrations[id]
		var skillReader adapterport.SkillReader
		if id == "analytix-documents" && registration.DocumentsSkillSHA256 != "" {
			skillReader = pluginstore.DocumentsSkillReader{Store: store, Authority: authority, SHA256: registration.DocumentsSkillSHA256}
		}
		hosted = append(hosted, hostapp.Registration{Identity: registration.Identity,
			SourceRegistrationSHA256: domainpackage.DevelopmentSourceRegistrationSHA256V1(registration),
			Materialization:          service, State: store, Adapter: adapters[id], SkillReader: skillReader})
	}
	host, err := hostapp.New(identity, authority, hosted, time.Now)
	if err != nil {
		return nil
	}
	return host
}
