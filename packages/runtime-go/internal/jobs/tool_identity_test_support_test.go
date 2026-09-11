package jobs

import (
	"crypto/sha256"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func jobsTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("jobs-test-tool-call:\x00" + seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}
