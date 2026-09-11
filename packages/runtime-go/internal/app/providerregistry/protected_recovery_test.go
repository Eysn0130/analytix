package providerregistry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	syntheticRecoverySourceProfile      = "synthetic-source-profile"
	syntheticRecoverySourceData         = "/isolated/synthetic-source-data"
	syntheticRecoveryDestinationProfile = "synthetic-destination-profile"
	syntheticRecoveryDestinationData    = "/isolated/synthetic-destination-data"
	syntheticRecoveryWindow             = "synthetic-main-window"
	syntheticRecoveryFrame              = "synthetic-main-frame"
	syntheticRecoverySecret             = "synthetic-source-protected-secret"
	syntheticRecoveryItemSetDigest      = "synthetic-item-set-digest"
)

// protectedRecoveryRecordingSecretStore is deliberately test-local. It makes
// the private K1 consumer contract observable without exposing plaintext to
// the request, bundle, receipt, or public result. The production Manager must
// select the operation-specific consumers; a caller cannot select them.
type protectedRecoveryRecordingSecretStore struct {
	delegate secretstoreport.RegistryStore

	mu       sync.Mutex
	prepares []protectedRecoveryPrepare
	accesses []secretstoreport.AccessRequest
}

type protectedRecoveryPrepare struct {
	purpose secretstoreport.Purpose
	ref     secretstoreport.CredentialRef
}

func (store *protectedRecoveryRecordingSecretStore) PreparePut(
	ctx context.Context,
	purpose secretstoreport.Purpose,
	secret []byte,
) (secretstoreport.PreparedCandidate, error) {
	candidate, err := store.delegate.PreparePut(ctx, purpose, secret)
	if err != nil {
		return nil, err
	}
	ref := candidate.CredentialRef()
	store.mu.Lock()
	store.prepares = append(store.prepares, protectedRecoveryPrepare{purpose: purpose, ref: ref})
	store.mu.Unlock()
	return &protectedRecoveryRecordingCandidate{delegate: candidate}, nil
}

func (store *protectedRecoveryRecordingSecretStore) GetForAuthorizedConsumer(
	ctx context.Context,
	request secretstoreport.AccessRequest,
) ([]byte, error) {
	store.mu.Lock()
	store.accesses = append(store.accesses, request)
	store.mu.Unlock()
	return store.delegate.GetForAuthorizedConsumer(ctx, request)
}

func (store *protectedRecoveryRecordingSecretStore) Tombstone(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
) error {
	return store.delegate.Tombstone(ctx, ref, purpose)
}

func (store *protectedRecoveryRecordingSecretStore) ExplicitDelete(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
	intent secretstoreport.CredentialMutation,
) error {
	return store.delegate.ExplicitDelete(ctx, ref, purpose, intent)
}

func (store *protectedRecoveryRecordingSecretStore) accessCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.accesses)
}

func (store *protectedRecoveryRecordingSecretStore) accessesSince(start int) []secretstoreport.AccessRequest {
	store.mu.Lock()
	defer store.mu.Unlock()
	if start < 0 || start > len(store.accesses) {
		return nil
	}
	return append([]secretstoreport.AccessRequest(nil), store.accesses[start:]...)
}

func (store *protectedRecoveryRecordingSecretStore) preparesSnapshot() []protectedRecoveryPrepare {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]protectedRecoveryPrepare(nil), store.prepares...)
}

type protectedRecoveryRecordingCandidate struct {
	delegate secretstoreport.PreparedCandidate
}

func (candidate *protectedRecoveryRecordingCandidate) CredentialRef() secretstoreport.CredentialRef {
	return candidate.delegate.CredentialRef()
}

func (candidate *protectedRecoveryRecordingCandidate) Commit(ctx context.Context) error {
	return candidate.delegate.Commit(ctx)
}

func (candidate *protectedRecoveryRecordingCandidate) Abort() {
	candidate.delegate.Abort()
}

type protectedRecoveryFixture struct {
	ctx context.Context

	source              *Manager
	sourceRegistry      *memoryRegistryStore
	sourceSecrets       *memorySecretStore
	sourceSecretRecords *protectedRecoveryRecordingSecretStore
	sourceProvider      domainregistry.Provider

	destination              *Manager
	destinationRegistry      *memoryRegistryStore
	destinationSecrets       *memorySecretStore
	destinationSecretRecords *protectedRecoveryRecordingSecretStore

	manifest       []byte
	imported       ProtectedRecoveryImportResult
	manifestDigest string
	itemSetDigest  string

	sourceBinding            domainregistry.ProtectedRecoveryLocalBindingV1
	destinationBinding       domainregistry.ProtectedRecoveryLocalBindingV1
	sourceOwnerBindings      []domainregistry.ProtectedRecoveryOwnerBindingV1
	destinationOwnerBindings []domainregistry.ProtectedRecoveryOwnerBindingV1
	sourceAction             domainregistry.ProtectedRecoveryLocalActionV1
	prepared                 domainregistry.ProtectedRecoveryPreparedRequestV1

	request []byte
	bundle  []byte
}

func newProtectedRecoveryFixture(t *testing.T) *protectedRecoveryFixture {
	t.Helper()
	ctx := context.Background()

	sourceRegistry := newMemoryRegistryStore()
	sourceSecrets := newMemorySecretStore()
	sourceSecretRecords := &protectedRecoveryRecordingSecretStore{delegate: sourceSecrets}
	source := mustManager(t, sourceRegistry, sourceSecretRecords, nil)
	sourceProvider := connectMemoryProvider(
		t, ctx, source, sourceRegistry, "provider-protected-source", syntheticRecoverySecret,
	)
	manifest, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest() error = %v", err)
	}

	destinationRegistry := newMemoryRegistryStore()
	destinationSecrets := newMemorySecretStore()
	destinationSecretRecords := &protectedRecoveryRecordingSecretStore{delegate: destinationSecrets}
	destination := mustManager(t, destinationRegistry, destinationSecretRecords, nil)
	imported, err := destination.ImportPortableManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("ImportPortableManifest() error = %v", err)
	}
	if len(imported.Entries) != 1 {
		t.Fatalf("portable import entries = %d, want one synthetic entry", len(imported.Entries))
	}
	privateImported := protectedRecoveryPrivateImportResult(
		t, destinationRegistry.snapshot(), manifest, imported, nil,
	)

	importReceiptBytes, err := json.Marshal(imported.Entries)
	if err != nil {
		t.Fatalf("Marshal(portable import correlation receipt) error = %v", err)
	}
	fixture := &protectedRecoveryFixture{
		ctx: ctx, source: source, sourceRegistry: sourceRegistry, sourceSecrets: sourceSecrets,
		sourceSecretRecords: sourceSecretRecords, sourceProvider: sourceProvider,
		destination: destination, destinationRegistry: destinationRegistry,
		destinationSecrets: destinationSecrets, destinationSecretRecords: destinationSecretRecords,
		manifest: manifest, imported: privateImported, manifestDigest: protectedRecoveryDigest(manifest),
		itemSetDigest: protectedRecoveryDigest(importReceiptBytes),
		sourceBinding: domainregistry.ProtectedRecoveryLocalBindingV1{
			BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
			ProfileBinding: syntheticRecoverySourceProfile, DataDirectoryBinding: syntheticRecoverySourceData,
		},
		destinationBinding: domainregistry.ProtectedRecoveryLocalBindingV1{
			BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
			ProfileBinding: syntheticRecoveryDestinationProfile, DataDirectoryBinding: syntheticRecoveryDestinationData,
		},
		sourceOwnerBindings:      []domainregistry.ProtectedRecoveryOwnerBindingV1{},
		destinationOwnerBindings: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
	}

	// Prepare receives only the authenticated Main local binding. It contains no
	// caller-selected operation, nonce, expiry, key, Registry fence, or
	// confirmation-consumed state; the Manager owns all generated session
	// material and confirmation is a separate phase below.
	fixture.prepared = fixture.createRequest(t)
	fixture.request = fixture.prepared.Request
	request := fixture.parseRequest(t, fixture.request)
	fixture.confirmDestination(t, fixture.prepared)
	fixture.sourceAction = domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: fixture.sourceBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest:      fixture.prepared.RequestDigest,
			RequestFingerprint: fixture.prepared.RequestFingerprint,
			OperationID:        fixture.prepared.OperationID,
			SessionNonce:       fixture.prepared.SessionNonce,
			ManifestDigest:     fixture.prepared.ManifestDigest,
			ItemSetDigest:      fixture.prepared.ItemSetDigest,
			ExpiresAt:          fixture.prepared.ExpiresAt,
			LocalBinding:       fixture.sourceBinding,
		},
	}
	if request.VerificationFingerprint != fixture.prepared.RequestFingerprint ||
		request.ManifestDigest != fixture.prepared.ManifestDigest || request.ExpiresAt != fixture.prepared.ExpiresAt {
		t.Fatal("prepared request metadata and canonical request disagree")
	}
	return fixture
}

func protectedRecoveryDigest(record []byte) string {
	digest := sha256.Sum256(record)
	return hex.EncodeToString(digest[:])
}

func protectedRecoveryPrivateImportResult(
	t *testing.T,
	state domainregistry.Registry,
	manifestBytes []byte,
	ordinary PortableImportResult,
	admittedOwners []domainregistry.ProtectedRecoveryOwnerBindingV1,
) ProtectedRecoveryImportResult {
	t.Helper()
	manifest, err := domainregistry.ParsePortableManifestV1(manifestBytes)
	if err != nil {
		t.Fatalf("ParsePortableManifestV1(private import receipt) error = %v", err)
	}
	result := ProtectedRecoveryImportResult{
		ProviderCount: ordinary.ProviderCount, AccountCount: ordinary.AccountCount,
		ReentryRequired: ordinary.ReentryRequired,
		Entries:         make([]ProtectedRecoveryImportEntryResult, 0, len(ordinary.Entries)),
	}
	for index, ordinaryEntry := range ordinary.Entries {
		provider, ok := state.Providers[ordinaryEntry.DestinationProviderID]
		if !ok {
			t.Fatalf("private import receipt destination %q missing", ordinaryEntry.DestinationProviderID)
		}
		entry := ProtectedRecoveryImportEntryResult{
			Correlation: ordinaryEntry.Correlation, DestinationProviderID: ordinaryEntry.DestinationProviderID,
			Status: ordinaryEntry.Status,
			Fence: ProtectedRecoveryImportFence{
				Revision: strconv.FormatUint(provider.Revision, 10), Generation: strconv.FormatUint(provider.Generation, 10),
				Incarnation: provider.Incarnation,
			},
		}
		if index >= len(manifest.Providers) {
			descriptor := manifest.Accounts[index-len(manifest.Providers)]
			owner, found := protectedOwnerForCorrelation(admittedOwners, descriptor.Correlation)
			if !found {
				t.Fatalf("private import receipt owner %q missing", descriptor.Correlation)
			}
			ownerCopy := owner
			entry.DestinationOwnerBinding = &ownerCopy
		}
		result.Entries = append(result.Entries, entry)
	}
	return result
}

func (fixture *protectedRecoveryFixture) createRequest(t *testing.T) domainregistry.ProtectedRecoveryPreparedRequestV1 {
	t.Helper()
	prepared, err := fixture.destination.PrepareProtectedRecoveryRequest(
		fixture.ctx,
		ProtectedRecoveryRequestCommand{
			ManifestJSON:          append([]byte(nil), fixture.manifest...),
			ImportResult:          fixture.imported,
			LocalBinding:          fixture.destinationBinding,
			OwnerBindingInventory: fixture.destinationOwnerBindings,
		},
	)
	if err != nil {
		t.Fatalf("PrepareProtectedRecoveryRequest() error = %v", err)
	}
	parsed := fixture.parseRequest(t, prepared.Request)
	if parsed.VerificationFingerprint == "" || parsed.OperationID == "" || parsed.SessionNonce == "" ||
		parsed.ExpiresAt == "" || len(parsed.DestinationEphemeralPublicKey) == 0 {
		t.Fatal("Manager did not generate complete bounded request session material")
	}
	if _, serializedDigest := requestObjectField(prepared.Request, "requestDigest"); serializedDigest {
		t.Fatal("request serialized a self-referential requestDigest")
	}
	assertProtectedRecoverySchema(t, "request", prepared.Request, domainregistry.ProtectedRecoveryRequestSchemaV1)
	assertProtectedRecoveryExternalRedacted(t, "request", prepared.Request,
		syntheticRecoverySourceProfile, syntheticRecoverySourceData,
		syntheticRecoveryDestinationProfile, syntheticRecoveryDestinationData,
		syntheticRecoveryWindow, syntheticRecoveryFrame,
	)
	if prepared.ManifestDigest != fixture.manifestDigest || prepared.ItemSetDigest == "" || prepared.RequestDigest == "" || prepared.RequestFingerprint == "" {
		t.Fatal("prepare result did not return bounded canonical request metadata")
	}
	fixture.itemSetDigest = prepared.ItemSetDigest
	return prepared
}

func (fixture *protectedRecoveryFixture) confirmDestination(t *testing.T, prepared domainregistry.ProtectedRecoveryPreparedRequestV1) {
	t.Helper()
	confirmation := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: fixture.destinationBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest:      prepared.RequestDigest,
			RequestFingerprint: prepared.RequestFingerprint,
			OperationID:        prepared.OperationID,
			SessionNonce:       prepared.SessionNonce,
			ManifestDigest:     prepared.ManifestDigest,
			ItemSetDigest:      prepared.ItemSetDigest,
			ExpiresAt:          prepared.ExpiresAt,
			LocalBinding:       fixture.destinationBinding,
		},
	}
	if err := fixture.destination.ConfirmProtectedRecoveryDestination(
		fixture.ctx,
		ProtectedRecoveryDestinationConfirmationCommand{
			Request:               prepared.Request,
			LocalAction:           confirmation,
			OwnerBindingInventory: fixture.destinationOwnerBindings,
		},
	); err != nil {
		t.Fatalf("ConfirmProtectedRecoveryDestination() error = %v", err)
	}
}

func (fixture *protectedRecoveryFixture) parseRequest(t *testing.T, request []byte) domainregistry.ProtectedRecoveryRequestV1 {
	t.Helper()
	parsed, err := domainregistry.ParseProtectedRecoveryRequestV1(request)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryRequestV1() error = %v", err)
	}
	return parsed
}

func (fixture *protectedRecoveryFixture) createBundle(t *testing.T) []byte {
	t.Helper()
	bundle, err := fixture.source.CreateProtectedRecoveryBundle(
		fixture.ctx,
		ProtectedRecoveryBundleCommand{
			Request:               fixture.request,
			LocalAction:           fixture.sourceAction,
			OwnerBindingInventory: fixture.sourceOwnerBindings,
		},
	)
	if err != nil {
		t.Fatalf("CreateProtectedRecoveryBundle() error = %v", err)
	}
	if _, err := domainregistry.ParseProtectedRecoveryBundleV1(bundle); err != nil {
		t.Fatalf("ParseProtectedRecoveryBundleV1() error = %v", err)
	}
	assertProtectedRecoverySchema(t, "bundle", bundle, domainregistry.ProtectedRecoveryBundleSchemaV1)
	assertProtectedRecoveryExternalRedacted(t, "bundle", bundle,
		syntheticRecoverySourceProfile, syntheticRecoverySourceData,
		syntheticRecoveryDestinationProfile, syntheticRecoveryDestinationData,
		syntheticRecoveryWindow, syntheticRecoveryFrame, fixture.sourceProvider.ID,
		fixture.sourceProvider.CredentialRef,
	)
	if bytes.Contains(bundle, []byte(syntheticRecoverySecret)) {
		t.Fatalf("bundle contains source credential plaintext marker")
	}
	return bundle
}

func requestObjectField(record []byte, field string) (json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(record, &object); err != nil {
		return nil, false
	}
	value, exists := object[field]
	return value, exists
}

