package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestProviderRegistryHTTPConnectConsumesCredentialWithoutEchoingSecretOrReference(t *testing.T) {
	t.Parallel()

	secretMarker := []byte("synthetic-provider-http-secret-marker")
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	credentialRef := "cred_" + strings.Repeat("B", 43)
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	providerIncarnation := "inc_" + strings.Repeat("b", 43)
	service := &recordingProviderRegistryHTTPService{
		result: domainregistry.Provider{
			ID: "provider-alpha", Kind: "openai-compatible",
			Endpoint: "https://provider.invalid/v1", Models: []string{"model-alpha"},
			MediaModels: []string{}, SelectedModel: "model-alpha", SelectedRoutes: []string{"primary"},
			CredentialRef: credentialRef, CredentialPurpose: "provider-api-key",
			Revision: 1, Generation: 1, Incarnation: providerIncarnation,
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	body := `{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"` + registryIncarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"provider":{"id":"provider-alpha","kind":"openai-compatible","endpoint":"https://provider.invalid/v1","proxy":"","models":["model-alpha"],"mediaModels":[],"selectedModel":"model-alpha","selectedMediaModel":"","selectedRoutes":["primary"]},"credential":{"kind":"set","purpose":"provider-api-key","valueBase64":"` + encodedMarker + `"}}`
	request := httptest.NewRequest(http.MethodPost, "/v1/provider-registry", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !bytes.Equal(service.receivedCredential, secretMarker) {
		t.Fatalf("service received credential = %q, want synthetic marker", service.receivedCredential)
	}
	publicBytes := append([]byte(nil), recorder.Body.Bytes()...)
	for key, values := range recorder.Header() {
		publicBytes = append(publicBytes, key...)
		publicBytes = append(publicBytes, strings.Join(values, ",")...)
	}
	for _, forbidden := range [][]byte{
		secretMarker,
		[]byte(encodedMarker),
		[]byte(credentialRef),
		[]byte("credentialRef"),
		[]byte("provider registry: persistence failure"),
	} {
		if bytes.Contains(publicBytes, forbidden) {
			t.Fatalf("public response contains forbidden value %q: %s", forbidden, publicBytes)
		}
	}
}

func TestProviderRegistryHTTPPortableManifestExportAndImportAreStrictAndKeyFree(t *testing.T) {
	t.Parallel()

	manifest := []byte(`{"schema":"analytix.provider-portable-manifest/v1","providers":[{"correlation":"provider-0","kind":"openai-compatible","endpoint":"https://provider.invalid/v1","models":["model-alpha"],"mediaModels":[],"routes":[],"intent":"reentry_required"}],"accounts":[]}`)
	service := &recordingProviderRegistryHTTPService{
		portableManifest: manifest,
		portableImportResult: providerregistryapp.PortableImportResult{
			ProviderCount: 1, ReentryRequired: 1,
			Entries: []providerregistryapp.PortableImportEntryResult{{
				Correlation: "provider-0", DestinationProviderID: "provider-destination", Status: domainregistry.PortableManifestIntentReentryRequired,
			}},
		},
	}
	handler := ProviderRegistryHandlers{Service: service}

	exported := httptest.NewRecorder()
	handler.Handle(exported, httptest.NewRequest(http.MethodGet, ProviderRegistryPortableManifestPathV1, nil))
	if exported.Code != http.StatusOK ||
		exported.Body.String() != `{"schemaVersion":1,"manifestJson":`+mustProviderRegistryJSONString(t, string(manifest))+"}\n" {
		t.Fatalf("portable export = (%d, %s)", exported.Code, exported.Body.String())
	}
	assertProviderRegistryHTTPKeyFree(t, exported)
	if !bytes.Equal(service.portableExportReceived, manifest) {
		t.Fatalf("portable export service result changed: got %q", service.portableExportReceived)
	}

	imported := httptest.NewRecorder()
	handler.Handle(imported, httptest.NewRequest(http.MethodPost, ProviderRegistryPortableManifestPathV1,
		strings.NewReader(`{"schemaVersion":1,"manifestJson":`+mustProviderRegistryJSONString(t, string(manifest))+`}`)))
	if imported.Code != http.StatusOK ||
		imported.Body.String() != `{"schemaVersion":1,"providerCount":1,"accountCount":0,"reentryRequired":1,"entries":[{"correlation":"provider-0","destinationProviderId":"provider-destination","status":"reentry_required"}]}`+"\n" {
		t.Fatalf("portable import = (%d, %s)", imported.Code, imported.Body.String())
	}
	assertProviderRegistryHTTPKeyFree(t, imported)
	if !bytes.Equal(service.portableImportReceived, manifest) {
		t.Fatalf("portable import body = %q, want canonical manifest", service.portableImportReceived)
	}

	multiManifest := []byte(`{"schema":"analytix.provider-portable-manifest/v1","providers":[{"correlation":"provider-0","kind":"openai-compatible","endpoint":"https://a.provider.invalid/v1","models":[],"mediaModels":[],"routes":[],"intent":"reentry_required"},{"correlation":"provider-1","kind":"openai-compatible","endpoint":"https://b.provider.invalid/v1","models":[],"mediaModels":[],"routes":[],"intent":"reentry_required"}],"accounts":[]}`)
	duplicateDestinationService := &recordingProviderRegistryHTTPService{
		portableImportResult: providerregistryapp.PortableImportResult{
			ProviderCount: 2, ReentryRequired: 2,
			Entries: []providerregistryapp.PortableImportEntryResult{
				{Correlation: "provider-0", DestinationProviderID: "provider-destination", Status: domainregistry.PortableManifestIntentReentryRequired},
				{Correlation: "provider-1", DestinationProviderID: "provider-destination", Status: domainregistry.PortableManifestIntentReentryRequired},
			},
		},
	}
	duplicateDestinationHandler := ProviderRegistryHandlers{Service: duplicateDestinationService}
	duplicateDestinationResponse := httptest.NewRecorder()
	duplicateDestinationHandler.Handle(duplicateDestinationResponse, httptest.NewRequest(http.MethodPost, ProviderRegistryPortableManifestPathV1,
		strings.NewReader(`{"schemaVersion":1,"manifestJson":`+mustProviderRegistryJSONString(t, string(multiManifest))+`}`)))
	if duplicateDestinationResponse.Code != http.StatusInternalServerError ||
		!strings.Contains(duplicateDestinationResponse.Body.String(), `"verification_failure"`) {
		t.Fatalf("duplicate destination Provider IDs = (%d, %s), want verification failure", duplicateDestinationResponse.Code, duplicateDestinationResponse.Body.String())
	}

	for name, body := range map[string]string{
		"duplicate wrapper key":    `{"schemaVersion":1,"schemaVersion":1,"manifestJson":` + mustProviderRegistryJSONString(t, string(manifest)) + `}`,
		"unknown wrapper field":    `{"schemaVersion":1,"manifestJson":` + mustProviderRegistryJSONString(t, string(manifest)) + `,"credentialRef":"cred_` + strings.Repeat("x", 43) + `"}`,
		"duplicate manifest key":   `{"schemaVersion":1,"manifestJson":"{\"schema\":\"analytix.provider-portable-manifest/v1\",\"schema\":\"analytix.provider-portable-manifest/v1\",\"providers\":[],\"accounts\":[]}"}`,
		"legacy manifest":          `{"schemaVersion":1,"manifestJson":"{\"schema\":\"analytix.provider-portable-manifest/v0\",\"providers\":[],\"accounts\":[]}"}`,
		"path-bearing manifest":    `{"schemaVersion":1,"manifestJson":"{\"schema\":\"analytix.provider-portable-manifest/v1\",\"providers\":[{\"correlation\":\"provider-0\",\"kind\":\"openai-compatible\",\"endpoint\":\"https://provider.invalid/v1\",\"models\":[\"../secret\"],\"mediaModels\":[],\"routes\":[],\"intent\":\"reentry_required\"}],\"accounts\":[]}"}`,
		"secret canary field":      `{"schemaVersion":1,"manifestJson":"{\"schema\":\"analytix.provider-portable-manifest/v1\",\"providers\":[{\"correlation\":\"provider-0\",\"kind\":\"openai-compatible\",\"endpoint\":\"https://provider.invalid/v1\",\"models\":[],\"mediaModels\":[],\"routes\":[],\"intent\":\"reentry_required\",\"credentialRef\":\"synthetic-marker\"}],\"accounts\":[]}"}`,
		"noncanonical correlation": `{"schemaVersion":1,"manifestJson":` + mustProviderRegistryJSONString(t, string(bytes.Replace(manifest, []byte(`"correlation":"provider-0"`), []byte(`"correlation":"alpha"`), 1))) + `}`,
	} {
		before := append([]byte(nil), service.portableImportReceived...)
		beforeCall := service.lastCall
		rejected := httptest.NewRecorder()
		handler.Handle(rejected, httptest.NewRequest(http.MethodPost, ProviderRegistryPortableManifestPathV1, strings.NewReader(body)))
		if rejected.Code != http.StatusBadRequest || service.lastCall != beforeCall ||
			!bytes.Equal(service.portableImportReceived, before) {
			t.Fatalf("%s = (%d, %s), service body=%q", name, rejected.Code, rejected.Body.String(), service.portableImportReceived)
		}
		assertProviderRegistryHTTPKeyFree(t, rejected)
	}

	emptyService := &recordingProviderRegistryHTTPService{
		portableManifest: []byte(`{"schema":"analytix.provider-portable-manifest/v1","providers":[],"accounts":[]}`),
	}
	emptyHandler := ProviderRegistryHandlers{Service: emptyService}
	emptyImport := httptest.NewRecorder()
	emptyManifest := string(emptyService.portableManifest)
	emptyHandler.Handle(emptyImport, httptest.NewRequest(http.MethodPost, ProviderRegistryPortableManifestPathV1,
		strings.NewReader(`{"schemaVersion":1,"manifestJson":`+mustProviderRegistryJSONString(t, emptyManifest)+`}`)))
	if emptyImport.Code != http.StatusOK ||
		emptyImport.Body.String() != `{"schemaVersion":1,"providerCount":0,"accountCount":0,"reentryRequired":0,"entries":[]}`+"\n" {
		t.Fatalf("empty portable import = (%d, %s)", emptyImport.Code, emptyImport.Body.String())
	}
}

func mustProviderRegistryJSONString(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(encoded)
}

func TestProviderRegistryHTTPPreparesLegacyMigrationRecoveryWithoutEchoingPlaintext(t *testing.T) {
	t.Parallel()

	secretMarker := []byte("synthetic-legacy-migration-http-secret-marker")
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	sourceLocator := "current:analytix-settings.json"
	sourceSnapshot := []byte(`{"provider":{"apiKey":"synthetic-http-source"}}`)
	expectedCleanedSourceSHA256 := strings.Repeat("b", 64)
	activeCredentialLocator := sourceLocator + ":provider.apiKey"
	recoveryCredentialRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43))
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationResult: providerregistryapp.LegacyMigrationRecoveryResult{
			Status:                             providerregistryapp.LegacyMigrationRecoveryStatusVerified,
			MigrationID:                        "migration-settings-alpha",
			RecoveryCredentialRef:              recoveryCredentialRef,
			SafeToProceedWithProviderMigration: true,
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	body, sourceSHA256 := providerRegistryHTTPLegacyMigrationPrepareBody(
		registryIncarnation, "migration-settings-alpha", sourceLocator, sourceSnapshot,
		expectedCleanedSourceSHA256, "provider-alpha", encodedMarker,
		[]string{activeCredentialLocator}, "",
	)
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		"/v1/provider-registry/_private/legacy-migration-recovery/prepare",
		strings.NewReader(body),
	))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if service.lastCall != "prepare legacy migration recovery" ||
		service.legacyMigrationCandidate.MigrationID != "migration-settings-alpha" ||
		service.legacyMigrationCandidate.SourceLocator != sourceLocator ||
		service.legacyMigrationCandidate.SourceSHA256 != sourceSHA256 ||
		service.legacyMigrationCandidate.ExpectedCleanedSourceSHA256 != expectedCleanedSourceSHA256 ||
		len(service.legacyMigrationCandidate.ActiveCredentialLocators) != 1 ||
		service.legacyMigrationCandidate.ActiveCredentialLocators[0] != activeCredentialLocator ||
		service.legacyMigrationCandidate.CredentialPurpose != "provider-api-key" ||
		service.legacyMigrationCandidate.Provider.ID != "provider-alpha" ||
		service.expected.RegistryRevision != 0 || service.expected.RegistryIncarnation != registryIncarnation ||
		service.expected.ProviderRevision != 0 || service.expected.ProviderGeneration != 0 ||
		service.expected.ProviderIncarnation != "" || service.expected.ProviderCredentialPurpose != "" {
		t.Fatalf("service candidate = %#v; expected = %#v", service.legacyMigrationCandidate, service.expected)
	}
	if !bytes.Equal(service.receivedCredential, secretMarker) {
		t.Fatalf("service received credential = %q, want synthetic marker", service.receivedCredential)
	}
	wantBody := `{"schemaVersion":1,"status":"VERIFIED_RECOVERY","migrationId":"migration-settings-alpha","recoveryCredentialRef":"` + string(recoveryCredentialRef) + `","safeToProceedWithProviderMigration":true}` + "\n"
	if recorder.Body.String() != wantBody {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), wantBody)
	}
	publicBytes := append([]byte(nil), recorder.Body.Bytes()...)
	for key, values := range recorder.Header() {
		publicBytes = append(publicBytes, key...)
		publicBytes = append(publicBytes, strings.Join(values, ",")...)
	}
	for _, forbidden := range [][]byte{
		secretMarker,
		[]byte(encodedMarker),
		[]byte(sourceSHA256),
		[]byte("https://provider.invalid/v1"),
		[]byte("raw-provider-body"),
		[]byte("provider registry: persistence failure at /private/secret-path"),
	} {
		if bytes.Contains(publicBytes, forbidden) {
			t.Fatalf("private response contains forbidden value %q: %s", forbidden, publicBytes)
		}
	}
}

func TestProviderRegistryHTTPRejectsLegacyMigrationPrepareWithoutProtectedSourceDescriptor(t *testing.T) {
	t.Parallel()

	secretMarker := []byte("synthetic-legacy-migration-missing-source-marker")
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationResult: providerregistryapp.LegacyMigrationRecoveryResult{
			Status:                             providerregistryapp.LegacyMigrationRecoveryStatusVerified,
			MigrationID:                        "migration-settings-missing-source",
			RecoveryCredentialRef:              secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43)),
			SafeToProceedWithProviderMigration: true,
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	body := `{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"` + registryIncarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"migrationId":"migration-settings-missing-source","sourceSHA256":"` + strings.Repeat("a", 64) + `","provider":` + providerRegistryHTTPProviderInputJSON("provider-alpha") + `,"credential":{"purpose":"provider-api-key","valueBase64":"` + encodedMarker + `"}}`
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		providerRegistryPrivateLegacyMigrationPreparePathV1,
		strings.NewReader(body),
	))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if service.lastCall != "" {
		t.Fatalf("service call = %q, want none", service.lastCall)
	}
}

func TestProviderRegistryHTTPReplaysCommittedRetainedLegacyMigrationRecoveryWithoutRequestingCommit(t *testing.T) {
	t.Parallel()

	secretMarker := []byte("synthetic-legacy-migration-retained-replay-marker")
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	sourceLocator := "current:analytix-settings.json"
	sourceSnapshot := []byte(`{"provider":{"apiKey":"synthetic-http-replay-source"}}`)
	expectedCleanedSourceSHA256 := strings.Repeat("b", 64)
	recoveryCredentialRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43))
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationResult: providerregistryapp.LegacyMigrationRecoveryResult{
			Status:                             providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained,
			MigrationID:                        "migration-settings-alpha",
			RecoveryCredentialRef:              recoveryCredentialRef,
			SafeToProceedWithProviderMigration: false,
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	body, sourceSHA256 := providerRegistryHTTPLegacyMigrationPrepareBody(
		registryIncarnation, "migration-settings-alpha", sourceLocator, sourceSnapshot,
		expectedCleanedSourceSHA256, "provider-alpha", encodedMarker,
		[]string{sourceLocator + ":provider.apiKey"}, "",
	)
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		providerRegistryPrivateLegacyMigrationPreparePathV1,
		strings.NewReader(body),
	))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if service.lastCall != "prepare legacy migration recovery" ||
		service.legacyMigrationCandidate.MigrationID != "migration-settings-alpha" ||
		service.legacyMigrationCandidate.SourceSHA256 != sourceSHA256 ||
		!bytes.Equal(service.receivedCredential, secretMarker) {
		t.Fatalf("service candidate = %#v", service.legacyMigrationCandidate)
	}
	wantBody := `{"schemaVersion":1,"status":"PROVIDER_COMMITTED_RECOVERY_RETAINED","migrationId":"migration-settings-alpha","recoveryCredentialRef":"` + string(recoveryCredentialRef) + `","safeToProceedWithProviderMigration":false}` + "\n"
	if recorder.Body.String() != wantBody {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), wantBody)
	}
	for _, forbidden := range [][]byte{
		secretMarker,
		[]byte(encodedMarker),
		[]byte(sourceSHA256),
		[]byte("https://provider.invalid/v1"),
		[]byte("raw-provider-body"),
	} {
		if bytes.Contains(recorder.Body.Bytes(), forbidden) {
			t.Fatalf("private response contains forbidden value %q: %s", forbidden, recorder.Body.Bytes())
		}
	}
}

