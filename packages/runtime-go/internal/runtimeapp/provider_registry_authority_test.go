package runtimeapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
	provider "analytix.local/runtime-go/internal/provider"
)

func TestProviderRegistryAuthorityResolvesSelectedCommittedCredentialJustInTime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDir, 'k')
	authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1() error = %v", err)
	}
	defer func() {
		if err := authority.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	first := connectProviderRegistryTestWinnerV1(t, ctx, authority.Manager(), "provider-k4", "synthetic-k4-first-credential")

	resolver := newProviderRegistryExecutionResolverV1(authority.Manager())
	execution, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil {
		t.Fatalf("ResolveTurnExecution() error = %v", err)
	}
	if execution.ProviderID != "provider-k4" || execution.Model != "model-k4" ||
		execution.Config.APIKey != "synthetic-k4-first-credential" {
		t.Fatal("runtime execution did not use the selected committed Registry winner")
	}
	callerRoute, err := resolver.ResolveTurnIntent(ctx, provider.TurnExecutionInput{
		RequestEndpointFormat: "messages",
	})
	if err != nil || callerRoute.Config.EndpointFormat != "chat_completions" {
		t.Fatalf("caller endpoint override escaped Registry route authority: format=%q err=%v", callerRoute.Config.EndpointFormat, err)
	}
	if err := authority.Close(); err != nil {
		t.Fatalf("Close(first composition) error = %v", err)
	}
	authority, err = openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1(restart) error = %v", err)
	}
	resolver = newProviderRegistryExecutionResolverV1(authority.Manager())
	restarted, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || restarted.Config.APIKey != "synthetic-k4-first-credential" {
		t.Fatal("runtime restart did not re-resolve the selected committed Registry winner")
	}
	staleAuthority := restarted.Authority
	snapshot, err := authority.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(update) error = %v", err)
	}
	updated, err := authority.Manager().Update(ctx, providerregistryapp.UpdateCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, first.ID),
		Provider: domainregistry.ProviderInput{
			ID: first.ID, Kind: first.Kind, Endpoint: "https://provider.invalid/v2", Proxy: "http://proxy-current.invalid",
			Models: []string{"model-k4-next"}, MediaModels: []string{}, SelectedModel: "model-k4-next",
			SelectedRoutes: []string{"primary"},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if err := resolver.ValidateTurnExecutionCurrent(ctx, staleAuthority); err == nil {
		t.Fatal("pre-update execution authority survived Registry proxy drift")
	}
	updatedExecution, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || updatedExecution.Config.BaseURL != "https://provider.invalid/v2" ||
		updatedExecution.Config.ProxyURL != "http://proxy-current.invalid" ||
		updatedExecution.Model != "model-k4-next" || updatedExecution.Config.APIKey != "synthetic-k4-first-credential" ||
		(updated.Revision == first.Revision && updated.Generation == first.Generation) {
		t.Fatalf(
			"runtime execution reused stale Provider metadata after update: baseURL=%q model=%q priorRevision=%d currentRevision=%d priorGeneration=%d currentGeneration=%d err=%v",
			updatedExecution.Config.BaseURL, updatedExecution.Model, first.Revision, updated.Revision,
			first.Generation, updated.Generation, err,
		)
	}

	replacement, err := secretstoreport.SetCredential([]byte("synthetic-k4-second-credential"))
	if err != nil {
		t.Fatalf("SetCredential(replacement) error = %v", err)
	}
	snapshot, err = authority.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(replacement) error = %v", err)
	}
	second, err := authority.Manager().ReplaceCredential(ctx, providerregistryapp.CredentialReplaceCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, first.ID), ProviderID: first.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: replacement,
	})
	if err != nil {
		t.Fatalf("ReplaceCredential() error = %v", err)
	}
	replaced, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || replaced.Config.APIKey != "synthetic-k4-second-credential" ||
		replaced.Config.APIKey == restarted.Config.APIKey || second.CredentialRef == first.CredentialRef {
		t.Fatal("runtime execution reused the superseded Provider credential")
	}
	snapshot, err = authority.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(disconnect) error = %v", err)
	}
	if _, err := authority.Manager().Disconnect(ctx, providerregistryapp.DisconnectCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, second.ID), ProviderID: second.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
	}); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if _, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{}); err == nil ||
		strings.Contains(err.Error(), "synthetic-k4") {
		t.Fatal("disconnected runtime resolution did not fail closed with a redacted error")
	}
}

