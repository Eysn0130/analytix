package pendingwork

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

const (
	reportStagePayloadPurpose = "report_stage.semantic_request"
	reportStageRouteIdentity  = "analytix/pending-work-route/report-stage/v1"
	ReportStageToolName       = "stage_case_report"
)

// ReportStageRequest contains only host-issued identities and hashes. Report
// bytes, claims, PII, prompts, and reasoning cannot enter the durable pending
// work authority through this contract.
type ReportStageRequest struct {
	PendingToolCall appmodel.PendingToolCall
	StageInputHash  string
	IssuedAt        time.Time
}

// ReportStageLease keeps the exact semantic request in process while the
// durable store retains only its keyed digest and signed context/grant refs.
type ReportStageLease struct {
	workID  string
	request ReportStageRequest
}

func (lease ReportStageLease) WorkID() string { return lease.workID }

func (service *Service) BeginReportStage(ctx context.Context, request ReportStageRequest) (ReportStageLease, error) {
	if service.restartOwnsThread(request.PendingToolCall.SecurityContext.ThreadID) {
		return ReportStageLease{}, ErrRestartPreserved
	}
	securityContext, reference, payload, expiresAt, err := reportStageAuthority(request)
	if err != nil {
		return ReportStageLease{}, err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, reportStagePayloadPurpose, payload)
	if err != nil {
		return ReportStageLease{}, err
	}
	issuedAt := request.IssuedAt.UTC()
	if issuedAt.IsZero() {
		return ReportStageLease{}, ErrOperationMismatch
	}
	receipt, err := service.issueExclusive(ctx, IssueInput{
		Kind: domainpendingwork.KindReportStage, SecurityContext: securityContext, GrantReferences: []GrantReference{reference},
		PayloadHash: payloadHash, RouteHash: reportStageRouteHash(), IssuedAt: issuedAt, ExpiresAt: expiresAt,
	})
	if err != nil {
		return ReportStageLease{}, err
	}
	request.IssuedAt = issuedAt
	return ReportStageLease{workID: receipt.WorkID, request: request}, nil
}

// VerifyReportStageRequest re-derives the exact stage identity immediately
// before any staging side effect. A valid lease cannot authorize changed
// claim/input hashes, arguments, grants, context, or tool routes.
func (service *Service) VerifyReportStageRequest(ctx context.Context, lease ReportStageLease, request ReportStageRequest, now time.Time) error {
	if strings.TrimSpace(lease.workID) == "" || !sameReportStageRequest(lease.request, request) {
		return ErrOperationMismatch
	}
	securityContext, _, payload, _, err := reportStageAuthority(request)
	if err != nil {
		return err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, reportStagePayloadPurpose, payload)
	if err != nil {
		return err
	}
	_, err = service.VerifyOpenFor(ctx, lease.workID, securityContext, domainpendingwork.KindReportStage, payloadHash, reportStageRouteHash(), now)
	return err
}

// ResolveReportStageLease returns the exact signed receipt behind a currently
// open lease. Callers use the complete canonical receipt bytes to bind a
// durable outbox plan; a WorkID string alone is not sufficient authority.
func (service *Service) ResolveReportStageLease(ctx context.Context, lease ReportStageLease, request ReportStageRequest) (domainpendingwork.PendingWorkReceiptV1, error) {
	receipt, disposition, err := service.reportStageLeaseRecord(ctx, lease, request)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, err
	}
	if disposition != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, ErrWorkClosed
	}
	receipt.GrantMembers = append([]domainpendingwork.GrantMemberV1(nil), receipt.GrantMembers...)
	return receipt, nil
}

// CloseReportStageLease binds every terminal disposition to the same exact
// request. Completed remains impossible until the grant has a durable tool
// settlement; this function does not treat a file path or renderer success as
// publication authority.
func (service *Service) CloseReportStageLease(ctx context.Context, lease ReportStageLease, request ReportStageRequest, status string, disposedAt time.Time) (domainpendingwork.PendingWorkDispositionV1, error) {
	if strings.TrimSpace(lease.workID) == "" || !sameReportStageRequest(lease.request, request) {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrOperationMismatch
	}
	receipt, existing, err := service.reportStageLeaseRecord(ctx, lease, request)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	status = strings.TrimSpace(status)
	if existing != nil {
		if existing.Status == status || (status != domainpendingwork.StatusCompleted && existing.Status != domainpendingwork.StatusCompleted) {
			return *existing, nil
		}
		return domainpendingwork.PendingWorkDispositionV1{}, ErrWorkClosed
	}
	reasonCode, ok := reportStageDispositionReason(status)
	if !ok {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrOperationMismatch
	}
	return service.Close(ctx, receipt.WorkID, request.PendingToolCall.SecurityContext, status, reasonCode, disposedAt)
}

