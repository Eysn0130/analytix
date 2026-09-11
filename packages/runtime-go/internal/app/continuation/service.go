package continuation

import (
	"context"
	"errors"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	appturn "analytix.local/runtime-go/internal/app/turn"
	domaincontinuation "analytix.local/runtime-go/internal/domain/continuation"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	continuationstoreport "analytix.local/runtime-go/internal/ports/continuationstore"
	authorityport "analytix.local/runtime-go/internal/ports/finalauthority"
)

const hostOperationTimeout = 15 * time.Second

var (
	ErrAuthorityUnavailable = errors.New("continuation authority is unavailable")
	ErrReceiptConsumed      = errors.New("continuation receipt is already consumed")
	ErrReceiptExpired       = errors.New("continuation receipt is expired")
)

type Service struct {
	authority authorityport.Authority
	store     continuationstoreport.Store
}

func NewService(authority authorityport.Authority, store continuationstoreport.Store) *Service {
	return &Service{authority: authority, store: store}
}

func (service *Service) Available() bool {
	return service != nil && service.authority != nil && service.store != nil && strings.TrimSpace(service.authority.KeyID()) != "" && len(service.authority.PublicKey()) > 0
}

func validatePayloadForExecution(payload domaincontinuation.Payload) error {
	if err := domaincontinuation.ValidatePayloadForExecution(payload); err != nil {
		return err
	}
	return executiongrantapp.ValidateExecutionGrantForCall(
		payload.SecurityContext,
		payload.ExecutionGrant,
		domainmodel.ToolCall{ID: payload.CallID, Name: payload.ToolName, Arguments: payload.Arguments},
	)
}

func validateReceiptForExecution(receipt domaincontinuation.Receipt) error {
	if err := domaincontinuation.ValidateReceiptForExecution(receipt); err != nil {
		return err
	}
	return validatePayloadForExecution(receipt.Payload)
}

func (service *Service) IssuePendingHost(kind, gateID, itemID string, pending appmodel.PendingToolCall, issuedAt time.Time) (domaincontinuation.Receipt, error) {
	if !service.Available() {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	payload, err := appturn.GateContinuationPayload(kind, gateID, itemID, pending, issuedAt)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	receipt, err := service.Issue(ctx, payload)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if receipt.Payload.GateID != strings.TrimSpace(gateID) || receipt.Payload.ItemID != strings.TrimSpace(itemID) {
		return domaincontinuation.Receipt{}, errors.New("continuation receipt identity does not match the gate")
	}
	return receipt, nil
}

// IssueOrResolvePendingHost makes the signed continuation receipt a durable
// request intent. The gate id is deterministic, so an acknowledgement-loss or
// concurrent exact retry must reuse the first trusted receipt (including its
// original IssuedAt) instead of attempting to mint conflicting authority.
func (service *Service) IssueOrResolvePendingHost(
	kind, gateID, itemID string,
	pending appmodel.PendingToolCall,
	issuedAt time.Time,
) (domaincontinuation.Receipt, error) {
	if !service.Available() {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	if existing, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID)); err == nil {
		return service.validateReusablePendingReceipt(ctx, existing, kind, gateID, itemID, pending, issuedAt)
	} else if !errors.Is(err, continuationstoreport.ErrNotFound) {
		return domaincontinuation.Receipt{}, err
	}
	payload, err := appturn.GateContinuationPayload(kind, gateID, itemID, pending, issuedAt)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	receipt, issueErr := service.Issue(ctx, payload)
	if issueErr == nil {
		return service.validateReusablePendingReceipt(ctx, receipt, kind, gateID, itemID, pending, issuedAt)
	}
	// A competing exact writer or an injected post-commit failure may have
	// committed the receipt even though this caller observed an error.
	existing, resolveErr := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if resolveErr != nil {
		return domaincontinuation.Receipt{}, errors.Join(issueErr, resolveErr)
	}
	verified, verifyErr := service.validateReusablePendingReceipt(ctx, existing, kind, gateID, itemID, pending, issuedAt)
	if verifyErr != nil {
		return domaincontinuation.Receipt{}, errors.Join(issueErr, verifyErr)
	}
	return verified, nil
}

