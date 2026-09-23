package providerregistry

import (
	"context"
	"errors"
	"sync"
	"time"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	RegistryReadbackConsumer                                       = "provider-registry-manager"
	ProviderExecutionConsumer                                      = "provider-runtime-execution"
	LegacyMigrationRecoveryPurpose                                 = secretstoreport.Purpose(domainregistry.LegacyMigrationRecoveryPurpose)
	LegacyMigrationRecoveryStatusVerified                          = LegacyMigrationRecoveryStatus("VERIFIED_RECOVERY")
	LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained = LegacyMigrationRecoveryStatus("PROVIDER_COMMITTED_RECOVERY_RETAINED")
	LegacyMigrationAbandonConfirmation                             = "ABANDON_PROVIDER_SETTINGS_MIGRATION_RECOVERY"
	LegacyMigrationCommitConfirmation                              = "COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY"
	LegacyMigrationRollbackConfirmation                            = "BEGIN_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK"
	LegacyMigrationRollbackCommitConfirmation                      = "COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK"
	LegacyMigrationFinalizeConfirmation                            = "FINALIZE_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY"
	LegacyMigrationRemigrationConfirmation                         = "REMIGRATE_RETAINED_PROVIDER_SETTINGS_MIGRATION_RECOVERY"
	LegacyMigrationProtectedDeleteConfirmation                     = "DELETE_RETAINED_PROVIDER_SETTINGS_MIGRATION_RECOVERY"
	LegacyMigrationRecoveryInventoryConfirmation                   = "INSPECT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERIES"
	LegacyMigrationSourceAuthorityChallengeConfirmation            = "ISSUE_PROVIDER_SETTINGS_SOURCE_AUTHORITY_CHALLENGE"
	LegacyMigrationSourceAuthorityOperationRollbackCommit          = "rollback-commit"
	LegacyMigrationSourceAuthorityOperationFinalize                = "finalize"
	LegacyMigrationSourceAuthorityOperationRemigrate               = "remigrate"
	LegacyMigrationSourceAuthorityOperationProtectedDelete         = "protected-delete"
)

var ErrInterrupted = errors.New("provider registry: interrupted")

type FaultPoint string

const (
	FaultAfterTransactionPrepared                        FaultPoint = "after-transaction-prepared"
	FaultAfterCandidateDurable                           FaultPoint = "after-candidate-durable"
	FaultAfterCandidateDurableRecorded                   FaultPoint = "after-candidate-durable-recorded"
	FaultAfterMetadataCommitted                          FaultPoint = "after-metadata-committed"
	FaultAfterReadbackVerified                           FaultPoint = "after-readback-verified"
	FaultAfterVerifiedRecorded                           FaultPoint = "after-verified-recorded"
	FaultAfterSupersededTombstoned                       FaultPoint = "after-superseded-tombstoned"
	FaultAfterTombstoneRecorded                          FaultPoint = "after-tombstone-recorded"
	FaultAfterSupersededDeleted                          FaultPoint = "after-superseded-deleted"
	FaultAfterDeleteRecorded                             FaultPoint = "after-delete-recorded"
	FaultAfterMigrationRecoveryPrepared                  FaultPoint = "after-migration-recovery-prepared"
	FaultAfterMigrationRecoverySecretDurable             FaultPoint = "after-migration-recovery-secret-durable"
	FaultAfterMigrationRecoverySecretDurableRecorded     FaultPoint = "after-migration-recovery-secret-durable-recorded"
	FaultAfterMigrationRecoveryReadbackVerified          FaultPoint = "after-migration-recovery-readback-verified"
	FaultAfterMigrationRecoveryVerifiedRecorded          FaultPoint = "after-migration-recovery-verified-recorded"
	FaultBeforeMigrationProviderTransactionPrepared      FaultPoint = "before-migration-provider-transaction-prepared"
	FaultBeforeMigrationRecoveryCommittedRetainedRecord  FaultPoint = "before-migration-recovery-committed-retained-record"
	FaultAfterMigrationRecoveryCommittedRetainedRecorded FaultPoint = "after-migration-recovery-committed-retained-recorded"
	FaultAfterMigrationRecoveryAbandonRecorded           FaultPoint = "after-migration-recovery-abandon-recorded"
	FaultAfterMigrationRollbackSourceRestoreRecorded     FaultPoint = "after-migration-rollback-source-restore-recorded"
	FaultAfterMigrationRollbackCommittedRecorded         FaultPoint = "after-migration-rollback-committed-recorded"
	FaultAfterMigrationFinalizingRecorded                FaultPoint = "after-migration-finalizing-recorded"
	FaultAfterMigrationFinalizationRecoveryDeleteIntent  FaultPoint = "after-migration-finalization-recovery-delete-intent"
	FaultAfterMigrationFinalizationWinnerDeleteIntent    FaultPoint = "after-migration-finalization-winner-delete-intent"
	FaultAfterMigrationFinalizationRecoveryTombstoned    FaultPoint = "after-migration-finalization-recovery-tombstoned"
	FaultAfterMigrationFinalizationRecoveryDeleted       FaultPoint = "after-migration-finalization-recovery-deleted"
	FaultAfterMigrationFinalizationWinnerTombstoned      FaultPoint = "after-migration-finalization-winner-tombstoned"
	FaultAfterMigrationFinalizationWinnerDeleted         FaultPoint = "after-migration-finalization-winner-deleted"
	FaultAfterMigrationFinalizedRecorded                 FaultPoint = "after-migration-finalized-recorded"
	FaultAfterMigrationProtectedDeleteIntent             FaultPoint = "after-migration-protected-delete-intent"
	FaultAfterMigrationProtectedDeleteTombstoned         FaultPoint = "after-migration-protected-delete-tombstoned"
	FaultAfterMigrationProtectedDeleteDeleted            FaultPoint = "after-migration-protected-delete-deleted"
	FaultAfterMigrationProtectedDeleteRecorded           FaultPoint = "after-migration-protected-delete-recorded"
)

