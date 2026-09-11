package continuation

import "strings"

// ResolutionProjection is the only supported mapping from a signed
// continuation disposition to its public gate projection. Keeping the reason
// code in the mapping prevents a disposition from being reinterpreted as a
// different approval or user-input outcome during retry or restart.
type ResolutionProjection struct {
	Kind              string
	DispositionStatus string
	ReasonCode        string
	PublicStatus      string
	Decision          string
}

func ResolveProjection(kind, status, reasonCode string) (ResolutionProjection, bool) {
	kind = strings.TrimSpace(kind)
	status = strings.TrimSpace(status)
	reasonCode = strings.TrimSpace(reasonCode)
	projection := ResolutionProjection{
		Kind: kind, DispositionStatus: status, ReasonCode: reasonCode,
	}
	switch {
	case kind == KindApproval && status == StatusAllowed && reasonCode == "approval_allowed":
		projection.PublicStatus, projection.Decision = "allowed", "allow"
	case kind == KindApproval && status == StatusDenied && reasonCode == "approval_denied":
		projection.PublicStatus, projection.Decision = "denied", "deny"
	case kind == KindUserInput && status == StatusSubmitted && reasonCode == "user_input_submitted":
		projection.PublicStatus = "submitted"
	case kind == KindUserInput && status == StatusCancelled && reasonCode == "user_input_cancelled":
		projection.PublicStatus = "cancelled"
	default:
		return ResolutionProjection{}, false
	}
	return projection, true
}
