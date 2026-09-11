//go:build darwin && (!analytix_native_build_probe || analytix_prod)

package processauthority

const darwinBuildProbeBootstrapEnvironment = "ANALYTIX_NATIVE_BUILD_PROBE_BOOTSTRAP_V1"

func prepareDarwinBuildProbeBootstrap() error {
	return nil
}
