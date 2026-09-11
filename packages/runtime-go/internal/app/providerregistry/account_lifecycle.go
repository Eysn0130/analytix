package providerregistry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const AccountExecutionConsumer = "account-lifecycle-execution"

type AccountCredentialStatus string

const (
	AccountCredentialStatusAbsent          AccountCredentialStatus = "absent"
	AccountCredentialStatusReady           AccountCredentialStatus = "ready"
	AccountCredentialStatusRevoked         AccountCredentialStatus = "revoked"
	AccountCredentialStatusDisconnected    AccountCredentialStatus = "disconnected"
	AccountCredentialStatusUnusable        AccountCredentialStatus = "unusable"
	AccountCredentialDispositionRevoke                             = "revoke"
	AccountCredentialDispositionDisconnect                         = "disconnect"
	AccountCredentialDispositionDelete                             = "delete"
)

type AccountCredentialState struct {
	Scope domainregistry.PrivateAccountScope
	// ProviderID may be a destination-minted imported alias. It is internal
	// routing metadata and is excluded from public JSON projections.
	ProviderID          string `json:"-"`
	Status              AccountCredentialStatus
	RegistryRevision    uint64
	RegistryIncarnation string
	ProviderRevision    uint64
	ProviderGeneration  uint64
	ProviderIncarnation string
	CredentialPurpose   string
}

func (state AccountCredentialState) Expected() domainregistry.ExpectedState {
	return domainregistry.ExpectedState{
		RegistryRevision: state.RegistryRevision, RegistryIncarnation: state.RegistryIncarnation,
		ProviderRevision: state.ProviderRevision, ProviderGeneration: state.ProviderGeneration,
		ProviderIncarnation: state.ProviderIncarnation, ProviderCredentialPurpose: state.CredentialPurpose,
	}
}

type AccountCredentialPutCommand struct {
	Scope      domainregistry.PrivateAccountScope
	Expected   domainregistry.ExpectedState
	Credential []byte
}

type AccountCredentialMutationCommand struct {
	Scope       domainregistry.PrivateAccountScope
	Expected    domainregistry.ExpectedState
	Disposition string
}

type AccountCredentialListFilter struct {
	Owner   string
	Purpose string
}

type AccountCredentialResolution struct {
	State      AccountCredentialState
	Credential []byte
}

func (resolution *AccountCredentialResolution) Clear() {
	if resolution == nil {
		return
	}
	clear(resolution.Credential)
	resolution.Credential = nil
}

func AccountProviderID(scope domainregistry.PrivateAccountScope) (string, error) {
	if scope.Validate() != nil {
		return "", registryport.ErrInvalidRequest
	}
	if providerOAuthAccountSlot(scope) {
		return scope.Provider, nil
	}
	digest := sha256.Sum256([]byte(scope.Owner + "\x00" + scope.Provider + "\x00" + scope.AccountID + "\x00" + scope.ChannelID + "\x00" + scope.Purpose))
	return "acct_" + hex.EncodeToString(digest[:24]), nil
}

func providerOAuthAccountSlot(scope domainregistry.PrivateAccountScope) bool {
	return scope.Owner == "provider" && scope.Purpose == "provider-oauth-token-bundle" &&
		scope.ChannelID == "" && scope.AccountID == scope.Provider && domainregistry.ValidProviderID(scope.Provider)
}

