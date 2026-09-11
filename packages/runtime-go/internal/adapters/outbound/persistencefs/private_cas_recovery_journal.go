package persistencefs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	domainprivatecas "analytix.local/runtime-go/internal/domain/privatecastopology"
	privatecasport "analytix.local/runtime-go/internal/ports/privatecas"
)

const (
	privateCASRecoveryJournalDirectoryV1 = "private-cas-recovery-v1"
	privateCASRecoveryTargetsDirectoryV1 = "targets"

	privateCASRecoveryPreparationFileV1 = "preparation.json"
	privateCASRecoveryManifestFileV1    = "manifest.json"
	privateCASRecoveryWitnessFileV1     = "commit-witness.json"
	privateCASRecoveryCompletionFileV1  = "completion-receipt.json"
	privateCASRecoveryRetirementFileV1  = "retirement.json"

	privateCASRecoveryRetiredPrefixV1 = ".retired-private-cas-recovery-"

	privateCASRecoveryRetirementSchemaVersionV1 = 1
	privateCASRecoveryRetirementPurposeV1       = "analytix.private-cas-recovery-retirement/v1"
	privateCASRecoveryTempSchemaVersionV1       = 1
	privateCASRecoveryTempPurposeV1             = "analytix.private-cas-recovery-exclusive-temp/v1"

	maxPrivateCASRecoveryRetirementBytesV1 = 64 << 10
	maxPrivateCASRecoverySigningBytesV1    = 1 << 20
	maxPrivateCASRecoveryTopEntriesV1      = 6
)

var (
	errPrivateCASRecoveryBootstrapIncompleteV1  = errors.New("private CAS recovery journal bootstrap is incomplete")
	privateCASRecoveryRetirementSignDomainV1    = []byte("analytix.private-cas-recovery-retirement/signature/v1\x00")
	privateCASRecoveryRetirementDigestDomainV1  = []byte("analytix.private-cas-recovery-retirement/digest/v1\x00")
	privateCASRecoveryInventoryDigestDomainV1   = []byte("analytix.private-cas-recovery-retirement-inventory/digest/v1\x00")
	privateCASRecoveryDirectoryIdentityDomainV1 = []byte("analytix.private-cas-recovery-directory-identity/digest/v1\x00")
	privateCASRecoveryTempSignatureDomainV1     = []byte("analytix.private-cas-recovery-exclusive-temp/signature/v1\x00")
)

type privateCASRecoveryJournalV1 struct {
	mu               sync.Mutex
	lease            *CompositeLease
	roots            RootSet
	rootAuthority    *RootAuthority
	journalAuthority *JournalNamespaceAuthority
}

type privateCASRecoveryRetirementV1 struct {
	SchemaVersion            int    `json:"schemaVersion"`
	Purpose                  string `json:"purpose"`
	TransactionID            string `json:"transactionId"`
	CompletionReceiptDigest  string `json:"completionReceiptDigest"`
	RootBindingDigest        string `json:"rootBindingDigest"`
	JournalDirectoryIdentity string `json:"journalDirectoryIdentity"`
	TargetsDirectoryIdentity string `json:"targetsDirectoryIdentity"`
	InventoryDigest          string `json:"inventoryDigest"`
	InventoryEntryCount      uint32 `json:"inventoryEntryCount"`
	AuthorityKeyID           string `json:"authorityKeyId"`
	AuthoritySignature       string `json:"authoritySignature"`
	RetirementDigest         string `json:"retirementDigest"`
}

type privateCASRecoveryInventoryEntryV1 struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Identity string `json:"identity"`
	SHA256   string `json:"sha256,omitempty"`
	Bytes    uint64 `json:"bytes,omitempty"`
}

type privateCASRecoveryTempAuthorityV1 struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Purpose           string `json:"purpose"`
	DirectoryIdentity string `json:"directoryIdentity"`
	TargetName        string `json:"targetName"`
	BodySHA256        string `json:"bodySha256"`
	ByteLength        uint64 `json:"byteLength"`
	Nonce             string `json:"nonce"`
}

var _ privatecasport.RecoveryJournalV1 = (*privateCASRecoveryJournalV1)(nil)

// NewPrivateCASRecoveryJournalV1 binds the adapter to the same live composite
// lease used by private CAS. Construction validates frozen authorities but
// never creates a signing key, recovery directory, or journal record.
func NewPrivateCASRecoveryJournalV1(lease *CompositeLease) (privatecasport.RecoveryJournalV1, error) {
	if lease == nil {
		return nil, errors.New("private CAS recovery journal lease is unavailable")
	}
	roots, rootsHeld := lease.FrozenRoots()
	rootAuthority, rootHeld := lease.FrozenAuthority()
	journalAuthority, journalHeld := lease.FrozenJournalAuthority()
	if !rootsHeld || !rootHeld || !journalHeld || rootAuthority == nil || journalAuthority == nil ||
		rootAuthority.Validate() != nil || journalAuthority.Validate() != nil || !journalAuthority.matchesRoots(roots) {
		return nil, errors.New("private CAS recovery journal authority is unavailable")
	}
	return &privateCASRecoveryJournalV1{
		lease: lease, roots: roots, rootAuthority: rootAuthority, journalAuthority: journalAuthority,
	}, nil
}

func (journal *privateCASRecoveryJournalV1) KeyID(ctx context.Context) (string, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		return "", err
	}
	return authority.keyID, nil
}

func (journal *privateCASRecoveryJournalV1) Sign(ctx context.Context, message []byte) (string, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if len(message) == 0 || len(message) > maxPrivateCASRecoverySigningBytesV1 {
		return "", errors.New("private CAS recovery signing input is outside its bound")
	}
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		return "", err
	}
	return authority.signDomain(nil, message)
}

func (journal *privateCASRecoveryJournalV1) Verify(ctx context.Context, keyID string, message []byte, signature string) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if len(message) == 0 || len(message) > maxPrivateCASRecoverySigningBytesV1 {
		return errors.New("private CAS recovery verification input is outside its bound")
	}
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		return err
	}
	return authority.verifyDomain(nil, keyID, message, signature)
}

func (journal *privateCASRecoveryJournalV1) BeginPreparationAfterValidatedPreflight(
	ctx context.Context,
	request privatecasport.RecoveryPreparationRequestV1,
) (domainprivatecas.RecoveryJournalPreparationV1, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if err := journal.validateAuthorityLocked(ctx); err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	if err := journal.recoverRetiredLocked(ctx); err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	if session, err := journal.loadActiveLocked(ctx, true); err == nil {
		if err := journal.validatePreparationReplayRequestLocked(ctx, session.Preparation, request); err != nil {
			return domainprivatecas.RecoveryJournalPreparationV1{}, err
		}
		return session.Preparation, nil
	} else if !errors.Is(err, privatecasport.ErrJournalAbsent) && !errors.Is(err, errPrivateCASRecoveryBootstrapIncompleteV1) {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	authority, err := journal.loadAuthorityLocked(ctx, true)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	directory, targets, err := journal.openOrCreateBootstrapLocked(ctx)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	journalRawIdentity := directory.Identity()
	journalIdentity := privateCASRecoveryDirectoryIdentityDigestV1(journalRawIdentity)
	targetsIdentity := privateCASRecoveryDirectoryIdentityDigestV1(targets.Identity())
	closeErr := targets.Close()
	if closeErr == nil {
		closeErr = directory.Close()
	} else {
		_ = directory.Close()
	}
	if closeErr != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, closeErr
	}
	draft, err := domainprivatecas.NewRecoveryJournalPreparationDraftV1(
		domainprivatecas.RecoveryJournalPreparationInputV1{
			RootBindingDigest: rootBindingDigest(journal.roots), AuthoritySetDigest: request.AuthoritySetDigest,
			ParticipantCount: request.ParticipantCount, PlanCount: request.PlanCount, TopologyCount: request.TopologyCount,
			JournalDirectoryIdentity: journalIdentity, TargetsDirectoryIdentity: targetsIdentity,
			PreparedAt: request.PreparedAt, AuthorityKeyID: authority.keyID,
		},
	)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, errors.New("private CAS recovery preparation request is invalid")
	}
	signingBytes, err := domainprivatecas.RecoveryJournalPreparationSigningBytesV1(draft)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	signature, err := authority.signDomain(nil, signingBytes)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	preparation, err := domainprivatecas.SealRecoveryJournalPreparationV1(draft, signature)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	body, err := domainprivatecas.RecoveryJournalPreparationV1Bytes(preparation)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	directory, err = secureStartupOpenDirectory(journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, journalRawIdentity)
	if err != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, err
	}
	writeErr := writePrivateCASRecoveryRecordExactV1(
		directory, authority, privateCASRecoveryPreparationFileV1, body, domainprivatecas.MaxRecoveryJournalRecordBytesV1,
	)
	closeErr = directory.Close()
	if writeErr != nil || closeErr != nil {
		return domainprivatecas.RecoveryJournalPreparationV1{}, errors.Join(writeErr, closeErr)
	}
	session, err := journal.loadActiveLocked(ctx, false)
	if err != nil || session.Preparation.PreparationDigest != preparation.PreparationDigest {
		return domainprivatecas.RecoveryJournalPreparationV1{}, errors.Join(
			errors.New("private CAS recovery preparation readback failed"), err,
		)
	}
	return session.Preparation, nil
}

func (journal *privateCASRecoveryJournalV1) Load(ctx context.Context) (domainprivatecas.RecoveryJournalSessionV1, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if err := journal.validateAuthorityLocked(ctx); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	if err := journal.recoverRetiredLocked(ctx); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	return journal.loadActiveLocked(ctx, true)
}

