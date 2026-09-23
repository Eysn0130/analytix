package providerregistry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/netip"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	FormatVersion                  = 1
	TransactionVersion             = 1
	RecoveryRecordVersion          = 1
	LegacyMigrationRecoveryVersion = 2
	MaxRegistryBytes               = 16 << 20
	MaxProviders                   = 256
	MaxPrivateAccounts             = 128
	MaxOAuthAuthorizationStates    = 32
	ReservedPublicProviderCapacity = 64
	MaxTransactions                = 256
	MaxLegacyMigrationRecoveries   = 256
	MaxModels                      = 512
	MaxRoutes                      = 128
	PrimaryRouteAlias              = "primary"
	ExactProviderRoutePrefix       = "provider:"
	LegacyMigrationRecoveryPurpose = "provider-settings-migration-recovery"
	PrivateAccountKind             = "analytix-private-account"
)

type Resolution string

const (
	ResolutionPending   Resolution = "pending"
	ResolutionWinner    Resolution = "winner"
	ResolutionRollback  Resolution = "rollback"
	ResolutionAbandoned Resolution = "abandoned"
)

var ErrInvalidRegistry = errors.New("provider registry: invalid state")

type Operation string

const (
	OperationConnect           Operation = "connect"
	OperationUpdate            Operation = "update"
	OperationSelect            Operation = "select"
	OperationCredentialReplace Operation = "credential-replace"
	OperationDisconnect        Operation = "disconnect"
	OperationExplicitDelete    Operation = "explicit-delete"
)

type Phase string

const (
	PhasePrepared             Phase = "prepared"
	PhaseCandidateDurable     Phase = "candidate-durable"
	PhaseMetadataCommitted    Phase = "metadata-committed"
	PhaseVerified             Phase = "verified"
	PhaseSupersededTombstoned Phase = "superseded-tombstoned"
	PhaseSupersededDeleted    Phase = "superseded-deleted"
)

type LegacyMigrationRecoveryPhase string

const (
	LegacyMigrationRecoveryPhasePrepared                          LegacyMigrationRecoveryPhase = "prepared"
	LegacyMigrationRecoveryPhaseSecretDurable                     LegacyMigrationRecoveryPhase = "secret-durable"
	LegacyMigrationRecoveryPhaseVerified                          LegacyMigrationRecoveryPhase = "verified"
	LegacyMigrationRecoveryPhaseProviderCommitPrepared            LegacyMigrationRecoveryPhase = "provider-commit-prepared"
	LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained LegacyMigrationRecoveryPhase = "provider-committed-recovery-retained"
	LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending      LegacyMigrationRecoveryPhase = "rollback-cleaned-source-authority-pending"
	LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending     LegacyMigrationRecoveryPhase = "rollback-registry-commit-pending"
	LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained LegacyMigrationRecoveryPhase = "rollback-committed-recovery-retained"
	LegacyMigrationRecoveryPhaseRollbackRecoveryRetained          LegacyMigrationRecoveryPhase = "rollback-recovery-retained"
	LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending    LegacyMigrationRecoveryPhase = "protected-recovery-delete-pending"
	LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted          LegacyMigrationRecoveryPhase = "protected-recovery-deleted"
	LegacyMigrationRecoveryPhaseFinalizingPreCommit               LegacyMigrationRecoveryPhase = "finalizing-pre-commit"
	LegacyMigrationRecoveryPhaseFinalizingCommitted               LegacyMigrationRecoveryPhase = "finalizing-committed"
	LegacyMigrationRecoveryPhaseFinalizingRollback                LegacyMigrationRecoveryPhase = "finalizing-rollback"
	LegacyMigrationRecoveryPhaseFinalized                         LegacyMigrationRecoveryPhase = "finalized"
	LegacyMigrationRecoveryPhaseAbandoning                        LegacyMigrationRecoveryPhase = "abandoning"
)

const (
	LegacyMigrationFinalizationOutcomeCommitted  = "migration-committed"
	LegacyMigrationFinalizationOutcomeRolledBack = "protected-recovery-retained"
)

type Registry struct {
	Version                   int                                `json:"version"`
	Revision                  uint64                             `json:"revision"`
	Incarnation               string                             `json:"incarnation"`
	SelectedProviderID        string                             `json:"selectedProviderId,omitempty"`
	Providers                 map[string]Provider                `json:"providers"`
	Transactions              map[string]Transaction             `json:"transactions"`
	LegacyMigrationRecoveries map[string]LegacyMigrationRecovery `json:"legacyMigrationRecoveries,omitempty"`
	// ProtectedRecoverySessions contains only local, key-free protocol
	// metadata and opaque K1 references. It is intentionally separate from
	// ordinary mutation transactions so recovery cannot be selected or
	// executed by legacy consumers.
	ProtectedRecoverySessions map[string]ProtectedRecoverySessionV1 `json:"protectedRecoverySessions,omitempty"`
}

type ProviderInput struct {
	ID                 string                     `json:"id"`
	Kind               string                     `json:"kind"`
	Endpoint           string                     `json:"endpoint"`
	Proxy              string                     `json:"proxy,omitempty"`
	Models             []string                   `json:"models"`
	MediaModels        []string                   `json:"mediaModels"`
	SelectedModel      string                     `json:"selectedModel,omitempty"`
	SelectedMedia      string                     `json:"selectedMediaModel,omitempty"`
	SelectedRoutes     []string                   `json:"selectedRoutes"`
	OAuthBinding       *OAuthBindingMetadata      `json:"oauthBinding,omitempty"`
	AccountObservation *AccountObservationBinding `json:"accountObservation,omitempty"`
	PrivateAccount     *PrivateAccountScope       `json:"privateAccount,omitempty"`
}

// OAuthBindingMetadata is the versioned, key-free OAuth authority owned by a
// public Provider. Redirect URIs are deliberately absent: Main derives them
// from the exact binding digest and the native callback protocol.
type OAuthBindingMetadata struct {
	SchemaVersion         int      `json:"schemaVersion"`
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorizationEndpoint"`
	TokenEndpoint         string   `json:"tokenEndpoint"`
	RevocationEndpoint    string   `json:"revocationEndpoint,omitempty"`
	ClientID              string   `json:"clientId"`
	Scopes                []string `json:"scopes"`
	RedirectModeVersion   int      `json:"redirectModeVersion"`
}

// AccountObservationBinding is the exact, key-free Provider-owned authority
// for one bounded quota/subscription observation. It never carries a token,
// credential reference, redirect policy, or response-field selector.
type AccountObservationBinding struct {
	SchemaVersion int    `json:"schemaVersion"`
	Endpoint      string `json:"endpoint"`
	Method        string `json:"method"`
	Projection    string `json:"projection"`
}

// PrivateAccountScope is key-free metadata for an account credential owned by
// the existing Provider Registry. It is never part of the public Provider
// projection; secret bytes remain exclusively in K1 under Purpose.
type PrivateAccountScope struct {
	SchemaVersion int    `json:"schemaVersion"`
	Owner         string `json:"owner"`
	Provider      string `json:"provider"`
	AccountID     string `json:"accountId"`
	ChannelID     string `json:"channelId,omitempty"`
	Purpose       string `json:"purpose"`
}

type Provider struct {
	ID                        string                     `json:"id"`
	Kind                      string                     `json:"kind"`
	Endpoint                  string                     `json:"endpoint"`
	Proxy                     string                     `json:"proxy,omitempty"`
	Models                    []string                   `json:"models"`
	MediaModels               []string                   `json:"mediaModels"`
	SelectedModel             string                     `json:"selectedModel,omitempty"`
	SelectedMedia             string                     `json:"selectedMediaModel,omitempty"`
	SelectedRoutes            []string                   `json:"selectedRoutes"`
	CredentialRef             string                     `json:"credentialRef,omitempty"`
	CredentialPurpose         string                     `json:"credentialPurpose,omitempty"`
	Revision                  uint64                     `json:"revision"`
	Generation                uint64                     `json:"generation"`
	Incarnation               string                     `json:"incarnation"`
	Tombstone                 bool                       `json:"tombstone"`
	PrivateAccount            *PrivateAccountScope       `json:"privateAccount,omitempty"`
	PrivateAccountDisposition string                     `json:"privateAccountDisposition,omitempty"`
	OAuthBinding              *OAuthBindingMetadata      `json:"oauthBinding,omitempty"`
	AccountObservation        *AccountObservationBinding `json:"accountObservation,omitempty"`
}

type ExpectedState struct {
	RegistryRevision          uint64 `json:"registryRevision"`
	RegistryIncarnation       string `json:"registryIncarnation"`
	ProviderRevision          uint64 `json:"providerRevision"`
	ProviderGeneration        uint64 `json:"providerGeneration"`
	ProviderIncarnation       string `json:"providerIncarnation,omitempty"`
	ProviderCredentialPurpose string `json:"providerCredentialPurpose,omitempty"`
}

type Fence struct {
	RegistryRevision         uint64 `json:"registryRevision"`
	RegistryIncarnation      string `json:"registryIncarnation"`
	ProviderRevision         uint64 `json:"providerRevision"`
	ProviderGeneration       uint64 `json:"providerGeneration"`
	ProviderIncarnation      string `json:"providerIncarnation,omitempty"`
	CurrentCredentialRef     string `json:"currentCredentialRef,omitempty"`
	CurrentCredentialPurpose string `json:"currentCredentialPurpose,omitempty"`
	SelectedProviderID       string `json:"selectedProviderId,omitempty"`
}

