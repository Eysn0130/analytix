package server

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	apploop "analytix.local/runtime-go/internal/app/loop"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/jobs"
)

type runtimeChildProducerPlanKeyV1 struct{}
type runtimeChildProducerSlotKeyV1 struct{}

type runtimeChildProducerPlanV1 struct {
	owner    *runtimeServerHandler
	children []*runtimeChildProducerSlotV1
}

type runtimeChildProducerSlotV1 struct {
	owner                    *runtimeServerHandler
	target                   domainpendingwork.ChildProducerTargetV1
	job                      jobs.ChildRunReservationV1
	thread                   *childThreadReservationV1
	turn                     *childTurnReservationV1
	request                  runtimeSubagentTaskRequest
	preparation              subagentapp.TaskRunPreparation
	projectionDigest         string
	semanticProjectionDigest string
	parentBindingDigest      string
}

func (h *runtimeServerHandler) admitRuntimeChildProducerV1(ctx context.Context, pending runtimePendingToolCall, request runtimeSubagentTaskRequest) (subagentapp.TaskRunPreparation, error) {
	var err error
	request, err = subagentapp.BindForegroundHandoffPendingRequest(pending, request)
	if err != nil {
		return subagentapp.TaskRunPreparation{}, err
	}
	binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil {
		return subagentapp.TaskRunPreparation{}, err
	}
	request, err = h.bindRuntimeCaseDelegationRequest(ctx, pending, binding, request)
	if err != nil {
		return subagentapp.TaskRunPreparation{}, err
	}
	return h.prepareRuntimeAdmittedChild(request, pending)
}

func childProducerPreparationProjectionV1(preparation subagentapp.TaskRunPreparation, pending runtimePendingToolCall) map[string]any {
	return preparation.SideEffectProjectionV1(pending.Call.Name, subagentapp.ParentMaxModelSteps(pending.MaxModelSteps, pending.EffectiveMaxModelSteps))
}

func childProducerProjectionDigestV1(projection map[string]any) string {
	body, err := json.Marshal(projection)
	if err != nil {
		return ""
	}
	return domainsecurity.CanonicalJSONHash(body)
}

// reserveChildV1 runs only after the existing complete task admission owners.
// All allocations remain process-local until the parent receipt is persisted.
func (plan *runtimeChildProducerPlanV1) reserveChildV1(ctx context.Context, pending runtimePendingToolCall, request runtimeSubagentTaskRequest) (map[string]any, error) {
	h := plan.owner
	// Preserve the original semantic WorkID, including profile override order.
	// The admitted execution has a separate private seal and does not redefine it.
	semantic, err := h.resolveRuntimeSubagentSideEffectProjection(ctx, pending, request)
	if err != nil {
		return nil, err
	}
	preparation, err := h.admitRuntimeChildProducerV1(ctx, pending, request)
	if err != nil {
		return nil, err
	}
	defer preparation.ReleaseSourceLock()
	projection := childProducerPreparationProjectionV1(preparation, pending)
	digest := childProducerProjectionDigestV1(projection)
	semanticDigest := childProducerProjectionDigestV1(semantic)
	if digest == "" || semanticDigest == "" || semantic["sourceRef"] != preparation.Source.ID {
		return nil, errors.New("child preparation projection is invalid")
	}
	// Never retain or transfer a source lock across preparation of another slot.
	preparation.ReleaseSourceLock()
	slot := &runtimeChildProducerSlotV1{owner: h, request: request, preparation: preparation, projectionDigest: digest, semanticProjectionDigest: semanticDigest}
	return plan.reservePreparedChildV1(ctx, pending, slot, semantic)
}

func (plan *runtimeChildProducerPlanV1) reservePreparedChildV1(ctx context.Context, pending runtimePendingToolCall, slot *runtimeChildProducerSlotV1, projection map[string]any) (map[string]any, error) {
	h, preparation := plan.owner, slot.preparation
	request, source := preparation.Request, preparation.Source
	childThreadID := ""
	if preparation.HasSource && request.ContinueFrom != "" && source.ParentThreadID == pending.ThreadID {
		childThreadID = source.ChildThreadID
	} else {
		forkSource := ""
		if preparation.HasSource && (request.ContinueFrom != "" || request.ForkFrom != "") {
			forkSource = source.ChildThreadID
		}
		reservation, err := h.store.reserveChildThreadV1(ctx, pending.ThreadID, forkSource)
		if err != nil {
			return nil, err
		}
		slot.thread, childThreadID = reservation, reservation.threadID
	}
	turn, err := h.reserveRuntimeChildTurnV1(ctx, childThreadID)
	if err != nil {
		return nil, err
	}
	slot.turn = turn
	binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil {
		return nil, err
	}
	ordinal := uint32(len(plan.children) + 1)
	slot.parentBindingDigest = binding.BindingDigest
	reservation, err := h.jobs.ReserveChildRunV1(ctx, binding, ordinal, childThreadID, turn.turnID)
	if err != nil {
		return nil, err
	}
	slot.job, slot.target = reservation, reservation.TargetV1()
	plan.children = append(plan.children, slot)
	return projection, nil
}