// PutPreparationIfAbsent is replay-only. A first preparation must use
// BeginPreparationAfterValidatedPreflight so key and directory identities are
// host-filled in one non-bypassable operation.
func (journal *privateCASRecoveryJournalV1) PutPreparationIfAbsent(
	ctx context.Context,
	preparation domainprivatecas.RecoveryJournalPreparationV1,
) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	session, err := journal.loadForMutationLocked(ctx)
	if err != nil {
		return err
	}
	return exactPrivateCASRecoveryRecordV1(
		preparation,
		session.Preparation,
		domainprivatecas.RecoveryJournalPreparationV1Bytes,
		"private CAS recovery preparation conflicts with the active journal",
	)
}

func (journal *privateCASRecoveryJournalV1) PutTargetChunkIfAbsent(
	ctx context.Context,
	chunk domainprivatecas.RecoveryTargetChunkV1,
) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if err := domainprivatecas.ValidateRecoveryTargetChunkV1(chunk); err != nil {
		return err
	}
	session, err := journal.loadForMutationLocked(ctx)
	if err != nil {
		return err
	}
	if int(chunk.ChunkIndex) < len(session.Chunks) {
		return exactPrivateCASRecoveryRecordV1(
			chunk, session.Chunks[chunk.ChunkIndex], domainprivatecas.RecoveryTargetChunkV1Bytes,
			"private CAS recovery target chunk conflicts with the active journal",
		)
	}
	if session.State != domainprivatecas.RecoveryJournalSessionPreparedV1 || int(chunk.ChunkIndex) != len(session.Chunks) {
		return errors.New("private CAS recovery target chunk is outside the append frontier")
	}
	candidate := make([]domainprivatecas.RecoveryTargetChunkV1, 0, len(session.Chunks)+1)
	candidate = append(candidate, session.Chunks...)
	candidate = append(candidate, chunk)
	if _, _, err := domainprivatecas.RecoveryTargetSetRootV1(candidate, session.Preparation.PlanCount); err != nil {
		return errors.New("private CAS recovery target chunk does not match the active preparation")
	}
	body, err := domainprivatecas.RecoveryTargetChunkV1Bytes(chunk)
	if err != nil {
		return err
	}
	directory, targets, err := journal.openActiveDirectoriesLocked(session.Preparation)
	if err != nil {
		return err
	}
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		_ = targets.Close()
		_ = directory.Close()
		return err
	}
	writeErr := writePrivateCASRecoveryRecordExactV1(
		targets, authority, privateCASRecoveryChunkNameV1(chunk.ChunkIndex), body, domainprivatecas.MaxRecoveryTargetChunkBytesV1,
	)
	closeErr := errors.Join(targets.Close(), directory.Close())
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	loaded, err := journal.loadActiveLocked(ctx, false)
	if err != nil || len(loaded.Chunks) != len(session.Chunks)+1 || loaded.Chunks[chunk.ChunkIndex].ChunkDigest != chunk.ChunkDigest {
		return errors.Join(errors.New("private CAS recovery target chunk readback failed"), err)
	}
	return nil
}

func (journal *privateCASRecoveryJournalV1) PutManifestIfAbsent(
	ctx context.Context,
	manifest domainprivatecas.RecoveryJournalManifestV1,
) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return putPrivateCASRecoverySignedRecordLockedV1(
		journal,
		ctx, privateCASRecoveryManifestFileV1, manifest,
		domainprivatecas.RecoveryJournalManifestV1Bytes,
		domainprivatecas.RecoveryJournalManifestSigningBytesV1,
		func(session domainprivatecas.RecoveryJournalSessionV1) error {
			if session.Manifest != nil {
				return exactPrivateCASRecoveryRecordV1(
					manifest, *session.Manifest, domainprivatecas.RecoveryJournalManifestV1Bytes,
					"private CAS recovery manifest conflicts with the active journal",
				)
			}
			if session.State != domainprivatecas.RecoveryJournalSessionPreparedV1 ||
				domainprivatecas.ValidateRecoveryJournalManifestForPreparationV1(manifest, session.Preparation) != nil ||
				domainprivatecas.ValidateRecoveryJournalManifestForChunksV1(manifest, session.Chunks) != nil {
				return errors.New("private CAS recovery manifest does not match the active preparation")
			}
			return nil
		},
		func(session domainprivatecas.RecoveryJournalSessionV1) bool {
			return session.Manifest != nil && session.Manifest.ManifestDigest == manifest.ManifestDigest
		},
	)
}

func (journal *privateCASRecoveryJournalV1) PutCommitWitnessIfAbsent(
	ctx context.Context,
	witness domainprivatecas.RecoveryJournalCommitWitnessV1,
) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return putPrivateCASRecoverySignedRecordLockedV1(
		journal,
		ctx, privateCASRecoveryWitnessFileV1, witness,
		domainprivatecas.RecoveryJournalCommitWitnessV1Bytes,
		domainprivatecas.RecoveryJournalCommitWitnessSigningBytesV1,
		func(session domainprivatecas.RecoveryJournalSessionV1) error {
			if session.CommitWitness != nil {
				return exactPrivateCASRecoveryRecordV1(
					witness, *session.CommitWitness, domainprivatecas.RecoveryJournalCommitWitnessV1Bytes,
					"private CAS recovery commit witness conflicts with the active journal",
				)
			}
			if session.State != domainprivatecas.RecoveryJournalSessionManifestedV1 || session.Manifest == nil ||
				domainprivatecas.ValidateRecoveryJournalCommitWitnessForManifestV1(witness, *session.Manifest) != nil {
				return errors.New("private CAS recovery commit witness does not match the active manifest")
			}
			return nil
		},
		func(session domainprivatecas.RecoveryJournalSessionV1) bool {
			return session.CommitWitness != nil && session.CommitWitness.WitnessDigest == witness.WitnessDigest
		},
	)
}

func (journal *privateCASRecoveryJournalV1) PutCompletionReceiptIfAbsent(
	ctx context.Context,
	receipt domainprivatecas.RecoveryJournalCompletionReceiptV1,
) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return putPrivateCASRecoverySignedRecordLockedV1(
		journal,
		ctx, privateCASRecoveryCompletionFileV1, receipt,
		domainprivatecas.RecoveryJournalCompletionReceiptV1Bytes,
		domainprivatecas.RecoveryJournalCompletionSigningBytesV1,
		func(session domainprivatecas.RecoveryJournalSessionV1) error {
			if session.CompletionReceipt != nil {
				return exactPrivateCASRecoveryRecordV1(
					receipt, *session.CompletionReceipt, domainprivatecas.RecoveryJournalCompletionReceiptV1Bytes,
					"private CAS recovery completion conflicts with the active journal",
				)
			}
			if session.State != domainprivatecas.RecoveryJournalSessionCommitWitnessedV1 || session.CommitWitness == nil ||
				domainprivatecas.ValidateRecoveryJournalCompletionForWitnessV1(receipt, *session.CommitWitness) != nil {
				return errors.New("private CAS recovery completion does not match the active witness")
			}
			return nil
		},
		func(session domainprivatecas.RecoveryJournalSessionV1) bool {
			return session.CompletionReceipt != nil && session.CompletionReceipt.ReceiptDigest == receipt.ReceiptDigest
		},
	)
}

func (journal *privateCASRecoveryJournalV1) Retire(
	ctx context.Context,
	request privatecasport.RecoveryRetirementRequestV1,
) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if !domainprivatecas.ValidDigestV1(request.TransactionID) ||
		!domainprivatecas.ValidDigestV1(request.CompletionReceiptDigest) {
		return errors.New("private CAS recovery retirement selector is invalid")
	}
	if err := journal.validateAuthorityLocked(ctx); err != nil {
		return err
	}
	if err := journal.recoverRetiredLocked(ctx); err != nil {
		return err
	}
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		return err
	}
	directory, err := secureStartupOpenDirectory(journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, "")
	if errors.Is(err, os.ErrNotExist) {
		return privatecasport.ErrJournalAbsent
	}
	if err != nil {
		return err
	}
	emptyBootstrap, err := journal.cleanActiveExclusiveWriteResiduesLocked(ctx, directory, authority)
	if err != nil {
		_ = directory.Close()
		return err
	}
	if emptyBootstrap {
		closeErr := directory.Close()
		if closeErr != nil {
			return closeErr
		}
		return privatecasport.ErrJournalAbsent
	}
	session, retirement, err := journal.readSessionFromDirectoryLocked(ctx, directory, authority)
	if err != nil {
		_ = directory.Close()
		return err
	}
	if session.State != domainprivatecas.RecoveryJournalSessionCompletedV1 || session.Manifest == nil || session.CompletionReceipt == nil ||
		session.Manifest.TransactionID != request.TransactionID ||
		session.CompletionReceipt.ReceiptDigest != request.CompletionReceiptDigest {
		_ = directory.Close()
		return errors.New("private CAS recovery retirement does not match a completed session")
	}
	if retirement == nil {
		retirement, err = journal.newRetirementLocked(ctx, directory, authority, session)
		if err == nil {
			body, bodyErr := privateCASRecoveryRetirementBytesV1(*retirement)
			if bodyErr != nil {
				err = bodyErr
			} else {
				err = writePrivateCASRecoveryRecordExactV1(
					directory, authority, privateCASRecoveryRetirementFileV1, body, maxPrivateCASRecoveryRetirementBytesV1,
				)
			}
		}
	}
	if err == nil {
		err = journal.verifyRetirementForDirectoryLocked(ctx, directory, authority, session, *retirement)
	}
	directoryIdentity := directory.Identity()
	closeErr := directory.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	return journal.finishActiveRetirementLocked(ctx, authority, directoryIdentity)
}

