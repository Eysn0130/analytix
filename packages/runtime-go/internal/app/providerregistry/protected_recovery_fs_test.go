package providerregistry

import (
	"context"
	"reflect"
	"testing"

	providerregistryfs "analytix.local/runtime-go/internal/adapters/outbound/providerregistryfs"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

// This is an isolated production-filesystem composition check. Secrets remain
// in a test-local Secret Store; the filesystem Store is the production
// journal/readback boundary being restarted.
func TestProtectedRecoveryFilesystemRestartReadback(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sourceDir := t.TempDir()
	destinationDir := t.TempDir()

	sourceStore, err := providerregistryfs.New(sourceDir)
	if err != nil {
		t.Fatalf("New(source) error = %v", err)
	}
	defer sourceStore.Close()
	sourceSecrets := newMemorySecretStore()
	source := mustManager(t, sourceStore, sourceSecrets, nil)
	var sourceInitial domainregistry.Registry
	if err := sourceStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		var loadErr error
		sourceInitial, loadErr = transaction.Load(ctx)
		return loadErr
	}); err != nil {
		t.Fatalf("Load(source) error = %v", err)
	}
	credential, err := secretstoreport.SetCredential([]byte("synthetic-protected-filesystem-secret"))
	if err != nil {
		t.Fatalf("SetCredential(source) error = %v", err)
	}
	sourceProvider, err := source.Connect(ctx, ConnectCommand{
		Expected: domainregistry.ExpectedState{RegistryRevision: sourceInitial.Revision, RegistryIncarnation: sourceInitial.Incarnation},
		Provider: domainregistry.ProviderInput{
			ID: "provider-protected-filesystem-source", Kind: "openai-compatible", Endpoint: "https://protected-filesystem-source.example/v1",
			Models: []string{"model-filesystem"}, MediaModels: []string{}, SelectedModel: "model-filesystem", SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if err != nil {
		t.Fatalf("Connect(source) error = %v", err)
	}
	sourceAfterConnect, err := source.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(source after connect) error = %v", err)
	}
	sourceSelectionBefore := sourceAfterConnect.SelectedProviderID
	manifest, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest(source) error = %v", err)
	}

	destinationStore, err := providerregistryfs.New(destinationDir)
	if err != nil {
		t.Fatalf("New(destination) error = %v", err)
	}
	destinationSecrets := newMemorySecretStore()
	destination := mustManager(t, destinationStore, destinationSecrets, nil)
	imported, err := destination.ImportPortableManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("ImportPortableManifest(destination) error = %v", err)
	}
	if imported.ProviderCount != 1 || imported.AccountCount != 0 || len(imported.Entries) != 1 {
		t.Fatalf("portable import result = %#v", imported)
	}
	if imported.Entries[0].DestinationProviderID == sourceProvider.ID {
		t.Fatal("filesystem protected recovery reused source Provider ID")
	}
	destinationSnapshot, err := destination.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(destination import) error = %v", err)
	}
	privateImported := protectedRecoveryPrivateImportResult(t, destinationSnapshot, manifest, imported, nil)

	destinationBinding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
		ProfileBinding: syntheticRecoveryDestinationProfile, DataDirectoryBinding: syntheticRecoveryDestinationData,
	}
	prepared, err := destination.PrepareProtectedRecoveryRequest(ctx, ProtectedRecoveryRequestCommand{
		ManifestJSON: manifest, ImportResult: privateImported, LocalBinding: destinationBinding,
		OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
	})
	if err != nil {
		t.Fatalf("PrepareProtectedRecoveryRequest(destination) error = %v", err)
	}
	confirmation := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: destinationBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: prepared.RequestDigest, RequestFingerprint: prepared.RequestFingerprint,
			OperationID: prepared.OperationID, SessionNonce: prepared.SessionNonce,
			ManifestDigest: prepared.ManifestDigest, ItemSetDigest: prepared.ItemSetDigest,
			ExpiresAt: prepared.ExpiresAt, LocalBinding: destinationBinding,
		},
	}
	if err := destination.ConfirmProtectedRecoveryDestination(ctx, ProtectedRecoveryDestinationConfirmationCommand{
		Request: prepared.Request, LocalAction: confirmation,
		OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
	}); err != nil {
		t.Fatalf("ConfirmProtectedRecoveryDestination(destination) error = %v", err)
	}
	request, err := domainregistry.ParseProtectedRecoveryRequestV1(prepared.Request)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryRequestV1() error = %v", err)
	}
	sourceBinding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: syntheticRecoveryWindow, MainFrameID: syntheticRecoveryFrame,
		ProfileBinding: syntheticRecoverySourceProfile, DataDirectoryBinding: syntheticRecoverySourceData,
	}
	sourceAction := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: sourceBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: prepared.RequestDigest, RequestFingerprint: prepared.RequestFingerprint,
			OperationID: prepared.OperationID, SessionNonce: prepared.SessionNonce,
			ManifestDigest: prepared.ManifestDigest, ItemSetDigest: prepared.ItemSetDigest,
			ExpiresAt: prepared.ExpiresAt, LocalBinding: sourceBinding,
		},
	}
	bundle, err := source.CreateProtectedRecoveryBundle(ctx, ProtectedRecoveryBundleCommand{
		Request: prepared.Request, LocalAction: sourceAction,
		OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
	})
	if err != nil {
		t.Fatalf("CreateProtectedRecoveryBundle(source) error = %v", err)
	}
	if _, err := domainregistry.ParseProtectedRecoveryBundleV1(bundle); err != nil {
		t.Fatalf("ParseProtectedRecoveryBundleV1() error = %v", err)
	}

	if err := destinationStore.Close(); err != nil {
		t.Fatalf("Close(destination) error = %v", err)
	}
	restartedStore, err := providerregistryfs.New(destinationDir)
	if err != nil {
		t.Fatalf("New(restarted destination) error = %v", err)
	}
	defer restartedStore.Close()
	restarted := mustManager(t, restartedStore, destinationSecrets, nil)
	pending, err := restarted.RecoverProtectedRecovery(ctx)
	if err != nil || pending.Status != "reconfirmation_required" || pending.Confirmed {
		t.Fatalf("RecoverProtectedRecovery(restarted destination) = %#v, %v; want reconfirmation_required", pending, err)
	}
	if err := restarted.ConfirmProtectedRecoveryDestination(ctx, ProtectedRecoveryDestinationConfirmationCommand{
		Request: prepared.Request,
		LocalAction: domainregistry.ProtectedRecoveryLocalActionV1{
			LocalBinding: destinationBinding,
			Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
				RequestDigest: prepared.RequestDigest, RequestFingerprint: prepared.RequestFingerprint,
				OperationID: prepared.OperationID, SessionNonce: prepared.SessionNonce,
				ManifestDigest: prepared.ManifestDigest, ItemSetDigest: prepared.ItemSetDigest,
				ExpiresAt: prepared.ExpiresAt, LocalBinding: destinationBinding,
			},
		},
		OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
	}); err != nil {
		t.Fatalf("ConfirmProtectedRecoveryDestination(restarted destination) error = %v", err)
	}
	receipt, err := restarted.ApplyProtectedRecoveryBundle(ctx, ProtectedRecoveryApplyCommand{
		Bundle: bundle, OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
	})
	if err != nil {
		t.Fatalf("ApplyProtectedRecoveryBundle(restarted destination) error = %v", err)
	}
	parsedReceipt, err := domainregistry.ParseProtectedRecoveryReceiptV1(receipt)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryReceiptV1() error = %v", err)
	}
	if parsedReceipt.RequestDigest != request.RequestDigest || parsedReceipt.BundleDigest != domainregistry.ProtectedRecoveryBundleDigest(bundle) {
		t.Fatalf("protected recovery receipt linkage = %#v", parsedReceipt)
	}
	destinationState, err := restarted.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(restarted destination) error = %v", err)
	}
	if destinationState.SelectedProviderID != "" || len(destinationState.Providers) != 1 {
		t.Fatalf("restarted destination authority = %#v", destinationState)
	}
	destinationProvider := destinationState.Providers[imported.Entries[0].DestinationProviderID]
	if destinationProvider.ID == "" || destinationProvider.CredentialRef == "" || destinationProvider.CredentialPurpose != "provider-api-key" {
		t.Fatalf("restarted destination winner = %#v", destinationProvider)
	}
	if !destinationSecrets.hasActive(destinationProvider.CredentialRef) {
		t.Fatal("restarted destination winner is not backed by an active K1 record")
	}

	if _, err := source.FinalizeProtectedRecoveryReceipt(ctx, ProtectedRecoveryFinalizeCommand{Receipt: receipt}); err != nil {
		t.Fatalf("FinalizeProtectedRecoveryReceipt(source) error = %v", err)
	}
	sourceState, err := source.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(source) error = %v", err)
	}
	if sourceState.SelectedProviderID != sourceSelectionBefore {
		t.Fatalf("source selection changed: before=%q after=%q", sourceSelectionBefore, sourceState.SelectedProviderID)
	}
	if !reflect.DeepEqual(sourceState.Providers[sourceProvider.ID].PrivateAccount, sourceProvider.PrivateAccount) ||
		sourceState.Providers[sourceProvider.ID].CredentialRef != sourceProvider.CredentialRef ||
		sourceState.Providers[sourceProvider.ID].CredentialPurpose != sourceProvider.CredentialPurpose ||
		!sourceSecrets.hasActive(sourceProvider.CredentialRef) {
		t.Fatal("source authority or credential changed after protected recovery")
	}
}
