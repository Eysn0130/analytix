package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
)

type protectedRecoveryHTTPService struct {
	*recordingProviderRegistryHTTPService
	prepared domainregistry.ProtectedRecoveryPreparedRequestV1
	bundle   []byte
	receipt  []byte
	calls    []string
	prepare  providerregistryapp.ProtectedRecoveryRequestCommand
}

func (service *protectedRecoveryHTTPService) PrepareProtectedRecoveryRequest(
	_ context.Context,
	command providerregistryapp.ProtectedRecoveryRequestCommand,
) (domainregistry.ProtectedRecoveryPreparedRequestV1, error) {
	service.calls = append(service.calls, "prepare")
	service.prepare = command
	return service.prepared, nil
}

func (service *protectedRecoveryHTTPService) ConfirmProtectedRecoveryDestination(
	_ context.Context,
	_ providerregistryapp.ProtectedRecoveryDestinationConfirmationCommand,
) error {
	service.calls = append(service.calls, "confirm")
	return nil
}

func (service *protectedRecoveryHTTPService) CreateProtectedRecoveryBundle(
	_ context.Context,
	_ providerregistryapp.ProtectedRecoveryBundleCommand,
) ([]byte, error) {
	service.calls = append(service.calls, "create-bundle")
	return append([]byte(nil), service.bundle...), nil
}

func (service *protectedRecoveryHTTPService) ApplyProtectedRecoveryBundle(
	_ context.Context,
	_ providerregistryapp.ProtectedRecoveryApplyCommand,
) ([]byte, error) {
	service.calls = append(service.calls, "apply")
	return append([]byte(nil), service.receipt...), nil
}

func (service *protectedRecoveryHTTPService) FinalizeProtectedRecoveryReceipt(
	_ context.Context,
	_ providerregistryapp.ProtectedRecoveryFinalizeCommand,
) (domainregistry.ProtectedRecoveryResultV1, error) {
	service.calls = append(service.calls, "finalize")
	return domainregistry.ProtectedRecoveryResultV1{Status: "finalized"}, nil
}

func (service *protectedRecoveryHTTPService) RecoverProtectedRecovery(context.Context, ...providerregistryapp.ProtectedRecoveryRecoverCommand) (domainregistry.ProtectedRecoveryResultV1, error) {
	service.calls = append(service.calls, "recover")
	return domainregistry.ProtectedRecoveryResultV1{Status: "pending", Confirmed: false}, nil
}

func (service *protectedRecoveryHTTPService) RollbackProtectedRecovery(
	_ context.Context,
	_ providerregistryapp.ProtectedRecoveryRollbackCommand,
) (domainregistry.ProtectedRecoveryResultV1, error) {
	service.calls = append(service.calls, "rollback")
	return domainregistry.ProtectedRecoveryResultV1{Status: "rolled_back"}, nil
}

