package loop

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
)

// ProtectedLaneUnavailableV1 is process-local loop state. It carries no
// execution, evidence, or publication authority; it only lets the runner
// continue an independently authorized ordinary lane after the exact
// protected capability became unavailable without an uncertain protected
// effect (including a canonical, durably settled public error result).
type ProtectedLaneUnavailableV1 struct {
	Code string
}

// RecoverableProtectedLaneFailureCodeV1 recognizes only closed, typed
// capability-currentness failures. The one entity-reference code means the
// exact host provenance set has no current member and is emitted before any
// grant or runner effect. It deliberately excludes provider identity,
// workspace, case, risk, generic context, schema, settlement, persistence,
// signer, and post-admission execution-integrity failures.
func RecoverableProtectedLaneFailureCodeV1(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		code := ""
		for _, cause := range joined.Unwrap() {
			if cause == nil {
				continue
			}
			current, recoverable := RecoverableProtectedLaneFailureCodeV1(cause)
			if !recoverable || (code != "" && current != code) {
				return "", false
			}
			code = current
		}
		return code, code != ""
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok && wrapped.Unwrap() != nil {
		return RecoverableProtectedLaneFailureCodeV1(wrapped.Unwrap())
	}
	classifyTurnFailure := func(turnFailure TurnFailureError) (string, bool) {
		if code, ok := recoverableProtectedLaneCodeV1(turnFailure.Code); ok {
			return code, true
		}
		if strings.TrimSpace(turnFailure.Code) != "execution_grant_rejected" {
			return "", false
		}
		code, _ := turnFailure.Details["code"].(string)
		return recoverableProtectedLaneCodeV1(code)
	}
	var turnFailure TurnFailureError
	if errors.As(err, &turnFailure) {
		return classifyTurnFailure(turnFailure)
	}
	var turnFailurePointer *TurnFailureError
	if errors.As(err, &turnFailurePointer) && turnFailurePointer != nil {
		return classifyTurnFailure(*turnFailurePointer)
	}
	var validation executiongrantapp.ValidationError
	if errors.As(err, &validation) {
		return recoverableProtectedLaneCodeV1(validation.Code)
	}
	var validationPointer *executiongrantapp.ValidationError
	if errors.As(err, &validationPointer) && validationPointer != nil {
		return recoverableProtectedLaneCodeV1(validationPointer.Code)
	}
	return "", false
}

// RecoverableProtectedLaneProjectionCodeV1 extracts the same closed code from
// a strictly validated public tool-result projection. Semantic status alone
// is intentionally insufficient because it cannot identify the exact host
// blocker.
func RecoverableProtectedLaneProjectionCodeV1(projection domaintoolresult.PublicToolResultProjectionV1) (string, bool) {
	if domaintoolresult.ValidatePublicToolResultProjectionV1(projection) != nil ||
		projection.Status == "completed" {
		return "", false
	}
	return recoverableProtectedLaneCodeV1(projection.Code)
}

// RecoverableProtectedLaneToolMessageCodeV1 accepts only a canonical JSON
// public projection carried by a paired tool message. It never scans or
// interprets free-form tool text.
func RecoverableProtectedLaneToolMessageCodeV1(message domainmodel.Message) (string, bool) {
	if strings.TrimSpace(message.Role) != "tool" || strings.TrimSpace(message.ToolCallID) == "" ||
		strings.TrimSpace(message.Content) == "" {
		return "", false
	}
	decoder := json.NewDecoder(strings.NewReader(message.Content))
	decoder.DisallowUnknownFields()
	var projection domaintoolresult.PublicToolResultProjectionV1
	if err := decoder.Decode(&projection); err != nil {
		return "", false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", false
	}
	return RecoverableProtectedLaneProjectionCodeV1(projection)
}

func recoverableProtectedLaneCodeV1(code string) (string, bool) {
	code = strings.TrimSpace(code)
	switch code {
	case "tool_source_unavailable",
		"execution_grant_source_unavailable",
		executionGrantEntityReferenceUnavailableV1,
		"source_probe_unavailable",
		"mcp_source_probe_mismatch",
		"turn_security_dataset_snapshot_mismatch",
		"execution_grant_connection_epoch_mismatch",
		"mcp_connection_epoch_mismatch":
		return code, true
	default:
		return "", false
	}
}