func (journal *privateCASRecoveryJournalV1) validateAuthorityLocked(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if journal == nil || journal.rootAuthority == nil || journal.journalAuthority == nil ||
		journal.rootAuthority.Validate() != nil || journal.journalAuthority.Validate() != nil ||
		!journal.journalAuthority.matchesRoots(journal.roots) {
		return errors.New("private CAS recovery journal authority is unavailable")
	}
	if journal.lease == nil {
		return nil
	}
	roots, rootsHeld := journal.lease.FrozenRoots()
	rootAuthority, rootHeld := journal.lease.FrozenAuthority()
	journalAuthority, journalHeld := journal.lease.FrozenJournalAuthority()
	if !rootsHeld || !rootHeld || !journalHeld || roots != journal.roots ||
		rootAuthority != journal.rootAuthority || journalAuthority != journal.journalAuthority {
		return errors.New("private CAS recovery journal lease authority changed")
	}
	return nil
}

func (journal *privateCASRecoveryJournalV1) authorityAnchorLocked() string {
	return filepath.Join(journal.journalAuthority.path(), "journal")
}

func (journal *privateCASRecoveryJournalV1) loadAuthorityLocked(ctx context.Context, create bool) (*startupJournalAuthority, error) {
	if err := journal.validateAuthorityLocked(ctx); err != nil {
		return nil, err
	}
	anchor := journal.authorityAnchorLocked()
	var (
		authority *startupJournalAuthority
		err       error
	)
	if create {
		authority, err = openOrCreateStartupJournalAuthority(journal.journalAuthority, anchor)
	} else {
		authority, err = loadStartupJournalAuthority(journal.journalAuthority, anchor)
	}
	if err != nil {
		return nil, errors.New("private CAS recovery journal signing authority is unavailable")
	}
	return authority, nil
}

func (journal *privateCASRecoveryJournalV1) openOrCreateBootstrapLocked(
	ctx context.Context,
) (*startupPrivateDirectory, *startupPrivateDirectory, error) {
	if err := journal.validateAuthorityLocked(ctx); err != nil {
		return nil, nil, err
	}
	directory, err := secureStartupCreateDirectory(journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1)
	if errors.Is(err, os.ErrExist) {
		directory, err = secureStartupOpenDirectory(journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, "")
	}
	if err != nil {
		return nil, nil, err
	}
	entries, err := directory.ReadEntriesBoundedContext(ctx, 1)
	if err != nil {
		_ = directory.Close()
		return nil, nil, err
	}
	var targets *startupPrivateDirectory
	switch len(entries) {
	case 0:
		targets, err = directory.CreateDirectory(privateCASRecoveryTargetsDirectoryV1)
	case 1:
		if entries[0].Name() != privateCASRecoveryTargetsDirectoryV1 || !entries[0].IsDir() {
			err = errors.New("private CAS recovery bootstrap inventory is unsafe")
		} else {
			targets, err = directory.OpenDirectory(privateCASRecoveryTargetsDirectoryV1, "")
		}
	default:
		err = errors.New("private CAS recovery bootstrap inventory is ambiguous")
	}
	if err != nil {
		_ = directory.Close()
		return nil, nil, err
	}
	targetEntries, err := targets.ReadEntriesBoundedContext(ctx, 0)
	if err != nil || len(targetEntries) != 0 {
		_ = targets.Close()
		_ = directory.Close()
		return nil, nil, errors.Join(errors.New("private CAS recovery bootstrap targets are not empty"), err)
	}
	return directory, targets, nil
}

func (journal *privateCASRecoveryJournalV1) validatePreparationReplayRequestLocked(
	ctx context.Context,
	preparation domainprivatecas.RecoveryJournalPreparationV1,
	request privatecasport.RecoveryPreparationRequestV1,
) error {
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		return err
	}
	draft, err := domainprivatecas.NewRecoveryJournalPreparationDraftV1(
		domainprivatecas.RecoveryJournalPreparationInputV1{
			RootBindingDigest: rootBindingDigest(journal.roots), AuthoritySetDigest: request.AuthoritySetDigest,
			ParticipantCount: request.ParticipantCount, PlanCount: request.PlanCount, TopologyCount: request.TopologyCount,
			JournalDirectoryIdentity: preparation.JournalDirectoryIdentity,
			TargetsDirectoryIdentity: preparation.TargetsDirectoryIdentity,
			PreparedAt:               request.PreparedAt, AuthorityKeyID: authority.keyID,
		},
	)
	if err != nil {
		return errors.New("private CAS recovery preparation replay request is invalid")
	}
	signingBytes, err := domainprivatecas.RecoveryJournalPreparationSigningBytesV1(draft)
	if err != nil {
		return err
	}
	signature, err := authority.signDomain(nil, signingBytes)
	if err != nil {
		return err
	}
	expected, err := domainprivatecas.SealRecoveryJournalPreparationV1(draft, signature)
	if err != nil {
		return err
	}
	return exactPrivateCASRecoveryRecordV1(
		expected, preparation, domainprivatecas.RecoveryJournalPreparationV1Bytes,
		"private CAS recovery preparation replay request conflicts with the active journal",
	)
}

func (journal *privateCASRecoveryJournalV1) loadForMutationLocked(ctx context.Context) (domainprivatecas.RecoveryJournalSessionV1, error) {
	if err := journal.validateAuthorityLocked(ctx); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	if err := journal.recoverRetiredLocked(ctx); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	return journal.loadActiveLocked(ctx, true)
}

func (journal *privateCASRecoveryJournalV1) loadActiveLocked(
	ctx context.Context,
	finishRetirement bool,
) (domainprivatecas.RecoveryJournalSessionV1, error) {
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		directory, openErr := secureStartupOpenDirectory(journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, "")
		if errors.Is(openErr, os.ErrNotExist) {
			return domainprivatecas.RecoveryJournalSessionV1{}, privatecasport.ErrJournalAbsent
		}
		if openErr == nil {
			_ = directory.Close()
		}
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	directory, err := secureStartupOpenDirectory(journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, "")
	if errors.Is(err, os.ErrNotExist) {
		return domainprivatecas.RecoveryJournalSessionV1{}, privatecasport.ErrJournalAbsent
	}
	if err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	emptyBootstrap, err := journal.cleanActiveExclusiveWriteResiduesLocked(ctx, directory, authority)
	if err != nil {
		_ = directory.Close()
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	if emptyBootstrap {
		closeErr := directory.Close()
		if closeErr != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, closeErr
		}
		return domainprivatecas.RecoveryJournalSessionV1{}, privatecasport.ErrJournalAbsent
	}
	session, retirement, err := journal.readSessionFromDirectoryLocked(ctx, directory, authority)
	if err != nil {
		_ = directory.Close()
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	if retirement == nil || !finishRetirement {
		closeErr := directory.Close()
		return session, closeErr
	}
	if err := journal.verifyRetirementForDirectoryLocked(ctx, directory, authority, session, *retirement); err != nil {
		_ = directory.Close()
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	directoryIdentity := directory.Identity()
	if err := directory.Close(); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	if err := journal.finishActiveRetirementLocked(ctx, authority, directoryIdentity); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, err
	}
	return domainprivatecas.RecoveryJournalSessionV1{}, privatecasport.ErrJournalAbsent
}

func (journal *privateCASRecoveryJournalV1) cleanActiveExclusiveWriteResiduesLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
) (bool, error) {
	if err := contextError(ctx); err != nil {
		return false, err
	}
	targets, err := directory.OpenDirectory(privateCASRecoveryTargetsDirectoryV1, "")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			entries, inventoryErr := directory.ReadEntriesBoundedContext(ctx, 0)
			if inventoryErr == nil && len(entries) == 0 {
				return true, nil
			}
		}
		return false, errors.New("private CAS recovery targets directory is unavailable during residue preflight")
	}
	topPolicy := journal.privateCASRecoveryTopResiduePolicyV1(directory, targets, authority)
	targetPolicy := privateCASRecoveryTargetResiduePolicyV1(targets, authority)
	topPlan, topErr := prepareStartupExclusiveWriteResidueCleanup(ctx, directory, topPolicy)
	targetPlan, targetErr := prepareStartupExclusiveWriteResidueCleanup(ctx, targets, targetPolicy)
	if topErr != nil || targetErr != nil {
		_ = targets.Close()
		return false, errors.Join(topErr, targetErr)
	}
	candidateCount := len(topPlan.candidates) + len(targetPlan.candidates)
	if candidateCount == 0 {
		emptyBootstrap := privateCASRecoveryResiduePlansYieldEmptyBootstrapV1(topPlan, targetPlan)
		return emptyBootstrap, targets.Close()
	}
	if candidateCount != 1 {
		_ = targets.Close()
		return false, errors.New("private CAS recovery contains multiple authenticated exclusive-write residues")
	}
	ignoredTop := startupExclusiveWriteResidueNamesV1(topPlan)
	ignoredTargets := startupExclusiveWriteResidueNamesV1(targetPlan)
	session, committedRetirement, sessionErr := journal.readSessionFromDirectoryIgnoringResiduesLocked(
		ctx, directory, authority, ignoredTop, ignoredTargets,
	)
	committedSnapshot, snapshotErr := privateCASRecoveryCommittedResidueSnapshotV1(
		session, committedRetirement, sessionErr,
	)
	if snapshotErr != nil {
		_ = targets.Close()
		return false, snapshotErr
	}
	if err := journal.validatePrivateCASRecoveryResidueFrontierLocked(
		ctx, directory, authority, session, committedRetirement, sessionErr,
		topPlan, targetPlan, ignoredTop, ignoredTargets,
	); err != nil {
		_ = targets.Close()
		return false, err
	}
	if err := topPlan.Revalidate(ctx); err != nil {
		_ = targets.Close()
		return false, err
	}
	if err := targetPlan.Revalidate(ctx); err != nil {
		_ = targets.Close()
		return false, err
	}
	reloadedSession, reloadedRetirement, reloadedErr := journal.readSessionFromDirectoryIgnoringResiduesLocked(
		ctx, directory, authority, ignoredTop, ignoredTargets,
	)
	reloadedSnapshot, reloadedSnapshotErr := privateCASRecoveryCommittedResidueSnapshotV1(
		reloadedSession, reloadedRetirement, reloadedErr,
	)
	if reloadedSnapshotErr != nil || !bytes.Equal(committedSnapshot, reloadedSnapshot) {
		_ = targets.Close()
		return false, errors.Join(
			errors.New("private CAS recovery committed frontier changed before residue cleanup"), reloadedSnapshotErr,
		)
	}
	emptyBootstrap := privateCASRecoveryResiduePlansYieldEmptyBootstrapV1(topPlan, targetPlan)
	applyErr := errors.Join(targetPlan.Apply(ctx), topPlan.Apply(ctx))
	closeErr := targets.Close()
	return emptyBootstrap && applyErr == nil && closeErr == nil, errors.Join(applyErr, closeErr)
}

