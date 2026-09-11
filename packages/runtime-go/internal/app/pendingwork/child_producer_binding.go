package pendingwork

import (
	"context"
	"reflect"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
)

// ChildProducerPlanV1 is a process-local host preparation. It cannot be
// populated from tool arguments or persisted receipt JSON. The allocator
// revalidation must still own every exact reservation at issuance and send.
// A durable receipt is only an association witness after restart, never a plan.
type ChildProducerPlanV1 struct {
	context    domainsecurity.TurnSecurityContext
	grant      domainsecurity.ExecutionGrant
	semantic   domainsideeffectidentity.IdentityV1
	producer   *domainpendingwork.ChildProducerV1
	revalidate func(context.Context) error
}

// NewChildProducerPlanV1 is called by the host preparer after the ordinary
// task/profile/source admission owners have validated the complete request.
// revalidate must check its private allocator reservations, not caller metadata.
func NewChildProducerPlanV1(
	pending appmodel.PendingToolCall,
	semantic domainsideeffectidentity.IdentityV1,
	targets []domainpendingwork.ChildProducerTargetV1,
	revalidate func(context.Context) error,
) (ChildProducerPlanV1, error) {
	binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil || revalidate == nil || !childProducingSideEffect(pending.ExecutionGrant.ToolName) {
		return ChildProducerPlanV1{}, ErrOperationMismatch
	}
	plan := ChildProducerPlanV1{
		context: pending.SecurityContext, grant: pending.ExecutionGrant, semantic: semantic,
		producer: &domainpendingwork.ChildProducerV1{ParentBindingDigest: binding.BindingDigest,
			Children: append([]domainpendingwork.ChildProducerTargetV1(nil), targets...)},
		revalidate: revalidate,
	}
	if _, err := childProducerForRequest(SideEffectIntentRequest{Pending: pending, SemanticIdentity: semantic, ChildProducer: &plan}); err != nil {
		return ChildProducerPlanV1{}, err
	}
	return plan, nil
}

func childProducingSideEffect(tool string) bool {
	switch tool {
	case "task", "delegate_task", "parallel_tasks", "run_skill":
		return true
	default:
		return false
	}
}

func childProducerForRequest(request SideEffectIntentRequest) (*domainpendingwork.ChildProducerV1, error) {
	pending, plan := request.Pending, request.ChildProducer
	if !childProducingSideEffect(pending.ExecutionGrant.ToolName) {
		if plan != nil {
			return nil, ErrOperationMismatch
		}
		return nil, nil
	}
	if plan == nil || plan.revalidate == nil || plan.context != pending.SecurityContext || plan.grant != pending.ExecutionGrant ||
		plan.semantic != request.SemanticIdentity || domainsideeffectidentity.ValidateV1(plan.semantic) != nil ||
		validatePendingGrantForCall(pending.SecurityContext, pending.ExecutionGrant, pending.Call) != nil ||
		domainpendingwork.ValidateChildProducerV1(plan.producer) != nil {
		return nil, ErrOperationMismatch
	}
	binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil || plan.producer.ParentBindingDigest != binding.BindingDigest {
		return nil, ErrOperationMismatch
	}
	count := 1
	if pending.ExecutionGrant.ToolName == "parallel_tasks" {
		arguments, err := domainsecurity.DecodeCanonicalJSONObject(pending.Call.Arguments)
		if err != nil {
			return nil, ErrOperationMismatch
		}
		tasks, ok := arguments["tasks"].([]any)
		if !ok || len(tasks) < 2 {
			return nil, ErrOperationMismatch
		}
		count = len(tasks)
	}
	if len(plan.producer.Children) != count {
		return nil, ErrOperationMismatch
	}
	for _, child := range plan.producer.Children {
		if child.ChildThreadID == pending.ThreadID || child.ChildTurnID == pending.TurnID {
			return nil, ErrOperationMismatch
		}
	}
	return domainpendingwork.CloneChildProducerV1(plan.producer), nil
}

func sameChildProducer(left, right *domainpendingwork.ChildProducerV1) bool {
	return reflect.DeepEqual(left, right)
}
