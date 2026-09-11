package attachmentauthority

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	appturn "analytix.local/runtime-go/internal/app/turn"
	"analytix.local/runtime-go/internal/contracts"
	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	attachmentuploadport "analytix.local/runtime-go/internal/ports/attachmentupload"
)

type ThreadReader interface {
	GetThread(string) (map[string]any, error)
}

type WorkspaceBindingAuthority interface {
	WorkspaceRealPath(string) (string, error)
	Observe(string) (domainsecurity.CaseBindingObservationV1, error)
}

type Service struct {
	Threads  ThreadReader
	Bindings WorkspaceBindingAuthority
	Owners   storeport.Store
	Uploads  storeport.UploadTransactionStore
}

func (service *Service) Available() bool {
	return service != nil && service.Threads != nil && service.Bindings != nil && service.Owners != nil
}

func (service *Service) UploadAvailable() bool {
	return service.Available() && service.Uploads != nil
}

func (service *Service) PrepareUpload(body map[string]any) (map[string]any, error) {
	if !service.Available() {
		return nil, errors.New("attachment owner authority is unavailable")
	}
	threadID := strings.TrimSpace(stringField(body, "threadId"))
	if threadID == "" || contracts.SafeRecordID(threadID) != threadID {
		return nil, appturn.ErrAttachmentNotAuthorized
	}
	thread, err := service.Threads.GetThread(threadID)
	if err != nil || thread == nil || strings.TrimSpace(stringField(thread, "id")) != threadID {
		return nil, appturn.ErrAttachmentNotAuthorized
	}
	workspaceRealPath, err := service.Bindings.WorkspaceRealPath(stringField(thread, "workspace"))
	if err != nil {
		return nil, appturn.ErrAttachmentNotAuthorized
	}
	requested := strings.TrimSpace(stringField(body, "workspace"))
	if requested == "" {
		return nil, appturn.ErrAttachmentNotAuthorized
	}
	requestedRealPath, err := service.Bindings.WorkspaceRealPath(requested)
	if err != nil || requestedRealPath != workspaceRealPath {
		return nil, appturn.ErrAttachmentNotAuthorized
	}
	prepared := contracts.CloneMap(body)
	prepared["threadId"] = threadID
	prepared["workspace"] = workspaceRealPath
	delete(prepared, "scope")
	return prepared, nil
}

func (service *Service) CommitUpload(ctx context.Context, metadata map[string]any) error {
	if !service.Available() {
		return errors.New("attachment owner authority is unavailable")
	}
	owner, err := appturn.AttachmentOwnerAuthorityRecord(metadata)
	if err != nil {
		return err
	}
	if err := service.validateOwnerCurrent(owner); err != nil {
		return err
	}
	if err := service.Owners.PutOwnerIfAbsent(ctx, owner); err != nil {
		return err
	}
	stored, err := service.Owners.ResolveOwner(ctx, owner.OwnerDigest)
	if err != nil || stored.OwnerDigest != owner.OwnerDigest || stored.AttachmentID != owner.AttachmentID ||
		stored.BlobSHA256 != owner.BlobSHA256 || stored.ByteSize != owner.ByteSize || stored.MIMEType != owner.MIMEType {
		return errors.New("attachment owner authority readback failed")
	}
	return nil
}