func TestProviderRegistryHTTPPreparesLegacyMigrationRecoveryWithProtectedShadowCredentialsWithoutEchoingSecrets(t *testing.T) {
	t.Parallel()

	activeSecret := []byte("synthetic-active-http-credential-not-a-real-key")
	shadowRuntimeSecret := []byte("synthetic-shadow-runtime-http-credential-not-a-real-key")
	shadowLegacySecret := []byte("synthetic-shadow-legacy-http-credential-not-a-real-key")
	activeEncoded := base64.StdEncoding.EncodeToString(activeSecret)
	shadowRuntimeEncoded := base64.StdEncoding.EncodeToString(shadowRuntimeSecret)
	shadowLegacyEncoded := base64.StdEncoding.EncodeToString(shadowLegacySecret)
	sourceLocator := "current:analytix-settings.json"
	activeLocators := []string{
		sourceLocator + ":agents.kun.apiKey",
		sourceLocator + ":provider.providers[0].apiKey",
	}
	shadowRuntimeLocators := []string{
		sourceLocator + ":provider.apiKey",
		sourceLocator + ":runtime.apiKey",
	}
	shadowLegacyLocators := []string{
		sourceLocator + ":deepseek.apiKey",
	}
	sourceSnapshot := []byte(`{"provider":{"apiKey":"synthetic-http-shadow-source"}}`)
	expectedCleanedSourceSHA256 := strings.Repeat("b", 64)
	recoveryCredentialRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43))
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationResult: providerregistryapp.LegacyMigrationRecoveryResult{
			Status:                             providerregistryapp.LegacyMigrationRecoveryStatusVerified,
			MigrationID:                        "migration-settings-shadow-credentials",
			RecoveryCredentialRef:              recoveryCredentialRef,
			SafeToProceedWithProviderMigration: true,
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	rollbackArtifacts := `[{"schemaVersion":1,"credentialLocators":["` +
		strings.Join(shadowRuntimeLocators, `","`) + `"],"credential":{"valueBase64":"` +
		shadowRuntimeEncoded + `"}},{"schemaVersion":1,"credentialLocators":["` +
		strings.Join(shadowLegacyLocators, `","`) + `"],"credential":{"valueBase64":"` +
		shadowLegacyEncoded + `"}}]`
	body, sourceSHA256 := providerRegistryHTTPLegacyMigrationPrepareBody(
		registryIncarnation, "migration-settings-shadow-credentials", sourceLocator, sourceSnapshot,
		expectedCleanedSourceSHA256, "provider-alpha", activeEncoded, activeLocators, rollbackArtifacts,
	)
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		"/v1/provider-registry/_private/legacy-migration-recovery/prepare",
		strings.NewReader(body),
	))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	candidate := service.legacyMigrationCandidate
	if service.lastCall != "prepare legacy migration recovery" ||
		len(candidate.ActiveCredentialLocators) != len(activeLocators) ||
		len(candidate.RollbackCredentialArtifacts) != 2 {
		t.Fatalf("service candidate = %#v", candidate)
	}
	for index, locator := range activeLocators {
		if candidate.ActiveCredentialLocators[index] != locator {
			t.Fatalf("active locator %d = %q, want %q", index, candidate.ActiveCredentialLocators[index], locator)
		}
	}
	for index, locator := range shadowRuntimeLocators {
		if candidate.RollbackCredentialArtifacts[0].Locators[index] != locator {
			t.Fatalf("shadow runtime locator %d = %q, want %q", index, candidate.RollbackCredentialArtifacts[0].Locators[index], locator)
		}
	}
	if len(candidate.RollbackCredentialArtifacts[1].Locators) != 1 ||
		candidate.RollbackCredentialArtifacts[1].Locators[0] != shadowLegacyLocators[0] ||
		!bytes.Equal(service.receivedCredential, activeSecret) ||
		!bytes.Equal(candidate.RollbackCredentialArtifacts[0].Credential, shadowRuntimeSecret) ||
		!bytes.Equal(candidate.RollbackCredentialArtifacts[1].Credential, shadowLegacySecret) {
		t.Fatalf("service candidate = %#v", candidate)
	}
	wantBody := `{"schemaVersion":1,"status":"VERIFIED_RECOVERY","migrationId":"migration-settings-shadow-credentials","recoveryCredentialRef":"` + string(recoveryCredentialRef) + `","safeToProceedWithProviderMigration":true}` + "\n"
	if recorder.Body.String() != wantBody {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), wantBody)
	}
	publicBytes := append([]byte(nil), recorder.Body.Bytes()...)
	for key, values := range recorder.Header() {
		publicBytes = append(publicBytes, key...)
		publicBytes = append(publicBytes, strings.Join(values, ",")...)
	}
	for _, forbidden := range [][]byte{
		activeSecret,
		shadowRuntimeSecret,
		shadowLegacySecret,
		[]byte(activeEncoded),
		[]byte(shadowRuntimeEncoded),
		[]byte(shadowLegacyEncoded),
		[]byte(activeLocators[0]),
		[]byte(activeLocators[1]),
		[]byte(shadowRuntimeLocators[0]),
		[]byte(shadowRuntimeLocators[1]),
		[]byte(shadowLegacyLocators[0]),
		[]byte(sourceSHA256),
		[]byte("https://provider.invalid/v1"),
		[]byte("raw-provider-body"),
		[]byte("provider registry: persistence failure at /private/secret-path"),
	} {
		if bytes.Contains(publicBytes, forbidden) {
			t.Fatalf("private response contains forbidden value %q: %s", forbidden, publicBytes)
		}
	}
}

