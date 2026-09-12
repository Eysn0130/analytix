package runtimeapp

import (
	"context"
	"errors"
	"path/filepath"
	"sync"

	providerregistryfs "analytix.local/runtime-go/internal/adapters/outbound/providerregistryfs"
	secretstore "analytix.local/runtime-go/internal/adapters/outbound/secretstore"
	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const providerRegistrySecretStoreFileV1 = "credentials.v1.json"

type providerRegistrySecretAuthorizerV1 struct{}

func (providerRegistrySecretAuthorizerV1) AuthorizeCredentialAccess(
	ctx context.Context,
	request secretstoreport.AccessRequest,
) error {
	if ctx == nil || ctx.Err() != nil {
		return secretstoreport.ErrUnauthorized
	}
	purpose, err := secretstoreport.NormalizePurpose(string(request.Purpose))
	if err != nil || purpose != request.Purpose {
		return secretstoreport.ErrUnauthorized
	}
	switch request.Consumer {
	case providerregistryapp.ProtectedRecoveryPendingKeyConsumer:
		if purpose != providerregistryapp.ProtectedRecoveryPendingKeyPurpose {
			return secretstoreport.ErrUnauthorized
		}
	case providerregistryapp.ProtectedTransferSourceConsumer, providerregistryapp.ProtectedRecoveryReadbackConsumer:
		switch purpose {
		case "provider-api-key", "provider-oauth-token-bundle", "mcp-oauth-access-token", "extension-provider-account-token":
		default:
			return secretstoreport.ErrUnauthorized
		}
	case providerregistryapp.RegistryReadbackConsumer, providerregistryapp.ProviderExecutionConsumer, providerregistryapp.AccountExecutionConsumer:
		if purpose == providerregistryapp.ProtectedRecoveryPendingKeyPurpose {
			return secretstoreport.ErrUnauthorized
		}
	default:
		return secretstoreport.ErrUnauthorized
	}
	return nil
}

type providerRegistryAuthorityV1 struct {
	manager  *providerregistryapp.Manager
	service  *providerRegistryOperationsV1
	registry *providerregistryfs.Store
	secrets  *secretstore.Store

	mu     sync.Mutex
	closed bool
}

func openProviderRegistryAuthorityV1(
	ctx context.Context,
	dataDir string,
	options ...secretstore.Options,
) (*providerRegistryAuthorityV1, error) {
	if ctx == nil {
		return nil, errors.New("provider registry authority is unavailable")
	}
	if len(options) > 1 {
		return nil, errors.New("provider registry authority is unavailable")
	}
	var secretOptions secretstore.Options
	if len(options) == 1 {
		secretOptions = options[0]
	}
	secretPath := filepath.Join(dataDir, "private", "provider-secrets", providerRegistrySecretStoreFileV1)
	if len(options) == 1 {
		secrets, err := secretstore.NewWithOptions(secretPath, providerRegistrySecretAuthorizerV1{}, secretOptions)
		if err != nil {
			return nil, errors.New("provider registry authority is unavailable")
		}
		registry, err := providerregistryfs.New(dataDir)
		if err != nil {
			return nil, errors.Join(errors.New("provider registry authority is unavailable"), secrets.Close())
		}
		return finishProviderRegistryAuthorityV1(ctx, registry, secrets)
	}
	registry, err := providerregistryfs.New(dataDir)
	if err != nil {
		return nil, errors.New("provider registry authority is unavailable")
	}
	secrets, err := secretstore.New(secretPath, providerRegistrySecretAuthorizerV1{})
	if err != nil {
		return nil, errors.Join(errors.New("provider registry authority is unavailable"), registry.Close())
	}
	return finishProviderRegistryAuthorityV1(ctx, registry, secrets)
}

func finishProviderRegistryAuthorityV1(
	ctx context.Context,
	registry *providerregistryfs.Store,
	secrets *secretstore.Store,
) (*providerRegistryAuthorityV1, error) {
	manager, err := providerregistryapp.NewManager(registry, secrets, providerregistryfs.LegacySourceReader{})
	if err != nil {
		return nil, errors.Join(errors.New("provider registry authority is unavailable"), registry.Close(), secrets.Close())
	}
	authority := &providerRegistryAuthorityV1{
		manager:  manager,
		service:  newProviderRegistryOperationsV1(manager),
		registry: registry,
		secrets:  secrets,
	}
	if err := manager.Recover(ctx); err != nil {
		return nil, errors.Join(errors.New("provider registry recovery failed"), authority.Close())
	}
	return authority, nil
}

func (authority *providerRegistryAuthorityV1) Service() *providerRegistryOperationsV1 {
	if authority == nil {
		return nil
	}
	return authority.service
}

func (authority *providerRegistryAuthorityV1) Manager() *providerregistryapp.Manager {
	if authority == nil {
		return nil
	}
	return authority.manager
}

func (authority *providerRegistryAuthorityV1) Close() error {
	if authority == nil {
		return nil
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.closed {
		return nil
	}
	var registryErr, secretErr error
	if authority.registry != nil {
		registryErr = authority.registry.Close()
	}
	if authority.secrets != nil {
		secretErr = authority.secrets.Close()
	}
	err := errors.Join(registryErr, secretErr)
	if err == nil {
		authority.closed = true
	}
	return err
}
