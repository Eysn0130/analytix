package providerregistry

import (
	"context"
	"errors"
	"strings"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestManagerSnapshotUsesExclusiveRecoveredCloneWithoutCredentialRead(t *testing.T) {
	t.Parallel()

	registryIncarnation := "inc_" + strings.Repeat("a", 43)
	providerIncarnation := "inc_" + strings.Repeat("b", 43)
	credentialRef := secretstoreport.CredentialRef("cred_" + strings.Repeat("C", 43))
	events := make([]string, 0, 1)
	registry := &recordingRegistryStore{
		state: domainregistry.Registry{
			Version: domainregistry.FormatVersion, Revision: 4, Incarnation: registryIncarnation,
			SelectedProviderID: "provider-alpha",
			Providers: map[string]domainregistry.Provider{
				"provider-alpha": {
					ID: "provider-alpha", Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
					Models: []string{"model-alpha"}, MediaModels: []string{}, SelectedModel: "model-alpha",
					SelectedRoutes: []string{"primary"}, CredentialRef: string(credentialRef),
					CredentialPurpose: "provider-api-key", Revision: 3, Generation: 2,
					Incarnation: providerIncarnation,
				},
			},
			Transactions: map[string]domainregistry.Transaction{},
		},
		events: &events,
	}
	secrets := &recordingSecretStore{candidateRef: credentialRef, events: &events}
	manager, err := NewManager(registry, secrets)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	snapshot, err := manager.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if secrets.readbackAuthorized {
		t.Fatal("Snapshot() read credential bytes for a transaction-free list/get query")
	}
	if len(snapshot.Transactions) != 0 || snapshot.Revision != 4 {
		t.Fatalf("Snapshot() = %#v", snapshot)
	}
	provider := snapshot.Providers["provider-alpha"]
	provider.Models[0] = "mutated"
	snapshot.Providers["provider-alpha"] = provider
	delete(snapshot.Providers, "provider-alpha")
	if got := registry.state.Providers["provider-alpha"].Models[0]; got != "model-alpha" {
		t.Fatalf("stored Provider model = %q after snapshot mutation", got)
	}
}

func TestManagerSnapshotRecoversMetadataTransactionAndClearsTransactions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	_ = connectMemoryProvider(t, ctx, base, registry, "provider-query-alpha", "synthetic-query-alpha")
	beta := connectMemoryProvider(t, ctx, base, registry, "provider-query-beta", "synthetic-query-beta")
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterTransactionPrepared))
	_, err := interrupted.Select(ctx, SelectCommand{
		Expected: expectedFor(registry.snapshot(), beta.ID), ProviderID: beta.ID,
	})
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("Select() error = %v, want interrupted", err)
	}

	snapshot, err := mustManager(t, registry, secrets, nil).Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() recovery error = %v", err)
	}
	if snapshot.SelectedProviderID != beta.ID || len(snapshot.Transactions) != 0 {
		t.Fatalf("recovered Snapshot = %#v", snapshot)
	}
}

func TestManagerRecoverFailsClosedWhenCommittedCredentialIsUnreadable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	provider := connectMemoryProvider(t, ctx, manager, registry, "provider-recovery-readback", "synthetic-recovery-readback")
	before := registry.snapshot()
	secrets.remove(provider.CredentialRef)

	err := manager.Recover(ctx)
	if !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("Recover() error = %v, want verification failure", err)
	}
	after := registry.snapshot()
	if after.Revision != before.Revision || after.Providers[provider.ID].CredentialRef != provider.CredentialRef || len(after.Transactions) != 0 {
		t.Fatalf("Recover() rewrote unreadable committed state: %#v", after)
	}
}