type RecoveryRecord struct {
	Version      int    `json:"version"`
	Attempts     uint32 `json:"attempts"`
	LastObserved Phase  `json:"lastObserved"`
}

type LegacyMigrationRecoveryFence struct {
	RegistryRevision          uint64 `json:"registryRevision"`
	RegistryIncarnation       string `json:"registryIncarnation"`
	SelectedProviderID        string `json:"selectedProviderId,omitempty"`
	ProviderExists            bool   `json:"providerExists"`
	ProviderRevision          uint64 `json:"providerRevision,omitempty"`
	ProviderGeneration        uint64 `json:"providerGeneration,omitempty"`
	ProviderIncarnation       string `json:"providerIncarnation,omitempty"`
	ProviderCredentialRef     string `json:"providerCredentialRef,omitempty"`
	ProviderCredentialPurpose string `json:"providerCredentialPurpose,omitempty"`
}

type LegacyMigrationRecovery struct {
	Version                                  int                          `json:"version"`
	ID                                       string                       `json:"id"`
	Phase                                    LegacyMigrationRecoveryPhase `json:"phase"`
	ProviderID                               string                       `json:"providerId"`
	Provider                                 ProviderInput                `json:"provider"`
	CredentialPurpose                        string                       `json:"credentialPurpose"`
	RecoveryCredentialRef                    string                       `json:"recoveryCredentialRef"`
	RecoveryPurpose                          string                       `json:"recoveryPurpose"`
	SourceLocator                            string                       `json:"sourceLocator,omitempty"`
	SourceSHA256                             string                       `json:"sourceSHA256,omitempty"`
	ExpectedCleanedSourceSHA256              string                       `json:"expectedCleanedSourceSHA256,omitempty"`
	SourcePhysicalIdentitySHA256             string                       `json:"sourcePhysicalIdentitySHA256,omitempty"`
	SourceIdentitySHA256                     string                       `json:"sourceIdentitySHA256,omitempty"`
	ExpectedCleanedSourceIdentitySHA256      string                       `json:"expectedCleanedSourceIdentitySHA256,omitempty"`
	PriorSelectedProviderIdentitySHA256      string                       `json:"priorSelectedProviderIdentitySHA256,omitempty"`
	Fence                                    LegacyMigrationRecoveryFence `json:"fence"`
	CommittedProviderCredentialRef           string                       `json:"committedProviderCredentialRef,omitempty"`
	CommittedProviderCredentialPurpose       string                       `json:"committedProviderCredentialPurpose,omitempty"`
	CommittedProviderRevision                uint64                       `json:"committedProviderRevision,omitempty"`
	CommittedProviderGeneration              uint64                       `json:"committedProviderGeneration,omitempty"`
	CommittedProviderIncarnation             string                       `json:"committedProviderIncarnation,omitempty"`
	RollbackRegistryRevision                 uint64                       `json:"rollbackRegistryRevision,omitempty"`
	RollbackRegistryIncarnation              string                       `json:"rollbackRegistryIncarnation,omitempty"`
	RollbackSelectedProviderID               string                       `json:"rollbackSelectedProviderId,omitempty"`
	RollbackCleanedSourceStateIdentitySHA256 string                       `json:"rollbackCleanedSourceStateIdentitySHA256,omitempty"`
	RollbackCleanedSourceGenerationSHA256    string                       `json:"rollbackCleanedSourceGenerationSHA256,omitempty"`
	FinalizationOutcome                      string                       `json:"finalizationOutcome,omitempty"`
	FinalizationPreCommit                    bool                         `json:"finalizationPreCommit,omitempty"`
	FinalizedSourceIdentitySHA256            string                       `json:"finalizedSourceIdentitySHA256,omitempty"`
	FinalizedSourceStateIdentitySHA256       string                       `json:"finalizedSourceStateIdentitySHA256,omitempty"`
	FinalizedSourceGenerationSHA256          string                       `json:"finalizedSourceGenerationSHA256,omitempty"`
	PendingSourceAuthorityIntentSHA256       string                       `json:"pendingSourceAuthorityIntentSHA256,omitempty"`
	RemigrationPending                       bool                         `json:"remigrationPending,omitempty"`
	ProtectedRecoveryRetained                bool                         `json:"protectedRecoveryRetained,omitempty"`
	FinalizationRecoveryDeleteAuthorized     bool                         `json:"finalizationRecoveryDeleteAuthorized,omitempty"`
	FinalizationWinnerDeleteAuthorized       bool                         `json:"finalizationWinnerDeleteAuthorized,omitempty"`
}

type Transaction struct {
	Version                     int            `json:"version"`
	ID                          string         `json:"id"`
	Operation                   Operation      `json:"operation"`
	DeferSelection              bool           `json:"deferSelection,omitempty"`
	Phase                       Phase          `json:"phase"`
	ProviderID                  string         `json:"providerId"`
	CandidateCredentialPurpose  string         `json:"candidateCredentialPurpose,omitempty"`
	SupersededCredentialPurpose string         `json:"supersededCredentialPurpose,omitempty"`
	Fence                       Fence          `json:"fence"`
	CandidateCredentialRef      string         `json:"candidateCredentialRef,omitempty"`
	SupersededCredentialRef     string         `json:"supersededCredentialRef,omitempty"`
	PriorProvider               *Provider      `json:"priorProvider,omitempty"`
	NextProvider                *Provider      `json:"nextProvider,omitempty"`
	NextSelectedProviderID      string         `json:"nextSelectedProviderId,omitempty"`
	Resolution                  Resolution     `json:"resolution"`
	ResolvedProvider            *Provider      `json:"resolvedProvider,omitempty"`
	ResolvedSelectedProviderID  string         `json:"resolvedSelectedProviderId,omitempty"`
	CleanupCredentialRef        string         `json:"cleanupCredentialRef,omitempty"`
	CleanupCredentialPurpose    string         `json:"cleanupCredentialPurpose,omitempty"`
	CommittedRegistryRevision   uint64         `json:"committedRegistryRevision,omitempty"`
	Recovery                    RecoveryRecord `json:"recovery"`
}