func assertProtectedRecoverySchema(t *testing.T, label string, record []byte, want string) {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(record, &object); err != nil {
		t.Fatalf("%s is not JSON: %v", label, err)
	}
	if object["schema"] != want {
		t.Fatalf("%s schema = %#v, want %q", label, object["schema"], want)
	}
	if len(record) == 0 || !bytes.Equal(bytes.TrimSpace(record), record) {
		t.Fatalf("%s is not a bounded canonical byte sequence", label)
	}
}

func assertProtectedRecoveryExternalRedacted(t *testing.T, label string, record []byte, localValues ...string) {
	t.Helper()
	for _, forbidden := range append(localValues,
		syntheticRecoverySecret, "credentialRef", "sourceProviderID", "sourceAccountID", "sourceRegistry",
		"masterKey", "privateKey", "profileBinding", "dataDirectoryBinding", "BrowserWindowID", "MainFrameID",
	) {
		if strings.Contains(string(record), forbidden) {
			t.Fatalf("%s contains local/secret value %q", label, forbidden)
		}
	}
}

func assertProtectedRecoverySourceAuthorityUnchanged(t *testing.T, fixture *protectedRecoveryFixture, before domainregistry.Provider, beforeSelection string) {
	t.Helper()
	afterState := fixture.sourceRegistry.snapshot()
	after, ok := afterState.Providers[before.ID]
	if !ok {
		t.Fatalf("source Provider %q disappeared", before.ID)
	}
	if after.ID != before.ID || after.Revision != before.Revision || after.Generation != before.Generation ||
		after.Incarnation != before.Incarnation || after.CredentialRef != before.CredentialRef ||
		after.CredentialPurpose != before.CredentialPurpose || after.Tombstone != before.Tombstone ||
		!reflect.DeepEqual(after.PrivateAccount, before.PrivateAccount) {
		t.Fatalf("source Provider authority changed: before=%#v after=%#v", before, after)
	}
	if afterState.SelectedProviderID != beforeSelection {
		t.Fatalf("source selection changed: before=%q after=%q", beforeSelection, afterState.SelectedProviderID)
	}
	if !fixture.sourceSecrets.hasActive(before.CredentialRef) {
		t.Fatalf("source credential %q is no longer active", before.CredentialRef)
	}
}

func readProtectedRecoverySourceSecret(t *testing.T, fixture *protectedRecoveryFixture) []byte {
	t.Helper()
	state := fixture.sourceRegistry.snapshot()
	provider := state.Providers[fixture.sourceProvider.ID]
	secret, err := fixture.sourceSecretRecords.GetForAuthorizedConsumer(fixture.ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
		Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
		Consumer:      ProtectedTransferSourceConsumer,
	})
	if err != nil {
		t.Fatalf("source protected credential readback error = %v", err)
	}
	return secret
}

func assertProtectedRecoveryConsumers(
	t *testing.T,
	fixture *protectedRecoveryFixture,
	sourceAccessStart int,
	destinationAccessStart int,
) {
	t.Helper()
	sourceAccesses := fixture.sourceSecretRecords.accessesSince(sourceAccessStart)
	if len(sourceAccesses) == 0 {
		t.Fatal("protected bundle creation did not read a source credential")
	}
	for _, access := range sourceAccesses {
		if access.Consumer != ProtectedTransferSourceConsumer {
			t.Fatalf("protected source read used consumer %q", access.Consumer)
		}
	}
	destinationAccesses := fixture.destinationSecretRecords.accessesSince(destinationAccessStart)
	readback := false
	for _, access := range destinationAccesses {
		if access.Consumer == ProtectedRecoveryReadbackConsumer {
			readback = true
		}
		if access.Consumer == ProviderExecutionConsumer || access.Consumer == AccountExecutionConsumer || access.Consumer == RegistryReadbackConsumer {
			t.Fatalf("protected destination path used general consumer %q", access.Consumer)
		}
	}
	if !readback {
		t.Fatal("protected destination path did not use operation-specific readback consumer")
	}

	hasPendingKey := false
	var pending protectedRecoveryPrepare
	for _, prepare := range fixture.destinationSecretRecords.preparesSnapshot() {
		if prepare.purpose == ProtectedRecoveryPendingKeyPurpose {
			hasPendingKey = true
			pending = prepare
		}
	}
	if !hasPendingKey {
		t.Fatal("destination did not protect its ephemeral private key under the pending-key purpose")
	}
	if _, err := fixture.destinationSecretRecords.GetForAuthorizedConsumer(fixture.ctx, secretstoreport.AccessRequest{
		CredentialRef: pending.ref, Purpose: pending.purpose, Consumer: ProviderExecutionConsumer,
	}); err == nil {
		t.Fatal("general Provider consumer read a protected pending private key")
	}
	if fixture.destinationSecrets.hasActive(string(pending.ref)) {
		t.Fatal("successful protected recovery retained the ephemeral pending private key")
	}
}

func TestProtectedRecoveryV1RequestBundleDestinationBatchReadbackReceipt(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	sourceBefore := fixture.sourceRegistry.snapshot()
	sourceProviderBefore := sourceBefore.Providers[fixture.sourceProvider.ID]
	destinationBefore := fixture.destinationRegistry.snapshot()
	sourceSelectionBefore := sourceBefore.SelectedProviderID
	sourceAccessStart := fixture.sourceSecretRecords.accessCount()
	fixture.bundle = fixture.createBundle(t)
	destinationAccessStart := fixture.destinationSecretRecords.accessCount()
	destinationPreparedBefore := fixture.destinationSecrets.preparedCount()

	receipt, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx,
		ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	)
	if err != nil {
		t.Fatalf("ApplyProtectedRecoveryBundle() error = %v", err)
	}
	if _, err := domainregistry.ParseProtectedRecoveryReceiptV1(receipt); err != nil {
		t.Fatalf("ParseProtectedRecoveryReceiptV1() error = %v", err)
	}
	assertProtectedRecoverySchema(t, "receipt", receipt, domainregistry.ProtectedRecoveryReceiptSchemaV1)
	assertProtectedRecoveryExternalRedacted(t, "receipt", receipt,
		syntheticRecoverySourceProfile, syntheticRecoverySourceData,
		syntheticRecoveryDestinationProfile, syntheticRecoveryDestinationData,
		syntheticRecoveryWindow, syntheticRecoveryFrame, fixture.sourceProvider.ID,
		sourceProviderBefore.CredentialRef,
	)

	destinationAfterApply := fixture.destinationRegistry.snapshot()
	destinationPreparedAfterApply := fixture.destinationSecrets.preparedCount()
	if destinationPreparedAfterApply <= destinationPreparedBefore {
		t.Fatal("destination K1 was not prepared by protected recovery")
	}

	// The consumed pending session rejects bundle replay. The receipt remains
	// available for source finalization/audit, but replay cannot return it or
	// create another winner.
	priorReplay := destinationAfterApply
	preparedBeforeReplay := fixture.destinationSecrets.preparedCount()
	readBeforeReplay, tombstoneBeforeReplay, deleteBeforeReplay := fixture.destinationSecrets.callCounts()
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); err == nil {
		t.Fatal("post-terminal bundle replay unexpectedly succeeded")
	}
	if !reflect.DeepEqual(priorReplay, fixture.destinationRegistry.snapshot()) {
		t.Fatal("post-terminal bundle replay changed destination Registry")
	}
	if fixture.destinationSecrets.preparedCount() != preparedBeforeReplay {
		t.Fatal("post-terminal bundle replay prepared a second destination credential")
	}
	readAfterReplay, tombstoneAfterReplay, deleteAfterReplay := fixture.destinationSecrets.callCounts()
	if readAfterReplay != readBeforeReplay || tombstoneAfterReplay != tombstoneBeforeReplay || deleteAfterReplay != deleteBeforeReplay {
		t.Fatal("post-terminal bundle replay touched Secret Store")
	}
	result, err := fixture.source.FinalizeProtectedRecoveryReceipt(
		fixture.ctx, ProtectedRecoveryFinalizeCommand{Receipt: receipt},
	)
	if err != nil {
		t.Fatalf("FinalizeProtectedRecoveryReceipt() error = %v", err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal(ProtectedRecoveryResultV1) error = %v", err)
	}
	var resultObject map[string]any
	if err := json.Unmarshal(resultJSON, &resultObject); err != nil {
		t.Fatalf("ProtectedRecoveryResultV1 JSON error = %v", err)
	}
	if resultObject["status"] != "finalized" {
		t.Fatalf("protected recovery result status = %#v, want finalized", resultObject["status"])
	}
	finalState := fixture.sourceRegistry.snapshot()
	finalSession, ok := finalState.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || finalSession.Phase != domainregistry.ProtectedRecoveryPhaseFinalized ||
		len(finalSession.Bundle) != 0 || len(finalSession.Receipt) != 0 ||
		len(finalSession.Request) != 0 || len(finalSession.ManifestJSON) != 0 ||
		len(finalSession.OwnerBindings) != 0 || finalSession.LocalBinding != (domainregistry.ProtectedRecoveryLocalBindingV1{}) {
		t.Fatalf("finalized source session retained non-terminal authority/transcript: %#v", finalSession)
	}
	assertProtectedRecoveryExternalRedacted(t, "result", resultJSON,
		syntheticRecoverySourceProfile, syntheticRecoverySourceData,
		syntheticRecoveryDestinationProfile, syntheticRecoveryDestinationData,
		syntheticRecoveryWindow, syntheticRecoveryFrame, sourceProviderBefore.CredentialRef,
	)

	destinationAfter := fixture.destinationRegistry.snapshot()
	importedProvider := destinationAfter.Providers[fixture.imported.Entries[0].DestinationProviderID]
	if importedProvider.CredentialRef == "" || importedProvider.CredentialPurpose != "provider-api-key" {
		t.Fatalf("destination imported provider did not receive a destination K1 credential: %#v", importedProvider)
	}
	if destinationAfter.SelectedProviderID != destinationBefore.SelectedProviderID {
		t.Fatalf("protected recovery transferred destination selection: before=%q after=%q", destinationBefore.SelectedProviderID, destinationAfter.SelectedProviderID)
	}
	assertProtectedRecoverySourceAuthorityUnchanged(t, fixture, sourceProviderBefore, sourceSelectionBefore)
	if !bytes.Equal(readProtectedRecoverySourceSecret(t, fixture), []byte(syntheticRecoverySecret)) {
		t.Fatal("source credential readback changed")
	}
	sourceReadAfter, sourceTombstoneAfter, sourceDeleteAfter := fixture.sourceSecrets.callCounts()
	if sourceReadAfter <= 0 || sourceTombstoneAfter != 0 || sourceDeleteAfter != 0 {
		t.Fatalf("source Secret Store calls = (%d,%d,%d); source must only be read", sourceReadAfter, sourceTombstoneAfter, sourceDeleteAfter)
	}
	if reflect.DeepEqual(destinationBefore, destinationAfter) {
		t.Fatal("successful protected recovery did not commit a destination Registry successor")
	}
	assertProtectedRecoveryConsumers(t, fixture, sourceAccessStart, destinationAccessStart)
}

func TestProtectedRecoveryV1AppliedTargetedRecoverReturnsDurableReceiptWithoutUncredentialedCurrentness(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	receipt, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx,
		ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	)
	if err != nil {
		t.Fatalf("ApplyProtectedRecoveryBundle(applied targeted recovery setup) error = %v", err)
	}
	state := fixture.destinationRegistry.snapshot()
	session, ok := state.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseApplied || session.DestinationKeyRef != "" {
		t.Fatalf("applied targeted recovery setup phase = %q, destination_key_present=%t, want applied/false",
			session.Phase, session.DestinationKeyRef != "")
	}
	action := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: fixture.destinationBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: fixture.prepared.RequestDigest, RequestFingerprint: fixture.prepared.RequestFingerprint,
			OperationID: fixture.prepared.OperationID, SessionNonce: fixture.prepared.SessionNonce,
			ManifestDigest: fixture.prepared.ManifestDigest, ItemSetDigest: fixture.prepared.ItemSetDigest,
			ExpiresAt: fixture.prepared.ExpiresAt, LocalBinding: fixture.destinationBinding,
		},
	}
	preparedBefore := fixture.destinationSecrets.preparedCount()
	accessBefore := fixture.destinationSecretRecords.accessCount()
	destinationBefore := fixture.destinationRegistry.snapshot()
	sourceBefore := fixture.sourceRegistry.snapshot()
	sourceAccessBefore := fixture.sourceSecretRecords.accessCount()

	result, err := fixture.destination.RecoverProtectedRecovery(fixture.ctx, ProtectedRecoveryRecoverCommand{
		Request: fixture.request, LocalAction: action, OwnerBindingInventory: fixture.destinationOwnerBindings,
	})
	if err != nil {
		t.Fatalf("RecoverProtectedRecovery(applied targeted) first_boundary=pre_recovery_uncredentialed_currentness error=%v k1_prepare_delta=%d k1_read_delta=%d registry_mutation=%t source_effect=%t",
			err,
			fixture.destinationSecrets.preparedCount()-preparedBefore,
			fixture.destinationSecretRecords.accessCount()-accessBefore,
			!reflect.DeepEqual(destinationBefore, fixture.destinationRegistry.snapshot()),
			!reflect.DeepEqual(sourceBefore, fixture.sourceRegistry.snapshot()) || fixture.sourceSecretRecords.accessCount() != sourceAccessBefore,
		)
	}
	if result.Status != "receipt_ready" || !result.Confirmed || !bytes.Equal(result.Receipt, receipt) {
		t.Fatalf("RecoverProtectedRecovery(applied targeted) status=%q confirmed=%t receipt_match=%t, want receipt_ready/true/true",
			result.Status, result.Confirmed, bytes.Equal(result.Receipt, receipt))
	}
	if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
		t.Fatalf("RecoverProtectedRecovery(applied targeted) K1 PreparePut delta = %d, want 0", delta)
	}
	if !reflect.DeepEqual(destinationBefore, fixture.destinationRegistry.snapshot()) {
		t.Fatal("RecoverProtectedRecovery(applied targeted) Registry mutation reached = true, want false")
	}
	if !reflect.DeepEqual(sourceBefore, fixture.sourceRegistry.snapshot()) ||
		fixture.sourceSecretRecords.accessCount() != sourceAccessBefore {
		t.Fatal("RecoverProtectedRecovery(applied targeted) source effect reached = true, want false")
	}
}

func TestProtectedRecoveryV1AppliedTargetedRecoverRejectsSuccessorFenceDriftBeforeK1(t *testing.T) {
	for _, drift := range []string{"revision", "generation", "incarnation"} {
		t.Run(drift, func(t *testing.T) {
			fixture := newProtectedRecoveryFixture(t)
			fixture.bundle = fixture.createBundle(t)
			if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
				fixture.ctx,
				ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
			); err != nil {
				t.Fatalf("ApplyProtectedRecoveryBundle(successor drift setup) error = %v", err)
			}
			providerID := fixture.imported.Entries[0].DestinationProviderID
			fixture.destinationRegistry.mutate(func(state *domainregistry.Registry) {
				provider := state.Providers[providerID]
				switch drift {
				case "revision":
					provider.Revision++
				case "generation":
					provider.Generation++
				case "incarnation":
					provider.Incarnation = "inc_" + strings.Repeat("s", 43)
				}
				state.Providers[providerID] = provider
			})
			action := domainregistry.ProtectedRecoveryLocalActionV1{
				LocalBinding: fixture.destinationBinding,
				Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
					RequestDigest: fixture.prepared.RequestDigest, RequestFingerprint: fixture.prepared.RequestFingerprint,
					OperationID: fixture.prepared.OperationID, SessionNonce: fixture.prepared.SessionNonce,
					ManifestDigest: fixture.prepared.ManifestDigest, ItemSetDigest: fixture.prepared.ItemSetDigest,
					ExpiresAt: fixture.prepared.ExpiresAt, LocalBinding: fixture.destinationBinding,
				},
			}
			prior := fixture.destinationRegistry.snapshot()
			preparedBefore := fixture.destinationSecrets.preparedCount()
			accessBefore := fixture.destinationSecretRecords.accessCount()
			sourceBefore := fixture.sourceRegistry.snapshot()
			sourceAccessBefore := fixture.sourceSecretRecords.accessCount()

			_, err := fixture.destination.RecoverProtectedRecovery(fixture.ctx, ProtectedRecoveryRecoverCommand{
				Request: fixture.request, LocalAction: action, OwnerBindingInventory: fixture.destinationOwnerBindings,
			})
			if err == nil {
				t.Fatalf("applied targeted %s successor drift error = nil, want fail closed", drift)
			}
			if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
				t.Fatalf("applied targeted %s successor drift K1 PreparePut delta = %d, want 0", drift, delta)
			}
			if delta := fixture.destinationSecretRecords.accessCount() - accessBefore; delta != 0 {
				t.Fatalf("applied targeted %s successor drift K1 Get delta = %d, want 0", drift, delta)
			}
			if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
				t.Fatalf("applied targeted %s successor drift Registry mutation reached = true, want false", drift)
			}
			if !reflect.DeepEqual(sourceBefore, fixture.sourceRegistry.snapshot()) ||
				fixture.sourceSecretRecords.accessCount() != sourceAccessBefore {
				t.Fatalf("applied targeted %s successor drift source effect reached = true, want false", drift)
			}
		})
	}
}