func TestManagerExecutionResolutionReReadsWinnerAndFailsClosedAfterDisconnect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	first := connectMemoryProvider(t, ctx, manager, registry, "provider-execution", "synthetic-execution-first")

	firstResolution, err := manager.ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedForExecution(first) error = %v", err)
	}
	if firstResolution.Provider.CredentialRef != first.CredentialRef || string(firstResolution.Credential) != "synthetic-execution-first" {
		firstResolution.Clear()
		t.Fatal("first execution resolution did not return the committed winner")
	}
	firstAuthority := firstResolution.Authority()
	firstResolution.Clear()
	if firstResolution.Credential != nil {
		t.Fatal("execution resolution retained credential bytes after Clear")
	}
	if err := manager.ValidateExecutionCurrent(ctx, firstAuthority); err != nil {
		t.Fatalf("ValidateExecutionCurrent(first) error = %v", err)
	}

	replacement, err := secretstoreport.SetCredential([]byte("synthetic-execution-second"))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	second, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: expectedFor(registry.snapshot(), first.ID), ProviderID: first.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: replacement,
	})
	if err != nil {
		t.Fatalf("ReplaceCredential() error = %v", err)
	}
	secondResolution, err := manager.ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedForExecution(second) error = %v", err)
	}
	if secondResolution.Provider.CredentialRef != second.CredentialRef ||
		secondResolution.Provider.CredentialRef == first.CredentialRef ||
		string(secondResolution.Credential) != "synthetic-execution-second" {
		secondResolution.Clear()
		t.Fatal("execution resolution reused a superseded credential")
	}
	secondAuthority := secondResolution.Authority()
	secondResolution.Clear()
	if err := manager.ValidateExecutionCurrent(ctx, firstAuthority); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("ValidateExecutionCurrent(replaced old authority) error = %v, want conflict", err)
	}
	if err := manager.ValidateExecutionCurrent(ctx, secondAuthority); err != nil {
		t.Fatalf("ValidateExecutionCurrent(replacement) error = %v", err)
	}

	if _, err := manager.Disconnect(ctx, DisconnectCommand{
		Expected: expectedFor(registry.snapshot(), second.ID), ProviderID: second.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
	}); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if _, err := manager.ResolveSelectedForExecution(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ResolveSelectedForExecution(disconnected) error = %v, want verification failure", err)
	}
	if err := manager.ValidateExecutionCurrent(ctx, secondAuthority); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ValidateExecutionCurrent(disconnected) error = %v, want verification failure", err)
	}
}

func TestManagerMediaExecutionReResolvesUpdateReplacementDisconnectAndRestart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	first := connectMemoryProvider(t, ctx, manager, registry, "provider-media-execution", "synthetic-media-first")

	initial, err := manager.ResolveSelectedMediaForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedMediaForExecution(initial) error = %v", err)
	}
	initialAuthority := initial.Authority()
	initial.Clear()

	updated, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), first.ID),
		Provider: domainregistry.ProviderInput{
			ID: first.ID, Kind: first.Kind, Endpoint: "https://media-updated.invalid/v1", Proxy: "http://proxy-updated.invalid:8080",
			Models: first.Models, MediaModels: []string{"media-updated"}, SelectedModel: first.SelectedModel,
			SelectedMedia: "media-updated", SelectedRoutes: first.SelectedRoutes,
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(media route) error = %v", err)
	}
	if err := manager.ValidateMediaExecutionCurrent(ctx, initialAuthority); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("ValidateMediaExecutionCurrent(updated old authority) error = %v, want conflict", err)
	}

	replacement, _ := secretstoreport.SetCredential([]byte("synthetic-media-second"))
	replaced, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: expectedFor(registry.snapshot(), updated.ID), ProviderID: updated.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: replacement,
	})
	if err != nil {
		t.Fatalf("ReplaceCredential(media) error = %v", err)
	}
	restarted := mustManager(t, registry, secrets, nil)
	current, err := restarted.ResolveSelectedMediaForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedMediaForExecution(restart) error = %v", err)
	}
	if current.Provider.Endpoint != "https://media-updated.invalid/v1" || current.Provider.Proxy != "http://proxy-updated.invalid:8080" ||
		current.Provider.SelectedMedia != "media-updated" || current.Provider.CredentialRef != replaced.CredentialRef ||
		string(current.Credential) != "synthetic-media-second" {
		current.Clear()
		t.Fatalf("restart media resolution reused stale authority: %#v", current.Provider)
	}
	currentAuthority := current.Authority()
	current.Clear()
	if err := restarted.ValidateMediaExecutionCurrent(ctx, currentAuthority); err != nil {
		t.Fatalf("ValidateMediaExecutionCurrent(restart winner) error = %v", err)
	}

	if _, err := restarted.Disconnect(ctx, DisconnectCommand{
		Expected: expectedFor(registry.snapshot(), replaced.ID), ProviderID: replaced.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
	}); err != nil {
		t.Fatalf("Disconnect(media) error = %v", err)
	}
	if _, err := restarted.ResolveSelectedMediaForExecution(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ResolveSelectedMediaForExecution(disconnected) error = %v, want verification failure", err)
	}
	if err := restarted.ValidateMediaExecutionCurrent(ctx, currentAuthority); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ValidateMediaExecutionCurrent(disconnected) error = %v, want verification failure", err)
	}
}

