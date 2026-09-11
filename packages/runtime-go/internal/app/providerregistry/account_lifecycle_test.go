package providerregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func telegramAccountScope(accountID string) domainregistry.PrivateAccountScope {
	return domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "transport", Provider: "telegram",
		AccountID: accountID, ChannelID: accountID, Purpose: "transport-telegram-bot-token",
	}
}

func TestAccountCredentialLifecycleIsPurposeBoundFencedAndKeyFree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	scope := telegramAccountScope("channel-primary")

	absent, err := manager.AccountCredentialState(ctx, scope)
	if err != nil || absent.Status != AccountCredentialStatusAbsent {
		t.Fatalf("AccountCredentialState(absent) = %#v, %v", absent, err)
	}
	firstSecret := []byte(`{"botToken":"synthetic-telegram-generation-1","allowedChatIds":"1001"}`)
	first, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: scope, Expected: absent.Expected(), Credential: firstSecret,
	})
	if err != nil || first.Status != AccountCredentialStatusReady || first.ProviderGeneration != 1 {
		t.Fatalf("PutAccountCredential(first) = %#v, %v", first, err)
	}
	committedJSON, err := json.Marshal(registry.snapshot())
	if err != nil || bytes.Contains(committedJSON, firstSecret) || registry.snapshot().SelectedProviderID != "" {
		t.Fatal("private account secret leaked into Registry or polluted selected Provider")
	}
	resolution, err := manager.ResolveAccountCredential(ctx, scope)
	if err != nil || !bytes.Equal(resolution.Credential, firstSecret) {
		t.Fatalf("ResolveAccountCredential(first) = %#v, %v", resolution.State, err)
	}
	firstFence := resolution.State
	resolution.Clear()

	secondSecret := []byte(`{"botToken":"synthetic-telegram-generation-2","allowedChatIds":"1002"}`)
	second, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: scope, Expected: first.Expected(), Credential: secondSecret,
	})
	if err != nil || second.ProviderGeneration != first.ProviderGeneration+1 {
		t.Fatalf("PutAccountCredential(replace) = %#v, %v", second, err)
	}
	if err := manager.ValidateAccountCredentialCurrent(ctx, firstFence); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("ValidateAccountCredentialCurrent(stale) error = %v, want conflict", err)
	}
	if _, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: scope, Expected: first.Expected(), Credential: []byte("synthetic-stale-refresh"),
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("PutAccountCredential(stale) error = %v, want conflict", err)
	}
	if _, err := manager.ResolveAccountCredential(ctx, telegramAccountScope("channel-other")); !errors.Is(err, registryport.ErrNotFound) {
		t.Fatalf("cross-account ResolveAccountCredential() error = %v, want not found", err)
	}
	for _, invalid := range []domainregistry.PrivateAccountScope{
		{SchemaVersion: 1, Owner: "mcp", Provider: "telegram", AccountID: scope.AccountID, ChannelID: scope.ChannelID, Purpose: scope.Purpose},
		{SchemaVersion: 1, Owner: scope.Owner, Provider: scope.Provider, AccountID: scope.AccountID, ChannelID: scope.ChannelID, Purpose: "mcp-oauth-access-token"},
	} {
		if _, err := manager.ResolveAccountCredential(ctx, invalid); !errors.Is(err, registryport.ErrInvalidRequest) {
			t.Fatalf("incoherent-scope ResolveAccountCredential(%#v) error = %v, want invalid request", invalid, err)
		}
	}

	revoked, err := manager.MutateAccountCredential(ctx, AccountCredentialMutationCommand{
		Scope: scope, Expected: second.Expected(), Disposition: AccountCredentialDispositionRevoke,
	})
	if err != nil || revoked.Status != AccountCredentialStatusRevoked {
		t.Fatalf("MutateAccountCredential(revoke) = %#v, %v", revoked, err)
	}
	if _, err := manager.ResolveAccountCredential(ctx, scope); !errors.Is(err, registryport.ErrNotFound) {
		t.Fatalf("ResolveAccountCredential(revoked) error = %v, want not found", err)
	}
	reconnected, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: scope, Expected: revoked.Expected(), Credential: []byte("synthetic-reconnected-token"),
	})
	if err != nil || reconnected.Status != AccountCredentialStatusReady || reconnected.ProviderIncarnation == revoked.ProviderIncarnation {
		t.Fatalf("PutAccountCredential(reconnect) = %#v, %v", reconnected, err)
	}
	deleted, err := manager.MutateAccountCredential(ctx, AccountCredentialMutationCommand{
		Scope: scope, Expected: reconnected.Expected(), Disposition: AccountCredentialDispositionDelete,
	})
	if err != nil || deleted.Status != AccountCredentialStatusAbsent {
		t.Fatalf("MutateAccountCredential(delete) = %#v, %v", deleted, err)
	}
}