func TestProtectedRecoveryV1ForgedAuthenticatedReceiptCannotFinalizeSource(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	receipt, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	)
	if err != nil {
		t.Fatalf("ApplyProtectedRecoveryBundle() error = %v", err)
	}
	parsed, err := domainregistry.ParseProtectedRecoveryReceiptV1(receipt)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryReceiptV1() error = %v", err)
	}
	if len(parsed.AuthenticationTag) != 32 {
		t.Fatalf("receipt authentication tag length = %d, want 32", len(parsed.AuthenticationTag))
	}
	parsed.AuthenticationTag = bytes.Repeat([]byte{0xA5}, 32)
	forged, err := domainregistry.MarshalProtectedRecoveryReceiptV1(parsed)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryReceiptV1(forged) error = %v", err)
	}
	if bytes.Equal(forged, receipt) {
		t.Fatal("forged receipt did not change the authenticated transcript")
	}
	prior := fixture.sourceRegistry.snapshot()
	readBefore, tombstoneBefore, deleteBefore := fixture.sourceSecrets.callCounts()
	if _, err := fixture.source.FinalizeProtectedRecoveryReceipt(
		fixture.ctx, ProtectedRecoveryFinalizeCommand{Receipt: forged},
	); err == nil {
		t.Fatal("syntactically valid forged authenticated receipt finalized source cleanup")
	}
	if !reflect.DeepEqual(prior, fixture.sourceRegistry.snapshot()) {
		t.Fatal("forged authenticated receipt changed source Registry")
	}
	readAfter, tombstoneAfter, deleteAfter := fixture.sourceSecrets.callCounts()
	if readAfter != readBefore || tombstoneAfter != tombstoneBefore || deleteAfter != deleteBefore {
		t.Fatalf("forged receipt touched source Secret Store: before=(%d,%d,%d) after=(%d,%d,%d)",
			readBefore, tombstoneBefore, deleteBefore, readAfter, tombstoneAfter, deleteAfter)
	}
	state := fixture.sourceRegistry.snapshot()
	session, ok := state.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseSourceBundle || session.Consumed {
		t.Fatalf("forged receipt source session = %#v, want unconsumed source bundle", session)
	}
	if _, err := fixture.source.FinalizeProtectedRecoveryReceipt(
		fixture.ctx, ProtectedRecoveryFinalizeCommand{Receipt: receipt},
	); err != nil {
		t.Fatalf("FinalizeProtectedRecoveryReceipt(authenticated receipt) error = %v", err)
	}
}

func resetProtectedRecoveryPreparationFixture(t *testing.T) *protectedRecoveryFixture {
	t.Helper()
	fixture := newProtectedRecoveryFixture(t)
	if _, err := fixture.destination.RollbackProtectedRecovery(
		fixture.ctx, ProtectedRecoveryRollbackCommand{Request: fixture.request},
	); err != nil {
		t.Fatalf("RollbackProtectedRecovery(reset preparation fixture) error = %v", err)
	}
	return fixture
}

func TestProtectedRecoveryV1PendingKeyPublicationFailuresRetainNoUncommittedWinner(t *testing.T) {
	t.Run("first Registry journal failure aborts uncommitted key candidate", func(t *testing.T) {
		fixture := resetProtectedRecoveryPreparationFixture(t)
		prior := fixture.destinationRegistry.snapshot()
		preparedBefore := fixture.destinationSecrets.preparedCount()
		fixture.destinationRegistry.stateMu.Lock()
		fixture.destinationRegistry.failNextCommit = true
		fixture.destinationRegistry.stateMu.Unlock()
		prepared, err := fixture.destination.PrepareProtectedRecoveryRequest(
			fixture.ctx,
			ProtectedRecoveryRequestCommand{
				ManifestJSON: fixture.manifest, ImportResult: fixture.imported,
				LocalBinding: fixture.destinationBinding, OwnerBindingInventory: fixture.destinationOwnerBindings,
			},
		)
		if !errors.Is(err, registryport.ErrPersistence) {
			t.Fatalf("PrepareProtectedRecoveryRequest(first journal failure) error = %v, want persistence", err)
		}
		if len(prepared.Request) != 0 || !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
			t.Fatal("failed first journal publication returned or persisted a protected request")
		}
		if fixture.destinationSecrets.preparedCount() <= preparedBefore || fixture.destinationSecrets.recordCount() != 0 {
			t.Fatal("failed first journal publication retained an uncommitted Secret Store candidate")
		}
	})

	t.Run("candidate commit failure leaves no durable winner", func(t *testing.T) {
		fixture := resetProtectedRecoveryPreparationFixture(t)
		prior := fixture.destinationRegistry.snapshot()
		fixture.destinationSecrets.failCandidateCommit = true
		prepared, err := fixture.destination.PrepareProtectedRecoveryRequest(
			fixture.ctx,
			ProtectedRecoveryRequestCommand{
				ManifestJSON: fixture.manifest, ImportResult: fixture.imported,
				LocalBinding: fixture.destinationBinding, OwnerBindingInventory: fixture.destinationOwnerBindings,
			},
		)
		if !errors.Is(err, registryport.ErrPersistence) {
			t.Fatalf("PrepareProtectedRecoveryRequest(candidate failure) error = %v, want persistence", err)
		}
		after := fixture.destinationRegistry.snapshot()
		if len(prepared.Request) != 0 || len(after.ProtectedRecoverySessions) != 0 ||
			!reflect.DeepEqual(prior.Providers, after.Providers) || prior.SelectedProviderID != after.SelectedProviderID ||
			fixture.destinationSecrets.recordCount() != 0 {
			t.Fatal("candidate commit failure published a request, Registry session, or Secret Store winner")
		}
	})

	t.Run("pending publication failure is recoverable and does not publish request", func(t *testing.T) {
		fixture := resetProtectedRecoveryPreparationFixture(t)
		fixture.destinationRegistry.stateMu.Lock()
		fixture.destinationRegistry.failCommitAt = fixture.destinationRegistry.commits + 2 // journal, then Pending publication
		fixture.destinationRegistry.stateMu.Unlock()
		prepared, err := fixture.destination.PrepareProtectedRecoveryRequest(
			fixture.ctx,
			ProtectedRecoveryRequestCommand{
				ManifestJSON: fixture.manifest, ImportResult: fixture.imported,
				LocalBinding: fixture.destinationBinding, OwnerBindingInventory: fixture.destinationOwnerBindings,
			},
		)
		if !errors.Is(err, registryport.ErrPersistence) || len(prepared.Request) != 0 {
			t.Fatalf("PrepareProtectedRecoveryRequest(pending publication failure) = %#v, %v", prepared, err)
		}
		interrupted := fixture.destinationRegistry.snapshot()
		var operationID string
		for id := range interrupted.ProtectedRecoverySessions {
			operationID = id
		}
		session, ok := interrupted.ProtectedRecoverySessions[operationID]
		if !ok || session.Phase != domainregistry.ProtectedRecoveryPhasePendingKeyCandidate || session.DestinationKeyRef == "" || !fixture.destinationSecrets.hasActive(session.DestinationKeyRef) {
			t.Fatalf("pending-key journal after publication failure = %#v", session)
		}
		fixture.destinationRegistry.clearFailures()
		status, err := fixture.destination.RecoverProtectedRecovery(fixture.ctx)
		if err != nil || status.Status != "none" {
			t.Fatalf("RecoverProtectedRecovery(pending-key journal) = %#v, %v; want no published request", status, err)
		}
		if fixture.destinationSecrets.recordCount() != 0 || len(fixture.destinationRegistry.snapshot().ProtectedRecoverySessions) != 0 {
			t.Fatal("pending-key recovery did not clean the exact pre-Pending artifacts")
		}
	})

	t.Run("pending key readback failure cleans exact journal", func(t *testing.T) {
		fixture := resetProtectedRecoveryPreparationFixture(t)
		fixture.destinationSecrets.wrongNextRead = true
		prior := fixture.destinationRegistry.snapshot()
		prepared, err := fixture.destination.PrepareProtectedRecoveryRequest(
			fixture.ctx,
			ProtectedRecoveryRequestCommand{
				ManifestJSON: fixture.manifest, ImportResult: fixture.imported,
				LocalBinding: fixture.destinationBinding, OwnerBindingInventory: fixture.destinationOwnerBindings,
			},
		)
		if !errors.Is(err, registryport.ErrVerification) || len(prepared.Request) != 0 {
			t.Fatalf("PrepareProtectedRecoveryRequest(pending-key readback failure) = %#v, %v", prepared, err)
		}
		after := fixture.destinationRegistry.snapshot()
		if len(prepared.Request) != 0 || len(after.ProtectedRecoverySessions) != 0 ||
			!reflect.DeepEqual(prior.Providers, after.Providers) || prior.SelectedProviderID != after.SelectedProviderID ||
			fixture.destinationSecrets.recordCount() != 0 {
			t.Fatal("pending-key readback failure left Registry or Secret Store authority")
		}
	})

	t.Run("cleanup failure retains retry authority", func(t *testing.T) {
		fixture := resetProtectedRecoveryPreparationFixture(t)
		fixture.destinationRegistry.stateMu.Lock()
		fixture.destinationRegistry.failCommitAt = fixture.destinationRegistry.commits + 2
		fixture.destinationRegistry.stateMu.Unlock()
		fixture.destinationSecrets.failTombstone = true
		if _, err := fixture.destination.PrepareProtectedRecoveryRequest(
			fixture.ctx,
			ProtectedRecoveryRequestCommand{
				ManifestJSON: fixture.manifest, ImportResult: fixture.imported,
				LocalBinding: fixture.destinationBinding, OwnerBindingInventory: fixture.destinationOwnerBindings,
			},
		); !errors.Is(err, registryport.ErrPersistence) {
			t.Fatalf("PrepareProtectedRecoveryRequest(cleanup failure setup) error = %v, want persistence", err)
		}
		pending := fixture.destinationRegistry.snapshot()
		var operationID string
		for id := range pending.ProtectedRecoverySessions {
			operationID = id
		}
		session, ok := pending.ProtectedRecoverySessions[operationID]
		if !ok || session.Phase != domainregistry.ProtectedRecoveryPhasePendingKeyCandidate || !fixture.destinationSecrets.hasActive(session.DestinationKeyRef) {
			t.Fatalf("cleanup failure lost pending-key retry authority: %#v", session)
		}
		fixture.destinationRegistry.clearFailures()
		if _, err := fixture.destination.RecoverProtectedRecovery(fixture.ctx); !errors.Is(err, registryport.ErrPersistence) {
			t.Fatalf("RecoverProtectedRecovery(first cleanup retry) error = %v, want persistence", err)
		}
		cleanupPending := fixture.destinationRegistry.snapshot().ProtectedRecoverySessions[operationID]
		if cleanupPending.Phase != domainregistry.ProtectedRecoveryPhaseCleanupPending || cleanupPending.CleanupKind != protectedRecoveryCleanupKey {
			t.Fatalf("cleanup failure phase = %#v, want cleanup_pending", cleanupPending)
		}
		fixture.destinationSecrets.clearFailures()
		status, err := fixture.destination.RecoverProtectedRecovery(fixture.ctx)
		if err != nil || status.Status != "pending" || status.Confirmed {
			t.Fatalf("RecoverProtectedRecovery(cleanup retry) = %#v, %v", status, err)
		}
		if fixture.destinationSecrets.recordCount() != 0 {
			t.Fatal("successful pending-key cleanup retained a Secret Store artifact")
		}
		cleaned := fixture.destinationRegistry.snapshot().ProtectedRecoverySessions[operationID]
		if cleaned.Phase != domainregistry.ProtectedRecoveryPhasePending || cleaned.DestinationKeyRef != "" {
			t.Fatalf("cleaned pending-key session = %#v", cleaned)
		}
	})
}

func TestProtectedRecoveryV1BatchFailureLeavesNoPartialWinner(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	prior := fixture.destinationRegistry.snapshot()
	preparedBefore := fixture.destinationSecrets.preparedCount()
	recordsBefore := fixture.destinationSecrets.recordCount()

	fixture.destinationSecrets.failCandidateCommit = true
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); err == nil {
		t.Fatal("candidate failure unexpectedly committed protected recovery")
	}
	if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
		t.Fatal("candidate failure changed destination Registry")
	}
	if fixture.destinationSecrets.recordCount() != recordsBefore {
		t.Fatal("candidate failure left a partial destination Secret Store winner")
	}
	if fixture.destinationSecrets.preparedCount() <= preparedBefore {
		t.Fatal("candidate failure did not exercise the destination batch preparation")
	}

	fixture.destinationSecrets.clearFailures()
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); err != nil {
		t.Fatalf("retry after candidate failure error = %v", err)
	}
	for _, entry := range fixture.imported.Entries {
		provider := fixture.destinationRegistry.snapshot().Providers[entry.DestinationProviderID]
		if provider.CredentialRef == "" || provider.CredentialPurpose == "" {
			t.Fatalf("retry did not publish the complete destination winner: %#v", provider)
		}
	}
}

func TestProtectedRecoveryV1CandidatesDurableRestartRollsBackExactCandidates(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	prior := fixture.destinationRegistry.snapshot()
	importedID := fixture.imported.Entries[0].DestinationProviderID
	priorImported := prior.Providers[importedID]

	fixture.destinationRegistry.stateMu.Lock()
	fixture.destinationRegistry.failCommitAt = fixture.destinationRegistry.commits + 2 // candidate journal, then batch CAS
	fixture.destinationRegistry.stateMu.Unlock()
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ApplyProtectedRecoveryBundle(candidate durable interruption) error = %v, want persistence", err)
	}
	interrupted := fixture.destinationRegistry.snapshot()
	session, ok := interrupted.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseCandidatesDurable || len(session.CandidateRefs) != 1 {
		t.Fatalf("interrupted protected session = %#v, want candidates_durable with one candidate", session)
	}
	if interrupted.Providers[importedID].CredentialRef != "" || !reflect.DeepEqual(priorImported, interrupted.Providers[importedID]) {
		t.Fatal("candidate-durable interruption published a destination credential")
	}
	for _, ref := range session.CandidateRefs {
		if !fixture.destinationSecrets.hasActive(ref) {
			t.Fatalf("candidate-durable interruption lost active retry candidate %q before recovery", ref)
		}
	}

	fixture.destinationRegistry.clearFailures()
	restarted := mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
	recovered, err := restarted.Snapshot(fixture.ctx)
	if err != nil {
		t.Fatalf("Snapshot(recover candidates-durable) error = %v", err)
	}
	if !reflect.DeepEqual(priorImported, recovered.Providers[importedID]) || recovered.SelectedProviderID != prior.SelectedProviderID {
		t.Fatal("candidate-durable recovery changed imported entry or selection")
	}
	recoveredSession := recovered.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if recoveredSession.Phase != domainregistry.ProtectedRecoveryPhasePending || len(recoveredSession.CandidateRefs) != 0 || recoveredSession.DestinationKeyRef == "" {
		t.Fatalf("candidate-durable recovery session = %#v, want pending with retained pending-key ref", recoveredSession)
	}
	if fixture.destinationSecrets.recordCount() != 1 {
		t.Fatalf("candidate-durable recovery Secret Store record count = %d, want only pending key", fixture.destinationSecrets.recordCount())
	}
}