func privateCASRecoveryCommittedResidueSnapshotV1(
	session domainprivatecas.RecoveryJournalSessionV1,
	retirement *privateCASRecoveryRetirementV1,
	sessionErr error,
) ([]byte, error) {
	if sessionErr != nil {
		if errors.Is(sessionErr, errPrivateCASRecoveryBootstrapIncompleteV1) {
			return []byte("private-cas-recovery-bootstrap-incomplete-v1"), nil
		}
		return nil, sessionErr
	}
	body, err := json.Marshal(struct {
		Session    domainprivatecas.RecoveryJournalSessionV1 `json:"session"`
		Retirement *privateCASRecoveryRetirementV1           `json:"retirement,omitempty"`
	}{Session: session, Retirement: retirement})
	if err != nil {
		return nil, errors.New("private CAS recovery committed frontier snapshot is invalid")
	}
	return body, nil
}

func privateCASRecoveryResiduePlansYieldEmptyBootstrapV1(
	topPlan *startupExclusiveWriteResidueCleanupPlan,
	targetPlan *startupExclusiveWriteResidueCleanupPlan,
) bool {
	if topPlan == nil || targetPlan == nil {
		return false
	}
	sawTargetsDirectory := false
	for _, entry := range topPlan.inventory {
		if entry.name == privateCASRecoveryTargetsDirectoryV1 && entry.directory {
			sawTargetsDirectory = true
			continue
		}
		isCandidate := false
		for _, candidate := range topPlan.candidates {
			if entry.name == candidate.tempName {
				isCandidate = true
				break
			}
		}
		if !isCandidate {
			return false
		}
	}
	if !sawTargetsDirectory {
		return false
	}
	for _, entry := range targetPlan.inventory {
		isCandidate := false
		for _, candidate := range targetPlan.candidates {
			if entry.name == candidate.tempName {
				isCandidate = true
				break
			}
		}
		if !isCandidate {
			return false
		}
	}
	return true
}

func (journal *privateCASRecoveryJournalV1) privateCASRecoveryTopResiduePolicyV1(
	directory *startupPrivateDirectory,
	targets *startupPrivateDirectory,
	authority *startupJournalAuthority,
) startupExclusiveWriteResidueInventoryPolicy {
	policies := map[string]startupExclusiveWriteResidueTargetPolicy{}
	policies[privateCASRecoveryPreparationFileV1] = startupExclusiveWriteResidueTargetPolicy{
		MaxBytes: domainprivatecas.MaxRecoveryJournalRecordBytesV1,
		Validate: func(tempName string, body []byte) error {
			if err := validatePrivateCASRecoveryAuthenticatedTempV1(
				directory, authority, privateCASRecoveryPreparationFileV1, tempName, body,
			); err != nil {
				return err
			}
			record, err := domainprivatecas.ParseRecoveryJournalPreparationV1(body)
			if err != nil || journal.verifyPreparationLocked(authority, record) != nil ||
				record.RootBindingDigest != rootBindingDigest(journal.roots) ||
				record.JournalDirectoryIdentity != privateCASRecoveryDirectoryIdentityDigestV1(directory.Identity()) ||
				record.TargetsDirectoryIdentity != privateCASRecoveryDirectoryIdentityDigestV1(targets.Identity()) {
				return errors.New("private CAS recovery preparation residue authority is invalid")
			}
			return nil
		},
	}
	policies[privateCASRecoveryManifestFileV1] = privateCASRecoverySignedTopResiduePolicyV1(
		directory, authority, privateCASRecoveryManifestFileV1,
		func(body []byte) error {
			record, err := domainprivatecas.ParseRecoveryJournalManifestV1(body)
			if err != nil {
				return err
			}
			return journal.verifyManifestLocked(authority, record)
		},
	)
	policies[privateCASRecoveryWitnessFileV1] = privateCASRecoverySignedTopResiduePolicyV1(
		directory, authority, privateCASRecoveryWitnessFileV1,
		func(body []byte) error {
			record, err := domainprivatecas.ParseRecoveryJournalCommitWitnessV1(body)
			if err != nil {
				return err
			}
			return journal.verifyWitnessLocked(authority, record)
		},
	)
	policies[privateCASRecoveryCompletionFileV1] = privateCASRecoverySignedTopResiduePolicyV1(
		directory, authority, privateCASRecoveryCompletionFileV1,
		func(body []byte) error {
			record, err := domainprivatecas.ParseRecoveryJournalCompletionReceiptV1(body)
			if err != nil {
				return err
			}
			return journal.verifyCompletionLocked(authority, record)
		},
	)
	policies[privateCASRecoveryRetirementFileV1] = privateCASRecoverySignedTopResiduePolicyV1(
		directory, authority, privateCASRecoveryRetirementFileV1,
		func(body []byte) error {
			_, err := parsePrivateCASRecoveryRetirementV1(body, authority)
			return err
		},
	)
	return startupExclusiveWriteResidueInventoryPolicy{
		EntryLimit: maxPrivateCASRecoveryTopEntriesV1 + len(policies), Targets: policies,
		AllowCommitted: func(name string, isDirectory bool) bool {
			switch name {
			case privateCASRecoveryTargetsDirectoryV1:
				return isDirectory
			case privateCASRecoveryPreparationFileV1, privateCASRecoveryManifestFileV1,
				privateCASRecoveryWitnessFileV1, privateCASRecoveryCompletionFileV1,
				privateCASRecoveryRetirementFileV1:
				return !isDirectory
			default:
				return false
			}
		},
	}
}

func privateCASRecoverySignedTopResiduePolicyV1(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	targetName string,
	validateRecord func([]byte) error,
) startupExclusiveWriteResidueTargetPolicy {
	return startupExclusiveWriteResidueTargetPolicy{
		MaxBytes: func() int64 {
			if targetName == privateCASRecoveryRetirementFileV1 {
				return maxPrivateCASRecoveryRetirementBytesV1
			}
			return domainprivatecas.MaxRecoveryJournalRecordBytesV1
		}(),
		Validate: func(tempName string, body []byte) error {
			if err := validatePrivateCASRecoveryAuthenticatedTempV1(directory, authority, targetName, tempName, body); err != nil {
				return err
			}
			return validateRecord(body)
		},
	}
}

func privateCASRecoveryTargetResiduePolicyV1(
	targets *startupPrivateDirectory,
	authority *startupJournalAuthority,
) startupExclusiveWriteResidueInventoryPolicy {
	policies := make(map[string]startupExclusiveWriteResidueTargetPolicy, domainprivatecas.MaxRecoveryTargetChunksV1)
	for index := uint32(0); index < domainprivatecas.MaxRecoveryTargetChunksV1; index++ {
		targetName := privateCASRecoveryChunkNameV1(index)
		expectedIndex := index
		policies[targetName] = startupExclusiveWriteResidueTargetPolicy{
			MaxBytes: domainprivatecas.MaxRecoveryTargetChunkBytesV1,
			Validate: func(tempName string, body []byte) error {
				if err := validatePrivateCASRecoveryAuthenticatedTempV1(targets, authority, targetName, tempName, body); err != nil {
					return err
				}
				chunk, err := domainprivatecas.ParseRecoveryTargetChunkV1(body)
				if err != nil || chunk.ChunkIndex != expectedIndex {
					return errors.New("private CAS recovery target residue is invalid")
				}
				return nil
			},
		}
	}
	return startupExclusiveWriteResidueInventoryPolicy{
		EntryLimit: int(domainprivatecas.MaxRecoveryTargetChunksV1) * 2, Targets: policies,
		AllowCommitted: func(name string, isDirectory bool) bool {
			return !isDirectory && validPrivateCASRecoveryChunkNameV1(name)
		},
	}
}

func startupExclusiveWriteResidueNamesV1(plan *startupExclusiveWriteResidueCleanupPlan) map[string]struct{} {
	if plan == nil || len(plan.candidates) == 0 {
		return nil
	}
	result := make(map[string]struct{}, len(plan.candidates))
	for _, candidate := range plan.candidates {
		result[candidate.tempName] = struct{}{}
	}
	return result
}

