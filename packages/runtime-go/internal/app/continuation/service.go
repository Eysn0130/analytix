package continuation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
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
	if status == domaincontinuation.StatusContinued && (receipt.Payload.UserInputTimedWait == nil || receipt.Payload.UserInputTimedWait.TimeoutSeconds == -1 || reasonCode != "user_input_continued") {
		return domaincontinuation.Disposition{}, errors.New("continued disposition requires the signed finite timed request")
	}
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

// QuestionRestartOutcomeHostV1 resolves the final real receipt/disposition
// pair after Gate reconciliation, then constructs the existing grant-map proof.
func (service *Service) QuestionRestartOutcomeHostV1(ctx context.Context, inputID string) (executiongrantapp.RestartGrantOutcomeAuthorityV1, error) {
	receipt, disposition, err := service.ResolveTrustedDispositionForAudit(ctx, inputID)
	if err != nil {
		return executiongrantapp.RestartGrantOutcomeAuthorityV1{}, err
	}
	return executiongrantapp.NewQuestionRestartOutcomeAuthorityV1(ctx, service.authority, receipt, disposition)
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
	if disposition.Status == domaincontinuation.StatusContinued && !domaincontinuation.ValidUserInputContinuedReasonV1(receipt.Payload.UserInputTimedWait, disposition.ReasonCode) {
		return errors.New("continued disposition does not match a finite timed request")
	}
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
	if link := receipt.Payload.UserInputReplyCapture; link != nil {
		source, err := service.resolveLateCaptureSource(ctx, receipt)
		if err != nil || disposition.UserInputReplySourceID != link.SourceInputID || disposition.UserInputSubmission == nil || disposition.DisposedAt != link.CapturedAt {
			return errors.New("late capture disposition has no original request authority")
		}
		return validateUserInputSubmissionForReceipt(source, *disposition.UserInputSubmission)
	}
	if disposition.UserInputReplySourceID != "" {
		return errors.New("ordinary disposition cannot carry late capture linkage")
	}
	if disposition.UserInputSubmission != nil {
		return validateUserInputSubmissionForReceipt(receipt, *disposition.UserInputSubmission)
	}
	return nil
}

func (service *Service) resolveLateCaptureSource(ctx context.Context, capture domaincontinuation.Receipt) (domaincontinuation.Receipt, error) {
	link := capture.Payload.UserInputReplyCapture
	if link == nil {
		return domaincontinuation.Receipt{}, errors.New("late capture linkage is missing")
	}
	source, err := service.store.ResolveReceipt(ctx, link.SourceInputID)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	source, err = service.verifyReceipt(ctx, source)
	if err != nil || domaincontinuation.ValidateUserInputReplyCaptureSourceV1(capture, source) != nil {
		return domaincontinuation.Receipt{}, errors.New("late capture source is invalid")
	}
	disposition, err := service.store.ResolveDisposition(ctx, source.Payload.GateID)
	if err != nil || service.verifyDisposition(ctx, disposition, source) != nil || disposition.Status != domaincontinuation.StatusContinued || !domaincontinuation.ValidUserInputContinuedReasonV1(source.Payload.UserInputTimedWait, disposition.ReasonCode) {
		return domaincontinuation.Receipt{}, errors.New("late capture source has not continued")
	}
	capturedAt, capturedErr := time.Parse(time.RFC3339Nano, link.CapturedAt)
	continuedAt, continuedErr := time.Parse(time.RFC3339Nano, disposition.DisposedAt)
	if capturedErr != nil || continuedErr != nil || capturedAt.Before(continuedAt) {
		return domaincontinuation.Receipt{}, errors.New("late capture predates continued request")
	}
	return source, nil
}

// UserInputLocalDisplayBindingV1 is supplied by the existing local-display view
// authority. It is internal and must never be accepted directly from a client.
type UserInputLocalDisplayBindingV1 struct {
	ThreadID, TurnID, ItemID, InputID, ReceiptID string
	SecurityContext                              domainsecurity.TurnSecurityContext
}

// ResolveUserInputLocalDisplayOriginHost verifies immutable request identity
// only. It neither requires a disposition nor acquires execution authority.
func (service *Service) ResolveUserInputLocalDisplayOriginHost(ctx context.Context, inputID string) (UserInputLocalDisplayBindingV1, error) {
	if !service.Available() || ctx == nil || ctx.Err() != nil {
		return UserInputLocalDisplayBindingV1{}, ErrAuthorityUnavailable
	}
	receipt, err := service.store.ResolveReceipt(ctx, inputID)
	if err == nil {
		receipt, err = service.verifyReceipt(ctx, receipt)
	}
	if err != nil || receipt.Payload.Kind != domaincontinuation.KindUserInput || receipt.Payload.GateID != inputID ||
		receipt.Payload.UserInputReplyCapture != nil || receipt.Payload.UserInputDraft != nil {
		return UserInputLocalDisplayBindingV1{}, errors.New("private user input origin is unavailable")
	}
	return UserInputLocalDisplayBindingV1{ThreadID: receipt.Payload.ThreadID, TurnID: receipt.Payload.TurnID, ItemID: receipt.Payload.ItemID,
		InputID: inputID, ReceiptID: receipt.ReceiptID, SecurityContext: receipt.Payload.SecurityContext}, nil
}

