//go:build !analytix_prod

package runtimego

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	finalauthority "analytix.local/runtime-go/internal/adapters/outbound/finalauthority"
)

// Opt in only when a contract needs its declared synthetic Provider to execute.
// The plain constructor remains available for absent/invalid authority tests.
// Legacy configuration still carries model metadata, never execution authority.
func newRuntimeServerProviderReadyTestHandler(t *testing.T, config RuntimeServerContractConfig) http.Handler {
	t.Helper()
	config, connect := prepareRuntimeServerReadyProviderRegistry(t, config)
	handler := newRuntimeServerContractTestHandler(t, config)
	connect(handler)
	return handler
}

func selectRuntimeServerFixtureProvider(t *testing.T, baseURL, token, providerID string) {
	t.Helper()
	snapshot := assertLiveJSON(t, baseURL, http.MethodGet, "/v1/provider-registry", token, nil, http.StatusOK)
	providers, _ := snapshot["providers"].([]any)
	for _, raw := range providers {
		profile := raw.(map[string]any)
		if profile["id"] != providerID {
			continue
		}
		expected := map[string]any{"registryRevision": snapshot["registryRevision"], "registryIncarnation": snapshot["registryIncarnation"],
			"providerRevision": profile["revision"], "providerGeneration": profile["generation"], "providerIncarnation": profile["incarnation"], "providerCredentialPurpose": profile["credentialPurpose"]}
		assertLiveJSON(t, baseURL, http.MethodPost, "/v1/provider-registry/providers/"+providerID+"/select", token,
			mustJSON(t, map[string]any{"schemaVersion": 1, "expected": expected}), http.StatusOK)
		return
	}
	t.Fatal("declared fixture Provider is absent from Registry")
}

func newRuntimeServerProviderReadyHostAuthorityTestHandler(t *testing.T, config RuntimeServerContractConfig) (http.Handler, finalauthority.SecurePrivateCASAccessAuthority) {
	t.Helper()
	config, connect := prepareRuntimeServerReadyProviderRegistry(t, config)
	handler, access := newRuntimeServerHostAuthorityTestHandler(t, config)
	connect(handler)
	return handler, access
}

func prepareRuntimeServerReadyProviderRegistry(t *testing.T, config RuntimeServerContractConfig) (RuntimeServerContractConfig, func(http.Handler)) {
	t.Helper()
	config = prepareRuntimeServerTestConfig(t, config)
	if strings.TrimSpace(config.ModelProvidersJSON) == "" && config.BaseURL != "" {
		config.ProviderID = firstNonEmptyTestString(config.ProviderID, "deepseek")
		config.APIKey = firstNonEmptyTestString(config.APIKey, "test-provider-key")
		config.Model = firstNonEmptyTestString(config.Model, "deepseek-chat")
		config.ModelProvidersJSON = string(mustJSON(t, map[string]any{
			"defaultProviderId": config.ProviderID,
			"providers": []map[string]any{{"id": config.ProviderID, "apiKey": config.APIKey,
				"baseUrl": config.BaseURL, "endpointFormat": config.EndpointFormat, "models": []string{config.Model}}},
		}))
	}
	var declarations struct {
		DefaultProviderID string `json:"defaultProviderId"`
		Providers         []struct {
			ID             string   `json:"id"`
			APIKey         string   `json:"apiKey"`
			BaseURL        string   `json:"baseUrl"`
			EndpointFormat string   `json:"endpointFormat"`
			Models         []string `json:"models"`
		} `json:"providers"`
	}
	if json.Unmarshal([]byte(config.ModelProvidersJSON), &declarations) != nil || len(declarations.Providers) == 0 {
		t.Fatal("ready Provider fixture requires explicit model declarations")
	}
	for _, declared := range declarations.Providers {
		endpoint, err := url.Parse(declared.BaseURL)
		if err != nil || endpoint.Scheme != "http" || endpoint.User != nil || !net.ParseIP(endpoint.Hostname()).IsLoopback() || declared.APIKey == "" || len(declared.Models) == 0 {
			t.Fatal("ready Provider fixture requires declared loopback models and a synthetic credential")
		}
	}
	if config.DataDir == "" {
		config.DataDir = t.TempDir()
	}
	masterRoot := filepath.Join(config.DataDir, "private", "provider-secrets", "master-key")
	if err := os.MkdirAll(masterRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	key, err := os.OpenFile(filepath.Join(masterRoot, "master.key"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		secret := bytes.Repeat([]byte{0x6a}, 32)
		_, writeErr := key.Write(secret)
		clear(secret)
		closeErr := key.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("synthetic master key installation failed")
		}
		if runtime.GOOS == "darwin" {
			if err := os.WriteFile(filepath.Join(masterRoot, "authority.v1"), []byte("analytix-master-key-authority:v1:fallback\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	} else if !os.IsExist(err) {
		t.Fatal(err)
	}
	connect := func(handler http.Handler) {
		token := config.RuntimeToken
		if token == "" && !config.Insecure {
			token = DefaultRuntimeToken
		}
		request := func(method string, body any) map[string]any {
			t.Helper()
			var encoded []byte
			if body != nil {
				var err error
				encoded, err = json.Marshal(body)
				if err != nil {
					t.Fatal("Registry fixture request encoding failed")
				}
			}
			req := httptest.NewRequest(method, "/v1/provider-registry", bytes.NewReader(encoded))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			clear(encoded)
			var decoded map[string]any
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &decoded) != nil {
				t.Fatalf("Registry fixture public operation status=%d", response.Code)
			}
			return decoded
		}
		snapshot := request(http.MethodGet, nil)
		existing, ok := snapshot["providers"].([]any)
		if !ok {
			t.Fatal("Registry fixture inventory is unavailable")
		}
		// A restart must retain the actual Registry, including deliberate test edits.
		if len(existing) != 0 {
			return
		}
		for _, declared := range declarations.Providers {
			kind := declared.EndpointFormat
			if kind == "" || kind == "chat_completions" {
				kind = "openai-compatible"
			}
			routes := []string{}
			if declared.ID == declarations.DefaultProviderID {
				routes = []string{"primary"}
			}
			credential := []byte(declared.APIKey)
			encodedCredential := base64.StdEncoding.EncodeToString(credential)
			clear(credential)
			request(http.MethodPost, map[string]any{
				"schemaVersion": 1,
				"expected":      map[string]any{"registryRevision": snapshot["registryRevision"], "registryIncarnation": snapshot["registryIncarnation"], "providerRevision": "0", "providerGeneration": "0", "providerIncarnation": "", "providerCredentialPurpose": ""},
				"provider":      map[string]any{"id": declared.ID, "kind": kind, "endpoint": declared.BaseURL, "proxy": "", "models": declared.Models, "mediaModels": []string{}, "selectedModel": declared.Models[0], "selectedMediaModel": "", "selectedRoutes": routes},
				"credential":    map[string]any{"kind": "set", "purpose": "provider-api-key", "valueBase64": encodedCredential},
			})
			snapshot = request(http.MethodGet, nil)
			public, err := json.Marshal(snapshot)
			if err != nil || strings.Contains(string(public), declared.APIKey) || strings.Contains(string(public), encodedCredential) {
				t.Fatal("Registry fixture readback exposed credential material")
			}
		}
		if snapshot["selectedProviderId"] != declarations.DefaultProviderID {
			t.Fatal("Registry fixture did not select the declared default")
		}
	}
	return config, connect
}
