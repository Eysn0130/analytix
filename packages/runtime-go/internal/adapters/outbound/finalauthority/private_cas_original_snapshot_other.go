//go:build !darwin && !linux && !windows

package finalauthority

import (
	"context"
	"errors"
)

func privateCASOriginalRootModeV1(privateCASRootAuthority) (uint32, error) {
	return 0, errors.New("original private CAS snapshot is unsupported")
}
func securePrivateCASOriginalSnapshotV1(context.Context, privateCASRootAuthority, privateCASRecoveryObservation, int, bool) (map[string]SecurePrivateCASOriginalEntryV1, error) {
	return nil, errors.New("original private CAS snapshot is unsupported")
}
