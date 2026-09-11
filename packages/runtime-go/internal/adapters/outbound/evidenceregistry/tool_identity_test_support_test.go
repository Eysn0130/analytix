package evidenceregistry

import (
	"crypto/sha256"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func storeTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("adapter-evidence-registry-test-tool-call:\x00" + seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}
