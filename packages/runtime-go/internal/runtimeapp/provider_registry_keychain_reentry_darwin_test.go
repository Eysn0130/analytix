//go:build darwin

package runtimeapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	providerregistryfs "analytix.local/runtime-go/internal/adapters/outbound/providerregistryfs"
	secretstore "analytix.local/runtime-go/internal/adapters/outbound/secretstore"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
	provider "analytix.local/runtime-go/internal/provider"
)

func TestFreshOrdinaryMacProfileSavesAndReloadsWithoutPreseededMasterKey(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = connectProviderRegistryTestWinnerV1(t, ctx, authority.Manager(), "provider-fresh-mac", "synthetic-fresh-key")
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(dataDir, "private", "provider-secrets", "master-key", "authority.v1"))
	if err != nil || string(marker) != "analytix-master-key-authority:v1:fallback\n" {
		t.Fatalf("fresh profile did not select private file authority: %v", err)
	}
	restarted, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	resolved, err := newProviderRegistryExecutionResolverV1(restarted.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || resolved.Config.APIKey != "synthetic-fresh-key" {
		t.Fatalf("fresh Provider credential did not survive restart: %v", err)
	}
}

func TestOrdinaryMacMissingMasterKeyPreservesCommittedCredential(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = connectProviderRegistryTestWinnerV1(t, ctx, authority.Manager(), "provider-missing-master-key", "synthetic-private-key")
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
	secretPath := filepath.Join(dataDir, "private", "provider-secrets", "credentials.v1.json")
	before, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(filepath.Dir(secretPath), "master-key", "master.key")); err != nil {
		t.Fatal(err)
	}
	if reopened, err := openProviderRegistryAuthorityV1(ctx, dataDir); err == nil {
		_ = reopened.Close()
		t.Fatal("missing file master key was silently replaced")
	}
	if after, err := os.ReadFile(secretPath); err != nil || !bytes.Equal(before, after) {
		t.Fatal("missing master key changed committed ciphertext")
	}
}

func TestLegacyKeychainProfileReentersWithoutOldMasterKey(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDir, 'q')
	old, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	prior := connectProviderRegistryTestWinnerV1(t, ctx, old.Manager(), "provider-legacy-reentry", "synthetic-old-key")
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	oldSecretPath := filepath.Join(dataDir, "private", "provider-secrets", "credentials.v1.json")
	before, err := os.ReadFile(oldSecretPath)
	if err != nil {
		t.Fatal(err)
	}
	masterDirectory := filepath.Join(filepath.Dir(oldSecretPath), "master-key")
	if err := os.WriteFile(filepath.Join(masterDirectory, "authority.v1"), []byte("analytix-master-key-authority:v1:keychain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(masterDirectory, "master.key")); err != nil {
		t.Fatal(err)
	}

	reentry, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("legacy profile did not reach limited re-entry: %v", err)
	}
	snapshot, err := reentry.Manager().Snapshot(ctx)
	if err != nil || snapshot.Providers[prior.ID].CredentialRef != prior.CredentialRef {
		t.Fatalf("legacy Registry reference was lost: %v", err)
	}
	if after, err := os.ReadFile(oldSecretPath); err != nil || !bytes.Equal(before, after) {
		t.Fatal("opening legacy profile changed its ciphertext before re-entry")
	}
	if err := reentry.Manager().CheckCredential(ctx, providerregistryapp.ProviderOperationCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, prior.ID), ProviderID: prior.ID,
	}); !errors.Is(err, registryport.ErrCredentialReentryRequired) {
		t.Fatalf("legacy credential check did not return visible re-entry status: %v", err)
	}
	if _, err := newProviderRegistryExecutionResolverV1(reentry.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{}); err == nil {
		t.Fatal("legacy Keychain credential was treated as executable")
	}
	mutation, err := secretstoreport.SetCredential([]byte("synthetic-new-key"))
	if err != nil {
		t.Fatal(err)
	}
	winner, err := reentry.Manager().ReplaceCredential(ctx, providerregistryapp.CredentialReplaceCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, prior.ID), ProviderID: prior.ID,
		CredentialPurpose: "provider-api-key", Credential: mutation,
	})
	if err != nil || winner.CredentialRef == prior.CredentialRef {
		t.Fatalf("fenced legacy credential replacement failed: %v", err)
	}
	if err := reentry.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("file-authority restart failed: %v", err)
	}
	defer restarted.Close()
	resolved, err := newProviderRegistryExecutionResolverV1(restarted.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || resolved.Config.APIKey != "synthetic-new-key" {
		t.Fatalf("replacement did not survive restart: %v", err)
	}
	retired, err := os.ReadFile(oldSecretPath)
	if err != nil || bytes.Contains(retired, []byte(prior.CredentialRef)) {
		t.Fatal("verified replacement did not retire legacy ciphertext reference")
	}
}

