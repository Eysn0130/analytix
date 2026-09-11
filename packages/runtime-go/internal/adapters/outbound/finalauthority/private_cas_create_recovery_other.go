//go:build !darwin && !linux && !windows

package finalauthority

import (
	"context"
	"errors"

	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

type privateCASCreateResiduePreparedPlan struct{}

func securePrivateCASObserveCreateResidueRecovery(
	context.Context,
	privatecasport.RootBinding,
	privateCASDirectoryRecoveryModeV1,
	string,
	...*PreparedSecurePrivateCASOriginalCreateResiduesV1,
) (privateCASCreateResiduePreparedPlan, error) {
	return privateCASCreateResiduePreparedPlan{}, errors.New("private CAS create-residue recovery is unsupported")
}

func securePrivateCASApplyCreateResidueRecovery(
	context.Context,
	privatecasport.RootBinding,
	privateCASDirectoryRecoveryModeV1,
	privateCASCreateResiduePreparedPlan,
	[]string,
	...*PreparedSecurePrivateCASOriginalCreateResiduesV1,
) error {
	return errors.New("private CAS create-residue recovery is unsupported")
}

func privateCASCreateResiduePreparedPlansEqual(
	privateCASCreateResiduePreparedPlan,
	privateCASCreateResiduePreparedPlan,
) bool {
	return false
}

func privateCASCreateResiduePreparedPlanCount(privateCASCreateResiduePreparedPlan) int {
	return 0
}

func privateCASPreparedPlanProvesEmptyPartialOwnerV1(privateCASCreateResiduePreparedPlan, string) bool {
	return false
}

func privateCASOriginalCreateResidueProjectionV1(privateCASCreateResiduePreparedPlan, string) privateCASCreateResiduePreparedPlan {
	return privateCASCreateResiduePreparedPlan{}
}
func privateCASOriginalCreateResiduePathsV1(privateCASCreateResiduePreparedPlan) []string { return nil }
func privateCASOriginalCreateResiduePlansEqualV1(privateCASCreateResiduePreparedPlan, privateCASCreateResiduePreparedPlan) bool {
	return false
}

func privateCASOriginalCreateLeafMatchesV1(privateCASCreateResiduePreparedPlan, string, privateCASPreparedRecoveryPlan) bool {
	return false
}

func privateCASOriginalCreateTargetShardV1(privateCASCreateResiduePreparedPlan, string, string) bool {
	return false
}

func privateCASOriginalCreateFingerprintMaterialV1(privateCASCreateResiduePreparedPlan) string {
	return ""
}

func privateCASOriginalCreateDirectoryStatesV1(privateCASCreateResiduePreparedPlan) []privatecasport.OriginalCreateDirectoryV1 {
	return nil
}

func privateCASOriginalCreateResidueProjectionForOwnersV1(privateCASCreateResiduePreparedPlan, []string) privateCASCreateResiduePreparedPlan {
	return privateCASCreateResiduePreparedPlan{}
}
