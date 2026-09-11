package loop

import (
	"context"
	"errors"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
)

type PreparedSideEffect struct {
	SemanticIdentity domainsideeffectidentity.IdentityV1
	Bind             func(context.Context) context.Context
	Rejection        *ToolDispatchOverride
	ChildProducer    *pendingworkapp.ChildProducerPlanV1
}

type SideEffectPreparer func(context.Context, appmodel.PendingToolCall, time.Time) (PreparedSideEffect, error)

type SideEffectIntentExecutionInput struct {
	Context            context.Context
	Pending            appmodel.PendingToolCall
	Override           any
	Transform          ToolOutputTransform
	Acquire            AcquireToolEffect
	Execute            func(context.Context, appmodel.PendingToolCall, any) (any, bool)
	Settle             ToolSettlement
	PendingWork        *pendingworkapp.Service
	Prepare            SideEffectPreparer
	ValidateApproved   func(appmodel.PendingToolCall) error
	ResolvedEffectFree func(appmodel.PendingToolCall) bool
	Now                func() time.Time
	// AccountFlowSource is the existing Host private-result reader, never provider input.
	AccountFlowSource any
}

func ExecuteAndSettleWithSideEffectIntent(input SideEffectIntentExecutionInput) (SettledToolExecution, error) {
	requiresIntent := RequiresSideEffectIntent(input.Pending, input.Override)
	if requiresIntent && input.ResolvedEffectFree != nil && input.ResolvedEffectFree(input.Pending) {
		requiresIntent = false
	}
	var dispatch ToolDispatchBoundary
	if requiresIntent {
		if input.PendingWork == nil || input.Prepare == nil {
			return SettledToolExecution{}, executiongrantapp.ValidationError{Code: "execution_grant_authority_unavailable"}
		}
		switch input.Pending.ExecutionGrant.ApprovalState {
		case "approved":
			if input.Pending.ApprovalTransition == nil || input.ValidateApproved == nil {
				return SettledToolExecution{}, executiongrantapp.ValidationError{Code: "execution_grant_approval_transition_missing"}
			}
			if err := input.ValidateApproved(input.Pending); err != nil {
				return SettledToolExecution{}, err
			}
		case "not_required":
			if input.Pending.ApprovalTransition != nil {
				return SettledToolExecution{}, executiongrantapp.ValidationError{Code: "execution_grant_approval_transition_invalid"}
			}
		default:
			return SettledToolExecution{}, executiongrantapp.ValidationError{Code: "execution_grant_invalid"}
		}
		dispatch = sideEffectIntentBoundary(input.PendingWork, input.Prepare, input.Now)
	}
	settled, err := executeAndSettleToolWithAccountFlowSourceV1(
		input.Context, input.Pending, input.Acquire, input.Execute, input.Override,
		input.Transform, input.Settle, dispatch, input.AccountFlowSource,
	)
	if !requiresIntent || err == nil {
		return settled, err
	}
	var duplicate pendingworkapp.SideEffectIntentDuplicateError
	if !errors.As(err, &duplicate) || duplicate.FirstGrantID == input.Pending.ExecutionGrant.GrantID {
		return settled, err
	}
	duplicateOutput, outputErr := toolcatalogapp.NewHostSideEffectDuplicateOutputV1(duplicate.IntentStatus)
	if outputErr != nil {
		return settled, outputErr
	}
	return SettleToolOutput(input.Context, input.Pending, input.Acquire, duplicateOutput, true, input.Settle)
}

func RequiresSideEffectIntent(pending appmodel.PendingToolCall, override any) bool {
	return override == nil && !pending.ExecutionGrant.ReadOnly &&
		pending.ExecutionGrant.ToolName != pendingworkapp.ReportStageToolName
}

func sideEffectIntentBoundary(service *pendingworkapp.Service, prepare SideEffectPreparer, now func() time.Time) ToolDispatchBoundary {
	return func(effectCtx context.Context, pending appmodel.PendingToolCall, run ToolDispatchRun) (settled SettledToolExecution, err error) {
		issuedAt := sideEffectNow(now)
		prepared, err := prepare(effectCtx, pending, issuedAt)
		if err != nil {
			return SettledToolExecution{}, err
		}
		if prepared.Rejection != nil {
			return run(effectCtx, prepared.Rejection)
		}
		request := pendingworkapp.SideEffectIntentRequest{
			Pending: pending, IssuedAt: issuedAt, SemanticIdentity: prepared.SemanticIdentity,
			ChildProducer: prepared.ChildProducer,
		}
		lease, err := service.BeginSideEffectIntent(effectCtx, request)
		if err != nil {
			return SettledToolExecution{}, err
		}
		if prepared.ChildProducer != nil {
			var endChildExecution context.CancelFunc
			effectCtx, endChildExecution = context.WithCancel(effectCtx)
			defer endChildExecution()
		}
		if err := service.VerifySideEffectIntentAtSend(effectCtx, lease, request, sideEffectNow(now)); err != nil {
			return SettledToolExecution{}, err
		}
		if prepared.ChildProducer != nil {
			effectCtx, err = service.BindChildProducerExecutionV1(effectCtx, lease, request)
			if err != nil {
				return SettledToolExecution{}, err
			}
		}
		if prepared.Bind != nil {
			effectCtx = prepared.Bind(effectCtx)
		}
		var runErr error
		func() {
			defer func() {
				if recover() != nil {
					runErr = errors.New("side effect dispatch panicked before durable settlement")
				}
			}()
			settled, runErr = run(effectCtx, nil)
		}()
		settlementCtx, cancel := context.WithTimeout(context.WithoutCancel(effectCtx), 15*time.Second)
		_, closeErr := service.CloseSideEffectIntentAfterSettlement(
			settlementCtx, lease, request, sideEffectNow(now),
		)
		cancel()
		return settled, errors.Join(runErr, closeErr)
	}
}

func sideEffectNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}
