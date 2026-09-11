package providerregistry

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	// These names are intentionally operation-specific.  The general Provider
	// and account execution consumers are never accepted for protected
	// recovery records.
	ProtectedTransferSourceConsumer     = "protected-recovery-source-transfer"
	ProtectedRecoveryReadbackConsumer   = "protected-recovery-readback"
	ProtectedRecoveryPendingKeyConsumer = "protected-recovery-pending-key"
	ProtectedRecoveryPendingKeyPurpose  = secretstoreport.Purpose("protected-recovery-pending-key")
	protectedRecoveryHKDFInfo           = "analytix.provider-protected-recovery/hkdf/v1"
	protectedRecoveryReceiptAuthInfo    = "analytix.provider-protected-recovery/receipt-auth/v1"
	protectedRecoveryExpiryWindow       = 15 * time.Minute
	protectedRecoveryMaxPlaintext       = 1 << 20
	protectedRecoveryCleanupCandidates  = "candidates"
	protectedRecoveryCleanupKey         = "destination-key"
	protectedRecoveryCleanupAll         = "all"
)

var (
	errProtectedRecoveryNotFound     = errors.New("protected recovery: session not found")
	errProtectedRecoveryConsumed     = errors.New("protected recovery: session already consumed")
	errProtectedRecoveryNotConfirmed = errors.New("protected recovery: destination confirmation required")
	errProtectedRecoveryCurrentness  = errors.New("protected recovery: currentness check failed")
	errProtectedRecoveryProtocol     = errors.New("protected recovery: protocol validation failed")
)

type ProtectedRecoveryRequestCommand struct {
	ManifestJSON          []byte
	ImportResult          ProtectedRecoveryImportResult
	LocalBinding          domainregistry.ProtectedRecoveryLocalBindingV1
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1
}

// ProtectedRecoveryImportResult is an operation-specific private receipt. It
// extends the ordinary key-free correlation result with destination-minted
// fences and Main-admitted private-account bindings without widening the
// ordinary/public portable-import response.
type ProtectedRecoveryImportResult struct {
	ProviderCount   int                                  `json:"providerCount"`
	AccountCount    int                                  `json:"accountCount"`
	ReentryRequired int                                  `json:"reentryRequired"`
	Entries         []ProtectedRecoveryImportEntryResult `json:"entries"`
}

