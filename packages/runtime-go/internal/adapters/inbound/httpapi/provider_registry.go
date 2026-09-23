package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	ProviderRegistryPathV1                                           = "/v1/provider-registry"
	ProviderRegistryPortableManifestPathV1                           = ProviderRegistryPathV1 + "/portable-manifest"
	ProviderRegistryPrivateProtectedRecoveryPreparePathV1            = ProviderRegistryPathV1 + "/_private/protected-recovery/prepare"
	ProviderRegistryPrivateProtectedRecoveryConfirmDestinationPathV1 = ProviderRegistryPathV1 + "/_private/protected-recovery/confirm-destination"
	ProviderRegistryPrivateProtectedRecoveryCreateBundlePathV1       = ProviderRegistryPathV1 + "/_private/protected-recovery/create-bundle"
	ProviderRegistryPrivateProtectedRecoveryApplyPathV1              = ProviderRegistryPathV1 + "/_private/protected-recovery/apply"
	ProviderRegistryPrivateProtectedRecoveryFinalizePathV1           = ProviderRegistryPathV1 + "/_private/protected-recovery/finalize"
	ProviderRegistryPrivateProtectedRecoveryRecoverPathV1            = ProviderRegistryPathV1 + "/_private/protected-recovery/recover"
	ProviderRegistryPrivateProtectedRecoveryRollbackPathV1           = ProviderRegistryPathV1 + "/_private/protected-recovery/rollback"
	// Short aliases keep the operation-specific path names discoverable to
	// Main without introducing a generic credential endpoint.
	ProviderRegistryPrivateProtectedRecoveryConfirmPathV1                = ProviderRegistryPrivateProtectedRecoveryConfirmDestinationPathV1
	ProviderRegistryPrivateProtectedRecoveryStatusPathV1                 = ProviderRegistryPrivateProtectedRecoveryRecoverPathV1
	providerRegistryPrivateLegacyMigrationPreparePathV1                  = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/prepare"
	providerRegistryPrivateLegacyMigrationCommitPathV1                   = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/commit"
	providerRegistryPrivateLegacyMigrationAbandonPathV1                  = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/abandon"
	providerRegistryPrivateLegacyMigrationRollbackBeginPathV1            = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/rollback/begin"
	providerRegistryPrivateLegacyMigrationRollbackCommitPathV1           = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/rollback/commit"
	providerRegistryPrivateLegacyMigrationRemigratePathV1                = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/remigrate"
	providerRegistryPrivateLegacyMigrationProtectedDeletePathV1          = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/protected-delete"
	providerRegistryPrivateLegacyMigrationFinalizePathV1                 = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/finalize"
	providerRegistryPrivateLegacyMigrationInventoryPathV1                = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/inventory"
	providerRegistryPrivateLegacyMigrationSourceAuthorityChallengePathV1 = ProviderRegistryPathV1 + "/_private/legacy-migration-recovery/source-authority/challenge"
	providerRegistryPrivateAccountStatusPathV1                           = ProviderRegistryPathV1 + "/_private/account-credentials/status"
	providerRegistryPrivateAccountListPathV1                             = ProviderRegistryPathV1 + "/_private/account-credentials/list"
	providerRegistryPrivateAccountPutPathV1                              = ProviderRegistryPathV1 + "/_private/account-credentials/put"
	providerRegistryPrivateAccountResolvePathV1                          = ProviderRegistryPathV1 + "/_private/account-credentials/resolve"
	providerRegistryPrivateAccountMutatePathV1                           = ProviderRegistryPathV1 + "/_private/account-credentials/mutate"
	providerRegistryMaxBodyBytes                                         = 2 << 20
	providerRegistryMaxCredentialBytes                                   = 1 << 20
	providerRegistryMaxLegacyMigrationLocatorBytes                       = 256
	providerRegistryMaxLegacyMigrationLocatorCount                       = 256
	providerRegistryMaxLegacyMigrationLocatorTotalBytes                  = 32 << 10
	providerRegistryMaxLegacyMigrationRollbackArtifacts                  = 64
	providerRegistryMaxLegacyMigrationSourceBytes                        = 256 << 10
)

type ProviderRegistryService interface {
	Snapshot(context.Context) (domainregistry.Registry, error)
	Connect(context.Context, providerregistryapp.ConnectCommand) (domainregistry.Provider, error)
	Update(context.Context, providerregistryapp.UpdateCommand) (domainregistry.Provider, error)
	Select(context.Context, providerregistryapp.SelectCommand) (domainregistry.Provider, error)
	ReplaceCredential(context.Context, providerregistryapp.CredentialReplaceCommand) (domainregistry.Provider, error)
	Disconnect(context.Context, providerregistryapp.DisconnectCommand) (domainregistry.Provider, error)
	ExplicitDelete(context.Context, providerregistryapp.ExplicitDeleteCommand) error
	Probe(context.Context, providerregistryapp.ProviderOperationCommand) (providerregistryapp.ProviderProbeResult, error)
	CheckCredential(context.Context, providerregistryapp.ProviderOperationCommand) error
	DiscoverModels(context.Context, providerregistryapp.ProviderOperationCommand) (domainregistry.Provider, error)
	ObserveAccount(context.Context, providerregistryapp.ProviderOperationCommand) (providerregistryapp.ProviderAccountObservationResult, error)
	Recover(context.Context) error
	PrepareLegacyMigrationRecovery(context.Context, providerregistryapp.LegacyMigrationCandidate) (providerregistryapp.LegacyMigrationRecoveryResult, error)
	CommitVerifiedLegacyMigrationRecovery(context.Context, providerregistryapp.CommitVerifiedLegacyMigrationRecoveryCommand) (providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult, error)
	AbandonLegacyMigrationRecovery(context.Context, providerregistryapp.AbandonLegacyMigrationRecoveryCommand) error
	BeginLegacyMigrationRollback(context.Context, providerregistryapp.BeginLegacyMigrationRollbackCommand) (providerregistryapp.BeginLegacyMigrationRollbackResult, error)
	CommitLegacyMigrationRollback(context.Context, providerregistryapp.CommitLegacyMigrationRollbackCommand) (providerregistryapp.CommitLegacyMigrationRollbackResult, error)
	RemigrateRetainedLegacyMigrationRecovery(context.Context, providerregistryapp.RemigrateRetainedLegacyMigrationRecoveryCommand) (providerregistryapp.CommitVerifiedLegacyMigrationRecoveryResult, error)
	DeleteRetainedLegacyMigrationRecovery(context.Context, providerregistryapp.DeleteRetainedLegacyMigrationRecoveryCommand) (providerregistryapp.DeleteRetainedLegacyMigrationRecoveryResult, error)
	FinalizeLegacyMigrationRecovery(context.Context, providerregistryapp.FinalizeLegacyMigrationRecoveryCommand) (providerregistryapp.FinalizeLegacyMigrationRecoveryResult, error)
	InventoryLegacyMigrationRecoveries(context.Context) (providerregistryapp.LegacyMigrationRecoveryInventoryResult, error)
	IssueLegacyMigrationSourceAuthorityChallenge(context.Context, providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeCommand) (providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeResult, error)
	AccountCredentialState(context.Context, domainregistry.PrivateAccountScope) (providerregistryapp.AccountCredentialState, error)
	ListAccountCredentialStates(context.Context, providerregistryapp.AccountCredentialListFilter) ([]providerregistryapp.AccountCredentialState, error)
	PutAccountCredential(context.Context, providerregistryapp.AccountCredentialPutCommand) (providerregistryapp.AccountCredentialState, error)
	ResolveAccountCredential(context.Context, domainregistry.PrivateAccountScope) (providerregistryapp.AccountCredentialResolution, error)
	MutateAccountCredential(context.Context, providerregistryapp.AccountCredentialMutationCommand) (providerregistryapp.AccountCredentialState, error)
	ExportPortableManifest(context.Context) ([]byte, error)
	ImportPortableManifest(context.Context, []byte) (providerregistryapp.PortableImportResult, error)
	PrepareProtectedRecoveryRequest(context.Context, providerregistryapp.ProtectedRecoveryRequestCommand) (domainregistry.ProtectedRecoveryPreparedRequestV1, error)
	ConfirmProtectedRecoveryDestination(context.Context, providerregistryapp.ProtectedRecoveryDestinationConfirmationCommand) error
	CreateProtectedRecoveryBundle(context.Context, providerregistryapp.ProtectedRecoveryBundleCommand) ([]byte, error)
	ApplyProtectedRecoveryBundle(context.Context, providerregistryapp.ProtectedRecoveryApplyCommand) ([]byte, error)
	FinalizeProtectedRecoveryReceipt(context.Context, providerregistryapp.ProtectedRecoveryFinalizeCommand) (domainregistry.ProtectedRecoveryResultV1, error)
	RecoverProtectedRecovery(context.Context, ...providerregistryapp.ProtectedRecoveryRecoverCommand) (domainregistry.ProtectedRecoveryResultV1, error)
	RollbackProtectedRecovery(context.Context, providerregistryapp.ProtectedRecoveryRollbackCommand) (domainregistry.ProtectedRecoveryResultV1, error)
}

type ProviderRegistryHandlers struct {
	Service                               ProviderRegistryService
	observeCredentialBufferCleared        func([]byte)
	observeEncodedCredentialBufferCleared func([]byte)
}

type providerRegistryPathOperation uint8

const (
	providerRegistryPathInvalid providerRegistryPathOperation = iota
	providerRegistryPathCollection
	providerRegistryPathPortableManifest
	providerRegistryPathRecover
	providerRegistryPathProvider
	providerRegistryPathSelect
	providerRegistryPathDisconnect
	providerRegistryPathCredential
	providerRegistryPathProbe
	providerRegistryPathCredentialCheck
	providerRegistryPathDiscoverModels
	providerRegistryPathObserveAccount
	providerRegistryPathPrivateLegacyMigrationPrepare
	providerRegistryPathPrivateLegacyMigrationCommit
	providerRegistryPathPrivateLegacyMigrationAbandon
	providerRegistryPathPrivateLegacyMigrationRollbackBegin
	providerRegistryPathPrivateLegacyMigrationRollbackCommit
	providerRegistryPathPrivateLegacyMigrationRemigrate
	providerRegistryPathPrivateLegacyMigrationProtectedDelete
	providerRegistryPathPrivateLegacyMigrationFinalize
	providerRegistryPathPrivateLegacyMigrationInventory
	providerRegistryPathPrivateLegacyMigrationSourceAuthorityChallenge
	providerRegistryPathPrivateAccountStatus
	providerRegistryPathPrivateAccountList
	providerRegistryPathPrivateAccountPut
	providerRegistryPathPrivateAccountResolve
	providerRegistryPathPrivateAccountMutate
	providerRegistryPathPrivateProtectedRecoveryPrepare
	providerRegistryPathPrivateProtectedRecoveryConfirmDestination
	providerRegistryPathPrivateProtectedRecoveryCreateBundle
	providerRegistryPathPrivateProtectedRecoveryApply
	providerRegistryPathPrivateProtectedRecoveryFinalize
	providerRegistryPathPrivateProtectedRecoveryRecover
	providerRegistryPathPrivateProtectedRecoveryRollback
)

type providerRegistryPath struct {
	operation  providerRegistryPathOperation
	providerID string
}

type providerRegistryExpectedRequest struct {
	RegistryRevision          *string `json:"registryRevision"`
	RegistryIncarnation       *string `json:"registryIncarnation"`
	ProviderRevision          *string `json:"providerRevision"`
	ProviderGeneration        *string `json:"providerGeneration"`
	ProviderIncarnation       *string `json:"providerIncarnation"`
	ProviderCredentialPurpose *string `json:"providerCredentialPurpose"`
}

type providerRegistryCredentialRequest struct {
	Kind        string  `json:"kind"`
	Purpose     *string `json:"purpose,omitempty"`
	ValueBase64 []byte  `json:"valueBase64,omitempty"`
}

type providerRegistryConnectRequest struct {
	DeferSelection bool                              `json:"deferSelection,omitempty"`
	SchemaVersion  int                               `json:"schemaVersion"`
	Expected       providerRegistryExpectedRequest   `json:"expected"`
	Provider       domainregistry.ProviderInput      `json:"provider"`
	Credential     providerRegistryCredentialRequest `json:"credential"`
}

type providerRegistryUpdateRequest struct {
	SchemaVersion int                                `json:"schemaVersion"`
	Expected      providerRegistryExpectedRequest    `json:"expected"`
	Provider      domainregistry.ProviderInput       `json:"provider"`
	Credential    *providerRegistryCredentialRequest `json:"credential,omitempty"`
}

type providerRegistryExpectedOnlyRequest struct {
	SchemaVersion int                             `json:"schemaVersion"`
	Expected      providerRegistryExpectedRequest `json:"expected"`
}

type providerRegistryCredentialReplaceRequest struct {
	SchemaVersion int                               `json:"schemaVersion"`
	Expected      providerRegistryExpectedRequest   `json:"expected"`
	Credential    providerRegistryCredentialRequest `json:"credential"`
}

type providerRegistryRecoverRequest struct {
	SchemaVersion int `json:"schemaVersion"`
}

type providerRegistryPortableManifestImportRequest struct {
	SchemaVersion int    `json:"schemaVersion"`
	ManifestJSON  string `json:"manifestJson"`
}

type providerRegistryPortableManifestExportResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	ManifestJSON  string `json:"manifestJson"`
}

type providerRegistryPortableManifestImportResponse struct {
	SchemaVersion   int                                             `json:"schemaVersion"`
	ProviderCount   int                                             `json:"providerCount"`
	AccountCount    int                                             `json:"accountCount"`
	ReentryRequired int                                             `json:"reentryRequired"`
	Entries         []providerregistryapp.PortableImportEntryResult `json:"entries"`
}

