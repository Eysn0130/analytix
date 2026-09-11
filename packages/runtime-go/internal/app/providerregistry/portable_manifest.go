package providerregistry

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
)

const portableImportEntryStatusReentryRequired = domainregistry.PortableManifestIntentReentryRequired

// PortableImportEntryResult is a key-free mapping from one manifest-local
// correlation to the destination-minted Provider identity. It contains no
// credential reference, source identity, or trust decision.
type PortableImportEntryResult struct {
	Correlation           string `json:"correlation"`
	DestinationProviderID string `json:"destinationProviderId"`
	Status                string `json:"status"`
}

// PortableImportResult contains only bounded key-free counts and destination
// mappings. The mapping is suitable for a later re-entry flow but is not an
// authorization or credential result.
type PortableImportResult struct {
	ProviderCount   int                         `json:"providerCount"`
	AccountCount    int                         `json:"accountCount"`
	ReentryRequired int                         `json:"reentryRequired"`
	Entries         []PortableImportEntryResult `json:"entries"`
}

// ExportPortableManifest projects committed Provider metadata without
// recovering, reading, or serializing Secret Store state.
func (manager *Manager) ExportPortableManifest(ctx context.Context) ([]byte, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return nil, registryport.ErrInvalidRequest
	}
	var result []byte
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := manager.recoverProtectedRecoverySessions(ctx, storage); err != nil {
			return err
		}
		state, err = storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if len(state.Transactions) != 0 || portableManifestHasNonTerminalRecovery(state) {
			return registryport.ErrConflict
		}
		manifest, err := portableManifestFromRegistry(state)
		if err != nil {
			return registryport.ErrInvalidRequest
		}
		result, err = domainregistry.MarshalPortableManifestV1(manifest)
		if err != nil {
			return registryport.ErrInvalidRequest
		}
		return nil
	})
	if err != nil {
		return nil, normalizeManagerError(err)
	}
	return result, nil
}

