//go:build darwin

package secretstore

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

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
	bindingPath := filepath.Join(filepath.Dir(storePath), "master-key", darwinExplicitBindingFile)
	marker, err := readPrivateCommittedFile(authorityPath, 64)
	if errors.Is(err, os.ErrNotExist) {
		if _, bindingErr := os.Lstat(bindingPath); errors.Is(bindingErr, os.ErrNotExist) {
			return nil, nil
		} else if bindingErr != nil {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	defer clearBytes(marker)
	if string(marker) == string(darwinAuthorityFallback) {
		if _, bindingErr := os.Lstat(bindingPath); !errors.Is(bindingErr, os.ErrNotExist) {
			return nil, portsecretstore.ErrMasterKeyUnavailable
		}
		return nil, nil
	}
	if (err == nil && string(marker) != string(darwinAuthorityKeychain)) || verifyLegacyKeychainMarker(storePath) != nil {
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
	if _, err := os.Lstat(filepath.Join(directory, "master.key")); !errors.Is(err, os.ErrNotExist) {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	marker, err := readPrivateCommittedFile(filepath.Join(directory, darwinAuthorityFile), 64)
	if errors.Is(err, os.ErrNotExist) {
		// Older isolated packages recorded an explicit task Keychain binding
		// without authority.v1. Validate only its owner-protected metadata;
		// re-entry never opens that Keychain or reads its master key.
		binding, err := readExplicitDarwinBinding(filepath.Join(directory, darwinExplicitBindingFile))
		if err != nil || !validLegacyExplicitBinding(binding) {
			return portsecretstore.ErrMasterKeyUnavailable
		}
		return nil
	}
	if err != nil {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	defer clearBytes(marker)
	if string(marker) != string(darwinAuthorityKeychain) {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	if _, err := os.Lstat(filepath.Join(directory, darwinExplicitBindingFile)); !errors.Is(err, os.ErrNotExist) {
		return portsecretstore.ErrMasterKeyUnavailable
	}
	return nil
}

func validLegacyExplicitBinding(binding darwinExplicitKeychainBindingV2) bool {
	security := binding.Security
	if security.SchemaVersion != 1 || !isSHA256Hex(security.PathDigest) ||
		security.Mode != "600" || security.Links != "1" ||
		security.Owner != strconv.FormatUint(uint64(os.Geteuid()), 10) {
		return false
	}
	for _, value := range []string{security.Device, security.Inode, security.Owner} {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || strconv.FormatUint(parsed, 10) != value {
			return false
		}
	}
	return true
}
