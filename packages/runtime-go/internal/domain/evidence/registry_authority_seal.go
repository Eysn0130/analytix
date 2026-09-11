package evidence

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	EvidenceRegistryAuthoritySealVersion    = 2
	EvidenceRegistryAuthorityPurpose        = "analytix.evidence-registry-head/v2"
	EvidenceRegistryAuthorityCapsuleVersion = 1
	EvidenceRegistryAuthorityCapsulePurpose = "analytix.evidence-registry-capsule/v1"
	EvidenceRegistryAuthorityIndexVersion   = 1
	EvidenceRegistryAuthorityIndexPurpose   = "analytix.evidence-registry-index/v1"
)

var evidenceRegistryAuthoritySignatureDomain = []byte("analytix.evidence-registry-head-authority/v2\x00")
var evidenceRegistryAuthorityIndexSignatureDomain = []byte("analytix.evidence-registry-index-authority/v1\x00")

type EvidenceRegistryAuthoritySignFunc func([]byte) ([]byte, error)

// EvidenceRegistryAuthoritySeal is the independently signed current head of
// one turn-scoped registry. The JSONL hash chain detects in-place corruption;
// this seal additionally makes a still-valid shorter prefix fail closed after
// tail deletion (including removal of a revocation entry).
type EvidenceRegistryAuthoritySeal struct {
	SchemaVersion             int    `json:"schemaVersion"`
	AuthorityPurpose          string `json:"authorityPurpose"`
	AuthorityAlgorithm        string `json:"authorityAlgorithm"`
	AuthorityKeyID            string `json:"authorityKeyId"`
	AuthorityPublicKey        string `json:"authorityPublicKey"`
	ThreadID                  string `json:"threadId"`
	TurnID                    string `json:"turnId"`
	ContextDigest             string `json:"contextDigest"`
	DatasetSnapshotID         string `json:"datasetSnapshotId"`
	Sequence                  uint64 `json:"sequence"`
	StateDigest               string `json:"stateDigest"`
	LastEntryDigest           string `json:"lastEntryDigest"`
	CanonicalLedgerSHA256     string `json:"canonicalLedgerSha256"`
	CanonicalLedgerByteLength uint64 `json:"canonicalLedgerByteLength"`
	AuthoritySignature        string `json:"authoritySignature"`
	RecordDigest              string `json:"recordDigest"`
}

func NewEvidenceRegistryAuthoritySeal(registry EvidenceReceiptRegistry, keyID string, publicKey []byte, sign EvidenceRegistryAuthoritySignFunc) (EvidenceRegistryAuthoritySeal, error) {
	if ValidateEvidenceReceiptRegistry(registry) != nil || registry.Sequence == 0 || len(registry.Entries) == 0 || sign == nil ||
		!domainsecurity.IsSHA256Hex(strings.TrimSpace(keyID)) || len(publicKey) != ed25519.PublicKeySize ||
		domainsecurity.SHA256Hex(publicKey) != strings.TrimSpace(keyID) {
		return EvidenceRegistryAuthoritySeal{}, errors.New("evidence registry authority seal input is invalid")
	}
	ledger, err := CanonicalEvidenceRegistryLedger(registry)
	if err != nil {
		return EvidenceRegistryAuthoritySeal{}, err
	}
	seal := EvidenceRegistryAuthoritySeal{
		SchemaVersion: EvidenceRegistryAuthoritySealVersion, AuthorityPurpose: EvidenceRegistryAuthorityPurpose,
		AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm, AuthorityKeyID: strings.TrimSpace(keyID),
		AuthorityPublicKey: base64.RawURLEncoding.EncodeToString(publicKey), ThreadID: registry.ThreadID, TurnID: registry.TurnID,
		ContextDigest: registry.ContextDigest, DatasetSnapshotID: registry.DatasetSnapshotID, Sequence: registry.Sequence,
		StateDigest: registry.StateDigest, LastEntryDigest: registry.Entries[len(registry.Entries)-1].EntryDigest,
		CanonicalLedgerSHA256: domainsecurity.SHA256Hex(ledger), CanonicalLedgerByteLength: uint64(len(ledger)),
	}
	signature, err := sign(EvidenceRegistryAuthoritySealSigningBytes(seal))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return EvidenceRegistryAuthoritySeal{}, errors.New("evidence registry authority seal signing failed")
	}
	seal.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	seal.RecordDigest = evidenceRegistryAuthoritySealDigest(seal)
	if err := ValidateEvidenceRegistryAuthoritySeal(seal); err != nil {
		return EvidenceRegistryAuthoritySeal{}, err
	}
	return seal, nil
}

