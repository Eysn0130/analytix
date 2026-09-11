package finalauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	persistencefs "analytix.local/runtime-go/internal/adapters/outbound/persistencefs"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	fileAuthorityKeyVersion = 1
	maxAuthorityKeyBytes    = 4096
)

type fileAuthorityKey struct {
	SchemaVersion int    `json:"schemaVersion"`
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"keyId"`
	PublicKey     string `json:"publicKey"`
	PrivateSeed   string `json:"privateSeed"`
}

type FileAuthority struct {
	keyID      string
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

type existingFileAuthorityIdentity struct {
	rootVolume     uint64
	rootObjectHigh uint64
	rootObjectLow  uint64
	rootFileID     [16]byte
	fileVolume     uint64
	fileObjectHigh uint64
	fileObjectLow  uint64
	fileFileID     [16]byte
	fileSize       uint64
}

// ExistingFileAuthorityAnchor binds an independently enrolled installation
// key to the startup-frozen data root. It cannot bootstrap or repair either
// authority.
type ExistingFileAuthorityAnchor struct {
	rootAuthority     *persistencefs.RootAuthority
	rootAuthorityID   string
	dataRoot          string
	expectedKeyID     string
	expectedPublicKey ed25519.PublicKey
}

// AnchoredFileAuthority is the independently anchored existing-key authority. Every
// cryptographic operation revalidates its frozen root, exact key, and the
// directory/file identity observed when it was opened.
type AnchoredFileAuthority struct {
	anchor   *ExistingFileAuthorityAnchor
	path     string
	identity existingFileAuthorityIdentity
}

func NewExistingFileAuthorityAnchor(
	rootAuthority *persistencefs.RootAuthority,
	expectedKeyID string,
	expectedPublicKey []byte,
) (*ExistingFileAuthorityAnchor, error) {
	expectedPublicKey = append([]byte(nil), expectedPublicKey...)
	if rootAuthority == nil || rootAuthority.Validate() != nil || len(expectedPublicKey) != ed25519.PublicKeySize ||
		expectedKeyID != strings.TrimSpace(expectedKeyID) || expectedKeyID != domainsecurity.SHA256Hex(expectedPublicKey) {
		return nil, errors.New("existing final authority independent anchor is invalid")
	}
	roots, ok := rootAuthority.Roots()
	rootAuthorityID := rootAuthority.Digest()
	if !ok || rootAuthorityID == "" || roots.DataDir == "" || !filepath.IsAbs(roots.DataDir) || filepath.Clean(roots.DataDir) != roots.DataDir {
		return nil, errors.New("existing final authority frozen data root is invalid")
	}
	return &ExistingFileAuthorityAnchor{
		rootAuthority: rootAuthority, rootAuthorityID: rootAuthorityID, dataRoot: roots.DataDir,
		expectedKeyID: expectedKeyID, expectedPublicKey: ed25519.PublicKey(expectedPublicKey),
	}, nil
}

func (anchor *ExistingFileAuthorityAnchor) Open(path string) (*AnchoredFileAuthority, error) {
	_, identity, err := anchor.load(path)
	if err != nil {
		return nil, err
	}
	return &AnchoredFileAuthority{anchor: anchor, path: path, identity: identity}, nil
}

func (anchor *ExistingFileAuthorityAnchor) load(path string) (*FileAuthority, existingFileAuthorityIdentity, error) {
	if err := anchor.validate(path); err != nil {
		return nil, existingFileAuthorityIdentity{}, err
	}
	authority, identity, err := readExistingFileAuthority(path)
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, err
	}
	if authority.KeyID() != anchor.expectedKeyID || !bytes.Equal(authority.PublicKey(), anchor.expectedPublicKey) {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing final authority key does not match the independent anchor")
	}
	if err := anchor.validate(path); err != nil {
		return nil, existingFileAuthorityIdentity{}, errors.New("existing final authority root changed during load")
	}
	return authority, identity, nil
}

func (anchor *ExistingFileAuthorityAnchor) validate(path string) error {
	if anchor == nil || anchor.rootAuthority == nil || anchor.rootAuthorityID == "" ||
		len(anchor.expectedPublicKey) != ed25519.PublicKeySize || anchor.expectedKeyID != domainsecurity.SHA256Hex(anchor.expectedPublicKey) ||
		path == "" || path != strings.TrimSpace(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("existing final authority capability is invalid")
	}
	roots, ok := anchor.rootAuthority.Roots()
	if !ok || roots.DataDir != anchor.dataRoot || anchor.rootAuthority.Digest() != anchor.rootAuthorityID || anchor.rootAuthority.Validate() != nil {
		return errors.New("existing final authority frozen root no longer matches")
	}
	relative, err := filepath.Rel(anchor.dataRoot, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("existing final authority path is outside the frozen data root")
	}
	return nil
}

// readExistingFileAuthority is intentionally package-private: self-consistent
// key bytes alone are not an installation trust anchor.
func readExistingFileAuthority(path string) (*FileAuthority, existingFileAuthorityIdentity, error) {
	if path == "" || path != strings.TrimSpace(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, existingFileAuthorityIdentity{}, errors.New("final authority key path is not canonical")
	}
	body, identity, err := secureReadExistingPrivateNamedFile(filepath.Dir(path), filepath.Base(path), maxAuthorityKeyBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, existingFileAuthorityIdentity{}, errors.New("final authority key is not enrolled")
	}
	if err != nil {
		return nil, existingFileAuthorityIdentity{}, err
	}
	authority, err := loadFileAuthority(body)
	return authority, identity, err
}

func openExistingFileAuthority(path string) (*FileAuthority, error) {
	authority, _, err := readExistingFileAuthority(path)
	return authority, err
}

func OpenOrCreateFileAuthority(path string, authorityStateExists bool) (*FileAuthority, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("final authority key path is required")
	}
	root, err := newPrivateNamedRootAuthority(filepath.Dir(path), filepath.Base(path), maxAuthorityKeyBytes)
	if err != nil {
		return nil, err
	}
	body, err := secureReadPrivateNamedFile(root, filepath.Base(path), maxAuthorityKeyBytes)
	if errors.Is(err, os.ErrNotExist) {
		if authorityStateExists {
			return nil, errors.New("final authority key is missing while authority records exist")
		}
		body, err = newAuthorityKeyBody()
		if err != nil {
			return nil, err
		}
		if err := secureWritePrivateNamedFileExclusive(root, filepath.Base(path), body, maxAuthorityKeyBytes); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		body, err = secureReadPrivateNamedFile(root, filepath.Base(path), maxAuthorityKeyBytes)
	}
	if err != nil {
		return nil, err
	}
	return loadFileAuthority(body)
}

func (authority *FileAuthority) KeyID() string {
	if authority == nil {
		return ""
	}
	return authority.keyID
}

func (authority *FileAuthority) PublicKey() []byte {
	if authority == nil {
		return nil
	}
	return append([]byte(nil), authority.publicKey...)
}

func (authority *FileAuthority) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if authority == nil || len(authority.privateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("final authority is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return ed25519.Sign(authority.privateKey, append([]byte(nil), message...)), nil
}

func (authority *FileAuthority) VerifyTrusted(ctx context.Context, keyID string, publicKey, message, signature []byte) error {
	if authority == nil || keyID != authority.keyID || !bytes.Equal(publicKey, authority.publicKey) ||
		len(signature) != ed25519.SignatureSize {
		return errors.New("accepted final key is not trusted by this installation")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if !ed25519.Verify(authority.publicKey, message, signature) {
		return errors.New("accepted final trusted signature is invalid")
	}
	return nil
}

func (authority *AnchoredFileAuthority) KeyID() string {
	if authority == nil || authority.anchor == nil {
		return ""
	}
	return authority.anchor.expectedKeyID
}

func (authority *AnchoredFileAuthority) PublicKey() []byte {
	if authority == nil || authority.anchor == nil {
		return nil
	}
	return append([]byte(nil), authority.anchor.expectedPublicKey...)
}

func (authority *AnchoredFileAuthority) Sign(ctx context.Context, message []byte) ([]byte, error) {
	loaded, err := authority.revalidate()
	if err != nil {
		return nil, err
	}
	return loaded.Sign(ctx, message)
}

func (authority *AnchoredFileAuthority) VerifyTrusted(ctx context.Context, keyID string, publicKey, message, signature []byte) error {
	loaded, err := authority.revalidate()
	if err != nil {
		return err
	}
	return loaded.VerifyTrusted(ctx, keyID, publicKey, message, signature)
}

func (authority *AnchoredFileAuthority) revalidate() (*FileAuthority, error) {
	if authority == nil || authority.anchor == nil || authority.path == "" {
		return nil, errors.New("anchored final authority is unavailable")
	}
	loaded, identity, err := authority.anchor.load(authority.path)
	if err != nil {
		return nil, err
	}
	if identity != authority.identity {
		return nil, errors.New("existing final authority path identity changed after enrollment")
	}
	return loaded, nil
}

// ValidateCurrentInstallation never classifies a failure as a record fault.
func (authority *AnchoredFileAuthority) ValidateCurrentInstallation(ctx context.Context) error {
	if ctx == nil {
		return errors.New("installation validation context is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := authority.revalidate()
	return errors.Join(err, ctx.Err())
}

func newAuthorityKeyBody() ([]byte, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	seed := privateKey.Seed()
	record := fileAuthorityKey{
		SchemaVersion: fileAuthorityKeyVersion, Algorithm: "Ed25519", KeyID: domainsecurity.SHA256Hex(publicKey),
		PublicKey: base64.RawURLEncoding.EncodeToString(publicKey), PrivateSeed: base64.RawURLEncoding.EncodeToString(seed),
	}
	body, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func loadFileAuthority(body []byte) (*FileAuthority, error) {
	if len(body) == 0 || len(body) > maxAuthorityKeyBytes {
		return nil, errors.New("final authority key file size is invalid")
	}
	if err := domainjsonstrict.Validate(body, domainjsonstrict.Options{
		RequireObject:  true,
		MaxBytes:       maxAuthorityKeyBytes,
		MaxDepth:       2,
		MaxTokens:      16,
		MaxStringBytes: maxAuthorityKeyBytes,
	}); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var record fileAuthorityKey
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("final authority key contains trailing JSON")
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(record.PublicKey)
	seed, seedErr := base64.RawURLEncoding.DecodeString(record.PrivateSeed)
	if record.SchemaVersion != fileAuthorityKeyVersion || record.Algorithm != "Ed25519" || publicErr != nil || seedErr != nil ||
		len(publicKey) != ed25519.PublicKeySize || len(seed) != ed25519.SeedSize || !domainsecurity.IsSHA256Hex(record.KeyID) ||
		base64.RawURLEncoding.EncodeToString(publicKey) != record.PublicKey ||
		base64.RawURLEncoding.EncodeToString(seed) != record.PrivateSeed || domainsecurity.SHA256Hex(publicKey) != record.KeyID {
		return nil, errors.New("final authority key material is invalid")
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	if !bytes.Equal(privateKey.Public().(ed25519.PublicKey), publicKey) {
		return nil, errors.New("final authority public and private key material do not match")
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(canonical, body) {
		return nil, errors.New("final authority key encoding is not canonical")
	}
	return &FileAuthority{keyID: record.KeyID, publicKey: ed25519.PublicKey(publicKey), privateKey: privateKey}, nil
}