func TestProviderRegistryHTTPCommitsVerifiedLegacyMigrationRecoveryWithoutReturningSecrets(t *testing.T) {
	t.Parallel()

	migrationID := "migration-settings-alpha"
	sourceSHA256 := strings.Repeat("a", 64)
	recoveryCredentialRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43))
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	providerIncarnation := "inc_" + strings.Repeat("b", 43)
	providerCredentialRef := "cred_" + strings.Repeat("C", 43)
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationCommitResult: providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult{
			Status:      providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained,
			MigrationID: migrationID,
			Provider: domainregistry.Provider{
				ID: "provider-alpha", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
				Models: []string{"model-alpha"}, MediaModels: []string{}, SelectedModel: "model-alpha",
				SelectedRoutes: []string{"primary"}, CredentialRef: providerCredentialRef,
				CredentialPurpose: "provider-api-key", Revision: 1, Generation: 1,
				Incarnation: providerIncarnation,
			},
			RecoveryCredentialRef:       recoveryCredentialRef,
			SafeToRemoveLegacyPlaintext: true,
		},
	}
	body := `{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"` + registryIncarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"expectedSelectedProviderID":"","migrationId":"` + migrationID + `","sourceSHA256":"` + sourceSHA256 + `","recoveryCredentialRef":"` + string(recoveryCredentialRef) + `","confirmation":"` + providerregistryapp.LegacyMigrationCommitConfirmation + `"}`
	handler := ProviderRegistryHandlers{Service: service}
	recorder := httptest.NewRecorder()

	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		"/v1/provider-registry/_private/legacy-migration-recovery/commit",
		strings.NewReader(body),
	))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	command := service.legacyMigrationCommitCommand
	if service.lastCall != "commit verified legacy migration recovery" ||
		command.Expected.RegistryRevision != 0 || command.Expected.RegistryIncarnation != registryIncarnation ||
		command.Expected.ProviderRevision != 0 || command.Expected.ProviderGeneration != 0 ||
		command.Expected.ProviderIncarnation != "" || command.Expected.ProviderCredentialPurpose != "" ||
		command.ExpectedSelectedProviderID != "" || command.MigrationID != migrationID ||
		command.SourceSHA256 != sourceSHA256 || command.RecoveryCredentialRef != recoveryCredentialRef ||
		command.Confirmation != providerregistryapp.LegacyMigrationCommitConfirmation {
		t.Fatalf("commit command = %#v", command)
	}
	wantBody := `{"schemaVersion":1,"status":"PROVIDER_COMMITTED_RECOVERY_RETAINED","migrationId":"` + migrationID + `","safeToRemoveLegacyPlaintext":true,"provider":{"id":"provider-alpha","kind":"openai-compatible","endpoint":"https://provider.invalid/v1","models":["model-alpha"],"mediaModels":[],"selectedModel":"model-alpha","selectedRoutes":["primary"],"credentialConfigured":true,"credentialPurpose":"provider-api-key","revision":"1","generation":"1","incarnation":"` + providerIncarnation + `","tombstone":false}}` + "\n"
	if recorder.Body.String() != wantBody {
		t.Fatalf("body = %q, want %q", recorder.Body.String(), wantBody)
	}
	publicBytes := append([]byte(nil), recorder.Body.Bytes()...)
	for key, values := range recorder.Header() {
		publicBytes = append(publicBytes, key...)
		publicBytes = append(publicBytes, strings.Join(values, ",")...)
	}
	for _, forbidden := range []string{
		string(recoveryCredentialRef), providerCredentialRef, "credentialRef", sourceSHA256,
		"valueBase64", "ciphertext", "nonce", "masterKey", "secret", "rawBody", "envelope",
	} {
		if strings.Contains(string(publicBytes), forbidden) {
			t.Fatalf("commit response contains forbidden value %q: %s", forbidden, publicBytes)
		}
	}
}

func TestProviderRegistryHTTPRejectsInvalidLegacyMigrationCommitRequestsWithoutServiceCalls(t *testing.T) {
	t.Parallel()

	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	migrationID := "migration-settings-alpha"
	sourceSHA256 := strings.Repeat("a", 64)
	recoveryCredentialRef := "cred_" + strings.Repeat("R", 43)
	validBody := `{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"` + registryIncarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"expectedSelectedProviderID":"","migrationId":"` + migrationID + `","sourceSHA256":"` + sourceSHA256 + `","recoveryCredentialRef":"` + recoveryCredentialRef + `","confirmation":"` + providerregistryapp.LegacyMigrationCommitConfirmation + `"}`
	tests := []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{name: "malformed", method: http.MethodPost, body: strings.TrimSuffix(validBody, "}"), wantStatus: http.StatusBadRequest},
		{name: "unknown", method: http.MethodPost, body: strings.TrimSuffix(validBody, "}") + `,"unknown":true}`, wantStatus: http.StatusBadRequest},
		{name: "trailing", method: http.MethodPost, body: validBody + `{}`, wantStatus: http.StatusBadRequest},
		{name: "wrong schema", method: http.MethodPost, body: strings.Replace(validBody, `"schemaVersion":1`, `"schemaVersion":2`, 1), wantStatus: http.StatusBadRequest},
		{name: "leading zero CAS", method: http.MethodPost, body: strings.Replace(validBody, `"registryRevision":"0"`, `"registryRevision":"00"`, 1), wantStatus: http.StatusBadRequest},
		{name: "number CAS", method: http.MethodPost, body: strings.Replace(validBody, `"providerGeneration":"0"`, `"providerGeneration":0`, 1), wantStatus: http.StatusBadRequest},
		{name: "missing selection fence", method: http.MethodPost, body: strings.Replace(validBody, `,"expectedSelectedProviderID":""`, "", 1), wantStatus: http.StatusBadRequest},
		{name: "invalid selection fence", method: http.MethodPost, body: strings.Replace(validBody, `"expectedSelectedProviderID":""`, `"expectedSelectedProviderID":"provider/alpha"`, 1), wantStatus: http.StatusBadRequest},
		{name: "migration", method: http.MethodPost, body: strings.Replace(validBody, migrationID, "migration/settings", 1), wantStatus: http.StatusBadRequest},
		{name: "source", method: http.MethodPost, body: strings.Replace(validBody, sourceSHA256, strings.ToUpper(sourceSHA256), 1), wantStatus: http.StatusBadRequest},
		{name: "reference", method: http.MethodPost, body: strings.Replace(validBody, recoveryCredentialRef, "invalid-ref", 1), wantStatus: http.StatusBadRequest},
		{name: "confirmation", method: http.MethodPost, body: strings.Replace(validBody, providerregistryapp.LegacyMigrationCommitConfirmation, "COMMIT", 1), wantStatus: http.StatusBadRequest},
		{name: "method", method: http.MethodPut, body: validBody, wantStatus: http.StatusMethodNotAllowed},
		{name: "oversized", method: http.MethodPost, body: strings.Repeat("x", providerRegistryMaxBodyBytes+1), wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingProviderRegistryHTTPService{}
			handler := ProviderRegistryHandlers{Service: service}
			recorder := httptest.NewRecorder()
			handler.Handle(recorder, httptest.NewRequest(
				test.method,
				providerRegistryPrivateLegacyMigrationCommitPathV1,
				strings.NewReader(test.body),
			))
			if recorder.Code != test.wantStatus || service.lastCall != "" ||
				!strings.Contains(recorder.Body.String(), `"schemaVersion":1`) {
				t.Fatalf("response = (%d, %s), call=%q; want status %d", recorder.Code, recorder.Body.String(), service.lastCall, test.wantStatus)
			}
			for _, forbidden := range []string{sourceSHA256, recoveryCredentialRef, "raw-provider-body", "secret-path"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("invalid response leaks %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestProviderRegistryHTTPRejectsUnsafeLegacyMigrationCommitResultsWithStableRedactedFailures(t *testing.T) {
	t.Parallel()

	migrationID := "migration-settings-alpha"
	sourceSHA256 := strings.Repeat("a", 64)
	recoveryCredentialRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43))
	providerCredentialRef := "cred_" + strings.Repeat("C", 43)
	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	body := `{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"` + registryIncarnation + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"expectedSelectedProviderID":"","migrationId":"` + migrationID + `","sourceSHA256":"` + sourceSHA256 + `","recoveryCredentialRef":"` + string(recoveryCredentialRef) + `","confirmation":"` + providerregistryapp.LegacyMigrationCommitConfirmation + `"}`
	validResult := func() providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult {
		return providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult{
			Status:      providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained,
			MigrationID: migrationID,
			Provider: domainregistry.Provider{
				ID: "provider-alpha", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
				Models: []string{"model-alpha"}, MediaModels: []string{}, SelectedModel: "model-alpha",
				SelectedRoutes: []string{"primary"}, CredentialRef: providerCredentialRef,
				CredentialPurpose: "provider-api-key", Revision: 1, Generation: 1,
				Incarnation: "inc_" + strings.Repeat("b", 43),
			},
			RecoveryCredentialRef:       recoveryCredentialRef,
			SafeToRemoveLegacyPlaintext: true,
		}
	}
	tests := []struct {
		name   string
		mutate func(*providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult)
		err    error
	}{
		{name: "wrong status", mutate: func(result *providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult) {
			result.Status = providerregistryapp.LegacyMigrationRecoveryStatusVerified
		}},
		{name: "wrong migration", mutate: func(result *providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult) {
			result.MigrationID = "migration-settings-other"
		}},
		{name: "wrong recovery reference", mutate: func(result *providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult) {
			result.RecoveryCredentialRef = "cred_" + secretstoreport.CredentialRef(strings.Repeat("Z", 43))
		}},
		{name: "unsafe flag", mutate: func(result *providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult) {
			result.SafeToRemoveLegacyPlaintext = false
		}},
		{name: "readable provider handle", mutate: func(result *providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult) {
			result.Provider.CredentialRef = string(recoveryCredentialRef)
		}},
		{name: "invalid provider", mutate: func(result *providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult) {
			result.Provider.CredentialRef = "readable-secret-handle"
		}},
		{name: "raw internal error", err: errors.New("raw-provider-body at /private/secret-path")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validResult()
			if test.mutate != nil {
				test.mutate(&result)
			}
			service := &recordingProviderRegistryHTTPService{
				legacyMigrationCommitResult: result,
				err:                         test.err,
			}
			recorder := httptest.NewRecorder()
			ProviderRegistryHandlers{Service: service}.Handle(recorder, httptest.NewRequest(
				http.MethodPost,
				providerRegistryPrivateLegacyMigrationCommitPathV1,
				strings.NewReader(body),
			))
			if recorder.Code != http.StatusInternalServerError || service.lastCall != "commit verified legacy migration recovery" ||
				!strings.Contains(recorder.Body.String(), `"code":"verification_failure"`) {
				t.Fatalf("response = (%d, %s), call=%q", recorder.Code, recorder.Body.String(), service.lastCall)
			}
			for _, forbidden := range []string{
				migrationID, sourceSHA256, string(recoveryCredentialRef), providerCredentialRef,
				"credentialRef", "raw-provider-body", "secret-path",
			} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("failure response leaks %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestProviderRegistryHTTPRejectsInvalidLegacyMigrationRecoveryRequestsWithoutServiceCalls(t *testing.T) {
	t.Parallel()

	secretMarker := []byte("synthetic-invalid-legacy-migration-marker")
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	sourceLocator := "current:analytix-settings.json"
	validBody, sourceSHA256 := providerRegistryHTTPLegacyMigrationPrepareBody(
		"inc_"+strings.Repeat("a", 43), "migration-settings-alpha", sourceLocator,
		[]byte(`{"provider":{"apiKey":"synthetic-invalid-source"}}`), strings.Repeat("b", 64),
		"provider-alpha", encodedMarker, []string{sourceLocator + ":provider.apiKey"}, "",
	)
	tests := []struct {
		name             string
		method           string
		path             string
		body             string
		wantStatus       int
		wantDecodedClear bool
	}{
		{name: "malformed", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.TrimSuffix(validBody, "}"), wantStatus: http.StatusBadRequest},
		{name: "unknown", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.TrimSuffix(validBody, "}") + `,"unknown":true}`, wantStatus: http.StatusBadRequest},
		{name: "trailing", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: validBody + `{}`, wantStatus: http.StatusBadRequest},
		{name: "wrong schema", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, `"schemaVersion":1`, `"schemaVersion":2`, 1), wantStatus: http.StatusBadRequest, wantDecodedClear: true},
		{name: "leading zero CAS", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, `"registryRevision":"0"`, `"registryRevision":"00"`, 1), wantStatus: http.StatusBadRequest, wantDecodedClear: true},
		{name: "number CAS", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, `"registryRevision":"0"`, `"registryRevision":0`, 1), wantStatus: http.StatusBadRequest},
		{name: "migration", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, `"migration-settings-alpha"`, `"migration/settings/alpha"`, 1), wantStatus: http.StatusBadRequest, wantDecodedClear: true},
		{name: "source", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, sourceSHA256, strings.ToUpper(sourceSHA256), 1), wantStatus: http.StatusBadRequest, wantDecodedClear: true},
		{name: "purpose", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, `"purpose":"provider-api-key"`, `"purpose":"Provider API Key"`, 1), wantStatus: http.StatusBadRequest, wantDecodedClear: true},
		{name: "provider", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, `"endpoint":"https://provider.invalid/v1"`, `"endpoint":"provider.invalid"`, 1), wantStatus: http.StatusBadRequest, wantDecodedClear: true},
		{name: "noncanonical base64 padding", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, encodedMarker, strings.TrimRight(encodedMarker, "="), 1), wantStatus: http.StatusBadRequest},
		{name: "noncanonical base64 escape", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Replace(validBody, encodedMarker, `\u0063\u0033\u006c\u0075`+encodedMarker[4:], 1), wantStatus: http.StatusBadRequest, wantDecodedClear: true},
		{name: "query", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1 + "?public=true", body: validBody, wantStatus: http.StatusBadRequest},
		{name: "method", method: http.MethodPut, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: validBody, wantStatus: http.StatusMethodNotAllowed},
		{name: "oversized", method: http.MethodPost, path: providerRegistryPrivateLegacyMigrationPreparePathV1, body: strings.Repeat("x", providerRegistryMaxBodyBytes+1), wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingProviderRegistryHTTPService{}
			var cleared []byte
			handler := ProviderRegistryHandlers{
				Service: service,
				observeCredentialBufferCleared: func(value []byte) {
					cleared = append([]byte(nil), value...)
				},
			}
			recorder := httptest.NewRecorder()
			handler.Handle(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if service.lastCall != "" {
				t.Fatalf("service call = %q, want none", service.lastCall)
			}
			if test.wantDecodedClear {
				if len(cleared) == 0 || !bytes.Equal(cleared, make([]byte, len(cleared))) {
					t.Fatalf("decoded credential was not cleared: %v", cleared)
				}
			} else if len(cleared) != 0 {
				t.Fatalf("unexpected decoded credential observation: %v", cleared)
			}
			for _, forbidden := range []string{string(secretMarker), encodedMarker, "credentialRef", "raw-provider-body"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("rejection leaks %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}

	t.Run("decoded credential size", func(t *testing.T) {
		oversizedCredential := bytes.Repeat([]byte{0x51}, providerRegistryMaxCredentialBytes+1)
		encoded := base64.StdEncoding.EncodeToString(oversizedCredential)
		clearProviderRegistryBytes(oversizedCredential)
		body := strings.Replace(validBody, encodedMarker, encoded, 1)
		service := &recordingProviderRegistryHTTPService{}
		var cleared []byte
		handler := ProviderRegistryHandlers{
			Service: service,
			observeCredentialBufferCleared: func(value []byte) {
				cleared = append([]byte(nil), value...)
			},
		}
		recorder := httptest.NewRecorder()
		handler.Handle(recorder, httptest.NewRequest(
			http.MethodPost, providerRegistryPrivateLegacyMigrationPreparePathV1, strings.NewReader(body),
		))
		if recorder.Code != http.StatusBadRequest || service.lastCall != "" ||
			len(cleared) != providerRegistryMaxCredentialBytes+1 ||
			!bytes.Equal(cleared, make([]byte, len(cleared))) {
			t.Fatalf("oversized credential response = (%d, %s), call=%q cleared=%d", recorder.Code, recorder.Body.String(), service.lastCall, len(cleared))
		}
	})
}

func TestProviderRegistryHTTPRejectsMalformedOrAmbiguousLegacyMigrationRecoveryV4WithoutServiceCalls(t *testing.T) {
	t.Parallel()

	activeSecret := []byte("synthetic-v2-active-invalid-request-marker")
	shadowSecret := []byte("synthetic-v2-shadow-invalid-request-marker")
	activeEncoded := base64.StdEncoding.EncodeToString(activeSecret)
	shadowEncoded := base64.StdEncoding.EncodeToString(shadowSecret)
	sourceLocator := "current:analytix-settings.json"
	activeLocator := sourceLocator + ":provider.apiKey"
	shadowLocator := sourceLocator + ":runtime.apiKey"
	rollbackArtifacts := `[{"schemaVersion":1,"credentialLocators":["` + shadowLocator +
		`"],"credential":{"valueBase64":"` + shadowEncoded + `"}}]`
	validBody, _ := providerRegistryHTTPLegacyMigrationPrepareBody(
		"inc_"+strings.Repeat("a", 43), "migration-settings-v2-invalid", sourceLocator,
		[]byte(`{"provider":{"apiKey":"synthetic-v4-invalid-source"}}`), strings.Repeat("b", 64),
		"provider-alpha", activeEncoded, []string{activeLocator}, rollbackArtifacts,
	)
	tests := []struct {
		name string
		body string
	}{
		{
			name: "artifacts without active locators",
			body: strings.Replace(validBody, `,"activeCredentialLocators":["`+activeLocator+`"]`, "", 1),
		},
		{
			name: "empty active locators with artifacts",
			body: strings.Replace(validBody, `"activeCredentialLocators":["`+activeLocator+`"]`, `"activeCredentialLocators":[]`, 1),
		},
		{
			name: "artifact schema",
			body: strings.Replace(validBody, `"rollbackCredentialArtifacts":[{"schemaVersion":1`, `"rollbackCredentialArtifacts":[{"schemaVersion":2`, 1),
		},
		{
			name: "artifact locators empty",
			body: strings.Replace(validBody, `"credentialLocators":["`+shadowLocator+`"]`, `"credentialLocators":[]`, 1),
		},
		{
			name: "malformed logical locator",
			body: strings.Replace(validBody, shadowLocator, "/private/analytix-settings.json:runtime.apiKey", 1),
		},
		{
			name: "cross-source logical locator",
			body: strings.Replace(validBody, shadowLocator, "compatibility:00:kun-settings.json:runtime.apiKey", 1),
		},
		{
			name: "duplicate locator",
			body: strings.Replace(validBody, shadowLocator, activeLocator, 1),
		},
		{
			name: "duplicate credential value",
			body: strings.Replace(validBody, shadowEncoded, activeEncoded, 1),
		},
		{
			name: "omitted artifact credential",
			body: strings.Replace(validBody, `,"credential":{"valueBase64":"`+shadowEncoded+`"}`, "", 1),
		},
		{
			name: "unknown artifact field",
			body: strings.Replace(validBody, `"credentialLocators":[`, `"unknown":true,"credentialLocators":[`, 1),
		},
		{
			name: "unknown artifact credential field",
			body: strings.Replace(validBody, `"credential":{"valueBase64":"`+shadowEncoded+`"}`, `"credential":{"unknown":true,"valueBase64":"`+shadowEncoded+`"}`, 1),
		},
		{
			name: "duplicate active locator field",
			body: strings.TrimSuffix(validBody, "}") + `,"activeCredentialLocators":["` + activeLocator + `"]}`,
		},
		{
			name: "duplicate artifact base64 field",
			body: strings.Replace(validBody, `"valueBase64":"`+shadowEncoded+`"`, `"valueBase64":"`+shadowEncoded+`","valueBase64":"`+shadowEncoded+`"`, 1),
		},
		{
			name: "noncanonical artifact base64",
			body: strings.Replace(validBody, shadowEncoded, shadowEncoded+"=", 1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingProviderRegistryHTTPService{}
			handler := ProviderRegistryHandlers{Service: service}
			recorder := httptest.NewRecorder()
			handler.Handle(recorder, httptest.NewRequest(
				http.MethodPost,
				providerRegistryPrivateLegacyMigrationPreparePathV1,
				strings.NewReader(test.body),
			))
			if recorder.Code != http.StatusBadRequest || service.lastCall != "" {
				t.Fatalf("response = (%d, %s), service call = %q", recorder.Code, recorder.Body.String(), service.lastCall)
			}
			wantBody := `{"schemaVersion":1,"error":{"code":"invalid_request","message":"The provider registry request was rejected."}}` + "\n"
			if recorder.Body.String() != wantBody {
				t.Fatalf("body = %q, want %q", recorder.Body.String(), wantBody)
			}
			for _, forbidden := range []string{
				string(activeSecret), string(shadowSecret), activeEncoded, shadowEncoded,
				activeLocator, shadowLocator, "private/analytix-settings.json",
			} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("rejection leaks %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestProviderRegistryHTTPClearsEveryLegacyMigrationRecoveryV5CredentialBufferOnServiceFailure(t *testing.T) {
	t.Parallel()

	activeSecret := []byte("synthetic-v2-active-clear-marker")
	shadowOne := []byte("synthetic-v2-shadow-one-clear-marker")
	shadowTwo := []byte("synthetic-v2-shadow-two-clear-marker")
	activeEncoded := base64.StdEncoding.EncodeToString(activeSecret)
	shadowOneEncoded := base64.StdEncoding.EncodeToString(shadowOne)
	shadowTwoEncoded := base64.StdEncoding.EncodeToString(shadowTwo)
	sourceLocator := "current:analytix-settings.json"
	locators := []string{
		sourceLocator + ":provider.apiKey",
		sourceLocator + ":runtime.apiKey",
		sourceLocator + ":deepseek.apiKey",
	}
	rollbackArtifacts := `[{"schemaVersion":1,"credentialLocators":["` + locators[1] +
		`"],"credential":{"valueBase64":"` + shadowOneEncoded +
		`"}},{"schemaVersion":1,"credentialLocators":["` + locators[2] +
		`"],"credential":{"valueBase64":"` + shadowTwoEncoded + `"}}]`
	body, _ := providerRegistryHTTPLegacyMigrationPrepareBody(
		"inc_"+strings.Repeat("a", 43), "migration-settings-v2-clear", sourceLocator,
		[]byte(`{"provider":{"apiKey":"synthetic-v4-clear-source"}}`), strings.Repeat("b", 64),
		"provider-alpha", activeEncoded, []string{locators[0]}, rollbackArtifacts,
	)
	service := &recordingProviderRegistryHTTPService{err: errors.New("unsafe provider body at /private/secret-path")}
	var decodedClears [][]byte
	var encodedClears [][]byte
	handler := ProviderRegistryHandlers{
		Service: service,
		observeCredentialBufferCleared: func(value []byte) {
			decodedClears = append(decodedClears, append([]byte(nil), value...))
		},
		observeEncodedCredentialBufferCleared: func(value []byte) {
			encodedClears = append(encodedClears, append([]byte(nil), value...))
		},
	}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost,
		providerRegistryPrivateLegacyMigrationPreparePathV1,
		strings.NewReader(body),
	))
	if recorder.Code != http.StatusInternalServerError || service.lastCall != "prepare legacy migration recovery" {
		t.Fatalf("response = (%d, %s), service call = %q", recorder.Code, recorder.Body.String(), service.lastCall)
	}
	if len(decodedClears) != 4 || len(encodedClears) != 4 {
		t.Fatalf("clear observations decoded=%d encoded=%d, want 4 each", len(decodedClears), len(encodedClears))
	}
	for _, cleared := range append(decodedClears, encodedClears...) {
		if len(cleared) == 0 || !bytes.Equal(cleared, make([]byte, len(cleared))) {
			t.Fatalf("credential buffer was not cleared: %v", cleared)
		}
	}
	for _, forbidden := range []string{
		string(activeSecret), string(shadowOne), string(shadowTwo),
		activeEncoded, shadowOneEncoded, shadowTwoEncoded,
		locators[0], locators[1], locators[2], "unsafe provider body", "private/secret-path",
	} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("service failure leaks %q: %s", forbidden, recorder.Body.String())
		}
	}
}

func TestProviderRegistryHTTPLegacyMigrationRecoveryAbandonIsExplicitMinimalAndRedacted(t *testing.T) {
	t.Parallel()

	migrationID := "migration-settings-alpha"
	sourceSHA256 := strings.Repeat("a", 64)
	recoveryCredentialRef := "cred_" + strings.Repeat("R", 43)
	body := `{"schemaVersion":1,"migrationId":"` + migrationID + `","sourceSHA256":"` + sourceSHA256 + `","recoveryCredentialRef":"` + recoveryCredentialRef + `","confirmation":"` + providerregistryapp.LegacyMigrationAbandonConfirmation + `"}`
	service := &recordingProviderRegistryHTTPService{}
	handler := ProviderRegistryHandlers{Service: service}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(http.MethodPost, providerRegistryPrivateLegacyMigrationAbandonPathV1, strings.NewReader(body)))
	if recorder.Code != http.StatusOK || recorder.Body.String() != `{"schemaVersion":1,"status":"COMPLETED"}`+"\n" {
		t.Fatalf("response = (%d, %q)", recorder.Code, recorder.Body.String())
	}
	if service.lastCall != "abandon legacy migration recovery" ||
		service.abandonCommand.MigrationID != migrationID || service.abandonCommand.SourceSHA256 != sourceSHA256 ||
		string(service.abandonCommand.RecoveryCredentialRef) != recoveryCredentialRef ||
		service.abandonCommand.Confirmation != providerregistryapp.LegacyMigrationAbandonConfirmation {
		t.Fatalf("abandon command = %#v", service.abandonCommand)
	}
	for _, forbidden := range []string{migrationID, sourceSHA256, recoveryCredentialRef, "credentialRef"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("abandon response leaks %q: %s", forbidden, recorder.Body.String())
		}
	}

	for _, test := range []struct {
		name       string
		method     string
		body       string
		wantStatus int
	}{
		{name: "migration", method: http.MethodPost, body: strings.Replace(body, migrationID, "migration/settings", 1), wantStatus: http.StatusBadRequest},
		{name: "source", method: http.MethodPost, body: strings.Replace(body, sourceSHA256, strings.ToUpper(sourceSHA256), 1), wantStatus: http.StatusBadRequest},
		{name: "reference", method: http.MethodPost, body: strings.Replace(body, recoveryCredentialRef, "invalid-ref", 1), wantStatus: http.StatusBadRequest},
		{name: "confirmation", method: http.MethodPost, body: strings.Replace(body, providerregistryapp.LegacyMigrationAbandonConfirmation, "ABANDON", 1), wantStatus: http.StatusBadRequest},
		{name: "unknown", method: http.MethodPost, body: strings.TrimSuffix(body, "}") + `,"unknown":true}`, wantStatus: http.StatusBadRequest},
		{name: "trailing", method: http.MethodPost, body: body + `{}`, wantStatus: http.StatusBadRequest},
		{name: "method", method: http.MethodDelete, body: body, wantStatus: http.StatusMethodNotAllowed},
		{name: "oversized", method: http.MethodPost, body: strings.Repeat("x", providerRegistryMaxBodyBytes+1), wantStatus: http.StatusRequestEntityTooLarge},
	} {
		t.Run("invalid/"+test.name, func(t *testing.T) {
			invalidService := &recordingProviderRegistryHTTPService{}
			invalid := httptest.NewRecorder()
			ProviderRegistryHandlers{Service: invalidService}.Handle(invalid, httptest.NewRequest(
				test.method, providerRegistryPrivateLegacyMigrationAbandonPathV1, strings.NewReader(test.body),
			))
			if invalid.Code != test.wantStatus || invalidService.lastCall != "" {
				t.Fatalf("invalid abandon response = (%d, %s), call=%q", invalid.Code, invalid.Body.String(), invalidService.lastCall)
			}
			for _, forbidden := range []string{sourceSHA256, recoveryCredentialRef, "credentialRef", "raw-provider-body"} {
				if strings.Contains(invalid.Body.String(), forbidden) {
					t.Fatalf("invalid abandon response leaks %q: %s", forbidden, invalid.Body.String())
				}
			}
		})
	}

	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid", err: registryport.ErrInvalidRequest, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "conflict", err: registryport.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "not found", err: registryport.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "persistence", err: registryport.ErrPersistence, wantStatus: http.StatusServiceUnavailable, wantCode: "persistence_failure"},
		{name: "verification", err: registryport.ErrVerification, wantStatus: http.StatusInternalServerError, wantCode: "verification_failure"},
		{name: "unsafe", err: errors.New("raw-provider-body at /private/secret-path"), wantStatus: http.StatusInternalServerError, wantCode: "verification_failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			failed := httptest.NewRecorder()
			failedService := &recordingProviderRegistryHTTPService{err: test.err}
			ProviderRegistryHandlers{Service: failedService}.Handle(failed, httptest.NewRequest(
				http.MethodPost, providerRegistryPrivateLegacyMigrationAbandonPathV1, strings.NewReader(body),
			))
			if failed.Code != test.wantStatus || !strings.Contains(failed.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("response = (%d, %s)", failed.Code, failed.Body.String())
			}
			for _, forbidden := range []string{sourceSHA256, recoveryCredentialRef, "raw-provider-body", "secret-path"} {
				if strings.Contains(failed.Body.String(), forbidden) {
					t.Fatalf("failure leaks %q: %s", forbidden, failed.Body.String())
				}
			}
		})
	}
}

