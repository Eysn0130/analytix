package providerregistry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestManagerAllSixMutationsKeepAndExplicitDeleteSemantics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)

	alpha := connectMemoryProvider(t, ctx, manager, registry, "provider-alpha", "synthetic-alpha-secret")
	alphaRef := alpha.CredentialRef
	preparedAfterConnect := secrets.preparedCount()

	state := registry.snapshot()
	updated, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(state, alpha.ID),
		Provider: domainregistry.ProviderInput{
			ID: alpha.ID, Kind: alpha.Kind, Endpoint: "https://provider.invalid/updated",
			Models: []string{"model-alpha", "model-beta"}, MediaModels: []string{"media-alpha"},
			SelectedModel: "model-beta", SelectedMedia: "media-alpha", SelectedRoutes: []string{"primary"},
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(Keep) error = %v", err)
	}
	if updated.CredentialRef != alphaRef || secrets.preparedCount() != preparedAfterConnect || !secrets.hasActive(alphaRef) {
		t.Fatal("Keep update changed or destructively touched the committed credential")
	}

	beta := connectMemoryProvider(t, ctx, manager, registry, "provider-beta", "synthetic-beta-secret")
	state = registry.snapshot()
	selected, err := manager.Select(ctx, SelectCommand{
		Expected: expectedFor(state, beta.ID), ProviderID: beta.ID,
	})
	if err != nil || registry.snapshot().SelectedProviderID != beta.ID || selected.CredentialRef != beta.CredentialRef {
		t.Fatalf("Select() = %#v, %v", selected, err)
	}

	state = registry.snapshot()
	replacement, err := secretstoreport.SetCredential([]byte("synthetic-beta-replacement"))
	if err != nil {
		t.Fatalf("SetCredential() error = %v", err)
	}
	replaced, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: expectedFor(state, beta.ID), ProviderID: beta.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: replacement,
	})
	if err != nil {
		t.Fatalf("ReplaceCredential() error = %v", err)
	}
	if replaced.CredentialRef == beta.CredentialRef || secrets.exists(beta.CredentialRef) || !secrets.hasActive(replaced.CredentialRef) {
		t.Fatal("credential replace did not retire only the superseded credential")
	}

	state = registry.snapshot()
	disconnected, err := manager.Disconnect(ctx, DisconnectCommand{
		Expected: expectedFor(state, beta.ID), ProviderID: beta.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
	})
	if err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	state = registry.snapshot()
	retained, ok := state.Providers[beta.ID]
	if !ok || !retained.Tombstone || retained.CredentialRef != "" || !disconnected.Tombstone || secrets.exists(replaced.CredentialRef) {
		t.Fatal("disconnect did not retain a key-free Provider tombstone and remove only its credential")
	}
	if err := manager.ExplicitDelete(ctx, ExplicitDeleteCommand{
		Expected: expectedFor(state, beta.ID), ProviderID: beta.ID,
	}); err != nil {
		t.Fatalf("ExplicitDelete(tombstone) error = %v", err)
	}
	if _, exists := registry.snapshot().Providers[beta.ID]; exists {
		t.Fatal("explicit delete did not remove the credential-free Provider tombstone")
	}

	state = registry.snapshot()
	if err := manager.ExplicitDelete(ctx, ExplicitDeleteCommand{
		Expected: expectedFor(state, alpha.ID), ProviderID: alpha.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
	}); err != nil {
		t.Fatalf("ExplicitDelete() error = %v", err)
	}
	state = registry.snapshot()
	if _, exists := state.Providers[alpha.ID]; exists || secrets.exists(alphaRef) {
		t.Fatal("explicit delete did not remove the Provider entry and its credential")
	}
	if len(state.Transactions) != 0 {
		t.Fatalf("completed mutations retained transactions: %d", len(state.Transactions))
	}
}

func TestProviderOAuthBindingMutationAdvancesOwnerCASAndPreservesNonOAuthCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	connected := connectMemoryProvider(t, ctx, manager, registry, "provider-oauth", "synthetic-prior-api-key")
	prior := registry.snapshot()
	expected := expectedFor(prior, connected.ID)
	binding := &domainregistry.OAuthBindingMetadata{
		SchemaVersion:         1,
		Issuer:                "https://issuer.example.test/",
		AuthorizationEndpoint: "https://login.example.test/authorize",
		TokenEndpoint:         "https://tokens.example.test/token",
		RevocationEndpoint:    "https://tokens.example.test/revoke",
		ClientID:              "analytix-public-client",
		Scopes:                []string{"openid", "profile"},
		RedirectModeVersion:   1,
	}
	updated, err := manager.Update(ctx, UpdateCommand{
		Expected: expected,
		Provider: domainregistry.ProviderInput{
			ID: connected.ID, Kind: connected.Kind, Endpoint: connected.Endpoint,
			Models: connected.Models, MediaModels: connected.MediaModels,
			SelectedModel: connected.SelectedModel, SelectedMedia: connected.SelectedMedia,
			SelectedRoutes: connected.SelectedRoutes, OAuthBinding: binding,
		},
		Credential: secretstoreport.KeepCredential(),
	})
	if err != nil {
		t.Fatalf("Update(OAuth binding) error = %v", err)
	}
	if updated.Generation != connected.Generation+1 || updated.Revision != connected.Revision+1 ||
		updated.CredentialRef != connected.CredentialRef || updated.CredentialPurpose != connected.CredentialPurpose ||
		updated.OAuthBinding == nil || !reflect.DeepEqual(updated.OAuthBinding, binding) {
		t.Fatalf("OAuth binding successor = %#v", updated)
	}
	if !secrets.hasActive(connected.CredentialRef) {
		t.Fatal("OAuth binding successor discarded the last usable non-OAuth credential")
	}
	if _, err := manager.Update(ctx, UpdateCommand{
		Expected: expected,
		Provider: domainregistry.ProviderInput{
			ID: connected.ID, Kind: connected.Kind, Endpoint: connected.Endpoint,
			Models: connected.Models, MediaModels: connected.MediaModels,
			SelectedModel: connected.SelectedModel, SelectedMedia: connected.SelectedMedia,
			SelectedRoutes: connected.SelectedRoutes, OAuthBinding: binding,
		},
		Credential: secretstoreport.KeepCredential(),
	}); !errors.Is(err, registryport.ErrConflict) {
		t.Fatalf("stale OAuth binding update error = %v", err)
	}
}

func TestManagerSelectVerifiesCurrentCredentialBeforePublishingWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	alpha := connectMemoryProvider(t, ctx, manager, registry, "provider-select-prior", "synthetic-select-prior")
	beta := connectMemoryProvider(t, ctx, manager, registry, "provider-select-missing", "synthetic-select-missing")
	before := registry.snapshot()
	secrets.remove(beta.CredentialRef)
	_, err := manager.Select(ctx, SelectCommand{
		Expected: expectedFor(before, beta.ID), ProviderID: beta.ID,
	})
	if !errors.Is(err, registryport.ErrVerification) {
		t.Fatalf("Select(missing current credential) error = %v, want verification failure", err)
	}
	after := registry.snapshot()
	if after.SelectedProviderID != alpha.ID || !reflect.DeepEqual(after.Providers, before.Providers) ||
		len(after.Transactions) != 0 || !secrets.hasActive(alpha.CredentialRef) {
		t.Fatal("failed Select changed prior metadata/selection or an unrelated active credential")
	}
}

