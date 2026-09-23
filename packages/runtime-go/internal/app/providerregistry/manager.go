package providerregistry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

type mutationPlan struct {
	deferSelection            bool
	operation                 domainregistry.Operation
	expected                  domainregistry.ExpectedState
	provider                  domainregistry.ProviderInput
	providerID                string
	purpose                   secretstoreport.Purpose
	credential                secretstoreport.CredentialMutation
	recoveredCredential       []byte
	legacyMigrationID         string
	privateAccountDisposition string
}

func (manager *Manager) Connect(ctx context.Context, command ConnectCommand) (domainregistry.Provider, error) {
	if command.Provider.Validate() != nil || command.Credential.Validate() != nil ||
		command.Credential.Kind() != secretstoreport.MutationSet {
		return domainregistry.Provider{}, registryport.ErrInvalidRequest
	}
	purpose, err := normalizePurpose(command.CredentialPurpose, true)
	if err != nil {
		return domainregistry.Provider{}, err
	}
	return manager.execute(ctx, mutationPlan{
		operation: domainregistry.OperationConnect, expected: command.Expected,
		deferSelection: command.DeferSelection,
		provider:       command.Provider, providerID: command.Provider.ID,
		purpose: purpose, credential: command.Credential,
	})
}

func (manager *Manager) Update(ctx context.Context, command UpdateCommand) (domainregistry.Provider, error) {
	if command.Provider.Validate() != nil || command.Credential.Validate() != nil ||
		(command.Credential.Kind() != secretstoreport.MutationKeep && command.Credential.Kind() != secretstoreport.MutationSet &&
			command.Credential.Kind() != secretstoreport.MutationExplicitDelete) ||
		(command.Credential.Kind() == secretstoreport.MutationKeep && command.CredentialPurpose != "") {
		return domainregistry.Provider{}, registryport.ErrInvalidRequest
	}
	requiredPurpose := command.Credential.Kind() == secretstoreport.MutationSet ||
		command.Credential.Kind() == secretstoreport.MutationExplicitDelete
	purpose, err := normalizePurpose(command.CredentialPurpose, requiredPurpose)
	if err != nil {
		return domainregistry.Provider{}, err
	}
	return manager.execute(ctx, mutationPlan{
		operation: domainregistry.OperationUpdate, expected: command.Expected,
		provider: command.Provider, providerID: command.Provider.ID,
		purpose: purpose, credential: command.Credential,
	})
}

func (manager *Manager) Select(ctx context.Context, command SelectCommand) (domainregistry.Provider, error) {
	return manager.execute(ctx, mutationPlan{
		operation: domainregistry.OperationSelect, expected: command.Expected,
		providerID: command.ProviderID,
		credential: secretstoreport.KeepCredential(),
	})
}

func (manager *Manager) ReplaceCredential(
	ctx context.Context,
	command CredentialReplaceCommand,
) (domainregistry.Provider, error) {
	if command.Credential.Validate() != nil || command.Credential.Kind() != secretstoreport.MutationSet {
		return domainregistry.Provider{}, registryport.ErrInvalidRequest
	}
	purpose, err := normalizePurpose(command.CredentialPurpose, true)
	if err != nil {
		return domainregistry.Provider{}, err
	}
	return manager.execute(ctx, mutationPlan{
		operation: domainregistry.OperationCredentialReplace, expected: command.Expected,
		providerID: command.ProviderID, purpose: purpose, credential: command.Credential,
	})
}

func (manager *Manager) Disconnect(ctx context.Context, command DisconnectCommand) (domainregistry.Provider, error) {
	if command.PrivateAccountDisposition != "" &&
		command.PrivateAccountDisposition != AccountCredentialDispositionRevoke &&
		command.PrivateAccountDisposition != AccountCredentialDispositionDisconnect {
		return domainregistry.Provider{}, registryport.ErrInvalidRequest
	}
	purpose, err := normalizePurpose(command.CredentialPurpose, true)
	if err != nil {
		return domainregistry.Provider{}, err
	}
	return manager.execute(ctx, mutationPlan{
		operation: domainregistry.OperationDisconnect, expected: command.Expected,
		providerID: command.ProviderID, purpose: purpose,
		privateAccountDisposition: command.PrivateAccountDisposition,
		credential:                secretstoreport.ExplicitlyDeleteCredential(),
	})
}

func (manager *Manager) ExplicitDelete(ctx context.Context, command ExplicitDeleteCommand) error {
	purpose, err := normalizePurpose(command.CredentialPurpose, false)
	if err != nil {
		return err
	}
	_, err = manager.execute(ctx, mutationPlan{
		operation: domainregistry.OperationExplicitDelete, expected: command.Expected,
		providerID: command.ProviderID, purpose: purpose,
		credential: secretstoreport.ExplicitlyDeleteCredential(),
	})
	return err
}

func (manager *Manager) Recover(ctx context.Context) error {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return registryport.ErrInvalidRequest
	}
	err := manager.registry.WithExclusive(ctx, func(transaction registryport.Transaction) error {
		if err := manager.recoverAll(ctx, transaction); err != nil {
			return err
		}
		state, err := transaction.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		return manager.verifyCommittedSnapshot(ctx, state)
	})
	return normalizeManagerError(err)
}

func (manager *Manager) verifyCommittedSnapshot(ctx context.Context, state domainregistry.Registry) error {
	ids := make([]string, 0, len(state.Providers))
	for id := range state.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		provider := state.Providers[id]
		if provider.CredentialRef == "" {
			continue
		}
		plaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
			CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
			Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
			Consumer:      RegistryReadbackConsumer,
		})
		if err != nil {
			clear(plaintext)
			return normalizeSecretError(err)
		}
		if len(plaintext) == 0 {
			clear(plaintext)
			return registryport.ErrVerification
		}
		clear(plaintext)
	}
	return nil
}

// Snapshot returns the current internal Registry state after completing any
// deterministic recovery under the same data-directory exclusive transaction
// used by mutations. The returned value is detached from the Store.
func (manager *Manager) Snapshot(ctx context.Context) (domainregistry.Registry, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return domainregistry.Registry{}, registryport.ErrInvalidRequest
	}
	var result domainregistry.Registry
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		result = state.Clone()
		return nil
	})
	if err != nil {
		return domainregistry.Registry{}, normalizeManagerError(err)
	}
	return result, nil
}

// ResolveSelectedIntent returns the current executable route without returning
// its protected credential. It checks route availability under the same lock
// and immediately clears the bounded read so route-pool fallback remains
// deterministic; the physical Provider effect must separately re-resolve the
// credential through ResolveSelectedForExecution.
func (manager *Manager) ResolveSelectedIntent(ctx context.Context) (ExecutionIntentResolution, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return ExecutionIntentResolution{}, registryport.ErrInvalidRequest
	}
	var result ExecutionIntentResolution
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		providers, err := executionRouteProviders(state)
		if err != nil {
			return err
		}
		for _, provider := range providers {
			plaintext, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
				Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
				Consumer:      ProviderExecutionConsumer,
			})
			usable := readErr == nil && len(plaintext) > 0
			clear(plaintext)
			if readErr != nil {
				if errors.Is(readErr, secretstoreport.ErrNotFound) || errors.Is(readErr, secretstoreport.ErrTombstoned) {
					continue
				}
				return normalizeSecretError(readErr)
			}
			if !usable {
				continue
			}
			result = ExecutionIntentResolution{
				RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
				Provider: provider.Clone(),
			}
			return nil
		}
		return registryport.ErrVerification
	})
	if err != nil {
		return ExecutionIntentResolution{}, normalizeManagerError(err)
	}
	return result, nil
}

