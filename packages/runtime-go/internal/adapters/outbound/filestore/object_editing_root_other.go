//go:build !darwin && !linux

package filestore

import objectediting "analytix.local/runtime-go/internal/ports/objectediting"

// Existing-file conditional exchange and private receipt ownership are not
// qualified on these platforms. Never downgrade to a path-only rename.
func objectEditingPrivateRootIdentity(string) (string, error) { return "", objectediting.ErrForbidden }
func objectEditingPrivateReceipt(string) error                { return objectediting.ErrForbidden }
func objectEditingSyncTarget(string) error                    { return objectediting.ErrForbidden }
