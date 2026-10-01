//go:build analytix_dev_credentials && windows

package runtimeapp

import (
	"os"

	secretstore "analytix.local/runtime-go/internal/adapters/outbound/secretstore"
)

const developmentProviderAuthorityEnabled = true

func developmentProviderDirectorySecure(path string, _ os.FileInfo) bool {
	return secretstore.ValidateWindowsDevelopmentAuthorityDirectory(path) == nil
}
