package providerregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestPortableManifestV1ExportProjectionAndAtomicDestinationImport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRegistry := newMemoryRegistryStore()
	sourceSecrets := newMemorySecretStore()
	source := mustManager(t, sourceRegistry, sourceSecrets, nil)
	sourceProvider := connectMemoryProvider(
		t, ctx, source, sourceRegistry, "provider-portable-source", "synthetic-provider-secret-marker",
	)
	sourceBefore := sourceRegistry.snapshot()
	sourceSecretReadsBefore, _, _ := sourceSecrets.callCounts()

	exported, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest() error = %v", err)
	}
	var envelope map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(exported))
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("exported portable manifest is not JSON: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("exported portable manifest has trailing data: %v", err)
	}
	if len(envelope) == 0 || !bytes.Equal(exported, bytes.TrimSpace(exported)) {
		t.Fatalf("exported portable manifest is not a strict object: %s", exported)
	}
	if !portableManifestHasSchema(envelope, "analytix.provider-portable-manifest/v1") {
		t.Fatalf("exported portable manifest has no canonical v1 schema identifier: %s", exported)
	}
	for _, want := range []string{
		"https://provider.invalid/v1",
		"model-alpha",
		"media-alpha",
		"reentry_required",
	} {
		if !bytes.Contains(exported, []byte(want)) {
			t.Fatalf("exported portable manifest is missing key-free metadata %q: %s", want, exported)
		}
	}
	for _, forbidden := range []string{
		"synthetic-provider-secret-marker",
		`"credentialRef"`,
		`"credentialPurpose"`,
		`"revision"`,
		`"generation"`,
		`"incarnation"`,
		`"tombstone"`,
		`"transactions"`,
		`"selectedProviderId"`,
		`"recoveryCode"`,
		`"privateKey"`,
		`"accessToken"`,
		`"refreshToken"`,
		`"subscriptionToken"`,
		`"masterKey"`,
		`"deviceIdentity"`,
		`"trust"`,
		`"permissions"`,
		`"storePath"`,
		`"payload"`,
		`"userData"`,
	} {
		if bytes.Contains(exported, []byte(forbidden)) {
			t.Fatalf("exported portable manifest contains forbidden material %q: %s", forbidden, exported)
		}
	}
	if !reflect.DeepEqual(sourceBefore, sourceRegistry.snapshot()) {
		t.Fatal("ExportPortableManifest mutated the source Registry")
	}
	if sourceSecretReadsAfter, _, _ := sourceSecrets.callCounts(); sourceSecretReadsAfter != sourceSecretReadsBefore {
		t.Fatalf("ExportPortableManifest read Secret Store: before=%d after=%d", sourceSecretReadsBefore, sourceSecretReadsAfter)
	}
	destinationRegistry := newMemoryRegistryStore()
	destinationSecrets := newMemorySecretStore()
	destination := mustManager(t, destinationRegistry, destinationSecrets, nil)
	imported, err := destination.ImportPortableManifest(ctx, exported)
	if err != nil {
		t.Fatalf("ImportPortableManifest(valid) error = %v", err)
	}
	if imported.ProviderCount != 1 || imported.AccountCount != 0 || imported.ReentryRequired != 1 ||
		len(imported.Entries) != 1 || imported.Entries[0].Status != domainregistry.PortableManifestIntentReentryRequired {
		t.Fatalf("portable import result = %#v", imported)
	}
	destinationState := destinationRegistry.snapshot()
	if destinationState.SelectedProviderID != "" || len(destinationState.Transactions) != 0 {
		t.Fatalf("portable import selected a source Provider or left a transaction: %#v", destinationState)
	}
	if len(destinationState.Providers) != 1 {
		t.Fatalf("portable import Provider count = %d, want 1", len(destinationState.Providers))
	}
	beforeSelection := destinationRegistry.snapshot()
	for id, provider := range destinationState.Providers {
		if id == sourceProvider.ID || provider.ID == sourceProvider.ID {
			t.Fatalf("portable import reused source Provider identity: %#v", provider)
		}
		if provider.CredentialRef != "" || provider.CredentialPurpose != "" || len(provider.SelectedRoutes) != 1 ||
			!strings.HasPrefix(provider.SelectedRoutes[0], domainregistry.ExactProviderRoutePrefix) ||
			strings.Contains(provider.SelectedRoutes[0], sourceProvider.ID) {
			t.Fatalf("portable import created an executable or selected Provider: %#v", provider)
		}
		if _, err := destination.Select(ctx, SelectCommand{
			Expected: domainregistry.ExpectedState{
				RegistryRevision: destinationState.Revision, RegistryIncarnation: destinationState.Incarnation,
				ProviderRevision: provider.Revision, ProviderGeneration: provider.Generation,
				ProviderIncarnation: provider.Incarnation,
			},
			ProviderID: id,
		}); err == nil {
			t.Fatalf("portable import allowed selecting an uncredentialed Provider: %#v", provider)
		}
		if !reflect.DeepEqual(beforeSelection, destinationRegistry.snapshot()) {
			t.Fatal("selecting an uncredentialed imported Provider mutated the Registry")
		}
	}
	if reads, _, _ := destinationSecrets.callCounts(); reads != 0 {
		t.Fatalf("portable import/select performed a Secret Store read before K2 credential commit: %d", reads)
	}
	if _, err := destination.ResolveSelectedForExecution(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ResolveSelectedForExecution(imported) error = %v, want verification failure", err)
	}

	collisionRegistry := newMemoryRegistryStore()
	collisionSecrets := newMemorySecretStore()
	collisionManager := mustManager(t, collisionRegistry, collisionSecrets, nil)
	connectMemoryProvider(
		t, ctx, collisionManager, collisionRegistry, "provider-destination-existing", "synthetic-destination-secret-marker",
	)
	collisionBefore := collisionRegistry.snapshot()
	collisionSecretCount := collisionSecrets.recordCount()
	collisionSecretReads := func() int { reads, _, _ := collisionSecrets.callCounts(); return reads }()
	if _, err := collisionManager.ImportPortableManifest(ctx, exported); err == nil {
		t.Fatal("portable import accepted a canonical Provider collision")
	}
	if !reflect.DeepEqual(collisionBefore, collisionRegistry.snapshot()) {
		t.Fatal("portable import mutated the destination Registry after a collision")
	}
	if collisionSecrets.recordCount() != collisionSecretCount {
		t.Fatal("portable import mutated destination Secret Store after a collision")
	}
	if reads, _, _ := collisionSecrets.callCounts(); reads != collisionSecretReads {
		t.Fatalf("portable collision preflight read Secret Store: before=%d after=%d", collisionSecretReads, reads)
	}

	blockedRegistry := newMemoryRegistryStore()
	blockedSecrets := newMemorySecretStore()
	blocked := mustManager(t, blockedRegistry, blockedSecrets, nil)
	blockedBefore := blockedRegistry.snapshot()
	for _, testCase := range []struct {
		name string
		body []byte
	}{
		{name: "duplicate key", body: duplicatePortableManifestFirstField(exported)},
		{name: "secret canary", body: appendPortableManifestField(exported, `"credentialRef":"synthetic-provider-secret-marker"`)},
		{name: "path bearing", body: appendPortableManifestField(exported, `"storePath":"../../private"`)},
		{name: "legacy schema", body: bytes.Replace(exported, []byte("analytix.provider-portable-manifest/v1"), []byte("legacy-provider-manifest/v0"), 1)},
		{name: "noncanonical correlation", body: bytes.Replace(exported, []byte(`"correlation":"provider-0"`), []byte(`"correlation":"alpha"`), 1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := blocked.ImportPortableManifest(ctx, testCase.body); err == nil {
				t.Fatal("portable import accepted malformed or security-sensitive input")
			}
			if !reflect.DeepEqual(blockedBefore, blockedRegistry.snapshot()) {
				t.Fatal("portable import mutated the Registry before rejecting input")
			}
			if blockedSecrets.recordCount() != 0 {
				t.Fatal("portable import wrote a credential before rejecting input")
			}
		})
	}
}