func NewRegistry(incarnation string) (Registry, error) {
	registry := Registry{
		Version:                   FormatVersion,
		Incarnation:               incarnation,
		Providers:                 make(map[string]Provider),
		Transactions:              make(map[string]Transaction),
		LegacyMigrationRecoveries: make(map[string]LegacyMigrationRecovery),
	}
	if err := registry.Validate(); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func (registry Registry) Clone() Registry {
	clone := registry
	clone.Providers = make(map[string]Provider, len(registry.Providers))
	for id, provider := range registry.Providers {
		clone.Providers[id] = provider.Clone()
	}
	clone.Transactions = make(map[string]Transaction, len(registry.Transactions))
	for id, transaction := range registry.Transactions {
		clone.Transactions[id] = transaction.Clone()
	}
	if registry.LegacyMigrationRecoveries != nil {
		clone.LegacyMigrationRecoveries = make(map[string]LegacyMigrationRecovery, len(registry.LegacyMigrationRecoveries))
		for id, recovery := range registry.LegacyMigrationRecoveries {
			clone.LegacyMigrationRecoveries[id] = recovery.Clone()
		}
	}
	if registry.ProtectedRecoverySessions != nil {
		clone.ProtectedRecoverySessions = make(map[string]ProtectedRecoverySessionV1, len(registry.ProtectedRecoverySessions))
		for id, session := range registry.ProtectedRecoverySessions {
			clone.ProtectedRecoverySessions[id] = session.Clone()
		}
	}
	return clone
}

func (provider Provider) Clone() Provider {
	provider.Models = slices.Clone(provider.Models)
	provider.MediaModels = slices.Clone(provider.MediaModels)
	provider.SelectedRoutes = slices.Clone(provider.SelectedRoutes)
	if provider.PrivateAccount != nil {
		account := *provider.PrivateAccount
		provider.PrivateAccount = &account
	}
	if provider.OAuthBinding != nil {
		binding := *provider.OAuthBinding
		binding.Scopes = slices.Clone(provider.OAuthBinding.Scopes)
		provider.OAuthBinding = &binding
	}
	if provider.AccountObservation != nil {
		binding := *provider.AccountObservation
		provider.AccountObservation = &binding
	}
	return provider
}

// RouteProviderIDs gives the existing selectedRoutes field one canonical,
// backwards-compatible interpretation. An empty pool names the policy Provider
// itself when it has a selected model, but is an explicit disabled terminal
// when its model selection is empty. The legacy "primary" token names the
// policy Provider itself. Canonical writes use "provider:<id>" so a local
// Provider whose exact ID is "primary" remains addressable; legacy raw Provider
// IDs remain readable. Availability remains a Manager concern because
// credentials and tombstones can change after commit.
func (provider Provider) RouteProviderIDs() ([]string, error) {
	routes := provider.SelectedRoutes
	if len(routes) == 0 {
		if provider.SelectedModel == "" {
			return []string{}, nil
		}
		return []string{provider.ID}, nil
	}
	result := make([]string, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		providerID := ""
		switch {
		case route == PrimaryRouteAlias:
			providerID = provider.ID
		case strings.HasPrefix(route, ExactProviderRoutePrefix):
			providerID = strings.TrimPrefix(route, ExactProviderRoutePrefix)
		default:
			providerID = route
		}
		if !ValidProviderID(providerID) {
			return nil, ErrInvalidRegistry
		}
		if _, duplicate := seen[providerID]; duplicate {
			return nil, ErrInvalidRegistry
		}
		seen[providerID] = struct{}{}
		result = append(result, providerID)
	}
	return result, nil
}

func (transaction Transaction) Clone() Transaction {
	if transaction.PriorProvider != nil {
		prior := transaction.PriorProvider.Clone()
		transaction.PriorProvider = &prior
	}
	if transaction.NextProvider != nil {
		next := transaction.NextProvider.Clone()
		transaction.NextProvider = &next
	}
	if transaction.ResolvedProvider != nil {
		resolved := transaction.ResolvedProvider.Clone()
		transaction.ResolvedProvider = &resolved
	}
	return transaction
}

func (recovery LegacyMigrationRecovery) Clone() LegacyMigrationRecovery {
	recovery.Provider.Models = slices.Clone(recovery.Provider.Models)
	recovery.Provider.MediaModels = slices.Clone(recovery.Provider.MediaModels)
	recovery.Provider.SelectedRoutes = slices.Clone(recovery.Provider.SelectedRoutes)
	if recovery.Provider.OAuthBinding != nil {
		binding := *recovery.Provider.OAuthBinding
		binding.Scopes = slices.Clone(recovery.Provider.OAuthBinding.Scopes)
		recovery.Provider.OAuthBinding = &binding
	}
	if recovery.Provider.AccountObservation != nil {
		binding := *recovery.Provider.AccountObservation
		recovery.Provider.AccountObservation = &binding
	}
	return recovery
}

func (input ProviderInput) Provider(incarnation, credentialRef, credentialPurpose string, revision, generation uint64) Provider {
	provider := Provider{
		ID: input.ID, Kind: input.Kind, Endpoint: input.Endpoint, Proxy: input.Proxy,
		Models: slices.Clone(input.Models), MediaModels: slices.Clone(input.MediaModels),
		SelectedModel: input.SelectedModel, SelectedMedia: input.SelectedMedia,
		SelectedRoutes: slices.Clone(input.SelectedRoutes), CredentialRef: credentialRef,
		CredentialPurpose: credentialPurpose,
		Revision:          revision, Generation: generation, Incarnation: incarnation,
	}
	if input.OAuthBinding != nil {
		binding := *input.OAuthBinding
		binding.Scopes = slices.Clone(input.OAuthBinding.Scopes)
		provider.OAuthBinding = &binding
	}
	if input.AccountObservation != nil {
		binding := *input.AccountObservation
		provider.AccountObservation = &binding
	}
	if input.PrivateAccount != nil {
		account := *input.PrivateAccount
		provider.PrivateAccount = &account
	}
	return provider
}

func (registry Registry) Validate() error {
	if registry.Version != FormatVersion || !ValidIncarnation(registry.Incarnation) ||
		registry.Providers == nil || registry.Transactions == nil ||
		len(registry.Providers) > MaxProviders || len(registry.Transactions) > MaxTransactions ||
		len(registry.LegacyMigrationRecoveries) > MaxLegacyMigrationRecoveries ||
		len(registry.ProtectedRecoverySessions) > ProtectedRecoveryMaxEntries {
		return ErrInvalidRegistry
	}
	if len(registry.Providers) != 0 && registry.Revision == 0 {
		return ErrInvalidRegistry
	}
	seenCredentialRefs := make(map[string]struct{}, len(registry.Providers))
	credentialOwners := make(map[string]string, len(registry.Providers))
	for id, provider := range registry.Providers {
		if id != provider.ID || provider.Validate() != nil || provider.Revision > registry.Revision {
			return ErrInvalidRegistry
		}
		if provider.CredentialRef != "" {
			if _, duplicate := seenCredentialRefs[provider.CredentialRef]; duplicate {
				return ErrInvalidRegistry
			}
			seenCredentialRefs[provider.CredentialRef] = struct{}{}
			credentialOwners[provider.CredentialRef] = provider.ID
		}
	}
	if registry.SelectedProviderID != "" {
		selected, ok := registry.Providers[registry.SelectedProviderID]
		if !ok || selected.Tombstone {
			return ErrInvalidRegistry
		}
	}
	seenTransactionRefs := make(map[string]struct{}, len(registry.Transactions)*2)
	transactionOwnershipRefs := make(map[string]struct{}, len(registry.Transactions)*3)
	seenTransactionProviders := make(map[string]struct{}, len(registry.Transactions))
	for id, transaction := range registry.Transactions {
		if id != transaction.ID || transaction.Validate() != nil ||
			validateTransactionOwnership(registry, transaction, credentialOwners) != nil {
			return ErrInvalidRegistry
		}
		if _, duplicate := seenTransactionProviders[transaction.ProviderID]; duplicate {
			return ErrInvalidRegistry
		}
		seenTransactionProviders[transaction.ProviderID] = struct{}{}
		for _, ref := range []string{transaction.CandidateCredentialRef, transaction.SupersededCredentialRef} {
			if ref == "" {
				continue
			}
			if _, duplicate := seenTransactionRefs[ref]; duplicate {
				return ErrInvalidRegistry
			}
			seenTransactionRefs[ref] = struct{}{}
		}
		for _, ref := range []string{
			transaction.CandidateCredentialRef,
			transaction.SupersededCredentialRef,
			transaction.CleanupCredentialRef,
		} {
			if ref != "" {
				transactionOwnershipRefs[ref] = struct{}{}
			}
		}
	}
	seenRecoveryRefs := make(map[string]struct{}, len(registry.LegacyMigrationRecoveries))
	committedRecoveryAliases := make(map[string]string, len(registry.LegacyMigrationRecoveries))
	seenRecoveryProviders := make(map[string]struct{}, len(registry.LegacyMigrationRecoveries))
	for id, recovery := range registry.LegacyMigrationRecoveries {
		if id != recovery.ID || recovery.Validate() != nil {
			return ErrInvalidRegistry
		}
		if recovery.RecoveryCredentialRef != "" {
			if _, duplicate := seenRecoveryRefs[recovery.RecoveryCredentialRef]; duplicate {
				return ErrInvalidRegistry
			}
			seenRecoveryRefs[recovery.RecoveryCredentialRef] = struct{}{}
		}
		if recovery.Phase != LegacyMigrationRecoveryPhaseFinalized &&
			recovery.Phase != LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
			if _, duplicate := seenRecoveryProviders[recovery.ProviderID]; duplicate {
				return ErrInvalidRegistry
			}
			seenRecoveryProviders[recovery.ProviderID] = struct{}{}
		}
		if recovery.RecoveryCredentialRef != "" {
			if _, owned := credentialOwners[recovery.RecoveryCredentialRef]; owned {
				return ErrInvalidRegistry
			}
		}
		if recovery.RecoveryCredentialRef != "" {
			if _, owned := transactionOwnershipRefs[recovery.RecoveryCredentialRef]; owned {
				return ErrInvalidRegistry
			}
		}
		if recovery.Phase == LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
			recovery.Phase == LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending ||
			recovery.Phase == LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending ||
			recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingCommitted {
			provider, exists := registry.Providers[recovery.ProviderID]
			if !exists || !providerMatchesCommittedRecovery(provider, recovery) {
				return ErrInvalidRegistry
			}
			if owner := credentialOwners[recovery.CommittedProviderCredentialRef]; owner != recovery.ProviderID {
				return ErrInvalidRegistry
			}
			if _, owned := transactionOwnershipRefs[recovery.CommittedProviderCredentialRef]; owned {
				return ErrInvalidRegistry
			}
			if prior, duplicate := committedRecoveryAliases[recovery.CommittedProviderCredentialRef]; duplicate && prior != recovery.ID {
				return ErrInvalidRegistry
			}
			committedRecoveryAliases[recovery.CommittedProviderCredentialRef] = recovery.ID
		} else if recovery.Phase == LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained ||
			recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingRollback {
			if _, exists := registry.Providers[recovery.ProviderID]; exists ||
				recovery.RollbackRegistryRevision != registry.Revision ||
				recovery.RollbackRegistryIncarnation != registry.Incarnation ||
				recovery.RollbackSelectedProviderID != registry.SelectedProviderID {
				return ErrInvalidRegistry
			}
			if owner := credentialOwners[recovery.CommittedProviderCredentialRef]; owner != "" {
				return ErrInvalidRegistry
			}
			if _, owned := transactionOwnershipRefs[recovery.CommittedProviderCredentialRef]; owned {
				return ErrInvalidRegistry
			}
			if prior, duplicate := committedRecoveryAliases[recovery.CommittedProviderCredentialRef]; duplicate && prior != recovery.ID {
				return ErrInvalidRegistry
			}
			committedRecoveryAliases[recovery.CommittedProviderCredentialRef] = recovery.ID
		} else if recovery.Phase == LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending {
			if _, exists := registry.Providers[recovery.ProviderID]; exists ||
				recovery.RollbackRegistryRevision != registry.Revision ||
				recovery.RollbackRegistryIncarnation != registry.Incarnation ||
				recovery.RollbackSelectedProviderID != registry.SelectedProviderID {
				return ErrInvalidRegistry
			}
		} else if recovery.Phase == LegacyMigrationRecoveryPhaseRollbackRecoveryRetained ||
			recovery.Phase == LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
			if _, exists := registry.Providers[recovery.ProviderID]; exists ||
				recovery.RollbackRegistryIncarnation != registry.Incarnation {
				return ErrInvalidRegistry
			}
		}
	}
	protectedRecoveryRefs := make(map[string]string)
	for id, session := range registry.ProtectedRecoverySessions {
		if id != session.OperationID || session.Validate() != nil {
			return ErrInvalidRegistry
		}
		registerProtectedRecoveryRef := func(ref string) error {
			if ref == "" {
				return nil
			}
			if previous, exists := protectedRecoveryRefs[ref]; exists && previous != id {
				return ErrInvalidRegistry
			}
			if _, legacy := seenRecoveryRefs[ref]; legacy {
				return ErrInvalidRegistry
			}
			if _, transactionOwned := transactionOwnershipRefs[ref]; transactionOwned {
				return ErrInvalidRegistry
			}
			protectedRecoveryRefs[ref] = id
			return nil
		}
		if err := registerProtectedRecoveryRef(session.DestinationKeyRef); err != nil {
			return err
		}
		if session.DestinationKeyRef != "" && credentialOwners[session.DestinationKeyRef] != "" {
			return ErrInvalidRegistry
		}
		for index, ref := range session.CandidateRefs {
			if err := registerProtectedRecoveryRef(ref); err != nil {
				return err
			}
			if owner := credentialOwners[ref]; owner != "" &&
				(session.Phase != ProtectedRecoveryPhaseVerificationPending && session.Phase != ProtectedRecoveryPhaseApplied &&
					(session.Phase != ProtectedRecoveryPhaseCleanupPending || session.CleanupKind != "destination-key") ||
					index >= len(session.Entries) || owner != session.Entries[index].DestinationProviderID) {
				return ErrInvalidRegistry
			}
		}
		for _, ref := range session.CleanupRefs {
			if err := registerProtectedRecoveryRef(ref); err != nil {
				return err
			}
			if credentialOwners[ref] != "" {
				return ErrInvalidRegistry
			}
		}
	}
	for recoveryRef := range seenRecoveryRefs {
		if _, alias := committedRecoveryAliases[recoveryRef]; alias {
			return ErrInvalidRegistry
		}
	}
	return nil
}

