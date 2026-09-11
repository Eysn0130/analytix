//go:build !analytix_prod

package processauthority

import "runtime/debug"

const (
	processAuthorityTestMainPath      = "analytix.local/runtime-go/internal/adapters/outbound/processauthority.test"
	nativeComponentRunnerTestMainPath = "analytix.local/runtime-go/internal/adapters/outbound/nativecomponentrunner.test"
)

func bootstrapTestMainPathAllowed(buildPath string) bool {
	switch buildPath {
	case processAuthorityTestMainPath, nativeComponentRunnerTestMainPath:
		return true
	default:
		return false
	}
}

func currentBootstrapTestMainAllowed() bool {
	build, ok := debug.ReadBuildInfo()
	return ok && bootstrapTestMainPathAllowed(build.Path)
}

func currentBootstrapMainAllowed() bool {
	return currentBootstrapTestMainAllowed()
}