func TestPortableManifestV1EmptyImportReturnsEmptyEntryArray(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	body, err := domainregistry.MarshalPortableManifestV1(domainregistry.PortableManifestV1{
		Schema:    domainregistry.PortableManifestSchemaV1,
		Providers: []domainregistry.PortableProviderDescriptorV1{},
		Accounts:  []domainregistry.PortableAccountDescriptorV1{},
	})
	if err != nil {
		t.Fatalf("MarshalPortableManifestV1() error = %v", err)
	}
	result, err := manager.ImportPortableManifest(ctx, body)
	if err != nil {
		t.Fatalf("ImportPortableManifest(empty) error = %v", err)
	}
	if result.Entries == nil || len(result.Entries) != 0 {
		t.Fatalf("ImportPortableManifest(empty) entries = %#v, want a non-nil empty array", result.Entries)
	}
	if reads, _, _ := secrets.callCounts(); reads != 0 {
		t.Fatalf("ImportPortableManifest(empty) read Secret Store %d times", reads)
	}
}

func portableManifestHasSchema(envelope map[string]json.RawMessage, want string) bool {
	for _, raw := range envelope {
		var value string
		if json.Unmarshal(raw, &value) == nil && value == want {
			return true
		}
	}
	return false
}

func appendPortableManifestField(body []byte, field string) []byte {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) < 2 || trimmed[len(trimmed)-1] != '}' {
		return append([]byte(nil), body...)
	}
	result := append([]byte(nil), trimmed[:len(trimmed)-1]...)
	if len(result) > 0 && result[len(result)-1] != '{' {
		result = append(result, ',')
	}
	result = append(result, []byte(field)...)
	return append(result, '}')
}

func duplicatePortableManifestFirstField(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return append([]byte(nil), body...)
	}
	field, err := decoder.Token()
	if err != nil {
		return append([]byte(nil), body...)
	}
	key, ok := field.(string)
	if !ok {
		return append([]byte(nil), body...)
	}
	quoted, err := json.Marshal(key)
	if err != nil {
		return append([]byte(nil), body...)
	}
	result := append([]byte{'{'}, quoted...)
	result = append(result, []byte(":null,")...)
	return append(result, trimmed[1:]...)
}