func TestAccountCredentialLifecycleCoversProviderOAuthSubscriptionMCPOAuthExtensionAndWeixinAccounts(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name  string
		scope domainregistry.PrivateAccountScope
	}{
		{
			name:  "mcp-oauth",
			scope: domainregistry.PrivateAccountScope{SchemaVersion: 1, Owner: "mcp", Provider: "server-a", AccountID: "account-a", ChannelID: "server-a", Purpose: "mcp-oauth-access-token"},
		},
		{
			name:  "extension-account",
			scope: domainregistry.PrivateAccountScope{SchemaVersion: 1, Owner: "extension", Provider: "extension-a", AccountID: "account-a", ChannelID: "extension-a", Purpose: "extension-provider-account-token"},
		},
		{
			name:  "weixin-transport",
			scope: domainregistry.PrivateAccountScope{SchemaVersion: 1, Owner: "transport", Provider: "weixin", AccountID: "account-a", ChannelID: "channel-a", Purpose: "transport-weixin-session-key"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			manager := mustManager(t, newMemoryRegistryStore(), newMemorySecretStore(), nil)
			absent, err := manager.AccountCredentialState(ctx, testCase.scope)
			if err != nil || absent.Status != AccountCredentialStatusAbsent {
				t.Fatalf("AccountCredentialState(absent) = %#v, %v", absent, err)
			}
			ready, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
				Scope: testCase.scope, Expected: absent.Expected(), Credential: []byte("synthetic-" + testCase.name + "-generation-1"),
			})
			if err != nil || ready.Status != AccountCredentialStatusReady {
				t.Fatalf("PutAccountCredential() = %#v, %v", ready, err)
			}
			resolution, err := manager.ResolveAccountCredential(ctx, testCase.scope)
			if err != nil || resolution.State.ProviderGeneration != ready.ProviderGeneration {
				t.Fatalf("ResolveAccountCredential() = %#v, %v", resolution.State, err)
			}
			resolution.Clear()
			replaced, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
				Scope: testCase.scope, Expected: ready.Expected(), Credential: []byte("synthetic-" + testCase.name + "-generation-2"),
			})
			if err != nil || replaced.ProviderGeneration != ready.ProviderGeneration+1 {
				t.Fatalf("PutAccountCredential(replace) = %#v, %v", replaced, err)
			}
			revoked, err := manager.MutateAccountCredential(ctx, AccountCredentialMutationCommand{
				Scope: testCase.scope, Expected: replaced.Expected(), Disposition: AccountCredentialDispositionRevoke,
			})
			if err != nil || revoked.Status != AccountCredentialStatusRevoked {
				t.Fatalf("MutateAccountCredential(revoke) = %#v, %v", revoked, err)
			}
			reconnected, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
				Scope: testCase.scope, Expected: revoked.Expected(), Credential: []byte("synthetic-" + testCase.name + "-reconnected"),
			})
			if err != nil || reconnected.Status != AccountCredentialStatusReady {
				t.Fatalf("PutAccountCredential(reconnect) = %#v, %v", reconnected, err)
			}
			disconnected, err := manager.MutateAccountCredential(ctx, AccountCredentialMutationCommand{
				Scope: testCase.scope, Expected: reconnected.Expected(), Disposition: AccountCredentialDispositionDisconnect,
			})
			if err != nil || disconnected.Status != AccountCredentialStatusDisconnected {
				t.Fatalf("MutateAccountCredential(disconnect) = %#v, %v", disconnected, err)
			}
			finalReady, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
				Scope: testCase.scope, Expected: disconnected.Expected(), Credential: []byte("synthetic-" + testCase.name + "-after-disconnect"),
			})
			if err != nil || finalReady.Status != AccountCredentialStatusReady {
				t.Fatalf("PutAccountCredential(after disconnect) = %#v, %v", finalReady, err)
			}
			deleted, err := manager.MutateAccountCredential(ctx, AccountCredentialMutationCommand{
				Scope: testCase.scope, Expected: finalReady.Expected(), Disposition: AccountCredentialDispositionDelete,
			})
			if err != nil || deleted.Status != AccountCredentialStatusAbsent {
				t.Fatalf("MutateAccountCredential(delete) = %#v, %v", deleted, err)
			}
			if _, err := manager.ResolveAccountCredential(ctx, testCase.scope); !errors.Is(err, registryport.ErrNotFound) {
				t.Fatalf("ResolveAccountCredential(deleted) error = %v, want not found", err)
			}
		})
	}
}

