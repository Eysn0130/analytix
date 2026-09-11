package attachmentuse

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	domainattachment "analytix.local/runtime-go/internal/domain/attachment"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	storeport "analytix.local/runtime-go/internal/ports/attachmentauthority"
	finalauthorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

var (
	ErrUnavailable = errors.New("attachment use authority is unavailable")
	ErrClosed      = errors.New("attachment use authority is closed")
	ErrMismatch    = errors.New("attachment use authority binding is invalid")
	ErrOpen        = errors.New("attachment use authority remains open")
)

type CurrentContextValidator func(context.Context, domainsecurity.TurnSecurityContext) error

type Service struct {
	authority       finalauthorityport.Authority
	store           storeport.Store
	validateCurrent CurrentContextValidator
}

// RequireNoOpenForFrozenPublicationContext is the terminal publication guard.
// It validates the complete private inventory before proving that the exact
// frozen turn/context has no indeterminate attachment effect still in flight.
// Current-context freshness belongs to the publication authority that follows
// this guard: repeating it here would prevent a stale turn from reaching the
// Final Evidence Gate and persisting its deterministic fact-free boundary.
func (service *Service) RequireNoOpenForFrozenPublicationContext(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	if !service.Available() || domainsecurity.ValidateTurnSecurityContextForCasePublication(securityContext) != nil {
		return ErrUnavailable
	}
	plan, err := service.PlanRestartDispositions(ctx)
	if err != nil {
		return err
	}
	for _, receipt := range plan.OpenReceipts {
		if receipt.Context.ContextDigest == securityContext.ContextDigest &&
			receipt.Context.ThreadID == securityContext.ThreadID && receipt.Context.TurnID == securityContext.TurnID {
			return ErrOpen
		}
	}
	return nil
}

type IssueBatchInput struct {
	SecurityContext       domainsecurity.TurnSecurityContext
	Owners                []domainattachment.OwnerRecordV1
	Projections           []domainattachment.AttachmentUseProjectionV1
	EffectBindingDigest   string
	ConsumerKind          string
	ConsumerBindingDigest string
	IssuedAt              time.Time
}

type RestartPlanV1 struct {
	OpenReceipts    []domainattachment.AttachmentUseReceiptV1
	PreservedUseIDs []string
}

func NewService(
	authority finalauthorityport.Authority,
	store storeport.Store,
	validateCurrent CurrentContextValidator,
) *Service {
	return &Service{authority: authority, store: store, validateCurrent: validateCurrent}
}

func (service *Service) Available() bool {
	return service != nil && service.authority != nil && service.store != nil && service.validateCurrent != nil
}

