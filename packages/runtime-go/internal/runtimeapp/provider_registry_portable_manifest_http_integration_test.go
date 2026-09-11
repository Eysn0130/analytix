package runtimeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
)

type portableManifestHTTPExportResponseV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	ManifestJSON  string `json:"manifestJson"`
}

type portableManifestHTTPImportResponseV1 struct {
	SchemaVersion   int `json:"schemaVersion"`
	ProviderCount   int `json:"providerCount"`
	AccountCount    int `json:"accountCount"`
	ReentryRequired int `json:"reentryRequired"`
	Entries         []struct {
		Correlation           string `json:"correlation"`
		DestinationProviderID string `json:"destinationProviderId"`
		Status                string `json:"status"`
	} `json:"entries"`
}

type portableManifestHTTPListResponseV1 struct {
	SchemaVersion       int    `json:"schemaVersion"`
	RegistryRevision    string `json:"registryRevision"`
	RegistryIncarnation string `json:"registryIncarnation"`
	SelectedProviderID  string `json:"selectedProviderId"`
	Providers           []struct {
		ID                   string   `json:"id"`
		CredentialConfigured bool     `json:"credentialConfigured"`
		CredentialPurpose    string   `json:"credentialPurpose"`
		SelectedRoutes       []string `json:"selectedRoutes"`
		Revision             string   `json:"revision"`
		Generation           string   `json:"generation"`
		Incarnation          string   `json:"incarnation"`
		Tombstone            bool     `json:"tombstone"`
	} `json:"providers"`
}

