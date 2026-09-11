package providerregistry

// This file contains the versioned, key-free wire records and the durable
// state shape for the protected recovery operation.  The application layer is
// responsible for ownership, key handling, and the Registry transaction; the
// domain layer only accepts one canonical bounded representation of the
// protocol records.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"analytix.local/runtime-go/internal/domain/jsonstrict"
)

const (
	ProtectedRecoveryRequestSchemaV1     = "analytix.provider-protected-recovery-request/v1"
	ProtectedRecoveryBundleSchemaV1      = "analytix.provider-protected-recovery-bundle/v1"
	ProtectedRecoveryReceiptSchemaV1     = "analytix.provider-protected-recovery-receipt/v1"
	ProtectedRecoveryPendingKeyPurposeV1 = "protected-recovery-pending-key"
	ProtectedRecoveryProtocolVersion     = 1
	ProtectedRecoveryMaxBytes            = 1 << 20
	ProtectedRecoveryMaxEntries          = 128
	ProtectedRecoveryMaxBindingEntries   = 128
	ProtectedRecoveryMaxCiphertext       = 1 << 20
)

const (
	ProtectedRecoveryPhasePending             = "pending"
	ProtectedRecoveryPhasePendingKeyCandidate = "pending_key_candidate"
	ProtectedRecoveryPhaseConfirmed           = "confirmed"
	ProtectedRecoveryPhaseCandidatesDurable   = "candidates_durable"
	ProtectedRecoveryPhaseVerificationPending = "verification_pending"
	ProtectedRecoveryPhaseApplied             = "applied"
	ProtectedRecoveryPhaseSourceBundle        = "source_bundle"
	ProtectedRecoveryPhaseCleanupPending      = "cleanup_pending"
	ProtectedRecoveryPhaseRollbackPending     = "rollback_pending"
	ProtectedRecoveryPhaseFinalized           = "finalized"
	ProtectedRecoveryPhaseRolledBack          = "rolled_back"
)

// ProtectedRecoveryLocalBindingV1 is deliberately not a wire record.  It is
// retained in the local Manager-owned session and is compared against Main's
// current action before a state transition.
type ProtectedRecoveryLocalBindingV1 struct {
	BrowserWindowID      string `json:"browserWindowId"`
	MainFrameID          string `json:"mainFrameId"`
	ProfileBinding       string `json:"profileBinding"`
	DataDirectoryBinding string `json:"dataDirectoryBinding"`
}

// ProtectedRecoveryConfirmationV1 is a Main-authenticated action assertion.
// It is local authority, not an external cryptographic proof.
type ProtectedRecoveryConfirmationV1 struct {
	RequestDigest      string                          `json:"requestDigest"`
	RequestFingerprint string                          `json:"requestFingerprint"`
	OperationID        string                          `json:"operationId"`
	SessionNonce       string                          `json:"sessionNonce"`
	ManifestDigest     string                          `json:"manifestDigest"`
	ItemSetDigest      string                          `json:"itemSetDigest"`
	ExpiresAt          string                          `json:"expiresAt"`
	LocalBinding       ProtectedRecoveryLocalBindingV1 `json:"localBinding"`
}

type ProtectedRecoveryLocalActionV1 struct {
	LocalBinding ProtectedRecoveryLocalBindingV1 `json:"localBinding"`
	Confirmation ProtectedRecoveryConfirmationV1 `json:"confirmation"`
}

// ProtectedRecoveryOwnerBindingV1 is a Main-owned, key-free binding
// inventory item.  It is persisted only in a local pending/session record and
// never copied into a request, bundle, or receipt.
type ProtectedRecoveryOwnerBindingV1 struct {
	Correlation string `json:"correlation"`
	Owner       string `json:"owner"`
	Provider    string `json:"provider"`
	AccountID   string `json:"accountId"`
	ChannelID   string `json:"channelId"`
	Purpose     string `json:"purpose"`
	Fingerprint string `json:"fingerprint"`
}

// ProtectedRecoveryEntryV1 is the request's destination mapping and fence.
// Correlation is manifest-local.  No source identity is represented.
type ProtectedRecoveryEntryV1 struct {
	Correlation                    string `json:"correlation"`
	DestinationProviderID          string `json:"destinationProviderId"`
	DestinationProviderRevision    uint64 `json:"destinationProviderRevision"`
	DestinationProviderGeneration  uint64 `json:"destinationProviderGeneration"`
	DestinationProviderIncarnation string `json:"destinationProviderIncarnation"`
}

type ProtectedRecoveryRequestV1 struct {
	Schema                        string                     `json:"schema"`
	ProtocolVersion               int                        `json:"protocolVersion"`
	ManifestDigest                string                     `json:"manifestDigest"`
	OperationID                   string                     `json:"operationId"`
	SessionNonce                  string                     `json:"sessionNonce"`
	ExpiresAt                     string                     `json:"expiresAt"`
	DestinationEphemeralPublicKey []byte                     `json:"destinationEphemeralPublicKey"`
	VerificationFingerprint       string                     `json:"verificationFingerprint"`
	Entries                       []ProtectedRecoveryEntryV1 `json:"entries"`
	RequestDigest                 string                     `json:"-"`
}

type ProtectedRecoveryBundleEntryV1 struct {
	Correlation           string `json:"correlation"`
	DestinationProviderID string `json:"destinationProviderId"`
	Purpose               string `json:"purpose"`
}

type ProtectedRecoveryBundleV1 struct {
	Schema                   string                           `json:"schema"`
	ProtocolVersion          int                              `json:"protocolVersion"`
	RequestDigest            string                           `json:"requestDigest"`
	ManifestDigest           string                           `json:"manifestDigest"`
	OperationID              string                           `json:"operationId"`
	SessionNonce             string                           `json:"sessionNonce"`
	ExpiresAt                string                           `json:"expiresAt"`
	SourceEphemeralPublicKey []byte                           `json:"sourceEphemeralPublicKey"`
	Nonce                    []byte                           `json:"nonce"`
	Ciphertext               []byte                           `json:"ciphertext"`
	Entries                  []ProtectedRecoveryBundleEntryV1 `json:"entries"`
}

type ProtectedRecoveryReceiptEntryV1 struct {
	Correlation           string `json:"correlation"`
	DestinationProviderID string `json:"destinationProviderId"`
	Status                string `json:"status"`
}

