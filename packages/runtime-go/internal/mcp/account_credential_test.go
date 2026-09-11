package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
)

const mcpAccountBindingFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func mcpOAuthMetadata() *domainregistry.OAuthBindingMetadata {
	return &domainregistry.OAuthBindingMetadata{
		SchemaVersion: 1, Issuer: "https://issuer.invalid/",
		AuthorizationEndpoint: "https://login.invalid/authorize",
		TokenEndpoint:         "https://tokens.invalid/token", ClientID: "client-a",
		Scopes: []string{"openid"}, RedirectModeVersion: 1,
	}
}

func protectedMCPOAuthBundle(
	t *testing.T,
	kind string,
	owner string,
	provider string,
	accountID string,
	channelID string,
	accessToken string,
	expiresAtMS int64,
) string {
	t.Helper()
	bindingKey := "oauthb_" + strings.Repeat("b", 43)
	payload := map[string]any{
		"kind": kind, "accessToken": accessToken, "tokenType": "Bearer",
		"oauthBinding": map[string]any{
			"owner": owner, "issuer": "https://issuer.invalid/",
			"authorizationEndpoint": "https://login.invalid/authorize",
			"tokenEndpoint":         "https://tokens.invalid/token", "clientId": "client-a",
			"provider": provider, "accountId": accountID, "channelId": channelID,
			"redirectUri": "com.analytix.desktop:/oauth/callback/" + bindingKey,
			"scopes":      []string{"openid"}, "bindingKey": bindingKey,
			"ownerFingerprint": mcpAccountBindingFingerprint,
			"ownerRevision":    "1", "ownerGeneration": "1",
			"ownerIncarnation": "cfg_" + mcpAccountBindingFingerprint,
			"profileBinding":   strings.Repeat("c", 64),
		},
	}
	if expiresAtMS > 0 {
		payload["expiresAtMs"] = expiresAtMS
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(OAuth bundle) error = %v", err)
	}
	return string(encoded)
}

type mcpAccountResolverTestDouble struct {
	mu         sync.Mutex
	credential []byte
	state      providerregistryapp.AccountCredentialState
	scopes     []domainregistry.PrivateAccountScope
	current    bool
}

func (resolver *mcpAccountResolverTestDouble) ResolveAccountCredential(
	_ context.Context,
	scope domainregistry.PrivateAccountScope,
) (providerregistryapp.AccountCredentialResolution, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if !resolver.current {
		return providerregistryapp.AccountCredentialResolution{}, errors.New("revoked")
	}
	resolver.scopes = append(resolver.scopes, scope)
	credential := append([]byte(nil), resolver.credential...)
	state := resolver.state
	state.Scope = scope
	return providerregistryapp.AccountCredentialResolution{State: state, Credential: credential}, nil
}

func (resolver *mcpAccountResolverTestDouble) ValidateAccountCredentialCurrent(
	_ context.Context,
	state providerregistryapp.AccountCredentialState,
) error {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.current && state.ProviderGeneration == resolver.state.ProviderGeneration &&
		state.ProviderIncarnation == resolver.state.ProviderIncarnation {
		return nil
	}
	return errors.New("stale")
}

func (resolver *mcpAccountResolverTestDouble) replace(credential string, generation uint64) {
	resolver.mu.Lock()
	resolver.credential = []byte(credential)
	resolver.state.ProviderGeneration = generation
	resolver.current = true
	resolver.mu.Unlock()
}

func (resolver *mcpAccountResolverTestDouble) revoke() {
	resolver.mu.Lock()
	resolver.current = false
	resolver.mu.Unlock()
}

type mcpAccountClientTestDouble struct {
	authorization string
	onEffect      func()
	closed        bool
}

func (client *mcpAccountClientTestDouble) effect() {
	if client.onEffect != nil {
		client.onEffect()
	}
}

func (client *mcpAccountClientTestDouble) ListTools() ([]ToolSpec, error) {
	client.effect()
	return []ToolSpec{}, nil
}
func (client *mcpAccountClientTestDouble) ListPrompts() ([]PromptSpec, error) {
	client.effect()
	return []PromptSpec{}, nil
}
func (client *mcpAccountClientTestDouble) ListResources() ([]ResourceSpec, error) {
	client.effect()
	return []ResourceSpec{}, nil
}
func (client *mcpAccountClientTestDouble) CallTool(string, map[string]any) (any, error) {
	client.effect()
	return map[string]any{"private": "must-be-discarded"}, nil
}
func (client *mcpAccountClientTestDouble) ObservedServerIdentity() domainmcp.ServerIdentity {
	return domainmcp.ServerIdentity{ProtocolVersion: "2025-06-18", Name: "server-a", Version: "1.0.0"}
}
func (client *mcpAccountClientTestDouble) Close() { client.closed = true }

