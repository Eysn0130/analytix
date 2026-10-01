package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
)

func maintenanceRequest(t *testing.T, h http.Handler, body string, authorized bool) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/quiescence", strings.NewReader(body))
	if authorized {
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)
	result := map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode maintenance response: %v", err)
	}
	return recorder.Code, result
}

func newMaintenanceTestHandler(t *testing.T) *runtimeServerHandler {
	t.Helper()
	return NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		Host: "127.0.0.1",
	}).(*runtimeServerHandler)
}

func TestRuntimeMaintenanceFencesNewMutationsWithoutCancellingWork(t *testing.T) {
	h := newMaintenanceTestHandler(t)
	if code, _ := maintenanceRequest(t, h, `{"operation":"prepare"}`, false); code != http.StatusUnauthorized {
		t.Fatalf("unauthorized maintenance prepare status = %d", code)
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"prepare","operation":"release"}`, true); code != http.StatusBadRequest {
		t.Fatalf("ambiguous maintenance request status = %d", code)
	}

	finish, admitted := h.runtimeControl().BeginTurnOperation()
	if !admitted {
		t.Fatal("ordinary turn operation did not start")
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"prepare"}`, true); code != http.StatusConflict {
		t.Fatalf("active turn operation status = %d", code)
	}
	finish()
	backgroundCancel := func() {}
	if _, ok := h.runtimeSubagentState().RegisterBackgroundJobWithStartBarrier("job-active", backgroundCancel); !ok {
		t.Fatal("background job did not register")
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"prepare"}`, true); code != http.StatusConflict {
		t.Fatalf("active background job status = %d", code)
	}
	h.runtimeSubagentState().UnregisterBackgroundJob("job-active")

	active := httptest.NewRequest(http.MethodPost, "/v1/threads", nil)
	if admitted, _ := h.admitRuntimeMaintenanceRequest(httptest.NewRecorder(), active); !admitted {
		t.Fatal("preexisting mutation did not enter")
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"prepare"}`, true); code != http.StatusConflict {
		t.Fatalf("in-flight HTTP mutation status = %d", code)
	}
	h.finishRuntimeMaintenanceRequest(active)

	code, result := maintenanceRequest(t, h, `{"operation":"prepare"}`, true)
	lease, ok := result["lease"].(string)
	if code != http.StatusOK || !ok || len(lease) != 43 || result["state"] != "idle" ||
		result["runtimePid"] == nil || result["expiresAtUnixMs"] == nil {
		t.Fatalf("maintenance prepare did not return a bounded same-process lease: status=%d result=%#v", code, result)
	}
	if _, admitted := h.runtimeControl().BeginTurnOperation(); admitted {
		t.Fatal("Go-owned turn entered during maintenance")
	}
	if _, _, err := h.runtimeControl().BeginAuxiliary(context.Background(), "thread-late"); err == nil {
		t.Fatal("Go-owned auxiliary operation entered during maintenance")
	}
	if _, err := h.runtimeControl().PrepareForeground(context.Background(), "thread-late"); err == nil {
		t.Fatal("foreground preparation entered during maintenance")
	}
	if _, admitted := h.runtimeSubagentState().RegisterBackgroundJobWithStartBarrier("late-job", func() {}); admitted {
		t.Fatal("Go-owned background job entered during maintenance")
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/v1/threads", nil)
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("new %s thread request crossed maintenance: %d", method, recorder.Code)
		}
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"release","lease":"`+strings.Repeat("x", 43)+`"}`, true); code != http.StatusConflict {
		t.Fatalf("wrong lease release status = %d", code)
	}
	if code, result := maintenanceRequest(t, h, `{"operation":"release","lease":"`+lease+`"}`, true); code != http.StatusOK || result["released"] != true {
		t.Fatalf("exact lease release status=%d result=%#v", code, result)
	}
	finish, admitted = h.runtimeControl().BeginTurnOperation()
	if !admitted {
		t.Fatal("ordinary turn admission did not resume after release")
	}
	finish()
	if _, admitted := h.runtimeSubagentState().RegisterBackgroundJobWithStartBarrier("later-job", func() {}); !admitted {
		t.Fatal("background job admission did not resume after release")
	}
	h.runtimeSubagentState().UnregisterBackgroundJob("later-job")
	_ = h.Shutdown(context.Background())
}

func TestRuntimeMaintenanceCoversPrivateMuxAndDrainsSSE(t *testing.T) {
	h := newMaintenanceTestHandler(t)
	privateEntered := make(chan struct{})
	privateRelease := make(chan struct{})
	private := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-privateEntered:
		default:
			close(privateEntered)
		}
		<-privateRelease
		w.WriteHeader(http.StatusOK)
	})
	mux := httpapi.LocalDisplayMuxV1{RuntimeToken: DefaultRuntimeToken, Next: h, LocalDisplay: private}
	wrapped, err := WrapRuntimeMaintenanceAdmissionV1(h, mux)
	if err != nil {
		t.Fatal(err)
	}
	privateRequest := func(path string) *http.Request {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
		request.Header.Set(httpapi.LocalDisplayHeaderV1, httpapi.LocalDisplayHeaderValueV1)
		return request
	}
	privateDone := make(chan struct{})
	go func() {
		wrapped.ServeHTTP(httptest.NewRecorder(), privateRequest(httpapi.HostFundsImportStagePathV1))
		close(privateDone)
	}()
	<-privateEntered
	if code, _ := maintenanceRequest(t, wrapped, `{"operation":"prepare"}`, true); code != http.StatusConflict {
		t.Fatalf("private mutation did not block prepare: %d", code)
	}
	close(privateRelease)
	<-privateDone

	sseEntered := make(chan struct{})
	sseDone := make(chan struct{})
	sseNext := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/events") {
			close(sseEntered)
			<-r.Context().Done()
			close(sseDone)
			return
		}
		mux.ServeHTTP(w, r)
	})
	wrappedSSE, err := WrapRuntimeMaintenanceAdmissionV1(h, sseNext)
	if err != nil {
		t.Fatal(err)
	}
	sseRequest := httptest.NewRequest(http.MethodGet, "/v1/threads/stream/events", nil)
	go wrappedSSE.ServeHTTP(httptest.NewRecorder(), sseRequest)
	<-sseEntered
	code, ready := maintenanceRequest(t, wrappedSSE, `{"operation":"prepare"}`, true)
	if code != http.StatusOK {
		t.Fatalf("active SSE did not drain before prepare: %d %#v", code, ready)
	}
	<-sseDone
	lease, _ := ready["lease"].(string)
	rejected := httptest.NewRecorder()
	wrapped.ServeHTTP(rejected, privateRequest(httpapi.HostFundsImportConfirmPathV1))
	if rejected.Code != http.StatusServiceUnavailable {
		t.Fatalf("private selector crossed full-mux fence: %d", rejected.Code)
	}
	if code, _ := maintenanceRequest(t, wrapped, `{"operation":"release","lease":"`+lease+`"}`, true); code != http.StatusOK {
		t.Fatalf("release after SSE drain: %d", code)
	}
	_ = wrapped.(interface{ Shutdown(context.Context) error }).Shutdown(context.Background())
}

func TestRuntimeMaintenanceCancelledScanCannotPublishLateLease(t *testing.T) {
	h := newMaintenanceTestHandler(t)
	entered := make(chan struct{})
	releaseRead := make(chan struct{})
	h.store.beforeMaintenanceReadHook = func() { close(entered); <-releaseRead }
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/quiescence", strings.NewReader(`{"operation":"prepare"}`)).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.ServeHTTP(recorder, request); close(done) }()
	<-entered
	cancel()
	deadline := time.After(time.Second)
	for {
		h.maintenanceMu.Lock()
		closed := h.maintenanceClosed
		h.maintenanceMu.Unlock()
		if !closed {
			break
		}
		select {
		case <-deadline:
			t.Fatal("cancelled prepare retained ordinary admission")
		case <-time.After(time.Millisecond):
		}
	}
	ordinary := httptest.NewRequest(http.MethodPost, "/v1/threads", nil)
	admitted, _ := h.admitRuntimeMaintenanceRequest(httptest.NewRecorder(), ordinary)
	if !admitted {
		t.Fatal("ordinary request was not admitted after prepare cancellation")
	}
	h.finishRuntimeMaintenanceRequest(ordinary)
	close(releaseRead)
	<-done
	h.maintenanceMu.Lock()
	closed, lease := h.maintenanceClosed, h.maintenanceLease
	h.maintenanceMu.Unlock()
	if closed || lease != "" || recorder.Code == http.StatusOK {
		t.Fatalf("late scan published a lease: closed=%t lease=%q status=%d", closed, lease, recorder.Code)
	}
	_ = h.Shutdown(context.Background())
}

func TestRuntimeMaintenanceDeadlineReleasesFenceDuringBlockedRead(t *testing.T) {
	h := newMaintenanceTestHandler(t)
	entered := make(chan struct{})
	releaseRead := make(chan struct{})
	h.store.beforeMaintenanceReadHook = func() { close(entered); <-releaseRead }
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/quiescence", strings.NewReader(`{"operation":"prepare"}`))
	request.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.ServeHTTP(recorder, request); close(done) }()
	<-entered
	h.maintenanceMu.Lock()
	attempt := h.maintenanceAttempt
	h.maintenanceMu.Unlock()
	h.expireRuntimeMaintenanceAttempt(attempt)
	ordinary := httptest.NewRequest(http.MethodPost, "/v1/threads", nil)
	admitted, _ := h.admitRuntimeMaintenanceRequest(httptest.NewRecorder(), ordinary)
	if !admitted {
		t.Fatal("deadline did not restore ordinary admission while read was blocked")
	}
	h.finishRuntimeMaintenanceRequest(ordinary)
	close(releaseRead)
	<-done
	h.maintenanceMu.Lock()
	closed, lease := h.maintenanceClosed, h.maintenanceLease
	h.maintenanceMu.Unlock()
	if closed || lease != "" || recorder.Code == http.StatusOK {
		t.Fatalf("late read published a lease after deadline: closed=%t lease=%q status=%d", closed, lease, recorder.Code)
	}
	_ = h.Shutdown(context.Background())
}

func TestRuntimeMaintenanceCommitWinsQueuedExpiryOnlyBeforeDeadline(t *testing.T) {
	h := newMaintenanceTestHandler(t)
	code, ready := maintenanceRequest(t, h, `{"operation":"prepare"}`, true)
	lease, _ := ready["lease"].(string)
	if code != http.StatusOK {
		t.Fatalf("prepare: %d", code)
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"commit-stop","lease":"`+lease+`"}`, true); code != http.StatusOK {
		t.Fatalf("commit stop: %d", code)
	}
	h.expireRuntimeMaintenance(lease)
	h.maintenanceMu.Lock()
	closed, committed := h.maintenanceClosed, h.maintenanceCommitted
	h.maintenanceMu.Unlock()
	if !closed || !committed {
		t.Fatal("queued expiry reopened a committed stop fence")
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"release","lease":"`+lease+`"}`, true); code != http.StatusOK {
		t.Fatalf("committed release: %d", code)
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"release","lease":"`+lease+`"}`, true); code != http.StatusOK {
		t.Fatalf("lost release acknowledgement was not idempotent: %d", code)
	}
	oldLease := lease
	code, ready = maintenanceRequest(t, h, `{"operation":"prepare"}`, true)
	lease, _ = ready["lease"].(string)
	if code != http.StatusOK {
		t.Fatalf("second prepare: %d", code)
	}
	if code, _ := maintenanceRequest(t, h, `{"operation":"release","lease":"`+oldLease+`"}`, true); code != http.StatusConflict {
		t.Fatalf("old lease released a new fence: %d", code)
	}
	h.maintenanceMu.Lock()
	h.maintenanceExpiresAt = time.Now().Add(-time.Second)
	h.maintenanceMu.Unlock()
	if code, _ := maintenanceRequest(t, h, `{"operation":"commit-stop","lease":"`+lease+`"}`, true); code != http.StatusConflict {
		t.Fatalf("expired lease committed: %d", code)
	}
	h.maintenanceMu.Lock()
	closed = h.maintenanceClosed
	h.maintenanceMu.Unlock()
	if closed {
		t.Fatal("expired lease retained admission fence")
	}
	_ = h.Shutdown(context.Background())
}