// ImportPortableManifest performs all parsing and manifest preflight before
// entering the Registry transaction. The successful path performs exactly one
// Registry commit and never calls the Secret Store.
func (manager *Manager) ImportPortableManifest(
	ctx context.Context,
	body []byte,
) (PortableImportResult, error) {
	result := PortableImportResult{
		Entries: make([]PortableImportEntryResult, 0),
	}
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return result, registryport.ErrInvalidRequest
	}
	manifest, err := domainregistry.ParsePortableManifestV1(body)
	if err != nil {
		return result, registryport.ErrInvalidRequest
	}
	if len(manifest.Providers) == 0 && len(manifest.Accounts) == 0 {
		return result, nil
	}
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := manager.recoverProtectedRecoverySessions(ctx, storage); err != nil {
			return err
		}
		state, err = storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if len(state.Transactions) != 0 || portableManifestHasNonTerminalRecovery(state) {
			return registryport.ErrConflict
		}
		entryCount := len(manifest.Providers) + len(manifest.Accounts)
		if entryCount > domainregistry.MaxProviders || len(state.Providers) > domainregistry.MaxProviders-entryCount {
			return registryport.ErrConflict
		}
		privateCount := 0
		for _, provider := range state.Providers {
			if provider.PrivateAccount != nil {
				privateCount++
			}
		}
		if len(manifest.Accounts) > domainregistry.MaxPrivateAccounts ||
			privateCount > domainregistry.MaxPrivateAccounts-len(manifest.Accounts) {
			return registryport.ErrConflict
		}
		if state.Revision == math.MaxUint64 {
			return registryport.ErrConflict
		}

		providerIdentities := make(map[string]struct{}, len(state.Providers)+len(manifest.Providers))
		accountIdentities := make(map[string]struct{}, privateCount+len(manifest.Accounts))
		for _, provider := range state.Providers {
			if provider.Kind == domainregistry.PrivateAccountKind {
				identity, err := portableAccountIdentityForRegistryProvider(provider)
				if err != nil {
					return registryport.ErrInvalidRequest
				}
				if _, duplicate := accountIdentities[identity]; duplicate {
					return registryport.ErrConflict
				}
				accountIdentities[identity] = struct{}{}
				continue
			}
			identity, err := portableProviderIdentityForRegistryProvider(provider)
			if err != nil {
				return registryport.ErrInvalidRequest
			}
			if _, duplicate := providerIdentities[identity]; duplicate {
				return registryport.ErrConflict
			}
			providerIdentities[identity] = struct{}{}
		}
		for _, descriptor := range manifest.Providers {
			identity, err := descriptor.CanonicalIdentity()
			if err != nil {
				return registryport.ErrInvalidRequest
			}
			if _, collision := providerIdentities[identity]; collision {
				return registryport.ErrConflict
			}
			providerIdentities[identity] = struct{}{}
		}
		for _, descriptor := range manifest.Accounts {
			identity, err := descriptor.CanonicalIdentity()
			if err != nil {
				return registryport.ErrInvalidRequest
			}
			if _, collision := accountIdentities[identity]; collision {
				return registryport.ErrConflict
			}
			accountIdentities[identity] = struct{}{}
		}

		// Allocate every destination Provider ID and incarnation before building
		// any candidate. No source ID is used as an authority or persistence key.
		allocatedProviders := make(map[string]domainregistry.Provider, len(state.Providers)+entryCount)
		for id, provider := range state.Providers {
			allocatedProviders[id] = provider
		}
		type providerAllocation struct {
			descriptor  domainregistry.PortableProviderDescriptorV1
			id          string
			incarnation string
		}
		providerAllocations := make([]providerAllocation, 0, len(manifest.Providers))
		providerIDsByCorrelation := make(map[string]string, len(manifest.Providers))
		for _, descriptor := range manifest.Providers {
			providerID, err := newPortableProviderID(allocatedProviders)
			if err != nil {
				return registryport.ErrPersistence
			}
			allocatedProviders[providerID] = domainregistry.Provider{ID: providerID}
			incarnation, err := newOpaqueID("inc_")
			if err != nil {
				return registryport.ErrPersistence
			}
			providerAllocations = append(providerAllocations, providerAllocation{
				descriptor: descriptor, id: providerID, incarnation: incarnation,
			})
			providerIDsByCorrelation[descriptor.Correlation] = providerID
		}

		type accountAllocation struct {
			descriptor     domainregistry.PortableAccountDescriptorV1
			id             string
			incarnation    string
			scopeProvider  string
			scopeAccountID string
			scopeChannelID string
		}
		accountAllocations := make([]accountAllocation, 0, len(manifest.Accounts))
		for _, descriptor := range manifest.Accounts {
			providerID, err := newPortableProviderID(allocatedProviders)
			if err != nil {
				return registryport.ErrPersistence
			}
			allocatedProviders[providerID] = domainregistry.Provider{ID: providerID}
			incarnation, err := newOpaqueID("inc_")
			if err != nil {
				return registryport.ErrPersistence
			}
			scopeProvider, err := newOpaqueID("owner-")
			if err != nil {
				return registryport.ErrPersistence
			}
			accountID, err := newOpaqueID("acct_")
			if err != nil {
				return registryport.ErrPersistence
			}
			channelID, err := newOpaqueID("chan_")
			if err != nil {
				return registryport.ErrPersistence
			}
			accountAllocations = append(accountAllocations, accountAllocation{
				descriptor: descriptor, id: providerID, incarnation: incarnation,
				scopeProvider: scopeProvider, scopeAccountID: accountID, scopeChannelID: channelID,
			})
		}

		// Translate manifest-local route correlations only after all public
		// destination IDs exist. Persisted routes contain destination IDs with
		// an explicit prefix, never source IDs or manifest correlation strings.
		next := state.Clone()
		if next.Providers == nil {
			next.Providers = make(map[string]domainregistry.Provider)
		}
		pendingResult := PortableImportResult{
			Entries: make([]PortableImportEntryResult, 0, entryCount),
		}
		for _, allocation := range providerAllocations {
			normalized, err := allocation.descriptor.Normalize()
			if err != nil {
				return registryport.ErrInvalidRequest
			}
			translatedRoutes := make([]string, len(normalized.Routes))
			for index, routeCorrelation := range normalized.Routes {
				destinationID, exists := providerIDsByCorrelation[routeCorrelation]
				if !exists || !domainregistry.ValidProviderID(destinationID) {
					return registryport.ErrInvalidRequest
				}
				translatedRoutes[index] = domainregistry.ExactProviderRoutePrefix + destinationID
			}
			input := domainregistry.ProviderInput{
				ID: allocation.id, Kind: normalized.Kind, Endpoint: normalized.Endpoint, Proxy: normalized.Proxy,
				Models: slices.Clone(normalized.Models), MediaModels: slices.Clone(normalized.MediaModels),
				SelectedModel: normalized.SelectedModel, SelectedMedia: normalized.SelectedMedia,
				SelectedRoutes: translatedRoutes, OAuthBinding: clonePortableOAuthBinding(normalized.OAuthBinding),
				AccountObservation: clonePortableAccountObservation(normalized.AccountObservation),
			}
			provider := input.Provider(allocation.incarnation, "", "", 1, 1)
			if provider.Validate() != nil {
				return registryport.ErrInvalidRequest
			}
			next.Providers[provider.ID] = provider
			pendingResult.ProviderCount++
			pendingResult.ReentryRequired++
			pendingResult.Entries = append(pendingResult.Entries, PortableImportEntryResult{
				Correlation: allocation.descriptor.Correlation, DestinationProviderID: provider.ID,
				Status: portableImportEntryStatusReentryRequired,
			})
		}
		for _, allocation := range accountAllocations {
			normalized, err := allocation.descriptor.Normalize()
			if err != nil || normalized.Proxy != "" {
				return registryport.ErrInvalidRequest
			}
			scope := &domainregistry.PrivateAccountScope{
				SchemaVersion: 1, Owner: normalized.Owner, Provider: allocation.scopeProvider,
				AccountID: allocation.scopeAccountID, ChannelID: allocation.scopeChannelID, Purpose: normalized.Purpose,
			}
			if scope.Validate() != nil {
				return registryport.ErrInvalidRequest
			}
			input := domainregistry.ProviderInput{
				ID: allocation.id, Kind: domainregistry.PrivateAccountKind, Endpoint: normalized.Endpoint,
				PrivateAccount: scope,
			}
			provider := input.Provider(allocation.incarnation, "", "", 1, 1)
			if provider.Validate() != nil {
				return registryport.ErrInvalidRequest
			}
			next.Providers[provider.ID] = provider
			pendingResult.AccountCount++
			pendingResult.ReentryRequired++
			pendingResult.Entries = append(pendingResult.Entries, PortableImportEntryResult{
				Correlation: allocation.descriptor.Correlation, DestinationProviderID: provider.ID,
				Status: portableImportEntryStatusReentryRequired,
			})
		}
		nextRevision := state.Revision + 1
		if nextRevision == 0 {
			nextRevision = 1
		}
		next.Revision = nextRevision
		if next.Validate() != nil {
			return registryport.ErrInvalidRequest
		}
		if err := storage.Commit(ctx, next); err != nil {
			return normalizeRegistryStoreError(err)
		}
		result = pendingResult
		return nil
	})
	if err != nil {
		return PortableImportResult{}, normalizeManagerError(err)
	}
	return result, nil
}

