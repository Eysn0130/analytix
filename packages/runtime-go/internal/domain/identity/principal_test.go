package identity

import (
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestPrincipalV1BindsTenantUserToInstallationIdentity(t *testing.T) {
	installationID := domainsecurity.SHA256Hex([]byte("installation-a"))
	principal, err := NewPrincipalV1(installationID, LocalTenantID, LocalUserID)
	if err != nil {
		t.Fatal(err)
	}
	if principal.InstallationID != installationID || principal.TenantID != LocalTenantID ||
		principal.UserID != LocalUserID || ValidatePrincipalV1(principal) != nil {
		t.Fatalf("unexpected principal: %#v", principal)
	}
	other, err := NewPrincipalV1(domainsecurity.SHA256Hex([]byte("installation-b")), LocalTenantID, LocalUserID)
	if err != nil {
		t.Fatal(err)
	}
	if SamePrincipalV1(principal, other) {
		t.Fatal("the same local projection crossed installation identity")
	}
}

func TestPrincipalV1RejectsTamperingAndNonCanonicalIdentity(t *testing.T) {
	principal, err := NewPrincipalV1(
		domainsecurity.SHA256Hex([]byte("installation")), LocalTenantID, LocalUserID,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PrincipalV1){
		"installation": func(value *PrincipalV1) { value.InstallationID = domainsecurity.SHA256Hex([]byte("other")) },
		"tenant":       func(value *PrincipalV1) { value.TenantID = "other" },
		"user":         func(value *PrincipalV1) { value.UserID = "other" },
		"digest":       func(value *PrincipalV1) { value.PrincipalDigest = domainsecurity.SHA256Hex([]byte("forged")) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := principal
			mutate(&changed)
			if ValidatePrincipalV1(changed) == nil {
				t.Fatal("tampered principal was accepted")
			}
		})
	}
	if _, err := NewPrincipalV1(principal.InstallationID, " local ", LocalUserID); err != nil {
		t.Fatalf("constructor did not canonicalize host-owned identity: %v", err)
	}
	if _, err := NewPrincipalV1(principal.InstallationID, "bad\nidentity", LocalUserID); err == nil {
		t.Fatal("control-bearing principal identity was accepted")
	}
}
