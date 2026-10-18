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
	docapp "analytix.local/runtime-go/internal/app/documentgeneration"
	editingapp "analytix.local/runtime-go/internal/app/objectediting"
	"analytix.local/runtime-go/internal/app/officeediting"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func newOfficeEditingAdapters(ctx context.Context, config Config, identity identityport.Authority, protected []string) map[string]adapterport.Adapter {
	if config.DevelopmentPluginSourceRoot == "" || identity == nil ||
		(runtime.GOOS != "darwin" && runtime.GOOS != "linux") || !filepath.IsAbs(config.DataDir) {
		return nil
	}
	current := func(ctx context.Context) bool {
		if ctx == nil || ctx.Err() != nil {
			return false
		}
		_, err := packagedauthority.InspectCurrentPackageV2(ctx)
		return errors.Is(err, packagedauthority.ErrNotPackagedRuntimeV2)
	}
	if !current(ctx) {
		return nil
	}
	result := composeOfficeEditingAdapters(config, identity, protected, current, "docx")
	if config.DevelopmentOfficeAssetRoot != "" {
		if assets, err := officeengineassets.Open(ctx, config.DevelopmentOfficeAssetRoot); err == nil {
			for id, adapter := range composeOfficeEditingAdapters(config, identity, protected, assets.Current, "xlsx", "pptx") {
				result[id] = adapter
			}
		}
	}
	return result
}

func composeOfficeEditingAdapters(config Config, identity identityport.Authority, protected []string, current func(context.Context) bool, kinds ...string) map[string]adapterport.Adapter {
	if identity == nil || current == nil || !filepath.IsAbs(config.DataDir) {
		return nil
	}
	root := filepath.Join(config.DataDir, "object-editing")
	if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil
	}
	result := make(map[string]adapterport.Adapter)
	if len(kinds) == 0 {
		kinds = []string{"docx", "xlsx", "pptx"}
	}
	for _, kind := range kinds {
		id := map[string]string{"docx": "analytix-documents", "xlsx": "analytix-spreadsheets", "pptx": "analytix-presentations"}[kind]
		if id == "" {
			continue
		}
		files, err := filestore.NewOfficeObjectEditingFiles(root, protected, kind)
		if err != nil {
			continue
		}
		adapter := officeediting.New(kind, editingapp.New(identity, files), current)
		if kind == "docx" {
			if err := adapter.BindUserCreator(docapp.NewUserCreator(identity, files)); err != nil {
				continue
			}
		}
		result[id] = adapter
	}
	return result
}