// ResolveSelectedForExecution recovers and reads the current selected winner
// under the Registry's data-directory lock. It never caches a Provider or
// credential across calls, so every bounded execution observes the latest
// committed revision or fails closed.
func (manager *Manager) ResolveSelectedForExecution(ctx context.Context) (ExecutionResolution, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return ExecutionResolution{}, registryport.ErrInvalidRequest
	}
	var result ExecutionResolution
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		providers, err := executionRouteProviders(state)
		if err != nil {
			return err
		}
		for _, provider := range providers {
			plaintext, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
				Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
				Consumer:      ProviderExecutionConsumer,
			})
			if readErr != nil {
				clear(plaintext)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if errors.Is(readErr, secretstoreport.ErrNotFound) || errors.Is(readErr, secretstoreport.ErrTombstoned) {
					continue
				}
				return normalizeSecretError(readErr)
			}
			if len(plaintext) == 0 {
				clear(plaintext)
				continue
			}
			result = ExecutionResolution{
				RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
				Provider: provider.Clone(), Credential: plaintext,
			}
			return nil
		}
		return registryport.ErrVerification
	})
	if err != nil {
		result.Clear()
		return ExecutionResolution{}, normalizeManagerError(err)
	}
	return result, nil
}

// ResolveSelectedMediaForExecution resolves the current committed media route
// and protected credential for one bounded physical media effect. Media
// execution deliberately has its own route predicate: a provider that can run
// text does not implicitly become an image or speech provider.
func (manager *Manager) ResolveSelectedMediaForExecution(ctx context.Context) (ExecutionResolution, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return ExecutionResolution{}, registryport.ErrInvalidRequest
	}
	var result ExecutionResolution
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		providers, err := mediaExecutionRouteProviders(state)
		if err != nil {
			return err
		}
		for _, provider := range providers {
			plaintext, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
				Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
				Consumer:      ProviderExecutionConsumer,
			})
			if readErr != nil {
				clear(plaintext)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if errors.Is(readErr, secretstoreport.ErrNotFound) || errors.Is(readErr, secretstoreport.ErrTombstoned) {
					continue
				}
				return normalizeSecretError(readErr)
			}
			if len(plaintext) == 0 {
				clear(plaintext)
				continue
			}
			result = ExecutionResolution{
				RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
				Provider: provider.Clone(), Credential: plaintext,
			}
			return nil
		}
		return registryport.ErrVerification
	})
	if err != nil {
		result.Clear()
		return ExecutionResolution{}, normalizeManagerError(err)
	}
	return result, nil
}

// ValidateExecutionCurrent rechecks the complete Registry/Provider/credential
// fence after a physical Provider response and before any result is consumed.
func (manager *Manager) ValidateExecutionCurrent(ctx context.Context, authority ExecutionAuthority) error {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		authority.RegistryRevision == 0 || authority.RegistryIncarnation == "" || authority.ProviderID == "" ||
		authority.ProviderRevision == 0 || authority.ProviderGeneration == 0 || authority.ProviderIncarnation == "" ||
		authority.ProviderCredentialRef == "" || authority.ProviderCredentialPurpose == "" {
		return registryport.ErrInvalidRequest
	}
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		providers, err := executionRouteProviders(state)
		if err != nil {
			return err
		}
		var provider domainregistry.Provider
		for _, candidate := range providers {
			plaintext, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef(candidate.CredentialRef),
				Purpose:       secretstoreport.Purpose(candidate.CredentialPurpose),
				Consumer:      ProviderExecutionConsumer,
			})
			usable := readErr == nil && len(plaintext) > 0
			clear(plaintext)
			if readErr != nil {
				if errors.Is(readErr, secretstoreport.ErrNotFound) || errors.Is(readErr, secretstoreport.ErrTombstoned) {
					continue
				}
				return normalizeSecretError(readErr)
			}
			if usable {
				provider = candidate
				break
			}
		}
		if provider.ID == "" {
			return registryport.ErrVerification
		}
		if state.Revision != authority.RegistryRevision || state.Incarnation != authority.RegistryIncarnation ||
			provider.ID != authority.ProviderID || provider.Revision != authority.ProviderRevision ||
			provider.Generation != authority.ProviderGeneration || provider.Incarnation != authority.ProviderIncarnation ||
			provider.CredentialRef != authority.ProviderCredentialRef || provider.CredentialPurpose != authority.ProviderCredentialPurpose {
			return registryport.ErrConflict
		}
		return nil
	})
	return normalizeManagerError(err)
}

// ValidateMediaExecutionCurrent rechecks the complete media route and
// credential fence immediately around each physical send and before its
// response can be used.
func (manager *Manager) ValidateMediaExecutionCurrent(ctx context.Context, authority ExecutionAuthority) error {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		authority.RegistryRevision == 0 || authority.RegistryIncarnation == "" || authority.ProviderID == "" ||
		authority.ProviderRevision == 0 || authority.ProviderGeneration == 0 || authority.ProviderIncarnation == "" ||
		authority.ProviderCredentialRef == "" || authority.ProviderCredentialPurpose == "" {
		return registryport.ErrInvalidRequest
	}
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		providers, err := mediaExecutionRouteProviders(state)
		if err != nil {
			return err
		}
		var provider domainregistry.Provider
		for _, candidate := range providers {
			plaintext, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
				CredentialRef: secretstoreport.CredentialRef(candidate.CredentialRef),
				Purpose:       secretstoreport.Purpose(candidate.CredentialPurpose),
				Consumer:      ProviderExecutionConsumer,
			})
			usable := readErr == nil && len(plaintext) > 0
			clear(plaintext)
			if readErr != nil {
				if errors.Is(readErr, secretstoreport.ErrNotFound) || errors.Is(readErr, secretstoreport.ErrTombstoned) {
					continue
				}
				return normalizeSecretError(readErr)
			}
			if usable {
				provider = candidate
				break
			}
		}
		if provider.ID == "" {
			return registryport.ErrVerification
		}
		if state.Revision != authority.RegistryRevision || state.Incarnation != authority.RegistryIncarnation ||
			provider.ID != authority.ProviderID || provider.Revision != authority.ProviderRevision ||
			provider.Generation != authority.ProviderGeneration || provider.Incarnation != authority.ProviderIncarnation ||
			provider.CredentialRef != authority.ProviderCredentialRef || provider.CredentialPurpose != authority.ProviderCredentialPurpose {
			return registryport.ErrConflict
		}
		return nil
	})
	return normalizeManagerError(err)
}