type ProtectedRecoveryReceiptV1 struct {
	Schema            string                            `json:"schema"`
	ProtocolVersion   int                               `json:"protocolVersion"`
	RequestDigest     string                            `json:"requestDigest"`
	ManifestDigest    string                            `json:"manifestDigest"`
	BundleDigest      string                            `json:"bundleDigest"`
	OperationID       string                            `json:"operationId"`
	SessionNonce      string                            `json:"sessionNonce"`
	ExpiresAt         string                            `json:"expiresAt"`
	Entries           []ProtectedRecoveryReceiptEntryV1 `json:"entries"`
	AuthenticationTag []byte                            `json:"authenticationTag"`
}

// ProtectedRecoveryPreparedRequestV1 is a redacted result returned to Main
// after request preparation.  Request is the canonical public request bytes.
type ProtectedRecoveryPreparedRequestV1 struct {
	Request            []byte `json:"request"`
	RequestDigest      string `json:"requestDigest"`
	RequestFingerprint string `json:"requestFingerprint"`
	ManifestDigest     string `json:"manifestDigest"`
	ItemSetDigest      string `json:"itemSetDigest"`
	OperationID        string `json:"operationId"`
	SessionNonce       string `json:"sessionNonce"`
	ExpiresAt          string `json:"expiresAt"`
}

// ProtectedRecoveryResultV1 is the only result shape exposed by the Go
// protected recovery owner seam.  It contains no references or secret bytes.
type ProtectedRecoveryResultV1 struct {
	Status          string                            `json:"status"`
	ProviderCount   int                               `json:"providerCount,omitempty"`
	AccountCount    int                               `json:"accountCount,omitempty"`
	ReentryRequired int                               `json:"reentryRequired,omitempty"`
	Confirmed       bool                              `json:"confirmed"`
	Entries         []ProtectedRecoveryReceiptEntryV1 `json:"entries,omitempty"`
	// Receipt is a key-free authenticated artifact returned only through the
	// operation-specific Main recovery continuation. It is never persisted in
	// the public result envelope or exposed through ordinary status responses.
	Receipt []byte `json:"-"`
}

// ProtectedRecoverySessionV1 is local Registry state. Request and manifest
// bytes are canonical metadata only. Pending/source phases may retain
// encrypted bundle/receipt transcript bytes and K1 references, but never
// plaintext, source keys or refs, or trust/permission authority. A finalized
// phase is a redacted terminal audit and retains no local bindings or artifact
// bytes.
type ProtectedRecoverySessionV1 struct {
	Version                  int                               `json:"version"`
	OperationID              string                            `json:"operationId"`
	Phase                    string                            `json:"phase"`
	Request                  []byte                            `json:"request"`
	ManifestJSON             []byte                            `json:"manifestJson"`
	RequestDigest            string                            `json:"requestDigest"`
	ManifestDigest           string                            `json:"manifestDigest"`
	ItemSetDigest            string                            `json:"itemSetDigest"`
	SessionNonce             string                            `json:"sessionNonce"`
	ExpiresAt                string                            `json:"expiresAt"`
	DestinationKeyRef        string                            `json:"destinationKeyRef,omitempty"`
	DestinationKeyPurpose    string                            `json:"destinationKeyPurpose,omitempty"`
	Entries                  []ProtectedRecoveryEntryV1        `json:"entries"`
	OwnerBindings            []ProtectedRecoveryOwnerBindingV1 `json:"ownerBindings,omitempty"`
	LocalBinding             ProtectedRecoveryLocalBindingV1   `json:"localBinding"`
	PriorRegistryRevision    uint64                            `json:"priorRegistryRevision,omitempty"`
	PriorRegistryIncarnation string                            `json:"priorRegistryIncarnation,omitempty"`
	PriorSelectedProviderID  string                            `json:"priorSelectedProviderId,omitempty"`
	BatchRegistryRevision    uint64                            `json:"batchRegistryRevision,omitempty"`
	Bundle                   []byte                            `json:"bundle,omitempty"`
	BundleDigest             string                            `json:"bundleDigest,omitempty"`
	PriorProviders           []Provider                        `json:"priorProviders,omitempty"`
	Confirmed                bool                              `json:"confirmed"`
	Consumed                 bool                              `json:"consumed"`
	ReconfirmationRequired   bool                              `json:"reconfirmationRequired,omitempty"`
	CandidateRefs            []string                          `json:"candidateRefs,omitempty"`
	CandidatePurposes        []string                          `json:"candidatePurposes,omitempty"`
	CandidateDigests         []string                          `json:"candidateDigests,omitempty"`
	CleanupKind              string                            `json:"cleanupKind,omitempty"`
	CleanupRefs              []string                          `json:"cleanupRefs,omitempty"`
	CleanupPurposes          []string                          `json:"cleanupPurposes,omitempty"`
	Receipt                  []byte                            `json:"receipt,omitempty"`
}

func (request ProtectedRecoveryRequestV1) Clone() ProtectedRecoveryRequestV1 {
	request.DestinationEphemeralPublicKey = bytes.Clone(request.DestinationEphemeralPublicKey)
	request.Entries = append([]ProtectedRecoveryEntryV1(nil), request.Entries...)
	return request
}

func (bundle ProtectedRecoveryBundleV1) Clone() ProtectedRecoveryBundleV1 {
	bundle.SourceEphemeralPublicKey = bytes.Clone(bundle.SourceEphemeralPublicKey)
	bundle.Nonce = bytes.Clone(bundle.Nonce)
	bundle.Ciphertext = bytes.Clone(bundle.Ciphertext)
	bundle.Entries = append([]ProtectedRecoveryBundleEntryV1(nil), bundle.Entries...)
	return bundle
}

func (receipt ProtectedRecoveryReceiptV1) Clone() ProtectedRecoveryReceiptV1 {
	receipt.AuthenticationTag = bytes.Clone(receipt.AuthenticationTag)
	receipt.Entries = append([]ProtectedRecoveryReceiptEntryV1(nil), receipt.Entries...)
	return receipt
}