func accountState(scope domainregistry.PrivateAccountScope, generation uint64) providerregistryapp.AccountCredentialState {
	return providerregistryapp.AccountCredentialState{
		Scope: scope, Status: providerregistryapp.AccountCredentialStatusReady,
		RegistryRevision: 2, RegistryIncarnation: "registry-a",
		ProviderRevision: 1, ProviderGeneration: generation, ProviderIncarnation: "provider-a",
		CredentialPurpose: scope.Purpose,
	}
}

func TestMCPAccountCredentialIsResolvedForEveryEffectAndStaleResultsFailClosed(t *testing.T) {
	scope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "mcp", Provider: "server-a", AccountID: "account-a",
		ChannelID: "server-a", Purpose: "mcp-oauth-access-token",
	}
	resolver := &mcpAccountResolverTestDouble{
		credential: []byte(protectedMCPOAuthBundle(t, "mcp-oauth-bundle", "mcp", "server-a", "account-a", "server-a", "synthetic-generation-1", 0)),
		state:      accountState(scope, 1), current: true,
	}
	var mu sync.Mutex
	requests := []string{}
	clients := []*mcpAccountClientTestDouble{}
	client := &mcpAccountCredentialBoundClient{
		spec: ServerSpec{
			ID: "server-a", Transport: "http", URL: "https://mcp.invalid/mcp",
			AccountCredential: &domainmcp.AccountCredentialScope{
				Owner: "mcp", Provider: "server-a", AccountID: "account-a", Purpose: "mcp-oauth-access-token",
				BindingFingerprint: mcpAccountBindingFingerprint,
			},
			OAuthBinding: mcpOAuthMetadata(),
		},
		resolver: resolver,
		newTransport: func(spec ServerSpec, _ string, _ []string) (mcpTransportClient, error) {
			created := &mcpAccountClientTestDouble{authorization: spec.Headers["Authorization"]}
			mu.Lock()
			requests = append(requests, created.authorization)
			clients = append(clients, created)
			mu.Unlock()
			return created, nil
		},
	}
	if _, err := client.CallTool("first", nil); err != nil {
		t.Fatalf("CallTool(first) error = %v", err)
	}
	resolver.replace(protectedMCPOAuthBundle(t, "mcp-oauth-bundle", "mcp", "server-a", "account-a", "server-a", "synthetic-generation-2", 0), 2)
	if _, err := client.CallTool("second", nil); err != nil {
		t.Fatalf("CallTool(second) error = %v", err)
	}
	if !reflect.DeepEqual(requests, []string{"Bearer synthetic-generation-1", "Bearer synthetic-generation-2"}) {
		t.Fatalf("effect Authorization values = %#v", requests)
	}
	for _, created := range clients {
		if !created.closed {
			t.Fatal("effect-bounded transport was retained after return")
		}
	}
	if client.spec.Headers["Authorization"] != "" {
		t.Fatal("long-lived MCP spec retained Authorization")
	}
	client.spec.AccountCredential.BindingFingerprint = strings.Repeat("d", 64)
	beforeBindingDrift := len(requests)
	if result, err := client.CallTool("binding-drift", nil); err == nil || result != nil {
		t.Fatalf("binding-drift CallTool result=%#v error=%v", result, err)
	}
	if len(requests) != beforeBindingDrift {
		t.Fatal("binding fingerprint drift caused a remote effect")
	}
	client.spec.AccountCredential.BindingFingerprint = mcpAccountBindingFingerprint

	resolver.revoke()
	before := len(requests)
	if result, err := client.CallTool("revoked", nil); err == nil || result != nil {
		t.Fatalf("revoked CallTool result=%#v error=%v", result, err)
	}
	if len(requests) != before {
		t.Fatal("revoked account caused a remote effect")
	}

	resolver.replace(protectedMCPOAuthBundle(t, "mcp-oauth-bundle", "mcp", "server-a", "account-a", "server-a", "synthetic-generation-3", 0), 3)
	client.newTransport = func(spec ServerSpec, _ string, _ []string) (mcpTransportClient, error) {
		created := &mcpAccountClientTestDouble{authorization: spec.Headers["Authorization"], onEffect: resolver.revoke}
		requests = append(requests, created.authorization)
		return created, nil
	}
	if result, err := client.CallTool("late", nil); err == nil || result != nil {
		t.Fatalf("late stale CallTool result=%#v error=%v", result, err)
	}
	if got := resolver.scopes; len(got) != 4 || !reflect.DeepEqual(got[0], scope) {
		t.Fatalf("resolved scopes = %#v", got)
	}
}

