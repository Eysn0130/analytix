//go:build unix

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func validateRuntimeDarwinKeychainBindingFilesystemV1(
	binding runtimeDarwinSecretStoreKeychainBindingV1,
) error {
	for _, path := range []string{
		binding.IsolationRoot,
		binding.UserDataDir,
		binding.DataDir,
		filepath.Dir(binding.KeychainDBPath),
	} {
		identity, err := os.Lstat(path)
		if err != nil || !identity.IsDir() || identity.Mode()&os.ModeSymlink != 0 || identity.Mode().Perm()&0o077 != 0 {
			return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
		}
		stat, ok := identity.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) {
			return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil || real != path {
			return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
		}
	}
	identity, err := os.Lstat(binding.KeychainDBPath)
	if err != nil || !identity.Mode().IsRegular() || identity.Mode().Perm() != 0o600 || identity.Size() <= 0 {
		return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
	}
	stat, ok := identity.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
	}
	real, err := filepath.EvalSymlinks(binding.KeychainDBPath)
	if err != nil || real != binding.KeychainDBPath {
		return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
	}
	document := struct {
		SchemaVersion int    `json:"schemaVersion"`
		PathDigest    string `json:"pathDigest"`
		Device        string `json:"device"`
		Inode         string `json:"inode"`
		Owner         string `json:"owner"`
		Mode          string `json:"mode"`
		Links         string `json:"links"`
	}{
		1, runtimeSHA256HexBytesV1([]byte(binding.KeychainDBPath)),
		strconv.FormatUint(uint64(stat.Dev), 10), strconv.FormatUint(stat.Ino, 10),
		strconv.FormatUint(uint64(stat.Uid), 10), strconv.FormatUint(uint64(identity.Mode().Perm()), 8),
		strconv.FormatUint(uint64(stat.Nlink), 10),
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
	}
	digest := runtimeSHA256HexBytesV1(encoded)
	clear(encoded)
	if digest != binding.KeychainSecurityDigest {
		return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
	}
	return nil
}
