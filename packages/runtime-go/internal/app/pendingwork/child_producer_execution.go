package pendingwork

import (
	"bytes"
	"context"
	"sync/atomic"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
)

type childProducerExecutionKey struct{}

type childProducerExecutionV1 struct {
	lease   SideEffectIntentLease
	request SideEffectIntentRequest
}

func newSideEffectIntentClaim(ctx context.Context, service *Service, producer *domainpendingwork.ChildProducerV1) *sideEffectIntentClaim {
	claim := &sideEffectIntentClaim{owner: service, issuedContext: ctx}
	if producer != nil {
		claim.children = make([]atomic.Bool, len(producer.Children))
	}
	return claim
}

func (claim *sideEffectIntentClaim) childExecutionActive() bool {
	return claim != nil && claim.claimed.Load() && claim.issuedContext != nil && claim.issuedContext.Err() == nil &&
		claim.sentContext != nil && claim.sentContext.Err() == nil
}

// BindChildProducerExecutionV1 carries only an already claimed process lease.
// Reading signed receipts after restart cannot construct this capability.
func (service *Service) BindChildProducerExecutionV1(ctx context.Context, lease SideEffectIntentLease, request SideEffectIntentRequest) (context.Context, error) {
	if ctx == nil || ctx.Err() != nil || service == nil || lease.claim == nil ||
		lease.claim.owner != service || !lease.claim.childExecutionActive() || lease.childProducer == nil {
		return nil, ErrOperationMismatch
	}
	authority, err := sideEffectIntentSemanticAuthority(request)
	if err != nil || !childProducerLeaseMatches(authority, lease) {
		return nil, ErrOperationMismatch
	}
	return context.WithValue(ctx, childProducerExecutionKey{}, childProducerExecutionV1{lease: lease, request: request}), nil
}

func childProducerLeaseMatches(authority sideEffectIntentAuthorityV1, lease SideEffectIntentLease) bool {
	return authority.securityContext == lease.securityContext && authority.grant == lease.grant &&
		authority.routeHash == lease.routeHash && authority.issuedAt.Equal(lease.issuedAt) &&
		authority.expiresAt.Equal(lease.expiresAt) && bytes.Equal(authority.payload, lease.payload) &&
		sameChildProducer(authority.childProducer, lease.childProducer)
}

// UseChildProducerV1 performs the first child-related writes under the exact signed
// parent intent and original ordinal. A failed or ambiguous write burns this
// process-local slot; neither another context nor a restart can retry it.
// produce may apply the admitted case alias continuity CAS prerequisites, then
// consume only the host's matching queued-job reservation. The existing writes
// are not an atomic transaction; failure after any prefix still burns the slot.
func (service *Service) UseChildProducerV1(ctx context.Context, pending appmodel.PendingToolCall, target domainpendingwork.ChildProducerTargetV1, now time.Time, produce func() error) error {
	if service == nil || ctx == nil || ctx.Err() != nil || produce == nil {
		return ErrOperationMismatch
	}
	execution, ok := ctx.Value(childProducerExecutionKey{}).(childProducerExecutionV1)
	lease := execution.lease
	if !ok || lease.claim == nil || lease.claim.owner != service || !lease.claim.childExecutionActive() ||
		lease.childProducer == nil || target.Ordinal == 0 || uint64(target.Ordinal) > uint64(len(lease.claim.children)) ||
		lease.childProducer.Children[target.Ordinal-1] != target {
		return ErrOperationMismatch
	}
	request := execution.request
	request.Pending = pending
	authority, err := sideEffectIntentSemanticAuthority(request)
	if err != nil || !childProducerLeaseMatches(authority, lease) {
		return ErrOperationMismatch
	}
	// Use the same order as terminal disposition and preservation installation.
	// Neither may invalidate the intent between readback and the first job write.
	service.reportStageTerminalMu.Lock()
	defer service.reportStageTerminalMu.Unlock()
	payloadHash, err := service.KeyedPayloadHash(ctx, sideEffectIntentPayloadPurpose, authority.payload)
	if err != nil {
		return err
	}
	receipt, err := service.VerifyOpenFor(ctx, lease.workID, authority.securityContext, domainpendingwork.KindSideEffectIntent, payloadHash, authority.routeHash, now)
	if err != nil {
		return err
	}
	if len(receipt.GrantMembers) != 1 || receipt.GrantMembers[0].GrantID != authority.grant.GrantID ||
		!sameChildProducer(receipt.ChildProducer, lease.childProducer) {
		return ErrOperationMismatch
	}
	release, err := service.lockRestartWrite(pending.ThreadID)
	if err != nil {
		return err
	}
	defer release()
	if service.restartPreserved.OwnsThread(target.ChildThreadID) {
		return ErrRestartPreserved
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !lease.claim.childExecutionActive() {
		return ErrOperationMismatch
	}
	if !lease.claim.children[target.Ordinal-1].CompareAndSwap(false, true) {
		return ErrSideEffectIntentAlreadyClaimed
	}
	return produce()
}
