package identity

import (
	"context"
	"errors"

	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

// InstallationLocalAuthority is the production single-user identity
// authority. "local/local" is only its non-secret TSC projection; the
// authority itself is installation-bound and cannot be selected by an HTTP,
// IPC, model, MCP, or tool payload.
type InstallationLocalAuthority struct {
	principal domainidentity.PrincipalV1
}

func NewInstallationLocalAuthority(installationID string) (*InstallationLocalAuthority, error) {
	principal, err := domainidentity.NewPrincipalV1(
		installationID, domainidentity.LocalTenantID, domainidentity.LocalUserID,
	)
	if err != nil {
		return nil, errors.Join(identityport.ErrUnavailable, err)
	}
	return &InstallationLocalAuthority{principal: principal}, nil
}

func (authority *InstallationLocalAuthority) ResolveCurrent(ctx context.Context) (domainidentity.PrincipalV1, error) {
	if authority == nil || domainidentity.ValidatePrincipalV1(authority.principal) != nil {
		return domainidentity.PrincipalV1{}, identityport.ErrUnavailable
	}
	if ctx == nil {
		return domainidentity.PrincipalV1{}, identityport.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domainidentity.PrincipalV1{}, err
	}
	return authority.principal, nil
}

func (authority *InstallationLocalAuthority) ValidateCurrent(ctx context.Context, principal domainidentity.PrincipalV1) error {
	current, err := authority.ResolveCurrent(ctx)
	if err != nil {
		return err
	}
	if !domainidentity.SamePrincipalV1(current, principal) {
		return identityport.ErrMismatch
	}
	return nil
}
