//go:build !analytix_native_build_probe || analytix_prod

package processauthority

func currentBuildProbeMainAllowed() bool {
	return false
}
