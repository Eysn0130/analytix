package providerregistry

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	maxLegacyMigrationCredentialBytes              = 1 << 20
	maxLegacyMigrationRecoveryPayloadBytes         = 1 << 20
	maxLegacyMigrationLocatorBytes                 = 256
	maxLegacyMigrationLocatorCount                 = 256
	maxLegacyMigrationLocatorTotalBytes            = 32 << 10
	maxLegacyMigrationRollbackArtifactCount        = 64
	maxLegacyMigrationRecoveryCredentialTotalBytes = 1 << 20
	maxLegacyMigrationSourceSnapshotBytes          = 256 << 10
	legacyMigrationRecoveryPayloadMagic            = "ALMR"
	legacyMigrationRecoveryPayloadVersion          = 1
	legacyMigrationRecoveryPayloadVersionV2        = 2
	legacyMigrationRecoveryPayloadVersionV3        = 3
	legacyMigrationRecoveryPayloadVersionV4        = 4
	legacyMigrationRecoveryPayloadVersionV5        = 5
	legacyMigrationRecoveryPayloadVersionV6        = 6
	legacyMigrationSourceLockSuffix                = ".analytix-provider-credential-migration.lock"
	legacyMigrationSourceAuthorityPrefix           = "lmsa_"
	maxLegacyMigrationSourceAuthorityPathBytes     = 4096
	maxLegacyMigrationSourceLockOwnerBytes         = 4096
)

type legacyMigrationRollbackCredentialArtifactView struct {
	locators   [][]byte
	credential []byte
}

type legacyMigrationRecoveryPayloadView struct {
	version                      uint32
	migrationID                  []byte
	sourceLocator                []byte
	sourceSHA256                 []byte
	expectedCleanedSourceSHA256  []byte
	sourcePhysicalIdentitySHA256 []byte
	provider                     domainregistry.ProviderInput
	credentialPurpose            []byte
	credential                   []byte
	activeCredentialLocators     [][]byte
	rollbackCredentialArtifacts  []legacyMigrationRollbackCredentialArtifactView
	sourceSnapshot               []byte
	priorSelectedProvider        *domainregistry.Provider
}

type legacyMigrationSourceLockAuthority struct {
	SchemaVersion                int    `json:"schemaVersion"`
	Challenge                    string `json:"challenge"`
	Operation                    string `json:"operation"`
	MigrationID                  string `json:"migrationId"`
	SourceLocator                string `json:"sourceLocator"`
	SourceSHA256                 string `json:"sourceSHA256"`
	CurrentSourceSHA256          string `json:"currentSourceSHA256"`
	SourcePhysicalIdentitySHA256 string `json:"sourcePhysicalIdentitySHA256"`
	SourceDevice                 string `json:"sourceDevice"`
	SourceInode                  string `json:"sourceInode"`
}

type legacyMigrationSourceLockOwner struct {
	SchemaVersion int                                `json:"schemaVersion"`
	Purpose       string                             `json:"purpose"`
	PID           int                                `json:"pid"`
	Token         string                             `json:"token"`
	CreatedAtMS   int64                              `json:"createdAtMs"`
	Authority     legacyMigrationSourceLockAuthority `json:"sourceAuthority"`
}

func validLegacyMigrationSourceAuthorityChallenge(value string) bool {
	if !strings.HasPrefix(value, legacyMigrationSourceAuthorityPrefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, legacyMigrationSourceAuthorityPrefix))
	return err == nil && len(decoded) == 32
}

func validLegacyMigrationSourceLockToken(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, current := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if current != '-' {
				return false
			}
			continue
		}
		if !((current >= '0' && current <= '9') || (current >= 'a' && current <= 'f') ||
			(current >= 'A' && current <= 'F')) {
			return false
		}
	}
	return true
}

func legacyMigrationFileInfoUintField(info os.FileInfo, fieldName string) (uint64, bool) {
	if info == nil || info.Sys() == nil {
		return 0, false
	}
	value := reflect.ValueOf(info.Sys())
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName(fieldName)
	if !field.IsValid() {
		return 0, false
	}
	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return field.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Int() < 0 {
			return 0, false
		}
		return uint64(field.Int()), true
	default:
		return 0, false
	}
}

func legacyMigrationRegularSingleLinkFileInfo(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	links, ok := legacyMigrationFileInfoUintField(info, "Nlink")
	if !ok {
		links, ok = legacyMigrationFileInfoUintField(info, "NumberOfLinks")
	}
	return ok && links == 1
}

func legacyMigrationSourcePhysicalIdentity(sourceLocator, physicalPath string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("analytix-provider-settings-physical-source-v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sourceLocator))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(physicalPath))
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyMigrationSourceGenerationIdentity(
	sourceLocator string,
	sourceSHA256 string,
	currentSourceSHA256 string,
	sourcePhysicalIdentitySHA256 string,
	device string,
	inode string,
) string {
	hash := sha256.New()
	for _, value := range []string{
		"analytix-provider-settings-source-generation-v1", sourceLocator, sourceSHA256,
		currentSourceSHA256, sourcePhysicalIdentitySHA256, device, inode,
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyMigrationSourceAuthorityIntentIdentity(
	challenge string,
	generationIdentity string,
) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("analytix-provider-settings-source-authority-intent-v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(challenge))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(generationIdentity))
	return hex.EncodeToString(hash.Sum(nil))
}

func (manager *Manager) IssueLegacyMigrationSourceAuthorityChallenge(
	ctx context.Context,
	command IssueLegacyMigrationSourceAuthorityChallengeCommand,
) (IssueLegacyMigrationSourceAuthorityChallengeResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		(command.Operation != LegacyMigrationSourceAuthorityOperationRollbackCommit &&
			command.Operation != LegacyMigrationSourceAuthorityOperationFinalize &&
			command.Operation != LegacyMigrationSourceAuthorityOperationRemigrate &&
			command.Operation != LegacyMigrationSourceAuthorityOperationProtectedDelete) ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		!validLegacyMigrationSourceLocator([]byte(command.SourceLocator)) ||
		!validLowerSHA256(command.SourceSHA256) || !validLowerSHA256(command.CurrentSourceSHA256) ||
		!validLowerSHA256(command.SourcePhysicalIdentitySHA256) ||
		secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil ||
		command.Confirmation != LegacyMigrationSourceAuthorityChallengeConfirmation {
		return IssueLegacyMigrationSourceAuthorityChallengeResult{}, registryport.ErrInvalidRequest
	}
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		_, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
			recovery.SourceLocator != command.SourceLocator || recovery.SourceSHA256 != command.SourceSHA256 ||
			recovery.SourcePhysicalIdentitySHA256 != command.SourcePhysicalIdentitySHA256 {
			return registryport.ErrConflict
		}
		expectedCurrent := recovery.ExpectedCleanedSourceSHA256
		if command.CurrentSourceSHA256 != expectedCurrent {
			return registryport.ErrConflict
		}
		switch command.Operation {
		case LegacyMigrationSourceAuthorityOperationRollbackCommit:
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained {
				return registryport.ErrConflict
			}
		case LegacyMigrationSourceAuthorityOperationFinalize:
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback {
				return registryport.ErrConflict
			}
		case LegacyMigrationSourceAuthorityOperationRemigrate:
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained &&
				!(recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseVerified &&
					recovery.RemigrationPending) &&
				!(recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared &&
					recovery.RemigrationPending) {
				return registryport.ErrConflict
			}
		case LegacyMigrationSourceAuthorityOperationProtectedDelete:
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending {
				return registryport.ErrConflict
			}
		}
		return nil
	})
	if err != nil {
		return IssueLegacyMigrationSourceAuthorityChallengeResult{}, normalizeManagerError(err)
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return IssueLegacyMigrationSourceAuthorityChallengeResult{}, registryport.ErrPersistence
	}
	challenge := legacyMigrationSourceAuthorityPrefix + base64.RawURLEncoding.EncodeToString(random)
	clear(random)
	manager.sourceAuthorityMu.Lock()
	defer manager.sourceAuthorityMu.Unlock()
	if manager.sourceAuthorityChallenges == nil {
		manager.sourceAuthorityChallenges = make(map[string]legacyMigrationSourceAuthorityChallenge)
	}
	for existing, current := range manager.sourceAuthorityChallenges {
		if current.MigrationID == command.MigrationID && current.Operation == command.Operation {
			delete(manager.sourceAuthorityChallenges, existing)
		}
	}
	manager.sourceAuthorityChallenges[challenge] = legacyMigrationSourceAuthorityChallenge{
		Operation: command.Operation, MigrationID: command.MigrationID,
		SourceLocator: command.SourceLocator, SourceSHA256: command.SourceSHA256,
		CurrentSourceSHA256:          command.CurrentSourceSHA256,
		SourcePhysicalIdentitySHA256: command.SourcePhysicalIdentitySHA256,
		RecoveryCredentialRef:        command.RecoveryCredentialRef,
	}
	return IssueLegacyMigrationSourceAuthorityChallengeResult{Challenge: challenge}, nil
}

func (manager *Manager) consumeLegacyMigrationSourceAuthority(
	command legacyMigrationSourceAuthorityChallenge,
	proof LegacyMigrationSourceAuthorityProof,
	currentSource []byte,
) (string, string, error) {
	if !validLegacyMigrationSourceAuthorityChallenge(proof.Challenge) ||
		len(proof.SourcePath) == 0 || len(proof.SourcePath) > maxLegacyMigrationSourceAuthorityPathBytes ||
		strings.IndexByte(proof.SourcePath, 0) >= 0 || !filepath.IsAbs(proof.SourcePath) ||
		!validLegacyMigrationSourceLockToken(proof.LockOwnerToken) {
		return "", "", registryport.ErrInvalidRequest
	}
	device, deviceErr := strconv.ParseUint(proof.SourceDevice, 10, 64)
	inode, inodeErr := strconv.ParseUint(proof.SourceInode, 10, 64)
	if deviceErr != nil || inodeErr != nil || inode == 0 {
		return "", "", registryport.ErrInvalidRequest
	}
	manager.sourceAuthorityMu.Lock()
	issued, exists := manager.sourceAuthorityChallenges[proof.Challenge]
	if exists {
		delete(manager.sourceAuthorityChallenges, proof.Challenge)
	}
	manager.sourceAuthorityMu.Unlock()
	if !exists || issued != command {
		return "", "", registryport.ErrVerification
	}
	before, err := os.Lstat(proof.SourcePath)
	if err != nil || !legacyMigrationRegularSingleLinkFileInfo(before) {
		return "", "", registryport.ErrVerification
	}
	beforeDevice, deviceOK := legacyMigrationFileInfoUintField(before, "Dev")
	beforeInode, inodeOK := legacyMigrationFileInfoUintField(before, "Ino")
	if !deviceOK || !inodeOK || beforeDevice != device || beforeInode != inode {
		return "", "", registryport.ErrVerification
	}
	physicalPath, err := filepath.EvalSymlinks(proof.SourcePath)
	if err != nil || !filepath.IsAbs(physicalPath) ||
		legacyMigrationSourcePhysicalIdentity(command.SourceLocator, physicalPath) !=
			command.SourcePhysicalIdentitySHA256 {
		return "", "", registryport.ErrVerification
	}
	file, err := os.Open(proof.SourcePath)
	if err != nil {
		return "", "", registryport.ErrVerification
	}
	opened, statErr := file.Stat()
	loaded, readErr := io.ReadAll(io.LimitReader(file, maxLegacyMigrationSourceSnapshotBytes+1))
	closeErr := file.Close()
	defer clear(loaded)
	if statErr != nil || readErr != nil || closeErr != nil || !legacyMigrationRegularSingleLinkFileInfo(opened) ||
		!os.SameFile(before, opened) || len(loaded) == 0 ||
		len(loaded) > maxLegacyMigrationSourceSnapshotBytes || !bytes.Equal(loaded, currentSource) ||
		!validLegacyMigrationSourceRepresentation(command.CurrentSourceSHA256, loaded) {
		return "", "", registryport.ErrVerification
	}
	after, err := os.Lstat(proof.SourcePath)
	physicalAfter, physicalErr := filepath.EvalSymlinks(proof.SourcePath)
	if err != nil || physicalErr != nil || !legacyMigrationRegularSingleLinkFileInfo(after) ||
		!os.SameFile(opened, after) || physicalAfter != physicalPath {
		return "", "", registryport.ErrVerification
	}
	lockPath := physicalPath + legacyMigrationSourceLockSuffix
	lockInfo, err := os.Lstat(lockPath)
	if err != nil || !lockInfo.IsDir() || lockInfo.Mode()&os.ModeSymlink != 0 {
		return "", "", registryport.ErrVerification
	}
	entries, err := os.ReadDir(lockPath)
	ownerName := "owner-" + proof.LockOwnerToken + ".json"
	if err != nil || len(entries) != 1 || entries[0].Name() != ownerName ||
		entries[0].Type()&os.ModeSymlink != 0 {
		return "", "", registryport.ErrVerification
	}
	ownerPath := filepath.Join(lockPath, ownerName)
	ownerInfo, err := os.Lstat(ownerPath)
	if err != nil || !legacyMigrationRegularSingleLinkFileInfo(ownerInfo) {
		return "", "", registryport.ErrVerification
	}
	ownerBytes, err := os.ReadFile(ownerPath)
	defer clear(ownerBytes)
	if err != nil || len(ownerBytes) == 0 || len(ownerBytes) > maxLegacyMigrationSourceLockOwnerBytes {
		return "", "", registryport.ErrVerification
	}
	decoder := json.NewDecoder(bytes.NewReader(ownerBytes))
	decoder.DisallowUnknownFields()
	var owner legacyMigrationSourceLockOwner
	if err := decoder.Decode(&owner); err != nil {
		return "", "", registryport.ErrVerification
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", "", registryport.ErrVerification
	}
	wantAuthority := legacyMigrationSourceLockAuthority{
		SchemaVersion: 1, Challenge: proof.Challenge, Operation: command.Operation,
		MigrationID: command.MigrationID, SourceLocator: command.SourceLocator,
		SourceSHA256: command.SourceSHA256, CurrentSourceSHA256: command.CurrentSourceSHA256,
		SourcePhysicalIdentitySHA256: command.SourcePhysicalIdentitySHA256,
		SourceDevice:                 proof.SourceDevice, SourceInode: proof.SourceInode,
	}
	if owner.SchemaVersion != 1 || owner.Purpose != "analytix-provider-credential-migration-source-lock" ||
		owner.PID <= 0 || owner.Token != proof.LockOwnerToken || owner.CreatedAtMS <= 0 ||
		owner.Authority != wantAuthority {
		return "", "", registryport.ErrVerification
	}
	generation := legacyMigrationSourceGenerationIdentity(
		command.SourceLocator, command.SourceSHA256, command.CurrentSourceSHA256,
		command.SourcePhysicalIdentitySHA256, proof.SourceDevice, proof.SourceInode,
	)
	return legacyMigrationSourceAuthorityIntentIdentity(proof.Challenge, generation), generation, nil
}

func legacyMigrationPriorSelectedProviderIdentity(provider *domainregistry.Provider) (string, error) {
	if provider == nil {
		return "", nil
	}
	if provider.Validate() != nil || provider.Tombstone || provider.CredentialRef == "" {
		return "", registryport.ErrVerification
	}
	encoded, err := json.Marshal(provider)
	if err != nil {
		return "", registryport.ErrVerification
	}
	defer clear(encoded)
	hash := sha256.New()
	_, _ = hash.Write([]byte("analytix-provider-settings-prior-selected-provider-v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(encoded)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func mustLegacyMigrationPriorSelectedProviderIdentity(provider *domainregistry.Provider) string {
	identity, err := legacyMigrationPriorSelectedProviderIdentity(provider)
	if err != nil {
		return ""
	}
	return identity
}

func bindLegacyMigrationPriorSelectedProvider(
	protectedPayload []byte,
	state domainregistry.Registry,
) ([]byte, string, error) {
	if len(protectedPayload) < len(legacyMigrationRecoveryPayloadMagic)+4 ||
		!bytes.Equal(protectedPayload[:len(legacyMigrationRecoveryPayloadMagic)],
			[]byte(legacyMigrationRecoveryPayloadMagic)) {
		return nil, "", registryport.ErrVerification
	}
	if binary.BigEndian.Uint32(protectedPayload[len(legacyMigrationRecoveryPayloadMagic):]) !=
		legacyMigrationRecoveryPayloadVersionV5 {
		return bytes.Clone(protectedPayload), "", nil
	}
	var prior *domainregistry.Provider
	if state.SelectedProviderID != "" {
		selected, exists := state.Providers[state.SelectedProviderID]
		if !exists {
			return nil, "", registryport.ErrVerification
		}
		if selected.Validate() != nil {
			return nil, "", registryport.ErrVerification
		}
		// Only an exact, usable protected winner can be restored by rollback.
		// Credentialless or tombstoned Registry entries are deliberately not
		// promoted into the protected recovery payload.
		if !selected.Tombstone && selected.CredentialRef != "" {
			clone := selected.Clone()
			prior = &clone
		}
	}
	identity, err := legacyMigrationPriorSelectedProviderIdentity(prior)
	if err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(prior)
	if err != nil {
		return nil, "", registryport.ErrVerification
	}
	defer clear(encoded)
	payload := bytes.Clone(protectedPayload)
	binary.BigEndian.PutUint32(
		payload[len(legacyMigrationRecoveryPayloadMagic):len(legacyMigrationRecoveryPayloadMagic)+4],
		legacyMigrationRecoveryPayloadVersionV6,
	)
	payload, err = appendLegacyMigrationPayloadField(payload, encoded)
	if err != nil {
		clear(payload)
		return nil, "", err
	}
	return payload, identity, nil
}

func (manager *Manager) PrepareLegacyMigrationRecovery(
	ctx context.Context,
	candidate LegacyMigrationCandidate,
) (LegacyMigrationRecoveryResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return LegacyMigrationRecoveryResult{}, registryport.ErrInvalidRequest
	}
	normalized, protectedPayload, err := normalizeLegacyMigrationCandidate(candidate)
	if err != nil {
		return LegacyMigrationRecoveryResult{}, err
	}
	defer func() { clear(protectedPayload) }()

	var result LegacyMigrationRecoveryResult
	err = manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		for migrationID, recovery := range state.LegacyMigrationRecoveries {
			if migrationID != normalized.ID &&
				recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared {
				return registryport.ErrConflict
			}
		}
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, err = storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := checkLegacyMigrationExpected(state, candidate.Expected, normalized.ProviderID); err != nil {
			return err
		}
		if existing, ok := state.LegacyMigrationRecoveries[normalized.ID]; ok {
			if !legacyMigrationCandidateMatches(existing, normalized) ||
				existing.Phase == domainregistry.LegacyMigrationRecoveryPhaseAbandoning ||
				existing.RemigrationPending {
				return registryport.ErrConflict
			}
			if existing.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
				result, err = manager.replayCommittedRetainedLegacyMigrationRecovery(
					ctx,
					state,
					existing,
					protectedPayload,
				)
				return err
			}
			if !legacyMigrationRecoveryFenceMatches(state, existing) {
				return registryport.ErrConflict
			}
			result, err = manager.advanceLegacyMigrationRecovery(ctx, storage, existing.ID, protectedPayload, true)
			return err
		}

		if err := manager.recoverLegacyMigrationRecoveries(ctx, storage); err != nil {
			return err
		}
		state, err = storage.Load(ctx)
		if err != nil || state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := checkLegacyMigrationExpected(state, candidate.Expected, normalized.ProviderID); err != nil {
			return err
		}
		boundPayload, priorIdentity, bindErr := bindLegacyMigrationPriorSelectedProvider(
			protectedPayload, state,
		)
		if bindErr != nil {
			clear(boundPayload)
			return bindErr
		}
		clear(protectedPayload)
		protectedPayload = boundPayload
		normalized.PriorSelectedProviderIdentitySHA256 = priorIdentity
		for _, recovery := range state.LegacyMigrationRecoveries {
			if recovery.ProviderID == normalized.ProviderID &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalized {
				return registryport.ErrConflict
			}
		}
		if err := manager.prepareLegacyMigrationRecoverySecret(ctx, storage, state, normalized, protectedPayload); err != nil {
			return err
		}
		result, err = manager.advanceLegacyMigrationRecovery(ctx, storage, normalized.ID, protectedPayload, true)
		return err
	})
	if err != nil {
		return LegacyMigrationRecoveryResult{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) CommitVerifiedLegacyMigrationRecovery(
	ctx context.Context,
	command CommitVerifiedLegacyMigrationRecoveryCommand,
) (CommitVerifiedLegacyMigrationRecoveryResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		(command.SourceLocator != "" && !validLegacyMigrationSourceLocator([]byte(command.SourceLocator))) ||
		!validLowerSHA256(command.SourceSHA256) ||
		secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil ||
		command.Confirmation != LegacyMigrationCommitConfirmation {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrInvalidRequest
	}

	credential, provider, purpose, result, complete, err := manager.prepareVerifiedLegacyMigrationProviderCommit(ctx, command)
	if err != nil {
		clear(credential)
		return CommitVerifiedLegacyMigrationRecoveryResult{}, normalizeManagerError(err)
	}
	defer clear(credential)
	if complete {
		return result, nil
	}
	if err := manager.observe(FaultBeforeMigrationProviderTransactionPrepared); err != nil {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, err
	}
	_, err = manager.execute(ctx, mutationPlan{
		operation: domainregistry.OperationConnect,
		expected:  command.Expected,
		provider:  provider, providerID: provider.ID,
		purpose:             purpose,
		recoveredCredential: credential,
		legacyMigrationID:   command.MigrationID,
	})
	if err != nil && !errors.Is(err, registryport.ErrConflict) {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, err
	}
	completed, completionErr := manager.completeVerifiedLegacyMigrationProviderCommit(ctx, command)
	if completionErr != nil {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, normalizeManagerError(completionErr)
	}
	return completed, nil
}

func (manager *Manager) prepareVerifiedLegacyMigrationProviderCommit(
	ctx context.Context,
	command CommitVerifiedLegacyMigrationRecoveryCommand,
) ([]byte, domainregistry.ProviderInput, secretstoreport.Purpose, CommitVerifiedLegacyMigrationRecoveryResult, bool, error) {
	var credential []byte
	var provider domainregistry.ProviderInput
	var purpose secretstoreport.Purpose
	var result CommitVerifiedLegacyMigrationRecoveryResult
	complete := false
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
			!legacyMigrationCommitExpectedMatches(command, recovery) {
			return registryport.ErrConflict
		}
		if recovery.RemigrationPending &&
			(!validLowerSHA256(command.remigrationAuthorityIntent) ||
				command.remigrationAuthorityIntent != recovery.PendingSourceAuthorityIntentSHA256) {
			return registryport.ErrVerification
		}
		payloadPlaintext, payload, err := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
		if err != nil {
			clear(payloadPlaintext)
			return err
		}
		defer clear(payloadPlaintext)
		if !legacyMigrationPayloadSourceMatches(payload, command.SourceLocator, command.SourceSHA256) ||
			(payload.version != legacyMigrationRecoveryPayloadVersionV5 &&
				payload.version != legacyMigrationRecoveryPayloadVersionV6) {
			return registryport.ErrConflict
		}

		switch recovery.Phase {
		case domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained:
			result, err = manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload)
			complete = err == nil
			return err
		case domainregistry.LegacyMigrationRecoveryPhaseVerified:
			if len(state.Transactions) != 0 || !legacyMigrationRecoveryFenceMatches(state, recovery) ||
				recovery.Fence.ProviderExists ||
				!legacyMigrationCommitAllowsRetainedRecoveries(state, recovery) {
				return registryport.ErrConflict
			}
			if _, exists := state.Providers[recovery.ProviderID]; exists {
				return registryport.ErrConflict
			}
			if !recovery.RemigrationPending && command.remigrationAuthorityIntent != "" {
				return registryport.ErrVerification
			}
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
			if state.Validate() != nil {
				return registryport.ErrPersistence
			}
			if err := storage.Commit(ctx, state); err != nil {
				return normalizeRegistryStoreError(err)
			}
		case domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared:
			if len(state.Transactions) != 0 {
				if len(state.Transactions) != 1 {
					return registryport.ErrConflict
				}
				for _, transaction := range state.Transactions {
					if transaction.ProviderID != recovery.ProviderID {
						return registryport.ErrConflict
					}
					if err := manager.validateLegacyMigrationProviderTransaction(ctx, state, transaction); err != nil {
						return err
					}
				}
				if err := manager.recoverTransactions(ctx, storage); err != nil {
					return err
				}
				state, recovery, err = loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
				if err != nil {
					return err
				}
			}
		default:
			return registryport.ErrConflict
		}

		state, recovery, err = loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if _, exists := state.Providers[recovery.ProviderID]; exists {
			result, err = manager.promoteLegacyMigrationProviderWinner(ctx, storage, state, recovery, payload)
			complete = err == nil
			return err
		}
		if len(state.Transactions) != 0 || !legacyMigrationProviderCommitPristine(state, recovery) {
			return registryport.ErrConflict
		}
		credential = bytes.Clone(payload.credential)
		provider = cloneProviderInput(recovery.Provider)
		purpose = secretstoreport.Purpose(recovery.CredentialPurpose)
		result = pendingLegacyMigrationProviderCommitResult(recovery)
		return nil
	})
	if err != nil {
		clear(credential)
		return nil, domainregistry.ProviderInput{}, "", CommitVerifiedLegacyMigrationRecoveryResult{}, false, err
	}
	return credential, provider, purpose, result, complete, nil
}

