package runtimeapp

import (
	"context"
	"errors"

	evidenceregistrystore "analytix.local/runtime-go/internal/adapters/outbound/evidenceregistry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	registryport "analytix.local/runtime-go/internal/ports/evidenceregistry"
)

type runtimeOriginalRegistryInventoryV1 struct {
	legacy      []registryport.InventoryRecord
	v2          *evidenceregistrystore.OriginalGraphV2
	unavailable bool
}

func parseRuntimeOriginalRegistryInventoryV1(ctx context.Context, core *runtimeChildIdentityStartupV1, files runtimeOriginalSemanticFilesV1, contexts []domainsecurity.TurnSecurityContext) (inventory runtimeOriginalRegistryInventoryV1, resultErr error) {
	if !evidenceregistrystore.OriginalFilesContainV2(files) {
		inventory.legacy, resultErr = evidenceregistrystore.ParseOriginalLegacyInventoryV1(ctx, files, contexts, core.verification)
		return inventory, resultErr
	}
	trust := core.originalRegistryTrust
	if err := trust.Revalidate(ctx); err != nil {
		return inventory, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, trust.Revalidate(ctx))
		if resultErr != nil {
			inventory = runtimeOriginalRegistryInventoryV1{}
		}
	}()
	graph, err := evidenceregistrystore.ParseOriginalGraphV2(ctx, files, contexts, trust.projection.InstallationID, trust.projection.Enrollment.EnrollmentID, core.verification)
	if err != nil {
		return inventory, err
	}
	inventory.v2 = &graph
	return inventory, nil
}
