//go:build analytix_native_build_probe && !analytix_prod

package processauthority

import "runtime/debug"

const buildProbeMainPath = "analytix.local/runtime-go/cmd/native-component-build-probe"

func currentBuildProbeMainAllowed() bool {
	build, ok := debug.ReadBuildInfo()
	return ok && build.Path == buildProbeMainPath
}
