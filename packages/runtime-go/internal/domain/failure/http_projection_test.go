package failure

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHTTPFailureProjectionNeverRelaysUntrustedBody(t *testing.T) {
	sentinel := "sk-secret /private/case.csv 6222021234567890 SELECT_SECRET MCP_REASONING"
	for _, test := range []struct {
		status int
		body   map[string]any
		code   string
	}{
		{status: 400, body: map[string]any{"code": "attacker_code", "message": sentinel, "details": sentinel}, code: "validation_error"},
		{status: 404, body: map[string]any{"code": "not_found", "message": sentinel, "id": sentinel}, code: CodeNotFound},
		{status: 401, body: map[string]any{"code": "attachment_upload_unavailable", "message": sentinel}, code: CodeUnauthorized},
		{status: 404, body: map[string]any{"code": "case_history_restricted", "message": sentinel}, code: CodeNotFound},
		{status: 409, body: map[string]any{"code": sentinel, "message": sentinel}, code: CodeConflict},
		{status: 500, body: map[string]any{"code": "internal_error", "message": sentinel, "error": sentinel}, code: CodeInternalError},
	} {
		projected := ProjectHTTPFailure(test.status, test.body)
		encoded, _ := json.Marshal(projected)
		if projected["code"] != test.code || strings.Contains(string(encoded), sentinel) {
			t.Fatalf("unsafe HTTP failure projection: status=%d projected=%s", test.status, encoded)
		}
		if test.status == 401 && projected["message"] != "Runtime authentication is required." {
			t.Fatalf("unauthorized response contract drifted: %#v", projected)
		}
	}
}

func TestHTTPFailureProjectionKeepsClosedTurnReason(t *testing.T) {
	projected := ProjectHTTPFailure(500, map[string]any{
		"code": "turn_failed", "reasonCode": CodeProviderEndpointNotFound, "message": "PRIVATE_PROVIDER_BODY",
	})
	if projected["code"] != CodeTurnFailed || projected["reasonCode"] != CodeProviderEndpointNotFound ||
		projected["message"] != New(CodeProviderEndpointNotFound, nil).Message() {
		t.Fatalf("closed turn failure projection mismatch: %#v", projected)
	}
}

func TestHTTPFailureProjectionKeepsOnlyExactToolNotAdvertisedDiagnostics(t *testing.T) {
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
	projected := ProjectHTTPFailure(500, map[string]any{
		"code": CodeTurnFailed, "reasonCode": "tool_not_advertised", "details": details,
	})
	projectedDetails, ok := projected["details"].(map[string]any)
	if !ok || !ValidateToolNotAdvertisedDetails(projectedDetails) {
		t.Fatalf("closed unadvertised-tool diagnostics were lost: %#v", projected)
	}
	details["toolName"] = "read_file"
	if rejected := ProjectHTTPFailure(500, map[string]any{
		"code": CodeTurnFailed, "reasonCode": "tool_not_advertised", "details": details,
	}); rejected["details"] != nil {
		t.Fatalf("open unadvertised-tool diagnostics crossed HTTP: %#v", rejected)
	}
}

func TestHTTPFailureProjectionKeepsClosedTurnAdmissionCodes(t *testing.T) {
	for _, test := range []struct {
		status int
		code   string
	}{
		{status: 409, code: "turn_execution_conflict"},
		{status: 409, code: "worktree_isolation_authority_required"},
		{status: 503, code: "runtime_shutting_down"},
		{status: 503, code: "accepted_final_hydration_unavailable"},
	} {
		projected := ProjectHTTPFailure(test.status, map[string]any{"code": test.code, "message": "UNTRUSTED"})
		if projected["code"] != test.code || strings.Contains(projected["message"].(string), "UNTRUSTED") {
			t.Fatalf("closed admission projection mismatch: %#v", projected)
		}
	}
}

func TestHTTPFailureProjectionKeepsClosedNewTurnRequiredBoundary(t *testing.T) {
	sentinel := "6222021234567890 /private/case.csv MCP_REASONING"
	for _, blocker := range []string{
		"case_risk_raise",
		"context_changing_input",
		"turn_security_context_invalid",
		"turn_security_workspace_mismatch",
		"turn_security_case_binding_mismatch",
		"turn_security_dataset_snapshot_mismatch",
		"turn_security_risk_policy_mismatch",
	} {
		projected := ProjectHTTPFailure(409, map[string]any{
			"code": CodeNewTurnRequired, "blockerCode": blocker, "message": sentinel,
			"threadId": sentinel, "turnId": sentinel, "extra": sentinel,
		})
		encoded, _ := json.Marshal(projected)
		if projected["code"] != CodeNewTurnRequired || projected["blockerCode"] != blocker ||
			projected["message"] != "This input requires a newly admitted turn under current host security authority." ||
			strings.Contains(string(encoded), sentinel) {
			t.Fatalf("closed new-turn boundary mismatch: blocker=%q projected=%s", blocker, encoded)
		}
		if _, ok := projected["threadId"]; ok {
			t.Fatalf("new-turn boundary reflected an untrusted identifier: %#v", projected)
		}
	}
}

func TestHTTPFailureProjectionRejectsMalformedNewTurnRequiredBoundary(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		blocker string
		code    string
	}{
		{name: "wrong status", status: 400, blocker: "case_risk_raise", code: "validation_error"},
		{name: "missing blocker", status: 409, code: CodeConflict},
		{name: "unknown blocker", status: 409, blocker: "attacker_selected", code: CodeConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			projected := ProjectHTTPFailure(test.status, map[string]any{
				"code": CodeNewTurnRequired, "blockerCode": test.blocker,
			})
			if projected["code"] != test.code {
				t.Fatalf("malformed new-turn boundary retained authority: %#v", projected)
			}
			if _, ok := projected["blockerCode"]; ok {
				t.Fatalf("malformed new-turn boundary retained blocker: %#v", projected)
			}
		})
	}
}
