package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	"analytix.local/runtime-go/internal/jobs"
)

func TestTaskJobHandlersValidateRouteAndThread(t *testing.T) {
	handler := TaskJobHandlers{Service: taskJobServiceStub{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/wait", strings.NewReader(`{"jobIds":["job-1"]}`))
	handler.Handle(recorder, request)
	body := decodeTaskJobBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected missing thread response code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/unknown", strings.NewReader(`{}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected unknown route status: %d", recorder.Code)
	}
}

func TestStrictTaskJobSteerRejectsDuplicateUnknownAndHostAuthorityFields(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "duplicate", body: `{"threadId":"thr_parent","threadId":"thr_other","jobId":"job_1","message":"continue"}`},
		{name: "unknown", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","extra":"denied"}`},
		{name: "case authority", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","caseId":"forged"}`},
		{name: "security authority", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","securityContext":{}}`},
		{name: "risk authority", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","riskIntent":"general"}`},
		{name: "attachment", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","attachmentIds":["att_1"]}`},
		{name: "file reference", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","fileReferences":[{"path":"secret"}]}`},
		{name: "source provenance", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","sourceTurnId":"forged"}`},
		{name: "job aliases", body: `{"threadId":"thr_parent","jobId":"job_1","id":"job_2","message":"continue"}`},
		{name: "message aliases", body: `{"threadId":"thr_parent","jobId":"job_1","message":"continue","text":"replace"}`},
		{name: "thread aliases", body: `{"threadId":"thr_parent","parentThreadId":"thr_other","jobId":"job_1","message":"continue"}`},
		{name: "wrong type", body: `{"threadId":"thr_parent","jobId":"job_1","message":{"text":"continue"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			handler := TaskJobHandlers{Service: taskJobServiceStub{steerCalls: &calls}}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/steer", strings.NewReader(test.body))
			handler.Handle(recorder, request)
			if recorder.Code != http.StatusBadRequest || calls != 0 || strings.Contains(recorder.Body.String(), "forged") || strings.Contains(recorder.Body.String(), "secret") {
				t.Fatalf("strict steer accepted or reflected invalid authority: code=%d calls=%d body=%s", recorder.Code, calls, recorder.Body.String())
			}
		})
	}
	calls := 0
	handler := TaskJobHandlers{Service: taskJobServiceStub{steerCalls: &calls}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/steer", strings.NewReader(`{"threadId":"thr_parent","jobId":"job_1","message":"`+strings.Repeat("x", maxTaskJobSteerRequestBytes)+`"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusBadRequest || calls != 0 {
		t.Fatalf("oversized steer body reached service: code=%d calls=%d", recorder.Code, calls)
	}
	calls = 0
	handler = TaskJobHandlers{Service: taskJobServiceStub{
		steerCalls: &calls,
		steerResult: subagentapp.TaskJobServiceResult{
			IsError: true, ErrorCode: subagentapp.TaskJobErrorConflict,
			Response: map[string]any{"code": "new_turn_required", "blockerCode": "case_risk_raise", "threadId": "thr_child", "turnId": "turn_child"},
		},
	}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/steer", strings.NewReader(`{"threadId":"thr_parent","jobId":"job_1","message":"continue"}`))
	handler.Handle(recorder, request)
	response := decodeTaskJobBody(t, recorder)
	if recorder.Code != http.StatusConflict || calls != 1 || response["code"] != "new_turn_required" || response["blockerCode"] != "case_risk_raise" {
		t.Fatalf("valid strict steer lost fixed security boundary: code=%d calls=%d response=%#v", recorder.Code, calls, response)
	}
}

func TestTaskJobHandlersRebuildVersionedOutputBeforeHTTP(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	malicious := map[string]any{"output": privateSentinel, "error": privateSentinel, "reasoning": privateSentinel}
	handler := TaskJobHandlers{Service: taskJobServiceStub{outputResult: subagentapp.TaskJobServiceResult{
		Record:   domainjob.Record{ID: "job-1", Status: "completed", Output: privateSentinel, Error: privateSentinel},
		Response: malicious,
	}}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/output", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	body := decodeTaskJobBody(t, recorder)
	if recorder.Code != http.StatusOK || body["schemaVersion"] != float64(1) || body["availability"] != "withheld" || body["jobId"] != "job-1" || strings.Contains(recorder.Body.String(), privateSentinel) {
		t.Fatalf("host-rebuilt task output was not returned: code=%d body=%#v", recorder.Code, body)
	}

	handler.Service = taskJobServiceStub{outputResult: subagentapp.TaskJobServiceResult{
		Record: domainjob.Record{ID: "<think>" + privateSentinel, Status: "completed"}, Response: malicious,
	}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/output", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	body = decodeTaskJobBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["code"] != "task_job_output_schema_invalid" || strings.Contains(recorder.Body.String(), privateSentinel) {
		t.Fatalf("invalid typed task output authority crossed HTTP: code=%d body=%#v", recorder.Code, body)
	}
}

func TestTaskJobHandlersRebuildEveryMetadataSurfaceAndFailure(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	record := domainjob.Record{
		ID: "job-1", Kind: "background-shell", Status: "failed", Background: true,
		Output: privateSentinel, Error: privateSentinel, Label: privateSentinel,
		ModelExecution: map[string]any{"providerId": privateSentinel},
	}
	malicious := map[string]any{
		"output": privateSentinel, "error": privateSentinel, "reasoning_content": privateSentinel,
		"diagnostics": map[string]any{"result": privateSentinel},
	}
	handler := TaskJobHandlers{Service: taskJobServiceStub{
		listResult: subagentapp.TaskJobServiceResult{Records: []domainjob.Record{record}, Response: malicious},
		waitResult: subagentapp.TaskJobServiceResult{Records: []domainjob.Record{record}, Response: malicious},
		killResult: subagentapp.TaskJobServiceResult{Record: record, Response: malicious},
	}}
	for _, test := range []struct {
		path string
		body string
		key  string
	}{
		{path: "/v1/runtime/task-jobs/list", body: `{"threadId":"thread-1"}`, key: "jobs"},
		{path: "/v1/runtime/task-jobs/wait", body: `{"threadId":"thread-1","jobIds":["job-1"]}`, key: "jobs"},
		{path: "/v1/runtime/task-jobs/kill", body: `{"threadId":"thread-1","jobId":"job-1"}`, key: "job"},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		handler.Handle(recorder, request)
		body := decodeTaskJobBody(t, recorder)
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), privateSentinel) {
			t.Fatalf("%s leaked service response bytes: code=%d body=%s", test.path, recorder.Code, recorder.Body.String())
		}
		var metadata map[string]any
		if test.key == "job" {
			metadata, _ = body["job"].(map[string]any)
		} else if values, _ := body["jobs"].([]any); len(values) == 1 {
			metadata, _ = values[0].(map[string]any)
		}
		assertClosedTaskJobHTTPMetadataV1(t, metadata, "job-1", "failed")
	}

	handler.Service = taskJobServiceStub{
		waitResult:   subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorForbidden, Response: malicious},
		killResult:   subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorNotFound, Response: malicious},
		outputResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorValidation, Response: malicious},
	}
	for _, test := range []struct {
		path string
		body string
		code int
	}{
		{path: "/v1/runtime/task-jobs/wait", body: `{"threadId":"thread-1","jobIds":["job-1"]}`, code: http.StatusForbidden},
		{path: "/v1/runtime/task-jobs/kill", body: `{"threadId":"thread-1","jobId":"job-1"}`, code: http.StatusNotFound},
		{path: "/v1/runtime/task-jobs/output", body: `{"threadId":"thread-1","jobId":"job-1"}`, code: http.StatusBadRequest},
	} {
		recorder := httptest.NewRecorder()
		handler.Handle(recorder, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
		if recorder.Code != test.code || strings.Contains(recorder.Body.String(), privateSentinel) {
			t.Fatalf("%s leaked failure bytes: code=%d body=%s", test.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestTaskJobDurableReloadHTTPProjectionWithholdsPrivateSentinel(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	root := t.TempDir()
	manager, err := jobs.NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal-1", ParentThreadID: "thread-1", Kind: "background-shell",
		Status: "completed", Background: true, Output: "private job bytes " + privateSentinel,
	})
	if err != nil {
		t.Fatal(err)
	}
	durable, err := os.ReadFile(filepath.Join(root, record.ID+".json"))
	if err != nil || !strings.Contains(string(durable), privateSentinel) {
		t.Fatalf("fixture did not persist the private sentinel: err=%v body=%s", err, durable)
	}
	reloaded, err := jobs.NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	service := subagentapp.NewService(subagentapp.Dependencies{Jobs: reloaded})
	handler := TaskJobHandlers{Service: subagentapp.NewTaskJobHTTPService(subagentapp.TaskJobHTTPServiceDeps{
		Service: service, Jobs: reloaded,
	})}

	for _, test := range []struct {
		path string
		body string
	}{
		{path: "/v1/runtime/task-jobs/list", body: `{"threadId":"thread-1"}`},
		{path: "/v1/runtime/task-jobs/wait", body: `{"threadId":"thread-1","jobIds":["job-1"]}`},
		{path: "/v1/runtime/task-jobs/output", body: `{"threadId":"thread-1","jobId":"job-1"}`},
		{path: "/v1/runtime/task-jobs/kill", body: `{"threadId":"thread-1","jobId":"job-1"}`},
	} {
		recorder := httptest.NewRecorder()
		handler.Handle(recorder, httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body)))
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), privateSentinel) ||
			strings.Contains(recorder.Body.String(), "reasoning") || strings.Contains(recorder.Body.String(), "<think>") {
			t.Fatalf("durable reload leaked through %s: code=%d body=%s", test.path, recorder.Code, recorder.Body.String())
		}
	}
}

func assertClosedTaskJobHTTPMetadataV1(t *testing.T, metadata map[string]any, id string, status string) {
	t.Helper()
	if len(metadata) != 12 || metadata["schemaVersion"] != float64(1) || metadata["id"] != id ||
		metadata["status"] != status || metadata["outputWithheld"] != true || metadata["canReadOutput"] != false ||
		metadata["factAnswerAllowed"] != false || metadata["evidenceAuthority"] != false || metadata["canContinueParent"] != false {
		t.Fatalf("task-job metadata is not the exact closed projection: %#v", metadata)
	}
	for _, forbidden := range []string{"output", "error", "reasoning", "reasoning_content", "diagnostics", "offset", "nextOffset", "outputBytes"} {
		if _, ok := metadata[forbidden]; ok {
			t.Fatalf("task-job metadata exposed %q: %#v", forbidden, metadata)
		}
	}
}

func TestTaskJobHandlersKillFailsClosedWhenCancellationDoesNotSettle(t *testing.T) {
	const privateSentinel = "ZXQ_KILL_PRIVATE_4D1B"
	for _, test := range []struct {
		name       string
		errorCode  string
		statusCode int
		code       string
		message    string
	}{
		{name: "conflict", errorCode: subagentapp.TaskJobErrorConflict, statusCode: http.StatusConflict, code: "conflict", message: testConflictFailureMessage},
		{name: "validation", errorCode: subagentapp.TaskJobErrorValidation, statusCode: http.StatusBadRequest, code: "validation_error", message: testValidationFailureMessage},
		{name: "unknown", statusCode: http.StatusInternalServerError, code: "internal_error", message: testInternalFailureMessage},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := TaskJobHandlers{Service: taskJobServiceStub{killResult: subagentapp.TaskJobServiceResult{
				Record:   domainjob.Record{ID: "job-1", Kind: "subagent", Status: "running", Background: true},
				Response: map[string]any{"error": privateSentinel}, IsError: true, ErrorCode: test.errorCode,
			}}}
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/kill", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
			handler.Handle(recorder, request)
			body := decodeTaskJobBody(t, recorder)
			if recorder.Code != test.statusCode || body["code"] != test.code || body["message"] != test.message {
				t.Fatalf("kill failure response mismatch: code=%d body=%#v", recorder.Code, body)
			}
			if body["job"] != nil || strings.Contains(recorder.Body.String(), privateSentinel) || strings.Contains(recorder.Body.String(), `"status":"running"`) {
				t.Fatalf("kill failure exposed stale or private state: %s", recorder.Body.String())
			}
		})
	}
}

func TestTaskJobHandlersMapServiceErrors(t *testing.T) {
	handler := TaskJobHandlers{Service: taskJobServiceStub{
		outputResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorForbidden},
		killResult:   subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorNotFound},
		restartResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorValidation,
			Response:  map[string]any{"error": "subagent job is still active: job-1"},
		},
		recoverResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorConflict,
			Response:  map[string]any{"error": "delivery_id does not match task job delivery"},
		},
		waitResult:   subagentapp.TaskJobServiceResult{Response: map[string]any{"jobs": []any{}}, ErrorCode: subagentapp.TaskJobErrorForbidden},
		steerResult:  subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorConflict},
		pauseResult:  subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorConflict},
		resumeResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorConflict},
		reviewResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorValidation},
		rejectResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorForbidden},
		cleanupResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorConflict,
			Response:  map[string]any{"error": "cleanup conflict"},
		},
		acceptResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorValidation,
			Response:  map[string]any{"error": "approval_id is required"},
		},
		conflictReportResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorConflict,
			Response:  map[string]any{"error": "not conflicted"},
		},
		repairCheckResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorValidation,
			Response:  map[string]any{"error": "repair patch is required"},
		},
		repairAcceptResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorConflict,
			Response:  map[string]any{"error": "clean repair review not found"},
		},
		childTodosResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorForbidden},
		childTodosProjectResult: subagentapp.TaskJobServiceResult{
			ErrorCode: subagentapp.TaskJobErrorConflict,
			Response:  map[string]any{"error": "projection requires terminal child job"},
		},
		childTodosRejectResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorValidation, Response: map[string]any{"error": "projection not found"}},
		childTodosAcceptResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorValidation, Response: map[string]any{"error": "approval_id is required"}},
	}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/output", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden output status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/kill", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected not found kill status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/restart", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict restart status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/recover", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict recover status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/wait", strings.NewReader(`{"threadId":"thread-1","jobIds":["job-1"]}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden wait status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/steer", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1","message":"continue"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict steer status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/pause", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict pause status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/resume", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict resume status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/isolation-review", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation isolation review status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/isolation-reject", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden isolation reject status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/isolation-cleanup", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict isolation cleanup status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/isolation-accept", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1","approvalId":"approval-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation isolation accept status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/isolation-conflict-report", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict isolation conflict report status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/isolation-repair-check", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1","conflictReportId":"conflict-1","repairPatch":"patch"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation isolation repair check status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/isolation-repair-accept", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1","repairReviewId":"review-1","approvalId":"approval-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict isolation repair accept status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/child-todos", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden child todos status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/child-todos/project", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected conflict child todo project status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/child-todos/reject", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1","projectionId":"projection-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation child todo reject status, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/child-todos/accept", strings.NewReader(`{"threadId":"thread-1","jobId":"job-1","projectionId":"projection-1"}`))
	handler.Handle(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation child todo accept status, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestTaskJobHandlersSuccess(t *testing.T) {
	record := domainjob.Record{ID: "job-1", Kind: "background-shell", Status: "completed", Background: true}
	handler := TaskJobHandlers{Service: taskJobServiceStub{
		outputResult: subagentapp.TaskJobServiceResult{Record: record, Response: map[string]any{
			"schemaVersion": 1, "availability": "withheld", "jobId": "job-1", "status": "completed",
			"reasonCode": "security_bound_child_output", "outputWithheld": true, "outputTrustStatus": "untrusted_child_output",
			"factAnswerAllowed": false, "evidenceAuthority": false, "canReadOutput": false, "canContinueParent": false,
		}},
		killResult:           subagentapp.TaskJobServiceResult{Record: record, Response: map[string]any{"job": map[string]any{"id": "job-1"}}},
		restartResult:        subagentapp.TaskJobServiceResult{Response: map[string]any{"job": map[string]any{"id": "job-2"}}},
		recoverResult:        subagentapp.TaskJobServiceResult{Response: map[string]any{"result": "recovered", "jobId": "job-1"}},
		waitResult:           subagentapp.TaskJobServiceResult{Records: []domainjob.Record{record}, Response: map[string]any{"jobs": []any{map[string]any{"id": "job-1"}}}},
		listResult:           subagentapp.TaskJobServiceResult{Records: []domainjob.Record{record}, Response: map[string]any{"jobs": []any{map[string]any{"id": "job-1"}}}},
		steerResult:          subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "queued", "jobId": "job-1"}},
		pauseResult:          subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "requested", "jobId": "job-1"}},
		resumeResult:         subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "resume_requested", "jobId": "job-1"}},
		reviewResult:         subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "reviewed", "jobId": "job-1"}},
		rejectResult:         subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "rejected", "jobId": "job-1"}},
		cleanupResult:        subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "cleaned", "jobId": "job-1"}},
		acceptResult:         subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "accepted", "jobId": "job-1"}},
		conflictReportResult: subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "conflicted", "jobId": "job-1"}},
		repairCheckResult:    subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "clean", "jobId": "job-1"}},
		repairAcceptResult:   subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "repair_accepted", "jobId": "job-1"}},
		childTodosResult:     subagentapp.TaskJobServiceResult{Response: map[string]any{"childTodoCount": float64(2), "jobId": "job-1"}},
		childTodosProjectResult: subagentapp.TaskJobServiceResult{Response: map[string]any{
			"status":       "proposed",
			"projectionId": "projection-1",
			"jobId":        "job-1",
		}},
		childTodosRejectResult: subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "rejected", "projectionId": "projection-1", "jobId": "job-1"}},
		childTodosAcceptResult: subagentapp.TaskJobServiceResult{Response: map[string]any{"status": "accepted", "projectionId": "projection-1", "decisionId": "decision-1", "jobId": "job-1"}},
	}}
	for _, path := range []string{"/v1/runtime/task-jobs/output", "/v1/runtime/task-jobs/kill", "/v1/runtime/task-jobs/restart", "/v1/runtime/task-jobs/recover", "/v1/runtime/task-jobs/wait", "/v1/runtime/task-jobs/list", "/v1/runtime/task-jobs/steer", "/v1/runtime/task-jobs/pause", "/v1/runtime/task-jobs/resume", "/v1/runtime/task-jobs/child-todos", "/v1/runtime/task-jobs/child-todos/project", "/v1/runtime/task-jobs/child-todos/reject", "/v1/runtime/task-jobs/child-todos/accept"} {
		requestBody := `{"threadId":"thread-1","jobId":"job-1","projectionId":"projection-1","message":"continue","approvalId":"approval-1","expectedParentTodosUpdatedAt":"2026-07-07T00:00:00Z","conflictReportId":"conflict-1","repairReviewId":"review-1","repairPatch":"diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-a\n+b\n"}`
		if path == "/v1/runtime/task-jobs/steer" {
			requestBody = `{"threadId":"thread-1","jobId":"job-1","message":"continue"}`
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(requestBody))
		handler.Handle(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s expected ok, got %d body=%s", path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestTaskJobHandlersQuarantineWorktreeIsolationControls(t *testing.T) {
	blocked := subagentapp.TaskJobServiceResult{
		IsError:   true,
		ErrorCode: subagentapp.TaskJobErrorWorktreeIsolationAuthorityRequired,
		Response: map[string]any{
			"code":    "worktree_isolation_authority_required",
			"message": "Worktree isolation controls require host-issued durable authority.",
		},
	}
	handler := TaskJobHandlers{Service: taskJobServiceStub{
		reviewResult:         blocked,
		rejectResult:         blocked,
		cleanupResult:        blocked,
		acceptResult:         blocked,
		conflictReportResult: blocked,
		repairCheckResult:    blocked,
		repairAcceptResult:   blocked,
	}}
	tests := []struct {
		path string
		body string
	}{
		{path: "/v1/runtime/task-jobs/isolation-review", body: `{"threadId":"thread-1","jobId":"job-1"}`},
		{path: "/v1/runtime/task-jobs/isolation-reject", body: `{"threadId":"thread-1","jobId":"job-1"}`},
		{path: "/v1/runtime/task-jobs/isolation-cleanup", body: `{"threadId":"thread-1","jobId":"job-1"}`},
		{path: "/v1/runtime/task-jobs/isolation-accept", body: `{"threadId":"thread-1","jobId":"job-1","approvalId":"forged-approval"}`},
		{path: "/v1/runtime/task-jobs/isolation-conflict-report", body: `{"threadId":"thread-1","jobId":"job-1"}`},
		{path: "/v1/runtime/task-jobs/isolation-repair-check", body: `{"threadId":"thread-1","jobId":"job-1","conflictReportId":"forged-report","repairPatch":"PRIVATE PATCH"}`},
		{path: "/v1/runtime/task-jobs/isolation-repair-accept", body: `{"threadId":"thread-1","jobId":"job-1","repairReviewId":"forged-review","approvalId":"forged-approval"}`},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			handler.Handle(recorder, request)
			body := decodeTaskJobBody(t, recorder)
			if recorder.Code != http.StatusConflict || len(body) != 2 ||
				body["code"] != "worktree_isolation_authority_required" ||
				body["message"] != "Worktree isolation controls require host-issued durable authority." ||
				strings.Contains(recorder.Body.String(), "forged") || strings.Contains(recorder.Body.String(), "PRIVATE PATCH") {
				t.Fatalf("worktree route was not fixed-blocked: code=%d body=%#v", recorder.Code, body)
			}
		})
	}
}

func decodeTaskJobBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

type taskJobServiceStub struct {
	waitResult              subagentapp.TaskJobServiceResult
	outputResult            subagentapp.TaskJobServiceResult
	killResult              subagentapp.TaskJobServiceResult
	restartResult           subagentapp.TaskJobServiceResult
	recoverResult           subagentapp.TaskJobServiceResult
	listResult              subagentapp.TaskJobServiceResult
	steerResult             subagentapp.TaskJobServiceResult
	steerCalls              *int
	pauseResult             subagentapp.TaskJobServiceResult
	resumeResult            subagentapp.TaskJobServiceResult
	reviewResult            subagentapp.TaskJobServiceResult
	rejectResult            subagentapp.TaskJobServiceResult
	cleanupResult           subagentapp.TaskJobServiceResult
	acceptResult            subagentapp.TaskJobServiceResult
	conflictReportResult    subagentapp.TaskJobServiceResult
	repairCheckResult       subagentapp.TaskJobServiceResult
	repairAcceptResult      subagentapp.TaskJobServiceResult
	childTodosResult        subagentapp.TaskJobServiceResult
	childTodosProjectResult subagentapp.TaskJobServiceResult
	childTodosRejectResult  subagentapp.TaskJobServiceResult
	childTodosAcceptResult  subagentapp.TaskJobServiceResult
}