func ParseEvidenceRegistryAuthoritySeal(raw []byte) (EvidenceRegistryAuthoritySeal, error) {
	var seal EvidenceRegistryAuthoritySeal
	if err := decodeStrictJSON(raw, &seal, "evidence registry authority seal"); err != nil {
		return EvidenceRegistryAuthoritySeal{}, err
	}
	canonical, err := json.Marshal(seal)
	if err != nil || !bytes.Equal(raw, canonical) {
		return EvidenceRegistryAuthoritySeal{}, errors.New("evidence registry authority seal is not canonically encoded")
	}
	return seal, ValidateEvidenceRegistryAuthoritySeal(seal)
}

func ValidateEvidenceRegistryAuthoritySeal(seal EvidenceRegistryAuthoritySeal) error {
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(seal.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(seal.AuthoritySignature)
	if seal.SchemaVersion != EvidenceRegistryAuthoritySealVersion || seal.AuthorityPurpose != EvidenceRegistryAuthorityPurpose ||
		seal.AuthorityAlgorithm != AcceptedFinalAuthorityAlgorithm || !domainsecurity.IsSHA256Hex(seal.AuthorityKeyID) ||
		publicErr != nil || len(publicKey) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(publicKey) != seal.AuthorityKeyID ||
		signatureErr != nil || len(signature) != ed25519.SignatureSize || strings.TrimSpace(seal.ThreadID) == "" ||
		strings.TrimSpace(seal.TurnID) == "" || !validSHA256(seal.ContextDigest) || strings.TrimSpace(seal.DatasetSnapshotID) == "" ||
		seal.Sequence == 0 || !validSHA256(seal.StateDigest) || !validSHA256(seal.LastEntryDigest) ||
		!validSHA256(seal.CanonicalLedgerSHA256) || seal.CanonicalLedgerByteLength == 0 || !validSHA256(seal.RecordDigest) {
		return errors.New("evidence registry authority seal is incomplete")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), EvidenceRegistryAuthoritySealSigningBytes(seal), signature) {
		return errors.New("evidence registry authority seal signature is invalid")
	}
	if evidenceRegistryAuthoritySealDigest(seal) != seal.RecordDigest {
		return errors.New("evidence registry authority seal integrity is invalid")
	}
	return nil
}

func EvidenceRegistryAuthoritySealMatchesRegistry(seal EvidenceRegistryAuthoritySeal, registry EvidenceReceiptRegistry) bool {
	ledger, ledgerErr := CanonicalEvidenceRegistryLedger(registry)
	return ValidateEvidenceRegistryAuthoritySeal(seal) == nil && ValidateEvidenceReceiptRegistry(registry) == nil && registry.Sequence > 0 &&
		ledgerErr == nil && seal.CanonicalLedgerSHA256 == domainsecurity.SHA256Hex(ledger) && seal.CanonicalLedgerByteLength == uint64(len(ledger)) &&
		seal.ThreadID == registry.ThreadID && seal.TurnID == registry.TurnID && seal.ContextDigest == registry.ContextDigest &&
		seal.DatasetSnapshotID == registry.DatasetSnapshotID && seal.Sequence == registry.Sequence && seal.StateDigest == registry.StateDigest &&
		seal.LastEntryDigest == registry.Entries[len(registry.Entries)-1].EntryDigest
}