func TestProviderRegistryPortableManifestProductionHTTPRoundTripRestartAndReentryGate(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the synthetic provider Secret Store integration")
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
	defer func() { _ = source.Close() }()
	destination, err := openProviderRegistryAuthorityV1(ctx, destinationDir)
	if err != nil {
		t.Fatalf("open destination authority error = %v", err)
	}
	defer func() { _ = destination.Close() }()

	const sourceProviderID = "provider-source"
	secretMarker := []byte("synthetic-portable-http-k2-secret")
	encodedSecret := base64.StdEncoding.EncodeToString(secretMarker)
	sourceSnapshot, err := source.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("source Snapshot() error = %v", err)
	}
	connectBody := `{"schemaVersion":1,"expected":{"registryRevision":"` +
		stringUint64(sourceSnapshot.Revision) + `","registryIncarnation":"` + sourceSnapshot.Incarnation +
		`","providerRevision":"0","providerGeneration":"0","providerIncarnation":"","providerCredentialPurpose":""},"provider":{"id":"` +
		sourceProviderID + `","kind":"openai-compatible","endpoint":"https://provider.invalid/v1","proxy":"","models":["model-alpha"],"mediaModels":[],"selectedModel":"model-alpha","selectedMediaModel":"","selectedRoutes":[]},"credential":{"kind":"set","purpose":"provider-api-key","valueBase64":"` +
		encodedSecret + `"}}`
	sourceHandler := httpapi.ProviderRegistryHandlers{Service: source.Service()}
	connected := httptest.NewRecorder()
	sourceHandler.Handle(connected, httptest.NewRequest(http.MethodPost, httpapi.ProviderRegistryPathV1, strings.NewReader(connectBody)))
	if connected.Code != http.StatusOK || strings.Contains(connected.Body.String(), string(secretMarker)) ||
		strings.Contains(connected.Body.String(), "credentialRef") {
		t.Fatalf("source connect = (%d, %s)", connected.Code, connected.Body.String())
	}

	exported := httptest.NewRecorder()
	sourceHandler.Handle(exported, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPortableManifestPathV1, nil))
	if exported.Code != http.StatusOK || strings.Contains(exported.Body.String(), string(secretMarker)) ||
		strings.Contains(exported.Body.String(), encodedSecret) || strings.Contains(exported.Body.String(), "credentialRef") {
		t.Fatalf("source portable export = (%d, %s)", exported.Code, exported.Body.String())
	}
	var exportResponse portableManifestHTTPExportResponseV1
	if err := json.Unmarshal(exported.Body.Bytes(), &exportResponse); err != nil || exportResponse.SchemaVersion != 1 {
		t.Fatalf("portable export response = (%s), error=%v", exported.Body.String(), err)
	}
	if _, err := domainregistry.ParsePortableManifestV1([]byte(exportResponse.ManifestJSON)); err != nil {
		t.Fatalf("exported manifest parse error = %v", err)
	}

	destinationHandler := httpapi.ProviderRegistryHandlers{Service: destination.Service()}
	importBodyBytes, err := json.Marshal(struct {
		SchemaVersion int    `json:"schemaVersion"`
		ManifestJSON  string `json:"manifestJson"`
	}{SchemaVersion: 1, ManifestJSON: exportResponse.ManifestJSON})
	if err != nil {
		t.Fatalf("marshal import wrapper error = %v", err)
	}
	imported := httptest.NewRecorder()
	destinationHandler.Handle(imported, httptest.NewRequest(http.MethodPost,
		httpapi.ProviderRegistryPortableManifestPathV1, bytes.NewReader(importBodyBytes)))
	if imported.Code != http.StatusOK || strings.Contains(imported.Body.String(), string(secretMarker)) ||
		strings.Contains(imported.Body.String(), encodedSecret) || strings.Contains(imported.Body.String(), "credentialRef") {
		t.Fatalf("destination portable import = (%d, %s)", imported.Code, imported.Body.String())
	}
	var importResponse portableManifestHTTPImportResponseV1
	if err := json.Unmarshal(imported.Body.Bytes(), &importResponse); err != nil ||
		importResponse.ProviderCount != 1 || importResponse.AccountCount != 0 ||
		importResponse.ReentryRequired != 1 || len(importResponse.Entries) != 1 ||
		importResponse.Entries[0].Status != domainregistry.PortableManifestIntentReentryRequired ||
		importResponse.Entries[0].DestinationProviderID == sourceProviderID {
		t.Fatalf("destination import response = (%d, %s), error=%v", imported.Code, imported.Body.String(), err)
	}
	destinationProviderID := importResponse.Entries[0].DestinationProviderID

	listAfterImport := httptest.NewRecorder()
	destinationHandler.Handle(listAfterImport, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil))
	var importedList portableManifestHTTPListResponseV1
	if listAfterImport.Code != http.StatusOK || json.Unmarshal(listAfterImport.Body.Bytes(), &importedList) != nil ||
		importedList.SelectedProviderID != "" || len(importedList.Providers) != 1 ||
		importedList.Providers[0].ID != destinationProviderID || importedList.Providers[0].CredentialConfigured ||
		importedList.Providers[0].CredentialPurpose != "" || importedList.Providers[0].Tombstone {
		t.Fatalf("destination list after import = (%d, %s)", listAfterImport.Code, listAfterImport.Body.String())
	}

	selectBody := portableManifestHTTPExpectedBody(importedList.RegistryRevision, importedList.RegistryIncarnation,
		importedList.Providers[0].Revision, importedList.Providers[0].Generation, importedList.Providers[0].Incarnation, "")
	blockedSelect := httptest.NewRecorder()
	destinationHandler.Handle(blockedSelect, httptest.NewRequest(http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/providers/"+destinationProviderID+"/select", strings.NewReader(selectBody)))
	if blockedSelect.Code != http.StatusInternalServerError || !strings.Contains(blockedSelect.Body.String(), `"code":"verification_failure"`) {
		t.Fatalf("uncredentialed destination select = (%d, %s)", blockedSelect.Code, blockedSelect.Body.String())
	}
	if _, err := destination.Manager().ResolveSelectedForExecution(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("uncredentialed destination execution error = %v, want verification failure", err)
	}

	if err := destination.Close(); err != nil {
		t.Fatalf("destination Close() before restart error = %v", err)
	}
	destination, err = openProviderRegistryAuthorityV1(ctx, destinationDir)
	if err != nil {
		t.Fatalf("reopen destination authority error = %v", err)
	}
	destinationHandler = httpapi.ProviderRegistryHandlers{Service: destination.Service()}
	restartedList := httptest.NewRecorder()
	destinationHandler.Handle(restartedList, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil))
	var restarted portableManifestHTTPListResponseV1
	if restartedList.Code != http.StatusOK || json.Unmarshal(restartedList.Body.Bytes(), &restarted) != nil ||
		restarted.SelectedProviderID != "" || len(restarted.Providers) != 1 || restarted.Providers[0].ID != destinationProviderID ||
		restarted.Providers[0].CredentialConfigured {
		t.Fatalf("destination list after restart = (%d, %s)", restartedList.Code, restartedList.Body.String())
	}

	replaceBody := portableManifestHTTPExpectedBody(restarted.RegistryRevision, restarted.RegistryIncarnation,
		restarted.Providers[0].Revision, restarted.Providers[0].Generation, restarted.Providers[0].Incarnation, "")
	replaceBody = strings.TrimSuffix(replaceBody, "}") + `,"credential":{"kind":"set","purpose":"provider-api-key","valueBase64":"` + encodedSecret + `"}}`
	replaced := httptest.NewRecorder()
	destinationHandler.Handle(replaced, httptest.NewRequest(http.MethodPut,
		httpapi.ProviderRegistryPathV1+"/providers/"+destinationProviderID+"/credential", strings.NewReader(replaceBody)))
	if replaced.Code != http.StatusOK || strings.Contains(replaced.Body.String(), string(secretMarker)) ||
		strings.Contains(replaced.Body.String(), encodedSecret) || strings.Contains(replaced.Body.String(), "credentialRef") {
		t.Fatalf("destination K2 credential replace = (%d, %s)", replaced.Code, replaced.Body.String())
	}

	postReplace, err := destination.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("destination Snapshot() after K2 error = %v", err)
	}
	provider := postReplace.Providers[destinationProviderID]
	selectAfterCredential := portableManifestHTTPExpectedBody(stringUint64(postReplace.Revision), postReplace.Incarnation,
		stringUint64(provider.Revision), stringUint64(provider.Generation), provider.Incarnation, provider.CredentialPurpose)
	selected := httptest.NewRecorder()
	destinationHandler.Handle(selected, httptest.NewRequest(http.MethodPost,
		httpapi.ProviderRegistryPathV1+"/providers/"+destinationProviderID+"/select", strings.NewReader(selectAfterCredential)))
	if selected.Code != http.StatusOK || strings.Contains(selected.Body.String(), string(secretMarker)) ||
		strings.Contains(selected.Body.String(), "credentialRef") {
		t.Fatalf("destination select after K2 = (%d, %s)", selected.Code, selected.Body.String())
	}
	resolved, err := destination.Manager().ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("destination execution after K2 error = %v", err)
	}
	if !bytes.Equal(resolved.Credential, secretMarker) {
		resolved.Clear()
		t.Fatal("destination execution did not read the synthetic K2 credential")
	}
	resolved.Clear()

	registryBytes, err := os.ReadFile(filepath.Join(destinationDir, "private", "provider-registry", "registry.v1.json"))
	if err != nil {
		t.Fatalf("ReadFile(destination Registry) error = %v", err)
	}
	if bytes.Contains(registryBytes, secretMarker) || bytes.Contains(registryBytes, []byte(encodedSecret)) {
		t.Fatal("destination Registry contains the synthetic credential")
	}
}

func portableManifestHTTPExpectedBody(registryRevision, registryIncarnation, providerRevision, providerGeneration,
	providerIncarnation, purpose string) string {
	return `{"schemaVersion":1,"expected":{"registryRevision":"` + registryRevision +
		`","registryIncarnation":"` + registryIncarnation + `","providerRevision":"` + providerRevision +
		`","providerGeneration":"` + providerGeneration + `","providerIncarnation":"` + providerIncarnation +
		`","providerCredentialPurpose":"` + purpose + `"}}`
}

func stringUint64(value uint64) string {
	return strconv.FormatUint(value, 10)
}
