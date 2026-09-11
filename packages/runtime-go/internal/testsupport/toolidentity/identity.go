package toolidentity

import (
	"crypto/sha256"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// MustHostToolCallIDV1 returns a deterministic host-issued identity for test
// fixtures. Production inputs must still obtain fresh entropy from the host;
// tests use a stable seed so every bound grant/item/receipt can share the exact
// same identity without falling back to a provider-authored raw call id.
func MustHostToolCallIDV1(seed string) string {
	entropy := sha256.Sum256([]byte(seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}
