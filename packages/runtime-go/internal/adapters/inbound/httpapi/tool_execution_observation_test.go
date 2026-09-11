package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainsideeffectidentity "analytix.local/runtime-go/internal/domain/sideeffectidentity"
)

func TestToolExecutionObservationHandlersV1ReturnClosedMetadataOnly(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		check func(input pendingworkapp.SuccessfulToolExecutionObservationInputV1)
	}{
		{
			name: "bash",
			body: `{"threadId":"thread-1","turnId":"turn-1","toolName":"bash","workspace":"/workspace/repo","arguments":{"command":"npm test -- acceptance.test.ts","timeout":120}}`,
			check: func(input pendingworkapp.SuccessfulToolExecutionObservationInputV1) {
				if domainsideeffectidentity.ValidateV1(input.SemanticIdentity) != nil || input.SemanticIdentity.ToolName != "bash" || input.ReadPathResolved {
					t.Fatalf("bash semantic identity was not host-derived: %#v", input)
				}
			},
		},
		{
			name: "read",
			body: `{"threadId":"thread-1","turnId":"turn-1","toolName":"read","workspace":"/workspace/repo","arguments":{"path":"src/deeplaw/knowledge_store.py","offset":1,"limit":200}}`,
			check: func(input pendingworkapp.SuccessfulToolExecutionObservationInputV1) {
				if input.SemanticIdentity != (domainsideeffectidentity.IdentityV1{}) || !input.ReadPathResolved {
					t.Fatalf("read expectation was not host-resolved: %#v", input)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &toolExecutionObservationServiceStubV1{check: test.check}
			handlers := observationHandlersForTestV1(stub)
			recorder := httptest.NewRecorder()
			handlers.Handle(recorder, httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, strings.NewReader(test.body)))
			if recorder.Code != http.StatusOK || stub.calls != 1 || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("observation response failed: status=%d calls=%d headers=%v body=%s", recorder.Code, stub.calls, recorder.Header(), recorder.Body.String())
			}
			for _, forbidden := range []string{"npm test", "/workspace/repo", "knowledge_store.py", "command", "arguments", "output"} {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("observation response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}
}

func TestToolExecutionObservationHandlersV1RejectAmbiguousProtectedAndUnresolvedRequests(t *testing.T) {
	valid := `{"threadId":"thread-1","turnId":"turn-1","toolName":"bash","workspace":"/workspace/repo","arguments":{"command":"private-command"}}`
	cases := map[string]string{
		"duplicate field":     strings.Replace(valid, `"threadId":"thread-1"`, `"threadId":"thread-1","threadId":"thread-1"`, 1),
		"unknown field":       strings.TrimSuffix(valid, "}") + `,"receiptId":"forged"}`,
		"protected tool":      strings.Replace(valid, `"toolName":"bash"`, `"toolName":"mcp__analytix_funds__account_flow"`, 1),
		"background bash":     strings.Replace(valid, `"command":"private-command"`, `"command":"private-command","run_in_background":true`, 1),
		"malformed arguments": strings.Replace(valid, `{"command":"private-command"}`, `[]`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &toolExecutionObservationServiceStubV1{}
			recorder := httptest.NewRecorder()
			observationHandlersForTestV1(stub).Handle(
				recorder,
				httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, strings.NewReader(body)),
			)
			if recorder.Code != http.StatusBadRequest || stub.calls != 0 || strings.Contains(recorder.Body.String(), "private-command") {
				t.Fatalf("invalid request reached observation service: status=%d calls=%d body=%s", recorder.Code, stub.calls, recorder.Body.String())
			}
		})
	}

	stub := &toolExecutionObservationServiceStubV1{}
	handlers := observationHandlersForTestV1(stub)
	handlers.ResolveReadPath = func(string, string) (string, bool) { return "", false }
	recorder := httptest.NewRecorder()
	handlers.Handle(recorder, httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, strings.NewReader(
		`{"threadId":"thread-1","turnId":"turn-1","toolName":"read","workspace":"/workspace/repo","arguments":{"path":"docs/acceptance-context.txt"}}`,
	)))
	if recorder.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("unresolved read path reached observation service: status=%d calls=%d", recorder.Code, stub.calls)
	}

	stub = &toolExecutionObservationServiceStubV1{}
	handlers = observationHandlersForTestV1(stub)
	handlers.ResolveReadPath = func(string, string) (string, bool) { return "/outside/README.md", true }
	recorder = httptest.NewRecorder()
	handlers.Handle(recorder, httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, strings.NewReader(
		`{"threadId":"thread-1","turnId":"turn-1","toolName":"read","workspace":"/workspace/repo","arguments":{"path":"../README.md"}}`,
	)))
	if recorder.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("non-canonical path crossed a permissive resolver: status=%d calls=%d", recorder.Code, stub.calls)
	}

	stub = &toolExecutionObservationServiceStubV1{}
	recorder = httptest.NewRecorder()
	observationHandlersForTestV1(stub).Handle(recorder, httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, strings.NewReader(
		`{"threadId":"thread-1","turnId":"turn-1","toolName":"read","workspace":"/workspace/repo","arguments":{"path":"../README.md"}}`,
	)))
	if recorder.Code != http.StatusBadRequest || stub.calls != 0 {
		t.Fatalf("unsafe read path reached observation service: status=%d calls=%d", recorder.Code, stub.calls)
	}
}

