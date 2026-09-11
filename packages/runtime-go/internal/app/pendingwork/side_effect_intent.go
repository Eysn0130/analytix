package pendingwork

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainjsonstrict "analytix.local/runtime-go/internal/domain/jsonstrict"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
	pendingworkstoreport "analytix.local/runtime-go/internal/ports/pendingworkstore"
)

const (
	sideEffectIntentPayloadPurpose = "side_effect_intent.semantic_request"
	sideEffectIntentRouteVersion   = 1
)

var ErrSideEffectIntentAlreadyClaimed = errors.New("side effect intent lease is already claimed")

// SideEffectIntentDuplicateError identifies the first durable semantic intent
// without disclosing its arguments. Callers may settle a different claimant's
// grant as a blocked duplicate, but must not race-settle the first claimant.
type SideEffectIntentDuplicateError struct {
	IntentStatus string
	FirstGrantID string
	cause        error
}

func (err SideEffectIntentDuplicateError) Error() string {
	return "side effect intent is already " + err.IntentStatus
}

func (err SideEffectIntentDuplicateError) Unwrap() error { return err.cause }

type SideEffectIntentRequest struct {
	Pending          appmodel.PendingToolCall
	IssuedAt         time.Time
	SemanticIdentity domainsideeffectidentity.IdentityV1
	ChildProducer    *ChildProducerPlanV1
}

type sideEffectIntentClaim struct {
	claimed       atomic.Bool
	owner         *Service
	children      []atomic.Bool
	mu            sync.Mutex
	issuedContext context.Context
	sentContext   context.Context
}

// SideEffectIntentLease is process-local, one-shot execution authority. The
// signed receipt is durable audit state and never becomes permission to retry.
type SideEffectIntentLease struct {
	workID          string
	securityContext domainsecurity.TurnSecurityContext
	grant           domainsecurity.ExecutionGrant
	payload         []byte
	routeHash       string
	registryDigest  string
	issuedAt        time.Time
	expiresAt       time.Time
	claim           *sideEffectIntentClaim
	childProducer   *domainpendingwork.ChildProducerV1
}

func (lease SideEffectIntentLease) WorkID() string { return lease.workID }

// BeginSideEffectIntent is the exclusive semantic effect CAS. Changing a
// provider call id, grant id, approval transition, registry head, or route
// cannot create a second WorkID for the same frozen context/tool/arguments.
func (service *Service) BeginSideEffectIntent(ctx context.Context, request SideEffectIntentRequest) (SideEffectIntentLease, error) {
	authority, err := service.sideEffectIntentAuthority(ctx, request)
	if err != nil {
		return SideEffectIntentLease{}, err
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, sideEffectIntentPayloadPurpose, authority.payload)
	if err != nil {
		return SideEffectIntentLease{}, err
	}
	receipt, err := service.prepareIssue(ctx, IssueInput{
		Kind: domainpendingwork.KindSideEffectIntent, SecurityContext: authority.securityContext,
		GrantReferences: []GrantReference{{GrantID: authority.grant.GrantID}}, ExpectedRegistryDigest: authority.registryDigest,
		PayloadHash: payloadHash, RouteHash: authority.routeHash, IssuedAt: authority.issuedAt, ExpiresAt: authority.expiresAt,
		ChildProducer: authority.childProducer,
	})
	if err != nil {
		return SideEffectIntentLease{}, err
	}
	candidateWorkID := receipt.WorkID
	receipt, err = service.issuePreparedExclusive(ctx, receipt)
	if errors.Is(err, ErrWorkAlreadyOpen) || errors.Is(err, ErrWorkClosed) {
		return SideEffectIntentLease{}, service.sideEffectIntentDuplicateError(ctx, candidateWorkID, err)
	}
	if err != nil {
		return SideEffectIntentLease{}, err
	}
	return SideEffectIntentLease{
		workID: receipt.WorkID, securityContext: authority.securityContext, grant: authority.grant,
		payload: append([]byte(nil), authority.payload...), routeHash: authority.routeHash,
		registryDigest: authority.registryDigest, issuedAt: authority.issuedAt, expiresAt: authority.expiresAt,
		claim:         newSideEffectIntentClaim(ctx, service, authority.childProducer),
		childProducer: domainpendingwork.CloneChildProducerV1(authority.childProducer),
	}, nil
}

