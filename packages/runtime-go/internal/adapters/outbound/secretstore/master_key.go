package secretstore

import (
	"context"
	"crypto/rand"
	"errors"
	"io"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

const masterKeySize = 32

var errOSCredentialUnavailable = errors.New("secret store: operating-system credential facility unavailable")

type masterKeyProvider interface {
	LoadOrCreate(context.Context) ([]byte, error)
}

type randomMasterKeySource struct {
	random io.Reader
}

func (source randomMasterKeySource) Generate() ([]byte, error) {
	random := source.random
	if random == nil {
		random = rand.Reader
	}
	key := make([]byte, masterKeySize)
	if _, err := io.ReadFull(random, key); err != nil {
		clearBytes(key)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return key, nil
}

func loadMasterKey(ctx context.Context, provider masterKeyProvider) ([]byte, error) {
	if provider == nil {
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	key, err := provider.LoadOrCreate(ctx)
	if err != nil || len(key) != masterKeySize {
		clearBytes(key)
		return nil, portsecretstore.ErrMasterKeyUnavailable
	}
	return key, nil
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