func TestPortableManifestV1PreservesOrderedRoutesAndPublicMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	oauthBinding := &domainregistry.OAuthBindingMetadata{
		SchemaVersion: 1, Issuer: "https://issuer.example/oauth", AuthorizationEndpoint: "https://issuer.example/oauth/authorize",
		TokenEndpoint: "https://issuer.example/oauth/token", RevocationEndpoint: "https://issuer.example/oauth/revoke",
		ClientID: "public-client-secret-token-label", Scopes: []string{"scope-profile", "scope-usage"}, RedirectModeVersion: 1,
	}
	observation := &domainregistry.AccountObservationBinding{
		SchemaVersion: 1, Endpoint: "https://billing.example/v1/quota", Method: "GET", Projection: "normalized-quota-v1",
	}
	alpha := connectPortableTestProvider(t, ctx, manager, registry, domainregistry.ProviderInput{
		ID: "provider-route-alpha", Kind: "openai-compatible", Endpoint: "https://alpha.provider.invalid/v1",
		Models: []string{"model-secret-token", "model-alpha"}, MediaModels: []string{"media-alpha"},
		SelectedModel: "model-secret-token", SelectedMedia: "media-alpha", SelectedRoutes: []string{"primary"},
		OAuthBinding: oauthBinding, AccountObservation: observation,
	}, "synthetic-route-alpha")
	beta := connectPortableTestProvider(t, ctx, manager, registry, domainregistry.ProviderInput{
		ID: "provider-route-beta", Kind: "openai-compatible", Endpoint: "https://beta.provider.invalid/v1",
		Models: []string{"model-beta"}, MediaModels: []string{}, SelectedModel: "model-beta", SelectedRoutes: []string{"primary"},
	}, "synthetic-route-beta")
	alphaState := registry.snapshot()
	updated, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(alphaState, alpha.ID),
		Provider: domainregistry.ProviderInput{
			ID: alpha.ID, Kind: alpha.Kind, Endpoint: alpha.Endpoint, Models: alpha.Models, MediaModels: alpha.MediaModels,
			SelectedModel: alpha.SelectedModel, SelectedMedia: alpha.SelectedMedia,
			SelectedRoutes: []string{domainregistry.ExactProviderRoutePrefix + beta.ID, domainregistry.PrimaryRouteAlias},
			OAuthBinding:   alpha.OAuthBinding, AccountObservation: alpha.AccountObservation,
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(ordered route) error = %v", err)
	}
	if updated.SelectedRoutes[0] != domainregistry.ExactProviderRoutePrefix+beta.ID || updated.SelectedRoutes[1] != domainregistry.PrimaryRouteAlias {
		t.Fatalf("source route order = %#v", updated.SelectedRoutes)
	}

	exported, err := manager.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest() error = %v", err)
	}
	manifest, err := domainregistry.ParsePortableManifestV1(exported)
	if err != nil {
		t.Fatalf("ParsePortableManifestV1(exported) error = %v", err)
	}
	correlationByEndpoint := make(map[string]string, len(manifest.Providers))
	for _, descriptor := range manifest.Providers {
		correlationByEndpoint[descriptor.Endpoint] = descriptor.Correlation
		if strings.Contains(descriptor.Endpoint, alpha.ID) || strings.Contains(descriptor.Endpoint, beta.ID) {
			t.Fatal("exported endpoint contains a source Provider identity")
		}
	}
	alphaDescriptor := manifest.Providers[0]
	betaDescriptor := manifest.Providers[1]
	if alphaDescriptor.Endpoint != alpha.Endpoint || betaDescriptor.Endpoint != beta.Endpoint ||
		!reflect.DeepEqual(alphaDescriptor.OAuthBinding, oauthBinding) ||
		!reflect.DeepEqual(alphaDescriptor.AccountObservation, observation) ||
		!slices.Equal(alphaDescriptor.Models, []string{"model-alpha", "model-secret-token"}) {
		t.Fatalf("exported public metadata = %#v", manifest.Providers)
	}
	if !slices.Equal(alphaDescriptor.Routes, []string{betaDescriptor.Correlation, alphaDescriptor.Correlation}) {
		t.Fatalf("exported ordered route correlations = %#v, want [%s %s]", alphaDescriptor.Routes, betaDescriptor.Correlation, alphaDescriptor.Correlation)
	}
	if len(betaDescriptor.Routes) != 1 || betaDescriptor.Routes[0] != betaDescriptor.Correlation {
		t.Fatalf("exported default route correlation = %#v", betaDescriptor.Routes)
	}

	destinationRegistry := newMemoryRegistryStore()
	destinationSecrets := newMemorySecretStore()
	destination := mustManager(t, destinationRegistry, destinationSecrets, nil)
	result, err := destination.ImportPortableManifest(ctx, exported)
	if err != nil {
		t.Fatalf("ImportPortableManifest() error = %v", err)
	}
	if len(result.Entries) != 2 || result.ProviderCount != 2 || result.AccountCount != 0 || result.ReentryRequired != 2 {
		t.Fatalf("ImportPortableManifest() result = %#v", result)
	}
	destinationByCorrelation := make(map[string]string, len(result.Entries))
	for _, entry := range result.Entries {
		destinationByCorrelation[entry.Correlation] = entry.DestinationProviderID
		if entry.Status != domainregistry.PortableManifestIntentReentryRequired || entry.DestinationProviderID == alpha.ID || entry.DestinationProviderID == beta.ID {
			t.Fatalf("import mapping transfers source authority: %#v", entry)
		}
	}
	destinationState := destinationRegistry.snapshot()
	var importedAlpha, importedBeta domainregistry.Provider
	for _, provider := range destinationState.Providers {
		switch provider.Endpoint {
		case alpha.Endpoint:
			importedAlpha = provider
		case beta.Endpoint:
			importedBeta = provider
		}
	}
	if importedAlpha.ID == "" || importedBeta.ID == "" ||
		!slices.Equal(importedAlpha.SelectedRoutes, []string{
			domainregistry.ExactProviderRoutePrefix + destinationByCorrelation[betaDescriptor.Correlation],
			domainregistry.ExactProviderRoutePrefix + destinationByCorrelation[alphaDescriptor.Correlation],
		}) || importedAlpha.CredentialRef != "" || importedBeta.CredentialRef != "" ||
		!reflect.DeepEqual(importedAlpha.OAuthBinding, oauthBinding) || !reflect.DeepEqual(importedAlpha.AccountObservation, observation) {
		t.Fatalf("imported route/metadata authority = alpha=%#v beta=%#v", importedAlpha, importedBeta)
	}
	if destinationState.SelectedProviderID != "" {
		t.Fatalf("portable import changed Registry.SelectedProviderID = %q", destinationState.SelectedProviderID)
	}
	readsBefore, _, _ := destinationSecrets.callCounts()
	beforeSelect := destinationRegistry.snapshot()
	if _, err := destination.Select(ctx, SelectCommand{Expected: expectedFor(destinationState, importedAlpha.ID), ProviderID: importedAlpha.ID}); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("Select(imported route pool) error = %v, want verification failure", err)
	}
	if readsAfter, _, _ := destinationSecrets.callCounts(); readsAfter != readsBefore {
		t.Fatalf("Select(imported route pool) read Secret Store: before=%d after=%d", readsBefore, readsAfter)
	}
	if !reflect.DeepEqual(beforeSelect, destinationRegistry.snapshot()) {
		t.Fatal("failed imported route admission mutated Registry")
	}
}

