package pendingwork

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

const (
	approvedToolDispatchPayloadPurpose = "approved_tool_dispatch.semantic_request"
	approvedToolDispatchRouteVersion   = 1
)

var ErrDispatchAlreadyClaimed = errors.New("approved tool dispatch lease is already claimed")

type ApprovedToolDispatchRequest struct {
	Pending  appmodel.PendingToolCall
	IssuedAt time.Time
}

type approvedToolDispatchClaim struct {
	claimed atomic.Bool
}

// ApprovedToolDispatchLease is intentionally process-local and one-shot. A
// persisted receipt is restart audit authority, never permission to resend.
type ApprovedToolDispatchLease struct {
	workID          string
	securityContext domainsecurity.TurnSecurityContext
	grant           domainsecurity.ExecutionGrant
	transition      domainsecurity.ApprovalGrantTransitionV1
	payload         []byte
	routeHash       string
	registryDigest  string
	issuedAt        time.Time
	expiresAt       time.Time
	claim           *approvedToolDispatchClaim
}

func (lease ApprovedToolDispatchLease) WorkID() string { return lease.workID }

// BeginApprovedToolDispatch persists exclusive write-effect intent. Only an
// unambiguous first CAS creator receives the process-local execution claim;
// an existing open receipt is outcome-ambiguous and cannot be reissued.
func (service *Service) BeginApprovedToolDispatch(ctx context.Context, request ApprovedToolDispatchRequest) (ApprovedToolDispatchLease, error) {
	authority, err := service.approvedToolDispatchAuthority(request)
	if err != nil {
		return ApprovedToolDispatchLease{}, err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, approvedToolDispatchPayloadPurpose, authority.payload)
	if err != nil {
		return ApprovedToolDispatchLease{}, err
	}
	receipt, err := service.issueExclusive(ctx, IssueInput{
		Kind: domainpendingwork.KindApprovedToolDispatch, SecurityContext: authority.securityContext,
		GrantReferences: []GrantReference{{GrantID: authority.grant.GrantID}}, ExpectedRegistryDigest: authority.registryDigest,
		PayloadHash: payloadHash, RouteHash: authority.routeHash, IssuedAt: authority.issuedAt, ExpiresAt: authority.expiresAt,
	})
	if err != nil {
		return ApprovedToolDispatchLease{}, err
	}
	return ApprovedToolDispatchLease{
		workID: receipt.WorkID, securityContext: authority.securityContext, grant: authority.grant,
		transition: authority.transition, payload: append([]byte(nil), authority.payload...), routeHash: authority.routeHash,
		registryDigest: authority.registryDigest, issuedAt: authority.issuedAt, expiresAt: authority.expiresAt,
		claim: &approvedToolDispatchClaim{},
	}, nil
}

// VerifyApprovedToolDispatchAtSend re-derives current context, registry,
// transition, tool, arguments, and route immediately before the physical
// external call, then consumes the process-local claim exactly once.
func (service *Service) VerifyApprovedToolDispatchAtSend(
	ctx context.Context,
	lease ApprovedToolDispatchLease,
	request ApprovedToolDispatchRequest,
	now time.Time,
) error {
	if lease.claim == nil || strings.TrimSpace(lease.workID) == "" {
		return ErrOperationMismatch
	}
	authority, err := service.approvedToolDispatchAuthority(request)
	if err != nil || authority.securityContext != lease.securityContext || authority.grant != lease.grant ||
		authority.transition != lease.transition ||
		authority.routeHash != lease.routeHash || !authority.issuedAt.Equal(lease.issuedAt) ||
		!authority.expiresAt.Equal(lease.expiresAt) || !bytes.Equal(authority.payload, lease.payload) {
		return ErrOperationMismatch
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, approvedToolDispatchPayloadPurpose, authority.payload)
	if err != nil {
		return err
	}
	receipt, err := service.VerifyOpenFor(
		ctx, lease.workID, authority.securityContext, domainpendingwork.KindApprovedToolDispatch,
		payloadHash, authority.routeHash, now,
	)
	if err != nil || len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != authority.grant.GrantID {
		return firstNonNilPendingWorkError(err, ErrGrantAuthority)
	}
	if !lease.claim.claimed.CompareAndSwap(false, true) {
		return ErrDispatchAlreadyClaimed
	}
	return nil
}