func TestProtectedRecoveryV1VerificationPendingRestartVerifiesBeforePublication(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	importedID := fixture.imported.Entries[0].DestinationProviderID

	fixture.destinationRegistry.stateMu.Lock()
	fixture.destinationRegistry.failCommitAt = fixture.destinationRegistry.commits + 3 // candidate journal, batch CAS, Applied journal
	fixture.destinationRegistry.stateMu.Unlock()
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ApplyProtectedRecoveryBundle(verification-pending interruption) error = %v, want persistence", err)
	}
	interrupted := fixture.destinationRegistry.snapshot()
	session, ok := interrupted.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseVerificationPending || len(session.CandidateRefs) != 1 || session.BatchRegistryRevision == 0 {
		t.Fatalf("verification-pending session = %#v", session)
	}
	if interrupted.Providers[importedID].CredentialRef == "" {
		t.Fatal("verification-pending test did not exercise the published candidate state")
	}

	fixture.destinationRegistry.clearFailures()
	restarted := mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
	recovered, err := restarted.Snapshot(fixture.ctx)
	if err != nil {
		t.Fatalf("Snapshot(recover verification-pending) error = %v", err)
	}
	recoveredSession, ok := recovered.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || recoveredSession.Phase != domainregistry.ProtectedRecoveryPhaseApplied || !recoveredSession.Consumed || recoveredSession.DestinationKeyRef != "" {
		t.Fatalf("verified protected session = %#v, want applied/consumed with cleaned key", recoveredSession)
	}
	if recovered.Providers[importedID].CredentialRef == "" || recovered.Providers[importedID].CredentialPurpose != "provider-api-key" {
		t.Fatalf("verified destination winner = %#v", recovered.Providers[importedID])
	}
}

func TestProtectedRecoveryV1WrongByteReadbackRestoresPriorRegistry(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	prior := fixture.destinationRegistry.snapshot()
	recordsBefore := fixture.destinationSecrets.recordCount()
	fixture.destinationSecrets.wrongProtectedReadback = true

	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ApplyProtectedRecoveryBundle(wrong protected readback) error = %v, want verification", err)
	}
	if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
		t.Fatal("wrong protected readback did not restore the exact prior Registry")
	}
	if fixture.destinationSecrets.recordCount() != recordsBefore {
		t.Fatal("wrong protected readback left a destination candidate")
	}
}

func TestProtectedRecoveryV1CleanupFailureIsDurableAndRetryable(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	fixture.destinationSecrets.failTombstone = true

	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ApplyProtectedRecoveryBundle(cleanup failure) error = %v, want persistence", err)
	}
	state := fixture.destinationRegistry.snapshot()
	session, ok := state.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseCleanupPending || session.CleanupKind != protectedRecoveryCleanupKey || len(session.CleanupRefs) != 1 {
		t.Fatalf("cleanup-failure session = %#v, want durable key cleanup", session)
	}
	if _, err := fixture.destination.Snapshot(fixture.ctx); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("Snapshot(unresolved cleanup) error = %v, want persistence", err)
	}
	// Recovery gates every ordinary authority consumer, not only Snapshot. A
	// cleanup-pending winner must not be selectable or executable while the
	// exact Secret Store cleanup remains unverified.
	unresolved := fixture.destinationRegistry.snapshot()
	importedID := fixture.imported.Entries[0].DestinationProviderID
	if _, err := fixture.destination.Select(fixture.ctx, SelectCommand{
		Expected: expectedFor(unresolved, importedID), ProviderID: importedID,
	}); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("Select(unresolved cleanup) error = %v, want persistence", err)
	}
	if _, err := fixture.destination.ResolveSelectedIntent(fixture.ctx); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ResolveSelectedIntent(unresolved cleanup) error = %v, want persistence", err)
	}

	fixture.destinationSecrets.clearFailures()
	recovered, err := fixture.destination.Snapshot(fixture.ctx)
	if err != nil {
		t.Fatalf("Snapshot(retry cleanup) error = %v", err)
	}
	session = recovered.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if session.Phase != domainregistry.ProtectedRecoveryPhaseApplied || session.DestinationKeyRef != "" || len(session.CleanupRefs) != 0 {
		t.Fatalf("retried cleanup session = %#v, want applied with no pending key", session)
	}
	if recovered.Providers[fixture.imported.Entries[0].DestinationProviderID].CredentialRef == "" || fixture.destinationSecrets.recordCount() != 1 {
		t.Fatal("cleanup retry did not retain exactly one verified destination winner")
	}
}

func TestProtectedRecoveryV1CleanupRestartRejectsSuccessorFenceDriftBeforeK1Cleanup(t *testing.T) {
	for _, drift := range []string{"revision", "generation", "incarnation"} {
		t.Run(drift, func(t *testing.T) {
			fixture := newProtectedRecoveryFixture(t)
			fixture.bundle = fixture.createBundle(t)
			fixture.destinationSecrets.failTombstone = true
			if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
				fixture.ctx,
				ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
			); !errors.Is(err, registryport.ErrPersistence) {
				t.Fatalf("ApplyProtectedRecoveryBundle(cleanup restart drift setup) error = %v, want persistence", err)
			}
			fixture.destinationSecrets.clearFailures()
			providerID := fixture.imported.Entries[0].DestinationProviderID
			fixture.destinationRegistry.mutate(func(state *domainregistry.Registry) {
				provider := state.Providers[providerID]
				switch drift {
				case "revision":
					provider.Revision++
				case "generation":
					provider.Generation++
				case "incarnation":
					provider.Incarnation = "inc_" + strings.Repeat("k", 43)
				}
				state.Providers[providerID] = provider
			})
			prior := fixture.destinationRegistry.snapshot()
			preparedBefore := fixture.destinationSecrets.preparedCount()
			accessBefore := fixture.destinationSecretRecords.accessCount()
			readBefore, tombstoneBefore, deleteBefore := fixture.destinationSecrets.callCounts()
			sourceBefore := fixture.sourceRegistry.snapshot()
			sourceAccessBefore := fixture.sourceSecretRecords.accessCount()

			restarted := mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
			if _, err := restarted.Snapshot(fixture.ctx); !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("Snapshot(cleanup restart %s drift) error = %v, want verification", drift, err)
			}
			if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
				t.Fatalf("cleanup restart %s drift K1 PreparePut delta = %d, want 0", drift, delta)
			}
			if delta := fixture.destinationSecretRecords.accessCount() - accessBefore; delta != 0 {
				t.Fatalf("cleanup restart %s drift K1 Get delta = %d, want 0", drift, delta)
			}
			readAfter, tombstoneAfter, deleteAfter := fixture.destinationSecrets.callCounts()
			if readAfter != readBefore || tombstoneAfter != tombstoneBefore || deleteAfter != deleteBefore {
				t.Fatalf("cleanup restart %s drift K1 effect delta = (%d,%d,%d), want (0,0,0)", drift,
					readAfter-readBefore, tombstoneAfter-tombstoneBefore, deleteAfter-deleteBefore)
			}
			if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
				t.Fatalf("cleanup restart %s drift Registry mutation reached = true, want false", drift)
			}
			if !reflect.DeepEqual(sourceBefore, fixture.sourceRegistry.snapshot()) ||
				fixture.sourceSecretRecords.accessCount() != sourceAccessBefore {
				t.Fatalf("cleanup restart %s drift source effect reached = true, want false", drift)
			}
		})
	}
}

func TestProtectedRecoveryV1RollbackCleanupFailureRetainsRetryAuthority(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	prior := fixture.destinationRegistry.snapshot()
	importedID := fixture.imported.Entries[0].DestinationProviderID
	fixture.destinationSecrets.failTombstone = true

	if _, err := fixture.destination.RollbackProtectedRecovery(
		fixture.ctx, ProtectedRecoveryRollbackCommand{Request: fixture.request},
	); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("RollbackProtectedRecovery(cleanup failure) error = %v, want persistence", err)
	}
	pending := fixture.destinationRegistry.snapshot()
	session, ok := pending.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseCleanupPending || session.CleanupKind != protectedRecoveryCleanupAll || len(session.CleanupRefs) != 1 {
		t.Fatalf("rollback cleanup-failure session = %#v", session)
	}
	if !reflect.DeepEqual(prior.Providers[importedID], pending.Providers[importedID]) || pending.Providers[importedID].CredentialRef != "" {
		t.Fatal("rollback cleanup failure changed imported uncredentialed entry")
	}

	fixture.destinationSecrets.clearFailures()
	if _, err := fixture.destination.Snapshot(fixture.ctx); err != nil {
		t.Fatalf("Snapshot(retry rollback cleanup) error = %v", err)
	}
	cleaned := fixture.destinationRegistry.snapshot()
	session = cleaned.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if session.Phase != domainregistry.ProtectedRecoveryPhasePending || session.DestinationKeyRef != "" || len(session.CleanupRefs) != 0 {
		t.Fatalf("retried rollback cleanup session = %#v", session)
	}
	if fixture.destinationSecrets.recordCount() != 0 || !reflect.DeepEqual(prior.Providers[importedID], cleaned.Providers[importedID]) {
		t.Fatal("rollback retry did not clean only exact app-owned artifacts")
	}
}

func TestProtectedRecoveryV1VerificationDriftRestoresPriorAuthority(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	prior := fixture.destinationRegistry.snapshot()
	importedID := fixture.imported.Entries[0].DestinationProviderID
	priorProvider := prior.Providers[importedID]

	// Stop after the Provider batch has been journaled. The verification phase
	// must remain durable so a restart can detect a Registry successor drift.
	fixture.destinationRegistry.stateMu.Lock()
	fixture.destinationRegistry.failCommitAt = fixture.destinationRegistry.commits + 3
	fixture.destinationRegistry.stateMu.Unlock()
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ApplyProtectedRecoveryBundle(verification journal) error = %v, want persistence", err)
	}
	fixture.destinationRegistry.clearFailures()

	// Simulate an external committed Registry successor drift. It remains a
	// valid Registry, but no longer matches the exact Provider fence recorded by
	// the protected journal. Recovery must restore the prior logical Provider
	// authority and retain no executable candidate.
	fixture.destinationRegistry.mutate(func(state *domainregistry.Registry) {
		provider := state.Providers[importedID]
		provider.Revision++
		provider.Generation++
		state.Revision++
		state.Providers[importedID] = provider
	})
	if _, err := fixture.destination.Snapshot(fixture.ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("Snapshot(Registry reload drift) error = %v, want verification", err)
	}
	recovered := fixture.destinationRegistry.snapshot()
	if !reflect.DeepEqual(priorProvider, recovered.Providers[importedID]) {
		t.Fatalf("Registry reload drift did not restore prior Provider: %#v", recovered.Providers[importedID])
	}
	recoveredSession, ok := recovered.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || recoveredSession.Phase != domainregistry.ProtectedRecoveryPhasePending ||
		len(recoveredSession.CandidateRefs) != 0 || recoveredSession.DestinationKeyRef == "" {
		t.Fatalf("Registry reload drift recovery session = %#v, want pending without candidates", recoveredSession)
	}
}

func TestProtectedRecoveryV1RollbackCommitFailureRetainsCandidateJournal(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	prior := fixture.destinationRegistry.snapshot()
	importedID := fixture.imported.Entries[0].DestinationProviderID
	priorProvider := prior.Providers[importedID]

	// Force candidate readback to fail, then fail the synchronous restoration
	// commit. The durable CandidatesDurable journal must retain exact cleanup
	// refs instead of claiming rollback succeeded.
	fixture.destinationSecrets.wrongProtectedReadback = true
	fixture.destinationRegistry.stateMu.Lock()
	fixture.destinationRegistry.failCommitAt = fixture.destinationRegistry.commits + 2
	fixture.destinationRegistry.stateMu.Unlock()
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ApplyProtectedRecoveryBundle(rollback commit failure) error = %v, want persistence", err)
	}
	interrupted := fixture.destinationRegistry.snapshot()
	session, ok := interrupted.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if !ok || session.Phase != domainregistry.ProtectedRecoveryPhaseCandidatesDurable || len(session.CandidateRefs) != 1 {
		t.Fatalf("rollback commit failure session = %#v, want candidate journal", session)
	}
	if !reflect.DeepEqual(priorProvider, interrupted.Providers[importedID]) || interrupted.Providers[importedID].CredentialRef != "" {
		t.Fatal("rollback commit failure published a destination Provider winner")
	}
	for _, ref := range session.CandidateRefs {
		if fixture.destinationSecrets.hasActive(ref) {
			t.Fatalf("rollback commit failure left candidate %q active", ref)
		}
	}

	fixture.destinationRegistry.clearFailures()
	recovered, err := fixture.destination.Snapshot(fixture.ctx)
	if err != nil {
		t.Fatalf("Snapshot(retry rollback commit) error = %v", err)
	}
	if !reflect.DeepEqual(priorProvider, recovered.Providers[importedID]) {
		t.Fatal("rollback commit retry changed the prior Provider authority")
	}
	recoveredSession := recovered.ProtectedRecoverySessions[fixture.prepared.OperationID]
	if recoveredSession.Phase != domainregistry.ProtectedRecoveryPhasePending || len(recoveredSession.CandidateRefs) != 0 || recoveredSession.DestinationKeyRef == "" {
		t.Fatalf("rollback commit retry session = %#v, want clean pending retry state", recoveredSession)
	}
}

func TestProtectedRecoveryV1PrepareCrossChecksCanonicalImportReceiptBeforePending(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	if _, err := fixture.destination.RollbackProtectedRecovery(
		fixture.ctx, ProtectedRecoveryRollbackCommand{Request: fixture.request},
	); err != nil {
		t.Fatalf("RollbackProtectedRecovery(preflight setup) error = %v", err)
	}
	prior := fixture.destinationRegistry.snapshot()
	preparedBefore := fixture.destinationSecrets.preparedCount()
	readBefore, tombstoneBefore, deleteBefore := fixture.destinationSecrets.callCounts()

	forged := fixture.imported
	forged.Entries = append([]ProtectedRecoveryImportEntryResult(nil), fixture.imported.Entries...)
	forged.Entries[0].DestinationProviderID = "provider-forged-destination"
	if _, err := fixture.destination.PrepareProtectedRecoveryRequest(
		fixture.ctx,
		ProtectedRecoveryRequestCommand{
			// The manifest bytes and their digest are valid; only the caller's
			// correlation result is forged. The Manager must compare both with
			// current imported Registry metadata before creating pending state.
			ManifestJSON:          append([]byte(nil), fixture.manifest...),
			ImportResult:          forged,
			LocalBinding:          fixture.destinationBinding,
			OwnerBindingInventory: fixture.destinationOwnerBindings,
		},
	); err == nil {
		t.Fatal("PrepareProtectedRecoveryRequest accepted a forged import correlation result")
	}
	if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
		t.Fatal("forged import correlation changed destination Registry")
	}
	if fixture.destinationSecrets.preparedCount() != preparedBefore {
		t.Fatal("forged import correlation prepared a protected key/candidate")
	}
	readAfter, tombstoneAfter, deleteAfter := fixture.destinationSecrets.callCounts()
	if readAfter != readBefore || tombstoneAfter != tombstoneBefore || deleteAfter != deleteBefore {
		t.Fatal("forged import correlation touched Secret Store")
	}
}

