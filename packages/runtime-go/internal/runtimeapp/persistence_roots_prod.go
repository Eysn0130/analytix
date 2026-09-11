//go:build analytix_prod

package runtimeapp

import (
	"strings"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

func resolveRuntimePersistenceRoots(config Config) (persistencefs.RootSet, error) {
	durableRoot := strings.TrimSpace(config.ProductionDurableRoot)
	if durableRoot == "" {
		durableRoot = strings.TrimSpace(config.DurableTempDir)
	}
	return persistencefs.ResolveRootSet(config.DataDir, durableRoot)
}

func applyRuntimePersistenceRoots(config Config, roots persistencefs.RootSet) Config {
	config.DataDir = roots.DataDir
	if strings.TrimSpace(config.ProductionDurableRoot) != "" {
		config.ProductionDurableRoot = roots.DurableDir
	} else {
		config.DurableTempDir = roots.DurableDir
	}
	return config
}
