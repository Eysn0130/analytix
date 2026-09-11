//go:build analytix_prod

package runtimeapp

import (
	datasetsnapshotport "analytix.local/runtime-go/internal/ports/datasetsnapshot"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

func newContractDatasetSnapshotAuthority(
	Config,
	finalauthorityport.Authority,
) (datasetsnapshotport.Authority, bool, error) {
	return nil, false, nil
}