func TestProviderRegistryHTTPRejectsUnsafeLegacyMigrationRecoveryServiceResultsWithoutReferenceLeakage(t *testing.T) {
	t.Parallel()

	secretMarker := []byte("synthetic-unsafe-result-marker")
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	recoveryCredentialRef := "cred_" + strings.Repeat("U", 43)
	sourceLocator := "current:analytix-settings.json"
	body, _ := providerRegistryHTTPLegacyMigrationPrepareBody(
		"inc_"+strings.Repeat("a", 43), "migration-settings-alpha", sourceLocator,
		[]byte(`{"provider":{"apiKey":"synthetic-unsafe-result-source"}}`), strings.Repeat("b", 64),
		"provider-alpha", encodedMarker, []string{sourceLocator + ":provider.apiKey"}, "",
	)
	tests := []providerregistryapp.LegacyMigrationRecoveryResult{
		{Status: "UNSAFE", MigrationID: "migration-settings-alpha", RecoveryCredentialRef: secretstoreport.CredentialRef(recoveryCredentialRef), SafeToProceedWithProviderMigration: true},
		{Status: "UNSAFE", MigrationID: "migration-settings-alpha", RecoveryCredentialRef: secretstoreport.CredentialRef(recoveryCredentialRef), SafeToProceedWithProviderMigration: false},
		{Status: providerregistryapp.LegacyMigrationRecoveryStatusVerified, MigrationID: "migration-settings-other", RecoveryCredentialRef: secretstoreport.CredentialRef(recoveryCredentialRef), SafeToProceedWithProviderMigration: true},
		{Status: providerregistryapp.LegacyMigrationRecoveryStatusVerified, MigrationID: "migration-settings-alpha", RecoveryCredentialRef: "invalid-ref", SafeToProceedWithProviderMigration: true},
		{Status: providerregistryapp.LegacyMigrationRecoveryStatusVerified, MigrationID: "migration-settings-alpha", RecoveryCredentialRef: secretstoreport.CredentialRef(recoveryCredentialRef), SafeToProceedWithProviderMigration: false},
		{Status: providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained, MigrationID: "migration-settings-alpha", RecoveryCredentialRef: secretstoreport.CredentialRef(recoveryCredentialRef), SafeToProceedWithProviderMigration: true},
	}
	for index, result := range tests {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			service := &recordingProviderRegistryHTTPService{legacyMigrationResult: result}
			ProviderRegistryHandlers{Service: service}.Handle(recorder, httptest.NewRequest(
				http.MethodPost, providerRegistryPrivateLegacyMigrationPreparePathV1, strings.NewReader(body),
			))
			if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), `"code":"verification_failure"`) {
				t.Fatalf("response = (%d, %s)", recorder.Code, recorder.Body.String())
			}
			for _, forbidden := range []string{recoveryCredentialRef, encodedMarker, string(secretMarker), "recoveryCredentialRef"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("failure leaks %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}

	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid", err: registryport.ErrInvalidRequest, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "conflict", err: registryport.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "not found", err: registryport.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "persistence", err: registryport.ErrPersistence, wantStatus: http.StatusServiceUnavailable, wantCode: "persistence_failure"},
		{name: "verification", err: registryport.ErrVerification, wantStatus: http.StatusInternalServerError, wantCode: "verification_failure"},
		{name: "unsafe", err: errors.New("raw-provider-body at /private/secret-path"), wantStatus: http.StatusInternalServerError, wantCode: "verification_failure"},
	} {
		t.Run("service error/"+test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			service := &recordingProviderRegistryHTTPService{
				err: test.err,
				legacyMigrationResult: providerregistryapp.LegacyMigrationRecoveryResult{
					RecoveryCredentialRef: secretstoreport.CredentialRef(recoveryCredentialRef),
				},
			}
			ProviderRegistryHandlers{Service: service}.Handle(recorder, httptest.NewRequest(
				http.MethodPost, providerRegistryPrivateLegacyMigrationPreparePathV1, strings.NewReader(body),
			))
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("response = (%d, %s)", recorder.Code, recorder.Body.String())
			}
			for _, forbidden := range []string{recoveryCredentialRef, encodedMarker, string(secretMarker), "recoveryCredentialRef", "raw-provider-body", "secret-path"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("failure leaks %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestProviderRegistryHTTPRejectedSecretBodiesClearDecodedCredentialBuffers(t *testing.T) {
	t.Parallel()

	secretMarker := []byte("synthetic-rejected-provider-secret-marker")
	encodedMarker := base64.StdEncoding.EncodeToString(secretMarker)
	credentialRef := "cred_" + strings.Repeat("R", 43)
	credential := `{"kind":"set","purpose":"provider-api-key","valueBase64":"` + encodedMarker + `"}`
	connectExpected := `{"registryRevision":"0","registryIncarnation":"inc_` + strings.Repeat("a", 43) + `","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""}`
	existingExpected := providerRegistryHTTPExpectedJSON("4", "7", "3", "provider-api-key")
	provider := providerRegistryHTTPProviderInputJSON("provider-alpha")
	operations := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name: "connect", method: http.MethodPost, path: "/v1/provider-registry",
			body: `{"schemaVersion":1,"expected":` + connectExpected + `,"provider":` + provider + `,"credential":` + credential + `}`,
		},
		{
			name: "update", method: http.MethodPatch, path: "/v1/provider-registry/providers/provider-alpha",
			body: `{"schemaVersion":1,"expected":` + existingExpected + `,"provider":` + provider + `,"credential":` + credential + `}`,
		},
		{
			name: "credential replace", method: http.MethodPut, path: "/v1/provider-registry/providers/provider-alpha/credential",
			body: `{"schemaVersion":1,"expected":` + existingExpected + `,"credential":` + credential + `}`,
		},
	}
	rejections := []struct {
		name   string
		mutate func(string) string
	}{
		{name: "wrong schema", mutate: func(body string) string {
			return strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":2`, 1)
		}},
		{name: "unknown field", mutate: func(body string) string {
			return strings.TrimSuffix(body, "}") + `,"unexpected":true}`
		}},
		{name: "trailing document", mutate: func(body string) string { return body + `{}` }},
	}

	for _, operation := range operations {
		for _, rejection := range rejections {
			t.Run(operation.name+"/"+rejection.name, func(t *testing.T) {
				service := &recordingProviderRegistryHTTPService{}
				var clearedBuffer []byte
				handler := ProviderRegistryHandlers{
					Service: service,
					observeCredentialBufferCleared: func(value []byte) {
						clearedBuffer = append([]byte(nil), value...)
					},
				}
				recorder := httptest.NewRecorder()
				handler.Handle(recorder, httptest.NewRequest(
					operation.method,
					operation.path,
					strings.NewReader(rejection.mutate(operation.body)),
				))

				if recorder.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
				}
				if service.lastCall != "" {
					t.Fatalf("service mutation = %q, want none", service.lastCall)
				}
				if len(clearedBuffer) != len(secretMarker) {
					t.Fatalf("cleared buffer length = %d, want %d", len(clearedBuffer), len(secretMarker))
				}
				if !bytes.Equal(clearedBuffer, make([]byte, len(secretMarker))) {
					t.Fatalf("decoded credential buffer was not zeroed: %v", clearedBuffer)
				}
				wantBody := `{"schemaVersion":1,"error":{"code":"invalid_request","message":"The provider registry request was rejected."}}` + "\n"
				if recorder.Body.String() != wantBody {
					t.Fatalf("body = %q, want %q", recorder.Body.String(), wantBody)
				}
				publicBytes := append([]byte(nil), recorder.Body.Bytes()...)
				for key, values := range recorder.Header() {
					publicBytes = append(publicBytes, key...)
					publicBytes = append(publicBytes, strings.Join(values, ",")...)
				}
				for _, forbidden := range [][]byte{
					secretMarker,
					[]byte(encodedMarker),
					[]byte(credentialRef),
					[]byte("credentialRef"),
					[]byte("raw-provider-body"),
					[]byte("provider registry: persistence failure at /private/secret-path"),
				} {
					if bytes.Contains(publicBytes, forbidden) {
						t.Fatalf("public rejection contains forbidden value %q: %s", forbidden, publicBytes)
					}
				}
			})
		}
	}
}