// CloseStaleReportStageLease records the fail-closed disposition against the
// new authoritative context after a case/epoch switch. It never resumes or
// executes the stale stage.
func (service *Service) CloseStaleReportStageLease(ctx context.Context, lease ReportStageLease, request ReportStageRequest, current domainsecurity.TurnSecurityContext, disposedAt time.Time) (domainpendingwork.PendingWorkDispositionV1, error) {
	receipt, existing, err := service.reportStageLeaseRecord(ctx, lease, request)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	if existing != nil {
		if existing.Status == domainpendingwork.StatusStaleContext {
			return *existing, nil
		}
		return domainpendingwork.PendingWorkDispositionV1{}, ErrWorkClosed
	}
	reasonCode, _ := reportStageDispositionReason(domainpendingwork.StatusStaleContext)
	return service.Close(ctx, receipt.WorkID, current, domainpendingwork.StatusStaleContext, reasonCode, disposedAt)
}

func (service *Service) reportStageLeaseRecord(ctx context.Context, lease ReportStageLease, request ReportStageRequest) (domainpendingwork.PendingWorkReceiptV1, *domainpendingwork.PendingWorkDispositionV1, error) {
	if service.restartOwnsThread(request.PendingToolCall.SecurityContext.ThreadID) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrRestartPreserved
	}
	securityContext, _, payload, _, err := reportStageAuthority(request)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, reportStagePayloadPurpose, payload)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, err
	}
	receipt, err := service.store.ReadReceipt(ctx, lease.workID)
	if err != nil || service.verifyReceipt(ctx, receipt) != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, firstNonNilPendingWorkError(err, ErrOperationMismatch)
	}
	binding, err := domainpendingwork.ContextBindingFromSecurityContextV1(securityContext)
	if err != nil || receipt.Kind != domainpendingwork.KindReportStage || receipt.Context != binding || len(receipt.GrantMembers) != 1 ||
		receipt.GrantMembers[0].GrantID != request.PendingToolCall.ExecutionGrant.GrantID || receipt.GrantMembers[0].ResultItemID != "" ||
		receipt.PayloadHash != payloadHash || receipt.RouteHash != reportStageRouteHash() {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrOperationMismatch
	}
	disposition, err := service.store.ReadDisposition(ctx, receipt.WorkID)
	if err == nil {
		if service.verifyDisposition(ctx, disposition, receipt) != nil {
			return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrOperationMismatch
		}
		return receipt, &disposition, nil
	}
	if !errors.Is(err, pendingworkstoreport.ErrNotFound) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, err
	}
	return receipt, nil, nil
}

func reportStageAuthority(request ReportStageRequest) (domainsecurity.TurnSecurityContext, GrantReference, []byte, time.Time, error) {
	pending := request.PendingToolCall
	securityContext := pending.SecurityContext
	grant := pending.ExecutionGrant
	stageInputHash := strings.TrimSpace(request.StageInputHash)
	if domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(grant, securityContext) != nil ||
		pending.ThreadID != securityContext.ThreadID || pending.TurnID != securityContext.TurnID ||
		grant.ContextDigest != securityContext.ContextDigest || grant.TurnID != securityContext.TurnID ||
		grant.GrantID == "" || grant.ToolCallID != strings.TrimSpace(pending.Call.ID) || grant.ToolName != strings.TrimSpace(pending.Call.Name) ||
		grant.ToolName != ReportStageToolName ||
		grant.ArgsHash != domainsecurity.CanonicalJSONHash(pending.Call.Arguments) || grant.ReadOnly || grant.ApprovalState != "approved" ||
		!canonicalReportStageDigest(stageInputHash) {
		return domainsecurity.TurnSecurityContext{}, GrantReference{}, nil, time.Time{}, ErrGrantAuthority
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, GrantReference{}, nil, time.Time{}, ErrGrantAuthority
	}
	payload, err := reportStagePayloadV1(securityContext.ContextDigest, securityContext.DatasetSnapshotID, grant, stageInputHash)
	if err != nil {
		return domainsecurity.TurnSecurityContext{}, GrantReference{}, nil, time.Time{}, err
	}
	return securityContext, GrantReference{GrantID: grant.GrantID}, payload, expiresAt, nil
}