func TestProviderRegistryHTTPProtectedRecoveryUsesStrictPrivateArtifactOperations(t *testing.T) {
	t.Parallel()

	requestBytes, parsedRequest := protectedRecoveryHTTPFixtureRequest(t)
	bundleBytes := protectedRecoveryHTTPFixtureBundle(t, parsedRequest)
	receiptBytes := protectedRecoveryHTTPFixtureReceipt(t, parsedRequest, bundleBytes)
	service := &protectedRecoveryHTTPService{
		recordingProviderRegistryHTTPService: &recordingProviderRegistryHTTPService{},
		prepared: domainregistry.ProtectedRecoveryPreparedRequestV1{
			Request: bytes.Clone(requestBytes), RequestDigest: parsedRequest.RequestDigest,
			RequestFingerprint: parsedRequest.VerificationFingerprint, ManifestDigest: parsedRequest.ManifestDigest,
			ItemSetDigest: domainregistry.ProtectedRecoveryItemSetDigest(parsedRequest.Entries),
			OperationID:   parsedRequest.OperationID, SessionNonce: parsedRequest.SessionNonce, ExpiresAt: parsedRequest.ExpiresAt,
		},
		bundle: bundleBytes, receipt: receiptBytes,
	}
	handler := ProviderRegistryHandlers{Service: service}
	manifest := []byte(`{"schema":"analytix.provider-portable-manifest/v1","providers":[{"correlation":"provider-0","kind":"openai-compatible","endpoint":"https://provider.invalid/v1","models":[],"mediaModels":[],"routes":[],"intent":"reentry_required"}],"accounts":[]}`)
	importResult := providerregistryapp.ProtectedRecoveryImportResult{
		ProviderCount: 1, ReentryRequired: 1,
		Entries: []providerregistryapp.ProtectedRecoveryImportEntryResult{{
			Correlation: "provider-0", DestinationProviderID: "provider-destination", Status: "reentry_required",
			Fence: providerregistryapp.ProtectedRecoveryImportFence{
				Revision: "17", Generation: "5", Incarnation: "inc_" + strings.Repeat("f", 43),
			},
		}},
	}
	binding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: "window", MainFrameID: "frame", ProfileBinding: "profile", DataDirectoryBinding: "/tmp/profile",
	}
	action := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: binding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: parsedRequest.RequestDigest, RequestFingerprint: parsedRequest.VerificationFingerprint,
			OperationID: parsedRequest.OperationID, SessionNonce: parsedRequest.SessionNonce,
			ManifestDigest: parsedRequest.ManifestDigest, ItemSetDigest: domainregistry.ProtectedRecoveryItemSetDigest(parsedRequest.Entries),
			ExpiresAt: parsedRequest.ExpiresAt, LocalBinding: binding,
		},
	}
	if err := action.Validate(); err != nil {
		t.Fatalf("test local action is invalid: %v", err)
	}
	ownerInventory := []domainregistry.ProtectedRecoveryOwnerBindingV1{}

	prepareBody := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryPrepareRequest{
		SchemaVersion: 1, ManifestBase64: protectedRecoveryHTTPBase64(manifest), ImportResult: importResult,
		LocalBinding: binding, OwnerBindingInventory: ownerInventory,
	})
	prepareResponse := protectedRecoveryHTTPCall(t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryPreparePathV1, prepareBody)
	if prepareResponse.Code != http.StatusOK || !bytes.Contains(prepareResponse.Body.Bytes(), []byte(`"requestBase64"`)) {
		t.Fatalf("prepare response = (%d, %s)", prepareResponse.Code, prepareResponse.Body.String())
	}
	if len(service.prepare.ImportResult.Entries) != 1 ||
		service.prepare.ImportResult.Entries[0].Fence != importResult.Entries[0].Fence {
		t.Fatalf("private prepare receipt fence = %#v, want exact admitted fence", service.prepare.ImportResult.Entries)
	}
	assertProtectedRecoveryHTTPRedacted(t, prepareResponse, "synthetic-secret-marker", "credentialRef", "privateKey")

	confirmBody := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryConfirmRequest{
		SchemaVersion: 1, RequestBase64: protectedRecoveryHTTPBase64(requestBytes), LocalAction: action,
		OwnerBindingInventory: ownerInventory,
	})
	confirmResponse := protectedRecoveryHTTPCall(t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryConfirmDestinationPathV1, confirmBody)
	if confirmResponse.Code != http.StatusOK || !bytes.Contains(confirmResponse.Body.Bytes(), []byte(`"confirmed":true`)) {
		t.Fatalf("confirm response = (%d, %s)", confirmResponse.Code, confirmResponse.Body.String())
	}

	createBody := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryCreateBundleRequest{
		SchemaVersion: 1, RequestBase64: protectedRecoveryHTTPBase64(requestBytes), LocalAction: action,
		OwnerBindingInventory: ownerInventory,
	})
	createResponse := protectedRecoveryHTTPCall(t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryCreateBundlePathV1, createBody)
	if createResponse.Code != http.StatusOK || !bytes.Contains(createResponse.Body.Bytes(), []byte(`"bundleBase64"`)) {
		t.Fatalf("create response = (%d, %s)", createResponse.Code, createResponse.Body.String())
	}

	applyBody := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryApplyRequest{
		SchemaVersion: 1, BundleBase64: protectedRecoveryHTTPBase64(bundleBytes), RequestBase64: protectedRecoveryHTTPBase64(requestBytes),
		OwnerBindingInventory: ownerInventory,
	})
	applyResponse := protectedRecoveryHTTPCall(t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryApplyPathV1, applyBody)
	if applyResponse.Code != http.StatusOK || !bytes.Contains(applyResponse.Body.Bytes(), []byte(`"receiptBase64"`)) {
		t.Fatalf("apply response = (%d, %s)", applyResponse.Code, applyResponse.Body.String())
	}

	finalizeBody := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryFinalizeRequest{
		SchemaVersion: 1, ReceiptBase64: protectedRecoveryHTTPBase64(receiptBytes),
	})
	finalizeResponse := protectedRecoveryHTTPCall(t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryFinalizePathV1, finalizeBody)
	if finalizeResponse.Code != http.StatusOK || !bytes.Contains(finalizeResponse.Body.Bytes(), []byte(`"status":"finalized"`)) {
		t.Fatalf("finalize response = (%d, %s)", finalizeResponse.Code, finalizeResponse.Body.String())
	}

	for _, operation := range []struct {
		name string
		path string
	}{
		{name: "recover", path: ProviderRegistryPrivateProtectedRecoveryRecoverPathV1},
		{name: "rollback", path: ProviderRegistryPrivateProtectedRecoveryRollbackPathV1},
	} {
		body := []byte(`{"schemaVersion":1}`)
		if operation.name == "rollback" {
			body = marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryRollbackRequest{
				SchemaVersion: 1, RequestBase64: protectedRecoveryHTTPBase64(requestBytes),
			})
		}
		response := protectedRecoveryHTTPCall(t, handler, http.MethodPost, operation.path, body)
		if response.Code != http.StatusOK {
			t.Fatalf("%s response = (%d, %s)", operation.name, response.Code, response.Body.String())
		}
	}

	if strings.Join(service.calls, ",") != "prepare,confirm,create-bundle,apply,finalize,recover,rollback" {
		t.Fatalf("private operation calls = %v", service.calls)
	}
}