func TestProviderOAuthBundleCrossesExistingProviderManagementAndExecutionSeams(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	prior := connectMemoryProvider(t, ctx, manager, registry, "provider-oauth", "synthetic-prior-api-key")
	oauthBinding := &domainregistry.OAuthBindingMetadata{
		SchemaVersion: 1, Issuer: "https://issuer.invalid/",
		AuthorizationEndpoint: "https://login.invalid/authorize",
		TokenEndpoint:         "https://tokens.invalid/token",
		ClientID:              "client-a", Scopes: []string{"openid"}, RedirectModeVersion: 1,
	}
	configured, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), prior.ID),
		Provider: domainregistry.ProviderInput{
			ID: prior.ID, Kind: prior.Kind, Endpoint: prior.Endpoint,
			Models: prior.Models, MediaModels: prior.MediaModels,
			SelectedModel: prior.SelectedModel, SelectedMedia: prior.SelectedMedia,
			SelectedRoutes: prior.SelectedRoutes, OAuthBinding: oauthBinding,
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(Provider OAuth binding) error = %v", err)
	}
	prior = configured
	scope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "provider", Provider: prior.ID,
		AccountID: prior.ID, Purpose: "provider-oauth-token-bundle",
	}
	current, err := manager.AccountCredentialState(ctx, scope)
	if err != nil || current.Status != AccountCredentialStatusReady || current.CredentialPurpose != "provider-api-key" {
		t.Fatalf("AccountCredentialState(existing Provider) = %#v, %v", current, err)
	}
	bundle, err := json.Marshal(map[string]any{
		"kind": "provider-oauth-bundle", "accessToken": "synthetic-provider-oauth-access",
		"refreshToken": "synthetic-provider-oauth-refresh", "tokenType": "Bearer",
		"expiresAtMs": int64(4102444800000),
		"oauthBinding": map[string]any{
			"owner": "provider", "issuer": oauthBinding.Issuer,
			"authorizationEndpoint": oauthBinding.AuthorizationEndpoint,
			"tokenEndpoint":         oauthBinding.TokenEndpoint, "clientId": oauthBinding.ClientID,
			"provider": prior.ID, "accountId": prior.ID,
			"redirectUri":      "com.analytix.desktop:/oauth/callback/oauthb_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"scopes":           oauthBinding.Scopes,
			"bindingKey":       "oauthb_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"ownerFingerprint": strings.Repeat("b", 64),
			"ownerRevision":    fmt.Sprint(prior.Revision + 1), "ownerGeneration": fmt.Sprint(prior.Generation + 1),
			"ownerIncarnation": prior.Incarnation, "profileBinding": strings.Repeat("c", 64),
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal(OAuth bundle) error = %v", err)
	}
	ready, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: scope, Expected: current.Expected(), Credential: bundle,
	})
	if err != nil || ready.Status != AccountCredentialStatusReady || ready.CredentialPurpose != scope.Purpose {
		t.Fatalf("PutAccountCredential(Provider OAuth bundle) = %#v, %v", ready, err)
	}
	if got := registry.snapshot().Providers[prior.ID]; got.PrivateAccount != nil || got.Kind == domainregistry.PrivateAccountKind {
		t.Fatalf("Provider OAuth created a private shadow Provider: %#v", got)
	}
	protected, err := manager.ResolveAccountCredential(ctx, scope)
	if err != nil || !bytes.Equal(protected.Credential, bundle) {
		t.Fatalf("ResolveAccountCredential(bundle) = %#v, %v", protected.State, err)
	}
	protected.Clear()
	execution, err := manager.ResolveProviderForOperation(ctx, ProviderOperationCommand{
		ProviderID: prior.ID, Expected: ready.Expected(),
	})
	if err != nil || string(execution.Credential) != "synthetic-provider-oauth-access" {
		t.Fatalf("ResolveProviderForOperation(OAuth) credential=%q error=%v", execution.Credential, err)
	}
	execution.Clear()
	committedProvider := registry.snapshot().Providers[prior.ID]
	for _, testCase := range []struct {
		name  string
		field string
		value string
	}{
		{name: "old revision", field: "ownerRevision", value: fmt.Sprint(committedProvider.Revision - 1)},
		{name: "old generation", field: "ownerGeneration", value: fmt.Sprint(committedProvider.Generation - 1)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var forged map[string]any
			if err := json.Unmarshal(bundle, &forged); err != nil {
				t.Fatalf("json.Unmarshal(bundle) error = %v", err)
			}
			bindingMap, ok := forged["oauthBinding"].(map[string]any)
			if !ok {
				t.Fatal("OAuth bundle binding is unavailable")
			}
			bindingMap[testCase.field] = testCase.value
			forgedBytes, err := json.Marshal(forged)
			if err != nil {
				t.Fatalf("json.Marshal(forged bundle) error = %v", err)
			}
			secrets.mu.Lock()
			record := secrets.records[committedProvider.CredentialRef]
			clear(record.secret)
			record.secret = bytes.Clone(forgedBytes)
			secrets.records[committedProvider.CredentialRef] = record
			secrets.mu.Unlock()
			resolution, resolveErr := manager.ResolveProviderForOperation(ctx, ProviderOperationCommand{
				ProviderID: prior.ID, Expected: ready.Expected(),
			})
			resolution.Clear()
			if !errors.Is(resolveErr, registryport.ErrVerification) {
				t.Fatalf("ResolveProviderForOperation(%s) error = %v, want verification failure", testCase.name, resolveErr)
			}
		})
	}
	secrets.mu.Lock()
	record := secrets.records[committedProvider.CredentialRef]
	clear(record.secret)
	record.secret = bytes.Clone(bundle)
	secrets.records[committedProvider.CredentialRef] = record
	secrets.mu.Unlock()
	revoked, err := manager.MutateAccountCredential(ctx, AccountCredentialMutationCommand{
		Scope: scope, Expected: ready.Expected(), Disposition: AccountCredentialDispositionRevoke,
	})
	if err != nil || revoked.Status != AccountCredentialStatusRevoked {
		t.Fatalf("MutateAccountCredential(revoke Provider OAuth) = %#v, %v", revoked, err)
	}
}