type portableProviderExportCandidate struct {
	sourceID   string
	provider   domainregistry.Provider
	descriptor domainregistry.PortableProviderDescriptorV1
}

func portableManifestFromRegistry(state domainregistry.Registry) (domainregistry.PortableManifestV1, error) {
	manifest := domainregistry.PortableManifestV1{
		Schema:    domainregistry.PortableManifestSchemaV1,
		Providers: make([]domainregistry.PortableProviderDescriptorV1, 0, len(state.Providers)),
		Accounts:  make([]domainregistry.PortableAccountDescriptorV1, 0),
	}
	candidates := make([]portableProviderExportCandidate, 0, len(state.Providers))
	for id, provider := range state.Providers {
		if provider.Tombstone {
			continue
		}
		if provider.Kind == domainregistry.PrivateAccountKind {
			descriptor, err := portableAccountDescriptorForRegistryProvider(provider)
			if err != nil {
				return domainregistry.PortableManifestV1{}, err
			}
			manifest.Accounts = append(manifest.Accounts, descriptor)
			continue
		}
		descriptor, err := portableProviderDescriptorForRegistryProvider(provider)
		if err != nil {
			return domainregistry.PortableManifestV1{}, err
		}
		candidates = append(candidates, portableProviderExportCandidate{
			sourceID: id, provider: provider, descriptor: descriptor,
		})
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftKey, _ := candidates[left].descriptor.CanonicalIdentity()
		rightKey, _ := candidates[right].descriptor.CanonicalIdentity()
		return leftKey < rightKey
	})
	providerCorrelationBySourceID := make(map[string]string, len(candidates))
	for index := range candidates {
		correlation := fmt.Sprintf("provider-%d", index)
		candidates[index].descriptor.Correlation = correlation
		providerCorrelationBySourceID[candidates[index].sourceID] = correlation
	}
	for index := range candidates {
		provider := candidates[index].provider
		if len(provider.SelectedRoutes) == 0 {
			candidates[index].descriptor.Routes = []string{}
			continue
		}
		routeProviderIDs, err := provider.RouteProviderIDs()
		if err != nil {
			return domainregistry.PortableManifestV1{}, err
		}
		routes := make([]string, len(routeProviderIDs))
		for routeIndex, routeProviderID := range routeProviderIDs {
			correlation, exists := providerCorrelationBySourceID[routeProviderID]
			if !exists {
				// The target is tombstoned, private, unexported, or otherwise
				// ambiguous. Never degrade topology to a cardinality-only route.
				return domainregistry.PortableManifestV1{}, registryport.ErrInvalidRequest
			}
			routes[routeIndex] = correlation
		}
		candidates[index].descriptor.Routes = routes
	}
	for _, candidate := range candidates {
		manifest.Providers = append(manifest.Providers, candidate.descriptor)
	}
	sort.Slice(manifest.Accounts, func(left, right int) bool {
		leftKey, _ := manifest.Accounts[left].CanonicalIdentity()
		rightKey, _ := manifest.Accounts[right].CanonicalIdentity()
		return leftKey < rightKey
	})
	for index := range manifest.Accounts {
		manifest.Accounts[index].Correlation = fmt.Sprintf("account-%d", index)
	}
	if err := manifest.Validate(); err != nil {
		return domainregistry.PortableManifestV1{}, err
	}
	return manifest, nil
}