// ReadUserInputHistoryHost reads only original signed arguments under an
// already acquired original-owner display scope. Later drafts, answers and
// delivery records cannot enter an earlier inherited prefix.
func (service *Service) ReadUserInputHistoryHost(ctx context.Context, binding UserInputLocalDisplayBindingV1) (UserInputLocalDisplayV1, error) {
	origin, err := service.ResolveUserInputLocalDisplayOriginHost(ctx, binding.InputID)
	if err != nil || !reflect.DeepEqual(origin, binding) {
		return UserInputLocalDisplayV1{}, errors.New("private user input history is unavailable")
	}
	receipt, err := service.store.ResolveReceipt(ctx, binding.InputID)
	if err == nil {
		receipt, err = service.verifyReceipt(ctx, receipt)
	}
	if err != nil || receipt.ReceiptID != binding.ReceiptID {
		return UserInputLocalDisplayV1{}, errors.New("private user input history is unavailable")
	}
	return UserInputLocalDisplayV1{ToolName: receipt.Payload.ToolName, CallID: receipt.Payload.CallID,
		ThreadID: binding.ThreadID, TurnID: binding.TurnID, ItemID: binding.ItemID, InputID: binding.InputID, ReceiptID: binding.ReceiptID,
		Arguments: append(json.RawMessage(nil), receipt.Payload.Arguments...), Status: "pending"}, nil
}

type UserInputLateReplyDeliveryV1 struct {
	ClientUserMessageID string `json:"clientUserMessageId"`
	TurnID              string `json:"turnId"`
	State               string `json:"state"`
}

type UserInputLocalDisplayV1 struct {
	DraftRevision                                *string
	Draft                                        *domaincontinuation.UserInputDraftSnapshotV1
	CallID                                       string
	ToolName                                     string
	ContinuedReason                              *string
	LateReply                                    *UserInputLateReplyDeliveryV1
	TimedWait                                    *domaincontinuation.UserInputTimedWaitV1
	IssuedAt                                     string
	ThreadID, TurnID, ItemID, InputID, ReceiptID string
	Arguments                                    json.RawMessage
	Status, AnswerState                          string
	Submission                                   *domaincontinuation.UserInputSubmissionV1
}

// ReadUserInputLocalDisplayHost reads a trusted request/disposition pair after
// the caller's current view authority has admitted the exact item. It grants no
// execution, ignores execution-grant expiry, and never returns grants or context.
func (service *Service) ReadUserInputLocalDisplayHost(ctx context.Context, binding UserInputLocalDisplayBindingV1) (UserInputLocalDisplayV1, error) {
	unavailable := func() (UserInputLocalDisplayV1, error) {
		return UserInputLocalDisplayV1{}, errors.New("private user input display is unavailable")
	}
	if !service.Available() || ctx == nil || ctx.Err() != nil || binding.ReceiptID == "" {
		return unavailable()
	}
	receipt, err := service.store.ResolveReceipt(ctx, binding.InputID)
	if err != nil {
		return unavailable()
	}
	receipt, err = service.verifyReceipt(ctx, receipt)
	if err != nil || receipt.Payload.UserInputReplyCapture != nil || receipt.Payload.Kind != domaincontinuation.KindUserInput || receipt.ReceiptID != binding.ReceiptID ||
		receipt.Payload.GateID != binding.InputID || receipt.Payload.ThreadID != binding.ThreadID || receipt.Payload.TurnID != binding.TurnID ||
		receipt.Payload.ItemID != binding.ItemID || !reflect.DeepEqual(receipt.Payload.SecurityContext, binding.SecurityContext) {
		return unavailable()
	}
	result := UserInputLocalDisplayV1{ToolName: receipt.Payload.ToolName, CallID: receipt.Payload.CallID, ThreadID: binding.ThreadID, TurnID: binding.TurnID, ItemID: binding.ItemID, InputID: binding.InputID, ReceiptID: binding.ReceiptID,
		Arguments: append(json.RawMessage(nil), receipt.Payload.Arguments...), Status: "pending", TimedWait: domaincontinuation.CloneUserInputTimedWaitV1(receipt.Payload.UserInputTimedWait), IssuedAt: receipt.Payload.IssuedAt}
	disposition, err := service.store.ResolveDisposition(ctx, binding.InputID)
	if errors.Is(err, continuationstoreport.ErrNotFound) {
		if service.attachUserInputDraftV1(ctx, receipt, &result) != nil {
			return unavailable()
		}
		return result, nil
	}
	if err != nil || service.verifyDisposition(ctx, disposition, receipt) != nil {
		return unavailable()
	}
	if projection, ok := domaincontinuation.ResolveProjection(disposition.Kind, disposition.Status, disposition.ReasonCode); ok {
		result.Status = projection.PublicStatus
	} else {
		switch disposition.Status {
		case domaincontinuation.StatusRejected, domaincontinuation.StatusInterrupted, domaincontinuation.StatusRestartInvalid:
			result.Status = "cancelled"
		default:
			return unavailable()
		}
	}
	if result.Status == "continued" {
		reason := disposition.ReasonCode
		result.ContinuedReason = &reason
		result.AnswerState = "unavailable"
	}
	if result.Status == "submitted" {
		result.AnswerState = "unavailable"
		if disposition.UserInputSubmission != nil {
			copy := domaincontinuation.CloneUserInputSubmissionV1(*disposition.UserInputSubmission)
			result.Submission, result.AnswerState = &copy, "available"
		}
	}
	if service.attachUserInputDraftV1(ctx, receipt, &result) != nil {
		return unavailable()
	}
	return result, nil
}

