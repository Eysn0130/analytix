package failure

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestRecordProjectsOnlyFixedMessageAndBoundedTypedDetails(t *testing.T) {
	record := New(CodeProviderAuthenticationFailed, map[string]any{
		"providerId": "deepseek", "family": "openai", "endpointFormat": "chat_completions",
		"status": float64(401), "kind": "auth", "hasApiKey": true, "authStatus": "required", "retryable": false,
		"message": "PRIVATE_REASONING_SENTINEL 6222021234567890", "requestUrl": "https://secret.invalid/account/6222021234567890",
		"nested": map[string]any{"reasoning_content": "PRIVATE_REASONING_SENTINEL"},
	})
	if record.Code() != CodeProviderAuthenticationFailed || record.Message() != "Provider authentication failed. Check the configured credential." {
		t.Fatalf("public failure mismatch: code=%q message=%q", record.Code(), record.Message())
	}
	body, _ := json.Marshal(record.Details())
	serialized := string(body)
	for _, forbidden := range []string{"PRIVATE_REASONING_SENTINEL", "6222021234567890", "requestUrl", "message", "nested"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("public failure details leaked %q: %s", forbidden, serialized)
		}
	}
	if !strings.Contains(serialized, `"status":401`) || !strings.Contains(serialized, `"authStatus":"required"`) {
		t.Fatalf("bounded diagnostics were lost: %s", serialized)
	}
}

func TestRecordRejectsArbitraryCodesIdentifiersAndSeverity(t *testing.T) {
	record := New("PRIVATE_REASONING_SENTINEL", map[string]any{
		"providerId": "6222021234567890", "kind": "provider invented kind", "status": float64(9999),
	})
	if record.Code() != CodeTurnFailed || record.Message() != "The turn failed before a verified response was available." || record.Severity() != "error" {
		t.Fatalf("arbitrary failure was not downgraded: %#v", record)
	}
	if details := record.Details(); details != nil {
		t.Fatalf("arbitrary details escaped: %#v", details)
	}
}

func TestRecordDetailsReturnsDefensiveCopy(t *testing.T) {
	record := New(CodeProviderRateLimited, map[string]any{"retryAfterMs": float64(1200)})
	details := record.Details()
	details["retryAfterMs"] = float64(1)
	if record.Details()["retryAfterMs"] != float64(1200) {
		t.Fatalf("record details were mutated: %#v", record.Details())
	}
}

func TestValidatePublicDetailsRequiresExactClosedProjection(t *testing.T) {
	valid := New(CodeProviderRateLimited, map[string]any{
		"retryAfterMs": 1200, "attempt": 1, "endpointFormat": "messages", "retryable": true,
		"failureStage": "transport_after_observed_send", "dispatchState": "sent",
	}).Details()
	if !ValidatePublicDetails(valid) {
		t.Fatalf("exact public details were rejected: %#v", valid)
	}
	for name, candidate := range map[string]map[string]any{
		"empty":                    {},
		"unknown":                  {"retryAfterMs": 1200, "untrusted": true},
		"fractional":               {"attempt": 1.5},
		"legacy endpoint spelling": {"endpointFormat": "anthropic_messages"},
		"trimmed":                  {"endpointFormat": " messages "},
		"negative zero":            {"attempt": math.Copysign(0, -1)},
		"unknown failure stage":    {"failureStage": "provider_text"},
		"unknown dispatch state":   {"dispatchState": "maybe_sent"},
		"non-canonical stage":      {"failureStage": " transport_after_observed_send"},
	} {
		t.Run(name, func(t *testing.T) {
			if ValidatePublicDetails(candidate) {
				t.Fatalf("open or non-canonical details were accepted: %#v", candidate)
			}
		})
	}
}

func TestToolNotAdvertisedDiagnosticsRequireExactClosedShape(t *testing.T) {
	valid := map[string]any{
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
	if !ValidatePublicDetails(valid) || !ValidateToolNotAdvertisedDetails(valid) {
		t.Fatalf("closed tool rejection diagnostics were rejected: %#v", valid)
	}
	for name, mutate := range map[string]func(map[string]any){
		"raw name":         func(details map[string]any) { details["toolName"] = "read_file" },
		"missing hash":     func(details map[string]any) { delete(details, "advertisedToolManifestHash") },
		"invalid category": func(details map[string]any) { details["rejectedToolCategory"] = "provider_text" },
		"invalid route":    func(details map[string]any) { details["promptRoute"] = "private_route" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := map[string]any{}
			for key, value := range valid {
				candidate[key] = value
			}
			mutate(candidate)
			if ValidateToolNotAdvertisedDetails(candidate) {
				t.Fatalf("open tool rejection diagnostics were accepted: %#v", candidate)
			}
		})
	}
}

func TestErrorCarriesOnlyClosedPublicFailure(t *testing.T) {
	err := NewError(CodeProviderToolArgumentsInvalid, map[string]any{
		"message": "SOL_PRIVATE_TRACE_7C 6222021234567890",
	})
	public, ok := err.(interface{ PublicFailureRecord() Record })
	if !ok {
		t.Fatalf("closed public error contract missing: %T", err)
	}
	record := public.PublicFailureRecord()
	if record.Code() != CodeProviderToolArgumentsInvalid ||
		err.Error() != "The provider supplied incomplete or invalid tool arguments; execution was blocked." ||
		record.Details() != nil || strings.Contains(err.Error(), "SOL_PRIVATE_TRACE_7C") {
		t.Fatalf("closed public error leaked caller text: err=%q record=%#v", err.Error(), record)
	}
}