func (service *Service) validateReusablePendingReceipt(
	ctx context.Context,
	receipt domaincontinuation.Receipt,
	kind, gateID, itemID string,
	pending appmodel.PendingToolCall,
	now time.Time,
) (domaincontinuation.Receipt, error) {
	receipt, err := service.verifyReceipt(ctx, receipt)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if err := validateReceiptForExecution(receipt); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if receipt.Payload.Kind != strings.TrimSpace(kind) || receipt.Payload.GateID != strings.TrimSpace(gateID) ||
		receipt.Payload.ItemID != strings.TrimSpace(itemID) {
		return domaincontinuation.Receipt{}, errors.New("continuation request intent identity does not match the gate")
	}
	if err := appturn.ValidatePendingContinuationReceipt(receipt, pending); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if _, err := service.store.ResolveDisposition(ctx, receipt.Payload.GateID); err == nil {
		return domaincontinuation.Receipt{}, ErrReceiptConsumed
	} else if !errors.Is(err, continuationstoreport.ErrNotFound) {
		return domaincontinuation.Receipt{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, receipt.Payload.ExecutionGrant.ExpiresAt)
	if err != nil || !now.UTC().Before(expiresAt) {
		return domaincontinuation.Receipt{}, ErrReceiptExpired
	}
	return receipt, nil
}

func (service *Service) DisposeHost(gateID, status, reasonCode string, at time.Time) error {
	if !service.Available() {
		return ErrAuthorityUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	if strings.TrimSpace(status) == domaincontinuation.StatusAllowed || strings.TrimSpace(status) == domaincontinuation.StatusSubmitted {
		_, err := service.Consume(ctx, gateID, status, reasonCode, at)
		return err
	}
	_, err := service.Close(ctx, gateID, status, reasonCode, at)
	return err
}

// DisposePendingHost is the live same-process claim path. The signed private
// payload must exactly match the pending authority retained by the gate
// registry before the receipt can be consumed or monotonically closed.
func (service *Service) DisposePendingHost(gateID, status, reasonCode string, at time.Time, pending appmodel.PendingToolCall) error {
	_, err := service.DisposePendingHostDisposition(gateID, status, reasonCode, at, pending)
	return err
}

// DisposePendingHostDisposition returns the immutable signed disposition so
// downstream approval transitions can derive all timestamps and identities
// from the same host authority. An exact consumed retry resolves and returns
// the first disposition; an opposite disposition remains a hard conflict.
func (service *Service) DisposePendingHostDisposition(
	gateID, status, reasonCode string,
	at time.Time,
	pending appmodel.PendingToolCall,
) (domaincontinuation.Disposition, error) {
	if !service.Available() {
		return domaincontinuation.Disposition{}, ErrAuthorityUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	receipt, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if err == nil {
		receipt, err = service.verifyReceipt(ctx, receipt)
	}
	if err == nil {
		err = validateReceiptForExecution(receipt)
	}
	if err == nil {
		err = appturn.ValidatePendingContinuationReceipt(receipt, pending)
	}
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	status = strings.TrimSpace(status)
	reasonCode = strings.TrimSpace(reasonCode)
	if existing, dispositionErr := service.store.ResolveDisposition(ctx, receipt.Payload.GateID); dispositionErr == nil {
		if service.verifyDisposition(ctx, existing, receipt) == nil && existing.Status == status && existing.ReasonCode == reasonCode {
			return existing, nil
		}
		return domaincontinuation.Disposition{}, ErrReceiptConsumed
	} else if !errors.Is(dispositionErr, continuationstoreport.ErrNotFound) {
		return domaincontinuation.Disposition{}, dispositionErr
	}
	if status == domaincontinuation.StatusAllowed || status == domaincontinuation.StatusSubmitted {
		if at.IsZero() {
			at = time.Now().UTC()
		}
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, receipt.Payload.ExecutionGrant.ExpiresAt)
		if parseErr != nil || !at.UTC().Before(expiresAt) {
			return domaincontinuation.Disposition{}, ErrReceiptExpired
		}
	}
	disposition, err := service.dispose(ctx, receipt, status, reasonCode, at)
	if err == nil {
		return disposition, nil
	}
	if !errors.Is(err, ErrReceiptConsumed) {
		return domaincontinuation.Disposition{}, err
	}
	existing, resolveErr := service.store.ResolveDisposition(ctx, receipt.Payload.GateID)
	if resolveErr != nil || service.verifyDisposition(ctx, existing, receipt) != nil ||
		existing.Status != strings.TrimSpace(status) || existing.ReasonCode != strings.TrimSpace(reasonCode) {
		return domaincontinuation.Disposition{}, err
	}
	return existing, nil
}

// ValidatePendingHost verifies the complete signed private continuation
// identity without consuming or reopening it. Terminal claim callers use it
// to validate an entire batch before mutating the manager, item projection,
// event log, or owning turn. A prior disposition is allowed because retry
// validation must remain exact after a partial settlement.
func (service *Service) ValidatePendingHost(
	gateID, receiptID, kind, itemID string,
	pending appmodel.PendingToolCall,
) error {
	_, err := service.ResolvePendingReceiptHost(gateID, receiptID, kind, itemID, pending)
	return err
}

// ResolvePendingReceiptHost returns the exact installation-trusted request
// intent for public outbox repair. It deliberately permits an existing
// disposition: callers may repair projection only, never resume execution.
func (service *Service) ResolvePendingReceiptHost(
	gateID, receiptID, kind, itemID string,
	pending appmodel.PendingToolCall,
) (domaincontinuation.Receipt, error) {
	if !service.Available() {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	receipt, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	receipt, err = service.verifyReceipt(ctx, receipt)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if err := validateReceiptForExecution(receipt); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if receipt.ReceiptID != strings.TrimSpace(receiptID) ||
		receipt.Payload.GateID != strings.TrimSpace(gateID) ||
		receipt.Payload.Kind != strings.TrimSpace(kind) ||
		receipt.Payload.ItemID != strings.TrimSpace(itemID) {
		return domaincontinuation.Receipt{}, errors.New("continuation terminal claim does not match its signed receipt")
	}
	if err := appturn.ValidatePendingContinuationReceipt(receipt, pending); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	return receipt, nil
}

// ValidateRestartRecordHost binds an event-replayed gate record to the exact
// signed receipt before restart reconciliation may patch an item or emit a
// cancellation. Replay fields alone are not authority.
func (service *Service) ValidateRestartRecordHost(
	gateID, receiptID, kind, threadID, turnID, itemID string,
) error {
	if !service.Available() {
		return ErrAuthorityUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	receipt, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if err != nil {
		return err
	}
	receipt, err = service.verifyReceipt(ctx, receipt)
	if err != nil {
		return err
	}
	if receipt.ReceiptID != strings.TrimSpace(receiptID) ||
		receipt.Payload.GateID != strings.TrimSpace(gateID) ||
		receipt.Payload.Kind != strings.TrimSpace(kind) ||
		receipt.Payload.ThreadID != strings.TrimSpace(threadID) ||
		receipt.Payload.TurnID != strings.TrimSpace(turnID) ||
		receipt.Payload.ItemID != strings.TrimSpace(itemID) {
		return errors.New("restarted continuation record does not match its signed receipt")
	}
	return nil
}

func (service *Service) RestartDispositionReason(gateID, receiptID, kind, threadID, turnID, itemID string, thread map[string]any, resolver appmodel.StrictPendingToolProviderResolver, authorize func(appmodel.PendingToolCall) error) string {
	reason := "restart_receipt_invalid"
	if !service.Available() || strings.TrimSpace(receiptID) == "" {
		return reason
	}
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	receipt, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if err == nil {
		receipt, err = service.verifyReceipt(ctx, receipt)
	}
	cancel()
	if err != nil || receipt.ReceiptID != receiptID || receipt.Payload.Kind != kind || receipt.Payload.ThreadID != threadID ||
		receipt.Payload.TurnID != turnID || receipt.Payload.ItemID != itemID {
		return reason
	}
	if receipt.Payload.Version == domaincontinuation.ContractVersionV2 {
		return "restart_legacy_continuation_contract"
	}
	if !domaincontinuation.HasExactProviderStepBinding(receipt.Payload) {
		return "restart_provider_step_binding_missing"
	}
	if authorize == nil {
		return reason
	}
	ctx, cancel = context.WithTimeout(context.Background(), hostOperationTimeout)
	receipt, err = service.VerifyOpen(ctx, gateID, time.Now().UTC())
	cancel()
	if err != nil {
		return reason
	}
	pending, err := appmodel.RebuildPendingToolCallFromReceiptWithSteeringAuthority(
		receipt, thread, resolver, service.authority,
	)
	if err != nil {
		return "restart_rebuild_rejected"
	}
	if err := authorize(pending); err != nil {
		var validation executiongrantapp.ValidationError
		if executiongrantapp.AsValidationError(err, &validation) {
			code := strings.TrimSpace(validation.Code)
			if code != "" && len(code) <= 96 {
				return "restart_" + strings.ReplaceAll(code, "-", "_")
			}
		}
		return "restart_authority_rejected"
	}
	return "restart_revalidation_passed_nonresumable"
}

func (service *Service) Issue(ctx context.Context, payload domaincontinuation.Payload) (domaincontinuation.Receipt, error) {
	if !service.Available() {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	if err := validatePayloadForExecution(payload); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	receipt, err := domaincontinuation.NewReceipt(payload, service.authority.KeyID(), service.authority.PublicKey(), func(message []byte) ([]byte, error) {
		return service.authority.Sign(ctx, message)
	})
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if err := service.store.PutReceiptIfAbsent(ctx, receipt); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	verified, err := service.verifyReceipt(ctx, receipt)
	if err != nil || verified.ReceiptID != receipt.ReceiptID {
		return domaincontinuation.Receipt{}, errors.New("persisted continuation receipt verification failed")
	}
	return verified, nil
}

func (service *Service) VerifyOpen(ctx context.Context, gateID string, now time.Time) (domaincontinuation.Receipt, error) {
	if !service.Available() {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	receipt, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	receipt, err = service.verifyReceipt(ctx, receipt)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if err := validateReceiptForExecution(receipt); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if _, err := service.store.ResolveDisposition(ctx, receipt.Payload.GateID); err == nil {
		return domaincontinuation.Receipt{}, ErrReceiptConsumed
	} else if !errors.Is(err, continuationstoreport.ErrNotFound) {
		return domaincontinuation.Receipt{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, receipt.Payload.ExecutionGrant.ExpiresAt)
	if err != nil || !now.UTC().Before(expiresAt) {
		return domaincontinuation.Receipt{}, ErrReceiptExpired
	}
	return receipt, nil
}

// ResolveTrustedDisposition returns the immutable host-signed approval or
// user-input record pair after validating current execution authority. It is
// intentionally read-only, but its consumers may rely on the pair to approve
// a same-process effect, so audit-only receipts are rejected.
func (service *Service) ResolveTrustedDisposition(ctx context.Context, gateID string) (domaincontinuation.Receipt, domaincontinuation.Disposition, error) {
	receipt, disposition, err := service.ResolveTrustedDispositionForAudit(ctx, gateID)
	if err != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, err
	}
	if validateReceiptForExecution(receipt) != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, errors.New("continuation receipt is not current trusted authority")
	}
	return receipt, disposition, nil
}

// ResolveTrustedDispositionForAudit verifies the installation signature and
// immutable disposition without granting provider/tool execution. Restart
// projection uses this path so legacy V2 and pre-binding V3 records remain
// readable and monotonically closable, but can never resume an effect.
func (service *Service) ResolveTrustedDispositionForAudit(ctx context.Context, gateID string) (domaincontinuation.Receipt, domaincontinuation.Disposition, error) {
	if !service.Available() {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, ErrAuthorityUnavailable
	}
	gateID = strings.TrimSpace(gateID)
	receipt, err := service.store.ResolveReceipt(ctx, gateID)
	if err != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, err
	}
	receipt, err = service.verifyReceipt(ctx, receipt)
	if err != nil || receipt.Payload.GateID != gateID {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, errors.New("continuation receipt is not trusted audit authority")
	}
	disposition, err := service.store.ResolveDisposition(ctx, gateID)
	if err != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, err
	}
	if err := service.verifyDisposition(ctx, disposition, receipt); err != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, err
	}
	return receipt, disposition, nil
}

// ResolveApprovedGrantHost converts one current-installation signed approval
// disposition into the only approved execution grant it can authorize. The
// disposition timestamp is the grant issuance timestamp, so projection repair
// and acknowledgement-loss retries cannot mint a different grant identity.
func (service *Service) ResolveApprovedGrantHost(
	gateID string,
	pending appmodel.PendingToolCall,
) (domaincontinuation.Receipt, domaincontinuation.Disposition, domainsecurity.ExecutionGrant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	receipt, disposition, err := service.ResolveTrustedDisposition(ctx, gateID)
	if err != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, domainsecurity.ExecutionGrant{}, err
	}
	if receipt.Payload.Kind != domaincontinuation.KindApproval ||
		disposition.Status != domaincontinuation.StatusAllowed || disposition.ReasonCode != "approval_allowed" ||
		appturn.ValidatePendingContinuationReceipt(receipt, pending) != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, domainsecurity.ExecutionGrant{}, errors.New("approval disposition does not authorize the pending execution")
	}
	disposedAt, err := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, receipt.Payload.IssuedAt)
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, pending.ExecutionGrant.ExpiresAt)
	if err != nil || issuedErr != nil || expiresErr != nil ||
		disposition.DisposedAt != disposedAt.UTC().Format(time.RFC3339Nano) ||
		receipt.Payload.IssuedAt != issuedAt.UTC().Format(time.RFC3339Nano) ||
		pending.ExecutionGrant.ExpiresAt != expiresAt.UTC().Format(time.RFC3339Nano) ||
		disposedAt.Before(issuedAt) || !disposedAt.Before(expiresAt) {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, domainsecurity.ExecutionGrant{}, errors.New("approval disposition time is invalid")
	}
	approved, err := executiongrantapp.Approve(pending.ExecutionGrant, disposedAt)
	if err != nil {
		return domaincontinuation.Receipt{}, domaincontinuation.Disposition{}, domainsecurity.ExecutionGrant{}, err
	}
	return receipt, disposition, approved, nil
}

