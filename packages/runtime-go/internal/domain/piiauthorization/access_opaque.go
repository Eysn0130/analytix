package piiauthorization

import (
	"errors"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	MinControlledAccessOpaqueTokenBytesV1 = 16
	MaxControlledAccessOpaqueTokenBytesV1 = 1024
)

var (
	controlledAccessHandleDigestDomainV1    = []byte("analytix.controlled-artifact-access/handle/v1\x00")
	controlledAccessUseSlotDigestDomainV1   = []byte("analytix.controlled-artifact-access/use-slot/v1\x00")
	controlledAccessPrincipalDigestDomainV1 = []byte("analytix.controlled-artifact-access/renderer-principal/v1\x00")
	controlledAccessHandleDigestDomainV2    = []byte("analytix.controlled-artifact-access/handle/v2\x00")
	controlledAccessUseSlotDigestDomainV2   = []byte("analytix.controlled-artifact-access/use-slot/v2\x00")
	controlledAccessPrincipalDigestDomainV2 = []byte("analytix.controlled-artifact-access/renderer-principal/v2\x00")
)

// ControlledAccessHandleDigestV1 binds an opaque host-issued publication
// handle without persisting the bearer token itself.
func ControlledAccessHandleDigestV1(value string) (string, error) {
	return controlledAccessOpaqueDigestV1(controlledAccessHandleDigestDomainV1, value)
}

// ControlledAccessUseSlotDigestV1 binds one host-issued use slot. AccessID is
// derived from this digest, so the same slot cannot release twice.
func ControlledAccessUseSlotDigestV1(value string) (string, error) {
	return controlledAccessOpaqueDigestV1(controlledAccessUseSlotDigestDomainV1, value)
}

// ControlledAccessRendererPrincipalDigestV1 binds the exact main-frame
// principal without placing its bearer token in the audit journal.
func ControlledAccessRendererPrincipalDigestV1(value string) (string, error) {
	return controlledAccessOpaqueDigestV1(controlledAccessPrincipalDigestDomainV1, value)
}

// ControlledAccessHandleDigestV2 is domain-separated from every legacy
// bearer-token digest. A V1 admission can therefore never satisfy V2 access.
func ControlledAccessHandleDigestV2(value string) (string, error) {
	return controlledAccessOpaqueDigestV1(controlledAccessHandleDigestDomainV2, value)
}

// ControlledAccessUseSlotDigestV2 is the sole input to AccessID V2. The
// projected outcome intentionally does not change this digest, preserving one
// release opportunity per host-issued slot across outcome replacement.
func ControlledAccessUseSlotDigestV2(value string) (string, error) {
	return controlledAccessOpaqueDigestV1(controlledAccessUseSlotDigestDomainV2, value)
}

// ControlledAccessRendererPrincipalDigestV2 binds the current main-frame
// principal under the V2 authority domain.
func ControlledAccessRendererPrincipalDigestV2(value string) (string, error) {
	return controlledAccessOpaqueDigestV1(controlledAccessPrincipalDigestDomainV2, value)
}

func controlledAccessOpaqueDigestV1(domain []byte, value string) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || len(value) < MinControlledAccessOpaqueTokenBytesV1 ||
		len(value) > MaxControlledAccessOpaqueTokenBytesV1 || !controlledAccessOpaqueTokenV1(value) {
		return "", errors.New("controlled access opaque token is invalid")
	}
	material := append(append([]byte(nil), domain...), []byte(value)...)
	return domainsecurity.SHA256Hex(material), nil
}

func controlledAccessOpaqueTokenV1(value string) bool {
	for _, character := range []byte(value) {
		switch {
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character >= '0' && character <= '9':
		case character == '-', character == '_', character == '.', character == '~':
		default:
			return false
		}
	}
	return true
}
