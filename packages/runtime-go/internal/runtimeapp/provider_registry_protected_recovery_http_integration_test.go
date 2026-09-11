package runtimeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

type protectedRecoveryR7ImportResponse struct {
	SchemaVersion   int                                             `json:"schemaVersion"`
	ProviderCount   int                                             `json:"providerCount"`
	AccountCount    int                                             `json:"accountCount"`
	ReentryRequired int                                             `json:"reentryRequired"`
	Entries         []providerregistryapp.PortableImportEntryResult `json:"entries"`
}

type protectedRecoveryR7PrepareResponse struct {
	SchemaVersion      int    `json:"schemaVersion"`
	RequestBase64      string `json:"requestBase64"`
	RequestDigest      string `json:"requestDigest"`
	RequestFingerprint string `json:"requestFingerprint"`
	ManifestDigest     string `json:"manifestDigest"`
	ItemSetDigest      string `json:"itemSetDigest"`
	OperationID        string `json:"operationId"`
	SessionNonce       string `json:"sessionNonce"`
	ExpiresAt          string `json:"expiresAt"`
}

type protectedRecoveryR7ArtifactResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	BundleBase64  string `json:"bundleBase64,omitempty"`
	ReceiptBase64 string `json:"receiptBase64,omitempty"`
}

type protectedRecoveryR7StatusResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Confirmed     bool   `json:"confirmed"`
}