func executionRouteProviders(state domainregistry.Registry) ([]domainregistry.Provider, error) {
	selected, exists := state.Providers[state.SelectedProviderID]
	if state.SelectedProviderID == "" || !exists || selected.Validate() != nil || selected.Tombstone {
		return nil, registryport.ErrVerification
	}
	routeProviderIDs, err := selected.RouteProviderIDs()
	if err != nil {
		return nil, registryport.ErrVerification
	}
	providers := make([]domainregistry.Provider, 0, len(routeProviderIDs))
	for _, providerID := range routeProviderIDs {
		provider, available := state.Providers[providerID]
		if !available || provider.Validate() != nil || provider.Tombstone ||
			provider.CredentialRef == "" || provider.CredentialPurpose == "" || provider.SelectedModel == "" {
			continue
		}
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return nil, registryport.ErrVerification
	}
	return providers, nil
}

func mediaExecutionRouteProviders(state domainregistry.Registry) ([]domainregistry.Provider, error) {
	selected, exists := state.Providers[state.SelectedProviderID]
	if state.SelectedProviderID == "" || !exists || selected.Validate() != nil || selected.Tombstone {
		return nil, registryport.ErrVerification
	}
	policy := selected.Clone()
	if policy.SelectedModel == "" {
		policy.SelectedModel = policy.SelectedMedia
	}
	routeProviderIDs, err := policy.RouteProviderIDs()
	if err != nil {
		return nil, registryport.ErrVerification
	}
	providers := make([]domainregistry.Provider, 0, len(routeProviderIDs))
	for _, providerID := range routeProviderIDs {
		provider, available := state.Providers[providerID]
		if !available || provider.Validate() != nil || provider.Tombstone ||
			provider.CredentialRef == "" || provider.CredentialPurpose == "" ||
			provider.SelectedMedia == "" || !slices.Contains(provider.MediaModels, provider.SelectedMedia) {
			continue
		}
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return nil, registryport.ErrVerification
	}
	return providers, nil
}

// ResolveProviderForOperation resolves the exact fenced Provider requested by
// Provider Settings and reads its K1 credential for one bounded network call.
// Callers must Clear the result immediately after constructing that call.
func (manager *Manager) ResolveProviderForOperation(
	ctx context.Context,
	command ProviderOperationCommand,
) (ExecutionResolution, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return ExecutionResolution{}, registryport.ErrInvalidRequest
	}
	var result ExecutionResolution
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		provider, _, err := checkExpected(state, mutationPlan{
			operation:  domainregistry.OperationSelect,
			expected:   command.Expected,
			providerID: command.ProviderID,
		})
		if err != nil {
			return err
		}
		if provider.Tombstone || provider.CredentialRef == "" || provider.CredentialPurpose == "" {
			return registryport.ErrVerification
		}
		plaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
			CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
			Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
			Consumer:      ProviderExecutionConsumer,
		})
		if err != nil {
			clear(plaintext)
			return normalizeSecretError(err)
		}
		if len(plaintext) == 0 {
			clear(plaintext)
			return registryport.ErrVerification
		}
		plaintext, err = providerExecutionCredential(provider, plaintext)
		if err != nil {
			clear(plaintext)
			return err
		}
		result = ExecutionResolution{
			RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
			Provider: provider.Clone(), Credential: plaintext,
		}
		return nil
	})
	if err != nil {
		result.Clear()
		return ExecutionResolution{}, normalizeManagerError(err)
	}
	return result, nil
}

// CheckCredential resolves through the execution owner without a network call or
// returning the secret. The caller's complete fence is rechecked after the read.
func (manager *Manager) CheckCredential(ctx context.Context, command ProviderOperationCommand) error {
	resolution, err := manager.ResolveProviderForOperation(ctx, command)
	resolution.Clear()
	if err != nil {
		return err
	}
	return manager.ValidateProviderOperationCurrent(ctx, command)
}

func providerExecutionCredential(provider domainregistry.Provider, plaintext []byte) ([]byte, error) {
	if provider.CredentialPurpose != "provider-oauth-token-bundle" {
		return plaintext, nil
	}
	var bundle struct {
		Kind              string `json:"kind"`
		AccessToken       string `json:"accessToken"`
		RefreshToken      string `json:"refreshToken,omitempty"`
		IDToken           string `json:"idToken,omitempty"`
		SubscriptionToken string `json:"subscriptionToken,omitempty"`
		TokenType         string `json:"tokenType"`
		ExpiresAtMS       int64  `json:"expiresAtMs,omitempty"`
		OAuthBinding      struct {
			Owner                 string   `json:"owner"`
			Issuer                string   `json:"issuer"`
			AuthorizationEndpoint string   `json:"authorizationEndpoint"`
			TokenEndpoint         string   `json:"tokenEndpoint"`
			RevocationEndpoint    string   `json:"revocationEndpoint,omitempty"`
			ClientID              string   `json:"clientId"`
			Provider              string   `json:"provider"`
			AccountID             string   `json:"accountId"`
			ChannelID             string   `json:"channelId,omitempty"`
			RedirectURI           string   `json:"redirectUri"`
			Scopes                []string `json:"scopes"`
			BindingKey            string   `json:"bindingKey"`
			OwnerFingerprint      string   `json:"ownerFingerprint"`
			OwnerRevision         string   `json:"ownerRevision"`
			OwnerGeneration       string   `json:"ownerGeneration"`
			OwnerIncarnation      string   `json:"ownerIncarnation"`
			ProfileBinding        string   `json:"profileBinding"`
		} `json:"oauthBinding"`
	}
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&bundle) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) ||
		bundle.Kind != "provider-oauth-bundle" || bundle.TokenType != "Bearer" ||
		strings.TrimSpace(bundle.AccessToken) == "" || len(bundle.AccessToken) > 32*1024 ||
		bundle.OAuthBinding.Owner != "provider" || bundle.OAuthBinding.Provider != provider.ID ||
		bundle.OAuthBinding.AccountID != provider.ID || bundle.OAuthBinding.ChannelID != "" ||
		len(bundle.OAuthBinding.BindingKey) != len("oauthb_")+43 ||
		bundle.OAuthBinding.RedirectURI != "com.analytix.desktop:/oauth/callback/"+bundle.OAuthBinding.BindingKey ||
		len(bundle.OAuthBinding.OwnerFingerprint) != 64 || len(bundle.OAuthBinding.ProfileBinding) != 64 ||
		bundle.OAuthBinding.OwnerRevision != strconv.FormatUint(provider.Revision, 10) ||
		bundle.OAuthBinding.OwnerGeneration != strconv.FormatUint(provider.Generation, 10) ||
		bundle.OAuthBinding.OwnerIncarnation != provider.Incarnation ||
		(bundle.ExpiresAtMS > 0 && time.Now().UnixMilli() >= bundle.ExpiresAtMS) {
		clear(plaintext)
		return nil, registryport.ErrVerification
	}
	keyFreeBinding := domainregistry.OAuthBindingMetadata{
		SchemaVersion:         1,
		Issuer:                bundle.OAuthBinding.Issuer,
		AuthorizationEndpoint: bundle.OAuthBinding.AuthorizationEndpoint,
		TokenEndpoint:         bundle.OAuthBinding.TokenEndpoint,
		RevocationEndpoint:    bundle.OAuthBinding.RevocationEndpoint,
		ClientID:              bundle.OAuthBinding.ClientID,
		Scopes:                slices.Clone(bundle.OAuthBinding.Scopes),
		RedirectModeVersion:   1,
	}
	if provider.OAuthBinding == nil || !reflect.DeepEqual(provider.OAuthBinding, &keyFreeBinding) {
		clear(plaintext)
		return nil, registryport.ErrVerification
	}
	accessToken := []byte(bundle.AccessToken)
	clear(plaintext)
	return accessToken, nil
}