type EvidenceRegistryAuthorityCapsule struct {
	SchemaVersion             int                                `json:"schemaVersion"`
	Purpose                   string                             `json:"purpose"`
	SecurityContext           domainsecurity.TurnSecurityContext `json:"securityContext"`
	Registry                  EvidenceReceiptRegistry            `json:"registry"`
	Seal                      EvidenceRegistryAuthoritySeal      `json:"seal"`
	CanonicalLedgerSHA256     string                             `json:"canonicalLedgerSha256"`
	CanonicalLedgerByteLength uint64                             `json:"canonicalLedgerByteLength"`
	RecordDigest              string                             `json:"recordDigest"`
}

func NewEvidenceRegistryAuthorityCapsule(context domainsecurity.TurnSecurityContext, registry EvidenceReceiptRegistry, keyID string, publicKey []byte, sign EvidenceRegistryAuthoritySignFunc) (EvidenceRegistryAuthorityCapsule, error) {
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		!EvidenceReceiptRegistryMatchesContext(registry, context) || registry.Sequence == 0 {
		return EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule context is invalid")
	}
	seal, err := NewEvidenceRegistryAuthoritySeal(registry, keyID, publicKey, sign)
	if err != nil {
		return EvidenceRegistryAuthorityCapsule{}, err
	}
	capsule := EvidenceRegistryAuthorityCapsule{
		SchemaVersion: EvidenceRegistryAuthorityCapsuleVersion, Purpose: EvidenceRegistryAuthorityCapsulePurpose,
		SecurityContext: context, Registry: registry, Seal: seal,
		CanonicalLedgerSHA256: seal.CanonicalLedgerSHA256, CanonicalLedgerByteLength: seal.CanonicalLedgerByteLength,
	}
	capsule.RecordDigest = evidenceRegistryAuthorityCapsuleDigest(capsule)
	if err := ValidateEvidenceRegistryAuthorityCapsule(capsule); err != nil {
		return EvidenceRegistryAuthorityCapsule{}, err
	}
	return capsule, nil
}

func ParseEvidenceRegistryAuthorityCapsule(raw []byte) (EvidenceRegistryAuthorityCapsule, error) {
	var capsule EvidenceRegistryAuthorityCapsule
	if err := decodeStrictJSON(raw, &capsule, "evidence registry authority capsule"); err != nil {
		return EvidenceRegistryAuthorityCapsule{}, err
	}
	canonical, err := json.Marshal(capsule)
	if err != nil || !bytes.Equal(raw, canonical) {
		return EvidenceRegistryAuthorityCapsule{}, errors.New("evidence registry authority capsule is not canonically encoded")
	}
	return capsule, ValidateEvidenceRegistryAuthorityCapsule(capsule)
}

func ValidateEvidenceRegistryAuthorityCapsule(capsule EvidenceRegistryAuthorityCapsule) error {
	ledger, ledgerErr := CanonicalEvidenceRegistryLedger(capsule.Registry)
	if capsule.SchemaVersion != EvidenceRegistryAuthorityCapsuleVersion || capsule.Purpose != EvidenceRegistryAuthorityCapsulePurpose ||
		domainsecurity.ValidateTurnSecurityContext(capsule.SecurityContext) != nil ||
		!EvidenceReceiptRegistryMatchesContext(capsule.Registry, capsule.SecurityContext) || capsule.Registry.Sequence == 0 ||
		ledgerErr != nil || !EvidenceRegistryAuthoritySealMatchesRegistry(capsule.Seal, capsule.Registry) ||
		capsule.CanonicalLedgerSHA256 != domainsecurity.SHA256Hex(ledger) || capsule.CanonicalLedgerSHA256 != capsule.Seal.CanonicalLedgerSHA256 ||
		capsule.CanonicalLedgerByteLength != uint64(len(ledger)) || capsule.CanonicalLedgerByteLength != capsule.Seal.CanonicalLedgerByteLength ||
		!validSHA256(capsule.RecordDigest) || capsule.RecordDigest != evidenceRegistryAuthorityCapsuleDigest(capsule) {
		return errors.New("evidence registry authority capsule is invalid")
	}
	return nil
}

