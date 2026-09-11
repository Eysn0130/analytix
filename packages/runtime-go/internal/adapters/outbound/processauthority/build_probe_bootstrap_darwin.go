//go:build darwin && analytix_native_build_probe && !analytix_prod

package processauthority

import (
	"os"
	"syscall"
)

const darwinBuildProbeBootstrapEnvironment = "ANALYTIX_NATIVE_BUILD_PROBE_BOOTSTRAP_V1"

func init() {
	if !currentBuildProbeMainAllowed() || os.Getenv(darwinBootstrapEnvironment) != "1" ||
		os.Getenv(darwinBuildProbeBootstrapEnvironment) != "1" || bootstrapMarkerIndex(os.Args) <= 1 {
		return
	}
	handled, err := dispatchBuildProbeBootstrapInvocation(os.Args)
	if !handled || err != nil {
		os.Exit(125)
	}
	os.Exit(125)
}

func dispatchBuildProbeBootstrapInvocation(arguments []string) (bool, error) {
	markerIndex := bootstrapMarkerIndex(arguments)
	if markerIndex == -1 {
		return false, nil
	}
	if !currentBuildProbeMainAllowed() || markerIndex < 2 ||
		os.Getenv(darwinBootstrapEnvironment) != "1" ||
		os.Getenv(darwinBuildProbeBootstrapEnvironment) != "1" ||
		markerIndex != len(arguments)-1 || arguments[markerIndex-1] != "--" {
		return true, errBootstrapInvalid
	}
	return true, dispatchBootstrap(arguments, markerIndex)
}

func prepareDarwinBuildProbeBootstrap() error {
	if os.Getenv(darwinBuildProbeBootstrapEnvironment) != "1" {
		return nil
	}
	if !currentBuildProbeMainAllowed() || syscall.Getpgrp() == os.Getpid() {
		return errBootstrapInvalid
	}
	if err := syscall.Setpgid(0, 0); err != nil || syscall.Getpgrp() != os.Getpid() {
		return errBootstrapInvalid
	}
	return nil
}
