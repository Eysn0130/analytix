//go:build !darwin && !linux && !windows

package finalauthority

import "errors"

func privateCASRecoveryPlanBindingForJournalV4(
	*PreparedSecurePrivateCASRecoveryV1,
	string,
	uint32,
) (privateCASRecoveryPlanBindingV4, error) {
	return privateCASRecoveryPlanBindingV4{}, errors.New("private CAS secure recovery journal binding is unsupported")
}