func (session ProtectedRecoverySessionV1) Clone() ProtectedRecoverySessionV1 {
	session.Request = bytes.Clone(session.Request)
	session.ManifestJSON = bytes.Clone(session.ManifestJSON)
	entries := session.Entries
	session.Entries = make([]ProtectedRecoveryEntryV1, len(entries))
	copy(session.Entries, entries)
	ownerBindings := session.OwnerBindings
	session.OwnerBindings = make([]ProtectedRecoveryOwnerBindingV1, len(ownerBindings))
	copy(session.OwnerBindings, ownerBindings)
	candidateRefs := session.CandidateRefs
	session.CandidateRefs = make([]string, len(candidateRefs))
	copy(session.CandidateRefs, candidateRefs)
	session.Receipt = bytes.Clone(session.Receipt)
	session.Bundle = bytes.Clone(session.Bundle)
	priorProviders := session.PriorProviders
	session.PriorProviders = make([]Provider, len(priorProviders))
	for index, provider := range priorProviders {
		session.PriorProviders[index] = provider.Clone()
	}
	candidatePurposes := session.CandidatePurposes
	session.CandidatePurposes = make([]string, len(candidatePurposes))
	copy(session.CandidatePurposes, candidatePurposes)
	candidateDigests := session.CandidateDigests
	session.CandidateDigests = make([]string, len(candidateDigests))
	copy(session.CandidateDigests, candidateDigests)
	cleanupRefs := session.CleanupRefs
	session.CleanupRefs = make([]string, len(cleanupRefs))
	copy(session.CleanupRefs, cleanupRefs)
	cleanupPurposes := session.CleanupPurposes
	session.CleanupPurposes = make([]string, len(cleanupPurposes))
	copy(session.CleanupPurposes, cleanupPurposes)
	return session
}

func (binding ProtectedRecoveryLocalBindingV1) Validate() error {
	if validProtectedLocalString(binding.BrowserWindowID, 256, false) != nil ||
		validProtectedLocalString(binding.MainFrameID, 256, false) != nil ||
		validProtectedLocalString(binding.ProfileBinding, 512, false) != nil ||
		validProtectedLocalString(binding.DataDirectoryBinding, 2048, false) != nil {
		return ErrInvalidRegistry
	}
	return nil
}

func (action ProtectedRecoveryLocalActionV1) Validate() error {
	if action.LocalBinding.Validate() != nil || action.Confirmation.Validate() != nil {
		return ErrInvalidRegistry
	}
	if action.Confirmation.LocalBinding != action.LocalBinding {
		return ErrInvalidRegistry
	}
	return nil
}

func (confirmation ProtectedRecoveryConfirmationV1) Validate() error {
	if confirmation.LocalBinding.Validate() != nil ||
		!validSHA256(confirmation.RequestDigest) ||
		!validFingerprint(confirmation.RequestFingerprint) ||
		!validOperationID(confirmation.OperationID) ||
		!validNonce(confirmation.SessionNonce) ||
		!validSHA256(confirmation.ManifestDigest) ||
		!validSHA256(confirmation.ItemSetDigest) ||
		!validExpiry(confirmation.ExpiresAt) {
		return ErrInvalidRegistry
	}
	return nil
}

func (binding ProtectedRecoveryOwnerBindingV1) Validate() error {
	if !validPortableAccountCorrelation(binding.Correlation) || (binding.Owner != "mcp" && binding.Owner != "extension") {
		return ErrInvalidRegistry
	}
	if !validProtectedMetadata(binding.Provider, 256, false) ||
		!validProtectedMetadata(binding.AccountID, 256, false) ||
		!validProtectedMetadata(binding.ChannelID, 256, false) ||
		!validPurpose(binding.Purpose) || !validProtectedMetadata(binding.Fingerprint, 512, false) {
		return ErrInvalidRegistry
	}
	if binding.Owner == "mcp" && binding.Purpose != "mcp-oauth-access-token" ||
		binding.Owner == "extension" && binding.Purpose != "extension-provider-account-token" {
		return ErrInvalidRegistry
	}
	return nil
}

func (entry ProtectedRecoveryEntryV1) Validate() error {
	if !validPortableCorrelation(entry.Correlation) || !ValidProviderID(entry.DestinationProviderID) ||
		entry.DestinationProviderRevision == 0 || entry.DestinationProviderGeneration == 0 ||
		!ValidIncarnation(entry.DestinationProviderIncarnation) {
		return ErrInvalidRegistry
	}
	return nil
}

func (request ProtectedRecoveryRequestV1) Validate() error {
	if request.Schema != ProtectedRecoveryRequestSchemaV1 || request.ProtocolVersion != ProtectedRecoveryProtocolVersion ||
		!validSHA256(request.ManifestDigest) || !validOperationID(request.OperationID) ||
		!validNonce(request.SessionNonce) || !validExpiry(request.ExpiresAt) ||
		len(request.DestinationEphemeralPublicKey) != 32 ||
		!validFingerprint(request.VerificationFingerprint) || request.Entries == nil || len(request.Entries) == 0 || len(request.Entries) > ProtectedRecoveryMaxEntries {
		return ErrInvalidRegistry
	}
	seen := make(map[string]struct{}, len(request.Entries))
	seenDestinationIDs := make(map[string]struct{}, len(request.Entries))
	for _, entry := range request.Entries {
		if entry.Validate() != nil {
			return ErrInvalidRegistry
		}
		if _, duplicate := seen[entry.Correlation]; duplicate {
			return ErrInvalidRegistry
		}
		seen[entry.Correlation] = struct{}{}
		if _, duplicate := seenDestinationIDs[entry.DestinationProviderID]; duplicate {
			return ErrInvalidRegistry
		}
		seenDestinationIDs[entry.DestinationProviderID] = struct{}{}
	}
	if !validProtectedRecoveryCorrelationOrder(request.Entries) {
		return ErrInvalidRegistry
	}
	return nil
}

func (bundle ProtectedRecoveryBundleEntryV1) Validate() error {
	if !validPortableCorrelation(bundle.Correlation) || !ValidProviderID(bundle.DestinationProviderID) ||
		!validPurpose(bundle.Purpose) {
		return ErrInvalidRegistry
	}
	return nil
}