func TestToolExecutionObservationHandlersV1ProjectServiceFailures(t *testing.T) {
	request := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, toolExecutionObservationPathV1, strings.NewReader(
			`{"threadId":"thread-1","turnId":"turn-1","toolName":"bash","workspace":"/workspace/repo","arguments":{"command":"private-command"}}`,
		))
	}
	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"not observed", pendingworkapp.ErrToolExecutionNotObserved, http.StatusNotFound, "not_found"},
		{"ambiguous", pendingworkapp.ErrToolExecutionObservationAmbiguous, http.StatusServiceUnavailable, "internal_error"},
		{"authority", errors.New("forged receipt contains private-command"), http.StatusServiceUnavailable, "internal_error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &toolExecutionObservationServiceStubV1{err: test.err}
			recorder := httptest.NewRecorder()
			observationHandlersForTestV1(stub).Handle(recorder, request())
			if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) || strings.Contains(recorder.Body.String(), "private-command") {
				t.Fatalf("service failure projection drifted: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

type toolExecutionObservationServiceStubV1 struct {
	calls int
	err   error
	check func(pendingworkapp.SuccessfulToolExecutionObservationInputV1)
}

func (stub *toolExecutionObservationServiceStubV1) ObserveSuccessfulToolExecutionV1(
	_ context.Context,
	input pendingworkapp.SuccessfulToolExecutionObservationInputV1,
) (domainpendingwork.SuccessfulToolExecutionObservationV1, error) {
	stub.calls++
	if stub.check != nil {
		stub.check(input)
	}
	if stub.err != nil {
		return domainpendingwork.SuccessfulToolExecutionObservationV1{}, stub.err
	}
	return validToolExecutionObservationV1(input.ToolName), nil
}

func observationHandlersForTestV1(service SuccessfulToolExecutionObserverV1) ToolExecutionObservationHandlersV1 {
	return ToolExecutionObservationHandlersV1{
		Service: service,
		ResolveMutationPath: func(workspace string, requested string) (string, bool) {
			if workspace == "/workspace/repo" && requested == "." {
				return workspace, true
			}
			return "", false
		},
		ResolveReadPath: func(workspace string, requested string) (string, bool) {
			if workspace == "/workspace/repo" && (requested == "docs/acceptance-context.txt" ||
				requested == "src/deeplaw/knowledge_store.py") {
				return workspace + "/" + requested, true
			}
			return "", false
		},
	}
}

func validToolExecutionObservationV1(toolName string) domainpendingwork.SuccessfulToolExecutionObservationV1 {
	return domainpendingwork.SuccessfulToolExecutionObservationV1{
		SchemaVersion: domainpendingwork.SuccessfulToolExecutionObservationSchemaVersionV1,
		Disclosure:    domainpendingwork.ToolExecutionObservationDisclosureV1, PrivatePayloadWithheld: true,
		ThreadID: "thread-1", TurnID: "turn-1", ToolName: toolName, Status: domainpendingwork.StatusCompleted,
		WorkID: domainsecurity.SHA256Hex([]byte("work")), ReceiptID: domainsecurity.SHA256Hex([]byte("receipt")),
		DispositionID: domainsecurity.SHA256Hex([]byte("disposition")), ExecutionGrantID: domainsecurity.SHA256Hex([]byte("grant")),
		ResultItemID: "item_result_" + strings.Repeat("a", 64), ResultItemDigest: domainsecurity.SHA256Hex([]byte("result")),
	}
}

func TestValidToolExecutionObservationV1TestFixture(t *testing.T) {
	for _, toolName := range []string{"bash", "read"} {
		observation := validToolExecutionObservationV1(toolName)
		if err := domainpendingwork.ValidateSuccessfulToolExecutionObservationV1(observation); err != nil {
			t.Fatalf("invalid test observation %s: %v", toolName, err)
		}
		if _, err := json.Marshal(observation); err != nil {
			t.Fatal(err)
		}
	}
}
