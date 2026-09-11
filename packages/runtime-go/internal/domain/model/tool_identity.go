package model

import domainsecurity "analytix.local/runtime-go/internal/domain/security"

const (
	HostToolCallIDPrefixV1       = domainsecurity.HostToolCallIDPrefixV1
	HostToolCallIDEntropyBytesV1 = domainsecurity.HostToolCallIDEntropyBytesV1
)

// NewHostToolCallIDV1 constructs a provider-independent host identity from
// cryptographically secure host entropy. Provider identifiers are deliberately
// not hashed into this value: they can contain low-entropy account or card
// numbers and must not become dictionary-testable durable identifiers.
func NewHostToolCallIDV1(entropy []byte) (string, error) {
	return domainsecurity.NewHostToolCallIDV1(entropy)
}

func IsHostToolCallIDV1(value string) bool {
	return domainsecurity.IsHostToolCallIDV1(value)
}