func TestProtectedRecoveryV1PrepareRejectsOrdinaryImportFenceDriftBeforePending(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*domainregistry.Registry, string)
	}{
		{
			name: "revision",
			mutate: func(state *domainregistry.Registry, providerID string) {
				provider := state.Providers[providerID]
				provider.Revision++
				state.Providers[providerID] = provider
			},
		},
		{
			name: "generation",
			mutate: func(state *domainregistry.Registry, providerID string) {
				provider := state.Providers[providerID]
				provider.Generation++
				state.Providers[providerID] = provider
			},
		},
		{
			name: "incarnation",
			mutate: func(state *domainregistry.Registry, providerID string) {
				provider := state.Providers[providerID]
				provider.Incarnation = "inc_" + strings.Repeat("i", 43)
				state.Providers[providerID] = provider
			},
		},
		{
			name: "replacement",
			mutate: func(state *domainregistry.Registry, providerID string) {
				replacement := state.Providers[providerID].Clone()
				replacement.Revision++
				replacement.Generation++
				state.Providers[providerID] = replacement
			},
		},
		{
			name: "recreation",
			mutate: func(state *domainregistry.Registry, providerID string) {
				recreated := state.Providers[providerID].Clone()
				recreated.Revision = 1
				recreated.Generation = 1
				recreated.Incarnation = "inc_" + strings.Repeat("r", 43)
				state.Providers[providerID] = recreated
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newProtectedRecoveryFixture(t)
			if _, err := fixture.destination.RollbackProtectedRecovery(
				fixture.ctx, ProtectedRecoveryRollbackCommand{Request: fixture.request},
			); err != nil {
				t.Fatalf("RollbackProtectedRecovery(fence drift setup) error = %v", err)
			}
			providerID := fixture.imported.Entries[0].DestinationProviderID
			fixture.destinationRegistry.mutate(func(state *domainregistry.Registry) {
				testCase.mutate(state, providerID)
			})
			prior := fixture.destinationRegistry.snapshot()
			preparedBefore := fixture.destinationSecrets.preparedCount()
			accessBefore := fixture.destinationSecretRecords.accessCount()

			_, err := fixture.destination.PrepareProtectedRecoveryRequest(
				fixture.ctx,
				ProtectedRecoveryRequestCommand{
					ManifestJSON:          append([]byte(nil), fixture.manifest...),
					ImportResult:          fixture.imported,
					LocalBinding:          fixture.destinationBinding,
					OwnerBindingInventory: fixture.destinationOwnerBindings,
				},
			)
			if err == nil {
				t.Errorf("PrepareProtectedRecoveryRequest(%s drift) error = nil, want fail closed", testCase.name)
			}
			if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
				t.Errorf("PrepareProtectedRecoveryRequest(%s drift) K1 PreparePut delta = %d, want 0", testCase.name, delta)
			}
			if delta := fixture.destinationSecretRecords.accessCount() - accessBefore; delta != 0 {
				t.Errorf("PrepareProtectedRecoveryRequest(%s drift) K1 Get delta = %d, want 0", testCase.name, delta)
			}
			if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
				t.Errorf("PrepareProtectedRecoveryRequest(%s drift) Registry mutation reached = true, want false", testCase.name)
			}
		})
	}
}

func TestProtectedRecoveryV1DestinationContinuationsRejectAdmittedFenceDriftBeforeEffects(t *testing.T) {
	for _, operation := range []string{"confirm", "targeted-recover", "apply", "restart"} {
		for _, drift := range []string{"revision", "generation", "incarnation"} {
			t.Run(operation+"/"+drift, func(t *testing.T) {
				fixture := newProtectedRecoveryFixture(t)
				request := fixture.request
				prepared := fixture.prepared
				manager := fixture.destination
				if operation == "confirm" {
					if _, err := fixture.destination.RollbackProtectedRecovery(
						fixture.ctx, ProtectedRecoveryRollbackCommand{Request: fixture.request},
					); err != nil {
						t.Fatalf("RollbackProtectedRecovery(confirm setup) error = %v", err)
					}
					prepared = fixture.createRequest(t)
					request = prepared.Request
				}
				if operation == "apply" {
					fixture.bundle = fixture.createBundle(t)
				}
				if operation == "restart" {
					manager = mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
				}
				providerID := fixture.imported.Entries[0].DestinationProviderID
				fixture.destinationRegistry.mutate(func(state *domainregistry.Registry) {
					provider := state.Providers[providerID]
					switch drift {
					case "revision":
						provider.Revision++
					case "generation":
						provider.Generation++
					case "incarnation":
						provider.Incarnation = "inc_" + strings.Repeat("c", 43)
					}
					state.Providers[providerID] = provider
				})
				prior := fixture.destinationRegistry.snapshot()
				preparedBefore := fixture.destinationSecrets.preparedCount()
				accessBefore := fixture.destinationSecretRecords.accessCount()
				sourceBefore := fixture.sourceRegistry.snapshot()
				sourceAccessBefore := fixture.sourceSecretRecords.accessCount()
				confirmation := domainregistry.ProtectedRecoveryLocalActionV1{
					LocalBinding: fixture.destinationBinding,
					Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
						RequestDigest: prepared.RequestDigest, RequestFingerprint: prepared.RequestFingerprint,
						OperationID: prepared.OperationID, SessionNonce: prepared.SessionNonce,
						ManifestDigest: prepared.ManifestDigest, ItemSetDigest: prepared.ItemSetDigest,
						ExpiresAt: prepared.ExpiresAt, LocalBinding: fixture.destinationBinding,
					},
				}

				var err error
				switch operation {
				case "confirm":
					err = manager.ConfirmProtectedRecoveryDestination(fixture.ctx, ProtectedRecoveryDestinationConfirmationCommand{
						Request: request, LocalAction: confirmation, OwnerBindingInventory: fixture.destinationOwnerBindings,
					})
				case "targeted-recover":
					_, err = manager.RecoverProtectedRecovery(fixture.ctx, ProtectedRecoveryRecoverCommand{
						Request: request, LocalAction: confirmation, OwnerBindingInventory: fixture.destinationOwnerBindings,
					})
				case "apply":
					_, err = manager.ApplyProtectedRecoveryBundle(fixture.ctx, ProtectedRecoveryApplyCommand{
						Bundle: fixture.bundle, Request: request, OwnerBindingInventory: fixture.destinationOwnerBindings,
					})
				case "restart":
					err = manager.Recover(fixture.ctx)
				}
				if err == nil {
					t.Fatalf("%s %s admitted fence drift error = nil, want fail closed", operation, drift)
				}
				if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
					t.Fatalf("%s %s admitted fence drift K1 PreparePut delta = %d, want 0", operation, drift, delta)
				}
				if delta := fixture.destinationSecretRecords.accessCount() - accessBefore; delta != 0 {
					t.Fatalf("%s %s admitted fence drift K1 Get delta = %d, want 0", operation, drift, delta)
				}
				if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
					t.Fatalf("%s %s admitted fence drift Registry mutation reached = true, want false", operation, drift)
				}
				if !reflect.DeepEqual(sourceBefore, fixture.sourceRegistry.snapshot()) ||
					fixture.sourceSecretRecords.accessCount() != sourceAccessBefore {
					t.Fatalf("%s %s admitted fence drift source effect reached = true, want false", operation, drift)
				}
			})
		}
	}
}

func TestProtectedRecoveryV1UnconfirmedPrepareRequiresConfirmationAndRollback(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	if _, err := fixture.destination.RollbackProtectedRecovery(
		fixture.ctx, ProtectedRecoveryRollbackCommand{Request: fixture.request},
	); err != nil {
		t.Fatalf("RollbackProtectedRecovery(preflight setup) error = %v", err)
	}
	prior := fixture.destinationRegistry.snapshot()
	importedID := fixture.imported.Entries[0].DestinationProviderID
	priorImported := prior.Providers[importedID]

	prepared, err := fixture.destination.PrepareProtectedRecoveryRequest(
		fixture.ctx,
		ProtectedRecoveryRequestCommand{
			ManifestJSON:          append([]byte(nil), fixture.manifest...),
			ImportResult:          fixture.imported,
			LocalBinding:          fixture.destinationBinding,
			OwnerBindingInventory: fixture.destinationOwnerBindings,
		},
	)
	if err != nil {
		t.Fatalf("PrepareProtectedRecoveryRequest(unconfirmed) error = %v", err)
	}
	// Preparation durably records an unconfirmed, non-executable session. The
	// failed apply must leave that pending record exactly as prepared; the
	// imported Provider and its fence are checked against the pre-prepare
	// snapshot below.
	pendingBeforeApply := fixture.destinationRegistry.snapshot()
	fixture.request = prepared.Request
	fixture.sourceAction.Confirmation = domainregistry.ProtectedRecoveryConfirmationV1{
		RequestDigest: prepared.RequestDigest, RequestFingerprint: prepared.RequestFingerprint,
		OperationID: prepared.OperationID, SessionNonce: prepared.SessionNonce,
		ManifestDigest: prepared.ManifestDigest, ItemSetDigest: prepared.ItemSetDigest,
		ExpiresAt: prepared.ExpiresAt, LocalBinding: fixture.sourceBinding,
	}
	validBundle := fixture.createBundle(t)
	pending, err := fixture.destination.RecoverProtectedRecovery(fixture.ctx)
	if err != nil {
		t.Fatalf("RecoverProtectedRecovery(unconfirmed) error = %v", err)
	}
	pendingJSON, err := json.Marshal(pending)
	if err != nil {
		t.Fatalf("Marshal(unconfirmed pending status) error = %v", err)
	}
	var pendingObject map[string]any
	if err := json.Unmarshal(pendingJSON, &pendingObject); err != nil {
		t.Fatalf("unconfirmed pending status JSON error = %v", err)
	}
	if pendingObject["status"] != "pending" || pendingObject["confirmed"] != false {
		t.Fatalf("unconfirmed pending status = %#v, want pending/confirmed=false", pendingObject)
	}
	if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx,
		ProtectedRecoveryApplyCommand{Bundle: validBundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); err == nil {
		t.Fatal("unconfirmed pending session applied without a destination confirmation")
	}
	if !reflect.DeepEqual(pendingBeforeApply, fixture.destinationRegistry.snapshot()) {
		t.Fatal("unconfirmed apply changed destination Registry")
	}

	if _, err := fixture.destination.RollbackProtectedRecovery(
		fixture.ctx, ProtectedRecoveryRollbackCommand{Request: prepared.Request},
	); err != nil {
		t.Fatalf("RollbackProtectedRecovery(unconfirmed) error = %v", err)
	}
	after := fixture.destinationRegistry.snapshot()
	if !reflect.DeepEqual(priorImported, after.Providers[importedID]) || after.SelectedProviderID != prior.SelectedProviderID {
		t.Fatal("unconfirmed rollback changed the imported entry or selection")
	}
	if after.Providers[importedID].CredentialRef != "" || after.Providers[importedID].CredentialPurpose != "" {
		t.Fatal("unconfirmed rollback left the imported entry executable")
	}
}

func mutateProtectedRecoveryDestinationRequest(t *testing.T, request []byte) []byte {
	t.Helper()
	parsed, err := domainregistry.ParseProtectedRecoveryRequestV1(request)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryRequestV1() error = %v", err)
	}
	parsed.DestinationEphemeralPublicKey = bytes.Repeat([]byte("x"), len(parsed.DestinationEphemeralPublicKey))
	mutated, err := domainregistry.MarshalProtectedRecoveryRequestV1(parsed)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryRequestV1() error = %v", err)
	}
	return mutated
}

func TestProtectedRecoveryV1AADTranscriptChangesRejectBeforeMutation(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	prior := fixture.destinationRegistry.snapshot()
	preparedBefore := fixture.destinationSecrets.preparedCount()
	readBefore, tombstoneBefore, deleteBefore := fixture.destinationSecrets.callCounts()

	tests := []struct {
		name          string
		mutateBundle  func(*domainregistry.ProtectedRecoveryBundleV1)
		mutateRequest bool
	}{
		{name: "manifest digest", mutateBundle: func(bundle *domainregistry.ProtectedRecoveryBundleV1) {
			bundle.ManifestDigest = strings.Repeat("a", 64)
		}},
		{name: "request digest", mutateBundle: func(bundle *domainregistry.ProtectedRecoveryBundleV1) {
			bundle.RequestDigest = strings.Repeat("b", 64)
		}},
		{name: "destination public key", mutateRequest: true},
		{name: "source public key"},
		{name: "operation id", mutateBundle: func(bundle *domainregistry.ProtectedRecoveryBundleV1) {
			bundle.OperationID = "synthetic-other-operation"
		}},
		{name: "session nonce", mutateBundle: func(bundle *domainregistry.ProtectedRecoveryBundleV1) {
			bundle.SessionNonce = "synthetic-other-nonce"
		}},
		{name: "mapping", mutateBundle: func(bundle *domainregistry.ProtectedRecoveryBundleV1) {
			bundle.Entries[0].DestinationProviderID = "provider-other-destination"
		}},
		{name: "purpose", mutateBundle: func(bundle *domainregistry.ProtectedRecoveryBundleV1) {
			bundle.Entries[0].Purpose = "provider-oauth-token-bundle"
		}},
		{name: "expiry"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle, err := domainregistry.ParseProtectedRecoveryBundleV1(fixture.bundle)
			if err != nil {
				t.Fatalf("ParseProtectedRecoveryBundleV1() error = %v", err)
			}
			if test.mutateBundle != nil {
				test.mutateBundle(&bundle)
			}
			var mutated []byte
			if test.name == "expiry" {
				mutated = mutateProtectedRecoveryBundleExpiryRaw(t, fixture.bundle, "2026-09-02T00:00:00.000Z")
			} else if test.name == "source public key" {
				mutated = mutateProtectedRecoveryBundleSourceKeyRaw(t, fixture.bundle)
			} else {
				mutated, err = domainregistry.MarshalProtectedRecoveryBundleV1(bundle)
				if err != nil {
					t.Fatalf("MarshalProtectedRecoveryBundleV1() error = %v", err)
				}
			}
			command := ProtectedRecoveryApplyCommand{Bundle: mutated, OwnerBindingInventory: fixture.destinationOwnerBindings}
			if test.mutateRequest {
				// Request bytes are the native file input. The Manager still loads
				// its own pending session and key; this is not caller key authority.
				command.Request = mutateProtectedRecoveryDestinationRequest(t, fixture.request)
			}
			if _, err := fixture.destination.ApplyProtectedRecoveryBundle(fixture.ctx, command); err == nil {
				t.Fatalf("AAD/transcript mutation %q unexpectedly committed", test.name)
			}
			if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
				t.Fatalf("AAD/transcript mutation %q changed destination Registry", test.name)
			}
			if fixture.destinationSecrets.preparedCount() != preparedBefore {
				t.Fatalf("AAD/transcript mutation %q prepared destination K1", test.name)
			}
			readAfter, tombstoneAfter, deleteAfter := fixture.destinationSecrets.callCounts()
			if readAfter != readBefore || tombstoneAfter != tombstoneBefore || deleteAfter != deleteBefore {
				t.Fatalf("AAD/transcript mutation %q touched Secret Store", test.name)
			}
		})
	}
}

func mutateProtectedRecoveryBundleExpiryRaw(t *testing.T, record []byte, expiry string) []byte {
	t.Helper()
	bundle, err := domainregistry.ParseProtectedRecoveryBundleV1(record)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryBundleV1() error = %v", err)
	}
	oldExpiry, err := json.Marshal(bundle.ExpiresAt)
	if err != nil {
		t.Fatalf("Marshal(old protected recovery expiry) error = %v", err)
	}
	newExpiry, err := json.Marshal(expiry)
	if err != nil {
		t.Fatalf("Marshal(hostile protected recovery expiry) error = %v", err)
	}
	oldField := append([]byte(`"expiresAt":`), oldExpiry...)
	newField := append([]byte(`"expiresAt":`), newExpiry...)
	mutated := bytes.Replace(record, oldField, newField, 1)
	if bytes.Equal(mutated, record) {
		t.Fatal("hostile expiry fixture did not change raw bundle bytes")
	}
	return mutated
}

func mutateProtectedRecoveryBundleSourceKeyRaw(t *testing.T, record []byte) []byte {
	t.Helper()
	bundle, err := domainregistry.ParseProtectedRecoveryBundleV1(record)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryBundleV1() error = %v", err)
	}
	oldKey, err := json.Marshal(bundle.SourceEphemeralPublicKey)
	if err != nil {
		t.Fatalf("Marshal(old source public key) error = %v", err)
	}
	newKey, err := json.Marshal(bytes.Repeat([]byte("x"), 31))
	if err != nil {
		t.Fatalf("Marshal(hostile source public key) error = %v", err)
	}
	oldField := append([]byte(`"sourceEphemeralPublicKey":`), oldKey...)
	newField := append([]byte(`"sourceEphemeralPublicKey":`), newKey...)
	mutated := bytes.Replace(record, oldField, newField, 1)
	if bytes.Equal(mutated, record) {
		t.Fatal("hostile source public key fixture did not change raw bundle bytes")
	}
	return mutated
}