func (manager *Manager) completeVerifiedLegacyMigrationProviderCommit(
	ctx context.Context,
	command CommitVerifiedLegacyMigrationRecoveryCommand,
) (CommitVerifiedLegacyMigrationRecoveryResult, error) {
	var result CommitVerifiedLegacyMigrationRecoveryResult
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
			!legacyMigrationCommitExpectedMatches(command, recovery) {
			return registryport.ErrConflict
		}
		plaintext, payload, err := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
		if err != nil {
			clear(plaintext)
			return err
		}
		defer clear(plaintext)
		if !legacyMigrationPayloadSourceMatches(payload, command.SourceLocator, command.SourceSHA256) {
			return registryport.ErrConflict
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
			result, err = manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload)
			return err
		}
		if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
			len(state.Transactions) != 0 {
			return registryport.ErrConflict
		}
		result, err = manager.promoteLegacyMigrationProviderWinner(ctx, storage, state, recovery, payload)
		return err
	})
	return result, err
}

func legacyMigrationCommitExpectedMatches(
	command CommitVerifiedLegacyMigrationRecoveryCommand,
	recovery domainregistry.LegacyMigrationRecovery,
) bool {
	expected := command.Expected
	fence := recovery.Fence
	return !fence.ProviderExists && expected.RegistryRevision == fence.RegistryRevision &&
		expected.RegistryIncarnation == fence.RegistryIncarnation &&
		expected.ProviderRevision == 0 && expected.ProviderGeneration == 0 &&
		expected.ProviderIncarnation == "" && expected.ProviderCredentialPurpose == "" &&
		command.ExpectedSelectedProviderID == fence.SelectedProviderID
}

func legacyMigrationProviderCommitPristine(
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
) bool {
	if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
		recovery.Fence.ProviderExists || state.Revision != recovery.Fence.RegistryRevision ||
		state.Incarnation != recovery.Fence.RegistryIncarnation ||
		state.SelectedProviderID != recovery.Fence.SelectedProviderID ||
		!legacyMigrationCommitAllowsRetainedRecoveries(state, recovery) {
		return false
	}
	_, exists := state.Providers[recovery.ProviderID]
	return !exists
}

func legacyMigrationCommitAllowsRetainedRecoveries(
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
) bool {
	for migrationID, existing := range state.LegacyMigrationRecoveries {
		if migrationID == recovery.ID {
			continue
		}
		if existing.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained &&
			existing.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalized {
			return false
		}
	}
	return true
}

func pendingLegacyMigrationProviderCommitResult(
	recovery domainregistry.LegacyMigrationRecovery,
) CommitVerifiedLegacyMigrationRecoveryResult {
	return CommitVerifiedLegacyMigrationRecoveryResult{
		MigrationID:           recovery.ID,
		RecoveryCredentialRef: secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
	}
}

func (manager *Manager) readAndValidateLegacyMigrationRecovery(
	ctx context.Context,
	recovery domainregistry.LegacyMigrationRecovery,
) ([]byte, legacyMigrationRecoveryPayloadView, error) {
	plaintext, err := manager.readLegacyMigrationRecovery(ctx, recovery)
	if err != nil {
		return plaintext, legacyMigrationRecoveryPayloadView{}, normalizeSecretError(err)
	}
	payload, err := parseLegacyMigrationRecoveryPayload(plaintext)
	if err != nil || !legacyMigrationRecoveryPayloadMatchesRecord(recovery, payload) {
		return plaintext, legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	return plaintext, payload, nil
}

func (manager *Manager) InventoryLegacyMigrationRecoveries(
	ctx context.Context,
) (LegacyMigrationRecoveryInventoryResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil {
		return LegacyMigrationRecoveryInventoryResult{}, registryport.ErrInvalidRequest
	}
	result := LegacyMigrationRecoveryInventoryResult{Recoveries: []LegacyMigrationRecoveryDescriptor{}}
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, err := storage.Load(ctx)
		if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
			return registryport.ErrPersistence
		}
		ids := make([]string, 0, len(state.LegacyMigrationRecoveries))
		for id, recovery := range state.LegacyMigrationRecoveries {
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalized &&
				recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		identities := make(map[string]string, len(ids))
		cleanedDigests := make(map[string]string, len(ids))
		commitOrders := make(map[string]map[uint64]struct{}, len(ids))
		seenProviders := make(map[string]struct{}, len(ids))
		seenRecoveryRefs := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			recovery := state.LegacyMigrationRecoveries[id]
			plaintext, rawReadErr := manager.readLegacyMigrationRecovery(ctx, recovery)
			payload := legacyMigrationRecoveryPayloadView{}
			readErr := rawReadErr
			if readErr == nil {
				payload, readErr = parseLegacyMigrationRecoveryPayload(plaintext)
				if readErr != nil || !legacyMigrationRecoveryPayloadMatchesRecord(recovery, payload) {
					readErr = registryport.ErrVerification
				}
			}
			payloadAvailable := readErr == nil
			if readErr != nil {
				clear(plaintext)
				missingAfterIntent := (recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted ||
					recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback ||
					recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending) &&
					recovery.FinalizationRecoveryDeleteAuthorized &&
					validLowerSHA256(recovery.PendingSourceAuthorityIntentSHA256) &&
					(errors.Is(readErr, secretstoreport.ErrNotFound) ||
						errors.Is(readErr, secretstoreport.ErrTombstoned))
				if !missingAfterIntent {
					return normalizeSecretError(readErr)
				}
			}
			if payloadAvailable && ((payload.version != legacyMigrationRecoveryPayloadVersionV5 &&
				payload.version != legacyMigrationRecoveryPayloadVersionV6) ||
				!legacyMigrationPayloadLocatorsBelongToSource(payload)) {
				clear(plaintext)
				return registryport.ErrVerification
			}
			sourceLocator := recovery.SourceLocator
			sourceSHA256 := recovery.SourceSHA256
			sourcePhysicalIdentitySHA256 := recovery.SourcePhysicalIdentitySHA256
			expectedCleanedSourceSHA256 := recovery.ExpectedCleanedSourceSHA256
			if payloadAvailable {
				sourceLocator = string(payload.sourceLocator)
				sourceSHA256 = string(payload.sourceSHA256)
				sourcePhysicalIdentitySHA256 = string(payload.sourcePhysicalIdentitySHA256)
				expectedCleanedSourceSHA256 = string(payload.expectedCleanedSourceSHA256)
			}
			if !validLegacyMigrationSourceLocator([]byte(sourceLocator)) ||
				!validLowerSHA256(sourceSHA256) || !validLowerSHA256(expectedCleanedSourceSHA256) ||
				!validLowerSHA256(sourcePhysicalIdentitySHA256) {
				clear(plaintext)
				return registryport.ErrVerification
			}
			identity := legacyMigrationSourceIdentity(
				sourceLocator, sourceSHA256, sourcePhysicalIdentitySHA256,
			)
			if recovery.SourceIdentitySHA256 != identity {
				clear(plaintext)
				return registryport.ErrVerification
			}
			if prior, exists := identities[identity]; exists &&
				prior != sourceLocator+"\x00"+sourceSHA256+"\x00"+sourcePhysicalIdentitySHA256 {
				clear(plaintext)
				return registryport.ErrVerification
			}
			identities[identity] = sourceLocator + "\x00" + sourceSHA256 + "\x00" + sourcePhysicalIdentitySHA256
			if prior, exists := cleanedDigests[identity]; exists && prior != expectedCleanedSourceSHA256 {
				clear(plaintext)
				return registryport.ErrVerification
			}
			cleanedDigests[identity] = expectedCleanedSourceSHA256
			if _, duplicate := seenProviders[recovery.ProviderID]; duplicate {
				clear(plaintext)
				return registryport.ErrVerification
			}
			seenProviders[recovery.ProviderID] = struct{}{}
			if _, duplicate := seenRecoveryRefs[recovery.RecoveryCredentialRef]; duplicate {
				clear(plaintext)
				return registryport.ErrVerification
			}
			seenRecoveryRefs[recovery.RecoveryCredentialRef] = struct{}{}

			committed := false
			switch recovery.Phase {
			case domainregistry.LegacyMigrationRecoveryPhasePrepared,
				domainregistry.LegacyMigrationRecoveryPhaseSecretDurable,
				domainregistry.LegacyMigrationRecoveryPhaseVerified:
				if !legacyMigrationRecoveryFenceMatches(state, recovery) {
					clear(plaintext)
					return registryport.ErrVerification
				}
			case domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared:
				if !legacyMigrationProviderCommitPristine(state, recovery) {
					clear(plaintext)
					return registryport.ErrVerification
				}
			case domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained,
				domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending,
				domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending:
				if _, verifyErr := manager.verifyCommittedRetainedLegacyMigrationRecovery(
					ctx, state, recovery, payload,
				); verifyErr != nil {
					clear(plaintext)
					return verifyErr
				}
				committed = true
			case domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained:
				if verifyErr := manager.verifyRollbackCommittedLegacyMigrationRecovery(
					ctx, state, recovery, payload,
				); verifyErr != nil {
					clear(plaintext)
					return verifyErr
				}
				committed = true
			case domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained:
				if verifyErr := manager.verifyRollbackRetainedLegacyMigrationRecovery(
					ctx, state, recovery, payload,
				); verifyErr != nil {
					clear(plaintext)
					return verifyErr
				}
				committed = true
			case domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending:
				if payloadAvailable {
					verificationRecord := recovery.Clone()
					verificationRecord.Phase = domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained
					verificationRecord.PendingSourceAuthorityIntentSHA256 = ""
					verificationRecord.FinalizationRecoveryDeleteAuthorized = false
					if verifyErr := manager.verifyRollbackRetainedLegacyMigrationRecovery(
						ctx, state, verificationRecord, payload,
					); verifyErr != nil {
						clear(plaintext)
						return verifyErr
					}
				} else if state.Revision != recovery.RollbackRegistryRevision ||
					state.Incarnation != recovery.RollbackRegistryIncarnation ||
					state.SelectedProviderID != recovery.RollbackSelectedProviderID {
					return registryport.ErrVerification
				}
				committed = true
			case domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted:
				if payloadAvailable {
					if _, verifyErr := manager.verifyCommittedRetainedLegacyMigrationRecovery(
						ctx, state, recovery, payload,
					); verifyErr != nil {
						clear(plaintext)
						return verifyErr
					}
				} else {
					provider, exists := state.Providers[recovery.ProviderID]
					if !exists || !legacyMigrationProviderWinnerMatchesRecovery(provider, recovery) {
						return registryport.ErrVerification
					}
				}
				committed = true
			case domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback:
				if payloadAvailable {
					if verifyErr := manager.verifyRollbackCommittedLegacyMigrationRecovery(
						ctx, state, recovery, payload,
					); verifyErr != nil {
						clear(plaintext)
						return verifyErr
					}
				} else if state.Revision != recovery.RollbackRegistryRevision ||
					state.Incarnation != recovery.RollbackRegistryIncarnation ||
					state.SelectedProviderID != recovery.RollbackSelectedProviderID {
					return registryport.ErrVerification
				}
				committed = true
			default:
				clear(plaintext)
				return registryport.ErrVerification
			}
			clear(plaintext)
			if committed {
				orders := commitOrders[identity]
				if orders == nil {
					orders = make(map[uint64]struct{})
					commitOrders[identity] = orders
				}
				if _, duplicate := orders[recovery.Fence.RegistryRevision]; duplicate {
					return registryport.ErrVerification
				}
				orders[recovery.Fence.RegistryRevision] = struct{}{}
			}
			result.Recoveries = append(result.Recoveries, LegacyMigrationRecoveryDescriptor{
				MigrationID: recovery.ID, ProviderID: recovery.ProviderID,
				SourceLocator: sourceLocator, SourceSHA256: sourceSHA256,
				ExpectedCleanedSourceSHA256:  expectedCleanedSourceSHA256,
				SourcePhysicalIdentitySHA256: sourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
				Phase:                        recovery.Phase, CommitOrder: recovery.Fence.RegistryRevision,
			})
		}
		sort.Slice(result.Recoveries, func(left, right int) bool {
			first, second := result.Recoveries[left], result.Recoveries[right]
			if first.SourceLocator != second.SourceLocator {
				return first.SourceLocator < second.SourceLocator
			}
			if first.SourceSHA256 != second.SourceSHA256 {
				return first.SourceSHA256 < second.SourceSHA256
			}
			if first.CommitOrder != second.CommitOrder {
				return first.CommitOrder < second.CommitOrder
			}
			return first.MigrationID < second.MigrationID
		})
		return nil
	})
	if err != nil {
		return LegacyMigrationRecoveryInventoryResult{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) promoteLegacyMigrationProviderWinner(
	ctx context.Context,
	storage registryport.Transaction,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
	payload legacyMigrationRecoveryPayloadView,
) (CommitVerifiedLegacyMigrationRecoveryResult, error) {
	if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
		len(state.Transactions) != 0 || recovery.Fence.RegistryRevision == math.MaxUint64 ||
		state.Revision != recovery.Fence.RegistryRevision+1 ||
		state.Incarnation != recovery.Fence.RegistryIncarnation ||
		!legacyMigrationCommitAllowsRetainedRecoveries(state, recovery) {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrVerification
	}
	expectedSelected := recovery.Fence.SelectedProviderID
	if expectedSelected == "" {
		expectedSelected = recovery.ProviderID
	}
	provider, exists := state.Providers[recovery.ProviderID]
	if !exists || state.SelectedProviderID != expectedSelected ||
		!legacyMigrationProviderWinnerMatchesRecovery(provider, recovery) {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrVerification
	}
	winnerPlaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
		Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
		Consumer:      RegistryReadbackConsumer,
	})
	if err != nil {
		clear(winnerPlaintext)
		return CommitVerifiedLegacyMigrationRecoveryResult{}, normalizeSecretError(err)
	}
	if !bytes.Equal(winnerPlaintext, payload.credential) {
		clear(winnerPlaintext)
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrVerification
	}
	clear(winnerPlaintext)
	if err := manager.observe(FaultBeforeMigrationRecoveryCommittedRetainedRecord); err != nil {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, err
	}

	state = state.Clone()
	if recovery.RemigrationPending {
		clearLegacyMigrationRollbackTerminalForRemigration(&recovery)
	}
	recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained
	recovery.CommittedProviderCredentialRef = provider.CredentialRef
	recovery.CommittedProviderCredentialPurpose = provider.CredentialPurpose
	recovery.CommittedProviderRevision = provider.Revision
	recovery.CommittedProviderGeneration = provider.Generation
	recovery.CommittedProviderIncarnation = provider.Incarnation
	state.LegacyMigrationRecoveries[recovery.ID] = recovery
	if state.Validate() != nil {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrPersistence
	}
	if err := storage.Commit(ctx, state); err != nil {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, normalizeRegistryStoreError(err)
	}
	if err := manager.observe(FaultAfterMigrationRecoveryCommittedRetainedRecorded); err != nil {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, err
	}
	return committedRetainedLegacyMigrationResult(provider, recovery), nil
}

func (manager *Manager) verifyCommittedRetainedLegacyMigrationRecovery(
	ctx context.Context,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
	payload legacyMigrationRecoveryPayloadView,
) (CommitVerifiedLegacyMigrationRecoveryResult, error) {
	if (recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained &&
		recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending &&
		recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending &&
		recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted) ||
		len(state.Transactions) != 0 {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrVerification
	}
	provider, exists := state.Providers[recovery.ProviderID]
	if !exists || !legacyMigrationProviderWinnerMatchesRecovery(provider, recovery) ||
		provider.CredentialRef != recovery.CommittedProviderCredentialRef ||
		provider.CredentialPurpose != recovery.CommittedProviderCredentialPurpose ||
		provider.Revision != recovery.CommittedProviderRevision ||
		provider.Generation != recovery.CommittedProviderGeneration ||
		provider.Incarnation != recovery.CommittedProviderIncarnation {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrVerification
	}
	winnerPlaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(provider.CredentialRef),
		Purpose:       secretstoreport.Purpose(provider.CredentialPurpose),
		Consumer:      RegistryReadbackConsumer,
	})
	if err != nil {
		clear(winnerPlaintext)
		return CommitVerifiedLegacyMigrationRecoveryResult{}, normalizeSecretError(err)
	}
	defer clear(winnerPlaintext)
	if !bytes.Equal(winnerPlaintext, payload.credential) {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrVerification
	}
	return committedRetainedLegacyMigrationResult(provider, recovery), nil
}

