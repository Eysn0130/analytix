//go:build darwin

package secretstore

import (
	"errors"
	"os"
	"path/filepath"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

// InspectLegacyKeychainProfile reads only owner-protected marker and envelope
// metadata. Neither this path nor the re-entry store invokes Security.framework
// or the security command.
func InspectLegacyKeychainProfile(storePath string) (*LegacyKeychainProfile, error) {
	if storePath == "" || !filepath.IsAbs(storePath) || filepath.Clean(storePath) != storePath {
		return nil, portsecretstore.ErrInvalidRequest
	}
	authorityPath := filepath.Join(filepath.Dir(storePath), "master-key", darwinAuthorityFile)
	marker, err := readPrivateCommittedFile(authorityPath, 64)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	defer clearBytes(marker)
	if string(marker) == string(darwinAuthorityFallback) {
		return nil, nil
	}
	if string(marker) != string(darwinAuthorityKeychain) || verifyLegacyKeychainMarker(storePath) != nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	// Inventory must not replay an old writer's ambiguous commit. Recovery of
	// these bytes is only allowed later inside a fenced replacement cleanup.
	journal, backup, committed := atomicRecoveryPaths(storePath)
	for _, path := range []string{journal, backup, committed, atomicRollbackRequiredPath(storePath)} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return nil, portsecretstore.ErrPersistence
		}
	}
	content, err := readPrivateCommittedFile(storePath, maxPersistentStoreBytes)
	if err != nil {
		return nil, portsecretstore.ErrPersistence
	}
	defer clearBytes(content)
	document, err := parsePersistentDocument(content)
	if err != nil {
		return nil, portsecretstore.ErrPersistence
	}
	records := make(map[portsecretstore.CredentialRef]persistentRecord, len(document.Records))
	for ref, record := range document.Records {
		records[portsecretstore.CredentialRef(ref)] = record
	}
	return &LegacyKeychainProfile{path: storePath, records: records}, nil
}

func verifyLegacyKeychainMarker(storePath string) error {
	directory := filepath.Join(filepath.Dir(storePath), "master-key")
	for _, incompatible := range []string{filepath.Join(directory, "master.key"), filepath.Join(directory, darwinExplicitBindingFile)} {
		if _, err := os.Lstat(incompatible); !errors.Is(err, os.ErrNotExist) {
			return portsecretstore.ErrMasterKeyUnavailable
		}
	}
	marker, err := readPrivateCommittedFile(filepath.Join(directory, darwinAuthorityFile), 64)
	if err != nil {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	defer clearBytes(marker)
	if string(marker) != string(darwinAuthorityKeychain) {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	return nil
}
