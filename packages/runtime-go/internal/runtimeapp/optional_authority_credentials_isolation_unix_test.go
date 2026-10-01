//go:build darwin || linux

package runtimeapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
	authoritycompositionfixture "analytix.local/runtime-go/internal/formalauthority"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

func TestUnavailableThreadRiskCredentialDoesNotBlockOrdinaryRuntime(t *testing.T) {
	fixture, err := authoritycompositionfixture.New(runtimeWitnessedRegistryHostTempV2(t, "thread-risk-credential"))
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	if blocker := authoritycompositionfixture.SecureConfigurationFilesystemBlocker(fixture.ManifestRoot); blocker != "" {
		t.Skipf("BLOCKED: %s", blocker)
	}
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer provider.Close()
	durableRoot := t.TempDir()
	config := Config{
		RuntimeToken: DefaultRuntimeToken, ProductionDurableRoot: durableRoot, DataDir: fixture.DataDir,
		ProviderID: "synthetic-ordinary", BaseURL: provider.URL, APIKey: "test-only",
		Model: "synthetic-ordinary-model", EndpointFormat: "chat_completions",
		AuthorityAnchorV1: fixture.AnchorEnvelope, AuthorityManifestRoot: fixture.ManifestRoot,
		AuthorityCredentialProfileRoot: fixture.CredentialProfileRoot,
		AuthorityCredentialBundleRoot:  fixture.CredentialBundleRoot,
	}
	initial, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	created := runtimeStartupJSON(t, initial, http.MethodPost, "/v1/threads", map[string]any{
		"workspace": workspace, "providerId": config.ProviderID, "model": config.Model,
	}, http.StatusCreated)
	threadID, _ := created["id"].(string)
	if threadID == "" {
		t.Fatalf("ordinary thread was not created: %#v", created)
	}
	shutdownOwnedRuntimeHandler(t, initial)
	// Preserve a preexisting unknown registry Original through optional loss.
	registryOriginalRoot := filepath.Join(fixture.DataDir, "private", "evidence-registry")
	if err := os.MkdirAll(registryOriginalRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registryOriginalRoot, ".registry-authority-index.json"),
		[]byte(`{"domainCanary":"`+runtimeOptionalDomainPrivateCanary+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// This is a task-owned synthetic credential, not an installed/user secret.
	if err := os.Remove(filepath.Join(fixture.CredentialBundleRoot, "thread-risk-client-key.pk8")); err != nil {
		t.Fatal(err)
	}
	originalRoots := []string{
		fixture.AuthorityPath, fixture.ManifestRoot, fixture.CredentialProfileRoot,
		fixture.CredentialBundleRoot, registryOriginalRoot,
	}
	original := startupWholeTreeRecordMapForTest(t, originalRoots...)
	beforeWitness := fixture.TotalAttempts()
	other, err := finalauthority.OpenOrCreateFileAuthority(filepath.Join(t.TempDir(), "other-authority.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, configured, err := loadRuntimeSharedEvidenceEnrollmentV2(context.Background(), config, other); !configured || err == nil ||
		!strings.Contains(err.Error(), "runtime installation authority does not match protected enrollment") {
		t.Fatalf("private credential loss masked the installation identity error: configured=%t err=%v", configured, err)
	}
	invalidRoot := config
	invalidRoot.AuthorityCredentialBundleRoot += "/."
	if _, configured, err := loadRuntimeSharedEvidenceEnrollmentV2(context.Background(), invalidRoot, fixture.Authority); !configured || err == nil ||
		errors.Is(err, errRuntimeOptionalAuthorityCredentialsUnavailable) {
		t.Fatalf("private credential loss masked the authority root error: configured=%t err=%v", configured, err)
	}

	restarted, err := NewRuntimeServerHandlerE(config)
	if err != nil {
		t.Fatalf("optional ThreadRisk credential blocked ordinary startup: %v", err)
	}
	if health := runtimeStartupJSON(t, restarted, http.MethodGet, "/health", nil, http.StatusOK); health["status"] != "ok" {
		t.Fatalf("ordinary health failed: %#v", health)
	}
	if previous := runtimeStartupJSON(t, restarted, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK); previous["id"] != threadID {
		t.Fatalf("ordinary thread was not recovered: %#v", previous)
	}
	newThread := runtimeStartupJSON(t, restarted, http.MethodPost, "/v1/threads", map[string]any{
		"workspace": workspace, "providerId": config.ProviderID, "model": config.Model,
	}, http.StatusCreated)
	if newThread["id"] == "" || newThread["id"] == threadID {
		t.Fatalf("ordinary thread creation after credential loss failed: %#v", newThread)
	}
	request := httptest.NewRequest(http.MethodPost, httpapi.HostFundsImportStagePathV1, strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
	response := httptest.NewRecorder()
	restarted.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"code":"funds_import_capability_unavailable"`) {
		t.Fatalf("protected import did not fail closed: status=%d body=%s", response.Code, response.Body.String())
	}
	shutdownOwnedRuntimeHandler(t, restarted)
	if fixture.TotalAttempts() != beforeWitness {
		t.Fatal("ordinary startup or Funds refusal reached a witness")
	}
	if providerCalls.Load() != 0 {
		t.Fatal("ordinary startup or Funds refusal reached a Provider")
	}
	if after := startupWholeTreeRecordMapForTest(t, originalRoots...); !reflect.DeepEqual(original, after) {
		t.Fatalf("optional credential loss changed historical original state: %v", changedWholeTreeRecordKeysForTest(original, after))
	}
	// An absent or mismatched signed manifest remains a startup identity error.
	wrongRoot := config
	wrongRoot.AuthorityManifestRoot = t.TempDir()
	if bad, err := NewRuntimeServerHandlerE(wrongRoot); err == nil {
		shutdownOwnedRuntimeHandler(t, bad)
		t.Fatal("missing signed manifest was treated as an optional credential fault")
	}
	if after := startupWholeTreeRecordMapForTest(t, originalRoots...); !reflect.DeepEqual(original, after) {
		t.Fatalf("failed identity validation changed historical original state: %v", changedWholeTreeRecordKeysForTest(original, after))
	}
}