func TestProtectedRecoveryV1RestartNeedsFreshDestinationConfirmationAndRollback(t *testing.T) {
	fixture := newProtectedRecoveryFixture(t)
	fixture.bundle = fixture.createBundle(t)
	priorState := fixture.destinationRegistry.snapshot()
	importedID := fixture.imported.Entries[0].DestinationProviderID
	priorImported := priorState.Providers[importedID]
	restarted := mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)

	// Apply itself must recover the process-local confirmation boundary. A
	// restart cannot publish merely because the durable session says confirmed;
	// it must first record a fresh destination confirmation.
	if _, err := restarted.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); err == nil {
		t.Fatal("direct restart apply published without a fresh destination confirmation")
	}
	if current := fixture.destinationRegistry.snapshot().Providers[importedID]; !reflect.DeepEqual(priorImported, current) || current.CredentialRef != "" {
		t.Fatal("direct restart apply changed the imported uncredentialed entry")
	}

	pending, err := restarted.RecoverProtectedRecovery(fixture.ctx)
	if err != nil {
		t.Fatalf("RecoverProtectedRecovery(restart) error = %v", err)
	}
	pendingJSON, err := json.Marshal(pending)
	if err != nil {
		t.Fatalf("Marshal(pending protected recovery status) error = %v", err)
	}
	var pendingObject map[string]any
	if err := json.Unmarshal(pendingJSON, &pendingObject); err != nil {
		t.Fatalf("pending protected recovery status JSON error = %v", err)
	}
	if pendingObject["status"] != "reconfirmation_required" || pendingObject["confirmed"] != false {
		t.Fatalf("restart status = %#v, want reconfirmation_required/confirmed=false", pendingObject)
	}
	assertProtectedRecoveryExternalRedacted(t, "pending status", pendingJSON,
		syntheticRecoverySourceProfile, syntheticRecoverySourceData,
		syntheticRecoveryDestinationProfile, syntheticRecoveryDestinationData,
		syntheticRecoveryWindow, syntheticRecoveryFrame, syntheticRecoverySecret,
	)

	if _, err := restarted.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); err == nil {
		t.Fatal("restart auto-published without a newly recorded destination confirmation")
	}
	current := fixture.destinationRegistry.snapshot()
	if !reflect.DeepEqual(priorImported, current.Providers[importedID]) || current.SelectedProviderID != priorState.SelectedProviderID {
		t.Fatal("stale restart path changed the imported uncredentialed entry or selection")
	}
	if current.Providers[importedID].CredentialRef != "" || current.Providers[importedID].CredentialPurpose != "" {
		t.Fatal("stale restart path made an imported entry executable")
	}

	freshAction := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: fixture.destinationBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest:      fixture.prepared.RequestDigest,
			RequestFingerprint: fixture.prepared.RequestFingerprint,
			OperationID:        fixture.prepared.OperationID,
			SessionNonce:       fixture.prepared.SessionNonce,
			ManifestDigest:     fixture.prepared.ManifestDigest,
			ItemSetDigest:      fixture.prepared.ItemSetDigest,
			ExpiresAt:          fixture.prepared.ExpiresAt,
			LocalBinding:       fixture.destinationBinding,
		},
	}
	if err := restarted.ConfirmProtectedRecoveryDestination(
		fixture.ctx,
		ProtectedRecoveryDestinationConfirmationCommand{
			Request: fixture.request, LocalAction: freshAction,
			OwnerBindingInventory: fixture.destinationOwnerBindings,
		},
	); err != nil {
		t.Fatalf("ConfirmProtectedRecoveryDestination(restart) error = %v", err)
	}
	if _, err := restarted.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwnerBindings},
	); err != nil {
		t.Fatalf("freshly confirmed protected recovery apply error = %v", err)
	}

	// A separate uncommitted pending session exercises rollback cleanup. Only
	// exact app-owned pending key/candidate artifacts may be removed; the
	// ordinary imported entry and its fence remain untouched.
	rollbackFixture := newProtectedRecoveryFixture(t)
	rollbackFixture.bundle = rollbackFixture.createBundle(t)
	rollbackPrior := rollbackFixture.destinationRegistry.snapshot()
	rollbackImportedID := rollbackFixture.imported.Entries[0].DestinationProviderID
	rollbackImported := rollbackPrior.Providers[rollbackImportedID]
	rollbackRecordsBefore := rollbackFixture.destinationSecrets.recordCount()
	rollbackManager := mustManager(t, rollbackFixture.destinationRegistry, rollbackFixture.destinationSecretRecords, nil)
	if _, err := rollbackManager.RollbackProtectedRecovery(
		rollbackFixture.ctx,
		ProtectedRecoveryRollbackCommand{Request: rollbackFixture.request},
	); err != nil {
		t.Fatalf("RollbackProtectedRecovery() error = %v", err)
	}
	rollbackAfter := rollbackFixture.destinationRegistry.snapshot()
	if !reflect.DeepEqual(rollbackImported, rollbackAfter.Providers[rollbackImportedID]) || rollbackAfter.SelectedProviderID != rollbackPrior.SelectedProviderID {
		t.Fatal("rollback changed imported uncredentialed entry or selection")
	}
	if rollbackAfter.Providers[rollbackImportedID].CredentialRef != "" || rollbackAfter.Providers[rollbackImportedID].CredentialPurpose != "" {
		t.Fatal("rollback left imported entry executable")
	}
	if rollbackFixture.destinationSecrets.recordCount() >= rollbackRecordsBefore {
		t.Fatal("rollback did not clean exact pending private-key/candidate artifacts")
	}
}

type protectedRecoveryPurposeCase struct {
	name             string
	purpose          string
	sourceProviderID string
	sourceProvider   domainregistry.Provider
	scope            *domainregistry.PrivateAccountScope
	destinationOwner domainregistry.ProtectedRecoveryOwnerBindingV1
	correlation      string
}

type protectedRecoveryPurposeFixture struct {
	ctx context.Context

	source              *Manager
	sourceRegistry      *memoryRegistryStore
	sourceSecrets       *memorySecretStore
	sourceSecretRecords *protectedRecoveryRecordingSecretStore

	destination              *Manager
	destinationRegistry      *memoryRegistryStore
	destinationSecrets       *memorySecretStore
	destinationSecretRecords *protectedRecoveryRecordingSecretStore

	manifest          []byte
	imported          ProtectedRecoveryImportResult
	request           []byte
	bundle            []byte
	prepared          domainregistry.ProtectedRecoveryPreparedRequestV1
	cases             []protectedRecoveryPurposeCase
	action            domainregistry.ProtectedRecoveryLocalActionV1
	entries           []domainregistry.ProtectedRecoveryEntryV1
	owners            []domainregistry.ProtectedRecoveryOwnerBindingV1
	destinationOwners []domainregistry.ProtectedRecoveryOwnerBindingV1
}

func connectProtectedPurposeProvider(
	t *testing.T,
	ctx context.Context,
	manager *Manager,
	registry *memoryRegistryStore,
	id, endpoint, purpose, secret string,
	binding *domainregistry.OAuthBindingMetadata,
) domainregistry.Provider {
	t.Helper()
	credential, err := secretstoreport.SetCredential([]byte(secret))
	if err != nil {
		t.Fatalf("SetCredential(%s) error = %v", id, err)
	}
	state := registry.snapshot()
	provider, err := manager.Connect(ctx, ConnectCommand{
		Expected: domainregistry.ExpectedState{RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation},
		Provider: domainregistry.ProviderInput{
			ID: id, Kind: "openai-compatible", Endpoint: endpoint,
			Models: []string{"model-alpha"}, MediaModels: []string{}, SelectedModel: "model-alpha",
			SelectedRoutes: []string{"primary"}, OAuthBinding: binding,
		},
		CredentialPurpose: secretstoreport.Purpose(purpose), Credential: credential,
	})
	if err != nil {
		t.Fatalf("Connect(%s) error = %v", id, err)
	}
	return provider
}

func newProtectedRecoveryPurposeFixture(t *testing.T) *protectedRecoveryPurposeFixture {
	t.Helper()
	ctx := context.Background()
	sourceRegistry := newMemoryRegistryStore()
	sourceSecrets := newMemorySecretStore()
	sourceSecretRecords := &protectedRecoveryRecordingSecretStore{delegate: sourceSecrets}
	source := mustManager(t, sourceRegistry, sourceSecretRecords, nil)

	apiProvider := connectProtectedPurposeProvider(t, ctx, source, sourceRegistry,
		"provider-protected-api", "https://api-purpose.invalid/v1", "provider-api-key", "synthetic-api-key", nil)
	oauthBinding := &domainregistry.OAuthBindingMetadata{
		SchemaVersion: 1, Issuer: "https://oauth-purpose.invalid",
		AuthorizationEndpoint: "https://oauth-purpose.invalid/authorize",
		TokenEndpoint:         "https://oauth-purpose.invalid/token",
		ClientID:              "client/synthetic", Scopes: []string{"https://www.googleapis.com/auth/drive.readonly"},
		RedirectModeVersion: 1,
	}
	oauthProvider := connectProtectedPurposeProvider(t, ctx, source, sourceRegistry,
		"provider-protected-oauth", "https://oauth-provider.invalid/v1", "provider-oauth-token-bundle",
		`{"accessToken":"synthetic-oauth-access","subscription":{"plan":"synthetic"}}`, oauthBinding)

	mcpScope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "mcp", Provider: "mcp-purpose-server", AccountID: "mcp-purpose-account",
		ChannelID: "mcp-purpose-channel", Purpose: "mcp-oauth-access-token",
	}
	extScope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "extension", Provider: "extension-purpose-package", AccountID: "extension-purpose-account",
		ChannelID: "extension-purpose-channel", Purpose: "extension-provider-account-token",
	}
	putAccount := func(scope domainregistry.PrivateAccountScope, secret string) domainregistry.Provider {
		absent, err := source.AccountCredentialState(ctx, scope)
		if err != nil {
			t.Fatalf("AccountCredentialState(%s) error = %v", scope.Owner, err)
		}
		if _, err := source.PutAccountCredential(ctx, AccountCredentialPutCommand{
			Scope: scope, Expected: absent.Expected(), Credential: []byte(secret),
		}); err != nil {
			t.Fatalf("PutAccountCredential(%s) error = %v", scope.Owner, err)
		}
		providerID, err := AccountProviderID(scope)
		if err != nil {
			t.Fatalf("AccountProviderID(%s) error = %v", scope.Owner, err)
		}
		return sourceRegistry.snapshot().Providers[providerID]
	}
	mcpProvider := putAccount(mcpScope, "synthetic-mcp-oauth-token")
	extProvider := putAccount(extScope, "synthetic-extension-token")

	manifest, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest(multi-purpose) error = %v", err)
	}
	destinationRegistry := newMemoryRegistryStore()
	destinationSecrets := newMemorySecretStore()
	destinationSecretRecords := &protectedRecoveryRecordingSecretStore{delegate: destinationSecrets}
	destination := mustManager(t, destinationRegistry, destinationSecretRecords, nil)
	imported, err := destination.ImportPortableManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("ImportPortableManifest(multi-purpose) error = %v", err)
	}
	fixture := &protectedRecoveryPurposeFixture{
		ctx: ctx, source: source, sourceRegistry: sourceRegistry, sourceSecrets: sourceSecrets,
		sourceSecretRecords: sourceSecretRecords, destination: destination,
		destinationRegistry: destinationRegistry, destinationSecrets: destinationSecrets,
		destinationSecretRecords: destinationSecretRecords, manifest: manifest,
		cases: []protectedRecoveryPurposeCase{
			{name: "provider api key", purpose: "provider-api-key", sourceProviderID: apiProvider.ID, sourceProvider: apiProvider},
			{name: "provider oauth bundle", purpose: "provider-oauth-token-bundle", sourceProviderID: oauthProvider.ID, sourceProvider: oauthProvider},
			{name: "mcp account", purpose: "mcp-oauth-access-token", sourceProviderID: mcpProvider.ID, sourceProvider: mcpProvider, scope: &mcpScope},
			{name: "extension account", purpose: "extension-provider-account-token", sourceProviderID: extProvider.ID, sourceProvider: extProvider, scope: &extScope},
		},
		owners: []domainregistry.ProtectedRecoveryOwnerBindingV1{
			{Owner: "mcp", Provider: mcpScope.Provider, AccountID: mcpScope.AccountID, ChannelID: mcpScope.ChannelID, Purpose: mcpScope.Purpose, Fingerprint: "synthetic-mcp-current-binding"},
			{Owner: "extension", Provider: extScope.Provider, AccountID: extScope.AccountID, ChannelID: extScope.ChannelID, Purpose: extScope.Purpose, Fingerprint: "synthetic-extension-current-binding"},
		},
		destinationOwners: []domainregistry.ProtectedRecoveryOwnerBindingV1{
			{Owner: "mcp", Provider: mcpScope.Provider, AccountID: "mcp-destination-account", ChannelID: "mcp-destination-channel", Purpose: mcpScope.Purpose, Fingerprint: "synthetic-destination-mcp-current-binding"},
			{Owner: "extension", Provider: extScope.Provider, AccountID: "extension-destination-account", ChannelID: "extension-destination-channel", Purpose: extScope.Purpose, Fingerprint: "synthetic-destination-extension-current-binding"},
		},
	}
	manifestModel, err := domainregistry.ParsePortableManifestV1(manifest)
	if err != nil {
		t.Fatalf("ParsePortableManifestV1(multi-purpose) error = %v", err)
	}
	for index := range fixture.cases {
		caseValue := &fixture.cases[index]
		for _, descriptor := range manifestModel.Providers {
			if caseValue.scope == nil && descriptor.Endpoint == caseValue.sourceProvider.Endpoint {
				caseValue.correlation = descriptor.Correlation
			}
		}
		for _, descriptor := range manifestModel.Accounts {
			if caseValue.scope != nil && descriptor.Owner == caseValue.scope.Owner && descriptor.Provider == caseValue.scope.Provider && descriptor.Purpose == caseValue.scope.Purpose {
				caseValue.correlation = descriptor.Correlation
			}
		}
		if caseValue.correlation == "" {
			t.Fatalf("could not map %s to exported correlation", caseValue.name)
		}
		if caseValue.scope != nil {
			for ownerIndex := range fixture.destinationOwners {
				owner := &fixture.destinationOwners[ownerIndex]
				if owner.Owner == caseValue.scope.Owner && owner.Purpose == caseValue.scope.Purpose {
					owner.Correlation = caseValue.correlation
					caseValue.destinationOwner = *owner
				}
			}
			for ownerIndex := range fixture.owners {
				owner := &fixture.owners[ownerIndex]
				if owner.Owner == caseValue.scope.Owner && owner.Purpose == caseValue.scope.Purpose {
					owner.Correlation = caseValue.correlation
				}
			}
			if caseValue.destinationOwner.Fingerprint == "" {
				t.Fatalf("could not map %s to destination owner binding", caseValue.name)
			}
		}
	}
	fixture.imported = protectedRecoveryPrivateImportResult(
		t, destinationRegistry.snapshot(), manifest, imported, fixture.destinationOwners,
	)
	destinationSnapshot := destinationRegistry.snapshot()
	for _, entry := range imported.Entries {
		provider := destinationSnapshot.Providers[entry.DestinationProviderID]
		fixture.entries = append(fixture.entries, domainregistry.ProtectedRecoveryEntryV1{
			Correlation: entry.Correlation, DestinationProviderID: entry.DestinationProviderID,
			DestinationProviderRevision: provider.Revision, DestinationProviderGeneration: provider.Generation,
			DestinationProviderIncarnation: provider.Incarnation,
		})
	}
	manifestDigest := protectedRecoveryDigest(manifest)
	localBinding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
		ProfileBinding: syntheticRecoveryDestinationProfile, DataDirectoryBinding: syntheticRecoveryDestinationData,
	}
	action := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: localBinding,
	}
	fixture.action = action
	fixture.prepared, err = destination.PrepareProtectedRecoveryRequest(ctx, ProtectedRecoveryRequestCommand{
		ManifestJSON:          append([]byte(nil), manifest...),
		ImportResult:          fixture.imported,
		LocalBinding:          localBinding,
		OwnerBindingInventory: fixture.destinationOwners,
	})
	if err != nil {
		t.Fatalf("PrepareProtectedRecoveryRequest(multi-purpose) error = %v", err)
	}
	fixture.request = fixture.prepared.Request
	request, err := domainregistry.ParseProtectedRecoveryRequestV1(fixture.request)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryRequestV1(multi-purpose) error = %v", err)
	}
	if request.ManifestDigest != manifestDigest || fixture.prepared.ManifestDigest != manifestDigest {
		t.Fatalf("protected request manifest digest = %q, want %q", request.ManifestDigest, manifestDigest)
	}
	fixture.action = domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: localBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: fixture.prepared.RequestDigest, RequestFingerprint: fixture.prepared.RequestFingerprint,
			OperationID: fixture.prepared.OperationID, SessionNonce: fixture.prepared.SessionNonce,
			ManifestDigest: fixture.prepared.ManifestDigest, ItemSetDigest: fixture.prepared.ItemSetDigest,
			ExpiresAt: fixture.prepared.ExpiresAt, LocalBinding: localBinding,
		},
	}
	if err := destination.ConfirmProtectedRecoveryDestination(ctx, ProtectedRecoveryDestinationConfirmationCommand{
		Request: fixture.request, LocalAction: fixture.action, OwnerBindingInventory: fixture.destinationOwners,
	}); err != nil {
		t.Fatalf("ConfirmProtectedRecoveryDestination(multi-purpose) error = %v", err)
	}

	sourceAction := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: domainregistry.ProtectedRecoveryLocalBindingV1{
			BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
			ProfileBinding: syntheticRecoverySourceProfile, DataDirectoryBinding: syntheticRecoverySourceData,
		},
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: fixture.prepared.RequestDigest, RequestFingerprint: fixture.prepared.RequestFingerprint,
			OperationID: fixture.prepared.OperationID, SessionNonce: fixture.prepared.SessionNonce,
			ManifestDigest: fixture.prepared.ManifestDigest, ItemSetDigest: fixture.prepared.ItemSetDigest,
			ExpiresAt: fixture.prepared.ExpiresAt,
			LocalBinding: domainregistry.ProtectedRecoveryLocalBindingV1{
				BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
				ProfileBinding: syntheticRecoverySourceProfile, DataDirectoryBinding: syntheticRecoverySourceData,
			},
		},
	}
	fixture.bundle, err = source.CreateProtectedRecoveryBundle(ctx, ProtectedRecoveryBundleCommand{
		Request: fixture.request, LocalAction: sourceAction, OwnerBindingInventory: fixture.owners,
	})
	if err != nil {
		t.Fatalf("CreateProtectedRecoveryBundle(multi-purpose) error = %v", err)
	}
	return fixture

}