// ValidateProviderOperationCurrent rechecks the complete Registry/Provider
// fence after egress. It intentionally performs no credential read and returns
// conflict when any revision, generation, incarnation, or credential purpose
// changed while the network call was in flight.
func (manager *Manager) ValidateProviderOperationCurrent(
	ctx context.Context,
	command ProviderOperationCommand,
) error {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return registryport.ErrInvalidRequest
	}
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		provider, _, err := checkExpected(state, mutationPlan{
			operation:  domainregistry.OperationSelect,
			expected:   command.Expected,
			providerID: command.ProviderID,
		})
		if err != nil {
			return err
		}
		if provider.Tombstone || provider.CredentialRef == "" || provider.CredentialPurpose == "" {
			return registryport.ErrVerification
		}
		return nil
	})
	return normalizeManagerError(err)
}

func (manager *Manager) execute(ctx context.Context, plan mutationPlan) (domainregistry.Provider, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return domainregistry.Provider{}, registryport.ErrInvalidRequest
	}
	var result domainregistry.Provider
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		prior, exists, err := checkExpected(state, plan)
		if err != nil {
			return err
		}
		if plan.operation == domainregistry.OperationSelect &&
			(!exists || prior.Tombstone || prior.CredentialRef == "" || prior.CredentialPurpose == "") {
			// Ordinary selection is an executable authority transition.  Imported
			// portable entries remain uncredentialed until a normal K2 commit.
			return registryport.ErrInvalidRequest
		}
		if plan.operation == domainregistry.OperationUpdate && plan.provider.Kind != prior.Kind {
			return registryport.ErrInvalidRequest
		}
		if plan.operation == domainregistry.OperationUpdate {
			bindingChanged := !reflect.DeepEqual(prior.OAuthBinding, plan.provider.OAuthBinding)
			if bindingChanged {
				bindingBoundCredential := prior.CredentialRef != "" &&
					prior.CredentialPurpose == "provider-oauth-token-bundle"
				if bindingBoundCredential && (plan.credential.Kind() != secretstoreport.MutationExplicitDelete ||
					string(plan.purpose) != prior.CredentialPurpose) {
					return registryport.ErrInvalidRequest
				}
				if !bindingBoundCredential && plan.credential.Kind() != secretstoreport.MutationKeep {
					return registryport.ErrInvalidRequest
				}
			}
			if !bindingChanged && plan.credential.Kind() == secretstoreport.MutationExplicitDelete {
				return registryport.ErrInvalidRequest
			}
		}
		selfCredentialConfigured := plan.operation == domainregistry.OperationConnect || prior.CredentialRef != "" ||
			plan.credential.Kind() == secretstoreport.MutationSet || len(plan.recoveredCredential) != 0
		if (plan.operation == domainregistry.OperationConnect || plan.operation == domainregistry.OperationUpdate) &&
			validateRoutePoolMutation(state, plan.provider, selfCredentialConfigured) != nil {
			return registryport.ErrInvalidRequest
		}

		var candidate secretstoreport.PreparedCandidate
		var expectedSecret []byte
		if plan.credential.Kind() == secretstoreport.MutationSet || len(plan.recoveredCredential) != 0 {
			if len(plan.recoveredCredential) != 0 {
				expectedSecret = bytes.Clone(plan.recoveredCredential)
			} else {
				expectedSecret = plan.credential.Secret()
			}
			defer clear(expectedSecret)
			candidate, err = manager.secrets.PreparePut(ctx, plan.purpose, expectedSecret)
			if err != nil {
				return normalizeSecretError(err)
			}
			defer candidate.Abort()
			if secretstoreport.ValidateCredentialRef(candidate.CredentialRef()) != nil {
				return registryport.ErrVerification
			}
			if !registryCredentialRefAvailable(state, string(candidate.CredentialRef()), "") {
				return registryport.ErrConflict
			}
		}

		pending, next, err := buildTransaction(state, plan, prior, exists, candidate)
		if err != nil {
			return err
		}
		state.Transactions[pending.ID] = pending
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterTransactionPrepared); err != nil {
			return err
		}

		if candidate != nil {
			if err := candidate.Commit(ctx); err != nil {
				return normalizeSecretError(err)
			}
			if err := manager.observe(FaultAfterCandidateDurable); err != nil {
				return err
			}
			state, pending, err = loadPending(ctx, storage, pending.ID)
			if err != nil {
				return err
			}
			pending = withPhase(pending, domainregistry.PhaseCandidateDurable)
			state.Transactions[pending.ID] = pending
			if err := storage.Commit(ctx, state); err != nil {
				return normalizeRegistryStoreError(err)
			}
			if err := manager.observe(FaultAfterCandidateDurableRecorded); err != nil {
				return err
			}
		}

		state, pending, err = loadPending(ctx, storage, pending.ID)
		if err != nil {
			return err
		}
		if !fenceMatches(state, pending) {
			return registryport.ErrConflict
		}
		state, pending, err = commitWinner(state, pending)
		if err != nil {
			return err
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterMetadataCommitted); err != nil {
			return err
		}

		if err := manager.verifyWinner(ctx, storage, pending, expectedSecret); err != nil {
			rollbackErr := manager.rollbackWinner(ctx, storage, pending)
			if rollbackErr == nil {
				_ = manager.finishCleanup(ctx, storage, pending.ID)
			}
			return registryport.ErrVerification
		}
		if err := manager.observe(FaultAfterReadbackVerified); err != nil {
			return err
		}
		state, pending, err = loadPending(ctx, storage, pending.ID)
		if err != nil {
			return err
		}
		pending = withPhase(pending, domainregistry.PhaseVerified)
		state.Transactions[pending.ID] = pending
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterVerifiedRecorded); err != nil {
			return err
		}
		if err := manager.finishCleanup(ctx, storage, pending.ID); err != nil {
			return err
		}
		if next != nil {
			result = next.Clone()
		}
		return nil
	})
	return result, normalizeManagerError(err)
}

func validateRoutePoolMutation(
	state domainregistry.Registry,
	input domainregistry.ProviderInput,
	selfCredentialConfigured bool,
) error {
	routeProviderIDs, err := (domainregistry.Provider{
		ID: input.ID, SelectedModel: input.SelectedModel, SelectedRoutes: input.SelectedRoutes,
	}).RouteProviderIDs()
	if err != nil {
		return err
	}
	for _, providerID := range routeProviderIDs {
		if providerID == input.ID {
			if !selfCredentialConfigured || input.SelectedModel == "" ||
				!slices.Contains(input.Models, input.SelectedModel) {
				return registryport.ErrInvalidRequest
			}
			continue
		}
		provider, exists := state.Providers[providerID]
		if !exists || provider.Validate() != nil || provider.Tombstone ||
			provider.CredentialRef == "" || provider.CredentialPurpose == "" ||
			provider.SelectedModel == "" || !slices.Contains(provider.Models, provider.SelectedModel) {
			return registryport.ErrInvalidRequest
		}
	}
	return nil
}