func TestManagerCurrentWinnerReadbackFailuresRollbackLiveAndRestart(t *testing.T) {
	t.Parallel()

	credentialFaults := []struct {
		name  string
		apply func(*testing.T, context.Context, *memorySecretStore, domainregistry.Provider)
	}{
		{name: "missing", apply: func(_ *testing.T, _ context.Context, secrets *memorySecretStore, provider domainregistry.Provider) {
			secrets.remove(provider.CredentialRef)
		}},
		{name: "tombstoned", apply: func(t *testing.T, ctx context.Context, secrets *memorySecretStore, provider domainregistry.Provider) {
			t.Helper()
			if err := secrets.Tombstone(ctx, secretstoreport.CredentialRef(provider.CredentialRef), secretstoreport.Purpose(provider.CredentialPurpose)); err != nil {
				t.Fatalf("Tombstone(current credential) error = %v", err)
			}
		}},
		{name: "tampered", apply: func(_ *testing.T, _ context.Context, secrets *memorySecretStore, provider domainregistry.Provider) {
			secrets.tamper(provider.CredentialRef)
		}},
		{name: "unauthorized", apply: func(_ *testing.T, _ context.Context, secrets *memorySecretStore, _ domainregistry.Provider) {
			secrets.mu.Lock()
			secrets.denyRead = true
			secrets.mu.Unlock()
		}},
	}
	for _, operation := range []string{"select", "update-keep"} {
		for _, restart := range []bool{false, true} {
			mode := "live"
			if restart {
				mode = "restart"
			}
			for _, fault := range credentialFaults {
				t.Run(operation+"/"+mode+"/"+fault.name, func(t *testing.T) {
					ctx := context.Background()
					registry := newMemoryRegistryStore()
					secrets := newMemorySecretStore()
					base := mustManager(t, registry, secrets, nil)
					alpha := connectMemoryProvider(t, ctx, base, registry, "provider-current-alpha", "synthetic-current-alpha")
					beta := connectMemoryProvider(t, ctx, base, registry, "provider-current-beta", "synthetic-current-beta")
					target := beta
					unrelated := alpha
					before := registry.snapshot()
					invoke := func(manager *Manager) error {
						if operation == "select" {
							_, err := manager.Select(ctx, SelectCommand{
								Expected: expectedFor(registry.snapshot(), beta.ID), ProviderID: beta.ID,
							})
							return err
						}
						target = alpha
						unrelated = beta
						state := registry.snapshot()
						current := state.Providers[alpha.ID]
						_, err := manager.Update(ctx, UpdateCommand{
							Expected: expectedFor(state, alpha.ID),
							Provider: domainregistry.ProviderInput{
								ID: current.ID, Kind: current.Kind, Endpoint: "https://provider.invalid/current-updated",
								Models: current.Models, MediaModels: current.MediaModels,
								SelectedModel: current.SelectedModel, SelectedMedia: current.SelectedMedia,
								SelectedRoutes: current.SelectedRoutes,
							},
							Credential: secretstoreport.KeepCredential(),
						})
						return err
					}
					if operation == "update-keep" {
						target = alpha
						unrelated = beta
					}
					if restart {
						interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMetadataCommitted))
						if err := invoke(interrupted); !errors.Is(err, ErrInterrupted) {
							t.Fatalf("%s() error = %v, want interrupted", operation, err)
						}
						fault.apply(t, ctx, secrets, target)
						if err := mustManager(t, registry, secrets, nil).Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
							t.Fatalf("Recover(%s) error = %v, want verification failure", fault.name, err)
						}
					} else {
						fault.apply(t, ctx, secrets, target)
						if err := invoke(base); !errors.Is(err, registryport.ErrVerification) {
							t.Fatalf("%s(%s) error = %v, want verification failure", operation, fault.name, err)
						}
					}
					after := registry.snapshot()
					if after.SelectedProviderID != before.SelectedProviderID ||
						!reflect.DeepEqual(after.Providers, before.Providers) || len(after.Transactions) != 0 ||
						!secrets.hasActive(unrelated.CredentialRef) {
						t.Fatal("winner verification failure changed prior metadata/selection or an unrelated credential")
					}
				})
			}
		}
	}
}

func TestManagerCredentialPurposeChangeUsesAuthoritativeRefPurpose(t *testing.T) {
	t.Parallel()

	t.Run("live replacement", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		prior := connectMemoryProvider(t, ctx, manager, registry, "provider-purpose-live", "synthetic-purpose-prior")
		credential, _ := secretstoreport.SetCredential([]byte("synthetic-purpose-winner"))
		winner, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
			Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
			CredentialPurpose: secretstoreport.Purpose("provider-bearer-token"), Credential: credential,
		})
		if err != nil {
			t.Fatalf("ReplaceCredential(purpose change) error = %v", err)
		}
		if winner.CredentialRef == prior.CredentialRef || winner.CredentialPurpose != "provider-bearer-token" ||
			secrets.exists(prior.CredentialRef) ||
			!secrets.hasActive(winner.CredentialRef) || len(registry.snapshot().Transactions) != 0 {
			t.Fatal("purpose-changing replacement lost the winner or stranded cleanup")
		}
	})

	t.Run("rollback keeps prior purpose", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		prior := connectMemoryProvider(t, ctx, manager, registry, "provider-purpose-rollback", "synthetic-purpose-rollback-prior")
		secrets.wrongNextRead = true
		credential, _ := secretstoreport.SetCredential([]byte("synthetic-purpose-rollback-candidate"))
		_, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
			Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
			CredentialPurpose: secretstoreport.Purpose("provider-bearer-token"), Credential: credential,
		})
		if !errors.Is(err, registryport.ErrVerification) {
			t.Fatalf("ReplaceCredential(wrong readback) error = %v, want verification failure", err)
		}
		current := registry.snapshot().Providers[prior.ID]
		if current.CredentialRef != prior.CredentialRef || current.CredentialPurpose != prior.CredentialPurpose ||
			!secrets.hasActive(prior.CredentialRef) || secrets.recordCount() != 1 || len(registry.snapshot().Transactions) != 0 {
			t.Fatal("purpose-changing rollback lost the prior authority or stranded the candidate")
		}
	})

	t.Run("current purpose CAS", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		prior := connectMemoryProvider(t, ctx, manager, registry, "provider-purpose-cas", "synthetic-purpose-cas-prior")
		before, _ := domainregistry.Marshal(registry.snapshot())
		prepared := secrets.preparedCount()
		expected := expectedFor(registry.snapshot(), prior.ID)
		expected.ProviderCredentialPurpose = "provider-bearer-token"
		credential, _ := secretstoreport.SetCredential([]byte("synthetic-purpose-cas-candidate"))
		_, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
			Expected: expected, ProviderID: prior.ID,
			CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
		})
		if !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("ReplaceCredential(stale purpose) error = %v, want conflict", err)
		}
		after, _ := domainregistry.Marshal(registry.snapshot())
		if !bytes.Equal(before, after) || secrets.preparedCount() != prepared || !secrets.hasActive(prior.CredentialRef) {
			t.Fatal("stale current-purpose CAS changed Registry state or the active credential")
		}
	})

	t.Run("restart cleanup", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		base := mustManager(t, registry, secrets, nil)
		prior := connectMemoryProvider(t, ctx, base, registry, "provider-purpose-restart", "synthetic-purpose-restart-prior")
		credential, _ := secretstoreport.SetCredential([]byte("synthetic-purpose-restart-winner"))
		interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterVerifiedRecorded))
		_, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
			Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
			CredentialPurpose: secretstoreport.Purpose("provider-bearer-token"), Credential: credential,
		})
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("ReplaceCredential() error = %v, want interrupted", err)
		}
		winner := registry.snapshot().Providers[prior.ID]
		if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
			t.Fatalf("Recover(purpose-changing cleanup) error = %v", err)
		}
		if secrets.exists(prior.CredentialRef) || !secrets.hasActive(winner.CredentialRef) || len(registry.snapshot().Transactions) != 0 {
			t.Fatal("restart cleanup used the candidate purpose for the superseded credential")
		}
	})

	t.Run("mismatched delete purpose", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		manager := mustManager(t, registry, secrets, nil)
		provider := connectMemoryProvider(t, ctx, manager, registry, "provider-purpose-delete", "synthetic-purpose-delete")
		before, _ := domainregistry.Marshal(registry.snapshot())
		err := manager.ExplicitDelete(ctx, ExplicitDeleteCommand{
			Expected: expectedFor(registry.snapshot(), provider.ID), ProviderID: provider.ID,
			CredentialPurpose: secretstoreport.Purpose("provider-bearer-token"),
		})
		if !errors.Is(err, registryport.ErrConflict) {
			t.Fatalf("ExplicitDelete(mismatched purpose) error = %v, want conflict", err)
		}
		after, _ := domainregistry.Marshal(registry.snapshot())
		if !bytes.Equal(before, after) || !secrets.hasActive(provider.CredentialRef) {
			t.Fatal("mismatched delete purpose changed Registry state or the active credential")
		}
	})
}

