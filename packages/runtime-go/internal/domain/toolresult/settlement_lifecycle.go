package toolresult

import "errors"

var ErrToolResultLifecycleInvalidV1 = errors.New("tool result lifecycle is invalid")

// SettlementLifecycleStatusV1 validates semantic projection status against
// isError and returns the closed root item lifecycle. Root lifecycle is only
// completed/failed; blocked, cancelled, and exact restart outcome-unknown stay
// inside the typed output projection and cannot become open root states.
func SettlementLifecycleStatusV1(projection PublicToolResultProjectionV1, isError bool) (string, error) {
	if err := ValidatePublicToolResultProjectionV1(projection); err != nil {
		return "", errors.Join(ErrToolResultLifecycleInvalidV1, err)
	}
	if !settlementProjectionSemanticsValidV1(projection, isError) {
		return "", ErrToolResultLifecycleInvalidV1
	}
	switch projection.Status {
	case "completed":
		if isError {
			return "", ErrToolResultLifecycleInvalidV1
		}
		return "completed", nil
	case "failed", "blocked", "cancelled":
		if !isError {
			return "", ErrToolResultLifecycleInvalidV1
		}
		return "failed", nil
	case "unknown":
		if !isError || projection != OutcomeUnknownAfterRestartProjectionV1() {
			return "", ErrToolResultLifecycleInvalidV1
		}
		return "failed", nil
	default:
		return "", ErrToolResultLifecycleInvalidV1
	}
}

func settlementProjectionSemanticsValidV1(projection PublicToolResultProjectionV1, isError bool) bool {
	switch projection.ProjectionKind {
	case ProjectionWithheld:
		return projection.MessageKey == "tool_output_withheld"
	case ProjectionHostStatus:
		switch projection.Status {
		case "completed":
			return projection.MessageKey == "tool_completed"
		case "failed":
			return projection.MessageKey == "tool_failed"
		case "blocked":
			return projection.MessageKey == "tool_blocked"
		case "cancelled":
			return projection.MessageKey == "tool_cancelled"
		case "unknown":
			return projection == OutcomeUnknownAfterRestartProjectionV1()
		default:
			return false
		}
	case ProjectionArtifactStatus:
		return projection.Status == "completed" && projection.MessageKey == "artifact_created" && !isError
	case ProjectionPlanStatus:
		return (projection.Status == "completed" && projection.MessageKey == "plan_updated" && !isError) ||
			(projection.Status == "failed" && projection.MessageKey == "plan_failed" && isError)
	case ProjectionCaseSourceStatus:
		return (projection.Status == "completed" && projection.MessageKey == "case_source_private" && !isError) ||
			(projection.Status == "failed" && projection.MessageKey == "case_source_failed" && isError)
	case ProjectionMCPDiagnostic:
		return isError && projection.Status == "failed" && projection.MessageKey == "mcp_request_rejected"
	default:
		return false
	}
}