func TestProviderRegistryAuthorityResolvesCommittedRoutePoolJustInTime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDir, 'r')
	authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("openProviderRegistryAuthorityV1() error = %v", err)
	}
	defer authority.Close()

	fallback := connectProviderRegistryTestWinnerV1(
		t, ctx, authority.Manager(), "provider-route-fallback", "synthetic-runtime-route-fallback",
	)
	primary := connectProviderRegistryTestWinnerV1(
		t, ctx, authority.Manager(), "provider-route-primary", "synthetic-runtime-route-primary",
	)
	snapshot, err := authority.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(route pool) error = %v", err)
	}
	primary, err = authority.Manager().Update(ctx, providerregistryapp.UpdateCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, primary.ID),
		Provider: domainregistry.ProviderInput{
			ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint,
			Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: "",
			SelectedRoutes: []string{"provider:" + fallback.ID},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(route pool) error = %v", err)
	}
	snapshot, err = authority.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(select) error = %v", err)
	}
	if _, err := authority.Manager().Select(ctx, providerregistryapp.SelectCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, primary.ID), ProviderID: primary.ID,
	}); err != nil {
		t.Fatalf("Select(route policy) error = %v", err)
	}

	resolver := newProviderRegistryExecutionResolverV1(authority.Manager())
	resolved, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || resolved.ProviderID != fallback.ID || resolved.Model != fallback.SelectedModel ||
		resolved.Config.APIKey != "synthetic-runtime-route-fallback" {
		t.Fatalf("fallback execution = provider=%q model=%q err=%v", resolved.ProviderID, resolved.Model, err)
	}

	snapshot, err = authority.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(repair selection) error = %v", err)
	}
	primary = snapshot.Providers[primary.ID]
	if _, err := authority.Manager().Update(ctx, providerregistryapp.UpdateCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, primary.ID),
		Provider: domainregistry.ProviderInput{
			ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint,
			Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: primary.Models[0],
			SelectedRoutes: []string{"provider:" + primary.ID, "provider:" + fallback.ID},
		},
		Credential: secretstoreport.KeepCredential(),
	}); err != nil {
		t.Fatalf("Update(repair selection) error = %v", err)
	}
	resolved, err = resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || resolved.ProviderID != primary.ID || resolved.Model != primary.Models[0] ||
		resolved.Config.APIKey != "synthetic-runtime-route-primary" {
		t.Fatalf("repaired primary execution = provider=%q model=%q err=%v", resolved.ProviderID, resolved.Model, err)
	}

	snapshot, err = authority.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(disable route policy) error = %v", err)
	}
	primary = snapshot.Providers[primary.ID]
	if _, err := authority.Manager().Update(ctx, providerregistryapp.UpdateCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, primary.ID),
		Provider: domainregistry.ProviderInput{
			ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint,
			Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: "",
			SelectedRoutes: []string{},
		},
		Credential: secretstoreport.KeepCredential(),
	}); err != nil {
		t.Fatalf("Update(empty disabled terminal) error = %v", err)
	}
	disabledExecution, err := resolver.ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err == nil || disabledExecution.ProviderID != "" || disabledExecution.Model != "" ||
		disabledExecution.Config.APIKey != "" || disabledExecution.Config.BaseURL != "" {
		t.Fatalf(
			"empty disabled execution guessed a Provider: provider=%q model=%q baseURL=%q err=%v",
			disabledExecution.ProviderID, disabledExecution.Model, disabledExecution.Config.BaseURL, err,
		)
	}
}