func (journal *privateCASRecoveryJournalV1) validatePrivateCASRecoveryResidueFrontierLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	session domainprivatecas.RecoveryJournalSessionV1,
	committedRetirement *privateCASRecoveryRetirementV1,
	sessionErr error,
	topPlan *startupExclusiveWriteResidueCleanupPlan,
	targetPlan *startupExclusiveWriteResidueCleanupPlan,
	ignoredTop map[string]struct{},
	ignoredTargets map[string]struct{},
) error {
	candidate := startupExclusiveWriteResidueCandidate{}
	if len(topPlan.candidates) == 1 {
		candidate = topPlan.candidates[0]
	} else if len(targetPlan.candidates) == 1 {
		candidate = targetPlan.candidates[0]
	} else {
		return errors.New("private CAS recovery authenticated residue frontier is ambiguous")
	}
	if candidate.targetPresent {
		if sessionErr != nil {
			return errors.New("private CAS recovery committed target is invalid beside its authenticated residue")
		}
		if candidate.targetName == privateCASRecoveryRetirementFileV1 &&
			(committedRetirement == nil || journal.verifyPrivateCASRecoveryRetirementResidueLocked(
				ctx, directory, authority, session, *committedRetirement, ignoredTop, ignoredTargets,
			) != nil) {
			return errors.New("private CAS recovery committed retirement is invalid beside its authenticated residue")
		}
		return nil
	}
	if candidate.targetName == privateCASRecoveryPreparationFileV1 {
		if !errors.Is(sessionErr, errPrivateCASRecoveryBootstrapIncompleteV1) || len(targetPlan.inventory) != 0 {
			return errors.New("private CAS recovery preparation residue is outside the bootstrap frontier")
		}
		return nil
	}
	if sessionErr != nil {
		return errors.New("private CAS recovery authenticated residue has no valid committed frontier")
	}
	switch candidate.targetName {
	case privateCASRecoveryManifestFileV1:
		record, err := domainprivatecas.ParseRecoveryJournalManifestV1(candidate.body)
		if err != nil || session.State != domainprivatecas.RecoveryJournalSessionPreparedV1 ||
			domainprivatecas.ValidateRecoveryJournalManifestForPreparationV1(record, session.Preparation) != nil ||
			domainprivatecas.ValidateRecoveryJournalManifestForChunksV1(record, session.Chunks) != nil {
			return errors.New("private CAS recovery manifest residue is outside the append frontier")
		}
	case privateCASRecoveryWitnessFileV1:
		record, err := domainprivatecas.ParseRecoveryJournalCommitWitnessV1(candidate.body)
		if err != nil || session.State != domainprivatecas.RecoveryJournalSessionManifestedV1 || session.Manifest == nil ||
			domainprivatecas.ValidateRecoveryJournalCommitWitnessForManifestV1(record, *session.Manifest) != nil {
			return errors.New("private CAS recovery witness residue is outside the append frontier")
		}
	case privateCASRecoveryCompletionFileV1:
		record, err := domainprivatecas.ParseRecoveryJournalCompletionReceiptV1(candidate.body)
		if err != nil || session.State != domainprivatecas.RecoveryJournalSessionCommitWitnessedV1 || session.CommitWitness == nil ||
			domainprivatecas.ValidateRecoveryJournalCompletionForWitnessV1(record, *session.CommitWitness) != nil {
			return errors.New("private CAS recovery completion residue is outside the append frontier")
		}
	case privateCASRecoveryRetirementFileV1:
		record, err := parsePrivateCASRecoveryRetirementV1(candidate.body, authority)
		if err != nil || journal.verifyPrivateCASRecoveryRetirementResidueLocked(
			ctx, directory, authority, session, record, ignoredTop, ignoredTargets,
		) != nil {
			return errors.New("private CAS recovery retirement residue is outside the append frontier")
		}
	default:
		if !validPrivateCASRecoveryChunkNameV1(candidate.targetName) ||
			candidate.targetName != privateCASRecoveryChunkNameV1(uint32(len(session.Chunks))) ||
			session.State != domainprivatecas.RecoveryJournalSessionPreparedV1 {
			return errors.New("private CAS recovery target residue is outside the append frontier")
		}
		chunk, err := domainprivatecas.ParseRecoveryTargetChunkV1(candidate.body)
		if err != nil || chunk.ChunkIndex != uint32(len(session.Chunks)) {
			return errors.New("private CAS recovery target residue is invalid")
		}
		chunks := append(append([]domainprivatecas.RecoveryTargetChunkV1(nil), session.Chunks...), chunk)
		if _, _, err := domainprivatecas.RecoveryTargetSetRootV1(chunks, session.Preparation.PlanCount); err != nil {
			return errors.New("private CAS recovery target residue does not match the active preparation")
		}
	}
	return nil
}

func (journal *privateCASRecoveryJournalV1) readSessionFromDirectoryLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
) (domainprivatecas.RecoveryJournalSessionV1, *privateCASRecoveryRetirementV1, error) {
	return journal.readSessionFromDirectoryIgnoringResiduesLocked(ctx, directory, authority, nil, nil)
}

func (journal *privateCASRecoveryJournalV1) readSessionFromDirectoryIgnoringResiduesLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	ignoredTop map[string]struct{},
	ignoredTargets map[string]struct{},
) (domainprivatecas.RecoveryJournalSessionV1, *privateCASRecoveryRetirementV1, error) {
	if err := contextError(ctx); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, err
	}
	entries, err := directory.ReadEntriesBoundedContext(ctx, maxPrivateCASRecoveryTopEntriesV1+len(ignoredTop))
	if err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, err
	}
	entryMap := make(map[string]os.DirEntry, len(entries))
	for _, entry := range entries {
		if _, ignored := ignoredTop[entry.Name()]; ignored {
			continue
		}
		switch entry.Name() {
		case privateCASRecoveryPreparationFileV1, privateCASRecoveryManifestFileV1,
			privateCASRecoveryWitnessFileV1, privateCASRecoveryCompletionFileV1,
			privateCASRecoveryRetirementFileV1:
			if entry.IsDir() {
				return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery record is not a regular file")
			}
		case privateCASRecoveryTargetsDirectoryV1:
			if !entry.IsDir() {
				return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery targets entry is not a directory")
			}
		default:
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery journal contains an unknown entry")
		}
		if _, duplicate := entryMap[entry.Name()]; duplicate {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery journal inventory is ambiguous")
		}
		entryMap[entry.Name()] = entry
	}
	if _, present := entryMap[privateCASRecoveryPreparationFileV1]; !present {
		if len(entryMap) == 0 || len(entryMap) == 1 && entryMap[privateCASRecoveryTargetsDirectoryV1] != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, errPrivateCASRecoveryBootstrapIncompleteV1
		}
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery journal has no preparation")
	}
	if _, present := entryMap[privateCASRecoveryTargetsDirectoryV1]; !present {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery journal has no targets directory")
	}
	preparationBody, _, err := directory.ReadFile(
		privateCASRecoveryPreparationFileV1, domainprivatecas.MaxRecoveryJournalRecordBytesV1, false,
	)
	if err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, err
	}
	preparation, err := domainprivatecas.ParseRecoveryJournalPreparationV1(preparationBody)
	if err != nil || journal.verifyPreparationLocked(authority, preparation) != nil ||
		preparation.RootBindingDigest != rootBindingDigest(journal.roots) ||
		preparation.JournalDirectoryIdentity != privateCASRecoveryDirectoryIdentityDigestV1(directory.Identity()) {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery preparation authority is invalid")
	}
	targets, err := openPrivateCASRecoveryChildDirectoryV1(
		directory, privateCASRecoveryTargetsDirectoryV1, preparation.TargetsDirectoryIdentity,
	)
	if err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, err
	}
	chunks, chunkErr := readPrivateCASRecoveryChunksIgnoringResiduesV1(ctx, targets, ignoredTargets)
	targetsCloseErr := targets.Close()
	if chunkErr != nil || targetsCloseErr != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.Join(chunkErr, targetsCloseErr)
	}
	session := domainprivatecas.RecoveryJournalSessionV1{
		State: domainprivatecas.RecoveryJournalSessionPreparedV1, Preparation: preparation, Chunks: chunks,
	}
	if _, present := entryMap[privateCASRecoveryManifestFileV1]; present {
		body, _, readErr := directory.ReadFile(
			privateCASRecoveryManifestFileV1, domainprivatecas.MaxRecoveryJournalRecordBytesV1, false,
		)
		if readErr != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, readErr
		}
		manifest, parseErr := domainprivatecas.ParseRecoveryJournalManifestV1(body)
		if parseErr != nil || journal.verifyManifestLocked(authority, manifest) != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery manifest authority is invalid")
		}
		session.Manifest = &manifest
		session.State = domainprivatecas.RecoveryJournalSessionManifestedV1
	}
	if _, present := entryMap[privateCASRecoveryWitnessFileV1]; present {
		body, _, readErr := directory.ReadFile(
			privateCASRecoveryWitnessFileV1, domainprivatecas.MaxRecoveryJournalRecordBytesV1, false,
		)
		if readErr != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, readErr
		}
		witness, parseErr := domainprivatecas.ParseRecoveryJournalCommitWitnessV1(body)
		if parseErr != nil || journal.verifyWitnessLocked(authority, witness) != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery commit witness authority is invalid")
		}
		session.CommitWitness = &witness
		session.State = domainprivatecas.RecoveryJournalSessionCommitWitnessedV1
	}
	if _, present := entryMap[privateCASRecoveryCompletionFileV1]; present {
		body, _, readErr := directory.ReadFile(
			privateCASRecoveryCompletionFileV1, domainprivatecas.MaxRecoveryJournalRecordBytesV1, false,
		)
		if readErr != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, readErr
		}
		receipt, parseErr := domainprivatecas.ParseRecoveryJournalCompletionReceiptV1(body)
		if parseErr != nil || journal.verifyCompletionLocked(authority, receipt) != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, errors.New("private CAS recovery completion authority is invalid")
		}
		session.CompletionReceipt = &receipt
		session.State = domainprivatecas.RecoveryJournalSessionCompletedV1
	}
	if err := domainprivatecas.ValidateRecoveryJournalSessionV1(session); err != nil {
		return domainprivatecas.RecoveryJournalSessionV1{}, nil, err
	}
	var retirement *privateCASRecoveryRetirementV1
	if _, present := entryMap[privateCASRecoveryRetirementFileV1]; present {
		body, _, readErr := directory.ReadFile(
			privateCASRecoveryRetirementFileV1, maxPrivateCASRecoveryRetirementBytesV1, false,
		)
		if readErr != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, readErr
		}
		record, parseErr := parsePrivateCASRecoveryRetirementV1(body, authority)
		if parseErr != nil {
			return domainprivatecas.RecoveryJournalSessionV1{}, nil, parseErr
		}
		retirement = &record
	}
	return session, retirement, nil
}