func TestProtectedRecoveryV1AllowedPurposesAreEndToEndAndReservedFailClosed(t *testing.T) {
	fixture := newProtectedRecoveryPurposeFixture(t)
	for _, caseValue := range fixture.cases {
		forbidden := []string{caseValue.sourceProviderID, caseValue.sourceProvider.CredentialRef}
		if caseValue.scope != nil {
			forbidden = append(forbidden, caseValue.scope.Provider, caseValue.scope.AccountID, caseValue.scope.ChannelID)
		}
		assertProtectedRecoveryExternalRedacted(t, "four-purpose bundle", fixture.bundle, forbidden...)
	}
	for _, marker := range []string{"synthetic-api-key", "synthetic-oauth-access", "synthetic-mcp-oauth-token", "synthetic-extension-token"} {
		if bytes.Contains(fixture.bundle, []byte(marker)) {
			t.Fatalf("four-purpose bundle contains plaintext marker %q", marker)
		}
	}
	sourceBefore := fixture.sourceRegistry.snapshot()
	before := fixture.destinationRegistry.snapshot()
	preparedBefore := fixture.destinationSecrets.preparedCount()
	receipt, err := fixture.destination.ApplyProtectedRecoveryBundle(
		fixture.ctx, ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: fixture.destinationOwners},
	)
	if err != nil {
		t.Fatalf("four-purpose protected recovery apply error = %v", err)
	}
	if _, err := domainregistry.ParseProtectedRecoveryReceiptV1(receipt); err != nil {
		t.Fatalf("four-purpose receipt parse error = %v", err)
	}
	after := fixture.destinationRegistry.snapshot()
	for _, caseValue := range fixture.cases {
		var importedID string
		for _, entry := range fixture.imported.Entries {
			if entry.Correlation == caseValue.correlation {
				importedID = entry.DestinationProviderID
				break
			}
		}
		if importedID == "" {
			t.Fatalf("missing destination mapping for %s", caseValue.name)
		}
		if importedID == caseValue.sourceProviderID {
			t.Fatalf("%s reused the source Provider ID %q", caseValue.name, importedID)
		}
		destinationProvider := after.Providers[importedID]
		if destinationProvider.CredentialRef == "" || destinationProvider.CredentialPurpose != caseValue.purpose {
			t.Fatalf("%s did not receive destination K1 purpose: %#v", caseValue.name, destinationProvider)
		}
		if caseValue.scope != nil {
			if destinationProvider.PrivateAccount == nil ||
				destinationProvider.PrivateAccount.Owner != caseValue.destinationOwner.Owner ||
				destinationProvider.PrivateAccount.Provider != caseValue.destinationOwner.Provider ||
				destinationProvider.PrivateAccount.AccountID != caseValue.destinationOwner.AccountID ||
				destinationProvider.PrivateAccount.ChannelID != caseValue.destinationOwner.ChannelID ||
				destinationProvider.PrivateAccount.Purpose != caseValue.destinationOwner.Purpose {
				t.Fatalf("%s did not bind the exact destination private-account scope: %#v", caseValue.name, destinationProvider.PrivateAccount)
			}
			if destinationProvider.PrivateAccount.AccountID == caseValue.scope.AccountID ||
				destinationProvider.PrivateAccount.ChannelID == caseValue.scope.ChannelID {
				t.Fatalf("%s reused source private-account identity: %#v", caseValue.name, destinationProvider.PrivateAccount)
			}
		}
	}
	for index, left := range after.Providers {
		if left.PrivateAccount == nil {
			continue
		}
		for otherID, right := range after.Providers {
			if index >= otherID || right.PrivateAccount == nil {
				continue
			}
			if reflect.DeepEqual(left.PrivateAccount, right.PrivateAccount) {
				t.Fatalf("destination contains duplicate private-account scope on %q and %q", index, otherID)
			}
		}
	}
	for _, caseValue := range fixture.cases {
		beforeProvider := sourceBefore.Providers[caseValue.sourceProviderID]
		afterProvider, exists := fixture.sourceRegistry.snapshot().Providers[caseValue.sourceProviderID]
		if !exists || afterProvider.ID != beforeProvider.ID || afterProvider.Revision != beforeProvider.Revision ||
			afterProvider.Generation != beforeProvider.Generation || afterProvider.Incarnation != beforeProvider.Incarnation ||
			afterProvider.CredentialRef != beforeProvider.CredentialRef || afterProvider.CredentialPurpose != beforeProvider.CredentialPurpose ||
			afterProvider.Tombstone != beforeProvider.Tombstone || !reflect.DeepEqual(afterProvider.PrivateAccount, beforeProvider.PrivateAccount) {
			t.Fatalf("%s changed source Provider/account authority", caseValue.name)
		}
	}
	if fixture.sourceRegistry.snapshot().SelectedProviderID != sourceBefore.SelectedProviderID {
		t.Fatal("four-purpose recovery changed source selection")
	}
	if after.SelectedProviderID != before.SelectedProviderID || preparedBefore >= fixture.destinationSecrets.preparedCount() {
		t.Fatal("four-purpose recovery changed selection or did not prepare destination credentials")
	}

	// Reserved purposes are malicious-record mutations of one otherwise valid
	// bundle. No source caller chooses a purpose and no failed preflight reaches
	// destination Registry/K1 mutation.
	for _, reserved := range []string{
		"hub-token", "oauth-authorization-state", "transport-credential", "provider-private-key", "trust-permission", "unknown-purpose",
	} {
		reservedFixture := newProtectedRecoveryPurposeFixture(t)
		prior := reservedFixture.destinationRegistry.snapshot()
		prepared := reservedFixture.destinationSecrets.preparedCount()
		bundle, err := domainregistry.ParseProtectedRecoveryBundleV1(reservedFixture.bundle)
		if err != nil {
			t.Fatalf("ParseProtectedRecoveryBundleV1(%s) error = %v", reserved, err)
		}
		bundle.Entries[0].Purpose = reserved
		mutated, marshalErr := domainregistry.MarshalProtectedRecoveryBundleV1(bundle)
		if marshalErr == nil {
			if _, applyErr := reservedFixture.destination.ApplyProtectedRecoveryBundle(
				reservedFixture.ctx, ProtectedRecoveryApplyCommand{Bundle: mutated, OwnerBindingInventory: reservedFixture.destinationOwners},
			); applyErr == nil {
				t.Fatalf("reserved purpose %q unexpectedly committed", reserved)
			}
		}
		if !reflect.DeepEqual(prior, reservedFixture.destinationRegistry.snapshot()) {
			t.Fatalf("reserved purpose %q changed destination Registry", reserved)
		}
		if reservedFixture.destinationSecrets.preparedCount() != prepared {
			t.Fatalf("reserved purpose %q touched destination Secret Store", reserved)
		}
	}
}

func TestProtectedRecoveryV1SameOwnerPurposeUsesExactCorrelations(t *testing.T) {
	ctx := context.Background()
	sourceRegistry := newMemoryRegistryStore()
	sourceSecrets := newMemorySecretStore()
	sourceSecretRecords := &protectedRecoveryRecordingSecretStore{delegate: sourceSecrets}
	source := mustManager(t, sourceRegistry, sourceSecretRecords, nil)

	// Two independent MCP owners intentionally share owner and purpose. Their
	// distinct manifest-local correlations and exact scopes must remain separate
	// throughout source lookup, destination mapping, and final replacement.
	scopes := []domainregistry.PrivateAccountScope{
		{SchemaVersion: 1, Owner: "mcp", Provider: "mcp-shared-server-a", AccountID: "mcp-shared-account-a", ChannelID: "mcp-shared-channel-a", Purpose: "mcp-oauth-access-token"},
		{SchemaVersion: 1, Owner: "mcp", Provider: "mcp-shared-server-b", AccountID: "mcp-shared-account-b", ChannelID: "mcp-shared-channel-b", Purpose: "mcp-oauth-access-token"},
	}
	sourceProviders := make(map[string]domainregistry.Provider, len(scopes))
	for index, scope := range scopes {
		absent, err := source.AccountCredentialState(ctx, scope)
		if err != nil {
			t.Fatalf("AccountCredentialState(%d) error = %v", index, err)
		}
		if _, err := source.PutAccountCredential(ctx, AccountCredentialPutCommand{
			Scope: scope, Expected: absent.Expected(), Credential: []byte("synthetic-mcp-shared-secret-" + string(rune('a'+index))),
		}); err != nil {
			t.Fatalf("PutAccountCredential(%d) error = %v", index, err)
		}
		providerID, err := AccountProviderID(scope)
		if err != nil {
			t.Fatalf("AccountProviderID(%d) error = %v", index, err)
		}
		sourceProviders[scope.Provider] = sourceRegistry.snapshot().Providers[providerID]
	}

	manifest, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest(shared MCP owners) error = %v", err)
	}
	manifestModel, err := domainregistry.ParsePortableManifestV1(manifest)
	if err != nil || len(manifestModel.Providers) != 0 || len(manifestModel.Accounts) != len(scopes) {
		t.Fatalf("shared MCP manifest = %#v, error = %v", manifestModel, err)
	}

	destinationRegistry := newMemoryRegistryStore()
	destinationSecrets := newMemorySecretStore()
	destinationSecretRecords := &protectedRecoveryRecordingSecretStore{delegate: destinationSecrets}
	destination := mustManager(t, destinationRegistry, destinationSecretRecords, nil)
	imported, err := destination.ImportPortableManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("ImportPortableManifest(shared MCP owners) error = %v", err)
	}

	sourceOwners := make([]domainregistry.ProtectedRecoveryOwnerBindingV1, 0, len(scopes))
	destinationOwners := make([]domainregistry.ProtectedRecoveryOwnerBindingV1, 0, len(scopes))
	for _, descriptor := range manifestModel.Accounts {
		provider, ok := sourceProviders[descriptor.Provider]
		if !ok || provider.PrivateAccount == nil {
			t.Fatalf("missing source owner for manifest descriptor %#v", descriptor)
		}
		scope := *provider.PrivateAccount
		sourceOwners = append(sourceOwners, domainregistry.ProtectedRecoveryOwnerBindingV1{
			Correlation: descriptor.Correlation, Owner: scope.Owner, Provider: scope.Provider,
			AccountID: scope.AccountID, ChannelID: scope.ChannelID, Purpose: scope.Purpose,
			Fingerprint: "synthetic-source-binding-" + scope.Provider,
		})
		destinationOwners = append(destinationOwners, domainregistry.ProtectedRecoveryOwnerBindingV1{
			Correlation: descriptor.Correlation, Owner: "mcp", Provider: scope.Provider,
			AccountID: "destination-" + scope.AccountID, ChannelID: "destination-" + scope.ChannelID,
			Purpose: "mcp-oauth-access-token", Fingerprint: "synthetic-destination-binding-" + scope.Provider,
		})
	}
	privateImported := protectedRecoveryPrivateImportResult(
		t, destinationRegistry.snapshot(), manifest, imported, destinationOwners,
	)
	localBinding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
		ProfileBinding: syntheticRecoveryDestinationProfile, DataDirectoryBinding: syntheticRecoveryDestinationData,
	}
	prepared, err := destination.PrepareProtectedRecoveryRequest(ctx, ProtectedRecoveryRequestCommand{
		ManifestJSON: manifest, ImportResult: privateImported, LocalBinding: localBinding,
		OwnerBindingInventory: destinationOwners,
	})
	if err != nil {
		t.Fatalf("PrepareProtectedRecoveryRequest(shared MCP owners) error = %v", err)
	}
	confirmation := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: localBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: prepared.RequestDigest, RequestFingerprint: prepared.RequestFingerprint,
			OperationID: prepared.OperationID, SessionNonce: prepared.SessionNonce,
			ManifestDigest: prepared.ManifestDigest, ItemSetDigest: prepared.ItemSetDigest,
			ExpiresAt: prepared.ExpiresAt, LocalBinding: localBinding,
		},
	}
	if err := destination.ConfirmProtectedRecoveryDestination(ctx, ProtectedRecoveryDestinationConfirmationCommand{
		Request: prepared.Request, LocalAction: confirmation, OwnerBindingInventory: destinationOwners,
	}); err != nil {
		t.Fatalf("ConfirmProtectedRecoveryDestination(shared MCP owners) error = %v", err)
	}

	sourceBinding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
		ProfileBinding: syntheticRecoverySourceProfile, DataDirectoryBinding: syntheticRecoverySourceData,
	}
	sourceAction := confirmation
	sourceAction.LocalBinding = sourceBinding
	sourceAction.Confirmation.LocalBinding = sourceBinding
	bundle, err := source.CreateProtectedRecoveryBundle(ctx, ProtectedRecoveryBundleCommand{
		Request: prepared.Request, LocalAction: sourceAction, OwnerBindingInventory: sourceOwners,
	})
	if err != nil {
		t.Fatalf("CreateProtectedRecoveryBundle(shared MCP owners) error = %v", err)
	}
	receipt, err := destination.ApplyProtectedRecoveryBundle(ctx, ProtectedRecoveryApplyCommand{
		Bundle: bundle, OwnerBindingInventory: destinationOwners,
	})
	if err != nil {
		t.Fatalf("ApplyProtectedRecoveryBundle(shared MCP owners) error = %v", err)
	}
	if _, err := source.FinalizeProtectedRecoveryReceipt(ctx, ProtectedRecoveryFinalizeCommand{Receipt: receipt}); err != nil {
		t.Fatalf("FinalizeProtectedRecoveryReceipt(shared MCP owners) error = %v", err)
	}

	after := destinationRegistry.snapshot()
	seenIDs := make(map[string]struct{}, len(imported.Entries))
	for _, entry := range imported.Entries {
		provider := after.Providers[entry.DestinationProviderID]
		if _, duplicate := seenIDs[provider.ID]; duplicate {
			t.Fatalf("shared MCP owners reused destination Provider ID %q", provider.ID)
		}
		seenIDs[provider.ID] = struct{}{}
		var owner domainregistry.ProtectedRecoveryOwnerBindingV1
		for _, candidate := range destinationOwners {
			if candidate.Correlation == entry.Correlation {
				owner = candidate
				break
			}
		}
		if owner.Correlation == "" || provider.CredentialPurpose != owner.Purpose || provider.PrivateAccount == nil ||
			provider.PrivateAccount.Owner != owner.Owner || provider.PrivateAccount.Provider != owner.Provider ||
			provider.PrivateAccount.AccountID != owner.AccountID || provider.PrivateAccount.ChannelID != owner.ChannelID ||
			provider.PrivateAccount.Purpose != owner.Purpose {
			t.Fatalf("destination owner mapping for %q = %#v, want %#v", entry.Correlation, provider.PrivateAccount, owner)
		}
		for _, sourceProvider := range sourceProviders {
			if provider.ID == sourceProvider.ID ||
				provider.PrivateAccount.AccountID == sourceProvider.PrivateAccount.AccountID || provider.PrivateAccount.ChannelID == sourceProvider.PrivateAccount.ChannelID {
				t.Fatalf("destination mapping reused source owner identity: %#v", provider.PrivateAccount)
			}
		}
	}
	if len(seenIDs) != len(scopes) {
		t.Fatalf("shared MCP destination mappings = %d, want %d", len(seenIDs), len(scopes))
	}
}