func (service *Service) sideEffectIntentDuplicateError(ctx context.Context, workID string, cause error) error {
	receipt, err := service.store.ReadReceipt(ctx, workID)
	if err != nil || service.verifyReceipt(ctx, receipt) != nil || receipt.Kind != domainpendingwork.KindSideEffectIntent || len(receipt.GrantMembers) != 1 {
		return ErrOperationMismatch
	}
	status := "open"
	if errors.Is(cause, ErrWorkClosed) {
		status = "closed"
	}
	return SideEffectIntentDuplicateError{IntentStatus: status, FirstGrantID: receipt.GrantMembers[0].GrantID, cause: cause}
}

func (service *Service) VerifySideEffectIntentAtSend(
	ctx context.Context,
	lease SideEffectIntentLease,
	request SideEffectIntentRequest,
	now time.Time,
) error {
	if lease.claim == nil || lease.claim.owner != service || strings.TrimSpace(lease.workID) == "" {
		return ErrOperationMismatch
	}
	authority, err := service.sideEffectIntentAuthority(ctx, request)
	if err != nil || authority.securityContext != lease.securityContext || authority.grant != lease.grant ||
		authority.routeHash != lease.routeHash || !authority.issuedAt.Equal(lease.issuedAt) ||
		!authority.expiresAt.Equal(lease.expiresAt) || !bytes.Equal(authority.payload, lease.payload) ||
		!sameChildProducer(authority.childProducer, lease.childProducer) {
		return firstNonNilPendingWorkError(err, ErrOperationMismatch)
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, sideEffectIntentPayloadPurpose, authority.payload)
	if err != nil {
		return err
	}
	receipt, err := service.VerifyOpenFor(
		ctx, lease.workID, authority.securityContext, domainpendingwork.KindSideEffectIntent,
		payloadHash, authority.routeHash, now,
	)
	if err != nil || len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != authority.grant.GrantID ||
		!sameChildProducer(receipt.ChildProducer, authority.childProducer) {
		return firstNonNilPendingWorkError(err, ErrGrantAuthority)
	}
	lease.claim.mu.Lock()
	defer lease.claim.mu.Unlock()
	if lease.claim.claimed.Load() {
		return ErrSideEffectIntentAlreadyClaimed
	}
	if lease.claim.issuedContext == nil || lease.claim.issuedContext.Err() != nil || ctx.Err() != nil {
		return ErrOperationMismatch
	}
	lease.claim.sentContext = ctx
	lease.claim.claimed.Store(true)
	return nil
}