type FaultRecorder interface {
	Observe(FaultPoint) error
}

type FaultRecorderFunc func(FaultPoint) error

func (record FaultRecorderFunc) Observe(point FaultPoint) error { return record(point) }

type Manager struct {
	registry                   registryport.Store
	secrets                    secretstoreport.RegistryStore
	faults                     FaultRecorder
	legacySource               registryport.LegacySourceReader
	sourceAuthorityMu          sync.Mutex
	sourceAuthorityChallenges  map[string]legacyMigrationSourceAuthorityChallenge
	protectedRecoveryMu        sync.Mutex
	protectedRecoveryConfirmed map[string]struct{}
}

// ExecutionResolution is a single bounded read of the selected committed
// Provider and its protected credential. Callers must Clear it as soon as the
// execution configuration has taken ownership of the credential bytes.
type ExecutionResolution struct {
	RegistryRevision    uint64
	RegistryIncarnation string
	Provider            domainregistry.Provider
	Credential          []byte
}

// ExecutionIntentResolution is the key-free current Provider winner used to
// build one model-step request shape before the physical Provider effect is
// acquired. Credential material is deliberately absent.
type ExecutionIntentResolution struct {
	RegistryRevision    uint64
	RegistryIncarnation string
	Provider            domainregistry.Provider
}

// ExecutionAuthority binds one physical Provider effect to the exact Registry
// and protected credential winner that authorized it.
type ExecutionAuthority struct {
	RegistryRevision          uint64
	RegistryIncarnation       string
	ProviderID                string
	ProviderRevision          uint64
	ProviderGeneration        uint64
	ProviderIncarnation       string
	ProviderCredentialRef     string
	ProviderCredentialPurpose string
}

// ProviderOperationCommand identifies one exact committed Provider winner for
// a bounded settings operation. Endpoint, proxy, and credential values are
// deliberately absent: the Manager resolves them from the Registry and Secret
// Store after validating this fence.
type ProviderOperationCommand struct {
	Expected   domainregistry.ExpectedState
	ProviderID string
}

type ProviderProbeStatus string

const (
	ProviderProbeStatusReachable       ProviderProbeStatus = "reachable"
	ProviderProbeStatusAuthFailed      ProviderProbeStatus = "auth_failed"
	ProviderProbeStatusTimeout         ProviderProbeStatus = "timeout"
	ProviderProbeStatusRedirectBlocked ProviderProbeStatus = "redirect_blocked"
	ProviderProbeStatusProviderError   ProviderProbeStatus = "provider_error"
	ProviderProbeStatusUnavailable     ProviderProbeStatus = "unavailable"
	ProviderProbeStatusInvalidResponse ProviderProbeStatus = "invalid_response"
)

