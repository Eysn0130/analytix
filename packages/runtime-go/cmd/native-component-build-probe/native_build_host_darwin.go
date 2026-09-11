//go:build darwin && analytix_native_build_probe && !analytix_prod

package main

import (
	nativebuildcli "analytix.local/runtime-go/internal/adapters/inbound/nativebuildcli"
	nativebuildhost "analytix.local/runtime-go/internal/adapters/outbound/nativebuildhost"
	portnativebuild "analytix.local/runtime-go/internal/ports/nativebuild"
)

func openNativeBuildHostV1(request nativebuildcli.RequestV1) (portnativebuild.Host, error) {
	return nativebuildhost.OpenProductionV1(nativebuildhost.ConfigV1{
		RepositoryRoot:  request.RepositoryRoot,
		PublicationRoot: request.PublicationRoot,
		TargetKey:       request.TargetKey,
	})
}