func (bundle ProtectedRecoveryBundleV1) Validate() error {
	if bundle.Schema != ProtectedRecoveryBundleSchemaV1 || bundle.ProtocolVersion != ProtectedRecoveryProtocolVersion ||
		!validSHA256(bundle.RequestDigest) || !validSHA256(bundle.ManifestDigest) ||
		!validOperationID(bundle.OperationID) || !validNonce(bundle.SessionNonce) || !validExpiry(bundle.ExpiresAt) ||
		len(bundle.SourceEphemeralPublicKey) != 32 ||
		len(bundle.Nonce) != 12 || len(bundle.Ciphertext) == 0 || len(bundle.Ciphertext) > ProtectedRecoveryMaxCiphertext ||
		bundle.Entries == nil || len(bundle.Entries) == 0 || len(bundle.Entries) > ProtectedRecoveryMaxEntries {
		return ErrInvalidRegistry
	}
	seen := make(map[string]struct{}, len(bundle.Entries))
	seenDestinationIDs := make(map[string]struct{}, len(bundle.Entries))
	for _, entry := range bundle.Entries {
		if entry.Validate() != nil {
			return ErrInvalidRegistry
		}
		if _, duplicate := seen[entry.Correlation]; duplicate {
			return ErrInvalidRegistry
		}
		seen[entry.Correlation] = struct{}{}
		if _, duplicate := seenDestinationIDs[entry.DestinationProviderID]; duplicate {
			return ErrInvalidRegistry
		}
		seenDestinationIDs[entry.DestinationProviderID] = struct{}{}
	}
	if !validProtectedRecoveryCorrelationOrderFromBundle(bundle.Entries) {
		return ErrInvalidRegistry
	}
	return nil
}

func (entry ProtectedRecoveryReceiptEntryV1) Validate() error {
	if !validPortableCorrelation(entry.Correlation) || !ValidProviderID(entry.DestinationProviderID) ||
		(entry.Status != "applied" && entry.Status != "reentry_required") {
		return ErrInvalidRegistry
	}
	return nil
}

func (receipt ProtectedRecoveryReceiptV1) Validate() error {
	if receipt.Schema != ProtectedRecoveryReceiptSchemaV1 || receipt.ProtocolVersion != ProtectedRecoveryProtocolVersion ||
		!validSHA256(receipt.RequestDigest) || !validSHA256(receipt.ManifestDigest) || !validSHA256(receipt.BundleDigest) ||
		!validOperationID(receipt.OperationID) || !validNonce(receipt.SessionNonce) || !validExpiry(receipt.ExpiresAt) ||
		receipt.Entries == nil || len(receipt.Entries) == 0 || len(receipt.Entries) > ProtectedRecoveryMaxEntries ||
		len(receipt.AuthenticationTag) != 32 {
		return ErrInvalidRegistry
	}
	seen := make(map[string]struct{}, len(receipt.Entries))
	seenDestinationIDs := make(map[string]struct{}, len(receipt.Entries))
	for _, entry := range receipt.Entries {
		if entry.Validate() != nil {
			return ErrInvalidRegistry
		}
		if _, duplicate := seen[entry.Correlation]; duplicate {
			return ErrInvalidRegistry
		}
		seen[entry.Correlation] = struct{}{}
		if _, duplicate := seenDestinationIDs[entry.DestinationProviderID]; duplicate {
			return ErrInvalidRegistry
		}
		seenDestinationIDs[entry.DestinationProviderID] = struct{}{}
	}
	if !validProtectedRecoveryCorrelationOrderFromReceipt(receipt.Entries) {
		return ErrInvalidRegistry
	}
	return nil
}