func accountProviderIDForState(state domainregistry.Registry, scope domainregistry.PrivateAccountScope) (string, error) {
	deterministic, err := AccountProviderID(scope)
	if err != nil {
		return "", err
	}
	if provider, exists := state.Providers[deterministic]; exists {
		if providerOAuthAccountSlot(scope) {
			if provider.Kind == domainregistry.PrivateAccountKind || provider.PrivateAccount != nil || provider.ID != scope.Provider {
				return "", registryport.ErrConflict
			}
			return deterministic, nil
		}
		if provider.Kind != domainregistry.PrivateAccountKind || provider.PrivateAccount == nil ||
			!reflect.DeepEqual(*provider.PrivateAccount, scope) {
			return "", registryport.ErrConflict
		}
		return deterministic, nil
	}
	if providerOAuthAccountSlot(scope) {
		return deterministic, nil
	}
	var alias string
	for id, provider := range state.Providers {
		if provider.Kind != domainregistry.PrivateAccountKind || provider.PrivateAccount == nil ||
			!reflect.DeepEqual(*provider.PrivateAccount, scope) {
			continue
		}
		if alias != "" && alias != id {
			return "", registryport.ErrConflict
		}
		alias = id
	}
	if alias != "" {
		return alias, nil
	}
	return deterministic, nil
}

func (manager *Manager) AccountCredentialState(
	ctx context.Context,
	scope domainregistry.PrivateAccountScope,
) (AccountCredentialState, error) {
	providerID, err := AccountProviderID(scope)
	if err != nil {
		return AccountCredentialState{}, err
	}
	snapshot, err := manager.Snapshot(ctx)
	if err != nil {
		return AccountCredentialState{}, err
	}
	result := AccountCredentialState{
		Scope: scope, ProviderID: providerID, Status: AccountCredentialStatusAbsent,
		RegistryRevision: snapshot.Revision, RegistryIncarnation: snapshot.Incarnation,
	}
	providerID, err = accountProviderIDForState(snapshot, scope)
	if err != nil {
		return AccountCredentialState{}, err
	}
	result.ProviderID = providerID
	provider, exists := snapshot.Providers[providerID]
	if !exists {
		return result, nil
	}
	if providerOAuthAccountSlot(scope) {
		if provider.Kind == domainregistry.PrivateAccountKind || provider.PrivateAccount != nil || provider.ID != scope.Provider {
			return AccountCredentialState{}, registryport.ErrConflict
		}
	} else {
		if provider.Kind != domainregistry.PrivateAccountKind || provider.PrivateAccount == nil ||
			!reflect.DeepEqual(*provider.PrivateAccount, scope) {
			return AccountCredentialState{}, registryport.ErrConflict
		}
	}
	result.ProviderRevision = provider.Revision
	result.ProviderGeneration = provider.Generation
	result.ProviderIncarnation = provider.Incarnation
	result.CredentialPurpose = provider.CredentialPurpose
	if provider.Tombstone {
		if provider.PrivateAccountDisposition == AccountCredentialDispositionDisconnect {
			result.Status = AccountCredentialStatusDisconnected
		} else {
			result.Status = AccountCredentialStatusRevoked
		}
	} else if provider.CredentialRef == "" && provider.CredentialPurpose == "" {
		// An imported, destination-minted account alias is a real Registry
		// entry but intentionally has no credential yet; expose it as absent
		// so the ordinary K2 re-entry path can fill that exact alias.
		result.Status = AccountCredentialStatusAbsent
	} else if provider.CredentialRef == "" || (!providerOAuthAccountSlot(scope) && provider.CredentialPurpose != scope.Purpose) {
		result.Status = AccountCredentialStatusUnusable
	} else {
		result.Status = AccountCredentialStatusReady
	}
	return result, nil
}