func (service *Service) IssueBatch(ctx context.Context, input IssueBatchInput) ([]domainattachment.AttachmentUseReceiptV1, error) {
	if !service.Available() || len(input.Owners) == 0 || len(input.Owners) != len(input.Projections) {
		return nil, ErrUnavailable
	}
	if err := service.requireRestartWritableV1(ctx, input.SecurityContext.ThreadID); err != nil {
		return nil, err
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return nil, errors.Join(ErrMismatch, err)
	}
	for _, owner := range input.Owners {
		if err := service.verifyStoredOwner(ctx, owner); err != nil {
			return nil, err
		}
	}
	receipts := make([]domainattachment.AttachmentUseReceiptV1, len(input.Owners))
	for index, owner := range input.Owners {
		receipt, err := domainattachment.NewAttachmentUseReceiptV1(domainattachment.AttachmentUseReceiptInputV1{
			SecurityContext: input.SecurityContext, Owner: owner,
			AttachmentSet: append([]domainattachment.OwnerRecordV1(nil), input.Owners...),
			ProjectionSet: append([]domainattachment.AttachmentUseProjectionV1(nil), input.Projections...),
			BatchIndex:    uint32(index), EffectBindingDigest: strings.TrimSpace(input.EffectBindingDigest),
			ConsumerKind: strings.TrimSpace(input.ConsumerKind), ConsumerBindingDigest: strings.TrimSpace(input.ConsumerBindingDigest),
			IssuedAt: input.IssuedAt, AuthorityKeyID: service.authority.KeyID(), AuthorityPublicKey: service.authority.PublicKey(),
		}, func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
		if err != nil {
			return nil, err
		}
		receipts[index] = receipt
	}
	for index, receipt := range receipts {
		if _, err := service.store.ResolveUseDisposition(ctx, receipt.UseID); err == nil {
			return nil, ErrClosed
		} else if !errors.Is(err, storeport.ErrNotFound) {
			return nil, err
		}
		if existing, err := service.store.ResolveUseReceipt(ctx, receipt.UseID); err == nil {
			if !sameReceipt(existing, receipt) || service.verifyOpenReceipt(ctx, input, index, existing) != nil {
				return nil, ErrMismatch
			}
		} else if !errors.Is(err, storeport.ErrNotFound) {
			return nil, err
		}
	}
	committed := make([]domainattachment.AttachmentUseReceiptV1, 0, len(receipts))
	for index, receipt := range receipts {
		if _, err := service.store.ResolveUseDisposition(ctx, receipt.UseID); err == nil {
			service.closeCommittedBestEffort(ctx, committed, domainattachment.AttachmentUseDispositionRejectedV1, "batch_closed_during_commit", input.IssuedAt)
			return nil, ErrClosed
		} else if !errors.Is(err, storeport.ErrNotFound) {
			return nil, err
		}
		if existing, err := service.store.ResolveUseReceipt(ctx, receipt.UseID); err == nil {
			if !sameReceipt(existing, receipt) || service.verifyOpenReceipt(ctx, input, index, existing) != nil {
				return nil, ErrMismatch
			}
			committed = append(committed, existing)
			continue
		} else if !errors.Is(err, storeport.ErrNotFound) {
			return nil, err
		}
		if err := service.store.PutUseReceiptIfAbsent(ctx, receipt); err != nil {
			service.closeCommittedBestEffort(ctx, committed, domainattachment.AttachmentUseDispositionRejectedV1, "batch_commit_failed", input.IssuedAt)
			return nil, err
		}
		persisted, err := service.store.ResolveUseReceipt(ctx, receipt.UseID)
		if err != nil || !sameReceipt(persisted, receipt) || service.verifyOpenReceipt(ctx, input, index, persisted) != nil {
			service.closeCommittedBestEffort(ctx, committed, domainattachment.AttachmentUseDispositionRejectedV1, "batch_readback_failed", input.IssuedAt)
			return nil, errors.Join(ErrMismatch, err)
		}
		committed = append(committed, persisted)
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		service.closeCommittedBestEffort(ctx, committed, domainattachment.AttachmentUseDispositionStaleContextV1, "current_context_changed", input.IssuedAt)
		return nil, errors.Join(ErrMismatch, err)
	}
	return committed, nil
}

func (service *Service) VerifyOpenBatch(
	ctx context.Context,
	input IssueBatchInput,
	receipts []domainattachment.AttachmentUseReceiptV1,
) error {
	if !service.Available() || len(receipts) == 0 || len(receipts) != len(input.Owners) || len(receipts) != len(input.Projections) {
		return ErrUnavailable
	}
	if err := service.requireRestartWritableV1(ctx, input.SecurityContext.ThreadID); err != nil {
		return err
	}
	if err := service.validateCurrent(ctx, input.SecurityContext); err != nil {
		return errors.Join(ErrMismatch, err)
	}
	setDigest := receipts[0].AttachmentSetDigest
	seen := map[string]bool{}
	for index, receipt := range receipts {
		if seen[receipt.UseID] || receipt.BatchIndex != uint32(index) || receipt.BatchCount != uint32(len(receipts)) ||
			receipt.AttachmentSetDigest != setDigest {
			return ErrMismatch
		}
		seen[receipt.UseID] = true
		if err := service.verifyStoredOwner(ctx, input.Owners[index]); err != nil {
			return err
		}
		if err := service.verifyOpenReceipt(ctx, input, index, receipt); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) CloseBatch(
	ctx context.Context,
	receipts []domainattachment.AttachmentUseReceiptV1,
	status string,
	reasonCode string,
	disposedAt time.Time,
) ([]domainattachment.AttachmentUseDispositionV1, error) {
	if !service.Available() || len(receipts) == 0 {
		return nil, ErrUnavailable
	}
	for _, receipt := range receipts {
		if err := service.requireRestartWritableV1(ctx, receipt.Context.ThreadID); err != nil {
			return nil, err
		}
	}
	dispositions := make([]domainattachment.AttachmentUseDispositionV1, 0, len(receipts))
	for _, receipt := range receipts {
		stored, err := service.store.ResolveUseReceipt(ctx, receipt.UseID)
		if err != nil || !sameReceipt(stored, receipt) || service.verifyTrustedReceipt(ctx, stored) != nil {
			return nil, errors.Join(ErrMismatch, err)
		}
		if existing, err := service.store.ResolveUseDisposition(ctx, receipt.UseID); err == nil {
			if service.verifyTrustedDisposition(ctx, existing, receipt) != nil || existing.Status != status || existing.ReasonCode != reasonCode {
				return nil, ErrClosed
			}
			dispositions = append(dispositions, existing)
			continue
		} else if !errors.Is(err, storeport.ErrNotFound) {
			return nil, err
		}
		disposition, err := domainattachment.NewAttachmentUseDispositionV1(
			receipt, status, reasonCode, disposedAt, service.authority.KeyID(), service.authority.PublicKey(),
			func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) },
		)
		if err != nil {
			return nil, err
		}
		if err := service.store.PutUseDispositionIfAbsent(ctx, disposition); err != nil {
			return nil, err
		}
		persisted, err := service.store.ResolveUseDisposition(ctx, receipt.UseID)
		if err != nil || persisted.RecordDigest != disposition.RecordDigest || service.verifyTrustedDisposition(ctx, persisted, receipt) != nil {
			return nil, errors.Join(ErrMismatch, err)
		}
		dispositions = append(dispositions, persisted)
	}
	return dispositions, nil
}

