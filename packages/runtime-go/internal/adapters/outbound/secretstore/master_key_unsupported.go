//go:build !darwin && !linux && !windows

package secretstore

import (
	"context"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

type unsupportedMasterKeyProvider struct{}

func (unsupportedMasterKeyProvider) LoadOrCreate(context.Context) ([]byte, error) {
	return nil, portsecretstore.ErrMasterKeyUnavailable
}

func defaultMasterKeyProvider(_ string, options Options) (masterKeyProvider, error) {
	if !options.empty() {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return unsupportedMasterKeyProvider{}, nil
}
