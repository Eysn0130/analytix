package persistencefs

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const startupJournalAuthoritySchemaVersion = 1

var (
	semanticJournalSignatureDomain   = []byte("analytix.semantic-startup-journal/v4\x00")
	semanticJournalV3SignatureDomain = []byte("analytix.semantic-startup-journal/v3\x00")
)

type startupJournalAuthorityKeyV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"keyId"`
	PublicKey     string `json:"publicKey"`
	PrivateSeed   string `json:"privateSeed"`
}

type startupJournalAuthority struct {
	keyID           string
	publicKey       ed25519.PublicKey
	privateKey      ed25519.PrivateKey
	namespace       *JournalNamespaceAuthority
	name            string
	keyFileIdentity string
	keyBodyDigest   string
}

func startupJournalAuthorityPath(journalRoot string) string {
	return filepath.Join(filepath.Dir(journalRoot), filepath.Base(journalRoot)+"-authority-v1.json")
}

func startupJournalAuthorityTempPrefix(journalRoot string) string {
	return "." + filepath.Base(startupJournalAuthorityPath(journalRoot)) + "-"
}

func loadStartupJournalAuthority(namespace *JournalNamespaceAuthority, journalRoot string) (*startupJournalAuthority, error) {
	if err := namespace.Validate(); err != nil {
		return nil, err
	}
	name := filepath.Base(startupJournalAuthorityPath(journalRoot))
	body, identity, err := secureStartupAuthorityRead(namespace.root, name, 4096)
	if err != nil {
		return nil, err
	}
	return parseStartupJournalAuthority(namespace, name, identity, body)
}

// openOrCreateStartupJournalAuthority is called only after the complete
// read-only semantic simulation succeeded and immediately before the first
// journal prepare. A pending journal is never allowed to bootstrap or rotate
// its own trust root.
func openOrCreateStartupJournalAuthority(namespace *JournalNamespaceAuthority, journalRoot string) (*startupJournalAuthority, error) {
	return openOrCreateStartupJournalAuthorityGuarded(namespace, journalRoot, nil)
}

func openOrCreateStartupJournalAuthorityGuarded(
	namespace *JournalNamespaceAuthority,
	journalRoot string,
	externalDependencyGuard func() error,
) (*startupJournalAuthority, error) {
	if _, err := loadStartupJournalAuthority(namespace, journalRoot); err == nil {
		if err := callStartupAuthorityExternalGuard(externalDependencyGuard); err != nil {
			return nil, err
		}
		if _, err := cleanBenignStartupAuthorityCreateResidue(namespace, journalRoot); err != nil {
			return nil, err
		}
		if err := callStartupAuthorityExternalGuard(externalDependencyGuard); err != nil {
			return nil, err
		}
		return loadStartupJournalAuthority(namespace, journalRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := callStartupAuthorityExternalGuard(externalDependencyGuard); err != nil {
		return nil, err
	}
	if err := validateStartupAuthorityBootstrapState(namespace); err != nil {
		return nil, err
	}
	discardedKeyIDs, err := cleanBenignStartupAuthorityCreateResidue(namespace, journalRoot)
	if err != nil {
		return nil, err
	}
	if err := callStartupAuthorityExternalGuard(externalDependencyGuard); err != nil {
		return nil, err
	}
	if err := validateStartupAuthorityBootstrapState(namespace); err != nil {
		return nil, err
	}
	body, err := newStartupJournalAuthorityBodyExcluding(discardedKeyIDs)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(startupJournalAuthorityPath(journalRoot))
	if err := secureStartupAuthorityWriteExclusive(namespace.root, name, body, 4096); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	authority, err := loadStartupJournalAuthority(namespace, journalRoot)
	if err != nil {
		return nil, err
	}
	if err := callStartupAuthorityExternalGuard(externalDependencyGuard); err != nil {
		return nil, err
	}
	return authority, nil
}

func callStartupAuthorityExternalGuard(guard func() error) error {
	if guard == nil {
		return nil
	}
	return guard()
}

func validateStartupAuthorityBootstrapState(namespace *JournalNamespaceAuthority) error {
	if namespace == nil || namespace.Validate() != nil {
		return errors.New("startup journal authority bootstrap namespace is invalid")
	}
	directory, err := secureStartupOpenRootDirectory(namespace.root)
	if err != nil {
		return err
	}
	defer directory.Close()
	entries, err := directory.ReadEntriesBounded(maxSemanticNamespaceEntries)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "journal" || name == privateCASRecoveryJournalDirectoryV1 ||
			strings.HasPrefix(name, ".retired-journal-") ||
			strings.HasPrefix(name, privateCASRecoveryRetiredPrefixV1) {
			return errors.New("startup journal authority cannot bootstrap beside authority-dependent state")
		}
	}
	return nil
}

func newStartupJournalAuthorityBody() ([]byte, error) {
	return newStartupJournalAuthorityBodyExcluding(nil)
}

func newStartupJournalAuthorityBodyExcluding(excluded map[string]struct{}) ([]byte, error) {
	for attempt := 0; attempt < 8; attempt++ {
		body, keyID, err := newStartupJournalAuthorityBodyOnce()
		if err != nil {
			return nil, err
		}
		if _, rejected := excluded[keyID]; !rejected {
			return body, nil
		}
	}
	return nil, errors.New("startup journal authority randomness repeated a discarded uncommitted key")
}

func newStartupJournalAuthorityBodyOnce() ([]byte, string, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}
	record := startupJournalAuthorityKeyV1{
		SchemaVersion: startupJournalAuthoritySchemaVersion,
		Algorithm:     "Ed25519",
		KeyID:         domainsecurity.SHA256Hex(publicKey),
		PublicKey:     base64.RawURLEncoding.EncodeToString(publicKey),
		PrivateSeed:   base64.RawURLEncoding.EncodeToString(privateKey.Seed()),
	}
	body, err := json.Marshal(record)
	return body, record.KeyID, err
}

func parseStartupJournalAuthority(namespace *JournalNamespaceAuthority, name, identity string, body []byte) (*startupJournalAuthority, error) {
	record, err := parseStartupJournalAuthorityBody(body)
	if err != nil {
		return nil, err
	}
	publicKey, _ := base64.RawURLEncoding.DecodeString(record.PublicKey)
	seed, _ := base64.RawURLEncoding.DecodeString(record.PrivateSeed)
	privateKey := ed25519.NewKeyFromSeed(seed)
	return &startupJournalAuthority{
		keyID: record.KeyID, publicKey: publicKey, privateKey: privateKey, namespace: namespace, name: name,
		keyFileIdentity: identity, keyBodyDigest: domainsecurity.SHA256Hex(body),
	}, nil
}

func validateStartupJournalAuthorityBody(body []byte) error {
	_, err := parseStartupJournalAuthorityBody(body)
	return err
}

func parseStartupJournalAuthorityBody(body []byte) (startupJournalAuthorityKeyV1, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var record startupJournalAuthorityKeyV1
	if err := decoder.Decode(&record); err != nil {
		return startupJournalAuthorityKeyV1{}, errors.New("startup journal authority key is invalid")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return startupJournalAuthorityKeyV1{}, errors.New("startup journal authority key has trailing JSON")
	}
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(record.PublicKey)
	seed, seedErr := base64.RawURLEncoding.DecodeString(record.PrivateSeed)
	if record.SchemaVersion != startupJournalAuthoritySchemaVersion || record.Algorithm != "Ed25519" ||
		publicErr != nil || seedErr != nil || len(publicKey) != ed25519.PublicKeySize || len(seed) != ed25519.SeedSize ||
		base64.RawURLEncoding.EncodeToString(publicKey) != record.PublicKey ||
		base64.RawURLEncoding.EncodeToString(seed) != record.PrivateSeed ||
		record.KeyID != domainsecurity.SHA256Hex(publicKey) {
		return startupJournalAuthorityKeyV1{}, errors.New("startup journal authority key material is invalid")
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	if !bytes.Equal(privateKey.Public().(ed25519.PublicKey), publicKey) {
		return startupJournalAuthorityKeyV1{}, errors.New("startup journal authority key pair does not match")
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(canonical, body) {
		return startupJournalAuthorityKeyV1{}, errors.New("startup journal authority key is not canonical")
	}
	return record, nil
}

func (authority *startupJournalAuthority) sign(message []byte) (string, error) {
	return authority.signDomain(semanticJournalSignatureDomain, message)
}

func (authority *startupJournalAuthority) signDomain(domain, message []byte) (string, error) {
	if authority == nil || len(authority.privateKey) != ed25519.PrivateKeySize || authority.revalidate() != nil {
		return "", errors.New("startup journal authority is unavailable")
	}
	signature := ed25519.Sign(authority.privateKey, startupAuthoritySignatureMessage(domain, message))
	return base64.RawURLEncoding.EncodeToString(signature), nil
}

func (authority *startupJournalAuthority) verify(keyID string, message []byte, encodedSignature string) error {
	return authority.verifyDomain(semanticJournalSignatureDomain, keyID, message, encodedSignature)
}

func (authority *startupJournalAuthority) verifyDomain(domain []byte, keyID string, message []byte, encodedSignature string) error {
	if authority == nil || authority.revalidate() != nil || keyID != authority.keyID || len(authority.publicKey) != ed25519.PublicKeySize {
		return errors.New("startup journal is not signed by this installation")
	}
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(authority.publicKey, startupAuthoritySignatureMessage(domain, message), signature) {
		return errors.New("startup journal trusted signature is invalid")
	}
	return nil
}

func startupJournalSignatureMessage(message []byte) []byte {
	return startupAuthoritySignatureMessage(semanticJournalSignatureDomain, message)
}

func startupAuthoritySignatureMessage(domain, message []byte) []byte {
	payload := make([]byte, 0, len(domain)+len(message))
	payload = append(payload, domain...)
	payload = append(payload, message...)
	return payload
}

func (authority *startupJournalAuthority) revalidate() error {
	if authority == nil || authority.namespace == nil {
		return errors.New("startup journal authority is unavailable")
	}
	body, identity, err := secureStartupAuthorityRead(authority.namespace.root, authority.name, 4096)
	if err != nil || identity != authority.keyFileIdentity || domainsecurity.SHA256Hex(body) != authority.keyBodyDigest {
		return errors.New("startup journal authority key identity changed")
	}
	return nil
}

func cleanBenignStartupAuthorityCreateResidue(
	namespace *JournalNamespaceAuthority,
	journalRoot string,
) (map[string]struct{}, error) {
	if err := namespace.Validate(); err != nil {
		return nil, err
	}
	targetAbsent := false
	if _, err := loadStartupJournalAuthority(namespace, journalRoot); errors.Is(err, os.ErrNotExist) {
		targetAbsent = true
	} else if err != nil {
		return nil, err
	}
	if targetAbsent {
		if err := validateStartupAuthorityBootstrapState(namespace); err != nil {
			return nil, err
		}
	}
	name := filepath.Base(startupJournalAuthorityPath(journalRoot))
	discardedKeyIDs := map[string]struct{}{}
	err := secureStartupAuthorityCleanCreateResidue(namespace.root, name, 4096, func(body []byte) error {
		record, err := parseStartupJournalAuthorityBody(body)
		if err == nil {
			discardedKeyIDs[record.KeyID] = struct{}{}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if targetAbsent {
		if err := validateStartupAuthorityBootstrapState(namespace); err != nil {
			return nil, err
		}
	}
	return discardedKeyIDs, nil
}

func preflightBenignStartupAuthorityCreateResidue(
	namespace *JournalNamespaceAuthority,
	journalRoot string,
) error {
	if err := namespace.Validate(); err != nil {
		return err
	}
	targetAbsent := false
	if _, err := loadStartupJournalAuthority(namespace, journalRoot); errors.Is(err, os.ErrNotExist) {
		targetAbsent = true
	} else if err != nil {
		return err
	}
	if targetAbsent {
		if err := validateStartupAuthorityBootstrapState(namespace); err != nil {
			return err
		}
	}
	name := filepath.Base(startupJournalAuthorityPath(journalRoot))
	if err := preflightStartupAuthorityCreateResidue(
		namespace.root, name, 4096, validateStartupJournalAuthorityBody,
	); err != nil {
		return err
	}
	if targetAbsent {
		return validateStartupAuthorityBootstrapState(namespace)
	}
	return nil
}

func startupAuthorityCreateTempName(target string, body []byte, nonce string) (string, error) {
	if !startupAuthorityNamedComponent(target) || len(body) == 0 || len(nonce) != 24 || !isLowerHex(nonce) {
		return "", errors.New("startup authority create temp input is invalid")
	}
	name := "." + target + "-" + domainsecurity.SHA256Hex(body) + "-" + nonce + ".tmp"
	if !startupAuthorityNamedComponent(name) {
		return "", errors.New("startup authority create temp name is invalid")
	}
	return name, nil
}

func validStartupAuthorityCreateTemp(name, target string, body []byte) bool {
	prefix := "." + target + "-"
	remainder := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".tmp")
	if len(remainder) != 64+1+24 || remainder[64] != '-' {
		return false
	}
	digest := remainder[:64]
	nonce := remainder[65:]
	return digest == domainsecurity.SHA256Hex(body) && isLowerHex(digest) && isLowerHex(nonce) &&
		name == prefix+digest+"-"+nonce+".tmp" && startupAuthorityNamedComponent(name)
}