func (service *Service) PlanRestartDispositions(ctx context.Context) (plan RestartPlanV1, resultErr error) {
	if !service.Available() {
		return RestartPlanV1{}, ErrUnavailable
	}
	if ctx == nil {
		return RestartPlanV1{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return RestartPlanV1{}, err
	}
	preserved, _ := service.store.(storeport.RestartPreservationV1)
	if preserved != nil {
		if err := preserved.RevalidateAttachmentRestartV1(ctx); err != nil {
			return RestartPlanV1{}, err
		}
		defer func() {
			resultErr = errors.Join(resultErr, preserved.RevalidateAttachmentRestartV1(ctx))
			if resultErr != nil {
				plan = RestartPlanV1{}
			}
		}()
	}
	inventory, ok := service.store.(storeport.InventoryStore)
	if !ok {
		return RestartPlanV1{}, ErrUnavailable
	}
	owners := map[string]domainattachment.OwnerRecordV1{}
	if err := inventory.VisitOwners(ctx, func(owner domainattachment.OwnerRecordV1) error {
		owners[owner.OwnerDigest] = owner
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	receipts := map[string]domainattachment.AttachmentUseReceiptV1{}
	if err := inventory.VisitUseReceipts(ctx, func(receipt domainattachment.AttachmentUseReceiptV1) error {
		owner, exists := owners[receipt.OwnerDigest]
		if !exists || !receiptMatchesOwner(receipt, owner) {
			return ErrMismatch
		}
		if err := service.verifyTrustedReceipt(ctx, receipt); err != nil {
			return err
		}
		receipts[receipt.UseID] = receipt
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	closed := map[string]bool{}
	if err := inventory.VisitUseDispositions(ctx, func(disposition domainattachment.AttachmentUseDispositionV1) error {
		receipt, exists := receipts[disposition.UseID]
		if !exists {
			return ErrMismatch
		}
		if err := service.verifyTrustedDisposition(ctx, disposition, receipt); err != nil {
			return err
		}
		closed[disposition.UseID] = true
		return nil
	}); err != nil {
		return RestartPlanV1{}, err
	}
	plan = RestartPlanV1{OpenReceipts: make([]domainattachment.AttachmentUseReceiptV1, 0), PreservedUseIDs: []string{}}
	for useID, receipt := range receipts {
		if !closed[useID] {
			plan.OpenReceipts = append(plan.OpenReceipts, receipt)
			if preserved != nil && preserved.RestartPreservesThreadV1(receipt.Context.ThreadID) {
				plan.PreservedUseIDs = append(plan.PreservedUseIDs, useID)
			}
		}
	}
	sort.Slice(plan.OpenReceipts, func(i, j int) bool { return plan.OpenReceipts[i].UseID < plan.OpenReceipts[j].UseID })
	sort.Strings(plan.PreservedUseIDs)
	return plan, nil
}

func (service *Service) ApplyRestartDispositions(ctx context.Context, plan RestartPlanV1, disposedAt time.Time) error {
	current, err := service.PlanRestartDispositions(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, plan) {
		return errors.New("attachment use restart inventory changed")
	}
	preserved := make(map[string]bool, len(current.PreservedUseIDs))
	for _, useID := range current.PreservedUseIDs {
		preserved[useID] = true
	}
	active := make([]domainattachment.AttachmentUseReceiptV1, 0, len(current.OpenReceipts))
	for _, receipt := range current.OpenReceipts {
		if !preserved[receipt.UseID] {
			active = append(active, receipt)
		}
	}
	if len(active) == 0 {
		return nil
	}
	_, err = service.CloseBatch(
		ctx, active, domainattachment.AttachmentUseDispositionRestartInvalidV1,
		"restart_indeterminate", disposedAt,
	)
	return err
}

func (service *Service) requireRestartWritableV1(ctx context.Context, threadID string) error {
	if preserved, ok := service.store.(storeport.RestartPreservationV1); ok {
		if err := preserved.RevalidateAttachmentRestartV1(ctx); err != nil {
			return err
		}
		if preserved.RestartPreservesThreadV1(threadID) {
			return storeport.ErrRestartPreserved
		}
	}
	return nil
}

func (service *Service) verifyOpenReceipt(ctx context.Context, input IssueBatchInput, index int, receipt domainattachment.AttachmentUseReceiptV1) error {
	if index < 0 || index >= len(input.Owners) || domainattachment.ValidateAttachmentUseReceiptForBindingsV1(
		receipt, input.SecurityContext, input.Owners[index], input.Owners, input.Projections,
		input.EffectBindingDigest, input.ConsumerKind, input.ConsumerBindingDigest,
	) != nil || service.verifyTrustedReceipt(ctx, receipt) != nil {
		return ErrMismatch
	}
	persisted, err := service.store.ResolveUseReceipt(ctx, receipt.UseID)
	if err != nil || !sameReceipt(persisted, receipt) {
		return errors.Join(ErrMismatch, err)
	}
	if _, err := service.store.ResolveUseDisposition(ctx, receipt.UseID); err == nil {
		return ErrClosed
	} else if !errors.Is(err, storeport.ErrNotFound) {
		return err
	}
	return nil
}

func (service *Service) verifyStoredOwner(ctx context.Context, owner domainattachment.OwnerRecordV1) error {
	stored, err := service.store.ResolveOwner(ctx, owner.OwnerDigest)
	if err != nil || !sameOwner(stored, owner) {
		return errors.Join(ErrMismatch, err)
	}
	return nil
}

func (service *Service) verifyTrustedReceipt(ctx context.Context, receipt domainattachment.AttachmentUseReceiptV1) error {
	keyID, publicKey, signature, err := domainattachment.AttachmentUseReceiptV1AuthorityMaterial(receipt)
	if err != nil {
		return errors.Join(ErrMismatch, err)
	}
	if err := service.authority.VerifyTrusted(
		ctx, keyID, publicKey, domainattachment.AttachmentUseReceiptV1SigningBytes(receipt), signature,
	); err != nil {
		return errors.Join(ErrMismatch, err)
	}
	return nil
}

func (service *Service) verifyTrustedDisposition(
	ctx context.Context,
	disposition domainattachment.AttachmentUseDispositionV1,
	receipt domainattachment.AttachmentUseReceiptV1,
) error {
	if domainattachment.ValidateAttachmentUseDispositionForReceiptV1(disposition, receipt) != nil {
		return ErrMismatch
	}
	keyID, publicKey, signature, err := domainattachment.AttachmentUseDispositionV1AuthorityMaterial(disposition)
	if err != nil {
		return errors.Join(ErrMismatch, err)
	}
	if err := service.authority.VerifyTrusted(
		ctx, keyID, publicKey, domainattachment.AttachmentUseDispositionV1SigningBytes(disposition), signature,
	); err != nil {
		return errors.Join(ErrMismatch, err)
	}
	return nil
}

func (service *Service) closeCommittedBestEffort(
	ctx context.Context,
	receipts []domainattachment.AttachmentUseReceiptV1,
	status string,
	reason string,
	at time.Time,
) {
	if len(receipts) == 0 {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.Add(time.Nanosecond)
	}
	_, _ = service.CloseBatch(ctx, receipts, status, reason, at)
}

func sameOwner(left, right domainattachment.OwnerRecordV1) bool {
	leftBody, leftErr := domainattachment.OwnerRecordV1Bytes(left)
	rightBody, rightErr := domainattachment.OwnerRecordV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func sameReceipt(left, right domainattachment.AttachmentUseReceiptV1) bool {
	leftBody, leftErr := domainattachment.AttachmentUseReceiptV1Bytes(left)
	rightBody, rightErr := domainattachment.AttachmentUseReceiptV1Bytes(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBody, rightBody)
}

func receiptMatchesOwner(receipt domainattachment.AttachmentUseReceiptV1, owner domainattachment.OwnerRecordV1) bool {
	return domainattachment.ValidateAttachmentUseReceiptV1(receipt) == nil && domainattachment.ValidateOwnerRecordV1(owner) == nil &&
		owner.ThreadID == receipt.Context.ThreadID && owner.WorkspaceRealPath == receipt.Context.WorkspaceRealPath &&
		receipt.OwnerDigest == owner.OwnerDigest && receipt.AttachmentID == owner.AttachmentID &&
		receipt.BlobSHA256 == owner.BlobSHA256 && receipt.BlobByteSize == owner.ByteSize && receipt.MIMEType == owner.MIMEType
}
