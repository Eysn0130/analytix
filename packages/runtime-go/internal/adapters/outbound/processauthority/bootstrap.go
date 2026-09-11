//go:build !analytix_prod

package processauthority

import (
	"errors"
	"os"
)

const (
	darwinBootstrapMarker      = "analytix-process-authority-bootstrap-v1"
	darwinBootstrapEnvironment = "ANALYTIX_PROCESS_AUTHORITY_BOOTSTRAP_V1"
	darwinBootstrapNonceEnv    = "ANALYTIX_PROCESS_AUTHORITY_BOOTSTRAP_NONCE"
)

var errBootstrapInvalid = errors.New("process_authority_bootstrap_invalid")

// dispatchBootstrapInvocation is reachable only from an exact, build-tagged
// bootstrap identity. The immutable Go build identity is checked before any
// platform effect or bootstrap command is accepted.
func dispatchBootstrapInvocation(arguments []string) (handled bool, dispatchErr error) {
	markerIndex := bootstrapMarkerIndex(arguments)
	if markerIndex == -1 {
		return false, nil
	}
	if !currentBootstrapMainAllowed() || markerIndex < 2 || os.Getenv(darwinBootstrapEnvironment) != "1" ||
		markerIndex != len(arguments)-1 || arguments[markerIndex-1] != "--" {
		return true, errBootstrapInvalid
	}
	return true, dispatchBootstrap(arguments, markerIndex)
}

func bootstrapMarkerIndex(arguments []string) int {
	found := -1
	for index := 1; index < len(arguments); index++ {
		if arguments[index] != darwinBootstrapMarker {
			continue
		}
		if found >= 0 {
			return -2
		}
		found = index
	}
	return found
}
