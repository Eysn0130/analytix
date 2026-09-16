package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	packagedauthority "analytix.local/runtime-go/internal/adapters/outbound/packagedbuildauthorityfs"
	"analytix.local/runtime-go/internal/app/canvasediting"
	objectapp "analytix.local/runtime-go/internal/app/objectediting"
	identityport "analytix.local/runtime-go/internal/ports/identity"
	adapterport "analytix.local/runtime-go/internal/ports/pluginpackagehost"
)

func newDevelopmentCanvasAdapter(ctx context.Context, config Config, identity identityport.Authority, protected []string) adapterport.Adapter {
	if config.DevelopmentPluginSourceRoot == "" || (runtime.GOOS != "darwin" && runtime.GOOS != "linux") {
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
	return composeCanvasAdapter(config, identity, protected, current)
}

func composeCanvasAdapter(config Config, identity identityport.Authority, protected []string, current func(context.Context) bool) adapterport.Adapter {
	if identity == nil || current == nil || !filepath.IsAbs(config.DataDir) {
		return nil
	}
	root := filepath.Join(config.DataDir, "object-editing")
	if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil
	}
	objects := make(map[string]canvasediting.Objects)
	for _, kind := range []string{"canvas", "png"} {
		files, err := filestore.NewCanvasObjectEditingFiles(root, protected, kind)
		if err != nil {
			return nil
		}
		// Each object service belongs exclusively to this Canvas owner. Sharing
		// these session maps with other adapters would break reference release.
		objects[kind] = objectapp.New(identity, files)
	}
	return canvasediting.NewAdapter(canvasediting.New(identity, objects), current)
}