type ProviderProbeResult struct {
	RegistryRevision    uint64
	RegistryIncarnation string
	ProviderRevision    uint64
	ProviderGeneration  uint64
	ProviderIncarnation string
	Status              ProviderProbeStatus
	Code                uint16
	ModelCount          int
	LatencyMillis       uint64
}

type ProviderAccountObservationStatus string

const (
	ProviderAccountObservationAvailable       ProviderAccountObservationStatus = "available"
	ProviderAccountObservationUnavailable     ProviderAccountObservationStatus = "unavailable"
	ProviderAccountObservationAuthFailed      ProviderAccountObservationStatus = "auth_failed"
	ProviderAccountObservationTimeout         ProviderAccountObservationStatus = "timeout"
	ProviderAccountObservationRedirectBlocked ProviderAccountObservationStatus = "redirect_blocked"
	ProviderAccountObservationProviderError   ProviderAccountObservationStatus = "provider_error"
	ProviderAccountObservationInvalidResponse ProviderAccountObservationStatus = "invalid_response"
)

type ProviderAccountObservationResult struct {
	RegistryRevision          uint64
	RegistryIncarnation       string
	ProviderID                string
	ProviderRevision          uint64
	ProviderGeneration        uint64
	ProviderIncarnation       string
	ProviderCredentialPurpose string
	Status                    ProviderAccountObservationStatus
	ObservedAt                time.Time
	ExpiresAt                 time.Time
	Quota                     *float64
	Usage                     *float64
	Remaining                 *float64
}

func (resolution *ExecutionResolution) Clear() {
	if resolution == nil {
		return
	}
	clear(resolution.Credential)
	resolution.Credential = nil
}

func (resolution ExecutionResolution) Authority() ExecutionAuthority {
	return ExecutionAuthority{
		RegistryRevision:          resolution.RegistryRevision,
		RegistryIncarnation:       resolution.RegistryIncarnation,
		ProviderID:                resolution.Provider.ID,
		ProviderRevision:          resolution.Provider.Revision,
		ProviderGeneration:        resolution.Provider.Generation,
		ProviderIncarnation:       resolution.Provider.Incarnation,
		ProviderCredentialRef:     resolution.Provider.CredentialRef,
		ProviderCredentialPurpose: resolution.Provider.CredentialPurpose,
	}
}

type legacyMigrationSourceAuthorityChallenge struct {
	Operation                    string
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	CurrentSourceSHA256          string
	SourcePhysicalIdentitySHA256 string
	RecoveryCredentialRef        secretstoreport.CredentialRef
}

// NewManager permits ordinary Registry operations without a legacy source reader.
// Source-authority operations fail closed unless a reader is explicitly composed.
func NewManager(registry registryport.Store, secrets secretstoreport.RegistryStore, legacySource ...registryport.LegacySourceReader) (*Manager, error) {
	return NewManagerWithFaultRecorder(registry, secrets, nil, legacySource...)
}

func NewManagerWithFaultRecorder(
	registry registryport.Store,
	secrets secretstoreport.RegistryStore,
	faults FaultRecorder,
	legacySource ...registryport.LegacySourceReader,
) (*Manager, error) {
	if registry == nil || secrets == nil || len(legacySource) > 1 {
		return nil, registryport.ErrInvalidRequest
	}
	var reader registryport.LegacySourceReader
	if len(legacySource) == 1 {
		reader = legacySource[0]
	}
	return &Manager{
		registry: registry, secrets: secrets, faults: faults, legacySource: reader,
		protectedRecoveryConfirmed: make(map[string]struct{}),
	}, nil
}

type ConnectCommand struct {
	DeferSelection    bool
	Expected          domainregistry.ExpectedState
	Provider          domainregistry.ProviderInput
	CredentialPurpose secretstoreport.Purpose
	Credential        secretstoreport.CredentialMutation
}

type UpdateCommand struct {
	Expected          domainregistry.ExpectedState
	Provider          domainregistry.ProviderInput
	CredentialPurpose secretstoreport.Purpose
	Credential        secretstoreport.CredentialMutation
}

type SelectCommand struct {
	Expected   domainregistry.ExpectedState
	ProviderID string
}

type CredentialReplaceCommand struct {
	Expected          domainregistry.ExpectedState
	ProviderID        string
	CredentialPurpose secretstoreport.Purpose
	Credential        secretstoreport.CredentialMutation
}

