//go:build !darwin && !linux && !windows

package persistencefs

import (
	"context"

	domainstartup "analytix.local/runtime-go/internal/domain/startup"
)

func readSemanticStageManagedFile(
	context.Context,
	*startupPrivateDirectory,
	string,
	domainstartup.SemanticEntryStateV1,
) ([]byte, error) {
	return nil, errSecureManagedUnsupported
}

func readLegacyTypeScriptLineageManagedSourceV1(
	context.Context,
	*RootAuthority,
	string,
	string,
	domainstartup.SemanticEntryStateV1,
) ([]byte, bool, error) {
	return nil, false, errSecureManagedUnsupported
}