func TestManagerCrashPointsRecoverFreshAndRepeatIdempotently(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		point         FaultPoint
		wantCommitted bool
	}{
		{point: FaultAfterTransactionPrepared},
		{point: FaultAfterCandidateDurable, wantCommitted: true},
		{point: FaultAfterCandidateDurableRecorded, wantCommitted: true},
		{point: FaultAfterMetadataCommitted, wantCommitted: true},
		{point: FaultAfterReadbackVerified, wantCommitted: true},
		{point: FaultAfterVerifiedRecorded, wantCommitted: true},
	} {
		t.Run(string(testCase.point), func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, faultOnce(testCase.point))
			_, err := manager.Connect(ctx, connectCommand(registry.snapshot(), "provider-crash", "synthetic-crash-secret"))
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("Connect() error = %v, want interrupted", err)
			}

			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("fresh Recover() error = %v", err)
			}
			state := registry.snapshot()
			provider, committed := state.Providers["provider-crash"]
			if committed != testCase.wantCommitted {
				t.Fatalf("committed = %t, want %t; state=%#v", committed, testCase.wantCommitted, state)
			}
			if committed && !secrets.hasActive(provider.CredentialRef) {
				t.Fatal("recovered Provider does not have its exact durable candidate")
			}
			if !committed && secrets.recordCount() != 0 {
				t.Fatal("prepared-no-secret recovery retained a credential")
			}
			if len(state.Transactions) != 0 {
				t.Fatalf("fresh recovery retained transactions: %d", len(state.Transactions))
			}
			before, _ := domainregistry.Marshal(state)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("repeat Recover() error = %v", err)
			}
			after, _ := domainregistry.Marshal(registry.snapshot())
			if !bytes.Equal(before, after) {
				t.Fatal("repeat recovery changed finalized Registry bytes")
			}
		})
	}
}

func TestManagerCleanupCrashPointsReplayWithoutDeletingWinner(t *testing.T) {
	t.Parallel()

	for _, point := range []FaultPoint{
		FaultAfterSupersededTombstoned,
		FaultAfterTombstoneRecorded,
		FaultAfterSupersededDeleted,
		FaultAfterDeleteRecorded,
	} {
		t.Run(string(point), func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			base := mustManager(t, registry, secrets, nil)
			prior := connectMemoryProvider(t, ctx, base, registry, "provider-cleanup", "synthetic-prior-secret")
			state := registry.snapshot()
			mutation, _ := secretstoreport.SetCredential([]byte("synthetic-winner-secret"))
			manager := mustManager(t, registry, secrets, faultOnce(point))
			_, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
				Expected: expectedFor(state, prior.ID), ProviderID: prior.ID,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: mutation,
			})
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("ReplaceCredential() error = %v, want interrupted", err)
			}
			winner := registry.snapshot().Providers[prior.ID]
			if winner.CredentialRef == prior.CredentialRef || !secrets.hasActive(winner.CredentialRef) {
				t.Fatal("interrupted cleanup lost or failed to commit the winner")
			}
			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
			if secrets.exists(prior.CredentialRef) || !secrets.hasActive(winner.CredentialRef) {
				t.Fatal("cleanup replay deleted the winner or retained the superseded credential")
			}
			if err := restarted.Recover(ctx); err != nil || len(registry.snapshot().Transactions) != 0 {
				t.Fatalf("repeat cleanup recovery = %v, transactions=%d", err, len(registry.snapshot().Transactions))
			}
		})
	}
}

func TestMetadataOnlyMutationsRecoverFromDurablePreparedPhase(t *testing.T) {
	t.Parallel()

	t.Run("update keep", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		base := mustManager(t, registry, secrets, nil)
		provider := connectMemoryProvider(t, ctx, base, registry, "provider-update-recovery", "synthetic-update-prior")
		manager := mustManager(t, registry, secrets, faultOnce(FaultAfterTransactionPrepared))
		_, err := manager.Update(ctx, UpdateCommand{
			Expected: expectedFor(registry.snapshot(), provider.ID),
			Provider: domainregistry.ProviderInput{
				ID: provider.ID, Kind: provider.Kind, Endpoint: "https://provider.invalid/recovered-update",
				Models: provider.Models, MediaModels: provider.MediaModels, SelectedModel: provider.SelectedModel,
				SelectedMedia: provider.SelectedMedia, SelectedRoutes: provider.SelectedRoutes,
			}, Credential: secretstoreport.KeepCredential(),
		})
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("Update() error = %v", err)
		}
		restarted := mustManager(t, registry, secrets, nil)
		if err := restarted.Recover(ctx); err != nil {
			t.Fatalf("Recover() error = %v", err)
		}
		current := registry.snapshot().Providers[provider.ID]
		if current.Endpoint != "https://provider.invalid/recovered-update" || current.CredentialRef != provider.CredentialRef {
			t.Fatal("prepared Keep update did not recover without changing the credential")
		}
	})

	t.Run("select", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		base := mustManager(t, registry, secrets, nil)
		_ = connectMemoryProvider(t, ctx, base, registry, "provider-select-alpha", "synthetic-select-alpha")
		beta := connectMemoryProvider(t, ctx, base, registry, "provider-select-beta", "synthetic-select-beta")
		manager := mustManager(t, registry, secrets, faultOnce(FaultAfterTransactionPrepared))
		_, err := manager.Select(ctx, SelectCommand{
			Expected: expectedFor(registry.snapshot(), beta.ID), ProviderID: beta.ID,
		})
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("Select() error = %v", err)
		}
		if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
			t.Fatalf("Recover() error = %v", err)
		}
		if registry.snapshot().SelectedProviderID != beta.ID {
			t.Fatal("prepared select did not recover its exact selected Provider")
		}
	})

	t.Run("disconnect", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		base := mustManager(t, registry, secrets, nil)
		provider := connectMemoryProvider(t, ctx, base, registry, "provider-disconnect-recovery", "synthetic-disconnect-prior")
		manager := mustManager(t, registry, secrets, faultOnce(FaultAfterTransactionPrepared))
		_, err := manager.Disconnect(ctx, DisconnectCommand{
			Expected: expectedFor(registry.snapshot(), provider.ID), ProviderID: provider.ID,
			CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
		})
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("Disconnect() error = %v", err)
		}
		if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
			t.Fatalf("Recover() error = %v", err)
		}
		current := registry.snapshot().Providers[provider.ID]
		if !current.Tombstone || current.CredentialRef != "" || secrets.exists(provider.CredentialRef) {
			t.Fatal("prepared disconnect did not recover the tombstone/delete decision")
		}
	})

	t.Run("explicit delete", func(t *testing.T) {
		ctx := context.Background()
		registry := newMemoryRegistryStore()
		secrets := newMemorySecretStore()
		base := mustManager(t, registry, secrets, nil)
		provider := connectMemoryProvider(t, ctx, base, registry, "provider-delete-recovery", "synthetic-delete-prior")
		manager := mustManager(t, registry, secrets, faultOnce(FaultAfterTransactionPrepared))
		err := manager.ExplicitDelete(ctx, ExplicitDeleteCommand{
			Expected: expectedFor(registry.snapshot(), provider.ID), ProviderID: provider.ID,
			CredentialPurpose: secretstoreport.Purpose("provider-api-key"),
		})
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("ExplicitDelete() error = %v", err)
		}
		if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil {
			t.Fatalf("Recover() error = %v", err)
		}
		if _, exists := registry.snapshot().Providers[provider.ID]; exists || secrets.exists(provider.CredentialRef) {
			t.Fatal("prepared explicit delete did not recover the exact removal decision")
		}
	})
}