func (service *Service) CloseSideEffectIntentAfterSettlement(
	ctx context.Context,
	lease SideEffectIntentLease,
	request SideEffectIntentRequest,
	disposedAt time.Time,
) (domainpendingwork.PendingWorkDispositionV1, error) {
	if lease.claim == nil || !lease.claim.claimed.Load() {
		return domainpendingwork.PendingWorkDispositionV1{}, ErrOperationMismatch
	}
	receipt, existing, err := service.sideEffectIntentLeaseRecord(ctx, lease, request)
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

type sideEffectIntentAuthorityV1 struct {
	securityContext domainsecurity.TurnSecurityContext
	grant           domainsecurity.ExecutionGrant
	payload         []byte
	routeHash       string
	registryDigest  string
	issuedAt        time.Time
	expiresAt       time.Time
	childProducer   *domainpendingwork.ChildProducerV1
}

func (service *Service) sideEffectIntentAuthority(ctx context.Context, request SideEffectIntentRequest) (sideEffectIntentAuthorityV1, error) {
	authority, err := sideEffectIntentSemanticAuthority(request)
	if err != nil || !service.Available() {
		return sideEffectIntentAuthorityV1{}, firstNonNilPendingWorkError(err, ErrAuthorityUnavailable)
	}
	if ctx == nil || ctx.Err() != nil {
		return sideEffectIntentAuthorityV1{}, ErrAuthorityUnavailable
	}
	if request.ChildProducer != nil {
		if err := request.ChildProducer.revalidate(ctx); err != nil {
			return sideEffectIntentAuthorityV1{}, err
		}
	}
	thread, registry, err := service.currentAuthority(authority.securityContext)
	if err != nil {
		return sideEffectIntentAuthorityV1{}, err
	}
	if authority.grant.ApprovalState == "approved" {
		transition := request.Pending.ApprovalTransition
		if transition == nil {
			return sideEffectIntentAuthorityV1{}, ErrGrantAuthority
		}
		durable, err := executiongrantapp.DurableApprovalTransitionFromThread(
			authority.securityContext.ThreadID,
			thread,
			authority.securityContext.TurnID,
			transition.TransitionID,
		)
		if err != nil || durable.Transition != *transition || durable.ApprovedGrant != authority.grant {
			return sideEffectIntentAuthorityV1{}, ErrGrantAuthority
		}
	}
	authority.registryDigest = registry.StateDigest
	return authority, nil
}

func sideEffectIntentSemanticAuthority(request SideEffectIntentRequest) (sideEffectIntentAuthorityV1, error) {
	childProducer, err := childProducerForRequest(request)
	if err != nil {
		return sideEffectIntentAuthorityV1{}, err
	}
	pending := request.Pending
	grant := pending.ExecutionGrant
	issuedAt := request.IssuedAt.UTC()
	approvalState := strings.TrimSpace(grant.ApprovalState)
	transition := pending.ApprovalTransition
	if issuedAt.IsZero() || validatePendingGrantForCall(pending.SecurityContext, grant, pending.Call) != nil || grant.ReadOnly ||
		(approvalState != "approved" && approvalState != "not_required") || grant.ToolName == ReportStageToolName ||
		pending.ThreadID != pending.SecurityContext.ThreadID || pending.TurnID != pending.SecurityContext.TurnID ||
		grant.ToolName != strings.TrimSpace(pending.Call.Name) || grant.ToolCallID != strings.TrimSpace(pending.Call.ID) ||
		grant.ArgsHash != domainsecurity.CanonicalJSONHash(pending.Call.Arguments) {
		return sideEffectIntentAuthorityV1{}, ErrGrantAuthority
	}
	if approvalState == "approved" {
		if transition == nil || transition.ApprovedGrantID != grant.GrantID || transition.ToolCallID != grant.ToolCallID ||
			transition.ToolName != grant.ToolName || transition.ThreadID != pending.ThreadID || transition.TurnID != pending.TurnID ||
			transition.ContextDigest != pending.SecurityContext.ContextDigest || transition.ContextEpoch != pending.SecurityContext.ContextEpoch {
			return sideEffectIntentAuthorityV1{}, ErrGrantAuthority
		}
	} else if transition != nil {
		return sideEffectIntentAuthorityV1{}, ErrGrantAuthority
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || !issuedAt.Before(expiresAt) {
		return sideEffectIntentAuthorityV1{}, ErrWorkExpired
	}
	semanticIdentity := request.SemanticIdentity
	if domainsideeffectidentity.ValidateV1(semanticIdentity) != nil ||
		semanticIdentity.ToolName != domainsideeffectidentity.CanonicalToolNameV1(grant.ToolName) {
		return sideEffectIntentAuthorityV1{}, ErrOperationMismatch
	}
	payload := struct {
		SchemaVersion int    `json:"schemaVersion"`
		Kind          string `json:"kind"`
		ToolName      string `json:"toolName"`
		ArgsHash      string `json:"argsHash"`
	}{semanticIdentity.SchemaVersion, domainpendingwork.KindSideEffectIntent, semanticIdentity.ToolName, semanticIdentity.ArgsHash}
	canonicalPayload, err := strictCanonicalPendingWorkValue(payload)
	if err != nil {
		return sideEffectIntentAuthorityV1{}, err
	}
	route := struct {
		SchemaVersion   int    `json:"schemaVersion"`
		Kind            string `json:"kind"`
		Provider        string `json:"provider"`
		ServerIdentity  string `json:"serverIdentity"`
		ConnectionEpoch uint64 `json:"connectionEpoch"`
		ToolName        string `json:"toolName"`
		SchemaHash      string `json:"schemaHash"`
		ScopeHash       string `json:"scopeHash"`
		ApprovalState   string `json:"approvalState"`
	}{
		sideEffectIntentRouteVersion, domainpendingwork.KindSideEffectIntent, grant.Provider,
		grant.ServerIdentity, grant.ConnectionEpoch, grant.ToolName, grant.SchemaHash, grant.ScopeHash, grant.ApprovalState,
	}
	routeBytes, err := strictCanonicalPendingWorkValue(route)
	if err != nil {
		return sideEffectIntentAuthorityV1{}, err
	}
	return sideEffectIntentAuthorityV1{
		securityContext: pending.SecurityContext, grant: grant, payload: canonicalPayload,
		routeHash: domainsecurity.SHA256Hex(routeBytes), issuedAt: issuedAt, expiresAt: expiresAt,
		childProducer: childProducer,
	}, nil
}

func strictCanonicalPendingWorkValue(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	parsed, err := domainjsonstrict.DecodeValue(raw, domainjsonstrict.Options{
		RequireObject: true, MaxBytes: 64 * 1024, MaxDepth: 16, MaxTokens: 2_000, MaxStringBytes: 4096,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(parsed)
}

func (service *Service) sideEffectIntentLeaseRecord(
	ctx context.Context,
	lease SideEffectIntentLease,
	request SideEffectIntentRequest,
) (domainpendingwork.PendingWorkReceiptV1, *domainpendingwork.PendingWorkDispositionV1, error) {
	authority, err := sideEffectIntentSemanticAuthority(request)
	if err != nil || authority.securityContext != lease.securityContext || authority.grant != lease.grant ||
		authority.routeHash != lease.routeHash || !authority.issuedAt.Equal(lease.issuedAt) ||
		!authority.expiresAt.Equal(lease.expiresAt) || !bytes.Equal(authority.payload, lease.payload) ||
		!sameChildProducer(authority.childProducer, lease.childProducer) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, firstNonNilPendingWorkError(err, ErrOperationMismatch)
	}
	receipt, err := service.store.ReadReceipt(ctx, lease.workID)
	if err != nil || service.verifyReceipt(ctx, receipt) != nil || receipt.Kind != domainpendingwork.KindSideEffectIntent ||
		len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != authority.grant.GrantID ||
		!sameChildProducer(receipt.ChildProducer, authority.childProducer) {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, firstNonNilPendingWorkError(err, ErrOperationMismatch)
	}
	payloadHash, err := service.KeyedPayloadHash(ctx, sideEffectIntentPayloadPurpose, authority.payload)
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
	// The exact grant must already have a durable result before Close performs
	// its current-registry settlement proof.
	thread, registry, err := service.currentAuthority(authority.securityContext)
	if err != nil {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, err
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, authority.grant.GrantID)
	if !found || entry.Status != domainsecurity.GrantRegistrySettled {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrGrantAuthority
	}
	if _, _, found := exactDurableResultItem(thread, authority.securityContext, authority.grant); !found {
		return domainpendingwork.PendingWorkReceiptV1{}, nil, ErrGrantAuthority
	}
	return receipt, nil, nil
}
