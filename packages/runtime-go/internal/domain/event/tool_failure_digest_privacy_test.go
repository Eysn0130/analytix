package event

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainprivacy "analytix.local/runtime-go/internal/domain/privacyprojection"
)

const numericToolFailureDigestV1 = "a2395506f617489269663216b2cfee4cadd88c257073fde828aaa1207f963292"

func numericToolFailureRecordV1(kind string) map[string]any {
	details := map[string]any{
		"rejectedToolNormalizedNameSha256": numericToolFailureDigestV1,
		"rejectedToolCategory":             "unknown_provider_name", "promptRoute": "tool_agent",
		"loopStep": float64(0), "advertisedToolCount": float64(21),
		"advertisedToolManifestHash":             numericToolFailureDigestV1,
		"advertisedNameSetSortedHash":            numericToolFailureDigestV1,
		"providerRequestToolManifestHash":        numericToolFailureDigestV1,
		"runToolStepManifestHash":                numericToolFailureDigestV1,
		"providerRequestRunToolStepManifestSame": true,
	}
	failure := domainfailure.New("tool_not_advertised", details)
	record := map[string]any{"kind": kind, "threadId": "thread_fixture", "turnId": "turn_fixture", "status": "failed",
		"code": failure.Code(), "message": failure.Message(), "severity": failure.Severity(), "details": details}
	if kind == "error" {
		record["id"], record["role"] = "item_turn_fixture_error", "system"
		record["createdAt"], record["finishedAt"] = "2026-09-15T00:00:00Z", "2026-09-15T00:00:00Z"
	} else {
		record["itemId"], record["timestamp"], record["terminalReason"] = "item_turn_fixture_error", "2026-09-15T00:00:00Z", "tool_failure"
		record["error"] = failure.Message()
	}
	return record
}

func TestClosedToolFailureNumericDigestsSurvivePrivacyValidationAndProjection(t *testing.T) {
	if len(numericToolFailureDigestV1) != 64 || len(domainprivacy.ProjectText(numericToolFailureDigestV1).Findings) == 0 {
		t.Fatal("fixture must be a valid SHA-256 whose numeric segment exercises the prose PII detector")
	}
	for _, kind := range []string{"error", "turn_failed"} {
		t.Run(kind, func(t *testing.T) {
			record := numericToolFailureRecordV1(kind)
			source, _ := json.Marshal(record)
			wrapped := map[string]any{"events": []any{map[string]any{"item": record}}}
			if err := ValidatePublicRecord(wrapped); err != nil {
				t.Fatalf("canonical closed diagnostic was rejected: %v", err)
			}
			projected, changed := ProjectPublicValuePreservingClosedPlanDigestV1(wrapped)
			if changed || !reflect.DeepEqual(projected, wrapped) {
				t.Fatalf("closed diagnostic digest changed: changed=%t projected=%#v", changed, projected)
			}
			after, _ := json.Marshal(record)
			if string(source) != string(after) {
				t.Fatal("privacy handling mutated the source diagnostic")
			}
		})
	}
}

func TestClosedToolFailureDigestProtectionRejectsLookalikesAndExtraRawFields(t *testing.T) {
	tests := map[string]func(map[string]any){
		"wrong kind":        func(r map[string]any) { r["kind"] = "progress" },
		"wrong code":        func(r map[string]any) { r["code"] = "provider_unavailable" },
		"forged message":    func(r map[string]any) { r["message"] = "provider raw failure" },
		"outer raw field":   func(r map[string]any) { r["raw"] = "account 6222020000000000000" },
		"details raw field": func(r map[string]any) { r["details"].(map[string]any)["raw"] = "account 6222020000000000000" },
		"extra tool name":   func(r map[string]any) { r["details"].(map[string]any)["toolName"] = "create_plan" },
		"malformed digest": func(r map[string]any) {
			r["details"].(map[string]any)["advertisedNameSetSortedHash"] = numericToolFailureDigestV1[:63]
		},
		"wrong equality": func(r map[string]any) {
			r["details"].(map[string]any)["providerRequestRunToolStepManifestSame"] = false
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			record := numericToolFailureRecordV1("error")
			mutate(record)
			if ValidatePublicRecord(record) == nil {
				t.Fatal("lookalike or extra raw field was admitted")
			}
			projected, changed := ProjectPublicValuePreservingClosedPlanDigestV1(record)
			body, _ := json.Marshal(projected)
			if !changed || strings.Contains(string(body), numericToolFailureDigestV1) {
				t.Fatal("invalid diagnostic received structural digest protection")
			}
		})
	}
}

func TestClosedToolFailureDigestProtectionDoesNotExemptOrdinaryPII(t *testing.T) {
	record := numericToolFailureRecordV1("error")
	value := map[string]any{"events": []any{record}, "message": "account 6222020000000000000", "details": map[string]any{"hash": numericToolFailureDigestV1}}
	if ValidatePublicRecord(value) == nil {
		t.Fatal("ordinary PII alongside a diagnostic was admitted")
	}
	projected, changed := ProjectPublicValuePreservingClosedPlanDigestV1(value)
	got := projected.(map[string]any)
	if !changed || got["message"] == value["message"] || reflect.DeepEqual(got["details"], value["details"]) {
		t.Fatal("ordinary content or arbitrary hash was exempted")
	}
	if !reflect.DeepEqual(got["events"], value["events"]) {
		t.Fatal("ordinary PII projection changed the canonical diagnostic")
	}
}
