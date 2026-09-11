package identity

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	PrincipalVersionV1 = 1
	LocalTenantID      = "local"
	LocalUserID        = "local"
)

// PrincipalV1 is host-private authentication state. It is deliberately not a
// TurnSecurityContext wire field: the public context keeps its existing
// tenant/user projection while the host binds those values to the current
// installation authority at every execution boundary.
type PrincipalV1 struct {
	Version         int
	InstallationID  string
	TenantID        string
	UserID          string
	PrincipalDigest string
}

func NewPrincipalV1(installationID, tenantID, userID string) (PrincipalV1, error) {
	principal := PrincipalV1{
		Version:        PrincipalVersionV1,
		InstallationID: strings.TrimSpace(installationID),
		TenantID:       strings.TrimSpace(tenantID),
		UserID:         strings.TrimSpace(userID),
	}
	principal.PrincipalDigest = principalDigestV1(principal)
	if err := ValidatePrincipalV1(principal); err != nil {
		return PrincipalV1{}, err
	}
	return principal, nil
}

func ValidatePrincipalV1(principal PrincipalV1) error {
	if principal.Version != PrincipalVersionV1 ||
		!domainsecurity.IsSHA256Hex(principal.InstallationID) ||
		!canonicalPrincipalText(principal.TenantID) ||
		!canonicalPrincipalText(principal.UserID) ||
		!domainsecurity.IsSHA256Hex(principal.PrincipalDigest) ||
		principal.PrincipalDigest != principalDigestV1(principal) {
		return errors.New("host principal authority is invalid")
	}
	return nil
}

func SamePrincipalV1(left, right PrincipalV1) bool {
	return ValidatePrincipalV1(left) == nil &&
		ValidatePrincipalV1(right) == nil &&
		left == right
}

func principalDigestV1(principal PrincipalV1) string {
	principal.PrincipalDigest = ""
	body, _ := json.Marshal(struct {
		Version        int    `json:"version"`
		InstallationID string `json:"installationId"`
		TenantID       string `json:"tenantId"`
		UserID         string `json:"userId"`
	}{
		Version: principal.Version, InstallationID: principal.InstallationID,
		TenantID: principal.TenantID, UserID: principal.UserID,
	})
	return domainsecurity.SHA256Hex(body)
}

func canonicalPrincipalText(value string) bool {
	return value != "" && value == strings.TrimSpace(value) &&
		len(value) <= 256 && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}