func (manager *Manager) verifyRollbackCommittedLegacyMigrationRecovery(
	ctx context.Context,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
	payload legacyMigrationRecoveryPayloadView,
) error {
	if (recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained &&
		recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback) ||
		len(state.Transactions) != 0 || state.Revision != recovery.RollbackRegistryRevision ||
		state.Incarnation != recovery.RollbackRegistryIncarnation ||
		state.SelectedProviderID != recovery.RollbackSelectedProviderID ||
		((payload.version == legacyMigrationRecoveryPayloadVersionV5 ||
			payload.version == legacyMigrationRecoveryPayloadVersionV6) &&
			recovery.RollbackCleanedSourceStateIdentitySHA256 != legacyMigrationSourceStateIdentity(
				string(payload.sourceLocator), string(payload.sourceSHA256),
				string(payload.expectedCleanedSourceSHA256),
				string(payload.sourcePhysicalIdentitySHA256),
			)) {
		return registryport.ErrVerification
	}
	if _, exists := state.Providers[recovery.ProviderID]; exists {
		return registryport.ErrVerification
	}
	if _, err := manager.verifyLegacyMigrationPriorSelectedProvider(ctx, state, recovery, payload); err != nil {
		return err
	}
	winnerPlaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(recovery.CommittedProviderCredentialRef),
		Purpose:       secretstoreport.Purpose(recovery.CommittedProviderCredentialPurpose),
		Consumer:      RegistryReadbackConsumer,
	})
	if err != nil {
		clear(winnerPlaintext)
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback &&
			recovery.FinalizationWinnerDeleteAuthorized &&
			(errors.Is(err, secretstoreport.ErrNotFound) || errors.Is(err, secretstoreport.ErrTombstoned)) {
			return nil
		}
		return normalizeSecretError(err)
	}
	defer clear(winnerPlaintext)
	if !bytes.Equal(winnerPlaintext, payload.credential) {
		return registryport.ErrVerification
	}
	return nil
}

func (manager *Manager) verifyLegacyMigrationPriorSelectedProvider(
	ctx context.Context,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
	payload legacyMigrationRecoveryPayloadView,
) (string, error) {
	if payload.version != legacyMigrationRecoveryPayloadVersionV6 ||
		recovery.PriorSelectedProviderIdentitySHA256 !=
			mustLegacyMigrationPriorSelectedProviderIdentity(payload.priorSelectedProvider) {
		return "", registryport.ErrVerification
	}
	if payload.priorSelectedProvider == nil {
		if state.SelectedProviderID == "" || state.SelectedProviderID == recovery.ProviderID {
			return "", nil
		}
		selected, exists := state.Providers[state.SelectedProviderID]
		if !exists || selected.Tombstone {
			return "", registryport.ErrVerification
		}
		if selected.CredentialRef == "" {
			return selected.ID, nil
		}
		for _, sibling := range state.LegacyMigrationRecoveries {
			if sibling.ID != recovery.ID && sibling.ProviderID == selected.ID &&
				sibling.SourceIdentitySHA256 == recovery.SourceIdentitySHA256 {
				return selected.ID, nil
			}
		}
		if selected.ID != recovery.Fence.SelectedProviderID {
			return "", registryport.ErrVerification
		}
		plaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
			CredentialRef: secretstoreport.CredentialRef(selected.CredentialRef),
			Purpose:       secretstoreport.Purpose(selected.CredentialPurpose),
			Consumer:      RegistryReadbackConsumer,
		})
		if err != nil {
			clear(plaintext)
			return "", normalizeSecretError(err)
		}
		defer clear(plaintext)
		if len(plaintext) == 0 {
			return "", registryport.ErrVerification
		}
		return selected.ID, nil
	}
	prior := payload.priorSelectedProvider.Clone()
	if prior.ID != recovery.Fence.SelectedProviderID || prior.ID == recovery.ProviderID {
		return "", registryport.ErrVerification
	}
	current, exists := state.Providers[prior.ID]
	if !exists || !reflect.DeepEqual(current.Clone(), prior) {
		// A later member of the same source group can legitimately name an
		// earlier migrated winner as its prior selection. Once that sibling has
		// itself been rolled back, the protected prior is intentionally absent;
		// the group's terminal selection fence is authoritative.
		for _, sibling := range state.LegacyMigrationRecoveries {
			if sibling.ID == recovery.ID || sibling.ProviderID != prior.ID ||
				sibling.SourceIdentitySHA256 != recovery.SourceIdentitySHA256 {
				continue
			}
			switch sibling.Phase {
			case domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained,
				domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback,
				domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained:
				if state.SelectedProviderID == recovery.RollbackSelectedProviderID {
					return state.SelectedProviderID, nil
				}
			}
		}
		return "", registryport.ErrVerification
	}
	if state.SelectedProviderID != prior.ID {
		return "", registryport.ErrVerification
	}
	plaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(prior.CredentialRef),
		Purpose:       secretstoreport.Purpose(prior.CredentialPurpose),
		Consumer:      RegistryReadbackConsumer,
	})
	if err != nil {
		clear(plaintext)
		return "", normalizeSecretError(err)
	}
	defer clear(plaintext)
	if len(plaintext) == 0 {
		return "", registryport.ErrVerification
	}
	return prior.ID, nil
}

func (manager *Manager) verifyRollbackRetainedLegacyMigrationRecovery(
	_ context.Context,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
	payload legacyMigrationRecoveryPayloadView,
) error {
	if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained ||
		len(state.Transactions) != 0 ||
		state.Incarnation != recovery.RollbackRegistryIncarnation ||
		recovery.FinalizationOutcome != domainregistry.LegacyMigrationFinalizationOutcomeRolledBack ||
		recovery.RollbackCleanedSourceStateIdentitySHA256 != legacyMigrationSourceStateIdentity(
			string(payload.sourceLocator), string(payload.sourceSHA256),
			string(payload.expectedCleanedSourceSHA256), string(payload.sourcePhysicalIdentitySHA256),
		) {
		return registryport.ErrVerification
	}
	if _, exists := state.Providers[recovery.ProviderID]; exists {
		return registryport.ErrVerification
	}
	for _, provider := range state.Providers {
		if provider.CredentialRef == recovery.RecoveryCredentialRef {
			return registryport.ErrVerification
		}
	}
	return nil
}

func (manager *Manager) replayCommittedRetainedLegacyMigrationRecovery(
	ctx context.Context,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
	expectedProtectedPayload []byte,
) (LegacyMigrationRecoveryResult, error) {
	expectedSelectedProviderID := recovery.Fence.SelectedProviderID
	if expectedSelectedProviderID == "" {
		expectedSelectedProviderID = recovery.ProviderID
	}
	if state.SelectedProviderID != expectedSelectedProviderID {
		return LegacyMigrationRecoveryResult{}, registryport.ErrVerification
	}
	plaintext, payload, err := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
	if err != nil {
		clear(plaintext)
		return LegacyMigrationRecoveryResult{}, err
	}
	defer clear(plaintext)
	if !legacyMigrationProtectedPayloadMatchesCandidate(plaintext, expectedProtectedPayload) {
		return LegacyMigrationRecoveryResult{}, registryport.ErrConflict
	}
	if _, err := manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
		return LegacyMigrationRecoveryResult{}, err
	}
	return LegacyMigrationRecoveryResult{
		Status:                LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained,
		MigrationID:           recovery.ID,
		RecoveryCredentialRef: secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
	}, nil
}

func legacyMigrationProtectedPayloadMatchesCandidate(actual, candidate []byte) bool {
	if bytes.Equal(actual, candidate) {
		return true
	}
	header := len(legacyMigrationRecoveryPayloadMagic)
	if len(actual) <= len(candidate) || len(candidate) < header+4 ||
		binary.BigEndian.Uint32(actual[header:header+4]) != legacyMigrationRecoveryPayloadVersionV6 ||
		binary.BigEndian.Uint32(candidate[header:header+4]) != legacyMigrationRecoveryPayloadVersionV5 {
		return false
	}
	prefix := bytes.Clone(actual[:len(candidate)])
	defer clear(prefix)
	binary.BigEndian.PutUint32(prefix[header:header+4], legacyMigrationRecoveryPayloadVersionV5)
	return bytes.Equal(prefix, candidate)
}

func legacyMigrationProviderWinnerMatchesRecovery(
	provider domainregistry.Provider,
	recovery domainregistry.LegacyMigrationRecovery,
) bool {
	if provider.Tombstone || provider.Revision != 1 || provider.Generation != 1 ||
		provider.CredentialRef == "" || provider.CredentialRef == recovery.RecoveryCredentialRef ||
		provider.CredentialPurpose != recovery.CredentialPurpose {
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

func committedRetainedLegacyMigrationResult(
	provider domainregistry.Provider,
	recovery domainregistry.LegacyMigrationRecovery,
) CommitVerifiedLegacyMigrationRecoveryResult {
	return CommitVerifiedLegacyMigrationRecoveryResult{
		Status:                      LegacyMigrationRecoveryStatusProviderCommittedRecoveryRetained,
		MigrationID:                 recovery.ID,
		Provider:                    provider.Clone(),
		RecoveryCredentialRef:       secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
		SafeToRemoveLegacyPlaintext: recovery.ExpectedCleanedSourceIdentitySHA256 != "",
	}
}

func (manager *Manager) validateLegacyMigrationProviderTransaction(
	ctx context.Context,
	state domainregistry.Registry,
	transaction domainregistry.Transaction,
) error {
	var recovery domainregistry.LegacyMigrationRecovery
	found := false
	for _, candidate := range state.LegacyMigrationRecoveries {
		if candidate.ProviderID == transaction.ProviderID &&
			candidate.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared {
			recovery = candidate
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	if !legacyMigrationTransactionMatchesRecovery(transaction, recovery) {
		return registryport.ErrVerification
	}
	recoveryPlaintext, payload, err := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
	if err != nil {
		clear(recoveryPlaintext)
		return err
	}
	defer clear(recoveryPlaintext)
	candidatePlaintext, err := manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(transaction.CandidateCredentialRef),
		Purpose:       secretstoreport.Purpose(transaction.CandidateCredentialPurpose),
		Consumer:      RegistryReadbackConsumer,
	})
	if errors.Is(err, secretstoreport.ErrNotFound) &&
		(transaction.Phase == domainregistry.PhasePrepared || transaction.Phase == domainregistry.PhaseCandidateDurable) {
		clear(candidatePlaintext)
		return nil
	}
	if errors.Is(err, secretstoreport.ErrTombstoned) {
		clear(candidatePlaintext)
		return nil
	}
	if err != nil {
		clear(candidatePlaintext)
		return normalizeSecretError(err)
	}
	defer clear(candidatePlaintext)
	if !bytes.Equal(candidatePlaintext, payload.credential) {
		return registryport.ErrVerification
	}
	return nil
}

func legacyMigrationTransactionMatchesRecovery(
	transaction domainregistry.Transaction,
	recovery domainregistry.LegacyMigrationRecovery,
) bool {
	if transaction.Operation != domainregistry.OperationConnect || transaction.ProviderID != recovery.ProviderID ||
		transaction.PriorProvider != nil || transaction.NextProvider == nil ||
		transaction.CandidateCredentialRef == "" ||
		transaction.CandidateCredentialRef == recovery.RecoveryCredentialRef ||
		transaction.CandidateCredentialPurpose != recovery.CredentialPurpose ||
		transaction.SupersededCredentialRef != "" || transaction.SupersededCredentialPurpose != "" {
		return false
	}
	fence := transaction.Fence
	recoveryFence := recovery.Fence
	if recoveryFence.ProviderExists || fence.RegistryRevision != recoveryFence.RegistryRevision ||
		fence.RegistryIncarnation != recoveryFence.RegistryIncarnation ||
		fence.SelectedProviderID != recoveryFence.SelectedProviderID ||
		fence.ProviderRevision != 0 || fence.ProviderGeneration != 0 || fence.ProviderIncarnation != "" ||
		fence.CurrentCredentialRef != "" || fence.CurrentCredentialPurpose != "" {
		return false
	}
	return legacyMigrationProviderWinnerMatchesRecovery(transaction.NextProvider.Clone(), recovery)
}

func (manager *Manager) AbandonLegacyMigrationRecovery(
	ctx context.Context,
	command AbandonLegacyMigrationRecoveryCommand,
) error {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		(command.SourceLocator != "" && !validLegacyMigrationSourceLocator([]byte(command.SourceLocator))) ||
		!validLowerSHA256(command.SourceSHA256) ||
		secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil ||
		command.Confirmation != LegacyMigrationAbandonConfirmation {
		return registryport.ErrInvalidRequest
	}
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) {
			return registryport.ErrConflict
		}
		if recovery.RemigrationPending {
			return registryport.ErrConflict
		}
		if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseAbandoning {
			if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared ||
				recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
				return registryport.ErrConflict
			}
			if !legacyMigrationRecoveryFenceMatches(state, recovery) {
				return registryport.ErrConflict
			}
			plaintext, readErr := manager.readLegacyMigrationRecovery(ctx, recovery)
			preparedMissing := recovery.Phase == domainregistry.LegacyMigrationRecoveryPhasePrepared &&
				errors.Is(readErr, secretstoreport.ErrNotFound)
			if readErr != nil && !preparedMissing {
				clear(plaintext)
				return normalizeSecretError(readErr)
			}
			if preparedMissing {
				clear(plaintext)
			} else {
				payload, payloadErr := parseLegacyMigrationRecoveryPayload(plaintext)
				recordMatches := payloadErr == nil && legacyMigrationRecoveryPayloadMatchesRecord(recovery, payload)
				sourceMatches := recordMatches && legacyMigrationPayloadSourceMatches(
					payload, command.SourceLocator, command.SourceSHA256,
				)
				clear(plaintext)
				if !recordMatches {
					return registryport.ErrVerification
				}
				if !sourceMatches {
					return registryport.ErrConflict
				}
			}
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseAbandoning
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
			if state.Validate() != nil {
				return registryport.ErrPersistence
			}
			if err := storage.Commit(ctx, state); err != nil {
				return normalizeRegistryStoreError(err)
			}
			if err := manager.observe(FaultAfterMigrationRecoveryAbandonRecorded); err != nil {
				return err
			}
		}
		return manager.finishLegacyMigrationAbandon(ctx, storage, recovery.ID)
	})
	return normalizeManagerError(err)
}

func (manager *Manager) RemigrateRetainedLegacyMigrationRecovery(
	ctx context.Context,
	command RemigrateRetainedLegacyMigrationRecoveryCommand,
) (CommitVerifiedLegacyMigrationRecoveryResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		!validLegacyMigrationSourceLocator([]byte(command.SourceLocator)) ||
		!validLowerSHA256(command.SourceSHA256) ||
		!validLowerSHA256(command.VerifiedCleanedSourceSHA256) ||
		!validLowerSHA256(command.SourcePhysicalIdentitySHA256) ||
		!validLegacyMigrationSourceRepresentation(
			command.VerifiedCleanedSourceSHA256, command.VerifiedCleanedSource,
		) || secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil ||
		command.Confirmation != LegacyMigrationRemigrationConfirmation {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, registryport.ErrInvalidRequest
	}

	var commitCommand CommitVerifiedLegacyMigrationRecoveryCommand
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
			recovery.SourceLocator != command.SourceLocator || recovery.SourceSHA256 != command.SourceSHA256 ||
			recovery.ExpectedCleanedSourceSHA256 != command.VerifiedCleanedSourceSHA256 ||
			recovery.SourcePhysicalIdentitySHA256 != command.SourcePhysicalIdentitySHA256 ||
			checkLegacyMigrationExpected(state, command.Expected, recovery.ProviderID) != nil ||
			len(state.Transactions) != 0 {
			return registryport.ErrConflict
		}
		authorityIntent, sourceGeneration, authorityErr := manager.consumeLegacyMigrationSourceAuthority(
			legacyMigrationSourceAuthorityChallenge{
				Operation:   LegacyMigrationSourceAuthorityOperationRemigrate,
				MigrationID: command.MigrationID, SourceLocator: command.SourceLocator,
				SourceSHA256: command.SourceSHA256, CurrentSourceSHA256: command.VerifiedCleanedSourceSHA256,
				SourcePhysicalIdentitySHA256: command.SourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        command.RecoveryCredentialRef,
			},
			command.SourceAuthority,
			command.VerifiedCleanedSource,
		)
		if authorityErr != nil {
			return authorityErr
		}
		plaintext, payload, err := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
		if err != nil {
			clear(plaintext)
			return err
		}
		defer clear(plaintext)
		if !legacyMigrationSourceReceiptMatches(
			payload, command.SourceLocator, command.SourceSHA256, command.VerifiedCleanedSourceSHA256,
			command.SourcePhysicalIdentitySHA256, command.VerifiedCleanedSource,
		) || !bytes.Equal(payload.expectedCleanedSourceSHA256, []byte(command.VerifiedCleanedSourceSHA256)) ||
			sourceGeneration != recovery.RollbackCleanedSourceGenerationSHA256 {
			return registryport.ErrVerification
		}
		switch {
		case recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained:
			if err := manager.verifyRollbackRetainedLegacyMigrationRecovery(
				ctx, state, recovery, payload,
			); err != nil {
				return err
			}
			if _, exists := state.Providers[recovery.ProviderID]; exists {
				return registryport.ErrConflict
			}
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseVerified
			recovery.Fence = legacyMigrationRecoveryFence(state, recovery.ProviderID)
			recovery.RollbackRegistryRevision = recovery.Fence.RegistryRevision
			recovery.RollbackRegistryIncarnation = recovery.Fence.RegistryIncarnation
			recovery.RollbackSelectedProviderID = recovery.Fence.SelectedProviderID
			recovery.RemigrationPending = true
			recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
		case recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseVerified &&
			recovery.RemigrationPending:
			if !legacyMigrationRecoveryFenceMatches(state, recovery) ||
				recovery.PendingSourceAuthorityIntentSHA256 == "" {
				return registryport.ErrVerification
			}
			recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
		case recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared &&
			recovery.RemigrationPending:
			recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
		default:
			return registryport.ErrConflict
		}
		if state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		commitCommand = CommitVerifiedLegacyMigrationRecoveryCommand{
			Expected: domainregistry.ExpectedState{
				RegistryRevision: recovery.Fence.RegistryRevision, RegistryIncarnation: recovery.Fence.RegistryIncarnation,
			},
			ExpectedSelectedProviderID: recovery.Fence.SelectedProviderID,
			MigrationID:                command.MigrationID, SourceLocator: command.SourceLocator,
			SourceSHA256: command.SourceSHA256, RecoveryCredentialRef: command.RecoveryCredentialRef,
			Confirmation: LegacyMigrationCommitConfirmation, remigrationAuthorityIntent: authorityIntent,
		}
		return nil
	})
	if err != nil {
		return CommitVerifiedLegacyMigrationRecoveryResult{}, normalizeManagerError(err)
	}
	return manager.CommitVerifiedLegacyMigrationRecovery(ctx, commitCommand)
}

func clearLegacyMigrationRollbackTerminalForRemigration(
	recovery *domainregistry.LegacyMigrationRecovery,
) {
	recovery.RollbackRegistryRevision = 0
	recovery.RollbackRegistryIncarnation = ""
	recovery.RollbackSelectedProviderID = ""
	recovery.RollbackCleanedSourceStateIdentitySHA256 = ""
	recovery.RollbackCleanedSourceGenerationSHA256 = ""
	recovery.FinalizationOutcome = ""
	recovery.FinalizationPreCommit = false
	recovery.FinalizedSourceIdentitySHA256 = ""
	recovery.FinalizedSourceStateIdentitySHA256 = ""
	recovery.FinalizedSourceGenerationSHA256 = ""
	recovery.PendingSourceAuthorityIntentSHA256 = ""
	recovery.RemigrationPending = false
	recovery.FinalizationRecoveryDeleteAuthorized = false
	recovery.FinalizationWinnerDeleteAuthorized = false
}

