//go:build !darwin && !linux && !windows

package finalauthority

import "errors"

func secureReadExistingPrivateNamedFile(string, string, int) ([]byte, existingFileAuthorityIdentity, error) {
	return nil, existingFileAuthorityIdentity{}, errors.New("existing private authority secure storage is unsupported")
}
