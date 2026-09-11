package filestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	turnapp "analytix.local/runtime-go/internal/app/turn"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	attachmentauthorityport "analytix.local/runtime-go/internal/ports/attachmentauthority"
)

type AttachmentQuarantineObservationV1 struct {
	AttachmentID   string
	MetadataSHA256 string
	ContentSHA256  string
	ReasonCode     string
}

type AttachmentUploadRecoveryActionV1 struct {
	Intent      domainattachment.UploadIntentV1
	Disposition domainattachment.UploadDispositionV1
	CommitOwner bool
}

type AttachmentAuthorityReconciliationPlanV1 struct {
	Quarantined            []AttachmentQuarantineObservationV1
	Actions                []AttachmentUploadRecoveryActionV1
	PreservedAttachmentIDs []string
}

type AttachmentUploadCurrentValidator func(domainattachment.OwnerRecordV1) error

// PreflightAuthorityReconciliation validates the complete owner/intent/
// disposition/file inventory before any recovery mutation. Only a private
// intent with exact durable files and a still-current binding may plan owner
// recovery. Unattributed, partial, corrupt, or stale state remains
// non-executable quarantine. This pass is read-only and does not create a
// missing attachment root.
func (s *PersistentAttachmentStore) PreflightAuthorityReconciliation(
	ctx context.Context,
	authority attachmentauthorityport.UploadInventoryStore,
	validateCurrent AttachmentUploadCurrentValidator,
	observedAt time.Time,
) (result AttachmentAuthorityReconciliationPlanV1, resultErr error) {
	if ctx == nil || s == nil || authority == nil || validateCurrent == nil {
		return AttachmentAuthorityReconciliationPlanV1{}, errors.New("attachment reconciliation authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return AttachmentAuthorityReconciliationPlanV1{}, err
	}
	preserved, _ := authority.(attachmentauthorityport.RestartPreservationV1)
	if preserved != nil {
		if err := preserved.RevalidateAttachmentRestartV1(ctx); err != nil {
			return AttachmentAuthorityReconciliationPlanV1{}, err
		}
		defer func() {
			resultErr = errors.Join(resultErr, preserved.RevalidateAttachmentRestartV1(ctx))
			if resultErr != nil {
				result = AttachmentAuthorityReconciliationPlanV1{}
			}
		}()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	owners := map[string]domainattachment.OwnerRecordV1{}
	if err := authority.VisitOwners(ctx, func(owner domainattachment.OwnerRecordV1) error {
		if existing, ok := owners[owner.AttachmentID]; ok && !sameAttachmentRecoveryOwner(existing, owner) {
			return errors.New("attachment authority contains conflicting owners")
		}
		owners[owner.AttachmentID] = owner
		return nil
	}); err != nil {
		return AttachmentAuthorityReconciliationPlanV1{}, err
	}
	intents := map[string]domainattachment.UploadIntentV1{}
	if err := authority.VisitUploadIntents(ctx, func(intent domainattachment.UploadIntentV1) error {
		if existing, ok := intents[intent.Owner.AttachmentID]; ok && !sameAttachmentRecoveryIntent(existing, intent) {
			return errors.New("attachment authority contains conflicting upload intents")
		}
		intents[intent.Owner.AttachmentID] = intent
		return nil
	}); err != nil {
		return AttachmentAuthorityReconciliationPlanV1{}, err
	}
	dispositions := map[string]domainattachment.UploadDispositionV1{}
	if err := authority.VisitUploadDispositions(ctx, func(disposition domainattachment.UploadDispositionV1) error {
		intent, ok := intents[disposition.AttachmentID]
		if !ok || domainattachment.ValidateUploadDispositionForIntentV1(disposition, intent) != nil {
			return errors.New("attachment authority contains an upload disposition without its exact intent")
		}
		if existing, ok := dispositions[disposition.AttachmentID]; ok && !sameAttachmentRecoveryDisposition(existing, disposition) {
			return errors.New("attachment authority contains conflicting upload dispositions")
		}
		dispositions[disposition.AttachmentID] = disposition
		return nil
	}); err != nil {
		return AttachmentAuthorityReconciliationPlanV1{}, err
	}
	metadataFiles, err := attachmentRecoveryFiles(ctx, s.metadataDir, ".json")
	if err != nil {
		return AttachmentAuthorityReconciliationPlanV1{}, err
	}
	contentFiles, err := attachmentRecoveryFiles(ctx, s.contentDir, ".bin")
	if err != nil {
		return AttachmentAuthorityReconciliationPlanV1{}, err
	}
	ids := map[string]bool{}
	for id := range owners {
		ids[id] = true
	}
	for id := range intents {
		ids[id] = true
	}
	for id := range metadataFiles {
		ids[id] = true
	}
	for id := range contentFiles {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	plan := AttachmentAuthorityReconciliationPlanV1{
		Quarantined: []AttachmentQuarantineObservationV1{}, Actions: []AttachmentUploadRecoveryActionV1{},
	}
	observedAt = observedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	for _, id := range ordered {
		if err := ctx.Err(); err != nil {
			return AttachmentAuthorityReconciliationPlanV1{}, err
		}
		owner, authorized := owners[id]
		intent, hasIntent := intents[id]
		disposition, hasDisposition := dispositions[id]
		metadataPath, hasMetadata := metadataFiles[id]
		contentPath, hasContent := contentFiles[id]
		held := preserved != nil && ((authorized && preserved.RestartPreservesThreadV1(owner.ThreadID)) || (hasIntent && preserved.RestartPreservesThreadV1(intent.Owner.ThreadID)))
		if held {
			plan.PreservedAttachmentIDs = append(plan.PreservedAttachmentIDs, id)
		}
		if authorized {
			if !hasMetadata || !hasContent {
				return AttachmentAuthorityReconciliationPlanV1{}, fmt.Errorf("authorized attachment %s is incomplete", id)
			}
			metadata, err := ReadJSONMapFile(metadataPath)
			if err != nil {
				return AttachmentAuthorityReconciliationPlanV1{}, err
			}
			storedOwner, err := validateAttachmentMetadataIntegrity(id, metadata)
			if err != nil || !sameAttachmentRecoveryOwner(storedOwner, owner) {
				return AttachmentAuthorityReconciliationPlanV1{}, errors.Join(fmt.Errorf("authorized attachment %s metadata is inconsistent", id), err)
			}
			if err := validateAttachmentRecoveryContent(ctx, contentPath, owner); err != nil {
				return AttachmentAuthorityReconciliationPlanV1{}, fmt.Errorf("authorized attachment %s content is inconsistent: %w", id, err)
			}
			if hasIntent {
				if !sameAttachmentRecoveryOwner(intent.Owner, owner) {
					return AttachmentAuthorityReconciliationPlanV1{}, fmt.Errorf("authorized attachment %s upload intent is inconsistent", id)
				}
				metadataDigest, digestErr := attachmentRecoveryFileDigest(ctx, metadataPath)
				if digestErr != nil || metadataDigest != intent.MetadataSHA256 {
					return AttachmentAuthorityReconciliationPlanV1{}, errors.Join(fmt.Errorf("authorized attachment %s metadata digest is inconsistent", id), digestErr)
				}
				if hasDisposition {
					if disposition.Status != domainattachment.UploadDispositionCommittedV1 {
						return AttachmentAuthorityReconciliationPlanV1{}, fmt.Errorf("authorized attachment %s has a non-committed upload disposition", id)
					}
				} else if !held {
					committed, dispositionErr := domainattachment.NewUploadDispositionV1(
						intent, domainattachment.UploadDispositionCommittedV1, "restart_completed_exact_upload",
						metadataDigest, owner.BlobSHA256, observedAt,
					)
					if dispositionErr != nil {
						return AttachmentAuthorityReconciliationPlanV1{}, dispositionErr
					}
					plan.Actions = append(plan.Actions, AttachmentUploadRecoveryActionV1{Intent: intent, Disposition: committed})
				}
			}
			continue
		}
		if hasDisposition && disposition.Status == domainattachment.UploadDispositionCommittedV1 {
			return AttachmentAuthorityReconciliationPlanV1{}, fmt.Errorf("committed attachment upload %s is missing owner authority", id)
		}
		if hasIntent {
			metadataDigest, contentDigest := "", ""
			if hasMetadata {
				metadataDigest, err = attachmentRecoveryFileDigest(ctx, metadataPath)
				if err != nil {
					return AttachmentAuthorityReconciliationPlanV1{}, err
				}
			}
			if hasContent {
				contentDigest, err = attachmentRecoveryFileDigest(ctx, contentPath)
				if err != nil {
					return AttachmentAuthorityReconciliationPlanV1{}, err
				}
			}
			if hasDisposition {
				if held {
					continue
				}
				plan.Quarantined = append(plan.Quarantined, AttachmentQuarantineObservationV1{
					AttachmentID: id, MetadataSHA256: metadataDigest, ContentSHA256: contentDigest, ReasonCode: disposition.ReasonCode,
				})
				continue
			}
			var metadataErr, contentErr error
			if hasMetadata {
				metadataErr = validateAttachmentRecoveryMetadata(id, metadataPath, intent.Owner)
			}
			if hasContent {
				contentErr = validateAttachmentRecoveryContent(ctx, contentPath, intent.Owner)
			}
			if metadataErr != nil && !errors.Is(metadataErr, ErrAttachmentContentIntegrity) || contentErr != nil && !errors.Is(contentErr, ErrAttachmentContentIntegrity) {
				return AttachmentAuthorityReconciliationPlanV1{}, errors.Join(metadataErr, contentErr)
			}
			metadataExact := hasMetadata && metadataDigest == intent.MetadataSHA256 && metadataErr == nil
			contentExact := hasContent && contentDigest == intent.Owner.BlobSHA256 && contentErr == nil
			if held {
				if (hasMetadata && !metadataExact) || (hasContent && !contentExact) {
					return AttachmentAuthorityReconciliationPlanV1{}, errors.Join(errors.New("preserved attachment upload files are inconsistent"), metadataErr, contentErr)
				}
				continue
			}
			reason := "restart_corrupt_upload_files"
			commitOwner := false
			if !hasMetadata && !hasContent {
				reason = "restart_missing_upload_files"
			} else if !hasMetadata || !hasContent {
				reason = "restart_partial_upload_files"
			} else if metadataExact && contentExact {
				currentErr := validateCurrent(intent.Owner)
				if currentErr == nil {
					commitOwner = true
					reason = "restart_completed_exact_upload"
				} else if errors.Is(currentErr, turnapp.ErrAttachmentNotAuthorized) {
					reason = "restart_stale_upload_binding"
				} else {
					return AttachmentAuthorityReconciliationPlanV1{}, currentErr
				}
			}
			status := domainattachment.UploadDispositionQuarantinedV1
			if commitOwner {
				status = domainattachment.UploadDispositionCommittedV1
			}
			settlement, settlementErr := domainattachment.NewUploadDispositionV1(
				intent, status, reason, metadataDigest, contentDigest, observedAt,
			)
			if settlementErr != nil {
				return AttachmentAuthorityReconciliationPlanV1{}, settlementErr
			}
			plan.Actions = append(plan.Actions, AttachmentUploadRecoveryActionV1{
				Intent: intent, Disposition: settlement, CommitOwner: commitOwner,
			})
			if !commitOwner {
				plan.Quarantined = append(plan.Quarantined, AttachmentQuarantineObservationV1{
					AttachmentID: id, MetadataSHA256: metadataDigest, ContentSHA256: contentDigest, ReasonCode: reason,
				})
			}
			continue
		}
		candidate := AttachmentQuarantineObservationV1{AttachmentID: id}
		if hasMetadata {
			candidate.MetadataSHA256, err = attachmentRecoveryFileDigest(ctx, metadataPath)
			if err != nil {
				return AttachmentAuthorityReconciliationPlanV1{}, err
			}
		}
		if hasContent {
			candidate.ContentSHA256, err = attachmentRecoveryFileDigest(ctx, contentPath)
			if err != nil {
				return AttachmentAuthorityReconciliationPlanV1{}, err
			}
		}
		candidate.ReasonCode = "legacy_ownerless_attachment"
		plan.Quarantined = append(plan.Quarantined, candidate)
	}
	return plan, nil
}

func (s *PersistentAttachmentStore) ApplyAuthorityReconciliation(
	ctx context.Context,
	authority attachmentauthorityport.UploadInventoryStore,
	validateCurrent AttachmentUploadCurrentValidator,
	plan AttachmentAuthorityReconciliationPlanV1,
) (resultErr error) {
	if ctx == nil || s == nil || authority == nil || validateCurrent == nil {
		return errors.New("attachment reconciliation authority is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	preserved, _ := authority.(attachmentauthorityport.RestartPreservationV1)
	if preserved != nil {
		if err := preserved.RevalidateAttachmentRestartV1(ctx); err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, preserved.RevalidateAttachmentRestartV1(ctx)) }()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, action := range plan.Actions {
		if preserved != nil && preserved.RestartPreservesThreadV1(action.Intent.Owner.ThreadID) {
			return attachmentauthorityport.ErrRestartPreserved
		}
		if domainattachment.ValidateUploadIntentV1(action.Intent) != nil ||
			domainattachment.ValidateUploadDispositionForIntentV1(action.Disposition, action.Intent) != nil {
			return errors.New("attachment reconciliation action is invalid")
		}
		if action.CommitOwner {
			if err := validateCurrent(action.Intent.Owner); err != nil {
				return fmt.Errorf("attachment upload binding changed after recovery preflight: %w", err)
			}
			if err := s.validateIntentFilesNoLock(ctx, action.Intent); err != nil {
				return fmt.Errorf("attachment upload files changed after recovery preflight: %w", err)
			}
		} else if action.Disposition.Status == domainattachment.UploadDispositionCommittedV1 {
			if err := s.validateIntentFilesNoLock(ctx, action.Intent); err != nil {
				return fmt.Errorf("committed attachment files changed after recovery preflight: %w", err)
			}
		}
	}
	for _, action := range plan.Actions {
		if action.CommitOwner {
			if err := authority.CommitOwnerForOpenUpload(ctx, action.Intent); err != nil {
				return err
			}
		}
		if err := authority.PutUploadDispositionIfAbsent(ctx, action.Disposition); err != nil {
			return err
		}
	}
	return nil
}

func attachmentRecoveryFiles(ctx context.Context, dir, suffix string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() || !strings.HasSuffix(entry.Name(), suffix) {
			return nil, errors.Join(errors.New("attachment store contains unknown recovery residue"), infoErr)
		}
		id := strings.TrimSuffix(entry.Name(), suffix)
		if !validAttachmentID(id) || id+suffix != entry.Name() {
			return nil, errors.New("attachment store contains an invalid recovery identity")
		}
		out[id] = filepath.Join(dir, entry.Name())
	}
	return out, nil
}

func attachmentRecoveryFileDigest(ctx context.Context, path string) (digest string, resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
		if resultErr != nil {
			digest = ""
		}
	}()
	hash := sha256.New()
	buffer := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		read, readErr := file.Read(buffer)
		if read > 0 {
			if _, err := hash.Write(buffer[:read]); err != nil {
				return "", err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func validateAttachmentRecoveryContent(
	ctx context.Context,
	path string,
	owner domainattachment.OwnerRecordV1,
) error {
	digest, err := attachmentRecoveryFileDigest(ctx, path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != owner.ByteSize || digest != owner.BlobSHA256 {
		return ErrAttachmentContentIntegrity
	}
	return nil
}

func validateAttachmentRecoveryMetadata(
	id string,
	path string,
	owner domainattachment.OwnerRecordV1,
) error {
	metadata, err := ReadJSONMapFile(path)
	if err != nil {
		var syntax *json.SyntaxError
		var shape *json.UnmarshalTypeError
		if errors.As(err, &syntax) || errors.As(err, &shape) {
			return errors.Join(ErrAttachmentContentIntegrity, err)
		}
		return err
	}
	storedOwner, err := validateAttachmentMetadataIntegrity(id, metadata)
	if err != nil || !sameAttachmentRecoveryOwner(storedOwner, owner) {
		return ErrAttachmentContentIntegrity
	}
	return nil
}

func (s *PersistentAttachmentStore) validateIntentFilesNoLock(
	ctx context.Context,
	intent domainattachment.UploadIntentV1,
) error {
	if domainattachment.ValidateUploadIntentV1(intent) != nil {
		return ErrAttachmentContentIntegrity
	}
	metadataPath := s.metadataPath(intent.Owner.AttachmentID)
	metadataDigest, err := attachmentRecoveryFileDigest(ctx, metadataPath)
	if err != nil {
		return err
	}
	if metadataDigest != intent.MetadataSHA256 {
		return ErrAttachmentContentIntegrity
	}
	if err := validateAttachmentRecoveryMetadata(intent.Owner.AttachmentID, metadataPath, intent.Owner); err != nil {
		return err
	}
	return validateAttachmentRecoveryContent(ctx, s.contentPath(intent.Owner.AttachmentID), intent.Owner)
}

func sameAttachmentRecoveryOwner(left, right domainattachment.OwnerRecordV1) bool {
	leftBody, leftErr := domainattachment.OwnerRecordV1Bytes(left)
	rightBody, rightErr := domainattachment.OwnerRecordV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func sameAttachmentRecoveryIntent(left, right domainattachment.UploadIntentV1) bool {
	leftBody, leftErr := domainattachment.UploadIntentV1Bytes(left)
	rightBody, rightErr := domainattachment.UploadIntentV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func sameAttachmentRecoveryDisposition(left, right domainattachment.UploadDispositionV1) bool {
	leftBody, leftErr := domainattachment.UploadDispositionV1Bytes(left)
	rightBody, rightErr := domainattachment.UploadDispositionV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}