func TestPortableManifestV1EmptyRoutesRemainDefaultDuringProtectedPrepare(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRegistry := newMemoryRegistryStore()
	source := mustManager(t, sourceRegistry, newMemorySecretStore(), nil)
	connectPortableTestProvider(t, ctx, source, sourceRegistry, domainregistry.ProviderInput{
		ID: "provider-default-route-source", Kind: "openai-compatible", Endpoint: "https://default-route.example/v1",
		Models: []string{"model-default"}, MediaModels: []string{}, SelectedModel: "model-default",
		SelectedRoutes: []string{},
	}, "synthetic-default-route-secret")
	manifest, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest() error = %v", err)
	}
	parsed, err := domainregistry.ParsePortableManifestV1(manifest)
	if err != nil || len(parsed.Providers) != 1 || len(parsed.Providers[0].Routes) != 0 {
		t.Fatalf("portable default route projection = %#v, err=%v", parsed.Providers, err)
	}

	destinationRegistry := newMemoryRegistryStore()
	destination := mustManager(t, destinationRegistry, newMemorySecretStore(), nil)
	imported, err := destination.ImportPortableManifest(ctx, manifest)
	if err != nil {
		t.Fatalf("ImportPortableManifest() error = %v", err)
	}
	privateImported := protectedRecoveryPrivateImportResult(
		t, destinationRegistry.snapshot(), manifest, imported, nil,
	)
	prepared, err := destination.PrepareProtectedRecoveryRequest(ctx, ProtectedRecoveryRequestCommand{
		ManifestJSON: manifest, ImportResult: privateImported,
		LocalBinding: domainregistry.ProtectedRecoveryLocalBindingV1{
			BrowserWindowID: "synthetic-default-route-window", MainFrameID: "synthetic-default-route-frame",
			ProfileBinding: "synthetic-default-route-profile", DataDirectoryBinding: "/isolated/default-route-data",
		},
	})
	if err != nil {
		t.Fatalf("PrepareProtectedRecoveryRequest(default routes) error = %v", err)
	}
	if _, err := destination.RollbackProtectedRecovery(ctx, ProtectedRecoveryRollbackCommand{Request: prepared.Request}); err != nil {
		t.Fatalf("RollbackProtectedRecovery(default routes) error = %v", err)
	}
	for _, provider := range destinationRegistry.snapshot().Providers {
		if len(provider.SelectedRoutes) != 0 {
			t.Fatalf("imported default route became an explicit route: %#v", provider.SelectedRoutes)
		}
	}
}

func TestPortableManifestV1MintsPrivateAccountScopeIdentity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRegistry := newMemoryRegistryStore()
	sourceSecrets := newMemorySecretStore()
	source := mustManager(t, sourceRegistry, sourceSecrets, nil)
	sourceScope := &domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "mcp", Provider: "mcp-source-owner", AccountID: "source-account", ChannelID: "source-channel",
		Purpose: "mcp-oauth-access-token",
	}
	credential, _ := secretstoreport.SetCredential([]byte("synthetic-mcp-account-secret"))
	account, err := source.Connect(ctx, ConnectCommand{
		Expected: expectedForNewPortableTestProvider(sourceRegistry.snapshot()),
		Provider: domainregistry.ProviderInput{ID: "provider-private-account-source", Kind: domainregistry.PrivateAccountKind,
			Endpoint: "https://mcp.example/v1", PrivateAccount: sourceScope},
		CredentialPurpose: secretstoreport.Purpose(sourceScope.Purpose), Credential: credential,
	})
	if err != nil {
		t.Fatalf("Connect(private account) error = %v", err)
	}
	exported, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest(private account) error = %v", err)
	}
	manifest, err := domainregistry.ParsePortableManifestV1(exported)
	if err != nil || len(manifest.Accounts) != 1 {
		t.Fatalf("exported private account manifest = %#v, err=%v", manifest, err)
	}
	if manifest.Accounts[0].Provider != sourceScope.Provider {
		t.Fatalf("portable owner descriptor = %q, want source descriptor", manifest.Accounts[0].Provider)
	}
	destinationRegistry := newMemoryRegistryStore()
	destinationSecrets := newMemorySecretStore()
	destination := mustManager(t, destinationRegistry, destinationSecrets, nil)
	result, err := destination.ImportPortableManifest(ctx, exported)
	if err != nil {
		t.Fatalf("ImportPortableManifest(private account) error = %v", err)
	}
	if result.ProviderCount != 0 || result.AccountCount != 1 || len(result.Entries) != 1 {
		t.Fatalf("private account import result = %#v", result)
	}
	state := destinationRegistry.snapshot()
	if len(state.Providers) != 1 {
		t.Fatalf("destination private account count = %d", len(state.Providers))
	}
	for _, provider := range state.Providers {
		if provider.ID == account.ID || provider.CredentialRef != "" || provider.CredentialPurpose != "" || provider.PrivateAccount == nil ||
			provider.PrivateAccount.Provider == sourceScope.Provider || provider.PrivateAccount.AccountID == sourceScope.AccountID ||
			provider.PrivateAccount.ChannelID == sourceScope.ChannelID || provider.PrivateAccount.Provider == manifest.Accounts[0].Provider {
			t.Fatalf("destination private account reused source authority: %#v", provider)
		}
		if provider.Revision != 1 || provider.Generation != 1 {
			t.Fatalf("destination private account fence = revision=%d generation=%d", provider.Revision, provider.Generation)
		}
	}
	if reads, _, _ := destinationSecrets.callCounts(); reads != 0 {
		t.Fatalf("private account import read Secret Store %d times", reads)
	}
}

