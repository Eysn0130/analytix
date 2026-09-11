package server

import (
	"context"

	appidentity "analytix.local/runtime-go/internal/app/identity"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

func testIdentityAuthority() identityport.Authority {
	authority, err := appidentity.NewInstallationLocalAuthority(
		domainsecurity.SHA256Hex([]byte("server-test-installation")),
	)
	if err != nil {
		panic(err)
	}
	return authority
}

func testIdentityPrincipal() domainidentity.PrincipalV1 {
	principal, err := testIdentityAuthority().ResolveCurrent(context.Background())
	if err != nil {
		panic(err)
	}
	return principal
}