func TestProviderRegistryAuthorityIsolatesDataDirectoriesAndFailsClosedWhenUnreadable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDirA := t.TempDir()
	dataDirB := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDirA, 'a')
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDirB, 'b')
	authorityA, err := openProviderRegistryAuthorityV1(ctx, dataDirA)
	if err != nil {
		t.Fatalf("open authority A: %v", err)
	}
	defer authorityA.Close()
	authorityB, err := openProviderRegistryAuthorityV1(ctx, dataDirB)
	if err != nil {
		t.Fatalf("open authority B: %v", err)
	}
	defer authorityB.Close()
	connectProviderRegistryTestWinnerV1(t, ctx, authorityA.Manager(), "provider-shared-id", "synthetic-profile-a")
	connectProviderRegistryTestWinnerV1(t, ctx, authorityB.Manager(), "provider-shared-id", "synthetic-profile-b")

	resolvedA, errA := newProviderRegistryExecutionResolverV1(authorityA.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	resolvedB, errB := newProviderRegistryExecutionResolverV1(authorityB.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if errA != nil || errB != nil || resolvedA.Config.APIKey != "synthetic-profile-a" ||
		resolvedB.Config.APIKey != "synthetic-profile-b" || resolvedA.Config.APIKey == resolvedB.Config.APIKey {
		t.Fatal("isolated data directories shared Provider credential authority")
	}
	if err := authorityA.Close(); err != nil {
		t.Fatalf("Close(authority A) error = %v", err)
	}
	masterKeyA := filepath.Join(dataDirA, "private", "provider-secrets", "master-key", "master.key")
	if err := os.Chmod(masterKeyA, 0o644); err != nil {
		t.Fatalf("make authority A unreadable: %v", err)
	}
	reopened, err := openProviderRegistryAuthorityV1(ctx, dataDirA)
	if err == nil || reopened != nil || strings.Contains(err.Error(), "synthetic-profile-a") || strings.Contains(err.Error(), dataDirA) {
		t.Fatal("unreadable authority did not fail closed with a redacted error")
	}
}

func TestProviderRegistryAuthorityConcurrentManagersCommitOneWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDir, 'c')
	first, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("open first authority: %v", err)
	}
	defer first.Close()
	second, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("open second authority: %v", err)
	}
	defer second.Close()
	snapshot, err := first.Manager().Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	expected := domainregistry.ExpectedState{
		RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation,
	}
	input := domainregistry.ProviderInput{
		ID: "provider-concurrent", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
		Models: []string{"model-concurrent"}, MediaModels: []string{}, SelectedModel: "model-concurrent",
		SelectedRoutes: []string{"primary"},
	}
	credentials := []string{"synthetic-concurrent-a", "synthetic-concurrent-b"}
	managers := []*providerregistryapp.Manager{first.Manager(), second.Manager()}
	errorsByAttempt := make([]error, len(managers))
	var attempts sync.WaitGroup
	for index := range managers {
		attempts.Add(1)
		go func(index int) {
			defer attempts.Done()
			credential, setErr := secretstoreport.SetCredential([]byte(credentials[index]))
			if setErr != nil {
				errorsByAttempt[index] = setErr
				return
			}
			_, errorsByAttempt[index] = managers[index].Connect(ctx, providerregistryapp.ConnectCommand{
				Expected: expected, Provider: input,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
			})
		}(index)
	}
	attempts.Wait()
	successes := 0
	for _, attemptErr := range errorsByAttempt {
		if attemptErr == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent Manager successes = %d, want 1", successes)
	}
	resolved, err := newProviderRegistryExecutionResolverV1(first.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || (resolved.Config.APIKey != credentials[0] && resolved.Config.APIKey != credentials[1]) {
		t.Fatal("concurrent Managers did not retain one usable committed winner")
	}
}