func TestPortableManifestV1ImportedPrivateAliasIsUsedByAccountLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRegistry := newMemoryRegistryStore()
	source := mustManager(t, sourceRegistry, newMemorySecretStore(), nil)
	scope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "mcp", Provider: "mcp-alias-owner", AccountID: "mcp-alias-account",
		ChannelID: "mcp-alias-channel", Purpose: "mcp-oauth-access-token",
	}
	credential, err := secretstoreport.SetCredential([]byte("synthetic-private-alias-source"))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	if _, err := source.Connect(ctx, ConnectCommand{
		Expected: expectedForNewPortableTestProvider(sourceRegistry.snapshot()),
		Provider: domainregistry.ProviderInput{ID: "provider-private-alias-source", Kind: domainregistry.PrivateAccountKind,
			Endpoint: "https://mcp-alias.example/v1", PrivateAccount: &scope},
		CredentialPurpose: secretstoreport.Purpose(scope.Purpose), Credential: credential,
	}); err != nil {
		t.Fatalf("Connect(private alias source) error = %v", err)
	}
	manifest, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest() error = %v", err)
	}
	destinationRegistry := newMemoryRegistryStore()
	destination := mustManager(t, destinationRegistry, newMemorySecretStore(), nil)
	result, err := destination.ImportPortableManifest(ctx, manifest)
	if err != nil || len(result.Entries) != 1 {
		t.Fatalf("ImportPortableManifest() = %#v, %v", result, err)
	}
	importedID := result.Entries[0].DestinationProviderID
	placeholder := destinationRegistry.snapshot().Providers[importedID]
	if placeholder.PrivateAccount == nil || placeholder.ID == "provider-private-alias-source" {
		t.Fatalf("portable alias placeholder = %#v", placeholder)
	}
	absent, err := destination.AccountCredentialState(ctx, *placeholder.PrivateAccount)
	if err != nil || absent.Status != AccountCredentialStatusAbsent || absent.ProviderID != importedID {
		t.Fatalf("AccountCredentialState(imported alias) = %#v, %v", absent, err)
	}
	ready, err := destination.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: *placeholder.PrivateAccount, Expected: absent.Expected(), Credential: []byte("synthetic-private-alias-destination"),
	})
	if err != nil || ready.Status != AccountCredentialStatusReady || ready.ProviderID != importedID {
		t.Fatalf("PutAccountCredential(imported alias) = %#v, %v", ready, err)
	}
	resolved, err := destination.ResolveAccountCredential(ctx, *placeholder.PrivateAccount)
	if err != nil || !bytes.Equal(resolved.Credential, []byte("synthetic-private-alias-destination")) {
		resolved.Clear()
		t.Fatalf("ResolveAccountCredential(imported alias) = %#v, %v", resolved.State, err)
	}
	resolved.Clear()
}

func TestPortableManifestV1StableIdentityRejectsMutableDifferences(t *testing.T) {
	t.Parallel()

	providerBase := domainregistry.PortableProviderDescriptorV1{
		Correlation: "provider-0", Kind: "openai-compatible", Endpoint: "https://identity.example/v1",
		Models: []string{"model-a"}, MediaModels: []string{"media-a"}, SelectedModel: "model-a", Routes: []string{},
		Intent: domainregistry.PortableManifestIntentReentryRequired,
	}
	providerVariants := []domainregistry.PortableProviderDescriptorV1{
		{Correlation: "provider-1", Kind: providerBase.Kind, Endpoint: providerBase.Endpoint, Proxy: "http://proxy.identity.invalid:8080", Models: providerBase.Models, MediaModels: providerBase.MediaModels, SelectedModel: providerBase.SelectedModel, Routes: []string{}, Intent: providerBase.Intent},
		{Correlation: "provider-1", Kind: providerBase.Kind, Endpoint: providerBase.Endpoint, Models: []string{"model-a", "model-b"}, MediaModels: providerBase.MediaModels, SelectedModel: providerBase.SelectedModel, Routes: []string{}, Intent: providerBase.Intent},
		{Correlation: "provider-1", Kind: providerBase.Kind, Endpoint: providerBase.Endpoint, Models: providerBase.Models, MediaModels: providerBase.MediaModels, SelectedModel: "", Routes: []string{}, Intent: providerBase.Intent},
	}
	baseIdentity, err := providerBase.CanonicalIdentity()
	if err != nil {
		t.Fatalf("provider base identity error = %v", err)
	}
	for index, variant := range providerVariants {
		identity, err := variant.CanonicalIdentity()
		if err != nil || identity != baseIdentity {
			t.Fatalf("provider identity variant %d = %q, err=%v; want %q", index, identity, err, baseIdentity)
		}
		manifest := domainregistry.PortableManifestV1{Schema: domainregistry.PortableManifestSchemaV1, Providers: []domainregistry.PortableProviderDescriptorV1{providerBase, variant}, Accounts: []domainregistry.PortableAccountDescriptorV1{}}
		if err := manifest.Validate(); err == nil {
			t.Fatalf("provider mutable identity variant %d was accepted", index)
		}
	}
	accountBase := domainregistry.PortableAccountDescriptorV1{
		Correlation: "account-0", Owner: "mcp", Provider: "mcp-owner-secret-token", Endpoint: "https://account-one.example/v1",
		Purpose: "mcp-oauth-access-token", Intent: domainregistry.PortableManifestIntentReentryRequired,
	}
	accountVariant := accountBase
	accountVariant.Correlation = "account-1"
	accountVariant.Endpoint = "https://account-two.example/v1"
	baseAccountIdentity, err := accountBase.CanonicalIdentity()
	if err != nil {
		t.Fatalf("account base identity error = %v", err)
	}
	variantAccountIdentity, err := accountVariant.CanonicalIdentity()
	if err != nil || variantAccountIdentity != baseAccountIdentity {
		t.Fatalf("account endpoint changed canonical identity: %q vs %q, err=%v", variantAccountIdentity, baseAccountIdentity, err)
	}
	manifest := domainregistry.PortableManifestV1{Schema: domainregistry.PortableManifestSchemaV1, Providers: []domainregistry.PortableProviderDescriptorV1{}, Accounts: []domainregistry.PortableAccountDescriptorV1{accountBase, accountVariant}}
	if err := manifest.Validate(); err == nil {
		t.Fatal("account mutable endpoint identity collision was accepted")
	}
}