func (s taskJobServiceStub) ListTaskJobs(string, subagentapp.TaskJobListRequest) subagentapp.TaskJobServiceResult {
	return s.listResult
}

func (s taskJobServiceStub) WaitTaskJobs(string, subagentapp.TaskJobWaitRequest, bool) subagentapp.TaskJobServiceResult {
	return s.waitResult
}

func (s taskJobServiceStub) OutputTaskJob(string, subagentapp.TaskJobOutputRequest, bool) subagentapp.TaskJobServiceResult {
	return s.outputResult
}

func (s taskJobServiceStub) KillTaskJob(string, subagentapp.TaskJobKillRequest) subagentapp.TaskJobServiceResult {
	return s.killResult
}

func (s taskJobServiceStub) RestartTaskJob(string, subagentapp.TaskJobRestartRequest) subagentapp.TaskJobServiceResult {
	return s.restartResult
}

func (s taskJobServiceStub) RecoverTaskJob(string, subagentapp.TaskJobRecoverRequest) subagentapp.TaskJobServiceResult {
	return s.recoverResult
}

func (s taskJobServiceStub) SteerTaskJob(context.Context, string, subagentapp.TaskJobSteerRequest) subagentapp.TaskJobServiceResult {
	if s.steerCalls != nil {
		*s.steerCalls++
	}
	return s.steerResult
}