func TestRecoveryDeletesTombstonedPreparedCandidateBeforeFinalizing(t *testing.T) {
	t.Parallel()

	for _, crashPoint := range []FaultPoint{FaultAfterCandidateDurable, FaultAfterCandidateDurableRecorded} {
		for _, restart := range []bool{false, true} {
			mode := "same-manager"
			if restart {
				mode = "fresh-manager"
			}
			t.Run(string(crashPoint)+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				registry := newMemoryRegistryStore()
				secrets := newMemorySecretStore()
				base := mustManager(t, registry, secrets, nil)
				prior := connectMemoryProvider(t, ctx, base, registry, "provider-tombstoned-candidate", "synthetic-tombstoned-prior")
				credential, _ := secretstoreport.SetCredential([]byte("synthetic-tombstoned-candidate"))
				interrupted := mustManager(t, registry, secrets, faultOnce(crashPoint))
				_, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
					Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
					CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
				})
				if !errors.Is(err, ErrInterrupted) {
					t.Fatalf("ReplaceCredential() error = %v, want interrupted", err)
				}
				candidateRef := onlyTransaction(registry.snapshot()).CandidateCredentialRef
				if err := secrets.Tombstone(ctx, secretstoreport.CredentialRef(candidateRef), secretstoreport.Purpose("provider-api-key")); err != nil {
					t.Fatalf("Tombstone(candidate) error = %v", err)
				}
				recovering := interrupted
				if restart {
					recovering = mustManager(t, registry, secrets, nil)
				}
				if err := recovering.Recover(ctx); err != nil {
					t.Fatalf("Recover(tombstoned candidate) error = %v", err)
				}
				state := registry.snapshot()
				current := state.Providers[prior.ID]
				if current.CredentialRef != prior.CredentialRef || !secrets.hasActive(prior.CredentialRef) ||
					secrets.exists(candidateRef) || len(state.Transactions) != 0 {
					t.Fatal("tombstoned candidate recovery lost the prior winner or left an untracked candidate")
				}
				if err := mustManager(t, registry, secrets, nil).Recover(ctx); err != nil || len(registry.snapshot().Transactions) != 0 {
					t.Fatalf("repeat Recover() error = %v, transactions=%d", err, len(registry.snapshot().Transactions))
				}
			})
		}
	}
}

func TestInterruptedCleanupRefusesToDeleteWhenWinnerFenceChanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	prior := connectMemoryProvider(t, ctx, base, registry, "provider-cleanup-fence", "synthetic-cleanup-fence-prior")
	mutation, _ := secretstoreport.SetCredential([]byte("synthetic-cleanup-fence-winner"))
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterVerifiedRecorded))
	_, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: mutation,
	})
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("ReplaceCredential() error = %v", err)
	}
	registry.stateMu.Lock()
	tampered := registry.state.Clone()
	provider := tampered.Providers[prior.ID]
	provider.Generation++
	tampered.Providers[prior.ID] = provider
	registry.state = tampered
	registry.stateMu.Unlock()
	before, _ := json.Marshal(registry.snapshot())
	err = mustManager(t, registry, secrets, nil).Recover(ctx)
	if !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("Recover(changed winner fence) error = %v, want persistence failure", err)
	}
	after, _ := json.Marshal(registry.snapshot())
	if !bytes.Equal(before, after) || !secrets.exists(prior.CredentialRef) || len(registry.snapshot().Transactions) != 1 {
		t.Fatal("changed winner fence allowed recovery to delete or forget the superseded credential")
	}
}

func TestRecoveryRejectsCandidateAliasBeforeDeletingAnotherProviderCredential(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	alpha := connectMemoryProvider(t, ctx, base, registry, "provider-alias-alpha", "synthetic-alias-alpha")
	beta := connectMemoryProvider(t, ctx, base, registry, "provider-alias-beta", "synthetic-alias-beta")
	credential, _ := secretstoreport.SetCredential([]byte("synthetic-alias-candidate"))
	interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterCandidateDurable))
	_, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: expectedFor(registry.snapshot(), alpha.ID), ProviderID: alpha.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("ReplaceCredential() error = %v, want interrupted", err)
	}
	registry.stateMu.Lock()
	tampered := registry.state.Clone()
	for id, transaction := range tampered.Transactions {
		transaction.CandidateCredentialRef = beta.CredentialRef
		next := transaction.NextProvider.Clone()
		next.CredentialRef = beta.CredentialRef
		transaction.NextProvider = &next
		tampered.Transactions[id] = transaction
	}
	tampered.Revision++
	registry.state = tampered
	registry.stateMu.Unlock()
	before, _ := json.Marshal(registry.snapshot())

	err = mustManager(t, registry, secrets, nil).Recover(ctx)
	if !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("Recover(candidate alias) error = %v, want persistence failure", err)
	}
	after, _ := json.Marshal(registry.snapshot())
	if !bytes.Equal(before, after) || !secrets.hasActive(alpha.CredentialRef) || !secrets.hasActive(beta.CredentialRef) {
		t.Fatal("ambiguous candidate ownership changed Registry state or an active credential")
	}
}