func connectProviderRegistryTestWinnerV1(
	t *testing.T,
	ctx context.Context,
	manager *providerregistryapp.Manager,
	providerID string,
	secret string,
) domainregistry.Provider {
	t.Helper()
	snapshot, err := manager.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	credential, err := secretstoreport.SetCredential([]byte(secret))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	winner, err := manager.Connect(ctx, providerregistryapp.ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
			Models: []string{"model-k4"}, MediaModels: []string{}, SelectedModel: "model-k4",
			SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	return winner
}

func expectedProviderRegistryStateV1(state domainregistry.Registry, providerID string) domainregistry.ExpectedState {
	provider := state.Providers[providerID]
	return domainregistry.ExpectedState{
		RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
		ProviderRevision: provider.Revision, ProviderGeneration: provider.Generation,
		ProviderIncarnation: provider.Incarnation, ProviderCredentialPurpose: provider.CredentialPurpose,
	}
}

func writeProviderRegistrySyntheticFallbackMasterKeyV1(t *testing.T, dataDir string, marker byte) {
	t.Helper()
	directory := filepath.Join(dataDir, "private", "provider-secrets", "master-key")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create synthetic master-key directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(directory, "authority.v1"),
		[]byte("analytix-master-key-authority:v1:fallback\n"),
		0o600,
	); err != nil {
		t.Fatalf("write synthetic master-key authority: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "master.key"), []byte(strings.Repeat(string(marker), 32)), 0o600); err != nil {
		t.Fatalf("write synthetic fallback master key: %v", err)
	}
}

func TestProviderRegistrySecretAuthorizerPermitsOnlyManagerAndRuntimeExecution(t *testing.T) {
	t.Parallel()

	authorizer := providerRegistrySecretAuthorizerV1{}
	request := secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef("cred_" + strings.Repeat("R", 43)),
		Purpose:       secretstoreport.Purpose("provider-api-key"),
		Consumer:      providerregistryapp.RegistryReadbackConsumer,
	}
	if err := authorizer.AuthorizeCredentialAccess(context.Background(), request); err != nil {
		t.Fatalf("manager readback authorization error = %v", err)
	}
	request.Consumer = "renderer"
	if err := authorizer.AuthorizeCredentialAccess(context.Background(), request); !errors.Is(err, secretstoreport.ErrUnauthorized) {
		t.Fatalf("renderer authorization error = %v", err)
	}
	request.Consumer = providerregistryapp.ProviderExecutionConsumer
	if err := authorizer.AuthorizeCredentialAccess(context.Background(), request); err != nil {
		t.Fatalf("provider execution authorization error = %v", err)
	}
	if err := authorizer.AuthorizeCredentialAccess(nil, secretstoreport.AccessRequest{Consumer: providerregistryapp.RegistryReadbackConsumer}); !errors.Is(err, secretstoreport.ErrUnauthorized) {
		t.Fatalf("nil-context authorization error = %v", err)
	}
}

func TestProviderRegistrySecretAuthorizerProtectedRecoveryPurposeMatrix(t *testing.T) {
	t.Parallel()

	authorizer := providerRegistrySecretAuthorizerV1{}
	allowedPurposes := []secretstoreport.Purpose{
		"provider-api-key", "provider-oauth-token-bundle", "mcp-oauth-access-token", "extension-provider-account-token",
	}
	for _, purpose := range allowedPurposes {
		for _, consumer := range []string{providerregistryapp.ProtectedTransferSourceConsumer, providerregistryapp.ProtectedRecoveryReadbackConsumer} {
			if err := authorizer.AuthorizeCredentialAccess(context.Background(), secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef("cred_" + strings.Repeat("A", 43)),
				Purpose:       purpose, Consumer: consumer,
			}); err != nil {
				t.Fatalf("protected consumer %q purpose %q error = %v", consumer, purpose, err)
			}
		}
	}
	if err := authorizer.AuthorizeCredentialAccess(context.Background(), secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef("cred_" + strings.Repeat("A", 43)),
		Purpose:       secretstoreport.Purpose(providerregistryapp.ProtectedRecoveryPendingKeyPurpose),
		Consumer:      providerregistryapp.ProtectedRecoveryPendingKeyConsumer,
	}); err != nil {
		t.Fatalf("pending-key consumer error = %v", err)
	}
	for _, testCase := range []struct {
		name     string
		consumer string
		purpose  secretstoreport.Purpose
	}{
		{name: "general pending-key", consumer: providerregistryapp.RegistryReadbackConsumer, purpose: secretstoreport.Purpose(providerregistryapp.ProtectedRecoveryPendingKeyPurpose)},
		{name: "source unknown", consumer: providerregistryapp.ProtectedTransferSourceConsumer, purpose: "hub-api-key"},
		{name: "readback transport", consumer: providerregistryapp.ProtectedRecoveryReadbackConsumer, purpose: "transport-telegram-bot-token"},
		{name: "pending-key wrong purpose", consumer: providerregistryapp.ProtectedRecoveryPendingKeyConsumer, purpose: "provider-api-key"},
		{name: "unknown consumer", consumer: "registry-generic", purpose: "provider-api-key"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := authorizer.AuthorizeCredentialAccess(context.Background(), secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef("cred_" + strings.Repeat("A", 43)),
				Purpose:       testCase.purpose, Consumer: testCase.consumer,
			})
			if !errors.Is(err, secretstoreport.ErrUnauthorized) {
				t.Fatalf("authorization error = %v, want unauthorized", err)
			}
		})
	}
}