func (s taskJobServiceStub) PauseTaskJob(string, subagentapp.TaskJobPauseRequest) subagentapp.TaskJobServiceResult {
	return s.pauseResult
}

func (s taskJobServiceStub) ResumeTaskJob(string, subagentapp.TaskJobResumeRequest) subagentapp.TaskJobServiceResult {
	return s.resumeResult
}

func (s taskJobServiceStub) ReviewTaskJobIsolation(string, subagentapp.TaskJobIsolationReviewRequest) subagentapp.TaskJobServiceResult {
	return s.reviewResult
}

func (s taskJobServiceStub) RejectTaskJobIsolation(string, subagentapp.TaskJobIsolationRejectRequest) subagentapp.TaskJobServiceResult {
	return s.rejectResult
}

func (s taskJobServiceStub) CleanupTaskJobIsolation(string, subagentapp.TaskJobIsolationCleanupRequest) subagentapp.TaskJobServiceResult {
	return s.cleanupResult
}

func (s taskJobServiceStub) AcceptTaskJobIsolation(string, subagentapp.TaskJobIsolationAcceptRequest) subagentapp.TaskJobServiceResult {
	return s.acceptResult
}

func (s taskJobServiceStub) ConflictReportTaskJobIsolation(string, subagentapp.TaskJobIsolationConflictReportRequest) subagentapp.TaskJobServiceResult {
	return s.conflictReportResult
}

