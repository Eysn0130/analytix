package runtimeapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
)

type accountHTTPStateV1 struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Status              string `json:"status"`
	RegistryRevision    string `json:"registryRevision"`
	RegistryIncarnation string `json:"registryIncarnation"`
	ProviderRevision    string `json:"providerRevision"`
	ProviderGeneration  string `json:"providerGeneration"`
	ProviderIncarnation string `json:"providerIncarnation"`
}

func accountHTTPExpectedV1(state accountHTTPStateV1, purpose string) string {
	return `{"registryRevision":"` + state.RegistryRevision +
		`","registryIncarnation":"` + state.RegistryIncarnation +
		`","providerRevision":"` + state.ProviderRevision +
		`","providerGeneration":"` + state.ProviderGeneration +
		`","providerIncarnation":"` + state.ProviderIncarnation +
		`","providerCredentialPurpose":"` + purpose + `"}`
}

func TestProviderRegistryProductionAccountHTTPIsPurposeBoundKeyFreeAndRestartSafe(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows is compile-only for the private synthetic-master-key integration")
	}

	dataDir := t.TempDir()
	installSyntheticProviderRegistryFallbackMasterKey(t, dataDir)
	open := func() *providerRegistryAuthorityV1 {
		authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
		if err != nil {
			t.Fatalf("openProviderRegistryAuthorityV1() error = %v", err)
		}
		return authority
	}
	authority := open()
	handler := httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	path := httpapi.ProviderRegistryPathV1 + "/_private/account-credentials/"
	scope := `{"owner":"transport","provider":"telegram","accountId":"synthetic-account","channelId":"synthetic-channel","purpose":"transport-telegram-bot-token"}`
	marker := `{"kind":"telegram","botToken":"synthetic-telegram-token","allowedChatIds":"1001"}`

	status := httptest.NewRecorder()
	handler.Handle(status, httptest.NewRequest(http.MethodPost, path+"status", strings.NewReader(
		`{"schemaVersion":1,"scope":`+scope+`}`,
	)))
	var absent accountHTTPStateV1
	if status.Code != http.StatusOK || json.Unmarshal(status.Body.Bytes(), &absent) != nil || absent.Status != "absent" {
		t.Fatalf("absent status = (%d, %s)", status.Code, status.Body.String())
	}

	put := httptest.NewRecorder()
	handler.Handle(put, httptest.NewRequest(http.MethodPost, path+"put", strings.NewReader(
		`{"schemaVersion":1,"scope":`+scope+`,"expected":`+
			accountHTTPExpectedV1(absent, "")+`,"valueBase64":"`+
			base64.StdEncoding.EncodeToString([]byte(marker))+`"}`,
	)))
	var ready accountHTTPStateV1
	if put.Code != http.StatusOK || json.Unmarshal(put.Body.Bytes(), &ready) != nil || ready.Status != "ready" {
		t.Fatalf("put = (%d, %s)", put.Code, put.Body.String())
	}
	for _, forbidden := range []string{marker, "synthetic-telegram-token", "valueBase64", "credentialRef"} {
		if strings.Contains(put.Body.String(), forbidden) {
			t.Fatalf("put response leaked %q: %s", forbidden, put.Body.String())
		}
	}

	foreign := httptest.NewRecorder()
	handler.Handle(foreign, httptest.NewRequest(http.MethodPost, path+"resolve", strings.NewReader(
		`{"schemaVersion":1,"scope":{"owner":"transport","provider":"telegram","accountId":"synthetic-account","channelId":"foreign-channel","purpose":"transport-telegram-bot-token"}}`,
	)))
	if foreign.Code != http.StatusNotFound || strings.Contains(foreign.Body.String(), "synthetic-telegram-token") {
		t.Fatalf("cross-account resolve = (%d, %s)", foreign.Code, foreign.Body.String())
	}

	resolved := httptest.NewRecorder()
	handler.Handle(resolved, httptest.NewRequest(http.MethodPost, path+"resolve", strings.NewReader(
		`{"schemaVersion":1,"scope":`+scope+`}`,
	)))
	if resolved.Code != http.StatusOK || !strings.Contains(resolved.Body.String(), base64.StdEncoding.EncodeToString([]byte(marker))) {
		t.Fatalf("resolve = (%d, %s)", resolved.Code, resolved.Body.String())
	}

	listed := httptest.NewRecorder()
	handler.Handle(listed, httptest.NewRequest(http.MethodGet, httpapi.ProviderRegistryPathV1, nil))
	for _, forbidden := range []string{"synthetic-account", "synthetic-channel", "synthetic-telegram-token", "acct_", "credentialRef"} {
		if strings.Contains(listed.Body.String(), forbidden) {
			t.Fatalf("public list leaked %q: %s", forbidden, listed.Body.String())
		}
	}

	revokedRecorder := httptest.NewRecorder()
	handler.Handle(revokedRecorder, httptest.NewRequest(http.MethodPost, path+"mutate", strings.NewReader(
		`{"schemaVersion":1,"scope":`+scope+`,"expected":`+
			accountHTTPExpectedV1(ready, "transport-telegram-bot-token")+`,"disposition":"revoke"}`,
	)))
	var revoked accountHTTPStateV1
	if revokedRecorder.Code != http.StatusOK || json.Unmarshal(revokedRecorder.Body.Bytes(), &revoked) != nil || revoked.Status != "revoked" {
		t.Fatalf("revoke = (%d, %s)", revokedRecorder.Code, revokedRecorder.Body.String())
	}
	if err := authority.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	authority = open()
	defer func() {
		if err := authority.Close(); err != nil {
			t.Fatalf("Close() after restart error = %v", err)
		}
	}()
	handler = httpapi.ProviderRegistryHandlers{Service: authority.Service()}
	afterRestart := httptest.NewRecorder()
	handler.Handle(afterRestart, httptest.NewRequest(http.MethodPost, path+"status", strings.NewReader(
		`{"schemaVersion":1,"scope":`+scope+`}`,
	)))
	var restarted accountHTTPStateV1
	if afterRestart.Code != http.StatusOK || json.Unmarshal(afterRestart.Body.Bytes(), &restarted) != nil || restarted.Status != "revoked" {
		t.Fatalf("restart status = (%d, %s)", afterRestart.Code, afterRestart.Body.String())
	}

	deleted := httptest.NewRecorder()
	handler.Handle(deleted, httptest.NewRequest(http.MethodPost, path+"mutate", strings.NewReader(
		`{"schemaVersion":1,"scope":`+scope+`,"expected":`+
			accountHTTPExpectedV1(restarted, "")+`,"disposition":"delete"}`,
	)))
	var final accountHTTPStateV1
	if deleted.Code != http.StatusOK || json.Unmarshal(deleted.Body.Bytes(), &final) != nil || final.Status != "absent" {
		t.Fatalf("delete = (%d, %s)", deleted.Code, deleted.Body.String())
	}
}
