package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadsummaryapp "analytix.local/runtime-go/internal/app/threadsummary"
	domainjob "analytix.local/runtime-go/internal/domain/job"
)

func TestThreadSummaryHandlersSummaryAndCommandOutput(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	stub := &threadSummaryServiceStub{
		summary: validThreadSummaryResponseTestV1("thr_1"),
		commandOutput: map[string]any{
			"schemaVersion": 1, "availability": "withheld", "taskId": "command:call-1", "status": "completed",
			"reasonCode": "tool_output_private", "outputWithheld": true, "outputTrustStatus": "private_tool_output",
			"factAnswerAllowed": false, "evidenceAuthority": false, "canReadOutput": false, "canContinueParent": false,
		},
		commandOK: true,
	}
	handler := ThreadSummaryHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary")
	body := decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusOK || body["threadId"] != "thr_1" {
		t.Fatalf("unexpected summary response code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ignored?offset=2&limit=3000000", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/command%3Acall-1/output")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusOK || body["schemaVersion"] != float64(1) || body["availability"] != "withheld" || body["status"] != "completed" || stub.outputTaskID != "command:call-1" || stub.outputOffset != 2 || stub.outputLimit != maxThreadSummaryOutputLimit {
		t.Fatalf("unexpected output response code=%d body=%#v stub=%#v", recorder.Code, body, stub)
	}
	assertThreadSummaryOutputWithheld(t, body, "command:call-1", "tool_output_private", "private_tool_output", privateSentinel)
}

func TestThreadSummaryHandlersTaskJobOutputAndKillErrors(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	stub := &threadSummaryServiceStub{
		taskJobOutput: subagentapp.TaskJobServiceResult{
			Record: domainjob.Record{ID: "job-1", ParentThreadID: "thr_1", Status: "running", Output: privateSentinel, Error: privateSentinel},
			Response: subagentapp.TaskJobOutputWithheldResponseV1(domainjob.Record{
				ID: "job-1", Status: "running", Output: privateSentinel, Error: privateSentinel,
			}),
		},
		killResult: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorForbidden},
	}
	handler := ThreadSummaryHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored?offset=1&limit=3", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/taskjob%3Ajob-1/output")
	body := decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusOK || body["schemaVersion"] != float64(1) || body["availability"] != "withheld" || body["taskId"] != "taskjob:job-1" || stub.outputJobID != "job-1" {
		t.Fatalf("unexpected task job output code=%d body=%#v stub=%#v", recorder.Code, body, stub)
	}
	assertThreadSummaryOutputWithheld(t, body, "taskjob:job-1", "security_bound_child_output", "untrusted_child_output", privateSentinel)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ignored?offset=1&limit=3", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/run%3Ajob-1/output")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusOK || body["taskId"] != "run:job-1" || stub.outputJobID != "job-1" {
		t.Fatalf("unexpected run job output code=%d body=%#v stub=%#v", recorder.Code, body, stub)
	}
	assertThreadSummaryOutputWithheld(t, body, "run:job-1", "security_bound_child_output", "untrusted_child_output", privateSentinel)

	stub.taskJobOutput = subagentapp.TaskJobServiceResult{
		Record: domainjob.Record{ID: "job-1", ParentThreadID: "thr_1", Status: "completed", Output: privateSentinel, Error: privateSentinel},
		Response: map[string]any{
			"schemaVersion": 1, "availability": "available", "jobId": "job-1", "status": "completed",
			"output": privateSentinel, "offset": 0, "nextOffset": 1, "outputBytes": 1, "truncated": false,
		},
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ignored?offset=99&limit=1", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/taskjob%3Ajob-1/output")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusOK || body["availability"] != "withheld" || strings.Contains(recorder.Body.String(), privateSentinel) {
		t.Fatalf("fact-bearing service response was not ignored code=%d body=%#v", recorder.Code, body)
	}
	stub.taskJobOutput = subagentapp.TaskJobServiceResult{
		Record:   domainjob.Record{ID: "<think>" + privateSentinel, Status: "completed"},
		Response: map[string]any{"output": privateSentinel},
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/taskjob%3Ajob-1/output")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusInternalServerError || body["code"] != "task_job_output_schema_invalid" || strings.Contains(recorder.Body.String(), privateSentinel) {
		t.Fatalf("invalid typed task-job authority did not fail closed code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/taskjob%3Ajob-1/kill")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusForbidden || body["message"] != testForbiddenFailureMessage {
		t.Fatalf("unexpected kill response code=%d body=%#v", recorder.Code, body)
	}

	stub.killResult = subagentapp.TaskJobServiceResult{
		Record: domainjob.Record{
			ID:             "job-1",
			Kind:           "subagent",
			ParentThreadID: "thr_1",
			Status:         "killed",
		},
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/job-1/kill")
	body = decodeThreadSummaryBody(t, recorder)
	task, _ := body["task"].(map[string]any)
	if recorder.Code != http.StatusOK || task["id"] != "taskjob:job-1" || task["status"] != "killed" || task["kind"] != "subagent" ||
		task["terminal"] != true || task["outputWithheld"] != true || task["canReadOutput"] != false || stub.killJobID != "job-1" {
		t.Fatalf("unexpected successful task job kill response code=%d body=%#v stub=%#v", recorder.Code, body, stub)
	}
}

func assertThreadSummaryOutputWithheld(t *testing.T, body map[string]any, taskID string, reasonCode string, trustStatus string, privateSentinel string) {
	t.Helper()
	if body["schemaVersion"] != float64(1) || body["availability"] != "withheld" || body["taskId"] != taskID ||
		body["reasonCode"] != reasonCode || body["outputWithheld"] != true ||
		body["outputTrustStatus"] != trustStatus || body["factAnswerAllowed"] != false || body["evidenceAuthority"] != false ||
		body["canReadOutput"] != false || body["canContinueParent"] != false {
		t.Fatalf("unexpected withheld summary task output: %#v", body)
	}
	encoded, _ := json.Marshal(body)
	if strings.Contains(string(encoded), privateSentinel) {
		t.Fatalf("summary task output leaked private persisted bytes: %s", encoded)
	}
	for _, forbidden := range []string{"output", "offset", "nextOffset", "outputBytes", "truncated", "artifactPath", "usage", "error"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("withheld summary task output exposed %s: %#v", forbidden, body)
		}
	}
}