type DisconnectCommand struct {
	Expected                  domainregistry.ExpectedState
	ProviderID                string
	CredentialPurpose         secretstoreport.Purpose
	PrivateAccountDisposition string
}

type ExplicitDeleteCommand struct {
	Expected          domainregistry.ExpectedState
	ProviderID        string
	CredentialPurpose secretstoreport.Purpose
}

type LegacyMigrationRollbackCredentialArtifact struct {
	Locators   []string
	Credential []byte
}

type LegacyMigrationCandidate struct {
	Expected                     domainregistry.ExpectedState
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	ExpectedCleanedSourceSHA256  string
	SourcePhysicalIdentitySHA256 string
	Provider                     domainregistry.ProviderInput
	CredentialPurpose            secretstoreport.Purpose
	Credential                   []byte
	SourceSnapshot               []byte
	ActiveCredentialLocators     []string
	RollbackCredentialArtifacts  []LegacyMigrationRollbackCredentialArtifact
}

type LegacyMigrationRecoveryStatus string

type LegacyMigrationRecoveryResult struct {
	Status                             LegacyMigrationRecoveryStatus
	MigrationID                        string
	RecoveryCredentialRef              secretstoreport.CredentialRef
	SafeToProceedWithProviderMigration bool
}

type AbandonLegacyMigrationRecoveryCommand struct {
	MigrationID           string
	SourceLocator         string
	SourceSHA256          string
	RecoveryCredentialRef secretstoreport.CredentialRef
	Confirmation          string
}

type CommitVerifiedLegacyMigrationRecoveryCommand struct {
	Expected                   domainregistry.ExpectedState
	ExpectedSelectedProviderID string
	MigrationID                string
	SourceLocator              string
	SourceSHA256               string
	RecoveryCredentialRef      secretstoreport.CredentialRef
	Confirmation               string
	remigrationAuthorityIntent string
}

type CommitVerifiedLegacyMigrationRecoveryResult struct {
	Status                      LegacyMigrationRecoveryStatus
	MigrationID                 string
	Provider                    domainregistry.Provider
	RecoveryCredentialRef       secretstoreport.CredentialRef
	SafeToRemoveLegacyPlaintext bool
}

type LegacyMigrationRollbackStatus string

const (
	LegacyMigrationRollbackStatusPreCommitCompleted        = LegacyMigrationRollbackStatus("PRE_COMMIT_ROLLBACK_COMPLETED")
	LegacyMigrationRollbackStatusCleanedSourceRequired     = LegacyMigrationRollbackStatus("CLEANED_SOURCE_AUTHORITY_REQUIRED")
	LegacyMigrationRollbackStatusCommittedRecoveryRetained = LegacyMigrationRollbackStatus("ROLLBACK_COMMITTED_RECOVERY_RETAINED")
	LegacyMigrationRollbackStatusRecoveryRetainedTerminal  = LegacyMigrationRollbackStatus("ROLLBACK_RECOVERY_RETAINED")
	LegacyMigrationRollbackStatusAlreadyFinalized          = LegacyMigrationRollbackStatus("ALREADY_FINALIZED")
)

type BeginLegacyMigrationRollbackCommand struct {
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	SourcePhysicalIdentitySHA256 string
	RecoveryCredentialRef        secretstoreport.CredentialRef
	Confirmation                 string
}

type BeginLegacyMigrationRollbackResult struct {
	Status      LegacyMigrationRollbackStatus
	MigrationID string
}

type CommitLegacyMigrationRollbackCommand struct {
	Expected                     domainregistry.ExpectedState
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	VerifiedCleanedSourceSHA256  string
	SourcePhysicalIdentitySHA256 string
	VerifiedCleanedSource        []byte
	SourceAuthority              LegacyMigrationSourceAuthorityProof
	RecoveryCredentialRef        secretstoreport.CredentialRef
	Confirmation                 string
}

type CommitLegacyMigrationRollbackResult struct {
	Status      LegacyMigrationRollbackStatus
	MigrationID string
}

type RemigrateRetainedLegacyMigrationRecoveryCommand struct {
	Expected                     domainregistry.ExpectedState
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	VerifiedCleanedSourceSHA256  string
	SourcePhysicalIdentitySHA256 string
	VerifiedCleanedSource        []byte
	SourceAuthority              LegacyMigrationSourceAuthorityProof
	RecoveryCredentialRef        secretstoreport.CredentialRef
	Confirmation                 string
}