func (manager *Manager) DeleteRetainedLegacyMigrationRecovery(
	ctx context.Context,
	command DeleteRetainedLegacyMigrationRecoveryCommand,
) (DeleteRetainedLegacyMigrationRecoveryResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		!validLegacyMigrationSourceLocator([]byte(command.SourceLocator)) ||
		!validLowerSHA256(command.SourceSHA256) ||
		!validLowerSHA256(command.VerifiedCleanedSourceSHA256) ||
		!validLowerSHA256(command.SourcePhysicalIdentitySHA256) ||
		!validLegacyMigrationSourceRepresentation(
			command.VerifiedCleanedSourceSHA256, command.VerifiedCleanedSource,
		) || secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil ||
		command.Confirmation != LegacyMigrationProtectedDeleteConfirmation {
		return DeleteRetainedLegacyMigrationRecoveryResult{}, registryport.ErrInvalidRequest
	}

	var result DeleteRetainedLegacyMigrationRecoveryResult
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if checkLegacyMigrationExpected(state, command.Expected, recovery.ProviderID) != nil ||
			recovery.SourceLocator != command.SourceLocator || recovery.SourceSHA256 != command.SourceSHA256 ||
			recovery.ExpectedCleanedSourceSHA256 != command.VerifiedCleanedSourceSHA256 ||
			recovery.SourcePhysicalIdentitySHA256 != command.SourcePhysicalIdentitySHA256 {
			return registryport.ErrConflict
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
			if recovery.FinalizedSourceIdentitySHA256 != legacyMigrationSourceIdentity(
				command.SourceLocator, command.SourceSHA256, command.SourcePhysicalIdentitySHA256,
			) || recovery.FinalizedSourceStateIdentitySHA256 != legacyMigrationSourceStateIdentity(
				command.SourceLocator, command.SourceSHA256, command.VerifiedCleanedSourceSHA256,
				command.SourcePhysicalIdentitySHA256,
			) {
				return registryport.ErrConflict
			}
			result = DeleteRetainedLegacyMigrationRecoveryResult{
				Status: LegacyMigrationProtectedDeleteStatusAlreadyDeleted, MigrationID: recovery.ID,
			}
			return nil
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
			len(state.Transactions) != 0 {
			return registryport.ErrConflict
		}
		authorityIntent, sourceGeneration, authorityErr := manager.consumeLegacyMigrationSourceAuthority(
			legacyMigrationSourceAuthorityChallenge{
				Operation:   LegacyMigrationSourceAuthorityOperationProtectedDelete,
				MigrationID: command.MigrationID, SourceLocator: command.SourceLocator,
				SourceSHA256: command.SourceSHA256, CurrentSourceSHA256: command.VerifiedCleanedSourceSHA256,
				SourcePhysicalIdentitySHA256: command.SourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        command.RecoveryCredentialRef,
			},
			command.SourceAuthority,
			command.VerifiedCleanedSource,
		)
		if authorityErr != nil {
			return authorityErr
		}
		if sourceGeneration != recovery.RollbackCleanedSourceGenerationSHA256 {
			return registryport.ErrVerification
		}

		payloadAvailable := false
		plaintext, readErr := manager.readLegacyMigrationRecovery(ctx, recovery)
		payload := legacyMigrationRecoveryPayloadView{}
		if readErr == nil {
			payload, readErr = parseLegacyMigrationRecoveryPayload(plaintext)
			if readErr != nil || !legacyMigrationRecoveryPayloadMatchesRecord(recovery, payload) {
				readErr = registryport.ErrVerification
			}
		}
		if readErr == nil {
			payloadAvailable = true
			defer clear(plaintext)
			if !legacyMigrationSourceReceiptMatches(
				payload, command.SourceLocator, command.SourceSHA256, command.VerifiedCleanedSourceSHA256,
				command.SourcePhysicalIdentitySHA256, command.VerifiedCleanedSource,
			) {
				return registryport.ErrVerification
			}
		} else {
			clear(plaintext)
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending ||
				!recovery.FinalizationRecoveryDeleteAuthorized ||
				(!errors.Is(readErr, secretstoreport.ErrNotFound) &&
					!errors.Is(readErr, secretstoreport.ErrTombstoned)) {
				return readErr
			}
		}

		switch recovery.Phase {
		case domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained:
			if !payloadAvailable || manager.verifyRollbackRetainedLegacyMigrationRecovery(
				ctx, state, recovery, payload,
			) != nil {
				return registryport.ErrVerification
			}
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending
			recovery.RollbackRegistryRevision = state.Revision
			recovery.RollbackRegistryIncarnation = state.Incarnation
			recovery.RollbackSelectedProviderID = state.SelectedProviderID
			recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
			recovery.FinalizationRecoveryDeleteAuthorized = true
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
		case domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending:
			if !recovery.FinalizationRecoveryDeleteAuthorized {
				return registryport.ErrVerification
			}
			recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
		default:
			return registryport.ErrConflict
		}
		if state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterMigrationProtectedDeleteIntent); err != nil {
			return err
		}
		ref := secretstoreport.CredentialRef(recovery.RecoveryCredentialRef)
		if err := manager.secrets.Tombstone(ctx, ref, LegacyMigrationRecoveryPurpose); err != nil &&
			!errors.Is(err, secretstoreport.ErrTombstoned) && !errors.Is(err, secretstoreport.ErrNotFound) {
			return normalizeSecretError(err)
		}
		if err := manager.observe(FaultAfterMigrationProtectedDeleteTombstoned); err != nil {
			return err
		}
		if err := manager.secrets.ExplicitDelete(
			ctx, ref, LegacyMigrationRecoveryPurpose, secretstoreport.ExplicitlyDeleteCredential(),
		); err != nil && !errors.Is(err, secretstoreport.ErrNotFound) {
			return normalizeSecretError(err)
		}
		if err := manager.observe(FaultAfterMigrationProtectedDeleteDeleted); err != nil {
			return err
		}
		state, recovery, err = loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil || recovery.Phase !=
			domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending ||
			recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
			recovery.PendingSourceAuthorityIntentSHA256 != authorityIntent ||
			!recovery.FinalizationRecoveryDeleteAuthorized {
			return registryport.ErrConflict
		}
		recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted
		recovery.RecoveryCredentialRef = ""
		recovery.ProtectedRecoveryRetained = false
		recovery.PendingSourceAuthorityIntentSHA256 = ""
		recovery.FinalizationRecoveryDeleteAuthorized = false
		state.LegacyMigrationRecoveries[recovery.ID] = recovery
		if state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterMigrationProtectedDeleteRecorded); err != nil {
			return err
		}
		result = DeleteRetainedLegacyMigrationRecoveryResult{
			Status: LegacyMigrationProtectedDeleteStatusCompleted, MigrationID: recovery.ID,
		}
		return nil
	})
	if err != nil {
		return DeleteRetainedLegacyMigrationRecoveryResult{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) BeginLegacyMigrationRollback(
	ctx context.Context,
	command BeginLegacyMigrationRollbackCommand,
) (BeginLegacyMigrationRollbackResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		!validLegacyMigrationSourceLocator([]byte(command.SourceLocator)) ||
		!validLowerSHA256(command.SourceSHA256) ||
		(command.SourcePhysicalIdentitySHA256 != "" &&
			!validLowerSHA256(command.SourcePhysicalIdentitySHA256)) ||
		secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil ||
		command.Confirmation != LegacyMigrationRollbackConfirmation {
		return BeginLegacyMigrationRollbackResult{}, registryport.ErrInvalidRequest
	}

	var result BeginLegacyMigrationRollbackResult
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized {
			expectedSourceIdentity := recovery.SourceIdentitySHA256
			if expectedSourceIdentity == "" {
				expectedSourceIdentity = legacyMigrationSourceIdentityV3(command.SourceLocator, command.SourceSHA256)
			} else if command.SourcePhysicalIdentitySHA256 != "" {
				expectedSourceIdentity = legacyMigrationSourceIdentity(
					command.SourceLocator, command.SourceSHA256, command.SourcePhysicalIdentitySHA256,
				)
			}
			if recovery.FinalizedSourceIdentitySHA256 != expectedSourceIdentity {
				return registryport.ErrConflict
			}
			result = BeginLegacyMigrationRollbackResult{
				Status: LegacyMigrationRollbackStatusAlreadyFinalized, MigrationID: recovery.ID,
			}
			return nil
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
			if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
				recovery.SourceIdentitySHA256 != legacyMigrationSourceIdentity(
					command.SourceLocator, command.SourceSHA256, command.SourcePhysicalIdentitySHA256,
				) {
				return registryport.ErrConflict
			}
			plaintext, payload, readErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
			defer clear(plaintext)
			if readErr != nil {
				return readErr
			}
			if err := manager.verifyRollbackRetainedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
				return err
			}
			result = BeginLegacyMigrationRollbackResult{
				Status: LegacyMigrationRollbackStatusRecoveryRetainedTerminal, MigrationID: recovery.ID,
			}
			return nil
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) {
			return registryport.ErrConflict
		}
		if recovery.RemigrationPending {
			return registryport.ErrConflict
		}

		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingPreCommit {
			outcome, finishErr := manager.finishLegacyMigrationFinalization(ctx, storage, recovery.ID, "")
			if finishErr != nil {
				return finishErr
			}
			if outcome != LegacyMigrationFinalizationOutcomeRolledBack {
				return registryport.ErrVerification
			}
			result = BeginLegacyMigrationRollbackResult{
				Status: LegacyMigrationRollbackStatusPreCommitCompleted, MigrationID: recovery.ID,
			}
			return nil
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback {
			return registryport.ErrConflict
		}

		plaintext, payload, payloadErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
		if payloadErr != nil {
			clear(plaintext)
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhasePrepared {
				return payloadErr
			}
			missingPlaintext, missingErr := manager.readLegacyMigrationRecovery(ctx, recovery)
			clear(missingPlaintext)
			if !errors.Is(missingErr, secretstoreport.ErrNotFound) {
				return payloadErr
			}
		} else {
			defer clear(plaintext)
			if !legacyMigrationPayloadSourceMatches(payload, command.SourceLocator, command.SourceSHA256) ||
				((payload.version == legacyMigrationRecoveryPayloadVersionV5 ||
					payload.version == legacyMigrationRecoveryPayloadVersionV6) &&
					command.SourcePhysicalIdentitySHA256 != "" &&
					!bytes.Equal(payload.sourcePhysicalIdentitySHA256,
						[]byte(command.SourcePhysicalIdentitySHA256))) ||
				(payload.version != legacyMigrationRecoveryPayloadVersionV5 &&
					payload.version != legacyMigrationRecoveryPayloadVersionV6 &&
					command.SourcePhysicalIdentitySHA256 != "") {
				return registryport.ErrConflict
			}
		}

		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared {
			if provider, exists := state.Providers[recovery.ProviderID]; exists {
				if payloadErr != nil {
					return registryport.ErrVerification
				}
				if _, err := manager.promoteLegacyMigrationProviderWinner(ctx, storage, state, recovery, payload); err != nil {
					return err
				}
				state, recovery, err = loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
				if err != nil {
					return err
				}
				_ = provider
			} else if len(state.Transactions) != 0 || !legacyMigrationProviderCommitPristine(state, recovery) {
				return registryport.ErrConflict
			}
		}

		rollbackStatus := LegacyMigrationRollbackStatusCleanedSourceRequired
		switch recovery.Phase {
		case domainregistry.LegacyMigrationRecoveryPhasePrepared,
			domainregistry.LegacyMigrationRecoveryPhaseSecretDurable,
			domainregistry.LegacyMigrationRecoveryPhaseVerified,
			domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared:
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseFinalizingPreCommit
			recovery.FinalizationOutcome = domainregistry.LegacyMigrationFinalizationOutcomeRolledBack
			recovery.FinalizationPreCommit = true
			// A pre-commit recovery never authorized settings cleanup. Preserve the
			// key-free target identity for record coherence, while the explicit
			// pre-commit marker proves that no restored-source receipt or live source
			// generation was required.
			if payload.version == legacyMigrationRecoveryPayloadVersionV5 ||
				payload.version == legacyMigrationRecoveryPayloadVersionV6 {
				sourcePhysicalIdentitySHA256 := string(payload.sourcePhysicalIdentitySHA256)
				recovery.FinalizedSourceIdentitySHA256 = recovery.SourceIdentitySHA256
				recovery.FinalizedSourceStateIdentitySHA256 = legacyMigrationSourceStateIdentity(
					command.SourceLocator, command.SourceSHA256, command.SourceSHA256,
					sourcePhysicalIdentitySHA256,
				)
			} else if recovery.SourceIdentitySHA256 != "" && payloadErr != nil {
				recovery.FinalizedSourceIdentitySHA256 = recovery.SourceIdentitySHA256
				recovery.FinalizedSourceStateIdentitySHA256 = recovery.SourceIdentitySHA256
			} else {
				recovery.FinalizedSourceIdentitySHA256 = legacyMigrationSourceIdentityV3(
					command.SourceLocator, command.SourceSHA256,
				)
				recovery.FinalizedSourceStateIdentitySHA256 = legacyMigrationSourceStateIdentityV4(
					command.SourceLocator, command.SourceSHA256, command.SourceSHA256,
				)
			}
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
			if state.Validate() != nil {
				return registryport.ErrPersistence
			}
			if err := storage.Commit(ctx, state); err != nil {
				return normalizeRegistryStoreError(err)
			}
			if err := manager.observe(FaultAfterMigrationFinalizingRecorded); err != nil {
				return err
			}
			outcome, err := manager.finishLegacyMigrationFinalization(ctx, storage, recovery.ID, "")
			if err != nil {
				return err
			}
			if outcome != LegacyMigrationFinalizationOutcomeRolledBack {
				return registryport.ErrVerification
			}
			result = BeginLegacyMigrationRollbackResult{
				Status: LegacyMigrationRollbackStatusPreCommitCompleted, MigrationID: recovery.ID,
			}
			return nil
		case domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained:
			if payloadErr != nil {
				return payloadErr
			}
			if _, err := manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
				return err
			}
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
			if state.Validate() != nil {
				return registryport.ErrPersistence
			}
			if err := storage.Commit(ctx, state); err != nil {
				return normalizeRegistryStoreError(err)
			}
			if err := manager.observe(FaultAfterMigrationRollbackSourceRestoreRecorded); err != nil {
				return err
			}
		case domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending:
			if payloadErr != nil {
				return payloadErr
			}
			if _, err := manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
				return err
			}
		case domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending:
			if payloadErr != nil {
				return payloadErr
			}
			if _, err := manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
				return err
			}
		case domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained:
			if payloadErr != nil {
				return payloadErr
			}
			if err := manager.verifyRollbackCommittedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
				return err
			}
			rollbackStatus = LegacyMigrationRollbackStatusCommittedRecoveryRetained
		default:
			return registryport.ErrConflict
		}

		result = BeginLegacyMigrationRollbackResult{
			Status: rollbackStatus, MigrationID: recovery.ID,
		}
		return nil
	})
	if err != nil {
		return BeginLegacyMigrationRollbackResult{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) CommitLegacyMigrationRollback(
	ctx context.Context,
	command CommitLegacyMigrationRollbackCommand,
) (CommitLegacyMigrationRollbackResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		!validLegacyMigrationSourceLocator([]byte(command.SourceLocator)) ||
		!validLowerSHA256(command.SourceSHA256) ||
		!validLowerSHA256(command.VerifiedCleanedSourceSHA256) ||
		!validLowerSHA256(command.SourcePhysicalIdentitySHA256) ||
		!validLegacyMigrationSourceRepresentation(command.VerifiedCleanedSourceSHA256, command.VerifiedCleanedSource) ||
		secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil ||
		command.Confirmation != LegacyMigrationRollbackCommitConfirmation {
		return CommitLegacyMigrationRollbackResult{}, registryport.ErrInvalidRequest
	}
	var result CommitLegacyMigrationRollbackResult
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
			if recovery.FinalizedSourceIdentitySHA256 != legacyMigrationSourceIdentity(
				command.SourceLocator, command.SourceSHA256, command.SourcePhysicalIdentitySHA256,
			) ||
				recovery.FinalizationOutcome != domainregistry.LegacyMigrationFinalizationOutcomeRolledBack ||
				recovery.FinalizedSourceStateIdentitySHA256 != legacyMigrationSourceStateIdentity(
					command.SourceLocator, command.SourceSHA256, command.VerifiedCleanedSourceSHA256,
					command.SourcePhysicalIdentitySHA256,
				) {
				return registryport.ErrConflict
			}
			result = CommitLegacyMigrationRollbackResult{
				Status: LegacyMigrationRollbackStatusAlreadyFinalized, MigrationID: recovery.ID,
			}
			return nil
		}
		if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) ||
			command.VerifiedCleanedSourceSHA256 != recovery.ExpectedCleanedSourceSHA256 {
			return registryport.ErrConflict
		}
		authorityIntent, sourceGeneration, authorityErr := manager.consumeLegacyMigrationSourceAuthority(
			legacyMigrationSourceAuthorityChallenge{
				Operation:   LegacyMigrationSourceAuthorityOperationRollbackCommit,
				MigrationID: command.MigrationID, SourceLocator: command.SourceLocator,
				SourceSHA256: command.SourceSHA256, CurrentSourceSHA256: command.VerifiedCleanedSourceSHA256,
				SourcePhysicalIdentitySHA256: command.SourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        command.RecoveryCredentialRef,
			},
			command.SourceAuthority,
			command.VerifiedCleanedSource,
		)
		if authorityErr != nil {
			return authorityErr
		}
		plaintext, payload, err := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
		if err != nil {
			clear(plaintext)
			return err
		}
		defer clear(plaintext)
		if !legacyMigrationSourceReceiptMatches(
			payload, command.SourceLocator, command.SourceSHA256, command.VerifiedCleanedSourceSHA256,
			command.SourcePhysicalIdentitySHA256, command.VerifiedCleanedSource,
		) || !bytes.Equal(payload.expectedCleanedSourceSHA256, []byte(command.VerifiedCleanedSourceSHA256)) {
			return registryport.ErrConflict
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained {
			if recovery.RollbackCleanedSourceGenerationSHA256 != sourceGeneration {
				return registryport.ErrConflict
			}
			if err := manager.verifyRollbackCommittedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
				return err
			}
			result = CommitLegacyMigrationRollbackResult{
				Status: LegacyMigrationRollbackStatusCommittedRecoveryRetained, MigrationID: recovery.ID,
			}
			return nil
		}
		if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending &&
			recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending {
			return registryport.ErrConflict
		}
		if len(state.Transactions) != 0 ||
			checkLegacyMigrationExpected(state, command.Expected, recovery.ProviderID) != nil ||
			!legacyMigrationRollbackOrderAllows(state, recovery) {
			return registryport.ErrConflict
		}
		if _, err := manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload); err != nil {
			return err
		}
		rollbackSelectedProviderID, err := manager.verifyLegacyMigrationPriorSelectedProvider(
			ctx, state, recovery, payload,
		)
		if err != nil {
			return err
		}
		cleanedStateIdentity := legacyMigrationSourceStateIdentity(
			command.SourceLocator, command.SourceSHA256, command.VerifiedCleanedSourceSHA256,
			command.SourcePhysicalIdentitySHA256,
		)
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending &&
			(recovery.RollbackCleanedSourceStateIdentitySHA256 != cleanedStateIdentity ||
				recovery.RollbackCleanedSourceGenerationSHA256 != sourceGeneration) {
			return registryport.ErrConflict
		}
		state = state.Clone()
		recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending
		recovery.RollbackCleanedSourceStateIdentitySHA256 = cleanedStateIdentity
		recovery.RollbackCleanedSourceGenerationSHA256 = sourceGeneration
		recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
		state.LegacyMigrationRecoveries[recovery.ID] = recovery
		if state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterMigrationRollbackSourceRestoreRecorded); err != nil {
			return err
		}
		state, recovery, err = loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending ||
			recovery.PendingSourceAuthorityIntentSHA256 != authorityIntent ||
			recovery.RollbackCleanedSourceStateIdentitySHA256 != cleanedStateIdentity ||
			recovery.RollbackCleanedSourceGenerationSHA256 != sourceGeneration {
			return registryport.ErrConflict
		}
		if state.Revision == math.MaxUint64 {
			return registryport.ErrConflict
		}
		delete(state.Providers, recovery.ProviderID)
		state.SelectedProviderID = rollbackSelectedProviderID
		state.Revision++
		recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained
		recovery.ProtectedRecoveryRetained = true
		recovery.PendingSourceAuthorityIntentSHA256 = ""
		recovery.RollbackRegistryRevision = state.Revision
		recovery.RollbackRegistryIncarnation = state.Incarnation
		recovery.RollbackSelectedProviderID = state.SelectedProviderID
		state.LegacyMigrationRecoveries[recovery.ID] = recovery
		for migrationID, current := range state.LegacyMigrationRecoveries {
			if migrationID == recovery.ID || current.SourceIdentitySHA256 != recovery.SourceIdentitySHA256 ||
				current.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained {
				continue
			}
			current.RollbackRegistryRevision = state.Revision
			current.RollbackRegistryIncarnation = state.Incarnation
			current.RollbackSelectedProviderID = state.SelectedProviderID
			current.RollbackCleanedSourceStateIdentitySHA256 = recovery.RollbackCleanedSourceStateIdentitySHA256
			current.RollbackCleanedSourceGenerationSHA256 = recovery.RollbackCleanedSourceGenerationSHA256
			state.LegacyMigrationRecoveries[migrationID] = current
		}
		if state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterMigrationRollbackCommittedRecorded); err != nil {
			return err
		}
		result = CommitLegacyMigrationRollbackResult{
			Status: LegacyMigrationRollbackStatusCommittedRecoveryRetained, MigrationID: recovery.ID,
		}
		return nil
	})
	if err != nil {
		return CommitLegacyMigrationRollbackResult{}, normalizeManagerError(err)
	}
	return result, nil
}