func (session ProtectedRecoverySessionV1) Validate() error {
	finalized := session.Phase == ProtectedRecoveryPhaseFinalized
	if session.Version != ProtectedRecoveryProtocolVersion || !validOperationID(session.OperationID) ||
		!validProtectedRecoveryPhase(session.Phase) ||
		(!finalized && (len(session.Request) == 0 || len(session.ManifestJSON) == 0 || session.LocalBinding.Validate() != nil)) ||
		(finalized && (len(session.Request) != 0 || len(session.ManifestJSON) != 0 ||
			session.LocalBinding != (ProtectedRecoveryLocalBindingV1{}))) ||
		len(session.ManifestJSON) > ProtectedRecoveryMaxBytes ||
		!validSHA256(session.RequestDigest) || !validSHA256(session.ManifestDigest) ||
		!validSHA256(session.ItemSetDigest) || !validNonce(session.SessionNonce) ||
		!validExpiry(session.ExpiresAt) ||
		session.Entries == nil || len(session.Entries) == 0 || len(session.Entries) > ProtectedRecoveryMaxEntries ||
		len(session.OwnerBindings) > ProtectedRecoveryMaxBindingEntries {
		return ErrInvalidRegistry
	}
	if session.ReconfirmationRequired && session.Phase != ProtectedRecoveryPhasePending {
		return ErrInvalidRegistry
	}
	if (session.DestinationKeyRef == "") != (session.DestinationKeyPurpose == "") {
		return ErrInvalidRegistry
	}
	if session.DestinationKeyRef != "" && !ValidCredentialRef(session.DestinationKeyRef) {
		return ErrInvalidRegistry
	}
	if session.DestinationKeyPurpose != "" && session.DestinationKeyPurpose != ProtectedRecoveryPendingKeyPurposeV1 {
		return ErrInvalidRegistry
	}
	if len(session.CandidateRefs) != len(session.CandidatePurposes) ||
		len(session.CandidateRefs) != len(session.CandidateDigests) || len(session.CandidateRefs) > len(session.Entries) {
		return ErrInvalidRegistry
	}
	if len(session.CleanupRefs) != len(session.CleanupPurposes) || len(session.CleanupRefs) > len(session.Entries)+1 {
		return ErrInvalidRegistry
	}
	if (session.PriorRegistryRevision == 0) != (session.PriorRegistryIncarnation == "") {
		return ErrInvalidRegistry
	}
	if session.PriorRegistryIncarnation != "" && !ValidIncarnation(session.PriorRegistryIncarnation) {
		return ErrInvalidRegistry
	}
	if session.BatchRegistryRevision != 0 && session.PriorRegistryRevision == 0 {
		return ErrInvalidRegistry
	}
	if !finalized {
		request, err := ParseProtectedRecoveryRequestV1(session.Request)
		manifest, manifestErr := ParsePortableManifestV1(session.ManifestJSON)
		if err != nil || manifestErr != nil || len(request.DestinationEphemeralPublicKey) != 32 ||
			request.RequestDigest != session.RequestDigest || request.ManifestDigest != session.ManifestDigest ||
			request.OperationID != session.OperationID || request.SessionNonce != session.SessionNonce ||
			request.ExpiresAt != session.ExpiresAt || !reflect.DeepEqual(request.Entries, session.Entries) ||
			ProtectedRecoveryItemSetDigest(session.Entries) != session.ItemSetDigest ||
			ProtectedRecoveryManifestDigest(session.ManifestJSON) != session.ManifestDigest ||
			manifest.Schema != PortableManifestSchemaV1 {
			return ErrInvalidRegistry
		}
	}
	seen := make(map[string]struct{}, len(session.Entries))
	for _, entry := range session.Entries {
		if entry.Validate() != nil {
			return ErrInvalidRegistry
		}
		if _, duplicate := seen[entry.Correlation]; duplicate {
			return ErrInvalidRegistry
		}
		seen[entry.Correlation] = struct{}{}
	}
	for _, binding := range session.OwnerBindings {
		if binding.Validate() != nil {
			return ErrInvalidRegistry
		}
	}
	seenOwnerCorrelations := make(map[string]struct{}, len(session.OwnerBindings))
	for _, binding := range session.OwnerBindings {
		if _, duplicate := seenOwnerCorrelations[binding.Correlation]; duplicate {
			return ErrInvalidRegistry
		}
		seenOwnerCorrelations[binding.Correlation] = struct{}{}
	}
	seenCredentialRefs := make(map[string]struct{}, len(session.CandidateRefs)+len(session.CleanupRefs)+1)
	if session.DestinationKeyRef != "" {
		if _, duplicate := seenCredentialRefs[session.DestinationKeyRef]; duplicate {
			return ErrInvalidRegistry
		}
		seenCredentialRefs[session.DestinationKeyRef] = struct{}{}
	}
	for index, ref := range session.CandidateRefs {
		if !ValidCredentialRef(ref) || session.CandidatePurposes[index] == "" || !validPurpose(session.CandidatePurposes[index]) ||
			!validSHA256(session.CandidateDigests[index]) {
			return ErrInvalidRegistry
		}
		if _, duplicate := seenCredentialRefs[ref]; duplicate {
			return ErrInvalidRegistry
		}
		seenCredentialRefs[ref] = struct{}{}
	}
	candidateRefSet := make(map[string]struct{}, len(session.CandidateRefs))
	for _, ref := range session.CandidateRefs {
		candidateRefSet[ref] = struct{}{}
	}
	for index, ref := range session.CleanupRefs {
		if !ValidCredentialRef(ref) || session.CleanupPurposes[index] == "" || !validPurpose(session.CleanupPurposes[index]) {
			return ErrInvalidRegistry
		}
		if _, duplicate := seenCredentialRefs[ref]; duplicate && ref != session.DestinationKeyRef {
			if _, candidate := candidateRefSet[ref]; !candidate {
				return ErrInvalidRegistry
			}
		}
		if ref == session.DestinationKeyRef && session.CleanupKind != "destination-key" && session.CleanupKind != "all" {
			return ErrInvalidRegistry
		}
		seenCredentialRefs[ref] = struct{}{}
	}
	if len(session.PriorProviders) != 0 && len(session.PriorProviders) != len(session.Entries) {
		return ErrInvalidRegistry
	}
	for index, provider := range session.PriorProviders {
		entry := session.Entries[index]
		if provider.Validate() != nil || provider.ID != entry.DestinationProviderID || provider.Tombstone ||
			provider.CredentialRef != "" || provider.CredentialPurpose != "" || provider.Revision != entry.DestinationProviderRevision ||
			provider.Generation != entry.DestinationProviderGeneration || provider.Incarnation != entry.DestinationProviderIncarnation {
			return ErrInvalidRegistry
		}
	}
	if len(session.Bundle) == 0 {
		if finalized {
			if !validSHA256(session.BundleDigest) {
				return ErrInvalidRegistry
			}
		} else if session.BundleDigest != "" {
			return ErrInvalidRegistry
		}
	} else {
		bundle, bundleErr := ParseProtectedRecoveryBundleV1(session.Bundle)
		if bundleErr != nil || session.BundleDigest != ProtectedRecoveryBundleDigest(session.Bundle) ||
			bundle.RequestDigest != session.RequestDigest || bundle.ManifestDigest != session.ManifestDigest ||
			bundle.OperationID != session.OperationID || bundle.SessionNonce != session.SessionNonce ||
			bundle.ExpiresAt != session.ExpiresAt || len(bundle.Entries) != len(session.Entries) {
			return ErrInvalidRegistry
		}
		for index, entry := range bundle.Entries {
			if entry.Correlation != session.Entries[index].Correlation || entry.DestinationProviderID != session.Entries[index].DestinationProviderID {
				return ErrInvalidRegistry
			}
		}
	}
	if len(session.Receipt) != 0 {
		receipt, receiptErr := ParseProtectedRecoveryReceiptV1(session.Receipt)
		if receiptErr != nil || receipt.RequestDigest != session.RequestDigest || receipt.ManifestDigest != session.ManifestDigest ||
			receipt.OperationID != session.OperationID || receipt.SessionNonce != session.SessionNonce ||
			receipt.ExpiresAt != session.ExpiresAt || len(receipt.Entries) != len(session.Entries) {
			return ErrInvalidRegistry
		}
		for index, entry := range receipt.Entries {
			if entry.Correlation != session.Entries[index].Correlation || entry.DestinationProviderID != session.Entries[index].DestinationProviderID {
				return ErrInvalidRegistry
			}
		}
		if len(session.Bundle) != 0 && receipt.BundleDigest != session.BundleDigest {
			return ErrInvalidRegistry
		}
	} else if session.Phase == ProtectedRecoveryPhaseApplied || session.Phase == ProtectedRecoveryPhaseSourceBundle {
		return ErrInvalidRegistry
	}
	switch session.Phase {
	case ProtectedRecoveryPhasePending:
		if session.Confirmed || session.Consumed || len(session.CandidateRefs) != 0 || len(session.Bundle) != 0 || len(session.Receipt) != 0 || session.BatchRegistryRevision != 0 ||
			session.CleanupKind != "" || len(session.CleanupRefs) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhasePendingKeyCandidate:
		// The Registry journal is published before the pending-key candidate is
		// committed. This is the only phase allowed to name a destination key
		// before the ordinary unconfirmed pending session becomes available.
		if session.ReconfirmationRequired || session.Confirmed || session.Consumed || session.DestinationKeyRef == "" ||
			session.DestinationKeyPurpose != ProtectedRecoveryPendingKeyPurposeV1 ||
			len(session.CandidateRefs) != 0 || len(session.Bundle) != 0 || len(session.Receipt) != 0 ||
			session.BatchRegistryRevision != 0 || len(session.PriorProviders) != len(session.Entries) ||
			session.CleanupKind != "" || len(session.CleanupRefs) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseConfirmed:
		if session.ReconfirmationRequired || !session.Confirmed || session.Consumed || len(session.CandidateRefs) != 0 || len(session.Bundle) != 0 || len(session.Receipt) != 0 || session.BatchRegistryRevision != 0 ||
			session.CleanupKind != "" || len(session.CleanupRefs) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseCandidatesDurable:
		if session.ReconfirmationRequired || !session.Confirmed || session.Consumed || session.DestinationKeyRef == "" || session.DestinationKeyPurpose != ProtectedRecoveryPendingKeyPurposeV1 ||
			len(session.CandidateRefs) == 0 || len(session.Bundle) == 0 || len(session.Receipt) != 0 || session.BatchRegistryRevision != 0 ||
			len(session.PriorProviders) != len(session.Entries) || session.CleanupKind != "" || len(session.CleanupRefs) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseVerificationPending:
		if session.ReconfirmationRequired || !session.Confirmed || session.Consumed || session.DestinationKeyRef == "" || session.DestinationKeyPurpose != ProtectedRecoveryPendingKeyPurposeV1 ||
			len(session.CandidateRefs) == 0 || len(session.Bundle) == 0 || len(session.Receipt) == 0 || session.BatchRegistryRevision == 0 ||
			len(session.PriorProviders) != len(session.Entries) || session.CleanupKind != "" || len(session.CleanupRefs) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseApplied:
		if session.ReconfirmationRequired || !session.Confirmed || !session.Consumed || len(session.CandidateRefs) == 0 || len(session.Bundle) == 0 || len(session.Receipt) == 0 || session.BatchRegistryRevision == 0 ||
			len(session.PriorProviders) != len(session.Entries) || session.CleanupKind != "" || len(session.CleanupRefs) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseSourceBundle:
		if session.ReconfirmationRequired || !session.Confirmed || session.Consumed || session.DestinationKeyRef != "" || len(session.CandidateRefs) != 0 || len(session.Bundle) == 0 ||
			len(session.Receipt) == 0 || len(session.PriorProviders) != 0 || session.CleanupKind != "" || len(session.CleanupRefs) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseCleanupPending:
		if len(session.CleanupRefs) == 0 || session.CleanupKind == "" ||
			(session.CleanupKind != "destination-key" && session.CleanupKind != "candidates" && session.CleanupKind != "all") {
			return ErrInvalidRegistry
		}
		if session.CleanupKind == "destination-key" {
			// A key-only cleanup record can be left behind if the Registry
			// publication of the initial pending session failed after its K1
			// candidate was committed. It has no batch authority and is only
			// allowed to name that exact pending key for retryable cleanup.
			if !session.Confirmed && !session.Consumed && session.BatchRegistryRevision == 0 &&
				len(session.CandidateRefs) == 0 && len(session.Bundle) == 0 && len(session.Receipt) == 0 {
				if session.DestinationKeyRef == "" || len(session.CleanupRefs) != 1 || session.CleanupRefs[0] != session.DestinationKeyRef {
					return ErrInvalidRegistry
				}
			} else if !session.Confirmed || !session.Consumed || session.BatchRegistryRevision == 0 || len(session.Receipt) == 0 || len(session.CandidateRefs) == 0 {
				return ErrInvalidRegistry
			}
		}
		if session.CleanupKind == "candidates" && (session.Consumed || len(session.Receipt) != 0) {
			return ErrInvalidRegistry
		}
		if session.CleanupKind == "all" && (session.Consumed || len(session.Receipt) != 0) {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseRollbackPending:
		if session.DestinationKeyRef == "" || session.DestinationKeyPurpose != ProtectedRecoveryPendingKeyPurposeV1 ||
			session.CleanupKind != "candidates" || len(session.CleanupRefs) == 0 || session.Consumed || len(session.Receipt) != 0 {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseFinalized:
		if session.ReconfirmationRequired || !session.Confirmed || !session.Consumed || session.DestinationKeyRef != "" ||
			session.DestinationKeyPurpose != "" || len(session.Request) != 0 || len(session.ManifestJSON) != 0 ||
			session.LocalBinding != (ProtectedRecoveryLocalBindingV1{}) || len(session.OwnerBindings) != 0 ||
			len(session.CandidateRefs) != 0 || len(session.CandidatePurposes) != 0 || len(session.CandidateDigests) != 0 ||
			len(session.Receipt) != 0 || len(session.Bundle) != 0 || !validSHA256(session.BundleDigest) ||
			session.BatchRegistryRevision != 0 || session.PriorRegistryRevision != 0 || session.PriorRegistryIncarnation != "" ||
			session.PriorSelectedProviderID != "" || len(session.PriorProviders) != 0 ||
			len(session.CleanupRefs) != 0 || len(session.CleanupPurposes) != 0 || session.CleanupKind != "" {
			return ErrInvalidRegistry
		}
	case ProtectedRecoveryPhaseRolledBack:
		if session.ReconfirmationRequired || session.Confirmed || session.Consumed || session.DestinationKeyRef != "" || session.DestinationKeyPurpose != "" ||
			len(session.CleanupRefs) != 0 || len(session.CleanupPurposes) != 0 || len(session.CandidateRefs) != 0 ||
			len(session.CandidatePurposes) != 0 || len(session.CandidateDigests) != 0 || len(session.Bundle) != 0 ||
			session.BundleDigest != "" || len(session.Receipt) != 0 || session.BatchRegistryRevision != 0 ||
			len(session.PriorProviders) != 0 || session.CleanupKind != "" {
			return ErrInvalidRegistry
		}
	default:
		return ErrInvalidRegistry
	}
	if session.Consumed && !session.Confirmed {
		return ErrInvalidRegistry
	}
	return nil
}

func ParseProtectedRecoveryRequestV1(data []byte) (ProtectedRecoveryRequestV1, error) {
	var request ProtectedRecoveryRequestV1
	if err := decodeProtectedRecovery(data, &request, func() error { return request.Validate() }, marshalProtectedRecoveryRequestCanonical); err != nil {
		return ProtectedRecoveryRequestV1{}, err
	}
	request.RequestDigest = protectedRecoveryRequestDigest(request)
	if request.RequestDigest == "" || request.VerificationFingerprint != ProtectedRecoveryFingerprint(request.RequestDigest) {
		return ProtectedRecoveryRequestV1{}, ErrInvalidRegistry
	}
	return request, nil
}

func MarshalProtectedRecoveryRequestV1(request ProtectedRecoveryRequestV1) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return marshalProtectedRecoveryRequestCanonical(request)
}

// MarshalProtectedRecoveryRequestV1ForDigest returns the canonical request
// payload with its derived fingerprint omitted.  It is used by the app owner
// to compute the non-self-referential request digest.
func MarshalProtectedRecoveryRequestV1ForDigest(request ProtectedRecoveryRequestV1) ([]byte, error) {
	// This helper omits only the derived fingerprint; it must retain the same
	// producer-side shape checks as the public request marshaler. A fixed valid
	// placeholder lets Validate enforce every other field without introducing a
	// self-referential fingerprint into the digest payload.
	request.VerificationFingerprint = "0000000000000000"
	if err := request.Validate(); err != nil {
		return nil, err
	}
	request.VerificationFingerprint = ""
	return marshalProtectedRecoveryRequestCanonical(request)
}

func ParseProtectedRecoveryBundleV1(data []byte) (ProtectedRecoveryBundleV1, error) {
	var bundle ProtectedRecoveryBundleV1
	if err := decodeProtectedRecovery(data, &bundle, func() error { return bundle.Validate() }, marshalProtectedRecoveryBundleCanonical); err != nil {
		return ProtectedRecoveryBundleV1{}, err
	}
	return bundle, nil
}

func MarshalProtectedRecoveryBundleV1(bundle ProtectedRecoveryBundleV1) ([]byte, error) {
	if err := bundle.Validate(); err != nil {
		return nil, err
	}
	return marshalProtectedRecoveryBundleCanonical(bundle)
}

func ParseProtectedRecoveryReceiptV1(data []byte) (ProtectedRecoveryReceiptV1, error) {
	var receipt ProtectedRecoveryReceiptV1
	if err := decodeProtectedRecovery(data, &receipt, func() error { return receipt.Validate() }, marshalProtectedRecoveryReceiptCanonical); err != nil {
		return ProtectedRecoveryReceiptV1{}, err
	}
	return receipt, nil
}

func MarshalProtectedRecoveryReceiptV1(receipt ProtectedRecoveryReceiptV1) ([]byte, error) {
	if err := receipt.Validate(); err != nil {
		return nil, err
	}
	return marshalProtectedRecoveryReceiptCanonical(receipt)
}

// MarshalProtectedRecoveryReceiptV1ForAuthentication returns the canonical
// receipt transcript with its authentication tag omitted. The tag is derived
// from the transfer key by the application layer, so the transcript cannot be
// self-referential while the wire receipt remains strict and tag-bearing.
func MarshalProtectedRecoveryReceiptV1ForAuthentication(receipt ProtectedRecoveryReceiptV1) ([]byte, error) {
	if len(receipt.AuthenticationTag) != 32 {
		return nil, ErrInvalidRegistry
	}
	if err := receipt.Validate(); err != nil {
		return nil, err
	}
	return marshalProtectedRecoveryReceiptAuthenticationCanonical(receipt)
}

func decodeProtectedRecovery[T any](data []byte, result *T, validate func() error, marshal func(T) ([]byte, error)) error {
	if len(data) == 0 || len(data) > ProtectedRecoveryMaxBytes {
		return ErrInvalidRegistry
	}
	if err := jsonstrict.Validate(data, jsonstrict.Options{
		RequireObject: true, MaxBytes: ProtectedRecoveryMaxBytes, MaxDepth: 16,
		MaxTokens: 1 << 16, MaxStringBytes: 4096,
	}); err != nil {
		return ErrInvalidRegistry
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return ErrInvalidRegistry
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidRegistry
	}
	if err := validate(); err != nil {
		return err
	}
	canonical, err := marshal(*result)
	if err != nil || !bytes.Equal(data, canonical) {
		return ErrInvalidRegistry
	}
	return nil
}

func marshalProtectedRecoveryCanonical(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	data := buffer.Bytes()
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, ErrInvalidRegistry
	}
	return bytes.Clone(data[:len(data)-1]), nil
}