func readPrivateCASRecoveryChunksV1(
	ctx context.Context,
	targets *startupPrivateDirectory,
) ([]domainprivatecas.RecoveryTargetChunkV1, error) {
	return readPrivateCASRecoveryChunksIgnoringResiduesV1(ctx, targets, nil)
}

func readPrivateCASRecoveryChunksIgnoringResiduesV1(
	ctx context.Context,
	targets *startupPrivateDirectory,
	ignored map[string]struct{},
) ([]domainprivatecas.RecoveryTargetChunkV1, error) {
	entries, err := targets.ReadEntriesBoundedContext(ctx, domainprivatecas.MaxRecoveryTargetChunksV1+len(ignored))
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	chunks := make([]domainprivatecas.RecoveryTargetChunkV1, 0, len(entries))
	for _, entry := range entries {
		if _, skip := ignored[entry.Name()]; skip {
			continue
		}
		index := len(chunks)
		if entry.IsDir() || entry.Name() != privateCASRecoveryChunkNameV1(uint32(index)) {
			return nil, errors.New("private CAS recovery target chunk inventory is not canonical")
		}
		body, _, err := targets.ReadFile(entry.Name(), domainprivatecas.MaxRecoveryTargetChunkBytesV1, false)
		if err != nil {
			return nil, err
		}
		chunk, err := domainprivatecas.ParseRecoveryTargetChunkV1(body)
		if err != nil || chunk.ChunkIndex != uint32(index) {
			return nil, errors.New("private CAS recovery target chunk is invalid")
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

func putPrivateCASRecoverySignedRecordLockedV1[T any](
	journal *privateCASRecoveryJournalV1,
	ctx context.Context,
	name string,
	record T,
	encode func(T) ([]byte, error),
	signingBytes func(T) ([]byte, error),
	validateFrontier func(domainprivatecas.RecoveryJournalSessionV1) error,
	readback func(domainprivatecas.RecoveryJournalSessionV1) bool,
) error {
	session, err := journal.loadForMutationLocked(ctx)
	if err != nil {
		return err
	}
	if err := validateFrontier(session); err != nil {
		return err
	}
	if readback(session) {
		return nil
	}
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		return err
	}
	message, err := signingBytes(record)
	if err != nil {
		return err
	}
	keyID, signature, err := privateCASRecoveryRecordAuthorityV1(record)
	if err != nil || authority.verifyDomain(nil, keyID, message, signature) != nil {
		return errors.New("private CAS recovery record signature is invalid")
	}
	body, err := encode(record)
	if err != nil {
		return err
	}
	directory, err := openPrivateCASRecoveryDirectoryV1(
		journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, session.Preparation.JournalDirectoryIdentity,
	)
	if err != nil {
		return err
	}
	writeErr := writePrivateCASRecoveryRecordExactV1(
		directory, authority, name, body, domainprivatecas.MaxRecoveryJournalRecordBytesV1,
	)
	closeErr := directory.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	loaded, err := journal.loadActiveLocked(ctx, false)
	if err != nil || !readback(loaded) {
		return errors.Join(errors.New("private CAS recovery signed record readback failed"), err)
	}
	return nil
}

func privateCASRecoveryRecordAuthorityV1[T any](record T) (string, string, error) {
	switch value := any(record).(type) {
	case domainprivatecas.RecoveryJournalManifestV1:
		return value.AuthorityKeyID, value.AuthoritySignature, nil
	case domainprivatecas.RecoveryJournalCommitWitnessV1:
		return value.AuthorityKeyID, value.AuthoritySignature, nil
	case domainprivatecas.RecoveryJournalCompletionReceiptV1:
		return value.AuthorityKeyID, value.AuthoritySignature, nil
	default:
		return "", "", errors.New("private CAS recovery record type is unsupported")
	}
}

func (journal *privateCASRecoveryJournalV1) verifyPreparationLocked(
	authority *startupJournalAuthority,
	record domainprivatecas.RecoveryJournalPreparationV1,
) error {
	message, err := domainprivatecas.RecoveryJournalPreparationSigningBytesV1(record)
	if err != nil {
		return err
	}
	return authority.verifyDomain(nil, record.AuthorityKeyID, message, record.AuthoritySignature)
}

func (journal *privateCASRecoveryJournalV1) verifyManifestLocked(
	authority *startupJournalAuthority,
	record domainprivatecas.RecoveryJournalManifestV1,
) error {
	message, err := domainprivatecas.RecoveryJournalManifestSigningBytesV1(record)
	if err != nil {
		return err
	}
	return authority.verifyDomain(nil, record.AuthorityKeyID, message, record.AuthoritySignature)
}

func (journal *privateCASRecoveryJournalV1) verifyWitnessLocked(
	authority *startupJournalAuthority,
	record domainprivatecas.RecoveryJournalCommitWitnessV1,
) error {
	message, err := domainprivatecas.RecoveryJournalCommitWitnessSigningBytesV1(record)
	if err != nil {
		return err
	}
	return authority.verifyDomain(nil, record.AuthorityKeyID, message, record.AuthoritySignature)
}

func (journal *privateCASRecoveryJournalV1) verifyCompletionLocked(
	authority *startupJournalAuthority,
	record domainprivatecas.RecoveryJournalCompletionReceiptV1,
) error {
	message, err := domainprivatecas.RecoveryJournalCompletionSigningBytesV1(record)
	if err != nil {
		return err
	}
	return authority.verifyDomain(nil, record.AuthorityKeyID, message, record.AuthoritySignature)
}

func (journal *privateCASRecoveryJournalV1) openActiveDirectoriesLocked(
	preparation domainprivatecas.RecoveryJournalPreparationV1,
) (*startupPrivateDirectory, *startupPrivateDirectory, error) {
	directory, err := openPrivateCASRecoveryDirectoryV1(
		journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, preparation.JournalDirectoryIdentity,
	)
	if err != nil {
		return nil, nil, err
	}
	targets, err := openPrivateCASRecoveryChildDirectoryV1(
		directory, privateCASRecoveryTargetsDirectoryV1, preparation.TargetsDirectoryIdentity,
	)
	if err != nil {
		_ = directory.Close()
		return nil, nil, err
	}
	return directory, targets, nil
}

func (journal *privateCASRecoveryJournalV1) newRetirementLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	session domainprivatecas.RecoveryJournalSessionV1,
) (*privateCASRecoveryRetirementV1, error) {
	if session.Manifest == nil || session.CompletionReceipt == nil {
		return nil, errors.New("private CAS recovery completion is unavailable for retirement")
	}
	inventoryDigest, inventoryCount, err := journal.retirementInventoryLocked(ctx, directory, session.Preparation)
	if err != nil {
		return nil, err
	}
	record := privateCASRecoveryRetirementV1{
		SchemaVersion: privateCASRecoveryRetirementSchemaVersionV1, Purpose: privateCASRecoveryRetirementPurposeV1,
		TransactionID: session.Manifest.TransactionID, CompletionReceiptDigest: session.CompletionReceipt.ReceiptDigest,
		RootBindingDigest:        rootBindingDigest(journal.roots),
		JournalDirectoryIdentity: privateCASRecoveryDirectoryIdentityDigestV1(directory.Identity()),
		TargetsDirectoryIdentity: session.Preparation.TargetsDirectoryIdentity,
		InventoryDigest:          inventoryDigest, InventoryEntryCount: inventoryCount, AuthorityKeyID: authority.keyID,
	}
	message := privateCASRecoveryRetirementSigningBytesV1(record)
	signature, err := authority.signDomain(nil, message)
	if err != nil {
		return nil, err
	}
	record.AuthoritySignature = signature
	record.RetirementDigest = privateCASRecoveryRetirementDigestV1(record)
	if err := validatePrivateCASRecoveryRetirementV1(record, authority); err != nil {
		return nil, err
	}
	return &record, nil
}

func (journal *privateCASRecoveryJournalV1) verifyRetirementForDirectoryLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	session domainprivatecas.RecoveryJournalSessionV1,
	record privateCASRecoveryRetirementV1,
) error {
	if session.State != domainprivatecas.RecoveryJournalSessionCompletedV1 || session.Manifest == nil || session.CompletionReceipt == nil ||
		record.TransactionID != session.Manifest.TransactionID ||
		record.CompletionReceiptDigest != session.CompletionReceipt.ReceiptDigest ||
		record.RootBindingDigest != rootBindingDigest(journal.roots) ||
		record.JournalDirectoryIdentity != privateCASRecoveryDirectoryIdentityDigestV1(directory.Identity()) ||
		record.TargetsDirectoryIdentity != session.Preparation.TargetsDirectoryIdentity ||
		validatePrivateCASRecoveryRetirementV1(record, authority) != nil {
		return errors.New("private CAS recovery retirement authority is invalid")
	}
	digest, count, err := journal.retirementInventoryLocked(ctx, directory, session.Preparation)
	if err != nil || digest != record.InventoryDigest || count != record.InventoryEntryCount {
		return errors.Join(errors.New("private CAS recovery retirement inventory changed"), err)
	}
	return directory.PreflightTreeContext(ctx)
}

