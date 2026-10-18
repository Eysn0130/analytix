package runtimeapp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
)

func TestProductionHostChildInputRouteRequiresTypedLocalHostV1(t *testing.T) {
	h, err := NewRuntimeServerHandlerE(Config{RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdownOwnedRuntimeHandler(t, h)
	valid := `{"mode":"inspect","threadId":"thread_1","jobId":"job-1"}`
	for _, test := range []struct {
		name, body, token string
		headers           []string
		want              int
	}{
		{name: "missing token", body: valid, headers: []string{httpapi.LocalDisplayHeaderValueV1}, want: 401},
		{name: "body is not Host authority", body: valid, token: DefaultRuntimeToken, want: 403},
		{name: "duplicate Host header", body: valid, token: DefaultRuntimeToken, headers: []string{httpapi.LocalDisplayHeaderValueV1, httpapi.LocalDisplayHeaderValueV1}, want: 403},
		{name: "authorized unknown scope", body: valid, token: DefaultRuntimeToken, headers: []string{httpapi.LocalDisplayHeaderValueV1}, want: 503},
		{name: "forged authority body", body: `{"mode":"inspect","threadId":"thread_1","jobId":"job-1","authority":"forged"}`, token: DefaultRuntimeToken, headers: []string{httpapi.LocalDisplayHeaderValueV1}, want: 400},
		{name: "full 4000 rune intent", body: `{"mode":"start_or_steer","threadId":"thread_1","jobId":"job-1","expectedRunId":"job-1","expectedChildTurnId":"turn_1","message":"` + strings.Repeat("界", 4000) + `","clientMessageId":"11111111-1111-4111-8111-111111111111"}`, token: DefaultRuntimeToken, headers: []string{httpapi.LocalDisplayHeaderValueV1}, want: 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/steer", bytes.NewBufferString(test.body))
			if test.token != "" {
				r.Header.Set("Authorization", "Bearer "+test.token)
			}
			for _, value := range test.headers {
				r.Header.Add(httpapi.LocalDisplayHeaderV1, value)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.want {
				t.Fatalf("status=%d want=%d", w.Code, test.want)
			}
			if w.Code == 403 || w.Code == 503 {
				var value any
				if json.Unmarshal(w.Body.Bytes(), &value) != nil {
					t.Fatal("invalid response")
				}
				response, err := subagentapp.DecodeChildInputResponseV1(value)
				if err != nil || response.Receipt != nil || response.Capabilities.Send != "disabled" || response.Capabilities.Stop || response.Capabilities.RunID != "" || response.Capabilities.ChildThreadID != "" || response.Capabilities.ChildTurnID != "" {
					t.Fatal("production route invented IDs or authority", err)
				}
			}
		})
	}
}
