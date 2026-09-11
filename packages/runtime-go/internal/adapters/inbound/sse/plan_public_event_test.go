package sse

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appthread "analytix.local/runtime-go/internal/app/thread"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func TestCreatePlanResultReplaysThroughThePublicSSEProjection(t *testing.T) {
	const (
		threadID     = "thread-plan-sse"
		turnID       = "turn-plan-sse"
		relativePath = ".analytixsdd/plan/auth.md"
	)
	callID := toolidentitytest.MustHostToolCallIDV1("create-plan-public-sse")
	itemID := domaintoolresult.ToolResultItemIDV1(turnID, callID)
	contextDigest := domainsecurity.SHA256Hex([]byte("private-plan-context"))
	executionGrantID := domainsecurity.SHA256Hex([]byte("private-plan-grant"))
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion:          domaintoolresult.PublicProjectionSchemaVersion,
		ProjectionKind:         domaintoolresult.ProjectionPlanStatus,
		Disclosure:             domaintoolresult.MetadataOnlyDisclosure,
		MessageKey:             "plan_updated",
		Status:                 "completed",
		Code:                   "plan_updated",
		PrivatePayloadWithheld: true,
		FactAnswerAllowed:      false,
		EvidenceAuthority:      false,
		Plan: &domaintoolresult.PlanStatusV1{
			PlanID:       "plan-auth",
			RelativePath: relativePath,
			Operation:    "draft",
			ContentHash:  domainsecurity.SHA256Hex([]byte("plan body")),
			ByteSize:     128,
			SavedAt:      "2026-08-01T00:00:01Z",
		},
	}
	item := map[string]any{
		"id": itemID, "turnId": turnID, "threadId": threadID, "role": "tool",
		"status": "completed", "createdAt": "2026-08-01T00:00:00Z", "finishedAt": "2026-08-01T00:00:01Z",
		"kind": "tool_result", "toolName": "create_plan", "callId": callID, "toolKind": "file_change", "isError": false,
		"contextDigest": contextDigest, "contextEpoch": uint64(3), "executionGrantId": executionGrantID,
		"output": domaintoolresult.PublicToolResultProjectionRecordV1(projection),
	}
	event := map[string]any{
		"kind": "tool_call_finished", "seq": float64(47), "timestamp": "2026-08-01T00:00:01Z",
		"threadId": threadID, "turnId": turnID, "itemId": itemID, "item": item,
		"contextDigest": contextDigest, "contextEpoch": uint64(3), "executionGrantId": executionGrantID,
	}
	thread := map[string]any{
		"id": threadID,
		"turns": []any{map[string]any{
			"id": turnID, "threadId": threadID, "items": []any{item},
		}},
	}

	handler := ThreadEventsHandler{Store: ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 47, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) {
			return []map[string]any{event}, nil
		},
		PreflightPublic: func(string) string { return "" },
		ProjectPublic: func(routeThreadID string, raw map[string]any) (map[string]any, bool, string) {
			projected, visible, err := appthread.ProjectPublicThreadEvent(routeThreadID, thread, raw)
			if err != nil {
				return nil, false, casePublicAuthorityRejectedCode
			}
			return projected, visible, ""
		},
	}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=46", nil)
	handler.ServeHTTP(recorder, request, threadID)
	body := recorder.Body.String()

	for _, expected := range []string{
		"event: tool_call_finished",
		`"toolName":"create_plan"`,
		`"projectionKind":"plan_status"`,
		`"relativePath":"` + relativePath + `"`,
		`"privatePayloadWithheld":true`,
		`"factAnswerAllowed":false`,
		`"evidenceAuthority":false`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("public create_plan SSE is missing %q: %s", expected, body)
		}
	}
	for _, forbidden := range []string{contextDigest, executionGrantID, "contextDigest", "contextEpoch", "executionGrantId"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("public create_plan SSE leaked private authority %q: %s", forbidden, body)
		}
	}
}