func legacyMigrationRollbackOrderAllows(
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
) bool {
	if recovery.SourceIdentitySHA256 == "" {
		return true
	}
	for migrationID, current := range state.LegacyMigrationRecoveries {
		if migrationID == recovery.ID || current.SourceIdentitySHA256 != recovery.SourceIdentitySHA256 {
			continue
		}
		if current.Fence.RegistryRevision == recovery.Fence.RegistryRevision {
			return false
		}
		if current.Fence.RegistryRevision < recovery.Fence.RegistryRevision {
			continue
		}
		switch current.Phase {
		case domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained,
			domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback,
			domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained:
			continue
		case domainregistry.LegacyMigrationRecoveryPhaseFinalized:
			if current.FinalizationOutcome == domainregistry.LegacyMigrationFinalizationOutcomeRolledBack {
				continue
			}
		}
		return false
	}
	return true
}

func (manager *Manager) FinalizeLegacyMigrationRecovery(
	ctx context.Context,
	command FinalizeLegacyMigrationRecoveryCommand,
) (FinalizeLegacyMigrationRecoveryResult, error) {
	if manager == nil || manager.registry == nil || manager.secrets == nil || ctx == nil ||
		!domainregistry.ValidLegacyMigrationID(command.MigrationID) ||
		!validLegacyMigrationSourceLocator([]byte(command.SourceLocator)) ||
		!validLowerSHA256(command.SourceSHA256) ||
		!validLowerSHA256(command.VerifiedSourceSHA256) ||
		!validLowerSHA256(command.SourcePhysicalIdentitySHA256) ||
		!validLegacyMigrationSourceRepresentation(command.VerifiedSourceSHA256, command.VerifiedSource) ||
		(command.RecoveryCredentialRef != "" &&
			secretstoreport.ValidateCredentialRef(command.RecoveryCredentialRef) != nil) ||
		command.Confirmation != LegacyMigrationFinalizeConfirmation {
		return FinalizeLegacyMigrationRecoveryResult{}, registryport.ErrInvalidRequest
	}
	var result FinalizeLegacyMigrationRecoveryResult
	err := manager.registry.WithExclusive(ctx, func(storage registryport.Transaction) error {
		if err := manager.recoverTransactions(ctx, storage); err != nil {
			return err
		}
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, command.MigrationID)
		if err != nil {
			return err
		}
		wasFinalized := recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
			if recovery.FinalizedSourceIdentitySHA256 != legacyMigrationSourceIdentity(
				command.SourceLocator, command.SourceSHA256, command.SourcePhysicalIdentitySHA256,
			) || (recovery.FinalizedSourceStateIdentitySHA256 != "" &&
				recovery.FinalizedSourceStateIdentitySHA256 != legacyMigrationSourceStateIdentity(
					command.SourceLocator, command.SourceSHA256, command.VerifiedSourceSHA256,
					command.SourcePhysicalIdentitySHA256,
				)) {
				return registryport.ErrConflict
			}
			result = finalizedLegacyMigrationResult(
				recovery, LegacyMigrationFinalizationStatusAlreadyFinalized,
			)
			return nil
		} else if recovery.RecoveryCredentialRef != string(command.RecoveryCredentialRef) {
			return registryport.ErrConflict
		}
		if recovery.ProtectedRecoveryRetained &&
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
			return registryport.ErrConflict
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingPreCommit {
			outcome, err := manager.finishLegacyMigrationFinalization(ctx, storage, recovery.ID, "")
			if err != nil {
				return err
			}
			result = FinalizeLegacyMigrationRecoveryResult{
				Status: LegacyMigrationFinalizationStatusCompleted, Outcome: outcome, MigrationID: recovery.ID,
			}
			return nil
		}
		authorityIntent, sourceGeneration, authorityErr := manager.consumeLegacyMigrationSourceAuthority(
			legacyMigrationSourceAuthorityChallenge{
				Operation:   LegacyMigrationSourceAuthorityOperationFinalize,
				MigrationID: command.MigrationID, SourceLocator: command.SourceLocator,
				SourceSHA256: command.SourceSHA256, CurrentSourceSHA256: command.VerifiedSourceSHA256,
				SourcePhysicalIdentitySHA256: command.SourcePhysicalIdentitySHA256,
				RecoveryCredentialRef:        command.RecoveryCredentialRef,
			},
			command.SourceAuthority,
			command.VerifiedSource,
		)
		if authorityErr != nil {
			return authorityErr
		}

		outcome, err := manager.finalizeLegacyMigrationSourceGroup(
			ctx, storage, state, recovery, command.SourceLocator, command.SourceSHA256,
			command.VerifiedSourceSHA256, command.SourcePhysicalIdentitySHA256, command.VerifiedSource,
			authorityIntent, sourceGeneration,
		)
		if err != nil {
			return err
		}
		status := LegacyMigrationFinalizationStatusCompleted
		if wasFinalized {
			status = LegacyMigrationFinalizationStatusAlreadyFinalized
		}
		result = FinalizeLegacyMigrationRecoveryResult{
			Status:  status,
			Outcome: outcome, MigrationID: recovery.ID,
		}
		return nil
	})
	if err != nil {
		return FinalizeLegacyMigrationRecoveryResult{}, normalizeManagerError(err)
	}
	return result, nil
}

func (manager *Manager) finalizeLegacyMigrationSourceGroup(
	ctx context.Context,
	storage registryport.Transaction,
	state domainregistry.Registry,
	anchor domainregistry.LegacyMigrationRecovery,
	sourceLocator string,
	sourceSHA256 string,
	verifiedSourceSHA256 string,
	sourcePhysicalIdentitySHA256 string,
	verifiedSource []byte,
	authorityIntent string,
	sourceGeneration string,
) (LegacyMigrationFinalizationOutcome, error) {
	sourceIdentity := legacyMigrationSourceIdentity(
		sourceLocator, sourceSHA256, sourcePhysicalIdentitySHA256,
	)
	sourceStateIdentity := legacyMigrationSourceStateIdentity(
		sourceLocator, sourceSHA256, verifiedSourceSHA256, sourcePhysicalIdentitySHA256,
	)
	if anchor.SourceIdentitySHA256 != "" && anchor.SourceIdentitySHA256 != sourceIdentity {
		return "", registryport.ErrConflict
	}
	outcome := anchor.FinalizationOutcome
	if outcome == "" {
		switch anchor.Phase {
		case domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained:
			outcome = domainregistry.LegacyMigrationFinalizationOutcomeCommitted
		case domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained:
			outcome = domainregistry.LegacyMigrationFinalizationOutcomeRolledBack
		default:
			return "", registryport.ErrConflict
		}
	}
	if outcome != domainregistry.LegacyMigrationFinalizationOutcomeCommitted &&
		outcome != domainregistry.LegacyMigrationFinalizationOutcomeRolledBack {
		return "", registryport.ErrConflict
	}

	ids := []string{anchor.ID}
	if anchor.SourceIdentitySHA256 != "" {
		ids = ids[:0]
		for migrationID, recovery := range state.LegacyMigrationRecoveries {
			if recovery.SourceIdentitySHA256 == anchor.SourceIdentitySHA256 {
				ids = append(ids, migrationID)
			}
		}
	}
	sort.Strings(ids)
	staged := false
	for _, migrationID := range ids {
		recovery := state.LegacyMigrationRecoveries[migrationID]
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized {
			if recovery.FinalizedSourceIdentitySHA256 != sourceIdentity ||
				recovery.FinalizedSourceStateIdentitySHA256 != sourceStateIdentity ||
				recovery.FinalizationOutcome != outcome ||
				recovery.FinalizedSourceGenerationSHA256 != sourceGeneration {
				return "", registryport.ErrConflict
			}
			continue
		}
		if outcome == domainregistry.LegacyMigrationFinalizationOutcomeCommitted &&
			recovery.ProtectedRecoveryRetained {
			return "", registryport.ErrConflict
		}
		if recovery.RecoveryCredentialRef == "" {
			return "", registryport.ErrVerification
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback {
			wantPhase := domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted
			if outcome == domainregistry.LegacyMigrationFinalizationOutcomeRolledBack {
				wantPhase = domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback
			}
			if recovery.Phase != wantPhase || recovery.FinalizationOutcome != outcome ||
				recovery.FinalizedSourceIdentitySHA256 != sourceIdentity ||
				recovery.FinalizedSourceStateIdentitySHA256 != sourceStateIdentity {
				return "", registryport.ErrConflict
			}
			// A fresh settings-owner authority may replace only the source-side
			// authorization intent. Re-authenticate the still-current K1/Registry
			// winner before persisting that replacement so a stale finalizing marker
			// cannot advance after protected material drift.
			if err := manager.reverifyAuthorizedLegacyMigrationFinalization(
				ctx, state, recovery,
			); err != nil {
				return "", err
			}
			recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
			recovery.FinalizedSourceGenerationSHA256 = sourceGeneration
			state.LegacyMigrationRecoveries[migrationID] = recovery
			staged = true
			continue
		}

		plaintext, payload, err := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
		if err != nil {
			clear(plaintext)
			return "", err
		}
		if !legacyMigrationPayloadSourceMatches(payload, sourceLocator, sourceSHA256) {
			clear(plaintext)
			return "", registryport.ErrConflict
		}
		if !legacyMigrationFinalizationSourceReceiptMatches(
			payload, outcome, sourceLocator, sourceSHA256, verifiedSourceSHA256,
			sourcePhysicalIdentitySHA256, verifiedSource,
		) ||
			recovery.ExpectedCleanedSourceIdentitySHA256 != legacyMigrationSourceStateIdentity(
				sourceLocator, sourceSHA256, string(payload.expectedCleanedSourceSHA256),
				string(payload.sourcePhysicalIdentitySHA256),
			) {
			clear(plaintext)
			return "", registryport.ErrVerification
		}
		if outcome == domainregistry.LegacyMigrationFinalizationOutcomeCommitted {
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
				clear(plaintext)
				return "", registryport.ErrConflict
			}
			_, err = manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload)
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted
		} else {
			if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained {
				clear(plaintext)
				return "", registryport.ErrConflict
			}
			err = manager.verifyRollbackCommittedLegacyMigrationRecovery(ctx, state, recovery, payload)
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback
		}
		clear(plaintext)
		if err != nil {
			return "", err
		}
		recovery.FinalizationOutcome = outcome
		recovery.FinalizedSourceIdentitySHA256 = sourceIdentity
		recovery.FinalizedSourceStateIdentitySHA256 = sourceStateIdentity
		recovery.FinalizedSourceGenerationSHA256 = sourceGeneration
		recovery.PendingSourceAuthorityIntentSHA256 = authorityIntent
		if outcome == domainregistry.LegacyMigrationFinalizationOutcomeRolledBack &&
			(recovery.RollbackCleanedSourceStateIdentitySHA256 != sourceStateIdentity ||
				recovery.RollbackCleanedSourceGenerationSHA256 != sourceGeneration) {
			clear(plaintext)
			return "", registryport.ErrVerification
		}
		state.LegacyMigrationRecoveries[migrationID] = recovery
		staged = true
	}
	if staged {
		if state.Validate() != nil {
			return "", registryport.ErrPersistence
		}
		if err := storage.Commit(ctx, state); err != nil {
			return "", normalizeRegistryStoreError(err)
		}
		if err := manager.observe(FaultAfterMigrationFinalizingRecorded); err != nil {
			return "", err
		}
	}
	for _, migrationID := range ids {
		_, current, err := loadLegacyMigrationRecovery(ctx, storage, migrationID)
		if err != nil {
			return "", err
		}
		if current.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
			current.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
			continue
		}
		if _, err := manager.finishLegacyMigrationFinalization(
			ctx, storage, migrationID, authorityIntent,
		); err != nil {
			return "", err
		}
	}
	if outcome == domainregistry.LegacyMigrationFinalizationOutcomeRolledBack {
		return LegacyMigrationFinalizationOutcomeRolledBack, nil
	}
	return LegacyMigrationFinalizationOutcomeCommitted, nil
}

func legacyMigrationSourceIdentityV3(sourceLocator, sourceSHA256 string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(sourceLocator))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sourceSHA256))
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyMigrationSourceIdentity(
	sourceLocator string,
	sourceSHA256 string,
	sourcePhysicalIdentitySHA256 string,
) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(sourceLocator))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sourceSHA256))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sourcePhysicalIdentitySHA256))
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyMigrationSourceStateIdentityV4(
	sourceLocator string,
	originalSourceSHA256 string,
	currentSourceSHA256 string,
) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(sourceLocator))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(originalSourceSHA256))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(currentSourceSHA256))
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyMigrationSourceStateIdentity(
	sourceLocator string,
	originalSourceSHA256 string,
	currentSourceSHA256 string,
	sourcePhysicalIdentitySHA256 string,
) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(sourceLocator))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(originalSourceSHA256))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(currentSourceSHA256))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(sourcePhysicalIdentitySHA256))
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyMigrationFinalizationSourceStateMatches(
	payload legacyMigrationRecoveryPayloadView,
	outcome string,
	verifiedSourceSHA256 string,
) bool {
	if (payload.version != legacyMigrationRecoveryPayloadVersionV5 &&
		payload.version != legacyMigrationRecoveryPayloadVersionV6) ||
		!validLowerSHA256(verifiedSourceSHA256) {
		return false
	}
	return (outcome == domainregistry.LegacyMigrationFinalizationOutcomeCommitted ||
		outcome == domainregistry.LegacyMigrationFinalizationOutcomeRolledBack) &&
		bytes.Equal(payload.expectedCleanedSourceSHA256, []byte(verifiedSourceSHA256))
}

func legacyMigrationSourceReceiptMatches(
	payload legacyMigrationRecoveryPayloadView,
	sourceLocator string,
	sourceSHA256 string,
	verifiedSourceSHA256 string,
	sourcePhysicalIdentitySHA256 string,
	verifiedSource []byte,
) bool {
	if (payload.version != legacyMigrationRecoveryPayloadVersionV5 &&
		payload.version != legacyMigrationRecoveryPayloadVersionV6) ||
		!legacyMigrationPayloadSourceMatches(payload, sourceLocator, sourceSHA256) ||
		!bytes.Equal(payload.sourcePhysicalIdentitySHA256, []byte(sourcePhysicalIdentitySHA256)) ||
		!validLowerSHA256(sourcePhysicalIdentitySHA256) ||
		!validLowerSHA256(verifiedSourceSHA256) || len(verifiedSource) == 0 ||
		len(verifiedSource) > maxLegacyMigrationSourceSnapshotBytes {
		return false
	}
	return validLegacyMigrationSourceRepresentation(verifiedSourceSHA256, verifiedSource)
}

func validLegacyMigrationSourceRepresentation(sourceSHA256 string, source []byte) bool {
	if !validLowerSHA256(sourceSHA256) || len(source) == 0 ||
		len(source) > maxLegacyMigrationSourceSnapshotBytes {
		return false
	}
	digest := sha256.Sum256(source)
	return bytes.Equal([]byte(hex.EncodeToString(digest[:])), []byte(sourceSHA256))
}

func legacyMigrationFinalizationSourceReceiptMatches(
	payload legacyMigrationRecoveryPayloadView,
	outcome string,
	sourceLocator string,
	sourceSHA256 string,
	verifiedSourceSHA256 string,
	sourcePhysicalIdentitySHA256 string,
	verifiedSource []byte,
) bool {
	if !legacyMigrationSourceReceiptMatches(
		payload, sourceLocator, sourceSHA256, verifiedSourceSHA256,
		sourcePhysicalIdentitySHA256, verifiedSource,
	) || !legacyMigrationFinalizationSourceStateMatches(payload, outcome, verifiedSourceSHA256) {
		return false
	}
	return true
}

func finalizedLegacyMigrationResult(
	recovery domainregistry.LegacyMigrationRecovery,
	status LegacyMigrationFinalizationStatus,
) FinalizeLegacyMigrationRecoveryResult {
	outcome := LegacyMigrationFinalizationOutcomeCommitted
	if recovery.FinalizationOutcome == domainregistry.LegacyMigrationFinalizationOutcomeRolledBack {
		outcome = LegacyMigrationFinalizationOutcomeRolledBack
	}
	return FinalizeLegacyMigrationRecoveryResult{
		Status: status, Outcome: outcome, MigrationID: recovery.ID,
	}
}

func (manager *Manager) authorizeLegacyMigrationFinalizationGroup(
	ctx context.Context,
	storage registryport.Transaction,
	anchor domainregistry.LegacyMigrationRecovery,
) error {
	if anchor.SourceIdentitySHA256 == "" ||
		(anchor.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted &&
			anchor.Phase != domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback) ||
		!validLowerSHA256(anchor.PendingSourceAuthorityIntentSHA256) ||
		!validLowerSHA256(anchor.FinalizedSourceGenerationSHA256) {
		return registryport.ErrVerification
	}
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil || len(state.Transactions) != 0 {
		return registryport.ErrPersistence
	}
	ids := make([]string, 0, len(state.LegacyMigrationRecoveries))
	for id, recovery := range state.LegacyMigrationRecoveries {
		if recovery.SourceIdentitySHA256 == anchor.SourceIdentitySHA256 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return registryport.ErrVerification
	}
	authorizationChanged := false
	for _, id := range ids {
		recovery := state.LegacyMigrationRecoveries[id]
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
			if recovery.FinalizationOutcome != anchor.FinalizationOutcome ||
				recovery.FinalizedSourceIdentitySHA256 != anchor.FinalizedSourceIdentitySHA256 ||
				recovery.FinalizedSourceStateIdentitySHA256 != anchor.FinalizedSourceStateIdentitySHA256 ||
				recovery.FinalizedSourceGenerationSHA256 != anchor.FinalizedSourceGenerationSHA256 {
				return registryport.ErrConflict
			}
			continue
		}
		if recovery.Phase != anchor.Phase || recovery.FinalizationOutcome != anchor.FinalizationOutcome ||
			recovery.FinalizedSourceIdentitySHA256 != anchor.SourceIdentitySHA256 ||
			recovery.FinalizedSourceStateIdentitySHA256 != anchor.FinalizedSourceStateIdentitySHA256 ||
			recovery.FinalizedSourceGenerationSHA256 != anchor.FinalizedSourceGenerationSHA256 ||
			recovery.PendingSourceAuthorityIntentSHA256 != anchor.PendingSourceAuthorityIntentSHA256 {
			return registryport.ErrConflict
		}
		if err := manager.reverifyAuthorizedLegacyMigrationFinalization(
			ctx, state, recovery,
		); err != nil {
			return err
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted &&
			!recovery.FinalizationRecoveryDeleteAuthorized {
			recovery.FinalizationRecoveryDeleteAuthorized = true
			authorizationChanged = true
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback &&
			!recovery.FinalizationWinnerDeleteAuthorized {
			recovery.FinalizationWinnerDeleteAuthorized = true
			authorizationChanged = true
		}
		state.LegacyMigrationRecoveries[id] = recovery
	}
	if authorizationChanged {
		if state.Validate() != nil {
			return registryport.ErrPersistence
		}
		if err := storage.Commit(ctx, state); err != nil {
			return normalizeRegistryStoreError(err)
		}
	}
	return nil
}

func (manager *Manager) reverifyAuthorizedLegacyMigrationFinalization(
	ctx context.Context,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
) error {
	plaintext, err := manager.readLegacyMigrationRecovery(ctx, recovery)
	if err != nil {
		clear(plaintext)
		if recovery.FinalizationRecoveryDeleteAuthorized &&
			(errors.Is(err, secretstoreport.ErrNotFound) || errors.Is(err, secretstoreport.ErrTombstoned)) {
			return nil
		}
		return normalizeSecretError(err)
	}
	defer clear(plaintext)
	payload, err := parseLegacyMigrationRecoveryPayload(plaintext)
	if err != nil || !legacyMigrationRecoveryPayloadMatchesRecord(recovery, payload) {
		return registryport.ErrVerification
	}
	verifiedSourceSHA256 := string(payload.expectedCleanedSourceSHA256)
	if !legacyMigrationFinalizationSourceStateMatches(
		payload, recovery.FinalizationOutcome, verifiedSourceSHA256,
	) || recovery.FinalizedSourceStateIdentitySHA256 != legacyMigrationSourceStateIdentity(
		string(payload.sourceLocator), string(payload.sourceSHA256), verifiedSourceSHA256,
		string(payload.sourcePhysicalIdentitySHA256),
	) {
		return registryport.ErrVerification
	}
	if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted {
		_, err = manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload)
		return err
	}
	if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback {
		return manager.verifyRollbackCommittedLegacyMigrationRecovery(ctx, state, recovery, payload)
	}
	return registryport.ErrConflict
}

