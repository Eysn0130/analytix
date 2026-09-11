package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeHandlerAuthHealthAndDispatch(t *testing.T) {
	dispatcher := &recordingRuntimeDispatcher{}
	handler := RuntimeHandler{
		RuntimeToken: DefaultRuntimeToken,
		Dispatcher:   dispatcher,
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("health should be public and OK: %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/health", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("health POST should be method_not_allowed: %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/runtime/info", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth should be unauthorized: %d", recorder.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/runtime/info", nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if dispatcher.called != string(RouteRuntimeInfo) {
		t.Fatalf("runtime info route did not dispatch: %q", dispatcher.called)
	}
}

func TestRuntimeHandlerUnknownAndUserInputRoutes(t *testing.T) {
	dispatcher := &recordingRuntimeDispatcher{}
	handler := RuntimeHandler{
		RuntimeToken: DefaultRuntimeToken,
		Dispatcher:   dispatcher,
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/unknown", nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown route should be not_found: %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/user-input/abc", nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if dispatcher.called != string(RouteUserInputPath) || dispatcher.userInputPrefix != "/v1/user-input/" {
		t.Fatalf("legacy user-input route did not preserve prefix: called=%q prefix=%q", dispatcher.called, dispatcher.userInputPrefix)
	}
}

func TestRuntimeHandlerProviderRegistryRequiresAuthAndUsesVersionedFailure(t *testing.T) {
	dispatcher := &recordingRuntimeDispatcher{}
	handler := RuntimeHandler{RuntimeToken: DefaultRuntimeToken, Dispatcher: dispatcher}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, ProviderRegistryPathV1, nil))
	if recorder.Code != http.StatusUnauthorized || dispatcher.called != "" ||
		!strings.Contains(recorder.Body.String(), `"schemaVersion":1`) ||
		!strings.Contains(recorder.Body.String(), `"code":"unauthorized"`) {
		t.Fatalf("unauthorized Provider Registry response = (%d, %q, %s)", recorder.Code, dispatcher.called, recorder.Body.String())
	}

	request := httptest.NewRequest(http.MethodGet, ProviderRegistryPathV1, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if dispatcher.called != string(RouteProviderRegistry) {
		t.Fatalf("Provider Registry route did not dispatch: %q", dispatcher.called)
	}
}

func TestRuntimeHandlerPrivateMediaExecutionRequiresRuntimeAuthAndNoStore(t *testing.T) {
	dispatcher := &recordingRuntimeDispatcher{}
	handler := RuntimeHandler{RuntimeToken: DefaultRuntimeToken, Dispatcher: dispatcher}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, MediaExecutionPathV1, nil))
	if recorder.Code != http.StatusUnauthorized || dispatcher.called != "" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unauthorized media response = (%d, %q, %v)", recorder.Code, dispatcher.called, recorder.Header())
	}

	request := httptest.NewRequest(http.MethodPost, MediaExecutionPathV1, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if dispatcher.called != string(RouteMediaExecution) || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("private media route did not dispatch: called=%q headers=%v", dispatcher.called, recorder.Header())
	}
}

func TestRuntimeHandlerToolExecutionObservationRequiresExactAuthAndTarget(t *testing.T) {
	dispatcher := &recordingRuntimeDispatcher{}
	handler := RuntimeHandler{RuntimeToken: DefaultRuntimeToken, Dispatcher: dispatcher, Insecure: true}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, nil))
	if recorder.Code != http.StatusUnauthorized || dispatcher.called != "" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("insecure mode bypassed observation auth: status=%d called=%q headers=%v", recorder.Code, dispatcher.called, recorder.Header())
	}

	request := httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if dispatcher.called != string(RouteToolExecutionObserve) || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("exact observation route did not dispatch privately: called=%q headers=%v", dispatcher.called, recorder.Header())
	}

	for _, target := range []string{
		toolExecutionObservationPathV1 + "?scope=other",
		toolExecutionObservationPathV1 + "?",
		"/v1/runtime/tool-executions/%6fbserve",
	} {
		dispatcher.called = ""
		request = httptest.NewRequest(http.MethodPost, target, nil)
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || dispatcher.called != "" || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("non-canonical observation target dispatched: target=%q status=%d called=%q", target, recorder.Code, dispatcher.called)
		}
	}

	dispatcher.called = ""
	request = httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, nil)
	request.Header.Add("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Add("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || dispatcher.called != "" {
		t.Fatalf("duplicate observation auth headers dispatched: status=%d called=%q", recorder.Code, dispatcher.called)
	}
}