func TestRecoveryRejectsSupersededAliasAndInvalidOperationPhaseShapes(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		fault  FaultPoint
		mutate func(*domainregistry.Registry, domainregistry.Provider)
	}{
		{name: "superseded aliases second Provider", fault: FaultAfterVerifiedRecorded, mutate: func(state *domainregistry.Registry, beta domainregistry.Provider) {
			for id, transaction := range state.Transactions {
				transaction.SupersededCredentialRef = beta.CredentialRef
				transaction.SupersededCredentialPurpose = beta.CredentialPurpose
				transaction.CleanupCredentialRef = beta.CredentialRef
				transaction.CleanupCredentialPurpose = beta.CredentialPurpose
				state.Transactions[id] = transaction
			}
		}},
		{name: "operation shape", fault: FaultAfterCandidateDurable, mutate: func(state *domainregistry.Registry, _ domainregistry.Provider) {
			for id, transaction := range state.Transactions {
				transaction.Operation = domainregistry.OperationSelect
				state.Transactions[id] = transaction
			}
		}},
		{name: "phase shape", fault: FaultAfterCandidateDurable, mutate: func(state *domainregistry.Registry, _ domainregistry.Provider) {
			for id, transaction := range state.Transactions {
				transaction.Phase = domainregistry.PhaseSupersededDeleted
				transaction.Recovery.LastObserved = domainregistry.PhaseSupersededDeleted
				state.Transactions[id] = transaction
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			base := mustManager(t, registry, secrets, nil)
			alpha := connectMemoryProvider(t, ctx, base, registry, "provider-shape-alpha", "synthetic-shape-alpha")
			beta := connectMemoryProvider(t, ctx, base, registry, "provider-shape-beta", "synthetic-shape-beta")
			credential, _ := secretstoreport.SetCredential([]byte("synthetic-shape-candidate"))
			interrupted := mustManager(t, registry, secrets, faultOnce(testCase.fault))
			_, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
				Expected: expectedFor(registry.snapshot(), alpha.ID), ProviderID: alpha.ID,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
			})
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("ReplaceCredential() error = %v, want interrupted", err)
			}
			registry.stateMu.Lock()
			tampered := registry.state.Clone()
			testCase.mutate(&tampered, beta)
			registry.state = tampered
			registry.stateMu.Unlock()
			before, _ := json.Marshal(registry.snapshot())
			alphaCurrent := registry.snapshot().Providers[alpha.ID]

			err = mustManager(t, registry, secrets, nil).Recover(ctx)
			if !errors.Is(err, registryport.ErrPersistence) {
				t.Fatalf("Recover(ambiguous transaction) error = %v, want persistence failure", err)
			}
			after, _ := json.Marshal(registry.snapshot())
			if !bytes.Equal(before, after) || !secrets.hasActive(alphaCurrent.CredentialRef) || !secrets.hasActive(beta.CredentialRef) {
				t.Fatal("invalid operation/phase/reference ownership changed Registry state or an active credential")
			}
		})
	}
}

func TestRecoveryRejectsTombstoneResurrectionAndSelectionBeforeMutation(t *testing.T) {
	t.Parallel()

	for _, operation := range []domainregistry.Operation{domainregistry.OperationUpdate, domainregistry.OperationSelect} {
		t.Run(string(operation), func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			base := mustManager(t, registry, secrets, nil)
			beta := connectMemoryProvider(t, ctx, base, registry, "provider-tombstone-beta", "synthetic-tombstone-beta")
			alpha := connectMemoryProvider(t, ctx, base, registry, "provider-tombstone-alpha", "synthetic-tombstone-alpha")
			interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterTransactionPrepared))
			var err error
			if operation == domainregistry.OperationUpdate {
				_, err = interrupted.Update(ctx, UpdateCommand{
					Expected: expectedFor(registry.snapshot(), alpha.ID),
					Provider: domainregistry.ProviderInput{
						ID: alpha.ID, Kind: alpha.Kind, Endpoint: "https://provider.invalid/resurrection",
						Models: alpha.Models, MediaModels: alpha.MediaModels, SelectedModel: alpha.SelectedModel,
						SelectedMedia: alpha.SelectedMedia, SelectedRoutes: alpha.SelectedRoutes,
					}, Credential: secretstoreport.KeepCredential(),
				})
			} else {
				_, err = interrupted.Select(ctx, SelectCommand{
					Expected: expectedFor(registry.snapshot(), alpha.ID), ProviderID: alpha.ID,
				})
			}
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("prepare %s error = %v, want interrupted", operation, err)
			}
			registry.stateMu.Lock()
			tampered := registry.state.Clone()
			pending := onlyTransaction(tampered)
			prior := pending.PriorProvider.Clone()
			prior.CredentialRef = ""
			prior.CredentialPurpose = ""
			prior.SelectedRoutes = nil
			prior.Tombstone = true
			next := pending.NextProvider.Clone()
			next.CredentialRef = ""
			next.CredentialPurpose = ""
			if operation == domainregistry.OperationSelect {
				next = prior.Clone()
				next.Revision++
			}
			pending.PriorProvider = &prior
			pending.NextProvider = &next
			pending.Fence.CurrentCredentialRef = ""
			pending.Fence.CurrentCredentialPurpose = ""
			tampered.Providers[alpha.ID] = prior
			tampered.Transactions[pending.ID] = pending
			registry.state = tampered
			registry.stateMu.Unlock()
			before, _ := json.Marshal(registry.snapshot())
			reads, tombstones, deletes := secrets.callCounts()
			err = mustManager(t, registry, secrets, nil).Recover(ctx)
			if !errors.Is(err, registryport.ErrPersistence) {
				t.Fatalf("Recover(tombstoned %s) error = %v, want persistence failure", operation, err)
			}
			after, _ := json.Marshal(registry.snapshot())
			afterReads, afterTombstones, afterDeletes := secrets.callCounts()
			if !bytes.Equal(before, after) || reads != afterReads || tombstones != afterTombstones || deletes != afterDeletes ||
				!secrets.hasActive(alpha.CredentialRef) || !secrets.hasActive(beta.CredentialRef) {
				t.Fatal("malformed tombstone operation mutated Registry recovery state or called K1")
			}
		})
	}
}

func TestManagerStaleRevisionGenerationAndIncarnationFailBeforeSecretMutation(t *testing.T) {
	t.Parallel()

	for _, mutate := range []struct {
		name string
		use  func(*domainregistry.ExpectedState)
	}{
		{name: "registry revision", use: func(expected *domainregistry.ExpectedState) { expected.RegistryRevision++ }},
		{name: "provider revision", use: func(expected *domainregistry.ExpectedState) { expected.ProviderRevision++ }},
		{name: "provider generation", use: func(expected *domainregistry.ExpectedState) { expected.ProviderGeneration++ }},
		{name: "provider incarnation", use: func(expected *domainregistry.ExpectedState) {
			expected.ProviderIncarnation = "inc_" + strings.Repeat("z", 43)
		}},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			provider := connectMemoryProvider(t, ctx, manager, registry, "provider-stale", "synthetic-stale-prior")
			before, _ := domainregistry.Marshal(registry.snapshot())
			prepared := secrets.preparedCount()
			expected := expectedFor(registry.snapshot(), provider.ID)
			mutate.use(&expected)
			credential, _ := secretstoreport.SetCredential([]byte("synthetic-stale-candidate"))
			_, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
				Expected: expected, ProviderID: provider.ID,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
			})
			if !errors.Is(err, registryport.ErrConflict) || secrets.preparedCount() != prepared {
				t.Fatalf("ReplaceCredential(stale) error = %v, prepared=%d want=%d", err, secrets.preparedCount(), prepared)
			}
			after, _ := domainregistry.Marshal(registry.snapshot())
			if !bytes.Equal(before, after) || strings.Contains(err.Error(), provider.ID) || strings.Contains(err.Error(), provider.CredentialRef) {
				t.Fatal("stale conflict changed winner state or leaked identifiers")
			}
		})
	}
}

