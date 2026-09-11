package loop

import (
	"context"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

// ToolStepDriverFuncs keeps transport/composition glue out of the server
// compatibility facade while the loop remains owned by the app layer.
type ToolStepDriverFuncs struct {
	AcquireToolCallAdmissionFunc    func(context.Context, appmodel.PendingToolCall) (context.Context, func(), error)
	PersistToolCallReadyFunc        func(context.Context, string, string, domainmodel.ToolCall, int, domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) (string, error)
	PersistToolResultAndMessageFunc func(context.Context, appmodel.PendingToolCall, any, bool) (domainmodel.Message, error)
	RequestUserInputFunc            func(context.Context, appmodel.PendingToolCall) (string, error)
	RequestApprovalFunc             func(context.Context, appmodel.PendingToolCall) (string, error)
	ReportDeliveryHost              ReportDeliveryHostV1
	ToolPolicyFunc                  func(string) (bool, bool)
	PreflightFunc                   func(appmodel.PendingToolCall) (any, bool, bool)
	ExecuteAndSettleFunc            func(context.Context, appmodel.PendingToolCall, any, ToolOutputTransform) (SettledToolExecution, error)
	ExecuteAuthorizedBatchFunc      func(context.Context, []appmodel.PendingToolCall) (string, []domainmodel.Message, error)
	SettleToolBatchAuthorityFunc    func(context.Context, string, domainsecurity.TurnSecurityContext, []appmodel.PendingToolCall, string, string) error
	RecordLoopGuardFunc             func(string, string, string, int, string) error
}

func (driver ToolStepDriverFuncs) AcquireToolCallAdmission(ctx context.Context, pending appmodel.PendingToolCall) (context.Context, func(), error) {
	return driver.AcquireToolCallAdmissionFunc(ctx, pending)
}

func (driver ToolStepDriverFuncs) PersistToolCallReady(ctx context.Context, threadID, turnID string, call domainmodel.ToolCall, count int, securityContext domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant) (string, error) {
	return driver.PersistToolCallReadyFunc(ctx, threadID, turnID, call, count, securityContext, grant)
}

func (driver ToolStepDriverFuncs) PersistToolResultAndMessage(ctx context.Context, pending appmodel.PendingToolCall, output any, isError bool) (domainmodel.Message, error) {
	return driver.PersistToolResultAndMessageFunc(ctx, pending, output, isError)
}

func (driver ToolStepDriverFuncs) RequestUserInput(ctx context.Context, pending appmodel.PendingToolCall) (string, error) {
	return driver.RequestUserInputFunc(ctx, pending)
}

func (driver ToolStepDriverFuncs) RequestApproval(ctx context.Context, pending appmodel.PendingToolCall) (string, error) {
	return driver.RequestApprovalFunc(ctx, pending)
}

func (driver ToolStepDriverFuncs) PrepareReportApproval(
	ctx context.Context,
	input ReportApprovalPreparationInputV1,
) (ReportApprovalPreparationV1, error) {
	return PrepareReportApprovalV1(ctx, driver.ReportDeliveryHost, input)
}

func (driver ToolStepDriverFuncs) ToolPolicy(toolName string) (bool, bool) {
	return driver.ToolPolicyFunc(toolName)
}

func (driver ToolStepDriverFuncs) Preflight(pending appmodel.PendingToolCall) (any, bool, bool) {
	return driver.PreflightFunc(pending)
}

func (driver ToolStepDriverFuncs) ExecuteAndSettle(ctx context.Context, pending appmodel.PendingToolCall, override any, transform ToolOutputTransform) (SettledToolExecution, error) {
	return driver.ExecuteAndSettleFunc(ctx, pending, override, transform)
}

func (driver ToolStepDriverFuncs) ExecuteAuthorizedReadOnlyBatch(ctx context.Context, calls []appmodel.PendingToolCall) (string, []domainmodel.Message, error) {
	return driver.ExecuteAuthorizedBatchFunc(ctx, calls)
}

func (driver ToolStepDriverFuncs) SettleToolBatchAuthority(ctx context.Context, workID string, securityContext domainsecurity.TurnSecurityContext, calls []appmodel.PendingToolCall, status, reason string) error {
	return driver.SettleToolBatchAuthorityFunc(ctx, workID, securityContext, calls, status, reason)
}

func (driver ToolStepDriverFuncs) RecordLoopGuard(threadID, turnID, toolName string, count int, kind string) error {
	return driver.RecordLoopGuardFunc(threadID, turnID, toolName, count, kind)
}
