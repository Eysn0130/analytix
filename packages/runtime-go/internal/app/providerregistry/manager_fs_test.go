package providerregistry

import (
	"context"
	"errors"
	"sync"
	"testing"

	providerregistryfs "analytix.local/runtime-go/internal/adapters/outbound/providerregistryfs"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func TestCompetingManagersAndFilesystemStoreInstancesShareOneCASWinner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dataDir := t.TempDir()
	firstStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("New(first) error = %v", err)
	}
	secondStore, err := providerregistryfs.New(dataDir)
	if err != nil {
		t.Fatalf("New(second) error = %v", err)
	}
	secrets := newMemorySecretStore()
	first := mustManager(t, firstStore, secrets, nil)
	second := mustManager(t, secondStore, secrets, nil)
	var initialRevision uint64
	var initialIncarnation string
	if err := firstStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		initialRevision = state.Revision
		initialIncarnation = state.Incarnation
		return err
	}); err != nil {
		t.Fatalf("initial Load() error = %v", err)
	}

	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for index, manager := range []*Manager{first, second} {
		waitGroup.Add(1)
		go func(index int, manager *Manager) {
			defer waitGroup.Done()
			<-start
			command := connectCommandForExpected(initialRevision, initialIncarnation, index)
			_, err := manager.Connect(ctx, command)
			errorsSeen <- err
		}(index, manager)
	}
	close(start)
	waitGroup.Wait()
	close(errorsSeen)
	var successes, conflicts int
	for err := range errorsSeen {
		if err == nil {
			successes++
		} else if errors.Is(err, registryport.ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("competing filesystem Manager error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
	if secrets.recordCount() != 1 || secrets.preparedCount() != 1 {
		t.Fatal("filesystem CAS loser created or committed a candidate secret")
	}
	if err := secondStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		state, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		if len(state.Providers) != 1 || len(state.Transactions) != 0 {
			t.Fatalf("final filesystem Registry = %#v", state)
		}
		return nil
	}); err != nil {
		t.Fatalf("final Load() error = %v", err)
	}
}

func TestFreshFilesystemStoreInstanceRecoversDurableCrashTransaction(t *testing.T) {
	t.Parallel()

	for _, point := range []FaultPoint{FaultAfterCandidateDurable, FaultAfterMetadataCommitted} {
		t.Run(string(point), func(t *testing.T) {
			ctx := context.Background()
			dataDir := t.TempDir()
			firstStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("New(first) error = %v", err)
			}
			secrets := newMemorySecretStore()
			var stateRevision uint64
			var stateIncarnation string
			if err := firstStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				stateRevision = state.Revision
				stateIncarnation = state.Incarnation
				return err
			}); err != nil {
				t.Fatalf("initial Load() error = %v", err)
			}
			manager := mustManager(t, firstStore, secrets, faultOnce(point))
			_, err = manager.Connect(ctx, connectCommandForExpected(stateRevision, stateIncarnation, 0))
			if !errors.Is(err, ErrInterrupted) {
				t.Fatalf("Connect() error = %v, want interrupted", err)
			}
			if err := firstStore.Close(); err != nil {
				t.Fatalf("Close(first) error = %v", err)
			}
			restartedStore, err := providerregistryfs.New(dataDir)
			if err != nil {
				t.Fatalf("New(restarted) error = %v", err)
			}
			restarted := mustManager(t, restartedStore, secrets, nil)
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("fresh filesystem Recover() error = %v", err)
			}
			if err := restarted.Recover(ctx); err != nil {
				t.Fatalf("repeat filesystem Recover() error = %v", err)
			}
			if err := restartedStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
				state, err := transaction.Load(ctx)
				if err != nil {
					return err
				}
				provider, ok := state.Providers["provider-filesystem"]
				if !ok || !secrets.hasActive(provider.CredentialRef) || len(state.Transactions) != 0 {
					t.Fatalf("recovered filesystem Registry = %#v", state)
				}
				return nil
			}); err != nil {
				t.Fatalf("final Load() error = %v", err)
			}
		})
	}
}