func TestManagerRejectsProviderKindRewriteWithoutMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	provider := connectMemoryProvider(t, ctx, manager, registry, "provider-stable-kind", "synthetic-stable-kind")
	before, _ := domainregistry.Marshal(registry.snapshot())
	_, err := manager.Update(ctx, UpdateCommand{
		Expected: expectedFor(registry.snapshot(), provider.ID),
		Provider: domainregistry.ProviderInput{
			ID: provider.ID, Kind: "different-provider-kind", Endpoint: provider.Endpoint,
			Models: provider.Models, MediaModels: provider.MediaModels, SelectedModel: provider.SelectedModel,
			SelectedMedia: provider.SelectedMedia, SelectedRoutes: provider.SelectedRoutes,
		}, Credential: secretstoreport.KeepCredential(),
	})
	if !errors.Is(err, registryport.ErrInvalidRequest) {
		t.Fatalf("Update(kind rewrite) error = %v, want invalid request", err)
	}
	after, _ := domainregistry.Marshal(registry.snapshot())
	if !bytes.Equal(before, after) || secrets.recordCount() != 1 {
		t.Fatal("rejected Provider kind rewrite changed Registry or Secret Store state")
	}
}

func TestManagerDependencyFailuresAreStableAndRedacted(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name    string
		manager func(*testing.T) *Manager
	}{
		{name: "Registry lock failure", manager: func(t *testing.T) *Manager {
			return mustManager(t, errorRegistryStore{err: errors.New("/private/data cred_" + strings.Repeat("S", 43) + " synthetic-provider-secret-marker")}, newMemorySecretStore(), nil)
		}},
		{name: "Secret Store prepare failure", manager: func(t *testing.T) *Manager {
			secrets := newMemorySecretStore()
			secrets.prepareErr = errors.New("/private/data provider-api-key synthetic-provider-secret-marker")
			return mustManager(t, newMemoryRegistryStore(), secrets, nil)
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			state := newMemoryRegistryStore().snapshot()
			_, err := testCase.manager(t).Connect(context.Background(), connectCommand(state, "provider-redaction", "synthetic-provider-secret-marker"))
			if err == nil {
				t.Fatal("Connect() error = nil")
			}
			for _, forbidden := range []string{"/private/data", "cred_", "provider-api-key", "synthetic-provider-secret-marker"} {
				if strings.Contains(err.Error(), forbidden) {
					t.Fatalf("error leaked forbidden material: %q", err.Error())
				}
			}
		})
	}
}

func TestCompetingManagersCommitAtMostOneWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	base := mustManager(t, registry, secrets, nil)
	provider := connectMemoryProvider(t, ctx, base, registry, "provider-race", "synthetic-race-prior")
	expected := expectedFor(registry.snapshot(), provider.ID)
	managers := []*Manager{mustManager(t, registry, secrets, nil), mustManager(t, registry, secrets, nil)}
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	for index, manager := range managers {
		go func(index int, manager *Manager) {
			<-start
			credential, _ := secretstoreport.SetCredential([]byte(fmt.Sprintf("synthetic-race-candidate-%d", index)))
			_, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
				Expected: expected, ProviderID: provider.ID,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
			})
			errorsSeen <- err
		}(index, manager)
	}
	close(start)
	var successes, conflicts int
	for range managers {
		err := <-errorsSeen
		switch {
		case err == nil:
			successes++
		case errors.Is(err, registryport.ErrConflict):
			conflicts++
		default:
			t.Fatalf("competing mutation error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	winner := registry.snapshot().Providers[provider.ID]
	if !secrets.hasActive(winner.CredentialRef) || secrets.recordCount() != 1 || secrets.preparedCount() != 2 {
		t.Fatal("competing Managers did not retain exactly one candidate winner")
	}
}

func TestManagerFailuresFailClosedAndPreserveLastUsableProvider(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		configure func(*memoryRegistryStore, *memorySecretStore)
		want      error
	}{
		{name: "candidate commit failure", configure: func(_ *memoryRegistryStore, secrets *memorySecretStore) { secrets.failCandidateCommit = true }, want: registryport.ErrPersistence},
		{name: "Registry prepare persistence failure", configure: func(registry *memoryRegistryStore, _ *memorySecretStore) { registry.failNextCommit = true }, want: registryport.ErrPersistence},
		{name: "missing committed candidate", configure: func(_ *memoryRegistryStore, secrets *memorySecretStore) { secrets.dropNextCommit = true }, want: registryport.ErrVerification},
		{name: "unauthorized readback", configure: func(_ *memoryRegistryStore, secrets *memorySecretStore) { secrets.denyRead = true }, want: registryport.ErrVerification},
		{name: "wrong candidate readback", configure: func(_ *memoryRegistryStore, secrets *memorySecretStore) { secrets.wrongNextRead = true }, want: registryport.ErrVerification},
		{name: "tampered candidate readback", configure: func(_ *memoryRegistryStore, secrets *memorySecretStore) { secrets.tamperNextCommit = true }, want: registryport.ErrVerification},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			manager := mustManager(t, registry, secrets, nil)
			prior := connectMemoryProvider(t, ctx, manager, registry, "provider-failure", "synthetic-failure-prior")
			testCase.configure(registry, secrets)
			credential, _ := secretstoreport.SetCredential([]byte("synthetic-failure-candidate"))
			_, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
				Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
			})
			if !errors.Is(err, testCase.want) {
				t.Fatalf("ReplaceCredential() error = %v, want %v", err, testCase.want)
			}
			secrets.clearFailures()
			registry.clearFailures()
			if recoverErr := manager.Recover(ctx); recoverErr != nil {
				t.Fatalf("Recover() error = %v", recoverErr)
			}
			current := registry.snapshot().Providers[prior.ID]
			if current.CredentialRef != prior.CredentialRef || !secrets.hasActive(prior.CredentialRef) {
				t.Fatal("failed mutation did not preserve the last usable Provider")
			}
			if len(registry.snapshot().Transactions) != 0 || secrets.recordCount() != 1 {
				t.Fatal("failed mutation retained an untracked candidate or unfinished transaction")
			}
		})
	}
}