func TestManagerMediaExecutionFailsClosedForUnreadableCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	provider := connectMemoryProvider(t, ctx, manager, registry, "provider-media-unreadable", "synthetic-media-unreadable")
	resolution, err := manager.ResolveSelectedMediaForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedMediaForExecution() error = %v", err)
	}
	authority := resolution.Authority()
	resolution.Clear()
	secrets.remove(provider.CredentialRef)
	if _, err := manager.ResolveSelectedMediaForExecution(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ResolveSelectedMediaForExecution(unreadable) error = %v, want verification", err)
	}
	if err := manager.ValidateMediaExecutionCurrent(ctx, authority); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ValidateMediaExecutionCurrent(unreadable) error = %v, want verification", err)
	}
}

func TestManagerExecutionIntentReturnsOnlyKeyFreeCurrentRoute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	provider := connectMemoryProvider(t, ctx, manager, registry, "provider-intent", "synthetic-intent-secret")

	secrets.mu.Lock()
	readsBefore := secrets.readCalls
	secrets.mu.Unlock()
	intent, err := manager.ResolveSelectedIntent(ctx)
	if err != nil || intent.Provider.ID != provider.ID {
		t.Fatalf("ResolveSelectedIntent() provider=%q err=%v", intent.Provider.ID, err)
	}
	secrets.mu.Lock()
	readsAfter := secrets.readCalls
	secrets.mu.Unlock()
	if readsAfter-readsBefore != 1 {
		t.Fatalf("key-free intent resolution checked %d credential routes, want 1", readsAfter-readsBefore)
	}
}

func TestManagerExecutionResolutionUsesDeterministicCommittedRoutePool(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	primary := connectMemoryProvider(t, ctx, manager, registry, "provider-route-primary", "synthetic-route-primary")
	fallback := connectMemoryProvider(t, ctx, manager, registry, "provider-route-fallback", "synthetic-route-fallback")

	configured, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), primary.ID),
		Provider: domainregistry.ProviderInput{
			ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint, Proxy: primary.Proxy,
			Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: primary.SelectedModel,
			SelectedMedia: primary.SelectedMedia, SelectedRoutes: []string{"primary", fallback.ID},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(route pool) error = %v", err)
	}
	if configured.Revision == primary.Revision {
		t.Fatal("route-pool update did not commit a new Provider revision")
	}

	resolved, err := manager.ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedForExecution(primary) error = %v", err)
	}
	if resolved.Provider.ID != primary.ID || string(resolved.Credential) != "synthetic-route-primary" {
		resolved.Clear()
		t.Fatal("legacy primary route did not resolve to the selected committed Provider")
	}
	resolved.Clear()

	secrets.mu.Lock()
	readsBeforeDenied := secrets.readCalls
	secrets.denyRead = true
	secrets.mu.Unlock()
	if _, err := manager.ResolveSelectedForExecution(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ResolveSelectedForExecution(unauthorized) error = %v, want verification failure", err)
	}
	secrets.mu.Lock()
	deniedReads := secrets.readCalls - readsBeforeDenied
	secrets.denyRead = false
	secrets.mu.Unlock()
	if deniedReads != 1 {
		t.Fatalf("unauthorized route resolution read %d credentials, want fail-closed after 1", deniedReads)
	}

	secrets.remove(primary.CredentialRef)
	intent, err := manager.ResolveSelectedIntent(ctx)
	if err != nil || intent.Provider.ID != fallback.ID {
		t.Fatalf("ResolveSelectedIntent(fallback) provider=%q err=%v", intent.Provider.ID, err)
	}
	resolved, err = manager.ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedForExecution(fallback) error = %v", err)
	}
	if resolved.Provider.ID != fallback.ID || string(resolved.Credential) != "synthetic-route-fallback" ||
		resolved.RegistryRevision != registry.snapshot().Revision {
		resolved.Clear()
		t.Fatal("route resolution did not use the first available committed fallback")
	}
	resolved.Clear()

	if _, err := manager.Disconnect(ctx, DisconnectCommand{
		Expected: expectedFor(registry.snapshot(), fallback.ID), ProviderID: fallback.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
	}); err != nil {
		t.Fatalf("Disconnect(fallback) error = %v", err)
	}
	if _, err := manager.ResolveSelectedForExecution(ctx); !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("ResolveSelectedForExecution(exhausted routes) error = %v, want verification failure", err)
	}
}