func (manager *Manager) ListAccountCredentialStates(
	ctx context.Context,
	filter AccountCredentialListFilter,
) ([]AccountCredentialState, error) {
	validFilter := (filter.Owner == "provider" && filter.Purpose == "provider-oauth-authorization-state") ||
		(filter.Owner == "provider" && filter.Purpose == "provider-oauth-token-bundle") ||
		(filter.Owner == "mcp" && filter.Purpose == "mcp-oauth-authorization-state") ||
		(filter.Owner == "mcp" && filter.Purpose == "mcp-oauth-access-token") ||
		(filter.Owner == "extension" && (filter.Purpose == "extension-provider-account-token" ||
			filter.Purpose == "extension-oauth-authorization-state"))
	if manager == nil || manager.registry == nil || ctx == nil || !validFilter {
		return nil, registryport.ErrInvalidRequest
	}
	result := []AccountCredentialState{}
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		registry, err := storage.Load(ctx)
		if err != nil || registry.Validate() != nil || len(registry.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		for _, provider := range registry.Providers {
			if filter.Owner == "provider" && filter.Purpose == "provider-oauth-token-bundle" &&
				provider.Kind != domainregistry.PrivateAccountKind && provider.PrivateAccount == nil &&
				provider.CredentialPurpose == filter.Purpose {
				scope := domainregistry.PrivateAccountScope{
					SchemaVersion: 1, Owner: "provider", Provider: provider.ID,
					AccountID: provider.ID, Purpose: filter.Purpose,
				}
				status := AccountCredentialStatusReady
				if provider.Tombstone {
					status = AccountCredentialStatusRevoked
				}
				result = append(result, AccountCredentialState{
					Scope: scope, ProviderID: provider.ID, Status: status,
					RegistryRevision: registry.Revision, RegistryIncarnation: registry.Incarnation,
					ProviderRevision: provider.Revision, ProviderGeneration: provider.Generation,
					ProviderIncarnation: provider.Incarnation, CredentialPurpose: provider.CredentialPurpose,
				})
				continue
			}
			if provider.PrivateAccount == nil || provider.PrivateAccount.Owner != filter.Owner ||
				provider.PrivateAccount.Purpose != filter.Purpose {
				continue
			}
			status := AccountCredentialStatusReady
			if provider.Tombstone {
				if provider.PrivateAccountDisposition == AccountCredentialDispositionDisconnect {
					status = AccountCredentialStatusDisconnected
				} else {
					status = AccountCredentialStatusRevoked
				}
			} else if provider.CredentialRef == "" || provider.CredentialPurpose != provider.PrivateAccount.Purpose {
				status = AccountCredentialStatusUnusable
			}
			result = append(result, AccountCredentialState{
				Scope: *provider.PrivateAccount, ProviderID: provider.ID, Status: status,
				RegistryRevision: registry.Revision, RegistryIncarnation: registry.Incarnation,
				ProviderRevision: provider.Revision, ProviderGeneration: provider.Generation,
				ProviderIncarnation: provider.Incarnation, CredentialPurpose: provider.CredentialPurpose,
			})
		}
		return nil
	})
	if err != nil {
		return nil, normalizeManagerError(err)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].Scope, result[j].Scope
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		if left.AccountID != right.AccountID {
			return left.AccountID < right.AccountID
		}
		return left.ChannelID < right.ChannelID
	})
	return result, nil
}

