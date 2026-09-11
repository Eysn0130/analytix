//go:build !darwin && !linux && !windows

package persistencefs

import (
	"errors"
	"os"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

var errSecureManagedUnsupported = errors.New("secure semantic startup mutation is unsupported on this platform")

func secureEnsureManagedRoot(*RootAuthority, string) error { return errSecureManagedUnsupported }
func secureManagedTargetMatches(*RootAuthority, string, string, domainstartup.SemanticEntryStateV1) (bool, error) {
	return false, errSecureManagedUnsupported
}
func secureManagedCreateDirectory(*RootAuthority, string, string, os.FileMode) error {
	return errSecureManagedUnsupported
}
func secureManagedInstallFile(*RootAuthority, string, string, []byte, os.FileMode, string, domainstartup.SemanticEntryStateV1, domainstartup.SemanticEntryStateV1) error {
	return errSecureManagedUnsupported
}
func secureManagedSetMode(*RootAuthority, string, string, domainstartup.SemanticEntryStateV1, os.FileMode) error {
	return errSecureManagedUnsupported
}
func secureManagedRemove(*RootAuthority, string, string, domainstartup.SemanticEntryStateV1) error {
	return errSecureManagedUnsupported
}