func providerMatchesCommittedRecovery(provider Provider, recovery LegacyMigrationRecovery) bool {
	if provider.Tombstone || provider.CredentialRef != recovery.CommittedProviderCredentialRef ||
		provider.CredentialPurpose != recovery.CommittedProviderCredentialPurpose ||
		provider.Revision != recovery.CommittedProviderRevision ||
		provider.Generation != recovery.CommittedProviderGeneration ||
		provider.Incarnation != recovery.CommittedProviderIncarnation {
		return false
	}
	expected := recovery.Provider.Provider(
		provider.Incarnation,
		provider.CredentialRef,
		provider.CredentialPurpose,
		provider.Revision,
		provider.Generation,
	)
	return reflect.DeepEqual(provider.Clone(), expected)
}

func validateTransactionOwnership(registry Registry, transaction Transaction, credentialOwners map[string]string) error {
	candidateOwner := credentialOwners[transaction.CandidateCredentialRef]
	supersededOwner := credentialOwners[transaction.SupersededCredentialRef]
	cleanupOwner := credentialOwners[transaction.CleanupCredentialRef]
	if transaction.Resolution == ResolutionPending {
		if candidateOwner != "" {
			return ErrInvalidRegistry
		}
		if supersededOwner != "" && (supersededOwner != transaction.ProviderID ||
			transaction.PriorProvider == nil || transaction.PriorProvider.CredentialRef != transaction.SupersededCredentialRef) {
			return ErrInvalidRegistry
		}
		return nil
	}
	if registry.Revision != transaction.CommittedRegistryRevision ||
		registry.Incarnation != transaction.Fence.RegistryIncarnation ||
		registry.SelectedProviderID != transaction.ResolvedSelectedProviderID || cleanupOwner != "" {
		return ErrInvalidRegistry
	}
	resolved, exists := registry.Providers[transaction.ProviderID]
	if transaction.ResolvedProvider == nil {
		if exists {
			return ErrInvalidRegistry
		}
	} else if !exists || !reflect.DeepEqual(resolved, transaction.ResolvedProvider.Clone()) {
		return ErrInvalidRegistry
	}
	switch transaction.Resolution {
	case ResolutionWinner:
		if transaction.CandidateCredentialRef != "" && (candidateOwner != transaction.ProviderID ||
			transaction.ResolvedProvider == nil || transaction.ResolvedProvider.CredentialRef != transaction.CandidateCredentialRef) {
			return ErrInvalidRegistry
		}
		if supersededOwner != "" {
			return ErrInvalidRegistry
		}
	case ResolutionRollback, ResolutionAbandoned:
		if candidateOwner != "" {
			return ErrInvalidRegistry
		}
		if supersededOwner != "" && (supersededOwner != transaction.ProviderID ||
			transaction.ResolvedProvider == nil || transaction.ResolvedProvider.CredentialRef != transaction.SupersededCredentialRef) {
			return ErrInvalidRegistry
		}
	default:
		return ErrInvalidRegistry
	}
	return nil
}

func (input ProviderInput) Validate() error {
	provider := input.Provider("inc_"+strings.Repeat("a", 43), "", "", 1, 1)
	return provider.Validate()
}

func (provider Provider) Validate() error {
	if !validIdentifier(provider.ID, 96) || !validIdentifier(provider.Kind, 96) ||
		!validEndpoint(provider.Endpoint, false) || !validEndpoint(provider.Proxy, true) ||
		provider.Revision == 0 || provider.Generation == 0 || !ValidIncarnation(provider.Incarnation) ||
		len(provider.Models) > MaxModels || len(provider.MediaModels) > MaxModels ||
		len(provider.SelectedRoutes) > MaxRoutes || !validUniqueValues(provider.Models, 256) ||
		!validUniqueValues(provider.MediaModels, 256) || !validUniqueValues(provider.SelectedRoutes, 256) {
		return ErrInvalidRegistry
	}
	if provider.SelectedModel != "" && !slices.Contains(provider.Models, provider.SelectedModel) {
		return ErrInvalidRegistry
	}
	if provider.SelectedMedia != "" && !slices.Contains(provider.MediaModels, provider.SelectedMedia) {
		return ErrInvalidRegistry
	}
	if _, err := provider.RouteProviderIDs(); err != nil {
		return ErrInvalidRegistry
	}
	if provider.CredentialRef != "" && !ValidCredentialRef(provider.CredentialRef) {
		return ErrInvalidRegistry
	}
	if (provider.CredentialRef == "") != (provider.CredentialPurpose == "") ||
		(provider.CredentialPurpose != "" && !validPurpose(provider.CredentialPurpose)) {
		return ErrInvalidRegistry
	}
	if provider.Tombstone && (provider.CredentialRef != "" || len(provider.SelectedRoutes) != 0) {
		return ErrInvalidRegistry
	}
	if provider.Kind == PrivateAccountKind {
		if provider.PrivateAccount == nil || provider.PrivateAccount.Validate() != nil || provider.Proxy != "" ||
			len(provider.Models) != 0 || len(provider.MediaModels) != 0 || provider.SelectedModel != "" ||
			provider.SelectedMedia != "" || len(provider.SelectedRoutes) != 0 ||
			(provider.CredentialPurpose != "" && provider.CredentialPurpose != provider.PrivateAccount.Purpose) ||
			(provider.Tombstone && provider.PrivateAccountDisposition != "revoke" && provider.PrivateAccountDisposition != "disconnect") ||
			(!provider.Tombstone && provider.PrivateAccountDisposition != "") {
			return ErrInvalidRegistry
		}
	} else if provider.PrivateAccount != nil || provider.PrivateAccountDisposition != "" ||
		(provider.OAuthBinding != nil && provider.OAuthBinding.Validate() != nil) {
		return ErrInvalidRegistry
	}
	if provider.AccountObservation != nil && provider.AccountObservation.Validate() != nil {
		return ErrInvalidRegistry
	}
	if provider.Kind == PrivateAccountKind && (provider.OAuthBinding != nil || provider.AccountObservation != nil) {
		return ErrInvalidRegistry
	}
	return nil
}

func (binding OAuthBindingMetadata) Validate() error {
	if binding.SchemaVersion != 1 || binding.RedirectModeVersion != 1 ||
		!validOAuthEndpoint(binding.Issuer) || !validOAuthEndpoint(binding.AuthorizationEndpoint) ||
		!validOAuthEndpoint(binding.TokenEndpoint) ||
		(binding.RevocationEndpoint != "" && !validOAuthEndpoint(binding.RevocationEndpoint)) ||
		!validScopeComponent(binding.ClientID, 256) || len(binding.Scopes) == 0 ||
		len(binding.Scopes) > 32 || !validUniqueValues(binding.Scopes, 256) {
		return ErrInvalidRegistry
	}
	return nil
}

func (binding AccountObservationBinding) Validate() error {
	if binding.SchemaVersion != 1 || binding.Method != "GET" ||
		binding.Projection != "normalized-quota-v1" || !validOAuthEndpoint(binding.Endpoint) {
		return ErrInvalidRegistry
	}
	return nil
}

