package runtimeapp

import (
	"context"
	"testing"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

func seedProviderRegistryExecutionAuthorityV1(
	t *testing.T,
	dataDir string,
	providerID string,
	endpoint string,
	models []string,
	selectedModel string,
	secret string,
) {
	t.Helper()
	installSyntheticProviderRegistryFallbackMasterKey(t, dataDir)
	ctx := context.Background()
	authority, err := openProviderRegistryAuthorityV1(ctx, dataDir)
	if err != nil {
		t.Fatalf("open Provider Registry execution fixture: %v", err)
	}
	snapshot, err := authority.Manager().Snapshot(ctx)
	if err != nil {
		_ = authority.Close()
		t.Fatalf("snapshot Provider Registry execution fixture: %v", err)
	}
	credential, err := secretstoreport.SetCredential([]byte(secret))
	if err != nil {
		_ = authority.Close()
		t.Fatalf("prepare Provider Registry execution fixture: %v", err)
	}
	_, err = authority.Manager().Connect(ctx, providerregistryapp.ConnectCommand{
		Expected: domainregistry.ExpectedState{
			RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation,
		},
		Provider: domainregistry.ProviderInput{
			ID: providerID, Kind: "openai-compatible", Endpoint: endpoint,
			Models: append([]string(nil), models...), MediaModels: []string{}, SelectedModel: selectedModel,
			SelectedRoutes: []string{"primary"},
		},
		CredentialPurpose: secretstoreport.Purpose("provider-api-key"), Credential: credential,
	})
	if err != nil {
		_ = authority.Close()
		t.Fatalf("connect Provider Registry execution fixture: %v", err)
	}
	if err := authority.Close(); err != nil {
		t.Fatalf("close Provider Registry execution fixture: %v", err)
	}
}
