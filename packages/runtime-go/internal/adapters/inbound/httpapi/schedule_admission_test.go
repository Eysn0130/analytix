package httpapi

import (
	admission "analytix.local/runtime-go/internal/domain/scheduleadmission"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScheduleAdmissionPrivateHTTPBoundaryV1(t *testing.T) {
	r := admission.RequestV1{SchemaVersion: 1, Kind: "query", TaskID: "task-1", Origin: admission.OriginV1{ThreadID: "thread-1", TurnID: "turn-1", ToolCallID: toolidentity.MustHostToolCallIDV1("http-schedule"), GrantID: "grant-1"}, AttemptID: "a8b6850e-10e3-4a54-a80c-140899708b67", ScheduledAt: "2026-10-12T01:00:00Z", Prompt: "frozen", Execution: admission.ExecutionV1{}}
	b, _ := json.Marshal(r)
	calls := 0
	leaf := ScheduleAdmissionHandlerV1{Admit: func(_ context.Context, got admission.RequestV1) admission.ResponseV1 {
		calls++
		if got.Kind != "query" {
			t.Fatal("decoded request changed")
		}
		return admission.ResultV1("not_found", nil)
	}}
	for _, tc := range []struct {
		name, token, auth, header, body string
		insecure                        bool
		want                            int
	}{
		{"normal", "token", "Bearer token", "typed-v1", string(b), false, 200},
		{"no-bearer", "token", "", "typed-v1", string(b), true, 401},
		{"empty-token", "", "", "typed-v1", string(b), true, 401},
		{"no-header", "token", "Bearer token", "", string(b), false, 403},
		{"duplicate", "token", "Bearer token", "typed-v1", strings.Replace(string(b), `"kind":"query"`, `"kind":"query","kind":"submit"`, 1), false, 400},
		{"old-origin", "token", "Bearer token", "typed-v1", strings.Replace(string(b), `"kind":"query"`, `"kind":"origin"`, 1), false, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, ScheduleAdmissionPathV1, strings.NewReader(tc.body))
			req.Header.Set("Authorization", tc.auth)
			if tc.header != "" {
				req.Header.Set(LocalDisplayHeaderV1, tc.header)
			}
			w := httptest.NewRecorder()
			LocalDisplayMuxV1{RuntimeToken: tc.token, Insecure: tc.insecure, ScheduleAdmissions: leaf}.ServeHTTP(w, req)
			if w.Code != tc.want || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("private result %d want %d", w.Code, tc.want)
			}
		})
	}
	if calls != 1 {
		t.Fatalf("rejected private requests reached dispatch: %d", calls)
	}
}
