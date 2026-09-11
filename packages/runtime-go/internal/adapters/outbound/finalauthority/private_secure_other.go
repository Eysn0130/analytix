//go:build !darwin && !linux && !windows

package finalauthority

import "errors"

type privateRootAuthority struct{}

func newPrivateNamedRootAuthority(string, string, int) (privateRootAuthority, error) {
	return privateRootAuthority{}, errors.New("private named authority secure storage is unsupported")
}

func secureWritePrivateFileExclusive(privateRootAuthority, string, []byte) error {
	return errors.New("private authority secure storage is unsupported")
}

func secureReadPrivateFile(privateRootAuthority, string) ([]byte, error) {
	return nil, errors.New("private authority secure storage is unsupported")
}

func secureListPrivateFiles(privateRootAuthority) ([]securePrivateFile, error) {
	return nil, errors.New("private authority secure storage is unsupported")
}

func secureWritePrivateNamedFileExclusive(privateRootAuthority, string, []byte, int) error {
	return errors.New("private named authority secure storage is unsupported")
}

func secureReadPrivateNamedFile(privateRootAuthority, string, int) ([]byte, error) {
	return nil, errors.New("private named authority secure storage is unsupported")
}
