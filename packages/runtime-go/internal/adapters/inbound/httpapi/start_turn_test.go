package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	controlapp "analytix.local/runtime-go/internal/app/control"
	loopapp "analytix.local/runtime-go/internal/app/loop"
)

func TestStartTurnHandlersDecodesAndSendsThroughControl(t *testing.T) {
	stub := &startTurnControlStub{response: map[string]any{"turnId": "turn_1"}}
	handler := StartTurnHandlers{Control: stub}
	body := map[string]any{
		"prompt":                "  hello  ",
		"displayText":           "Hello",
		"riskIntent":            "case",
		"async":                 true,
		"model":                 " gpt-4.1 ",
		"providerId":            " openai ",
		"endpointFormat":        " responses ",
		"reasoningEffort":       "high",
		"mode":                  " plan ",
		"approvalPolicy":        " on-request ",
		"sandboxMode":           " workspace-write ",
		"attachmentIds":         []string{" att_1 ", "att_2"},
		"workspaceCheckpointId": " chk_1 ",
		"disableUserInput":      true,
		"maxModelSteps":         2,
		"guiPlan": map[string]any{
			"operation":     "draft",
			"workspaceRoot": "/tmp",
			"relativePath":  ".analytixsdd/plan/plan-1.md",
			"planId":        "plan_1",
		},
		"fileReferences": []map[string]any{
			{"path": " /tmp/a.go ", "relativePath": " a.go ", "name": " a.go ", "kind": " file "},
		},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/threads/thr_1/turns", jsonBody(t, body))

	handler.HandleThreadTurns(recorder, request, "thr_1")

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected accepted, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.request.ThreadID != "thr_1" ||
		stub.request.Prompt != "  hello  " ||
		stub.request.RiskIntent != "case" ||
		!stub.request.Async ||
		stub.request.Model != "gpt-4.1" ||
		stub.request.ProviderID != "openai" ||
		stub.request.EndpointFormat != "responses" ||
		stub.request.ReasoningEffort != "high" ||
		stub.request.Mode != "plan" ||
		stub.request.ApprovalPolicy != "on-request" ||
		stub.request.SandboxMode != "workspace-write" ||
		stub.request.WorkspaceCheckpointID != "chk_1" ||
		!stub.request.DisableUserInput ||
		!stub.request.DisableUserInputSet {
		t.Fatalf("unexpected control request: %+v", stub.request)
	}
	if stub.request.MaxModelSteps == nil || *stub.request.MaxModelSteps != 2 {
		t.Fatalf("expected max model steps to round trip, got %+v", stub.request.MaxModelSteps)
	}
	if len(stub.request.AttachmentIDs) != 2 || stub.request.AttachmentIDs[0] != "att_1" || stub.request.AttachmentIDs[1] != "att_2" {
		t.Fatalf("unexpected attachment ids: %#v", stub.request.AttachmentIDs)
	}
	if len(stub.request.FileReferences) != 1 {
		t.Fatalf("expected one normalized file reference, got %#v", stub.request.FileReferences)
	}
	ref, _ := stub.request.FileReferences[0].(map[string]any)
	if ref["path"] != "/tmp/a.go" || ref["relativePath"] != "a.go" || ref["name"] != "a.go" || ref["kind"] != "file" {
		t.Fatalf("unexpected file reference: %#v", ref)
	}
}

func TestStartTurnDefaultsToSyncUnlessExplicitlyEnabled(t *testing.T) {
	syncRequest, err := DecodeStartTurnRequest("thr_1", []byte(`{"prompt":"hello"}`))
	if err != nil {
		t.Fatalf("decode sync request: %v", err)
	}
	if syncRequest.Async {
		t.Fatalf("expected omitted async to default false: %+v", syncRequest)
	}

	asyncRequest, err := DecodeStartTurnRequest("thr_1", []byte(`{"prompt":"hello","async":true}`))
	if err != nil {
		t.Fatalf("decode async request: %v", err)
	}
	if !asyncRequest.Async {
		t.Fatalf("expected explicit async=true to be preserved: %+v", asyncRequest)
	}
}

func TestStrictStartRequestRejectsUnknownDuplicateAndWrongType(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unknown field", `{"prompt":"hello","publicationPolicy":"case_evidence_gate"}`},
		{"internal tool scope", `{"prompt":"hello","internalToolScope":["read"]}`},
		{"internal turn id", `{"prompt":"hello","internalTurnId":"turn_1"}`},
		{"internal auto continue job", `{"prompt":"hello","internalAutoContinueJobId":"job-1"}`},
		{"internal auto continue parent", `{"prompt":"hello","internalAutoContinueParentTurnId":"turn-parent"}`},
		{"internal usage source", `{"prompt":"hello","internalUsageSource":"turn"}`},
		{"internal system prompt", `{"prompt":"hello","internalSystemPrompt":"private"}`},
		{"internal child run", `{"prompt":"hello","internalChildRunId":"job-1"}`},
		{"internal output budget", `{"prompt":"hello","internalOutputTokenBudget":1}`},
		{"caller case id", `{"prompt":"hello","caseId":"case-spoof"}`},
		{"duplicate field", `{"prompt":"hello","prompt":"override"}`},
		{"nested duplicate field", `{"prompt":"hello","fileReferences":[{"path":"/tmp/a","path":"/tmp/b","relativePath":"a","name":"a"}]}`},
		{"trailing JSON", `{"prompt":"hello"}{"prompt":"again"}`},
		{"wrong prompt type", `{"prompt":7}`},
		{"wrong async type", `{"prompt":"hello","async":"true"}`},
		{"wrong attachments type", `{"prompt":"hello","attachmentIds":"att_1"}`},
		{"null attachment item", `{"prompt":"hello","attachmentIds":[null]}`},
		{"fractional steps", `{"prompt":"hello","maxModelSteps":1.5}`},
		{"negative steps", `{"prompt":"hello","maxModelSteps":-1}`},
		{"steps above limit", `{"prompt":"hello","maxModelSteps":10001}`},
		{"steps integer overflow", `{"prompt":"hello","maxModelSteps":9223372036854775808}`},
		{"unknown nested GUI field", `{"prompt":"hello","guiPlan":{"operation":"draft","workspaceRoot":"/tmp","relativePath":".analytixsdd/plan/a.md","planId":"a","supportStatus":"verified"}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stub := &startTurnControlStub{response: map[string]any{"turnId": "must_not_run"}}
			handler := StartTurnHandlers{Control: stub}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/threads/thr_1/turns", bytes.NewBufferString(tc.body))

			handler.HandleThreadTurns(recorder, request, "thr_1")

			if recorder.Code != http.StatusBadRequest || stub.request.ThreadID != "" {
				t.Fatalf("expected closed-schema rejection without dispatch, status=%d request=%+v body=%s", recorder.Code, stub.request, recorder.Body.String())
			}
		})
	}
}

func TestStartTurnRiskIntentIsRaiseOnly(t *testing.T) {
	accepted, err := DecodeStartTurnRequest("thr_1", []byte(`{"prompt":"inspect case evidence","riskIntent":"case"}`))
	if err != nil || accepted.RiskIntent != "case" {
		t.Fatalf("expected case risk intent to round trip, request=%+v err=%v", accepted, err)
	}
	for _, body := range []string{
		`{"prompt":"hello","riskIntent":"general"}`,
		`{"prompt":"hello","riskIntent":"unknown"}`,
		`{"prompt":"hello","riskIntent":false}`,
		`{"prompt":"hello","riskIntent":null}`,
		`{"prompt":"hello","riskIntent":" case "}`,
	} {
		if _, err := DecodeStartTurnRequest("thr_1", []byte(body)); err == nil {
			t.Fatalf("expected risk downgrade/invalid intent rejection for %s", body)
		}
	}
}

func TestStrictStartTurnRequestLegalRoundTrip(t *testing.T) {
	request, err := DecodeStartTurnRequest(" thr_1 ", []byte(`{
		"prompt":"  inspect evidence  ",
		"displayText":"Inspect evidence",
		"riskIntent":"case",
		"async":true,
		"model":" model-1 ",
		"providerId":" provider-1 ",
		"endpointFormat":" responses ",
		"reasoningEffort":"high",
		"mode":" plan ",
		"approvalPolicy":" on-request ",
		"sandboxMode":" workspace-write ",
		"attachmentIds":[" att-1 "],
		"fileReferences":[{"path":" /tmp/a.md ","relativePath":" a.md ","name":" a.md ","kind":" file "}],
		"guiPlan":{"operation":"draft","workspaceRoot":"/tmp","relativePath":".analytixsdd/plan/a.md","planId":"plan-a","sourceRequest":"source","title":"title"},
		"workspaceCheckpointId":" checkpoint-1 ",
		"disableUserInput":false,
		"maxModelSteps":1e2
	}`))
	if err != nil {
		t.Fatalf("decode legal request: %v", err)
	}
	if request.ThreadID != "thr_1" || request.RiskIntent != "case" || !request.Async ||
		request.Model != "model-1" || request.ProviderID != "provider-1" || request.EndpointFormat != "responses" ||
		request.ReasoningEffort != "high" || request.Mode != "plan" || request.ApprovalPolicy != "on-request" ||
		request.SandboxMode != "workspace-write" || !request.DisableUserInputSet || request.DisableUserInput ||
		request.MaxModelSteps == nil || *request.MaxModelSteps != 100 {
		t.Fatalf("unexpected round-trip request: %+v", request)
	}
}

func TestStrictStartTurnRequestPreservesAutoAndRejectsReasoningAliases(t *testing.T) {
	request, err := DecodeStartTurnRequest("thread-1", []byte(`{"prompt":"hello","reasoningEffort":"auto"}`))
	if err != nil || request.ReasoningEffort != "auto" {
		t.Fatalf("auto reasoning effort did not round-trip: request=%#v err=%v", request, err)
	}
	for _, effort := range []string{" high ", "HIGH", "SOL_PRIVATE_REASONING_SENTINEL_7F3C"} {
		body, err := json.Marshal(map[string]any{"prompt": "hello", "reasoningEffort": effort})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeStartTurnRequest("thread-1", body); err == nil || strings.Contains(err.Error(), effort) {
			t.Fatalf("invalid reasoning effort was accepted or reflected: effort=%q err=%v", effort, err)
		}
	}
}

func TestStartTurnHandlersMapsControlErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"missing prompt", controlapp.ErrMissingPrompt, http.StatusBadRequest, "validation_error"},
		{"thread missing", controlapp.ErrThreadNotFound, http.StatusNotFound, "not_found"},
		{"attachment forbidden", controlapp.ErrAttachmentNotAuthorized, http.StatusForbidden, "forbidden"},
		{"turn terminal active", controlapp.ErrTurnExecutionConflict, http.StatusConflict, "turn_execution_conflict"},
		{"runtime shutdown", controlapp.ErrRuntimeShuttingDown, http.StatusServiceUnavailable, "runtime_shutting_down"},
		{"driver failed", errors.New("driver failed"), http.StatusInternalServerError, "turn_failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := StartTurnHandlers{Control: &startTurnControlStub{err: tc.err}}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/threads/thr_1/turns", jsonBody(t, map[string]any{"prompt": "hi"}))

			handler.HandleThreadTurns(recorder, request, "thr_1")

			if recorder.Code != tc.status {
				t.Fatalf("expected status %d, got %d body=%s", tc.status, recorder.Code, recorder.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != tc.code {
				t.Fatalf("expected code %q, got %#v", tc.code, body)
			}
			if tc.code == "turn_failed" {
				if body["reasonCode"] != "turn_failed" || body["message"] != "The turn failed before a verified response was available." || bytes.Contains(recorder.Body.Bytes(), []byte("driver failed")) {
					t.Fatalf("runtime failure must use the closed host projection: %#v", body)
				}
			}
		})
	}
}

func TestStartTurnFailureDropsNonToolDiagnosticDetails(t *testing.T) {
	handler := StartTurnHandlers{Control: &startTurnControlStub{err: loopapp.StepLimitExceededError(5)}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/threads/thr_1/turns", jsonBody(t, map[string]any{"prompt": "hi"}))

	handler.HandleThreadTurns(recorder, request, "thr_1")

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if recorder.Code != http.StatusInternalServerError || body["reasonCode"] != "turn_step_limit_exceeded" {
		t.Fatalf("unexpected failure response: status=%d body=%#v", recorder.Code, body)
	}
	if _, present := body["details"]; present {
		t.Fatalf("non-tool diagnostic details crossed start-turn boundary: %#v", body)
	}
}

type startTurnControlStub struct {
	request  controlapp.StartTurnRequest
	response map[string]any
	err      error
}

func (s *startTurnControlStub) SendTurn(_ context.Context, request controlapp.StartTurnRequest) (map[string]any, error) {
	s.request = request
	return s.response, s.err
}

func jsonBody(t *testing.T, body any) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return bytes.NewReader(data)
}