func validOAuthEndpoint(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n\t") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.RawPath != "" || strings.Contains(value, "\\") || strings.Contains(parsed.Path, "\\") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	loopback := host == "127.0.0.1" || host == "::1"
	if address, parseErr := netip.ParseAddr(host); parseErr == nil {
		if address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
			address.IsLoopback() && !loopback || address.IsUnspecified() {
			return false
		}
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http" && loopback
}

func (scope PrivateAccountScope) Validate() error {
	if scope.SchemaVersion != 1 ||
		(scope.Owner != "provider" && scope.Owner != "mcp" && scope.Owner != "extension" && scope.Owner != "transport") ||
		!validScopeComponent(scope.Provider, 128) || !validScopeComponent(scope.AccountID, 256) ||
		(scope.ChannelID != "" && !validScopeComponent(scope.ChannelID, 256)) || !validPurpose(scope.Purpose) {
		return ErrInvalidRegistry
	}
	coherent := false
	switch scope.Owner {
	case "provider":
		switch scope.Purpose {
		case "provider-oauth-token-bundle":
			coherent = scope.ChannelID == "" && scope.AccountID == scope.Provider && ValidProviderID(scope.Provider)
		case "provider-oauth-authorization-state":
			coherent = scope.ChannelID != ""
		}
	case "mcp":
		coherent = scope.ChannelID != "" &&
			(scope.Purpose == "mcp-oauth-access-token" || scope.Purpose == "mcp-oauth-authorization-state")
	case "extension":
		coherent = scope.ChannelID != "" && (scope.Purpose == "extension-provider-account-token" ||
			scope.Purpose == "extension-oauth-authorization-state")
	case "transport":
		coherent = scope.ChannelID != "" && ((scope.Provider == "telegram" && scope.Purpose == "transport-telegram-bot-token") ||
			(scope.Provider == "weixin" && (scope.Purpose == "transport-weixin-session-key" ||
				scope.Purpose == "transport-weixin-context-token")) ||
			(scope.Provider == "feishu" && scope.Purpose == "transport-feishu-app-secret"))
	}
	if !coherent {
		return ErrInvalidRegistry
	}
	return nil
}

func (transaction Transaction) Validate() error {
	if transaction.DeferSelection && transaction.Operation != OperationConnect {
		return ErrInvalidRegistry
	}
	if transaction.Version != TransactionVersion || !ValidTransactionID(transaction.ID) ||
		!validOperation(transaction.Operation) || !validPhase(transaction.Phase) ||
		!validIdentifier(transaction.ProviderID, 96) || transaction.Fence.Validate() != nil ||
		transaction.Recovery.Version != RecoveryRecordVersion || transaction.Recovery.LastObserved != transaction.Phase {
		return ErrInvalidRegistry
	}
	if transaction.CandidateCredentialPurpose != "" && !validPurpose(transaction.CandidateCredentialPurpose) {
		return ErrInvalidRegistry
	}
	if transaction.SupersededCredentialPurpose != "" && !validPurpose(transaction.SupersededCredentialPurpose) {
		return ErrInvalidRegistry
	}
	if transaction.CandidateCredentialRef != "" && !ValidCredentialRef(transaction.CandidateCredentialRef) {
		return ErrInvalidRegistry
	}
	if transaction.SupersededCredentialRef != "" && !ValidCredentialRef(transaction.SupersededCredentialRef) {
		return ErrInvalidRegistry
	}
	if transaction.PriorProvider != nil && transaction.PriorProvider.Validate() != nil {
		return ErrInvalidRegistry
	}
	if transaction.NextProvider != nil && transaction.NextProvider.Validate() != nil {
		return ErrInvalidRegistry
	}
	if transaction.ResolvedProvider != nil && transaction.ResolvedProvider.Validate() != nil {
		return ErrInvalidRegistry
	}
	if (transaction.CandidateCredentialRef == "") != (transaction.CandidateCredentialPurpose == "") {
		return ErrInvalidRegistry
	}
	if (transaction.SupersededCredentialRef == "") != (transaction.SupersededCredentialPurpose == "") {
		return ErrInvalidRegistry
	}
	if (transaction.CleanupCredentialRef == "") != (transaction.CleanupCredentialPurpose == "") ||
		(transaction.CleanupCredentialRef != "" && (!ValidCredentialRef(transaction.CleanupCredentialRef) ||
			!validPurpose(transaction.CleanupCredentialPurpose))) {
		return ErrInvalidRegistry
	}
	if transaction.CandidateCredentialRef != "" && transaction.CandidateCredentialRef == transaction.SupersededCredentialRef {
		return ErrInvalidRegistry
	}
	if transaction.PriorProvider != nil && transaction.PriorProvider.ID != transaction.ProviderID {
		return ErrInvalidRegistry
	}
	if transaction.NextProvider != nil && transaction.NextProvider.ID != transaction.ProviderID {
		return ErrInvalidRegistry
	}
	if transaction.CandidateCredentialRef != "" &&
		(transaction.NextProvider == nil || transaction.NextProvider.CredentialRef != transaction.CandidateCredentialRef) {
		return ErrInvalidRegistry
	}
	if validateOperationShape(transaction) != nil {
		return ErrInvalidRegistry
	}
	switch transaction.Phase {
	case PhasePrepared, PhaseCandidateDurable:
		if transaction.Resolution != ResolutionPending || transaction.CommittedRegistryRevision != 0 ||
			transaction.ResolvedProvider != nil || transaction.ResolvedSelectedProviderID != "" ||
			transaction.CleanupCredentialRef != "" {
			return ErrInvalidRegistry
		}
	case PhaseMetadataCommitted:
		if transaction.Resolution != ResolutionWinner || transaction.CommittedRegistryRevision == 0 {
			return ErrInvalidRegistry
		}
	case PhaseVerified:
		if !validCommittedResolution(transaction.Resolution) || transaction.CommittedRegistryRevision == 0 {
			return ErrInvalidRegistry
		}
	case PhaseSupersededTombstoned, PhaseSupersededDeleted:
		if !validCommittedResolution(transaction.Resolution) || transaction.CommittedRegistryRevision == 0 ||
			transaction.CleanupCredentialRef == "" {
			return ErrInvalidRegistry
		}
	}
	if transaction.Resolution != ResolutionPending {
		if transaction.ResolvedProvider != nil && transaction.ResolvedProvider.ID != transaction.ProviderID {
			return ErrInvalidRegistry
		}
		switch transaction.Resolution {
		case ResolutionWinner:
			if !providerPointersEqual(transaction.ResolvedProvider, transaction.NextProvider) ||
				transaction.ResolvedSelectedProviderID != transaction.NextSelectedProviderID ||
				transaction.CleanupCredentialRef != transaction.SupersededCredentialRef ||
				transaction.CleanupCredentialPurpose != transaction.SupersededCredentialPurpose {
				return ErrInvalidRegistry
			}
		case ResolutionRollback:
			if (transaction.CandidateCredentialRef == "" &&
				transaction.Operation != OperationUpdate && transaction.Operation != OperationSelect) ||
				!providerPointersEqual(transaction.ResolvedProvider, transaction.PriorProvider) ||
				transaction.ResolvedSelectedProviderID != transaction.Fence.SelectedProviderID ||
				transaction.CleanupCredentialRef != transaction.CandidateCredentialRef ||
				transaction.CleanupCredentialPurpose != transaction.CandidateCredentialPurpose {
				return ErrInvalidRegistry
			}
		case ResolutionAbandoned:
			if transaction.CandidateCredentialRef == "" || transaction.CleanupCredentialRef != transaction.CandidateCredentialRef ||
				transaction.CleanupCredentialPurpose != transaction.CandidateCredentialPurpose {
				return ErrInvalidRegistry
			}
		}
	}
	return nil
}

func (recovery LegacyMigrationRecovery) Validate() error {
	if recovery.Version != LegacyMigrationRecoveryVersion || !ValidLegacyMigrationID(recovery.ID) ||
		!validLegacyMigrationRecoveryPhase(recovery.Phase) || !validIdentifier(recovery.ProviderID, 96) ||
		recovery.Provider.ID != recovery.ProviderID || recovery.Provider.Validate() != nil ||
		!validPurpose(recovery.CredentialPurpose) || recovery.RecoveryPurpose != LegacyMigrationRecoveryPurpose ||
		recovery.Fence.Validate() != nil {
		return ErrInvalidRegistry
	}
	if recovery.SourceIdentitySHA256 != "" && !validLowerSHA256(recovery.SourceIdentitySHA256) {
		return ErrInvalidRegistry
	}
	if recovery.ExpectedCleanedSourceIdentitySHA256 != "" &&
		(!validLowerSHA256(recovery.ExpectedCleanedSourceIdentitySHA256) || recovery.SourceIdentitySHA256 == "") {
		return ErrInvalidRegistry
	}
	if recovery.PriorSelectedProviderIdentitySHA256 != "" &&
		(recovery.Fence.SelectedProviderID == "" ||
			!validLowerSHA256(recovery.PriorSelectedProviderIdentitySHA256)) {
		return ErrInvalidRegistry
	}
	sourceMetadataPresent := recovery.SourceLocator != "" || recovery.SourceSHA256 != "" ||
		recovery.ExpectedCleanedSourceSHA256 != "" || recovery.SourcePhysicalIdentitySHA256 != ""
	if sourceMetadataPresent && (len(recovery.SourceLocator) > 256 || recovery.SourceLocator == "" ||
		!validLowerSHA256(recovery.SourceSHA256) ||
		!validLowerSHA256(recovery.ExpectedCleanedSourceSHA256) ||
		!validLowerSHA256(recovery.SourcePhysicalIdentitySHA256) ||
		recovery.SourceIdentitySHA256 == "" || recovery.ExpectedCleanedSourceIdentitySHA256 == "") {
		return ErrInvalidRegistry
	}
	if recovery.RollbackCleanedSourceStateIdentitySHA256 != "" &&
		!validLowerSHA256(recovery.RollbackCleanedSourceStateIdentitySHA256) {
		return ErrInvalidRegistry
	}
	if (recovery.RollbackCleanedSourceGenerationSHA256 != "" &&
		!validLowerSHA256(recovery.RollbackCleanedSourceGenerationSHA256)) ||
		(recovery.FinalizedSourceGenerationSHA256 != "" &&
			!validLowerSHA256(recovery.FinalizedSourceGenerationSHA256)) ||
		(recovery.PendingSourceAuthorityIntentSHA256 != "" &&
			!validLowerSHA256(recovery.PendingSourceAuthorityIntentSHA256)) {
		return ErrInvalidRegistry
	}
	if recovery.Phase == LegacyMigrationRecoveryPhaseFinalized {
		if recovery.RecoveryCredentialRef != "" || recovery.CommittedProviderCredentialRef != "" ||
			recovery.CommittedProviderCredentialPurpose != "" || recovery.CommittedProviderRevision != 0 ||
			recovery.CommittedProviderGeneration != 0 || recovery.CommittedProviderIncarnation != "" ||
			recovery.RollbackRegistryRevision != 0 || recovery.RollbackRegistryIncarnation != "" ||
			recovery.RollbackSelectedProviderID != "" ||
			recovery.FinalizationRecoveryDeleteAuthorized || recovery.FinalizationWinnerDeleteAuthorized ||
			recovery.PendingSourceAuthorityIntentSHA256 != "" || recovery.RemigrationPending ||
			recovery.ProtectedRecoveryRetained ||
			(recovery.FinalizationOutcome != LegacyMigrationFinalizationOutcomeCommitted &&
				recovery.FinalizationOutcome != LegacyMigrationFinalizationOutcomeRolledBack) ||
			!validLowerSHA256(recovery.FinalizedSourceIdentitySHA256) ||
			(recovery.FinalizedSourceStateIdentitySHA256 != "" &&
				!validLowerSHA256(recovery.FinalizedSourceStateIdentitySHA256)) ||
			(recovery.ExpectedCleanedSourceIdentitySHA256 != "" &&
				recovery.FinalizedSourceStateIdentitySHA256 == "") ||
			(sourceMetadataPresent && recovery.FinalizedSourceGenerationSHA256 == "" &&
				!recovery.FinalizationPreCommit) ||
			(recovery.SourceIdentitySHA256 != "" &&
				recovery.SourceIdentitySHA256 != recovery.FinalizedSourceIdentitySHA256) {
			return ErrInvalidRegistry
		}
		if recovery.FinalizationPreCommit &&
			(recovery.FinalizationOutcome != LegacyMigrationFinalizationOutcomeRolledBack ||
				recovery.RollbackCleanedSourceStateIdentitySHA256 != "" ||
				recovery.RollbackCleanedSourceGenerationSHA256 != "" ||
				recovery.FinalizedSourceGenerationSHA256 != "") {
			return ErrInvalidRegistry
		}
		if (recovery.FinalizationOutcome == LegacyMigrationFinalizationOutcomeCommitted &&
			(recovery.RollbackCleanedSourceStateIdentitySHA256 != "" ||
				recovery.RollbackCleanedSourceGenerationSHA256 != "" || recovery.FinalizationPreCommit)) ||
			(recovery.FinalizationOutcome == LegacyMigrationFinalizationOutcomeRolledBack &&
				recovery.ExpectedCleanedSourceIdentitySHA256 != "" && !recovery.FinalizationPreCommit &&
				(recovery.RollbackCleanedSourceStateIdentitySHA256 != recovery.FinalizedSourceStateIdentitySHA256 ||
					recovery.RollbackCleanedSourceGenerationSHA256 != recovery.FinalizedSourceGenerationSHA256)) {
			return ErrInvalidRegistry
		}
		return nil
	}
	if recovery.RemigrationPending {
		if (recovery.Phase != LegacyMigrationRecoveryPhaseVerified &&
			recovery.Phase != LegacyMigrationRecoveryPhaseProviderCommitPrepared) ||
			!ValidCredentialRef(recovery.RecoveryCredentialRef) || recovery.Fence.ProviderExists ||
			recovery.CommittedProviderCredentialRef != "" || recovery.CommittedProviderCredentialPurpose != "" ||
			recovery.CommittedProviderRevision != 0 || recovery.CommittedProviderGeneration != 0 ||
			recovery.CommittedProviderIncarnation != "" || recovery.RollbackRegistryRevision == 0 ||
			!ValidIncarnation(recovery.RollbackRegistryIncarnation) ||
			recovery.RollbackRegistryRevision != recovery.Fence.RegistryRevision ||
			recovery.RollbackRegistryIncarnation != recovery.Fence.RegistryIncarnation ||
			recovery.RollbackSelectedProviderID != recovery.Fence.SelectedProviderID ||
			!validLowerSHA256(recovery.RollbackCleanedSourceStateIdentitySHA256) ||
			!validLowerSHA256(recovery.RollbackCleanedSourceGenerationSHA256) ||
			recovery.FinalizationOutcome != LegacyMigrationFinalizationOutcomeRolledBack ||
			recovery.FinalizationPreCommit ||
			recovery.FinalizedSourceIdentitySHA256 != recovery.SourceIdentitySHA256 ||
			recovery.FinalizedSourceStateIdentitySHA256 != recovery.RollbackCleanedSourceStateIdentitySHA256 ||
			recovery.FinalizedSourceGenerationSHA256 != recovery.RollbackCleanedSourceGenerationSHA256 ||
			!validLowerSHA256(recovery.PendingSourceAuthorityIntentSHA256) ||
			!recovery.ProtectedRecoveryRetained || recovery.FinalizationRecoveryDeleteAuthorized ||
			recovery.FinalizationWinnerDeleteAuthorized {
			return ErrInvalidRegistry
		}
		return nil
	}
	if recovery.Phase == LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending ||
		recovery.Phase == LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
		deleted := recovery.Phase == LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted
		if (deleted && recovery.RecoveryCredentialRef != "") ||
			(!deleted && !ValidCredentialRef(recovery.RecoveryCredentialRef)) ||
			recovery.CommittedProviderCredentialRef != "" || recovery.CommittedProviderCredentialPurpose != "" ||
			recovery.CommittedProviderRevision != 0 || recovery.CommittedProviderGeneration != 0 ||
			recovery.CommittedProviderIncarnation != "" || recovery.RollbackRegistryRevision == 0 ||
			!ValidIncarnation(recovery.RollbackRegistryIncarnation) ||
			(recovery.RollbackSelectedProviderID != "" && !ValidProviderID(recovery.RollbackSelectedProviderID)) ||
			!validLowerSHA256(recovery.RollbackCleanedSourceStateIdentitySHA256) ||
			!validLowerSHA256(recovery.RollbackCleanedSourceGenerationSHA256) ||
			recovery.FinalizationOutcome != LegacyMigrationFinalizationOutcomeRolledBack ||
			recovery.FinalizationPreCommit ||
			recovery.FinalizedSourceIdentitySHA256 != recovery.SourceIdentitySHA256 ||
			recovery.FinalizedSourceStateIdentitySHA256 != recovery.RollbackCleanedSourceStateIdentitySHA256 ||
			recovery.FinalizedSourceGenerationSHA256 != recovery.RollbackCleanedSourceGenerationSHA256 ||
			recovery.FinalizationWinnerDeleteAuthorized ||
			(deleted && recovery.ProtectedRecoveryRetained) ||
			(!deleted && !recovery.ProtectedRecoveryRetained) ||
			(deleted && (recovery.PendingSourceAuthorityIntentSHA256 != "" ||
				recovery.FinalizationRecoveryDeleteAuthorized)) ||
			(!deleted && (!validLowerSHA256(recovery.PendingSourceAuthorityIntentSHA256) ||
				!recovery.FinalizationRecoveryDeleteAuthorized)) {
			return ErrInvalidRegistry
		}
		return nil
	}
	if recovery.Phase == LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
		if !ValidCredentialRef(recovery.RecoveryCredentialRef) ||
			recovery.CommittedProviderCredentialRef != "" || recovery.CommittedProviderCredentialPurpose != "" ||
			recovery.CommittedProviderRevision != 0 || recovery.CommittedProviderGeneration != 0 ||
			recovery.CommittedProviderIncarnation != "" || recovery.RollbackRegistryRevision == 0 ||
			!ValidIncarnation(recovery.RollbackRegistryIncarnation) ||
			(recovery.RollbackSelectedProviderID != "" && !ValidProviderID(recovery.RollbackSelectedProviderID)) ||
			!validLowerSHA256(recovery.RollbackCleanedSourceStateIdentitySHA256) ||
			!validLowerSHA256(recovery.RollbackCleanedSourceGenerationSHA256) ||
			recovery.FinalizationOutcome != LegacyMigrationFinalizationOutcomeRolledBack ||
			recovery.FinalizationPreCommit ||
			recovery.FinalizedSourceIdentitySHA256 != recovery.SourceIdentitySHA256 ||
			recovery.FinalizedSourceStateIdentitySHA256 != recovery.RollbackCleanedSourceStateIdentitySHA256 ||
			recovery.FinalizedSourceGenerationSHA256 != recovery.RollbackCleanedSourceGenerationSHA256 ||
			recovery.PendingSourceAuthorityIntentSHA256 != "" || recovery.RemigrationPending ||
			!recovery.ProtectedRecoveryRetained ||
			recovery.FinalizationRecoveryDeleteAuthorized || recovery.FinalizationWinnerDeleteAuthorized {
			return ErrInvalidRegistry
		}
		return nil
	}
	if !ValidCredentialRef(recovery.RecoveryCredentialRef) {
		return ErrInvalidRegistry
	}
	committed := recovery.Phase == LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained ||
		recovery.Phase == LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending ||
		recovery.Phase == LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending ||
		recovery.Phase == LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained ||
		recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingCommitted ||
		recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingRollback
	if committed {
		if !ValidCredentialRef(recovery.CommittedProviderCredentialRef) ||
			recovery.CommittedProviderCredentialRef == recovery.RecoveryCredentialRef ||
			recovery.CommittedProviderCredentialPurpose != recovery.CredentialPurpose ||
			recovery.CommittedProviderRevision != 1 || recovery.CommittedProviderGeneration != 1 ||
			!ValidIncarnation(recovery.CommittedProviderIncarnation) {
			return ErrInvalidRegistry
		}
	} else if recovery.CommittedProviderCredentialRef != "" ||
		recovery.CommittedProviderCredentialPurpose != "" || recovery.CommittedProviderRevision != 0 ||
		recovery.CommittedProviderGeneration != 0 || recovery.CommittedProviderIncarnation != "" {
		return ErrInvalidRegistry
	}
	if recovery.ProtectedRecoveryRetained {
		switch recovery.Phase {
		case LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained,
			LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending,
			LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending,
			LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained,
			LegacyMigrationRecoveryPhaseFinalizingRollback:
		default:
			return ErrInvalidRegistry
		}
	}
	rollbackIntent := recovery.Phase == LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending
	rolledBack := recovery.Phase == LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained ||
		recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingRollback
	if rollbackIntent {
		if recovery.RollbackRegistryRevision != 0 || recovery.RollbackRegistryIncarnation != "" ||
			recovery.RollbackSelectedProviderID != "" ||
			!validLowerSHA256(recovery.RollbackCleanedSourceStateIdentitySHA256) ||
			!validLowerSHA256(recovery.RollbackCleanedSourceGenerationSHA256) ||
			!validLowerSHA256(recovery.PendingSourceAuthorityIntentSHA256) {
			return ErrInvalidRegistry
		}
	}
	if rolledBack {
		if recovery.RollbackRegistryRevision == 0 ||
			!ValidIncarnation(recovery.RollbackRegistryIncarnation) ||
			(recovery.RollbackSelectedProviderID != "" && !ValidProviderID(recovery.RollbackSelectedProviderID)) {
			return ErrInvalidRegistry
		}
		if recovery.ExpectedCleanedSourceIdentitySHA256 != "" &&
			(recovery.RollbackCleanedSourceStateIdentitySHA256 == "" ||
				(sourceMetadataPresent && recovery.RollbackCleanedSourceGenerationSHA256 == "")) {
			return ErrInvalidRegistry
		}
	} else if !rollbackIntent && (recovery.RollbackRegistryRevision != 0 || recovery.RollbackRegistryIncarnation != "" ||
		recovery.RollbackSelectedProviderID != "" || recovery.RollbackCleanedSourceStateIdentitySHA256 != "" ||
		recovery.RollbackCleanedSourceGenerationSHA256 != "") {
		return ErrInvalidRegistry
	}
	finalizing := recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingPreCommit ||
		recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingCommitted ||
		recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingRollback
	if finalizing {
		wantOutcome := LegacyMigrationFinalizationOutcomeCommitted
		if recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingPreCommit ||
			recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingRollback {
			wantOutcome = LegacyMigrationFinalizationOutcomeRolledBack
		}
		if recovery.FinalizationOutcome != wantOutcome ||
			recovery.FinalizationPreCommit !=
				(recovery.Phase == LegacyMigrationRecoveryPhaseFinalizingPreCommit) ||
			!validLowerSHA256(recovery.FinalizedSourceIdentitySHA256) ||
			(recovery.FinalizedSourceStateIdentitySHA256 != "" &&
				!validLowerSHA256(recovery.FinalizedSourceStateIdentitySHA256)) ||
			(recovery.ExpectedCleanedSourceIdentitySHA256 != "" &&
				recovery.FinalizedSourceStateIdentitySHA256 == "") ||
			(sourceMetadataPresent && recovery.Phase != LegacyMigrationRecoveryPhaseFinalizingPreCommit &&
				(recovery.FinalizedSourceGenerationSHA256 == "" ||
					recovery.PendingSourceAuthorityIntentSHA256 == "")) ||
			(recovery.SourceIdentitySHA256 != "" &&
				recovery.SourceIdentitySHA256 != recovery.FinalizedSourceIdentitySHA256) {
			return ErrInvalidRegistry
		}
		if recovery.FinalizationWinnerDeleteAuthorized &&
			recovery.Phase != LegacyMigrationRecoveryPhaseFinalizingRollback {
			return ErrInvalidRegistry
		}
	} else if recovery.FinalizationOutcome != "" || recovery.FinalizationPreCommit ||
		recovery.FinalizedSourceIdentitySHA256 != "" ||
		recovery.FinalizedSourceStateIdentitySHA256 != "" ||
		recovery.FinalizedSourceGenerationSHA256 != "" ||
		(!rollbackIntent && recovery.PendingSourceAuthorityIntentSHA256 != "") {
		return ErrInvalidRegistry
	} else if recovery.FinalizationRecoveryDeleteAuthorized || recovery.FinalizationWinnerDeleteAuthorized {
		return ErrInvalidRegistry
	}
	return nil
}