func CanonicalEvidenceRegistryLedger(registry EvidenceReceiptRegistry) ([]byte, error) {
	if ValidateEvidenceReceiptRegistry(registry) != nil || registry.Sequence == 0 || len(registry.Entries) == 0 {
		return nil, errors.New("evidence registry canonical ledger is invalid")
	}
	ledger := make([]byte, 0)
	for _, entry := range registry.Entries {
		body, err := json.Marshal(entry)
		if err != nil {
			return nil, errors.New("evidence registry entry cannot be encoded")
		}
		ledger = append(ledger, body...)
		ledger = append(ledger, '\n')
	}
	return ledger, nil
}

func EvidenceRegistryAuthoritySealSigningBytes(seal EvidenceRegistryAuthoritySeal) []byte {
	seal.AuthoritySignature = ""
	seal.RecordDigest = ""
	body, _ := json.Marshal(seal)
	return append(append([]byte(nil), evidenceRegistryAuthoritySignatureDomain...), body...)
}

func evidenceRegistryAuthoritySealDigest(seal EvidenceRegistryAuthoritySeal) string {
	seal.RecordDigest = ""
	body, _ := json.Marshal(seal)
	return domainsecurity.SHA256Hex(body)
}

func evidenceRegistryAuthorityCapsuleDigest(capsule EvidenceRegistryAuthorityCapsule) string {
	capsule.RecordDigest = ""
	body, _ := json.Marshal(capsule)
	return domainsecurity.SHA256Hex(body)
}

type EvidenceRegistryAuthorityIndexEntry struct {
	ThreadID                  string `json:"threadId"`
	TurnID                    string `json:"turnId"`
	ContextDigest             string `json:"contextDigest"`
	ProjectionKey             string `json:"projectionKey"`
	CapsuleSHA256             string `json:"capsuleSha256"`
	CapsuleByteLength         uint64 `json:"capsuleByteLength"`
	CapsuleRecordDigest       string `json:"capsuleRecordDigest"`
	RegistrySequence          uint64 `json:"registrySequence"`
	RegistryStateDigest       string `json:"registryStateDigest"`
	LastEntryDigest           string `json:"lastEntryDigest"`
	CanonicalLedgerSHA256     string `json:"canonicalLedgerSha256"`
	CanonicalLedgerByteLength uint64 `json:"canonicalLedgerByteLength"`
}

type EvidenceRegistryAuthorityIndex struct {
	SchemaVersion       int                                   `json:"schemaVersion"`
	Purpose             string                                `json:"purpose"`
	InstallationID      string                                `json:"installationId"`
	Generation          uint64                                `json:"generation"`
	PreviousIndexDigest string                                `json:"previousIndexDigest"`
	Entries             []EvidenceRegistryAuthorityIndexEntry `json:"entries"`
	AuthorityAlgorithm  string                                `json:"authorityAlgorithm"`
	AuthorityKeyID      string                                `json:"authorityKeyId"`
	AuthorityPublicKey  string                                `json:"authorityPublicKey"`
	AuthoritySignature  string                                `json:"authoritySignature"`
	RecordDigest        string                                `json:"recordDigest"`
}

