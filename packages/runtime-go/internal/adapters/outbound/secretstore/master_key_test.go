package secretstore

import (
	"bytes"
	"context"
	"errors"
	"testing"

	portsecretstore "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestLoadMasterKeyRejectsInvalidLengthAndRedactsProviderErrors(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		provider masterKeyProvider
	}{
		{name: "missing provider"},
		{name: "short key", provider: fixedMasterKeyProvider{key: bytes.Repeat([]byte{0x12}, masterKeySize-1)}},
		{
			name: "raw provider error",
			provider: &countingMasterKeyProvider{
				err: errors.New("synthetic-master-key path stdout stderr"),
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			key, err := loadMasterKey(context.Background(), testCase.provider)
			clearBytes(key)
			if !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
				t.Fatalf("loadMasterKey() error = %v, want unavailable", err)
			}
			assertRedactedError(t, err, "synthetic-master-key", "path", "stdout", "stderr")
		})
	}
}

func TestRandomMasterKeySourceRequiresFullEntropy(t *testing.T) {
	t.Parallel()

	key, err := (randomMasterKeySource{random: bytes.NewReader(bytes.Repeat([]byte{0x57}, masterKeySize))}).Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if len(key) != masterKeySize || !bytes.Equal(key, bytes.Repeat([]byte{0x57}, masterKeySize)) {
		t.Fatal("Generate() returned unexpected key bytes")
	}
	clearBytes(key)
	if _, err := (randomMasterKeySource{random: bytes.NewReader([]byte{0x01})}).Generate(); !errors.Is(err, portsecretstore.ErrMasterKeyUnavailable) {
		t.Fatalf("Generate(short entropy) error = %v, want unavailable", err)
	}
}