func (fence LegacyMigrationRecoveryFence) Validate() error {
	if !ValidIncarnation(fence.RegistryIncarnation) ||
		(fence.SelectedProviderID != "" && !validIdentifier(fence.SelectedProviderID, 96)) {
		return ErrInvalidRegistry
	}
	if !fence.ProviderExists {
		if fence.ProviderRevision != 0 || fence.ProviderGeneration != 0 || fence.ProviderIncarnation != "" ||
			fence.ProviderCredentialRef != "" || fence.ProviderCredentialPurpose != "" {
			return ErrInvalidRegistry
		}
		return nil
	}
	if fence.ProviderRevision == 0 || fence.ProviderGeneration == 0 || !ValidIncarnation(fence.ProviderIncarnation) ||
		(fence.ProviderCredentialRef == "") != (fence.ProviderCredentialPurpose == "") ||
		(fence.ProviderCredentialRef != "" && !ValidCredentialRef(fence.ProviderCredentialRef)) ||
		(fence.ProviderCredentialPurpose != "" && !validPurpose(fence.ProviderCredentialPurpose)) {
		return ErrInvalidRegistry
	}
	return nil
}

func validCommittedResolution(resolution Resolution) bool {
	switch resolution {
	case ResolutionWinner, ResolutionRollback, ResolutionAbandoned:
		return true
	default:
		return false
	}
}