func portableProviderDescriptorForRegistryProvider(
	provider domainregistry.Provider,
) (domainregistry.PortableProviderDescriptorV1, error) {
	if provider.PrivateAccount != nil || provider.Kind == domainregistry.PrivateAccountKind {
		return domainregistry.PortableProviderDescriptorV1{}, registryport.ErrInvalidRequest
	}
	descriptor := domainregistry.PortableProviderDescriptorV1{
		Correlation:   "provider-0",
		Kind:          provider.Kind,
		Endpoint:      provider.Endpoint,
		Proxy:         provider.Proxy,
		Models:        slices.Clone(provider.Models),
		MediaModels:   slices.Clone(provider.MediaModels),
		SelectedModel: provider.SelectedModel,
		SelectedMedia: provider.SelectedMedia,
		Routes:        []string{},
		Intent:        domainregistry.PortableManifestIntentReentryRequired,
	}
	if provider.OAuthBinding != nil {
		binding := *provider.OAuthBinding
		binding.Scopes = slices.Clone(provider.OAuthBinding.Scopes)
		descriptor.OAuthBinding = &binding
	}
	if provider.AccountObservation != nil {
		observation := *provider.AccountObservation
		descriptor.AccountObservation = &observation
	}
	return descriptor.Normalize()
}

func portableAccountDescriptorForRegistryProvider(
	provider domainregistry.Provider,
) (domainregistry.PortableAccountDescriptorV1, error) {
	if provider.PrivateAccount == nil || provider.PrivateAccount.Owner == "provider" ||
		provider.PrivateAccount.Owner == "transport" || provider.Proxy != "" {
		return domainregistry.PortableAccountDescriptorV1{}, registryport.ErrInvalidRequest
	}
	scope := provider.PrivateAccount
	return domainregistry.PortableAccountDescriptorV1{
		Correlation: "account-0", Owner: scope.Owner, Provider: scope.Provider,
		Endpoint: provider.Endpoint, Purpose: scope.Purpose,
		Intent: domainregistry.PortableManifestIntentReentryRequired,
	}.Normalize()
}

func portableProviderIdentityForRegistryProvider(provider domainregistry.Provider) (string, error) {
	descriptor, err := portableProviderDescriptorForRegistryProvider(provider)
	if err != nil {
		return "", err
	}
	return descriptor.CanonicalIdentity()
}

func portableAccountIdentityForRegistryProvider(provider domainregistry.Provider) (string, error) {
	descriptor, err := portableAccountDescriptorForRegistryProvider(provider)
	if err != nil {
		return "", err
	}
	return descriptor.CanonicalIdentity()
}

func portableManifestHasNonTerminalRecovery(state domainregistry.Registry) bool {
	for _, recovery := range state.LegacyMigrationRecoveries {
		if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalized &&
			recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
			return true
		}
	}
	for _, session := range state.ProtectedRecoverySessions {
		switch session.Phase {
		case domainregistry.ProtectedRecoveryPhaseFinalized, domainregistry.ProtectedRecoveryPhaseRolledBack:
			continue
		case domainregistry.ProtectedRecoveryPhaseApplied:
			// An Applied session with no pending destination key is already a
			// verified receipt-ready terminal. The source may still finalize its
			// own session, but it cannot change destination authority.
			if session.DestinationKeyRef == "" {
				continue
			}
			return true
		default:
			return true
		}
	}
	return false
}

func clonePortableOAuthBinding(binding *domainregistry.OAuthBindingMetadata) *domainregistry.OAuthBindingMetadata {
	if binding == nil {
		return nil
	}
	clone := *binding
	clone.Scopes = slices.Clone(binding.Scopes)
	return &clone
}

func clonePortableAccountObservation(observation *domainregistry.AccountObservationBinding) *domainregistry.AccountObservationBinding {
	if observation == nil {
		return nil
	}
	clone := *observation
	return &clone
}

func newPortableProviderID(providers map[string]domainregistry.Provider) (string, error) {
	for attempts := 0; attempts < 4; attempts++ {
		id, err := newOpaqueID("provider-")
		if err != nil {
			return "", err
		}
		id = strings.ToLower(id)
		if _, exists := providers[id]; !exists && domainregistry.ValidProviderID(id) {
			return id, nil
		}
	}
	return "", registryport.ErrConflict
}