func (manager *Manager) PutAccountCredential(
	ctx context.Context,
	command AccountCredentialPutCommand,
) (AccountCredentialState, error) {
	providerID, err := AccountProviderID(command.Scope)
	if err != nil || len(command.Credential) == 0 {
		return AccountCredentialState{}, registryport.ErrInvalidRequest
	}
	credential, err := secretstoreport.SetCredential(command.Credential)
	if err != nil {
		return AccountCredentialState{}, registryport.ErrInvalidRequest
	}
	input := domainregistry.ProviderInput{
		ID: providerID, Kind: domainregistry.PrivateAccountKind,
		Endpoint: "https://private-account.invalid/" + providerID,
		Models:   []string{}, MediaModels: []string{}, SelectedRoutes: []string{},
		PrivateAccount: &command.Scope,
	}
	current, err := manager.AccountCredentialState(ctx, command.Scope)
	if err != nil {
		return AccountCredentialState{}, err
	}
	if !reflect.DeepEqual(current.Expected(), command.Expected) {
		return AccountCredentialState{}, registryport.ErrConflict
	}
	if providerOAuthAccountSlot(command.Scope) {
		if current.Status != AccountCredentialStatusReady {
			return AccountCredentialState{}, registryport.ErrConflict
		}
		_, err = manager.ReplaceCredential(ctx, CredentialReplaceCommand{
			Expected: command.Expected, ProviderID: providerID,
			CredentialPurpose: secretstoreport.Purpose(command.Scope.Purpose), Credential: credential,
		})
		if err != nil {
			return AccountCredentialState{}, err
		}
		return manager.AccountCredentialState(ctx, command.Scope)
	}
	if current.ProviderID != "" {
		providerID = current.ProviderID
		input.ID = providerID
	}
	if current.Status == AccountCredentialStatusRevoked || current.Status == AccountCredentialStatusDisconnected {
		if err := manager.ExplicitDelete(ctx, ExplicitDeleteCommand{
			Expected: command.Expected, ProviderID: providerID,
		}); err != nil {
			return AccountCredentialState{}, err
		}
		current, err = manager.AccountCredentialState(ctx, command.Scope)
		if err != nil || current.Status != AccountCredentialStatusAbsent {
			return AccountCredentialState{}, registryport.ErrConflict
		}
	}
	if current.Status == AccountCredentialStatusAbsent {
		if current.ProviderRevision != 0 && current.ProviderID != "" {
			// A protected portable import keeps its destination Provider entry
			// ID while the exact destination account scope is re-entered. Update
			// that existing alias instead of attempting Connect, whose contract
			// correctly rejects an already-present Provider ID.
			snapshot, snapshotErr := manager.Snapshot(ctx)
			if snapshotErr != nil {
				return AccountCredentialState{}, snapshotErr
			}
			existing, exists := snapshot.Providers[current.ProviderID]
			if !exists || existing.Tombstone || existing.PrivateAccount == nil {
				return AccountCredentialState{}, registryport.ErrConflict
			}
			existing = existing.Clone()
			input = domainregistry.ProviderInput{
				ID: existing.ID, Kind: existing.Kind, Endpoint: existing.Endpoint, Proxy: existing.Proxy,
				Models: existing.Models, MediaModels: existing.MediaModels,
				SelectedModel: existing.SelectedModel, SelectedMedia: existing.SelectedMedia,
				SelectedRoutes: existing.SelectedRoutes, OAuthBinding: existing.OAuthBinding,
				AccountObservation: existing.AccountObservation, PrivateAccount: &command.Scope,
			}
			_, err = manager.Update(ctx, UpdateCommand{
				Expected: current.Expected(), Provider: input,
				CredentialPurpose: secretstoreport.Purpose(command.Scope.Purpose), Credential: credential,
			})
		} else {
			_, err = manager.Connect(ctx, ConnectCommand{
				Expected: current.Expected(), Provider: input,
				CredentialPurpose: secretstoreport.Purpose(command.Scope.Purpose), Credential: credential,
			})
		}
	} else if current.Status == AccountCredentialStatusReady {
		_, err = manager.ReplaceCredential(ctx, CredentialReplaceCommand{
			Expected: command.Expected, ProviderID: providerID,
			CredentialPurpose: secretstoreport.Purpose(command.Scope.Purpose), Credential: credential,
		})
	} else {
		return AccountCredentialState{}, registryport.ErrVerification
	}
	if err != nil {
		return AccountCredentialState{}, err
	}
	return manager.AccountCredentialState(ctx, command.Scope)
}

