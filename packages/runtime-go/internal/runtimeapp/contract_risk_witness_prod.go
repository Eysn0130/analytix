//go:build analytix_prod

package runtimeapp

import (
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

// Production must use an independently enrolled monotonic witness. A local
// in-memory authority would make rollback look fresh, so this factory always
// remains disabled even when a legacy temp-root field is populated.
func newContractThreadRiskAuthority(
	Config,
	finalauthorityport.Authority,
) (turnsecurityapp.RiskPolicyAuthority, bool, error) {
	return nil, false, nil
}