func TestAccountCredentialRefreshAndRevokeRaceHasOneDurableWinner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	scope := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "provider", Provider: "provider-race", AccountID: "provider-race",
		Purpose: "provider-oauth-token-bundle",
	}
	connectMemoryProvider(t, ctx, manager, registry, scope.Provider, "synthetic-race-api-key")
	absent, _ := manager.AccountCredentialState(ctx, scope)
	ready, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: scope, Expected: absent.Expected(), Credential: []byte("synthetic-race-prior"),
	})
	if err != nil {
		t.Fatalf("PutAccountCredential(prior) error = %v", err)
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	errorsByOperation := make(chan error, 2)
	go func() {
		defer wait.Done()
		<-start
		_, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
			Scope: scope, Expected: ready.Expected(), Credential: []byte("synthetic-race-refresh"),
		})
		errorsByOperation <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := manager.MutateAccountCredential(ctx, AccountCredentialMutationCommand{
			Scope: scope, Expected: ready.Expected(), Disposition: AccountCredentialDispositionRevoke,
		})
		errorsByOperation <- err
	}()
	close(start)
	wait.Wait()
	close(errorsByOperation)
	successes, conflicts := 0, 0
	for err := range errorsByOperation {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, registryport.ErrConflict):
			conflicts++
		default:
			t.Fatalf("race operation error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("race outcomes successes=%d conflicts=%d", successes, conflicts)
	}
	final, err := manager.AccountCredentialState(ctx, scope)
	if err != nil || (final.Status != AccountCredentialStatusReady && final.Status != AccountCredentialStatusRevoked) {
		t.Fatalf("final race state = %#v, %v", final, err)
	}
}

