package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	"analytix.local/runtime-go/internal/adapters/outbound/officeengineassets"
	packagedauthority "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	"analytix.local/runtime-go/internal/app/officeediting"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func newOfficeEditingAdapters(ctx context.Context, config Config, identity identityport.Authority, protected []string) map[string]adapterport.Adapter {
	if config.DevelopmentPluginSourceRoot == "" || config.DevelopmentOfficeAssetRoot == "" || identity == nil ||
		(runtime.GOOS != "darwin" && runtime.GOOS != "linux") || !filepath.IsAbs(config.DataDir) {
		return nil
	}
	if _, err := packagedauthority.InspectCurrentPackageV2(ctx); !errors.Is(err, packagedauthority.ErrNotPackagedRuntimeV2) {
		return nil
	}
	assets, err := officeengineassets.Open(ctx, config.DevelopmentOfficeAssetRoot)
	if err != nil {
		return nil
	}
	return composeOfficeEditingAdapters(config, identity, protected, assets.Current)
}

func composeOfficeEditingAdapters(config Config, identity identityport.Authority, protected []string, current func(context.Context) bool) map[string]adapterport.Adapter {
	if identity == nil || current == nil || !filepath.IsAbs(config.DataDir) {
		return nil
	}
	root := filepath.Join(config.DataDir, "object-editing")
	if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil
	}
	result := make(map[string]adapterport.Adapter)
	for kind, id := range map[string]string{"docx": "analytix-documents", "xlsx": "analytix-spreadsheets", "pptx": "analytix-presentations"} {
		files, err := filestore.NewOfficeObjectEditingFiles(root, protected, kind)
		if err != nil {
			continue
		}
		result[id] = officeediting.New(kind, editingapp.New(identity, files), current)
	}
	return result
}
