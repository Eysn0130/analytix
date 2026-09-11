package terminal

import (
	"reflect"
	"strings"
	"testing"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
)

func TestDispositionsV1AreClosedUniqueAndCandidateSafe(t *testing.T) {
	dispositions := AllDispositionsV1()
	if len(dispositions) != 18 {
		t.Fatalf("terminal disposition count changed: %d", len(dispositions))
	}
	seen := map[string]bool{}
	for _, disposition := range dispositions {
		if seen[disposition.Reason] || disposition.Reason == "" ||
			(disposition.Status != "completed" && disposition.Status != "failed" && disposition.Status != "aborted") {
			t.Fatalf("invalid terminal disposition: %#v", disposition)
		}
		seen[disposition.Reason] = true
		if actual, ok := LookupV1(disposition.Reason); !ok || actual != disposition {
			t.Fatalf("terminal disposition lookup diverged: %#v", disposition)
		}
	}
	for _, reason := range []string{"source_unavailable", "semantic_failure", "provider_failure", "cancel", "timeout", "stream_abort", "recovery", "restart", "report_fallback", "step_limit", "tool_failure", "approval_denied", "input_cancelled"} {
		if CandidateAllowedV1(reason) {
			t.Fatalf("boundary/failure reason can publish a candidate: %s", reason)
		}
	}
	for _, reason := range []string{"success", "approval", "user_input", "resume", "background_completion"} {
		if !CandidateAllowedV1(reason) {
			t.Fatalf("candidate reason was not admitted: %s", reason)
		}
	}
	for _, disposition := range dispositions {
		requiresError, ok := AcceptedFinalDeliveryRequiresErrorItemV1(disposition.Reason)
		if !ok {
			t.Fatalf("accepted-final delivery profile missing: %s", disposition.Reason)
		}
		expected := disposition.Status == "failed" || disposition.Status == "aborted" ||
			disposition.Reason == "approval_denied" || disposition.Reason == "input_cancelled"
		if requiresError != expected {
			t.Fatalf("accepted-final delivery profile drifted for %s: got %v want %v", disposition.Reason, requiresError, expected)
		}
	}
	if _, ok := AcceptedFinalDeliveryRequiresErrorItemV1("invented"); ok {
		t.Fatal("unknown terminal reason received an accepted-final delivery profile")
	}
	if _, ok := LookupV1("invented"); ok {
		t.Fatal("unknown terminal reason was accepted")
	}
}

func TestGeneralToolFailureProjectionRetainsOnlyExactToolNotAdvertisedDiagnostics(t *testing.T) {
	details := map[string]any{
		"rejectedToolNormalizedNameSha256":       strings.Repeat("a", 64),
		"rejectedToolCategory":                   "known_builtin_not_advertised",
		"promptRoute":                            "tool_agent",
		"loopStep":                               float64(1),
		"advertisedToolCount":                    float64(7),
		"advertisedToolManifestHash":             strings.Repeat("b", 64),
		"advertisedNameSetSortedHash":            strings.Repeat("c", 64),
		"providerRequestToolManifestHash":        strings.Repeat("d", 64),
		"runToolStepManifestHash":                strings.Repeat("d", 64),
		"providerRequestRunToolStepManifestSame": true,
	}
	record, ok := GeneralFailureRecordForCauseV1(
		"tool_failure", domainfailure.New("tool_not_advertised", details),
	)
	if !ok || !reflect.DeepEqual(record.Details(), details) {
		t.Fatalf("closed unadvertised-tool diagnostics were not retained: record=%#v ok=%t", record, ok)
	}
	delete(details, "advertisedToolManifestHash")
	rejected, ok := GeneralFailureRecordForCauseV1(
		"tool_failure", domainfailure.New("tool_not_advertised", details),
	)
	if !ok || rejected.Details() != nil {
		t.Fatalf("incomplete unadvertised-tool diagnostics reached the terminal: record=%#v ok=%t", rejected, ok)
	}
}

func TestGeneralFailureProjectionV1CoversEveryNonCandidateReasonExactly(t *testing.T) {
	expectedCodes := map[string]string{
		"source_unavailable": "source_probe_unavailable",
		"semantic_failure":   "validation_error",
		"provider_failure":   "provider_error",
		"cancel":             "turn_cancelled",
		"timeout":            "provider_timeout",
		"stream_abort":       "provider_stream_interrupted",
		"recovery":           "turn_recovery_boundary",
		"restart":            "runtime_restarted",
		"report_fallback":    "publication_receipt_required",
		"step_limit":         "turn_step_limit_exceeded",
		"tool_failure":       "tool_failure_storm",
		"approval_denied":    "approval_denied",
		"input_cancelled":    "input_cancelled",
	}
	for _, disposition := range AllDispositionsV1() {
		projection, ok := GeneralFailureProjectionV1(disposition.Reason)
		if disposition.CandidateAllowed {
			if ok {
				t.Fatalf("candidate reason received a failure projection: %s %#v", disposition.Reason, projection)
			}
			continue
		}
		if !ok || projection.Status != disposition.Status || projection.Code != expectedCodes[disposition.Reason] ||
			projection.Message == "" || projection.Severity == "" {
			t.Fatalf("general failure projection drifted for %s: %#v ok=%t", disposition.Reason, projection, ok)
		}
		record, recordOK := GeneralFailureRecordV1(disposition.Reason)
		if !recordOK || record.Code() != projection.Code || record.Message() != projection.Message ||
			record.Severity() != projection.Severity || record.Details() != nil {
			t.Fatalf("general failure record diverged for %s: %#v ok=%t", disposition.Reason, record, recordOK)
		}
	}
	if _, ok := GeneralFailureProjectionV1("invented"); ok {
		t.Fatal("unknown terminal reason received a general failure projection")
	}
}