func marshalProtectedRecoveryRequestCanonical(request ProtectedRecoveryRequestV1) ([]byte, error) {
	copy := request.Clone()
	copy.RequestDigest = ""
	return marshalProtectedRecoveryCanonical(struct {
		Schema                        string                     `json:"schema"`
		ProtocolVersion               int                        `json:"protocolVersion"`
		ManifestDigest                string                     `json:"manifestDigest"`
		OperationID                   string                     `json:"operationId"`
		SessionNonce                  string                     `json:"sessionNonce"`
		ExpiresAt                     string                     `json:"expiresAt"`
		DestinationEphemeralPublicKey []byte                     `json:"destinationEphemeralPublicKey"`
		VerificationFingerprint       string                     `json:"verificationFingerprint,omitempty"`
		Entries                       []ProtectedRecoveryEntryV1 `json:"entries"`
	}{Schema: copy.Schema, ProtocolVersion: copy.ProtocolVersion, ManifestDigest: copy.ManifestDigest,
		OperationID: copy.OperationID, SessionNonce: copy.SessionNonce, ExpiresAt: copy.ExpiresAt,
		DestinationEphemeralPublicKey: copy.DestinationEphemeralPublicKey,
		VerificationFingerprint:       copy.VerificationFingerprint, Entries: copy.Entries})
}