func (plan *runtimeChildProducerPlanV1) bindPreparedSideEffectV1(pending runtimePendingToolCall, prepared apploop.PreparedSideEffect) (apploop.PreparedSideEffect, error) {
	if len(plan.children) == 0 || prepared.Rejection != nil {
		return prepared, nil
	}
	targets := make([]domainpendingwork.ChildProducerTargetV1, len(plan.children))
	for index, slot := range plan.children {
		targets[index] = slot.target
	}
	childPlan, err := pendingworkapp.NewChildProducerPlanV1(pending, prepared.SemanticIdentity, targets, func(ctx context.Context) error {
		for _, slot := range plan.children {
			semantic, err := plan.owner.resolveRuntimeSubagentSideEffectProjection(ctx, pending, slot.request)
			if err != nil || childProducerProjectionDigestV1(semantic) != slot.semanticProjectionDigest {
				return errors.New("child semantic preparation changed before signed intent")
			}
			preparation, err := plan.owner.admitRuntimeChildProducerV1(ctx, pending, slot.request)
			if err != nil {
				return err
			}
			valid := childProducerProjectionDigestV1(childProducerPreparationProjectionV1(preparation, pending)) == slot.projectionDigest &&
				preparation.HasSource == slot.preparation.HasSource && preparation.Source.ID == slot.preparation.Source.ID &&
				preparation.Source.ChildThreadID == slot.preparation.Source.ChildThreadID &&
				reflect.DeepEqual(preparation.Request.CaseDelegation, slot.preparation.Request.CaseDelegation)
			preparation.ReleaseSourceLock()
			if !valid {
				return errors.New("child admission changed before signed intent")
			}
			if err := slot.revalidateReservationsV1(ctx); err != nil {
				return err
			}
		}
		return ctx.Err()
	})
	if err != nil {
		return apploop.PreparedSideEffect{}, err
	}
	prepared.ChildProducer = &childPlan
	bind := prepared.Bind
	prepared.Bind = func(ctx context.Context) context.Context {
		if bind != nil {
			ctx = bind(ctx)
		}
		return context.WithValue(ctx, runtimeChildProducerPlanKeyV1{}, plan)
	}
	return prepared, nil
}

func (slot *runtimeChildProducerSlotV1) revalidateReservationsV1(ctx context.Context) error {
	if slot == nil || slot.owner == nil {
		return errors.New("host child producer is unavailable")
	}
	if err := slot.owner.jobs.RevalidateChildRunReservationV1(ctx, slot.job); err != nil {
		return err
	}
	if slot.thread != nil {
		if err := slot.owner.store.revalidateChildThreadReservationV1(ctx, slot.thread); err != nil {
			return err
		}
	}
	return slot.owner.revalidateRuntimeChildTurnV1(ctx, slot.turn)
}

func (h *runtimeServerHandler) childProducerForRunV1(ctx context.Context, pending runtimePendingToolCall, preparation subagentapp.TaskRunPreparation, producerRequest runtimeSubagentTaskRequest) (*runtimeChildProducerSlotV1, error) {
	plan, ok := ctx.Value(runtimeChildProducerPlanKeyV1{}).(*runtimeChildProducerPlanV1)
	ordinal := max(1, preparation.Request.ParallelIndex)
	if !ok || plan == nil || plan.owner != h || ordinal > len(plan.children) {
		return nil, errors.New("host child producer binding is unavailable")
	}
	slot := plan.children[ordinal-1]
	projection := childProducerPreparationProjectionV1(preparation, pending)
	// Normalize only the host runner's opaque expansion, then apply the same
	// profile/return-format owner. Prompt prefixes are not dependency authority.
	if original, expanded := producerRequest.OriginalParallelRequestV1(); expanded {
		original, err := subagentapp.ApplyProfile(original, h.subagents)
		if err != nil {
			return nil, err
		}
		projection["prompt"] = original.Prompt
	}
	if childProducerProjectionDigestV1(projection) != slot.projectionDigest || preparation.HasSource != slot.preparation.HasSource ||
		preparation.Source.ID != slot.preparation.Source.ID || preparation.Source.ChildThreadID != slot.preparation.Source.ChildThreadID ||
		!reflect.DeepEqual(preparation.Request.CaseDelegation, slot.preparation.Request.CaseDelegation) {
		return nil, errors.New("child execution differs from signed host preparation")
	}
	if err := slot.revalidateReservationsV1(ctx); err != nil {
		return nil, err
	}
	return slot, nil
}

func (h *runtimeServerHandler) startPreparedReservedChildV1(ctx context.Context, pending runtimePendingToolCall, slot *runtimeChildProducerSlotV1, request domainjob.StartRequest) (domainjob.Record, error) {
	var record domainjob.Record
	if h.pendingWork == nil {
		return record, errors.New("child intent authority is unavailable")
	}
	err := h.pendingWork.UseChildProducerV1(ctx, pending, slot.target, time.Now().UTC(), func() error {
		if !reflect.DeepEqual(request.CaseDelegation, slot.preparation.Request.CaseDelegation) {
			return errors.New("child case preparation differs from queued job")
		}
		if err := slot.preparation.Request.ApplyPreparedCaseDelegationV1(ctx, pending.SecurityContext); err != nil {
			return err
		}
		var err error
		record, err = h.jobs.StartReservedChildRunV1(ctx, request, slot.job)
		return err
	})
	return record, err
}

type reservedChildThreadStoreV1 struct {
	ctx  context.Context
	slot *runtimeChildProducerSlotV1
}

func (store reservedChildThreadStoreV1) CreateThread(request map[string]any, workspace string) (map[string]any, error) {
	return store.slot.owner.store.createReservedChildThreadV1(store.ctx, store.slot.thread, request, workspace)
}

func (store reservedChildThreadStoreV1) ForkThread(source string, request map[string]any) (map[string]any, error) {
	return store.slot.owner.store.forkReservedChildThreadV1(store.ctx, store.slot.thread, source, request)
}

func (store reservedChildThreadStoreV1) PatchThread(threadID string, patch map[string]any) (map[string]any, error) {
	if threadID != store.slot.target.ChildThreadID || store.ctx.Err() != nil {
		return nil, errors.New("child patch differs from signed host target")
	}
	return store.slot.owner.store.PatchThread(threadID, patch)
}
