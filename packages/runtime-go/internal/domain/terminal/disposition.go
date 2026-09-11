package terminal

import (
	"strings"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

type Disposition struct {
	Reason           string
	Status           string
	CandidateAllowed bool
}

type PublicFailureProjectionV1 struct {
	Status   string
	Code     string
	Message  string
	Severity string
}

var dispositionsV1 = []Disposition{
	{Reason: "success", Status: "completed", CandidateAllowed: true},
	{Reason: "source_unavailable", Status: "completed"},
	{Reason: "semantic_failure", Status: "failed"},
	{Reason: "provider_failure", Status: "failed"},
	{Reason: "cancel", Status: "aborted"},
	{Reason: "timeout", Status: "failed"},
	{Reason: "stream_abort", Status: "failed"},
	{Reason: "recovery", Status: "completed"},
	{Reason: "approval", Status: "completed", CandidateAllowed: true},
	{Reason: "user_input", Status: "completed", CandidateAllowed: true},
	{Reason: "resume", Status: "completed", CandidateAllowed: true},
	{Reason: "restart", Status: "aborted"},
	{Reason: "report_fallback", Status: "failed"},
	{Reason: "step_limit", Status: "failed"},
	{Reason: "background_completion", Status: "completed", CandidateAllowed: true},
	{Reason: "tool_failure", Status: "failed"},
	{Reason: "approval_denied", Status: "completed"},
	{Reason: "input_cancelled", Status: "completed"},
}

func AllDispositionsV1() []Disposition {
	out := make([]Disposition, len(dispositionsV1))
	copy(out, dispositionsV1)
	return out
}

func LookupV1(reason string) (Disposition, bool) {
	reason = strings.TrimSpace(reason)
	for _, disposition := range dispositionsV1 {
		if disposition.Reason == reason {
			return disposition, true
		}
	}
	return Disposition{}, false
}

func StatusForReasonV1(reason string) (string, bool) {
	disposition, ok := LookupV1(reason)
	return disposition.Status, ok
}

func CandidateAllowedV1(reason string) bool {
	disposition, ok := LookupV1(reason)
	return ok && disposition.CandidateAllowed
}

// FailureProjectionV1 returns the closed, fact-free failure vocabulary used by
// host-authored case terminal items and events. It contains no provider, tool,
// process, or job error bytes and is safe to validate before the atomic public
// terminal CAS has installed its winner.
func FailureProjectionV1(reason string) (PublicFailureProjectionV1, bool) {
	disposition, ok := LookupV1(reason)
	if !ok {
		return PublicFailureProjectionV1{}, false
	}
	projection := PublicFailureProjectionV1{
		Status: disposition.Status,
		Code:   "case_terminal_" + disposition.Reason,
	}
	switch disposition.Status {
	case "completed":
		projection.Message = "案件分析已结束；仅发布通过宿主证据门的固定边界答复。"
		projection.Severity = "warning"
	case "aborted":
		projection.Message = "案件分析已终止；未经核验的案件事实未发布。"
		projection.Severity = "warning"
	case "failed":
		projection.Message = "案件分析未完成；未经核验的案件事实未发布。"
		projection.Severity = "error"
	default:
		return PublicFailureProjectionV1{}, false
	}
	return projection, true
}

// GeneralFailureRecordV1 is the single closed mapping from an ordinary
// terminal reason to the fact-free host failure record that may cross public
// persistence and delivery boundaries. Candidate-bearing terminal reasons do
// not have a failure projection.
func GeneralFailureRecordV1(reason string) (domainfailure.Record, bool) {
	disposition, ok := LookupV1(reason)
	if !ok || disposition.CandidateAllowed {
		return domainfailure.Record{}, false
	}
	code := ""
	switch disposition.Reason {
	case "source_unavailable":
		code = "source_probe_unavailable"
	case "semantic_failure":
		code = "validation_error"
	case "provider_failure":
		code = domainfailure.CodeProviderError
	case "cancel":
		code = domainfailure.CodeTurnCancelled
	case "timeout":
		code = domainfailure.CodeProviderTimeout
	case "stream_abort":
		code = domainfailure.CodeProviderStreamInterrupted
	case "recovery":
		code = "turn_recovery_boundary"
	case "restart":
		code = "runtime_restarted"
	case "report_fallback":
		code = "publication_receipt_required"
	case "step_limit":
		code = "turn_step_limit_exceeded"
	case "tool_failure":
		code = "tool_failure_storm"
	case "approval_denied":
		code = "approval_denied"
	case "input_cancelled":
		code = "input_cancelled"
	default:
		return domainfailure.Record{}, false
	}
	record := domainfailure.New(code, nil)
	if record.Code() != code || record.Details() != nil {
		return domainfailure.Record{}, false
	}
	return record, true
}

// GeneralFailureRecordForCauseV1 selects the narrow public failure code that
// may accompany an ordinary terminal reason. Most reasons deliberately keep
// their single fixed projection. Tool, provider, and post-provider host
// semantic failures retain small, already-public code vocabularies so an
// immediate rejection is never collapsed into an unrelated generic failure.
// The one tool_not_advertised diagnostic projection is retained only when it
// matches its exact hash-and-enum-only public shape.
func GeneralFailureRecordForCauseV1(reason string, cause domainfailure.Record) (domainfailure.Record, bool) {
	fallback, ok := GeneralFailureRecordV1(reason)
	if !ok {
		return domainfailure.Record{}, false
	}
	disposition, _ := LookupV1(reason)
	if disposition.Reason != "tool_failure" && disposition.Reason != "semantic_failure" &&
		disposition.Reason != "provider_failure" {
		return fallback, true
	}
	code := cause.Code()
	if _, allowed := GeneralFailureProjectionForCodeV1(disposition.Reason, code); !allowed {
		if disposition.Reason == "semantic_failure" || disposition.Reason == "provider_failure" {
			return fallback, true
		}
		code = domainfailure.CodeTurnFailed
	}
	var details map[string]any
	if code == "tool_not_advertised" && domainfailure.ValidateToolNotAdvertisedDetails(cause.Details()) {
		details = cause.Details()
	}
	record := domainfailure.New(code, details)
	if record.Code() != code || (details == nil) != (record.Details() == nil) {
		return domainfailure.Record{}, false
	}
	return record, true
}

// GeneralFailureProjectionForCodeV1 validates an exact reason/code pair and
// reconstructs its host-authored public projection. The legacy reason-only
// projection remains valid for durable replay, while tool, provider, and host
// semantic terminals may preserve only the closed codes enumerated here.
func GeneralFailureProjectionForCodeV1(reason, code string) (PublicFailureProjectionV1, bool) {
	disposition, ok := LookupV1(reason)
	if !ok || disposition.CandidateAllowed {
		return PublicFailureProjectionV1{}, false
	}
	fallback, ok := GeneralFailureRecordV1(disposition.Reason)
	if !ok {
		return PublicFailureProjectionV1{}, false
	}
	code = strings.TrimSpace(code)
	if code != fallback.Code() &&
		(disposition.Reason != "tool_failure" || !generalToolFailureCodeV1(code)) &&
		(disposition.Reason != "provider_failure" || !generalProviderFailureCodeV1(code)) &&
		(disposition.Reason != "semantic_failure" || !generalSemanticFailureCodeV1(code)) {
		return PublicFailureProjectionV1{}, false
	}
	record := domainfailure.New(code, nil)
	if record.Code() != code || record.Details() != nil {
		return PublicFailureProjectionV1{}, false
	}
	return PublicFailureProjectionV1{
		Status: disposition.Status, Code: record.Code(), Message: record.Message(), Severity: record.Severity(),
	}, true
}

func generalProviderFailureCodeV1(code string) bool {
	switch strings.TrimSpace(code) {
	case domainfailure.CodeProviderAuthenticationFailed,
		domainfailure.CodeProviderRateLimited,
		domainfailure.CodeProviderInsufficientBalance,
		domainfailure.CodeProviderEndpointNotFound,
		domainfailure.CodeProviderRequestRejected,
		domainfailure.CodeProviderUnavailable,
		domainfailure.CodeProviderNetworkUnavailable,
		domainfailure.CodeProviderTimeout,
		domainfailure.CodeProviderStreamInterrupted,
		domainfailure.CodeProviderModelInvalid,
		domainfailure.CodeProviderNotConfigured,
		domainfailure.CodeProviderToolArgumentsInvalid,
		domainfailure.CodeProviderReasoningMarkupInvalid,
		domainfailure.CodeProviderEmptyFinal,
		"provider_stream_failed":
		return true
	default:
		return false
	}
}

func generalSemanticFailureCodeV1(code string) bool {
	switch strings.TrimSpace(code) {
	case domainfailure.CodeHostResponseEventFailed,
		domainfailure.CodeHostCandidateLifecycleFailed,
		domainfailure.CodeHostCandidateAuthorityFailed,
		domainfailure.CodeHostCandidateSteeringFailed,
		domainfailure.CodeHostCandidateProjectionFailed,
		domainfailure.CodeHostCandidatePublicationFailed,
		"context_window_hard_limit":
		return true
	default:
		return false
	}
}

func generalToolFailureCodeV1(code string) bool {
	switch strings.TrimSpace(code) {
	case domainfailure.CodeTurnFailed,
		domainfailure.CodeProviderToolArgumentsInvalid,
		"tool_call_identity_invalid",
		"tool_not_advertised",
		"tool_schema_missing",
		"tool_schema_invalid",
		"tool_private_arguments",
		"tool_invalid_arguments_storm",
		"tool_failure_storm":
		return true
	default:
		return false
	}
}

// GeneralFailureProjectionV1 binds the ordinary terminal lifecycle status to
// its exact host-authored code, message, and severity. It is transport
// authority only and cannot authorize facts, citations, or evidence.
func GeneralFailureProjectionV1(reason string) (PublicFailureProjectionV1, bool) {
	disposition, ok := LookupV1(reason)
	if !ok {
		return PublicFailureProjectionV1{}, false
	}
	record, ok := GeneralFailureRecordV1(reason)
	if !ok {
		return PublicFailureProjectionV1{}, false
	}
	return PublicFailureProjectionV1{
		Status: disposition.Status, Code: record.Code(), Message: record.Message(), Severity: record.Severity(),
	}, true
}

// AcceptedFinalDeliveryRequiresErrorItemV1 returns the canonical accepted-final
// delivery shape for a closed terminal reason. Failed and aborted terminals,
// plus the two completed denial/cancellation boundaries, carry a host-authored
// error item. Other completed terminals use the three-event shape.
func AcceptedFinalDeliveryRequiresErrorItemV1(reason string) (bool, bool) {
	disposition, ok := LookupV1(reason)
	if !ok {
		return false, false
	}
	if disposition.Status == "failed" || disposition.Status == "aborted" {
		return true, true
	}
	switch strings.TrimSpace(reason) {
	case "approval_denied", "input_cancelled":
		return true, true
	default:
		return false, true
	}
}