func TestExtensionAccountCredentialBindingRequiresVerifiedInstalledIdentity(t *testing.T) {
	pluginRoot := filepath.Join(t.TempDir(), "extension-a", "1.0.0")
	valid := ServerSpec{
		ID: "server-a", Transport: "http", URL: "https://extension.invalid/mcp",
		IdentitySource: "installed-plugin-manifest", PluginRootPath: pluginRoot,
		AccountCredential: &domainmcp.AccountCredentialScope{
			Owner: "extension", Provider: "extension-a", AccountID: "account-a",
			Purpose: "extension-provider-account-token", BindingFingerprint: mcpAccountBindingFingerprint,
		},
		OAuthBinding: mcpOAuthMetadata(),
	}
	if !validMCPAccountCredentialBinding(valid) {
		t.Fatal("verified installed extension account binding was rejected")
	}
	for _, mutate := range []func(*ServerSpec){
		func(spec *ServerSpec) { spec.AccountCredential.Owner = "mcp" },
		func(spec *ServerSpec) { spec.AccountCredential.Provider = "extension-b" },
		func(spec *ServerSpec) { spec.ID = "server-b"; spec.AccountCredential.Owner = "mcp" },
		func(spec *ServerSpec) { spec.IdentitySource = "" },
	} {
		candidate := cloneServerSpecForAuthority(valid)
		mutate(&candidate)
		if validMCPAccountCredentialBinding(candidate) {
			t.Fatalf("cross-owner extension binding was accepted: %#v", candidate.AccountCredential)
		}
	}

	scope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "extension", Provider: "extension-a", AccountID: "account-a",
		ChannelID: "server-a", Purpose: "extension-provider-account-token",
	}
	resolver := &mcpAccountResolverTestDouble{
		credential: []byte(protectedMCPOAuthBundle(t, "extension-oauth-bundle", "extension", "extension-a", "account-a", "server-a", "synthetic-extension-access", 0)),
		state:      accountState(scope, 1), current: true,
	}
	requests := 0
	client := &mcpAccountCredentialBoundClient{
		spec: valid, resolver: resolver,
		newTransport: func(spec ServerSpec, _ string, _ []string) (mcpTransportClient, error) {
			requests++
			if got := spec.Headers["Authorization"]; got != "Bearer synthetic-extension-access" {
				t.Fatalf("extension effect Authorization = %q", got)
			}
			return &mcpAccountClientTestDouble{}, nil
		},
	}
	if _, err := client.CallTool("extension-effect", nil); err != nil {
		t.Fatalf("extension OAuth effect error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("extension OAuth effect requests = %d", requests)
	}
	resolver.replace(protectedMCPOAuthBundle(t, "extension-oauth-bundle", "extension", "extension-b", "account-a", "server-a", "synthetic-cross-extension", 0), 2)
	if result, err := client.CallTool("cross-extension", nil); err == nil || result != nil {
		t.Fatalf("cross-extension OAuth result=%#v error=%v", result, err)
	}
	if requests != 1 {
		t.Fatalf("cross-extension OAuth caused %d remote requests", requests)
	}
}

func TestExpiredMCPOAuthBundleCausesZeroRemoteEffect(t *testing.T) {
	scope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "mcp", Provider: "server-expired", AccountID: "account-expired",
		ChannelID: "server-expired", Purpose: "mcp-oauth-access-token",
	}
	resolver := &mcpAccountResolverTestDouble{
		credential: []byte(protectedMCPOAuthBundle(
			t, "mcp-oauth-bundle", "mcp", "server-expired", "account-expired", "server-expired",
			"synthetic-expired", time.Now().Add(-time.Minute).UnixMilli(),
		)),
		state: accountState(scope, 1), current: true,
	}
	requests := 0
	client := &mcpAccountCredentialBoundClient{
		spec: ServerSpec{
			ID: "server-expired", Transport: "http", URL: "https://mcp.invalid/mcp",
			AccountCredential: &domainmcp.AccountCredentialScope{
				Owner: "mcp", Provider: "server-expired", AccountID: "account-expired", Purpose: "mcp-oauth-access-token",
				BindingFingerprint: mcpAccountBindingFingerprint,
			},
			OAuthBinding: mcpOAuthMetadata(),
		},
		resolver: resolver,
		newTransport: func(ServerSpec, string, []string) (mcpTransportClient, error) {
			requests++
			return &mcpAccountClientTestDouble{}, nil
		},
	}
	if _, err := client.CallTool("expired", nil); err == nil {
		t.Fatal("expired MCP OAuth bundle was accepted")
	}
	if requests != 0 {
		t.Fatalf("expired MCP OAuth bundle caused %d remote effects", requests)
	}
}

