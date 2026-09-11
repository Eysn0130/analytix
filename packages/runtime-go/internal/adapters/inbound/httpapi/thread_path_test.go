package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestThreadPathHandlersRoutesThreadSubpaths(t *testing.T) {
	tests := []struct {
		path      string
		wantName  string
		wantValue string
	}{
		{"/v1/threads/thr_1/summary", "summary", "thr_1/summary"},
		{"/v1/threads/thr_1/summary/tasks/task_1", "summary", "thr_1/summary/tasks/task_1"},
		{"/v1/threads/thr_1/events", "events", "thr_1"},
		{"/v1/threads/thr_1/fork", "fork", "thr_1"},
		{"/v1/threads/thr_1/goal", "goal", "thr_1"},
		{"/v1/threads/thr_1/todos", "todos", "thr_1"},
		{"/v1/threads/thr_1/compact", "compact", "thr_1"},
		{"/v1/threads/thr_1/review", "review", "thr_1"},
		{"/v1/threads/thr_1/checkpoints/cp_1", "checkpoints", "thr_1/checkpoints/cp_1"},
		{"/v1/threads/thr_1/turns/turn_1/cancel", "turn_action", "thr_1/turns/turn_1/cancel"},
		{"/v1/threads/thr_1/turns", "turns", "thr_1"},
		{"/v1/threads/thr_1/rewind", "rewind", "thr_1"},
		{"/v1/threads/thr_1", "record", "thr_1"},
	}
	for _, tc := range tests {
		calledName := ""
		calledValue := ""
		handler := func(name string) ThreadPathHandler {
			return func(w http.ResponseWriter, r *http.Request, value string) {
				calledName = name
				calledValue = value
				WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
			}
		}
		response := httptest.NewRecorder()
		ThreadPathHandlers{
			Summary:     handler("summary"),
			Events:      handler("events"),
			Fork:        handler("fork"),
			Goal:        handler("goal"),
			Todos:       handler("todos"),
			Compact:     handler("compact"),
			Review:      handler("review"),
			Checkpoints: handler("checkpoints"),
			TurnAction:  handler("turn_action"),
			Turns:       handler("turns"),
			Rewind:      handler("rewind"),
			Record:      handler("record"),
		}.Handle(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != http.StatusOK || calledName != tc.wantName || calledValue != tc.wantValue {
			t.Fatalf("%s routed to %s(%q), status %d; want %s(%q)", tc.path, calledName, calledValue, response.Code, tc.wantName, tc.wantValue)
		}
	}
}

func TestThreadPathHandlersReturnsNotFoundForUnknownOrMissingHandler(t *testing.T) {
	for _, path := range []string{"/v1/threads/thr_1/unknown/path", "/v1/threads/thr_1/events"} {
		response := httptest.NewRecorder()
		ThreadPathHandlers{}.Handle(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, response.Code)
		}
	}
}
