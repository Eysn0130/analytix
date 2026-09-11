//go:build windows

package finalauthority

import (
	"context"
)

// Windows is intentionally fail-closed until handle-relative replace,
// directory durability, DACL/reparse validation, and crash behavior are proven
// on an approved Windows host. MoveFileEx alone is not that proof.
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
