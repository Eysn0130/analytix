package security

import (
	"crypto/sha256"
)

func securityTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("domain-security-test-tool-call:\x00" + seed))
	identity, err := NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}