type LegacyMigrationProtectedDeleteStatus string

const (
	LegacyMigrationProtectedDeleteStatusCompleted      = LegacyMigrationProtectedDeleteStatus("COMPLETED")
	LegacyMigrationProtectedDeleteStatusAlreadyDeleted = LegacyMigrationProtectedDeleteStatus("ALREADY_DELETED")
)

type DeleteRetainedLegacyMigrationRecoveryCommand struct {
	Expected                     domainregistry.ExpectedState
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	VerifiedCleanedSourceSHA256  string
	SourcePhysicalIdentitySHA256 string
	VerifiedCleanedSource        []byte
	SourceAuthority              LegacyMigrationSourceAuthorityProof
	RecoveryCredentialRef        secretstoreport.CredentialRef
	Confirmation                 string
}

type DeleteRetainedLegacyMigrationRecoveryResult struct {
	Status      LegacyMigrationProtectedDeleteStatus
	MigrationID string
}

type LegacyMigrationFinalizationStatus string
type LegacyMigrationFinalizationOutcome string

const (
	LegacyMigrationFinalizationStatusCompleted        = LegacyMigrationFinalizationStatus("COMPLETED")
	LegacyMigrationFinalizationStatusAlreadyFinalized = LegacyMigrationFinalizationStatus("ALREADY_FINALIZED")
	LegacyMigrationFinalizationOutcomeCommitted       = LegacyMigrationFinalizationOutcome("MIGRATION_COMMITTED")
	LegacyMigrationFinalizationOutcomeRolledBack      = LegacyMigrationFinalizationOutcome("PROTECTED_RECOVERY_RETAINED")
)

type FinalizeLegacyMigrationRecoveryCommand struct {
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	VerifiedSourceSHA256         string
	SourcePhysicalIdentitySHA256 string
	VerifiedSource               []byte
	SourceAuthority              LegacyMigrationSourceAuthorityProof
	RecoveryCredentialRef        secretstoreport.CredentialRef
	Confirmation                 string
}

type IssueLegacyMigrationSourceAuthorityChallengeCommand struct {
	Operation                    string
	MigrationID                  string
	SourceLocator                string
	SourceSHA256                 string
	CurrentSourceSHA256          string
	SourcePhysicalIdentitySHA256 string
	RecoveryCredentialRef        secretstoreport.CredentialRef
	Confirmation                 string
}

type IssueLegacyMigrationSourceAuthorityChallengeResult struct {
	Challenge string
}

type LegacyMigrationSourceAuthorityProof struct {
	Challenge      string
	SourcePath     string
	LockOwnerToken string
	SourceDevice   string
	SourceInode    string
}

type LegacyMigrationRecoveryDescriptor struct {
	MigrationID                  string
	ProviderID                   string
	SourceLocator                string
	SourceSHA256                 string
	ExpectedCleanedSourceSHA256  string
	SourcePhysicalIdentitySHA256 string
	RecoveryCredentialRef        secretstoreport.CredentialRef
	Phase                        domainregistry.LegacyMigrationRecoveryPhase
	CommitOrder                  uint64
}

type LegacyMigrationRecoveryInventoryResult struct {
	Recoveries []LegacyMigrationRecoveryDescriptor
}

type FinalizeLegacyMigrationRecoveryResult struct {
	Status      LegacyMigrationFinalizationStatus
	Outcome     LegacyMigrationFinalizationOutcome
	MigrationID string
}

func (manager *Manager) observe(point FaultPoint) error {
	if manager.faults == nil {
		return nil
	}
	if err := manager.faults.Observe(point); err != nil {
		return ErrInterrupted
	}
	return nil
}

func normalizePurpose(raw secretstoreport.Purpose, required bool) (secretstoreport.Purpose, error) {
	if raw == "" && !required {
		return "", nil
	}
	normalized, err := secretstoreport.NormalizePurpose(string(raw))
	if err != nil || normalized != raw {
		return "", registryport.ErrInvalidRequest
	}
	return normalized, nil
}

func normalizeSecretError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, secretstoreport.ErrInvalidRequest):
		return registryport.ErrInvalidRequest
	case errors.Is(err, secretstoreport.ErrMasterKeyUnavailable):
		return registryport.ErrCredentialUnavailable
	case errors.Is(err, secretstoreport.ErrPersistence):
		return registryport.ErrPersistence
	default:
		return registryport.ErrVerification
	}
}

func clear(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
