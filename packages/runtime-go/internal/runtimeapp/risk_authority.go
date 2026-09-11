package runtimeapp

import (
	"errors"

	"analytix.local/runtime-go/internal/adapters/outbound/filestore"
	threadriskauthorityapp "analytix.local/runtime-go/internal/app/threadriskauthority"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
	threadriskpolicyport "analytix.local/runtime-go/internal/ports/threadriskpolicy"
)

// newRuntimeThreadRiskAuthority keeps the independently enrolled witness as
// the preferred authority. When that composition is absent, the host may only
// derive the non-elevating general/read-only policy from a fresh filesystem
// observation. The accepted first-stage fallback reuses the installed final
// authority and one exact private policy CAS; it adds no selector or witness.
func newRuntimeThreadRiskAuthority(
	config Config,
	installationAuthority finalauthorityport.Authority,
	policies threadriskpolicyport.Store,
) (turnsecurityapp.RiskPolicyAuthority, error) {
	authority, witnessed, err := newContractThreadRiskAuthority(config, installationAuthority)
	if err != nil {
		return nil, err
	}
	if witnessed {
		if authority == nil {
			return nil, errors.New("witnessed thread risk authority composition is inconsistent")
		}
		return authority, nil
	}
	if authority != nil {
		return nil, errors.New("unwitnessed thread risk authority cannot enter production composition")
	}
	if installationAuthority != nil && policies != nil &&
		domainsecurity.IsSHA256Hex(installationAuthority.KeyID()) &&
		installationAuthority.KeyID() == domainsecurity.SHA256Hex(installationAuthority.PublicKey()) {
		return threadriskauthorityapp.NewHostPolicyAuthority(
			filestore.CaseBindingReader{}, installationAuthority, policies,
		)
	}
	return threadriskauthorityapp.NewGeneralOnlyAuthority(filestore.CaseBindingReader{})
}