func TestThreadSummaryHandlersRestart(t *testing.T) {
	stub := &threadSummaryServiceStub{}
	handler := ThreadSummaryHandlers{Service: stub}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/command%3Acall-1/restart")
	body := decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["message"] != testConflictFailureMessage {
		t.Fatalf("unexpected restart response code=%d body=%#v stub=%#v", recorder.Code, body, stub)
	}

	stub.killResult = subagentapp.TaskJobServiceResult{
		Record: domainjob.Record{
			ID:             "job-2",
			Kind:           "subagent",
			ParentThreadID: "thr_1",
			Status:         "queued",
		},
	}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/run%3Ajob-2/restart")
	body = decodeThreadSummaryBody(t, recorder)
	task, _ := body["task"].(map[string]any)
	if recorder.Code != http.StatusOK || task["id"] != "taskjob:job-2" || task["kind"] != "subagent" || task["status"] != "queued" || stub.killJobID != "job-2" {
		t.Fatalf("unexpected subagent restart response code=%d body=%#v stub=%#v", recorder.Code, body, stub)
	}
}

func TestThreadSummaryHandlersValidationAndErrors(t *testing.T) {
	handler := ThreadSummaryHandlers{Service: &threadSummaryServiceStub{summaryErr: threadsummaryapp.ErrThreadNotFound}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/%zz/output")
	body := decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusBadRequest || body["message"] != testValidationFailureMessage {
		t.Fatalf("unexpected invalid task id code=%d body=%#v", recorder.Code, body)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "missing/summary")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected not found code=%d body=%#v", recorder.Code, body)
	}

	handler = ThreadSummaryHandlers{Service: &threadSummaryServiceStub{taskJobOutput: subagentapp.TaskJobServiceResult{ErrorCode: subagentapp.TaskJobErrorNotFound, Err: os.ErrNotExist}}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/taskjob%3Ajob-1/output")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusNotFound || body["message"] != testNotFoundFailureMessage {
		t.Fatalf("unexpected task job not found code=%d body=%#v", recorder.Code, body)
	}

	handler = ThreadSummaryHandlers{Service: &threadSummaryServiceStub{}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/command%3Acall-1/restart")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["message"] != testConflictFailureMessage {
		t.Fatalf("unexpected command restart response code=%d body=%#v", recorder.Code, body)
	}

	handler = ThreadSummaryHandlers{Service: &threadSummaryServiceStub{}}
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/ignored", nil)
	handler.HandleThreadSummaryPath(recorder, request, "thr_1/summary/tasks/taskjob%3Ajob-1/restart")
	body = decodeThreadSummaryBody(t, recorder)
	if recorder.Code != http.StatusConflict || body["message"] != testConflictFailureMessage {
		t.Fatalf("unexpected task job restart conflict code=%d body=%#v", recorder.Code, body)
	}
}

func decodeThreadSummaryBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func validThreadSummaryResponseTestV1(threadID string) map[string]any {
	return map[string]any{
		"threadId": threadID, "generatedAt": "2026-07-20T00:00:00Z", "latestSeq": float64(0),
		"subagents": []any{}, "tasks": []any{}, "outputs": []any{}, "sources": []any{},
		"sideChats": []any{}, "backgroundProcesses": []any{},
	}
}

type threadSummaryServiceStub struct {
	summary       map[string]any
	summaryErr    error
	contextErr    error
	commandOutput map[string]any
	commandOK     bool
	outputTaskID  string
	outputJobID   string
	outputOffset  int
	outputLimit   int
	taskJobOutput subagentapp.TaskJobServiceResult
	killResult    subagentapp.TaskJobServiceResult
	killJobID     string
}

func (s *threadSummaryServiceStub) Summary(string) (map[string]any, error) {
	return s.summary, s.summaryErr
}

func (s *threadSummaryServiceStub) LoadContext(threadID string) (threadsummaryapp.Context, error) {
	return threadsummaryapp.Context{ThreadID: threadID, Thread: map[string]any{"id": threadID}}, s.contextErr
}

func (s *threadSummaryServiceStub) CommandOutput(_ threadsummaryapp.Context, taskID string, offset int, limit int) (map[string]any, bool) {
	s.outputTaskID = taskID
	s.outputOffset = offset
	s.outputLimit = limit
	return s.commandOutput, s.commandOK
}

func (s *threadSummaryServiceStub) OutputTaskJob(_ string, request subagentapp.TaskJobOutputRequest, _ bool) subagentapp.TaskJobServiceResult {
	s.outputJobID = request.JobID
	return s.taskJobOutput
}

func (s *threadSummaryServiceStub) KillTaskJob(_ string, request subagentapp.TaskJobKillRequest) subagentapp.TaskJobServiceResult {
	s.killJobID = request.JobID
	return s.killResult
}

func (s *threadSummaryServiceStub) RestartTaskJob(_ string, jobID string) subagentapp.TaskJobServiceResult {
	s.killJobID = jobID
	if s.killResult.Response == nil && s.killResult.Record.ID == "" && s.killResult.ErrorCode == "" {
		return subagentapp.TaskJobServiceResult{
			Response:  subagentapp.ValidationErrorResponse("summary task can not be restarted by the runtime route: taskjob:" + jobID),
			IsError:   true,
			ErrorCode: subagentapp.TaskJobErrorValidation,
		}
	}
	return s.killResult
}

var _ ThreadSummaryService = (*threadSummaryServiceStub)(nil)
