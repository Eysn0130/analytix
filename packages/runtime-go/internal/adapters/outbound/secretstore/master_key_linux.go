//go:build linux

package secretstore

import (
	"crypto/rand"
	"path/filepath"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

func defaultMasterKeyProvider(storePath string, options Options) (masterKeyProvider, error) {
	if !options.empty() || options.LegacyReentryFileAuthority || storePath == "" || !filepath.IsAbs(storePath) || filepath.Clean(storePath) != storePath {
		return nil, portsecretstore.ErrInvalidRequest
	}
	return newFallbackMasterKeyProvider(
		filepath.Join(filepath.Dir(storePath), "master-key", "master.key"),
		rand.Reader,
	), nil
}
