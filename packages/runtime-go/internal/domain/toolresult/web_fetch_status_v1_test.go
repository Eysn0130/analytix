package toolresult

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestWebFetchStatusV1ClosedOutcomeLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name       string
		web        WebFetchStatusV1
		status     string
		messageKey string
		code       string
		isError    bool
	}{
		{name: "partial content success", web: webFetchMeasuredFixtureV1("success", 206), status: "completed", messageKey: "tool_completed", code: "tool_completed"},
		{name: "HTTP failure", web: webFetchMeasuredFixtureV1("http_error", 503), status: "failed", messageKey: "tool_failed", code: "tool_failed", isError: true},
		{name: "read failure", web: WebFetchStatusV1{Outcome: "read_error", StatusCode: webFetchInt64V1(200), ByteCount: webFetchInt64V1(7)}, status: "failed", messageKey: "tool_failed", code: "tool_failed", isError: true},
		{name: "transport failure", web: WebFetchStatusV1{Outcome: "transport_error"}, status: "failed", messageKey: "tool_failed", code: "tool_failed", isError: true},
		{name: "invalid URL", web: WebFetchStatusV1{Outcome: "invalid_url"}, status: "failed", messageKey: "tool_failed", code: "tool_failed", isError: true},
		{name: "policy blocked redirect", web: WebFetchStatusV1{Outcome: "policy_blocked", StatusCode: webFetchInt64V1(302)}, status: "blocked", messageKey: "tool_blocked", code: "tool_blocked", isError: true},
		{name: "SSRF blocked", web: WebFetchStatusV1{Outcome: "ssrf_blocked"}, status: "blocked", messageKey: "tool_blocked", code: "tool_blocked", isError: true},
		{name: "disabled", web: WebFetchStatusV1{Outcome: "disabled"}, status: "blocked", messageKey: "tool_blocked", code: "tool_blocked", isError: true},
		{name: "timeout before response", web: WebFetchStatusV1{Outcome: "timeout"}, status: "cancelled", messageKey: "tool_cancelled", code: "tool_timeout", isError: true},
		{name: "timeout reading response", web: WebFetchStatusV1{Outcome: "timeout", StatusCode: webFetchInt64V1(200), ByteCount: webFetchInt64V1(7)}, status: "cancelled", messageKey: "tool_cancelled", code: "tool_timeout", isError: true},
		{name: "cancelled before response", web: WebFetchStatusV1{Outcome: "cancelled"}, status: "cancelled", messageKey: "tool_cancelled", code: "tool_cancelled", isError: true},
		{name: "cancelled reading response", web: WebFetchStatusV1{Outcome: "cancelled", StatusCode: webFetchInt64V1(200), ByteCount: webFetchInt64V1(7)}, status: "cancelled", messageKey: "tool_cancelled", code: "tool_cancelled", isError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projection, err := NewWebFetchStatusProjectionV1(tc.web, tc.isError)
			if err != nil || projection.ProjectionKind != ProjectionWebFetchStatus || projection.Status != tc.status ||
				projection.MessageKey != tc.messageKey || projection.Code != tc.code {
				t.Fatalf("closed Web projection mismatch: %#v err=%v", projection, err)
			}
			rootStatus, err := SettlementLifecycleStatusV1(projection, tc.isError)
			wantRootStatus := "completed"
			if tc.isError {
				wantRootStatus = "failed"
			}
			if err != nil || rootStatus != wantRootStatus {
				t.Fatalf("settlement lifecycle mismatch: status=%q err=%v", rootStatus, err)
			}
			if _, err := SettlementLifecycleStatusV1(projection, !tc.isError); !errors.Is(err, ErrToolResultLifecycleInvalidV1) {
				t.Fatalf("mismatched isError was admitted: %v", err)
			}
			if _, err := ParsePublicToolResultProjectionV1(PublicToolResultProjectionRecordV1(projection)); err != nil {
				t.Fatalf("closed projection did not round trip: %v", err)
			}
		})
	}
}