func checkExpected(
	state domainregistry.Registry,
	plan mutationPlan,
) (domainregistry.Provider, bool, error) {
	if !domainregistry.ValidProviderID(plan.providerID) {
		return domainregistry.Provider{}, false, registryport.ErrInvalidRequest
	}
	if !domainregistry.ValidIncarnation(plan.expected.RegistryIncarnation) ||
		state.Revision != plan.expected.RegistryRevision || state.Incarnation != plan.expected.RegistryIncarnation {
		return domainregistry.Provider{}, false, registryport.ErrConflict
	}
	for _, recovery := range state.LegacyMigrationRecoveries {
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared &&
			(plan.legacyMigrationID != recovery.ID || plan.providerID != recovery.ProviderID) {
			return domainregistry.Provider{}, false, registryport.ErrConflict
		}
		if recovery.ProviderID == plan.providerID &&
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
			return domainregistry.Provider{}, false, registryport.ErrConflict
		}
	}
	prior, exists := state.Providers[plan.providerID]
	if plan.operation == domainregistry.OperationConnect {
		if exists || plan.expected.ProviderRevision != 0 || plan.expected.ProviderGeneration != 0 ||
			plan.expected.ProviderIncarnation != "" || plan.expected.ProviderCredentialPurpose != "" {
			return domainregistry.Provider{}, false, registryport.ErrConflict
		}
		if plan.provider.PrivateAccount != nil {
			privateCount := 0
			authorizationStateCount := 0
			isAuthorizationState := func(purpose string) bool {
				return purpose == "provider-oauth-authorization-state" || purpose == "mcp-oauth-authorization-state" ||
					purpose == "extension-oauth-authorization-state"
			}
			for _, provider := range state.Providers {
				if provider.PrivateAccount == nil {
					continue
				}
				privateCount++
				if isAuthorizationState(provider.PrivateAccount.Purpose) && !provider.Tombstone {
					authorizationStateCount++
				}
			}
			if len(state.Providers) >= domainregistry.MaxProviders-domainregistry.ReservedPublicProviderCapacity ||
				privateCount >= domainregistry.MaxPrivateAccounts ||
				(isAuthorizationState(plan.provider.PrivateAccount.Purpose) &&
					authorizationStateCount >= domainregistry.MaxOAuthAuthorizationStates) {
				return domainregistry.Provider{}, false, registryport.ErrConflict
			}
		}
		return domainregistry.Provider{}, false, nil
	}
	if !exists {
		return domainregistry.Provider{}, false, registryport.ErrNotFound
	}
	if prior.Revision != plan.expected.ProviderRevision || prior.Generation != plan.expected.ProviderGeneration ||
		prior.Incarnation != plan.expected.ProviderIncarnation ||
		prior.CredentialPurpose != plan.expected.ProviderCredentialPurpose {
		return domainregistry.Provider{}, false, registryport.ErrConflict
	}
	if (plan.operation == domainregistry.OperationDisconnect || plan.operation == domainregistry.OperationExplicitDelete) &&
		plan.purpose != secretstoreport.Purpose(prior.CredentialPurpose) {
		return domainregistry.Provider{}, false, registryport.ErrConflict
	}
	if prior.Tombstone && plan.operation != domainregistry.OperationExplicitDelete {
		return domainregistry.Provider{}, false, registryport.ErrConflict
	}
	if plan.operation == domainregistry.OperationSelect && prior.PrivateAccount != nil {
		return domainregistry.Provider{}, false, registryport.ErrInvalidRequest
	}
	if plan.operation == domainregistry.OperationSelect &&
		(prior.CredentialRef == "" || prior.CredentialPurpose == "") {
		return domainregistry.Provider{}, false, registryport.ErrVerification
	}
	if plan.operation == domainregistry.OperationSelect &&
		!selectionRouteProvidersAvailable(state, prior) {
		return domainregistry.Provider{}, false, registryport.ErrVerification
	}
	return prior, true, nil
}

// selectionRouteProvidersAvailable is a Registry-only admission check. A
// selected Provider and every explicit route target must be public, live, and
// backed by a normal K2 credential commit. It deliberately does not read the
// Secret Store: K2 commit/readback establishes the nonempty credential fence,
// while execution performs the later authorized read.
func selectionRouteProvidersAvailable(state domainregistry.Registry, selected domainregistry.Provider) bool {
	routeProviderIDs, err := selected.RouteProviderIDs()
	if err != nil {
		return false
	}
	for _, providerID := range routeProviderIDs {
		provider, exists := state.Providers[providerID]
		if !exists || provider.Validate() != nil || provider.Tombstone || provider.PrivateAccount != nil ||
			provider.CredentialRef == "" || provider.CredentialPurpose == "" {
			return false
		}
	}
	return true
}

