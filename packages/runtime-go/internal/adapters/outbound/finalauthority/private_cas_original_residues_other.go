//go:build !darwin && !linux && !windows

package finalauthority

import (
	"context"
	"errors"
)

type privateCASOriginalResidueV1 struct{ emptyShard bool }

func privateCASOriginalResiduesFromPlanV1(privateCASPreparedRecoveryPlan, map[string]privateCASShardIdentity) (privateCASOriginalResiduesV1, error) {
	return nil, errors.New("private CAS secure storage is unsupported")
}

func privateCASOriginalResidueEqualV1(privateCASOriginalResidueV1, privateCASOriginalResidueV1) bool {
	return false
}

func privateCASRestoreOriginalEmptyShardsV1(privateCASPreparedRecoveryPlan, privateCASOriginalResiduesV1, privateCASOriginalResiduesV1) privateCASOriginalResiduesV1 {
	return nil
}

func privateCASOriginalShardPinsV1(privateCASPreparedRecoveryPlan) map[string]privateCASShardIdentity {
	return nil
}
func securePrivateCASObserveRecoveryIncludingOriginalCreatesV1(context.Context, privateCASRootAuthority, int) (privateCASRecoveryObservation, error) {
	return privateCASRecoveryObservation{}, errors.New("original CAS creation residues are unsupported")
}