func TestWebFetchStatusV1RequiresMeasuredCompatibleFields(t *testing.T) {
	for name, mutate := range map[string]func(*WebFetchStatusV1){
		"unknown outcome":      func(web *WebFetchStatusV1) { web.Outcome = "PRIVATE_OUTCOME_SENTINEL" },
		"missing HTTP status":  func(web *WebFetchStatusV1) { web.StatusCode = nil },
		"missing byte count":   func(web *WebFetchStatusV1) { web.ByteCount = nil },
		"missing truncation":   func(web *WebFetchStatusV1) { web.Truncated = nil },
		"status below range":   func(web *WebFetchStatusV1) { web.StatusCode = webFetchInt64V1(99) },
		"status above range":   func(web *WebFetchStatusV1) { web.StatusCode = webFetchInt64V1(600) },
		"success with failure": func(web *WebFetchStatusV1) { web.StatusCode = webFetchInt64V1(500) },
		"failure with success": func(web *WebFetchStatusV1) { web.Outcome = "http_error" },
		"negative byte count":  func(web *WebFetchStatusV1) { web.ByteCount = webFetchInt64V1(-1) },
		"unsafe byte count":    func(web *WebFetchStatusV1) { web.ByteCount = webFetchInt64V1(9007199254740992) },
		"read claims complete": func(web *WebFetchStatusV1) { web.Outcome = "read_error" },
		"timeout truncation":   func(web *WebFetchStatusV1) { web.Outcome = "timeout" },
		"blocked body count":   func(web *WebFetchStatusV1) { web.Outcome = "policy_blocked"; web.Truncated = nil },
		"disabled response":    func(web *WebFetchStatusV1) { web.Outcome = "disabled"; web.ByteCount = nil; web.Truncated = nil },
	} {
		t.Run(name, func(t *testing.T) {
			web := webFetchMeasuredFixtureV1("success", 200)
			mutate(&web)
			if err := ValidateWebFetchStatusV1(web); err == nil {
				t.Fatalf("incompatible measurement was admitted: %#v", web)
			}
		})
	}
	for _, outcome := range []string{"timeout", "cancelled"} {
		if err := ValidateWebFetchStatusV1(WebFetchStatusV1{Outcome: outcome, ByteCount: webFetchInt64V1(0)}); err == nil {
			t.Fatalf("%s body count without a response was admitted", outcome)
		}
	}
	for _, outcome := range []string{"transport_error", "invalid_url", "policy_blocked", "ssrf_blocked"} {
		if err := ValidateWebFetchStatusV1(WebFetchStatusV1{Outcome: outcome, StatusCode: webFetchInt64V1(302)}); err != nil {
			t.Fatalf("actual response status was rejected for %s: %v", outcome, err)
		}
	}
	web := webFetchMeasuredFixtureV1("success", 204)
	web.ByteCount = webFetchInt64V1(0)
	if err := ValidateWebFetchStatusV1(web); err != nil {
		t.Fatalf("measured zero-byte response was rejected: %v", err)
	}
	web.ByteCount = webFetchInt64V1(9007199254740991)
	if err := ValidateWebFetchStatusV1(web); err != nil {
		t.Fatalf("maximum safe integer was rejected: %v", err)
	}
}

func TestWebFetchStatusV1PublicSchemaRejectsOpenFieldsAndTypes(t *testing.T) {
	projection, err := NewWebFetchStatusProjectionV1(webFetchMeasuredFixtureV1("success", 206), false)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any, map[string]any){
		"URL":             func(_ map[string]any, web map[string]any) { web["url"] = "PRIVATE_URL_SENTINEL" },
		"title":           func(_ map[string]any, web map[string]any) { web["title"] = "PRIVATE_TITLE_SENTINEL" },
		"body":            func(_ map[string]any, web map[string]any) { web["body"] = "PRIVATE_BODY_SENTINEL" },
		"snippet":         func(_ map[string]any, web map[string]any) { web["snippet"] = "PRIVATE_SNIPPET_SENTINEL" },
		"status string":   func(_ map[string]any, web map[string]any) { web["statusCode"] = "206" },
		"status fraction": func(_ map[string]any, web map[string]any) { web["statusCode"] = 206.5 },
		"byte fraction":   func(_ map[string]any, web map[string]any) { web["byteCount"] = 7.5 },
		"byte string":     func(_ map[string]any, web map[string]any) { web["byteCount"] = "7" },
		"truncate string": func(_ map[string]any, web map[string]any) { web["truncated"] = "false" },
		"null outcome":    func(_ map[string]any, web map[string]any) { web["outcome"] = nil },
		"null status":     func(_ map[string]any, web map[string]any) { web["statusCode"] = nil },
		"null count":      func(_ map[string]any, web map[string]any) { web["byteCount"] = nil },
		"null truncated":  func(_ map[string]any, web map[string]any) { web["truncated"] = nil },
		"null web":        func(record map[string]any, _ map[string]any) { record["web"] = nil },
		"root body":       func(record map[string]any, _ map[string]any) { record["body"] = "PRIVATE_BODY_SENTINEL" },
		"wrong kind":      func(record map[string]any, _ map[string]any) { record["projectionKind"] = "host_status" },
		"wrong status":    func(record map[string]any, _ map[string]any) { record["status"] = "failed" },
		"wrong message":   func(record map[string]any, _ map[string]any) { record["messageKey"] = "tool_failed" },
		"wrong code":      func(record map[string]any, _ map[string]any) { record["code"] = "tool_failed" },
		"evidence claim":  func(record map[string]any, _ map[string]any) { record["evidenceAuthority"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			record := PublicToolResultProjectionRecordV1(projection)
			mutate(record, record["web"].(map[string]any))
			if _, err := ParsePublicToolResultProjectionV1(record); err == nil {
				t.Fatalf("open or incompatible Web metadata was admitted: %#v", record)
			}
		})
	}
	for _, kind := range []PublicProjectionKind{
		ProjectionWithheld, ProjectionHostStatus, ProjectionPlanStatus,
		ProjectionArtifactStatus, ProjectionCaseSourceStatus, ProjectionMCPDiagnostic,
	} {
		changed := projection
		changed.ProjectionKind = kind
		if err := ValidatePublicToolResultProjectionV1(changed); err == nil {
			t.Fatalf("%s accepted an incompatible Web payload", kind)
		}
	}
}

