//go:build !darwin

package nativecomponentregistry

import "os"

type PlatformVerifierConfig struct {
	WorkingDirectory          string
	WorkingDirectoryAuthority *os.File
	StagingRoot               string
	StagingRootAuthority      *os.File
	Policy                    PlatformPolicy
}

func NewPlatformVerifier(PlatformVerifierConfig) (PlatformVerifier, error) {
	return nil, ErrUnavailable
}