func TestProviderRegistryHTTPListAndGetAreDeterministicAndKeyFree(t *testing.T) {
	t.Parallel()

	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	providerAlpha := providerRegistryHTTPTestProvider("provider-alpha", "cred_"+strings.Repeat("A", 43))
	providerBeta := providerRegistryHTTPTestProvider("provider-beta", "cred_"+strings.Repeat("B", 43))
	service := &recordingProviderRegistryHTTPService{snapshot: domainregistry.Registry{
		Version: domainregistry.FormatVersion, Revision: 9, Incarnation: registryIncarnation,
		SelectedProviderID: "provider-alpha",
		Providers: map[string]domainregistry.Provider{
			"provider-beta": providerBeta, "provider-alpha": providerAlpha,
		},
		Transactions: map[string]domainregistry.Transaction{},
	}}
	handler := ProviderRegistryHandlers{Service: service}

	list := httptest.NewRecorder()
	handler.Handle(list, httptest.NewRequest(http.MethodGet, "/v1/provider-registry", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d; body = %s", list.Code, list.Body.String())
	}
	if strings.Index(list.Body.String(), `"id":"provider-alpha"`) >= strings.Index(list.Body.String(), `"id":"provider-beta"`) {
		t.Fatalf("list Providers are not sorted: %s", list.Body.String())
	}
	assertProviderRegistryHTTPKeyFree(t, list)
	if !strings.Contains(list.Body.String(), `"credentialConfigured":true`) ||
		!strings.Contains(list.Body.String(), `"revision":"7"`) ||
		!strings.Contains(list.Body.String(), `"generation":"3"`) {
		t.Fatalf("list response is missing decimal/key-free metadata: %s", list.Body.String())
	}

	get := httptest.NewRecorder()
	handler.Handle(get, httptest.NewRequest(http.MethodGet, "/v1/provider-registry/providers/provider-alpha", nil))
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), `"providers"`) {
		t.Fatalf("get response = (%d, %s)", get.Code, get.Body.String())
	}
	assertProviderRegistryHTTPKeyFree(t, get)
}

func TestProviderRegistryHTTPRoutesAllMutationsWithExplicitCAS(t *testing.T) {
	t.Parallel()

	service := &recordingProviderRegistryHTTPService{
		result:   providerRegistryHTTPTestProvider("provider-alpha", "cred_"+strings.Repeat("B", 43)),
		snapshot: providerRegistryHTTPTestRegistry(),
	}
	handler := ProviderRegistryHandlers{Service: service}
	expected := providerRegistryHTTPExpectedJSON("4", "7", "3", "provider-api-key")
	provider := providerRegistryHTTPProviderInputJSON("provider-alpha")
	marker := []byte("synthetic-replacement-marker")
	encoded := base64.StdEncoding.EncodeToString(marker)
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantCall   string
		wantKind   secretstoreport.MutationKind
		wantSecret []byte
	}{
		{name: "update omitted keeps", method: http.MethodPatch, path: "/v1/provider-registry/providers/provider-alpha", body: `{"schemaVersion":1,"expected":` + expected + `,"provider":` + provider + `}`, wantCall: "update", wantKind: secretstoreport.MutationKeep},
		{name: "update explicit keep", method: http.MethodPatch, path: "/v1/provider-registry/providers/provider-alpha", body: `{"schemaVersion":1,"expected":` + expected + `,"provider":` + provider + `,"credential":{"kind":"keep"}}`, wantCall: "update", wantKind: secretstoreport.MutationKeep},
		{name: "select", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/select", body: `{"schemaVersion":1,"expected":` + expected + `}`, wantCall: "select"},
		{name: "disconnect", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/disconnect", body: `{"schemaVersion":1,"expected":` + expected + `}`, wantCall: "disconnect"},
		{name: "delete", method: http.MethodDelete, path: "/v1/provider-registry/providers/provider-alpha", body: `{"schemaVersion":1,"expected":` + expected + `}`, wantCall: "delete"},
		{name: "credential replace", method: http.MethodPut, path: "/v1/provider-registry/providers/provider-alpha/credential", body: `{"schemaVersion":1,"expected":` + expected + `,"credential":{"kind":"set","purpose":"provider-api-key","valueBase64":"` + encoded + `"}}`, wantCall: "credential", wantKind: secretstoreport.MutationSet, wantSecret: marker},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service.lastCall = ""
			service.receivedCredential = nil
			recorder := httptest.NewRecorder()
			handler.Handle(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d; body = %s", recorder.Code, recorder.Body.String())
			}
			if service.lastCall != test.wantCall || service.providerID != "provider-alpha" {
				t.Fatalf("service call = (%q, %q), want (%q, provider-alpha)", service.lastCall, service.providerID, test.wantCall)
			}
			if test.wantKind != secretstoreport.MutationInvalid && service.mutationKind != test.wantKind {
				t.Fatalf("mutation kind = %v, want %v", service.mutationKind, test.wantKind)
			}
			if test.wantSecret != nil && !bytes.Equal(service.receivedCredential, test.wantSecret) {
				t.Fatalf("received credential = %q", service.receivedCredential)
			}
			if service.expected.RegistryRevision != 4 || service.expected.ProviderRevision != 7 ||
				service.expected.ProviderGeneration != 3 || service.expected.ProviderCredentialPurpose != "provider-api-key" {
				t.Fatalf("expected CAS = %#v", service.expected)
			}
			assertProviderRegistryHTTPKeyFree(t, recorder)
		})
	}

	recoverRecorder := httptest.NewRecorder()
	handler.Handle(recoverRecorder, httptest.NewRequest(http.MethodPost, "/v1/provider-registry/recover", strings.NewReader(`{"schemaVersion":1}`)))
	if recoverRecorder.Code != http.StatusOK || !service.recovered || !strings.Contains(recoverRecorder.Body.String(), `"recovered":true`) {
		t.Fatalf("recover response = (%d, %q, %s)", recoverRecorder.Code, service.lastCall, recoverRecorder.Body.String())
	}
}