func TestProviderRegistryHTTPProtectedRecoveryPreservesExactAdmittedAccountBinding(t *testing.T) {
	t.Parallel()

	requestBytes, parsedRequest := protectedRecoveryHTTPFixtureRequest(t)
	service := &protectedRecoveryHTTPService{
		recordingProviderRegistryHTTPService: &recordingProviderRegistryHTTPService{},
		prepared: domainregistry.ProtectedRecoveryPreparedRequestV1{
			Request: bytes.Clone(requestBytes), RequestDigest: parsedRequest.RequestDigest,
			RequestFingerprint: parsedRequest.VerificationFingerprint, ManifestDigest: parsedRequest.ManifestDigest,
			ItemSetDigest: domainregistry.ProtectedRecoveryItemSetDigest(parsedRequest.Entries),
			OperationID:   parsedRequest.OperationID, SessionNonce: parsedRequest.SessionNonce, ExpiresAt: parsedRequest.ExpiresAt,
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	manifest := []byte(`{"schema":"analytix.provider-portable-manifest/v1","providers":[],"accounts":[{"correlation":"account-0","owner":"mcp","provider":"synthetic-mcp-provider","endpoint":"https://account.invalid/v1","purpose":"mcp-oauth-access-token","intent":"reentry_required"}]}`)
	admitted := domainregistry.ProtectedRecoveryOwnerBindingV1{
		Correlation: "account-0", Owner: "mcp", Provider: "synthetic-mcp-provider",
		AccountID: "synthetic-destination-account", ChannelID: "synthetic-destination-channel",
		Purpose: "mcp-oauth-access-token", Fingerprint: "synthetic-destination-fingerprint",
	}
	importResult := providerregistryapp.ProtectedRecoveryImportResult{
		AccountCount: 1, ReentryRequired: 1,
		Entries: []providerregistryapp.ProtectedRecoveryImportEntryResult{{
			Correlation: "account-0", DestinationProviderID: "provider-destination-account", Status: "reentry_required",
			Fence: providerregistryapp.ProtectedRecoveryImportFence{
				Revision: "17", Generation: "5", Incarnation: "inc_" + strings.Repeat("a", 43),
			},
			DestinationOwnerBinding: &admitted,
		}},
	}
	body := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryPrepareRequest{
		SchemaVersion: 1, ManifestBase64: protectedRecoveryHTTPBase64(manifest), ImportResult: importResult,
		LocalBinding: domainregistry.ProtectedRecoveryLocalBindingV1{
			BrowserWindowID: "window", MainFrameID: "frame", ProfileBinding: "profile", DataDirectoryBinding: "/tmp/profile",
		},
		OwnerBindingInventory: []domainregistry.ProtectedRecoveryOwnerBindingV1{admitted},
	})
	response := protectedRecoveryHTTPCall(
		t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryPreparePathV1, body,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("private account prepare response = (%d, %s)", response.Code, response.Body.String())
	}
	if len(service.prepare.ImportResult.Entries) != 1 ||
		service.prepare.ImportResult.Entries[0].DestinationOwnerBinding == nil ||
		*service.prepare.ImportResult.Entries[0].DestinationOwnerBinding != admitted {
		t.Fatalf("private prepare admitted binding = %#v, want %#v", service.prepare.ImportResult.Entries, admitted)
	}
}

func TestProviderRegistryHTTPProtectedRecoveryContinuationAllowsMissingCorrelationButSourceRemainsStrict(t *testing.T) {
	t.Parallel()

	requestBytes, parsedRequest := protectedRecoveryHTTPFixtureRequest(t)
	service := &protectedRecoveryHTTPService{
		recordingProviderRegistryHTTPService: &recordingProviderRegistryHTTPService{},
		bundle:                               protectedRecoveryHTTPFixtureBundle(t, parsedRequest),
	}
	handler := ProviderRegistryHandlers{Service: service}
	binding := domainregistry.ProtectedRecoveryLocalBindingV1{
		BrowserWindowID: "window", MainFrameID: "frame", ProfileBinding: "profile", DataDirectoryBinding: "/tmp/profile",
	}
	action := domainregistry.ProtectedRecoveryLocalActionV1{
		LocalBinding: binding,
		Confirmation: domainregistry.ProtectedRecoveryConfirmationV1{
			RequestDigest: parsedRequest.RequestDigest, RequestFingerprint: parsedRequest.VerificationFingerprint,
			OperationID: parsedRequest.OperationID, SessionNonce: parsedRequest.SessionNonce,
			ManifestDigest: parsedRequest.ManifestDigest, ItemSetDigest: domainregistry.ProtectedRecoveryItemSetDigest(parsedRequest.Entries),
			ExpiresAt: parsedRequest.ExpiresAt, LocalBinding: binding,
		},
	}
	correlationless := []domainregistry.ProtectedRecoveryOwnerBindingV1{{
		Owner: "mcp", Provider: "destination-provider", AccountID: "destination-account",
		ChannelID: "destination-channel", Purpose: "mcp-oauth-access-token", Fingerprint: "destination-binding",
	}}

	confirmBody := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryConfirmRequest{
		SchemaVersion: 1, RequestBase64: protectedRecoveryHTTPBase64(requestBytes), LocalAction: action,
		OwnerBindingInventory: correlationless,
	})
	confirmResponse := protectedRecoveryHTTPCall(
		t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryConfirmDestinationPathV1, confirmBody,
	)
	if confirmResponse.Code != http.StatusOK || strings.Join(service.calls, ",") != "confirm" {
		t.Fatalf("correlationless continuation response = (%d, %s), calls=%v", confirmResponse.Code, confirmResponse.Body.String(), service.calls)
	}

	service.calls = nil
	createBody := marshalProtectedRecoveryHTTPJSON(t, providerRegistryPrivateProtectedRecoveryCreateBundleRequest{
		SchemaVersion: 1, RequestBase64: protectedRecoveryHTTPBase64(requestBytes), LocalAction: action,
		OwnerBindingInventory: correlationless,
	})
	createResponse := protectedRecoveryHTTPCall(
		t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryCreateBundlePathV1, createBody,
	)
	if createResponse.Code != http.StatusBadRequest || len(service.calls) != 0 {
		t.Fatalf("correlationless source response = (%d, %s), calls=%v; want strict pre-service rejection",
			createResponse.Code, createResponse.Body.String(), service.calls)
	}
}