func TestManagerRoutePoolMutationRejectsUnavailableAndAmbiguousRoutesWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	primary := connectMemoryProvider(t, ctx, manager, registry, "provider-route-validation", "synthetic-route-validation")
	before := registry.snapshot()

	for _, testCase := range []struct {
		name   string
		routes []string
	}{
		{name: "unknown local provider", routes: []string{"provider-route-missing"}},
		{name: "primary alias duplicates provider id", routes: []string{"primary", primary.ID}},
		{name: "legacy alias duplicates canonical self", routes: []string{"primary", "provider:" + primary.ID}},
		{name: "empty canonical provider id", routes: []string{"provider:"}},
		{name: "malformed canonical provider id", routes: []string{"provider:Provider-Invalid"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := manager.Update(ctx, UpdateCommand{
				Expected: expectedFor(before, primary.ID),
				Provider: domainregistry.ProviderInput{
					ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint,
					Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: primary.SelectedModel,
					SelectedMedia: primary.SelectedMedia, SelectedRoutes: testCase.routes,
				},
				Credential: secretstoreport.KeepCredential(),
			})
			if !errors.Is(err, registryport.ErrInvalidRequest) {
				t.Fatalf("Update() error = %v, want invalid request", err)
			}
			after := registry.snapshot()
			if after.Revision != before.Revision || after.Providers[primary.ID].Revision != primary.Revision ||
				len(after.Transactions) != 0 {
				t.Fatalf("invalid route mutation changed Registry state: %#v", after)
			}
		})
	}
}

func TestManagerRoutePoolRejectsCredentialedModelDisabledCandidateWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	primary := connectMemoryProvider(t, ctx, manager, registry, "provider-route-usable", "synthetic-route-usable")
	disabled := connectMemoryProvider(t, ctx, manager, registry, "provider-route-disabled", "synthetic-route-disabled")

	disabled, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), disabled.ID),
		Provider: domainregistry.ProviderInput{
			ID: disabled.ID, Kind: disabled.Kind, Endpoint: disabled.Endpoint,
			Models: disabled.Models, MediaModels: disabled.MediaModels, SelectedModel: "",
			SelectedMedia: disabled.SelectedMedia, SelectedRoutes: []string{"provider:" + primary.ID},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(disable policy Provider) error = %v", err)
	}
	if disabled.SelectedModel != "" || strings.Join(disabled.SelectedRoutes, ",") != "provider:"+primary.ID {
		t.Fatalf("disabled policy Provider = %#v", disabled)
	}

	before := registry.snapshot()
	_, err = manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(before, primary.ID),
		Provider: domainregistry.ProviderInput{
			ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint,
			Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: primary.SelectedModel,
			SelectedMedia: primary.SelectedMedia, SelectedRoutes: []string{"provider:" + disabled.ID},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if !errors.Is(err, registryport.ErrInvalidRequest) {
		t.Fatalf("Update(route to model-disabled Provider) error = %v, want invalid request", err)
	}
	after := registry.snapshot()
	if after.Revision != before.Revision || after.Providers[primary.ID].Revision != before.Providers[primary.ID].Revision ||
		len(after.Transactions) != 0 {
		t.Fatalf("model-disabled route mutation changed Registry state: %#v", after)
	}
}

func TestManagerEmptyDisabledRoutePoolCommitsAndExecutionFailsBeforeCredentialRead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	provider := connectMemoryProvider(t, ctx, manager, registry, "provider-route-disabled-terminal", "synthetic-disabled-terminal")

	disabled, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), provider.ID),
		Provider: domainregistry.ProviderInput{
			ID: provider.ID, Kind: provider.Kind, Endpoint: provider.Endpoint,
			Models: provider.Models, MediaModels: provider.MediaModels, SelectedModel: "",
			SelectedMedia: provider.SelectedMedia, SelectedRoutes: []string{},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(empty disabled terminal) error = %v", err)
	}
	if disabled.SelectedModel != "" || len(disabled.SelectedRoutes) != 0 {
		t.Fatalf("empty disabled Provider = %#v", disabled)
	}
	snapshot, err := manager.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(empty disabled terminal) error = %v", err)
	}
	committed := snapshot.Providers[provider.ID]
	if committed.SelectedModel != "" || len(committed.SelectedRoutes) != 0 ||
		committed.CredentialRef != disabled.CredentialRef {
		t.Fatalf("empty disabled Provider readback = %#v", committed)
	}

	secrets.mu.Lock()
	readsBeforeResolution := secrets.readCalls
	secrets.mu.Unlock()
	resolution, err := manager.ResolveSelectedForExecution(ctx)
	if !errors.Is(err, registryport.ErrVerification) {
		resolution.Clear()
		t.Fatalf("ResolveSelectedForExecution(empty disabled terminal) error = %v, want verification failure", err)
	}
	secrets.mu.Lock()
	readsAfterResolution := secrets.readCalls
	secrets.mu.Unlock()
	if readsAfterResolution != readsBeforeResolution {
		t.Fatalf("empty disabled terminal read %d credentials, want zero", readsAfterResolution-readsBeforeResolution)
	}
	if resolution.Provider.ID != "" || resolution.Credential != nil {
		resolution.Clear()
		t.Fatal("empty disabled terminal guessed an execution Provider")
	}
}