func TestOAuthAuthorizationCapacityCannotStarveOrdinaryPrivateAccounts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	manager := mustManager(t, newMemoryRegistryStore(), newMemorySecretStore(), nil)
	for index := 0; index < domainregistry.MaxOAuthAuthorizationStates; index++ {
		owner, purpose := "provider", "provider-oauth-authorization-state"
		if index%2 == 1 {
			owner, purpose = "mcp", "mcp-oauth-authorization-state"
		}
		scope := domainregistry.PrivateAccountScope{
			SchemaVersion: 1, Owner: owner, Provider: "oauth-capacity", AccountID: "account-capacity",
			ChannelID: fmt.Sprintf("authorization-%02d", index), Purpose: purpose,
		}
		absent, err := manager.AccountCredentialState(ctx, scope)
		if err != nil {
			t.Fatalf("AccountCredentialState(%d) error = %v", index, err)
		}
		if _, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
			Scope: scope, Expected: absent.Expected(), Credential: []byte("synthetic-authorization-state"),
		}); err != nil {
			t.Fatalf("PutAccountCredential(%d) error = %v", index, err)
		}
	}
	overflow := domainregistry.PrivateAccountScope{
		SchemaVersion: 1, Owner: "mcp", Provider: "oauth-capacity", AccountID: "account-capacity",
		ChannelID: "authorization-overflow", Purpose: "mcp-oauth-authorization-state",
	}
	absent, err := manager.AccountCredentialState(ctx, overflow)
	if err != nil {
		t.Fatalf("AccountCredentialState(overflow) error = %v", err)
	}
	if _, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: overflow, Expected: absent.Expected(), Credential: []byte("synthetic-overflow-state"),
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("PutAccountCredential(overflow) error = %v, want conflict", err)
	}

	transport := telegramAccountScope("ordinary-transport-after-oauth-capacity")
	transportAbsent, err := manager.AccountCredentialState(ctx, transport)
	if err != nil {
		t.Fatalf("AccountCredentialState(transport) error = %v", err)
	}
	if _, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: transport, Expected: transportAbsent.Expected(), Credential: []byte("synthetic-transport-token"),
	}); err != nil {
		t.Fatalf("ordinary transport account was starved by authorization state: %v", err)
	}
}

func TestPrivateAccountCapacityReservesOrdinaryProviderSlots(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := newMemoryRegistryStore()
	registry.mutate(func(state *domainregistry.Registry) {
		for index := 0; index < domainregistry.MaxProviders-domainregistry.ReservedPublicProviderCapacity; index++ {
			id := fmt.Sprintf("provider-%03d", index)
			state.Providers[id] = domainregistry.ProviderInput{
				ID: id, Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
				Models: []string{"model-a"}, SelectedModel: "model-a", SelectedRoutes: []string{"primary"},
			}.Provider("inc_"+strings.Repeat(string(rune('a'+index%20)), 43), "", "", 1, 1)
		}
		state.Revision = uint64(len(state.Providers))
	})
	manager := mustManager(t, registry, newMemorySecretStore(), nil)
	privateScope := telegramAccountScope("reserved-public-capacity")
	absent, err := manager.AccountCredentialState(ctx, privateScope)
	if err != nil {
		t.Fatalf("AccountCredentialState(private) error = %v", err)
	}
	if _, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: privateScope, Expected: absent.Expected(), Credential: []byte("synthetic-private-over-reserve"),
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("PutAccountCredential(private reserve) error = %v, want conflict", err)
	}
	credential, err := secretstoreport.SetCredential([]byte("synthetic-public-provider-secret"))
	if err != nil {
		t.Fatalf("SetCredential(public) error = %v", err)
	}
	state := registry.snapshot()
	connected, err := manager.Connect(ctx, ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: "provider-reserved-winner", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
			Models: []string{"model-a"}, SelectedModel: "model-a", SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: "provider-api-key", Credential: credential,
	})
	if err != nil || connected.ID != "provider-reserved-winner" {
		t.Fatalf("Connect(public reserved slot) = %#v, %v", connected, err)
	}
}