func (manager *Manager) preflightLegacyMigrationFinalizationMember(
	ctx context.Context,
	storage registryport.Transaction,
	migrationID string,
) (domainregistry.Registry, domainregistry.LegacyMigrationRecovery, error) {
	state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, migrationID)
	if err != nil {
		return domainregistry.Registry{}, domainregistry.LegacyMigrationRecovery{}, err
	}
	if err := manager.authorizeLegacyMigrationFinalizationGroup(ctx, storage, recovery); err != nil {
		return domainregistry.Registry{}, domainregistry.LegacyMigrationRecovery{}, err
	}
	state, recovery, err = loadLegacyMigrationRecovery(ctx, storage, migrationID)
	if err != nil {
		return domainregistry.Registry{}, domainregistry.LegacyMigrationRecovery{}, err
	}
	if err := manager.reverifyAuthorizedLegacyMigrationFinalization(ctx, state, recovery); err != nil {
		return domainregistry.Registry{}, domainregistry.LegacyMigrationRecovery{}, err
	}
	return state, recovery, nil
}

func (manager *Manager) finishLegacyMigrationFinalization(
	ctx context.Context,
	storage registryport.Transaction,
	migrationID string,
	sourceAuthorityIntent string,
) (LegacyMigrationFinalizationOutcome, error) {
	state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, migrationID)
	if err != nil {
		return "", err
	}
	if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
		recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
		return finalizedLegacyMigrationResult(
			recovery,
			LegacyMigrationFinalizationStatusAlreadyFinalized,
		).Outcome, nil
	}
	preCommit := recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingPreCommit
	committed := recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted
	rolledBack := recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback
	if !preCommit && !committed && !rolledBack {
		return "", registryport.ErrConflict
	}
	if (committed || rolledBack) && (!validLowerSHA256(sourceAuthorityIntent) ||
		recovery.PendingSourceAuthorityIntentSHA256 != sourceAuthorityIntent ||
		!validLowerSHA256(recovery.FinalizedSourceGenerationSHA256)) {
		return "", registryport.ErrVerification
	}
	if committed {
		provider, exists := state.Providers[recovery.ProviderID]
		if !exists || !legacyMigrationProviderWinnerMatchesRecovery(provider, recovery) {
			return "", registryport.ErrVerification
		}
	}
	if rolledBack && (state.Revision != recovery.RollbackRegistryRevision ||
		state.Incarnation != recovery.RollbackRegistryIncarnation ||
		state.SelectedProviderID != recovery.RollbackSelectedProviderID) {
		return "", registryport.ErrVerification
	}
	if committed || rolledBack {
		state, recovery, err = manager.preflightLegacyMigrationFinalizationMember(ctx, storage, migrationID)
		if err != nil {
			return "", err
		}
	}

	if rolledBack {
		if !recovery.FinalizationWinnerDeleteAuthorized {
			return "", registryport.ErrVerification
		}
		if err := manager.observe(FaultAfterMigrationFinalizationWinnerDeleteIntent); err != nil {
			return "", err
		}
		if err := manager.tombstoneLegacyMigrationFinalizationSecret(
			ctx,
			secretstoreport.CredentialRef(recovery.CommittedProviderCredentialRef),
			secretstoreport.Purpose(recovery.CommittedProviderCredentialPurpose),
		); err != nil {
			return "", err
		}
		if err := manager.observe(FaultAfterMigrationFinalizationWinnerTombstoned); err != nil {
			return "", err
		}
		state, recovery, err = manager.preflightLegacyMigrationFinalizationMember(ctx, storage, migrationID)
		if err != nil {
			return "", err
		}
		if err := manager.deleteTombstonedLegacyMigrationFinalizationSecret(
			ctx,
			secretstoreport.CredentialRef(recovery.CommittedProviderCredentialRef),
			secretstoreport.Purpose(recovery.CommittedProviderCredentialPurpose),
			FaultAfterMigrationFinalizationWinnerDeleted,
		); err != nil {
			return "", err
		}
		state, recovery, err = manager.preflightLegacyMigrationFinalizationMember(ctx, storage, migrationID)
		if err != nil {
			return "", err
		}
	}
	if committed {
		if !recovery.FinalizationRecoveryDeleteAuthorized {
			return "", registryport.ErrVerification
		}
		if err := manager.observe(FaultAfterMigrationFinalizationRecoveryDeleteIntent); err != nil {
			return "", err
		}
		if err := manager.tombstoneLegacyMigrationFinalizationSecret(
			ctx,
			secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
			LegacyMigrationRecoveryPurpose,
		); err != nil {
			return "", err
		}
		if err := manager.observe(FaultAfterMigrationFinalizationRecoveryTombstoned); err != nil {
			return "", err
		}
		state, recovery, err = manager.preflightLegacyMigrationFinalizationMember(ctx, storage, migrationID)
		if err != nil {
			return "", err
		}
		if err := manager.deleteTombstonedLegacyMigrationFinalizationSecret(
			ctx,
			secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
			LegacyMigrationRecoveryPurpose,
			FaultAfterMigrationFinalizationRecoveryDeleted,
		); err != nil {
			return "", err
		}
	} else if preCommit {
		if err := manager.deleteLegacyMigrationSecret(
			ctx,
			secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
			LegacyMigrationRecoveryPurpose,
			FaultAfterMigrationFinalizationRecoveryTombstoned,
			FaultAfterMigrationFinalizationRecoveryDeleted,
		); err != nil {
			return "", err
		}
	}

	state, current, err := loadLegacyMigrationRecovery(ctx, storage, migrationID)
	if err != nil {
		return "", err
	}
	if current.Phase != recovery.Phase || current.RecoveryCredentialRef != recovery.RecoveryCredentialRef ||
		current.CommittedProviderCredentialRef != recovery.CommittedProviderCredentialRef ||
		current.RollbackCleanedSourceStateIdentitySHA256 != recovery.RollbackCleanedSourceStateIdentitySHA256 ||
		current.RollbackCleanedSourceGenerationSHA256 != recovery.RollbackCleanedSourceGenerationSHA256 ||
		current.FinalizedSourceIdentitySHA256 != recovery.FinalizedSourceIdentitySHA256 ||
		current.FinalizedSourceStateIdentitySHA256 != recovery.FinalizedSourceStateIdentitySHA256 ||
		current.FinalizedSourceGenerationSHA256 != recovery.FinalizedSourceGenerationSHA256 ||
		current.PendingSourceAuthorityIntentSHA256 != recovery.PendingSourceAuthorityIntentSHA256 ||
		current.FinalizationOutcome != recovery.FinalizationOutcome {
		return "", registryport.ErrConflict
	}
	if rolledBack {
		current.Phase = domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained
		current.ProtectedRecoveryRetained = true
	} else {
		current.Phase = domainregistry.LegacyMigrationRecoveryPhaseFinalized
		current.RecoveryCredentialRef = ""
	}
	current.CommittedProviderCredentialRef = ""
	current.CommittedProviderCredentialPurpose = ""
	current.CommittedProviderRevision = 0
	current.CommittedProviderGeneration = 0
	current.CommittedProviderIncarnation = ""
	if !rolledBack {
		current.RollbackRegistryRevision = 0
		current.RollbackRegistryIncarnation = ""
		current.RollbackSelectedProviderID = ""
	}
	current.FinalizationRecoveryDeleteAuthorized = false
	current.FinalizationWinnerDeleteAuthorized = false
	current.PendingSourceAuthorityIntentSHA256 = ""
	state.LegacyMigrationRecoveries[current.ID] = current
	if state.Validate() != nil {
		return "", registryport.ErrPersistence
	}
	if err := storage.Commit(ctx, state); err != nil {
		return "", normalizeRegistryStoreError(err)
	}
	if err := manager.observe(FaultAfterMigrationFinalizedRecorded); err != nil {
		return "", err
	}
	return finalizedLegacyMigrationResult(current, LegacyMigrationFinalizationStatusCompleted).Outcome, nil
}

func (manager *Manager) deleteLegacyMigrationSecret(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
	afterTombstone FaultPoint,
	afterDelete FaultPoint,
) error {
	err := manager.secrets.Tombstone(ctx, ref, purpose)
	if err != nil && !errors.Is(err, secretstoreport.ErrTombstoned) &&
		!errors.Is(err, secretstoreport.ErrNotFound) {
		return normalizeSecretError(err)
	}
	if err := manager.observe(afterTombstone); err != nil {
		return err
	}
	err = manager.secrets.ExplicitDelete(
		ctx,
		ref,
		purpose,
		secretstoreport.ExplicitlyDeleteCredential(),
	)
	if err != nil && !errors.Is(err, secretstoreport.ErrNotFound) {
		return normalizeSecretError(err)
	}
	if err := manager.observe(afterDelete); err != nil {
		return err
	}
	return nil
}

func (manager *Manager) tombstoneLegacyMigrationFinalizationSecret(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
) error {
	if err := manager.secrets.Tombstone(ctx, ref, purpose); err != nil &&
		!errors.Is(err, secretstoreport.ErrTombstoned) && !errors.Is(err, secretstoreport.ErrNotFound) {
		return normalizeSecretError(err)
	}
	return nil
}

func (manager *Manager) deleteTombstonedLegacyMigrationFinalizationSecret(
	ctx context.Context,
	ref secretstoreport.CredentialRef,
	purpose secretstoreport.Purpose,
	afterDelete FaultPoint,
) error {
	err := manager.secrets.ExplicitDelete(
		ctx,
		ref,
		purpose,
		secretstoreport.ExplicitlyDeleteCredential(),
	)
	if err != nil && !errors.Is(err, secretstoreport.ErrNotFound) {
		return normalizeSecretError(err)
	}
	if err := manager.observe(afterDelete); err != nil {
		return err
	}
	return nil
}

func normalizeLegacyMigrationCandidate(
	candidate LegacyMigrationCandidate,
) (domainregistry.LegacyMigrationRecovery, []byte, error) {
	if !domainregistry.ValidLegacyMigrationID(candidate.MigrationID) ||
		!validLowerSHA256(candidate.SourceSHA256) || candidate.Provider.Validate() != nil ||
		len(candidate.Credential) == 0 || len(candidate.Credential) > maxLegacyMigrationCredentialBytes ||
		(candidate.ExpectedCleanedSourceSHA256 != "" &&
			(!validLowerSHA256(candidate.ExpectedCleanedSourceSHA256) ||
				!validLowerSHA256(candidate.SourcePhysicalIdentitySHA256) || len(candidate.SourceSnapshot) == 0)) ||
		(candidate.SourcePhysicalIdentitySHA256 != "" && candidate.ExpectedCleanedSourceSHA256 == "") ||
		(len(candidate.SourceSnapshot) == 0 && candidate.SourceLocator != "") ||
		(len(candidate.SourceSnapshot) != 0 && len(candidate.ActiveCredentialLocators) == 0) ||
		(len(candidate.SourceSnapshot) != 0 && !validLegacyMigrationSourceLocator([]byte(candidate.SourceLocator))) {
		return domainregistry.LegacyMigrationRecovery{}, nil, registryport.ErrInvalidRequest
	}
	purpose, err := normalizePurpose(candidate.CredentialPurpose, true)
	if err != nil {
		return domainregistry.LegacyMigrationRecovery{}, nil, err
	}
	provider := cloneProviderInput(candidate.Provider)
	var protectedPayload []byte
	if len(candidate.ActiveCredentialLocators) == 0 && len(candidate.RollbackCredentialArtifacts) == 0 {
		protectedPayload, err = marshalLegacyMigrationRecoveryPayload(
			candidate.MigrationID,
			candidate.SourceSHA256,
			provider,
			purpose,
			candidate.Credential,
		)
	} else if len(candidate.SourceSnapshot) == 0 {
		protectedPayload, err = marshalLegacyMigrationRecoveryPayloadV2(
			candidate.MigrationID,
			candidate.SourceSHA256,
			provider,
			purpose,
			candidate.Credential,
			candidate.ActiveCredentialLocators,
			candidate.RollbackCredentialArtifacts,
		)
	} else if candidate.ExpectedCleanedSourceSHA256 == "" {
		protectedPayload, err = marshalLegacyMigrationRecoveryPayloadV3(
			candidate.MigrationID,
			candidate.SourceLocator,
			candidate.SourceSHA256,
			provider,
			purpose,
			candidate.Credential,
			candidate.ActiveCredentialLocators,
			candidate.RollbackCredentialArtifacts,
			candidate.SourceSnapshot,
		)
	} else {
		protectedPayload, err = marshalLegacyMigrationRecoveryPayloadV5(
			candidate.MigrationID,
			candidate.SourceLocator,
			candidate.SourceSHA256,
			candidate.ExpectedCleanedSourceSHA256,
			candidate.SourcePhysicalIdentitySHA256,
			provider,
			purpose,
			candidate.Credential,
			candidate.ActiveCredentialLocators,
			candidate.RollbackCredentialArtifacts,
			candidate.SourceSnapshot,
		)
	}
	if err != nil {
		return domainregistry.LegacyMigrationRecovery{}, nil, registryport.ErrInvalidRequest
	}
	return domainregistry.LegacyMigrationRecovery{
		Version:           domainregistry.LegacyMigrationRecoveryVersion,
		ID:                candidate.MigrationID,
		Phase:             domainregistry.LegacyMigrationRecoveryPhasePrepared,
		ProviderID:        provider.ID,
		Provider:          provider,
		CredentialPurpose: string(purpose),
		RecoveryPurpose:   domainregistry.LegacyMigrationRecoveryPurpose,
		SourceLocator: func() string {
			if candidate.ExpectedCleanedSourceSHA256 == "" {
				return ""
			}
			return candidate.SourceLocator
		}(),
		SourceSHA256: func() string {
			if candidate.ExpectedCleanedSourceSHA256 == "" {
				return ""
			}
			return candidate.SourceSHA256
		}(),
		ExpectedCleanedSourceSHA256: candidate.ExpectedCleanedSourceSHA256,
		SourcePhysicalIdentitySHA256: func() string {
			if candidate.ExpectedCleanedSourceSHA256 == "" {
				return ""
			}
			return candidate.SourcePhysicalIdentitySHA256
		}(),
		SourceIdentitySHA256: func() string {
			if len(candidate.SourceSnapshot) == 0 {
				return ""
			}
			if candidate.ExpectedCleanedSourceSHA256 == "" {
				return legacyMigrationSourceIdentityV3(candidate.SourceLocator, candidate.SourceSHA256)
			}
			return legacyMigrationSourceIdentity(
				candidate.SourceLocator, candidate.SourceSHA256, candidate.SourcePhysicalIdentitySHA256,
			)
		}(),
		ExpectedCleanedSourceIdentitySHA256: func() string {
			if candidate.ExpectedCleanedSourceSHA256 == "" {
				return ""
			}
			return legacyMigrationSourceStateIdentity(
				candidate.SourceLocator, candidate.SourceSHA256, candidate.ExpectedCleanedSourceSHA256,
				candidate.SourcePhysicalIdentitySHA256,
			)
		}(),
	}, protectedPayload, nil
}

func (manager *Manager) prepareLegacyMigrationRecoverySecret(
	ctx context.Context,
	storage registryport.Transaction,
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
	protectedPayload []byte,
) error {
	prepared, err := manager.secrets.PreparePut(ctx, LegacyMigrationRecoveryPurpose, protectedPayload)
	if err != nil {
		return normalizeSecretError(err)
	}
	defer prepared.Abort()
	ref := prepared.CredentialRef()
	if secretstoreport.ValidateCredentialRef(ref) != nil ||
		!registryCredentialRefAvailable(state, string(ref), recovery.ID) {
		return registryport.ErrConflict
	}

	recovery.RecoveryCredentialRef = string(ref)
	recovery.RecoveryPurpose = domainregistry.LegacyMigrationRecoveryPurpose
	recovery.Phase = domainregistry.LegacyMigrationRecoveryPhasePrepared
	if _, exists := state.LegacyMigrationRecoveries[recovery.ID]; !exists {
		recovery.Fence = legacyMigrationRecoveryFence(state, recovery.ProviderID)
	}
	state = state.Clone()
	if state.LegacyMigrationRecoveries == nil {
		state.LegacyMigrationRecoveries = make(map[string]domainregistry.LegacyMigrationRecovery)
	}
	state.LegacyMigrationRecoveries[recovery.ID] = recovery.Clone()
	if state.Validate() != nil {
		return registryport.ErrConflict
	}
	if err := storage.Commit(ctx, state); err != nil {
		return normalizeRegistryStoreError(err)
	}
	if err := manager.observe(FaultAfterMigrationRecoveryPrepared); err != nil {
		return err
	}
	if err := prepared.Commit(ctx); err != nil {
		return normalizeSecretError(err)
	}
	if err := manager.observe(FaultAfterMigrationRecoverySecretDurable); err != nil {
		return err
	}
	state, current, err := loadLegacyMigrationRecovery(ctx, storage, recovery.ID)
	if err != nil {
		return err
	}
	if current.Phase != domainregistry.LegacyMigrationRecoveryPhasePrepared ||
		current.RecoveryCredentialRef != string(ref) {
		return registryport.ErrConflict
	}
	current.Phase = domainregistry.LegacyMigrationRecoveryPhaseSecretDurable
	state.LegacyMigrationRecoveries[current.ID] = current
	if err := storage.Commit(ctx, state); err != nil {
		return normalizeRegistryStoreError(err)
	}
	if err := manager.observe(FaultAfterMigrationRecoverySecretDurableRecorded); err != nil {
		return err
	}
	return nil
}

func (manager *Manager) advanceLegacyMigrationRecovery(
	ctx context.Context,
	storage registryport.Transaction,
	migrationID string,
	expectedProtectedPayload []byte,
	requireCurrentFence bool,
) (LegacyMigrationRecoveryResult, error) {
	for step := 0; step < 4; step++ {
		state, recovery, err := loadLegacyMigrationRecovery(ctx, storage, migrationID)
		if err != nil {
			return LegacyMigrationRecoveryResult{}, err
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseAbandoning {
			return LegacyMigrationRecoveryResult{}, registryport.ErrConflict
		}
		if requireCurrentFence && !legacyMigrationRecoveryFenceMatches(state, recovery) {
			return LegacyMigrationRecoveryResult{}, registryport.ErrConflict
		}
		switch recovery.Phase {
		case domainregistry.LegacyMigrationRecoveryPhasePrepared:
			plaintext, readErr := manager.readLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return LegacyMigrationRecoveryResult{}, normalizeSecretError(readErr)
			}
			if err := validateLegacyMigrationRecoveryPayload(recovery, plaintext, expectedProtectedPayload); err != nil {
				clear(plaintext)
				return LegacyMigrationRecoveryResult{}, err
			}
			clear(plaintext)
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseSecretDurable
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
			if err := storage.Commit(ctx, state); err != nil {
				return LegacyMigrationRecoveryResult{}, normalizeRegistryStoreError(err)
			}
			if err := manager.observe(FaultAfterMigrationRecoverySecretDurableRecorded); err != nil {
				return LegacyMigrationRecoveryResult{}, err
			}
		case domainregistry.LegacyMigrationRecoveryPhaseSecretDurable:
			plaintext, readErr := manager.readLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return LegacyMigrationRecoveryResult{}, normalizeSecretError(readErr)
			}
			if err := validateLegacyMigrationRecoveryPayload(recovery, plaintext, expectedProtectedPayload); err != nil {
				clear(plaintext)
				return LegacyMigrationRecoveryResult{}, err
			}
			clear(plaintext)
			if err := manager.observe(FaultAfterMigrationRecoveryReadbackVerified); err != nil {
				return LegacyMigrationRecoveryResult{}, err
			}
			recovery.Phase = domainregistry.LegacyMigrationRecoveryPhaseVerified
			state.LegacyMigrationRecoveries[recovery.ID] = recovery
			if err := storage.Commit(ctx, state); err != nil {
				return LegacyMigrationRecoveryResult{}, normalizeRegistryStoreError(err)
			}
			if err := manager.observe(FaultAfterMigrationRecoveryVerifiedRecorded); err != nil {
				return LegacyMigrationRecoveryResult{}, err
			}
			return verifiedLegacyMigrationRecoveryResult(recovery), nil
		case domainregistry.LegacyMigrationRecoveryPhaseVerified:
			plaintext, readErr := manager.readLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return LegacyMigrationRecoveryResult{}, normalizeSecretError(readErr)
			}
			validationErr := validateLegacyMigrationRecoveryPayload(recovery, plaintext, expectedProtectedPayload)
			clear(plaintext)
			if validationErr != nil {
				return LegacyMigrationRecoveryResult{}, validationErr
			}
			return verifiedLegacyMigrationRecoveryResult(recovery), nil
		case domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared:
			plaintext, readErr := manager.readLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return LegacyMigrationRecoveryResult{}, normalizeSecretError(readErr)
			}
			validationErr := validateLegacyMigrationRecoveryPayload(recovery, plaintext, expectedProtectedPayload)
			clear(plaintext)
			if validationErr != nil {
				return LegacyMigrationRecoveryResult{}, validationErr
			}
			return verifiedLegacyMigrationRecoveryResult(recovery), nil
		default:
			return LegacyMigrationRecoveryResult{}, registryport.ErrPersistence
		}
	}
	return LegacyMigrationRecoveryResult{}, registryport.ErrPersistence
}