func TestProtectedRecoveryV1DestinationOwnerBindingFailsBeforeBatch(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func([]domainregistry.ProtectedRecoveryOwnerBindingV1) []domainregistry.ProtectedRecoveryOwnerBindingV1
	}{
		{
			name: "missing destination owner",
			mutate: func(owners []domainregistry.ProtectedRecoveryOwnerBindingV1) []domainregistry.ProtectedRecoveryOwnerBindingV1 {
				return owners[:1]
			},
		},
		{
			name: "drifted destination fingerprint",
			mutate: func(owners []domainregistry.ProtectedRecoveryOwnerBindingV1) []domainregistry.ProtectedRecoveryOwnerBindingV1 {
				owners[0].Fingerprint = "synthetic-destination-mcp-drifted-binding"
				return owners
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newProtectedRecoveryPurposeFixture(t)
			prior := fixture.destinationRegistry.snapshot()
			preparedBefore := fixture.destinationSecrets.preparedCount()
			accessBefore := fixture.destinationSecretRecords.accessCount()
			owners := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), fixture.destinationOwners...)
			owners = testCase.mutate(owners)
			if _, err := fixture.destination.ApplyProtectedRecoveryBundle(
				fixture.ctx,
				ProtectedRecoveryApplyCommand{Bundle: fixture.bundle, OwnerBindingInventory: owners},
			); err == nil {
				t.Fatal("ApplyProtectedRecoveryBundle accepted missing or drifted destination owner binding")
			}
			if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
				t.Fatal("destination owner binding failure changed Registry")
			}
			if fixture.destinationSecrets.preparedCount() != preparedBefore ||
				fixture.destinationSecretRecords.accessCount() != accessBefore {
				t.Fatal("destination owner binding failure touched Secret Store")
			}
		})
	}
}

func TestProtectedRecoveryV1PrepareRejectsUnadmittedDestinationPrivateAccountBindingBeforePending(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*domainregistry.ProtectedRecoveryOwnerBindingV1)
	}{
		{
			name: "wrong provider",
			mutate: func(owner *domainregistry.ProtectedRecoveryOwnerBindingV1) {
				owner.Provider = "synthetic-unadmitted-provider"
			},
		},
		{
			name: "wrong accountId",
			mutate: func(owner *domainregistry.ProtectedRecoveryOwnerBindingV1) {
				owner.AccountID = "synthetic-unadmitted-account"
			},
		},
		{
			name: "wrong channelId",
			mutate: func(owner *domainregistry.ProtectedRecoveryOwnerBindingV1) {
				owner.ChannelID = "synthetic-unadmitted-channel"
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newProtectedRecoveryPurposeFixture(t)
			if _, err := fixture.destination.RollbackProtectedRecovery(
				fixture.ctx, ProtectedRecoveryRollbackCommand{Request: fixture.request},
			); err != nil {
				t.Fatalf("RollbackProtectedRecovery(destination binding setup) error = %v", err)
			}
			currentInventory := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), fixture.destinationOwners...)
			testCase.mutate(&currentInventory[0])
			prior := fixture.destinationRegistry.snapshot()
			preparedBefore := fixture.destinationSecrets.preparedCount()
			accessBefore := fixture.destinationSecretRecords.accessCount()

			_, err := fixture.destination.PrepareProtectedRecoveryRequest(
				fixture.ctx,
				ProtectedRecoveryRequestCommand{
					ManifestJSON:          append([]byte(nil), fixture.manifest...),
					ImportResult:          fixture.imported,
					LocalBinding:          fixture.action.LocalBinding,
					OwnerBindingInventory: currentInventory,
				},
			)
			if err == nil {
				t.Errorf("PrepareProtectedRecoveryRequest(%s) error = nil, want fail closed", testCase.name)
			}
			if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
				t.Errorf("PrepareProtectedRecoveryRequest(%s) K1 PreparePut delta = %d, want 0", testCase.name, delta)
			}
			if delta := fixture.destinationSecretRecords.accessCount() - accessBefore; delta != 0 {
				t.Errorf("PrepareProtectedRecoveryRequest(%s) K1 Get delta = %d, want 0", testCase.name, delta)
			}
			after := fixture.destinationRegistry.snapshot()
			if !reflect.DeepEqual(prior, after) {
				t.Errorf("PrepareProtectedRecoveryRequest(%s) Registry mutation reached = true, want false", testCase.name)
			}
			if !reflect.DeepEqual(prior.Providers, after.Providers) {
				t.Errorf("PrepareProtectedRecoveryRequest(%s) account mutation reached = true, want false", testCase.name)
			}
		})
	}
}

func TestProtectedRecoveryV1DestinationContinuationsRejectDurableBindingDriftBeforeEffects(t *testing.T) {
	for _, operation := range []string{"confirm", "restart-recover", "apply"} {
		for _, drift := range []string{"provider", "accountId", "channelId", "purpose", "fingerprint", "correlation"} {
			t.Run(operation+"/"+drift, func(t *testing.T) {
				fixture := newProtectedRecoveryPurposeFixture(t)
				manager := fixture.destination
				if operation == "confirm" {
					manager = mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
					if _, err := manager.RecoverProtectedRecovery(fixture.ctx); err != nil {
						t.Fatalf("RecoverProtectedRecovery(confirm setup) error = %v", err)
					}
				}
				if operation == "restart-recover" {
					manager = mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
				}
				currentInventory := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), fixture.destinationOwners...)
				switch drift {
				case "provider":
					currentInventory[0].Provider = "synthetic-current-drifted-provider"
				case "accountId":
					currentInventory[0].AccountID = "synthetic-current-drifted-account"
				case "channelId":
					currentInventory[0].ChannelID = "synthetic-current-drifted-channel"
				case "purpose":
					currentInventory[0].Purpose = "extension-provider-account-token"
				case "fingerprint":
					currentInventory[0].Fingerprint = "synthetic-current-drifted-fingerprint"
				case "correlation":
					currentInventory[0].Correlation = "account-99"
				}
				prior := fixture.destinationRegistry.snapshot()
				preparedBefore := fixture.destinationSecrets.preparedCount()
				accessBefore := fixture.destinationSecretRecords.accessCount()
				sourceBefore := fixture.sourceRegistry.snapshot()
				sourceAccessBefore := fixture.sourceSecretRecords.accessCount()

				var err error
				switch operation {
				case "confirm":
					err = manager.ConfirmProtectedRecoveryDestination(fixture.ctx, ProtectedRecoveryDestinationConfirmationCommand{
						Request: fixture.request, LocalAction: fixture.action, OwnerBindingInventory: currentInventory,
					})
				case "restart-recover":
					_, err = manager.RecoverProtectedRecovery(fixture.ctx, ProtectedRecoveryRecoverCommand{
						Request: fixture.request, LocalAction: fixture.action, OwnerBindingInventory: currentInventory,
					})
				case "apply":
					_, err = manager.ApplyProtectedRecoveryBundle(fixture.ctx, ProtectedRecoveryApplyCommand{
						Request: fixture.request, Bundle: fixture.bundle, OwnerBindingInventory: currentInventory,
					})
				}
				if err == nil {
					t.Fatalf("%s %s binding drift error = nil, want fail closed", operation, drift)
				}
				if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
					t.Fatalf("%s %s binding drift K1 PreparePut delta = %d, want 0", operation, drift, delta)
				}
				if delta := fixture.destinationSecretRecords.accessCount() - accessBefore; delta != 0 {
					t.Fatalf("%s %s binding drift K1 Get delta = %d, want 0", operation, drift, delta)
				}
				if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
					t.Fatalf("%s %s binding drift Registry/account mutation reached = true, want false", operation, drift)
				}
				if !reflect.DeepEqual(sourceBefore, fixture.sourceRegistry.snapshot()) ||
					fixture.sourceSecretRecords.accessCount() != sourceAccessBefore {
					t.Fatalf("%s %s binding drift source effect reached = true, want false", operation, drift)
				}
			})
		}
	}
}

func TestProtectedRecoveryV1RestartContinuationUsesDurableCorrelationForExactCorrelationlessInventory(t *testing.T) {
	t.Run("correlation omitted with unrelated earlier binding", func(t *testing.T) {
		fixture := newProtectedRecoveryPurposeFixture(t)
		manager := mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
		currentInventory := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), fixture.destinationOwners...)
		for index := range currentInventory {
			currentInventory[index].Correlation = ""
		}
		currentInventory = append(currentInventory, domainregistry.ProtectedRecoveryOwnerBindingV1{
			Owner: "extension", Provider: "aaa-unrelated-provider",
			AccountID: "unrelated-account", ChannelID: "unrelated-channel",
			Purpose: "extension-provider-account-token", Fingerprint: "unrelated-binding",
		})

		result, err := manager.RecoverProtectedRecovery(fixture.ctx, ProtectedRecoveryRecoverCommand{
			Request: fixture.request, LocalAction: fixture.action, OwnerBindingInventory: currentInventory,
		})
		if err != nil || result.Status != "reconfirmation_required" || result.Confirmed {
			t.Fatalf("correlationless restart continuation = status=%q confirmed=%t error=%v, want reconfirmation_required/false/nil",
				result.Status, result.Confirmed, err)
		}
	})

	t.Run("wrong explicit correlation fails before effects", func(t *testing.T) {
		fixture := newProtectedRecoveryPurposeFixture(t)
		manager := mustManager(t, fixture.destinationRegistry, fixture.destinationSecretRecords, nil)
		currentInventory := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), fixture.destinationOwners...)
		currentInventory[0].Correlation = "account-99"
		for index := 1; index < len(currentInventory); index++ {
			currentInventory[index].Correlation = ""
		}
		currentInventory = append(currentInventory, domainregistry.ProtectedRecoveryOwnerBindingV1{
			Owner: "extension", Provider: "aaa-unrelated-provider",
			AccountID: "unrelated-account", ChannelID: "unrelated-channel",
			Purpose: "extension-provider-account-token", Fingerprint: "unrelated-binding",
		})
		prior := fixture.destinationRegistry.snapshot()
		preparedBefore := fixture.destinationSecrets.preparedCount()
		accessBefore := fixture.destinationSecretRecords.accessCount()
		sourceBefore := fixture.sourceRegistry.snapshot()
		sourceAccessBefore := fixture.sourceSecretRecords.accessCount()

		_, err := manager.RecoverProtectedRecovery(fixture.ctx, ProtectedRecoveryRecoverCommand{
			Request: fixture.request, LocalAction: fixture.action, OwnerBindingInventory: currentInventory,
		})
		if err == nil {
			t.Fatal("wrong explicit restart correlation error = nil, want fail closed")
		}
		if delta := fixture.destinationSecrets.preparedCount() - preparedBefore; delta != 0 {
			t.Fatalf("wrong explicit restart correlation K1 PreparePut delta = %d, want 0", delta)
		}
		if delta := fixture.destinationSecretRecords.accessCount() - accessBefore; delta != 0 {
			t.Fatalf("wrong explicit restart correlation K1 Get delta = %d, want 0", delta)
		}
		if !reflect.DeepEqual(prior, fixture.destinationRegistry.snapshot()) {
			t.Fatal("wrong explicit restart correlation Registry/account mutation reached = true, want false")
		}
		if !reflect.DeepEqual(sourceBefore, fixture.sourceRegistry.snapshot()) ||
			fixture.sourceSecretRecords.accessCount() != sourceAccessBefore {
			t.Fatal("wrong explicit restart correlation source effect reached = true, want false")
		}
	})
}

func TestProtectedRecoveryV1RestartRequiresExactDurableCorrelationInCurrentInventory(t *testing.T) {
	expected := []domainregistry.ProtectedRecoveryOwnerBindingV1{
		{
			Correlation: "account-0", Owner: "mcp", Provider: "current-server",
			AccountID: "current-account", ChannelID: "current-channel",
			Purpose: "mcp-oauth-access-token", Fingerprint: "current-binding",
		},
	}
	actual := []domainregistry.ProtectedRecoveryOwnerBindingV1{
		{
			Correlation: "account-0", Owner: "mcp", Provider: "current-server",
			AccountID: "current-account", ChannelID: "current-channel",
			Purpose: "mcp-oauth-access-token", Fingerprint: "current-binding",
		},
		{
			Correlation: "account-1", Owner: "extension", Provider: "unrelated-extension",
			AccountID: "unrelated-account", ChannelID: "unrelated-channel",
			Purpose: "extension-provider-account-token", Fingerprint: "unrelated-binding",
		},
	}
	normalized, ok := protectedOwnerBindingsForSession(expected, actual)
	if !ok || len(normalized) != 1 || normalized[0] != expected[0] {
		t.Fatalf("durable exact owner binding = %#v, ok=%v, want %#v/true", normalized, ok, expected)
	}

	correlationless := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), actual...)
	for index := range correlationless {
		correlationless[index].Correlation = ""
	}
	normalized, ok = protectedOwnerBindingsForSession(expected, correlationless)
	if !ok || len(normalized) != 1 || normalized[0] != expected[0] {
		t.Fatalf("correlationless current binding normalized = %#v, ok=%v, want durable %#v/true", normalized, ok, expected)
	}

	drifted := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), actual...)
	drifted[0].Correlation = "account-9"
	if _, ok := protectedOwnerBindingsForSession(expected, drifted); ok {
		t.Fatal("owner correlation drift unexpectedly passed restart currentness")
	}

	drifted[0] = actual[0]
	drifted[0].Fingerprint = "drifted-binding"
	if _, ok := protectedOwnerBindingsForSession(expected, drifted); ok {
		t.Fatal("owner fingerprint drift unexpectedly passed restart currentness")
	}

	ambiguous := append([]domainregistry.ProtectedRecoveryOwnerBindingV1(nil), actual...)
	ambiguous = append(ambiguous, actual[0])
	if _, ok := protectedOwnerBindingsForSession(expected, ambiguous); ok {
		t.Fatal("ambiguous exact owner binding unexpectedly passed restart currentness")
	}
}