func providerPointersEqual(left, right *Provider) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return reflect.DeepEqual(left.Clone(), right.Clone())
}

func validateOperationShape(transaction Transaction) error {
	prior := transaction.PriorProvider
	next := transaction.NextProvider
	if prior == nil {
		if transaction.Operation != OperationConnect || transaction.Fence.ProviderRevision != 0 ||
			transaction.Fence.ProviderGeneration != 0 || transaction.Fence.ProviderIncarnation != "" ||
			transaction.Fence.CurrentCredentialRef != "" || transaction.Fence.CurrentCredentialPurpose != "" {
			return ErrInvalidRegistry
		}
	} else if transaction.Fence.ProviderRevision != prior.Revision ||
		transaction.Fence.ProviderGeneration != prior.Generation ||
		transaction.Fence.ProviderIncarnation != prior.Incarnation ||
		transaction.Fence.CurrentCredentialRef != prior.CredentialRef ||
		transaction.Fence.CurrentCredentialPurpose != prior.CredentialPurpose {
		return ErrInvalidRegistry
	}
	if prior != nil && prior.Tombstone && transaction.Operation != OperationExplicitDelete {
		return ErrInvalidRegistry
	}

	switch transaction.Operation {
	case OperationConnect:
		if prior != nil || next == nil || next.Tombstone || transaction.CandidateCredentialRef == "" ||
			transaction.SupersededCredentialRef != "" || next.CredentialRef != transaction.CandidateCredentialRef ||
			next.CredentialPurpose != transaction.CandidateCredentialPurpose || next.Revision != 1 || next.Generation != 1 {
			return ErrInvalidRegistry
		}
		selected := transaction.Fence.SelectedProviderID
		if selected == "" && next.PrivateAccount == nil && !transaction.DeferSelection {
			selected = transaction.ProviderID
		}
		if transaction.NextSelectedProviderID != selected {
			return ErrInvalidRegistry
		}
	case OperationUpdate:
		if !validProviderSuccessor(prior, next) || next.Tombstone || transaction.NextSelectedProviderID != transaction.Fence.SelectedProviderID {
			return ErrInvalidRegistry
		}
		if transaction.CandidateCredentialRef == "" {
			bindingChanged := !reflect.DeepEqual(prior.OAuthBinding, next.OAuthBinding)
			if bindingChanged {
				deletingBoundCredential := transaction.SupersededCredentialRef != ""
				if next.Generation != prior.Generation+1 ||
					(deletingBoundCredential && (next.CredentialRef != "" || next.CredentialPurpose != "" ||
						transaction.SupersededCredentialRef != prior.CredentialRef ||
						transaction.SupersededCredentialPurpose != prior.CredentialPurpose)) ||
					(!deletingBoundCredential && (next.CredentialRef != prior.CredentialRef ||
						next.CredentialPurpose != prior.CredentialPurpose || transaction.SupersededCredentialPurpose != "")) {
					return ErrInvalidRegistry
				}
			} else if transaction.SupersededCredentialRef != "" || next.CredentialRef != prior.CredentialRef ||
				next.CredentialPurpose != prior.CredentialPurpose || next.Generation != prior.Generation {
				return ErrInvalidRegistry
			}
		} else if !validCredentialSuccessor(prior, next, transaction) {
			return ErrInvalidRegistry
		}
	case OperationSelect:
		if transaction.CandidateCredentialRef != "" || transaction.SupersededCredentialRef != "" ||
			!validExactSuccessor(prior, next, false, false) || transaction.NextSelectedProviderID != transaction.ProviderID {
			return ErrInvalidRegistry
		}
	case OperationCredentialReplace:
		if transaction.CandidateCredentialRef == "" || !validCredentialSuccessor(prior, next, transaction) ||
			!validExactSuccessor(prior, next, true, false) || transaction.NextSelectedProviderID != transaction.Fence.SelectedProviderID {
			return ErrInvalidRegistry
		}
	case OperationDisconnect:
		if transaction.CandidateCredentialRef != "" || prior == nil || next == nil || prior.Tombstone || !next.Tombstone ||
			transaction.SupersededCredentialRef != prior.CredentialRef ||
			transaction.SupersededCredentialPurpose != prior.CredentialPurpose ||
			!validExactSuccessor(prior, next, false, true) {
			return ErrInvalidRegistry
		}
		selected := transaction.Fence.SelectedProviderID
		if selected == transaction.ProviderID {
			selected = ""
		}
		if transaction.NextSelectedProviderID != selected {
			return ErrInvalidRegistry
		}
	case OperationExplicitDelete:
		if prior == nil || next != nil || transaction.CandidateCredentialRef != "" ||
			transaction.SupersededCredentialRef != prior.CredentialRef ||
			transaction.SupersededCredentialPurpose != prior.CredentialPurpose {
			return ErrInvalidRegistry
		}
		selected := transaction.Fence.SelectedProviderID
		if selected == transaction.ProviderID {
			selected = ""
		}
		if transaction.NextSelectedProviderID != selected {
			return ErrInvalidRegistry
		}
	default:
		return ErrInvalidRegistry
	}
	return nil
}