func NewEvidenceRegistryAuthorityIndex(previous *EvidenceRegistryAuthorityIndex, capsule EvidenceRegistryAuthorityCapsule, keyID string, publicKey []byte, sign EvidenceRegistryAuthoritySignFunc) (EvidenceRegistryAuthorityIndex, error) {
	keyID = strings.TrimSpace(keyID)
	publicKeyEncoded := base64.RawURLEncoding.EncodeToString(publicKey)
	if ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil || sign == nil || !domainsecurity.IsSHA256Hex(keyID) ||
		len(publicKey) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(publicKey) != keyID ||
		capsule.Seal.AuthorityKeyID != keyID || capsule.Seal.AuthorityPublicKey != publicKeyEncoded {
		return EvidenceRegistryAuthorityIndex{}, errors.New("evidence registry authority index input is invalid")
	}
	replacement, err := NewEvidenceRegistryAuthorityIndexEntry(capsule)
	if err != nil {
		return EvidenceRegistryAuthorityIndex{}, err
	}
	index := EvidenceRegistryAuthorityIndex{
		SchemaVersion: EvidenceRegistryAuthorityIndexVersion, Purpose: EvidenceRegistryAuthorityIndexPurpose, InstallationID: keyID,
		Generation: 1, PreviousIndexDigest: EvidenceRegistryAuthorityIndexGenesisDigest(),
		Entries: []EvidenceRegistryAuthorityIndexEntry{}, AuthorityAlgorithm: AcceptedFinalAuthorityAlgorithm,
		AuthorityKeyID: keyID, AuthorityPublicKey: publicKeyEncoded,
	}
	if previous != nil {
		if ValidateEvidenceRegistryAuthorityIndex(*previous) != nil || previous.InstallationID != index.InstallationID ||
			previous.AuthorityKeyID != index.AuthorityKeyID || previous.AuthorityPublicKey != index.AuthorityPublicKey || previous.Generation == ^uint64(0) {
			return EvidenceRegistryAuthorityIndex{}, errors.New("previous evidence registry authority index is invalid")
		}
		index.Generation = previous.Generation + 1
		index.PreviousIndexDigest = previous.RecordDigest
		index.Entries = append(index.Entries, previous.Entries...)
	}
	replaced := false
	for position, entry := range index.Entries {
		if entry.ThreadID != replacement.ThreadID || entry.TurnID != replacement.TurnID {
			continue
		}
		if err := validateEvidenceRegistryAuthorityIndexEntryExtension(entry, replacement, capsule); err != nil {
			return EvidenceRegistryAuthorityIndex{}, errors.New("evidence registry authority index replacement is not monotonic")
		}
		index.Entries[position] = replacement
		replaced = true
		break
	}
	if !replaced {
		if replacement.RegistrySequence != 1 {
			return EvidenceRegistryAuthorityIndex{}, errors.New("new evidence registry authority index entry must start at sequence one")
		}
		index.Entries = append(index.Entries, replacement)
	}
	sort.Slice(index.Entries, func(i, j int) bool {
		if index.Entries[i].ThreadID != index.Entries[j].ThreadID {
			return index.Entries[i].ThreadID < index.Entries[j].ThreadID
		}
		return index.Entries[i].TurnID < index.Entries[j].TurnID
	})
	signature, err := sign(EvidenceRegistryAuthorityIndexSigningBytes(index))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return EvidenceRegistryAuthorityIndex{}, errors.New("evidence registry authority index signing failed")
	}
	index.AuthoritySignature = base64.RawURLEncoding.EncodeToString(signature)
	index.RecordDigest = evidenceRegistryAuthorityIndexDigest(index)
	if err := ValidateEvidenceRegistryAuthorityIndex(index); err != nil {
		return EvidenceRegistryAuthorityIndex{}, err
	}
	return index, nil
}

func ParseEvidenceRegistryAuthorityIndex(raw []byte) (EvidenceRegistryAuthorityIndex, error) {
	var index EvidenceRegistryAuthorityIndex
	if err := decodeStrictJSON(raw, &index, "evidence registry authority index"); err != nil {
		return EvidenceRegistryAuthorityIndex{}, err
	}
	canonical, err := json.Marshal(index)
	if err != nil || !bytes.Equal(raw, canonical) {
		return EvidenceRegistryAuthorityIndex{}, errors.New("evidence registry authority index is not canonically encoded")
	}
	return index, ValidateEvidenceRegistryAuthorityIndex(index)
}