func TestProviderRegistryHTTPProtectedRecoveryRejectsMalformedArtifactsBeforeService(t *testing.T) {
	t.Parallel()

	manifest := []byte(`{"schema":"analytix.provider-portable-manifest/v1","providers":[],"accounts":[]}`)
	encodedManifest := base64.StdEncoding.EncodeToString(manifest)
	base := `{"schemaVersion":1,"manifestBase64":"` + encodedManifest + `","importResult":{"providerCount":0,"accountCount":0,"reentryRequired":0,"entries":[]},"localBinding":{"browserWindowId":"window","mainFrameId":"frame","profileBinding":"profile","dataDirectoryBinding":"/tmp/profile"},"ownerBindingInventory":[]}`
	legacyManifest := base64.StdEncoding.EncodeToString([]byte(`{"schema":"analytix.provider-portable-manifest/v0","providers":[],"accounts":[]}`))
	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "duplicate wrapper key", body: strings.Replace(base, `{"schemaVersion":1,`, `{"schemaVersion":1,"schemaVersion":1,`, 1)},
		{name: "unknown wrapper field", body: strings.Replace(base, `,"ownerBindingInventory":[]}`, `,"ownerBindingInventory":[],"credentialRef":"cred_`+strings.Repeat("x", 43)+`"}`, 1)},
		{name: "invalid base64", body: strings.Replace(base, `"manifestBase64":"`+base64.StdEncoding.EncodeToString([]byte(`{"schema":"analytix.provider-portable-manifest/v1","providers":[],"accounts":[]}`))+`"`, `"manifestBase64":"%%%"`, 1)},
		{name: "legacy manifest", body: strings.Replace(base, encodedManifest, legacyManifest, 1)},
		{name: "trailing data", body: base + "{}"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &protectedRecoveryHTTPService{recordingProviderRegistryHTTPService: &recordingProviderRegistryHTTPService{}}
			handler := ProviderRegistryHandlers{Service: service}
			response := protectedRecoveryHTTPCall(t, handler, http.MethodPost, ProviderRegistryPrivateProtectedRecoveryPreparePathV1, []byte(testCase.body))
			if response.Code != http.StatusBadRequest || len(service.calls) != 0 {
				t.Fatalf("response = (%d, %s), calls=%v", response.Code, response.Body.String(), service.calls)
			}
		})
	}
	service := &protectedRecoveryHTTPService{recordingProviderRegistryHTTPService: &recordingProviderRegistryHTTPService{}}
	handler := ProviderRegistryHandlers{Service: service}
	getResponse := protectedRecoveryHTTPCall(t, handler, http.MethodGet, ProviderRegistryPrivateProtectedRecoveryPreparePathV1, nil)
	if getResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET private prepare status = %d, want method not allowed", getResponse.Code)
	}
}