func TestPortableManifestV1NormalizesOAuthObservationAndRejectsUnsafeMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	oauthBinding := &domainregistry.OAuthBindingMetadata{
		SchemaVersion: 1, Issuer: "HTTPS://ISSUER.example/oauth",
		AuthorizationEndpoint: "HTTPS://ISSUER.example/oauth/authorize",
		TokenEndpoint:         "HTTPS://ISSUER.example/oauth/token",
		RevocationEndpoint:    "HTTPS://ISSUER.example/oauth/revoke",
		ClientID:              "public-client/alpha<>&", Scopes: []string{"https://www.googleapis.com/auth/drive.readonly", "scope-secret-token"}, RedirectModeVersion: 1,
	}
	observation := &domainregistry.AccountObservationBinding{
		SchemaVersion: 1, Endpoint: "HTTPS://BILLING.example/v1/quota", Method: "GET", Projection: "normalized-quota-v1",
	}
	connectPortableTestProvider(t, ctx, manager, registry, domainregistry.ProviderInput{
		ID: "provider-portable-oauth", Kind: "openai-compatible", Endpoint: "https://oauth.provider.invalid/v1",
		Models: []string{"model<alpha>&"}, MediaModels: []string{}, SelectedModel: "model<alpha>&",
		SelectedRoutes: []string{}, OAuthBinding: oauthBinding, AccountObservation: observation,
	}, "synthetic-portable-oauth")
	exported, err := manager.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest() error = %v", err)
	}
	manifest, err := domainregistry.ParsePortableManifestV1(exported)
	if err != nil {
		t.Fatalf("ParsePortableManifestV1(exported) error = %v", err)
	}
	if len(manifest.Providers) != 1 || manifest.Providers[0].OAuthBinding == nil || manifest.Providers[0].AccountObservation == nil {
		t.Fatalf("portable OAuth/observation metadata = %#v", manifest.Providers)
	}
	normalizedOAuth := manifest.Providers[0].OAuthBinding
	if normalizedOAuth.Issuer != "https://issuer.example/oauth" ||
		normalizedOAuth.AuthorizationEndpoint != "https://issuer.example/oauth/authorize" ||
		normalizedOAuth.TokenEndpoint != "https://issuer.example/oauth/token" ||
		normalizedOAuth.RevocationEndpoint != "https://issuer.example/oauth/revoke" ||
		normalizedOAuth.ClientID != "public-client/alpha<>&" ||
		!slices.Equal(normalizedOAuth.Scopes, []string{"https://www.googleapis.com/auth/drive.readonly", "scope-secret-token"}) {
		t.Fatalf("portable OAuth normalization = %#v", normalizedOAuth)
	}
	if manifest.Providers[0].AccountObservation.Endpoint != "https://billing.example/v1/quota" ||
		!bytes.Contains(exported, []byte("model<alpha>&")) {
		t.Fatalf("portable observation or HTML-safe metadata = %#v; bytes=%s", manifest.Providers[0].AccountObservation, exported)
	}

	baseOAuth := &domainregistry.OAuthBindingMetadata{
		SchemaVersion: 1, Issuer: "https://issuer.example/oauth",
		AuthorizationEndpoint: "https://issuer.example/oauth/authorize",
		TokenEndpoint:         "https://issuer.example/oauth/token",
		ClientID:              "portable-client", Scopes: []string{"scope-a"}, RedirectModeVersion: 1,
	}
	baseDescriptor := domainregistry.PortableProviderDescriptorV1{
		Correlation: "provider-0", Kind: "openai-compatible", Endpoint: "https://unsafe-metadata.example/v1",
		Models: []string{}, MediaModels: []string{}, Routes: []string{}, OAuthBinding: baseOAuth,
		Intent: domainregistry.PortableManifestIntentReentryRequired,
	}
	unsafeVariants := map[string]func(*domainregistry.PortableProviderDescriptorV1){
		"client parent traversal": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = "../client"
			descriptor.OAuthBinding = &binding
		},
		"client absolute path": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = "/absolute"
			descriptor.OAuthBinding = &binding
		},
		"client home path": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = "~/client"
			descriptor.OAuthBinding = &binding
		},
		"client backslash traversal": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = "..\\client"
			descriptor.OAuthBinding = &binding
		},
		"client control": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = "client\x00id"
			descriptor.OAuthBinding = &binding
		},
		"client over-bound": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = strings.Repeat("c", 257)
			descriptor.OAuthBinding = &binding
		},
		"scope parent traversal": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.Scopes = []string{"../scope"}
			descriptor.OAuthBinding = &binding
		},
		"scope absolute path": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.Scopes = []string{"/scope"}
			descriptor.OAuthBinding = &binding
		},
		"scope home path": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.Scopes = []string{"~/scope"}
			descriptor.OAuthBinding = &binding
		},
		"scope backslash traversal": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.Scopes = []string{"..\\scope"}
			descriptor.OAuthBinding = &binding
		},
		"scope control": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.Scopes = []string{"scope\x00id"}
			descriptor.OAuthBinding = &binding
		},
		"scope over-bound": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.Scopes = []string{strings.Repeat("s", 257)}
			descriptor.OAuthBinding = &binding
		},
		"observation endpoint over-bound": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			descriptor.AccountObservation = &domainregistry.AccountObservationBinding{
				SchemaVersion: 1, Endpoint: "https://billing.example/" + strings.Repeat("q", 2048),
				Method: "GET", Projection: "normalized-quota-v1",
			}
		},
		"metadata line separator": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = "client\u2028id"
			descriptor.OAuthBinding = &binding
		},
		"metadata paragraph separator": func(descriptor *domainregistry.PortableProviderDescriptorV1) {
			binding := *descriptor.OAuthBinding
			binding.ClientID = "client\u2029id"
			descriptor.OAuthBinding = &binding
		},
	}
	for name, mutate := range unsafeVariants {
		descriptor := baseDescriptor
		binding := *baseOAuth
		binding.Scopes = slices.Clone(baseOAuth.Scopes)
		descriptor.OAuthBinding = &binding
		mutate(&descriptor)
		manifest := domainregistry.PortableManifestV1{
			Schema:    domainregistry.PortableManifestSchemaV1,
			Providers: []domainregistry.PortableProviderDescriptorV1{descriptor},
			Accounts:  []domainregistry.PortableAccountDescriptorV1{},
		}
		if _, err := domainregistry.MarshalPortableManifestV1(manifest); err == nil {
			t.Errorf("MarshalPortableManifestV1(%s) accepted unsafe OAuth/observation metadata", name)
		}
	}
}