func TestProviderRegistryHTTPRejectsNonCanonicalPathsBodiesAndDecimals(t *testing.T) {
	t.Parallel()

	handler := ProviderRegistryHandlers{Service: &recordingProviderRegistryHTTPService{snapshot: providerRegistryHTTPTestRegistry()}}
	expected := providerRegistryHTTPExpectedJSON("4", "7", "3", "provider-api-key")
	validBody := `{"schemaVersion":1,"expected":` + expected + `}`
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "query", method: http.MethodGet, path: "/v1/provider-registry?x=1", wantStatus: http.StatusBadRequest},
		{name: "trailing slash", method: http.MethodGet, path: "/v1/provider-registry/", wantStatus: http.StatusBadRequest},
		{name: "unknown suffix", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/select/extra", body: validBody, wantStatus: http.StatusBadRequest},
		{name: "method", method: http.MethodPut, path: "/v1/provider-registry", body: validBody, wantStatus: http.StatusMethodNotAllowed},
		{name: "number CAS", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/select", body: strings.Replace(validBody, `"registryRevision":"4"`, `"registryRevision":4`, 1), wantStatus: http.StatusBadRequest},
		{name: "leading zero CAS", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/select", body: strings.Replace(validBody, `"registryRevision":"4"`, `"registryRevision":"04"`, 1), wantStatus: http.StatusBadRequest},
		{name: "overflow CAS", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/select", body: strings.Replace(validBody, `"registryRevision":"4"`, `"registryRevision":"18446744073709551616"`, 1), wantStatus: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/select", body: strings.TrimSuffix(validBody, "}") + `,"unexpected":true}`, wantStatus: http.StatusBadRequest},
		{name: "trailing document", method: http.MethodPost, path: "/v1/provider-registry/providers/provider-alpha/select", body: validBody + `{}`, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.Handle(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"schemaVersion":1`) {
				t.Fatalf("response = (%d, %s), want status %d", recorder.Code, recorder.Body.String(), test.wantStatus)
			}
		})
	}

	large := httptest.NewRecorder()
	handler.Handle(large, httptest.NewRequest(http.MethodPost, "/v1/provider-registry/recover", strings.NewReader(strings.Repeat("x", providerRegistryMaxBodyBytes+1))))
	if large.Code != http.StatusRequestEntityTooLarge || strings.Count(large.Body.String(), `"schemaVersion"`) != 1 {
		t.Fatalf("large body response = (%d, %s)", large.Code, large.Body.String())
	}
}

func TestProviderRegistryHTTPProjectsStableRedactedServiceErrors(t *testing.T) {
	t.Parallel()

	rawInternalError := errors.New("provider registry: persistence failure at /private/secret-path with raw-provider-body")
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid", err: registryport.ErrInvalidRequest, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "conflict", err: registryport.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "not found", err: registryport.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "persistence", err: registryport.ErrPersistence, wantStatus: http.StatusServiceUnavailable, wantCode: "persistence_failure"},
		{name: "verification", err: registryport.ErrVerification, wantStatus: http.StatusInternalServerError, wantCode: "verification_failure"},
		{name: "unknown raw", err: rawInternalError, wantStatus: http.StatusInternalServerError, wantCode: "verification_failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &recordingProviderRegistryHTTPService{err: test.err}
			handler := ProviderRegistryHandlers{Service: service}
			recorder := httptest.NewRecorder()
			handler.Handle(recorder, httptest.NewRequest(http.MethodGet, "/v1/provider-registry", nil))
			if recorder.Code != test.wantStatus || !strings.Contains(recorder.Body.String(), `"code":"`+test.wantCode+`"`) {
				t.Fatalf("response = (%d, %s)", recorder.Code, recorder.Body.String())
			}
			for _, forbidden := range []string{"secret-path", "raw-provider-body", rawInternalError.Error()} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("response leaks %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestProviderRegistryHTTPPrivateLegacyMigrationRollbackAndFinalizationLifecycle(t *testing.T) {
	t.Parallel()

	migrationID := "migration-http-explicit-rollback"
	recoveryRef := "cred_" + strings.Repeat("R", 43)
	sourceSnapshot := []byte(`{"provider":{"apiKey":"synthetic-http-rollback"}}`)
	sourceDigestBytes := sha256.Sum256(sourceSnapshot)
	sourceSHA256 := hex.EncodeToString(sourceDigestBytes[:])
	cleanedSource := []byte(`{"provider":{"id":"provider-http"}}`)
	cleanedDigestBytes := sha256.Sum256(cleanedSource)
	cleanedSourceSHA256 := hex.EncodeToString(cleanedDigestBytes[:])
	sourceLocator := "current:analytix-settings.json"
	sourcePhysicalIdentitySHA256 := strings.Repeat("e", 64)
	sourceAuthorityChallenge := "lmsa_" + strings.Repeat("A", 43)
	sourceAuthorityPath := "/private/synthetic-analytix-settings.json"
	sourceAuthorityToken := "00000000-0000-4000-8000-000000000001"
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationSourceAuthorityResult: providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeResult{
			Challenge: sourceAuthorityChallenge,
		},
		legacyMigrationRollbackBeginResult: providerregistryapp.BeginLegacyMigrationRollbackResult{
			Status:      providerregistryapp.LegacyMigrationRollbackStatusCleanedSourceRequired,
			MigrationID: migrationID,
		},
	}
	cleared := 0
	handler := ProviderRegistryHandlers{
		Service: service,
		observeCredentialBufferCleared: func(value []byte) {
			if !allProviderRegistryHTTPBytesZero(value) {
				t.Fatal("private rollback material was not cleared")
			}
			cleared++
		},
	}
	beginBody := fmt.Sprintf(
		`{"schemaVersion":1,"migrationId":"%s","sourceLocator":"%s","sourceSHA256":"%s","recoveryCredentialRef":"%s","confirmation":"%s"}`,
		migrationID, sourceLocator, sourceSHA256, recoveryRef, providerregistryapp.LegacyMigrationRollbackConfirmation,
	)
	begin := httptest.NewRecorder()
	handler.Handle(begin, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationRollbackBeginPathV1, strings.NewReader(beginBody),
	))
	if begin.Code != http.StatusOK || service.lastCall != "begin legacy migration rollback" ||
		service.rollbackBeginCommand.MigrationID != migrationID ||
		service.rollbackBeginCommand.SourceSHA256 != sourceSHA256 ||
		string(service.rollbackBeginCommand.RecoveryCredentialRef) != recoveryRef || cleared != 0 {
		t.Fatalf("rollback begin response=(%d,%s) command=%#v cleared=%d",
			begin.Code, begin.Body.String(), service.rollbackBeginCommand, cleared)
	}
	privateBody := begin.Body.String()
	if strings.Contains(privateBody, recoveryRef) || strings.Contains(privateBody, sourceSHA256) ||
		strings.Contains(privateBody, base64.StdEncoding.EncodeToString([]byte("synthetic-http-rollback"))) ||
		strings.Contains(privateBody, sourceLocator) {
		t.Fatalf("rollback begin projected protected material: %s", privateBody)
	}

	service.lastCall = ""
	service.legacyMigrationRollbackBeginResult = providerregistryapp.BeginLegacyMigrationRollbackResult{
		Status:      providerregistryapp.LegacyMigrationRollbackStatusCommittedRecoveryRetained,
		MigrationID: migrationID,
	}
	repeatedBegin := httptest.NewRecorder()
	handler.Handle(repeatedBegin, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationRollbackBeginPathV1, strings.NewReader(beginBody),
	))
	if repeatedBegin.Code != http.StatusOK || service.lastCall != "begin legacy migration rollback" ||
		cleared != 0 || !strings.Contains(repeatedBegin.Body.String(), string(providerregistryapp.LegacyMigrationRollbackStatusCommittedRecoveryRetained)) ||
		strings.Contains(repeatedBegin.Body.String(), sourceSHA256) ||
		strings.Contains(repeatedBegin.Body.String(), base64.StdEncoding.EncodeToString([]byte("synthetic-http-rollback"))) ||
		strings.Contains(repeatedBegin.Body.String(), recoveryRef) {
		t.Fatalf("repeated rollback begin response=(%d,%s) command=%#v cleared=%d",
			repeatedBegin.Code, repeatedBegin.Body.String(), service.rollbackBeginCommand, cleared)
	}

	service.lastCall = ""
	service.legacyMigrationRollbackCommitResult = providerregistryapp.CommitLegacyMigrationRollbackResult{
		Status:      providerregistryapp.LegacyMigrationRollbackStatusCommittedRecoveryRetained,
		MigrationID: migrationID,
	}
	challengeBody := fmt.Sprintf(
		`{"schemaVersion":1,"operation":"%s","migrationId":"%s","sourceLocator":"%s","sourceSHA256":"%s","currentSourceSHA256":"%s","sourcePhysicalIdentitySHA256":"%s","recoveryCredentialRef":"%s","confirmation":"%s"}`,
		providerregistryapp.LegacyMigrationSourceAuthorityOperationRollbackCommit, migrationID,
		sourceLocator, sourceSHA256, cleanedSourceSHA256, sourcePhysicalIdentitySHA256, recoveryRef,
		providerregistryapp.LegacyMigrationSourceAuthorityChallengeConfirmation,
	)
	challenge := httptest.NewRecorder()
	handler.Handle(challenge, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationSourceAuthorityChallengePathV1,
		strings.NewReader(challengeBody),
	))
	if challenge.Code != http.StatusOK ||
		service.lastCall != "issue legacy migration source authority challenge" ||
		service.sourceAuthorityCommand.Operation != providerregistryapp.LegacyMigrationSourceAuthorityOperationRollbackCommit ||
		!strings.Contains(challenge.Body.String(), sourceAuthorityChallenge) ||
		strings.Contains(challenge.Body.String(), sourceAuthorityPath) ||
		strings.Contains(challenge.Body.String(), sourceSHA256) ||
		strings.Contains(challenge.Body.String(), recoveryRef) {
		t.Fatalf("source authority challenge response=(%d,%s) command=%#v",
			challenge.Code, challenge.Body.String(), service.sourceAuthorityCommand)
	}
	commitBody := fmt.Sprintf(
		`{"schemaVersion":1,"expected":%s,"migrationId":"%s","sourceLocator":"%s","sourceSHA256":"%s","verifiedCleanedSourceSHA256":"%s","sourcePhysicalIdentitySHA256":"%s","verifiedCleanedSource":{"valueBase64":"%s"},"sourceAuthority":{"challenge":"%s","sourcePath":"%s","lockOwnerToken":"%s","sourceDevice":"42","sourceInode":"84"},"recoveryCredentialRef":"%s","confirmation":"%s"}`,
		providerRegistryHTTPExpectedJSON("4", "7", "3", "provider-api-key"), migrationID,
		sourceLocator, sourceSHA256, cleanedSourceSHA256, sourcePhysicalIdentitySHA256,
		base64.StdEncoding.EncodeToString(cleanedSource), sourceAuthorityChallenge,
		sourceAuthorityPath, sourceAuthorityToken, recoveryRef,
		providerregistryapp.LegacyMigrationRollbackCommitConfirmation,
	)
	var parsedCommit providerRegistryPrivateLegacyMigrationRollbackCommitRequest
	if err := json.Unmarshal([]byte(commitBody), &parsedCommit); err != nil {
		t.Fatalf("rollback commit request fixture is invalid JSON: %v", err)
	}
	decodedVerifiedCleanedSource, decodedVerifiedCleanedSourceOK := decodeProviderRegistryCanonicalBase64(
		parsedCommit.VerifiedCleanedSource.ValueBase64,
	)
	if _, expectedOK := parsedCommit.Expected.parseExisting(); !expectedOK || !decodedVerifiedCleanedSourceOK ||
		!bytes.Equal(decodedVerifiedCleanedSource, cleanedSource) {
		clearProviderRegistryBytes(decodedVerifiedCleanedSource)
		t.Fatal("rollback commit request fixture failed bounded receipt validation")
	}
	fixtureDigest := sha256.Sum256(decodedVerifiedCleanedSource)
	if parsedCommit.SchemaVersion != 1 ||
		!domainregistry.ValidLegacyMigrationID(parsedCommit.MigrationID) ||
		!validProviderRegistryLegacyMigrationSourceLocator([]byte(parsedCommit.SourceLocator)) ||
		!validProviderRegistryLowerSHA256(parsedCommit.SourceSHA256) ||
		!validProviderRegistryLowerSHA256(parsedCommit.VerifiedCleanedSourceSHA256) ||
		!validProviderRegistryLowerSHA256(parsedCommit.SourcePhysicalIdentitySHA256) ||
		hex.EncodeToString(fixtureDigest[:]) != parsedCommit.VerifiedCleanedSourceSHA256 ||
		secretstoreport.ValidateCredentialRef(secretstoreport.CredentialRef(parsedCommit.RecoveryCredentialRef)) != nil ||
		parsedCommit.Confirmation != providerregistryapp.LegacyMigrationRollbackCommitConfirmation {
		clearProviderRegistryBytes(decodedVerifiedCleanedSource)
		t.Fatal("rollback commit request fixture failed command validation")
	}
	clearProviderRegistryBytes(decodedVerifiedCleanedSource)
	commit := httptest.NewRecorder()
	handler.Handle(commit, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationRollbackCommitPathV1, strings.NewReader(commitBody),
	))
	if commit.Code != http.StatusOK || service.lastCall != "commit legacy migration rollback" ||
		service.rollbackCommitCommand.VerifiedCleanedSourceSHA256 != cleanedSourceSHA256 ||
		strings.Contains(commit.Body.String(), sourceSHA256) || strings.Contains(commit.Body.String(), recoveryRef) {
		t.Fatalf("rollback commit response=(%d,%s) command=%#v",
			commit.Code, commit.Body.String(), service.rollbackCommitCommand)
	}

	service.lastCall = ""
	service.legacyMigrationFinalizeResult = providerregistryapp.FinalizeLegacyMigrationRecoveryResult{
		Status:      providerregistryapp.LegacyMigrationFinalizationStatusCompleted,
		Outcome:     providerregistryapp.LegacyMigrationFinalizationOutcomeRolledBack,
		MigrationID: migrationID,
	}
	service.lastCall = ""
	finalizeChallengeBody := strings.Replace(
		challengeBody,
		providerregistryapp.LegacyMigrationSourceAuthorityOperationRollbackCommit,
		providerregistryapp.LegacyMigrationSourceAuthorityOperationFinalize,
		1,
	)
	finalizeChallenge := httptest.NewRecorder()
	handler.Handle(finalizeChallenge, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationSourceAuthorityChallengePathV1,
		strings.NewReader(finalizeChallengeBody),
	))
	if finalizeChallenge.Code != http.StatusOK ||
		service.sourceAuthorityCommand.Operation != providerregistryapp.LegacyMigrationSourceAuthorityOperationFinalize {
		t.Fatalf("finalize source authority challenge response=(%d,%s) command=%#v",
			finalizeChallenge.Code, finalizeChallenge.Body.String(), service.sourceAuthorityCommand)
	}
	finalizeBody := fmt.Sprintf(
		`{"schemaVersion":1,"migrationId":"%s","sourceLocator":"%s","sourceSHA256":"%s","verifiedSourceSHA256":"%s","sourcePhysicalIdentitySHA256":"%s","verifiedSource":{"valueBase64":"%s"},"sourceAuthority":{"challenge":"%s","sourcePath":"%s","lockOwnerToken":"%s","sourceDevice":"42","sourceInode":"84"},"recoveryCredentialRef":"%s","confirmation":"%s"}`,
		migrationID, sourceLocator, sourceSHA256, cleanedSourceSHA256, sourcePhysicalIdentitySHA256,
		base64.StdEncoding.EncodeToString(cleanedSource), sourceAuthorityChallenge,
		sourceAuthorityPath, sourceAuthorityToken, recoveryRef,
		providerregistryapp.LegacyMigrationFinalizeConfirmation,
	)
	finalized := httptest.NewRecorder()
	handler.Handle(finalized, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationFinalizePathV1, strings.NewReader(finalizeBody),
	))
	if finalized.Code != http.StatusOK || service.lastCall != "finalize legacy migration recovery" ||
		strings.Contains(finalized.Body.String(), sourceSHA256) || strings.Contains(finalized.Body.String(), recoveryRef) ||
		!strings.Contains(finalized.Body.String(), `"outcome":"PROTECTED_RECOVERY_RETAINED"`) {
		t.Fatalf("finalize response=(%d,%s) command=%#v",
			finalized.Code, finalized.Body.String(), service.finalizeCommand)
	}
}

func TestProviderRegistryHTTPPrivateRemigrationAndProtectedDeleteStayKeyFree(t *testing.T) {
	t.Parallel()

	migrationID := "migration-http-retained-action"
	recoveryRef := "cred_" + strings.Repeat("R", 43)
	sourceLocator := "current:analytix-settings.json"
	sourceSHA256 := strings.Repeat("a", 64)
	physicalIdentity := strings.Repeat("e", 64)
	cleanedSource := []byte(`{"provider":{"id":"provider-http-retained"}}`)
	cleanedDigest := sha256.Sum256(cleanedSource)
	cleanedSHA256 := hex.EncodeToString(cleanedDigest[:])
	challenge := "lmsa_" + strings.Repeat("A", 43)
	provider := providerRegistryHTTPTestProvider("provider-http-retained", "cred_"+strings.Repeat("W", 43))
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationRemigrateResult: providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult{
			Status:      providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained,
			MigrationID: migrationID, Provider: provider,
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	expected := fmt.Sprintf(
		`{"registryRevision":"2","registryIncarnation":"inc_%s","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""}`,
		strings.Repeat("a", 43),
	)
	bodyFor := func(confirmation string) string {
		return fmt.Sprintf(
			`{"schemaVersion":1,"expected":%s,"migrationId":"%s","sourceLocator":"%s","sourceSHA256":"%s","verifiedCleanedSourceSHA256":"%s","sourcePhysicalIdentitySHA256":"%s","verifiedCleanedSource":{"valueBase64":"%s"},"sourceAuthority":{"challenge":"%s","sourcePath":"/private/synthetic-settings.json","lockOwnerToken":"00000000-0000-4000-8000-000000000001","sourceDevice":"42","sourceInode":"84"},"recoveryCredentialRef":"%s","confirmation":"%s"}`,
			expected, migrationID, sourceLocator, sourceSHA256, cleanedSHA256, physicalIdentity,
			base64.StdEncoding.EncodeToString(cleanedSource), challenge, recoveryRef, confirmation,
		)
	}

	remigrate := httptest.NewRecorder()
	handler.Handle(remigrate, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationRemigratePathV1,
		strings.NewReader(bodyFor(providerregistryapp.LegacyMigrationRemigrationConfirmation)),
	))
	if remigrate.Code != http.StatusOK ||
		service.lastCall != "remigrate retained legacy migration recovery" ||
		service.remigrateCommand.VerifiedCleanedSourceSHA256 != cleanedSHA256 ||
		!allProviderRegistryHTTPBytesZero(service.remigrateCommand.VerifiedCleanedSource) ||
		!strings.Contains(remigrate.Body.String(), `"credentialConfigured":true`) {
		t.Fatalf("remigration response=(%d,%s) command=%#v", remigrate.Code, remigrate.Body.String(), service.remigrateCommand)
	}
	for _, forbidden := range []string{
		recoveryRef, provider.CredentialRef, sourceSHA256, cleanedSHA256,
		base64.StdEncoding.EncodeToString(cleanedSource), "/private/synthetic-settings.json",
	} {
		if strings.Contains(remigrate.Body.String(), forbidden) {
			t.Fatalf("remigration response projected protected value %q: %s", forbidden, remigrate.Body.String())
		}
	}

	service.lastCall = ""
	service.legacyMigrationProtectedDeleteResult = providerregistryapp.DeleteRetainedLegacyMigrationRecoveryResult{
		Status: providerregistryapp.LegacyMigrationProtectedDeleteStatusCompleted, MigrationID: migrationID,
	}
	deleted := httptest.NewRecorder()
	handler.Handle(deleted, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationProtectedDeletePathV1,
		strings.NewReader(bodyFor(providerregistryapp.LegacyMigrationProtectedDeleteConfirmation)),
	))
	if deleted.Code != http.StatusOK ||
		service.lastCall != "delete retained legacy migration recovery" ||
		service.protectedDeleteCommand.VerifiedCleanedSourceSHA256 != cleanedSHA256 ||
		!strings.Contains(deleted.Body.String(), `"status":"COMPLETED"`) {
		t.Fatalf("protected delete response=(%d,%s) command=%#v", deleted.Code, deleted.Body.String(), service.protectedDeleteCommand)
	}
	for _, forbidden := range []string{recoveryRef, sourceSHA256, cleanedSHA256, "/private/"} {
		if strings.Contains(deleted.Body.String(), forbidden) {
			t.Fatalf("protected delete response projected protected value %q: %s", forbidden, deleted.Body.String())
		}
	}
}

func TestProviderRegistryHTTPPrivateLegacyMigrationInventoryIsBoundedAndKeyFree(t *testing.T) {
	t.Parallel()

	recoveryRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("I", 43))
	expectedCleanedSourceSHA256 := strings.Repeat("b", 64)
	sourcePhysicalIdentitySHA256 := strings.Repeat("e", 64)
	service := &recordingProviderRegistryHTTPService{
		legacyMigrationInventoryResult: providerregistryapp.LegacyMigrationRecoveryInventoryResult{
			Recoveries: []providerregistryapp.LegacyMigrationRecoveryDescriptor{{
				MigrationID: "migration-inventory-http", ProviderID: "provider-inventory-http",
				SourceLocator: "current:analytix-settings.json", SourceSHA256: strings.Repeat("a", 64),
				ExpectedCleanedSourceSHA256:  expectedCleanedSourceSHA256,
				SourcePhysicalIdentitySHA256: sourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        recoveryRef,
				Phase:                        domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained,
				CommitOrder:                  7,
			}},
		},
	}
	handler := ProviderRegistryHandlers{Service: service}
	recorder := httptest.NewRecorder()
	handler.Handle(recorder, httptest.NewRequest(
		http.MethodPost, providerRegistryPrivateLegacyMigrationInventoryPathV1,
		strings.NewReader(`{"schemaVersion":1,"confirmation":"INSPECT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERIES"}`),
	))
	if recorder.Code != http.StatusOK || service.lastCall != "inventory legacy migration recoveries" {
		t.Fatalf("inventory response=(%d,%s) call=%q", recorder.Code, recorder.Body.String(), service.lastCall)
	}
	want := `{"schemaVersion":1,"recoveries":[{"migrationId":"migration-inventory-http","providerId":"provider-inventory-http","sourceLocator":"current:analytix-settings.json","sourceSHA256":"` + strings.Repeat("a", 64) + `","expectedCleanedSourceSHA256":"` + expectedCleanedSourceSHA256 + `","sourcePhysicalIdentitySHA256":"` + sourcePhysicalIdentitySHA256 + `","recoveryCredentialRef":"` + string(recoveryRef) + `","phase":"provider-committed-recovery-retained","commitOrder":"7"}]}` + "\n"
	if recorder.Body.String() != want {
		t.Fatalf("inventory body = %q, want %q", recorder.Body.String(), want)
	}
	for _, forbidden := range []string{
		"sourceSnapshot", "rollbackSource", "credentialArtifacts", "valueBase64", "provider\"", "endpoint",
		"synthetic-secret", "raw-provider-body", "/private/",
	} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("inventory response contains forbidden field/value %q: %s", forbidden, recorder.Body.String())
		}
	}

	for _, body := range []string{
		`{"schemaVersion":1,"confirmation":"wrong"}`,
		`{"schemaVersion":1,"confirmation":"INSPECT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERIES","unexpected":true}`,
	} {
		service.lastCall = ""
		rejected := httptest.NewRecorder()
		handler.Handle(rejected, httptest.NewRequest(
			http.MethodPost, providerRegistryPrivateLegacyMigrationInventoryPathV1, strings.NewReader(body),
		))
		if rejected.Code != http.StatusBadRequest || service.lastCall != "" {
			t.Fatalf("rejected inventory=(%d,%s) call=%q", rejected.Code, rejected.Body.String(), service.lastCall)
		}
	}
}