func TestWebFetchStatusV1ItemRequiresExactToolNameKindAndLifecycle(t *testing.T) {
	projection, err := NewWebFetchStatusProjectionV1(webFetchMeasuredFixtureV1("success", 206), false)
	if err != nil {
		t.Fatal(err)
	}
	entropy := sha256.Sum256([]byte("analytix.web-fetch-public-item-test/v1"))
	callID, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	for _, toolName := range []string{"web_fetch", "web_search", "fetch_url", " web_fetch", "web_fetch "} {
		item := map[string]any{
			"turnId": "turn-web", "threadId": "thread-web", "callId": callID,
			"status": "completed", "toolName": toolName, "toolKind": "tool_call", "isError": false,
			"output": PublicToolResultProjectionRecordV1(projection), "body": "PRIVATE_BODY_SENTINEL",
		}
		projected := PublicToolResultItemRecordV1(item)
		output := projected["output"].(map[string]any)
		wantKind := string(ProjectionWithheld)
		if toolName == "web_fetch" {
			wantKind = string(ProjectionWebFetchStatus)
		}
		if output["projectionKind"] != wantKind {
			t.Fatalf("tool name %q projection mismatch: %#v", toolName, output)
		}
		encoded, err := json.Marshal(projected)
		if err != nil || strings.Contains(string(encoded), "PRIVATE_BODY_SENTINEL") {
			t.Fatalf("public item retained private root payload: %s err=%v", encoded, err)
		}
	}
	for _, toolKind := range []string{"command_execution", "file_change", "subagent", "skill", "", " tool_call", "tool_call "} {
		item := map[string]any{
			"turnId": "turn-web", "threadId": "thread-web", "callId": callID,
			"status": "completed", "toolName": "web_fetch", "toolKind": toolKind, "isError": false,
			"output": PublicToolResultProjectionRecordV1(projection),
		}
		output := PublicToolResultItemRecordV1(item)["output"].(map[string]any)
		if output["projectionKind"] != string(ProjectionWithheld) {
			t.Fatalf("tool kind %q retained Web metadata: %#v", toolKind, output)
		}
	}
	for _, mutation := range []map[string]any{{"status": "failed"}, {"isError": true}} {
		item := map[string]any{
			"turnId": "turn-web", "threadId": "thread-web", "callId": callID,
			"status": "completed", "toolName": "web_fetch", "toolKind": "tool_call", "isError": false,
			"output": PublicToolResultProjectionRecordV1(projection),
		}
		for key, value := range mutation {
			item[key] = value
		}
		output := PublicToolResultItemRecordV1(item)["output"].(map[string]any)
		if output["projectionKind"] != string(ProjectionWithheld) {
			t.Fatalf("mismatched item lifecycle retained Web metadata: %#v", output)
		}
	}
}

func webFetchMeasuredFixtureV1(outcome string, status int64) WebFetchStatusV1 {
	truncated := false
	return WebFetchStatusV1{Outcome: outcome, StatusCode: webFetchInt64V1(status), ByteCount: webFetchInt64V1(7), Truncated: &truncated}
}

func webFetchInt64V1(value int64) *int64 {
	return &value
}