func (service *Service) Upload(
	ctx context.Context,
	body map[string]any,
	store attachmentuploadport.Store,
) (map[string]any, error) {
	if !service.UploadAvailable() || store == nil {
		return nil, errors.New("attachment upload transaction authority is unavailable")
	}
	preparedBody, err := service.PrepareUpload(body)
	if err != nil {
		return nil, err
	}
	prepared, err := store.Prepare(ctx, preparedBody)
	if err != nil {
		return nil, err
	}
	intent, err := domainattachment.NewUploadIntentV1(prepared.Owner(), prepared.MetadataSHA256())
	if err != nil {
		return nil, err
	}
	if err := service.Uploads.PutUploadIntentIfAbsent(ctx, intent); err != nil {
		return nil, err
	}
	metadata, err := prepared.Commit(ctx)
	if err != nil {
		return nil, errors.Join(err, service.rejectUpload(ctx, prepared, intent, "upload_store_failed"))
	}
	if err := service.validateOwnerCurrent(intent.Owner); err != nil {
		return nil, errors.Join(err, service.rejectUpload(ctx, prepared, intent, "upload_authority_rejected"))
	}
	commitErr := service.Uploads.CommitOwnerForOpenUpload(ctx, intent)
	if commitErr != nil {
		settlementCtx, cancel := attachmentUploadSettlementContext(ctx)
		stored, resolveErr := service.Owners.ResolveOwner(settlementCtx, intent.Owner.OwnerDigest)
		cancel()
		if resolveErr != nil || !sameAttachmentOwner(intent.Owner, stored) {
			return nil, errors.Join(commitErr, resolveErr, service.rejectUpload(ctx, prepared, intent, "upload_authority_rejected"))
		}
	}
	disposition, err := domainattachment.NewUploadDispositionV1(
		intent, domainattachment.UploadDispositionCommittedV1, "upload_committed",
		intent.MetadataSHA256, intent.Owner.BlobSHA256, time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	settlementCtx, cancel := attachmentUploadSettlementContext(ctx)
	err = service.Uploads.PutUploadDispositionIfAbsent(settlementCtx, disposition)
	cancel()
	if err != nil {
		return nil, err
	}
	return metadata, nil
}

func (service *Service) rejectUpload(
	ctx context.Context,
	prepared attachmentuploadport.PreparedUpload,
	intent domainattachment.UploadIntentV1,
	reason string,
) error {
	settlementCtx, cancel := attachmentUploadSettlementContext(ctx)
	defer cancel()
	if stored, err := service.Owners.ResolveOwner(settlementCtx, intent.Owner.OwnerDigest); err == nil {
		if sameAttachmentOwner(stored, intent.Owner) {
			return errors.New("attachment upload owner committed before rejection")
		}
		return errors.New("attachment upload owner authority conflicts before rejection")
	} else if !errors.Is(err, storeport.ErrNotFound) {
		return err
	}
	disposition, err := domainattachment.NewUploadDispositionV1(
		intent, domainattachment.UploadDispositionRejectedV1, reason, "", "", time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if err := service.Uploads.PutUploadDispositionIfAbsent(settlementCtx, disposition); err != nil {
		return err
	}
	return prepared.Abort(settlementCtx)
}

func attachmentUploadSettlementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
}

func (service *Service) validateOwnerCurrent(owner domainattachment.OwnerRecordV1) error {
	thread, err := service.Threads.GetThread(owner.ThreadID)
	if err != nil {
		return err
	}
	if thread == nil || strings.TrimSpace(stringField(thread, "id")) != owner.ThreadID {
		return appturn.ErrAttachmentNotAuthorized
	}
	workspaceRealPath, err := service.Bindings.WorkspaceRealPath(stringField(thread, "workspace"))
	if err != nil {
		return err
	}
	if workspaceRealPath != owner.WorkspaceRealPath {
		return appturn.ErrAttachmentNotAuthorized
	}
	currentBinding, err := service.Bindings.Observe(workspaceRealPath)
	if err != nil {
		return err
	}
	if !attachmentOwnerMatchesCurrentBinding(owner, currentBinding) {
		return appturn.ErrAttachmentNotAuthorized
	}
	return nil
}

func (service *Service) ValidateUploadOwnerCurrent(owner domainattachment.OwnerRecordV1) error {
	if !service.UploadAvailable() || domainattachment.ValidateOwnerRecordV1(owner) != nil {
		return appturn.ErrAttachmentNotAuthorized
	}
	return service.validateOwnerCurrent(owner)
}

func sameAttachmentOwner(left, right domainattachment.OwnerRecordV1) bool {
	leftBody, leftErr := domainattachment.OwnerRecordV1Bytes(left)
	rightBody, rightErr := domainattachment.OwnerRecordV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func attachmentOwnerMatchesCurrentBinding(
	owner domainattachment.OwnerRecordV1,
	current domainsecurity.CaseBindingObservationV1,
) bool {
	if domainsecurity.ValidateCaseBindingObservationV1(current) != nil || current.WorkspaceRealPath != owner.WorkspaceRealPath {
		return false
	}
	if owner.CaseBindingObservation != nil {
		return current.State == domainsecurity.CaseBindingStateValid &&
			current.ObservationDigest == owner.CaseBindingObservation.ObservationDigest
	}
	return current.State == domainsecurity.CaseBindingStateMissing ||
		current.State == domainsecurity.CaseBindingStateNotApplicable
}

func (service *Service) AuthorizeContent(
	ctx context.Context,
	metadata map[string]any,
	threadID string,
	requestedWorkspace string,
) bool {
	if !service.Available() {
		return false
	}
	threadID = strings.TrimSpace(threadID)
	if threadID == "" || contracts.SafeRecordID(threadID) != threadID {
		return false
	}
	thread, err := service.Threads.GetThread(threadID)
	if err != nil || thread == nil || strings.TrimSpace(stringField(thread, "id")) != threadID {
		return false
	}
	workspaceRealPath, err := service.Bindings.WorkspaceRealPath(stringField(thread, "workspace"))
	if err != nil || strings.TrimSpace(requestedWorkspace) == "" {
		return false
	}
	requestedRealPath, err := service.Bindings.WorkspaceRealPath(requestedWorkspace)
	if err != nil || requestedRealPath != workspaceRealPath {
		return false
	}
	currentBinding, err := service.Bindings.Observe(workspaceRealPath)
	if err != nil || !appturn.AttachmentMetadataAuthorizedForCurrentCase(
		metadata, threadID, workspaceRealPath, currentBinding,
	) {
		return false
	}
	owner, err := appturn.AttachmentOwnerAuthorityRecord(metadata)
	if err != nil {
		return false
	}
	stored, err := service.Owners.ResolveOwner(ctx, owner.OwnerDigest)
	return err == nil && stored.OwnerDigest == owner.OwnerDigest && stored.AttachmentID == owner.AttachmentID &&
		stored.BlobSHA256 == owner.BlobSHA256 && stored.ByteSize == owner.ByteSize && stored.MIMEType == owner.MIMEType
}

func (service *Service) PublicMetadata(metadata map[string]any) map[string]any {
	return appturn.AttachmentPublicMetadata(metadata)
}

func stringField(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}