func TestGeneralToolFailureProjectionPreservesOnlyCompatibleClosedCauseCodes(t *testing.T) {
	allowed := []string{
		domainfailure.CodeTurnFailed,
		domainfailure.CodeProviderToolArgumentsInvalid,
		"tool_call_identity_invalid",
		"tool_not_advertised",
		"tool_schema_missing",
		"tool_schema_invalid",
		"tool_private_arguments",
		"tool_invalid_arguments_storm",
		"tool_failure_storm",
	}
	for _, code := range allowed {
		t.Run(code, func(t *testing.T) {
			cause := domainfailure.New(code, map[string]any{"stormCount": 3, "unsafe": "sentinel"})
			record, ok := GeneralFailureRecordForCauseV1("tool_failure", cause)
			if !ok || record.Code() != code || record.Message() != domainfailure.New(code, nil).Message() || record.Details() != nil {
				t.Fatalf("compatible tool cause was not projected exactly: code=%q record=%#v ok=%t", code, record, ok)
			}
			projection, ok := GeneralFailureProjectionForCodeV1("tool_failure", code)
			if !ok || projection.Status != "failed" || projection.Code != code ||
				projection.Message != record.Message() || projection.Severity != record.Severity() {
				t.Fatalf("compatible tool projection mismatch: code=%q projection=%#v ok=%t", code, projection, ok)
			}
		})
	}

	legacy, ok := GeneralFailureProjectionV1("tool_failure")
	if !ok || legacy.Code != "tool_failure_storm" {
		t.Fatalf("legacy reason-only tool projection changed: %#v ok=%t", legacy, ok)
	}
	for _, code := range []string{
		domainfailure.CodeProviderError,
		"source_probe_unavailable",
		"tool_pending_continuation_invalid",
		"invented",
	} {
		if projection, ok := GeneralFailureProjectionForCodeV1("tool_failure", code); ok {
			t.Fatalf("incompatible tool code was admitted: code=%q projection=%#v", code, projection)
		}
	}
	record, ok := GeneralFailureRecordForCauseV1(
		"tool_failure", domainfailure.New(domainfailure.CodeProviderError, map[string]any{"status": 500}),
	)
	if !ok || record.Code() != domainfailure.CodeTurnFailed || record.Details() != nil {
		t.Fatalf("incompatible tool cause did not fail closed to turn_failed: %#v ok=%t", record, ok)
	}
	provider, ok := GeneralFailureRecordForCauseV1(
		"provider_failure", domainfailure.New(domainfailure.CodeProviderAuthenticationFailed, nil),
	)
	if !ok || provider.Code() != domainfailure.CodeProviderAuthenticationFailed || provider.Details() != nil {
		t.Fatalf("closed provider reason was not preserved: %#v ok=%t", provider, ok)
	}
}

func TestGeneralSemanticFailureProjectionPreservesOnlyClosedHostPhaseCodes(t *testing.T) {
	for _, code := range []string{
		"host_response_event_failed",
		"host_candidate_lifecycle_failed",
		"host_candidate_authority_failed",
		"host_candidate_steering_failed",
		"host_candidate_projection_failed",
		"host_candidate_publication_failed",
		"context_window_hard_limit",
	} {
		record, ok := GeneralFailureRecordForCauseV1("semantic_failure", domainfailure.New(code, nil))
		if !ok || record.Code() != code || record.Details() != nil {
			t.Fatalf("closed host semantic cause was not retained: code=%q record=%#v ok=%t", code, record, ok)
		}
		projection, ok := GeneralFailureProjectionForCodeV1("semantic_failure", code)
		if !ok || projection.Status != "failed" || projection.Code != code ||
			projection.Message != record.Message() || projection.Severity != record.Severity() {
			t.Fatalf("closed host semantic projection mismatch: code=%q projection=%#v ok=%t", code, projection, ok)
		}
	}

	fallback, ok := GeneralFailureRecordForCauseV1(
		"semantic_failure", domainfailure.New(domainfailure.CodeProviderError, map[string]any{"status": 500}),
	)
	if !ok || fallback.Code() != "validation_error" || fallback.Details() != nil {
		t.Fatalf("incompatible semantic cause did not fail closed: record=%#v ok=%t", fallback, ok)
	}
}

func TestGeneralProviderFailureProjectionPreservesOnlyClosedProviderCodes(t *testing.T) {
	for _, code := range []string{
		domainfailure.CodeProviderAuthenticationFailed,
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
		"provider_stream_failed",
	} {
		record, ok := GeneralFailureRecordForCauseV1(
			"provider_failure",
			domainfailure.New(code, map[string]any{"status": 503}),
		)
		if !ok || record.Code() != code || record.Details() != nil {
			t.Fatalf("closed provider cause was not retained without details: code=%q record=%#v ok=%t", code, record, ok)
		}
		projection, ok := GeneralFailureProjectionForCodeV1("provider_failure", code)
		if !ok || projection.Status != "failed" || projection.Code != code ||
			projection.Message != record.Message() || projection.Severity != record.Severity() {
			t.Fatalf("closed provider projection mismatch: code=%q projection=%#v ok=%t", code, projection, ok)
		}
	}

	fallback, ok := GeneralFailureRecordForCauseV1(
		"provider_failure", domainfailure.New(domainfailure.CodeHostCandidateAuthorityFailed, nil),
	)
	if !ok || fallback.Code() != domainfailure.CodeProviderError || fallback.Details() != nil {
		t.Fatalf("incompatible provider cause did not fail closed: record=%#v ok=%t", fallback, ok)
	}
}
