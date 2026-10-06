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