// DisposePendingUserInputHost commits original answers and submitted state in
// the same signed disposition CAS record. A write error remains an error even
// if the record became visible: this attempt must not acknowledge or continue.
func (service *Service) DisposePendingUserInputHost(gateID string, at time.Time, pending appmodel.PendingToolCall, submission domaincontinuation.UserInputSubmissionV1) (domaincontinuation.Disposition, error) {
	if !service.Available() {
		return domaincontinuation.Disposition{}, ErrAuthorityUnavailable
	}
	submission = domaincontinuation.CloneUserInputSubmissionV1(submission)
	ctx, cancel := context.WithTimeout(context.Background(), hostOperationTimeout)
	defer cancel()
	receipt, err := service.store.ResolveReceipt(ctx, strings.TrimSpace(gateID))
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	receipt, err = service.verifyReceipt(ctx, receipt)
	if err == nil {
		err = validateReceiptForExecution(receipt)
	}
	if err == nil {
		err = appturn.ValidatePendingContinuationReceipt(receipt, pending)
	}
	if err == nil {
		err = validateUserInputSubmissionForReceipt(receipt, submission)
	}
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	if existing, resolveErr := service.store.ResolveDisposition(ctx, receipt.Payload.GateID); resolveErr == nil {
		if service.verifyDisposition(ctx, existing, receipt) == nil && existing.Status == domaincontinuation.StatusSubmitted && existing.ReasonCode == "user_input_submitted" &&
			existing.UserInputSubmission != nil && userInputSubmissionsEqual(*existing.UserInputSubmission, submission) {
			return existing, nil
		}
		return domaincontinuation.Disposition{}, ErrReceiptConsumed
	} else if !errors.Is(resolveErr, continuationstoreport.ErrNotFound) {
		return domaincontinuation.Disposition{}, resolveErr
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, receipt.Payload.ExecutionGrant.ExpiresAt)
	if err != nil || !at.UTC().Before(expiresAt) {
		return domaincontinuation.Disposition{}, ErrReceiptExpired
	}
	disposition, err := domaincontinuation.NewUserInputSubmittedDisposition(receipt, submission, at, service.authority.KeyID(), service.authority.PublicKey(), func(message []byte) ([]byte, error) { return service.authority.Sign(ctx, message) })
	if err != nil {
		return domaincontinuation.Disposition{}, err
	}
	if err := service.store.PutDispositionIfAbsent(ctx, disposition); err != nil {
		return domaincontinuation.Disposition{}, err
	}
	if err := service.verifyDisposition(ctx, disposition, receipt); err != nil {
		return domaincontinuation.Disposition{}, err
	}
	return disposition, nil
}