func buildTransaction(
	state domainregistry.Registry,
	plan mutationPlan,
	prior domainregistry.Provider,
	exists bool,
	candidate secretstoreport.PreparedCandidate,
) (domainregistry.Transaction, *domainregistry.Provider, error) {
	if state.Revision == math.MaxUint64 {
		return domainregistry.Transaction{}, nil, registryport.ErrConflict
	}
	if exists && prior.Revision == math.MaxUint64 {
		return domainregistry.Transaction{}, nil, registryport.ErrConflict
	}
	transactionID, err := newOpaqueID("txn_")
	if err != nil {
		return domainregistry.Transaction{}, nil, registryport.ErrPersistence
	}
	providerIncarnation := prior.Incarnation
	if !exists {
		providerIncarnation, err = newOpaqueID("inc_")
		if err != nil {
			return domainregistry.Transaction{}, nil, registryport.ErrPersistence
		}
	}
	candidateRef := ""
	candidatePurpose := ""
	if candidate != nil {
		candidateRef = string(candidate.CredentialRef())
		candidatePurpose = string(plan.purpose)
	}
	credentialRef := prior.CredentialRef
	credentialPurpose := prior.CredentialPurpose
	providerRevision := prior.Revision + 1
	generation := prior.Generation
	if !exists {
		providerRevision = 1
		generation = 1
	}
	if candidate != nil && exists {
		credentialRef = candidateRef
		credentialPurpose = string(plan.purpose)
		if generation == math.MaxUint64 {
			return domainregistry.Transaction{}, nil, registryport.ErrConflict
		}
		generation++
	} else if candidate != nil {
		credentialRef = candidateRef
		credentialPurpose = string(plan.purpose)
	}
	bindingChanged := exists && plan.operation == domainregistry.OperationUpdate &&
		!reflect.DeepEqual(prior.OAuthBinding, plan.provider.OAuthBinding)
	if bindingChanged {
		if generation == math.MaxUint64 {
			return domainregistry.Transaction{}, nil, registryport.ErrConflict
		}
		generation++
		if plan.credential.Kind() == secretstoreport.MutationExplicitDelete {
			credentialRef = ""
			credentialPurpose = ""
		}
	}

	var next *domainregistry.Provider
	nextSelected := state.SelectedProviderID
	switch plan.operation {
	case domainregistry.OperationConnect:
		value := plan.provider.Provider(providerIncarnation, candidateRef, string(plan.purpose), 1, 1)
		next = &value
		if nextSelected == "" && value.PrivateAccount == nil && !plan.deferSelection {
			nextSelected = value.ID
		}
	case domainregistry.OperationUpdate:
		value := plan.provider.Provider(providerIncarnation, credentialRef, credentialPurpose, providerRevision, generation)
		next = &value
	case domainregistry.OperationSelect:
		value := prior.Clone()
		value.Revision = providerRevision
		next = &value
		nextSelected = prior.ID
	case domainregistry.OperationCredentialReplace:
		value := prior.Clone()
		value.CredentialRef = candidateRef
		value.CredentialPurpose = string(plan.purpose)
		value.Revision = providerRevision
		value.Generation = generation
		next = &value
	case domainregistry.OperationDisconnect:
		if (prior.PrivateAccount == nil && plan.privateAccountDisposition != "") ||
			(prior.PrivateAccount != nil && plan.privateAccountDisposition == "") {
			return domainregistry.Transaction{}, nil, registryport.ErrInvalidRequest
		}
		if prior.Generation == math.MaxUint64 {
			return domainregistry.Transaction{}, nil, registryport.ErrConflict
		}
		value := prior.Clone()
		value.CredentialRef = ""
		value.CredentialPurpose = ""
		value.Revision = providerRevision
		value.Generation++
		value.Tombstone = true
		if value.PrivateAccount != nil {
			value.PrivateAccountDisposition = plan.privateAccountDisposition
		}
		value.SelectedRoutes = nil
		next = &value
		if nextSelected == prior.ID {
			nextSelected = ""
		}
	case domainregistry.OperationExplicitDelete:
		if nextSelected == prior.ID {
			nextSelected = ""
		}
	default:
		return domainregistry.Transaction{}, nil, registryport.ErrInvalidRequest
	}
	if next != nil && next.Validate() != nil {
		return domainregistry.Transaction{}, nil, registryport.ErrInvalidRequest
	}

	var priorPointer *domainregistry.Provider
	if exists {
		value := prior.Clone()
		priorPointer = &value
	}
	superseded := ""
	supersededPurpose := ""
	if candidate != nil || bindingChanged && plan.credential.Kind() == secretstoreport.MutationExplicitDelete ||
		plan.operation == domainregistry.OperationDisconnect || plan.operation == domainregistry.OperationExplicitDelete {
		superseded = prior.CredentialRef
		supersededPurpose = prior.CredentialPurpose
	}
	fence := domainregistry.Fence{
		RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
		SelectedProviderID: state.SelectedProviderID,
	}
	if exists {
		fence.ProviderRevision = prior.Revision
		fence.ProviderGeneration = prior.Generation
		fence.ProviderIncarnation = prior.Incarnation
		fence.CurrentCredentialRef = prior.CredentialRef
		fence.CurrentCredentialPurpose = prior.CredentialPurpose
	}
	pending := domainregistry.Transaction{
		DeferSelection: plan.deferSelection,
		Version:        domainregistry.TransactionVersion, ID: transactionID,
		Operation: plan.operation, Phase: domainregistry.PhasePrepared, ProviderID: plan.providerID,
		Resolution:                 domainregistry.ResolutionPending,
		CandidateCredentialPurpose: candidatePurpose, SupersededCredentialPurpose: supersededPurpose, Fence: fence,
		CandidateCredentialRef: candidateRef, SupersededCredentialRef: superseded,
		PriorProvider: priorPointer, NextProvider: next, NextSelectedProviderID: nextSelected,
		Recovery: domainregistry.RecoveryRecord{
			Version: domainregistry.RecoveryRecordVersion, LastObserved: domainregistry.PhasePrepared,
		},
	}
	if pending.Validate() != nil {
		return domainregistry.Transaction{}, nil, registryport.ErrInvalidRequest
	}
	return pending, next, nil
}

func commitWinner(
	state domainregistry.Registry,
	pending domainregistry.Transaction,
) (domainregistry.Registry, domainregistry.Transaction, error) {
	if !fenceMatches(state, pending) || state.Revision == math.MaxUint64 {
		return domainregistry.Registry{}, domainregistry.Transaction{}, registryport.ErrConflict
	}
	state = state.Clone()
	if pending.NextProvider == nil {
		delete(state.Providers, pending.ProviderID)
	} else {
		state.Providers[pending.ProviderID] = pending.NextProvider.Clone()
	}
	state.SelectedProviderID = pending.NextSelectedProviderID
	state.Revision++
	pending.CommittedRegistryRevision = state.Revision
	pending.Resolution = domainregistry.ResolutionWinner
	pending.ResolvedProvider = cloneProviderPointer(pending.NextProvider)
	pending.ResolvedSelectedProviderID = pending.NextSelectedProviderID
	pending.CleanupCredentialRef = pending.SupersededCredentialRef
	pending.CleanupCredentialPurpose = pending.SupersededCredentialPurpose
	pending = withPhase(pending, domainregistry.PhaseMetadataCommitted)
	state.Transactions[pending.ID] = pending
	if state.Validate() != nil {
		return domainregistry.Registry{}, domainregistry.Transaction{}, registryport.ErrPersistence
	}
	return state, pending, nil
}

func fenceMatches(state domainregistry.Registry, pending domainregistry.Transaction) bool {
	fence := pending.Fence
	if state.Revision != fence.RegistryRevision || state.Incarnation != fence.RegistryIncarnation ||
		state.SelectedProviderID != fence.SelectedProviderID {
		return false
	}
	provider, exists := state.Providers[pending.ProviderID]
	if fence.ProviderIncarnation == "" {
		return !exists
	}
	return exists && provider.Revision == fence.ProviderRevision && provider.Generation == fence.ProviderGeneration &&
		provider.Incarnation == fence.ProviderIncarnation && provider.CredentialRef == fence.CurrentCredentialRef &&
		provider.CredentialPurpose == fence.CurrentCredentialPurpose
}

func winnerMatches(state domainregistry.Registry, pending domainregistry.Transaction) bool {
	if state.Incarnation != pending.Fence.RegistryIncarnation || pending.CommittedRegistryRevision == 0 ||
		state.Revision != pending.CommittedRegistryRevision ||
		state.SelectedProviderID != pending.ResolvedSelectedProviderID {
		return false
	}
	provider, exists := state.Providers[pending.ProviderID]
	if pending.ResolvedProvider == nil {
		return !exists
	}
	return exists && reflect.DeepEqual(provider, pending.ResolvedProvider.Clone())
}

func (manager *Manager) verifyWinner(
	ctx context.Context,
	storage registryport.Transaction,
	pending domainregistry.Transaction,
	expectedSecret []byte,
) error {
	state, loaded, err := loadPending(ctx, storage, pending.ID)
	if err != nil || !winnerMatches(state, loaded) {
		return registryport.ErrVerification
	}
	if loaded.ResolvedProvider == nil || loaded.ResolvedProvider.CredentialRef == "" {
		return nil
	}
	plaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(loaded.ResolvedProvider.CredentialRef),
		Purpose:       secretstoreport.Purpose(loaded.ResolvedProvider.CredentialPurpose), Consumer: RegistryReadbackConsumer,
	})
	if err != nil {
		return normalizeSecretError(err)
	}
	defer clear(plaintext)
	if expectedSecret != nil && !bytes.Equal(plaintext, expectedSecret) {
		return registryport.ErrVerification
	}
	return nil
}