type protectedRecoveryR7PrepareRequest struct {
	SchemaVersion         int                                               `json:"schemaVersion"`
	ManifestBase64        string                                            `json:"manifestBase64"`
	ImportResult          providerregistryapp.ProtectedRecoveryImportResult `json:"importResult"`
	LocalBinding          domainregistry.ProtectedRecoveryLocalBindingV1    `json:"localBinding"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1  `json:"ownerBindingInventory"`
}

type protectedRecoveryR7ConfirmRequest struct {
	SchemaVersion         int                                              `json:"schemaVersion"`
	RequestBase64         string                                           `json:"requestBase64"`
	LocalAction           domainregistry.ProtectedRecoveryLocalActionV1    `json:"localAction"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"ownerBindingInventory"`
}

type protectedRecoveryR7CreateBundleRequest struct {
	SchemaVersion         int                                              `json:"schemaVersion"`
	RequestBase64         string                                           `json:"requestBase64"`
	LocalAction           domainregistry.ProtectedRecoveryLocalActionV1    `json:"localAction"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"ownerBindingInventory"`
}

type protectedRecoveryR7ApplyRequest struct {
	SchemaVersion         int                                              `json:"schemaVersion"`
	BundleBase64          string                                           `json:"bundleBase64"`
	RequestBase64         string                                           `json:"requestBase64"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"ownerBindingInventory"`
}

type protectedRecoveryR7ReceiptRequest struct {
	SchemaVersion int    `json:"schemaVersion"`
	ReceiptBase64 string `json:"receiptBase64"`
}

func protectedRecoveryR7HTTPCall(t *testing.T, handler httpapi.ProviderRegistryHandlers, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Marshal(%s request) error = %v", path, err)
	}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded)))
	return recorder
}

func protectedRecoveryR7DecodeBase64(t *testing.T, encoded string, maximum int) []byte {
	t.Helper()
	if encoded == "" || len(encoded)%4 != 0 {
		t.Fatalf("noncanonical protected recovery base64 length = %d", len(encoded))
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) == 0 || len(decoded) > maximum || base64.StdEncoding.EncodeToString(decoded) != encoded {
		t.Fatalf("noncanonical protected recovery base64: %v", err)
	}
	return decoded
}

func protectedRecoveryR7Encoded(value []byte) string {
	return base64.StdEncoding.EncodeToString(value)
}

func protectedRecoveryR7AssertRedacted(t *testing.T, body []byte, forbidden ...string) {
	t.Helper()
	for _, marker := range forbidden {
		if bytes.Contains(body, []byte(marker)) {
			t.Fatalf("protected recovery HTTP response leaked %q: %s", marker, body)
		}
	}
}

func TestProviderRegistryProtectedRecoveryProductionHTTPCompositionAuthenticatesReceiptAndCleansPendingKey(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the synthetic protected-recovery Secret Store integration")
	}

	ctx := context.Background()
	sourceDir := t.TempDir()
	destinationDir := t.TempDir()
	installSyntheticProviderRegistryFallbackMasterKey(t, sourceDir)
	installSyntheticProviderRegistryFallbackMasterKey(t, destinationDir)

	source, err := openProviderRegistryAuthorityV1(ctx, sourceDir)
	if err != nil {
		t.Fatalf("open source authority error = %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	destination, err := openProviderRegistryAuthorityV1(ctx, destinationDir)
	if err != nil {
		t.Fatalf("open destination authority error = %v", err)
	}
	t.Cleanup(func() {
		if destination != nil {
			_ = destination.Close()
		}
	})

	const sourceProviderID = "provider-protected-r7-source"
	secretMarker := []byte("synthetic-r7-protected-http-secret")
	sourceInitial, err := source.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("source initial Snapshot() error = %v", err)
	}
	credential, err := secretstoreport.SetCredential(secretMarker)
	if err != nil {
		t.Fatalf("SetCredential(source) error = %v", err)
	}
	sourceProvider, err := source.Manager().Connect(ctx, providerregistryapp.ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: sourceInitial.Revision, RegistryIncarnation: sourceInitial.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: sourceProviderID, Kind: "openai-compatible", Endpoint: "https://protected-r7-source.invalid/v1",
			Models: []string{"model-r7"}, MediaModels: []string{}, SelectedModel: "model-r7", SelectedRoutes: []string{},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if err != nil {
		t.Fatalf("source Connect() error = %v", err)
	}
	sourceBefore, err := source.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("source Snapshot(after connect) error = %v", err)
	}
	sourceProviderBefore := sourceBefore.Providers[sourceProvider.ID]
	sourceSelectionBefore := sourceBefore.SelectedProviderID

	sourceHandler := httpapi.ProviderRegistryHandlers{Service: source.Service()}
	exported := httptest.NewRecorder()
	sourceHandler.Handle(exported, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPortableManifestPathV1, nil))
	if exported.Code != http.StatusOK {
		t.Fatalf("source portable export = (%d, %s)", exported.Code, exported.Body.String())
	}
	protectedRecoveryR7AssertRedacted(t, exported.Body.Bytes(), string(secretMarker), "credentialRef", sourceProvider.ID)
	var exportResponse portableManifestHTTPExportResponseV1
	if err := json.Unmarshal(exported.Body.Bytes(), &exportResponse); err != nil || exportResponse.SchemaVersion != 1 {
		t.Fatalf("portable export response = %s, error = %v", exported.Body.String(), err)
	}
	manifest := []byte(exportResponse.ManifestJSON)
	if _, err := domainregistry.ParsePortableManifestV1(manifest); err != nil {
		t.Fatalf("exported manifest parse error = %v", err)
	}

	destinationHandler := httpapi.ProviderRegistryHandlers{Service: destination.Service()}
	importBody, err := json.Marshal(struct {
		SchemaVersion int    `json:"schemaVersion"`
		ManifestJSON  string `json:"manifestJson"`
	}{SchemaVersion: 1, ManifestJSON: exportResponse.ManifestJSON})
	if err != nil {
		t.Fatalf("Marshal(portable import) error = %v", err)
	}
	importedRecorder := httptest.NewRecorder()
	destinationHandler.Handle(importedRecorder, httptest.NewRequest(http.MethodPost,
		httpapi.ProviderRegistryPortableManifestPathV1, bytes.NewReader(importBody)))
	if importedRecorder.Code != http.StatusOK {
		t.Fatalf("destination portable import = (%d, %s)", importedRecorder.Code, importedRecorder.Body.String())
	}
	protectedRecoveryR7AssertRedacted(t, importedRecorder.Body.Bytes(), string(secretMarker), "credentialRef")
	var importResponse protectedRecoveryR7ImportResponse
	if err := json.Unmarshal(importedRecorder.Body.Bytes(), &importResponse); err != nil ||
		importResponse.SchemaVersion != 1 || importResponse.ProviderCount != 1 || importResponse.AccountCount != 0 ||
		importResponse.ReentryRequired != 1 || len(importResponse.Entries) != 1 ||
		importResponse.Entries[0].Status != domainregistry.PortableManifestIntentReentryRequired ||
		importResponse.Entries[0].DestinationProviderID == sourceProviderID {
		t.Fatalf("destination portable import response = (%d, %s), error = %v", importedRecorder.Code, importedRecorder.Body.String(), err)
	}
	destinationProviderID := importResponse.Entries[0].DestinationProviderID
	destinationImportedState, err := destination.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("destination Snapshot(after ordinary import) error = %v", err)
	}
	destinationImportedProvider, ok := destinationImportedState.Providers[destinationProviderID]
	if !ok {
		t.Fatal("destination ordinary import result did not identify a current Provider")
	}
	protectedImportResult := providerregistryapp.ProtectedRecoveryImportResult{
		ProviderCount: importResponse.ProviderCount, AccountCount: importResponse.AccountCount,
		ReentryRequired: importResponse.ReentryRequired,
		Entries: []providerregistryapp.ProtectedRecoveryImportEntryResult{{
			Correlation: importResponse.Entries[0].Correlation, DestinationProviderID: destinationProviderID,
			Status: importResponse.Entries[0].Status,
			Fence: providerregistryapp.ProtectedRecoveryImportFence{
				Revision:    strconv.FormatUint(destinationImportedProvider.Revision, 10),
				Generation:  strconv.FormatUint(destinationImportedProvider.Generation, 10),
				Incarnation: destinationImportedProvider.Incarnation,
			},
		}},
	}

	destinationBinding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: "synthetic-r7-main-window", MainFrameID: "synthetic-r7-main-frame",
		ProfileBinding: "synthetic-r7-destination-profile", DataDirectoryBinding: "synthetic-r7-destination-data",
	}
	prepareRecorder := protectedRecoveryR7HTTPCall(t, destinationHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryPreparePathV1,
		protectedRecoveryR7PrepareRequest{
			SchemaVersion: 1, ManifestBase64: protectedRecoveryR7Encoded(manifest),
			ImportResult: protectedImportResult,
			LocalBinding: destinationBinding, OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
		})
	if prepareRecorder.Code != http.StatusOK {
		t.Fatalf("protected prepare = (%d, %s)", prepareRecorder.Code, prepareRecorder.Body.String())
	}
	protectedRecoveryR7AssertRedacted(t, prepareRecorder.Body.Bytes(), string(secretMarker), sourceProvider.ID, "credentialRef")
	var prepareResponse protectedRecoveryR7PrepareResponse
	if err := json.Unmarshal(prepareRecorder.Body.Bytes(), &prepareResponse); err != nil || prepareResponse.SchemaVersion != 1 {
		t.Fatalf("protected prepare response = %s, error = %v", prepareRecorder.Body.String(), err)
	}
	request := protectedRecoveryR7DecodeBase64(t, prepareResponse.RequestBase64, domainregistry.ProtectedRecoveryMaxBytes)
	parsedRequest, err := domainregistry.ParseProtectedRecoveryRequestV1(request)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryRequestV1(production) error = %v", err)
	}
	if parsedRequest.ManifestDigest != domainregistry.ProtectedRecoveryManifestDigest(manifest) ||
		parsedRequest.OperationID != prepareResponse.OperationID || parsedRequest.SessionNonce != prepareResponse.SessionNonce ||
		parsedRequest.ExpiresAt != prepareResponse.ExpiresAt || prepareResponse.RequestDigest != parsedRequest.RequestDigest ||
		prepareResponse.RequestFingerprint != parsedRequest.VerificationFingerprint {
		t.Fatal("protected prepare response did not preserve the canonical generated request metadata")
	}
	confirmation := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: destinationBinding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: parsedRequest.RequestDigest, RequestFingerprint: parsedRequest.VerificationFingerprint,
			OperationID: parsedRequest.OperationID, SessionNonce: parsedRequest.SessionNonce,
			ManifestDigest: parsedRequest.ManifestDigest, ItemSetDigest: domainregistry.ProtectedRecoveryItemSetDigest(parsedRequest.Entries),
			ExpiresAt: parsedRequest.ExpiresAt, LocalBinding: destinationBinding,
		},
	}

	// A fresh Manager after request creation must not auto-confirm or publish.
	if err := destination.Close(); err != nil {
		t.Fatalf("destination Close(before fresh confirmation) error = %v", err)
	}
	destination, err = openProviderRegistryAuthorityV1(ctx, destinationDir)
	if err != nil {
		t.Fatalf("reopen destination authority(before fresh confirmation) error = %v", err)
	}
	destinationHandler = httpapi.ProviderRegistryHandlers{Service: destination.Service()}
	recoveredRecorder := protectedRecoveryR7HTTPCall(t, destinationHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryRecoverPathV1,
		struct {
			SchemaVersion int `json:"schemaVersion"`
		}{SchemaVersion: 1})
	var recoveredResponse protectedRecoveryR7StatusResponse
	if recoveredRecorder.Code != http.StatusOK || json.Unmarshal(recoveredRecorder.Body.Bytes(), &recoveredResponse) != nil ||
		recoveredResponse.Status != "pending" || recoveredResponse.Confirmed {
		t.Fatalf("protected recover after restart = (%d, %s)", recoveredRecorder.Code, recoveredRecorder.Body.String())
	}
	confirmedRecorder := protectedRecoveryR7HTTPCall(t, destinationHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryConfirmDestinationPathV1,
		protectedRecoveryR7ConfirmRequest{
			SchemaVersion: 1, RequestBase64: protectedRecoveryR7Encoded(request), LocalAction: confirmation,
			OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
		})
	if confirmedRecorder.Code != http.StatusOK || !strings.Contains(confirmedRecorder.Body.String(), `"status":"confirmed"`) {
		t.Fatalf("protected destination confirmation = (%d, %s)", confirmedRecorder.Code, confirmedRecorder.Body.String())
	}

	sourceBinding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: "synthetic-r7-main-window", MainFrameID: "synthetic-r7-main-frame",
		ProfileBinding: "synthetic-r7-source-profile", DataDirectoryBinding: "synthetic-r7-source-data",
	}
	sourceAction := confirmation
	sourceAction.LocalBinding = sourceBinding
	sourceAction.Confirmation.LocalBinding = sourceBinding
	bundleRecorder := protectedRecoveryR7HTTPCall(t, sourceHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryCreateBundlePathV1,
		protectedRecoveryR7CreateBundleRequest{
			SchemaVersion: 1, RequestBase64: protectedRecoveryR7Encoded(request), LocalAction: sourceAction,
			OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
		})
	if bundleRecorder.Code != http.StatusOK {
		t.Fatalf("protected source bundle = (%d, %s)", bundleRecorder.Code, bundleRecorder.Body.String())
	}
	protectedRecoveryR7AssertRedacted(t, bundleRecorder.Body.Bytes(), string(secretMarker), sourceProvider.ID, sourceProvider.CredentialRef, "credentialRef")
	var bundleResponse protectedRecoveryR7ArtifactResponse
	if err := json.Unmarshal(bundleRecorder.Body.Bytes(), &bundleResponse); err != nil || bundleResponse.SchemaVersion != 1 {
		t.Fatalf("protected bundle response = %s, error = %v", bundleRecorder.Body.String(), err)
	}
	bundle := protectedRecoveryR7DecodeBase64(t, bundleResponse.BundleBase64, domainregistry.ProtectedRecoveryMaxBytes)
	if _, err := domainregistry.ParseProtectedRecoveryBundleV1(bundle); err != nil {
		t.Fatalf("ParseProtectedRecoveryBundleV1(production) error = %v", err)
	}

	applyRecorder := protectedRecoveryR7HTTPCall(t, destinationHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryApplyPathV1,
		protectedRecoveryR7ApplyRequest{
			SchemaVersion: 1, BundleBase64: protectedRecoveryR7Encoded(bundle), RequestBase64: protectedRecoveryR7Encoded(request),
			OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
		})
	if applyRecorder.Code != http.StatusOK {
		t.Fatalf("protected destination apply = (%d, %s)", applyRecorder.Code, applyRecorder.Body.String())
	}
	protectedRecoveryR7AssertRedacted(t, applyRecorder.Body.Bytes(), string(secretMarker), sourceProvider.ID, sourceProvider.CredentialRef, "credentialRef")
	var receiptResponse protectedRecoveryR7ArtifactResponse
	if err := json.Unmarshal(applyRecorder.Body.Bytes(), &receiptResponse); err != nil || receiptResponse.SchemaVersion != 1 {
		t.Fatalf("protected receipt response = %s, error = %v", applyRecorder.Body.String(), err)
	}
	receipt := protectedRecoveryR7DecodeBase64(t, receiptResponse.ReceiptBase64, domainregistry.ProtectedRecoveryMaxBytes)
	parsedReceipt, err := domainregistry.ParseProtectedRecoveryReceiptV1(receipt)
	if err != nil || len(parsedReceipt.AuthenticationTag) != 32 {
		t.Fatalf("authenticated protected receipt = %#v, error = %v", parsedReceipt, err)
	}

	destinationState, err := destination.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("destination Snapshot(after apply) error = %v", err)
	}
	destinationProvider := destinationState.Providers[destinationProviderID]
	if destinationState.SelectedProviderID != "" || destinationProvider.CredentialRef == "" ||
		destinationProvider.CredentialPurpose != "provider-api-key" || destinationProvider.ID == sourceProviderID {
		t.Fatalf("destination protected winner = %#v, selected=%q", destinationProvider, destinationState.SelectedProviderID)
	}
	protectedSession, ok := destinationState.ProtectedRecoverySessions[parsedReceipt.OperationID]
	if !ok || protectedSession.Phase != domainregistry.ProtectedRecoveryPhaseApplied || !protectedSession.Consumed || protectedSession.DestinationKeyRef != "" {
		t.Fatalf("destination protected session after verified cleanup = %#v", protectedSession)
	}
	winnerSecret, err := destination.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(destinationProvider.CredentialRef),
		Purpose:       secretstoreport.Purpose(destinationProvider.CredentialPurpose), Consumer: providerregistryapp.RegistryReadbackConsumer,
	})
	if err != nil {
		clearRuntimeappTestBytes(winnerSecret)
		t.Fatalf("destination K1 winner readback error = %v", err)
	}
	if !bytes.Equal(winnerSecret, secretMarker) {
		clearRuntimeappTestBytes(winnerSecret)
		t.Fatal("destination K1 winner did not contain the synthetic source credential")
	}
	clearRuntimeappTestBytes(winnerSecret)

	// A replay is rejected after terminal verification and cannot read or
	// prepare another credential. The response is still key-free.
	destinationBeforeReplay := destinationState
	replayRecorder := protectedRecoveryR7HTTPCall(t, destinationHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryApplyPathV1,
		protectedRecoveryR7ApplyRequest{
			SchemaVersion: 1, BundleBase64: protectedRecoveryR7Encoded(bundle), RequestBase64: protectedRecoveryR7Encoded(request),
			OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{},
		})
	if replayRecorder.Code == http.StatusOK {
		t.Fatal("protected bundle replay unexpectedly succeeded")
	}
	protectedRecoveryR7AssertRedacted(t, replayRecorder.Body.Bytes(), string(secretMarker), sourceProvider.CredentialRef, "credentialRef")
	destinationAfterReplay, err := destination.Manager().Snapshot(ctx)
	if err != nil || !reflect.DeepEqual(destinationBeforeReplay, destinationAfterReplay) {
		t.Fatalf("protected replay changed destination state: error=%v before=%#v after=%#v", err, destinationBeforeReplay, destinationAfterReplay)
	}

	// A forged but syntactically valid receipt cannot finalize the source
	// session. Only the exact authenticated receipt persisted by source bundle
	// creation is accepted.
	forgedReceiptRecord := parsedReceipt.Clone()
	forgedReceiptRecord.AuthenticationTag = bytes.Repeat([]byte{0x5A}, 32)
	forgedReceipt, err := domainregistry.MarshalProtectedRecoveryReceiptV1(forgedReceiptRecord)
	if err != nil {
		t.Fatalf("Marshal(forged protected receipt) error = %v", err)
	}
	sourceBeforeForgedFinalize, err := source.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("source Snapshot(before forged finalize) error = %v", err)
	}
	forgedFinalize := protectedRecoveryR7HTTPCall(t, sourceHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryFinalizePathV1,
		protectedRecoveryR7ReceiptRequest{SchemaVersion: 1, ReceiptBase64: protectedRecoveryR7Encoded(forgedReceipt)})
	if forgedFinalize.Code == http.StatusOK {
		t.Fatal("forged authenticated receipt unexpectedly finalized source")
	}
	protectedRecoveryR7AssertRedacted(t, forgedFinalize.Body.Bytes(), string(secretMarker), sourceProvider.CredentialRef, "credentialRef")
	sourceAfterForgedFinalize, err := source.Manager().Snapshot(ctx)
	if err != nil || !reflect.DeepEqual(sourceBeforeForgedFinalize.Providers[sourceProvider.ID], sourceAfterForgedFinalize.Providers[sourceProvider.ID]) ||
		sourceAfterForgedFinalize.SelectedProviderID != sourceBeforeForgedFinalize.SelectedProviderID {
		t.Fatalf("forged receipt changed source authority: error=%v before=%#v after=%#v", err, sourceBeforeForgedFinalize, sourceAfterForgedFinalize)
	}
	validFinalize := protectedRecoveryR7HTTPCall(t, sourceHandler,
		httpapi.ProviderRegistryPrivateProtectedRecoveryFinalizePathV1,
		protectedRecoveryR7ReceiptRequest{SchemaVersion: 1, ReceiptBase64: protectedRecoveryR7Encoded(receipt)})
	if validFinalize.Code != http.StatusOK || !strings.Contains(validFinalize.Body.String(), `"status":"finalized"`) {
		t.Fatalf("authenticated source finalize = (%d, %s)", validFinalize.Code, validFinalize.Body.String())
	}

	sourceAfter, err := source.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("source Snapshot(after finalize) error = %v", err)
	}
	if !reflect.DeepEqual(sourceProviderBefore, sourceAfter.Providers[sourceProvider.ID]) || sourceAfter.SelectedProviderID != sourceSelectionBefore {
		t.Fatal("protected recovery changed source Provider or selection authority")
	}
	sourceSecret, err := source.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(sourceProviderBefore.CredentialRef),
		Purpose:       secretstoreport.Purpose(sourceProviderBefore.CredentialPurpose), Consumer: providerregistryapp.RegistryReadbackConsumer,
	})
	if err != nil {
		clearRuntimeappTestBytes(sourceSecret)
		t.Fatalf("source K1 readback after finalize error = %v", err)
	}
	if !bytes.Equal(sourceSecret, secretMarker) {
		clearRuntimeappTestBytes(sourceSecret)
		t.Fatal("source K1 credential changed during protected recovery")
	}
	clearRuntimeappTestBytes(sourceSecret)

	secretStoreBytes, err := os.ReadFile(filepath.Join(destinationDir, "private", "provider-secrets", providerRegistrySecretStoreFileV1))
	if err != nil {
		t.Fatalf("ReadFile(destination Secret Store) error = %v", err)
	}
	defer clearRuntimeappTestBytes(secretStoreBytes)
	protectedRecoveryR7AssertRedacted(t, secretStoreBytes, string(secretMarker), "protected-recovery-pending-key")
	if bytes.Contains(secretStoreBytes, []byte(sourceProvider.CredentialRef)) {
		t.Fatal("destination Secret Store contains the source credential reference")
	}

	// A fresh production authority observes the terminal, cleaned winner and
	// still keeps the imported Provider unselected until a normal K2 selection.
	if err := destination.Close(); err != nil {
		t.Fatalf("destination Close(after protected recovery) error = %v", err)
	}
	destination, err = openProviderRegistryAuthorityV1(ctx, destinationDir)
	if err != nil {
		t.Fatalf("reopen destination authority(after protected recovery) error = %v", err)
	}
	destinationHandler = httpapi.ProviderRegistryHandlers{Service: destination.Service()}
	restartedList := httptest.NewRecorder()
	destinationHandler.Handle(restartedList, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil))
	var restartedListResponse portableManifestHTTPListResponseV1
	if restartedList.Code != http.StatusOK || json.Unmarshal(restartedList.Body.Bytes(), &restartedListResponse) != nil ||
		restartedListResponse.SelectedProviderID != "" || len(restartedListResponse.Providers) != 1 ||
		!restartedListResponse.Providers[0].CredentialConfigured || restartedListResponse.Providers[0].ID != destinationProviderID {
		t.Fatalf("destination public list after protected restart = (%d, %s)", restartedList.Code, restartedList.Body.String())
	}
	protectedRecoveryR7AssertRedacted(t, restartedList.Body.Bytes(), string(secretMarker), sourceProvider.CredentialRef, "credentialRef")

}