func TestRuntimeHandlerLocalDisplayRequiresRuntimeAuthAndTypedSinkHeader(t *testing.T) {
	dispatcher := &recordingRuntimeDispatcher{}
	called := false
	handler := LocalDisplayMuxV1{
		RuntimeToken: DefaultRuntimeToken,
		Next:         RuntimeHandler{RuntimeToken: DefaultRuntimeToken, Dispatcher: dispatcher},
		LocalDisplay: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}),
	}

	request := httptest.NewRequest(http.MethodPost, LocalDisplayDirectPreviewPathV1, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || called || dispatcher.called != "" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("generic runtime request reached local display: status=%d called=%v dispatcher=%q headers=%v", recorder.Code, called, dispatcher.called, recorder.Header())
	}

	request = httptest.NewRequest(http.MethodPost, LocalDisplayAcceptedSlotsPathV1, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || !called || dispatcher.called != "" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("typed local sink did not dispatch: status=%d called=%v dispatcher=%q headers=%v", recorder.Code, called, dispatcher.called, recorder.Header())
	}

	called = false
	dispatcher.called = ""
	request = httptest.NewRequest(http.MethodGet, LocalDisplayAcceptedSlotsPathV1, nil)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	request.Header.Set(LocalDisplayHeaderV1, LocalDisplayHeaderValueV1)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed || called || dispatcher.called != "" {
		t.Fatalf("non-POST local display request dispatched: status=%d called=%v dispatcher=%q", recorder.Code, called, dispatcher.called)
	}
}

type recordingRuntimeDispatcher struct {
	called          string
	userInputPrefix string
}

func (d *recordingRuntimeDispatcher) mark(route RuntimeRoute) {
	d.called = string(route)
}

func (d *recordingRuntimeDispatcher) RuntimeInfo(http.ResponseWriter, *http.Request) {
	d.mark(RouteRuntimeInfo)
}

func (d *recordingRuntimeDispatcher) RuntimeTools(http.ResponseWriter, *http.Request) {
	d.mark(RouteRuntimeTools)
}

func (d *recordingRuntimeDispatcher) ToolExecutionObservation(http.ResponseWriter, *http.Request) {
	d.mark(RouteToolExecutionObserve)
}

func (d *recordingRuntimeDispatcher) RuntimeTaskJobs(http.ResponseWriter, *http.Request) {
	d.mark(RouteRuntimeTaskJobs)
}

func (d *recordingRuntimeDispatcher) Usage(http.ResponseWriter, *http.Request) {
	d.mark(RouteUsage)
}

func (d *recordingRuntimeDispatcher) Skills(http.ResponseWriter, *http.Request) {
	d.mark(RouteSkills)
}

func (d *recordingRuntimeDispatcher) Attachments(http.ResponseWriter, *http.Request) {
	d.mark(RouteAttachments)
}

func (d *recordingRuntimeDispatcher) AttachmentDiagnostics(http.ResponseWriter, *http.Request) {
	d.mark(RouteAttachmentDiagnostics)
}

func (d *recordingRuntimeDispatcher) AttachmentPath(http.ResponseWriter, *http.Request) {
	d.mark(RouteAttachmentPath)
}

func (d *recordingRuntimeDispatcher) Memory(http.ResponseWriter, *http.Request) {
	d.mark(RouteMemory)
}

func (d *recordingRuntimeDispatcher) MemoryDiagnostics(http.ResponseWriter, *http.Request) {
	d.mark(RouteMemoryDiagnostics)
}

func (d *recordingRuntimeDispatcher) MemoryRecordPath(http.ResponseWriter, *http.Request) {
	d.mark(RouteMemoryRecordPath)
}

func (d *recordingRuntimeDispatcher) WorkspaceStatus(http.ResponseWriter, *http.Request) {
	d.mark(RouteWorkspaceStatus)
}

func (d *recordingRuntimeDispatcher) ProviderRegistry(http.ResponseWriter, *http.Request) {
	d.mark(RouteProviderRegistry)
}

func (d *recordingRuntimeDispatcher) MediaExecution(http.ResponseWriter, *http.Request) {
	d.mark(RouteMediaExecution)
}

func (d *recordingRuntimeDispatcher) CaseProjects(http.ResponseWriter, *http.Request) {
	d.mark(RouteCaseProjects)
}

func (d *recordingRuntimeDispatcher) Threads(http.ResponseWriter, *http.Request) {
	d.mark(RouteThreads)
}

func (d *recordingRuntimeDispatcher) ThreadPath(http.ResponseWriter, *http.Request) {
	d.mark(RouteThreadPath)
}

func (d *recordingRuntimeDispatcher) SessionPath(http.ResponseWriter, *http.Request) {
	d.mark(RouteSessionPath)
}

func (d *recordingRuntimeDispatcher) ApprovalPath(http.ResponseWriter, *http.Request) {
	d.mark(RouteApprovalPath)
}

func (d *recordingRuntimeDispatcher) UserInputPath(_ http.ResponseWriter, _ *http.Request, prefix string) {
	d.mark(RouteUserInputPath)
	d.userInputPrefix = prefix
}