func TestRegistryPersistenceFailureAfterCandidateDurableIsRecoverable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	registry := newMemoryRegistryStore()
	secrets := newMemorySecretStore()
	manager := mustManager(t, registry, secrets, nil)
	prior := connectMemoryProvider(t, ctx, manager, registry, "provider-atomic", "synthetic-atomic-prior")
	registry.stateMu.Lock()
	registry.failCommitAt = registry.commits + 2
	registry.stateMu.Unlock()
	credential, _ := secretstoreport.SetCredential([]byte("synthetic-atomic-candidate"))
	_, err := manager.ReplaceCredential(ctx, CredentialReplaceCommand{
		Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if !errors.Is(err, registryport.ErrPersistence) {
		t.Fatalf("ReplaceCredential() error = %v, want persistence failure", err)
	}
	registry.clearFailures()
	if err := manager.Recover(ctx); err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	winner := registry.snapshot().Providers[prior.ID]
	if winner.CredentialRef == prior.CredentialRef || !secrets.hasActive(winner.CredentialRef) || secrets.exists(prior.CredentialRef) {
		t.Fatal("durably named orphan candidate was not recovered to one verified winner")
	}
}

func TestRestartRecoveryMissingTamperedOrUnauthorizedCandidateRollsBackExactly(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		configure func(*memorySecretStore, string)
	}{
		{name: "missing", configure: func(secrets *memorySecretStore, ref string) { secrets.remove(ref) }},
		{name: "tampered", configure: func(secrets *memorySecretStore, ref string) { secrets.tamper(ref) }},
		{name: "unauthorized", configure: func(secrets *memorySecretStore, _ string) {
			secrets.mu.Lock()
			defer secrets.mu.Unlock()
			secrets.denyRead = true
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			base := mustManager(t, registry, secrets, nil)
			prior := connectMemoryProvider(t, ctx, base, registry, "provider-restart-failure", "synthetic-restart-prior")
			mutation, _ := secretstoreport.SetCredential([]byte("synthetic-restart-candidate"))
			interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterMetadataCommitted))
			_, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
				Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: mutation,
			})
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("ReplaceCredential() error = %v", err)
			}
			pending := onlyTransaction(registry.snapshot())
			testCase.configure(secrets, pending.CandidateCredentialRef)
			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); !errors.Is(err, registryport.ErrVerification) {
				t.Fatalf("Recover() error = %v, want verification failure", err)
			}
			secrets.clearFailures()
			state := registry.snapshot()
			current := state.Providers[prior.ID]
			if current.CredentialRef != prior.CredentialRef || !secrets.hasActive(prior.CredentialRef) {
				t.Fatal("restart verification failure did not restore the exact prior usable Provider")
			}
			if len(state.Transactions) != 0 || secrets.recordCount() != 1 {
				t.Fatal("restart verification failure retained ambiguous transaction or candidate state")
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("repeat Recover() error = %v", err)
			}
		})
	}
}

func TestRecoveryCleansExactOrphanCandidateAfterFenceChanges(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name            string
		providerPresent bool
		mutate          func(*domainregistry.Registry, string)
	}{
		{name: "Registry revision", providerPresent: true, mutate: func(state *domainregistry.Registry, _ string) { state.Revision++ }},
		{name: "Provider generation", providerPresent: true, mutate: func(state *domainregistry.Registry, id string) {
			provider := state.Providers[id]
			provider.Generation++
			state.Providers[id] = provider
		}},
		{name: "Provider incarnation", providerPresent: true, mutate: func(state *domainregistry.Registry, id string) {
			provider := state.Providers[id]
			provider.Incarnation = "inc_" + strings.Repeat("q", 43)
			state.Providers[id] = provider
		}},
		{name: "Provider removed orphan", mutate: func(state *domainregistry.Registry, id string) {
			delete(state.Providers, id)
			state.SelectedProviderID = ""
			state.Revision++
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			registry := newMemoryRegistryStore()
			secrets := newMemorySecretStore()
			base := mustManager(t, registry, secrets, nil)
			prior := connectMemoryProvider(t, ctx, base, registry, "provider-orphan", "synthetic-orphan-prior")
			credential, _ := secretstoreport.SetCredential([]byte("synthetic-orphan-candidate"))
			interrupted := mustManager(t, registry, secrets, faultOnce(FaultAfterCandidateDurable))
			_, err := interrupted.ReplaceCredential(ctx, CredentialReplaceCommand{
				Expected: expectedFor(registry.snapshot(), prior.ID), ProviderID: prior.ID,
				CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
			})
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("ReplaceCredential() error = %v", err)
			}
			candidateRef := onlyTransaction(registry.snapshot()).CandidateCredentialRef
			registry.mutate(func(state *domainregistry.Registry) { testCase.mutate(state, prior.ID) })
			restarted := mustManager(t, registry, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("Recover() error = %v", err)
			}
			state := registry.snapshot()
			current, present := state.Providers[prior.ID]
			if present != testCase.providerPresent || (present && current.CredentialRef != prior.CredentialRef) ||
				secrets.exists(candidateRef) || !secrets.hasActive(prior.CredentialRef) || len(state.Transactions) != 0 {
				t.Fatal("stale fence recovery guessed a winner or retained the exact orphan candidate")
			}
		})
	}
}

type memoryRegistryStore struct {
	gate           sync.Mutex
	stateMu        sync.Mutex
	state          domainregistry.Registry
	commits        int
	failCommitAt   int
	failNextCommit bool
}

type errorRegistryStore struct{ err error }

func (store errorRegistryStore) WithExclusive(context.Context, func(registryport.Transaction) error) error {
	return store.err
}

func newMemoryRegistryStore() *memoryRegistryStore {
	return &memoryRegistryStore{state: domainregistry.Registry{
		Version: domainregistry.FormatVersion, Incarnation: "inc_" + strings.Repeat("a", 43),
		Providers: make(map[string]domainregistry.Provider), Transactions: make(map[string]domainregistry.Transaction),
	}}
}

func (store *memoryRegistryStore) WithExclusive(_ context.Context, use func(registryport.Transaction) error) error {
	store.gate.Lock()
	defer store.gate.Unlock()
	return use(memoryRegistryTransaction{store: store})
}

type memoryRegistryTransaction struct{ store *memoryRegistryStore }

func (transaction memoryRegistryTransaction) Load(context.Context) (domainregistry.Registry, error) {
	transaction.store.stateMu.Lock()
	defer transaction.store.stateMu.Unlock()
	return transaction.store.state.Clone(), nil
}

func (transaction memoryRegistryTransaction) Commit(_ context.Context, state domainregistry.Registry) error {
	transaction.store.stateMu.Lock()
	defer transaction.store.stateMu.Unlock()
	transaction.store.commits++
	if transaction.store.failNextCommit || (transaction.store.failCommitAt != 0 && transaction.store.commits == transaction.store.failCommitAt) {
		transaction.store.failNextCommit = false
		return registryport.ErrPersistence
	}
	if state.Validate() != nil {
		return registryport.ErrPersistence
	}
	transaction.store.state = state.Clone()
	return nil
}

func (store *memoryRegistryStore) snapshot() domainregistry.Registry {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	return store.state.Clone()
}

func (store *memoryRegistryStore) mutate(use func(*domainregistry.Registry)) {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	state := store.state.Clone()
	use(&state)
	if state.Validate() != nil {
		panic("test mutated Registry into invalid state")
	}
	store.state = state
}

func (store *memoryRegistryStore) clearFailures() {
	store.stateMu.Lock()
	defer store.stateMu.Unlock()
	store.failCommitAt = 0
	store.failNextCommit = false
}

type memorySecretRecord struct {
	purpose    secretstoreport.Purpose
	secret     []byte
	tombstoned bool
	tampered   bool
}

type memorySecretStore struct {
	mu                     sync.Mutex
	records                map[string]memorySecretRecord
	next                   int
	prepared               int
	failCandidateCommit    bool
	dropNextCommit         bool
	denyRead               bool
	wrongNextRead          bool
	wrongProtectedReadback bool
	tamperNextCommit       bool
	prepareErr             error
	failTombstone          bool
	failDelete             bool
	readCalls              int
	tombstoneCalls         int
	deleteCalls            int
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{records: make(map[string]memorySecretRecord)}
}

func (store *memorySecretStore) PreparePut(
	_ context.Context,
	purpose secretstoreport.Purpose,
	secret []byte,
) (secretstoreport.PreparedCandidate, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.prepareErr != nil {
		return nil, store.prepareErr
	}
	store.next++
	store.prepared++
	return &memoryPreparedCandidate{
		store: store, ref: secretstoreport.CredentialRef("cred_" + fmt.Sprintf("%043d", store.next)),
		purpose: purpose, secret: bytes.Clone(secret),
	}, nil
}