// CloseApprovedToolDispatchAfterSettlement succeeds only after the exact
// grant has a durable tool_result and registry settlement. Tool success is not
// required; an error outcome is still classified once durably recorded.
func (service *Service) CloseApprovedToolDispatchAfterSettlement(
	ctx context.Context,
	lease ApprovedToolDispatchLease,
	request ApprovedToolDispatchRequest,
	disposedAt time.Time,
) (domainpendingwork.PendingWorkDispositionV1, error) {
	if lease.claim == nil || !lease.claim.claimed.Load() {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrOperationMismatch
	}
	receipt, existing, err := service.approvedToolDispatchLeaseRecord(ctx, lease, request)
	if err != nil {
		return domainpendingwork.PendingWorkDispositionV1{}, err
	}
	if existing != nil {
		if existing.Status == domainpendingwork.StatusCompleted && existing.ReasonCode == "tool_outcome_durable" {
			return *existing, nil
		}
		return domainpendingwork.PendingWorkDispositionV1{}, ErrWorkClosed
	}
	return service.Close(
		ctx, receipt.WorkID, request.Pending.SecurityContext,
		domainpendingwork.StatusCompleted, "tool_outcome_durable", disposedAt,
	)
}

type approvedToolDispatchAuthorityV1 struct {
	securityContext domainsecurity.TurnSecurityContext
	grant           domainsecurity.ExecutionGrant
	transition      domainsecurity.ApprovalGrantTransitionV1
	payload         []byte
	routeHash       string
	registryDigest  string
	issuedAt        time.Time
	expiresAt       time.Time
}

func (service *Service) approvedToolDispatchAuthority(request ApprovedToolDispatchRequest) (approvedToolDispatchAuthorityV1, error) {
	// Child production requires the complete host allocation in the exclusive
	// side-effect intent. Keep the historical semantic/settlement reader below
	// unchanged; old receipts must not become fresh send authority.
	if childProducingSideEffect(request.Pending.ExecutionGrant.ToolName) {
		return approvedToolDispatchAuthorityV1{}, ErrOperationMismatch
	}
	authority, err := approvedToolDispatchSemanticAuthority(request)
	if err != nil || !service.Available() {
		return approvedToolDispatchAuthorityV1{}, firstNonNilPendingWorkError(err, ErrAuthorityUnavailable)
	}
	thread, registry, err := service.currentAuthority(authority.securityContext)
	if err != nil {
		return approvedToolDispatchAuthorityV1{}, err
	}
	durable, err := executiongrantapp.DurableApprovalTransitionFromThread(
		authority.securityContext.ThreadID, thread, authority.securityContext.TurnID, authority.transition.TransitionID,
	)
	if err != nil || durable.Transition != authority.transition || durable.ApprovedGrant != authority.grant {
		return approvedToolDispatchAuthorityV1{}, ErrGrantAuthority
	}
	authority.registryDigest = registry.StateDigest
	return authority, nil
}