func (service *Service) Consume(ctx context.Context, gateID, status, reasonCode string, disposedAt time.Time) (domaincontinuation.Disposition, error) {
	receipt, err := service.VerifyOpen(ctx, gateID, disposedAt)
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	return service.dispose(ctx, receipt, status, reasonCode, disposedAt)
}

// Close is the monotonic downgrade path. It verifies signature and exact-once
// state but deliberately permits an expired receipt to be rejected, denied,
// cancelled, interrupted, or invalidated after restart.
func (service *Service) Close(ctx context.Context, gateID, status, reasonCode string, disposedAt time.Time) (domaincontinuation.Disposition, error) {
	if !service.Available() {
		return domaincontinuation.Disposition{}, ErrAuthorityUnavailable
	}
	receipt, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	receipt, err = service.verifyReceipt(ctx, receipt)
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	if _, err := service.store.ResolveDisposition(ctx, receipt.Payload.GateID); err == nil {
		return domaincontinuation.Disposition{}, ErrReceiptConsumed
	} else if !errors.Is(err, continuationstoreport.ErrNotFound) {
		return domaincontinuation.Disposition{}, err
	}
	return service.dispose(ctx, receipt, status, reasonCode, disposedAt)
}

func (service *Service) dispose(ctx context.Context, receipt domaincontinuation.Receipt, status, reasonCode string, disposedAt time.Time) (domaincontinuation.Disposition, error) {
	disposition, err := domaincontinuation.NewDisposition(receipt, status, reasonCode, disposedAt, service.authority.KeyID(), service.authority.PublicKey(), func(message []byte) ([]byte, error) {
		return service.authority.Sign(ctx, message)
	})
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	if err := service.store.PutDispositionIfAbsent(ctx, disposition); err != nil {
		if existing, resolveErr := service.store.ResolveDisposition(ctx, receipt.Payload.GateID); resolveErr == nil {
			if verifyErr := service.verifyDisposition(ctx, existing, receipt); verifyErr == nil {
				return domaincontinuation.Disposition{}, ErrReceiptConsumed
			}
		}
		return domaincontinuation.Disposition{}, err
	}
	if err := service.verifyDisposition(ctx, disposition, receipt); err != nil {
		return domaincontinuation.Disposition{}, err
	}
	return disposition, nil
}

func (service *Service) verifyReceipt(ctx context.Context, receipt domaincontinuation.Receipt) (domaincontinuation.Receipt, error) {
	keyID, publicKey, signature, err := domaincontinuation.AuthorityMaterial(receipt)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if err := service.authority.VerifyTrusted(ctx, keyID, publicKey, domaincontinuation.ReceiptSigningBytes(receipt), signature); err != nil {
		return domaincontinuation.Receipt{}, errors.New("continuation receipt signature is not trusted")
	}
	return receipt, nil
}

func (service *Service) verifyDisposition(ctx context.Context, disposition domaincontinuation.Disposition, receipt domaincontinuation.Receipt) error {
	if disposition.GateID != receipt.Payload.GateID || disposition.ReceiptID != receipt.ReceiptID || disposition.Kind != receipt.Payload.Kind {
		return errors.New("continuation disposition does not match its receipt")
	}
	keyID, publicKey, signature, err := domaincontinuation.DispositionAuthorityMaterial(disposition)
	if err != nil {
		return err
	}
	if err := service.authority.VerifyTrusted(ctx, keyID, publicKey, domaincontinuation.DispositionSigningBytes(disposition), signature); err != nil {
		return errors.New("continuation disposition signature is not trusted")
	}
	return nil
}