func (s taskJobServiceStub) RepairCheckTaskJobIsolation(string, subagentapp.TaskJobIsolationRepairCheckRequest) subagentapp.TaskJobServiceResult {
	return s.repairCheckResult
}

func (s taskJobServiceStub) RepairAcceptTaskJobIsolation(string, subagentapp.TaskJobIsolationRepairAcceptRequest) subagentapp.TaskJobServiceResult {
	return s.repairAcceptResult
}

func (s taskJobServiceStub) ChildTodosTaskJob(string, subagentapp.TaskJobChildTodosRequest) subagentapp.TaskJobServiceResult {
	return s.childTodosResult
}

func (s taskJobServiceStub) ProjectChildTodosTaskJob(string, subagentapp.TaskJobChildTodoProjectRequest) subagentapp.TaskJobServiceResult {
	return s.childTodosProjectResult
}

func (s taskJobServiceStub) RejectChildTodoProjectionTaskJob(string, subagentapp.TaskJobChildTodoRejectRequest) subagentapp.TaskJobServiceResult {
	return s.childTodosRejectResult
}

func (s taskJobServiceStub) AcceptChildTodoProjectionTaskJob(string, subagentapp.TaskJobChildTodoAcceptRequest) subagentapp.TaskJobServiceResult {
	return s.childTodosAcceptResult
}
