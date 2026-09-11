//go:build analytix_native_build_probe && !analytix_prod && !darwin

package processauthority

import (
	"context"
	"os"
)

func openBuildProbeTarget() (*os.File, error) {
	return nil, ErrBuildProbeUnsupported
}

func openBuildProbeRequestInput(_ int, _ string) (*os.File, error) {
	return nil, ErrBuildProbeUnsupported
}

func probePinnedBuildArtifact(
	_ context.Context,
	_ *os.File,
	_ BuildProbeRequestV1,
	_ BuildProbeProtocol,
) (BuildProbeReceipt, error) {
	return BuildProbeReceipt{}, ErrBuildProbeUnsupported
}

func currentBuildProbeAuthorityIdentity(_ context.Context) (BuildProbeAuthorityIdentityV1, error) {
	return BuildProbeAuthorityIdentityV1{}, ErrBuildProbeUnsupported
}

func runBuildProbeCoordinator(
	_ context.Context,
	_ buildProbeRequest,
	target *os.File,
	_ BuildProbeProtocol,
) (BuildProbeReceipt, error) {
	if target != nil {
		_ = target.Close()
	}
	return BuildProbeReceipt{}, ErrBuildProbeUnsupported
}

func runBuildProbeGuardianMain(_ BuildProbeProtocol) int {
	return 1
}
