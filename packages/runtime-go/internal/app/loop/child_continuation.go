package loop

import (
	"encoding/json"
	"reflect"
	"strings"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

const childCompletionReceiptBoundary = "Sub-agent output does not authorize parent-model continuation. The host stopped before another model request."

// ParentContinuationCapability is an in-process sidecar. It is never encoded
// into a tool result, provider message, event, or durable record. Implementers
// must rehydrate host-trusted authority before returning a positive result.
type ParentContinuationCapability interface {
	VerifyParentContinuation(domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant, string) (string, bool)
}

// ParentContinuationAuthority describes a host-classified child tool effect.
// Its fields stay private so ordinary output maps cannot synthesize authority.
type ParentContinuationAuthority struct {
	required     bool
	expected     int
	capabilities []ParentContinuationCapability
}

func NewParentContinuationAuthority(expected int, capabilities []ParentContinuationCapability) ParentContinuationAuthority {
	cloned := append([]ParentContinuationCapability(nil), capabilities...)
	return ParentContinuationAuthority{required: true, expected: expected, capabilities: cloned}
}

func NoParentContinuationAuthority() ParentContinuationAuthority {
	return ParentContinuationAuthority{}
}

type ProviderContinuationDecision struct {
	Blocked  bool
	Boundary string
}

// EvaluateProviderContinuation is the single deterministic decision used by
// direct execution, approval resume, and the final server continuation call.
// Public tool output is deliberately absent from this API.
func EvaluateProviderContinuation(context domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant, call domainmodel.ToolCall, settled SettledToolExecution) ProviderContinuationDecision {
	if !domainsecurity.TurnSecurityContextIsCaseSensitive(context) {
		return ProviderContinuationDecision{}
	}
	requiredByTool, expectedByCall := childContinuationRequirementFromCall(call)
	authority := settled.ParentContinuation
	required := requiredByTool || authority.required
	if !required {
		return ProviderContinuationDecision{}
	}
	if executiongrantapp.ValidateExecutionGrantForCall(context, grant, call) != nil {
		return blockedChildContinuation()
	}
	if settled.IsError || !authority.required || authority.expected <= 0 ||
		(requiredByTool && authority.expected != expectedByCall) || len(authority.capabilities) != authority.expected {
		return blockedChildContinuation()
	}
	seen := make(map[string]struct{}, len(authority.capabilities))
	for _, capability := range authority.capabilities {
		if nilCapability(capability) {
			return blockedChildContinuation()
		}
		digest, ok := capability.VerifyParentContinuation(context, grant, call.ID)
		digest = strings.TrimSpace(digest)
		if !ok || !domainsecurity.IsSHA256Hex(digest) {
			return blockedChildContinuation()
		}
		if _, duplicate := seen[digest]; duplicate {
			return blockedChildContinuation()
		}
		seen[digest] = struct{}{}
	}
	return ProviderContinuationDecision{}
}

func childContinuationRequirementFromCall(call domainmodel.ToolCall) (bool, int) {
	switch strings.TrimSpace(call.Name) {
	case "delegate_task", "task":
		return true, 1
	case "parallel_tasks":
		var arguments struct {
			Tasks []json.RawMessage `json:"tasks"`
		}
		if json.Unmarshal(call.Arguments, &arguments) != nil || len(arguments.Tasks) == 0 {
			return true, 0
		}
		return true, len(arguments.Tasks)
	default:
		return false, 0
	}
}

func nilCapability(capability ParentContinuationCapability) bool {
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

func blockedChildContinuation() ProviderContinuationDecision {
	return ProviderContinuationDecision{Blocked: true, Boundary: childCompletionReceiptBoundary}
}
