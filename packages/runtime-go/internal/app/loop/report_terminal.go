package loop

import (
	"context"
	"reflect"
	"strings"

	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

const reportDeliveryTerminalBoundaryV1 = "The requested report was delivered through the host publication authority."

// ReportTerminalCapability is an in-process sidecar created only after the
// immutable report stage has a fresh projected delivery outcome. It is never
// encoded into tool output, provider messages, SSE, history, or logs.
type ReportTerminalCapability interface {
	VerifyReportTerminal(
		domainsecurity.TurnSecurityContext,
		domainsecurity.ExecutionGrant,
		string,
	) (string, bool)
}

// ReportDeliveryHostV1 is the only neutral live capability exposed by the
// private report owner. The caller must already hold the exact context effect
// lease and validate any durable approval transition before invoking it.
type ReportDeliveryHostV1 interface {
	ExecuteReportWithinHeldContextEffectV1(
		context.Context,
		appmodel.PendingToolCall,
	) (
		domaintoolresult.PublicToolResultProjectionV1,
		ReportTerminalCapability,
		error,
	)
}

type ReportTerminalDecisionV1 struct {
	Completed bool
	Invalid   bool
	Boundary  string
}

// EvaluateReportTerminalV1 is the shared direct/approval-resume callback. A
// public successful output cannot end the model loop; only the private
// capability bound to the exact context, grant, call, and delivery outcome
// can do so.
func EvaluateReportTerminalV1(
	context domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	call domainmodel.ToolCall,
	settled SettledToolExecution,
) ReportTerminalDecisionV1 {
	if strings.TrimSpace(call.Name) != toolcatalogapp.ReportDeliveryToolName {
		return ReportTerminalDecisionV1{}
	}
	if settled.IsError {
		return ReportTerminalDecisionV1{}
	}
	if domainsecurity.ValidateTurnSecurityContextForCaseFactPublication(context) != nil ||
		domainsecurity.ValidateExecutionGrantForContext(grant, context) != nil ||
		grant.ToolName != toolcatalogapp.ReportDeliveryToolName ||
		grant.ToolCallID != strings.TrimSpace(call.ID) ||
		nilReportTerminalCapabilityV1(settled.ReportTerminal) {
		return ReportTerminalDecisionV1{Invalid: true}
	}
	digest, ok := settled.ReportTerminal.VerifyReportTerminal(context, grant, call.ID)
	if !ok || !domainsecurity.IsSHA256Hex(strings.TrimSpace(digest)) {
		return ReportTerminalDecisionV1{Invalid: true}
	}
	return ReportTerminalDecisionV1{
		Completed: true,
		Boundary:  reportDeliveryTerminalBoundaryV1,
	}
}

func nilReportTerminalCapabilityV1(capability ReportTerminalCapability) bool {
	if capability == nil {
		return true
	}
	value := reflect.ValueOf(capability)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func ReportPublicationFallbackErrorV1() error {
	return TurnFailureError{
		Message:  "report publication terminal authority is unavailable",
		Code:     "report_publication_fallback",
		Severity: "error",
	}
}