func ValidateEvidenceRegistryAuthorityIndex(index EvidenceRegistryAuthorityIndex) error {
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(index.AuthorityPublicKey)
	signature, signatureErr := base64.RawURLEncoding.DecodeString(index.AuthoritySignature)
	if index.SchemaVersion != EvidenceRegistryAuthorityIndexVersion || index.Purpose != EvidenceRegistryAuthorityIndexPurpose ||
		!domainsecurity.IsSHA256Hex(index.InstallationID) || index.Generation == 0 || !validSHA256(index.PreviousIndexDigest) ||
		len(index.Entries) == 0 || index.AuthorityAlgorithm != AcceptedFinalAuthorityAlgorithm || !domainsecurity.IsSHA256Hex(index.AuthorityKeyID) ||
		publicErr != nil || len(publicKey) != ed25519.PublicKeySize || domainsecurity.SHA256Hex(publicKey) != index.AuthorityKeyID ||
		index.InstallationID != index.AuthorityKeyID || signatureErr != nil || len(signature) != ed25519.SignatureSize || !validSHA256(index.RecordDigest) ||
		(index.Generation == 1 && index.PreviousIndexDigest != EvidenceRegistryAuthorityIndexGenesisDigest()) ||
		(index.Generation > 1 && index.PreviousIndexDigest == EvidenceRegistryAuthorityIndexGenesisDigest()) {
		return errors.New("evidence registry authority index is incomplete")
	}
	previousThread, previousTurn := "", ""
	projectionKeys := make(map[string]struct{}, len(index.Entries))
	for _, entry := range index.Entries {
		if strings.TrimSpace(entry.ThreadID) == "" || strings.TrimSpace(entry.TurnID) == "" || !validSHA256(entry.ContextDigest) ||
			!validSHA256(entry.ProjectionKey) || entry.ProjectionKey != EvidenceRegistryProjectionKey(entry.ThreadID, entry.TurnID) ||
			!validSHA256(entry.CapsuleSHA256) || entry.CapsuleByteLength == 0 || !validSHA256(entry.CapsuleRecordDigest) ||
			entry.RegistrySequence == 0 || !validSHA256(entry.RegistryStateDigest) || !validSHA256(entry.LastEntryDigest) ||
			!validSHA256(entry.CanonicalLedgerSHA256) || entry.CanonicalLedgerByteLength == 0 ||
			(previousThread != "" && (entry.ThreadID < previousThread || entry.ThreadID == previousThread && entry.TurnID <= previousTurn)) {
			return errors.New("evidence registry authority index entry is invalid")
		}
		if _, duplicate := projectionKeys[entry.ProjectionKey]; duplicate {
			return errors.New("evidence registry authority index projection key is duplicated")
		}
		projectionKeys[entry.ProjectionKey] = struct{}{}
		previousThread, previousTurn = entry.ThreadID, entry.TurnID
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), EvidenceRegistryAuthorityIndexSigningBytes(index), signature) ||
		index.RecordDigest != evidenceRegistryAuthorityIndexDigest(index) {
		return errors.New("evidence registry authority index integrity is invalid")
	}
	return nil
}

func EvidenceRegistryAuthorityIndexSigningBytes(index EvidenceRegistryAuthorityIndex) []byte {
	index.AuthoritySignature = ""
	index.RecordDigest = ""
	body, _ := json.Marshal(index)
	return append(append([]byte(nil), evidenceRegistryAuthorityIndexSignatureDomain...), body...)
}

func EvidenceRegistryAuthorityIndexEntryForContext(index EvidenceRegistryAuthorityIndex, context domainsecurity.TurnSecurityContext) (EvidenceRegistryAuthorityIndexEntry, bool) {
	if ValidateEvidenceRegistryAuthorityIndex(index) != nil || domainsecurity.ValidateTurnSecurityContext(context) != nil {
		return EvidenceRegistryAuthorityIndexEntry{}, false
	}
	for _, entry := range index.Entries {
		if entry.ThreadID == context.ThreadID && entry.TurnID == context.TurnID && entry.ContextDigest == context.ContextDigest {
			return entry, true
		}
	}
	return EvidenceRegistryAuthorityIndexEntry{}, false
}

func evidenceRegistryAuthorityIndexDigest(index EvidenceRegistryAuthorityIndex) string {
	index.RecordDigest = ""
	body, _ := json.Marshal(index)
	return domainsecurity.SHA256Hex(body)
}

func EvidenceRegistryAuthorityIndexGenesisDigest() string {
	return domainsecurity.SHA256Hex([]byte("analytix.evidence-registry-index/genesis/v1"))
}

