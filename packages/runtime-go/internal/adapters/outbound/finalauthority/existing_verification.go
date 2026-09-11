package finalauthority

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
)

// ExistingFileVerificationV1 preserves the existing local-key trust model for
// read-only Core startup verification. It is not an independent installation
// anchor and cannot sign, create keys, or authorize optional-domain downgrade.
type ExistingFileVerificationV1 struct {
	root       *persistencefs.RootAuthority
	rootDigest string
	path       string
	identity   existingFileAuthorityIdentity
	keyID      string
	publicKey  []byte
}

func OpenExistingFileVerificationV1(root *persistencefs.RootAuthority) (*ExistingFileVerificationV1, error) {
	if root == nil || root.Validate() != nil || root.Digest() == "" {
		return nil, errors.New("existing key verification root is unavailable")
	}
	roots, ok := root.Roots()
	if !ok {
		return nil, errors.New("existing key verification roots are unavailable")
	}
	path := filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json")
	key, identity, err := readExistingFileAuthority(path)
	if err != nil {
		return nil, err
	}
	verification := &ExistingFileVerificationV1{root: root, rootDigest: root.Digest(), path: path, identity: identity, keyID: key.KeyID(), publicKey: key.PublicKey()}
	if err := verification.Revalidate(context.Background()); err != nil {
		return nil, err
	}
	return verification, nil
}

func (verification *ExistingFileVerificationV1) KeyID() string {
	if verification == nil {
		return ""
	}
	return verification.keyID
}

func (verification *ExistingFileVerificationV1) PublicKey() []byte {
	if verification == nil {
		return nil
	}
	return append([]byte(nil), verification.publicKey...)
}

func (*ExistingFileVerificationV1) Sign(context.Context, []byte) ([]byte, error) {
	return nil, errors.New("existing key verification cannot sign")
}

func (verification *ExistingFileVerificationV1) load(ctx context.Context) (*FileAuthority, error) {
	if verification == nil || ctx == nil || ctx.Err() != nil || verification.root == nil || verification.root.Validate() != nil || verification.root.Digest() != verification.rootDigest {
		return nil, errors.New("existing key verification root changed or context is unavailable")
	}
	roots, ok := verification.root.Roots()
	if !ok || verification.path != filepath.Join(roots.DataDir, "private", "authority", "final-answer-ed25519-v1.json") {
		return nil, errors.New("existing key verification path changed")
	}
	key, identity, err := readExistingFileAuthority(verification.path)
	if err != nil {
		return nil, err
	}
	if identity != verification.identity || key.KeyID() != verification.keyID || !bytes.Equal(key.PublicKey(), verification.publicKey) {
		return nil, errors.New("existing key verification identity changed")
	}
	if err := errors.Join(verification.root.Validate(), ctx.Err()); err != nil {
		return nil, err
	}
	return key, nil
}

func (verification *ExistingFileVerificationV1) Revalidate(ctx context.Context) error {
	_, err := verification.load(ctx)
	return err
}

func (verification *ExistingFileVerificationV1) VerifyTrusted(ctx context.Context, keyID string, publicKey, message, signature []byte) error {
	key, err := verification.load(ctx)
	if err != nil {
		return err
	}
	return key.VerifyTrusted(ctx, keyID, publicKey, message, signature)
}