func protectedRecoveryHTTPCall(t *testing.T, handler ProviderRegistryHandlers, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(string(body))
	}
	handler.Handle(recorder, httptest.NewRequest(method, path, reader))
	return recorder
}

func marshalProtectedRecoveryHTTPJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return encoded
}

func protectedRecoveryHTTPBase64(value []byte) json.RawMessage {
	encoded, _ := json.Marshal(base64.StdEncoding.EncodeToString(value))
	return encoded
}

func protectedRecoveryHTTPFixtureRequest(t *testing.T) ([]byte, domainregistry.ProtectedRecoveryRequestV1) {
	t.Helper()
	request := domainregistry.ProtectedRecoveryRequestV1{
		Schema: domainregistry.ProtectedRecoveryRequestSchemaV1, ProtocolVersion: 1,
		ManifestDigest: strings.Repeat("a", 64), OperationID: "operation-synthetic-wire",
		SessionNonce: strings.Repeat("b", 32), ExpiresAt: "2099-01-02T03:04:05Z",
		DestinationEphemeralPublicKey: bytes.Repeat([]byte{1}, 32),
		Entries: []domainregistry.ProtectedRecoveryEntryV1{{
			Correlation: "provider-0", DestinationProviderID: "provider-destination",
			DestinationProviderRevision: 1, DestinationProviderGeneration: 1,
			DestinationProviderIncarnation: "inc_" + strings.Repeat("a", 43),
		}},
	}
	request.VerificationFingerprint = "0000000000000000"
	if err := request.Validate(); err != nil {
		t.Fatalf("fixture request validation before digest = %v", err)
	}
	request.VerificationFingerprint = ""
	digestBytes, err := domainregistry.MarshalProtectedRecoveryRequestV1ForDigest(request)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryRequestV1ForDigest() error = %v", err)
	}
	digest := sha256.Sum256(digestBytes)
	request.RequestDigest = hex.EncodeToString(digest[:])
	request.VerificationFingerprint = domainregistry.ProtectedRecoveryFingerprint(request.RequestDigest)
	encoded, err := domainregistry.MarshalProtectedRecoveryRequestV1(request)
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryRequestV1() error = %v", err)
	}
	parsed, err := domainregistry.ParseProtectedRecoveryRequestV1(encoded)
	if err != nil {
		t.Fatalf("ParseProtectedRecoveryRequestV1() error = %v", err)
	}
	return encoded, parsed
}