func marshalProtectedRecoveryBundleCanonical(bundle ProtectedRecoveryBundleV1) ([]byte, error) {
	return marshalProtectedRecoveryCanonical(bundle)
}

func marshalProtectedRecoveryReceiptCanonical(receipt ProtectedRecoveryReceiptV1) ([]byte, error) {
	return marshalProtectedRecoveryCanonical(receipt)
}

func marshalProtectedRecoveryReceiptAuthenticationCanonical(receipt ProtectedRecoveryReceiptV1) ([]byte, error) {
	return marshalProtectedRecoveryCanonical(struct {
		Schema          string                            `json:"schema"`
		ProtocolVersion int                               `json:"protocolVersion"`
		RequestDigest   string                            `json:"requestDigest"`
		ManifestDigest  string                            `json:"manifestDigest"`
		BundleDigest    string                            `json:"bundleDigest"`
		OperationID     string                            `json:"operationId"`
		SessionNonce    string                            `json:"sessionNonce"`
		ExpiresAt       string                            `json:"expiresAt"`
		Entries         []ProtectedRecoveryReceiptEntryV1 `json:"entries"`
	}{
		Schema: receipt.Schema, ProtocolVersion: receipt.ProtocolVersion,
		RequestDigest: receipt.RequestDigest, ManifestDigest: receipt.ManifestDigest,
		BundleDigest: receipt.BundleDigest, OperationID: receipt.OperationID,
		SessionNonce: receipt.SessionNonce, ExpiresAt: receipt.ExpiresAt,
		Entries: receipt.Entries,
	})
}