func TestPortableManifestV1RejectsExistingStableIdentityDuplicatesPreMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	duplicatePublicProviders := []domainregistry.Provider{
		{
			ID: "provider-existing-public-a", Kind: "openai-compatible", Endpoint: "https://duplicate-existing.example/v1",
			Models: []string{"model-a"}, MediaModels: []string{}, SelectedRoutes: []string{}, Revision: 1, Generation: 1,
			Incarnation: "inc_" + strings.Repeat("c", 43),
		},
		{
			ID: "provider-existing-public-b", Kind: "openai-compatible", Endpoint: "https://duplicate-existing.example/v1",
			Proxy: "http://proxy.duplicate-existing.example:8080", Models: []string{"model-b"}, MediaModels: []string{},
			SelectedRoutes: []string{}, Revision: 1, Generation: 1, Incarnation: "inc_" + strings.Repeat("d", 43),
		},
	}
	duplicatePrivateProviders := []domainregistry.Provider{
		{
			ID: "provider-existing-private-a", Kind: domainregistry.PrivateAccountKind, Endpoint: "https://mcp-existing-a.example/v1",
			Models: []string{}, MediaModels: []string{}, SelectedRoutes: []string{}, Revision: 1, Generation: 1,
			Incarnation: "inc_" + strings.Repeat("e", 43), PrivateAccount: &domainregistry.PrivateAccountScope{
				SchemaVersion: 1, Owner: "mcp", Provider: "mcp-existing-owner", AccountID: "existing-account-a",
				ChannelID: "existing-channel-a", Purpose: "mcp-oauth-access-token",
			},
		},
		{
			ID: "provider-existing-private-b", Kind: domainregistry.PrivateAccountKind, Endpoint: "https://mcp-existing-b.example/v1",
			Models: []string{}, MediaModels: []string{}, SelectedRoutes: []string{}, Revision: 1, Generation: 1,
			Incarnation: "inc_" + strings.Repeat("f", 43), PrivateAccount: &domainregistry.PrivateAccountScope{
				SchemaVersion: 1, Owner: "mcp", Provider: "mcp-existing-owner", AccountID: "existing-account-b",
				ChannelID: "existing-channel-b", Purpose: "mcp-oauth-access-token",
			},
		},
	}
	cases := []struct {
		name      string
		providers []domainregistry.Provider
		manifest  domainregistry.PortableManifestV1
	}{
		{
			name:      "public mutable variant",
			providers: duplicatePublicProviders,
			manifest: domainregistry.PortableManifestV1{
				Schema: domainregistry.PortableManifestSchemaV1,
				Providers: []domainregistry.PortableProviderDescriptorV1{{
					Correlation: "provider-0", Kind: "openai-compatible", Endpoint: "https://new-provider.example/v1",
					Models: []string{}, MediaModels: []string{}, Routes: []string{}, Intent: domainregistry.PortableManifestIntentReentryRequired,
				}}, Accounts: []domainregistry.PortableAccountDescriptorV1{},
			},
		},
		{
			name:      "private owner tuple variant",
			providers: duplicatePrivateProviders,
			manifest: domainregistry.PortableManifestV1{
				Schema:    domainregistry.PortableManifestSchemaV1,
				Providers: []domainregistry.PortableProviderDescriptorV1{},
				Accounts: []domainregistry.PortableAccountDescriptorV1{{
					Correlation: "account-0", Owner: "mcp", Provider: "mcp-new-owner", Endpoint: "https://mcp-new.example/v1",
					Purpose: "mcp-oauth-access-token", Intent: domainregistry.PortableManifestIntentReentryRequired,
				}},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newMemoryRegistryStore()
			registry.mutate(func(state *domainregistry.Registry) {
				state.Revision = 2
				for _, provider := range testCase.providers {
					state.Providers[provider.ID] = provider
				}
			})
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			body, err := domainregistry.MarshalPortableManifestV1(testCase.manifest)
			if err != nil {
				t.Fatalf("MarshalPortableManifestV1() error = %v", err)
			}
			before := registry.snapshot()
			if _, err := manager.ImportPortableManifest(ctx, body); !errors.Is(err, registryport.ErrConflict) {
				t.Fatalf("ImportPortableManifest() error = %v, want conflict", err)
			}
			if !reflect.DeepEqual(before, registry.snapshot()) {
				t.Fatal("duplicate existing identity preflight changed the Registry")
			}
			if reads, _, _ := secrets.callCounts(); reads != 0 {
				t.Fatalf("duplicate existing identity preflight read Secret Store %d times", reads)
			}
			if secrets.recordCount() != 0 {
				t.Fatal("duplicate existing identity preflight wrote Secret Store state")
			}
		})
	}
}

