package sse

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type failingReplayDiagnosticWriter struct{}

func (failingReplayDiagnosticWriter) Write([]byte) (int, error) {
	panic("diagnostic writer failed")
}

func TestFiniteReplayDiagnosticsPreserveDeliveryWhenWriterFails(t *testing.T) {
	handler := ThreadEventsHandler{Now: func() time.Time { return time.Unix(0, 0) }, Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 1, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) {
			return []map[string]any{{"kind": "cursor_advanced", "threadId": "private-thread-canary", "seq": float64(1),
				"reason": "restricted_content_removed", "timestamp": "2026-10-19T00:00:00Z"}}, nil
		},
	})}
	request := httptest.NewRequest("GET", "/?since_seq=0", nil)
	baseline := httptest.NewRecorder()
	handler.ServeHTTP(baseline, request, "private-thread-canary")
	for _, writer := range []io.Writer{&bytes.Buffer{}, failingReplayDiagnosticWriter{}} {
		handler.ReplayDiagnostics = NewFiniteReplayDiagnostics(writer)
		observed := httptest.NewRecorder()
		handler.ServeHTTP(observed, request, "private-thread-canary")
		handler.ReplayDiagnostics.Finish(false)
		if baseline.Code != observed.Code || baseline.Body.String() != observed.Body.String() || !reflect.DeepEqual(baseline.Header(), observed.Header()) {
			t.Fatal("optional replay diagnostics changed the response")
		}
		if output, ok := writer.(*bytes.Buffer); ok {
			if strings.Contains(output.String(), "private-thread-canary") || strings.Contains(output.String(), "restricted_content_removed") {
				t.Fatal("replay diagnostics retained event values")
			}
		}
	}
}

func TestFiniteReplayDiagnosticsUseClosedPhasesAndNumericFields(t *testing.T) {
	var output bytes.Buffer
	diagnostic := NewFiniteReplayDiagnostics(&output)
	unknown := diagnostic.Begin(FiniteReplayPhase("private-path-canary"))
	unknown(7)
	complete := diagnostic.Begin(ReplayLoadEvents)
	complete(12)
	diagnostic.Finish(true)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("unexpected diagnostic record count: %d", len(lines))
	}
	expectedPhases := []string{"request_start", "load_events_start", "load_events_end", "request_cancelled"}
	for index, line := range lines {
		if !strings.HasPrefix(line, FiniteReplayDiagnosticPrefix) {
			t.Fatal("missing fixed diagnostic prefix")
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, FiniteReplayDiagnosticPrefix)), &value); err != nil {
			t.Fatal(err)
		}
		if len(value) != 5 || value["phase"] != expectedPhases[index] {
			t.Fatal("diagnostic shape or phase changed")
		}
		if expectedPhases[index] == "load_events_end" && value["count"] != float64(12) {
			t.Fatal("completed load count was not preserved")
		}
		for _, key := range []string{"elapsedMs", "phaseElapsedMs", "requestCount", "count"} {
			number, ok := value[key].(float64)
			if !ok || number < 0 {
				t.Fatal("diagnostics must contain nonnegative numeric measurements")
			}
		}
	}
	var disabled *FiniteReplayDiagnostics
	disabled.Begin(ReplayLoadEvents)(0)
	disabled.Finish(false)
}

func TestFiniteReplayDiagnosticsEnterLoadBeforeCallingStore(t *testing.T) {
	var output bytes.Buffer
	called := false
	handler := ThreadEventsHandler{ReplayDiagnostics: NewFiniteReplayDiagnostics(&output), Store: identityThreadEventStore(ThreadEventStore{
		HighestSeq: func(string) (int, error) { return 1, nil },
		LoadEventsSince: func(string, int) ([]map[string]any, error) {
			called = true
			if !strings.Contains(output.String(), `"phase":"load_events_start"`) || strings.Contains(output.String(), `"phase":"load_events_end"`) {
				t.Fatal("load entry was not observed before store work")
			}
			return nil, nil
		},
	})}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), "thread-canary")
	if !called || !strings.Contains(output.String(), `"phase":"load_events_end"`) {
		t.Fatal("load callback or completion was not observed")
	}
}

func TestFiniteReplayDiagnosticsCountProjectionRevocationResponse(t *testing.T) {
	var output bytes.Buffer
	handler := ThreadEventsHandler{ReplayDiagnostics: NewFiniteReplayDiagnostics(&output), Store: ThreadEventStore{
		PreflightPublic: func(string) string { return casePublicAuthorityRejectedCode },
	}}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), "thread-canary")
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var value map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, FiniteReplayDiagnosticPrefix)), &value); err != nil {
			t.Fatal(err)
		}
		if value["phase"] == "response_write_end" {
			if value["count"] != float64(1) {
				t.Fatal("revocation response must count its one control event")
			}
			return
		}
	}
	t.Fatal("missing revocation response measurement")
}
