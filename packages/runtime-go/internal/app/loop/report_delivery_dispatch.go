package loop

import (
	"context"
	"reflect"
	"strings"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

type reportDeliveryDispatchV1 struct {
	projection domaintoolresult.PublicToolResultProjectionV1
	terminal   ReportTerminalCapability
	cause      error
}

// ReportDeliveryAuthorizationFailureV1 converts a report authorization
// failure into an opaque dispatch result so settlement reaches the dedicated
// terminal fallback instead of persisting a generic model-visible error.
func ReportDeliveryAuthorizationFailureV1(
	pending appmodel.PendingToolCall,
	ordinaryOutput any,
) (any, bool) {
	if strings.TrimSpace(pending.Call.Name) == toolcatalogapp.ReportDeliveryToolName {
		return reportDeliveryDispatchV1{cause: ReportPublicationFallbackErrorV1()}, false
	}
	return ordinaryOutput, true
}

func DispatchReportDeliveryWithinHeldContextEffectV1(
	ctx context.Context,
	host ReportDeliveryHostV1,
	pending appmodel.PendingToolCall,
	override any,
	validateApproved func(appmodel.PendingToolCall) error,
) (any, bool) {
	if host == nil || ctx == nil || ctx.Err() != nil || override != nil ||
		pending.ExecutionGrant.ApprovalState != "approved" ||
		pending.ApprovalTransition == nil || validateApproved == nil ||
		validateApproved(pending) != nil {
		return reportDeliveryDispatchV1{cause: ReportPublicationFallbackErrorV1()}, false
	}
	projection, terminal, err := host.ExecuteReportWithinHeldContextEffectV1(ctx, pending)
	expected := domaintoolresult.WithheldProjectionV1("completed", "tool_output_private")
	if err != nil ||
		domaintoolresult.ValidatePublicToolResultProjectionV1(projection) != nil ||
		!reflect.DeepEqual(projection, expected) ||
		nilReportTerminalCapabilityV1(terminal) {
		return reportDeliveryDispatchV1{cause: ReportPublicationFallbackErrorV1()}, false
	}
	return reportDeliveryDispatchV1{projection: projection, terminal: terminal}, false
}

func SettleReportDeliveryDispatchV1(
	pending appmodel.PendingToolCall,
	output any,
) (SettledToolExecution, bool, error) {
	if strings.TrimSpace(pending.Call.Name) != toolcatalogapp.ReportDeliveryToolName {
		return SettledToolExecution{}, false, nil
	}
	dispatched, ok := output.(reportDeliveryDispatchV1)
	if !ok || dispatched.cause != nil ||
		domaintoolresult.ValidatePublicToolResultProjectionV1(dispatched.projection) != nil ||
		nilReportTerminalCapabilityV1(dispatched.terminal) {
		if ok && dispatched.cause != nil {
			return SettledToolExecution{}, true, dispatched.cause
		}
		return SettledToolExecution{}, true, ReportPublicationFallbackErrorV1()
	}
	publicOutput := domaintoolresult.PublicToolResultProjectionRecordV1(
		dispatched.projection,
	)
	return SettledToolExecution{
		Output: publicOutput,
		Message: toolcatalogapp.ToolResultMessage(
			pending.Call,
			appmodel.ToolResultContent(publicOutput),
		),
		ReportTerminal: dispatched.terminal,
	}, true, nil
}