func (manager *Manager) recoverLegacyMigrationRecoveries(
	ctx context.Context,
	storage registryport.Transaction,
) error {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return registryport.ErrPersistence
	}
	ids := make([]string, 0, len(state.LegacyMigrationRecoveries))
	for id := range state.LegacyMigrationRecoveries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		state, recovery, loadErr := loadLegacyMigrationRecovery(ctx, storage, id)
		if loadErr != nil {
			return loadErr
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseAbandoning {
			if err := manager.finishLegacyMigrationAbandon(ctx, storage, id); err != nil {
				return err
			}
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalized ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeleted {
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProtectedRecoveryDeletePending {
			// The durable delete intent cannot replay a destructive K1 effect
			// without a fresh settings-owner source authority.
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseVerified && recovery.RemigrationPending {
			plaintext, _, readErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
			clear(plaintext)
			if readErr != nil || !legacyMigrationRecoveryFenceMatches(state, recovery) {
				if readErr != nil {
					return readErr
				}
				return registryport.ErrVerification
			}
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRecoveryRetained {
			plaintext, payload, readErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return readErr
			}
			verifyErr := manager.verifyRollbackRetainedLegacyMigrationRecovery(ctx, state, recovery, payload)
			clear(plaintext)
			if verifyErr != nil {
				return verifyErr
			}
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingPreCommit {
			if _, err := manager.finishLegacyMigrationFinalization(ctx, storage, id, ""); err != nil {
				return err
			}
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingCommitted ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseFinalizingRollback {
			// A restarted Go manager must not replay source-destructive work from
			// an old in-memory receipt. Fresh main/settings authority resumes it.
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackCleanedSourcePending ||
			recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackRegistryCommitPending {
			plaintext, payload, readErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return readErr
			}
			_, verifyErr := manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload)
			clear(plaintext)
			if verifyErr != nil {
				return verifyErr
			}
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseRollbackCommittedRecoveryRetained {
			plaintext, payload, readErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return readErr
			}
			verifyErr := manager.verifyRollbackCommittedLegacyMigrationRecovery(ctx, state, recovery, payload)
			clear(plaintext)
			if verifyErr != nil {
				return verifyErr
			}
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommittedRecoveryRetained {
			plaintext, payload, readErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return readErr
			}
			_, verifyErr := manager.verifyCommittedRetainedLegacyMigrationRecovery(ctx, state, recovery, payload)
			clear(plaintext)
			if verifyErr != nil {
				return verifyErr
			}
			continue
		}
		if recovery.Phase == domainregistry.LegacyMigrationRecoveryPhaseProviderCommitPrepared {
			if recovery.RemigrationPending {
				// A fresh settings-owner source authority is required before the
				// remigration transaction may resume.
				continue
			}
			plaintext, payload, readErr := manager.readAndValidateLegacyMigrationRecovery(ctx, recovery)
			if readErr != nil {
				clear(plaintext)
				return readErr
			}
			if _, exists := state.Providers[recovery.ProviderID]; !exists {
				clear(plaintext)
				if !legacyMigrationProviderCommitPristine(state, recovery) {
					return registryport.ErrVerification
				}
				continue
			}
			_, promoteErr := manager.promoteLegacyMigrationProviderWinner(ctx, storage, state, recovery, payload)
			clear(plaintext)
			if promoteErr != nil {
				return promoteErr
			}
			continue
		}
		requireFence := recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseVerified
		if _, err := manager.advanceLegacyMigrationRecovery(ctx, storage, id, nil, requireFence); err != nil {
			return err
		}
	}
	return nil
}

func (manager *Manager) finishLegacyMigrationAbandon(
	ctx context.Context,
	storage registryport.Transaction,
	migrationID string,
) error {
	_, recovery, err := loadLegacyMigrationRecovery(ctx, storage, migrationID)
	if err != nil {
		return err
	}
	if recovery.Phase != domainregistry.LegacyMigrationRecoveryPhaseAbandoning {
		return registryport.ErrConflict
	}
	ref := secretstoreport.CredentialRef(recovery.RecoveryCredentialRef)
	err = manager.secrets.Tombstone(ctx, ref, LegacyMigrationRecoveryPurpose)
	if err != nil && !errors.Is(err, secretstoreport.ErrTombstoned) && !errors.Is(err, secretstoreport.ErrNotFound) {
		return normalizeSecretError(err)
	}
	err = manager.secrets.ExplicitDelete(
		ctx,
		ref,
		LegacyMigrationRecoveryPurpose,
		secretstoreport.ExplicitlyDeleteCredential(),
	)
	if err != nil && !errors.Is(err, secretstoreport.ErrNotFound) {
		return normalizeSecretError(err)
	}
	state, current, err := loadLegacyMigrationRecovery(ctx, storage, migrationID)
	if err != nil {
		return err
	}
	if current.Phase != domainregistry.LegacyMigrationRecoveryPhaseAbandoning ||
		current.RecoveryCredentialRef != recovery.RecoveryCredentialRef {
		return registryport.ErrConflict
	}
	delete(state.LegacyMigrationRecoveries, migrationID)
	if state.Validate() != nil {
		return registryport.ErrPersistence
	}
	return normalizeRegistryStoreError(storage.Commit(ctx, state))
}

func (manager *Manager) readLegacyMigrationRecovery(
	ctx context.Context,
	recovery domainregistry.LegacyMigrationRecovery,
) ([]byte, error) {
	return manager.secrets.GetForAuthorizedConsumer(ctx, secretstoreport.AccessRequest{
		CredentialRef: secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
		Purpose:       LegacyMigrationRecoveryPurpose,
		Consumer:      RegistryReadbackConsumer,
	})
}

func loadLegacyMigrationRecovery(
	ctx context.Context,
	storage registryport.Transaction,
	migrationID string,
) (domainregistry.Registry, domainregistry.LegacyMigrationRecovery, error) {
	state, err := storage.Load(ctx)
	if err != nil || state.Validate() != nil {
		return domainregistry.Registry{}, domainregistry.LegacyMigrationRecovery{}, registryport.ErrPersistence
	}
	recovery, ok := state.LegacyMigrationRecoveries[migrationID]
	if !ok {
		return domainregistry.Registry{}, domainregistry.LegacyMigrationRecovery{}, registryport.ErrNotFound
	}
	return state, recovery, nil
}

func checkLegacyMigrationExpected(
	state domainregistry.Registry,
	expected domainregistry.ExpectedState,
	providerID string,
) error {
	if !domainregistry.ValidIncarnation(expected.RegistryIncarnation) ||
		state.Revision != expected.RegistryRevision || state.Incarnation != expected.RegistryIncarnation {
		return registryport.ErrConflict
	}
	provider, exists := state.Providers[providerID]
	if !exists {
		if expected.ProviderRevision != 0 || expected.ProviderGeneration != 0 ||
			expected.ProviderIncarnation != "" || expected.ProviderCredentialPurpose != "" {
			return registryport.ErrConflict
		}
		return nil
	}
	if provider.Revision != expected.ProviderRevision || provider.Generation != expected.ProviderGeneration ||
		provider.Incarnation != expected.ProviderIncarnation ||
		provider.CredentialPurpose != expected.ProviderCredentialPurpose {
		return registryport.ErrConflict
	}
	return nil
}

func legacyMigrationRecoveryFence(
	state domainregistry.Registry,
	providerID string,
) domainregistry.LegacyMigrationRecoveryFence {
	fence := domainregistry.LegacyMigrationRecoveryFence{
		RegistryRevision: state.Revision, RegistryIncarnation: state.Incarnation,
		SelectedProviderID: state.SelectedProviderID,
	}
	if provider, exists := state.Providers[providerID]; exists {
		fence.ProviderExists = true
		fence.ProviderRevision = provider.Revision
		fence.ProviderGeneration = provider.Generation
		fence.ProviderIncarnation = provider.Incarnation
		fence.ProviderCredentialRef = provider.CredentialRef
		fence.ProviderCredentialPurpose = provider.CredentialPurpose
	}
	return fence
}

func legacyMigrationRecoveryFenceMatches(
	state domainregistry.Registry,
	recovery domainregistry.LegacyMigrationRecovery,
) bool {
	fence := recovery.Fence
	if state.Revision != fence.RegistryRevision || state.Incarnation != fence.RegistryIncarnation ||
		state.SelectedProviderID != fence.SelectedProviderID {
		return false
	}
	provider, exists := state.Providers[recovery.ProviderID]
	if !fence.ProviderExists {
		return !exists
	}
	return exists && provider.Revision == fence.ProviderRevision &&
		provider.Generation == fence.ProviderGeneration && provider.Incarnation == fence.ProviderIncarnation &&
		provider.CredentialRef == fence.ProviderCredentialRef &&
		provider.CredentialPurpose == fence.ProviderCredentialPurpose
}

func legacyMigrationCandidateMatches(
	existing domainregistry.LegacyMigrationRecovery,
	candidate domainregistry.LegacyMigrationRecovery,
) bool {
	return existing.Version == candidate.Version && existing.ID == candidate.ID &&
		existing.ProviderID == candidate.ProviderID && reflect.DeepEqual(existing.Provider, candidate.Provider) &&
		existing.CredentialPurpose == candidate.CredentialPurpose &&
		existing.RecoveryPurpose == candidate.RecoveryPurpose &&
		existing.SourceIdentitySHA256 == candidate.SourceIdentitySHA256 &&
		existing.ExpectedCleanedSourceIdentitySHA256 == candidate.ExpectedCleanedSourceIdentitySHA256
}

func validateLegacyMigrationRecoveryPayload(
	recovery domainregistry.LegacyMigrationRecovery,
	plaintext []byte,
	expectedProtectedPayload []byte,
) error {
	payload, err := parseLegacyMigrationRecoveryPayload(plaintext)
	if err != nil || !legacyMigrationRecoveryPayloadMatchesRecord(recovery, payload) {
		return registryport.ErrVerification
	}
	if expectedProtectedPayload != nil &&
		!legacyMigrationProtectedPayloadMatchesCandidate(plaintext, expectedProtectedPayload) {
		return registryport.ErrConflict
	}
	return nil
}

func legacyMigrationRecoveryPayloadMatchesRecord(
	recovery domainregistry.LegacyMigrationRecovery,
	payload legacyMigrationRecoveryPayloadView,
) bool {
	if !bytes.Equal(payload.migrationID, []byte(recovery.ID)) ||
		!reflect.DeepEqual(payload.provider, recovery.Provider) ||
		!bytes.Equal(payload.credentialPurpose, []byte(recovery.CredentialPurpose)) {
		return false
	}
	switch payload.version {
	case legacyMigrationRecoveryPayloadVersion, legacyMigrationRecoveryPayloadVersionV2:
		return recovery.SourceIdentitySHA256 == "" && len(payload.sourceLocator) == 0
	case legacyMigrationRecoveryPayloadVersionV3:
		return validLegacyMigrationSourceLocator(payload.sourceLocator) &&
			recovery.ExpectedCleanedSourceIdentitySHA256 == "" &&
			recovery.SourceIdentitySHA256 == legacyMigrationSourceIdentityV3(
				string(payload.sourceLocator), string(payload.sourceSHA256),
			) && legacyMigrationPayloadLocatorsBelongToSource(payload)
	case legacyMigrationRecoveryPayloadVersionV4:
		return validLegacyMigrationSourceLocator(payload.sourceLocator) &&
			recovery.SourceIdentitySHA256 == legacyMigrationSourceIdentityV3(
				string(payload.sourceLocator), string(payload.sourceSHA256),
			) && recovery.ExpectedCleanedSourceIdentitySHA256 == legacyMigrationSourceStateIdentityV4(
			string(payload.sourceLocator), string(payload.sourceSHA256),
			string(payload.expectedCleanedSourceSHA256),
		) && legacyMigrationPayloadLocatorsBelongToSource(payload)
	case legacyMigrationRecoveryPayloadVersionV5, legacyMigrationRecoveryPayloadVersionV6:
		priorMatches := recovery.PriorSelectedProviderIdentitySHA256 == "" &&
			payload.priorSelectedProvider == nil
		if payload.version == legacyMigrationRecoveryPayloadVersionV6 {
			priorMatches = recovery.PriorSelectedProviderIdentitySHA256 ==
				mustLegacyMigrationPriorSelectedProviderIdentity(payload.priorSelectedProvider)
		}
		return validLegacyMigrationSourceLocator(payload.sourceLocator) && priorMatches &&
			recovery.SourceLocator == string(payload.sourceLocator) &&
			recovery.SourceSHA256 == string(payload.sourceSHA256) &&
			recovery.ExpectedCleanedSourceSHA256 == string(payload.expectedCleanedSourceSHA256) &&
			recovery.SourcePhysicalIdentitySHA256 == string(payload.sourcePhysicalIdentitySHA256) &&
			recovery.SourceIdentitySHA256 == legacyMigrationSourceIdentity(
				string(payload.sourceLocator), string(payload.sourceSHA256),
				string(payload.sourcePhysicalIdentitySHA256),
			) && recovery.ExpectedCleanedSourceIdentitySHA256 == legacyMigrationSourceStateIdentity(
			string(payload.sourceLocator), string(payload.sourceSHA256),
			string(payload.expectedCleanedSourceSHA256), string(payload.sourcePhysicalIdentitySHA256),
		) && legacyMigrationPayloadLocatorsBelongToSource(payload)
	default:
		return false
	}
}

func legacyMigrationPayloadSourceMatches(
	payload legacyMigrationRecoveryPayloadView,
	sourceLocator string,
	sourceSHA256 string,
) bool {
	if !bytes.Equal(payload.sourceSHA256, []byte(sourceSHA256)) {
		return false
	}
	if payload.version < legacyMigrationRecoveryPayloadVersionV3 {
		return sourceLocator == ""
	}
	return bytes.Equal(payload.sourceLocator, []byte(sourceLocator)) &&
		legacyMigrationPayloadLocatorsBelongToSource(payload)
}

func marshalLegacyMigrationRecoveryPayload(
	migrationID string,
	sourceSHA256 string,
	provider domainregistry.ProviderInput,
	credentialPurpose secretstoreport.Purpose,
	credential []byte,
) ([]byte, error) {
	return marshalLegacyMigrationRecoveryPayloadBase(
		legacyMigrationRecoveryPayloadVersion,
		migrationID,
		sourceSHA256,
		provider,
		credentialPurpose,
		credential,
	)
}

func marshalLegacyMigrationRecoveryPayloadV2(
	migrationID string,
	sourceSHA256 string,
	provider domainregistry.ProviderInput,
	credentialPurpose secretstoreport.Purpose,
	credential []byte,
	activeCredentialLocators []string,
	rollbackCredentialArtifacts []LegacyMigrationRollbackCredentialArtifact,
) ([]byte, error) {
	return marshalLegacyMigrationRecoveryPayloadWithArtifacts(
		legacyMigrationRecoveryPayloadVersionV2,
		migrationID,
		sourceSHA256,
		provider,
		credentialPurpose,
		credential,
		activeCredentialLocators,
		rollbackCredentialArtifacts,
		nil,
		nil,
	)
}

func marshalLegacyMigrationRecoveryPayloadV3(
	migrationID string,
	sourceLocator string,
	sourceSHA256 string,
	provider domainregistry.ProviderInput,
	credentialPurpose secretstoreport.Purpose,
	credential []byte,
	activeCredentialLocators []string,
	rollbackCredentialArtifacts []LegacyMigrationRollbackCredentialArtifact,
	sourceSnapshot []byte,
) ([]byte, error) {
	if !validLegacyMigrationSourceLocator([]byte(sourceLocator)) || len(sourceSnapshot) == 0 ||
		len(sourceSnapshot) > maxLegacyMigrationSourceSnapshotBytes {
		return nil, registryport.ErrInvalidRequest
	}
	digest := sha256.Sum256(sourceSnapshot)
	if !bytes.Equal([]byte(hex.EncodeToString(digest[:])), []byte(sourceSHA256)) {
		return nil, registryport.ErrInvalidRequest
	}
	return marshalLegacyMigrationRecoveryPayloadWithArtifacts(
		legacyMigrationRecoveryPayloadVersionV3,
		migrationID,
		sourceSHA256,
		provider,
		credentialPurpose,
		credential,
		activeCredentialLocators,
		rollbackCredentialArtifacts,
		[]byte(sourceLocator),
		sourceSnapshot,
	)
}

func marshalLegacyMigrationRecoveryPayloadV4(
	migrationID string,
	sourceLocator string,
	sourceSHA256 string,
	expectedCleanedSourceSHA256 string,
	provider domainregistry.ProviderInput,
	credentialPurpose secretstoreport.Purpose,
	credential []byte,
	activeCredentialLocators []string,
	rollbackCredentialArtifacts []LegacyMigrationRollbackCredentialArtifact,
	sourceSnapshot []byte,
) ([]byte, error) {
	if !validLowerSHA256(expectedCleanedSourceSHA256) {
		return nil, registryport.ErrInvalidRequest
	}
	payload, err := marshalLegacyMigrationRecoveryPayloadV3(
		migrationID, sourceLocator, sourceSHA256, provider, credentialPurpose, credential,
		activeCredentialLocators, rollbackCredentialArtifacts, sourceSnapshot,
	)
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(
		payload[len(legacyMigrationRecoveryPayloadMagic):len(legacyMigrationRecoveryPayloadMagic)+4],
		legacyMigrationRecoveryPayloadVersionV4,
	)
	payload, err = appendLegacyMigrationPayloadField(payload, []byte(expectedCleanedSourceSHA256))
	if err != nil {
		clear(payload)
		return nil, err
	}
	return payload, nil
}

func marshalLegacyMigrationRecoveryPayloadV5(
	migrationID string,
	sourceLocator string,
	sourceSHA256 string,
	expectedCleanedSourceSHA256 string,
	sourcePhysicalIdentitySHA256 string,
	provider domainregistry.ProviderInput,
	credentialPurpose secretstoreport.Purpose,
	credential []byte,
	activeCredentialLocators []string,
	rollbackCredentialArtifacts []LegacyMigrationRollbackCredentialArtifact,
	sourceSnapshot []byte,
) ([]byte, error) {
	if !validLowerSHA256(sourcePhysicalIdentitySHA256) {
		return nil, registryport.ErrInvalidRequest
	}
	payload, err := marshalLegacyMigrationRecoveryPayloadV4(
		migrationID, sourceLocator, sourceSHA256, expectedCleanedSourceSHA256,
		provider, credentialPurpose, credential, activeCredentialLocators,
		rollbackCredentialArtifacts, sourceSnapshot,
	)
	if err != nil {
		return nil, err
	}
	binary.BigEndian.PutUint32(
		payload[len(legacyMigrationRecoveryPayloadMagic):len(legacyMigrationRecoveryPayloadMagic)+4],
		legacyMigrationRecoveryPayloadVersionV5,
	)
	payload, err = appendLegacyMigrationPayloadField(payload, []byte(sourcePhysicalIdentitySHA256))
	if err != nil {
		clear(payload)
		return nil, err
	}
	return payload, nil
}

func marshalLegacyMigrationRecoveryPayloadWithArtifacts(
	version uint32,
	migrationID string,
	sourceSHA256 string,
	provider domainregistry.ProviderInput,
	credentialPurpose secretstoreport.Purpose,
	credential []byte,
	activeCredentialLocators []string,
	rollbackCredentialArtifacts []LegacyMigrationRollbackCredentialArtifact,
	sourceLocator []byte,
	sourceSnapshot []byte,
) ([]byte, error) {
	if err := validateLegacyMigrationRecoveryV2Input(
		credential,
		activeCredentialLocators,
		rollbackCredentialArtifacts,
		sourceLocator,
	); err != nil {
		return nil, err
	}
	payload, err := marshalLegacyMigrationRecoveryPayloadBase(
		version,
		migrationID,
		sourceSHA256,
		provider,
		credentialPurpose,
		credential,
	)
	if err != nil {
		return nil, err
	}
	payload, err = appendLegacyMigrationPayloadUint32(payload, len(activeCredentialLocators))
	if err != nil {
		clear(payload)
		return nil, err
	}
	for _, locator := range activeCredentialLocators {
		payload, err = appendLegacyMigrationPayloadField(payload, []byte(locator))
		if err != nil {
			clear(payload)
			return nil, err
		}
	}
	payload, err = appendLegacyMigrationPayloadUint32(payload, len(rollbackCredentialArtifacts))
	if err != nil {
		clear(payload)
		return nil, err
	}
	for _, artifact := range rollbackCredentialArtifacts {
		payload, err = appendLegacyMigrationPayloadUint32(payload, len(artifact.Locators))
		if err != nil {
			clear(payload)
			return nil, err
		}
		for _, locator := range artifact.Locators {
			payload, err = appendLegacyMigrationPayloadField(payload, []byte(locator))
			if err != nil {
				clear(payload)
				return nil, err
			}
		}
		payload, err = appendLegacyMigrationPayloadField(payload, artifact.Credential)
		if err != nil {
			clear(payload)
			return nil, err
		}
	}
	if version == legacyMigrationRecoveryPayloadVersionV3 ||
		version == legacyMigrationRecoveryPayloadVersionV4 ||
		version == legacyMigrationRecoveryPayloadVersionV5 {
		payload, err = appendLegacyMigrationPayloadField(payload, sourceLocator)
		if err != nil {
			clear(payload)
			return nil, err
		}
		payload, err = appendLegacyMigrationPayloadField(payload, sourceSnapshot)
		if err != nil {
			clear(payload)
			return nil, err
		}
	}
	return payload, nil
}

func marshalLegacyMigrationRecoveryPayloadBase(
	version uint32,
	migrationID string,
	sourceSHA256 string,
	provider domainregistry.ProviderInput,
	credentialPurpose secretstoreport.Purpose,
	credential []byte,
) ([]byte, error) {
	providerBytes, err := json.Marshal(provider)
	if err != nil {
		return nil, err
	}
	defer clear(providerBytes)
	fields := [][]byte{
		[]byte(migrationID),
		[]byte(sourceSHA256),
		providerBytes,
		[]byte(credentialPurpose),
		credential,
	}
	payload := make([]byte, 0, len(legacyMigrationRecoveryPayloadMagic)+4)
	payload = append(payload, legacyMigrationRecoveryPayloadMagic...)
	payload = binary.BigEndian.AppendUint32(payload, version)
	for _, field := range fields {
		payload, err = appendLegacyMigrationPayloadField(payload, field)
		if err != nil {
			clear(payload)
			return nil, registryport.ErrInvalidRequest
		}
	}
	return payload, nil
}

func appendLegacyMigrationPayloadUint32(payload []byte, value int) ([]byte, error) {
	if value < 0 || uint64(value) > uint64(math.MaxUint32) ||
		len(payload) > maxLegacyMigrationRecoveryPayloadBytes-4 {
		return payload, registryport.ErrInvalidRequest
	}
	return binary.BigEndian.AppendUint32(payload, uint32(value)), nil
}

func appendLegacyMigrationPayloadField(payload []byte, field []byte) ([]byte, error) {
	if len(field) > maxLegacyMigrationRecoveryPayloadBytes ||
		len(payload) > maxLegacyMigrationRecoveryPayloadBytes-4-len(field) {
		return payload, registryport.ErrInvalidRequest
	}
	payload = binary.BigEndian.AppendUint32(payload, uint32(len(field)))
	payload = append(payload, field...)
	return payload, nil
}

func validateLegacyMigrationRecoveryV2Input(
	activeCredential []byte,
	activeCredentialLocators []string,
	rollbackCredentialArtifacts []LegacyMigrationRollbackCredentialArtifact,
	sourceLocator []byte,
) error {
	if len(activeCredentialLocators) == 0 ||
		len(activeCredentialLocators) > maxLegacyMigrationLocatorCount ||
		len(rollbackCredentialArtifacts) > maxLegacyMigrationRollbackArtifactCount {
		return registryport.ErrInvalidRequest
	}
	locatorCount := 0
	locatorBytes := 0
	locators := make(map[string]struct{}, len(activeCredentialLocators))
	validateLocator := func(locator string) error {
		locatorCount++
		if locatorCount > maxLegacyMigrationLocatorCount ||
			len(locator) > maxLegacyMigrationLocatorTotalBytes-locatorBytes ||
			!validLegacyMigrationLogicalLocator([]byte(locator)) ||
			(len(sourceLocator) != 0 && !bytes.HasPrefix([]byte(locator), append(bytes.Clone(sourceLocator), ':'))) {
			return registryport.ErrInvalidRequest
		}
		if _, duplicate := locators[locator]; duplicate {
			return registryport.ErrInvalidRequest
		}
		locators[locator] = struct{}{}
		locatorBytes += len(locator)
		return nil
	}
	for _, locator := range activeCredentialLocators {
		if err := validateLocator(locator); err != nil {
			return err
		}
	}
	credentialBytes := len(activeCredential)
	for index, artifact := range rollbackCredentialArtifacts {
		if len(artifact.Locators) == 0 || len(artifact.Credential) == 0 ||
			len(artifact.Credential) > maxLegacyMigrationCredentialBytes ||
			credentialBytes > maxLegacyMigrationRecoveryCredentialTotalBytes-len(artifact.Credential) ||
			bytes.Equal(activeCredential, artifact.Credential) {
			return registryport.ErrInvalidRequest
		}
		for prior := 0; prior < index; prior++ {
			if bytes.Equal(rollbackCredentialArtifacts[prior].Credential, artifact.Credential) {
				return registryport.ErrInvalidRequest
			}
		}
		for _, locator := range artifact.Locators {
			if err := validateLocator(locator); err != nil {
				return err
			}
		}
		credentialBytes += len(artifact.Credential)
	}
	return nil
}

func validLegacyMigrationLogicalLocator(locator []byte) bool {
	if len(locator) == 0 || len(locator) > maxLegacyMigrationLocatorBytes {
		return false
	}
	currentPrefix := []byte("current:analytix-settings.json:")
	if bytes.HasPrefix(locator, currentPrefix) {
		return validLegacyMigrationCredentialLocatorSuffix(locator[len(currentPrefix):])
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
			return validLegacyMigrationCredentialLocatorSuffix(remainder[len(filename):])
		}
	}
	return false
}

func validLegacyMigrationSourceLocator(locator []byte) bool {
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

func legacyMigrationLocatorBelongsToSource(locator, sourceLocator []byte) bool {
	return validLegacyMigrationSourceLocator(sourceLocator) && len(locator) > len(sourceLocator) &&
		bytes.Equal(locator[:len(sourceLocator)], sourceLocator) && locator[len(sourceLocator)] == ':' &&
		validLegacyMigrationLogicalLocator(locator)
}

func legacyMigrationPayloadLocatorsBelongToSource(payload legacyMigrationRecoveryPayloadView) bool {
	if (payload.version != legacyMigrationRecoveryPayloadVersionV3 &&
		payload.version != legacyMigrationRecoveryPayloadVersionV4 &&
		payload.version != legacyMigrationRecoveryPayloadVersionV5 &&
		payload.version != legacyMigrationRecoveryPayloadVersionV6) ||
		!validLegacyMigrationSourceLocator(payload.sourceLocator) {
		return false
	}
	for _, locator := range payload.activeCredentialLocators {
		if !legacyMigrationLocatorBelongsToSource(locator, payload.sourceLocator) {
			return false
		}
	}
	for _, artifact := range payload.rollbackCredentialArtifacts {
		for _, locator := range artifact.locators {
			if !legacyMigrationLocatorBelongsToSource(locator, payload.sourceLocator) {
				return false
			}
		}
	}
	return true
}

func validLegacyMigrationCredentialLocatorSuffix(suffix []byte) bool {
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

type legacyMigrationPayloadCursor struct {
	plaintext []byte
	offset    int
}

func (cursor *legacyMigrationPayloadCursor) readUint32() (uint32, error) {
	if cursor == nil || cursor.offset > len(cursor.plaintext)-4 {
		return 0, registryport.ErrVerification
	}
	value := binary.BigEndian.Uint32(cursor.plaintext[cursor.offset : cursor.offset+4])
	cursor.offset += 4
	return value, nil
}

func (cursor *legacyMigrationPayloadCursor) readCount(maximum int, requireNonzero bool) (int, error) {
	value, err := cursor.readUint32()
	if err != nil || uint64(value) > uint64(maximum) || (requireNonzero && value == 0) {
		return 0, registryport.ErrVerification
	}
	return int(value), nil
}

func (cursor *legacyMigrationPayloadCursor) readField() ([]byte, error) {
	length, err := cursor.readUint32()
	if err != nil || uint64(length) > uint64(len(cursor.plaintext)) ||
		cursor.offset > len(cursor.plaintext)-int(length) {
		return nil, registryport.ErrVerification
	}
	field := cursor.plaintext[cursor.offset : cursor.offset+int(length)]
	cursor.offset += int(length)
	return field, nil
}

func parseLegacyMigrationRecoveryPayload(plaintext []byte) (legacyMigrationRecoveryPayloadView, error) {
	if len(plaintext) == 0 || len(plaintext) > maxLegacyMigrationRecoveryPayloadBytes ||
		len(plaintext) < len(legacyMigrationRecoveryPayloadMagic)+4 ||
		!bytes.Equal(plaintext[:len(legacyMigrationRecoveryPayloadMagic)], []byte(legacyMigrationRecoveryPayloadMagic)) {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	cursor := legacyMigrationPayloadCursor{plaintext: plaintext, offset: len(legacyMigrationRecoveryPayloadMagic)}
	version, err := cursor.readUint32()
	if err != nil || (version != legacyMigrationRecoveryPayloadVersion &&
		version != legacyMigrationRecoveryPayloadVersionV2 &&
		version != legacyMigrationRecoveryPayloadVersionV3 &&
		version != legacyMigrationRecoveryPayloadVersionV4 &&
		version != legacyMigrationRecoveryPayloadVersionV5 &&
		version != legacyMigrationRecoveryPayloadVersionV6) {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	var fields [5][]byte
	for index := range fields {
		fields[index], err = cursor.readField()
		if err != nil {
			return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
		}
	}
	view, err := parseLegacyMigrationRecoveryPayloadBase(version, fields)
	if err != nil {
		return legacyMigrationRecoveryPayloadView{}, err
	}
	if version == legacyMigrationRecoveryPayloadVersion {
		if cursor.offset != len(plaintext) {
			return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
		}
		return view, nil
	}

	activeLocatorCount, err := cursor.readCount(maxLegacyMigrationLocatorCount, true)
	if err != nil {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	view.activeCredentialLocators = make([][]byte, activeLocatorCount)
	for index := range view.activeCredentialLocators {
		view.activeCredentialLocators[index], err = cursor.readField()
		if err != nil {
			return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
		}
	}
	artifactCount, err := cursor.readCount(maxLegacyMigrationRollbackArtifactCount, false)
	if err != nil {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	view.rollbackCredentialArtifacts = make([]legacyMigrationRollbackCredentialArtifactView, artifactCount)
	totalLocatorCount := activeLocatorCount
	for index := range view.rollbackCredentialArtifacts {
		locatorCount, countErr := cursor.readCount(maxLegacyMigrationLocatorCount, true)
		if countErr != nil || locatorCount > maxLegacyMigrationLocatorCount-totalLocatorCount {
			return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
		}
		totalLocatorCount += locatorCount
		artifact := &view.rollbackCredentialArtifacts[index]
		artifact.locators = make([][]byte, locatorCount)
		for locatorIndex := range artifact.locators {
			artifact.locators[locatorIndex], err = cursor.readField()
			if err != nil {
				return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
			}
		}
		artifact.credential, err = cursor.readField()
		if err != nil {
			return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
		}
	}
	if version == legacyMigrationRecoveryPayloadVersionV3 ||
		version == legacyMigrationRecoveryPayloadVersionV4 ||
		version == legacyMigrationRecoveryPayloadVersionV5 ||
		version == legacyMigrationRecoveryPayloadVersionV6 {
		view.sourceLocator, err = cursor.readField()
		if err != nil {
			return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
		}
		view.sourceSnapshot, err = cursor.readField()
		if err != nil {
			return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
		}
		if version == legacyMigrationRecoveryPayloadVersionV4 ||
			version == legacyMigrationRecoveryPayloadVersionV5 ||
			version == legacyMigrationRecoveryPayloadVersionV6 {
			view.expectedCleanedSourceSHA256, err = cursor.readField()
			if err != nil {
				return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
			}
		}
		if version == legacyMigrationRecoveryPayloadVersionV5 ||
			version == legacyMigrationRecoveryPayloadVersionV6 {
			view.sourcePhysicalIdentitySHA256, err = cursor.readField()
			if err != nil {
				return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
			}
		}
		if version == legacyMigrationRecoveryPayloadVersionV6 {
			priorBytes, fieldErr := cursor.readField()
			if fieldErr != nil {
				return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
			}
			if !bytes.Equal(priorBytes, []byte("null")) {
				decoder := json.NewDecoder(bytes.NewReader(priorBytes))
				decoder.DisallowUnknownFields()
				var prior domainregistry.Provider
				if decodeErr := decoder.Decode(&prior); decodeErr != nil || prior.Validate() != nil ||
					prior.Tombstone || prior.CredentialRef == "" {
					return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
				}
				var trailing json.RawMessage
				if decodeErr := decoder.Decode(&trailing); !errors.Is(decodeErr, io.EOF) {
					return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
				}
				view.priorSelectedProvider = &prior
			}
		}
	}
	if cursor.offset != len(plaintext) || validateLegacyMigrationRecoveryV2View(view) != nil {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	return view, nil
}

func parseLegacyMigrationRecoveryPayloadBase(
	version uint32,
	fields [5][]byte,
) (legacyMigrationRecoveryPayloadView, error) {
	if !domainregistry.ValidLegacyMigrationID(string(fields[0])) ||
		!validLowerSHA256Bytes(fields[1]) || len(fields[4]) == 0 ||
		len(fields[4]) > maxLegacyMigrationCredentialBytes {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	decoder := json.NewDecoder(bytes.NewReader(fields[2]))
	decoder.DisallowUnknownFields()
	var provider domainregistry.ProviderInput
	if err := decoder.Decode(&provider); err != nil || provider.Validate() != nil {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	purpose, err := secretstoreport.NormalizePurpose(string(fields[3]))
	if err != nil || !bytes.Equal(fields[3], []byte(purpose)) {
		return legacyMigrationRecoveryPayloadView{}, registryport.ErrVerification
	}
	return legacyMigrationRecoveryPayloadView{
		version: version, migrationID: fields[0], sourceSHA256: fields[1], provider: provider,
		credentialPurpose: fields[3], credential: fields[4],
	}, nil
}

func validateLegacyMigrationRecoveryV2View(view legacyMigrationRecoveryPayloadView) error {
	if (view.version != legacyMigrationRecoveryPayloadVersionV2 &&
		view.version != legacyMigrationRecoveryPayloadVersionV3 &&
		view.version != legacyMigrationRecoveryPayloadVersionV4 &&
		view.version != legacyMigrationRecoveryPayloadVersionV5 &&
		view.version != legacyMigrationRecoveryPayloadVersionV6) || len(view.activeCredentialLocators) == 0 ||
		len(view.activeCredentialLocators) > maxLegacyMigrationLocatorCount ||
		len(view.rollbackCredentialArtifacts) > maxLegacyMigrationRollbackArtifactCount {
		return registryport.ErrVerification
	}
	locatorCount := 0
	locatorBytes := 0
	locators := make(map[string]struct{}, len(view.activeCredentialLocators))
	validateLocator := func(locator []byte) error {
		locatorCount++
		if locatorCount > maxLegacyMigrationLocatorCount ||
			len(locator) > maxLegacyMigrationLocatorTotalBytes-locatorBytes ||
			!validLegacyMigrationLogicalLocator(locator) {
			return registryport.ErrVerification
		}
		key := string(locator)
		if _, duplicate := locators[key]; duplicate {
			return registryport.ErrVerification
		}
		locators[key] = struct{}{}
		locatorBytes += len(locator)
		return nil
	}
	for _, locator := range view.activeCredentialLocators {
		if err := validateLocator(locator); err != nil {
			return err
		}
	}
	credentialBytes := len(view.credential)
	for index, artifact := range view.rollbackCredentialArtifacts {
		if len(artifact.locators) == 0 || len(artifact.credential) == 0 ||
			len(artifact.credential) > maxLegacyMigrationCredentialBytes ||
			credentialBytes > maxLegacyMigrationRecoveryCredentialTotalBytes-len(artifact.credential) ||
			bytes.Equal(view.credential, artifact.credential) {
			return registryport.ErrVerification
		}
		for prior := 0; prior < index; prior++ {
			if bytes.Equal(view.rollbackCredentialArtifacts[prior].credential, artifact.credential) {
				return registryport.ErrVerification
			}
		}
		for _, locator := range artifact.locators {
			if err := validateLocator(locator); err != nil {
				return err
			}
		}
		credentialBytes += len(artifact.credential)
	}
	if view.version == legacyMigrationRecoveryPayloadVersionV3 ||
		view.version == legacyMigrationRecoveryPayloadVersionV4 ||
		view.version == legacyMigrationRecoveryPayloadVersionV5 ||
		view.version == legacyMigrationRecoveryPayloadVersionV6 {
		if len(view.sourceSnapshot) == 0 || len(view.sourceSnapshot) > maxLegacyMigrationSourceSnapshotBytes {
			return registryport.ErrVerification
		}
		digest := sha256.Sum256(view.sourceSnapshot)
		if !bytes.Equal([]byte(hex.EncodeToString(digest[:])), view.sourceSHA256) {
			return registryport.ErrVerification
		}
		if view.version == legacyMigrationRecoveryPayloadVersionV4 ||
			view.version == legacyMigrationRecoveryPayloadVersionV5 ||
			view.version == legacyMigrationRecoveryPayloadVersionV6 {
			if !validLowerSHA256Bytes(view.expectedCleanedSourceSHA256) {
				return registryport.ErrVerification
			}
		} else if len(view.expectedCleanedSourceSHA256) != 0 {
			return registryport.ErrVerification
		}
		if view.version == legacyMigrationRecoveryPayloadVersionV5 ||
			view.version == legacyMigrationRecoveryPayloadVersionV6 {
			if !validLowerSHA256Bytes(view.sourcePhysicalIdentitySHA256) {
				return registryport.ErrVerification
			}
		} else if len(view.sourcePhysicalIdentitySHA256) != 0 {
			return registryport.ErrVerification
		}
	} else if len(view.sourceSnapshot) != 0 {
		return registryport.ErrVerification
	}
	return nil
}

func registryCredentialRefAvailable(
	state domainregistry.Registry,
	credentialRef string,
	allowedMigrationID string,
) bool {
	if !domainregistry.ValidCredentialRef(credentialRef) {
		return false
	}
	for _, provider := range state.Providers {
		if provider.CredentialRef == credentialRef {
			return false
		}
	}
	for _, transaction := range state.Transactions {
		if transaction.CandidateCredentialRef == credentialRef ||
			transaction.SupersededCredentialRef == credentialRef || transaction.CleanupCredentialRef == credentialRef {
			return false
		}
	}
	for id, recovery := range state.LegacyMigrationRecoveries {
		if id != allowedMigrationID && recovery.RecoveryCredentialRef == credentialRef {
			return false
		}
	}
	return true
}

func verifiedLegacyMigrationRecoveryResult(
	recovery domainregistry.LegacyMigrationRecovery,
) LegacyMigrationRecoveryResult {
	return LegacyMigrationRecoveryResult{
		Status:                             LegacyMigrationRecoveryStatusVerified,
		MigrationID:                        recovery.ID,
		RecoveryCredentialRef:              secretstoreport.CredentialRef(recovery.RecoveryCredentialRef),
		SafeToProceedWithProviderMigration: recovery.ExpectedCleanedSourceIdentitySHA256 != "",
	}
}

func cloneProviderInput(input domainregistry.ProviderInput) domainregistry.ProviderInput {
	clone := input
	clone.Models = append([]string(nil), input.Models...)
	clone.MediaModels = append([]string(nil), input.MediaModels...)
	clone.SelectedRoutes = append([]string(nil), input.SelectedRoutes...)
	return clone
}

func validLowerSHA256(value string) bool {
	return validLowerSHA256Bytes([]byte(value))
}

func validLowerSHA256Bytes(value []byte) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, current := range value {
		if (current >= 'a' && current <= 'f') || (current >= '0' && current <= '9') {
			continue
		}
		return false
	}
	return true
}
