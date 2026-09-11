package security

import (
	"encoding/hex"
	"errors"
	"strings"
)

const (
	HostToolCallIDPrefixV1       = "call_host_"
	HostToolCallIDEntropyBytesV1 = 32
)

// NewHostToolCallIDV1 constructs a provider-independent host identity from
// cryptographically secure host entropy. Provider identifiers are deliberately
// excluded because they can contain low-entropy account or card numbers.
func NewHostToolCallIDV1(entropy []byte) (string, error) {
	if len(entropy) != HostToolCallIDEntropyBytesV1 {
		return "", errors.New("host tool-call identity entropy is invalid")
	}
	return HostToolCallIDPrefixV1 + hex.EncodeToString(entropy), nil
}

func IsHostToolCallIDV1(value string) bool {
	if !strings.HasPrefix(value, HostToolCallIDPrefixV1) {
		return false
	}
	digest := strings.TrimPrefix(value, HostToolCallIDPrefixV1)
	if len(digest) != HostToolCallIDEntropyBytesV1*2 || strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == HostToolCallIDEntropyBytesV1
}