func allProviderRegistryHTTPBytesZero(value []byte) bool {
	for _, current := range value {
		if current != 0 {
			return false
		}
	}
	return true
}

type recordingProviderRegistryHTTPService struct {
	ProviderRegistryService
	result                               domainregistry.Provider
	snapshot                             domainregistry.Registry
	portableManifest                     []byte
	portableImportResult                 providerregistryapp.PortableImportResult
	portableExportReceived               []byte
	portableImportReceived               []byte
	legacyMigrationResult                providerregistryapp.LegacyMigrationRecoveryResult
	legacyMigrationCommitResult          providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult
	legacyMigrationRollbackBeginResult   providerregistryapp.BeginLegacyMigrationRollbackResult
	legacyMigrationRollbackCommitResult  providerregistryapp.CommitLegacyMigrationRollbackResult
	legacyMigrationRemigrateResult       providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult
	legacyMigrationProtectedDeleteResult providerregistryapp.DeleteRetainedLegacyMigrationRecoveryResult
	legacyMigrationFinalizeResult        providerregistryapp.FinalizeLegacyMigrationRecoveryResult
	legacyMigrationSourceAuthorityResult providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeResult
	legacyMigrationInventoryResult       providerregistryapp.LegacyMigrationRecoveryInventoryResult
	legacyMigrationCandidate             providerregistryapp.LegacyMigrationCandidate
	legacyMigrationCommitCommand         providerregistryapp.CommitVerifiedLegacyMigrationRecoveryCommand
	abandonCommand                       providerregistryapp.AbandonLegacyMigrationRecoveryCommand
	rollbackBeginCommand                 providerregistryapp.BeginLegacyMigrationRollbackCommand
	rollbackCommitCommand                providerregistryapp.CommitLegacyMigrationRollbackCommand
	remigrateCommand                     providerregistryapp.RemigrateRetainedLegacyMigrationRecoveryCommand
	protectedDeleteCommand               providerregistryapp.DeleteRetainedLegacyMigrationRecoveryCommand
	finalizeCommand                      providerregistryapp.FinalizeLegacyMigrationRecoveryCommand
	sourceAuthorityCommand               providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeCommand
	err                                  error
	lastCall                             string
	providerID                           string
	expected                             domainregistry.ExpectedState
	mutationKind                         secretstoreport.MutationKind
	receivedCredential                   []byte
	recovered                            bool
}

func (service *recordingProviderRegistryHTTPService) ExportPortableManifest(context.Context) ([]byte, error) {
	service.lastCall = "export portable manifest"
	service.portableExportReceived = append(service.portableExportReceived[:0], service.portableManifest...)
	return append([]byte(nil), service.portableManifest...), service.err
}