func (journal *privateCASRecoveryJournalV1) retirementInventoryLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	preparation domainprivatecas.RecoveryJournalPreparationV1,
) (string, uint32, error) {
	return journal.retirementInventoryIgnoringResiduesLocked(ctx, directory, preparation, nil, nil)
}

func (journal *privateCASRecoveryJournalV1) retirementInventoryIgnoringResiduesLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	preparation domainprivatecas.RecoveryJournalPreparationV1,
	ignoredTop map[string]struct{},
	ignoredTargets map[string]struct{},
) (string, uint32, error) {
	entries, err := directory.ReadEntriesBoundedContext(ctx, maxPrivateCASRecoveryTopEntriesV1+len(ignoredTop))
	if err != nil {
		return "", 0, err
	}
	inventory := make([]privateCASRecoveryInventoryEntryV1, 0, len(entries)+domainprivatecas.MaxRecoveryTargetChunksV1)
	for _, entry := range entries {
		if _, ignored := ignoredTop[entry.Name()]; ignored {
			continue
		}
		if entry.Name() == privateCASRecoveryRetirementFileV1 {
			continue
		}
		if entry.Name() == privateCASRecoveryTargetsDirectoryV1 {
			if !entry.IsDir() {
				return "", 0, errors.New("private CAS recovery retirement targets are unsafe")
			}
			inventory = append(inventory, privateCASRecoveryInventoryEntryV1{
				Name: entry.Name(), Kind: "directory", Identity: preparation.TargetsDirectoryIdentity,
			})
			continue
		}
		limit, ok := privateCASRecoveryRecordLimitV1(entry.Name())
		if !ok || entry.IsDir() {
			return "", 0, errors.New("private CAS recovery retirement top-level inventory is unsafe")
		}
		body, identity, readErr := directory.ReadFile(entry.Name(), limit, false)
		if readErr != nil {
			return "", 0, readErr
		}
		inventory = append(inventory, privateCASRecoveryInventoryEntryV1{
			Name: entry.Name(), Kind: "file", Identity: identity,
			SHA256: privateCASRecoverySHA256V1(body), Bytes: uint64(len(body)),
		})
	}
	targets, err := openPrivateCASRecoveryChildDirectoryV1(
		directory, privateCASRecoveryTargetsDirectoryV1, preparation.TargetsDirectoryIdentity,
	)
	if err != nil {
		return "", 0, err
	}
	targetEntries, err := targets.ReadEntriesBoundedContext(
		ctx, domainprivatecas.MaxRecoveryTargetChunksV1+len(ignoredTargets),
	)
	if err == nil {
		for _, entry := range targetEntries {
			if _, ignored := ignoredTargets[entry.Name()]; ignored {
				continue
			}
			if entry.IsDir() || !validPrivateCASRecoveryChunkNameV1(entry.Name()) {
				err = errors.New("private CAS recovery retirement target inventory is unsafe")
				break
			}
			body, identity, readErr := targets.ReadFile(entry.Name(), domainprivatecas.MaxRecoveryTargetChunkBytesV1, false)
			if readErr != nil {
				err = readErr
				break
			}
			inventory = append(inventory, privateCASRecoveryInventoryEntryV1{
				Name: privateCASRecoveryTargetsDirectoryV1 + "/" + entry.Name(), Kind: "file", Identity: identity,
				SHA256: privateCASRecoverySHA256V1(body), Bytes: uint64(len(body)),
			})
		}
	}
	closeErr := targets.Close()
	if err != nil || closeErr != nil {
		return "", 0, errors.Join(err, closeErr)
	}
	sort.Slice(inventory, func(left, right int) bool { return inventory[left].Name < inventory[right].Name })
	body, err := json.Marshal(inventory)
	if err != nil {
		return "", 0, err
	}
	return privateCASRecoveryDomainDigestV1(privateCASRecoveryInventoryDigestDomainV1, body), uint32(len(inventory)), nil
}

func (journal *privateCASRecoveryJournalV1) verifyPrivateCASRecoveryRetirementResidueLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	session domainprivatecas.RecoveryJournalSessionV1,
	record privateCASRecoveryRetirementV1,
	ignoredTop map[string]struct{},
	ignoredTargets map[string]struct{},
) error {
	if session.State != domainprivatecas.RecoveryJournalSessionCompletedV1 || session.Manifest == nil ||
		session.CompletionReceipt == nil || record.TransactionID != session.Manifest.TransactionID ||
		record.CompletionReceiptDigest != session.CompletionReceipt.ReceiptDigest ||
		record.RootBindingDigest != rootBindingDigest(journal.roots) ||
		record.JournalDirectoryIdentity != privateCASRecoveryDirectoryIdentityDigestV1(directory.Identity()) ||
		record.TargetsDirectoryIdentity != session.Preparation.TargetsDirectoryIdentity ||
		validatePrivateCASRecoveryRetirementV1(record, authority) != nil {
		return errors.New("private CAS recovery retirement residue authority is invalid")
	}
	digest, count, err := journal.retirementInventoryIgnoringResiduesLocked(
		ctx, directory, session.Preparation, ignoredTop, ignoredTargets,
	)
	if err != nil || digest != record.InventoryDigest || count != record.InventoryEntryCount {
		return errors.Join(errors.New("private CAS recovery retirement residue inventory changed"), err)
	}
	return nil
}

func (journal *privateCASRecoveryJournalV1) finishActiveRetirementLocked(
	ctx context.Context,
	authority *startupJournalAuthority,
	directoryIdentity string,
) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	retiredName, retired, err := secureStartupRetireDirectory(
		journal.journalAuthority.root, privateCASRecoveryJournalDirectoryV1, directoryIdentity, privateCASRecoveryRetiredPrefixV1,
	)
	if err != nil || retired == nil {
		return errors.Join(errors.New("private CAS recovery journal retirement rename failed"), err)
	}
	defer retired.Close()
	if err := journal.verifyRetiredDirectoryLocked(ctx, retired, authority); err != nil {
		return err
	}
	return secureStartupRemoveRetiredDirectoryContext(ctx, journal.journalAuthority.root, retiredName, retired)
}

func (journal *privateCASRecoveryJournalV1) recoverRetiredLocked(ctx context.Context) error {
	root, err := secureStartupOpenRootDirectory(journal.journalAuthority.root)
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := root.ReadEntriesBoundedContext(ctx, maxSemanticNamespaceEntries)
	if err != nil {
		return err
	}
	retiredNames := make([]string, 0, 1)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), privateCASRecoveryRetiredPrefixV1) {
			continue
		}
		suffix := strings.TrimPrefix(entry.Name(), privateCASRecoveryRetiredPrefixV1)
		if !entry.IsDir() || len(suffix) != 32 || !isLowerHex(suffix) {
			return errors.New("private CAS retired recovery journal residue is unsafe")
		}
		retiredNames = append(retiredNames, entry.Name())
	}
	if len(retiredNames) == 0 {
		return nil
	}
	if len(retiredNames) != 1 {
		return errors.New("private CAS retired recovery journal inventory is ambiguous")
	}
	authority, err := journal.loadAuthorityLocked(ctx, false)
	if err != nil {
		return errors.New("private CAS retired recovery journal signing authority is unavailable")
	}
	retired, err := root.OpenDirectory(retiredNames[0], "")
	if err != nil {
		return err
	}
	identity := retired.Identity()
	if err := journal.verifyRetiredDirectoryLocked(ctx, retired, authority); err != nil {
		_ = retired.Close()
		return err
	}
	if err := retired.Close(); err != nil {
		return err
	}
	retired, err = root.OpenDirectory(retiredNames[0], identity)
	if err != nil {
		return err
	}
	if err := journal.verifyRetiredDirectoryLocked(ctx, retired, authority); err != nil {
		_ = retired.Close()
		return err
	}
	if err := root.RemoveRetiredDirectoryContext(ctx, retiredNames[0], retired); err != nil {
		_ = retired.Close()
		return err
	}
	return retired.Close()
}

func (journal *privateCASRecoveryJournalV1) verifyRetiredDirectoryLocked(
	ctx context.Context,
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
) error {
	session, retirement, err := journal.readSessionFromDirectoryLocked(ctx, directory, authority)
	if err != nil || retirement == nil {
		return errors.Join(errors.New("private CAS retired recovery journal has no valid signed retirement"), err)
	}
	return journal.verifyRetirementForDirectoryLocked(ctx, directory, authority, session, *retirement)
}

func privateCASRecoveryRetirementBytesV1(record privateCASRecoveryRetirementV1) ([]byte, error) {
	if record.RetirementDigest == "" {
		return nil, errors.New("private CAS recovery retirement is unsealed")
	}
	body, err := json.Marshal(record)
	if err != nil || len(body) > maxPrivateCASRecoveryRetirementBytesV1 {
		return nil, errors.New("private CAS recovery retirement exceeds its bound")
	}
	return body, nil
}