func (manager *Manager) rollbackWinner(
	ctx context.Context,
	storage registryport.Transaction,
	pending domainregistry.Transaction,
) error {
	state, loaded, err := loadPending(ctx, storage, pending.ID)
	if err != nil || !winnerMatches(state, loaded) || state.Revision == math.MaxUint64 {
		return registryport.ErrVerification
	}
	state = state.Clone()
	if loaded.PriorProvider == nil {
		delete(state.Providers, loaded.ProviderID)
	} else {
		state.Providers[loaded.ProviderID] = loaded.PriorProvider.Clone()
	}
	state.SelectedProviderID = loaded.Fence.SelectedProviderID
	state.Revision++
	loaded.Resolution = domainregistry.ResolutionRollback
	loaded.ResolvedProvider = cloneProviderPointer(loaded.PriorProvider)
	loaded.ResolvedSelectedProviderID = loaded.Fence.SelectedProviderID
	loaded.CleanupCredentialRef = loaded.CandidateCredentialRef
	loaded.CleanupCredentialPurpose = loaded.CandidateCredentialPurpose
	loaded.CommittedRegistryRevision = state.Revision
	loaded = withPhase(loaded, domainregistry.PhaseVerified)
	state.Transactions[loaded.ID] = loaded
	if state.Validate() != nil {
		return registryport.ErrVerification
	}
	return normalizeRegistryStoreError(storage.Commit(ctx, state))
}

func (manager *Manager) finishCleanup(
	ctx context.Context,
	storage registryport.Transaction,
	transactionID string,
) error {
	state, pending, err := loadPending(ctx, storage, transactionID)
	if err != nil {
		return err
	}
	if !winnerMatches(state, pending) {
		return registryport.ErrVerification
	}
	if pending.Phase == domainregistry.PhaseVerified {
		if pending.CleanupCredentialRef == "" {
			delete(state.Transactions, pending.ID)
			return normalizeRegistryStoreError(storage.Commit(ctx, state))
		}
		if !destructiveCleanupAllowed(state, pending) {
			return registryport.ErrVerification
		}
		err := manager.secrets.Tombstone(ctx,
			secretstoreport.CredentialRef(pending.CleanupCredentialRef),
			secretstoreport.Purpose(pending.CleanupCredentialPurpose),
		)
		if err != nil && !errors.Is(err, secretstoreport.ErrTombstoned) && !errors.Is(err, secretstoreport.ErrNotFound) {
			return normalizeSecretError(err)
		}
		if err := manager.observe(FaultAfterSupersededTombstoned); err != nil {
			return err
		}
		state, pending, err = loadPending(ctx, storage, transactionID)
		if err != nil {
			return err
		}
		pending = withPhase(pending, domainregistry.PhaseSupersededTombstoned)
		state.Transactions[pending.ID] = pending
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterTombstoneRecorded); err != nil {
			return err
		}
	}

	state, pending, err = loadPending(ctx, storage, transactionID)
	if err != nil {
		return err
	}
	if pending.Phase == domainregistry.PhaseSupersededTombstoned {
		if !destructiveCleanupAllowed(state, pending) {
			return registryport.ErrVerification
		}
		err := manager.secrets.ExplicitDelete(ctx,
			secretstoreport.CredentialRef(pending.CleanupCredentialRef),
			secretstoreport.Purpose(pending.CleanupCredentialPurpose),
			secretstoreport.ExplicitlyDeleteCredential(),
		)
		if err != nil && !errors.Is(err, secretstoreport.ErrNotFound) {
			return normalizeSecretError(err)
		}
		if err := manager.observe(FaultAfterSupersededDeleted); err != nil {
			return err
		}
		state, pending, err = loadPending(ctx, storage, transactionID)
		if err != nil {
			return err
		}
		pending = withPhase(pending, domainregistry.PhaseSupersededDeleted)
		state.Transactions[pending.ID] = pending
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterDeleteRecorded); err != nil {
			return err
		}
	}

	state, pending, err = loadPending(ctx, storage, transactionID)
	if err != nil {
		return err
	}
	if pending.Phase != domainregistry.PhaseSupersededDeleted {
		return registryport.ErrPersistence
	}
	delete(state.Transactions, pending.ID)
	return normalizeRegistryStoreError(storage.Commit(ctx, state))
}

func destructiveCleanupAllowed(state domainregistry.Registry, pending domainregistry.Transaction) bool {
	if state.Validate() != nil || pending.CleanupCredentialRef == "" ||
		(pending.Phase != domainregistry.PhaseVerified && pending.Phase != domainregistry.PhaseSupersededTombstoned) ||
		!winnerMatches(state, pending) {
		return false
	}
	for _, provider := range state.Providers {
		if provider.CredentialRef == pending.CleanupCredentialRef {
			return false
		}
	}
	for id, transaction := range state.Transactions {
		if id == pending.ID {
			continue
		}
		if transaction.CandidateCredentialRef == pending.CleanupCredentialRef ||
			transaction.SupersededCredentialRef == pending.CleanupCredentialRef ||
			transaction.CleanupCredentialRef == pending.CleanupCredentialRef {
			return false
		}
	}
	return true
}

func (manager *Manager) recoverAll(ctx context.Context, storage registryport.Transaction) error {
	if err := manager.recoverTransactions(ctx, storage); err != nil {
		return err
	}
	if err := manager.recoverLegacyMigrationRecoveries(ctx, storage); err != nil {
		return err
	}
	return manager.recoverProtectedRecoverySessions(ctx, storage)
}

func (manager *Manager) recoverTransactions(ctx context.Context, storage registryport.Transaction) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	for _, recovery := range state.LegacyMigrationRecoveries {
		if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
			len(state.Transactions) == 0 {
			continue
		}
		if len(state.Transactions) != 1 {
			return registryport.ErrVerification
		}
		for _, transaction := range state.Transactions {
			if transaction.ProviderID != recovery.ProviderID {
				return registryport.ErrVerification
			}
			if err := manager.validateLegacyMigrationProviderTransaction(ctx, state, transaction); err != nil {
				return err
			}
		}
	}
	ids := make([]string, 0, len(state.Transactions))
	for id := range state.Transactions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := manager.recoverOne(ctx, storage, id); err != nil {
			return err
		}
	}
	return nil
}

