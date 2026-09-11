package loop

import (
	"context"
	"errors"
	"strings"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const (
	toolSettlementTimeout          = 15 * time.Second
	accountFlowSettlementTimeoutV1 = 60 * time.Second
)

type AcquireToolEffect func(context.Context, domainsecurity.TurnSecurityContext) (context.Context, func(), error)

type ToolOutputTransform func(any, bool) (any, bool, error)

type ToolSettlement func(context.Context, appmodel.PendingToolCall, any, bool) (SettledToolExecution, error)

// ToolDispatchOverride is a host-owned outcome produced before a physical
// side effect starts. Dispatch boundaries use it for deterministic owner
// validation failures, which must settle as tool failures without minting an
// execution intent or invoking the tool.
type ToolDispatchOverride struct {
	Output  any
	IsError bool
}

type ToolDispatchRun func(context.Context, *ToolDispatchOverride) (SettledToolExecution, error)

// ToolDispatchBoundary wraps the physical effect and durable settlement with
// host-owned dispatch intent. It is nil for effect-free or separately leased
// operations; required write-effect policy is enforced by the composition
// boundary that selects it.
type ToolDispatchBoundary func(context.Context, appmodel.PendingToolCall, ToolDispatchRun) (SettledToolExecution, error)

type SettledToolExecution struct {
	Output             any
	IsError            bool
	Message            domainmodel.Message
	ParentContinuation ParentContinuationAuthority `json:"-"`
	ReportTerminal     ReportTerminalCapability    `json:"-"`
}

func (settled SettledToolExecution) AuthorityFailureCode() string {
	if !settled.IsError {
		return ""
	}
	record, _ := settled.Output.(map[string]any)
	code, _ := record["code"].(string)
	code = strings.TrimSpace(code)
	if strings.HasPrefix(code, "execution_grant_") || strings.HasPrefix(code, "turn_security_") ||
		code == "mcp_host_context_invalid" || code == "mcp_connection_epoch_mismatch" || code == "mcp_source_probe_mismatch" {
		return code
	}
	return ""
}

// ExecuteAndSettleTool holds one effect authority lease from the last grant
// revalidation through the external effect and its durable grant/evidence
// settlement. Once the effect has started, cancellation cannot open a gap in
// which a context transition wins before the outcome is durably classified.
func ExecuteAndSettleTool(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	acquire AcquireToolEffect,
	execute func(context.Context, appmodel.PendingToolCall, any) (any, bool),
	override any,
	transform ToolOutputTransform,
	settle ToolSettlement,
	dispatch ToolDispatchBoundary,
) (SettledToolExecution, error) {
	return executeAndSettleToolWithAccountFlowSourceV1(ctx, pending, acquire, execute, override, transform, settle, dispatch, nil)
}

func executeAndSettleToolWithAccountFlowSourceV1(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	acquire AcquireToolEffect,
	execute func(context.Context, appmodel.PendingToolCall, any) (any, bool),
	override any,
	transform ToolOutputTransform,
	settle ToolSettlement,
	dispatch ToolDispatchBoundary,
	accountFlowSource any,
) (SettledToolExecution, error) {
	if acquire == nil || execute == nil || settle == nil {
		return SettledToolExecution{}, errors.New("tool effect settlement dependencies are unavailable")
	}
	effectCtx, release, err := acquire(ctx, pending.SecurityContext)
	if err != nil {
		return SettledToolExecution{}, err
	}
	if effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return SettledToolExecution{}, errors.New("tool effect authority returned an invalid lease")
	}
	defer release()

	run := func(runCtx context.Context, dispatchOverride *ToolDispatchOverride) (SettledToolExecution, error) {
		output, isError := any(nil), false
		if dispatchOverride != nil {
			output, isError = dispatchOverride.Output, dispatchOverride.IsError
		} else {
			output, isError = execute(runCtx, pending, override)
		}
		timeout := toolSettlementTimeout
		if dispatchOverride == nil && override == nil && actualAccountFlowSettlementV1(pending, output, isError, accountFlowSource) {
			timeout = accountFlowSettlementTimeoutV1
		}
		var transformErr error
		if transform != nil {
			output, isError, transformErr = transform(output, isError)
		}
		settlementCtx, cancel := detachedToolSettlementContextWithTimeout(runCtx, timeout)
		defer cancel()
		settled, settlementErr := settle(settlementCtx, pending, output, isError)
		return settled, errors.Join(transformErr, settlementErr)
	}
	if dispatch != nil {
		return dispatch(effectCtx, pending, run)
	}
	return run(effectCtx, nil)
}

// SettleToolOutput gives synthetic outcomes (cancellation, denied approval,
// preflight rejection, and user input) the same context-transition exclusion
// and durable settlement boundary as a real tool effect.
func SettleToolOutput(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	acquire AcquireToolEffect,
	output any,
	isError bool,
	settle ToolSettlement,
) (SettledToolExecution, error) {
	return settleToolOutputWithTimeout(ctx, pending, acquire, output, isError, settle, toolSettlementTimeout)
}

// SettleReadOnlyBatchToolOutputV1 is called while the real batch effect lease
// remains held. Only the existing Host's live result carrier can select the
// native budget; synthetic cancelled batch members still use the default.
func SettleReadOnlyBatchToolOutputV1(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	acquire AcquireToolEffect,
	output any,
	isError bool,
	settle ToolSettlement,
	accountFlowSource any,
) (SettledToolExecution, error) {
	timeout := toolSettlementTimeout
	if actualAccountFlowSettlementV1(pending, output, isError, accountFlowSource) {
		timeout = accountFlowSettlementTimeoutV1
	}
	return settleToolOutputWithTimeout(ctx, pending, acquire, output, isError, settle, timeout)
}

func settleToolOutputWithTimeout(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	acquire AcquireToolEffect,
	output any,
	isError bool,
	settle ToolSettlement,
	timeout time.Duration,
) (SettledToolExecution, error) {
	if acquire == nil || settle == nil {
		return SettledToolExecution{}, errors.New("tool output settlement dependencies are unavailable")
	}
	settlementCtx, cancel := detachedToolSettlementContextWithTimeout(ctx, timeout)
	defer cancel()
	effectCtx, release, err := acquire(settlementCtx, pending.SecurityContext)
	if err != nil {
		return SettledToolExecution{}, err
	}
	if effectCtx == nil || release == nil {
		if release != nil {
			release()
		}
		return SettledToolExecution{}, errors.New("tool settlement authority returned an invalid lease")
	}
	defer release()
	settled, settlementErr := settle(effectCtx, pending, output, isError)
	return settled, settlementErr
}

// The existing Host reader verifies the opaque, live native result carrier.
// A tool name, output flag, or policy choice never supplies execution authority.
func actualAccountFlowSettlementV1(pending appmodel.PendingToolCall, output any, isError bool, source any) bool {
	if isError || pending.Call.Name != accountFlowProviderToolNameV1 || !pending.ExecutionGrant.ReadOnly ||
		executiongrantapp.ValidateExecutionGrantForCall(pending.SecurityContext, pending.ExecutionGrant, pending.Call) != nil {
		return false
	}
	record, ok := output.(map[string]any)
	raw, rawOK := record[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	reader, readerOK := source.(accountFlowProviderSemanticReaderV1)
	if !ok || !rawOK || !readerOK || !domainmcp.ValidLosslessToolResult(raw) {
		return false
	}
	_, valid := reader.HostFundsAccountFlowProviderSemanticV1(raw)
	return valid
}

func detachedToolSettlementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return detachedToolSettlementContextWithTimeout(ctx, toolSettlementTimeout)
}

func detachedToolSettlementContextWithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}