func TestManagerCanonicalRouteCanAddressProviderWhoseExactIDIsPrimary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	exactPrimary := connectMemoryProvider(t, ctx, manager, registry, "primary", "synthetic-exact-primary")
	policy := connectMemoryProvider(t, ctx, manager, registry, "provider-route-policy", "synthetic-route-policy")

	configured, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), policy.ID),
		Provider: domainregistry.ProviderInput{
			ID: policy.ID, Kind: policy.Kind, Endpoint: policy.Endpoint,
			Models: policy.Models, MediaModels: policy.MediaModels, SelectedModel: policy.SelectedModel,
			SelectedMedia:  policy.SelectedMedia,
			SelectedRoutes: []string{"provider:" + exactPrimary.ID, "provider:" + policy.ID},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(canonical collision-safe routes) error = %v", err)
	}
	if strings.Join(configured.SelectedRoutes, ",") != "provider:primary,provider:"+policy.ID {
		t.Fatalf("canonical selectedRoutes round trip = %#v", configured.SelectedRoutes)
	}
	if _, err := manager.Select(ctx, SelectCommand{
		Expected: expectedFor(registry.snapshot(), policy.ID), ProviderID: policy.ID,
	}); err != nil {
		t.Fatalf("Select(route policy) error = %v", err)
	}

	resolved, err := manager.ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedForExecution(exact primary ID) error = %v", err)
	}
	if resolved.Provider.ID != exactPrimary.ID || string(resolved.Credential) != "synthetic-exact-primary" {
		resolved.Clear()
		t.Fatal("canonical provider:primary route collided with the legacy primary alias")
	}
	resolved.Clear()

	policy = registry.snapshot().Providers[policy.ID]
	if _, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), policy.ID),
		Provider: domainregistry.ProviderInput{
			ID: policy.ID, Kind: policy.Kind, Endpoint: policy.Endpoint,
			Models: policy.Models, MediaModels: policy.MediaModels, SelectedModel: policy.SelectedModel,
			SelectedMedia: policy.SelectedMedia, SelectedRoutes: []string{"primary"},
		},
		Credential: secretstoreport.KeepCredential(),
	}); err != nil {
		t.Fatalf("Update(legacy primary alias) error = %v", err)
	}
	resolved, err = manager.ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedForExecution(legacy primary alias) error = %v", err)
	}
	defer resolved.Clear()
	if resolved.Provider.ID != policy.ID || string(resolved.Credential) != "synthetic-route-policy" {
		t.Fatal("legacy primary alias no longer resolved to the route policy Provider")
	}
}

func TestManagerStaleRoutePoolUpdateCannotMutateCommittedOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	primary := connectMemoryProvider(t, ctx, manager, registry, "provider-route-stale-primary", "synthetic-route-stale-primary")
	fallback := connectMemoryProvider(t, ctx, manager, registry, "provider-route-stale-fallback", "synthetic-route-stale-fallback")
	stale := registry.snapshot()

	configured, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(stale, primary.ID),
		Provider: domainregistry.ProviderInput{
			ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint,
			Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: primary.SelectedModel,
			SelectedMedia: primary.SelectedMedia, SelectedRoutes: []string{"primary", fallback.ID},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(first route order) error = %v", err)
	}
	committed := registry.snapshot()

	_, err = manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(stale, primary.ID),
		Provider: domainregistry.ProviderInput{
			ID: primary.ID, Kind: primary.Kind, Endpoint: primary.Endpoint,
			Models: primary.Models, MediaModels: primary.MediaModels, SelectedModel: primary.SelectedModel,
			SelectedMedia: primary.SelectedMedia, SelectedRoutes: []string{fallback.ID, "primary"},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("Update(stale route order) error = %v, want conflict", err)
	}
	after := registry.snapshot()
	if after.Revision != committed.Revision || after.Providers[primary.ID].Revision != configured.Revision ||
		strings.Join(after.Providers[primary.ID].SelectedRoutes, ",") != "primary,"+fallback.ID ||
		len(after.Transactions) != 0 {
		t.Fatalf("stale route update mutated committed order: %#v", after)
	}
}