type providerRegistryPrivateProtectedRecoveryPrepareRequest struct {
	SchemaVersion         int                                               `json:"schemaVersion"`
	ManifestBase64        json.RawMessage                                   `json:"manifestBase64"`
	ImportResult          providerregistryapp.ProtectedRecoveryImportResult `json:"importResult"`
	LocalBinding          domainregistry.ProtectedRecoveryLocalBindingV1    `json:"localBinding"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1  `json:"ownerBindingInventory"`
}

type providerRegistryPrivateProtectedRecoveryConfirmRequest struct {
	SchemaVersion         int                                              `json:"schemaVersion"`
	RequestBase64         json.RawMessage                                  `json:"requestBase64"`
	LocalAction           domainregistry.ProtectedRecoveryLocalActionV1    `json:"localAction"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"ownerBindingInventory"`
}

type providerRegistryPrivateProtectedRecoveryCreateBundleRequest struct {
	SchemaVersion         int                                              `json:"schemaVersion"`
	RequestBase64         json.RawMessage                                  `json:"requestBase64"`
	LocalAction           domainregistry.ProtectedRecoveryLocalActionV1    `json:"localAction"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"ownerBindingInventory"`
}

type providerRegistryPrivateProtectedRecoveryApplyRequest struct {
	SchemaVersion         int                                              `json:"schemaVersion"`
	BundleBase64          json.RawMessage                                  `json:"bundleBase64"`
	RequestBase64         json.RawMessage                                  `json:"requestBase64,omitempty"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"ownerBindingInventory"`
}

type providerRegistryPrivateProtectedRecoveryFinalizeRequest struct {
	SchemaVersion int             `json:"schemaVersion"`
	ReceiptBase64 json.RawMessage `json:"receiptBase64"`
}

type providerRegistryPrivateProtectedRecoveryRecoverRequest struct {
	SchemaVersion         int                                              `json:"schemaVersion"`
	RequestBase64         json.RawMessage                                  `json:"requestBase64,omitempty"`
	LocalAction           *domainregistry.ProtectedRecoveryLocalActionV1   `json:"localAction,omitempty"`
	OwnerBindingInventory []domainregistry.ProtectedRecoveryOwnerBindingV1 `json:"ownerBindingInventory,omitempty"`
}

type providerRegistryPrivateProtectedRecoveryRollbackRequest struct {
	SchemaVersion int             `json:"schemaVersion"`
	RequestBase64 json.RawMessage `json:"requestBase64"`
}

type providerRegistryPrivateProtectedRecoveryPrepareResponse struct {
	SchemaVersion      int    `json:"schemaVersion"`
	RequestBase64      []byte `json:"requestBase64"`
	RequestDigest      string `json:"requestDigest"`
	RequestFingerprint string `json:"requestFingerprint"`
	ManifestDigest     string `json:"manifestDigest"`
	ItemSetDigest      string `json:"itemSetDigest"`
	OperationID        string `json:"operationId"`
	SessionNonce       string `json:"sessionNonce"`
	ExpiresAt          string `json:"expiresAt"`
}

type providerRegistryPrivateProtectedRecoveryBundleResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	BundleBase64  []byte `json:"bundleBase64"`
}

type providerRegistryPrivateProtectedRecoveryReceiptResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	ReceiptBase64 []byte `json:"receiptBase64"`
}

type providerRegistryPrivateProtectedRecoveryStatusResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Confirmed     bool   `json:"confirmed"`
	ReceiptBase64 []byte `json:"receiptBase64,omitempty"`
}

type providerRegistryPrivateLegacyMigrationCredentialRequest struct {
	Purpose     string          `json:"purpose"`
	ValueBase64 json.RawMessage `json:"valueBase64"`
}

type providerRegistryPrivateLegacyMigrationArtifactCredentialRequest struct {
	ValueBase64 json.RawMessage `json:"valueBase64"`
}

type providerRegistryPrivateLegacyMigrationSourceRequest struct {
	ValueBase64 json.RawMessage `json:"valueBase64"`
}

type providerRegistryPrivateLegacyMigrationRollbackArtifactRequest struct {
	SchemaVersion      int                                                             `json:"schemaVersion"`
	CredentialLocators []string                                                        `json:"credentialLocators"`
	Credential         providerRegistryPrivateLegacyMigrationArtifactCredentialRequest `json:"credential"`
}

type providerRegistryPrivateLegacyMigrationPrepareRequest struct {
	SchemaVersion                int                                                             `json:"schemaVersion"`
	Expected                     providerRegistryExpectedRequest                                 `json:"expected"`
	MigrationID                  string                                                          `json:"migrationId"`
	SourceLocator                string                                                          `json:"sourceLocator"`
	SourceSHA256                 string                                                          `json:"sourceSHA256"`
	ExpectedCleanedSourceSHA256  string                                                          `json:"expectedCleanedSourceSHA256"`
	SourcePhysicalIdentitySHA256 string                                                          `json:"sourcePhysicalIdentitySHA256"`
	Provider                     domainregistry.ProviderInput                                    `json:"provider"`
	Credential                   providerRegistryPrivateLegacyMigrationCredentialRequest         `json:"credential"`
	RollbackSource               providerRegistryPrivateLegacyMigrationSourceRequest             `json:"rollbackSource"`
	ActiveCredentialLocators     []string                                                        `json:"activeCredentialLocators,omitempty"`
	RollbackCredentialArtifacts  []providerRegistryPrivateLegacyMigrationRollbackArtifactRequest `json:"rollbackCredentialArtifacts,omitempty"`
}

type providerRegistryPrivateLegacyMigrationAbandonRequest struct {
	SchemaVersion         int    `json:"schemaVersion"`
	MigrationID           string `json:"migrationId"`
	SourceLocator         string `json:"sourceLocator"`
	SourceSHA256          string `json:"sourceSHA256"`
	RecoveryCredentialRef string `json:"recoveryCredentialRef"`
	Confirmation          string `json:"confirmation"`
}

type providerRegistryPrivateLegacyMigrationCommitRequest struct {
	SchemaVersion              int                             `json:"schemaVersion"`
	Expected                   providerRegistryExpectedRequest `json:"expected"`
	ExpectedSelectedProviderID *string                         `json:"expectedSelectedProviderID"`
	MigrationID                string                          `json:"migrationId"`
	SourceLocator              string                          `json:"sourceLocator"`
	SourceSHA256               string                          `json:"sourceSHA256"`
	RecoveryCredentialRef      string                          `json:"recoveryCredentialRef"`
	Confirmation               string                          `json:"confirmation"`
}

type providerRegistryPrivateLegacyMigrationRollbackBeginRequest struct {
	SchemaVersion                int    `json:"schemaVersion"`
	MigrationID                  string `json:"migrationId"`
	SourceLocator                string `json:"sourceLocator"`
	SourceSHA256                 string `json:"sourceSHA256"`
	SourcePhysicalIdentitySHA256 string `json:"sourcePhysicalIdentitySHA256,omitempty"`
	RecoveryCredentialRef        string `json:"recoveryCredentialRef"`
	Confirmation                 string `json:"confirmation"`
}

type providerRegistryPrivateLegacyMigrationRollbackCommitRequest struct {
	SchemaVersion                int                                                               `json:"schemaVersion"`
	Expected                     providerRegistryExpectedRequest                                   `json:"expected"`
	MigrationID                  string                                                            `json:"migrationId"`
	SourceLocator                string                                                            `json:"sourceLocator"`
	SourceSHA256                 string                                                            `json:"sourceSHA256"`
	VerifiedCleanedSourceSHA256  string                                                            `json:"verifiedCleanedSourceSHA256"`
	SourcePhysicalIdentitySHA256 string                                                            `json:"sourcePhysicalIdentitySHA256"`
	VerifiedCleanedSource        providerRegistryPrivateLegacyMigrationSourceRequest               `json:"verifiedCleanedSource"`
	SourceAuthority              providerRegistryPrivateLegacyMigrationSourceAuthorityProofRequest `json:"sourceAuthority"`
	RecoveryCredentialRef        string                                                            `json:"recoveryCredentialRef"`
	Confirmation                 string                                                            `json:"confirmation"`
}

type providerRegistryPrivateLegacyMigrationSourceActionRequest = providerRegistryPrivateLegacyMigrationRollbackCommitRequest

type providerRegistryPrivateLegacyMigrationFinalizeRequest struct {
	SchemaVersion                int                                                               `json:"schemaVersion"`
	MigrationID                  string                                                            `json:"migrationId"`
	SourceLocator                string                                                            `json:"sourceLocator"`
	SourceSHA256                 string                                                            `json:"sourceSHA256"`
	VerifiedSourceSHA256         string                                                            `json:"verifiedSourceSHA256"`
	SourcePhysicalIdentitySHA256 string                                                            `json:"sourcePhysicalIdentitySHA256"`
	VerifiedSource               providerRegistryPrivateLegacyMigrationSourceRequest               `json:"verifiedSource"`
	SourceAuthority              providerRegistryPrivateLegacyMigrationSourceAuthorityProofRequest `json:"sourceAuthority"`
	RecoveryCredentialRef        string                                                            `json:"recoveryCredentialRef,omitempty"`
	Confirmation                 string                                                            `json:"confirmation"`
}

type providerRegistryPrivateLegacyMigrationSourceAuthorityProofRequest struct {
	Challenge      string `json:"challenge"`
	SourcePath     string `json:"sourcePath"`
	LockOwnerToken string `json:"lockOwnerToken"`
	SourceDevice   string `json:"sourceDevice"`
	SourceInode    string `json:"sourceInode"`
}

type providerRegistryPrivateLegacyMigrationSourceAuthorityChallengeRequest struct {
	SchemaVersion                int    `json:"schemaVersion"`
	Operation                    string `json:"operation"`
	MigrationID                  string `json:"migrationId"`
	SourceLocator                string `json:"sourceLocator"`
	SourceSHA256                 string `json:"sourceSHA256"`
	CurrentSourceSHA256          string `json:"currentSourceSHA256"`
	SourcePhysicalIdentitySHA256 string `json:"sourcePhysicalIdentitySHA256"`
	RecoveryCredentialRef        string `json:"recoveryCredentialRef"`
	Confirmation                 string `json:"confirmation"`
}

type providerRegistryPrivateLegacyMigrationInventoryRequest struct {
	SchemaVersion int    `json:"schemaVersion"`
	Confirmation  string `json:"confirmation"`
}

type providerRegistryPrivateLegacyMigrationPrepareResponse struct {
	SchemaVersion                      int    `json:"schemaVersion"`
	Status                             string `json:"status"`
	MigrationID                        string `json:"migrationId"`
	RecoveryCredentialRef              string `json:"recoveryCredentialRef"`
	SafeToProceedWithProviderMigration bool   `json:"safeToProceedWithProviderMigration"`
}

type providerRegistryPrivateLegacyMigrationAbandonResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
}

type providerRegistryPrivateLegacyMigrationCommitResponse struct {
	SchemaVersion               int                            `json:"schemaVersion"`
	Status                      string                         `json:"status"`
	MigrationID                 string                         `json:"migrationId"`
	SafeToRemoveLegacyPlaintext bool                           `json:"safeToRemoveLegacyPlaintext"`
	Provider                    providerRegistryPublicProvider `json:"provider"`
}

type providerRegistryPrivateLegacyMigrationRollbackBeginResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	MigrationID   string `json:"migrationId"`
}

type providerRegistryPrivateLegacyMigrationRollbackCommitResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	MigrationID   string `json:"migrationId"`
}

type providerRegistryPrivateLegacyMigrationProtectedDeleteResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	MigrationID   string `json:"migrationId"`
}

type providerRegistryPrivateLegacyMigrationFinalizeResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Status        string `json:"status"`
	Outcome       string `json:"outcome"`
	MigrationID   string `json:"migrationId"`
}

type providerRegistryPrivateLegacyMigrationRecoveryDescriptorResponse struct {
	MigrationID                  string `json:"migrationId"`
	ProviderID                   string `json:"providerId"`
	SourceLocator                string `json:"sourceLocator"`
	SourceSHA256                 string `json:"sourceSHA256"`
	ExpectedCleanedSourceSHA256  string `json:"expectedCleanedSourceSHA256"`
	SourcePhysicalIdentitySHA256 string `json:"sourcePhysicalIdentitySHA256"`
	RecoveryCredentialRef        string `json:"recoveryCredentialRef"`
	Phase                        string `json:"phase"`
	CommitOrder                  string `json:"commitOrder"`
}

type providerRegistryPrivateLegacyMigrationInventoryResponse struct {
	SchemaVersion int                                                                `json:"schemaVersion"`
	Recoveries    []providerRegistryPrivateLegacyMigrationRecoveryDescriptorResponse `json:"recoveries"`
}

type providerRegistryPrivateLegacyMigrationSourceAuthorityChallengeResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Challenge     string `json:"challenge"`
}

type providerRegistryPublicProvider struct {
	ID                   string                                    `json:"id"`
	Kind                 string                                    `json:"kind"`
	Endpoint             string                                    `json:"endpoint"`
	Proxy                string                                    `json:"proxy,omitempty"`
	Models               []string                                  `json:"models"`
	MediaModels          []string                                  `json:"mediaModels"`
	SelectedModel        string                                    `json:"selectedModel,omitempty"`
	SelectedMediaModel   string                                    `json:"selectedMediaModel,omitempty"`
	SelectedRoutes       []string                                  `json:"selectedRoutes"`
	CredentialConfigured bool                                      `json:"credentialConfigured"`
	CredentialPurpose    string                                    `json:"credentialPurpose,omitempty"`
	Revision             string                                    `json:"revision"`
	Generation           string                                    `json:"generation"`
	Incarnation          string                                    `json:"incarnation"`
	Tombstone            bool                                      `json:"tombstone"`
	OAuthBinding         *domainregistry.OAuthBindingMetadata      `json:"oauthBinding,omitempty"`
	AccountObservation   *domainregistry.AccountObservationBinding `json:"accountObservation,omitempty"`
}

type providerRegistryPublicResponse struct {
	SchemaVersion       int                               `json:"schemaVersion"`
	RegistryRevision    string                            `json:"registryRevision"`
	RegistryIncarnation string                            `json:"registryIncarnation"`
	SelectedProviderID  string                            `json:"selectedProviderId,omitempty"`
	Providers           *[]providerRegistryPublicProvider `json:"providers,omitempty"`
	Provider            *providerRegistryPublicProvider   `json:"provider,omitempty"`
	DeletedProviderID   string                            `json:"deletedProviderId,omitempty"`
	Recovered           bool                              `json:"recovered,omitempty"`
}

type providerRegistryProbeResponse struct {
	SchemaVersion       int    `json:"schemaVersion"`
	RegistryRevision    string `json:"registryRevision"`
	RegistryIncarnation string `json:"registryIncarnation"`
	ProviderID          string `json:"providerId"`
	ProviderRevision    string `json:"providerRevision"`
	ProviderGeneration  string `json:"providerGeneration"`
	ProviderIncarnation string `json:"providerIncarnation"`
	Status              string `json:"status"`
	Code                uint16 `json:"code,omitempty"`
	ModelCount          int    `json:"modelCount"`
	LatencyMillis       uint64 `json:"latencyMs"`
}

type providerRegistryAccountObservationResponse struct {
	SchemaVersion             int      `json:"schemaVersion"`
	RegistryRevision          string   `json:"registryRevision"`
	RegistryIncarnation       string   `json:"registryIncarnation"`
	ProviderID                string   `json:"providerId"`
	ProviderRevision          string   `json:"providerRevision"`
	ProviderGeneration        string   `json:"providerGeneration"`
	ProviderIncarnation       string   `json:"providerIncarnation"`
	ProviderCredentialPurpose string   `json:"providerCredentialPurpose"`
	Status                    string   `json:"status"`
	ObservedAt                string   `json:"observedAt"`
	ExpiresAt                 string   `json:"expiresAt"`
	Quota                     *float64 `json:"quota,omitempty"`
	Usage                     *float64 `json:"usage,omitempty"`
	Remaining                 *float64 `json:"remaining,omitempty"`
}

type providerRegistryPublicError struct {
	SchemaVersion int `json:"schemaVersion"`
	Error         struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type providerRegistryDecodeResult uint8

const (
	providerRegistryDecodeInvalid providerRegistryDecodeResult = iota
	providerRegistryDecodeOK
	providerRegistryDecodeTooLarge
)

func (handlers ProviderRegistryHandlers) Handle(w http.ResponseWriter, r *http.Request) {
	providerRegistryNoStore(w)
	path, ok := matchProviderRegistryPath(r)
	if !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if handlers.Service == nil {
		writeProviderRegistryFailure(w, http.StatusServiceUnavailable, "persistence_failure")
		return
	}
	switch path.operation {
	case providerRegistryPathCollection:
		switch r.Method {
		case http.MethodGet:
			handlers.handleList(w, r)
		case http.MethodPost:
			handlers.handleConnect(w, r)
		default:
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
		}
	case providerRegistryPathPortableManifest:
		switch r.Method {
		case http.MethodGet:
			handlers.handlePortableManifestExport(w, r)
		case http.MethodPost:
			handlers.handlePortableManifestImport(w, r)
		default:
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
		}
	case providerRegistryPathRecover:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleRecover(w, r)
	case providerRegistryPathProvider:
		switch r.Method {
		case http.MethodGet:
			handlers.handleGet(w, r, path.providerID)
		case http.MethodPatch:
			handlers.handleUpdate(w, r, path.providerID)
		case http.MethodDelete:
			handlers.handleDelete(w, r, path.providerID)
		default:
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
		}
	case providerRegistryPathSelect:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleSelect(w, r, path.providerID)
	case providerRegistryPathDisconnect:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleDisconnect(w, r, path.providerID)
	case providerRegistryPathCredential:
		if r.Method != http.MethodPut {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleCredentialReplace(w, r, path.providerID)
	case providerRegistryPathProbe:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleProbe(w, r, path.providerID)
	case providerRegistryPathCredentialCheck:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleCredentialCheck(w, r, path.providerID)
	case providerRegistryPathDiscoverModels:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleDiscoverModels(w, r, path.providerID)
	case providerRegistryPathObserveAccount:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handleObserveAccount(w, r, path.providerID)
	case providerRegistryPathPrivateLegacyMigrationPrepare:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationPrepare(w, r)
	case providerRegistryPathPrivateLegacyMigrationCommit:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationCommit(w, r)
	case providerRegistryPathPrivateLegacyMigrationAbandon:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationAbandon(w, r)
	case providerRegistryPathPrivateLegacyMigrationRollbackBegin:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationRollbackBegin(w, r)
	case providerRegistryPathPrivateLegacyMigrationRollbackCommit:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationRollbackCommit(w, r)
	case providerRegistryPathPrivateLegacyMigrationRemigrate:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationRemigrate(w, r)
	case providerRegistryPathPrivateLegacyMigrationProtectedDelete:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationProtectedDelete(w, r)
	case providerRegistryPathPrivateLegacyMigrationFinalize:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationFinalize(w, r)
	case providerRegistryPathPrivateLegacyMigrationInventory:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationInventory(w, r)
	case providerRegistryPathPrivateLegacyMigrationSourceAuthorityChallenge:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateLegacyMigrationSourceAuthorityChallenge(w, r)
	case providerRegistryPathPrivateAccountStatus:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateAccountStatus(w, r)
	case providerRegistryPathPrivateAccountList:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateAccountList(w, r)
	case providerRegistryPathPrivateAccountPut:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateAccountPut(w, r)
	case providerRegistryPathPrivateAccountResolve:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateAccountResolve(w, r)
	case providerRegistryPathPrivateAccountMutate:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateAccountMutate(w, r)
	case providerRegistryPathPrivateProtectedRecoveryPrepare:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateProtectedRecoveryPrepare(w, r)
	case providerRegistryPathPrivateProtectedRecoveryConfirmDestination:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateProtectedRecoveryConfirmDestination(w, r)
	case providerRegistryPathPrivateProtectedRecoveryCreateBundle:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateProtectedRecoveryCreateBundle(w, r)
	case providerRegistryPathPrivateProtectedRecoveryApply:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateProtectedRecoveryApply(w, r)
	case providerRegistryPathPrivateProtectedRecoveryFinalize:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateProtectedRecoveryFinalize(w, r)
	case providerRegistryPathPrivateProtectedRecoveryRecover:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateProtectedRecoveryRecover(w, r)
	case providerRegistryPathPrivateProtectedRecoveryRollback:
		if r.Method != http.MethodPost {
			writeProviderRegistryFailure(w, http.StatusMethodNotAllowed, "method_not_allowed")
			return
		}
		handlers.handlePrivateProtectedRecoveryRollback(w, r)
	default:
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
	}
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationCommit(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationCommitRequest
	if !requireProviderRegistryRequest(w, r, &request) {
		return
	}
	expected, expectedOK := request.Expected.parseLegacyMigration()
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	selectedProviderOK := request.ExpectedSelectedProviderID != nil &&
		(*request.ExpectedSelectedProviderID == "" || domainregistry.ValidProviderID(*request.ExpectedSelectedProviderID))
	sourceLocatorOK := request.SourceLocator == "" ||
		validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator))
	if request.SchemaVersion != 1 || !expectedOK || !selectedProviderOK ||
		!domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!sourceLocatorOK ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		secretstoreport.ValidateCredentialRef(ref) != nil ||
		request.Confirmation != providerregistryapp.LegacyMigrationCommitConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.CommitVerifiedLegacyMigrationRecovery(
		r.Context(),
		providerregistryapp.CommitVerifiedLegacyMigrationRecoveryCommand{
			Expected: expected, ExpectedSelectedProviderID: *request.ExpectedSelectedProviderID,
			MigrationID: request.MigrationID, SourceLocator: request.SourceLocator,
			SourceSHA256:          request.SourceSHA256,
			RecoveryCredentialRef: ref, Confirmation: request.Confirmation,
		},
	)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.Status != providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
		result.MigrationID != request.MigrationID || result.RecoveryCredentialRef != ref ||
		!result.SafeToRemoveLegacyPlaintext || result.Provider.Validate() != nil ||
		result.Provider.CredentialRef == "" || result.Provider.CredentialRef == string(ref) {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateLegacyMigrationCommitResponse{
		SchemaVersion: 1, Status: string(result.Status), MigrationID: result.MigrationID,
		SafeToRemoveLegacyPlaintext: result.SafeToRemoveLegacyPlaintext,
		Provider:                    projectProviderRegistryProvider(result.Provider),
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationPrepare(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationPrepareRequest
	var credential []byte
	var sourceSnapshot []byte
	var rollbackCredentialArtifacts []providerregistryapp.LegacyMigrationRollbackCredentialArtifact
	defer func() {
		clearProviderRegistryBytes(credential)
		if credential != nil && handlers.observeCredentialBufferCleared != nil {
			handlers.observeCredentialBufferCleared(credential)
		}
		clearProviderRegistryBytes(sourceSnapshot)
		if sourceSnapshot != nil && handlers.observeCredentialBufferCleared != nil {
			handlers.observeCredentialBufferCleared(sourceSnapshot)
		}
		for index := range rollbackCredentialArtifacts {
			clearProviderRegistryBytes(rollbackCredentialArtifacts[index].Credential)
			if rollbackCredentialArtifacts[index].Credential != nil && handlers.observeCredentialBufferCleared != nil {
				handlers.observeCredentialBufferCleared(rollbackCredentialArtifacts[index].Credential)
			}
			rollbackCredentialArtifacts[index].Credential = nil
		}
		handlers.clearPrivateLegacyMigrationPrepareRequest(&request)
	}()
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	var ok bool
	credential, ok = decodeProviderRegistryCanonicalBase64(request.Credential.ValueBase64)
	if !ok || providerRegistryCredentialIsSentinel(credential) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if len(request.RollbackSource.ValueBase64) != 0 {
		sourceSnapshot, ok = decodeProviderRegistryCanonicalBase64(request.RollbackSource.ValueBase64)
		if !ok || len(sourceSnapshot) > providerRegistryMaxLegacyMigrationSourceBytes {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
			return
		}
		sourceDigest := sha256.Sum256(sourceSnapshot)
		if hex.EncodeToString(sourceDigest[:]) != request.SourceSHA256 {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	expected, expectedOK := request.Expected.parseLegacyMigration()
	purpose, purposeErr := secretstoreport.NormalizePurpose(request.Credential.Purpose)
	sourceLocatorOK := validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator)) &&
		len(sourceSnapshot) > 0 && len(request.ActiveCredentialLocators) > 0
	if request.SchemaVersion != 1 || !expectedOK ||
		!domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!sourceLocatorOK ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.ExpectedCleanedSourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.SourcePhysicalIdentitySHA256) ||
		request.Provider.Validate() != nil || purposeErr != nil || string(purpose) != request.Credential.Purpose {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	rollbackCredentialArtifacts, ok = decodeProviderRegistryLegacyMigrationRollbackArtifacts(
		credential,
		request.SourceLocator,
		request.ActiveCredentialLocators,
		request.RollbackCredentialArtifacts,
	)
	if !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.PrepareLegacyMigrationRecovery(r.Context(), providerregistryapp.LegacyMigrationCandidate{
		Expected: expected, MigrationID: request.MigrationID, SourceLocator: request.SourceLocator,
		SourceSHA256: request.SourceSHA256, ExpectedCleanedSourceSHA256: request.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: request.SourcePhysicalIdentitySHA256,
		Provider:                     request.Provider, CredentialPurpose: purpose, Credential: credential,
		SourceSnapshot:              sourceSnapshot,
		ActiveCredentialLocators:    append([]string(nil), request.ActiveCredentialLocators...),
		RollbackCredentialArtifacts: rollbackCredentialArtifacts,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	verifiedRecovery := result.Status == providerregistryapp.LegacyMigrationRecoveryStatusVerified &&
		result.SafeToProceedWithProviderMigration
	committedRetainedRecovery :=
		result.Status == providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained &&
			!result.SafeToProceedWithProviderMigration
	if (!verifiedRecovery && !committedRetainedRecovery) ||
		result.MigrationID != request.MigrationID ||
		secretstoreport.ValidateCredentialRef(result.RecoveryCredentialRef) != nil {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateLegacyMigrationPrepareResponse{
		SchemaVersion: 1, Status: string(result.Status), MigrationID: result.MigrationID,
		RecoveryCredentialRef:              string(result.RecoveryCredentialRef),
		SafeToProceedWithProviderMigration: result.SafeToProceedWithProviderMigration,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationAbandon(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationAbandonRequest
	if !requireProviderRegistryRequest(w, r, &request) {
		return
	}
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	if request.SchemaVersion != 1 || !domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		(request.SourceLocator != "" &&
			!validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator))) ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		secretstoreport.ValidateCredentialRef(ref) != nil ||
		request.Confirmation != providerregistryapp.LegacyMigrationAbandonConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := handlers.Service.AbandonLegacyMigrationRecovery(r.Context(), providerregistryapp.AbandonLegacyMigrationRecoveryCommand{
		MigrationID: request.MigrationID, SourceLocator: request.SourceLocator,
		SourceSHA256:          request.SourceSHA256,
		RecoveryCredentialRef: ref, Confirmation: request.Confirmation,
	}); err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateLegacyMigrationAbandonResponse{
		SchemaVersion: 1, Status: "COMPLETED",
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationRollbackBegin(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationRollbackBeginRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	if request.SchemaVersion != 1 || !domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator)) ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		(request.SourcePhysicalIdentitySHA256 != "" &&
			!validProviderRegistryLowerSHA256(request.SourcePhysicalIdentitySHA256)) ||
		secretstoreport.ValidateCredentialRef(ref) != nil ||
		request.Confirmation != providerregistryapp.LegacyMigrationRollbackConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.BeginLegacyMigrationRollback(
		r.Context(),
		providerregistryapp.BeginLegacyMigrationRollbackCommand{
			MigrationID: request.MigrationID, SourceLocator: request.SourceLocator,
			SourceSHA256:                 request.SourceSHA256,
			SourcePhysicalIdentitySHA256: request.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef:        ref, Confirmation: request.Confirmation,
		},
	)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.MigrationID != request.MigrationID {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	response := providerRegistryPrivateLegacyMigrationRollbackBeginResponse{
		SchemaVersion: 1, Status: string(result.Status), MigrationID: result.MigrationID,
	}
	switch result.Status {
	case providerregistryapp.LegacyMigrationRollbackStatusCleanedSourceRequired,
		providerregistryapp.LegacyMigrationRollbackStatusCommittedRecoveryRetained,
		providerregistryapp.LegacyMigrationRollbackStatusRecoveryRetainedTerminal:
	case providerregistryapp.LegacyMigrationRollbackStatusPreCommitCompleted,
		providerregistryapp.LegacyMigrationRollbackStatusAlreadyFinalized:
	default:
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, response)
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationRollbackCommit(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationRollbackCommitRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	verifiedCleanedSource, ok := decodeProviderRegistryCanonicalBase64(request.VerifiedCleanedSource.ValueBase64)
	defer clearProviderRegistryBytes(verifiedCleanedSource)
	if !ok || len(verifiedCleanedSource) == 0 || len(verifiedCleanedSource) > providerRegistryMaxLegacyMigrationSourceBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	verifiedCleanedDigest := sha256.Sum256(verifiedCleanedSource)
	expected, expectedOK := request.Expected.parseExisting()
	sourceAuthority, sourceAuthorityOK := parseProviderRegistryLegacyMigrationSourceAuthorityProof(
		request.SourceAuthority,
	)
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	if request.SchemaVersion != 1 || !expectedOK || !sourceAuthorityOK ||
		!domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator)) ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.VerifiedCleanedSourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.SourcePhysicalIdentitySHA256) ||
		hex.EncodeToString(verifiedCleanedDigest[:]) != request.VerifiedCleanedSourceSHA256 ||
		secretstoreport.ValidateCredentialRef(ref) != nil ||
		request.Confirmation != providerregistryapp.LegacyMigrationRollbackCommitConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.CommitLegacyMigrationRollback(
		r.Context(),
		providerregistryapp.CommitLegacyMigrationRollbackCommand{
			Expected: expected, MigrationID: request.MigrationID,
			SourceLocator: request.SourceLocator, SourceSHA256: request.SourceSHA256,
			VerifiedCleanedSourceSHA256:  request.VerifiedCleanedSourceSHA256,
			SourcePhysicalIdentitySHA256: request.SourcePhysicalIdentitySHA256,
			VerifiedCleanedSource:        verifiedCleanedSource,
			SourceAuthority:              sourceAuthority,
			RecoveryCredentialRef:        ref, Confirmation: request.Confirmation,
		},
	)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.MigrationID != request.MigrationID ||
		(result.Status != providerregistryapp.LegacyMigrationRollbackStatusCommittedRecoveryRetained &&
			result.Status != providerregistryapp.LegacyMigrationRollbackStatusAlreadyFinalized) {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateLegacyMigrationRollbackCommitResponse{
		SchemaVersion: 1, Status: string(result.Status), MigrationID: result.MigrationID,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationRemigrate(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationSourceActionRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	verifiedCleanedSource, ok := decodeProviderRegistryCanonicalBase64(request.VerifiedCleanedSource.ValueBase64)
	defer clearProviderRegistryBytes(verifiedCleanedSource)
	if !ok || len(verifiedCleanedSource) == 0 ||
		len(verifiedCleanedSource) > providerRegistryMaxLegacyMigrationSourceBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	verifiedCleanedDigest := sha256.Sum256(verifiedCleanedSource)
	expected, expectedOK := request.Expected.parseLegacyMigration()
	sourceAuthority, sourceAuthorityOK := parseProviderRegistryLegacyMigrationSourceAuthorityProof(
		request.SourceAuthority,
	)
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	if request.SchemaVersion != 1 || !expectedOK || !sourceAuthorityOK ||
		!domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator)) ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.VerifiedCleanedSourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.SourcePhysicalIdentitySHA256) ||
		hex.EncodeToString(verifiedCleanedDigest[:]) != request.VerifiedCleanedSourceSHA256 ||
		secretstoreport.ValidateCredentialRef(ref) != nil ||
		request.Confirmation != providerregistryapp.LegacyMigrationRemigrationConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.RemigrateRetainedLegacyMigrationRecovery(
		r.Context(),
		providerregistryapp.RemigrateRetainedLegacyMigrationRecoveryCommand{
			Expected: expected, MigrationID: request.MigrationID,
			SourceLocator: request.SourceLocator, SourceSHA256: request.SourceSHA256,
			VerifiedCleanedSourceSHA256:  request.VerifiedCleanedSourceSHA256,
			SourcePhysicalIdentitySHA256: request.SourcePhysicalIdentitySHA256,
			VerifiedCleanedSource:        verifiedCleanedSource,
			SourceAuthority:              sourceAuthority,
			RecoveryCredentialRef:        ref, Confirmation: request.Confirmation,
		},
	)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.MigrationID != request.MigrationID ||
		result.Status != providerregistryapp.LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained ||
		result.Provider.Validate() != nil || result.Provider.Tombstone || result.Provider.CredentialRef == "" {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateLegacyMigrationCommitResponse{
		SchemaVersion: 1, Status: string(result.Status), MigrationID: result.MigrationID,
		SafeToRemoveLegacyPlaintext: false,
		Provider:                    projectProviderRegistryProvider(result.Provider),
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationProtectedDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request providerRegistryPrivateLegacyMigrationSourceActionRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	verifiedCleanedSource, ok := decodeProviderRegistryCanonicalBase64(request.VerifiedCleanedSource.ValueBase64)
	defer clearProviderRegistryBytes(verifiedCleanedSource)
	if !ok || len(verifiedCleanedSource) == 0 ||
		len(verifiedCleanedSource) > providerRegistryMaxLegacyMigrationSourceBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	verifiedCleanedDigest := sha256.Sum256(verifiedCleanedSource)
	expected, expectedOK := request.Expected.parseLegacyMigration()
	sourceAuthority, sourceAuthorityOK := parseProviderRegistryLegacyMigrationSourceAuthorityProof(
		request.SourceAuthority,
	)
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	if request.SchemaVersion != 1 || !expectedOK || !sourceAuthorityOK ||
		!domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator)) ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.VerifiedCleanedSourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.SourcePhysicalIdentitySHA256) ||
		hex.EncodeToString(verifiedCleanedDigest[:]) != request.VerifiedCleanedSourceSHA256 ||
		secretstoreport.ValidateCredentialRef(ref) != nil ||
		request.Confirmation != providerregistryapp.LegacyMigrationProtectedDeleteConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.DeleteRetainedLegacyMigrationRecovery(
		r.Context(),
		providerregistryapp.DeleteRetainedLegacyMigrationRecoveryCommand{
			Expected: expected, MigrationID: request.MigrationID,
			SourceLocator: request.SourceLocator, SourceSHA256: request.SourceSHA256,
			VerifiedCleanedSourceSHA256:  request.VerifiedCleanedSourceSHA256,
			SourcePhysicalIdentitySHA256: request.SourcePhysicalIdentitySHA256,
			VerifiedCleanedSource:        verifiedCleanedSource,
			SourceAuthority:              sourceAuthority,
			RecoveryCredentialRef:        ref, Confirmation: request.Confirmation,
		},
	)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.MigrationID != request.MigrationID ||
		(result.Status != providerregistryapp.LegacyMigrationProtectedDeleteStatusCompleted &&
			result.Status != providerregistryapp.LegacyMigrationProtectedDeleteStatusAlreadyDeleted) {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateLegacyMigrationProtectedDeleteResponse{
		SchemaVersion: 1, Status: string(result.Status), MigrationID: result.MigrationID,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationFinalize(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationFinalizeRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	verifiedSource, ok := decodeProviderRegistryCanonicalBase64(request.VerifiedSource.ValueBase64)
	defer clearProviderRegistryBytes(verifiedSource)
	if !ok || len(verifiedSource) == 0 || len(verifiedSource) > providerRegistryMaxLegacyMigrationSourceBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	verifiedDigest := sha256.Sum256(verifiedSource)
	sourceAuthority, sourceAuthorityOK := parseProviderRegistryLegacyMigrationSourceAuthorityProof(
		request.SourceAuthority,
	)
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	if request.SchemaVersion != 1 || !sourceAuthorityOK ||
		!domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator)) ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.VerifiedSourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.SourcePhysicalIdentitySHA256) ||
		hex.EncodeToString(verifiedDigest[:]) != request.VerifiedSourceSHA256 ||
		(request.RecoveryCredentialRef != "" && secretstoreport.ValidateCredentialRef(ref) != nil) ||
		request.Confirmation != providerregistryapp.LegacyMigrationFinalizeConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.FinalizeLegacyMigrationRecovery(
		r.Context(),
		providerregistryapp.FinalizeLegacyMigrationRecoveryCommand{
			MigrationID: request.MigrationID, SourceLocator: request.SourceLocator,
			SourceSHA256:                 request.SourceSHA256,
			VerifiedSourceSHA256:         request.VerifiedSourceSHA256,
			SourcePhysicalIdentitySHA256: request.SourcePhysicalIdentitySHA256,
			VerifiedSource:               verifiedSource,
			SourceAuthority:              sourceAuthority,
			RecoveryCredentialRef:        ref, Confirmation: request.Confirmation,
		},
	)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.MigrationID != request.MigrationID ||
		(result.Status != providerregistryapp.LegacyMigrationFinalizationStatusCompleted &&
			result.Status != providerregistryapp.LegacyMigrationFinalizationStatusAlreadyFinalized) ||
		(result.Outcome != providerregistryapp.LegacyMigrationFinalizationOutcomeCommitted &&
			result.Outcome != providerregistryapp.LegacyMigrationFinalizationOutcomeRolledBack) {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateLegacyMigrationFinalizeResponse{
		SchemaVersion: 1, Status: string(result.Status), Outcome: string(result.Outcome),
		MigrationID: result.MigrationID,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationInventory(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateLegacyMigrationInventoryRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	if request.SchemaVersion != 1 ||
		request.Confirmation != providerregistryapp.LegacyMigrationRecoveryInventoryConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.InventoryLegacyMigrationRecoveries(r.Context())
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	response := providerRegistryPrivateLegacyMigrationInventoryResponse{
		SchemaVersion: 1,
		Recoveries:    make([]providerRegistryPrivateLegacyMigrationRecoveryDescriptorResponse, 0, len(result.Recoveries)),
	}
	seen := make(map[string]struct{}, len(result.Recoveries))
	for _, recovery := range result.Recoveries {
		phaseOK := false
		switch recovery.Phase {
		case domainregistry.LegacyMigrationRecoveryPhasePrepared,
			domainregistry.LegacyMigrationRecoveryPhaseSecretDurable,
			domainregistry.LegacyMigrationRecoveryPhaseVerified,
			domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared,
			domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained,
			domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending,
			domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending,
			domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained,
			domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained,
			domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending,
			domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted,
			domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback:
			phaseOK = true
		}
		if !phaseOK || !domainregistry.ValidLegacyMigrationID(recovery.MigrationID) ||
			!domainregistry.ValidProviderID(recovery.ProviderID) ||
			!validProviderRegistryLegacyMigrationSourceLocator([]byte(recovery.SourceLocator)) ||
			!validProviderRegistryLowerSHA256(recovery.SourceSHA256) ||
			!validProviderRegistryLowerSHA256(recovery.ExpectedCleanedSourceSHA256) ||
			!validProviderRegistryLowerSHA256(recovery.SourcePhysicalIdentitySHA256) ||
			secretstoreport.ValidateCredentialRef(recovery.RecoveryCredentialRef) != nil {
			writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
			return
		}
		key := recovery.MigrationID + "\x00" + recovery.ProviderID + "\x00" + string(recovery.RecoveryCredentialRef)
		if _, duplicate := seen[key]; duplicate {
			writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
			return
		}
		seen[key] = struct{}{}
		response.Recoveries = append(response.Recoveries,
			providerRegistryPrivateLegacyMigrationRecoveryDescriptorResponse{
				MigrationID: recovery.MigrationID, ProviderID: recovery.ProviderID,
				SourceLocator: recovery.SourceLocator, SourceSHA256: recovery.SourceSHA256,
				ExpectedCleanedSourceSHA256:  recovery.ExpectedCleanedSourceSHA256,
				SourcePhysicalIdentitySHA256: recovery.SourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        string(recovery.RecoveryCredentialRef), Phase: string(recovery.Phase),
				CommitOrder: strconv.FormatUint(recovery.CommitOrder, 10),
			})
	}
	writeProviderRegistryJSON(w, http.StatusOK, response)
}

func (handlers ProviderRegistryHandlers) handlePrivateLegacyMigrationSourceAuthorityChallenge(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request providerRegistryPrivateLegacyMigrationSourceAuthorityChallengeRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	ref := secretstoreport.CredentialRef(request.RecoveryCredentialRef)
	if request.SchemaVersion != 1 ||
		(request.Operation != providerregistryapp.LegacyMigrationSourceAuthorityOperationRollbackCommit &&
			request.Operation != providerregistryapp.LegacyMigrationSourceAuthorityOperationFinalize &&
			request.Operation != providerregistryapp.LegacyMigrationSourceAuthorityOperationRemigrate &&
			request.Operation != providerregistryapp.LegacyMigrationSourceAuthorityOperationProtectedDelete) ||
		!domainregistry.ValidLegacyMigrationID(request.MigrationID) ||
		!validProviderRegistryLegacyMigrationSourceLocator([]byte(request.SourceLocator)) ||
		!validProviderRegistryLowerSHA256(request.SourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.CurrentSourceSHA256) ||
		!validProviderRegistryLowerSHA256(request.SourcePhysicalIdentitySHA256) ||
		secretstoreport.ValidateCredentialRef(ref) != nil ||
		request.Confirmation != providerregistryapp.LegacyMigrationSourceAuthorityChallengeConfirmation {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.IssueLegacyMigrationSourceAuthorityChallenge(
		r.Context(),
		providerregistryapp.IssueLegacyMigrationSourceAuthorityChallengeCommand{
			Operation: request.Operation, MigrationID: request.MigrationID,
			SourceLocator: request.SourceLocator, SourceSHA256: request.SourceSHA256,
			CurrentSourceSHA256:          request.CurrentSourceSHA256,
			SourcePhysicalIdentitySHA256: request.SourcePhysicalIdentitySHA256,
			RecoveryCredentialRef:        ref, Confirmation: request.Confirmation,
		},
	)
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if !validProviderRegistryLegacyMigrationSourceAuthorityChallenge(result.Challenge) {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK,
		providerRegistryPrivateLegacyMigrationSourceAuthorityChallengeResponse{
			SchemaVersion: 1, Challenge: result.Challenge,
		},
	)
}

func (handlers ProviderRegistryHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	if !providerRegistryNoBody(r) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	snapshot, err := handlers.Service.Snapshot(r.Context())
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, projectProviderRegistrySnapshot(snapshot))
}

func (handlers ProviderRegistryHandlers) handlePrivateProtectedRecoveryPrepare(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateProtectedRecoveryPrepareRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	manifestBytes, ok := decodeProviderRegistryCanonicalBase64(request.ManifestBase64)
	defer clearProviderRegistryBytes(manifestBytes)
	if request.SchemaVersion != 1 || !ok || len(manifestBytes) == 0 ||
		len(manifestBytes) > domainregistry.PortableManifestMaxBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	manifest, err := domainregistry.ParsePortableManifestV1(manifestBytes)
	if err != nil || !validProtectedRecoveryImportResult(request.ImportResult, manifest) ||
		!validProtectedRecoveryOwnerInventoryHTTP(request.OwnerBindingInventory) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	prepared, err := handlers.Service.PrepareProtectedRecoveryRequest(r.Context(), providerregistryapp.ProtectedRecoveryRequestCommand{
		ManifestJSON:          manifestBytes,
		ImportResult:          request.ImportResult,
		LocalBinding:          request.LocalBinding,
		OwnerBindingInventory: request.OwnerBindingInventory,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	parsed, parseErr := domainregistry.ParseProtectedRecoveryRequestV1(prepared.Request)
	if parseErr != nil || prepared.RequestDigest != parsed.RequestDigest ||
		prepared.RequestFingerprint != parsed.VerificationFingerprint ||
		prepared.ManifestDigest != parsed.ManifestDigest ||
		prepared.OperationID != parsed.OperationID || prepared.SessionNonce != parsed.SessionNonce ||
		prepared.ExpiresAt != parsed.ExpiresAt ||
		prepared.ItemSetDigest != domainregistry.ProtectedRecoveryItemSetDigest(parsed.Entries) {
		clearProviderRegistryBytes(prepared.Request)
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	defer clearProviderRegistryBytes(prepared.Request)
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateProtectedRecoveryPrepareResponse{
		SchemaVersion: 1, RequestBase64: prepared.Request,
		RequestDigest: prepared.RequestDigest, RequestFingerprint: prepared.RequestFingerprint,
		ManifestDigest: prepared.ManifestDigest, ItemSetDigest: prepared.ItemSetDigest,
		OperationID: prepared.OperationID, SessionNonce: prepared.SessionNonce, ExpiresAt: prepared.ExpiresAt,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateProtectedRecoveryConfirmDestination(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateProtectedRecoveryConfirmRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	requestBytes, ok := decodeProviderRegistryCanonicalBase64(request.RequestBase64)
	defer clearProviderRegistryBytes(requestBytes)
	if request.SchemaVersion != 1 || !ok || len(requestBytes) == 0 ||
		len(requestBytes) > domainregistry.ProtectedRecoveryMaxBytes ||
		request.LocalAction.Validate() != nil || !validProtectedRecoveryContinuationOwnerInventoryHTTP(request.OwnerBindingInventory) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := domainregistry.ParseProtectedRecoveryRequestV1(requestBytes); err != nil {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := handlers.Service.ConfirmProtectedRecoveryDestination(r.Context(), providerregistryapp.ProtectedRecoveryDestinationConfirmationCommand{
		Request: requestBytes, LocalAction: request.LocalAction,
		OwnerBindingInventory: request.OwnerBindingInventory,
	}); err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateProtectedRecoveryStatusResponse{
		SchemaVersion: 1, Status: "confirmed", Confirmed: true,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateProtectedRecoveryCreateBundle(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateProtectedRecoveryCreateBundleRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	requestBytes, ok := decodeProviderRegistryCanonicalBase64(request.RequestBase64)
	defer clearProviderRegistryBytes(requestBytes)
	if request.SchemaVersion != 1 || !ok || len(requestBytes) == 0 ||
		len(requestBytes) > domainregistry.ProtectedRecoveryMaxBytes ||
		request.LocalAction.Validate() != nil || !validProtectedRecoveryOwnerInventoryHTTP(request.OwnerBindingInventory) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := domainregistry.ParseProtectedRecoveryRequestV1(requestBytes); err != nil {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	bundle, err := handlers.Service.CreateProtectedRecoveryBundle(r.Context(), providerregistryapp.ProtectedRecoveryBundleCommand{
		Request: requestBytes, LocalAction: request.LocalAction,
		OwnerBindingInventory: request.OwnerBindingInventory,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if _, err := domainregistry.ParseProtectedRecoveryBundleV1(bundle); err != nil {
		clearProviderRegistryBytes(bundle)
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	defer clearProviderRegistryBytes(bundle)
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateProtectedRecoveryBundleResponse{
		SchemaVersion: 1, BundleBase64: bundle,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateProtectedRecoveryApply(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateProtectedRecoveryApplyRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	bundleBytes, bundleOK := decodeProviderRegistryCanonicalBase64(request.BundleBase64)
	requestBytes := []byte(nil)
	requestOK := len(request.RequestBase64) == 0
	if len(request.RequestBase64) != 0 {
		requestBytes, requestOK = decodeProviderRegistryCanonicalBase64(request.RequestBase64)
	}
	defer clearProviderRegistryBytes(bundleBytes)
	defer clearProviderRegistryBytes(requestBytes)
	if request.SchemaVersion != 1 || !bundleOK || len(bundleBytes) == 0 ||
		len(bundleBytes) > domainregistry.ProtectedRecoveryMaxBytes || !requestOK ||
		len(requestBytes) > domainregistry.ProtectedRecoveryMaxBytes ||
		!validProtectedRecoveryContinuationOwnerInventoryHTTP(request.OwnerBindingInventory) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := domainregistry.ParseProtectedRecoveryBundleV1(bundleBytes); err != nil {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if len(request.RequestBase64) != 0 {
		if _, err := domainregistry.ParseProtectedRecoveryRequestV1(requestBytes); err != nil {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	receipt, err := handlers.Service.ApplyProtectedRecoveryBundle(r.Context(), providerregistryapp.ProtectedRecoveryApplyCommand{
		Bundle: bundleBytes, Request: requestBytes,
		OwnerBindingInventory: request.OwnerBindingInventory,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	parsed, err := domainregistry.ParseProtectedRecoveryReceiptV1(receipt)
	if err != nil {
		clearProviderRegistryBytes(receipt)
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	for _, entry := range parsed.Entries {
		if entry.Status != "applied" {
			clearProviderRegistryBytes(receipt)
			writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
			return
		}
	}
	defer clearProviderRegistryBytes(receipt)
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateProtectedRecoveryReceiptResponse{
		SchemaVersion: 1, ReceiptBase64: receipt,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateProtectedRecoveryFinalize(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateProtectedRecoveryFinalizeRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	receiptBytes, ok := decodeProviderRegistryCanonicalBase64(request.ReceiptBase64)
	defer clearProviderRegistryBytes(receiptBytes)
	if request.SchemaVersion != 1 || !ok || len(receiptBytes) == 0 ||
		len(receiptBytes) > domainregistry.ProtectedRecoveryMaxBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := domainregistry.ParseProtectedRecoveryReceiptV1(receiptBytes); err != nil {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.FinalizeProtectedRecoveryReceipt(r.Context(), providerregistryapp.ProtectedRecoveryFinalizeCommand{
		Receipt: receiptBytes,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.Status != "finalized" {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateProtectedRecoveryStatusResponse{
		SchemaVersion: 1, Status: result.Status,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateProtectedRecoveryRecover(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateProtectedRecoveryRecoverRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	requestBytes, requestOK := decodeProviderRegistryCanonicalBase64(request.RequestBase64)
	defer clearProviderRegistryBytes(requestBytes)
	if request.SchemaVersion != 1 || (!requestOK && len(request.RequestBase64) != 0) ||
		len(requestBytes) > domainregistry.ProtectedRecoveryMaxBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var (
		result domainregistry.ProtectedRecoveryResultV1
		err    error
	)
	if len(requestBytes) == 0 {
		if request.LocalAction != nil || len(request.OwnerBindingInventory) != 0 {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
			return
		}
		result, err = handlers.Service.RecoverProtectedRecovery(r.Context())
	} else {
		if request.LocalAction == nil || request.LocalAction.Validate() != nil ||
			!validProtectedRecoveryContinuationOwnerInventoryHTTP(request.OwnerBindingInventory) {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
			return
		}
		if _, parseErr := domainregistry.ParseProtectedRecoveryRequestV1(requestBytes); parseErr != nil {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
			return
		}
		result, err = handlers.Service.RecoverProtectedRecovery(r.Context(), providerregistryapp.ProtectedRecoveryRecoverCommand{
			Request: requestBytes, LocalAction: *request.LocalAction,
			OwnerBindingInventory: request.OwnerBindingInventory,
		})
	}
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.Status != "none" && result.Status != "pending" &&
		result.Status != "reconfirmation_required" && result.Status != "receipt_ready" {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	if result.Status == "receipt_ready" {
		if !result.Confirmed || len(result.Receipt) == 0 {
			writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
			return
		}
		if _, parseErr := domainregistry.ParseProtectedRecoveryReceiptV1(result.Receipt); parseErr != nil {
			clearProviderRegistryBytes(result.Receipt)
			writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
			return
		}
	}
	defer clearProviderRegistryBytes(result.Receipt)
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateProtectedRecoveryStatusResponse{
		SchemaVersion: 1, Status: result.Status, Confirmed: result.Confirmed, ReceiptBase64: result.Receipt,
	})
}

func (handlers ProviderRegistryHandlers) handlePrivateProtectedRecoveryRollback(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPrivateProtectedRecoveryRollbackRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	requestBytes, ok := decodeProviderRegistryCanonicalBase64(request.RequestBase64)
	defer clearProviderRegistryBytes(requestBytes)
	if request.SchemaVersion != 1 || !ok || len(requestBytes) == 0 ||
		len(requestBytes) > domainregistry.ProtectedRecoveryMaxBytes {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := domainregistry.ParseProtectedRecoveryRequestV1(requestBytes); err != nil {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.RollbackProtectedRecovery(r.Context(), providerregistryapp.ProtectedRecoveryRollbackCommand{
		Request: requestBytes,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.Status != "rolled_back" {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPrivateProtectedRecoveryStatusResponse{
		SchemaVersion: 1, Status: result.Status,
	})
}

func (handlers ProviderRegistryHandlers) handlePortableManifestExport(w http.ResponseWriter, r *http.Request) {
	if !providerRegistryNoBody(r) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	manifestJSON, err := handlers.Service.ExportPortableManifest(r.Context())
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	defer clearProviderRegistryBytes(manifestJSON)
	if len(manifestJSON) > domainregistry.PortableManifestMaxBytes ||
		!utf8.Valid(manifestJSON) {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	if _, err := domainregistry.ParsePortableManifestV1(manifestJSON); err != nil {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPortableManifestExportResponse{
		SchemaVersion: 1, ManifestJSON: string(manifestJSON),
	})
}

func (handlers ProviderRegistryHandlers) handlePortableManifestImport(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryPortableManifestImportRequest
	if !requireProviderRegistryUniqueRequest(w, r, &request) {
		return
	}
	if request.SchemaVersion != 1 || len(request.ManifestJSON) == 0 ||
		len(request.ManifestJSON) > domainregistry.PortableManifestMaxBytes ||
		!utf8.ValidString(request.ManifestJSON) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	manifest, err := domainregistry.ParsePortableManifestV1([]byte(request.ManifestJSON))
	if err != nil {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	result, err := handlers.Service.ImportPortableManifest(r.Context(), []byte(request.ManifestJSON))
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.Entries == nil {
		result.Entries = make([]providerregistryapp.PortableImportEntryResult, 0)
	}
	if !validPortableManifestImportResult(result, len(manifest.Providers), len(manifest.Accounts)) {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPortableManifestImportResponse{
		SchemaVersion: 1, ProviderCount: result.ProviderCount, AccountCount: result.AccountCount,
		ReentryRequired: result.ReentryRequired, Entries: result.Entries,
	})
}

func validPortableManifestImportResult(result providerregistryapp.PortableImportResult, providerCount, accountCount int) bool {
	entryCount := providerCount + accountCount
	if result.ProviderCount != providerCount || result.AccountCount != accountCount ||
		result.ReentryRequired != entryCount || len(result.Entries) != entryCount ||
		entryCount > domainregistry.MaxProviders {
		return false
	}
	seen := make(map[string]struct{}, len(result.Entries))
	seenDestinationIDs := make(map[string]struct{}, len(result.Entries))
	for index, entry := range result.Entries {
		expectedCorrelation := "account-" + strconv.Itoa(index-providerCount)
		if index < providerCount {
			expectedCorrelation = "provider-" + strconv.Itoa(index)
		}
		if !validPortableManifestCorrelation(entry.Correlation) ||
			!domainregistry.ValidProviderID(entry.DestinationProviderID) ||
			entry.Status != domainregistry.PortableManifestIntentReentryRequired ||
			entry.Correlation != expectedCorrelation {
			return false
		}
		if _, duplicate := seen[entry.Correlation]; duplicate {
			return false
		}
		seen[entry.Correlation] = struct{}{}
		if _, duplicate := seenDestinationIDs[entry.DestinationProviderID]; duplicate {
			return false
		}
		seenDestinationIDs[entry.DestinationProviderID] = struct{}{}
	}
	return len(seen) == entryCount
}

func validProtectedRecoveryImportResult(
	result providerregistryapp.ProtectedRecoveryImportResult,
	manifest domainregistry.PortableManifestV1,
) bool {
	providerCount := len(manifest.Providers)
	accountCount := len(manifest.Accounts)
	entryCount := providerCount + accountCount
	if result.ProviderCount != providerCount || result.AccountCount != accountCount ||
		result.ReentryRequired != entryCount || len(result.Entries) != entryCount ||
		entryCount > domainregistry.MaxProviders {
		return false
	}
	seen := make(map[string]struct{}, len(result.Entries))
	seenDestinationIDs := make(map[string]struct{}, len(result.Entries))
	for index, entry := range result.Entries {
		expectedCorrelation := "account-" + strconv.Itoa(index-providerCount)
		if index < providerCount {
			expectedCorrelation = "provider-" + strconv.Itoa(index)
		}
		revision, revisionErr := strconv.ParseUint(entry.Fence.Revision, 10, 64)
		generation, generationErr := strconv.ParseUint(entry.Fence.Generation, 10, 64)
		if !validPortableManifestCorrelation(entry.Correlation) ||
			!domainregistry.ValidProviderID(entry.DestinationProviderID) ||
			entry.Status != domainregistry.PortableManifestIntentReentryRequired ||
			entry.Correlation != expectedCorrelation ||
			revisionErr != nil || generationErr != nil || revision == 0 || generation == 0 ||
			strconv.FormatUint(revision, 10) != entry.Fence.Revision ||
			strconv.FormatUint(generation, 10) != entry.Fence.Generation ||
			!domainregistry.ValidIncarnation(entry.Fence.Incarnation) {
			return false
		}
		if index < providerCount {
			if entry.DestinationOwnerBinding != nil {
				return false
			}
		} else {
			descriptor := manifest.Accounts[index-providerCount]
			binding := entry.DestinationOwnerBinding
			if binding == nil || binding.Validate() != nil || binding.Correlation != entry.Correlation ||
				binding.Owner != descriptor.Owner || binding.Provider != descriptor.Provider || binding.Purpose != descriptor.Purpose {
				return false
			}
		}
		if _, duplicate := seen[entry.Correlation]; duplicate {
			return false
		}
		seen[entry.Correlation] = struct{}{}
		if _, duplicate := seenDestinationIDs[entry.DestinationProviderID]; duplicate {
			return false
		}
		seenDestinationIDs[entry.DestinationProviderID] = struct{}{}
	}
	return len(seen) == entryCount
}

func validProtectedRecoveryOwnerInventoryHTTP(
	owners []domainregistry.ProtectedRecoveryOwnerBindingV1,
) bool {
	return validProtectedRecoveryOwnerInventoryForOperationHTTP(owners, false)
}

func validProtectedRecoveryContinuationOwnerInventoryHTTP(
	owners []domainregistry.ProtectedRecoveryOwnerBindingV1,
) bool {
	return validProtectedRecoveryOwnerInventoryForOperationHTTP(owners, true)
}

func validProtectedRecoveryOwnerInventoryForOperationHTTP(
	owners []domainregistry.ProtectedRecoveryOwnerBindingV1,
	allowMissingCorrelation bool,
) bool {
	if len(owners) > domainregistry.ProtectedRecoveryMaxBindingEntries {
		return false
	}
	seen := make(map[string]struct{}, len(owners))
	seenBindings := make(map[string]struct{}, len(owners))
	for _, owner := range owners {
		validated := owner
		if allowMissingCorrelation && validated.Correlation == "" {
			validated.Correlation = "account-0"
		}
		if validated.Validate() != nil {
			return false
		}
		bindingKey := owner.Owner + "\x00" + owner.Provider + "\x00" + owner.AccountID + "\x00" + owner.ChannelID + "\x00" + owner.Purpose
		if _, duplicate := seenBindings[bindingKey]; duplicate {
			return false
		}
		seenBindings[bindingKey] = struct{}{}
		if owner.Correlation != "" {
			if _, duplicate := seen[owner.Correlation]; duplicate {
				return false
			}
			seen[owner.Correlation] = struct{}{}
		}
	}
	return true
}

func validPortableManifestCorrelation(value string) bool {
	prefix := ""
	for _, candidate := range []string{"provider-", "account-"} {
		if strings.HasPrefix(value, candidate) {
			prefix = candidate
			break
		}
	}
	if len(prefix) == 0 || len(value) < len(prefix)+1 || len(value) > 96 || !utf8.ValidString(value) {
		return false
	}
	for index, current := range []byte(value) {
		if index < len(prefix) {
			if current != prefix[index] {
				return false
			}
			continue
		}
		if (current < 'a' || current > 'z') && (current < '0' || current > '9') && current != '-' {
			return false
		}
	}
	return true
}

func (handlers ProviderRegistryHandlers) handleGet(w http.ResponseWriter, r *http.Request, providerID string) {
	if !providerRegistryNoBody(r) {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	snapshot, err := handlers.Service.Snapshot(r.Context())
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	provider, ok := snapshot.Providers[providerID]
	if !ok || provider.PrivateAccount != nil || provider.Kind == domainregistry.PrivateAccountKind {
		writeProviderRegistryFailure(w, http.StatusNotFound, "not_found")
		return
	}
	response := projectProviderRegistrySnapshot(snapshot)
	response.Providers = nil
	projected := projectProviderRegistryProvider(provider)
	response.Provider = &projected
	writeProviderRegistryJSON(w, http.StatusOK, response)
}

func (handlers ProviderRegistryHandlers) handleConnect(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryConnectRequest
	defer func() {
		handlers.clearCredentialRequest(&request.Credential)
	}()
	if !requireProviderRegistryRequest(w, r, &request) {
		return
	}
	if request.SchemaVersion != 1 {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	expected, ok := request.Expected.parse()
	if !ok || !domainregistry.ValidIncarnation(expected.RegistryIncarnation) ||
		expected.ProviderRevision != 0 || expected.ProviderGeneration != 0 ||
		expected.ProviderIncarnation != "" || expected.ProviderCredentialPurpose != "" || request.Provider.Validate() != nil ||
		request.Provider.PrivateAccount != nil || request.Provider.Kind == domainregistry.PrivateAccountKind {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	credential, purpose, ok := request.Credential.setMutation()
	if !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	provider, err := handlers.Service.Connect(r.Context(), providerregistryapp.ConnectCommand{
		DeferSelection: request.DeferSelection,
		Expected:       expected, Provider: request.Provider, CredentialPurpose: purpose, Credential: credential,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryMutation(w, expected, provider)
}

func (handlers ProviderRegistryHandlers) handleUpdate(w http.ResponseWriter, r *http.Request, providerID string) {
	var request providerRegistryUpdateRequest
	defer func() {
		handlers.clearCredentialRequest(request.Credential)
	}()
	if !requireProviderRegistryRequest(w, r, &request) {
		return
	}
	if request.SchemaVersion != 1 || request.Provider.ID != providerID || request.Provider.Validate() != nil ||
		request.Provider.PrivateAccount != nil || request.Provider.Kind == domainregistry.PrivateAccountKind {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	expected, ok := request.Expected.parseExisting()
	if !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	credential := secretstoreport.KeepCredential()
	var purpose secretstoreport.Purpose
	if request.Credential != nil {
		credential, purpose, ok = request.Credential.updateMutation(expected.ProviderCredentialPurpose)
		if !ok {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	provider, err := handlers.Service.Update(r.Context(), providerregistryapp.UpdateCommand{
		Expected: expected, Provider: request.Provider, CredentialPurpose: purpose, Credential: credential,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryMutation(w, expected, provider)
}

func (handlers ProviderRegistryHandlers) handleSelect(w http.ResponseWriter, r *http.Request, providerID string) {
	request, expected, ok := decodeProviderRegistryExpectedOnly(w, r)
	if !ok {
		return
	}
	_ = request
	provider, err := handlers.Service.Select(r.Context(), providerregistryapp.SelectCommand{Expected: expected, ProviderID: providerID})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryMutation(w, expected, provider)
}

func (handlers ProviderRegistryHandlers) handleDisconnect(w http.ResponseWriter, r *http.Request, providerID string) {
	_, expected, ok := decodeProviderRegistryExpectedOnly(w, r)
	if !ok || expected.ProviderCredentialPurpose == "" {
		if ok {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		}
		return
	}
	provider, err := handlers.Service.Disconnect(r.Context(), providerregistryapp.DisconnectCommand{
		Expected: expected, ProviderID: providerID,
		CredentialPurpose: secretstoreport.Purpose(expected.ProviderCredentialPurpose),
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryMutation(w, expected, provider)
}

func (handlers ProviderRegistryHandlers) handleDelete(w http.ResponseWriter, r *http.Request, providerID string) {
	_, expected, ok := decodeProviderRegistryExpectedOnly(w, r)
	if !ok {
		return
	}
	err := handlers.Service.ExplicitDelete(r.Context(), providerregistryapp.ExplicitDeleteCommand{
		Expected: expected, ProviderID: providerID,
		CredentialPurpose: secretstoreport.Purpose(expected.ProviderCredentialPurpose),
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPublicResponse{
		SchemaVersion: 1, RegistryRevision: strconv.FormatUint(expected.RegistryRevision+1, 10),
		RegistryIncarnation: expected.RegistryIncarnation, DeletedProviderID: providerID,
	})
}

func (handlers ProviderRegistryHandlers) handleCredentialReplace(w http.ResponseWriter, r *http.Request, providerID string) {
	var request providerRegistryCredentialReplaceRequest
	defer func() {
		handlers.clearCredentialRequest(&request.Credential)
	}()
	if !requireProviderRegistryRequest(w, r, &request) {
		return
	}
	if request.SchemaVersion != 1 {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	expected, ok := request.Expected.parseExisting()
	if !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	credential, purpose, ok := request.Credential.setMutation()
	if !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	provider, err := handlers.Service.ReplaceCredential(r.Context(), providerregistryapp.CredentialReplaceCommand{
		Expected: expected, ProviderID: providerID, CredentialPurpose: purpose, Credential: credential,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryMutation(w, expected, provider)
}

func (handlers ProviderRegistryHandlers) handleCredentialCheck(w http.ResponseWriter, r *http.Request, providerID string) {
	_, expected, ok := decodeProviderRegistryExpectedOnly(w, r)
	if !ok || expected.ProviderCredentialPurpose == "" {
		if ok {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		}
		return
	}
	if err := handlers.Service.CheckCredential(r.Context(), providerregistryapp.ProviderOperationCommand{
		Expected: expected, ProviderID: providerID,
	}); err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, struct {
		SchemaVersion       int    `json:"schemaVersion"`
		RegistryRevision    string `json:"registryRevision"`
		RegistryIncarnation string `json:"registryIncarnation"`
		ProviderID          string `json:"providerId"`
		ProviderRevision    string `json:"providerRevision"`
		ProviderGeneration  string `json:"providerGeneration"`
		ProviderIncarnation string `json:"providerIncarnation"`
		CredentialAvailable bool   `json:"credentialAvailable"`
	}{1, strconv.FormatUint(expected.RegistryRevision, 10), expected.RegistryIncarnation,
		providerID, strconv.FormatUint(expected.ProviderRevision, 10),
		strconv.FormatUint(expected.ProviderGeneration, 10), expected.ProviderIncarnation, true})
}

func (handlers ProviderRegistryHandlers) handleProbe(w http.ResponseWriter, r *http.Request, providerID string) {
	_, expected, ok := decodeProviderRegistryExpectedOnly(w, r)
	if !ok || expected.ProviderCredentialPurpose == "" {
		if ok {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		}
		return
	}
	result, err := handlers.Service.Probe(r.Context(), providerregistryapp.ProviderOperationCommand{
		Expected: expected, ProviderID: providerID,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.RegistryRevision != expected.RegistryRevision ||
		result.RegistryIncarnation != expected.RegistryIncarnation ||
		result.ProviderRevision != expected.ProviderRevision ||
		result.ProviderGeneration != expected.ProviderGeneration ||
		result.ProviderIncarnation != expected.ProviderIncarnation {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryProbeResponse{
		SchemaVersion:       1,
		RegistryRevision:    strconv.FormatUint(result.RegistryRevision, 10),
		RegistryIncarnation: result.RegistryIncarnation,
		ProviderID:          providerID,
		ProviderRevision:    strconv.FormatUint(result.ProviderRevision, 10),
		ProviderGeneration:  strconv.FormatUint(result.ProviderGeneration, 10),
		ProviderIncarnation: result.ProviderIncarnation,
		Status:              string(result.Status), Code: result.Code,
		ModelCount: result.ModelCount, LatencyMillis: result.LatencyMillis,
	})
}

func (handlers ProviderRegistryHandlers) handleDiscoverModels(w http.ResponseWriter, r *http.Request, providerID string) {
	_, expected, ok := decodeProviderRegistryExpectedOnly(w, r)
	if !ok || expected.ProviderCredentialPurpose == "" {
		if ok {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		}
		return
	}
	provider, err := handlers.Service.DiscoverModels(r.Context(), providerregistryapp.ProviderOperationCommand{
		Expected: expected, ProviderID: providerID,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	writeProviderRegistryMutation(w, expected, provider)
}

func (handlers ProviderRegistryHandlers) handleObserveAccount(w http.ResponseWriter, r *http.Request, providerID string) {
	_, expected, ok := decodeProviderRegistryExpectedOnly(w, r)
	if !ok || expected.ProviderCredentialPurpose == "" {
		if ok {
			writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		}
		return
	}
	result, err := handlers.Service.ObserveAccount(r.Context(), providerregistryapp.ProviderOperationCommand{
		Expected: expected, ProviderID: providerID,
	})
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	if result.RegistryRevision != expected.RegistryRevision || result.RegistryIncarnation != expected.RegistryIncarnation ||
		result.ProviderID != providerID || result.ProviderRevision != expected.ProviderRevision ||
		result.ProviderGeneration != expected.ProviderGeneration || result.ProviderIncarnation != expected.ProviderIncarnation ||
		result.ProviderCredentialPurpose != expected.ProviderCredentialPurpose {
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
		return
	}
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryAccountObservationResponse{
		SchemaVersion: 1, RegistryRevision: strconv.FormatUint(result.RegistryRevision, 10),
		RegistryIncarnation: result.RegistryIncarnation, ProviderID: result.ProviderID,
		ProviderRevision:          strconv.FormatUint(result.ProviderRevision, 10),
		ProviderGeneration:        strconv.FormatUint(result.ProviderGeneration, 10),
		ProviderIncarnation:       result.ProviderIncarnation,
		ProviderCredentialPurpose: result.ProviderCredentialPurpose, Status: string(result.Status),
		ObservedAt: result.ObservedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:  result.ExpiresAt.UTC().Format(time.RFC3339Nano),
		Quota:      result.Quota, Usage: result.Usage, Remaining: result.Remaining,
	})
}

func (handlers ProviderRegistryHandlers) handleRecover(w http.ResponseWriter, r *http.Request) {
	var request providerRegistryRecoverRequest
	if !requireProviderRegistryRequest(w, r, &request) {
		return
	}
	if request.SchemaVersion != 1 {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := handlers.Service.Recover(r.Context()); err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	snapshot, err := handlers.Service.Snapshot(r.Context())
	if err != nil {
		writeProviderRegistryServiceError(w, err)
		return
	}
	response := projectProviderRegistrySnapshot(snapshot)
	response.Recovered = true
	writeProviderRegistryJSON(w, http.StatusOK, response)
}

func decodeProviderRegistryExpectedOnly(
	w http.ResponseWriter,
	r *http.Request,
) (providerRegistryExpectedOnlyRequest, domainregistry.ExpectedState, bool) {
	var request providerRegistryExpectedOnlyRequest
	if !requireProviderRegistryRequest(w, r, &request) {
		return request, domainregistry.ExpectedState{}, false
	}
	if request.SchemaVersion != 1 {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return request, domainregistry.ExpectedState{}, false
	}
	expected, ok := request.Expected.parseExisting()
	if !ok {
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
		return request, domainregistry.ExpectedState{}, false
	}
	return request, expected, true
}

func (request providerRegistryExpectedRequest) parse() (domainregistry.ExpectedState, bool) {
	registryRevision, ok := parseProviderRegistryDecimal(request.RegistryRevision)
	if !ok || request.RegistryIncarnation == nil || request.ProviderIncarnation == nil || request.ProviderCredentialPurpose == nil {
		return domainregistry.ExpectedState{}, false
	}
	providerRevision, ok := parseProviderRegistryDecimal(request.ProviderRevision)
	if !ok {
		return domainregistry.ExpectedState{}, false
	}
	providerGeneration, ok := parseProviderRegistryDecimal(request.ProviderGeneration)
	if !ok {
		return domainregistry.ExpectedState{}, false
	}
	return domainregistry.ExpectedState{
		RegistryRevision: registryRevision, RegistryIncarnation: *request.RegistryIncarnation,
		ProviderRevision: providerRevision, ProviderGeneration: providerGeneration,
		ProviderIncarnation:       *request.ProviderIncarnation,
		ProviderCredentialPurpose: *request.ProviderCredentialPurpose,
	}, true
}

func (request providerRegistryExpectedRequest) parseExisting() (domainregistry.ExpectedState, bool) {
	expected, ok := request.parse()
	purposeOK := expected.ProviderCredentialPurpose == ""
	if !purposeOK {
		normalized, err := secretstoreport.NormalizePurpose(expected.ProviderCredentialPurpose)
		purposeOK = err == nil && string(normalized) == expected.ProviderCredentialPurpose
	}
	return expected, ok && expected.RegistryRevision < math.MaxUint64 && expected.ProviderRevision != 0 &&
		expected.ProviderGeneration != 0 && domainregistry.ValidIncarnation(expected.RegistryIncarnation) &&
		domainregistry.ValidIncarnation(expected.ProviderIncarnation) && purposeOK
}

func (request providerRegistryExpectedRequest) parseLegacyMigration() (domainregistry.ExpectedState, bool) {
	expected, ok := request.parse()
	if !ok || !domainregistry.ValidIncarnation(expected.RegistryIncarnation) {
		return domainregistry.ExpectedState{}, false
	}
	if expected.ProviderRevision == 0 {
		return expected, expected.ProviderGeneration == 0 && expected.ProviderIncarnation == "" &&
			expected.ProviderCredentialPurpose == ""
	}
	if expected.ProviderGeneration == 0 || !domainregistry.ValidIncarnation(expected.ProviderIncarnation) {
		return domainregistry.ExpectedState{}, false
	}
	if expected.ProviderCredentialPurpose == "" {
		return expected, true
	}
	purpose, err := secretstoreport.NormalizePurpose(expected.ProviderCredentialPurpose)
	return expected, err == nil && string(purpose) == expected.ProviderCredentialPurpose
}

func parseProviderRegistryDecimal(value *string) (uint64, bool) {
	if value == nil || *value == "" || (len(*value) > 1 && (*value)[0] == '0') {
		return 0, false
	}
	for _, current := range []byte(*value) {
		if current < '0' || current > '9' {
			return 0, false
		}
	}
	parsed, err := strconv.ParseUint(*value, 10, 64)
	return parsed, err == nil
}

func (request providerRegistryCredentialRequest) setMutation() (
	secretstoreport.CredentialMutation,
	secretstoreport.Purpose,
	bool,
) {
	if request.Kind != "set" || request.Purpose == nil || len(request.ValueBase64) == 0 {
		return secretstoreport.CredentialMutation{}, "", false
	}
	purpose, err := secretstoreport.NormalizePurpose(*request.Purpose)
	if err != nil || string(purpose) != *request.Purpose {
		return secretstoreport.CredentialMutation{}, "", false
	}
	credential, err := secretstoreport.SetCredential(request.ValueBase64)
	return credential, purpose, err == nil
}

func (request providerRegistryCredentialRequest) updateMutation(currentPurpose string) (
	secretstoreport.CredentialMutation,
	secretstoreport.Purpose,
	bool,
) {
	if request.Kind == "keep" {
		return secretstoreport.KeepCredential(), "", request.Purpose == nil && len(request.ValueBase64) == 0
	}
	if request.Kind == "unset" {
		purpose, err := secretstoreport.NormalizePurpose(currentPurpose)
		return secretstoreport.ExplicitlyDeleteCredential(), purpose,
			err == nil && string(purpose) == currentPurpose && request.Purpose == nil && len(request.ValueBase64) == 0
	}
	return request.setMutation()
}

func matchProviderRegistryPath(r *http.Request) (providerRegistryPath, bool) {
	if r == nil || r.URL == nil || r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		return providerRegistryPath{}, false
	}
	path := r.URL.Path
	switch path {
	case ProviderRegistryPathV1:
		return providerRegistryPath{operation: providerRegistryPathCollection}, true
	case ProviderRegistryPortableManifestPathV1:
		return providerRegistryPath{operation: providerRegistryPathPortableManifest}, true
	case ProviderRegistryPathV1 + "/recover":
		return providerRegistryPath{operation: providerRegistryPathRecover}, true
	case providerRegistryPrivateLegacyMigrationPreparePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationPrepare}, true
	case providerRegistryPrivateLegacyMigrationCommitPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationCommit}, true
	case providerRegistryPrivateLegacyMigrationAbandonPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationAbandon}, true
	case providerRegistryPrivateLegacyMigrationRollbackBeginPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationRollbackBegin}, true
	case providerRegistryPrivateLegacyMigrationRollbackCommitPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationRollbackCommit}, true
	case providerRegistryPrivateLegacyMigrationRemigratePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationRemigrate}, true
	case providerRegistryPrivateLegacyMigrationProtectedDeletePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationProtectedDelete}, true
	case providerRegistryPrivateLegacyMigrationFinalizePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationFinalize}, true
	case providerRegistryPrivateLegacyMigrationInventoryPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationInventory}, true
	case providerRegistryPrivateLegacyMigrationSourceAuthorityChallengePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateLegacyMigrationSourceAuthorityChallenge}, true
	case providerRegistryPrivateAccountStatusPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateAccountStatus}, true
	case providerRegistryPrivateAccountListPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateAccountList}, true
	case providerRegistryPrivateAccountPutPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateAccountPut}, true
	case providerRegistryPrivateAccountResolvePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateAccountResolve}, true
	case providerRegistryPrivateAccountMutatePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateAccountMutate}, true
	case ProviderRegistryPrivateProtectedRecoveryPreparePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateProtectedRecoveryPrepare}, true
	case ProviderRegistryPrivateProtectedRecoveryConfirmDestinationPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateProtectedRecoveryConfirmDestination}, true
	case ProviderRegistryPrivateProtectedRecoveryCreateBundlePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateProtectedRecoveryCreateBundle}, true
	case ProviderRegistryPrivateProtectedRecoveryApplyPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateProtectedRecoveryApply}, true
	case ProviderRegistryPrivateProtectedRecoveryFinalizePathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateProtectedRecoveryFinalize}, true
	case ProviderRegistryPrivateProtectedRecoveryRecoverPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateProtectedRecoveryRecover}, true
	case ProviderRegistryPrivateProtectedRecoveryRollbackPathV1:
		return providerRegistryPath{operation: providerRegistryPathPrivateProtectedRecoveryRollback}, true
	}
	const prefix = ProviderRegistryPathV1 + "/providers/"
	if !strings.HasPrefix(path, prefix) {
		return providerRegistryPath{}, false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) < 1 || len(parts) > 2 || !domainregistry.ValidProviderID(parts[0]) {
		return providerRegistryPath{}, false
	}
	matched := providerRegistryPath{operation: providerRegistryPathProvider, providerID: parts[0]}
	if len(parts) == 1 {
		return matched, true
	}
	switch parts[1] {
	case "select":
		matched.operation = providerRegistryPathSelect
	case "disconnect":
		matched.operation = providerRegistryPathDisconnect
	case "credential":
		matched.operation = providerRegistryPathCredential
	case "probe":
		matched.operation = providerRegistryPathProbe
	case "credential-check":
		matched.operation = providerRegistryPathCredentialCheck
	case "discover-models":
		matched.operation = providerRegistryPathDiscoverModels
	case "account-observation":
		matched.operation = providerRegistryPathObserveAccount
	default:
		return providerRegistryPath{}, false
	}
	return matched, true
}

func decodeProviderRegistryCanonicalBase64(raw json.RawMessage) ([]byte, bool) {
	if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, false
	}
	encoded := raw[1 : len(raw)-1]
	if len(encoded) == 0 || len(encoded)%4 != 0 {
		return nil, false
	}
	credential := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	written, err := base64.StdEncoding.Strict().Decode(credential, encoded)
	if written > 0 {
		credential = credential[:written]
	}
	if err != nil || written == 0 || written > providerRegistryMaxCredentialBytes {
		return credential, false
	}
	canonical := make([]byte, base64.StdEncoding.EncodedLen(len(credential)))
	base64.StdEncoding.Encode(canonical, credential)
	ok := bytes.Equal(encoded, canonical)
	clearProviderRegistryBytes(canonical)
	if !ok {
		return credential, false
	}
	return credential, true
}

func decodeProviderRegistryLegacyMigrationRollbackArtifacts(
	activeCredential []byte,
	sourceLocator string,
	activeCredentialLocators []string,
	requests []providerRegistryPrivateLegacyMigrationRollbackArtifactRequest,
) ([]providerregistryapp.LegacyMigrationRollbackCredentialArtifact, bool) {
	if len(activeCredentialLocators) == 0 && len(requests) == 0 {
		return nil, true
	}
	if len(activeCredentialLocators) == 0 ||
		len(activeCredentialLocators) > providerRegistryMaxLegacyMigrationLocatorCount ||
		len(requests) > providerRegistryMaxLegacyMigrationRollbackArtifacts {
		return nil, false
	}

	locatorCount := 0
	locatorBytes := 0
	locators := make(map[string]struct{}, len(activeCredentialLocators))
	validateLocator := func(locator string) bool {
		locatorCount++
		if locatorCount > providerRegistryMaxLegacyMigrationLocatorCount ||
			len(locator) > providerRegistryMaxLegacyMigrationLocatorTotalBytes-locatorBytes ||
			!validProviderRegistryLegacyMigrationLogicalLocator([]byte(locator)) ||
			(sourceLocator != "" && !strings.HasPrefix(locator, sourceLocator+":")) {
			return false
		}
		if _, duplicate := locators[locator]; duplicate {
			return false
		}
		locators[locator] = struct{}{}
		locatorBytes += len(locator)
		return true
	}
	for _, locator := range activeCredentialLocators {
		if !validateLocator(locator) {
			return nil, false
		}
	}

	credentialBytes := len(activeCredential)
	artifacts := make([]providerregistryapp.LegacyMigrationRollbackCredentialArtifact, 0, len(requests))
	for index, request := range requests {
		credential, ok := decodeProviderRegistryCanonicalBase64(request.Credential.ValueBase64)
		artifact := providerregistryapp.LegacyMigrationRollbackCredentialArtifact{
			Locators:   append([]string(nil), request.CredentialLocators...),
			Credential: credential,
		}
		artifacts = append(artifacts, artifact)
		if !ok || request.SchemaVersion != 1 ||
			len(request.CredentialLocators) == 0 ||
			credentialBytes > providerRegistryMaxCredentialBytes-len(credential) ||
			bytes.Equal(activeCredential, credential) {
			return artifacts, false
		}
		for prior := 0; prior < index; prior++ {
			if bytes.Equal(artifacts[prior].Credential, credential) {
				return artifacts, false
			}
		}
		for _, locator := range request.CredentialLocators {
			if !validateLocator(locator) {
				return artifacts, false
			}
		}
		credentialBytes += len(credential)
	}
	return artifacts, true
}

func validProviderRegistryLegacyMigrationLogicalLocator(locator []byte) bool {
	if len(locator) == 0 || len(locator) > providerRegistryMaxLegacyMigrationLocatorBytes {
		return false
	}
	currentPrefix := []byte("current:analytix-settings.json:")
	if bytes.HasPrefix(locator, currentPrefix) {
		return validProviderRegistryLegacyMigrationCredentialLocatorSuffix(locator[len(currentPrefix):])
	}
	compatibilityPrefix := []byte("compatibility:")
	if !bytes.HasPrefix(locator, compatibilityPrefix) {
		return false
	}
	remainder := locator[len(compatibilityPrefix):]
	if len(remainder) < 3 || remainder[0] < '0' || remainder[0] > '9' ||
		remainder[1] < '0' || remainder[1] > '9' || remainder[2] != ':' {
		return false
	}
	remainder = remainder[3:]
	for _, filename := range [][]byte{
		[]byte("analytix-settings.json:"),
		[]byte("kun-settings.json:"),
	} {
		if bytes.HasPrefix(remainder, filename) {
			return validProviderRegistryLegacyMigrationCredentialLocatorSuffix(remainder[len(filename):])
		}
	}
	return false
}

func validProviderRegistryLegacyMigrationSourceLocator(locator []byte) bool {
	if bytes.Equal(locator, []byte("current:analytix-settings.json")) {
		return true
	}
	compatibilityPrefix := []byte("compatibility:")
	if !bytes.HasPrefix(locator, compatibilityPrefix) {
		return false
	}
	remainder := locator[len(compatibilityPrefix):]
	if len(remainder) < 4 || remainder[0] < '0' || remainder[0] > '9' ||
		remainder[1] < '0' || remainder[1] > '9' || remainder[2] != ':' {
		return false
	}
	remainder = remainder[3:]
	return bytes.Equal(remainder, []byte("analytix-settings.json")) ||
		bytes.Equal(remainder, []byte("kun-settings.json"))
}

func validProviderRegistryLegacyMigrationCredentialLocatorSuffix(suffix []byte) bool {
	for _, exact := range [][]byte{
		[]byte("runtime.apiKey"),
		[]byte("deepseek.apiKey"),
		[]byte("agents.reasonix.apiKey"),
		[]byte("agents.codewhale.apiKey"),
		[]byte("agents.kun.apiKey"),
		[]byte("provider.apiKey"),
	} {
		if bytes.Equal(suffix, exact) {
			return true
		}
	}
	profilePrefix := []byte("provider.providers[")
	profileSuffix := []byte("].apiKey")
	if !bytes.HasPrefix(suffix, profilePrefix) || !bytes.HasSuffix(suffix, profileSuffix) {
		return false
	}
	profileIndex := suffix[len(profilePrefix) : len(suffix)-len(profileSuffix)]
	if len(profileIndex) == 0 || len(profileIndex) > 2 ||
		(len(profileIndex) > 1 && profileIndex[0] == '0') {
		return false
	}
	value := 0
	for _, digit := range profileIndex {
		if digit < '0' || digit > '9' {
			return false
		}
		value = value*10 + int(digit-'0')
	}
	return value <= 63
}

func providerRegistryCredentialIsSentinel(value []byte) bool {
	normalized := bytes.TrimSpace(value)
	if len(normalized) == 0 {
		return true
	}
	for _, sentinel := range [][]byte{
		[]byte("redacted"), []byte("[redacted]"), []byte("<redacted>"), []byte("__redacted__"),
		[]byte("masked"), []byte("[masked]"), []byte("<masked>"), []byte("__masked__"),
		[]byte("unset"), []byte("not-set"), []byte("not_set"), []byte("null"), []byte("undefined"),
	} {
		if bytes.EqualFold(normalized, sentinel) {
			return true
		}
	}

	mask := normalized
	for index, current := range normalized {
		if current != '-' && current != '_' && current != ':' {
			continue
		}
		if index == 0 {
			break
		}
		validPrefix := true
		for _, prefix := range normalized[:index] {
			if (prefix < 'a' || prefix > 'z') && (prefix < 'A' || prefix > 'Z') &&
				(prefix < '0' || prefix > '9') {
				validPrefix = false
				break
			}
		}
		if validPrefix {
			mask = normalized[index+1:]
		}
		break
	}
	maskCount := 0
	for len(mask) > 0 {
		current, width := utf8.DecodeRune(mask)
		if current != '*' && current != '•' && current != '●' && current != 'x' && current != 'X' {
			return false
		}
		maskCount++
		mask = mask[width:]
	}
	return maskCount >= 4
}

func (handlers ProviderRegistryHandlers) clearPrivateLegacyMigrationPrepareRequest(
	request *providerRegistryPrivateLegacyMigrationPrepareRequest,
) {
	if request == nil {
		return
	}
	clearProviderRegistryBytes(request.Credential.ValueBase64)
	if request.Credential.ValueBase64 != nil && handlers.observeEncodedCredentialBufferCleared != nil {
		handlers.observeEncodedCredentialBufferCleared(request.Credential.ValueBase64)
	}
	request.Credential.ValueBase64 = nil
	clearProviderRegistryBytes(request.RollbackSource.ValueBase64)
	if request.RollbackSource.ValueBase64 != nil && handlers.observeEncodedCredentialBufferCleared != nil {
		handlers.observeEncodedCredentialBufferCleared(request.RollbackSource.ValueBase64)
	}
	request.RollbackSource.ValueBase64 = nil
	for index := range request.RollbackCredentialArtifacts {
		clearProviderRegistryBytes(request.RollbackCredentialArtifacts[index].Credential.ValueBase64)
		if request.RollbackCredentialArtifacts[index].Credential.ValueBase64 != nil &&
			handlers.observeEncodedCredentialBufferCleared != nil {
			handlers.observeEncodedCredentialBufferCleared(
				request.RollbackCredentialArtifacts[index].Credential.ValueBase64,
			)
		}
		request.RollbackCredentialArtifacts[index].Credential.ValueBase64 = nil
	}
}

func validProviderRegistryLowerSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, current := range []byte(value) {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}

func validProviderRegistryLegacyMigrationSourceAuthorityChallenge(value string) bool {
	if !strings.HasPrefix(value, "lmsa_") {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "lmsa_"))
	return err == nil && len(decoded) == 32
}

func validProviderRegistryLegacyMigrationLockToken(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, current := range []byte(value) {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if current != '-' {
				return false
			}
			continue
		}
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') &&
			(current < 'A' || current > 'F') {
			return false
		}
	}
	return true
}

func parseProviderRegistryLegacyMigrationSourceAuthorityProof(
	request providerRegistryPrivateLegacyMigrationSourceAuthorityProofRequest,
) (providerregistryapp.LegacyMigrationSourceAuthorityProof, bool) {
	_, deviceErr := strconv.ParseUint(request.SourceDevice, 10, 64)
	inode, inodeErr := strconv.ParseUint(request.SourceInode, 10, 64)
	if !validProviderRegistryLegacyMigrationSourceAuthorityChallenge(request.Challenge) ||
		len(request.SourcePath) == 0 || len(request.SourcePath) > 4096 ||
		strings.IndexByte(request.SourcePath, 0) >= 0 || !filepath.IsAbs(request.SourcePath) ||
		!validProviderRegistryLegacyMigrationLockToken(request.LockOwnerToken) ||
		deviceErr != nil || inodeErr != nil || inode == 0 {
		return providerregistryapp.LegacyMigrationSourceAuthorityProof{}, false
	}
	return providerregistryapp.LegacyMigrationSourceAuthorityProof{
		Challenge: request.Challenge, SourcePath: request.SourcePath,
		LockOwnerToken: request.LockOwnerToken,
		SourceDevice:   request.SourceDevice, SourceInode: request.SourceInode,
	}, true
}

func decodeProviderRegistryRequest(
	r *http.Request,
	target any,
	requireUniqueObjectKeys ...bool,
) providerRegistryDecodeResult {
	if r == nil || r.Body == nil || target == nil {
		return providerRegistryDecodeInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, providerRegistryMaxBodyBytes+1))
	_ = r.Body.Close()
	defer clearProviderRegistryBytes(raw)
	if err != nil || len(raw) == 0 {
		return providerRegistryDecodeInvalid
	}
	if len(raw) > providerRegistryMaxBodyBytes {
		return providerRegistryDecodeTooLarge
	}
	if len(requireUniqueObjectKeys) > 0 && requireUniqueObjectKeys[0] &&
		!providerRegistryJSONHasUniqueObjectKeys(raw) {
		return providerRegistryDecodeInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return providerRegistryDecodeInvalid
	}
	var trailing json.RawMessage
	if !errors.Is(decoder.Decode(&trailing), io.EOF) {
		return providerRegistryDecodeInvalid
	}
	return providerRegistryDecodeOK
}

type providerRegistryJSONKeyScanner struct {
	raw    []byte
	offset int
}

func providerRegistryJSONHasUniqueObjectKeys(raw []byte) bool {
	if !json.Valid(raw) {
		return false
	}
	scanner := providerRegistryJSONKeyScanner{raw: raw}
	if !scanner.scanValue() {
		return false
	}
	scanner.skipSpace()
	return scanner.offset == len(scanner.raw)
}

func (scanner *providerRegistryJSONKeyScanner) scanValue() bool {
	scanner.skipSpace()
	if scanner.offset >= len(scanner.raw) {
		return false
	}
	switch scanner.raw[scanner.offset] {
	case '{':
		return scanner.scanObject()
	case '[':
		return scanner.scanArray()
	case '"':
		_, _, ok := scanner.scanString()
		return ok
	default:
		start := scanner.offset
		for scanner.offset < len(scanner.raw) {
			switch scanner.raw[scanner.offset] {
			case ',', ']', '}', ' ', '\t', '\r', '\n':
				return scanner.offset > start
			default:
				scanner.offset++
			}
		}
		return scanner.offset > start
	}
}

func (scanner *providerRegistryJSONKeyScanner) scanObject() bool {
	scanner.offset++
	scanner.skipSpace()
	if scanner.offset < len(scanner.raw) && scanner.raw[scanner.offset] == '}' {
		scanner.offset++
		return true
	}
	keys := make(map[string]struct{})
	for {
		key, escaped, ok := scanner.scanString()
		if !ok || escaped {
			return false
		}
		keyValue := string(key)
		if _, duplicate := keys[keyValue]; duplicate {
			return false
		}
		keys[keyValue] = struct{}{}
		scanner.skipSpace()
		if scanner.offset >= len(scanner.raw) || scanner.raw[scanner.offset] != ':' {
			return false
		}
		scanner.offset++
		if !scanner.scanValue() {
			return false
		}
		scanner.skipSpace()
		if scanner.offset >= len(scanner.raw) {
			return false
		}
		switch scanner.raw[scanner.offset] {
		case '}':
			scanner.offset++
			return true
		case ',':
			scanner.offset++
			scanner.skipSpace()
		default:
			return false
		}
	}
}

func (scanner *providerRegistryJSONKeyScanner) scanArray() bool {
	scanner.offset++
	scanner.skipSpace()
	if scanner.offset < len(scanner.raw) && scanner.raw[scanner.offset] == ']' {
		scanner.offset++
		return true
	}
	for {
		if !scanner.scanValue() {
			return false
		}
		scanner.skipSpace()
		if scanner.offset >= len(scanner.raw) {
			return false
		}
		switch scanner.raw[scanner.offset] {
		case ']':
			scanner.offset++
			return true
		case ',':
			scanner.offset++
		default:
			return false
		}
	}
}

func (scanner *providerRegistryJSONKeyScanner) scanString() ([]byte, bool, bool) {
	scanner.skipSpace()
	if scanner.offset >= len(scanner.raw) || scanner.raw[scanner.offset] != '"' {
		return nil, false, false
	}
	scanner.offset++
	start := scanner.offset
	escaped := false
	for scanner.offset < len(scanner.raw) {
		switch scanner.raw[scanner.offset] {
		case '"':
			value := scanner.raw[start:scanner.offset]
			scanner.offset++
			return value, escaped, true
		case '\\':
			escaped = true
			scanner.offset += 2
		default:
			scanner.offset++
		}
	}
	return nil, escaped, false
}

func (scanner *providerRegistryJSONKeyScanner) skipSpace() {
	for scanner.offset < len(scanner.raw) {
		switch scanner.raw[scanner.offset] {
		case ' ', '\t', '\r', '\n':
			scanner.offset++
		default:
			return
		}
	}
}

func requireProviderRegistryRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	switch decodeProviderRegistryRequest(r, target) {
	case providerRegistryDecodeOK:
		return true
	case providerRegistryDecodeTooLarge:
		writeProviderRegistryFailure(w, http.StatusRequestEntityTooLarge, "request_too_large")
	default:
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
	}
	return false
}

func requireProviderRegistryUniqueRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	switch decodeProviderRegistryRequest(r, target, true) {
	case providerRegistryDecodeOK:
		return true
	case providerRegistryDecodeTooLarge:
		writeProviderRegistryFailure(w, http.StatusRequestEntityTooLarge, "request_too_large")
	default:
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
	}
	return false
}

func providerRegistryNoBody(r *http.Request) bool {
	if r == nil || r.Body == nil {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1))
	_ = r.Body.Close()
	defer clearProviderRegistryBytes(raw)
	return err == nil && len(raw) == 0
}

func projectProviderRegistrySnapshot(snapshot domainregistry.Registry) providerRegistryPublicResponse {
	ids := make([]string, 0, len(snapshot.Providers))
	for id, provider := range snapshot.Providers {
		if provider.PrivateAccount != nil || provider.Kind == domainregistry.PrivateAccountKind {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	providers := make([]providerRegistryPublicProvider, 0, len(ids))
	for _, id := range ids {
		providers = append(providers, projectProviderRegistryProvider(snapshot.Providers[id]))
	}
	return providerRegistryPublicResponse{
		SchemaVersion: 1, RegistryRevision: strconv.FormatUint(snapshot.Revision, 10),
		RegistryIncarnation: snapshot.Incarnation, SelectedProviderID: snapshot.SelectedProviderID,
		Providers: &providers,
	}
}

func projectProviderRegistryProvider(provider domainregistry.Provider) providerRegistryPublicProvider {
	var oauthBinding *domainregistry.OAuthBindingMetadata
	if provider.OAuthBinding != nil {
		binding := *provider.OAuthBinding
		binding.Scopes = append([]string{}, provider.OAuthBinding.Scopes...)
		oauthBinding = &binding
	}
	var accountObservation *domainregistry.AccountObservationBinding
	if provider.AccountObservation != nil {
		binding := *provider.AccountObservation
		accountObservation = &binding
	}
	return providerRegistryPublicProvider{
		ID: provider.ID, Kind: provider.Kind, Endpoint: provider.Endpoint, Proxy: provider.Proxy,
		Models: append([]string{}, provider.Models...), MediaModels: append([]string{}, provider.MediaModels...),
		SelectedModel: provider.SelectedModel, SelectedMediaModel: provider.SelectedMedia,
		SelectedRoutes:       append([]string{}, provider.SelectedRoutes...),
		CredentialConfigured: provider.CredentialRef != "", CredentialPurpose: provider.CredentialPurpose,
		Revision: strconv.FormatUint(provider.Revision, 10), Generation: strconv.FormatUint(provider.Generation, 10),
		Incarnation: provider.Incarnation, Tombstone: provider.Tombstone, OAuthBinding: oauthBinding,
		AccountObservation: accountObservation,
	}
}

func writeProviderRegistryMutation(w http.ResponseWriter, expected domainregistry.ExpectedState, provider domainregistry.Provider) {
	projected := projectProviderRegistryProvider(provider)
	writeProviderRegistryJSON(w, http.StatusOK, providerRegistryPublicResponse{
		SchemaVersion: 1, RegistryRevision: strconv.FormatUint(expected.RegistryRevision+1, 10),
		RegistryIncarnation: expected.RegistryIncarnation, Provider: &projected,
	})
}

func writeProviderRegistryServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, registryport.ErrCredentialUnavailable):
		writeProviderRegistryFailure(w, http.StatusServiceUnavailable, "credential_unavailable")
	case errors.Is(err, registryport.ErrInvalidRequest):
		writeProviderRegistryFailure(w, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, registryport.ErrConflict):
		writeProviderRegistryFailure(w, http.StatusConflict, "conflict")
	case errors.Is(err, registryport.ErrNotFound):
		writeProviderRegistryFailure(w, http.StatusNotFound, "not_found")
	case errors.Is(err, registryport.ErrVerification):
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
	case errors.Is(err, registryport.ErrPersistence), errors.Is(err, registryport.ErrClosed),
		errors.Is(err, providerregistryapp.ErrInterrupted), errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		writeProviderRegistryFailure(w, http.StatusServiceUnavailable, "persistence_failure")
	default:
		writeProviderRegistryFailure(w, http.StatusInternalServerError, "verification_failure")
	}
}

func writeProviderRegistryFailure(w http.ResponseWriter, status int, code string) {
	message := "The provider registry request was rejected."
	switch code {
	case "unauthorized":
		message = "Provider registry authentication is required."
	case "method_not_allowed":
		message = "The HTTP method is not allowed for this provider registry operation."
	case "not_found":
		message = "The requested provider was not found."
	case "conflict":
		message = "The provider registry state has changed."
	case "persistence_failure":
		message = "The provider registry is temporarily unavailable."
	case "credential_unavailable":
		message = "Secure credential storage is temporarily unavailable. Check system security access and retry. Existing settings have been kept."
	case "verification_failure":
		message = "The provider registry operation could not be verified."
	case "request_too_large":
		message = "The provider registry request is too large."
	}
	response := providerRegistryPublicError{SchemaVersion: 1}
	response.Error.Code = code
	response.Error.Message = message
	writeProviderRegistryJSON(w, status, response)
}

func writeProviderRegistryJSON(w http.ResponseWriter, status int, body any) {
	providerRegistryNoStore(w)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func providerRegistryNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func clearProviderRegistryBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func (handlers ProviderRegistryHandlers) clearCredentialRequest(request *providerRegistryCredentialRequest) {
	if request == nil || request.ValueBase64 == nil {
		return
	}
	clearProviderRegistryBytes(request.ValueBase64)
	if handlers.observeCredentialBufferCleared != nil {
		handlers.observeCredentialBufferCleared(request.ValueBase64)
	}
	request.ValueBase64 = nil
}