func TestMCPAccountCredentialConfigIsKeyFreeAndRejectsCrossOwnerBindings(t *testing.T) {
	document := []byte(`{"mcpServers":{"server-a":{"transport":"http","url":"https://mcp.invalid/mcp","trustScope":"user","accountCredential":{"owner":"mcp","provider":"server-a","accountId":"account-a","purpose":"mcp-oauth-access-token"}}}}`)
	specs, err := LoadMCPJSONDocument(document, t.TempDir())
	if err != nil || len(specs) != 1 || specs[0].AccountCredential == nil ||
		specs[0].AccountCredential.AccountID != "account-a" || len(specs[0].Headers) != 0 {
		t.Fatalf("LoadMCPJSONDocument() specs=%#v error=%v", specs, err)
	}
	oauthDocument := []byte(`{"mcpServers":{"server-a":{"transport":"http","url":"https://mcp.invalid/mcp","trustScope":"user","accountCredential":{"owner":"mcp","provider":"server-a","accountId":"account-a","purpose":"mcp-oauth-access-token","bindingFingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"oauthBinding":{"schemaVersion":1,"issuer":"https://issuer.invalid/","authorizationEndpoint":"https://login.invalid/authorize","tokenEndpoint":"https://tokens.invalid/token","clientId":"client-a","scopes":["openid"],"redirectModeVersion":1}}}}`)
	oauthSpecs, oauthErr := LoadMCPJSONDocument(oauthDocument, t.TempDir())
	if oauthErr != nil || len(oauthSpecs) != 1 || oauthSpecs[0].OAuthBinding == nil ||
		oauthSpecs[0].AccountCredential == nil ||
		oauthSpecs[0].AccountCredential.BindingFingerprint != mcpAccountBindingFingerprint {
		t.Fatalf("LoadMCPJSONDocument(OAuth) specs=%#v error=%v", oauthSpecs, oauthErr)
	}
	for _, invalid := range []string{
		`{"mcpServers":{"server-a":{"transport":"http","url":"https://mcp.invalid/mcp","trustScope":"user","headers":{"Authorization":"Bearer ordinary-config-token"},"accountCredential":{"owner":"mcp","provider":"server-a","accountId":"account-a","purpose":"mcp-oauth-access-token"}}}}`,
		`{"mcpServers":{"server-a":{"transport":"stdio","command":"server","trustScope":"user","accountCredential":{"owner":"mcp","provider":"server-a","accountId":"account-a","purpose":"mcp-oauth-access-token"}}}}`,
		`{"mcpServers":{"server-a":{"transport":"http","url":"https://mcp.invalid/mcp","trustScope":"user","accountCredential":{"owner":"mcp","provider":"server-b","accountId":"account-a","purpose":"mcp-oauth-access-token"}}}}`,
		`{"mcpServers":{"server-a":{"transport":"http","url":"https://mcp.invalid/mcp","trustScope":"user","accountCredential":{"owner":"extension","provider":"extension-a","accountId":"account-a","purpose":"extension-provider-account-token"}}}}`,
		`{"mcpServers":{"server-a":{"transport":"http","url":"https://mcp.invalid/mcp","trustScope":"user","oauthBinding":{"schemaVersion":1,"issuer":"https://issuer.invalid/","authorizationEndpoint":"https://login.invalid/authorize","tokenEndpoint":"https://tokens.invalid/token","clientId":"client-a","scopes":["openid"],"redirectModeVersion":1}}}}`,
	} {
		if _, err := LoadMCPJSONDocument([]byte(invalid), t.TempDir()); err == nil ||
			strings.Contains(err.Error(), "ordinary-config-token") {
			t.Fatalf("invalid MCP account credential config error=%v input=%s", err, invalid)
		}
	}
}