func userInputSubmissionsEqual(left, right domaincontinuation.UserInputSubmissionV1) bool {
	if left.SchemaVersion != right.SchemaVersion || left.Format != right.Format {
		return false
	}
	var leftValue, rightValue any
	return json.Unmarshal(left.Answers, &leftValue) == nil && json.Unmarshal(right.Answers, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func validateUserInputSubmissionForReceipt(receipt domaincontinuation.Receipt, submission domaincontinuation.UserInputSubmissionV1) error {
	invalid := func() error { return errors.New("private user input submission does not match its signed request") }
	if receipt.Payload.Kind != domaincontinuation.KindUserInput || domaincontinuation.ValidateUserInputSubmissionV1(submission) != nil {
		return invalid()
	}
	args := map[string]any{}
	if json.Unmarshal(receipt.Payload.Arguments, &args) != nil {
		return invalid()
	}
	prompt := "User input required"
	for _, key := range []string{"prompt", "question"} {
		if value, ok := args[key].(string); ok && strings.TrimSpace(value) != "" {
			prompt = strings.TrimSpace(value)
			break
		}
	}
	args, err := toolcatalogapp.UserInputArgumentsForTool(receipt.Payload.ToolName, args)
	if err != nil {
		return invalid()
	}
	questions := toolcatalogapp.UserInputQuestions(args, receipt.Payload.GateID, prompt)
	legacyQuestionID := ""
	if raw, ok := args["questions"].([]any); ok && len(raw) == 1 && len(questions) == 1 {
		if question, ok := raw[0].(map[string]any); ok {
			legacyQuestionID, _ = question["id"].(string)
		}
	}
	request := controlapp.UserInputResponse{}
	if submission.Format == "structured" {
		var records []domaincontinuation.UserInputStructuredAnswerV1
		if json.Unmarshal(submission.Answers, &records) != nil {
			return invalid()
		}
		request.StructuredAnswers = make([]controlapp.UserInputAnswer, 0, len(records))
		for _, record := range records {
			answer := controlapp.UserInputAnswer{ID: record.ID, Kind: record.Kind, CustomText: record.CustomText}
			if record.OptionIDs != nil {
				answer.OptionIDs = *record.OptionIDs
			}
			request.StructuredAnswers = append(request.StructuredAnswers, answer)
		}
	} else {
		var records []domaincontinuation.UserInputLegacyAnswerV1
		if json.Unmarshal(submission.Answers, &records) != nil {
			return invalid()
		}
		request.Answers = make([]map[string]string, 0, len(records))
		for _, record := range records {
			answer := map[string]string{"id": record.ID, "value": record.Value}
			if record.Label != nil {
				answer["label"] = *record.Label
			}
			request.Answers = append(request.Answers, answer)
		}
	}
	_, captured, err := controlapp.NormalizeAndCaptureUserInputAnswers(request, questions, legacyQuestionID)
	if err != nil || captured == nil || !userInputSubmissionsEqual(*captured, submission) {
		return invalid()
	}
	return nil
}

// CaptureLateUserInputSubmissionHost uses the existing private receipt and
// disposition CAS for one stable attempt. A write error never acknowledges
// this attempt, even when the record became visible; an explicit retry may
// subsequently verify and reuse it. No queue effect is authorized here.
func (service *Service) CaptureLateUserInputSubmissionHost(ctx context.Context, binding UserInputLocalDisplayBindingV1, attemptID string, submission domaincontinuation.UserInputSubmissionV1) (domaincontinuation.Receipt, error) {
	if !service.Available() || ctx == nil || !controlapp.IsOpaqueClientUserMessageID(attemptID) {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	display, err := service.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || display.Status != "continued" || display.ContinuedReason == nil || !domaincontinuation.ValidUserInputContinuedReasonV1(display.TimedWait, *display.ContinuedReason) {
		return domaincontinuation.Receipt{}, errors.New("late user input request is unavailable")
	}
	source, err := service.store.ResolveReceipt(ctx, binding.InputID)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	source, err = service.verifyReceipt(ctx, source)
	if err != nil || validateUserInputSubmissionForReceipt(source, submission) != nil {
		return domaincontinuation.Receipt{}, errors.New("late user input submission is invalid")
	}
	captureID := domaincontinuation.UserInputReplyCaptureGateIDV1(binding.InputID, binding.ReceiptID, attemptID)
	capture, err := service.store.ResolveReceipt(ctx, captureID)
	if errors.Is(err, continuationstoreport.ErrNotFound) {
		capture, err = domaincontinuation.NewUserInputReplyCaptureReceiptV1(source, attemptID, time.Now().UTC(), service.authority.KeyID(), service.authority.PublicKey(), func(body []byte) ([]byte, error) { return service.authority.Sign(ctx, body) })
		if err != nil {
			return domaincontinuation.Receipt{}, err
		}
		if err = service.store.PutReceiptIfAbsent(ctx, capture); err != nil {
			return domaincontinuation.Receipt{}, err
		}
		stored, readErr := service.store.ResolveReceipt(ctx, captureID)
		if readErr != nil || !reflect.DeepEqual(stored, capture) {
			return domaincontinuation.Receipt{}, errors.New("late capture receipt readback failed")
		}
		capture = stored
	} else if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	capture, err = service.verifyReceipt(ctx, capture)
	if err != nil || domaincontinuation.ValidateUserInputReplyCaptureSourceV1(capture, source) != nil || capture.Payload.UserInputReplyCapture.ClientUserMessageID != attemptID {
		return domaincontinuation.Receipt{}, errors.New("late capture authority conflicts")
	}
	if _, err = service.resolveLateCaptureSource(ctx, capture); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	existing, err := service.store.ResolveDisposition(ctx, captureID)
	if err == nil {
		if service.verifyDisposition(ctx, existing, capture) != nil || existing.UserInputSubmission == nil || !userInputSubmissionsEqual(*existing.UserInputSubmission, submission) {
			return domaincontinuation.Receipt{}, ErrReceiptConsumed
		}
		return capture, nil
	}
	if !errors.Is(err, continuationstoreport.ErrNotFound) {
		return domaincontinuation.Receipt{}, err
	}
	capturedAt, err := time.Parse(time.RFC3339Nano, capture.Payload.UserInputReplyCapture.CapturedAt)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	disposition, err := domaincontinuation.NewUserInputSubmittedDisposition(capture, submission, capturedAt, service.authority.KeyID(), service.authority.PublicKey(), func(body []byte) ([]byte, error) { return service.authority.Sign(ctx, body) })
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if err = service.store.PutDispositionIfAbsent(ctx, disposition); err != nil {
		return domaincontinuation.Receipt{}, err
	}
	stored, err := service.store.ResolveDisposition(ctx, captureID)
	if err != nil || !reflect.DeepEqual(stored, disposition) || service.verifyDisposition(ctx, stored, capture) != nil {
		return domaincontinuation.Receipt{}, errors.New("late capture submission readback failed")
	}
	return capture, nil
}

// ReadLateUserInputSubmissionHost is callable only under the original
// Question's existing private display authority. Projected queue text is never
// used to reconstruct raw answers.
func (service *Service) ReadLateUserInputSubmissionHost(ctx context.Context, binding UserInputLocalDisplayBindingV1, attemptID, captureReceiptID string) (domaincontinuation.UserInputSubmissionV1, error) {
	if !controlapp.IsOpaqueClientUserMessageID(attemptID) || !domainsecurity.IsSHA256Hex(captureReceiptID) {
		return domaincontinuation.UserInputSubmissionV1{}, errors.New("late submission is unavailable")
	}
	display, err := service.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || display.Status != "continued" {
		return domaincontinuation.UserInputSubmissionV1{}, errors.New("late submission is unavailable")
	}
	captureID := domaincontinuation.UserInputReplyCaptureGateIDV1(binding.InputID, binding.ReceiptID, attemptID)
	capture, err := service.store.ResolveReceipt(ctx, captureID)
	if err != nil {
		return domaincontinuation.UserInputSubmissionV1{}, err
	}
	capture, err = service.verifyReceipt(ctx, capture)
	if err != nil || capture.ReceiptID != captureReceiptID || capture.Payload.UserInputReplyCapture == nil || capture.Payload.UserInputReplyCapture.ClientUserMessageID != attemptID {
		return domaincontinuation.UserInputSubmissionV1{}, errors.New("late submission is unavailable")
	}
	source, err := service.resolveLateCaptureSource(ctx, capture)
	if err != nil || source.ReceiptID != binding.ReceiptID || source.Payload.GateID != binding.InputID {
		return domaincontinuation.UserInputSubmissionV1{}, errors.New("late submission is unavailable")
	}
	disposition, err := service.store.ResolveDisposition(ctx, captureID)
	if err != nil || service.verifyDisposition(ctx, disposition, capture) != nil || disposition.UserInputSubmission == nil {
		return domaincontinuation.UserInputSubmissionV1{}, errors.New("late submission is unavailable")
	}
	return domaincontinuation.CloneUserInputSubmissionV1(*disposition.UserInputSubmission), nil
}

type UserInputLateCapturedSubmissionV1 struct {
	AttemptID, CaptureReceiptID, CapturedAt string
	Submission                              domaincontinuation.UserInputSubmissionV1
}

// ReadLatestLateUserInputSubmissionHost retains editable raw answers even
// when steering admission fails. Absence of delivery proof must never be
// reconstructed into queued or accepted state.
func (service *Service) ReadLatestLateUserInputSubmissionHost(ctx context.Context, binding UserInputLocalDisplayBindingV1) (*UserInputLateCapturedSubmissionV1, error) {
	original, err := service.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || original.Status != "continued" {
		return nil, errors.New("late raw capture is unavailable")
	}
	source, err := service.store.ResolveReceipt(ctx, binding.InputID)
	if err != nil {
		return nil, err
	}
	source, err = service.verifyReceipt(ctx, source)
	if err != nil || source.ReceiptID != binding.ReceiptID {
		return nil, ErrAuthorityUnavailable
	}
	return service.readLatestLateUserInputForSourceV1(ctx, source)
}

func (service *Service) readLatestLateUserInputForSourceV1(ctx context.Context, source domaincontinuation.Receipt) (*UserInputLateCapturedSubmissionV1, error) {
	sourceReceiptID := source.ReceiptID
	inventory, ok := service.store.(continuationstoreport.InventoryStore)
	if !ok {
		return nil, errors.New("late raw capture inventory is unavailable")
	}
	var latest *UserInputLateCapturedSubmissionV1
	err := inventory.VisitReceipts(ctx, func(capture domaincontinuation.Receipt) error {
		link := capture.Payload.UserInputReplyCapture
		if link == nil || link.SourceInputID != source.Payload.GateID {
			return nil
		}
		capture, err := service.verifyReceipt(ctx, capture)
		if err != nil {
			return err
		}
		source, err := service.resolveLateCaptureSource(ctx, capture)
		if err != nil || source.ReceiptID != sourceReceiptID {
			return errors.New("late raw capture source conflicts")
		}
		disposition, err := service.store.ResolveDisposition(ctx, capture.Payload.GateID)
		if errors.Is(err, continuationstoreport.ErrNotFound) {
			return nil
		}
		if err != nil || service.verifyDisposition(ctx, disposition, capture) != nil || disposition.UserInputSubmission == nil {
			return errors.New("late raw capture disposition conflicts")
		}
		capturedAt, err := time.Parse(time.RFC3339Nano, link.CapturedAt)
		if err != nil {
			return err
		}
		if latest != nil {
			priorAt, err := time.Parse(time.RFC3339Nano, latest.CapturedAt)
			if err != nil {
				return err
			}
			if !capturedAt.After(priorAt) {
				return nil
			}
		}
		latest = &UserInputLateCapturedSubmissionV1{AttemptID: link.ClientUserMessageID, CaptureReceiptID: capture.ReceiptID, CapturedAt: link.CapturedAt, Submission: domaincontinuation.CloneUserInputSubmissionV1(*disposition.UserInputSubmission)}
		return nil
	})
	return latest, err
}

var (
	ErrUserInputDraftConflictV1 = errors.New("private Question draft revision conflicts")
	ErrUserInputDraftCapacityV1 = errors.New("private Question draft capacity exceeded")
	ErrUserInputDraftInvalidV1  = errors.New("private Question draft binding is invalid")
)

// ValidateUserInputDraftForReceiptV1 binds partial state to the same Host
// question/option projection used by private display and submitted answers.
func ValidateUserInputDraftForReceiptV1(receipt domaincontinuation.Receipt, snapshot domaincontinuation.UserInputDraftSnapshotV1) error {
	if receipt.Payload.Kind != domaincontinuation.KindUserInput || receipt.Payload.UserInputReplyCapture != nil || receipt.Payload.UserInputDraft != nil || domaincontinuation.ValidateUserInputDraftSnapshotV1(snapshot, receipt.Payload.GateID) != nil {
		return ErrUserInputDraftInvalidV1
	}
	var args map[string]any
	if json.Unmarshal(receipt.Payload.Arguments, &args) != nil {
		return ErrUserInputDraftInvalidV1
	}
	args, err := toolcatalogapp.UserInputArgumentsForTool(receipt.Payload.ToolName, args)
	if err != nil {
		return err
	}
	questions := toolcatalogapp.UserInputQuestionsForLocalDisplay(args, receipt.Payload.GateID, "User input required")
	if len(questions) < 1 || len(questions) > 3 || snapshot.CurrentPage >= len(questions) {
		return ErrUserInputDraftInvalidV1
	}
	for _, answer := range snapshot.Answers {
		var selected map[string]any
		for _, question := range questions {
			if question["id"] == answer.QuestionID {
				selected = question
				break
			}
		}
		if selected == nil {
			return ErrUserInputDraftInvalidV1
		}
		multi, ok := selected["multiSelect"].(bool)
		options, optionsOK := selected["options"].([]map[string]string)
		if !ok || !optionsOK || (!multi && len(answer.OptionIDs)+boolIntDraftV1(strings.TrimSpace(answer.CustomText) != "") > 1) {
			return ErrUserInputDraftInvalidV1
		}
		for _, id := range answer.OptionIDs {
			found := false
			for _, option := range options {
				if option["id"] == id {
					found = true
					break
				}
			}
			if !found {
				return ErrUserInputDraftInvalidV1
			}
		}
	}
	return nil
}

func boolIntDraftV1(value bool) int {
	if value {
		return 1
	}
	return 0
}

func equalDraftParentV1(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func draftSnapshotBytesV1(snapshot domaincontinuation.UserInputDraftSnapshotV1) int {
	body, _ := json.Marshal(snapshot)
	return len(body)
}

// Every parent has one deterministic slot in the existing immutable receipt
// CAS. Following those slots yields one current snapshot without a head store.
func (service *Service) readUserInputDraftChainV1(ctx context.Context, source domaincontinuation.Receipt) ([]domaincontinuation.Receipt, int, error) {
	var parent *string
	chain := []domaincontinuation.Receipt{}
	seen := map[string]bool{}
	bytesRetained := 0
	at, err := time.Parse(time.RFC3339Nano, source.Payload.IssuedAt)
	if err != nil {
		return nil, 0, ErrUserInputDraftInvalidV1
	}
	for {
		receipt, err := service.store.ResolveReceipt(ctx, domaincontinuation.UserInputDraftSlotGateIDV1(source.ReceiptID, parent))
		if errors.Is(err, continuationstoreport.ErrNotFound) {
			return chain, bytesRetained, nil
		}
		if err != nil {
			return nil, 0, err
		}
		receipt, err = service.verifyReceipt(ctx, receipt)
		link := receipt.Payload.UserInputDraft
		if err != nil || domaincontinuation.ValidateUserInputDraftSourceV1(receipt, source) != nil || link == nil || !equalDraftParentV1(link.ParentRevision, parent) || seen[link.Revision] || ValidateUserInputDraftForReceiptV1(source, link.Snapshot) != nil {
			return nil, 0, ErrUserInputDraftInvalidV1
		}
		savedAt, err := time.Parse(time.RFC3339Nano, receipt.Payload.IssuedAt)
		if err != nil || savedAt.Before(at) {
			return nil, 0, ErrUserInputDraftInvalidV1
		}
		if _, err := service.store.ResolveDisposition(ctx, receipt.Payload.GateID); !errors.Is(err, continuationstoreport.ErrNotFound) {
			return nil, 0, ErrUserInputDraftInvalidV1
		}
		bytesRetained += draftSnapshotBytesV1(link.Snapshot)
		if len(chain) == domaincontinuation.MaxUserInputDraftRevisionsV1 || bytesRetained > domaincontinuation.MaxUserInputDraftRetainedBytesV1 {
			return nil, 0, ErrUserInputDraftCapacityV1
		}
		chain = append(chain, receipt)
		seen[link.Revision] = true
		value := link.Revision
		parent = &value
		at = savedAt
	}
}

func (service *Service) attachUserInputDraftV1(ctx context.Context, source domaincontinuation.Receipt, display *UserInputLocalDisplayV1) error {
	chain, _, err := service.readUserInputDraftChainV1(ctx, source)
	if err != nil {
		return err
	}
	if len(chain) == 0 {
		return nil
	}
	latest := chain[len(chain)-1]
	revision := latest.Payload.UserInputDraft.Revision
	display.DraftRevision = &revision
	if display.Status == "pending" || display.Status == "continued" {
		captureSet, err := service.userInputDraftCaptureSetV1(ctx, source)
		if err != nil {
			return err
		}
		if latest.Payload.UserInputDraft.CaptureSetSHA256 != captureSet {
			return nil
		}
		snapshot := domaincontinuation.CloneUserInputDraftSnapshotV1(latest.Payload.UserInputDraft.Snapshot)
		display.Draft = &snapshot
	}
	return nil
}

// SaveUserInputDraftHost is private metadata only. A failed write/readback
// never acknowledges a save, even when its slot became visible. Exact retry
// can later verify that same immutable record; it cannot produce another slot.
func (service *Service) SaveUserInputDraftHost(ctx context.Context, binding UserInputLocalDisplayBindingV1, revision string, parent *string, snapshot domaincontinuation.UserInputDraftSnapshotV1) (domaincontinuation.Receipt, error) {
	if !service.Available() || ctx == nil || ctx.Err() != nil || !domaincontinuation.IsUserInputDraftRevisionV1(revision) || parent != nil && (!domaincontinuation.IsUserInputDraftRevisionV1(*parent) || *parent == revision) {
		return domaincontinuation.Receipt{}, ErrUserInputDraftInvalidV1
	}
	display, err := service.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || (display.Status != "pending" && display.Status != "continued") {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	source, err := service.store.ResolveReceipt(ctx, binding.InputID)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	source, err = service.verifyReceipt(ctx, source)
	if err != nil || source.ReceiptID != binding.ReceiptID || ValidateUserInputDraftForReceiptV1(source, snapshot) != nil {
		return domaincontinuation.Receipt{}, ErrUserInputDraftInvalidV1
	}
	chain, retained, err := service.readUserInputDraftChainV1(ctx, source)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	for _, receipt := range chain {
		link := receipt.Payload.UserInputDraft
		if link.Revision != revision {
			continue
		}
		if !equalDraftParentV1(link.ParentRevision, parent) || !reflect.DeepEqual(link.Snapshot, snapshot) {
			return domaincontinuation.Receipt{}, ErrUserInputDraftConflictV1
		}
		captureSet, err := service.userInputDraftCaptureSetV1(ctx, source)
		if err != nil || link.CaptureSetSHA256 != captureSet {
			return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
		}
		return receipt, nil
	}
	var latest *string
	if len(chain) != 0 {
		value := chain[len(chain)-1].Payload.UserInputDraft.Revision
		latest = &value
	}
	if !equalDraftParentV1(latest, parent) {
		return domaincontinuation.Receipt{}, ErrUserInputDraftConflictV1
	}
	if len(chain) == domaincontinuation.MaxUserInputDraftRevisionsV1 || retained+draftSnapshotBytesV1(snapshot) > domaincontinuation.MaxUserInputDraftRetainedBytesV1 {
		return domaincontinuation.Receipt{}, ErrUserInputDraftCapacityV1
	}
	at := time.Now().UTC()
	if len(chain) != 0 {
		previous, err := time.Parse(time.RFC3339Nano, chain[len(chain)-1].Payload.IssuedAt)
		if err != nil || at.Before(previous) {
			return domaincontinuation.Receipt{}, ErrUserInputDraftInvalidV1
		}
	}
	captureSet, err := service.userInputDraftCaptureSetV1(ctx, source)
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	receipt, err := domaincontinuation.NewUserInputDraftReceiptV1(source, revision, parent, snapshot, captureSet, at, service.authority.KeyID(), service.authority.PublicKey(), func(body []byte) ([]byte, error) { return service.authority.Sign(ctx, body) })
	if err != nil {
		return domaincontinuation.Receipt{}, err
	}
	if err := service.store.PutReceiptIfAbsent(ctx, receipt); err != nil {
		// Distinguish a real competing child from a write failure. A matching
		// newly visible child still requires an explicit retry to acknowledge.
		winner, readErr := service.store.ResolveReceipt(ctx, receipt.Payload.GateID)
		if readErr == nil {
			winner, verifyErr := service.verifyReceipt(ctx, winner)
			if verifyErr == nil && domaincontinuation.ValidateUserInputDraftSourceV1(winner, source) == nil && winner.Payload.UserInputDraft != nil && (!equalDraftParentV1(winner.Payload.UserInputDraft.ParentRevision, parent) || winner.Payload.UserInputDraft.Revision != revision || !reflect.DeepEqual(winner.Payload.UserInputDraft.Snapshot, snapshot)) {
				return domaincontinuation.Receipt{}, ErrUserInputDraftConflictV1
			}
		}
		return domaincontinuation.Receipt{}, err
	}
	stored, err := service.store.ResolveReceipt(ctx, receipt.Payload.GateID)
	if err != nil || !reflect.DeepEqual(stored, receipt) {
		return domaincontinuation.Receipt{}, errors.New("private draft receipt readback failed")
	}
	stored, err = service.verifyReceipt(ctx, stored)
	if err != nil || domaincontinuation.ValidateUserInputDraftSourceV1(stored, source) != nil {
		return domaincontinuation.Receipt{}, ErrUserInputDraftInvalidV1
	}
	final, err := service.ReadUserInputLocalDisplayHost(ctx, binding)
	if err != nil || (final.Status != "pending" && final.Status != "continued") || final.Draft == nil || ctx.Err() != nil {
		return domaincontinuation.Receipt{}, ErrAuthorityUnavailable
	}
	return stored, nil
}

// The complete verified capture set is a causal clearing witness. Comparing
// only the latest capture timestamp loses a later completion of an older
// receipt; every newly completed capture must invalidate an earlier snapshot.
func (service *Service) userInputDraftCaptureSetV1(ctx context.Context, source domaincontinuation.Receipt) (string, error) {
	inventory, ok := service.store.(continuationstoreport.InventoryStore)
	if !ok {
		return "", ErrUserInputDraftInvalidV1
	}
	ids := []string{}
	err := inventory.VisitReceipts(ctx, func(capture domaincontinuation.Receipt) error {
		link := capture.Payload.UserInputReplyCapture
		if link == nil || link.SourceInputID != source.Payload.GateID {
			return nil
		}
		capture, err := service.verifyReceipt(ctx, capture)
		if err != nil || domaincontinuation.ValidateUserInputReplyCaptureSourceV1(capture, source) != nil {
			return ErrUserInputDraftInvalidV1
		}
		if _, err := service.resolveLateCaptureSource(ctx, capture); err != nil {
			return err
		}
		disposition, err := service.store.ResolveDisposition(ctx, capture.Payload.GateID)
		if errors.Is(err, continuationstoreport.ErrNotFound) {
			return nil
		}
		if err != nil || service.verifyDisposition(ctx, disposition, capture) != nil || disposition.UserInputSubmission == nil {
			return ErrUserInputDraftInvalidV1
		}
		ids = append(ids, capture.ReceiptID)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(ids)
	body, err := json.Marshal(ids)
	if err != nil {
		return "", err
	}
	return domainsecurity.SHA256Hex(append([]byte("analytix/private-question-draft-capture-set/v1\x00"), body...)), nil
}