func EvidenceRegistryProjectionKey(threadID, turnID string) string {
	return domainsecurity.SHA256Hex([]byte(strings.TrimSpace(threadID) + "\x00" + strings.TrimSpace(turnID)))
}

func CanonicalEvidenceRegistryAuthorityCapsuleBytes(capsule EvidenceRegistryAuthorityCapsule) ([]byte, error) {
	if err := ValidateEvidenceRegistryAuthorityCapsule(capsule); err != nil {
		return nil, err
	}
	body, err := json.Marshal(capsule)
	if err != nil || len(body) == 0 {
		return nil, errors.New("evidence registry authority capsule encoding is invalid")
	}
	return body, nil
}

func NewEvidenceRegistryAuthorityIndexEntry(capsule EvidenceRegistryAuthorityCapsule) (EvidenceRegistryAuthorityIndexEntry, error) {
	body, err := CanonicalEvidenceRegistryAuthorityCapsuleBytes(capsule)
	if err != nil {
		return EvidenceRegistryAuthorityIndexEntry{}, err
	}
	registry := capsule.Registry
	entry := EvidenceRegistryAuthorityIndexEntry{
		ThreadID: registry.ThreadID, TurnID: registry.TurnID, ContextDigest: registry.ContextDigest,
		ProjectionKey: EvidenceRegistryProjectionKey(registry.ThreadID, registry.TurnID), CapsuleSHA256: domainsecurity.SHA256Hex(body),
		CapsuleByteLength: uint64(len(body)), CapsuleRecordDigest: capsule.RecordDigest, RegistrySequence: registry.Sequence,
		RegistryStateDigest: registry.StateDigest, LastEntryDigest: registry.Entries[len(registry.Entries)-1].EntryDigest,
		CanonicalLedgerSHA256: capsule.CanonicalLedgerSHA256, CanonicalLedgerByteLength: capsule.CanonicalLedgerByteLength,
	}
	return entry, nil
}

func EvidenceRegistryAuthorityIndexEntryMatchesCapsule(index EvidenceRegistryAuthorityIndex, entry EvidenceRegistryAuthorityIndexEntry, capsule EvidenceRegistryAuthorityCapsule) bool {
	if ValidateEvidenceRegistryAuthorityIndex(index) != nil || ValidateEvidenceRegistryAuthorityCapsule(capsule) != nil ||
		capsule.Seal.AuthorityKeyID != index.AuthorityKeyID || capsule.Seal.AuthorityPublicKey != index.AuthorityPublicKey {
		return false
	}
	expected, err := NewEvidenceRegistryAuthorityIndexEntry(capsule)
	return err == nil && expected == entry
}

func validateEvidenceRegistryAuthorityIndexEntryExtension(previous, next EvidenceRegistryAuthorityIndexEntry, capsule EvidenceRegistryAuthorityCapsule) error {
	registry := capsule.Registry
	if previous.ThreadID != next.ThreadID || previous.TurnID != next.TurnID || previous.ContextDigest != next.ContextDigest ||
		previous.ProjectionKey != next.ProjectionKey || next.RegistrySequence != previous.RegistrySequence+1 ||
		registry.Sequence != next.RegistrySequence || len(registry.Entries) != int(next.RegistrySequence) ||
		registry.Entries[len(registry.Entries)-1].PreviousRegistryDigest != previous.RegistryStateDigest ||
		registry.Entries[len(registry.Entries)-2].EntryDigest != previous.LastEntryDigest {
		return errors.New("evidence registry authority index entry is not a single extension")
	}
	prefix := registry
	prefix.Sequence = previous.RegistrySequence
	prefix.Entries = append([]EvidenceReceiptRegistryEntry(nil), registry.Entries[:previous.RegistrySequence]...)
	prefix.StateDigest = previous.RegistryStateDigest
	ledger, err := CanonicalEvidenceRegistryLedger(prefix)
	if err != nil || domainsecurity.SHA256Hex(ledger) != previous.CanonicalLedgerSHA256 || uint64(len(ledger)) != previous.CanonicalLedgerByteLength {
		return errors.New("evidence registry authority index entry does not extend the exact previous ledger")
	}
	return nil
}
