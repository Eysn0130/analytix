package turnsecurity

import (
	"context"
	"testing"
	"time"

	appidentity "analytix.local/runtime-go/internal/app/identity"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

func testIdentityAuthority() identityport.Authority {
	authority, err := appidentity.NewInstallationLocalAuthority(
		domainsecurity.SHA256Hex([]byte("turnsecurity-test-installation")),
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

type panicIdentityObserver struct{}

func (panicIdentityObserver) Observe(string) (domainsecurity.CaseBindingObservationV1, error) {
	panic("identity mismatch reached case observation")
}

func TestFreezeWorkspaceRejectsIdentityMismatchBeforeCaseOrDatasetAuthority(t *testing.T) {
	other, err := appidentity.NewInstallationLocalAuthority(
		domainsecurity.SHA256Hex([]byte("other-turnsecurity-test-installation")),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = FreezeWorkspace(WorkspaceFreezeInput{
		Context: context.Background(),
		Authority: WorkspaceSecurityAuthority{
			Identity: other,
			Observer: panicIdentityObserver{},
		},
		Thread:    map[string]any{},
		ThreadID:  "thread-identity-mismatch",
		TurnID:    "turn-identity-mismatch",
		Workspace: "/workspace/identity-mismatch",
		Principal: testIdentityPrincipal(),
		IssuedAt:  time.Now().UTC(),
	})
	if err == nil || err.Error() != "turn_security_identity_mismatch" ||
		CurrentFailureCode(err) != "turn_security_identity_mismatch" {
		t.Fatalf("identity mismatch did not fail closed with the fixed code: %v", err)
	}
}