type ProtectedRecoveryImportEntryResult struct {
	Correlation             string                                          `json:"correlation"`
	DestinationProviderID   string                                          `json:"destinationProviderId"`
	Status                  string                                          `json:"status"`
	Fence                   ProtectedRecoveryImportFence                    `json:"fence"`
	DestinationOwnerBinding *domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"destinationOwnerBinding,omitempty"`
}

type ProtectedRecoveryImportFence struct {
	Revision    string `json:"revision"`
	Generation  string `json:"generation"`
	Incarnation string `json:"incarnation"`
}

type ProtectedRecoveryDestinationConfirmationCommand struct {
	Request               []byte
	LocalAction           domainregistry.ProtectedRecoveryLocalActionV1
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1
}

type ProtectedRecoveryBundleCommand struct {
	Request               []byte
	LocalAction           domainregistry.ProtectedRecoveryLocalActionV1
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1
}

type ProtectedRecoveryApplyCommand struct {
	Bundle                []byte
	Request               []byte
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1
}

type ProtectedRecoveryFinalizeCommand struct {
	Receipt []byte
}

type ProtectedRecoveryRollbackCommand struct {
	Request []byte
}

// ProtectedRecoveryRecoverCommand is an optional, exact continuation selector.
// An empty command keeps the maintenance/status-only recovery path; a
// non-empty Request requires the current Main action and owner inventory and
// may return the already authenticated, key-free receipt artifact.
type ProtectedRecoveryRecoverCommand struct {
	Request               []byte
	LocalAction           domainregistry.ProtectedRecoveryLocalActionV1
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1
}

type protectedRecoveryPayload struct {
	Entries []protectedRecoveryPayloadEntry `json:"entries"`
}

type protectedRecoveryPayloadEntry struct {
	Correlation           string `json:"correlation"`
	DestinationProviderID string `json:"destinationProviderId"`
	Purpose               string `json:"purpose"`
	Secret                []byte `json:"secret"`
}

// PrepareProtectedRecoveryRequest validates the exact Main-only import receipt
// against the destination Registry before generating or persisting any
// protected key. The caller supplies admitted fences/bindings, never generated
// session or key authority.
func (manager *Manager) PrepareProtectedRecoveryRequest(
	ctx context.Context,
	command ProtectedRecoveryRequestCommand,
) (domainregistry.ProtectedRecoveryPreparedRequestV1, error) {
	var result domainregistry.ProtectedRecoveryPreparedRequestV1
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		domainregistry.ProtectedRecoveryLocalBindingV1(command.LocalBinding).Validate() != nil {
		return result, registryport.ErrInvalidRequest
	}
	manifest, err := domainregistry.ParsePortableManifestV1(command.ManifestJSON)
	if err != nil {
		return result, registryport.ErrInvalidRequest
	}
	manifestDigest := domainregistry.ProtectedRecoveryManifestDigest(command.ManifestJSON)
	if manifestDigest == "" {
		return result, registryport.ErrInvalidRequest
	}
	if err := validateProtectedOwnerInventory(command.OwnerBindingInventory); err != nil {
		return result, registryport.ErrInvalidRequest
	}

	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		entries, admittedOwners, validateErr := protectedRecoveryImportEntries(
			state, manifest, command.ImportResult, command.OwnerBindingInventory,
		)
		if validateErr != nil {
			return validateErr
		}
		if err := manager.recoverProtectedRecoverySessions(ctx, storage); err != nil {
			return err
		}
		state, loadErr = storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if len(state.Transactions) != 0 || portableManifestHasNonTerminalRecovery(state) ||
			len(state.ProtectedRecoverySessions) >= domainregistry.ProtectedRecoveryMaxEntries {
			return registryport.ErrConflict
		}
		entries, admittedOwners, validateErr = protectedRecoveryImportEntries(
			state, manifest, command.ImportResult, command.OwnerBindingInventory,
		)
		if validateErr != nil {
			return validateErr
		}
		priorProviders, err := protectedRecoveryPriorProviders(state, entries)
		if err != nil {
			return err
		}
		if state.Revision == ^uint64(0) {
			return registryport.ErrConflict
		}

		operationID, nonce, expiry, err := newProtectedRecoverySessionMaterial()
		if err != nil {
			return registryport.ErrPersistence
		}
		privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return registryport.ErrPersistence
		}
		request := domainregistry.ProtectedRecoveryRequestV1{
			Schema:                        domainregistry.ProtectedRecoveryRequestSchemaV1,
			ProtocolVersion:               domainregistry.ProtectedRecoveryProtocolVersion,
			ManifestDigest:                manifestDigest,
			OperationID:                   operationID,
			SessionNonce:                  nonce,
			ExpiresAt:                     expiry,
			DestinationEphemeralPublicKey: privateKey.PublicKey().Bytes(),
			Entries:                       entries,
		}
		requestDigest := requestDigestWithoutFingerprint(request)
		if requestDigest == "" {
			return registryport.ErrInvalidRequest
		}
		request.VerificationFingerprint = domainregistry.ProtectedRecoveryFingerprint(requestDigest)
		requestBytes, err := domainregistry.MarshalProtectedRecoveryRequestV1(request)
		if err != nil {
			return registryport.ErrInvalidRequest
		}
		parsed, err := domainregistry.ParseProtectedRecoveryRequestV1(requestBytes)
		if err != nil || parsed.RequestDigest != requestDigest {
			return registryport.ErrInvalidRequest
		}
		itemSetDigest := domainregistry.ProtectedRecoveryItemSetDigest(entries)
		if itemSetDigest == "" {
			return registryport.ErrInvalidRequest
		}

		privateKeyBytes := privateKey.Bytes()
		defer clear(privateKeyBytes)
		candidate, err := manager.secrets.PreparePut(ctx, ProtectedRecoveryPendingKeyPurpose, privateKeyBytes)
		if err != nil {
			return normalizeSecretError(err)
		}
		candidateRef := candidate.CredentialRef()
		if secretstoreport.ValidateCredentialRef(candidateRef) != nil {
			candidate.Abort()
			return registryport.ErrVerification
		}
		defer candidate.Abort()

		next := state.Clone()
		if next.ProtectedRecoverySessions == nil {
			next.ProtectedRecoverySessions = make(map[string]domainregistry.ProtectedRecoverySessionV1)
		}
		next.ProtectedRecoverySessions[operationID] = domainregistry.ProtectedRecoverySessionV1{
			Version:                  domainregistry.ProtectedRecoveryProtocolVersion,
			OperationID:              operationID,
			Phase:                    domainregistry.ProtectedRecoveryPhasePendingKeyCandidate,
			Request:                  bytes.Clone(requestBytes),
			ManifestJSON:             bytes.Clone(command.ManifestJSON),
			RequestDigest:            requestDigest,
			ManifestDigest:           manifestDigest,
			ItemSetDigest:            itemSetDigest,
			SessionNonce:             nonce,
			ExpiresAt:                expiry,
			DestinationKeyRef:        string(candidateRef),
			DestinationKeyPurpose:    string(ProtectedRecoveryPendingKeyPurpose),
			Entries:                  slices.Clone(entries),
			OwnerBindings:            cloneProtectedOwnerBindings(admittedOwners),
			LocalBinding:             command.LocalBinding,
			PriorProviders:           priorProviders,
			PriorRegistryRevision:    state.Revision,
			PriorRegistryIncarnation: state.Incarnation,
			PriorSelectedProviderID:  state.SelectedProviderID,
		}
		if next.Revision == ^uint64(0) {
			return registryport.ErrConflict
		}
		next.Revision++
		if next.Validate() != nil {
			return registryport.ErrInvalidRequest
		}
		if err := storage.Commit(ctx, next); err != nil {
			// The candidate has not been committed, so Abort is the only
			// appropriate cleanup. Do not issue a second Registry commit after a
			// failed first journal publication; there is no durable session to
			// which a retryable cleanup reference could be attached.
			return normalizeRegistryStoreError(err)
		}

		if err := candidate.Commit(ctx); err != nil {
			return manager.failProtectedRecoveryKeyCandidate(ctx, storage, operationID, normalizeSecretError(err))
		}
		if err := manager.readProtectedPendingKeyExact(ctx, string(candidateRef), privateKeyBytes); err != nil {
			return manager.failProtectedRecoveryKeyCandidate(ctx, storage, operationID, err)
		}

		state, loadErr = storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		pendingSession, pendingOK := state.ProtectedRecoverySessions[operationID]
		if !pendingOK || pendingSession.Phase != domainregistry.ProtectedRecoveryPhasePendingKeyCandidate ||
			pendingSession.DestinationKeyRef != string(candidateRef) {
			return registryport.ErrVerification
		}
		pendingState := state.Clone()
		pendingSession.Phase = domainregistry.ProtectedRecoveryPhasePending
		pendingState.ProtectedRecoverySessions[operationID] = pendingSession
		if err := protectedRecoveryIncrementRevision(&pendingState); err != nil || pendingState.Validate() != nil {
			return registryport.ErrVerification
		}
		if err := storage.Commit(ctx, pendingState); err != nil {
			// The key and its exact journal authority remain in the
			// pending_key_candidate phase for recoverAll to clean; no request is
			// returned from this failed publication.
			return normalizeRegistryStoreError(err)
		}
		result = domainregistry.ProtectedRecoveryPreparedRequestV1{
			Request:            bytes.Clone(requestBytes),
			RequestDigest:      requestDigest,
			RequestFingerprint: request.VerificationFingerprint,
			ManifestDigest:     manifestDigest,
			ItemSetDigest:      itemSetDigest,
			OperationID:        operationID,
			SessionNonce:       nonce,
			ExpiresAt:          expiry,
		}
		return nil
	})
	if err != nil {
		return domainregistry.ProtectedRecoveryPreparedRequestV1{}, normalizeManagerError(err)
	}
	return result, nil
}

// ConfirmProtectedRecoveryDestination records the exact, fresh local action
// in the Manager-owned pending record.  It is intentionally a separate
// transition from request preparation.
func (manager *Manager) ConfirmProtectedRecoveryDestination(
	ctx context.Context,
	command ProtectedRecoveryDestinationConfirmationCommand,
) error {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return registryport.ErrInvalidRequest
	}
	request, err := domainregistry.ParseProtectedRecoveryRequestV1(command.Request)
	if err != nil || command.LocalAction.Validate() != nil {
		return registryport.ErrInvalidRequest
	}
	if err := validateProtectedContinuationOwnerInventory(command.OwnerBindingInventory); err != nil {
		return registryport.ErrInvalidRequest
	}
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		session, ok := state.ProtectedRecoverySessions[request.OperationID]
		if !ok || session.RequestDigest != request.RequestDigest || !bytes.Equal(session.Request, command.Request) {
			return errProtectedRecoveryNotFound
		}
		if session.Phase != domainregistry.ProtectedRecoveryPhasePending || session.Confirmed || session.Consumed {
			return registryport.ErrConflict
		}
		if time.Now().UTC().After(parseProtectedExpiry(session.ExpiresAt)) {
			return registryport.ErrConflict
		}
		if !protectedRecoveryConfirmationBindingMatches(command.LocalAction, session) ||
			!protectedConfirmationMatches(command.LocalAction.Confirmation, session) ||
			!protectedOwnerBindingsMatchSession(session.OwnerBindings, command.OwnerBindingInventory) {
			return registryport.ErrConflict
		}
		if err := protectedRecoveryEntriesCurrent(state, session.Entries); err != nil {
			return err
		}
		next := state.Clone()
		updated := next.ProtectedRecoverySessions[request.OperationID]
		updated.Confirmed = true
		updated.ReconfirmationRequired = false
		updated.LocalBinding = command.LocalAction.LocalBinding
		updated.Phase = domainregistry.ProtectedRecoveryPhaseConfirmed
		next.ProtectedRecoverySessions[request.OperationID] = updated
		if next.Validate() != nil {
			return registryport.ErrInvalidRequest
		}
		if err := storage.Commit(ctx, next); err != nil {
			return normalizeRegistryStoreError(err)
		}
		manager.rememberProtectedRecoveryConfirmation(request.OperationID)
		return nil
	})
	return normalizeManagerError(err)
}

// CreateProtectedRecoveryBundle derives source owners from the source
// Registry's canonical export and reads credentials only through the private
// protected-transfer consumer.
func (manager *Manager) CreateProtectedRecoveryBundle(
	ctx context.Context,
	command ProtectedRecoveryBundleCommand,
) ([]byte, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return nil, registryport.ErrInvalidRequest
	}
	request, err := domainregistry.ParseProtectedRecoveryRequestV1(command.Request)
	if err != nil || command.LocalAction.Validate() != nil {
		return nil, registryport.ErrInvalidRequest
	}
	if err := validateProtectedOwnerInventory(command.OwnerBindingInventory); err != nil {
		return nil, registryport.ErrInvalidRequest
	}
	if !protectedActionMatchesRequest(command.LocalAction, request) {
		return nil, registryport.ErrConflict
	}
	manifestBytes, err := manager.ExportPortableManifest(ctx)
	if err != nil {
		return nil, err
	}
	manifest, err := domainregistry.ParsePortableManifestV1(manifestBytes)
	if err != nil || domainregistry.ProtectedRecoveryManifestDigest(manifestBytes) != request.ManifestDigest {
		return nil, registryport.ErrConflict
	}
	if expiry := parseProtectedExpiry(request.ExpiresAt); expiry.IsZero() || time.Now().UTC().After(expiry) {
		return nil, registryport.ErrConflict
	}

	var sourceState domainregistry.Registry
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		// A newly constructed Manager has no local confirmation authority. It
		// therefore downgrades a persisted confirmed session before any bundle
		// work; the process that recorded the fresh confirmation remembers the
		// operation until this apply consumes it.
		if err := manager.recoverProtectedRecoverySessions(ctx, storage); err != nil {
			return err
		}
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		if _, exists := state.ProtectedRecoverySessions[request.OperationID]; exists {
			return registryport.ErrConflict
		}
		sourceState = state
		return nil
	})
	if err != nil {
		return nil, normalizeManagerError(err)
	}
	owners, err := sourceProtectedOwners(sourceState, manifest)
	if err != nil {
		return nil, err
	}
	if err := validateSourceOwnerInventory(owners, command.OwnerBindingInventory); err != nil {
		return nil, err
	}

	payloadEntries := make([]protectedRecoveryPayloadEntry, 0, len(request.Entries))
	defer func() {
		for index := range payloadEntries {
			clear(payloadEntries[index].Secret)
		}
	}()
	for _, requestEntry := range request.Entries {
		provider, purpose, err := sourceProviderForCorrelation(sourceState, manifest, requestEntry.Correlation)
		if err != nil {
			return nil, err
		}
		if !protectedPurposeAllowed(purpose) || provider.CredentialRef == "" || provider.CredentialPurpose != purpose {
			return nil, registryport.ErrConflict
		}
		secret, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
			CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
			Purpose:       secretstoreport.Purpose(purpose),
			Consumer:      ProtectedTransferSourceConsumer,
		})
		if readErr != nil {
			clear(secret)
			return nil, normalizeSecretError(readErr)
		}
		if len(secret) == 0 || len(secret) > protectedRecoveryMaxPlaintext {
			clear(secret)
			return nil, registryport.ErrVerification
		}
		payloadEntries = append(payloadEntries, protectedRecoveryPayloadEntry{
			Correlation: requestEntry.Correlation, DestinationProviderID: requestEntry.DestinationProviderID,
			Purpose: purpose, Secret: bytes.Clone(secret),
		})
		clear(secret)
	}

	destinationPublic, err := ecdh.X25519().NewPublicKey(request.DestinationEphemeralPublicKey)
	if err != nil {
		return nil, registryport.ErrInvalidRequest
	}
	sourcePrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, registryport.ErrPersistence
	}
	shared, err := sourcePrivate.ECDH(destinationPublic)
	if err != nil {
		return nil, registryport.ErrVerification
	}
	key, err := protectedRecoveryHKDF(shared, request.RequestDigest)
	clear(shared)
	if err != nil {
		return nil, registryport.ErrVerification
	}
	defer clear(key)
	payloadBytes, err := marshalProtectedRecoveryPayload(payloadEntries)
	for index := range payloadEntries {
		clear(payloadEntries[index].Secret)
	}
	if err != nil {
		return nil, registryport.ErrInvalidRequest
	}
	aead, err := aes.NewCipher(key)
	if err != nil {
		clear(payloadBytes)
		return nil, registryport.ErrVerification
	}
	gcm, err := cipher.NewGCM(aead)
	if err != nil {
		clear(payloadBytes)
		return nil, registryport.ErrVerification
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		clear(payloadBytes)
		return nil, registryport.ErrPersistence
	}
	bundleMeta := domainregistry.ProtectedRecoveryBundleV1{
		Schema: domainregistry.ProtectedRecoveryBundleSchemaV1, ProtocolVersion: domainregistry.ProtectedRecoveryProtocolVersion,
		RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest, OperationID: request.OperationID,
		SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt, SourceEphemeralPublicKey: sourcePrivate.PublicKey().Bytes(),
		Nonce: nonce, Entries: protectedRecoveryBundleEntries(request, payloadEntries),
	}
	aad := protectedRecoveryAAD(request, bundleMeta)
	bundleMeta.Ciphertext = gcm.Seal(nil, nonce, payloadBytes, aad)
	clear(payloadBytes)
	bundleBytes, err := domainregistry.MarshalProtectedRecoveryBundleV1(bundleMeta)
	if err != nil {
		return nil, registryport.ErrInvalidRequest
	}
	expectedReceiptRecord := domainregistry.ProtectedRecoveryReceiptV1{
		Schema:          domainregistry.ProtectedRecoveryReceiptSchemaV1,
		ProtocolVersion: domainregistry.ProtectedRecoveryProtocolVersion,
		RequestDigest:   request.RequestDigest, ManifestDigest: request.ManifestDigest,
		BundleDigest: domainregistry.ProtectedRecoveryBundleDigest(bundleBytes),
		OperationID:  request.OperationID, SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt,
		Entries:           protectedRecoveryReceiptEntries(request),
		AuthenticationTag: make([]byte, 32),
	}
	expectedReceiptRecord.AuthenticationTag, err = protectedRecoveryReceiptAuthenticationTag(
		key, request, bundleMeta, expectedReceiptRecord,
	)
	if err != nil {
		return nil, registryport.ErrVerification
	}
	expectedReceipt, err := domainregistry.MarshalProtectedRecoveryReceiptV1(expectedReceiptRecord)
	if err != nil {
		return nil, registryport.ErrInvalidRequest
	}
	// Persist only the source operation/session transcript.  Source Provider,
	// account, credential, and selection authority remain untouched.
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		if _, ok := state.ProtectedRecoverySessions[request.OperationID]; ok {
			return registryport.ErrConflict
		}
		if !protectedRecoverySourceStateCurrent(state, sourceState, manifest, request, owners) {
			return errProtectedRecoveryCurrentness
		}
		next := state.Clone()
		if next.ProtectedRecoverySessions == nil {
			next.ProtectedRecoverySessions = make(map[string]domainregistry.ProtectedRecoverySessionV1)
		}
		next.ProtectedRecoverySessions[request.OperationID] = domainregistry.ProtectedRecoverySessionV1{
			Version: domainregistry.ProtectedRecoveryProtocolVersion, OperationID: request.OperationID,
			Phase: domainregistry.ProtectedRecoveryPhaseSourceBundle, Request: bytes.Clone(command.Request),
			ManifestJSON:  bytes.Clone(manifestBytes),
			RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest,
			ItemSetDigest: domainregistry.ProtectedRecoveryItemSetDigest(request.Entries), SessionNonce: request.SessionNonce,
			ExpiresAt: request.ExpiresAt, Entries: slices.Clone(request.Entries),
			OwnerBindings: cloneProtectedOwnerBindings(command.OwnerBindingInventory), LocalBinding: command.LocalAction.LocalBinding,
			Bundle: bytes.Clone(bundleBytes), BundleDigest: domainregistry.ProtectedRecoveryBundleDigest(bundleBytes),
			Confirmed: true, Receipt: bytes.Clone(expectedReceipt),
		}
		if next.Revision == ^uint64(0) {
			return registryport.ErrConflict
		}
		next.Revision++
		if next.Validate() != nil {
			return registryport.ErrInvalidRequest
		}
		return normalizeRegistryStoreError(storage.Commit(ctx, next))
	})
	if err != nil {
		return nil, normalizeManagerError(err)
	}
	return bundleBytes, nil
}

// ApplyProtectedRecoveryBundle performs all logical preflight and transcript
// authentication before the first destination candidate is prepared.
func (manager *Manager) ApplyProtectedRecoveryBundle(
	ctx context.Context,
	command ProtectedRecoveryApplyCommand,
) ([]byte, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return nil, registryport.ErrInvalidRequest
	}
	bundle, err := domainregistry.ParseProtectedRecoveryBundleV1(command.Bundle)
	if err != nil {
		return nil, registryport.ErrInvalidRequest
	}
	if err := validateProtectedContinuationOwnerInventory(command.OwnerBindingInventory); err != nil {
		return nil, registryport.ErrInvalidRequest
	}
	var result []byte
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		preRecoverySession, ok := state.ProtectedRecoverySessions[bundle.OperationID]
		if !ok {
			return errProtectedRecoveryNotFound
		}
		preRecoveryRequestBytes := preRecoverySession.Request
		if len(command.Request) != 0 {
			if !bytes.Equal(preRecoverySession.Request, command.Request) {
				return errProtectedRecoveryProtocol
			}
			preRecoveryRequestBytes = command.Request
		}
		preRecoveryRequest, parseErr := domainregistry.ParseProtectedRecoveryRequestV1(preRecoveryRequestBytes)
		if parseErr != nil || preRecoveryRequest.RequestDigest != preRecoverySession.RequestDigest ||
			preRecoveryRequest.ManifestDigest != preRecoverySession.ManifestDigest ||
			preRecoveryRequest.OperationID != preRecoverySession.OperationID ||
			preRecoveryRequest.SessionNonce != preRecoverySession.SessionNonce ||
			preRecoveryRequest.ExpiresAt != preRecoverySession.ExpiresAt ||
			!reflect.DeepEqual(preRecoveryRequest.Entries, preRecoverySession.Entries) {
			return errProtectedRecoveryProtocol
		}
		preRecoveryOwners, ownersMatch := protectedOwnerBindingsForSession(
			preRecoverySession.OwnerBindings, command.OwnerBindingInventory,
		)
		if !ownersMatch {
			return errProtectedRecoveryCurrentness
		}
		if err := protectedRecoveryPreflightBundle(
			state, preRecoveryRequest, bundle, preRecoverySession, preRecoveryOwners,
		); err != nil {
			return err
		}
		// A persisted confirmation is local process authority, not a durable
		// bearer capability. Recover first so a newly constructed Manager
		// downgrades an old confirmed session and requires a fresh confirmation
		// before any candidate preparation.
		if err := manager.recoverProtectedRecoverySessions(ctx, storage); err != nil {
			return err
		}
		state, loadErr = storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		session, ok := state.ProtectedRecoverySessions[bundle.OperationID]
		if !ok {
			return errProtectedRecoveryNotFound
		}
		if session.Phase != domainregistry.ProtectedRecoveryPhaseConfirmed || !session.Confirmed || session.Consumed {
			return errProtectedRecoveryConsumed
		}
		if parseProtectedExpiry(session.ExpiresAt).IsZero() || time.Now().UTC().After(parseProtectedExpiry(session.ExpiresAt)) {
			return registryport.ErrConflict
		}
		if bundle.RequestDigest != session.RequestDigest || bundle.ManifestDigest != session.ManifestDigest ||
			bundle.SessionNonce != session.SessionNonce || bundle.ExpiresAt != session.ExpiresAt ||
			!bytes.Equal(session.Request, command.Request) && len(command.Request) != 0 {
			return errProtectedRecoveryProtocol
		}
		requestBytes := session.Request
		if len(command.Request) != 0 {
			requestBytes = command.Request
		}
		request, parseErr := domainregistry.ParseProtectedRecoveryRequestV1(requestBytes)
		if parseErr != nil || request.RequestDigest != session.RequestDigest || request.ManifestDigest != session.ManifestDigest ||
			request.OperationID != session.OperationID || request.SessionNonce != session.SessionNonce ||
			request.ExpiresAt != session.ExpiresAt || !reflect.DeepEqual(request.Entries, session.Entries) {
			return errProtectedRecoveryProtocol
		}
		normalizedOwners, ownersMatch := protectedOwnerBindingsForSession(session.OwnerBindings, command.OwnerBindingInventory)
		if !ownersMatch {
			return errProtectedRecoveryCurrentness
		}
		if err := protectedRecoveryPreflightBundle(state, request, bundle, session, normalizedOwners); err != nil {
			return err
		}
		if session.DestinationKeyRef == "" || session.DestinationKeyPurpose != string(ProtectedRecoveryPendingKeyPurpose) {
			return registryport.ErrVerification
		}
		// The source public key and destination key are authenticated through
		// the transcript. Reject malformed key lengths before any K1 read; this
		// keeps obvious hostile records pre-mutation.
		if len(bundle.SourceEphemeralPublicKey) != 32 || len(request.DestinationEphemeralPublicKey) != 32 {
			return registryport.ErrInvalidRequest
		}
		privateBytes, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
			CredentialRef: secretstoreport.CredentialRef(session.DestinationKeyRef),
			Purpose:       secretstoreport.Purpose(session.DestinationKeyPurpose), Consumer: ProtectedRecoveryPendingKeyConsumer,
		})
		if readErr != nil {
			clear(privateBytes)
			return normalizeSecretError(readErr)
		}
		if len(privateBytes) != 32 {
			clear(privateBytes)
			return registryport.ErrVerification
		}
		privateKey, keyErr := ecdh.X25519().NewPrivateKey(privateBytes)
		candidateDigestKey := bytes.Clone(privateBytes)
		clear(privateBytes)
		if keyErr != nil || len(candidateDigestKey) != 32 || !bytes.Equal(privateKey.PublicKey().Bytes(), request.DestinationEphemeralPublicKey) {
			clear(candidateDigestKey)
			return registryport.ErrVerification
		}
		defer clear(candidateDigestKey)
		sourcePublic, keyErr := ecdh.X25519().NewPublicKey(bundle.SourceEphemeralPublicKey)
		if keyErr != nil {
			return registryport.ErrVerification
		}
		shared, keyErr := privateKey.ECDH(sourcePublic)
		if keyErr != nil {
			return registryport.ErrVerification
		}
		key, keyErr := protectedRecoveryHKDF(shared, request.RequestDigest)
		clear(shared)
		if keyErr != nil {
			return registryport.ErrVerification
		}
		defer clear(key)
		aead, keyErr := aes.NewCipher(key)
		if keyErr != nil {
			return registryport.ErrVerification
		}
		gcm, keyErr := cipher.NewGCM(aead)
		if keyErr != nil || len(bundle.Nonce) != gcm.NonceSize() {
			return registryport.ErrVerification
		}
		plaintext, openErr := gcm.Open(nil, bundle.Nonce, bundle.Ciphertext, protectedRecoveryAAD(request, bundle))
		if openErr != nil || len(plaintext) == 0 || len(plaintext) > protectedRecoveryMaxPlaintext {
			clear(plaintext)
			return registryport.ErrVerification
		}
		payload, parseErr := parseProtectedRecoveryPayload(plaintext)
		clear(plaintext)
		if parseErr != nil || !protectedPayloadMatchesBundle(payload, bundle) || len(payload.Entries) != len(request.Entries) {
			return registryport.ErrVerification
		}
		defer func() {
			for index := range payload.Entries {
				clear(payload.Entries[index].Secret)
			}
		}()
		for index := range payload.Entries {
			if len(payload.Entries[index].Secret) == 0 || len(payload.Entries[index].Secret) > protectedRecoveryMaxPlaintext {
				return registryport.ErrInvalidRequest
			}
		}

		// Candidate preparation is deliberately split from Registry publication.
		// The durable CandidatesDurable phase names every app-owned candidate and
		// its transcript-bound digest before any Provider becomes executable. A
		// restart can therefore clean/retry these exact refs without guessing.
		originalState := state.Clone()
		candidates := make([]secretstoreport.PreparedCandidate, 0, len(payload.Entries))
		candidateRefs := make([]string, 0, len(payload.Entries))
		candidatePurposes := make([]string, 0, len(payload.Entries))
		candidateDigests := make([]string, 0, len(payload.Entries))
		abortPrepared := func() {
			for _, candidate := range candidates {
				candidate.Abort()
			}
		}
		for index, payloadEntry := range payload.Entries {
			candidate, prepareErr := manager.secrets.PreparePut(ctx, secretstoreport.Purpose(payloadEntry.Purpose), payloadEntry.Secret)
			if prepareErr != nil {
				abortPrepared()
				return normalizeSecretError(prepareErr)
			}
			if secretstoreport.ValidateCredentialRef(candidate.CredentialRef()) != nil {
				candidate.Abort()
				abortPrepared()
				return registryport.ErrVerification
			}
			candidates = append(candidates, candidate)
			candidateRefs = append(candidateRefs, string(candidate.CredentialRef()))
			candidatePurposes = append(candidatePurposes, payloadEntry.Purpose)
			candidateDigests = append(candidateDigests, protectedRecoveryCandidateDigest(request, bundle, bundle.Entries[index], payloadEntry.Secret, candidateDigestKey))
		}

		candidateState := state.Clone()
		candidateSession := candidateState.ProtectedRecoverySessions[request.OperationID]
		candidateSession.Phase = domainregistry.ProtectedRecoveryPhaseCandidatesDurable
		candidateSession.CandidateRefs = append([]string(nil), candidateRefs...)
		candidateSession.CandidatePurposes = append([]string(nil), candidatePurposes...)
		candidateSession.CandidateDigests = append([]string(nil), candidateDigests...)
		candidateSession.Bundle = bytes.Clone(command.Bundle)
		candidateSession.BundleDigest = domainregistry.ProtectedRecoveryBundleDigest(command.Bundle)
		candidateState.ProtectedRecoverySessions[request.OperationID] = candidateSession
		if err := protectedRecoveryIncrementRevision(&candidateState); err != nil || candidateState.Validate() != nil {
			abortPrepared()
			return registryport.ErrVerification
		}
		if commitErr := storage.Commit(ctx, candidateState); commitErr != nil {
			abortPrepared()
			return normalizeRegistryStoreError(commitErr)
		}
		rollbackCandidates := func(reason error) error {
			abortPrepared()
			if cleanupErr := manager.cleanupProtectedReferences(ctx, candidateRefs, candidatePurposes); cleanupErr != nil {
				// Keep the durable ref list when cleanup is not verified. Recovery
				// can retry the exact tombstone/delete sequence on the next access.
				if durableErr := manager.persistProtectedRollbackPending(ctx, storage, candidateState, candidateSession,
					candidateRefs, candidatePurposes, candidateDigests, command.Bundle); durableErr != nil {
					return durableErr
				}
				return cleanupErr
			}
			// A synchronously rejected candidate batch has not changed Provider
			// authority. Restore the exact pre-apply logical Registry snapshot;
			// if this CAS fails, the durable candidate phase above remains the
			// retry authority.
			if restoreErr := storage.Commit(ctx, originalState); restoreErr != nil {
				return normalizeRegistryStoreError(restoreErr)
			}
			return reason
		}

		// Commit and byte-compare every candidate before creating a Registry
		// winner. A candidate commit/readback failure is rolled back from the
		// durable phase; cleanup errors leave that phase retryable and fail closed.
		for index, candidate := range candidates {
			if commitErr := candidate.Commit(ctx); commitErr != nil {
				return rollbackCandidates(normalizeSecretError(commitErr))
			}
			if readbackErr := manager.readProtectedCandidateExact(ctx, candidateRefs[index], candidatePurposes[index], payload.Entries[index].Secret); readbackErr != nil {
				return rollbackCandidates(readbackErr)
			}
		}

		state, loadErr = storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		session, ok = state.ProtectedRecoverySessions[request.OperationID]
		if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseCandidatesDurable || !protectedRecoverySessionCandidateRefsValid(session) {
			return registryport.ErrVerification
		}
		next := state.Clone()
		if err := applyProtectedPayloadToRegistry(&next, request, payload, candidateRefs, session.OwnerBindings); err != nil {
			return rollbackCandidates(err)
		}
		bundleDigest := domainregistry.ProtectedRecoveryBundleDigest(command.Bundle)
		receipt := domainregistry.ProtectedRecoveryReceiptV1{
			Schema: domainregistry.ProtectedRecoveryReceiptSchemaV1, ProtocolVersion: domainregistry.ProtectedRecoveryProtocolVersion,
			RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest, BundleDigest: bundleDigest,
			OperationID: request.OperationID, SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt,
			Entries: protectedRecoveryReceiptEntries(request), AuthenticationTag: make([]byte, 32),
		}
		receipt.AuthenticationTag, err = protectedRecoveryReceiptAuthenticationTag(
			key, request, bundle, receipt,
		)
		if err != nil {
			return rollbackCandidates(registryport.ErrVerification)
		}
		receiptBytes, marshalErr := domainregistry.MarshalProtectedRecoveryReceiptV1(receipt)
		if marshalErr != nil {
			return rollbackCandidates(registryport.ErrInvalidRequest)
		}
		updated := next.ProtectedRecoverySessions[request.OperationID]
		updated.Phase = domainregistry.ProtectedRecoveryPhaseVerificationPending
		updated.Confirmed = true
		updated.Consumed = false
		updated.CandidateRefs = append([]string(nil), candidateRefs...)
		updated.CandidatePurposes = append([]string(nil), candidatePurposes...)
		updated.CandidateDigests = append([]string(nil), candidateDigests...)
		updated.Bundle = bytes.Clone(command.Bundle)
		updated.BundleDigest = bundleDigest
		updated.Receipt = bytes.Clone(receiptBytes)
		if err := protectedRecoveryIncrementRevision(&next); err != nil {
			return rollbackCandidates(registryport.ErrVerification)
		}
		updated.BatchRegistryRevision = next.Revision
		next.ProtectedRecoverySessions[request.OperationID] = updated
		if next.Validate() != nil {
			return rollbackCandidates(registryport.ErrVerification)
		}
		if commitErr := storage.Commit(ctx, next); commitErr != nil {
			// The durable CandidatesDurable record remains the retry/rollback
			// authority if this verification-pending CAS did not publish.
			return normalizeRegistryStoreError(commitErr)
		}

		// The Provider batch is visible only as a verification-pending journal
		// state. Reload the exact successor, fences, owner replacement, and
		// operation-specific K1 bytes while the Registry lock is still held.
		state, loadErr = storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		session, ok = state.ProtectedRecoverySessions[request.OperationID]
		if !ok {
			return registryport.ErrVerification
		}
		if verifyErr := manager.verifyProtectedRecoverySuccessor(ctx, state, session); verifyErr != nil {
			return manager.restoreProtectedRecoveryAndCleanup(ctx, storage, session, verifyErr)
		}
		next = state.Clone()
		updated = next.ProtectedRecoverySessions[request.OperationID]
		updated.Phase = domainregistry.ProtectedRecoveryPhaseApplied
		updated.Confirmed = true
		updated.Consumed = true
		next.ProtectedRecoverySessions[request.OperationID] = updated
		if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
			return registryport.ErrVerification
		}
		if commitErr := storage.Commit(ctx, next); commitErr != nil {
			// recoverAll will verify and finish this exact successor on restart;
			// do not return a receipt until Applied and key cleanup are durable.
			return normalizeRegistryStoreError(commitErr)
		}
		if cleanupErr := manager.recoverProtectedKeyCleanup(ctx, storage, request.OperationID); cleanupErr != nil {
			return cleanupErr
		}
		manager.forgetProtectedRecoveryConfirmation(request.OperationID)
		result = receiptBytes
		return nil
	})
	if err != nil {
		return nil, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) FinalizeProtectedRecoveryReceipt(
	ctx context.Context,
	command ProtectedRecoveryFinalizeCommand,
) (domainregistry.ProtectedRecoveryResultV1, error) {
	var result domainregistry.ProtectedRecoveryResultV1
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return result, registryport.ErrInvalidRequest
	}
	receipt, err := domainregistry.ParseProtectedRecoveryReceiptV1(command.Receipt)
	if err != nil {
		return result, registryport.ErrInvalidRequest
	}
	for _, entry := range receipt.Entries {
		if entry.Status != "applied" {
			return result, registryport.ErrInvalidRequest
		}
	}
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		session, ok := state.ProtectedRecoverySessions[receipt.OperationID]
		if !ok || session.RequestDigest != receipt.RequestDigest || session.Phase != domainregistry.ProtectedRecoveryPhaseSourceBundle || session.Consumed ||
			!bytes.Equal(session.Receipt, command.Receipt) {
			return errProtectedRecoveryNotFound
		}
		next := state.Clone()
		updated := next.ProtectedRecoverySessions[receipt.OperationID]
		// A finalized source record is a redacted, key-free audit record. Keep
		// only bounded transcript digests, operation metadata, and the
		// correlation/status entries needed for audit; remove exact app-owned
		// request/manifest/bundle/receipt and local binding authority bytes.
		clear(updated.Request)
		clear(updated.ManifestJSON)
		clear(updated.Bundle)
		clear(updated.Receipt)
		updated.Request = nil
		updated.ManifestJSON = nil
		updated.Bundle = nil
		updated.Receipt = nil
		updated.OwnerBindings = nil
		updated.LocalBinding = domainregistry.ProtectedRecoveryLocalBindingV1{}
		updated.DestinationKeyRef = ""
		updated.DestinationKeyPurpose = ""
		updated.CandidateRefs = nil
		updated.CandidatePurposes = nil
		updated.CandidateDigests = nil
		updated.CleanupKind = ""
		updated.CleanupRefs = nil
		updated.CleanupPurposes = nil
		updated.PriorProviders = nil
		updated.PriorRegistryRevision = 0
		updated.PriorRegistryIncarnation = ""
		updated.PriorSelectedProviderID = ""
		updated.BatchRegistryRevision = 0
		updated.Phase = domainregistry.ProtectedRecoveryPhaseFinalized
		updated.Consumed = true
		next.ProtectedRecoverySessions[receipt.OperationID] = updated
		if next.Revision == ^uint64(0) {
			return registryport.ErrConflict
		}
		next.Revision++
		if next.Validate() != nil {
			return registryport.ErrInvalidRequest
		}
		if err := storage.Commit(ctx, next); err != nil {
			return normalizeRegistryStoreError(err)
		}
		result = domainregistry.ProtectedRecoveryResultV1{Status: "finalized"}
		return nil
	})
	if err != nil {
		return domainregistry.ProtectedRecoveryResultV1{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) RecoverProtectedRecovery(
	ctx context.Context,
	commands ...ProtectedRecoveryRecoverCommand,
) (domainregistry.ProtectedRecoveryResultV1, error) {
	var result domainregistry.ProtectedRecoveryResultV1
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil || len(commands) > 1 {
		return result, registryport.ErrInvalidRequest
	}
	var command ProtectedRecoveryRecoverCommand
	targeted := len(commands) == 1 && len(commands[0].Request) != 0
	if len(commands) == 1 {
		command = commands[0]
	}
	var request domainregistry.ProtectedRecoveryRequestV1
	var err error
	if targeted {
		request, err = domainregistry.ParseProtectedRecoveryRequestV1(command.Request)
		if err != nil || command.LocalAction.Validate() != nil ||
			validateProtectedContinuationOwnerInventory(command.OwnerBindingInventory) != nil {
			return result, registryport.ErrInvalidRequest
		}
	}
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if targeted {
			session, ok := state.ProtectedRecoverySessions[request.OperationID]
			if !ok || session.RequestDigest != request.RequestDigest || !bytes.Equal(session.Request, command.Request) {
				return errProtectedRecoveryNotFound
			}
			if expiry := parseProtectedExpiry(session.ExpiresAt); expiry.IsZero() || time.Now().UTC().After(expiry) {
				return registryport.ErrConflict
			}
			if !protectedOwnerBindingsMatchSession(session.OwnerBindings, command.OwnerBindingInventory) ||
				!protectedRecoveryActionMatchesSession(command.LocalAction, request, session, true) {
				return errProtectedRecoveryCurrentness
			}
			if err := protectedRecoveryTargetedSessionCurrent(state, session); err != nil {
				return err
			}
		}
		if err := manager.recoverProtectedRecoverySessions(ctx, storage); err != nil {
			return err
		}
		state, loadErr = storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if targeted {
			session, ok := state.ProtectedRecoverySessions[request.OperationID]
			if !ok || session.RequestDigest != request.RequestDigest || !bytes.Equal(session.Request, command.Request) {
				return errProtectedRecoveryNotFound
			}
			if expiry := parseProtectedExpiry(session.ExpiresAt); expiry.IsZero() || time.Now().UTC().After(expiry) {
				return registryport.ErrConflict
			}
			if !protectedOwnerBindingsMatchSession(session.OwnerBindings, command.OwnerBindingInventory) {
				return errProtectedRecoveryCurrentness
			}
			if !protectedRecoveryActionMatchesSession(command.LocalAction, request, session, true) {
				return errProtectedRecoveryCurrentness
			}
			switch session.Phase {
			case domainregistry.ProtectedRecoveryPhaseApplied:
				if err := manager.verifyProtectedRecoveryReceiptReady(state, session); err != nil {
					return err
				}
				result = domainregistry.ProtectedRecoveryResultV1{
					Status: "receipt_ready", Confirmed: true, Receipt: bytes.Clone(session.Receipt),
				}
				return nil
			case domainregistry.ProtectedRecoveryPhasePending:
				if session.ReconfirmationRequired || !session.Confirmed {
					result = domainregistry.ProtectedRecoveryResultV1{Status: "reconfirmation_required", Confirmed: false}
				} else {
					result = domainregistry.ProtectedRecoveryResultV1{Status: "pending", Confirmed: false}
				}
				return nil
			case domainregistry.ProtectedRecoveryPhaseConfirmed:
				result = domainregistry.ProtectedRecoveryResultV1{Status: "pending", Confirmed: true}
				return nil
			default:
				return errProtectedRecoveryConsumed
			}
		}

		next := state.Clone()
		changed := false
		for id, session := range next.ProtectedRecoverySessions {
			if session.Phase == domainregistry.ProtectedRecoveryPhaseConfirmed && !session.Consumed {
				session.Phase = domainregistry.ProtectedRecoveryPhasePending
				session.Confirmed = false
				session.ReconfirmationRequired = true
				next.ProtectedRecoverySessions[id] = session
				manager.forgetProtectedRecoveryConfirmation(id)
				changed = true
			}
		}
		if changed {
			if next.Revision == ^uint64(0) {
				return registryport.ErrConflict
			}
			next.Revision++
			if next.Validate() != nil {
				return registryport.ErrInvalidRequest
			}
			if err := storage.Commit(ctx, next); err != nil {
				return normalizeRegistryStoreError(err)
			}
			state = next
		}
		for _, session := range next.ProtectedRecoverySessions {
			if session.Phase == domainregistry.ProtectedRecoveryPhasePending || session.Phase == domainregistry.ProtectedRecoveryPhaseConfirmed {
				status := "pending"
				if session.ReconfirmationRequired {
					status = "reconfirmation_required"
				}
				result = domainregistry.ProtectedRecoveryResultV1{Status: status, Confirmed: session.Confirmed}
				return nil
			}
		}
		result = domainregistry.ProtectedRecoveryResultV1{Status: "none"}
		return nil
	})
	if err != nil {
		return domainregistry.ProtectedRecoveryResultV1{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) RollbackProtectedRecovery(
	ctx context.Context,
	command ProtectedRecoveryRollbackCommand,
) (domainregistry.ProtectedRecoveryResultV1, error) {
	var result domainregistry.ProtectedRecoveryResultV1
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return result, registryport.ErrInvalidRequest
	}
	request, err := domainregistry.ParseProtectedRecoveryRequestV1(command.Request)
	if err != nil {
		return result, registryport.ErrInvalidRequest
	}
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, loadErr := storage.Load(ctx)
		if loadErr != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		session, ok := state.ProtectedRecoverySessions[request.OperationID]
		if !ok || session.RequestDigest != request.RequestDigest || !bytes.Equal(session.Request, command.Request) {
			return errProtectedRecoveryNotFound
		}
		if session.Consumed || session.Phase == domainregistry.ProtectedRecoveryPhaseApplied || session.Phase == domainregistry.ProtectedRecoveryPhaseFinalized {
			return registryport.ErrConflict
		}
		cleanupRefs := make([]string, 0, len(session.CandidateRefs)+1)
		cleanupPurposes := make([]string, 0, len(session.CandidatePurposes)+1)
		if session.DestinationKeyRef != "" {
			cleanupRefs = append(cleanupRefs, session.DestinationKeyRef)
			cleanupPurposes = append(cleanupPurposes, session.DestinationKeyPurpose)
		}
		cleanupRefs = append(cleanupRefs, session.CandidateRefs...)
		cleanupPurposes = append(cleanupPurposes, session.CandidatePurposes...)
		if len(cleanupRefs) != len(cleanupPurposes) {
			return registryport.ErrVerification
		}
		if len(cleanupRefs) == 0 {
			next := state.Clone()
			delete(next.ProtectedRecoverySessions, request.OperationID)
			if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
				return registryport.ErrVerification
			}
			if err := storage.Commit(ctx, next); err != nil {
				return normalizeRegistryStoreError(err)
			}
			result = domainregistry.ProtectedRecoveryResultV1{Status: "rolled_back"}
			return nil
		}
		next := state.Clone()
		updated := next.ProtectedRecoverySessions[request.OperationID]
		updated.Phase = domainregistry.ProtectedRecoveryPhaseCleanupPending
		updated.Confirmed = false
		updated.Consumed = false
		updated.CleanupKind = protectedRecoveryCleanupAll
		updated.CleanupRefs = append([]string(nil), cleanupRefs...)
		updated.CleanupPurposes = append([]string(nil), cleanupPurposes...)
		next.ProtectedRecoverySessions[request.OperationID] = updated
		if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
			return registryport.ErrVerification
		}
		if err := storage.Commit(ctx, next); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.cleanupProtectedReferences(ctx, cleanupRefs, cleanupPurposes); err != nil {
			return err
		}
		if err := manager.completeProtectedRollbackPending(ctx, storage, request.OperationID); err != nil {
			return err
		}
		finalState, err := storage.Load(ctx)
		if err != nil || finalState.Validate() != nil {
			return registryport.ErrPersistence
		}
		delete(finalState.ProtectedRecoverySessions, request.OperationID)
		if err := protectedRecoveryIncrementRevision(&finalState); err != nil || finalState.Validate() != nil {
			return registryport.ErrVerification
		}
		if err := storage.Commit(ctx, finalState); err != nil {
			return normalizeRegistryStoreError(err)
		}
		result = domainregistry.ProtectedRecoveryResultV1{Status: "rolled_back"}
		return nil
	})
	if err != nil {
		return result, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) cleanupProtectedSecret(ctx context.Context, ref secretstoreport.CredentialRef, purpose secretstoreport.Purpose) error {
	if ref == "" {
		return nil
	}
	if purpose == "" {
		return registryport.ErrInvalidRequest
	}
	tombstoneErr := manager.secrets.Tombstone(ctx, ref, purpose)
	if errors.Is(tombstoneErr, secretstoreport.ErrNotFound) {
		// An exact not-found result proves there is no remaining artifact.
		return nil
	}
	if tombstoneErr != nil && !errors.Is(tombstoneErr, secretstoreport.ErrTombstoned) {
		return tombstoneErr
	}
	deleteErr := manager.secrets.ExplicitDelete(ctx, ref, purpose, secretstoreport.ExplicitlyDeleteCredential())
	if errors.Is(deleteErr, secretstoreport.ErrNotFound) {
		return nil
	}
	return deleteErr
}

// failProtectedRecoveryKeyCandidate handles failures after the durable
// pre-K1 journal was published. The exact journal remains the recovery
// authority until the key is tombstoned and explicitly deleted; if cleanup is
// not verified, it is moved to cleanup_pending rather than forgotten.
func (manager *Manager) failProtectedRecoveryKeyCandidate(
	ctx context.Context,
	storage registryport.Transaction,
	operationID string,
	reason error,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[operationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhasePendingKeyCandidate {
		return reason
	}
	cleanupErr := manager.cleanupProtectedSecret(ctx,
		secretstoreport.CredentialRef(session.DestinationKeyRef),
		secretstoreport.Purpose(session.DestinationKeyPurpose),
	)
	if cleanupErr != nil {
		if markErr := manager.markProtectedKeyCleanupPending(ctx, storage, state, session); markErr != nil {
			return markErr
		}
		return normalizeSecretError(cleanupErr)
	}
	next := state.Clone()
	delete(next.ProtectedRecoverySessions, operationID)
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	if err := storage.Commit(ctx, next); err != nil {
		// Cleanup is verified, but retaining the exact pre-K1 journal is safe:
		// the next recovery pass observes an exact-not-found key and can remove
		// this stale journal without publishing a request.
		return normalizeRegistryStoreError(err)
	}
	return reason
}

func (manager *Manager) readProtectedCandidateExact(
	ctx context.Context,
	ref string,
	purpose string,
	want []byte,
) error {
	got, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(ref),
		Purpose:       secretstoreport.Purpose(purpose),
		Consumer:      ProtectedRecoveryReadbackConsumer,
	})
	defer clear(got)
	if err != nil {
		return normalizeSecretError(err)
	}
	if !bytes.Equal(got, want) {
		return registryport.ErrVerification
	}
	return nil
}

func (manager *Manager) readProtectedPendingKeyExact(
	ctx context.Context,
	ref string,
	want []byte,
) error {
	got, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(ref),
		Purpose:       ProtectedRecoveryPendingKeyPurpose,
		Consumer:      ProtectedRecoveryPendingKeyConsumer,
	})
	defer clear(got)
	if err != nil {
		return normalizeSecretError(err)
	}
	if !bytes.Equal(got, want) {
		return registryport.ErrVerification
	}
	return nil
}

func protectedRecoveryCandidateDigest(
	request domainregistry.ProtectedRecoveryRequestV1,
	bundle domainregistry.ProtectedRecoveryBundleV1,
	entry domainregistry.ProtectedRecoveryBundleEntryV1,
	secret []byte,
	keyMaterial []byte,
) string {
	if len(keyMaterial) != 32 {
		return ""
	}
	keyInput := make([]byte, 0, len(keyMaterial)+len(request.RequestDigest)+len(request.SessionNonce)+2)
	keyInput = append(keyInput, keyMaterial...)
	keyInput = append(keyInput, 0)
	keyInput = append(keyInput, request.RequestDigest...)
	keyInput = append(keyInput, 0)
	keyInput = append(keyInput, request.SessionNonce...)
	key := sha256.Sum256(keyInput)
	clear(keyInput)
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write(protectedRecoveryAAD(request, bundle))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(entry.Correlation))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(entry.DestinationProviderID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(entry.Purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(secret)
	sum := mac.Sum(nil)
	return hex.EncodeToString(sum)
}
func protectedRecoverySessionCandidateRefsValid(session domainregistry.ProtectedRecoverySessionV1) bool {
	return len(session.CandidateRefs) != 0 && len(session.CandidateRefs) == len(session.CandidatePurposes) &&
		len(session.CandidateRefs) == len(session.CandidateDigests) && len(session.Bundle) != 0
}

func (manager *Manager) cleanupProtectedReferences(
	ctx context.Context,
	refs []string,
	purposes []string,
) error {
	if len(refs) != len(purposes) {
		return registryport.ErrInvalidRequest
	}
	for index, ref := range refs {
		if err := manager.cleanupProtectedSecret(ctx, secretstoreport.CredentialRef(ref), secretstoreport.Purpose(purposes[index])); err != nil {
			return normalizeSecretError(err)
		}
	}
	return nil
}

func protectedRecoveryIncrementRevision(state *domainregistry.Registry) error {
	if state == nil || state.Revision == ^uint64(0) {
		return registryport.ErrConflict
	}
	state.Revision++
	return nil
}

func (manager *Manager) persistProtectedRollbackPending(
	ctx context.Context,
	storage registryport.Transaction,
	state domainregistry.Registry,
	session domainregistry.ProtectedRecoverySessionV1,
	refs, purposes, digests []string,
	bundle []byte,
) error {
	if len(refs) != len(purposes) || len(refs) != len(digests) || len(refs) == 0 {
		return registryport.ErrInvalidRequest
	}
	next := state.Clone()
	updated := next.ProtectedRecoverySessions[session.OperationID]
	updated.Phase = domainregistry.ProtectedRecoveryPhaseRollbackPending
	updated.Confirmed = false
	updated.Consumed = false
	updated.CandidateRefs = append([]string(nil), refs...)
	updated.CandidatePurposes = append([]string(nil), purposes...)
	updated.CandidateDigests = append([]string(nil), digests...)
	updated.Bundle = bytes.Clone(bundle)
	updated.BundleDigest = domainregistry.ProtectedRecoveryBundleDigest(bundle)
	updated.Receipt = nil
	updated.CleanupKind = protectedRecoveryCleanupCandidates
	updated.CleanupRefs = append([]string(nil), refs...)
	updated.CleanupPurposes = append([]string(nil), purposes...)
	next.ProtectedRecoverySessions[session.OperationID] = updated
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	return normalizeRegistryStoreError(storage.Commit(ctx, next))
}

func (manager *Manager) completeProtectedRollbackPending(
	ctx context.Context,
	storage registryport.Transaction,
	operationID string,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[operationID]
	if !ok || (session.Phase != domainregistry.ProtectedRecoveryPhaseRollbackPending &&
		session.Phase != domainregistry.ProtectedRecoveryPhaseCleanupPending) {
		return errProtectedRecoveryNotFound
	}
	if session.CleanupKind != protectedRecoveryCleanupCandidates && session.CleanupKind != protectedRecoveryCleanupAll {
		return registryport.ErrVerification
	}
	next := state.Clone()
	updated := next.ProtectedRecoverySessions[operationID]
	updated.Phase = domainregistry.ProtectedRecoveryPhasePending
	updated.Confirmed = false
	updated.Consumed = false
	updated.BatchRegistryRevision = 0
	updated.CandidateRefs = nil
	updated.CandidatePurposes = nil
	updated.CandidateDigests = nil
	updated.Bundle = nil
	updated.BundleDigest = ""
	updated.Receipt = nil
	if session.CleanupKind == protectedRecoveryCleanupAll {
		updated.DestinationKeyRef = ""
		updated.DestinationKeyPurpose = ""
	}
	updated.CleanupKind = ""
	updated.CleanupRefs = nil
	updated.CleanupPurposes = nil
	next.ProtectedRecoverySessions[operationID] = updated
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	return normalizeRegistryStoreError(storage.Commit(ctx, next))
}

func (manager *Manager) recoverProtectedRollbackPhase(
	ctx context.Context,
	storage registryport.Transaction,
	sessionID string,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[sessionID]
	if !ok {
		return nil
	}
	if session.Phase == domainregistry.ProtectedRecoveryPhaseCandidatesDurable {
		if !protectedRecoverySessionCandidateRefsValid(session) {
			return registryport.ErrVerification
		}
		err = manager.persistProtectedRollbackPending(ctx, storage, state, session,
			session.CandidateRefs, session.CandidatePurposes, session.CandidateDigests, session.Bundle)
		if err != nil {
			return err
		}
		state, err = storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		session = state.ProtectedRecoverySessions[sessionID]
	}
	if session.Phase != domainregistry.ProtectedRecoveryPhaseRollbackPending &&
		!(session.Phase == domainregistry.ProtectedRecoveryPhaseCleanupPending && session.CleanupKind == protectedRecoveryCleanupCandidates) {
		return registryport.ErrVerification
	}
	if err := manager.cleanupProtectedReferences(ctx, session.CleanupRefs, session.CleanupPurposes); err != nil {
		return err
	}
	err = manager.completeProtectedRollbackPending(ctx, storage, sessionID)
	return err
}

func (manager *Manager) recoverProtectedKeyCandidatePhase(
	ctx context.Context,
	storage registryport.Transaction,
	sessionID string,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[sessionID]
	if !ok {
		return nil
	}
	if session.Phase != domainregistry.ProtectedRecoveryPhasePendingKeyCandidate {
		return registryport.ErrVerification
	}
	cleanupErr := manager.cleanupProtectedSecret(ctx,
		secretstoreport.CredentialRef(session.DestinationKeyRef),
		secretstoreport.Purpose(session.DestinationKeyPurpose),
	)
	if cleanupErr != nil {
		if markErr := manager.markProtectedKeyCleanupPending(ctx, storage, state, session); markErr != nil {
			return markErr
		}
		return normalizeSecretError(cleanupErr)
	}
	next := state.Clone()
	delete(next.ProtectedRecoverySessions, sessionID)
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	return normalizeRegistryStoreError(storage.Commit(ctx, next))
}

func (manager *Manager) markProtectedKeyCleanupPending(
	ctx context.Context,
	storage registryport.Transaction,
	state domainregistry.Registry,
	session domainregistry.ProtectedRecoverySessionV1,
) error {
	if session.DestinationKeyRef == "" {
		return nil
	}
	next := state.Clone()
	updated := next.ProtectedRecoverySessions[session.OperationID]
	updated.Phase = domainregistry.ProtectedRecoveryPhaseCleanupPending
	updated.CleanupKind = protectedRecoveryCleanupKey
	updated.CleanupRefs = []string{session.DestinationKeyRef}
	updated.CleanupPurposes = []string{session.DestinationKeyPurpose}
	next.ProtectedRecoverySessions[session.OperationID] = updated
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	return normalizeRegistryStoreError(storage.Commit(ctx, next))
}

func (manager *Manager) completeProtectedKeyCleanup(
	ctx context.Context,
	storage registryport.Transaction,
	sessionID string,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[sessionID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseCleanupPending || session.CleanupKind != protectedRecoveryCleanupKey {
		return errProtectedRecoveryNotFound
	}
	next := state.Clone()
	updated := next.ProtectedRecoverySessions[sessionID]
	updated.DestinationKeyRef = ""
	updated.DestinationKeyPurpose = ""
	updated.CleanupKind = ""
	updated.CleanupRefs = nil
	updated.CleanupPurposes = nil
	if !session.Confirmed && !session.Consumed && session.BatchRegistryRevision == 0 && len(session.Receipt) == 0 {
		// This is the prepare-publication fallback: only the pending private
		// key existed, so successful cleanup returns the local session to a
		// harmless pending state rather than inventing an applied winner.
		updated.Phase = domainregistry.ProtectedRecoveryPhasePending
		updated.CandidateRefs = nil
		updated.CandidatePurposes = nil
		updated.CandidateDigests = nil
		updated.Bundle = nil
		updated.BundleDigest = ""
		updated.Receipt = nil
		updated.BatchRegistryRevision = 0
	} else {
		updated.Phase = domainregistry.ProtectedRecoveryPhaseApplied
	}
	next.ProtectedRecoverySessions[sessionID] = updated
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	err = normalizeRegistryStoreError(storage.Commit(ctx, next))
	if err == nil && updated.Phase == domainregistry.ProtectedRecoveryPhaseApplied {
		manager.forgetProtectedRecoveryConfirmation(sessionID)
	}
	return err
}

func (manager *Manager) recoverProtectedKeyCleanup(
	ctx context.Context,
	storage registryport.Transaction,
	sessionID string,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[sessionID]
	if !ok {
		return nil
	}
	if session.Phase == domainregistry.ProtectedRecoveryPhaseApplied {
		if session.DestinationKeyRef == "" {
			return nil
		}
		if err := manager.markProtectedKeyCleanupPending(ctx, storage, state, session); err != nil {
			return err
		}
		state, err = storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		session = state.ProtectedRecoverySessions[sessionID]
	}
	if session.Phase != domainregistry.ProtectedRecoveryPhaseCleanupPending || session.CleanupKind != protectedRecoveryCleanupKey {
		return registryport.ErrVerification
	}
	if err := manager.cleanupProtectedReferences(ctx, session.CleanupRefs, session.CleanupPurposes); err != nil {
		return err
	}
	return manager.completeProtectedKeyCleanup(ctx, storage, sessionID)
}

func (manager *Manager) recoverProtectedAllCleanup(
	ctx context.Context,
	storage registryport.Transaction,
	sessionID string,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[sessionID]
	if !ok {
		return nil
	}
	if session.Phase != domainregistry.ProtectedRecoveryPhaseCleanupPending || session.CleanupKind != protectedRecoveryCleanupAll {
		return registryport.ErrVerification
	}
	if err := manager.cleanupProtectedReferences(ctx, session.CleanupRefs, session.CleanupPurposes); err != nil {
		return err
	}
	return manager.completeProtectedRollbackPending(ctx, storage, sessionID)
}

func protectedRecoveryTargetedSessionCurrent(
	state domainregistry.Registry,
	session domainregistry.ProtectedRecoverySessionV1,
) error {
	switch session.Phase {
	case domainregistry.ProtectedRecoveryPhaseVerificationPending,
		domainregistry.ProtectedRecoveryPhaseApplied:
		return protectedRecoverySuccessorMetadataCurrent(state, session)
	case domainregistry.ProtectedRecoveryPhaseCleanupPending:
		// Successful verification cleanup keeps the committed successor. A
		// rollback cleanup has no authenticated receipt and remains fenced by
		// the original uncredentialed entries instead.
		if session.BatchRegistryRevision != 0 && len(session.Receipt) != 0 {
			return protectedRecoverySuccessorMetadataCurrent(state, session)
		}
	}
	return protectedRecoveryEntriesCurrent(state, session.Entries)
}

// protectedRecoverySuccessorMetadataCurrent is the read-only authority gate
// for every post-commit phase. It derives the sole admissible Provider
// successor from the durable admitted session and performs no K1 access.
func protectedRecoverySuccessorMetadataCurrent(
	state domainregistry.Registry,
	session domainregistry.ProtectedRecoverySessionV1,
) error {
	if !protectedRecoverySessionCandidateRefsValid(session) ||
		len(session.PriorProviders) != len(session.Entries) ||
		len(session.CandidatePurposes) != len(session.Entries) ||
		validateProtectedOwnerInventory(session.OwnerBindings) != nil ||
		session.PriorRegistryIncarnation == "" || state.Incarnation != session.PriorRegistryIncarnation ||
		session.PriorRegistryRevision == 0 || session.BatchRegistryRevision == 0 || state.Revision < session.BatchRegistryRevision ||
		state.SelectedProviderID != session.PriorSelectedProviderID {
		return registryport.ErrVerification
	}
	matchedOwners := 0
	for index, entry := range session.Entries {
		prior := session.PriorProviders[index].Clone()
		if prior.ID != entry.DestinationProviderID || prior.Tombstone ||
			prior.Revision != entry.DestinationProviderRevision ||
			prior.Generation != entry.DestinationProviderGeneration ||
			prior.Incarnation != entry.DestinationProviderIncarnation ||
			prior.CredentialRef != "" || prior.CredentialPurpose != "" {
			return registryport.ErrVerification
		}
		expected := prior.Clone()
		expected.CredentialRef = session.CandidateRefs[index]
		expected.CredentialPurpose = session.CandidatePurposes[index]
		if expected.Revision == ^uint64(0) || expected.Generation == ^uint64(0) {
			return registryport.ErrVerification
		}
		expected.Revision++
		expected.Generation++
		if expected.PrivateAccount != nil {
			owner, found := protectedOwnerForCorrelation(session.OwnerBindings, entry.Correlation)
			if !found || owner.Owner != expected.PrivateAccount.Owner ||
				owner.Purpose != expected.CredentialPurpose {
				return registryport.ErrVerification
			}
			expected.PrivateAccount = &domainregistry.PrivateAccountScope{
				SchemaVersion: 1, Owner: owner.Owner, Provider: owner.Provider,
				AccountID: owner.AccountID, ChannelID: owner.ChannelID, Purpose: owner.Purpose,
			}
			if expected.PrivateAccount.Validate() != nil {
				return registryport.ErrVerification
			}
			matchedOwners++
		}
		provider, exists := state.Providers[entry.DestinationProviderID]
		if !exists || !reflect.DeepEqual(provider.Clone(), expected) {
			return registryport.ErrVerification
		}
	}
	if matchedOwners != len(session.OwnerBindings) {
		return registryport.ErrVerification
	}
	return nil
}

func (manager *Manager) verifyProtectedRecoverySuccessor(
	ctx context.Context,
	state domainregistry.Registry,
	session domainregistry.ProtectedRecoverySessionV1,
) error {
	if !protectedRecoverySessionCandidateRefsValid(session) || len(session.PriorProviders) != len(session.Entries) {
		return registryport.ErrVerification
	}
	if session.PriorRegistryIncarnation == "" || session.PriorRegistryIncarnation != state.Incarnation ||
		session.PriorRegistryRevision == 0 || session.BatchRegistryRevision == 0 || state.Revision < session.BatchRegistryRevision ||
		state.SelectedProviderID != session.PriorSelectedProviderID {
		return registryport.ErrVerification
	}
	if session.Phase == domainregistry.ProtectedRecoveryPhaseVerificationPending && state.Revision != session.BatchRegistryRevision {
		return registryport.ErrVerification
	}
	request, err := domainregistry.ParseProtectedRecoveryRequestV1(session.Request)
	if err != nil {
		return registryport.ErrVerification
	}
	bundle, err := domainregistry.ParseProtectedRecoveryBundleV1(session.Bundle)
	if err != nil || bundle.RequestDigest != request.RequestDigest || bundle.ManifestDigest != request.ManifestDigest ||
		bundle.OperationID != request.OperationID || bundle.SessionNonce != request.SessionNonce || bundle.ExpiresAt != request.ExpiresAt {
		return registryport.ErrVerification
	}
	receipt, err := domainregistry.ParseProtectedRecoveryReceiptV1(session.Receipt)
	if err != nil || receipt.RequestDigest != request.RequestDigest || receipt.ManifestDigest != request.ManifestDigest ||
		receipt.BundleDigest != session.BundleDigest || receipt.OperationID != request.OperationID ||
		receipt.SessionNonce != request.SessionNonce || receipt.ExpiresAt != request.ExpiresAt || len(receipt.Entries) != len(request.Entries) {
		return registryport.ErrVerification
	}
	for index, receiptEntry := range receipt.Entries {
		if receiptEntry.Correlation != request.Entries[index].Correlation ||
			receiptEntry.DestinationProviderID != request.Entries[index].DestinationProviderID || receiptEntry.Status != "applied" {
			return registryport.ErrVerification
		}
	}
	// Provider/Registry fences and the exact durable destination binding must
	// all be current before the first destination private-key or candidate read.
	if err := protectedRecoverySuccessorMetadataCurrent(state, session); err != nil {
		return err
	}
	if session.DestinationKeyRef == "" || session.DestinationKeyPurpose != string(ProtectedRecoveryPendingKeyPurpose) {
		return registryport.ErrVerification
	}
	privateBytes, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(session.DestinationKeyRef),
		Purpose:       secretstoreport.Purpose(session.DestinationKeyPurpose),
		Consumer:      ProtectedRecoveryPendingKeyConsumer,
	})
	if readErr != nil {
		clear(privateBytes)
		return normalizeSecretError(readErr)
	}
	if len(privateBytes) != 32 {
		clear(privateBytes)
		return registryport.ErrVerification
	}
	privateKey, keyErr := ecdh.X25519().NewPrivateKey(privateBytes)
	candidateDigestKey := bytes.Clone(privateBytes)
	clear(privateBytes)
	if keyErr != nil || len(candidateDigestKey) != 32 || !bytes.Equal(privateKey.PublicKey().Bytes(), request.DestinationEphemeralPublicKey) {
		clear(candidateDigestKey)
		return registryport.ErrVerification
	}
	defer clear(candidateDigestKey)
	sourcePublic, keyErr := ecdh.X25519().NewPublicKey(bundle.SourceEphemeralPublicKey)
	if keyErr != nil {
		return registryport.ErrVerification
	}
	shared, keyErr := privateKey.ECDH(sourcePublic)
	if keyErr != nil {
		return registryport.ErrVerification
	}
	transferKey, keyErr := protectedRecoveryHKDF(shared, request.RequestDigest)
	clear(shared)
	if keyErr != nil {
		return registryport.ErrVerification
	}
	defer clear(transferKey)
	expectedTag, tagErr := protectedRecoveryReceiptAuthenticationTag(transferKey, request, bundle, receipt)
	if tagErr != nil || !hmac.Equal(expectedTag, receipt.AuthenticationTag) {
		clear(expectedTag)
		return registryport.ErrVerification
	}
	clear(expectedTag)
	for index, requestEntry := range request.Entries {
		bundleEntry := bundle.Entries[index]
		if bundleEntry.Correlation != requestEntry.Correlation || bundleEntry.DestinationProviderID != requestEntry.DestinationProviderID ||
			!protectedPurposeAllowed(bundleEntry.Purpose) {
			return registryport.ErrVerification
		}
		provider, exists := state.Providers[requestEntry.DestinationProviderID]
		if !exists {
			return registryport.ErrVerification
		}
		expected := session.PriorProviders[index].Clone()
		expected.CredentialRef = session.CandidateRefs[index]
		expected.CredentialPurpose = bundleEntry.Purpose
		if expected.Revision == ^uint64(0) || expected.Generation == ^uint64(0) {
			return registryport.ErrVerification
		}
		expected.Revision++
		expected.Generation++
		if expected.PrivateAccount != nil {
			owner, found := protectedOwnerForCorrelation(session.OwnerBindings, requestEntry.Correlation)
			if !found || owner.Owner != expected.PrivateAccount.Owner || owner.Purpose != bundleEntry.Purpose {
				return registryport.ErrVerification
			}
			expected.PrivateAccount = &domainregistry.PrivateAccountScope{
				SchemaVersion: 1, Owner: owner.Owner, Provider: owner.Provider,
				AccountID: owner.AccountID, ChannelID: owner.ChannelID, Purpose: owner.Purpose,
			}
		}
		if !reflect.DeepEqual(provider.Clone(), expected) {
			return registryport.ErrVerification
		}
		got, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
			CredentialRef: secretstoreport.CredentialRef(session.CandidateRefs[index]),
			Purpose:       secretstoreport.Purpose(bundleEntry.Purpose), Consumer: ProtectedRecoveryReadbackConsumer,
		})
		if readErr != nil {
			clear(got)
			return normalizeSecretError(readErr)
		}
		digest := protectedRecoveryCandidateDigest(request, bundle, bundleEntry, got, candidateDigestKey)
		clear(got)
		if digest != session.CandidateDigests[index] {
			return registryport.ErrVerification
		}
	}
	return nil
}

func restoreProtectedRecoveryPriorProviders(
	state *domainregistry.Registry,
	session domainregistry.ProtectedRecoverySessionV1,
) error {
	if state == nil || len(session.PriorProviders) != len(session.Entries) {
		return registryport.ErrVerification
	}
	if session.PriorRegistryIncarnation != "" && state.Incarnation != session.PriorRegistryIncarnation {
		return registryport.ErrVerification
	}
	if session.PriorSelectedProviderID != state.SelectedProviderID {
		return registryport.ErrVerification
	}
	for index, entry := range session.Entries {
		prior := session.PriorProviders[index].Clone()
		if prior.ID != entry.DestinationProviderID || prior.CredentialRef != "" || prior.CredentialPurpose != "" {
			return registryport.ErrVerification
		}
		state.Providers[entry.DestinationProviderID] = prior
	}
	return nil
}

func (manager *Manager) restoreProtectedRecoveryAndCleanup(
	ctx context.Context,
	storage registryport.Transaction,
	session domainregistry.ProtectedRecoverySessionV1,
	reason error,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	if err := restoreProtectedRecoveryPriorProviders(&state, session); err != nil {
		return err
	}
	next := state.Clone()
	updated := next.ProtectedRecoverySessions[session.OperationID]
	updated.Phase = domainregistry.ProtectedRecoveryPhaseRollbackPending
	updated.Confirmed = false
	updated.Consumed = false
	updated.BatchRegistryRevision = 0
	updated.Receipt = nil
	updated.CleanupKind = protectedRecoveryCleanupCandidates
	updated.CleanupRefs = append([]string(nil), session.CandidateRefs...)
	updated.CleanupPurposes = append([]string(nil), session.CandidatePurposes...)
	next.ProtectedRecoverySessions[session.OperationID] = updated
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	if err := storage.Commit(ctx, next); err != nil {
		return normalizeRegistryStoreError(err)
	}
	if err := manager.cleanupProtectedReferences(ctx, updated.CleanupRefs, updated.CleanupPurposes); err != nil {
		return err
	}
	if err := manager.completeProtectedRollbackPending(ctx, storage, session.OperationID); err != nil {
		return err
	}
	return reason
}

func (manager *Manager) recoverProtectedVerificationPhase(
	ctx context.Context,
	storage registryport.Transaction,
	sessionID string,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	session, ok := state.ProtectedRecoverySessions[sessionID]
	if !ok {
		return nil
	}
	if session.Phase == domainregistry.ProtectedRecoveryPhaseCleanupPending {
		if session.CleanupKind == protectedRecoveryCleanupCandidates {
			if err := protectedRecoveryEntriesCurrent(state, session.Entries); err != nil {
				return err
			}
			return manager.recoverProtectedRollbackPhase(ctx, storage, sessionID)
		}
		if session.CleanupKind == protectedRecoveryCleanupAll {
			if err := protectedRecoveryEntriesCurrent(state, session.Entries); err != nil {
				return err
			}
			return manager.recoverProtectedAllCleanup(ctx, storage, sessionID)
		}
		if session.BatchRegistryRevision != 0 && len(session.Receipt) != 0 {
			if err := protectedRecoverySuccessorMetadataCurrent(state, session); err != nil {
				return err
			}
		} else if err := protectedRecoveryEntriesCurrent(state, session.Entries); err != nil {
			return err
		}
		return manager.recoverProtectedKeyCleanup(ctx, storage, sessionID)
	}
	if session.Phase == domainregistry.ProtectedRecoveryPhaseApplied {
		if err := manager.verifyProtectedRecoverySuccessor(ctx, state, session); err != nil {
			return manager.restoreProtectedRecoveryAndCleanup(ctx, storage, session, err)
		}
		return manager.recoverProtectedKeyCleanup(ctx, storage, sessionID)
	}
	if session.Phase != domainregistry.ProtectedRecoveryPhaseVerificationPending {
		return registryport.ErrVerification
	}
	if err := manager.verifyProtectedRecoverySuccessor(ctx, state, session); err != nil {
		return manager.restoreProtectedRecoveryAndCleanup(ctx, storage, session, err)
	}
	next := state.Clone()
	updated := next.ProtectedRecoverySessions[sessionID]
	updated.Phase = domainregistry.ProtectedRecoveryPhaseApplied
	updated.Confirmed = true
	updated.Consumed = true
	next.ProtectedRecoverySessions[sessionID] = updated
	if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
		return registryport.ErrVerification
	}
	if err := storage.Commit(ctx, next); err != nil {
		return normalizeRegistryStoreError(err)
	}
	return manager.recoverProtectedKeyCleanup(ctx, storage, sessionID)
}

func (manager *Manager) recoverProtectedRecoverySessions(
	ctx context.Context,
	storage registryport.Transaction,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	ids := make([]string, 0, len(state.ProtectedRecoverySessions))
	for id := range state.ProtectedRecoverySessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state, err = storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		session, ok := state.ProtectedRecoverySessions[id]
		if !ok {
			continue
		}
		switch session.Phase {
		case domainregistry.ProtectedRecoveryPhasePendingKeyCandidate,
			domainregistry.ProtectedRecoveryPhasePending,
			domainregistry.ProtectedRecoveryPhaseConfirmed,
			domainregistry.ProtectedRecoveryPhaseCandidatesDurable,
			domainregistry.ProtectedRecoveryPhaseRollbackPending:
			if err := protectedRecoveryEntriesCurrent(state, session.Entries); err != nil {
				return err
			}
		}
		switch session.Phase {
		case domainregistry.ProtectedRecoveryPhasePendingKeyCandidate:
			if err := manager.recoverProtectedKeyCandidatePhase(ctx, storage, id); err != nil {
				return err
			}
		case domainregistry.ProtectedRecoveryPhaseCandidatesDurable, domainregistry.ProtectedRecoveryPhaseRollbackPending:
			if err := manager.recoverProtectedRollbackPhase(ctx, storage, id); err != nil {
				return err
			}
		case domainregistry.ProtectedRecoveryPhaseVerificationPending, domainregistry.ProtectedRecoveryPhaseCleanupPending:
			if err := manager.recoverProtectedVerificationPhase(ctx, storage, id); err != nil {
				return err
			}
		case domainregistry.ProtectedRecoveryPhaseApplied:
			// Applied is receipt-ready only after the destination key cleanup
			// transition has completed. Once the key ref is gone, the terminal
			// successor was already verified while holding the Registry lock;
			// replay is rejected without another Secret Store read. An Applied
			// record that still names its key remains recoverable and must finish
			// the verification/cleanup path first.
			if session.DestinationKeyRef != "" {
				if err := manager.recoverProtectedVerificationPhase(ctx, storage, id); err != nil {
					return err
				}
			}
		case domainregistry.ProtectedRecoveryPhasePending,
			domainregistry.ProtectedRecoveryPhaseSourceBundle, domainregistry.ProtectedRecoveryPhaseFinalized,
			domainregistry.ProtectedRecoveryPhaseRolledBack:
			continue
		case domainregistry.ProtectedRecoveryPhaseConfirmed:
			if manager.hasProtectedRecoveryConfirmation(id) {
				continue
			}
			next := state.Clone()
			updated := next.ProtectedRecoverySessions[id]
			updated.Phase = domainregistry.ProtectedRecoveryPhasePending
			updated.Confirmed = false
			updated.Consumed = false
			updated.ReconfirmationRequired = true
			next.ProtectedRecoverySessions[id] = updated
			if err := protectedRecoveryIncrementRevision(&next); err != nil || next.Validate() != nil {
				return registryport.ErrVerification
			}
			if err := storage.Commit(ctx, next); err != nil {
				return normalizeRegistryStoreError(err)
			}
		default:
			return registryport.ErrVerification
		}
	}
	return nil
}

func (manager *Manager) rememberProtectedRecoveryConfirmation(operationID string) {
	if manager == nil || operationID == "" {
		return
	}
	manager.protectedRecoveryMu.Lock()
	defer manager.protectedRecoveryMu.Unlock()
	if manager.protectedRecoveryConfirmed == nil {
		manager.protectedRecoveryConfirmed = make(map[string]struct{})
	}
	manager.protectedRecoveryConfirmed[operationID] = struct{}{}
}

func (manager *Manager) hasProtectedRecoveryConfirmation(operationID string) bool {
	if manager == nil || operationID == "" {
		return false
	}
	manager.protectedRecoveryMu.Lock()
	defer manager.protectedRecoveryMu.Unlock()
	_, ok := manager.protectedRecoveryConfirmed[operationID]
	return ok
}

func (manager *Manager) forgetProtectedRecoveryConfirmation(operationID string) {
	if manager == nil || operationID == "" {
		return
	}
	manager.protectedRecoveryMu.Lock()
	defer manager.protectedRecoveryMu.Unlock()
	delete(manager.protectedRecoveryConfirmed, operationID)
}

func protectedRecoveryImportEntries(
	state domainregistry.Registry,
	manifest domainregistry.PortableManifestV1,
	result ProtectedRecoveryImportResult,
	currentOwners []domainregistry.ProtectedRecoveryOwnerBindingV1,
) ([]domainregistry.ProtectedRecoveryEntryV1, []domainregistry.ProtectedRecoveryOwnerBindingV1, error) {
	expectedCount := len(manifest.Providers) + len(manifest.Accounts)
	if result.ProviderCount != len(manifest.Providers) || result.AccountCount != len(manifest.Accounts) ||
		result.ReentryRequired != expectedCount || len(result.Entries) != expectedCount {
		return nil, nil, registryport.ErrInvalidRequest
	}
	byCorrelation := make(map[string]ProtectedRecoveryImportEntryResult, expectedCount)
	byDestinationID := make(map[string]struct{}, expectedCount)
	for index, entry := range result.Entries {
		correlation := protectedRecoveryExpectedCorrelation(manifest, index)
		if entry.Correlation != correlation || entry.Status != domainregistry.PortableManifestIntentReentryRequired ||
			!domainregistry.ValidProviderID(entry.DestinationProviderID) {
			return nil, nil, registryport.ErrInvalidRequest
		}
		if _, duplicate := byCorrelation[entry.Correlation]; duplicate {
			return nil, nil, registryport.ErrConflict
		}
		if _, duplicate := byDestinationID[entry.DestinationProviderID]; duplicate {
			return nil, nil, registryport.ErrConflict
		}
		byCorrelation[entry.Correlation] = entry
		byDestinationID[entry.DestinationProviderID] = struct{}{}
	}
	entries := make([]domainregistry.ProtectedRecoveryEntryV1, 0, expectedCount)
	for _, descriptor := range manifest.Providers {
		entry, ok := byCorrelation[descriptor.Correlation]
		if !ok || entry.DestinationOwnerBinding != nil {
			return nil, nil, registryport.ErrInvalidRequest
		}
		revision, generation, fenceErr := protectedRecoveryImportFenceValues(entry.Fence)
		if fenceErr != nil {
			return nil, nil, fenceErr
		}
		provider, exists := state.Providers[entry.DestinationProviderID]
		if !exists || provider.Tombstone || provider.CredentialRef != "" || provider.CredentialPurpose != "" ||
			provider.Revision != revision || provider.Generation != generation || provider.Incarnation != entry.Fence.Incarnation {
			return nil, nil, errProtectedRecoveryCurrentness
		}
		if err := validateImportedPublicProvider(provider, descriptor, byCorrelation); err != nil {
			return nil, nil, err
		}
		entries = append(entries, domainregistry.ProtectedRecoveryEntryV1{
			Correlation: descriptor.Correlation, DestinationProviderID: provider.ID,
			DestinationProviderRevision: revision, DestinationProviderGeneration: generation,
			DestinationProviderIncarnation: entry.Fence.Incarnation,
		})
	}
	admittedOwners := make([]domainregistry.ProtectedRecoveryOwnerBindingV1, 0, len(manifest.Accounts))
	for _, descriptor := range manifest.Accounts {
		entry, ok := byCorrelation[descriptor.Correlation]
		if !ok || entry.DestinationOwnerBinding == nil || entry.DestinationOwnerBinding.Validate() != nil {
			return nil, nil, registryport.ErrInvalidRequest
		}
		revision, generation, fenceErr := protectedRecoveryImportFenceValues(entry.Fence)
		if fenceErr != nil {
			return nil, nil, fenceErr
		}
		provider, exists := state.Providers[entry.DestinationProviderID]
		if !exists || provider.Tombstone || provider.CredentialRef != "" || provider.CredentialPurpose != "" ||
			provider.Kind != domainregistry.PrivateAccountKind || provider.PrivateAccount == nil || provider.Revision != revision ||
			provider.Generation != generation || provider.Incarnation != entry.Fence.Incarnation {
			return nil, nil, errProtectedRecoveryCurrentness
		}
		normalized, err := descriptor.Normalize()
		if err != nil || provider.Endpoint != normalized.Endpoint || provider.Proxy != "" ||
			provider.PrivateAccount.Owner != normalized.Owner || provider.PrivateAccount.Purpose != normalized.Purpose {
			return nil, nil, registryport.ErrInvalidRequest
		}
		admitted := *entry.DestinationOwnerBinding
		if admitted.Correlation != normalized.Correlation || admitted.Owner != normalized.Owner ||
			admitted.Provider != normalized.Provider || admitted.Purpose != normalized.Purpose {
			return nil, nil, registryport.ErrConflict
		}
		current, found := protectedOwnerForCorrelation(currentOwners, normalized.Correlation)
		if !found || !protectedOwnerBindingsEqual(
			[]domainregistry.ProtectedRecoveryOwnerBindingV1{admitted},
			[]domainregistry.ProtectedRecoveryOwnerBindingV1{current},
		) {
			return nil, nil, errProtectedRecoveryCurrentness
		}
		admittedOwners = append(admittedOwners, admitted)
		entries = append(entries, domainregistry.ProtectedRecoveryEntryV1{
			Correlation: descriptor.Correlation, DestinationProviderID: provider.ID,
			DestinationProviderRevision: revision, DestinationProviderGeneration: generation,
			DestinationProviderIncarnation: entry.Fence.Incarnation,
		})
	}
	if len(currentOwners) != len(admittedOwners) {
		return nil, nil, errProtectedRecoveryCurrentness
	}
	return entries, admittedOwners, nil
}

func protectedRecoveryImportFenceValues(fence ProtectedRecoveryImportFence) (uint64, uint64, error) {
	revision, revisionErr := strconv.ParseUint(fence.Revision, 10, 64)
	generation, generationErr := strconv.ParseUint(fence.Generation, 10, 64)
	if revisionErr != nil || generationErr != nil || revision == 0 || generation == 0 ||
		strconv.FormatUint(revision, 10) != fence.Revision || strconv.FormatUint(generation, 10) != fence.Generation ||
		!domainregistry.ValidIncarnation(fence.Incarnation) {
		return 0, 0, registryport.ErrInvalidRequest
	}
	return revision, generation, nil
}

func protectedRecoveryPriorProviders(
	state domainregistry.Registry,
	entries []domainregistry.ProtectedRecoveryEntryV1,
) ([]domainregistry.Provider, error) {
	prior := make([]domainregistry.Provider, 0, len(entries))
	for _, entry := range entries {
		provider, exists := state.Providers[entry.DestinationProviderID]
		if !exists || provider.Tombstone || provider.CredentialRef != "" || provider.CredentialPurpose != "" ||
			provider.Revision != entry.DestinationProviderRevision || provider.Generation != entry.DestinationProviderGeneration ||
			provider.Incarnation != entry.DestinationProviderIncarnation {
			return nil, registryport.ErrConflict
		}
		prior = append(prior, provider.Clone())
	}
	return prior, nil
}

func validateImportedPublicProvider(
	provider domainregistry.Provider,
	descriptor domainregistry.PortableProviderDescriptorV1,
	byCorrelation map[string]ProtectedRecoveryImportEntryResult,
) error {
	normalized, err := descriptor.Normalize()
	if err != nil || provider.Kind != normalized.Kind || provider.Endpoint != normalized.Endpoint || provider.Proxy != normalized.Proxy ||
		!slices.Equal(provider.Models, normalized.Models) || !slices.Equal(provider.MediaModels, normalized.MediaModels) ||
		provider.SelectedModel != normalized.SelectedModel || provider.SelectedMedia != normalized.SelectedMedia ||
		!reflect.DeepEqual(provider.OAuthBinding, normalized.OAuthBinding) ||
		!reflect.DeepEqual(provider.AccountObservation, normalized.AccountObservation) {
		return registryport.ErrInvalidRequest
	}
	routes, routeErr := provider.RouteProviderIDs()
	if routeErr != nil {
		return registryport.ErrInvalidRequest
	}
	// An empty portable route list deliberately preserves the Registry's
	// default self-route semantics. RouteProviderIDs reports that implicit
	// route for an executable Provider, so it must not be compared as an
	// explicit manifest route here.
	if len(normalized.Routes) == 0 {
		if len(provider.SelectedRoutes) != 0 {
			return registryport.ErrInvalidRequest
		}
		return nil
	}
	if len(routes) != len(normalized.Routes) {
		return registryport.ErrInvalidRequest
	}
	for index, routeID := range routes {
		mapped, ok := byCorrelation[normalized.Routes[index]]
		if !ok || mapped.DestinationProviderID != routeID || !domainregistry.ValidProviderID(routeID) {
			return registryport.ErrInvalidRequest
		}
	}
	return nil
}

func protectedRecoveryPreflightBundle(
	state domainregistry.Registry,
	request domainregistry.ProtectedRecoveryRequestV1,
	bundle domainregistry.ProtectedRecoveryBundleV1,
	session domainregistry.ProtectedRecoverySessionV1,
	owners []domainregistry.ProtectedRecoveryOwnerBindingV1,
) error {
	if bundle.RequestDigest != request.RequestDigest || bundle.ManifestDigest != request.ManifestDigest ||
		bundle.OperationID != request.OperationID || bundle.SessionNonce != request.SessionNonce || bundle.ExpiresAt != request.ExpiresAt ||
		len(bundle.Entries) != len(request.Entries) || !protectedOwnerBindingsMatchSession(session.OwnerBindings, owners) {
		return errProtectedRecoveryProtocol
	}
	if err := protectedRecoveryEntriesCurrent(state, request.Entries); err != nil {
		return err
	}
	for index, requestEntry := range request.Entries {
		bundleEntry := bundle.Entries[index]
		if bundleEntry.Correlation != requestEntry.Correlation || bundleEntry.DestinationProviderID != requestEntry.DestinationProviderID {
			return errProtectedRecoveryProtocol
		}
		provider := state.Providers[requestEntry.DestinationProviderID]
		expectedPurpose := protectedDestinationPurpose(provider)
		if !protectedPurposeAllowed(expectedPurpose) || bundleEntry.Purpose != expectedPurpose {
			return registryport.ErrInvalidRequest
		}
		if provider.PrivateAccount != nil {
			owner, ok := protectedOwnerForCorrelation(owners, requestEntry.Correlation)
			if !ok || owner.Owner != provider.PrivateAccount.Owner || owner.Purpose != provider.PrivateAccount.Purpose {
				return registryport.ErrConflict
			}
		}
	}
	return nil
}

func protectedRecoveryEntriesCurrent(
	state domainregistry.Registry,
	entries []domainregistry.ProtectedRecoveryEntryV1,
) error {
	for _, entry := range entries {
		provider, exists := state.Providers[entry.DestinationProviderID]
		if !exists || provider.Tombstone || provider.Revision != entry.DestinationProviderRevision ||
			provider.Generation != entry.DestinationProviderGeneration || provider.Incarnation != entry.DestinationProviderIncarnation ||
			provider.CredentialRef != "" || provider.CredentialPurpose != "" {
			return registryport.ErrConflict
		}
	}
	return nil
}

func applyProtectedPayloadToRegistry(
	state *domainregistry.Registry,
	request domainregistry.ProtectedRecoveryRequestV1,
	payload protectedRecoveryPayload,
	candidateRefs []string,
	owners []domainregistry.ProtectedRecoveryOwnerBindingV1,
) error {
	if len(payload.Entries) != len(request.Entries) || len(candidateRefs) != len(payload.Entries) {
		return registryport.ErrInvalidRequest
	}
	for index, payloadEntry := range payload.Entries {
		requestEntry := request.Entries[index]
		if payloadEntry.Correlation != requestEntry.Correlation || payloadEntry.DestinationProviderID != requestEntry.DestinationProviderID ||
			!protectedPurposeAllowed(payloadEntry.Purpose) {
			return registryport.ErrInvalidRequest
		}
		provider, exists := state.Providers[payloadEntry.DestinationProviderID]
		if !exists || provider.CredentialRef != "" || provider.CredentialPurpose != "" || provider.Revision != requestEntry.DestinationProviderRevision ||
			provider.Generation != requestEntry.DestinationProviderGeneration || provider.Incarnation != requestEntry.DestinationProviderIncarnation {
			return registryport.ErrConflict
		}
		provider.CredentialRef = candidateRefs[index]
		provider.CredentialPurpose = payloadEntry.Purpose
		if provider.PrivateAccount != nil {
			owner, found := protectedOwnerForCorrelation(owners, payloadEntry.Correlation)
			if !found || owner.Owner != provider.PrivateAccount.Owner || owner.Purpose != payloadEntry.Purpose {
				return registryport.ErrConflict
			}
			replacement := &domainregistry.PrivateAccountScope{
				SchemaVersion: 1, Owner: owner.Owner, Provider: owner.Provider,
				AccountID: owner.AccountID, ChannelID: owner.ChannelID, Purpose: owner.Purpose,
			}
			if replacement.Validate() != nil {
				return registryport.ErrInvalidRequest
			}
			for id, other := range state.Providers {
				if id != provider.ID && other.PrivateAccount != nil && reflect.DeepEqual(other.PrivateAccount, replacement) {
					return registryport.ErrConflict
				}
			}
			provider.PrivateAccount = replacement
		}
		if provider.Revision == ^uint64(0) || provider.Generation == ^uint64(0) {
			return registryport.ErrConflict
		}
		provider.Revision++
		provider.Generation++
		state.Providers[provider.ID] = provider
	}
	return nil
}

func sourceProtectedOwners(state domainregistry.Registry, manifest domainregistry.PortableManifestV1) ([]domainregistry.ProtectedRecoveryOwnerBindingV1, error) {
	owners := make([]domainregistry.ProtectedRecoveryOwnerBindingV1, 0, len(manifest.Accounts))
	for _, descriptor := range manifest.Accounts {
		provider, err := sourceProviderByIdentity(state, descriptor)
		if err != nil || provider.PrivateAccount == nil {
			return nil, registryport.ErrConflict
		}
		owners = append(owners, domainregistry.ProtectedRecoveryOwnerBindingV1{
			Correlation: descriptor.Correlation,
			Owner:       provider.PrivateAccount.Owner, Provider: provider.PrivateAccount.Provider,
			AccountID: provider.PrivateAccount.AccountID, ChannelID: provider.PrivateAccount.ChannelID,
			Purpose:     provider.PrivateAccount.Purpose,
			Fingerprint: "source-registry-" + provider.PrivateAccount.Provider,
		})
	}
	sortProtectedOwnerBindings(owners)
	return owners, nil
}

func validateSourceOwnerInventory(expected, actual []domainregistry.ProtectedRecoveryOwnerBindingV1) error {
	if len(expected) != len(actual) {
		return registryport.ErrConflict
	}
	// The current inventory is authoritative. Fingerprints may differ from a
	// source registry's derived diagnostic value, but every manifest-local
	// correlation must have exactly one matching owner/scope record.
	for _, want := range expected {
		found := false
		for _, got := range actual {
			if got.Correlation == want.Correlation && got.Owner == want.Owner && got.Provider == want.Provider && got.AccountID == want.AccountID &&
				got.ChannelID == want.ChannelID && got.Purpose == want.Purpose && got.Fingerprint != "" {
				found = true
				break
			}
		}
		if !found {
			return registryport.ErrConflict
		}
	}
	return nil
}

func protectedRecoverySourceStateCurrent(
	current, original domainregistry.Registry,
	manifest domainregistry.PortableManifestV1,
	request domainregistry.ProtectedRecoveryRequestV1,
	owners []domainregistry.ProtectedRecoveryOwnerBindingV1,
) bool {
	if current.Incarnation != original.Incarnation || current.Revision != original.Revision ||
		current.SelectedProviderID != original.SelectedProviderID {
		return false
	}
	for _, entry := range request.Entries {
		before, beforePurpose, beforeErr := sourceProviderForCorrelation(original, manifest, entry.Correlation)
		after, afterPurpose, afterErr := sourceProviderForCorrelation(current, manifest, entry.Correlation)
		if beforeErr != nil || afterErr != nil || beforePurpose != afterPurpose || !reflect.DeepEqual(before.Clone(), after.Clone()) {
			return false
		}
	}
	currentOwners, err := sourceProtectedOwners(current, manifest)
	if err != nil || validateSourceOwnerInventory(currentOwners, owners) != nil {
		return false
	}
	return true
}

func sourceProviderForCorrelation(
	state domainregistry.Registry,
	manifest domainregistry.PortableManifestV1,
	correlation string,
) (domainregistry.Provider, string, error) {
	for _, descriptor := range manifest.Providers {
		if descriptor.Correlation != correlation {
			continue
		}
		provider, err := sourceProviderByIdentity(state, descriptor)
		if err != nil {
			return domainregistry.Provider{}, "", err
		}
		purpose := "provider-api-key"
		if provider.OAuthBinding != nil {
			purpose = "provider-oauth-token-bundle"
		}
		return provider, purpose, nil
	}
	for _, descriptor := range manifest.Accounts {
		if descriptor.Correlation != correlation {
			continue
		}
		provider, err := sourceProviderByIdentity(state, descriptor)
		if err != nil || provider.PrivateAccount == nil {
			return domainregistry.Provider{}, "", registryport.ErrConflict
		}
		return provider, provider.PrivateAccount.Purpose, nil
	}
	return domainregistry.Provider{}, "", registryport.ErrInvalidRequest
}

func sourceProviderByIdentity(state domainregistry.Registry, descriptor any) (domainregistry.Provider, error) {
	var target string
	switch value := descriptor.(type) {
	case domainregistry.PortableProviderDescriptorV1:
		var err error
		target, err = value.CanonicalIdentity()
		if err != nil {
			return domainregistry.Provider{}, registryport.ErrInvalidRequest
		}
		var found domainregistry.Provider
		for _, provider := range state.Providers {
			if provider.Tombstone || provider.Kind == domainregistry.PrivateAccountKind || provider.PrivateAccount != nil {
				continue
			}
			candidate, err := portableProviderDescriptorForRegistryProvider(provider)
			if err != nil {
				return domainregistry.Provider{}, err
			}
			identity, err := candidate.CanonicalIdentity()
			if err != nil {
				return domainregistry.Provider{}, err
			}
			if identity == target {
				if found.ID != "" {
					return domainregistry.Provider{}, registryport.ErrConflict
				}
				found = provider
			}
		}
		if found.ID == "" {
			return domainregistry.Provider{}, registryport.ErrNotFound
		}
		return found, nil
	case domainregistry.PortableAccountDescriptorV1:
		var err error
		target, err = value.CanonicalIdentity()
		if err != nil {
			return domainregistry.Provider{}, registryport.ErrInvalidRequest
		}
		var found domainregistry.Provider
		for _, provider := range state.Providers {
			if provider.Tombstone || provider.PrivateAccount == nil {
				continue
			}
			candidate, err := portableAccountDescriptorForRegistryProvider(provider)
			if err != nil {
				return domainregistry.Provider{}, err
			}
			identity, err := candidate.CanonicalIdentity()
			if err != nil {
				return domainregistry.Provider{}, err
			}
			if identity == target {
				if found.ID != "" {
					return domainregistry.Provider{}, registryport.ErrConflict
				}
				found = provider
			}
		}
		if found.ID == "" {
			return domainregistry.Provider{}, registryport.ErrNotFound
		}
		return found, nil
	default:
		return domainregistry.Provider{}, registryport.ErrInvalidRequest
	}
}

func marshalProtectedRecoveryPayload(entries []protectedRecoveryPayloadEntry) ([]byte, error) {
	return json.Marshal(struct {
		Entries []protectedRecoveryPayloadEntry `json:"entries"`
	}{Entries: entries})
}

func parseProtectedRecoveryPayload(data []byte) (protectedRecoveryPayload, error) {
	if len(data) == 0 || len(data) > protectedRecoveryMaxPlaintext {
		return protectedRecoveryPayload{}, errProtectedRecoveryProtocol
	}
	var payload protectedRecoveryPayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || payload.Entries == nil || len(payload.Entries) > domainregistry.ProtectedRecoveryMaxEntries {
		return protectedRecoveryPayload{}, errProtectedRecoveryProtocol
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return protectedRecoveryPayload{}, errProtectedRecoveryProtocol
	}
	canonical, err := marshalProtectedRecoveryPayload(payload.Entries)
	if err != nil || !bytes.Equal(canonical, data) {
		return protectedRecoveryPayload{}, errProtectedRecoveryProtocol
	}
	seen := make(map[string]struct{}, len(payload.Entries))
	for _, entry := range payload.Entries {
		if !domainregistry.ValidProviderID(entry.DestinationProviderID) || !protectedPurposeAllowed(entry.Purpose) ||
			entry.Correlation == "" || len(entry.Secret) == 0 || len(entry.Secret) > protectedRecoveryMaxPlaintext {
			return protectedRecoveryPayload{}, errProtectedRecoveryProtocol
		}
		if _, duplicate := seen[entry.Correlation]; duplicate {
			return protectedRecoveryPayload{}, errProtectedRecoveryProtocol
		}
		seen[entry.Correlation] = struct{}{}
	}
	return payload, nil
}

func protectedPayloadMatchesBundle(payload protectedRecoveryPayload, bundle domainregistry.ProtectedRecoveryBundleV1) bool {
	if len(payload.Entries) != len(bundle.Entries) {
		return false
	}
	for index, payloadEntry := range payload.Entries {
		bundleEntry := bundle.Entries[index]
		if payloadEntry.Correlation != bundleEntry.Correlation || payloadEntry.DestinationProviderID != bundleEntry.DestinationProviderID ||
			payloadEntry.Purpose != bundleEntry.Purpose {
			return false
		}
	}
	return true
}

func protectedRecoveryBundleEntries(request domainregistry.ProtectedRecoveryRequestV1, entries []protectedRecoveryPayloadEntry) []domainregistry.ProtectedRecoveryBundleEntryV1 {
	result := make([]domainregistry.ProtectedRecoveryBundleEntryV1, 0, len(request.Entries))
	for index, requestEntry := range request.Entries {
		purpose := ""
		if index < len(entries) {
			purpose = entries[index].Purpose
		}
		result = append(result, domainregistry.ProtectedRecoveryBundleEntryV1{
			Correlation: requestEntry.Correlation, DestinationProviderID: requestEntry.DestinationProviderID, Purpose: purpose,
		})
	}
	return result
}

func protectedRecoveryReceiptEntries(request domainregistry.ProtectedRecoveryRequestV1) []domainregistry.ProtectedRecoveryReceiptEntryV1 {
	result := make([]domainregistry.ProtectedRecoveryReceiptEntryV1, 0, len(request.Entries))
	for _, entry := range request.Entries {
		result = append(result, domainregistry.ProtectedRecoveryReceiptEntryV1{
			Correlation: entry.Correlation, DestinationProviderID: entry.DestinationProviderID, Status: "applied",
		})
	}
	return result
}

func protectedRecoveryAAD(request domainregistry.ProtectedRecoveryRequestV1, bundle domainregistry.ProtectedRecoveryBundleV1) []byte {
	mapping := make([]string, 0, len(bundle.Entries))
	purposes := make([]string, 0, len(bundle.Entries))
	for _, entry := range bundle.Entries {
		mapping = append(mapping, entry.Correlation+"="+entry.DestinationProviderID)
		purposes = append(purposes, entry.Purpose)
	}
	components := [][]byte{
		[]byte(domainregistry.ProtectedRecoveryBundleSchemaV1 + ":" + fmt.Sprint(domainregistry.ProtectedRecoveryProtocolVersion)),
		[]byte(request.RequestDigest), []byte(request.ManifestDigest),
		request.DestinationEphemeralPublicKey, bundle.SourceEphemeralPublicKey,
		[]byte(request.OperationID), []byte(request.SessionNonce),
		[]byte(strings.Join(mapping, "\x00")), []byte(strings.Join(purposes, "\x00")), []byte(request.ExpiresAt),
	}
	var result bytes.Buffer
	for _, component := range components {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(component)))
		result.Write(length[:])
		result.Write(component)
	}
	return result.Bytes()
}

func protectedRecoveryHKDF(shared []byte, requestDigest string) ([]byte, error) {
	if len(shared) == 0 || len(requestDigest) != 64 {
		return nil, errProtectedRecoveryProtocol
	}
	salt, err := hex.DecodeString(requestDigest)
	if err != nil {
		return nil, errProtectedRecoveryProtocol
	}
	extractor := hmac.New(sha256.New, salt)
	_, _ = extractor.Write(shared)
	prk := extractor.Sum(nil)
	defer clear(prk)
	result := make([]byte, 0, 32)
	previous := []byte{}
	for counter := byte(1); len(result) < 32; counter++ {
		mac := hmac.New(sha256.New, prk)
		_, _ = mac.Write(previous)
		_, _ = mac.Write([]byte(protectedRecoveryHKDFInfo))
		_, _ = mac.Write([]byte{counter})
		previous = mac.Sum(nil)
		result = append(result, previous...)
	}
	clear(previous)
	return result[:32], nil
}

func protectedRecoveryReceiptAuthenticationTag(
	transferKey []byte,
	request domainregistry.ProtectedRecoveryRequestV1,
	bundle domainregistry.ProtectedRecoveryBundleV1,
	receipt domainregistry.ProtectedRecoveryReceiptV1,
) ([]byte, error) {
	if len(transferKey) != 32 || len(receipt.AuthenticationTag) != 32 {
		return nil, errProtectedRecoveryProtocol
	}
	transcript, err := domainregistry.MarshalProtectedRecoveryReceiptV1ForAuthentication(receipt)
	if err != nil {
		return nil, errProtectedRecoveryProtocol
	}
	defer clear(transcript)
	keyMAC := hmac.New(sha256.New, transferKey)
	_, _ = keyMAC.Write([]byte(protectedRecoveryReceiptAuthInfo))
	receiptKey := keyMAC.Sum(nil)
	defer clear(receiptKey)
	mac := hmac.New(sha256.New, receiptKey)
	_, _ = mac.Write(protectedRecoveryAAD(request, bundle))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(transcript)
	return mac.Sum(nil), nil
}

func newProtectedRecoverySessionMaterial() (string, string, string, error) {
	operationBytes := make([]byte, 24)
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(operationBytes); err != nil {
		return "", "", "", err
	}
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", "", err
	}
	return "operation-" + hex.EncodeToString(operationBytes), hex.EncodeToString(nonceBytes),
		time.Now().UTC().Add(protectedRecoveryExpiryWindow).Format(time.RFC3339Nano), nil
}

func requestDigestWithoutFingerprint(request domainregistry.ProtectedRecoveryRequestV1) string {
	request.VerificationFingerprint = ""
	data, err := marshalProtectedRecoveryRequestForDigest(request)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func marshalProtectedRecoveryRequestForDigest(request domainregistry.ProtectedRecoveryRequestV1) ([]byte, error) {
	// The domain parser owns the canonical representation.  Constructing a
	// temporary request with an empty fingerprint causes its omitempty field to
	// be absent without introducing a self-referential digest.
	return domainregistry.MarshalProtectedRecoveryRequestV1ForDigest(request)
}

func protectedRecoveryConfirmationBindingMatches(
	action domainregistry.ProtectedRecoveryLocalActionV1,
	session domainregistry.ProtectedRecoverySessionV1,
) bool {
	if action.Confirmation.LocalBinding != action.LocalBinding ||
		action.LocalBinding.ProfileBinding != session.LocalBinding.ProfileBinding ||
		action.LocalBinding.DataDirectoryBinding != session.LocalBinding.DataDirectoryBinding {
		return false
	}
	// The first confirmation is bound to the exact window/frame that prepared
	// the request. A restarted Manager marks the session for reconfirmation and
	// may then rebind only those two process-local identities.
	return session.ReconfirmationRequired || action.LocalBinding == session.LocalBinding
}

func protectedRecoveryActionMatchesSession(
	action domainregistry.ProtectedRecoveryLocalActionV1,
	request domainregistry.ProtectedRecoveryRequestV1,
	session domainregistry.ProtectedRecoverySessionV1,
	allowReceiptRebind bool,
) bool {
	if action.Validate() != nil || !protectedActionMatchesRequest(action, request) ||
		action.Confirmation.LocalBinding != action.LocalBinding ||
		action.LocalBinding.ProfileBinding != session.LocalBinding.ProfileBinding ||
		action.LocalBinding.DataDirectoryBinding != session.LocalBinding.DataDirectoryBinding {
		return false
	}
	if allowReceiptRebind && (session.Phase == domainregistry.ProtectedRecoveryPhaseApplied || session.ReconfirmationRequired) {
		return true
	}
	return action.LocalBinding == session.LocalBinding
}

func (manager *Manager) verifyProtectedRecoveryReceiptReady(
	state domainregistry.Registry,
	session domainregistry.ProtectedRecoverySessionV1,
) error {
	if session.Phase != domainregistry.ProtectedRecoveryPhaseApplied || !session.Confirmed ||
		!session.Consumed || session.DestinationKeyRef != "" || session.BatchRegistryRevision == 0 ||
		len(session.Entries) == 0 || len(session.PriorProviders) != len(session.Entries) ||
		len(session.CandidateRefs) != len(session.Entries) || len(session.CandidatePurposes) != len(session.Entries) {
		return registryport.ErrVerification
	}
	if session.PriorRegistryIncarnation == "" || state.Incarnation != session.PriorRegistryIncarnation ||
		state.SelectedProviderID != session.PriorSelectedProviderID || state.Revision < session.BatchRegistryRevision {
		return registryport.ErrVerification
	}
	request, err := domainregistry.ParseProtectedRecoveryRequestV1(session.Request)
	if err != nil || request.RequestDigest != session.RequestDigest || request.ManifestDigest != session.ManifestDigest {
		return registryport.ErrVerification
	}
	bundle, err := domainregistry.ParseProtectedRecoveryBundleV1(session.Bundle)
	if err != nil || bundle.RequestDigest != session.RequestDigest || bundle.ManifestDigest != session.ManifestDigest ||
		bundle.OperationID != session.OperationID || bundle.SessionNonce != session.SessionNonce ||
		bundle.ExpiresAt != session.ExpiresAt || session.BundleDigest != domainregistry.ProtectedRecoveryBundleDigest(session.Bundle) {
		return registryport.ErrVerification
	}
	receipt, err := domainregistry.ParseProtectedRecoveryReceiptV1(session.Receipt)
	if err != nil || receipt.RequestDigest != session.RequestDigest || receipt.ManifestDigest != session.ManifestDigest ||
		receipt.BundleDigest != session.BundleDigest || receipt.OperationID != session.OperationID ||
		receipt.SessionNonce != session.SessionNonce || receipt.ExpiresAt != session.ExpiresAt ||
		len(receipt.Entries) != len(session.Entries) {
		return registryport.ErrVerification
	}
	for index, entry := range session.Entries {
		if receipt.Entries[index].Correlation != entry.Correlation ||
			receipt.Entries[index].DestinationProviderID != entry.DestinationProviderID ||
			receipt.Entries[index].Status != "applied" ||
			bundle.Entries[index].Correlation != entry.Correlation ||
			bundle.Entries[index].DestinationProviderID != entry.DestinationProviderID ||
			bundle.Entries[index].Purpose != session.CandidatePurposes[index] {
			return registryport.ErrVerification
		}
		provider, exists := state.Providers[entry.DestinationProviderID]
		if !exists {
			return registryport.ErrVerification
		}
		expected := session.PriorProviders[index].Clone()
		expected.CredentialRef = session.CandidateRefs[index]
		expected.CredentialPurpose = session.CandidatePurposes[index]
		if expected.Revision == ^uint64(0) || expected.Generation == ^uint64(0) {
			return registryport.ErrVerification
		}
		expected.Revision++
		expected.Generation++
		if expected.PrivateAccount != nil {
			owner, found := protectedOwnerForCorrelation(session.OwnerBindings, entry.Correlation)
			if !found || owner.Owner != expected.PrivateAccount.Owner || owner.Purpose != expected.CredentialPurpose {
				return registryport.ErrVerification
			}
			expected.PrivateAccount = &domainregistry.PrivateAccountScope{
				SchemaVersion: 1, Owner: owner.Owner, Provider: owner.Provider,
				AccountID: owner.AccountID, ChannelID: owner.ChannelID, Purpose: owner.Purpose,
			}
		}
		if !reflect.DeepEqual(provider.Clone(), expected) {
			return registryport.ErrVerification
		}
	}
	return nil
}

func protectedActionMatchesRequest(action domainregistry.ProtectedRecoveryLocalActionV1, request domainregistry.ProtectedRecoveryRequestV1) bool {
	confirmation := action.Confirmation
	return confirmation.RequestDigest == request.RequestDigest && confirmation.RequestFingerprint == request.VerificationFingerprint &&
		confirmation.OperationID == request.OperationID && confirmation.SessionNonce == request.SessionNonce &&
		confirmation.ManifestDigest == request.ManifestDigest && confirmation.ExpiresAt == request.ExpiresAt &&
		confirmation.ItemSetDigest == domainregistry.ProtectedRecoveryItemSetDigest(request.Entries)
}

func protectedConfirmationMatches(confirmation domainregistry.ProtectedRecoveryConfirmationV1, session domainregistry.ProtectedRecoverySessionV1) bool {
	return confirmation.RequestDigest == session.RequestDigest && confirmation.RequestFingerprint == domainregistry.ProtectedRecoveryFingerprint(session.RequestDigest) &&
		confirmation.OperationID == session.OperationID && confirmation.SessionNonce == session.SessionNonce &&
		confirmation.ManifestDigest == session.ManifestDigest && confirmation.ItemSetDigest == session.ItemSetDigest && confirmation.ExpiresAt == session.ExpiresAt
}

func validateProtectedOwnerInventory(owners []domainregistry.ProtectedRecoveryOwnerBindingV1) error {
	if len(owners) > domainregistry.ProtectedRecoveryMaxBindingEntries {
		return registryport.ErrInvalidRequest
	}
	seen := make(map[string]struct{}, len(owners))
	seenCorrelations := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		if owner.Validate() != nil {
			return registryport.ErrInvalidRequest
		}
		key := owner.Owner + "\x00" + owner.Provider + "\x00" + owner.AccountID + "\x00" + owner.ChannelID + "\x00" + owner.Purpose
		if _, duplicate := seen[key]; duplicate {
			return registryport.ErrConflict
		}
		if _, duplicate := seenCorrelations[owner.Correlation]; duplicate {
			return registryport.ErrConflict
		}
		seen[key] = struct{}{}
		seenCorrelations[owner.Correlation] = struct{}{}
	}
	return nil
}

// Continuation inventory is a fresh destination-local observation. Production
// inventory may not know the manifest-local correlation after restart, so an
// omitted correlation is permitted here and is resolved only by an exact,
// unique match against the durable admitted binding. Prepare and source bundle
// creation continue to use validateProtectedOwnerInventory and remain strict.
func validateProtectedContinuationOwnerInventory(owners []domainregistry.ProtectedRecoveryOwnerBindingV1) error {
	if len(owners) > domainregistry.ProtectedRecoveryMaxBindingEntries {
		return registryport.ErrInvalidRequest
	}
	seenBindings := make(map[string]struct{}, len(owners))
	seenCorrelations := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		validated := owner
		if validated.Correlation == "" {
			validated.Correlation = "account-0"
		}
		if validated.Validate() != nil {
			return registryport.ErrInvalidRequest
		}
		key := owner.Owner + "\x00" + owner.Provider + "\x00" + owner.AccountID + "\x00" + owner.ChannelID + "\x00" + owner.Purpose
		if _, duplicate := seenBindings[key]; duplicate {
			return registryport.ErrConflict
		}
		seenBindings[key] = struct{}{}
		if owner.Correlation == "" {
			continue
		}
		if _, duplicate := seenCorrelations[owner.Correlation]; duplicate {
			return registryport.ErrConflict
		}
		seenCorrelations[owner.Correlation] = struct{}{}
	}
	return nil
}

func cloneProtectedOwnerBindings(owners []domainregistry.ProtectedRecoveryOwnerBindingV1) []domainregistry.ProtectedRecoveryOwnerBindingV1 {
	result := make([]domainregistry.ProtectedRecoveryOwnerBindingV1, len(owners))
	copy(result, owners)
	sortProtectedOwnerBindings(result)
	return result
}

func sortProtectedOwnerBindings(owners []domainregistry.ProtectedRecoveryOwnerBindingV1) {
	sort.Slice(owners, func(left, right int) bool {
		l, r := owners[left], owners[right]
		if l.Owner != r.Owner {
			return l.Owner < r.Owner
		}
		if l.Provider != r.Provider {
			return l.Provider < r.Provider
		}
		if l.AccountID != r.AccountID {
			return l.AccountID < r.AccountID
		}
		if l.ChannelID != r.ChannelID {
			return l.ChannelID < r.ChannelID
		}
		if l.Purpose != r.Purpose {
			return l.Purpose < r.Purpose
		}
		if l.Correlation != r.Correlation {
			return l.Correlation < r.Correlation
		}
		return l.Fingerprint < r.Fingerprint
	})
}

func protectedOwnerBindingsEqual(left, right []domainregistry.ProtectedRecoveryOwnerBindingV1) bool {
	return reflect.DeepEqual(cloneProtectedOwnerBindings(left), cloneProtectedOwnerBindings(right))
}

// protectedOwnerBindingsForSession authenticates every exact binding captured
// by the durable session while allowing Main to pass unrelated current
// inventory after a restart. A supplied correlation remains exact; when Main's
// current inventory omits it, only the full stable binding may match and the
// durable admitted correlation is retained in the normalized result.
func protectedOwnerBindingsForSession(
	expected []domainregistry.ProtectedRecoveryOwnerBindingV1,
	actual []domainregistry.ProtectedRecoveryOwnerBindingV1,
) ([]domainregistry.ProtectedRecoveryOwnerBindingV1, bool) {
	if validateProtectedOwnerInventory(expected) != nil || validateProtectedContinuationOwnerInventory(actual) != nil {
		return nil, false
	}
	used := make([]bool, len(actual))
	result := make([]domainregistry.ProtectedRecoveryOwnerBindingV1, len(expected))
	for index, want := range expected {
		match := -1
		for candidateIndex, got := range actual {
			if used[candidateIndex] || got.Owner != want.Owner || got.Provider != want.Provider ||
				got.AccountID != want.AccountID || got.ChannelID != want.ChannelID ||
				got.Purpose != want.Purpose || got.Fingerprint != want.Fingerprint ||
				(got.Correlation != "" && got.Correlation != want.Correlation) {
				continue
			}
			if match != -1 {
				return nil, false
			}
			match = candidateIndex
		}
		if match == -1 {
			return nil, false
		}
		used[match] = true
		result[index] = want
	}
	return result, true
}

func protectedOwnerBindingsMatchSession(
	expected []domainregistry.ProtectedRecoveryOwnerBindingV1,
	actual []domainregistry.ProtectedRecoveryOwnerBindingV1,
) bool {
	_, ok := protectedOwnerBindingsForSession(expected, actual)
	return ok
}

func protectedOwnerMatchesScope(scope domainregistry.PrivateAccountScope, owners []domainregistry.ProtectedRecoveryOwnerBindingV1) bool {
	for _, owner := range owners {
		if owner.Owner == scope.Owner && owner.Provider == scope.Provider && owner.AccountID == scope.AccountID &&
			owner.ChannelID == scope.ChannelID && owner.Purpose == scope.Purpose {
			return true
		}
	}
	return false
}

func protectedOwnerForCorrelation(
	owners []domainregistry.ProtectedRecoveryOwnerBindingV1,
	correlation string,
) (domainregistry.ProtectedRecoveryOwnerBindingV1, bool) {
	var match domainregistry.ProtectedRecoveryOwnerBindingV1
	found := false
	for _, owner := range owners {
		if owner.Correlation != correlation {
			continue
		}
		if found {
			return domainregistry.ProtectedRecoveryOwnerBindingV1{}, false
		}
		match = owner
		found = true
	}
	return match, found
}

func protectedPurposeAllowed(purpose string) bool {
	switch purpose {
	case "provider-api-key", "provider-oauth-token-bundle", "mcp-oauth-access-token", "extension-provider-account-token":
		return true
	default:
		return false
	}
}

func protectedDestinationPurpose(provider domainregistry.Provider) string {
	if provider.PrivateAccount != nil {
		return provider.PrivateAccount.Purpose
	}
	if provider.OAuthBinding != nil {
		return "provider-oauth-token-bundle"
	}
	return "provider-api-key"
}

func protectedRecoveryExpectedCorrelation(manifest domainregistry.PortableManifestV1, index int) string {
	if index < len(manifest.Providers) {
		return "provider-" + fmt.Sprint(index)
	}
	return "account-" + fmt.Sprint(index-len(manifest.Providers))
}

func parseProtectedExpiry(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
