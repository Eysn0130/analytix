//go:build !darwin && !linux

package finalauthority

import (
	"context"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type largeOpaqueRecoveryObservation struct{}

func largeOpaqueRecoveryPlatformSupported() bool { return false }

func largeOpaqueObserveRecoveryPlatform(
	context.Context,
	privatecasport.RootBinding,
	uint64,
	LargeOpaqueRecoveryLimitsV1,
) (privateCASRootAuthority, bool, largeOpaqueRecoveryObservation, error) {
	return privateCASRootAuthority{}, false, largeOpaqueRecoveryObservation{}, ErrLargeOpaqueUnsupportedPlatform
}

func largeOpaqueApplyRecoveryPlatform(
	context.Context,
	privatecasport.RootBinding,
	privateCASRootAuthority,
	bool,
	largeOpaqueRecoveryObservation,
	uint64,
	LargeOpaqueRecoveryLimitsV1,
) (LargeOpaqueRecoveryReportV1, largeOpaqueRecoveryObservation, error) {
	return LargeOpaqueRecoveryReportV1{}, largeOpaqueRecoveryObservation{}, ErrLargeOpaqueUnsupportedPlatform
}