func TestManagerExecutionResolutionRecoversInterruptedReplacementBeforeRead(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	first := connectMemoryProvider(t, ctx, base, registry, "provider-recovery-execution", "synthetic-recovery-first")
	replacement, err := secretstoreport.SetCredential([]byte("synthetic-recovery-second"))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMetadataCommitted))
	if _, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: expectedFor(registry.snapshot(), first.ID), ProviderID: first.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: replacement,
	}); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("ReplaceCredential() error = %v, want interrupted", err)
	}

	resolution, err := base.ResolveSelectedForExecution(ctx)
	if err != nil {
		t.Fatalf("ResolveSelectedForExecution() error = %v", err)
	}
	defer resolution.Clear()
	if string(resolution.Credential) != "synthetic-recovery-second" ||
		resolution.Provider.CredentialRef == first.CredentialRef || len(registry.snapshot().Transactions) != 0 {
		t.Fatal("execution resolution did not recover the committed replacement before read")
	}
}

type lockedCredentialReadStore struct {
	*memorySecretStore
	locked   bool
	lastRead []byte
}

func (store *lockedCredentialReadStore) GetForAuthorizedConsumer(ctx context.Context, request secretstoreport.AccessRequest) ([]byte, error) {
	if store.locked {
		return nil, secretstoreport.ErrMasterKeyUnavailable
	}
	value, err := store.memorySecretStore.GetForAuthorizedConsumer(ctx, request)
	store.lastRead = value
	return value, err
}

func TestManagerCredentialCheckKeepsConfiguredStateAcrossLockAndRestart(t *testing.T) {
	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := &lockedCredentialReadStore{memorySecretStore: newMemorySecretStore()}
	manager := mustManager(t, registry, secrets, nil)
	command := connectCommand(registry.snapshot(), "provider-onboarding", "synthetic-onboarding-key")
	command.DeferSelection = true
	provider, err := manager.Connect(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	before := registry.snapshot()
	if before.SelectedProviderID != "" {
		t.Fatal("connect completed initialization before validation")
	}
	check := ProviderOperationCommand{Expected: expectedFor(before, provider.ID), ProviderID: provider.ID}
	if err := manager.CheckCredential(ctx, check); err != nil {
		t.Fatal(err)
	}
	for _, value := range secrets.lastRead {
		if value != 0 {
			t.Fatal("credential check retained plaintext")
		}
	}
	secrets.locked = true
	restarted := mustManager(t, registry, secrets, nil)
	if err := restarted.CheckCredential(ctx, check); !errors.Is(err, registryport.ErrCredentialUnavailable) {
		t.Fatalf("locked check = %v", err)
	}
	after := registry.snapshot()
	if after.Revision != before.Revision || after.Providers[provider.ID].CredentialRef != provider.CredentialRef || after.SelectedProviderID != "" {
		t.Fatal("locked check changed initialization or credentials")
	}
	secrets.locked = false
	if err := restarted.CheckCredential(ctx, check); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Select(ctx, SelectCommand{Expected: check.Expected, ProviderID: provider.ID}); err != nil {
		t.Fatal(err)
	}
	if err := restarted.CheckCredential(ctx, check); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("stale check = %v", err)
	}
	current := registry.snapshot()
	currentCheck := ProviderOperationCommand{Expected: expectedFor(current, provider.ID), ProviderID: provider.ID}
	if err := restarted.CheckCredential(ctx, currentCheck); err != nil {
		t.Fatal(err)
	}
	replacement, _ := secretstoreport.SetCredential([]byte("synthetic-explicit-replacement"))
	if _, err := restarted.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: currentCheck.Expected, ProviderID: provider.ID, CredentialPurpose: "provider-api-key", Credential: replacement,
	}); err != nil {
		t.Fatal(err)
	}
	if err := restarted.CheckCredential(ctx, currentCheck); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("old credential generation = %v", err)
	}
	if err := restarted.CheckCredential(ctx, ProviderOperationCommand{Expected: expectedFor(registry.snapshot(), provider.ID), ProviderID: provider.ID}); err != nil {
		t.Fatal(err)
	}
}