func (manager *Manager) recoverOne(ctx context.Context, storage registryport.Transaction, transactionID string) error {
	state, pending, err := loadPending(ctx, storage, transactionID)
	if err != nil {
		return err
	}
	if err := manager.validateLegacyMigrationProviderTransaction(ctx, state, pending); err != nil {
		return err
	}
	if pending.Recovery.Attempts == math.MaxUint32 {
		return registryport.ErrPersistence
	}
	pending.Recovery.Attempts++
	pending.Recovery.LastObserved = pending.Phase
	state.Transactions[pending.ID] = pending
	if err := storage.Commit(ctx, state); err != nil {
		return normalizeRegistryStoreError(err)
	}

	state, pending, err = loadPending(ctx, storage, transactionID)
	if err != nil {
		return err
	}
	switch pending.Phase {
	case domainregistry.PhasePrepared:
		if pending.CandidateCredentialRef != "" {
			err := manager.readCandidate(ctx, pending)
			if errors.Is(err, secretstoreport.ErrNotFound) {
				delete(state.Transactions, pending.ID)
				return normalizeRegistryStoreError(storage.Commit(ctx, state))
			}
			if errors.Is(err, secretstoreport.ErrTombstoned) {
				return manager.cleanupStaleCandidate(ctx, storage, pending)
			}
			if err != nil {
				return normalizeSecretError(err)
			}
			if !fenceMatches(state, pending) {
				return manager.cleanupStaleCandidate(ctx, storage, pending)
			}
			pending = withPhase(pending, domainregistry.PhaseCandidateDurable)
			state.Transactions[pending.ID] = pending
			if err := storage.Commit(ctx, state); err != nil {
				return normalizeRegistryStoreError(err)
			}
		} else {
			if !fenceMatches(state, pending) {
				delete(state.Transactions, pending.ID)
				return normalizeRegistryStoreError(storage.Commit(ctx, state))
			}
			state, pending, err = commitWinner(state, pending)
			if err != nil {
				return err
			}
			if err := storage.Commit(ctx, state); err != nil {
				return normalizeRegistryStoreError(err)
			}
		}
		return manager.recoverOne(ctx, storage, transactionID)
	case domainregistry.PhaseCandidateDurable:
		if err := manager.readCandidate(ctx, pending); err != nil {
			if errors.Is(err, secretstoreport.ErrNotFound) {
				delete(state.Transactions, pending.ID)
				return normalizeRegistryStoreError(storage.Commit(ctx, state))
			}
			if errors.Is(err, secretstoreport.ErrTombstoned) {
				return manager.cleanupStaleCandidate(ctx, storage, pending)
			}
			return normalizeSecretError(err)
		}
		if !fenceMatches(state, pending) {
			return manager.cleanupStaleCandidate(ctx, storage, pending)
		}
		state, pending, err = commitWinner(state, pending)
		if err != nil {
			return err
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		return manager.recoverOne(ctx, storage, transactionID)
	case domainregistry.PhaseMetadataCommitted:
		if err := manager.verifyWinner(ctx, storage, pending, nil); err != nil {
			if rollbackErr := manager.rollbackWinner(ctx, storage, pending); rollbackErr != nil {
				return registryport.ErrVerification
			}
			if cleanupErr := manager.finishCleanup(ctx, storage, pending.ID); cleanupErr != nil {
				return cleanupErr
			}
			return registryport.ErrVerification
		}
		state, pending, err = loadPending(ctx, storage, transactionID)
		if err != nil {
			return err
		}
		pending = withPhase(pending, domainregistry.PhaseVerified)
		state.Transactions[pending.ID] = pending
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		return manager.finishCleanup(ctx, storage, transactionID)
	case domainregistry.PhaseVerified, domainregistry.PhaseSupersededTombstoned, domainregistry.PhaseSupersededDeleted:
		return manager.finishCleanup(ctx, storage, transactionID)
	default:
		return registryport.ErrPersistence
	}
}

func (manager *Manager) readCandidate(ctx context.Context, pending domainregistry.Transaction) error {
	plaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(pending.CandidateCredentialRef),
		Purpose:       secretstoreport.Purpose(pending.CandidateCredentialPurpose), Consumer: RegistryReadbackConsumer,
	})
	clear(plaintext)
	return err
}

func (manager *Manager) cleanupStaleCandidate(
	ctx context.Context,
	storage registryport.Transaction,
	pending domainregistry.Transaction,
) error {
	state, loaded, err := loadPending(ctx, storage, pending.ID)
	if err != nil {
		return err
	}
	loaded.Resolution = domainregistry.ResolutionAbandoned
	loaded.CleanupCredentialRef = loaded.CandidateCredentialRef
	loaded.CleanupCredentialPurpose = loaded.CandidateCredentialPurpose
	if current, exists := state.Providers[loaded.ProviderID]; exists {
		value := current.Clone()
		loaded.ResolvedProvider = &value
	} else {
		loaded.ResolvedProvider = nil
	}
	loaded.ResolvedSelectedProviderID = state.SelectedProviderID
	loaded.CommittedRegistryRevision = state.Revision
	loaded = withPhase(loaded, domainregistry.PhaseVerified)
	state.Transactions[loaded.ID] = loaded
	if err := storage.Commit(ctx, state); err != nil {
		return normalizeRegistryStoreError(err)
	}
	return manager.finishCleanup(ctx, storage, loaded.ID)
}

func loadPending(
	ctx context.Context,
	storage registryport.Transaction,
	transactionID string,
) (domainregistry.Registry, domainregistry.Transaction, error) {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return domainregistry.Registry{}, domainregistry.Transaction{}, registryport.ErrPersistence
	}
	pending, ok := state.Transactions[transactionID]
	if !ok {
		return domainregistry.Registry{}, domainregistry.Transaction{}, registryport.ErrNotFound
	}
	return state, pending, nil
}

func withPhase(pending domainregistry.Transaction, phase domainregistry.Phase) domainregistry.Transaction {
	pending.Phase = phase
	pending.Recovery.LastObserved = phase
	return pending
}

func cloneProviderPointer(provider *domainregistry.Provider) *domainregistry.Provider {
	if provider == nil {
		return nil
	}
	clone := provider.Clone()
	return &clone
}

func normalizeRegistryStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, registryport.ErrInvalidRequest):
		return registryport.ErrInvalidRequest
	case errors.Is(err, registryport.ErrConflict):
		return registryport.ErrConflict
	case errors.Is(err, registryport.ErrNotFound):
		return registryport.ErrNotFound
	default:
		return registryport.ErrPersistence
	}
}

func normalizeManagerError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, ErrInterrupted):
		return ErrInterrupted
	case errors.Is(err, registryport.ErrInvalidRequest):
		return registryport.ErrInvalidRequest
	case errors.Is(err, registryport.ErrConflict):
		return registryport.ErrConflict
	case errors.Is(err, registryport.ErrNotFound):
		return registryport.ErrNotFound
	case errors.Is(err, registryport.ErrVerification):
		return registryport.ErrVerification
	case errors.Is(err, registryport.ErrCredentialUnavailable):
		return registryport.ErrCredentialUnavailable
	case errors.Is(err, errProtectedRecoveryNotFound):
		return registryport.ErrNotFound
	case errors.Is(err, errProtectedRecoveryConsumed), errors.Is(err, errProtectedRecoveryNotConfirmed),
		errors.Is(err, errProtectedRecoveryCurrentness):
		return registryport.ErrConflict
	case errors.Is(err, errProtectedRecoveryProtocol):
		return registryport.ErrVerification
	default:
		return registryport.ErrPersistence
	}
}

func newOpaqueID(prefix string) (string, error) {
	random := make([]byte, 32)
	defer clear(random)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}