func (service *recordingProviderRegistryHTTPService) ImportPortableManifest(
	_ context.Context,
	body []byte,
) (providerregistryapp.PortableImportResult, error) {
	service.lastCall = "import portable manifest"
	service.portableImportReceived = append(service.portableImportReceived[:0], body...)
	return service.portableImportResult, service.err
}

func (service *recordingProviderRegistryHTTPService) Snapshot(context.Context) (domainregistry.Registry, error) {
	service.lastCall = "snapshot"
	return service.snapshot.Clone(), service.err
}

func (service *recordingProviderRegistryHTTPService) Connect(
	_ context.Context,
	command providerregistryapp.ConnectCommand,
) (domainregistry.Provider, error) {
	service.lastCall = "connect"
	service.providerID = command.Provider.ID
	service.expected = command.Expected
	service.mutationKind = command.Credential.Kind()
	service.receivedCredential = command.Credential.Secret()
	return service.result.Clone(), service.err
}

func (service *recordingProviderRegistryHTTPService) Update(
	_ context.Context,
	command providerregistryapp.UpdateCommand,
) (domainregistry.Provider, error) {
	service.lastCall = "update"
	service.providerID = command.Provider.ID
	service.expected = command.Expected
	service.mutationKind = command.Credential.Kind()
	service.receivedCredential = command.Credential.Secret()
	return service.result.Clone(), service.err
}

func (service *recordingProviderRegistryHTTPService) Select(
	_ context.Context,
	command providerregistryapp.SelectCommand,
) (domainregistry.Provider, error) {
	service.lastCall = "select"
	service.providerID = command.ProviderID
	service.expected = command.Expected
	return service.result.Clone(), service.err
}

func (service *recordingProviderRegistryHTTPService) ReplaceCredential(
	_ context.Context,
	command providerregistryapp.CredentialReplaceCommand,
) (domainregistry.Provider, error) {
	service.lastCall = "credential"
	service.providerID = command.ProviderID
	service.expected = command.Expected
	service.mutationKind = command.Credential.Kind()
	service.receivedCredential = command.Credential.Secret()
	return service.result.Clone(), service.err
}

func (service *recordingProviderRegistryHTTPService) Disconnect(
	_ context.Context,
	command providerregistryapp.DisconnectCommand,
) (domainregistry.Provider, error) {
	service.lastCall = "disconnect"
	service.providerID = command.ProviderID
	service.expected = command.Expected
	return service.result.Clone(), service.err
}

func (service *recordingProviderRegistryHTTPService) ExplicitDelete(
	_ context.Context,
	command providerregistryapp.ExplicitDeleteCommand,
) error {
	service.lastCall = "delete"
	service.providerID = command.ProviderID
	service.expected = command.Expected
	return service.err
}

func (service *recordingProviderRegistryHTTPService) Recover(context.Context) error {
	service.lastCall = "recover"
	service.recovered = true
	return service.err
}

func (service *recordingProviderRegistryHTTPService) PrepareLegacyMigrationRecovery(
	_ context.Context,
	candidate providerregistryapp.LegacyMigrationCandidate,
) (providerregistryapp.LegacyMigrationRecoveryResult, error) {
	service.lastCall = "prepare legacy migration recovery"
	service.providerID = candidate.Provider.ID
	service.expected = candidate.Expected
	service.receivedCredential = append([]byte(nil), candidate.Credential...)
	service.legacyMigrationCandidate = candidate
	service.legacyMigrationCandidate.Credential = nil
	service.legacyMigrationCandidate.ActiveCredentialLocators = append(
		[]string(nil),
		candidate.ActiveCredentialLocators...,
	)
	service.legacyMigrationCandidate.RollbackCredentialArtifacts = make(
		[]providerregistryapp.LegacyMigrationRollbackCredentialArtifact,
		len(candidate.RollbackCredentialArtifacts),
	)
	for index, artifact := range candidate.RollbackCredentialArtifacts {
		service.legacyMigrationCandidate.RollbackCredentialArtifacts[index] =
			providerregistryapp.LegacyMigrationRollbackCredentialArtifact{
				Locators:   append([]string(nil), artifact.Locators...),
				Credential: append([]byte(nil), artifact.Credential...),
			}
	}
	return service.legacyMigrationResult, service.err
}

func (service *recordingProviderRegistryHTTPService) CommitVerifiedLegacyMigrationRecovery(
	_ context.Context,
	command providerregistryapp.CommitVerifiedLegacyMigrationRecoveryCommand,
) (providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult, error) {
	service.lastCall = "commit verified legacy migration recovery"
	service.legacyMigrationCommitCommand = command
	return service.legacyMigrationCommitResult, service.err
}

func (service *recordingProviderRegistryHTTPService) AbandonLegacyMigrationRecovery(
	_ context.Context,
	command providerregistryapp.AbandonLegacyMigrationRecoveryCommand,
) error {
	service.lastCall = "abandon legacy migration recovery"
	service.abandonCommand = command
	return service.err
}

func (service *recordingProviderRegistryHTTPService) BeginLegacyMigrationRollback(
	_ context.Context,
	command providerregistryapp.BeginLegacyMigrationRollbackCommand,
) (providerregistryapp.BeginLegacyMigrationRollbackResult, error) {
	service.lastCall = "begin legacy migration rollback"
	service.rollbackBeginCommand = command
	return service.legacyMigrationRollbackBeginResult, service.err
}

func (service *recordingProviderRegistryHTTPService) CommitLegacyMigrationRollback(
	_ context.Context,
	command providerregistryapp.CommitLegacyMigrationRollbackCommand,
) (providerregistryapp.CommitLegacyMigrationRollbackResult, error) {
	service.lastCall = "commit legacy migration rollback"
	service.rollbackCommitCommand = command
	return service.legacyMigrationRollbackCommitResult, service.err
}

func (service *recordingProviderRegistryHTTPService) RemigrateRetainedLegacyMigrationRecovery(
	_ context.Context,
	command providerregistryapp.RemigrateRetainedLegacyMigrationRecoveryCommand,
) (providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult, error) {
	service.lastCall = "remigrate retained legacy migration recovery"
	service.remigrateCommand = command
	return service.legacyMigrationRemigrateResult, service.err
}

func (service *recordingProviderRegistryHTTPService) DeleteRetainedLegacyMigrationRecovery(
	_ context.Context,
	command providerregistryapp.DeleteRetainedLegacyMigrationRecoveryCommand,
) (providerregistryapp.DeleteRetainedLegacyMigrationRecoveryResult, error) {
	service.lastCall = "delete retained legacy migration recovery"
	service.protectedDeleteCommand = command
	return service.legacyMigrationProtectedDeleteResult, service.err
}

func (service *recordingProviderRegistryHTTPService) FinalizeLegacyMigrationRecovery(
	_ context.Context,
	command providerregistryapp.FinalizeLegacyMigrationRecoveryCommand,
) (providerregistryapp.FinalizeLegacyMigrationRecoveryResult, error) {
	service.lastCall = "finalize legacy migration recovery"
	service.finalizeCommand = command
	return service.legacyMigrationFinalizeResult, service.err
}

func (service *recordingProviderRegistryHTTPService) IssueLegacyMigrationSourceAuthorityChallenge(
	_ context.Context,
	command providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeCommand,
) (providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeResult, error) {
	service.lastCall = "issue legacy migration source authority challenge"
	service.sourceAuthorityCommand = command
	return service.legacyMigrationSourceAuthorityResult, service.err
}

func (service *recordingProviderRegistryHTTPService) InventoryLegacyMigrationRecoveries(
	context.Context,
) (providerregistryapp.LegacyMigrationRecoveryInventoryResult, error) {
	service.lastCall = "inventory legacy migration recoveries"
	return service.legacyMigrationInventoryResult, service.err
}

func providerRegistryHTTPTestProvider(id, credentialRef string) domainregistry.Provider {
	return domainregistry.Provider{
		ID: id, Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
		Models: []string{"model-alpha"}, MediaModels: []string{}, SelectedModel: "model-alpha",
		SelectedRoutes: []string{"primary"}, CredentialRef: credentialRef, CredentialPurpose: "provider-api-key",
		Revision: 7, Generation: 3, Incarnation: "inc_" + strings.Repeat("b", 43),
	}
}

func providerRegistryHTTPTestRegistry() domainregistry.Registry {
	provider := providerRegistryHTTPTestProvider("provider-alpha", "cred_"+strings.Repeat("B", 43))
	return domainregistry.Registry{
		Version: domainregistry.FormatVersion, Revision: 4, Incarnation: "inc_" + strings.Repeat("a", 43),
		SelectedProviderID: provider.ID, Providers: map[string]domainregistry.Provider{provider.ID: provider},
		Transactions: map[string]domainregistry.Transaction{},
	}
}

func providerRegistryHTTPExpectedJSON(registryRevision, providerRevision, providerGeneration, purpose string) string {
	return fmt.Sprintf(`{"registryRevision":"%s","registryIncarnation":"inc_%s","providerRevision":"%s","providerGeneration":"%s","providerIncarnation":"inc_%s","providerCredentialPurpose":"%s"}`,
		registryRevision, strings.Repeat("a", 43), providerRevision, providerGeneration, strings.Repeat("b", 43), purpose)
}

func providerRegistryHTTPLegacyMigrationPrepareBody(
	registryIncarnation string,
	migrationID string,
	sourceLocator string,
	sourceSnapshot []byte,
	expectedCleanedSourceSHA256 string,
	providerID string,
	encodedCredential string,
	activeCredentialLocators []string,
	rollbackCredentialArtifactsJSON string,
) (string, string) {
	sourceDigest := sha256.Sum256(sourceSnapshot)
	sourceSHA256 := hex.EncodeToString(sourceDigest[:])
	artifactField := ""
	if rollbackCredentialArtifactsJSON != "" {
		artifactField = `,"rollbackCredentialArtifacts":` + rollbackCredentialArtifactsJSON
	}
	return fmt.Sprintf(
		`{"schemaVersion":1,"expected":{"registryRevision":"0","registryIncarnation":"%s","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"migrationId":"%s","sourceLocator":"%s","sourceSHA256":"%s","expectedCleanedSourceSHA256":"%s","sourcePhysicalIdentitySHA256":"%s","provider":%s,"credential":{"purpose":"provider-api-key","valueBase64":"%s"},"rollbackSource":{"valueBase64":"%s"},"activeCredentialLocators":["%s"]%s}`,
		registryIncarnation, migrationID, sourceLocator, sourceSHA256, expectedCleanedSourceSHA256,
		strings.Repeat("e", 64), providerRegistryHTTPProviderInputJSON(providerID), encodedCredential,
		base64.StdEncoding.EncodeToString(sourceSnapshot), strings.Join(activeCredentialLocators, `","`),
		artifactField,
	), sourceSHA256
}

func providerRegistryHTTPProviderInputJSON(id string) string {
	return fmt.Sprintf(`{"id":"%s","kind":"openai-compatible","endpoint":"https://provider.invalid/v1","proxy":"","models":["model-alpha"],"mediaModels":[],"selectedModel":"model-alpha","selectedMediaModel":"","selectedRoutes":["primary"]}`, id)
}

func assertProviderRegistryHTTPKeyFree(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	public := recorder.Body.String() + fmt.Sprint(recorder.Header())
	for _, forbidden := range []string{"credentialRef", "cred_", "ciphertext", "nonce", "masterKey", "raw-provider-body"} {
		if strings.Contains(public, forbidden) {
			t.Fatalf("public response contains %q: %s", forbidden, public)
		}
	}
}

func (service *recordingProviderRegistryHTTPService) CheckCredential(_ context.Context, command providerregistryapp.ProviderOperationCommand) error {
	service.lastCall = "credential check"
	service.providerID, service.expected = command.ProviderID, command.Expected
	return service.err
}

func TestProviderRegistryHTTPCredentialCheckReturnsOnlyFencedAvailability(t *testing.T) {
	body := `{"schemaVersion":1,"expected":{"registryRevision":"4","registryIncarnation":"inc_` + strings.Repeat("a", 43) + `","providerRevision":"2","providerGeneration":"1","providerIncarnation":"inc_` + strings.Repeat("b", 43) + `","providerCredentialPurpose":"provider-api-key"}}`
	for _, err := range []error{nil, registryport.ErrCredentialUnavailable, registryport.ErrConflict} {
		service := &recordingProviderRegistryHTTPService{err: err}
		recorder := httptest.NewRecorder()
		ProviderRegistryHandlers{Service: service}.Handle(recorder, httptest.NewRequest(http.MethodPost, ProviderRegistryPathV1+"/providers/provider-alpha/credential-check", strings.NewReader(body)))
		if service.lastCall != "credential check" || service.expected.ProviderRevision != 2 {
			t.Fatal("credential fence did not reach owner")
		}
		assertProviderRegistryHTTPKeyFree(t, recorder)
		if err == nil {
			if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"credentialAvailable":true`) {
				t.Fatalf("available = %d %s", recorder.Code, recorder.Body.String())
			}
		} else if errors.Is(err, registryport.ErrCredentialUnavailable) {
			if recorder.Code != 503 || !strings.Contains(recorder.Body.String(), `"code":"credential_unavailable"`) {
				t.Fatal("storage unavailability misclassified")
			}
		} else if recorder.Code != 409 {
			t.Fatal("stale check was accepted")
		}
	}
}