func TestProviderRegistryAuthorityStartupFailsClosedOnCorruptRegistry(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	registryDirectory := dataDir + "/private/provider-registry"
	if err := writeProviderRegistryCorruptionFixtureV1(registryDirectory, []byte("not-json\n")); err != nil {
		t.Fatalf("write corruption fixture: %v", err)
	}
	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err == nil || authority != nil || strings.Contains(err.Error(), "not-json") || strings.Contains(err.Error(), registryDirectory) {
		t.Fatalf("corrupt startup result = (%v, %v)", authority, err)
	}
}

func TestProviderRegistryAuthorityStartupFailsClosedOnCorruptCommittedSecretStore(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	registryDirectory := filepath.Join(dataDir, "private", "provider-registry")
	provider := domainregistry.Provider{
		ID: "provider-alpha", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
		Models: []string{"model-alpha"}, MediaModels: []string{}, SelectedModel: "model-alpha",
		SelectedRoutes: []string{"primary"}, CredentialRef: "cred_" + strings.Repeat("B", 43),
		CredentialPurpose: "provider-api-key", Revision: 1, Generation: 1,
		Incarnation: "inc_" + strings.Repeat("b", 43),
	}
	registryBytes, err := domainregistry.Marshal(domainregistry.Registry{
		Version: domainregistry.FormatVersion, Revision: 1, Incarnation: "inc_" + strings.Repeat("a", 43),
		SelectedProviderID: provider.ID, Providers: map[string]domainregistry.Provider{provider.ID: provider},
		Transactions: map[string]domainregistry.Transaction{},
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := writeProviderRegistryCorruptionFixtureV1(registryDirectory, registryBytes); err != nil {
		t.Fatalf("write Registry fixture: %v", err)
	}
	secretDirectory := filepath.Join(dataDir, "private", "provider-secrets")
	if err := os.MkdirAll(secretDirectory, 0o700); err != nil {
		t.Fatalf("create secret directory: %v", err)
	}
	secretPath := filepath.Join(secretDirectory, providerRegistrySecretStoreFileV1)
	if err := os.WriteFile(secretPath, []byte("corrupt-secret-store\n"), 0o600); err != nil {
		t.Fatalf("write secret fixture: %v", err)
	}

	authority, err := openProviderRegistryAuthorityV1(context.Background(), dataDir)
	if err == nil || authority != nil || strings.Contains(err.Error(), "corrupt-secret-store") || strings.Contains(err.Error(), secretPath) {
		t.Fatalf("corrupt secret startup result = (%v, %v)", authority, err)
	}
}

func writeProviderRegistryCorruptionFixtureV1(directory string, content []byte) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(directory, "registry.v1.json"), content, 0o600)
}