func TestLegacyKeychainReentryRecoversCommittedReplacementAfterInterruption(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDir, 'r')
	old, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	prior := connectProviderRegistryTestWinnerV1(t, ctx, old.Manager(), "provider-legacy-interrupted", "synthetic-prior-key")
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	oldSecretPath := filepath.Join(dataDir, "private", "provider-secrets", "credentials.v1.json")
	masterDirectory := filepath.Join(filepath.Dir(oldSecretPath), "master-key")
	if err := os.WriteFile(filepath.Join(masterDirectory, "authority.v1"), []byte("analytix-master-key-authority:v1:keychain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(masterDirectory, "master.key")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(oldSecretPath)
	if err != nil {
		t.Fatal(err)
	}
	reentry, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := secretstore.InspectLegacyKeychainProfile(oldSecretPath)
	if err != nil || profile == nil {
		t.Fatalf("legacy inventory unavailable: %v", err)
	}
	faulted, err := providerregistryapp.NewManagerWithFaultRecorder(
		reentry.registry, profile.ReentryStore(reentry.secrets),
		providerregistryapp.FaultRecorderFunc(func(point providerregistryapp.FaultPoint) error {
			if point == providerregistryapp.FaultAfterMetadataCommitted {
				return errors.New("synthetic interruption")
			}
			return nil
		}), providerregistryfs.LegacySourceReader{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := faulted.PermitLegacyReentryRefs(profile.ActivePurposes()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := faulted.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := secretstoreport.SetCredential([]byte("synthetic-replacement-key"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = faulted.ReplaceCredential(ctx, providerregistryapp.CredentialReplaceCommand{
		Expected: expectedProviderRegistryStateV1(snapshot, prior.ID), ProviderID: prior.ID,
		CredentialPurpose: "provider-api-key", Credential: mutation,
	})
	if !errors.Is(err, providerregistryapp.ErrInterrupted) {
		t.Fatalf("replacement did not interrupt at committed metadata: %v", err)
	}
	if after, err := os.ReadFile(oldSecretPath); err != nil || !bytes.Equal(before, after) {
		t.Fatal("interruption changed old ciphertext before recovery verified the winner")
	}
	if err := reentry.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("committed replacement did not recover: %v", err)
	}
	defer restarted.Close()
	recovered, err := restarted.Manager().Snapshot(ctx)
	if err != nil || recovered.Providers[prior.ID].CredentialRef == prior.CredentialRef {
		t.Fatalf("replacement winner was not recovered: %v", err)
	}
	resolved, err := newProviderRegistryExecutionResolverV1(restarted.Manager()).ResolveTurnExecution(ctx, provider.TurnExecutionInput{})
	if err != nil || resolved.Config.APIKey != "synthetic-replacement-key" {
		t.Fatalf("new key was not available after recovery: %v", err)
	}
	retired, err := os.ReadFile(oldSecretPath)
	if err != nil || bytes.Contains(retired, []byte(prior.CredentialRef)) {
		t.Fatal("recovered replacement did not retire the old ciphertext reference")
	}
}

func TestLegacyKeychainReentryKeepsOtherProvidersAcrossPartialRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	writeProviderRegistrySyntheticFallbackMasterKeyV1(t, dataDir, 's')
	old, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	first := connectProviderRegistryTestWinnerV1(t, ctx, old.Manager(), "provider-legacy-first", "synthetic-old-first")
	second := connectProviderRegistryTestWinnerV1(t, ctx, old.Manager(), "provider-legacy-second", "synthetic-old-second")
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(dataDir, "private", "provider-secrets", "credentials.v1.json")
	masterDir := filepath.Join(filepath.Dir(oldPath), "master-key")
	if err := os.WriteFile(filepath.Join(masterDir, "authority.v1"), []byte("analytix-master-key-authority:v1:keychain\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(masterDir, "master.key")); err != nil {
		t.Fatal(err)
	}
	for index, providerID := range []string{first.ID, second.ID} {
		authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
		if err != nil {
			t.Fatalf("partial legacy restart %d failed: %v", index, err)
		}
		snapshot, err := authority.Manager().Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := authority.Manager().CheckCredential(ctx, providerregistryapp.ProviderOperationCommand{
			Expected: expectedProviderRegistryStateV1(snapshot, providerID), ProviderID: providerID,
		}); !errors.Is(err, registryport.ErrCredentialReentryRequired) {
			t.Fatalf("unreentered Provider %s lost explicit status: %v", providerID, err)
		}
		mutation, err := secretstoreport.SetCredential([]byte("synthetic-new-" + providerID))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := authority.Manager().ReplaceCredential(ctx, providerregistryapp.CredentialReplaceCommand{
			Expected: expectedProviderRegistryStateV1(snapshot, providerID), ProviderID: providerID,
			CredentialPurpose: "provider-api-key", Credential: mutation,
		}); err != nil {
			t.Fatalf("partial legacy replacement %d failed: %v", index, err)
		}
		if err := authority.Close(); err != nil {
			t.Fatal(err)
		}
		oldBytes, err := os.ReadFile(oldPath)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 && (!bytes.Contains(oldBytes, []byte(second.CredentialRef)) || bytes.Contains(oldBytes, []byte(first.CredentialRef))) {
			t.Fatal("first replacement changed the other Provider's legacy ciphertext")
		}
	}
	restarted, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	snapshot, err := restarted.Manager().Snapshot(ctx)
	if err != nil || len(snapshot.Providers) != 2 {
		t.Fatalf("partial migration lost a Provider: %v", err)
	}
	for _, providerID := range []string{first.ID, second.ID} {
		if err := restarted.Manager().CheckCredential(ctx, providerregistryapp.ProviderOperationCommand{
			Expected: expectedProviderRegistryStateV1(snapshot, providerID), ProviderID: providerID,
		}); err != nil {
			t.Fatalf("reentered Provider %s unavailable after restart: %v", providerID, err)
		}
	}
}