func TestPortableManifestV1CommitFailureCapacityAndCanonicalBoundsArePreMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceRegistry := newMemoryRegistryStore()
	sourceSecrets := newMemorySecretStore()
	source := mustManager(t, sourceRegistry, sourceSecrets, nil)
	connectMemoryProvider(t, ctx, source, sourceRegistry, "provider-preflight-source", "synthetic-preflight-source")
	exported, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest() error = %v", err)
	}
	failedRegistry := newMemoryRegistryStore()
	failedSecrets := newMemorySecretStore()
	failed := mustManager(t, failedRegistry, failedSecrets, nil)
	failedRegistry.failNextCommit = true
	before := failedRegistry.snapshot()
	if _, err := failed.ImportPortableManifest(ctx, exported); !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ImportPortableManifest(commit failure) error = %v, want persistence failure", err)
	}
	if !reflect.DeepEqual(before, failedRegistry.snapshot()) {
		t.Fatal("commit failure changed the prior Registry")
	}
	if reads, _, _ := failedSecrets.callCounts(); reads != 0 {
		t.Fatalf("commit failure read Secret Store %d times", reads)
	}

	blockedRegistry := newMemoryRegistryStore()
	blockedSecrets := newMemorySecretStore()
	blocked := mustManager(t, blockedRegistry, blockedSecrets, nil)
	blockedRegistry.mutate(func(state *domainregistry.Registry) {
		state.Revision = 1
		for index := 0; index < domainregistry.MaxProviders; index++ {
			id := fmt.Sprintf("provider-capacity-%03d", index)
			state.Providers[id] = domainregistry.Provider{ID: id, Kind: "openai-compatible", Endpoint: fmt.Sprintf("https://capacity-%03d.example/v1", index), Models: []string{}, MediaModels: []string{}, SelectedRoutes: []string{}, Revision: 1, Generation: 1, Incarnation: "inc_" + strings.Repeat("c", 43)}
		}
	})
	capacityBefore := blockedRegistry.snapshot()
	if _, err := blocked.ImportPortableManifest(ctx, exported); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("ImportPortableManifest(capacity) error = %v, want conflict", err)
	}
	if !reflect.DeepEqual(capacityBefore, blockedRegistry.snapshot()) {
		t.Fatal("capacity preflight changed the Registry")
	}
	if reads, _, _ := blockedSecrets.callCounts(); reads != 0 {
		t.Fatalf("capacity preflight read Secret Store %d times", reads)
	}

	trailing := append(append([]byte(nil), exported...), '\n')
	overBoundManifest := domainregistry.PortableManifestV1{Schema: domainregistry.PortableManifestSchemaV1, Providers: []domainregistry.PortableProviderDescriptorV1{{
		Correlation: "provider-0", Kind: "openai-compatible", Endpoint: "https://oversized.example/v1", Models: []string{strings.Repeat("m", 4097)}, MediaModels: []string{}, Routes: []string{}, Intent: domainregistry.PortableManifestIntentReentryRequired,
	}}, Accounts: []domainregistry.PortableAccountDescriptorV1{}}
	overBound, err := json.Marshal(overBoundManifest)
	if err != nil {
		t.Fatalf("json.Marshal(over-bound manifest) error = %v", err)
	}
	canonicalBefore := blockedRegistry.snapshot()
	for name, body := range map[string][]byte{"trailing": trailing, "over-bound": overBound} {
		if _, err := blocked.ImportPortableManifest(ctx, body); err == nil {
			t.Fatalf("ImportPortableManifest(%s) accepted malformed input", name)
		}
		if !reflect.DeepEqual(canonicalBefore, blockedRegistry.snapshot()) {
			t.Fatalf("ImportPortableManifest(%s) changed Registry", name)
		}
	}
}

func TestPortableManifestV1RejectsNonTerminalLegacyRecoveryBeforeRevisionChange(t *testing.T) {
	ctx := context.Background()
	events := make([]string, 0, 8)
	state := newMemoryRegistryStore().snapshot()
	recoveryRegistry := &legacyMigrationRecordingRegistryStore{state: state, events: &events}
	candidate := legacyMigrationCandidateFor(state, "migration-portable-recovery", "provider-portable-recovery", "portable-recovery", "synthetic-portable-recovery-secret")
	recoverySecrets := &legacyMigrationRecordingSecretStore{
		candidateRef:         secretstoreport.CredentialRef("cred_" + strings.Repeat("r", 43)),
		expectedCredential:   []byte("synthetic-portable-recovery-secret"),
		expectedSourceSHA256: candidate.SourceSHA256,
		events:               &events,
	}
	recoveryManager := mustManager(t, recoveryRegistry, recoverySecrets, nil)
	if _, err := recoveryManager.PrepareLegacyMigrationRecovery(ctx, candidate); err != nil {
		t.Fatalf("PrepareLegacyMigrationRecovery() error = %v", err)
	}
	before := recoveryRegistry.state.Clone()
	manifest, err := domainregistry.MarshalPortableManifestV1(domainregistry.PortableManifestV1{
		Schema: domainregistry.PortableManifestSchemaV1,
		Providers: []domainregistry.PortableProviderDescriptorV1{{
			Correlation: "provider-0", Kind: "openai-compatible", Endpoint: "https://recovery-portable.example/v1",
			Models: []string{"model-recovery"}, MediaModels: []string{}, Routes: []string{},
			Intent: domainregistry.PortableManifestIntentReentryRequired,
		}}, Accounts: []domainregistry.PortableAccountDescriptorV1{},
	})
	if err != nil {
		t.Fatalf("MarshalPortableManifestV1() error = %v", err)
	}
	if _, err := recoveryManager.ImportPortableManifest(ctx, manifest); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("ImportPortableManifest(non-terminal recovery) error = %v, want conflict", err)
	}
	if !reflect.DeepEqual(before, recoveryRegistry.state) {
		t.Fatal("non-terminal recovery preflight changed Registry revision or recovery state")
	}
}

func connectPortableTestProvider(t *testing.T, ctx context.Context, manager *Manager, registry *memoryRegistryStore, input domainregistry.ProviderInput, secret string) domainregistry.Provider {
	t.Helper()
	credential, err := secretstoreport.SetCredential([]byte(secret))
	if err != nil {
		t.Fatalf("SetCredential(%s) error = %v", input.ID, err)
	}
	provider, err := manager.Connect(ctx, ConnectCommand{
		Expected: expectedForNewPortableTestProvider(registry.snapshot()), Provider: input,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if err != nil {
		t.Fatalf("Connect(%s) error = %v", input.ID, err)
	}
	return provider
}

func expectedForNewPortableTestProvider(state domainregistry.Registry) domainregistry.ExpectedState {
	return domainregistry.ExpectedState{RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation}
}