func TestPortableManifestFilesystemImportRestartReadbackPreservesDestinationAuthority(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sourceStore, err := providerregistryfs.New(t.TempDir())
	if err != nil {
		t.Fatalf("New(source) error = %v", err)
	}
	sourceSecrets := newMemorySecretStore()
	source := mustManager(t, sourceStore, sourceSecrets, nil)
	var sourceState domainregistry.Registry
	if err := sourceStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		var loadErr error
		sourceState, loadErr = transaction.Load(ctx)
		return loadErr
	}); err != nil {
		t.Fatalf("Load(source) error = %v", err)
	}
	credential, err := secretstoreport.SetCredential([]byte("synthetic-filesystem-portable-secret"))
	if err != nil {
		t.Fatalf("SetCredential(source) error = %v", err)
	}
	sourceProvider, err := source.Connect(ctx, ConnectCommand{
		Expected: expectedForNewPortableTestProvider(sourceState),
		Provider: domainregistry.ProviderInput{
			ID: "provider-filesystem-portable-source", Kind: "openai-compatible", Endpoint: "https://filesystem-source.example/v1",
			Models: []string{"model-filesystem"}, MediaModels: []string{}, SelectedModel: "model-filesystem", SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if err != nil {
		t.Fatalf("Connect(source) error = %v", err)
	}
	exported, err := source.ExportPortableManifest(ctx)
	if err != nil {
		t.Fatalf("ExportPortableManifest(source) error = %v", err)
	}

	destinationDir := t.TempDir()
	destinationStore, err := providerregistryfs.New(destinationDir)
	if err != nil {
		t.Fatalf("New(destination) error = %v", err)
	}
	destinationSecrets := newMemorySecretStore()
	destination := mustManager(t, destinationStore, destinationSecrets, nil)
	result, err := destination.ImportPortableManifest(ctx, exported)
	if err != nil {
		t.Fatalf("ImportPortableManifest(destination) error = %v", err)
	}
	if result.ProviderCount != 1 || result.AccountCount != 0 || len(result.Entries) != 1 {
		t.Fatalf("filesystem import result = %#v", result)
	}
	if result.Entries[0].DestinationProviderID == sourceProvider.ID || result.Entries[0].Status != domainregistry.PortableManifestIntentReentryRequired {
		t.Fatalf("filesystem import transferred source authority: %#v", result.Entries[0])
	}
	if err := destinationStore.Close(); err != nil {
		t.Fatalf("Close(destination) error = %v", err)
	}
	restartedStore, err := providerregistryfs.New(destinationDir)
	if err != nil {
		t.Fatalf("New(restarted destination) error = %v", err)
	}
	restarted := mustManager(t, restartedStore, destinationSecrets, nil)
	state, err := restarted.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot(restarted destination) error = %v", err)
	}
	if state.SelectedProviderID != "" || len(state.Providers) != 1 || len(state.Transactions) != 0 {
		t.Fatalf("restarted destination Registry authority = %#v", state)
	}
	imported := state.Providers[result.Entries[0].DestinationProviderID]
	if imported.ID == "" || imported.ID == sourceProvider.ID || imported.Revision != 1 || imported.Generation != 1 ||
		imported.CredentialRef != "" || imported.CredentialPurpose != "" || len(imported.SelectedRoutes) != 1 ||
		imported.SelectedRoutes[0] != domainregistry.ExactProviderRoutePrefix+imported.ID {
		t.Fatalf("restarted destination Provider authority = %#v", imported)
	}
	if reads, _, _ := destinationSecrets.callCounts(); reads != 0 {
		t.Fatalf("filesystem portable import/restart read Secret Store %d times", reads)
	}
	if err := restartedStore.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		readback, err := transaction.Load(ctx)
		if err != nil {
			return err
		}
		if readback.Providers[imported.ID].ID != imported.ID || readback.Providers[imported.ID].Revision != 1 {
			t.Fatalf("filesystem exact readback Provider = %#v", readback.Providers[imported.ID])
		}
		return nil
	}); err != nil {
		t.Fatalf("filesystem exact readback error = %v", err)
	}
}

func connectCommandForExpected(revision uint64, incarnation string, index int) ConnectCommand {
	state := newMemoryRegistryStore().snapshot()
	state.Revision = revision
	state.Incarnation = incarnation
	command := connectCommand(state, "provider-filesystem", "synthetic-filesystem-candidate")
	credential, _ := secretstoreport.SetCredential([]byte{byte('a' + index), byte('0' + index)})
	command.Credential = credential
	return command
}

func TestDeferredOnboardingSelectionSurvivesFilesystemRecovery(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	first, err := providerregistryfs.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	secrets := newMemorySecretStore()
	manager := mustManager(t, first, secrets, nil)
	snapshot, err := manager.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	command := connectCommandForExpected(snapshot.Revision, snapshot.Incarnation, 0)
	command.DeferSelection = true
	interrupted := mustManager(t, first, secrets, faultOnce(FaultAfterMetadataCommitted))
	if _, err := interrupted.Connect(ctx, command); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("crash = %v", err)
	}
	if err := first.WithExclusive(ctx, func(tx registryport.Transaction) error {
		state, err := tx.Load(ctx)
		if err != nil {
			return err
		}
		for _, pending := range state.Transactions {
			if !pending.DeferSelection {
				t.Fatal("prepare lost deferred initialization intent")
			}
			pending.DeferSelection = false
			if pending.Validate() == nil {
				t.Fatal("tampered selection intent accepted")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := providerregistryfs.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	restarted := mustManager(t, second, secrets, nil)
	if err := restarted.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	restored, err := restarted.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if restored.SelectedProviderID != "" || len(restored.Providers) != 1 || len(restored.Transactions) != 0 {
		t.Fatal("restart incorrectly completed onboarding")
	}
	for id := range restored.Providers {
		if err := restarted.CheckCredential(ctx, ProviderOperationCommand{Expected: expectedFor(restored, id), ProviderID: id}); err != nil {
			t.Fatal(err)
		}
	}
}
