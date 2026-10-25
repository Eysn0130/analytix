package httpapi

import (
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolResultLocalDisplayTypedHostOnlyBoundary(t *testing.T) {
	calls := 0
	leaf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) })
	for _, test := range []struct {
		token, bearer, header, method string
		insecure                      bool
		status                        int
	}{
		{"host-secret", "", "local-display-v1", "POST", false, 401},
		{"host-secret", "Bearer host-secret", "", "POST", false, 403},
		{"host-secret", "Bearer host-secret", LocalDisplayHeaderValueV1, "GET", false, 405},
		{"", "", LocalDisplayHeaderValueV1, "POST", true, 401},
		{"host-secret", "Bearer host-secret", LocalDisplayHeaderValueV1, "POST", true, 200},
	} {
		r := httptest.NewRequest(test.method, ToolResultLocalDisplayPathV1, strings.NewReader(`{}`))
		r.Header.Set("Authorization", test.bearer)
		r.Header.Set(LocalDisplayHeaderV1, test.header)
		w := httptest.NewRecorder()
		LocalDisplayMuxV1{RuntimeToken: test.token, Insecure: test.insecure, ToolResults: leaf}.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("host boundary status=%d expected=%d", w.Code, test.status)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("host result caching enabled")
		}
	}
	if calls != 1 {
		t.Fatal("unauthorized request reached protected handler")
	}
}
func TestToolResultLocalDisplayStrictSelectorAndUnavailableBody(t *testing.T) {
	call := "call_host_" + strings.Repeat("a", 64)
	result := domaintoolresult.ToolResultItemIDV1("turn-a", call)
	valid := `{"threadId":"thread-a","turnId":"turn-a","callId":"` + call + `","resultItemId":"` + result + `"}`
	for _, test := range []struct {
		body   string
		status int
	}{
		{valid, 404}, {strings.Replace(valid, `"threadId":"thread-a"`, `"threadId":"thread-a","threadId":"thread-b"`, 1), 400},
		{strings.Replace(valid, `"threadId":`, `"body":"private","threadId":`, 1), 400}, {valid + `{}`, 400}, {strings.Repeat("x", 4097), 400},
	} {
		r := httptest.NewRequest("POST", ToolResultLocalDisplayPathV1, strings.NewReader(test.body))
		w := httptest.NewRecorder()
		ToolResultLocalDisplayHandlerV1{}.ServeHTTP(w, r)
		if w.Code != test.status || strings.Contains(w.Body.String(), "private") {
			t.Fatal("strict selector/unavailable failure violated")
		}
	}
}

type toolResultShutdownFixtureV1 struct {
	calls int
	err   error
}

func (*toolResultShutdownFixtureV1) ServeHTTP(http.ResponseWriter, *http.Request) {}
func (f *toolResultShutdownFixtureV1) Shutdown(context.Context) error             { f.calls++; return f.err }
func TestToolResultLocalDisplayShutdownRetainsStoreUntilCoreDrainSucceeds(t *testing.T) {
	next := &toolResultShutdownFixtureV1{err: errors.New("core drain incomplete")}
	store := &toolResultShutdownFixtureV1{}
	mux := LocalDisplayMuxV1{Next: next, ToolResults: store}
	if err := mux.Shutdown(context.Background()); err == nil || store.calls != 0 {
		t.Fatal("active settlement lost private owner after failed drain")
	}
	next.err = nil
	if err := mux.Shutdown(context.Background()); err != nil || next.calls != 2 || store.calls != 1 {
		t.Fatal("successful retry did not drain before closing private owner")
	}
}

func TestToolResultLocalDisplayPresentHistoryClosedRequestV1(t *testing.T) {
	valid := `{"kind":"present_history","threadId":"thread-a","turnId":"turn-a"}`
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"valid_unavailable", valid, 404},
		{"unknown_kind", strings.Replace(valid, "present_history", "history", 1), 400},
		{"empty_kind", strings.Replace(valid, "present_history", "", 1), 400},
		{"null_kind", strings.Replace(valid, `"present_history"`, `null`, 1), 400},
		{"null_turn", strings.Replace(valid, `"turn-a"`, `null`, 1), 400},
		{"missing_kind", `{"threadId":"thread-a","turnId":"turn-a"}`, 400},
		{"duplicate_kind", strings.Replace(valid, `"kind":`, `"kind":"present_history","kind":`, 1), 400},
		{"mixed_call", strings.Replace(valid, `"threadId":`, `"callId":"call_host_`+strings.Repeat("a", 64)+`","threadId":`, 1), 400},
		{"mixed_result", strings.Replace(valid, `"threadId":`, `"resultItemId":"item-result","threadId":`, 1), 400},
		{"raw", strings.Replace(valid, `"threadId":`, `"body":"PRIVATE_HISTORY_BODY","threadId":`, 1), 400},
		{"alias_thread", strings.Replace(valid, "thread-a", "thread..a", 1), 400},
		{"case_alias_thread", strings.Replace(valid, `"threadId"`, `"ThreadID"`, 1), 400},
		{"case_alias_turn", strings.Replace(valid, `"turnId"`, `"TurnID"`, 1), 400},
		{"mixed_case_thread", strings.Replace(valid, `"threadId":`, `"ThreadID":"thread-other","threadId":`, 1), 400},
		{"mixed_case_kind", strings.Replace(valid, `"kind":`, `"Kind":"history","kind":`, 1), 400},
		{"trailing_json", valid + `{}`, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, ToolResultLocalDisplayPathV1, strings.NewReader(test.body))
			w := httptest.NewRecorder()
			ToolResultLocalDisplayHandlerV1{}.ServeHTTP(w, r)
			if w.Code != test.status || strings.Contains(w.Body.String(), "PRIVATE_HISTORY_BODY") || strings.Contains(w.Body.String(), "orderedItems") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("closed history request released data or changed failure", w.Code)
			}
		})
	}
}