func protectedRecoveryHTTPFixtureBundle(t *testing.T, request domainregistry.ProtectedRecoveryRequestV1) []byte {
	t.Helper()
	bundle, err := domainregistry.MarshalProtectedRecoveryBundleV1(domainregistry.ProtectedRecoveryBundleV1{
		Schema: domainregistry.ProtectedRecoveryBundleSchemaV1, ProtocolVersion: 1,
		RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest,
		OperationID: request.OperationID, SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt,
		SourceEphemeralPublicKey: bytes.Repeat([]byte{2}, 32), Nonce: bytes.Repeat([]byte{3}, 12), Ciphertext: []byte("synthetic-ciphertext"),
		Entries: []domainregistry.ProtectedRecoveryBundleEntryV1{{Correlation: "provider-0", DestinationProviderID: "provider-destination", Purpose: "provider-api-key"}},
	})
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryBundleV1() error = %v", err)
	}
	return bundle
}

func protectedRecoveryHTTPFixtureReceipt(t *testing.T, request domainregistry.ProtectedRecoveryRequestV1, bundle []byte) []byte {
	t.Helper()
	receipt, err := domainregistry.MarshalProtectedRecoveryReceiptV1(domainregistry.ProtectedRecoveryReceiptV1{
		Schema: domainregistry.ProtectedRecoveryReceiptSchemaV1, ProtocolVersion: 1,
		RequestDigest: request.RequestDigest, ManifestDigest: request.ManifestDigest,
		BundleDigest: domainregistry.ProtectedRecoveryBundleDigest(bundle), OperationID: request.OperationID,
		SessionNonce: request.SessionNonce, ExpiresAt: request.ExpiresAt,
		Entries:           []domainregistry.ProtectedRecoveryReceiptEntryV1{{Correlation: "provider-0", DestinationProviderID: "provider-destination", Status: "applied"}},
		AuthenticationTag: bytes.Repeat([]byte{4}, 32),
	})
	if err != nil {
		t.Fatalf("MarshalProtectedRecoveryReceiptV1() error = %v", err)
	}
	return receipt
}

func assertProtectedRecoveryHTTPRedacted(t *testing.T, response *httptest.ResponseRecorder, forbidden ...string) {
	t.Helper()
	public := response.Body.String() + fmt.Sprint(response.Header())
	for _, value := range forbidden {
		if strings.Contains(public, value) {
			t.Fatalf("private response contains forbidden value %q: %s", value, public)
		}
	}
}
