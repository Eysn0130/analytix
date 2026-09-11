//go:build windows

package main

import "errors"

func validateRuntimeDarwinKeychainBindingFilesystemV1(
	runtimeDarwinSecretStoreKeychainBindingV1,
) error {
	return errors.New("runtime Darwin Secret Store task Keychain binding is unavailable")
}