func (manager *Manager) MutateAccountCredential(
	ctx context.Context,
	command AccountCredentialMutationCommand,
) (AccountCredentialState, error) {
	providerID, err := AccountProviderID(command.Scope)
	if err != nil {
		return AccountCredentialState{}, err
	}
	current, err := manager.AccountCredentialState(ctx, command.Scope)
	if err != nil {
		return AccountCredentialState{}, err
	}
	if !reflect.DeepEqual(current.Expected(), command.Expected) {
		return AccountCredentialState{}, registryport.ErrConflict
	}
	if current.ProviderID != "" {
		providerID = current.ProviderID
	}
	switch command.Disposition {
	case AccountCredentialDispositionRevoke, AccountCredentialDispositionDisconnect:
		if current.Status != AccountCredentialStatusReady {
			return AccountCredentialState{}, registryport.ErrConflict
		}
		privateDisposition := command.Disposition
		if providerOAuthAccountSlot(command.Scope) {
			privateDisposition = ""
		}
		_, err = manager.Disconnect(ctx, DisconnectCommand{
			Expected: command.Expected, ProviderID: providerID,
			CredentialPurpose:         secretstoreport.Purpose(current.CredentialPurpose),
			PrivateAccountDisposition: privateDisposition,
		})
	case AccountCredentialDispositionDelete:
		if current.Status == AccountCredentialStatusAbsent {
			return current, nil
		}
		err = manager.ExplicitDelete(ctx, ExplicitDeleteCommand{
			Expected: command.Expected, ProviderID: providerID,
			CredentialPurpose: secretstoreport.Purpose(current.CredentialPurpose),
		})
	default:
		return AccountCredentialState{}, registryport.ErrInvalidRequest
	}
	if err != nil {
		return AccountCredentialState{}, err
	}
	return manager.AccountCredentialState(ctx, command.Scope)
}

func (manager *Manager) ResolveAccountCredential(
	ctx context.Context,
	scope domainregistry.PrivateAccountScope,
) (AccountCredentialResolution, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return AccountCredentialResolution{}, registryport.ErrInvalidRequest
	}
	var result AccountCredentialResolution
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverAll(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		providerID, idErr := accountProviderIDForState(state, scope)
		if idErr != nil {
			return idErr
		}
		provider, exists := state.Providers[providerID]
		if !exists || provider.Tombstone || provider.CredentialRef == "" || provider.CredentialPurpose != scope.Purpose {
			return registryport.ErrNotFound
		}
		if providerOAuthAccountSlot(scope) {
			if provider.Kind == domainregistry.PrivateAccountKind || provider.PrivateAccount != nil || provider.ID != scope.Provider {
				return registryport.ErrNotFound
			}
		} else if provider.Kind != domainregistry.PrivateAccountKind || provider.PrivateAccount == nil ||
			!reflect.DeepEqual(*provider.PrivateAccount, scope) {
			return registryport.ErrNotFound
		}
		plaintext, readErr := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
			CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
			Purpose:       secretstoreport.Purpose(provider.CredentialPurpose), Consumer: AccountExecutionConsumer,
		})
		if readErr != nil {
			clear(plaintext)
			if errors.Is(readErr, secretstoreport.ErrNotFound) || errors.Is(readErr, secretstoreport.ErrTombstoned) {
				return registryport.ErrNotFound
			}
			return normalizeSecretError(readErr)
		}
		if len(plaintext) == 0 {
			clear(plaintext)
			return registryport.ErrVerification
		}
		result = AccountCredentialResolution{
			State: AccountCredentialState{
				Scope: scope, Status: AccountCredentialStatusReady,
				RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
				ProviderRevision: provider.Revision, ProviderGeneration: provider.Generation,
				ProviderIncarnation: provider.Incarnation, CredentialPurpose: provider.CredentialPurpose,
			},
			Credential: plaintext,
		}
		return nil
	})
	if err != nil {
		result.Clear()
		return AccountCredentialResolution{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) ValidateAccountCredentialCurrent(
	ctx context.Context,
	state AccountCredentialState,
) error {
	if state.Status != AccountCredentialStatusReady {
		return registryport.ErrInvalidRequest
	}
	current, err := manager.AccountCredentialState(ctx, state.Scope)
	if err != nil {
		return err
	}
	if current.Status != AccountCredentialStatusReady || !reflect.DeepEqual(current.Expected(), state.Expected()) {
		return registryport.ErrConflict
	}
	return nil
}