func reportStagePayloadV1(contextDigest, datasetSnapshotID string, grant domainsecurity.ExecutionGrant, stageInputHash string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"schemaVersion": 1, "kind": domainpendingwork.KindReportStage, "contextDigest": contextDigest,
		"datasetSnapshotId": datasetSnapshotID, "grantId": grant.GrantID, "toolCallId": grant.ToolCallID,
		"toolName": grant.ToolName, "argsHash": grant.ArgsHash, "stageInputHash": stageInputHash,
	})
}

// VerifyHistoricalReportStageInputV1 verifies only the original request binding.
// The caller supplies an authenticated, stable original primary. This neither
// reopens a lease nor checks current effect authority; the only signer input is
// the existing fixed payload-key derivation message. Withheld arguments are not
// used to reconstruct the original grant's ArgsHash.
func (service *Service) VerifyHistoricalReportStageInputV1(ctx context.Context, receipt domainpendingwork.PendingWorkReceiptV1, thread map[string]any, toolCallID, stageInputHash string) error {
	if ctx == nil || service == nil || service.authority == nil || receipt.Kind != domainpendingwork.KindReportStage || !canonicalReportStageDigest(stageInputHash) {
		return ErrOperationMismatch
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := service.verifyReceipt(ctx, receipt); err != nil {
		return err
	}
	if _, _, err := reportStageRestartDispositionFromThread(receipt, thread); err != nil {
		return err
	}
	prefix, err := executiongrantapp.RegistrySnapshotFromThread(receipt.Context.ThreadID, thread, receipt.Context.TurnID, receipt.GrantRegistrySequence, receipt.GrantRegistryDigest)
	if err != nil {
		return err
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(prefix, receipt.GrantMembers[0].GrantID)
	if !found || entry.Grant.ToolCallID != toolCallID {
		return ErrGrantAuthority
	}
	payload, err := reportStagePayloadV1(receipt.Context.ContextDigest, receipt.Context.DatasetSnapshotID, entry.Grant, stageInputHash)
	if err != nil {
		return err
	}
	hash, err := service.KeyedPayloadHash(ctx, reportStagePayloadPurpose, payload)
	if err != nil {
		return err
	}
	if hash != receipt.PayloadHash || receipt.RouteHash != reportStageRouteHash() {
		return ErrOperationMismatch
	}
	return ctx.Err()
}

func reportStageDispositionReason(status string) (string, bool) {
	switch strings.TrimSpace(status) {
	case domainpendingwork.StatusCompleted:
		return "report_stage_completed", true
	case domainpendingwork.StatusFailed:
		return "report_stage_failed", true
	case domainpendingwork.StatusCancelled:
		return "report_stage_cancelled", true
	case domainpendingwork.StatusExpired:
		return "report_stage_expired", true
	case domainpendingwork.StatusRejected:
		return "report_stage_rejected", true
	case domainpendingwork.StatusStaleContext:
		return "report_stage_stale_context", true
	default:
		return "", false
	}
}

func sameReportStageRequest(left, right ReportStageRequest) bool {
	if !left.IssuedAt.UTC().Equal(right.IssuedAt.UTC()) || strings.TrimSpace(left.StageInputHash) != strings.TrimSpace(right.StageInputHash) {
		return false
	}
	leftContext, leftReference, leftPayload, leftExpiry, leftErr := reportStageAuthority(left)
	rightContext, rightReference, rightPayload, rightExpiry, rightErr := reportStageAuthority(right)
	return leftErr == nil && rightErr == nil && leftContext == rightContext && leftReference == rightReference &&
		leftExpiry.Equal(rightExpiry) && bytes.Equal(leftPayload, rightPayload)
}

func reportStageRouteHash() string {
	return domainsecurity.SHA256Hex([]byte(reportStageRouteIdentity))
}

func canonicalReportStageDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}
