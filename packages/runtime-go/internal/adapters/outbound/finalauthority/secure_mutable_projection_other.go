//go:build !darwin && !linux && !windows

package finalauthority

import "context"

func openSecureMutableProjection(string, string, int) (privateRootAuthority, error) {
	return privateRootAuthority{}, ErrSecureMutableProjectionUnsupported
}

func observeSecureMutableProjection(context.Context, privateRootAuthority, string, int) (SecureMutableProjectionObservation, error) {
	return SecureMutableProjectionObservation{}, ErrSecureMutableProjectionUnsupported
}

func replaceSecureMutableProjection(context.Context, privateRootAuthority, string, int, string, []byte, *secureMutableProjectionFaults) (SecureMutableProjectionReplaceResult, error) {
	return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, ErrSecureMutableProjectionUnsupported
}

func reconcileSecureMutableProjection(context.Context, privateRootAuthority, string, int, string, string, *secureMutableProjectionFaults) (SecureMutableProjectionReplaceResult, error) {
	return SecureMutableProjectionReplaceResult{State: SecureMutableProjectionNotCommitted}, ErrSecureMutableProjectionUnsupported
}
