package identity

import (
	"context"
	"errors"
	"testing"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	identityport "analytix.local/runtime-go/internal/ports/identity"
)

func TestInstallationLocalAuthorityRejectsAnotherInstallation(t *testing.T) {
	first, err := NewInstallationLocalAuthority(domainsecurity.SHA256Hex([]byte("first")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewInstallationLocalAuthority(domainsecurity.SHA256Hex([]byte("second")))
	if err != nil {
		t.Fatal(err)
	}
	principal, err := first.ResolveCurrent(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ValidateCurrent(context.Background(), principal); err != nil {
		t.Fatal(err)
	}
	if err := second.ValidateCurrent(context.Background(), principal); !errors.Is(err, identityport.ErrMismatch) {
		t.Fatalf("another installation accepted the local projection: %v", err)
	}
}
