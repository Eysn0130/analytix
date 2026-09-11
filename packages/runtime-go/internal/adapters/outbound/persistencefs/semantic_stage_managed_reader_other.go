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