func parsePrivateCASRecoveryRetirementV1(
	body []byte,
	authority *startupJournalAuthority,
) (privateCASRecoveryRetirementV1, error) {
	if len(body) == 0 || len(body) > maxPrivateCASRecoveryRetirementBytesV1 ||
		validateSemanticControlJSON(body, maxPrivateCASRecoveryRetirementBytesV1) != nil {
		return privateCASRecoveryRetirementV1{}, errors.New("private CAS recovery retirement JSON is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var record privateCASRecoveryRetirementV1
	if err := decoder.Decode(&record); err != nil {
		return privateCASRecoveryRetirementV1{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return privateCASRecoveryRetirementV1{}, errors.New("private CAS recovery retirement contains trailing JSON")
	}
	canonical, err := json.Marshal(record)
	if err != nil || !bytes.Equal(canonical, body) || validatePrivateCASRecoveryRetirementV1(record, authority) != nil {
		return privateCASRecoveryRetirementV1{}, errors.New("private CAS recovery retirement is invalid")
	}
	return record, nil
}

func validatePrivateCASRecoveryRetirementV1(
	record privateCASRecoveryRetirementV1,
	authority *startupJournalAuthority,
) error {
	if record.SchemaVersion != privateCASRecoveryRetirementSchemaVersionV1 ||
		record.Purpose != privateCASRecoveryRetirementPurposeV1 ||
		!domainprivatecas.ValidDigestV1(record.TransactionID) ||
		!domainprivatecas.ValidDigestV1(record.CompletionReceiptDigest) ||
		!domainprivatecas.ValidDigestV1(record.RootBindingDigest) ||
		!domainprivatecas.ValidDigestV1(record.JournalDirectoryIdentity) ||
		!domainprivatecas.ValidDigestV1(record.TargetsDirectoryIdentity) ||
		!domainprivatecas.ValidDigestV1(record.InventoryDigest) || record.InventoryEntryCount == 0 ||
		!domainprivatecas.ValidDigestV1(record.AuthorityKeyID) ||
		!domainprivatecas.ValidDigestV1(record.RetirementDigest) ||
		privateCASRecoveryRetirementDigestV1(record) != record.RetirementDigest || authority == nil ||
		authority.verifyDomain(
			nil, record.AuthorityKeyID, privateCASRecoveryRetirementSigningBytesV1(record), record.AuthoritySignature,
		) != nil {
		return errors.New("private CAS recovery retirement authority is invalid")
	}
	return nil
}

func privateCASRecoveryRetirementSigningBytesV1(record privateCASRecoveryRetirementV1) []byte {
	record.AuthoritySignature = ""
	record.RetirementDigest = ""
	body, _ := json.Marshal(record)
	digest := sha256.Sum256(body)
	message := append([]byte(nil), privateCASRecoveryRetirementSignDomainV1...)
	return append(message, digest[:]...)
}

func privateCASRecoveryRetirementDigestV1(record privateCASRecoveryRetirementV1) string {
	record.RetirementDigest = ""
	body, _ := json.Marshal(record)
	return privateCASRecoveryDomainDigestV1(privateCASRecoveryRetirementDigestDomainV1, body)
}

func privateCASRecoveryDomainDigestV1(domain, body []byte) string {
	digest := sha256.New()
	_, _ = digest.Write(domain)
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

// Recovery records bind a platform-neutral digest of the handle-derived
// strong identity. The raw Unix or Windows identity is never persisted as the
// domain value and is never confused with an expected digest when reopening.
func privateCASRecoveryDirectoryIdentityDigestV1(identity string) string {
	if _, err := parseStrongDirectoryIdentity(identity); err != nil {
		return ""
	}
	return privateCASRecoveryDomainDigestV1(privateCASRecoveryDirectoryIdentityDomainV1, []byte(identity))
}

func openPrivateCASRecoveryDirectoryV1(
	root startupAuthorityRoot,
	name string,
	expectedIdentityDigest string,
) (*startupPrivateDirectory, error) {
	directory, err := secureStartupOpenDirectory(root, name, "")
	if err != nil {
		return nil, err
	}
	if !domainprivatecas.ValidDigestV1(expectedIdentityDigest) ||
		privateCASRecoveryDirectoryIdentityDigestV1(directory.Identity()) != expectedIdentityDigest {
		_ = directory.Close()
		return nil, errors.New("private CAS recovery directory identity changed")
	}
	return directory, nil
}

func openPrivateCASRecoveryChildDirectoryV1(
	parent *startupPrivateDirectory,
	name string,
	expectedIdentityDigest string,
) (*startupPrivateDirectory, error) {
	directory, err := parent.OpenDirectory(name, "")
	if err != nil {
		return nil, err
	}
	if !domainprivatecas.ValidDigestV1(expectedIdentityDigest) ||
		privateCASRecoveryDirectoryIdentityDigestV1(directory.Identity()) != expectedIdentityDigest {
		_ = directory.Close()
		return nil, errors.New("private CAS recovery child directory identity changed")
	}
	return directory, nil
}

func privateCASRecoverySHA256V1(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func privateCASRecoveryChunkNameV1(index uint32) string {
	return fmt.Sprintf("chunk-%03d.json", index)
}

func validPrivateCASRecoveryChunkNameV1(name string) bool {
	for index := uint32(0); index < domainprivatecas.MaxRecoveryTargetChunksV1; index++ {
		if name == privateCASRecoveryChunkNameV1(index) {
			return true
		}
	}
	return false
}

func privateCASRecoveryRecordLimitV1(name string) (int64, bool) {
	switch name {
	case privateCASRecoveryPreparationFileV1, privateCASRecoveryManifestFileV1,
		privateCASRecoveryWitnessFileV1, privateCASRecoveryCompletionFileV1:
		return domainprivatecas.MaxRecoveryJournalRecordBytesV1, true
	default:
		return 0, false
	}
}

func writePrivateCASRecoveryRecordExactV1(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	name string,
	body []byte,
	maxBytes int64,
) error {
	temporary, err := newPrivateCASRecoveryAuthenticatedTempNameV1(directory, authority, name, body)
	if err != nil {
		return err
	}
	writeErr := directory.writeExclusiveWithTemporaryName(name, temporary, body, maxBytes)
	if writeErr == nil {
		return nil
	}
	existing, _, readErr := directory.ReadFile(name, maxBytes, false)
	if readErr == nil && bytes.Equal(existing, body) {
		return nil
	}
	if errors.Is(writeErr, os.ErrExist) || readErr == nil {
		return errors.New("private CAS recovery append-only record conflicts with existing bytes")
	}
	return writeErr
}

func newPrivateCASRecoveryAuthenticatedTempNameV1(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	targetName string,
	body []byte,
) (string, error) {
	if directory == nil || authority == nil || !startupAuthorityNamedComponent(targetName) || len(body) == 0 {
		return "", errors.New("private CAS recovery authenticated temp input is invalid")
	}
	nonce, err := startupPrivateRandomName("")
	if err != nil {
		return "", err
	}
	payload, err := privateCASRecoveryTempAuthorityBytesV1(directory.Identity(), targetName, body, nonce)
	if err != nil {
		return "", err
	}
	signature, err := authority.signDomain(privateCASRecoveryTempSignatureDomainV1, payload)
	if err != nil {
		return "", err
	}
	name := "." + targetName + "-" + nonce + "-" + signature + ".tmp"
	if !startupAuthorityNamedComponent(name) {
		return "", errors.New("private CAS recovery authenticated temp name is invalid")
	}
	return name, nil
}

func validatePrivateCASRecoveryAuthenticatedTempV1(
	directory *startupPrivateDirectory,
	authority *startupJournalAuthority,
	targetName string,
	tempName string,
	body []byte,
) error {
	if directory == nil || authority == nil || !startupAuthorityNamedComponent(targetName) ||
		!startupAuthorityNamedComponent(tempName) || len(body) == 0 {
		return errors.New("private CAS recovery authenticated temp is invalid")
	}
	prefix := "." + targetName + "-"
	if !strings.HasPrefix(tempName, prefix) || !strings.HasSuffix(tempName, ".tmp") {
		return errors.New("private CAS recovery authenticated temp name is invalid")
	}
	remainder := strings.TrimSuffix(strings.TrimPrefix(tempName, prefix), ".tmp")
	if len(remainder) <= 33 || remainder[32] != '-' {
		return errors.New("private CAS recovery authenticated temp name is invalid")
	}
	nonce := remainder[:32]
	signature := remainder[33:]
	decoded, decodeErr := base64.RawURLEncoding.DecodeString(signature)
	if len(nonce) != 32 || !isLowerHex(nonce) || decodeErr != nil || len(decoded) != ed25519.SignatureSize ||
		base64.RawURLEncoding.EncodeToString(decoded) != signature ||
		tempName != prefix+nonce+"-"+signature+".tmp" {
		return errors.New("private CAS recovery authenticated temp name is invalid")
	}
	payload, err := privateCASRecoveryTempAuthorityBytesV1(directory.Identity(), targetName, body, nonce)
	if err != nil || authority.verifyDomain(
		privateCASRecoveryTempSignatureDomainV1, authority.keyID, payload, signature,
	) != nil {
		return errors.New("private CAS recovery authenticated temp signature is invalid")
	}
	return nil
}

func privateCASRecoveryTempAuthorityBytesV1(
	directoryIdentity string,
	targetName string,
	body []byte,
	nonce string,
) ([]byte, error) {
	directoryDigest := privateCASRecoveryDirectoryIdentityDigestV1(directoryIdentity)
	if !domainprivatecas.ValidDigestV1(directoryDigest) || !startupAuthorityNamedComponent(targetName) ||
		len(body) == 0 || len(nonce) != 32 || !isLowerHex(nonce) {
		return nil, errors.New("private CAS recovery authenticated temp payload is invalid")
	}
	payload := privateCASRecoveryTempAuthorityV1{
		SchemaVersion: privateCASRecoveryTempSchemaVersionV1, Purpose: privateCASRecoveryTempPurposeV1,
		DirectoryIdentity: directoryDigest, TargetName: targetName,
		BodySHA256: privateCASRecoverySHA256V1(body), ByteLength: uint64(len(body)), Nonce: nonce,
	}
	return json.Marshal(payload)
}

func exactPrivateCASRecoveryRecordV1[T any](
	left T,
	right T,
	encode func(T) ([]byte, error),
	message string,
) error {
	leftBody, leftErr := encode(left)
	rightBody, rightErr := encode(right)
	if leftErr != nil || rightErr != nil || !bytes.Equal(leftBody, rightBody) {
		return errors.New(message)
	}
	return nil
}