func validProviderSuccessor(prior, next *Provider) bool {
	return prior != nil && next != nil && prior.Revision != math.MaxUint64 &&
		next.ID == prior.ID && next.Kind == prior.Kind && next.Incarnation == prior.Incarnation &&
		next.Revision == prior.Revision+1
}

func validCredentialSuccessor(prior, next *Provider, transaction Transaction) bool {
	return validProviderSuccessor(prior, next) && prior.Generation != math.MaxUint64 &&
		transaction.SupersededCredentialRef == prior.CredentialRef &&
		transaction.SupersededCredentialPurpose == prior.CredentialPurpose &&
		next.CredentialRef == transaction.CandidateCredentialRef &&
		next.CredentialPurpose == transaction.CandidateCredentialPurpose && next.Generation == prior.Generation+1
}

func validExactSuccessor(prior, next *Provider, credentialChanged, disconnected bool) bool {
	if !validProviderSuccessor(prior, next) {
		return false
	}
	expected := prior.Clone()
	expected.Revision++
	if credentialChanged {
		expected.CredentialRef = next.CredentialRef
		expected.CredentialPurpose = next.CredentialPurpose
		if expected.Generation == math.MaxUint64 {
			return false
		}
		expected.Generation++
	}
	if disconnected {
		expected.CredentialRef = ""
		expected.CredentialPurpose = ""
		if expected.Generation == math.MaxUint64 {
			return false
		}
		expected.Generation++
		expected.Tombstone = true
		expected.SelectedRoutes = nil
		if expected.PrivateAccount != nil {
			expected.PrivateAccountDisposition = next.PrivateAccountDisposition
		}
	}
	return reflect.DeepEqual(expected, next.Clone())
}

func (fence Fence) Validate() error {
	if !ValidIncarnation(fence.RegistryIncarnation) {
		return ErrInvalidRegistry
	}
	if fence.ProviderRevision == 0 || fence.ProviderGeneration == 0 || fence.ProviderIncarnation == "" {
		if fence.ProviderRevision != 0 || fence.ProviderGeneration != 0 || fence.ProviderIncarnation != "" || fence.CurrentCredentialRef != "" {
			return ErrInvalidRegistry
		}
	} else if !ValidIncarnation(fence.ProviderIncarnation) {
		return ErrInvalidRegistry
	}
	if fence.CurrentCredentialRef != "" && !ValidCredentialRef(fence.CurrentCredentialRef) {
		return ErrInvalidRegistry
	}
	if (fence.CurrentCredentialRef == "") != (fence.CurrentCredentialPurpose == "") ||
		(fence.CurrentCredentialPurpose != "" && !validPurpose(fence.CurrentCredentialPurpose)) {
		return ErrInvalidRegistry
	}
	if fence.SelectedProviderID != "" && !validIdentifier(fence.SelectedProviderID, 96) {
		return ErrInvalidRegistry
	}
	return nil
}

func Marshal(registry Registry) ([]byte, error) {
	if err := registry.Validate(); err != nil {
		return nil, ErrInvalidRegistry
	}
	content, err := json.Marshal(registry)
	if err != nil {
		return nil, ErrInvalidRegistry
	}
	if len(content)+1 > MaxRegistryBytes {
		return nil, ErrInvalidRegistry
	}
	return append(content, '\n'), nil
}

func Unmarshal(content []byte) (Registry, error) {
	if len(content) == 0 || len(content) > MaxRegistryBytes {
		return Registry{}, ErrInvalidRegistry
	}
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(content, &rawFields); err != nil {
		return Registry{}, ErrInvalidRegistry
	}
	if raw, exists := rawFields["legacyMigrationRecoveries"]; exists &&
		bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return Registry{}, ErrInvalidRegistry
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var registry Registry
	if err := decoder.Decode(&registry); err != nil {
		return Registry{}, ErrInvalidRegistry
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Registry{}, ErrInvalidRegistry
	}
	if registry.LegacyMigrationRecoveries == nil {
		registry.LegacyMigrationRecoveries = make(map[string]LegacyMigrationRecovery)
	}
	if err := registry.Validate(); err != nil {
		return Registry{}, ErrInvalidRegistry
	}
	return registry, nil
}

func ValidIncarnation(value string) bool { return validOpaqueID(value, "inc_") }

func ValidTransactionID(value string) bool { return validOpaqueID(value, "txn_") }

func ValidLegacyMigrationID(value string) bool { return validIdentifier(value, 96) }

func ValidCredentialRef(value string) bool { return validOpaqueID(value, "cred_") }

func ValidProviderID(value string) bool { return validIdentifier(value, 96) }

func validOpaqueID(value, prefix string) bool {
	if len(value) != len(prefix)+43 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, current := range []byte(value[len(prefix):]) {
		if (current >= 'a' && current <= 'z') || (current >= 'A' && current <= 'Z') ||
			(current >= '0' && current <= '9') || current == '-' || current == '_' {
			continue
		}
		return false
	}
	return true
}

func validIdentifier(value string, maximum int) bool {
	if value == "" || len(value) > maximum || value != strings.TrimSpace(value) {
		return false
	}
	for index, current := range []byte(value) {
		if (current >= 'a' && current <= 'z') || (current >= '0' && current <= '9') {
			continue
		}
		if index > 0 && (current == '-' || current == '_' || current == '.') {
			continue
		}
		return false
	}
	return true
}

func validScopeComponent(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) &&
		utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n\t")
}

func validEndpoint(value string, optional bool) bool {
	if value == "" {
		return optional
	}
	if len(value) > 2048 || value != strings.TrimSpace(value) || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n\t") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return false
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return false
	}
	return true
}

func validUniqueValues(values []string, maximumLength int) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || len(value) > maximumLength || value != strings.TrimSpace(value) ||
			!utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n\t") {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validPurpose(value string) bool {
	if value == "" || len(value) > 96 || value != strings.ToLower(strings.TrimSpace(value)) {
		return false
	}
	for index, current := range []byte(value) {
		if (current >= 'a' && current <= 'z') || (current >= '0' && current <= '9') {
			continue
		}
		if index > 0 && (current == '-' || current == '_' || current == '.' || current == ':' || current == '/') {
			continue
		}
		return false
	}
	return true
}

func validOperation(operation Operation) bool {
	switch operation {
	case OperationConnect, OperationUpdate, OperationSelect, OperationCredentialReplace, OperationDisconnect, OperationExplicitDelete:
		return true
	default:
		return false
	}
}

func validPhase(phase Phase) bool {
	switch phase {
	case PhasePrepared, PhaseCandidateDurable, PhaseMetadataCommitted, PhaseVerified, PhaseSupersededTombstoned, PhaseSupersededDeleted:
		return true
	default:
		return false
	}
}

func validLegacyMigrationRecoveryPhase(phase LegacyMigrationRecoveryPhase) bool {
	switch phase {
	case LegacyMigrationRecoveryPhasePrepared, LegacyMigrationRecoveryPhaseSecretDurable,
		LegacyMigrationRecoveryPhaseVerified, LegacyMigrationRecoveryPhaseProviderCommitPrepared,
		LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained,
		LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending,
		LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending,
		LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained,
		LegacyMigrationRecoveryPhaseRollbackRecoveryRetained,
		LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending,
		LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted,
		LegacyMigrationRecoveryPhaseFinalizingPreCommit,
		LegacyMigrationRecoveryPhaseFinalizingCommitted,
		LegacyMigrationRecoveryPhaseFinalizingRollback,
		LegacyMigrationRecoveryPhaseFinalized,
		LegacyMigrationRecoveryPhaseAbandoning:
		return true
	default:
		return false
	}
}

func validLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, current := range value {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}