func TestAccountCredentialCrashRecoveryAndUnreadableSecretFailClosed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, testCase := range []struct {
		name       string
		fault      FaultPoint
		wantStatus AccountCredentialStatus
	}{
		{name: "before-secret-durable", fault: FaultAfterTransactionPrepared, wantStatus: AccountCredentialStatusAbsent},
		{name: "after-secret-durable", fault: FaultAfterCandidateDurable, wantStatus: AccountCredentialStatusReady},
		{name: "after-secret-durable-recorded", fault: FaultAfterCandidateDurableRecorded, wantStatus: AccountCredentialStatusReady},
		{name: "after-metadata-commit", fault: FaultAfterMetadataCommitted, wantStatus: AccountCredentialStatusReady},
		{name: "after-readback-verified", fault: FaultAfterReadbackVerified, wantStatus: AccountCredentialStatusReady},
		{name: "after-verified-recorded", fault: FaultAfterVerifiedRecorded, wantStatus: AccountCredentialStatusReady},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, faultOnce(testCase.fault))
			scope := telegramAccountScope("channel-crash-" + testCase.name)
			absent, _ := manager.AccountCredentialState(ctx, scope)
			_, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
				Scope: scope, Expected: absent.Expected(), Credential: []byte("synthetic-crash-token"),
			})
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("PutAccountCredential(interrupted) error = %v", err)
			}
			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
			state, err := restarted.AccountCredentialState(ctx, scope)
			if err != nil || state.Status != testCase.wantStatus {
				t.Fatalf("recovered state = %#v, %v, want %s", state, err, testCase.wantStatus)
			}
			if len(registry.snapshot().Transactions) != 0 {
				t.Fatal("recovery retained an account transaction")
			}
			wantSecretRecords := 0
			if testCase.wantStatus == AccountCredentialStatusReady {
				wantSecretRecords = 1
				resolution, resolveErr := restarted.ResolveAccountCredential(ctx, scope)
				if resolveErr != nil || string(resolution.Credential) != "synthetic-crash-token" {
					t.Fatalf("recovered credential = %q, %v", resolution.Credential, resolveErr)
				}
				resolution.Clear()
			}
			if got := secrets.recordCount(); got != wantSecretRecords {
				t.Fatalf("recovered secret records = %d, want %d", got, wantSecretRecords)
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover(idempotent repeat) error = %v", err)
			}
			repeated, err := restarted.AccountCredentialState(ctx, scope)
			if err != nil || repeated.Status != testCase.wantStatus || len(registry.snapshot().Transactions) != 0 {
				t.Fatalf("repeated recovery state = %#v, %v, want stable %s", repeated, err, testCase.wantStatus)
			}
		})
	}

	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	scope := telegramAccountScope("channel-tamper")
	absent, _ := manager.AccountCredentialState(ctx, scope)
	ready, err := manager.PutAccountCredential(ctx, AccountCredentialPutCommand{
		Scope: scope, Expected: absent.Expected(), Credential: []byte("synthetic-tamper-token"),
	})
	if err != nil {
		t.Fatalf("PutAccountCredential(tamper setup) error = %v", err)
	}
	providerID, _ := AccountProviderID(scope)
	secrets.tamper(registry.snapshot().Providers[providerID].CredentialRef)
	if _, err := manager.ResolveAccountCredential(ctx, scope); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ResolveAccountCredential(tampered) error = %v, want verification", err)
	}
	if err := manager.ValidateAccountCredentialCurrent(ctx, ready); err != nil {
		t.Fatalf("metadata current check unexpectedly read tampered secret: %v", err)
	}
	if err := manager.Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("Recover(tampered) error = %v, want verification", err)
	}
}