func protectedRecoveryRequestDigest(request ProtectedRecoveryRequestV1) string {
	request.VerificationFingerprint = ""
	canonical, err := marshalProtectedRecoveryRequestCanonical(request)
	if err != nil {
		return ""
	}
	return sha256Hex(canonical)
}

func ProtectedRecoveryRequestDigest(data []byte) (string, error) {
	request, err := ParseProtectedRecoveryRequestV1(data)
	if err != nil {
		return "", err
	}
	return request.RequestDigest, nil
}

func ProtectedRecoveryFingerprint(requestDigest string) string {
	if !validSHA256(requestDigest) {
		return ""
	}
	return sha256Hex([]byte(requestDigest))[:16]
}

func ProtectedRecoveryManifestDigest(data []byte) string { return sha256Hex(data) }

func ProtectedRecoveryItemSetDigest(entries []ProtectedRecoveryEntryV1) string {
	data, err := marshalProtectedRecoveryCanonical(entries)
	if err != nil {
		return ""
	}
	return sha256Hex(data)
}

func ProtectedRecoveryBundleDigest(data []byte) string { return sha256Hex(data) }

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validProtectedRecoveryPhase(value string) bool {
	switch value {
	case ProtectedRecoveryPhasePending, ProtectedRecoveryPhasePendingKeyCandidate, ProtectedRecoveryPhaseConfirmed, ProtectedRecoveryPhaseCandidatesDurable,
		ProtectedRecoveryPhaseVerificationPending, ProtectedRecoveryPhaseApplied, ProtectedRecoveryPhaseSourceBundle,
		ProtectedRecoveryPhaseCleanupPending, ProtectedRecoveryPhaseRollbackPending, ProtectedRecoveryPhaseFinalized,
		ProtectedRecoveryPhaseRolledBack:
		return true
	default:
		return false
	}
}

func validProtectedLocalString(value string, maximum int, allowEmpty bool) error {
	if !allowEmpty && value == "" || len(value) > maximum || value != strings.TrimSpace(value) ||
		!utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n\t\u2028\u2029") {
		return ErrInvalidRegistry
	}
	return nil
}

func validProtectedMetadata(value string, maximum int, allowEmpty bool) bool {
	return validProtectedLocalString(value, maximum, allowEmpty) == nil
}

func validPortableCorrelation(value string) bool {
	if strings.HasPrefix(value, "provider-") {
		return len(value) > len("provider-") && value == strings.TrimSpace(value) && validDigits(value[len("provider-"):])
	}
	if strings.HasPrefix(value, "account-") {
		return len(value) > len("account-") && value == strings.TrimSpace(value) && validDigits(value[len("account-"):])
	}
	return false
}

func validPortableAccountCorrelation(value string) bool {
	return strings.HasPrefix(value, "account-") && validPortableCorrelation(value)
}

func validProtectedRecoveryCorrelationOrder(entries []ProtectedRecoveryEntryV1) bool {
	correlations := make([]string, len(entries))
	for index, entry := range entries {
		correlations[index] = entry.Correlation
	}
	return validProtectedRecoveryCorrelationNames(correlations)
}

func validProtectedRecoveryCorrelationOrderFromBundle(entries []ProtectedRecoveryBundleEntryV1) bool {
	correlations := make([]string, len(entries))
	for index, entry := range entries {
		correlations[index] = entry.Correlation
	}
	return validProtectedRecoveryCorrelationNames(correlations)
}

func validProtectedRecoveryCorrelationOrderFromReceipt(entries []ProtectedRecoveryReceiptEntryV1) bool {
	correlations := make([]string, len(entries))
	for index, entry := range entries {
		correlations[index] = entry.Correlation
	}
	return validProtectedRecoveryCorrelationNames(correlations)
}

func validProtectedRecoveryCorrelationNames(correlations []string) bool {
	providerIndex, accountIndex := 0, 0
	accountsStarted := false
	for _, correlation := range correlations {
		switch {
		case strings.HasPrefix(correlation, "provider-"):
			if accountsStarted || correlation != "provider-"+strconv.Itoa(providerIndex) {
				return false
			}
			providerIndex++
		case strings.HasPrefix(correlation, "account-"):
			accountsStarted = true
			if correlation != "account-"+strconv.Itoa(accountIndex) {
				return false
			}
			accountIndex++
		default:
			return false
		}
	}
	return true
}

func validDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range value {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func validOperationID(value string) bool {
	return len(value) >= 8 && len(value) <= 128 && validProtectedMetadata(value, 128, false) &&
		!strings.ContainsAny(value, "/\\")
}

func validNonce(value string) bool {
	return len(value) >= 16 && len(value) <= 128 && validProtectedMetadata(value, 128, false) &&
		!strings.ContainsAny(value, "/\\")
}

func validFingerprint(value string) bool {
	if len(value) != 16 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validExpiry(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.UTC().Format(time.RFC3339Nano) == value
}