func approvedToolDispatchSemanticAuthority(request ApprovedToolDispatchRequest) (approvedToolDispatchAuthorityV1, error) {
	pending := request.Pending
	grant := pending.ExecutionGrant
	transition := pending.ApprovalTransition
	issuedAt := request.IssuedAt.UTC()
	if issuedAt.IsZero() || transition == nil ||
		validatePendingGrantForCall(pending.SecurityContext, grant, pending.Call) != nil ||
		grant.ApprovalState != "approved" || grant.ReadOnly || grant.ToolName == ReportStageToolName ||
		pending.ThreadID != pending.SecurityContext.ThreadID || pending.TurnID != pending.SecurityContext.TurnID ||
		grant.ToolName != strings.TrimSpace(pending.Call.Name) || grant.ToolCallID != strings.TrimSpace(pending.Call.ID) ||
		grant.ArgsHash != domainsecurity.CanonicalJSONHash(pending.Call.Arguments) {
		return approvedToolDispatchAuthorityV1{}, ErrGrantAuthority
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || !issuedAt.Before(expiresAt) {
		return approvedToolDispatchAuthorityV1{}, ErrWorkExpired
	}
	payload := struct {
		SchemaVersion             int    `json:"schemaVersion"`
		Kind                      string `json:"kind"`
		ApprovalTransitionID      string `json:"approvalTransitionId"`
		ApprovalID                string `json:"approvalId"`
		ContinuationReceiptID     string `json:"continuationReceiptId"`
		ContinuationDispositionID string `json:"continuationDispositionId"`
		GrantID                   string `json:"grantId"`
		ToolCallID                string `json:"toolCallId"`
		ToolName                  string `json:"toolName"`
		ArgsHash                  string `json:"argsHash"`
		SchemaHash                string `json:"schemaHash"`
		ScopeHash                 string `json:"scopeHash"`
	}{
		1, domainpendingwork.KindApprovedToolDispatch, transition.TransitionID, transition.ApprovalID,
		transition.ContinuationReceiptID, transition.ContinuationDispositionID, grant.GrantID,
		grant.ToolCallID, grant.ToolName, grant.ArgsHash, grant.SchemaHash, grant.ScopeHash,
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return approvedToolDispatchAuthorityV1{}, err
	}
	payloadValue, err := domainjsonstrict.DecodeValue(rawPayload, domainjsonstrict.Options{RequireObject: true, MaxBytes: 64 * 1024, MaxDepth: 16, MaxTokens: 2_000, MaxStringBytes: 4096})
	if err != nil {
		return approvedToolDispatchAuthorityV1{}, err
	}
	canonicalPayload, err := json.Marshal(payloadValue)
	if err != nil {
		return approvedToolDispatchAuthorityV1{}, err
	}
	route := struct {
		Version         int    `json:"version"`
		Kind            string `json:"kind"`
		Provider        string `json:"provider"`
		ServerIdentity  string `json:"serverIdentity"`
		ConnectionEpoch uint64 `json:"connectionEpoch"`
		ToolName        string `json:"toolName"`
		SchemaHash      string `json:"schemaHash"`
		ScopeHash       string `json:"scopeHash"`
	}{
		approvedToolDispatchRouteVersion, domainpendingwork.KindApprovedToolDispatch, grant.Provider,
		grant.ServerIdentity, grant.ConnectionEpoch, grant.ToolName, grant.SchemaHash, grant.ScopeHash,
	}
	rawRoute, err := json.Marshal(route)
	if err != nil {
		return approvedToolDispatchAuthorityV1{}, err
	}
	routeValue, err := domainjsonstrict.DecodeValue(rawRoute, domainjsonstrict.Options{RequireObject: true, MaxBytes: 64 * 1024, MaxDepth: 16, MaxTokens: 2_000, MaxStringBytes: 4096})
	if err != nil {
		return approvedToolDispatchAuthorityV1{}, err
	}
	routeBytes, err := json.Marshal(routeValue)
	if err != nil {
		return approvedToolDispatchAuthorityV1{}, err
	}
	return approvedToolDispatchAuthorityV1{
		securityContext: pending.SecurityContext, grant: grant, transition: *transition,
		payload: canonicalPayload, routeHash: domainsecurity.SHA256Hex(routeBytes),
		issuedAt: issuedAt, expiresAt: expiresAt,
	}, nil
}

func (service *Service) approvedToolDispatchLeaseRecord(
	ctx context.Context,
	lease ApprovedToolDispatchLease,
	request ApprovedToolDispatchRequest,
) (domainpendingwork.PendingWorkReceiptV1, *domainpendingwork.PendingWorkDispositionV1, error) {
	authority, err := approvedToolDispatchSemanticAuthority(request)
	if err != nil || authority.securityContext != lease.securityContext || authority.grant != lease.grant ||
		authority.transition != lease.transition ||
		authority.routeHash != lease.routeHash || !authority.issuedAt.Equal(lease.issuedAt) ||
		!authority.expiresAt.Equal(lease.expiresAt) || !bytes.Equal(authority.payload, lease.payload) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, firstNonNilPendingWorkError(err, ErrOperationMismatch)
	}
	receipt, err := service.store.ReadReceipt(ctx, lease.workID)
	if err != nil || service.verifyReceipt(ctx, receipt) != nil || receipt.Kind != domainpendingwork.KindApprovedToolDispatch ||
		len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != authority.grant.GrantID {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, firstNonNilPendingWorkError(err, ErrOperationMismatch)
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, approvedToolDispatchPayloadPurpose, authority.payload)
	if err != nil || receipt.PayloadHash != payloadHash || receipt.RouteHash != authority.routeHash ||
		receipt.GrantRegistryDigest != lease.registryDigest || receipt.IssuedAt != authority.issuedAt.Format(time.RFC3339Nano) ||
		receipt.ExpiresAt != authority.expiresAt.Format(time.RFC3339Nano) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, firstNonNilPendingWorkError(err, ErrOperationMismatch)
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