func (store *memorySecretStore) GetForAuthorizedConsumer(_ context.Context, request secretstoreport.AccessRequest) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.readCalls++
	if (request.Consumer != RegistryReadbackConsumer && request.Consumer != ProviderExecutionConsumer &&
		request.Consumer != AccountExecutionConsumer && request.Consumer != ProtectedTransferSourceConsumer &&
		request.Consumer != ProtectedRecoveryPendingKeyConsumer && request.Consumer != ProtectedRecoveryReadbackConsumer) || store.denyRead {
		return nil, secretstoreport.ErrUnauthorized
	}
	if request.Purpose == ProtectedRecoveryPendingKeyPurpose && request.Consumer != ProtectedRecoveryPendingKeyConsumer {
		return nil, secretstoreport.ErrUnauthorized
	}
	record, ok := store.records[string(request.CredentialRef)]
	if !ok {
		return nil, secretstoreport.ErrNotFound
	}
	if record.tombstoned {
		return nil, secretstoreport.ErrTombstoned
	}
	if record.purpose != request.Purpose {
		return nil, secretstoreport.ErrUnauthorized
	}
	if record.tampered {
		return nil, secretstoreport.ErrCryptographicFailure
	}
	if request.Consumer == ProtectedRecoveryReadbackConsumer && store.wrongProtectedReadback {
		store.wrongProtectedReadback = false
		return []byte("synthetic-wrong-protected-readback"), nil
	}
	if store.wrongNextRead {
		store.wrongNextRead = false
		return []byte("synthetic-wrong-candidate"), nil
	}
	return bytes.Clone(record.secret), nil
}

func (store *memorySecretStore) Tombstone(_ context.Context, ref secretstoreport.CredentialRef, purpose secretstoreport.Purpose) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.tombstoneCalls++
	if store.failTombstone {
		return secretstoreport.ErrPersistence
	}
	record, ok := store.records[string(ref)]
	if !ok {
		return secretstoreport.ErrNotFound
	}
	if record.purpose != purpose {
		return secretstoreport.ErrConflict
	}
	if record.tombstoned {
		return nil
	}
	record.tombstoned = true
	store.records[string(ref)] = record
	return nil
}

func (store *memorySecretStore) ExplicitDelete(
	_ context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
	intent secretstoreport.CredentialMutation,
) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.deleteCalls++
	if store.failDelete {
		return secretstoreport.ErrPersistence
	}
	record, ok := store.records[string(ref)]
	if !ok {
		return secretstoreport.ErrNotFound
	}
	if intent.Kind() != secretstoreport.MutationExplicitDelete || record.purpose != purpose || !record.tombstoned {
		return secretstoreport.ErrConflict
	}
	clear(record.secret)
	delete(store.records, string(ref))
	return nil
}

type memoryPreparedCandidate struct {
	store     *memorySecretStore
	ref       secretstoreport.CredentialRef
	purpose   secretstoreport.Purpose
	secret    []byte
	committed bool
	aborted   bool
}

func (candidate *memoryPreparedCandidate) CredentialRef() secretstoreport.CredentialRef {
	return candidate.ref
}

func (candidate *memoryPreparedCandidate) Commit(context.Context) error {
	candidate.store.mu.Lock()
	defer candidate.store.mu.Unlock()
	if candidate.aborted {
		return secretstoreport.ErrConflict
	}
	if candidate.committed {
		return nil
	}
	if candidate.store.failCandidateCommit {
		return secretstoreport.ErrPersistence
	}
	candidate.committed = true
	if candidate.store.dropNextCommit {
		candidate.store.dropNextCommit = false
		return nil
	}
	candidate.store.records[string(candidate.ref)] = memorySecretRecord{
		purpose: candidate.purpose, secret: bytes.Clone(candidate.secret), tampered: candidate.store.tamperNextCommit,
	}
	candidate.store.tamperNextCommit = false
	return nil
}

func (candidate *memoryPreparedCandidate) Abort() {
	if candidate.committed {
		return
	}
	candidate.aborted = true
	clear(candidate.secret)
}

func (store *memorySecretStore) clearFailures() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.failCandidateCommit = false
	store.dropNextCommit = false
	store.denyRead = false
	store.wrongNextRead = false
	store.wrongProtectedReadback = false
	store.tamperNextCommit = false
	store.prepareErr = nil
	store.failTombstone = false
	store.failDelete = false
}

func (store *memorySecretStore) exists(ref string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.records[ref]
	return ok
}

func (store *memorySecretStore) hasActive(ref string) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.records[ref]
	return ok && !record.tombstoned && !record.tampered
}

func (store *memorySecretStore) recordCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.records)
}

func (store *memorySecretStore) preparedCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.prepared
}

func (store *memorySecretStore) callCounts() (int, int, int) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.readCalls, store.tombstoneCalls, store.deleteCalls
}

func (store *memorySecretStore) remove(ref string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if record, ok := store.records[ref]; ok {
		clear(record.secret)
	}
	delete(store.records, ref)
}

func (store *memorySecretStore) tamper(ref string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record := store.records[ref]
	record.tampered = true
	store.records[ref] = record
}

func mustManager(t *testing.T, registry registryport.Store, secrets secretstoreport.RegistryStore, faults FaultRecorder) *Manager {
	t.Helper()
	manager, err := NewManagerWithFaultRecorder(registry, secrets, faults)
	if err != nil {
		t.Fatalf("NewManagerWithFaultRecorder() error = %v", err)
	}
	return manager
}

func connectMemoryProvider(
	t *testing.T,
	ctx context.Context,
	manager *Manager,
	registry *memoryRegistryStore,
	id, secret string,
) domainregistry.Provider {
	t.Helper()
	provider, err := manager.Connect(ctx, connectCommand(registry.snapshot(), id, secret))
	if err != nil {
		t.Fatalf("Connect(%s) error = %v", id, err)
	}
	return provider
}

func connectCommand(state domainregistry.Registry, id, secret string) ConnectCommand {
	credential, _ := secretstoreport.SetCredential([]byte(secret))
	return ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: id, Kind: "openai-compatible", Endpoint: "https://provider.invalid/v1",
			Models: []string{"model-alpha"}, MediaModels: []string{"media-alpha"},
			SelectedModel: "model-alpha", SelectedMedia: "media-alpha", SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	}
}

func expectedFor(state domainregistry.Registry, id string) domainregistry.ExpectedState {
	provider := state.Providers[id]
	return domainregistry.ExpectedState{
		RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
		ProviderRevision: provider.Revision, ProviderGeneration: provider.Generation,
		ProviderIncarnation: provider.Incarnation, ProviderCredentialPurpose: provider.CredentialPurpose,
	}
}

func faultOnce(want FaultPoint) FaultRecorder {
	var mu sync.Mutex
	fired := false
	return FaultRecorderFunc(func(point FaultPoint) error {
		mu.Lock()
		defer mu.Unlock()
		if point == want && !fired {
			fired = true
			return errors.New("synthetic crash")
		}
		return nil
	})
}

func onlyTransaction(state domainregistry.Registry) domainregistry.Transaction {
	for _, transaction := range state.Transactions {
		return transaction
	}
	panic("test expected one transaction")
}
