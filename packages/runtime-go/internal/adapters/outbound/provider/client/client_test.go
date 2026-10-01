package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
)

func TestStreamIdleTimeoutZeroDisablesWatchdog(t *testing.T) {
	client := NewHTTPProviderClient(nil)
	client.StreamIdleTimeout = 0
	if got := client.effectiveStreamIdleTimeout(); got != 0 {
		t.Fatalf("effective timeout = %s, want disabled", got)
	}
	body := newStreamIdleWatchdogBody(context.Background(), io.NopCloser(strings.NewReader("ok")), 0)
	defer body.Close()
	time.Sleep(10 * time.Millisecond)
	if body.Stalled() || body.Timeout() != 0 {
		t.Fatalf("zero timeout should not start watchdog: stalled=%v timeout=%s", body.Stalled(), body.Timeout())
	}
}

func TestProviderPipelineDetailsBindLogicalOuterAndPhysicalAttempts(t *testing.T) {
	request := domainmodel.Request{PrivateProviderTelemetry: &domainmodel.ProviderTelemetryBindingV1{
		LogicalSequence:  7,
		OuterAttempt:     3,
		LaneCallSequence: 2,
	}}
	details := providerRequestPipelineDetails(request, "https://provider.invalid", nil, nil, 4, nil)
	if details["logicalSequence"] != float64(7) || details["outerAttempt"] != float64(3) ||
		details["laneCallSequence"] != float64(2) || details["physicalAttempt"] != float64(4) ||
		details["attempt"] != float64(4) {
		t.Fatalf("provider pipeline correlation changed: %#v", details)
	}
}

func TestHTTPProviderClientFailureCarriesExactDispatchState(t *testing.T) {
	tests := []struct {
		name    string
		client  *HTTPProviderClient
		request domainmodel.Request
		want    domaincache.ProviderDispatchStateV1
	}{
		{
			name:   "request rejected before transport",
			client: NewHTTPProviderClient(nil),
			request: domainmodel.Request{
				ProviderID: "provider", Family: "openai", EndpointFormat: "chat_completions",
				BaseURL: "://invalid", Model: "model", Messages: []domainmodel.Message{{Role: "user", Content: "hello"}},
			},
			want: domaincache.ProviderDispatchStateNotSent,
		},
		{
			name: "transport outcome indeterminate",
			client: NewHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("transport failed")
			})}),
			request: domainmodel.Request{
				ProviderID: "provider", Family: "openai", EndpointFormat: "chat_completions",
				BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "model", Messages: []domainmodel.Message{{Role: "user", Content: "hello"}},
			},
			want: domaincache.ProviderDispatchStateIndeterminate,
		},
		{
			name: "response proves transport sent",
			client: NewHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
				}, nil
			})}),
			request: domainmodel.Request{
				ProviderID: "provider", Family: "openai", EndpointFormat: "chat_completions",
				BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "model", Messages: []domainmodel.Message{{Role: "user", Content: "hello"}},
			},
			want: domaincache.ProviderDispatchStateSent,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.client.Stream(context.Background(), test.request)
			var disposition interface {
				ProviderDispatchStateV1() domaincache.ProviderDispatchStateV1
			}
			if err == nil || !errors.As(err, &disposition) || disposition.ProviderDispatchStateV1() != test.want {
				t.Fatalf("provider failure dispatch state = %v, want %s (err=%v)", disposition, test.want, err)
			}
		})
	}
}

func TestProviderFailureStageIsClosedAndCarriesSafeDispatchContext(t *testing.T) {
	for _, test := range []struct {
		name  string
		stage ProviderFailureStageV1
	}{
		{name: "request validation", stage: ProviderFailureStageRequestValidation},
		{name: "request build", stage: ProviderFailureStageRequestBuild},
		{name: "pre-send body audit", stage: ProviderFailureStagePreSendBodyAudit},
		{name: "telemetry begin", stage: ProviderFailureStageTelemetryBegin},
		{name: "transport before observed send", stage: ProviderFailureStageTransportBeforeObserved},
		{name: "transport after observed send", stage: ProviderFailureStageTransportAfterObserved},
		{name: "local admission config", stage: ProviderFailureStageLocalAdmissionConfig},
		{name: "callback projection", stage: ProviderFailureStageCallbackProjection},
		{name: "unclassified", stage: ProviderFailureStageUnclassified},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := withProviderFailureContextV1(
				errors.New("provider body SECRET_PROVIDER_BODY_7F3C"),
				domaincache.ProviderDispatchStateSent, test.stage, 4,
			)
			var diagnostic interface{ Diagnostics() map[string]any }
			if !errors.As(err, &diagnostic) {
				t.Fatalf("provider failure did not expose bounded diagnostics: %T %v", err, err)
			}
			got := diagnostic.Diagnostics()
			if got["failureStage"] != string(test.stage) || got["dispatchState"] != "sent" || got["attempt"] != float64(4) {
				t.Fatalf("provider failure context mismatch: %#v", got)
			}
			serialized, marshalErr := json.Marshal(got)
			if marshalErr != nil || strings.Contains(string(serialized), "SECRET_PROVIDER_BODY_7F3C") {
				t.Fatalf("provider failure context leaked unsafe material: %s err=%v", serialized, marshalErr)
			}
		})
	}
	unknown := withProviderFailureContextV1(
		errors.New("unclassified"), domaincache.ProviderDispatchStateNotSent, ProviderFailureStageV1("not-a-stage"), 1,
	)
	var diagnostic interface{ Diagnostics() map[string]any }
	if !errors.As(unknown, &diagnostic) || diagnostic.Diagnostics()["failureStage"] != string(ProviderFailureStageUnclassified) {
		t.Fatalf("unknown provider failure stage did not fail closed: %#v", diagnostic)
	}
}

func TestHTTPProviderClientProjectsFailureStageAtObservedBoundaries(t *testing.T) {
	request := func() domainmodel.Request {
		return domainmodel.Request{
			ProviderID: "provider", Family: "openai", EndpointFormat: "chat_completions",
			BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "model",
			Messages: []domainmodel.Message{{Role: "user", Content: "private prompt sentinel"}},
		}
	}
	assertDiagnostic := func(t *testing.T, err error, stage ProviderFailureStageV1, dispatch string, attempt float64) {
		t.Helper()
		if err == nil {
			t.Fatal("expected provider failure")
		}
		var diagnostic interface{ Diagnostics() map[string]any }
		if !errors.As(err, &diagnostic) {
			t.Fatalf("provider failure lacked diagnostics: %T %v", err, err)
		}
		got := diagnostic.Diagnostics()
		if got["failureStage"] != string(stage) || got["dispatchState"] != dispatch ||
			(attempt > 0 && got["attempt"] != attempt) || (attempt == 0 && got["attempt"] != nil) {
			t.Fatalf("provider failure boundary mismatch: got=%#v want stage=%s dispatch=%s attempt=%v", got, stage, dispatch, attempt)
		}
	}

	t.Run("request validation", func(t *testing.T) {
		req := request()
		req.ReasoningEffort = "invalid"
		_, err := NewHTTPProviderClient(nil).Stream(context.Background(), req)
		assertDiagnostic(t, err, ProviderFailureStageRequestValidation, "not_sent", 0)
	})
	t.Run("request build", func(t *testing.T) {
		req := request()
		req.BaseURL = "://invalid"
		_, err := NewHTTPProviderClient(nil).Stream(context.Background(), req)
		assertDiagnostic(t, err, ProviderFailureStageRequestBuild, "not_sent", 1)
	})
	t.Run("body audit", func(t *testing.T) {
		client := NewHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport must not run")
		})})
		client.ProviderBodyAuditor = &recordingProviderBodyAuditorV1{err: errors.New("audit SECRET_PROVIDER_BODY_7F3C")}
		_, err := client.Stream(context.Background(), request())
		assertDiagnostic(t, err, ProviderFailureStagePreSendBodyAudit, "not_sent", 1)
	})
	t.Run("transport outcome unclassified before any response", func(t *testing.T) {
		client := NewHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport failed")
		})})
		client.MaxStreamReconnects = 0
		_, err := client.Stream(context.Background(), request())
		assertDiagnostic(t, err, ProviderFailureStageUnclassified, "indeterminate", 1)
	})
	t.Run("transport after observed send", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()
		req := request()
		req.BaseURL = server.URL
		_, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), req)
		assertDiagnostic(t, err, ProviderFailureStageTransportAfterObserved, "sent", 1)
	})
	t.Run("callback projection", func(t *testing.T) {
		client := NewHTTPProviderClient(nil)
		client.MaxStreamReconnects = 0
		req := request()
		req.OnPipelineStage = func(stage domainmodel.PipelineStage) error {
			if stage.Stage == "pre_send" {
				return errors.New("callback SECRET_PROVIDER_BODY_7F3C")
			}
			return nil
		}
		_, err := client.Stream(context.Background(), req)
		assertDiagnostic(t, err, ProviderFailureStageCallbackProjection, "not_sent", 1)
	})
}

type recordingProviderBodyAuditorV1 struct {
	mu       sync.Mutex
	attempts []int
	bodies   [][]byte
	err      error
}

func (auditor *recordingProviderBodyAuditorV1) AuditProviderRequestBody(
	_ context.Context,
	providerFamily string,
	attempt int,
	body []byte,
) error {
	if providerFamily != "analytix-hub" {
		return errors.New("provider family changed")
	}
	auditor.mu.Lock()
	defer auditor.mu.Unlock()
	auditor.attempts = append(auditor.attempts, attempt)
	auditor.bodies = append(auditor.bodies, append([]byte(nil), body...))
	return auditor.err
}

func TestHTTPProviderClientAuditRejectsBeforeTransport(t *testing.T) {
	transportCalled := false
	client := NewHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		transportCalled = true
		return nil, errors.New("transport must not run")
	})})
	auditor := &recordingProviderBodyAuditorV1{err: errors.New("audit rejected")}
	client.ProviderBodyAuditor = auditor
	_, err := client.Stream(context.Background(), domainmodel.Request{
		ProviderID: "analytix-hub", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "model",
		Messages: []domainmodel.Message{{Role: "user", Content: "audit sentinel"}},
	})
	var disposition interface {
		ProviderDispatchStateV1() domaincache.ProviderDispatchStateV1
	}
	if err == nil || !errors.As(err, &disposition) ||
		disposition.ProviderDispatchStateV1() != domaincache.ProviderDispatchStateNotSent || transportCalled {
		t.Fatalf("audit rejection did not fail before transport: err=%v called=%v", err, transportCalled)
	}
	if len(auditor.attempts) != 1 || auditor.attempts[0] != 1 ||
		len(auditor.bodies) != 1 || !strings.Contains(string(auditor.bodies[0]), "audit sentinel") {
		t.Fatalf("auditor did not receive exact final request body: %#v", auditor)
	}
}

func TestHTTPProviderClientAuditsEveryPhysicalReconnect(t *testing.T) {
	transportCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		transportCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		if transportCalls == 1 {
			_, _ = w.Write([]byte("data: "))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if hijacker, ok := w.(http.Hijacker); ok {
				connection, _, _ := hijacker.Hijack()
				_ = connection.Close()
			}
			return
		}
		_, _ = w.Write([]byte(
			"data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\n" +
				"data: [DONE]\n\n",
		))
	}))
	defer server.Close()
	client := NewHTTPProviderClient(server.Client())
	auditor := &recordingProviderBodyAuditorV1{}
	client.ProviderBodyAuditor = auditor
	result, err := client.Stream(context.Background(), domainmodel.Request{
		ProviderID: "analytix-hub", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: server.URL, APIKey: "test-key", Model: "model",
		Messages: []domainmodel.Message{{Role: "user", Content: "retry audit sentinel"}},
	})
	if err != nil || transportCalls != 2 || len(auditor.attempts) != 2 ||
		!reflect.DeepEqual(auditor.attempts, []int{1, 2}) || len(result.Chunks) == 0 {
		t.Fatalf("physical reconnect audit mismatch: calls=%d attempts=%v result=%#v err=%v",
			transportCalls, auditor.attempts, result, err)
	}
	if !reflect.DeepEqual(auditor.bodies[0], auditor.bodies[1]) {
		t.Fatal("physical reconnect audit body changed between attempts")
	}
}

func TestProviderTelemetryContextTerminalPrecedesStreamResult(t *testing.T) {
	tests := []struct {
		name       string
		streamErr  error
		contextErr error
		want       domaincache.ProviderCallStatusV1
	}{
		{name: "host deadline beats nil stream error", contextErr: context.DeadlineExceeded, want: domaincache.ProviderCallStatusTimedOut},
		{name: "host deadline beats stream cancellation", streamErr: context.Canceled, contextErr: context.DeadlineExceeded, want: domaincache.ProviderCallStatusTimedOut},
		{name: "host cancellation beats nil stream error", contextErr: context.Canceled, want: domaincache.ProviderCallStatusCancelled},
		{name: "host cancellation beats stream deadline", streamErr: context.DeadlineExceeded, contextErr: context.Canceled, want: domaincache.ProviderCallStatusCancelled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cacheProviderCallStatus(test.streamErr, test.contextErr, true); got != test.want {
				t.Fatalf("provider call status mismatch: got=%q want=%q", got, test.want)
			}
		})
	}
}

func TestHTTPProviderClientRejectsSuccessReturnedAfterHostCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	public := ""
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"choices\":[{\"delta\":{\"content\":\"CANCELLED_HTTP_PROVIDER_SENTINEL\"}}]}\n\n" +
					"data: [DONE]\n\n",
			)),
		}, nil
	})}
	result, err := NewHTTPProviderClient(httpClient).Stream(ctx, domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: "https://provider.invalid", APIKey: "test-key", Model: "deepseek-chat", Messages: []domainmodel.Message{{Role: "user", Content: "hello"}},
		OnChunk: func(chunk domainmodel.Chunk) error {
			public += chunk.Text
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) || public != "" || len(result.Chunks) != 0 {
		t.Fatalf("provider success bypassed host cancellation: result=%#v err=%v", result, err)
	}
	if len(result.CacheObservations) != 1 || result.CacheObservations[0].Status != domaincache.ProviderCallStatusCancelled {
		t.Fatalf("host cancellation telemetry was not settled deterministically: %#v", result.CacheObservations)
	}
}

func TestHTTPProviderClientReturnsSanitizedProviderError(t *testing.T) {
	const secret = "sk-test-provider-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Authentication Fails, api key ` + secret + ` is invalid"}}`))
	}))
	defer server.Close()

	_, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         secret,
		Model:          "deepseek-chat",
		Messages:       []domainmodel.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected provider auth error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if providerErr.Kind != "auth" || providerErr.Status != http.StatusUnauthorized || !providerErr.AuthRequired() {
		t.Fatalf("unexpected provider auth error: %#v", providerErr)
	}
	raw, _ := json.Marshal(providerErr.Diagnostics())
	if strings.Contains(err.Error(), secret) || strings.Contains(string(raw), secret) {
		t.Fatalf("provider diagnostics leaked API key: err=%q diagnostics=%s", err.Error(), string(raw))
	}
}

func TestHTTPProviderClientReturns404DiagnosticsWithModelAndRequestURL(t *testing.T) {
	const secret = "sk-test-provider-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model bad-model was not found for api key ` + secret + `"}`))
	}))
	defer server.Close()

	_, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID:     "custom-provider",
		Family:         "anthropic-compatible",
		EndpointFormat: "messages",
		BaseURL:        server.URL,
		APIKey:         secret,
		Model:          "bad-model",
		Messages:       []domainmodel.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected provider 404 error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	if providerErr.Status != http.StatusNotFound || providerErr.Kind != "http" || providerErr.Model != "bad-model" {
		t.Fatalf("unexpected provider error: %#v", providerErr)
	}
	diagnostics := providerErr.Diagnostics()
	if diagnostics["baseUrl"] != server.URL ||
		diagnostics["requestUrl"] != server.URL+"/v1/messages" ||
		diagnostics["providerId"] != "custom-provider" ||
		diagnostics["model"] != "bad-model" ||
		diagnostics["endpointFormat"] != "messages" ||
		diagnostics["status"] != float64(http.StatusNotFound) {
		t.Fatalf("diagnostics missing provider matrix fields: %#v", diagnostics)
	}
	raw, _ := json.Marshal(diagnostics)
	if strings.Contains(err.Error(), secret) || strings.Contains(string(raw), secret) {
		t.Fatalf("provider 404 diagnostics leaked API key: err=%q diagnostics=%s", err.Error(), string(raw))
	}
	if !strings.Contains(providerErr.Message, "check Provider Base URL and Endpoint format") || strings.Contains(providerErr.Message, secret) || strings.Contains(providerErr.Message, "bad-model was not found") {
		t.Fatalf("provider response body must be withheld while preserving safe guidance: %q", providerErr.Message)
	}
}

func TestProviderErrorBodyReasoningNeverLeavesBoundary(t *testing.T) {
	const sentinel = "PRIVATE_PROVIDER_REASONING_SENTINEL"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"reasoning_content":"` + sentinel + `","message":"<think>` + sentinel + `</think>"}}`))
	}))
	defer server.Close()

	_, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID:     "untrusted-provider",
		Family:         "openai-compatible",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "test-key",
		Model:          "test-model",
		Messages:       []domainmodel.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected *ProviderError, got %T: %v", err, err)
	}
	diagnostics, _ := json.Marshal(providerErr.Diagnostics())
	for _, output := range []string{err.Error(), providerErr.Message, string(diagnostics)} {
		if strings.Contains(output, sentinel) || strings.Contains(strings.ToLower(output), "reasoning_content") || strings.Contains(strings.ToLower(output), "<think") {
			t.Fatalf("provider-originated private reasoning escaped error boundary: %q", output)
		}
	}
}

func TestHTTPProviderClientReplaysPreOutputStreamCut(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		current := requests
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if current == 1 {
			_, _ = w.Write([]byte("data: "))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, _ := hijacker.Hijack()
				_ = conn.Close()
			}
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	var retryChunks []domainmodel.Chunk
	beforeSendAttempts := []int{}
	result, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-test-replay",
		Model:          "deepseek-chat",
		Messages:       []domainmodel.Message{{Role: "user", Content: "hello"}},
		BeforeSend: func(attempt int) error {
			beforeSendAttempts = append(beforeSendAttempts, attempt)
			return nil
		},
		OnChunk: func(chunk domainmodel.Chunk) error {
			if chunk.Kind == domainmodel.ChunkRetrying {
				retryChunks = append(retryChunks, chunk)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) == 0 || !hasChunkKind(result.Chunks, domainmodel.ChunkDone) {
		t.Fatalf("expected completed replayed stream: %#v", result.Chunks)
	}
	if len(retryChunks) != 1 || retryChunks[0].RetryAttempt != 2 {
		t.Fatalf("expected one retrying chunk before output: %#v", retryChunks)
	}
	mu.Lock()
	gotRequests := requests
	mu.Unlock()
	if gotRequests != 2 {
		t.Fatalf("expected stream replay once, got %d requests", gotRequests)
	}
	if !reflect.DeepEqual(beforeSendAttempts, []int{1, 2}) {
		t.Fatalf("before-send authority was not rechecked for each physical request: %#v", beforeSendAttempts)
	}
	if len(result.CacheObservations) != 2 {
		t.Fatalf("every physical provider attempt must settle exactly once: %#v", result.CacheObservations)
	}
	first, second := result.CacheObservations[0], result.CacheObservations[1]
	if first.Status != domaincache.ProviderCallStatusFailed || second.Status != domaincache.ProviderCallStatusSucceeded ||
		first.Shape.Attempt != 1 || second.Shape.Attempt != 2 ||
		first.Shape.LogicalCallHMAC != second.Shape.LogicalCallHMAC || first.Shape.WireBodyHMAC != second.Shape.WireBodyHMAC ||
		first.Shape.DigestEpoch == 0 || first.Shape.DigestEpoch != second.Shape.DigestEpoch {
		t.Fatalf("provider attempt telemetry lost ordered wire identity or terminal status: %#v", result.CacheObservations)
	}
	telemetryJSON, err := json.Marshal(result.CacheObservations)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{server.URL, "sk-test-replay", "deepseek-chat", "hello"} {
		if strings.Contains(string(telemetryJSON), forbidden) {
			t.Fatalf("safe provider attempt telemetry leaked %q: %s", forbidden, telemetryJSON)
		}
	}
}

func TestHTTPProviderClientRejectsRedirectBeforeSecondPhysicalSend(t *testing.T) {
	formats := []struct {
		name     string
		family   string
		format   string
		path     string
		provider string
	}{
		{name: "deepseek_chat", family: "deepseek", format: "chat_completions", provider: "deepseek"},
		{name: "openai_chat", family: "openai", format: "chat_completions", provider: "openai"},
		{name: "openai_responses", family: "openai", format: "responses", provider: "openai"},
		{name: "anthropic_messages", family: "anthropic", format: "messages", provider: "anthropic"},
		{name: "custom_chat", family: "custom", format: "custom_endpoint", path: "/custom/chat/completions", provider: "custom"},
		{name: "custom_responses", family: "custom", format: "custom_endpoint", path: "/custom/responses", provider: "custom"},
		{name: "custom_messages", family: "custom", format: "custom_endpoint", path: "/custom/messages", provider: "custom"},
	}
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, format := range formats {
			t.Run(format.name+"_"+http.StatusText(status), func(t *testing.T) {
				var mu sync.Mutex
				sourceRequests, targetRequests := 0, 0
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					targetRequests++
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
				}))
				defer target.Close()
				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					sourceRequests++
					mu.Unlock()
					w.Header().Set("Location", target.URL+"/exfiltrate")
					w.WriteHeader(status)
				}))
				defer source.Close()

				baseURL := source.URL
				if format.path != "" {
					baseURL += format.path
				}
				beforeSend := []int{}
				providerClient := NewHTTPProviderClient(source.Client())
				providerClient.MaxStreamReconnects = 0
				_, err := providerClient.Stream(context.Background(), domainmodel.Request{
					ProviderID: format.provider, Family: format.family, EndpointFormat: format.format,
					BaseURL: baseURL, APIKey: "redirect-secret", Model: "redirect-model",
					Messages: []domainmodel.Message{{Role: "user", Content: "private bank account 6222020000000000000"}},
					BeforeSend: func(attempt int) error {
						beforeSend = append(beforeSend, attempt)
						return nil
					},
				})
				if err == nil {
					t.Fatal("provider redirect was accepted")
				}
				var providerErr *ProviderError
				if !errors.As(err, &providerErr) || providerErr.Status != status {
					t.Fatalf("redirect did not fail as the original response: %T %v", err, err)
				}
				mu.Lock()
				gotSource, gotTarget := sourceRequests, targetRequests
				mu.Unlock()
				if gotSource != 1 || gotTarget != 0 {
					t.Fatalf("redirect replayed a provider body: source=%d target=%d", gotSource, gotTarget)
				}
				if !reflect.DeepEqual(beforeSend, []int{1}) {
					t.Fatalf("redirect changed physical-send authority accounting: %#v", beforeSend)
				}
			})
		}
	}
}

func TestHTTPProviderClientUsesAttemptBoundProxyWithoutPublicCredentialEcho(t *testing.T) {
	const credential = "synthetic-attempt-proxy-credential"
	var mu sync.Mutex
	proxyRequests := 0
	providerAuthorizationObserved := false
	proxyAuthorizationObserved := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		mu.Lock()
		proxyRequests++
		providerAuthorizationObserved = request.Header.Get("Authorization") != ""
		proxyAuthorizationObserved = request.Header.Get("Proxy-Authorization") != ""
		mu.Unlock()
		if request.URL.Host != "provider-unreachable.invalid" {
			t.Errorf("proxy target host = %q", request.URL.Host)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"current proxy result\"},\"finish_reason\":\"stop\"}]}\n\n"+
			"data: [DONE]\n\n")
	}))
	defer proxy.Close()

	request := domainmodel.Request{
		ProviderID: "openai", Family: "openai", EndpointFormat: "chat_completions",
		BaseURL: "http://provider-unreachable.invalid/v1", APIKey: credential, Model: "proxy-model",
		Messages:                      []domainmodel.Message{{Role: "user", Content: "bounded proxy request"}},
		PrivateProviderProxyAuthority: true,
	}
	proxyField := reflect.ValueOf(&request).Elem().FieldByName("ProxyURL")
	if !proxyField.IsValid() || proxyField.Kind() != reflect.String || !proxyField.CanSet() {
		t.Fatal("Provider request lacks an attempt-bound ProxyURL authority")
	}
	proxyField.SetString(proxy.URL)

	directClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("direct provider target is unreachable")
	})}
	result, err := NewHTTPProviderClient(directClient).Stream(context.Background(), request)
	if err != nil {
		t.Fatalf("attempt-bound proxy request error = %v", err)
	}
	mu.Lock()
	gotProxyRequests := proxyRequests
	gotProviderAuthorization := providerAuthorizationObserved
	gotProxyAuthorization := proxyAuthorizationObserved
	mu.Unlock()
	if gotProxyRequests != 1 || !gotProviderAuthorization || gotProxyAuthorization {
		t.Fatalf("proxy observations requests=%d providerAuthorization=%t proxyAuthorization=%t",
			gotProxyRequests, gotProviderAuthorization, gotProxyAuthorization)
	}
	encoded, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(encoded), credential) {
		t.Fatal("Provider credential crossed the result boundary")
	}
}

func TestHTTPProviderClientReusesOnlyCurrentProxyAuthorityConnection(t *testing.T) {
	var mu sync.Mutex
	newProxy := func(observed *[]string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			mu.Lock()
			*observed = append(*observed, request.RemoteAddr)
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"+
				"data: [DONE]\n\n")
		}))
	}
	var firstConnections, secondConnections []string
	first := newProxy(&firstConnections)
	defer first.Close()
	second := newProxy(&secondConnections)
	defer second.Close()
	client := NewHTTPProviderClient(nil)
	defer client.Close()
	var observations []map[string]any
	request := domainmodel.Request{
		ProviderID: "proxy-authority", Family: "openai", EndpointFormat: "chat_completions",
		BaseURL: "http://provider-unreachable.invalid/v1", APIKey: "synthetic-key", Model: "test",
		Messages:                      []domainmodel.Message{{Role: "user", Content: "hello"}},
		PrivateProviderProxyAuthority: true,
		OnPipelineStage: func(stage domainmodel.PipelineStage) error {
			if stage.Stage == "post_send" {
				observations = append(observations, stage.Details)
			}
			return nil
		},
	}
	for _, proxyURL := range []string{first.URL, first.URL, second.URL, first.URL} {
		request.ProxyURL = proxyURL
		if _, err := client.Stream(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	for index, observation := range observations {
		if observation["provider_connection_reused"] != (index == 1) {
			t.Fatalf("attempt %d connection observation does not match socket reuse", index)
		}
		for _, key := range []string{"provider_connection_acquired_ms", "provider_request_written_ms", "provider_first_response_byte_ms"} {
			if value, ok := observation[key].(float64); !ok || value < 0 {
				t.Fatalf("attempt %d lacks non-negative %s", index, key)
			}
		}
		if _, observed := observation["provider_tls_done_ms"]; observed {
			t.Fatal("HTTP loopback was reported as a TLS handshake")
		}
	}
	if len(observations) != 4 {
		t.Fatalf("observed %d attempts, want 4", len(observations))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(firstConnections) != 3 || firstConnections[0] != firstConnections[1] ||
		firstConnections[1] == firstConnections[2] || len(secondConnections) != 1 {
		t.Fatalf("proxy connection reuse or authority rotation is wrong: first=%#v second=%#v",
			firstConnections, secondConnections)
	}
}

func TestHTTPProviderClientTransportLifecycle(t *testing.T) {
	client := NewHTTPProviderClient(nil)
	defer client.Close()
	request := domainmodel.Request{ProviderID: "one", Family: "openai", PrivateProviderProxyAuthority: true}
	const endpoint = "https://provider.invalid/v1/chat/completions"
	first, release, err := client.httpClientForAttempt(request, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	owner := providerTransportOwner{providerID: request.ProviderID, family: request.Family}
	firstSlot := client.transportSlots[owner]
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			current, done, acquireErr := client.httpClientForAttempt(request, endpoint)
			if acquireErr != nil {
				t.Error(acquireErr)
				return
			}
			defer done()
			if current != first {
				t.Error("concurrent requests did not reuse the current transport")
			}
		}()
	}
	workers.Wait()
	invalid := request
	invalid.ProxyURL = "http://user:secret@proxy.invalid"
	if _, _, err := client.httpClientForAttempt(invalid, endpoint); err == nil {
		t.Fatal("invalid proxy authority was admitted")
	}
	if firstSlot.retired || firstSlot.active != 1 {
		t.Fatal("invalid authority or concurrent release retired the current active transport")
	}
	// Credential rotation changes request headers, never the transport identity.
	request.APIKey = "synthetic-rotated-key"
	same, done, err := client.httpClientForAttempt(request, endpoint)
	if err != nil || same != first {
		t.Fatal("credential rotation unexpectedly replaced the transport")
	}
	done()
	replacement, done, err := client.httpClientForAttempt(request, "https://next-provider.invalid/v1/messages")
	if err != nil || replacement == first || !firstSlot.retired || firstSlot.active != 1 {
		t.Fatal("endpoint change did not retire the old authority while retaining its active request")
	}
	done()
	release()
	if firstSlot.active != 0 {
		t.Fatal("retired transport was not released")
	}
	for i := 0; i < maxProviderTransportSlots+2; i++ {
		request.ProviderID = fmt.Sprintf("provider-%d", i)
		_, done, err := client.httpClientForAttempt(request, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		done()
	}
	if len(client.transportSlots) != maxProviderTransportSlots || client.transportSlots[owner] != nil {
		t.Fatal("idle transport pool was not bounded or did not evict the least recently used owner")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	for _, authoritative := range []bool{true, false} {
		request.PrivateProviderProxyAuthority = authoritative
		if _, _, err := client.httpClientForAttempt(request, endpoint); err == nil {
			t.Fatal("closed client resurrected a transport")
		}
	}
}

func TestHTTPProviderClientRetirementDoesNotCancelAnActiveStream(t *testing.T) {
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	finish := func() { once.Do(func() { close(unblock) }) }
	defer finish()
	send := func(w http.ResponseWriter) {
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"+
			"data: [DONE]\n\n")
	}
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		close(entered)
		select {
		case <-unblock:
			send(w)
		case <-r.Context().Done():
		}
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		send(w)
	}))
	defer second.Close()
	client := NewHTTPProviderClient(nil)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request := domainmodel.Request{
		ProviderID: "rotation", Family: "openai", EndpointFormat: "chat_completions",
		BaseURL: "http://provider.invalid/v1", APIKey: "synthetic-key", Model: "test",
		PrivateProviderProxyAuthority: true, ProxyURL: first.URL,
	}
	completed := make(chan error, 1)
	firstRequest := request
	go func() { _, err := client.Stream(ctx, firstRequest); completed <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first stream did not start")
	}
	request.ProxyURL = second.URL
	if _, err := client.Stream(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	finish()
	if err := <-completed; err != nil {
		t.Fatalf("transport retirement cancelled an already authorized active stream: %v", err)
	}
}

func TestHTTPProviderClientTreatsAuthoritativeEmptyProxyAsDirect(t *testing.T) {
	var mu sync.Mutex
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		targetRequests++
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"direct result\"},\"finish_reason\":\"stop\"}]}\n\n"+
			"data: [DONE]\n\n")
	}))
	defer target.Close()

	injectedTransportCalls := 0
	client := NewHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		injectedTransportCalls++
		return nil, errors.New("legacy injected transport must not own Registry execution")
	})})
	_, err := client.Stream(context.Background(), domainmodel.Request{
		ProviderID: "openai", Family: "openai", EndpointFormat: "chat_completions",
		BaseURL: target.URL, APIKey: "synthetic-direct-credential", Model: "proxy-model",
		Messages:                      []domainmodel.Message{{Role: "user", Content: "bounded direct request"}},
		PrivateProviderProxyAuthority: true,
	})
	mu.Lock()
	gotTargetRequests := targetRequests
	mu.Unlock()
	if err != nil || gotTargetRequests != 1 || injectedTransportCalls != 0 {
		t.Fatalf("authoritative direct transport target=%d injected=%d err=%v", gotTargetRequests, injectedTransportCalls, err)
	}
}

func TestHTTPProviderClientRejectsEmbeddedAttemptProxyCredentialsBeforeTransport(t *testing.T) {
	transportCalls := 0
	client := NewHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		transportCalls++
		return nil, errors.New("transport must not be reached")
	})})
	_, err := client.Stream(context.Background(), domainmodel.Request{
		ProviderID: "openai", Family: "openai", EndpointFormat: "chat_completions",
		BaseURL: "http://provider-unreachable.invalid/v1", ProxyURL: "http://proxy-user:proxy-password@proxy.invalid",
		APIKey: "synthetic-embedded-proxy-credential", Model: "proxy-model",
		Messages: []domainmodel.Message{{Role: "user", Content: "bounded proxy request"}},
	})
	if err == nil || transportCalls != 0 {
		t.Fatalf("embedded proxy credentials reached transport: calls=%d err=%v", transportCalls, err)
	}
	if strings.Contains(err.Error(), "proxy-user") || strings.Contains(err.Error(), "proxy-password") ||
		strings.Contains(err.Error(), "synthetic-embedded-proxy-credential") {
		t.Fatal("proxy or Provider credential escaped the fail-closed error")
	}
}

func TestHTTPProviderClientDoesNotReconnectAfterUncommittedText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if hijacker, ok := w.(http.Hijacker); ok {
			conn, _, _ := hijacker.Hijack()
			_ = conn.Close()
		}
	}))
	defer server.Close()

	result, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-test-partial",
		Model:          "deepseek-chat",
		Messages:       []domainmodel.Message{{Role: "user", Content: "hello"}},
	})
	if !errors.Is(err, io.ErrUnexpectedEOF) && (err == nil || !strings.Contains(strings.ToLower(err.Error()), "ended before completion")) {
		t.Fatalf("expected post-output interruption error, got %v", err)
	}
	if len(result.Chunks) != 0 {
		t.Fatalf("uncommitted partial chunks crossed the provider boundary: %#v", result.Chunks)
	}
	if len(result.CacheObservations) != 1 {
		t.Fatalf("post-output interruption must not reconnect: %#v", result.CacheObservations)
	}
	for _, observation := range result.CacheObservations {
		if observation.Status != domaincache.ProviderCallStatusStreamAborted {
			t.Fatalf("unpublished stream cut must settle as failed: %#v", result.CacheObservations)
		}
	}
	var outputStarted interface{ ProviderOutputStarted() bool }
	if !errors.As(err, &outputStarted) || !outputStarted.ProviderOutputStarted() {
		t.Fatalf("post-output interruption lost its host observation: %T %v", err, err)
	}
}

func TestHTTPProviderClientRetriesReasoningOnlyAbortAsNewPhysicalAttempt(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		attempt := requests
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if attempt == 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"reasoning_content":"PRIVATE_ABORTED_REASONING"}}]}` + "\n\n"))
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"replacement"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	client := NewHTTPProviderClient(server.Client())
	client.MaxStreamReconnects = 1
	result, err := client.Stream(context.Background(), domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: server.URL, APIKey: "private-key", Model: "deepseek-chat",
		Messages: []domainmodel.Message{{Role: "user", Content: "case text"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	totalRequests := requests
	mu.Unlock()
	if totalRequests != 2 || len(result.CacheObservations) != 2 {
		t.Fatalf("reasoning-only abort did not create two physical attempts: requests=%d observations=%#v", totalRequests, result.CacheObservations)
	}
	if result.CacheObservations[0].Status != domaincache.ProviderCallStatusFailed ||
		result.CacheObservations[1].Status != domaincache.ProviderCallStatusSucceeded {
		t.Fatalf("reasoning-only retry settlements are invalid: %#v", result.CacheObservations)
	}
	if collectClientChunkText(result.Chunks, domainmodel.ChunkText) != "replacement" ||
		strings.Contains(collectClientChunkText(result.Chunks, domainmodel.ChunkReasoning), "PRIVATE_ABORTED_REASONING") {
		t.Fatalf("failed reasoning attempt survived replacement: %#v", result.Chunks)
	}
}

func TestHTTPProviderClientReconnectRevalidatesBeforeSecondPhysicalSend(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		current := requests
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if current == 1 {
			_, _ = w.Write([]byte("data: "))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if hijacker, ok := w.(http.Hijacker); ok {
				conn, _, _ := hijacker.Hijack()
				_ = conn.Close()
			}
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"unexpected"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	beforeSendAttempts := []int{}
	currentnessAttempts := []int{}
	pipelineStages := []string{}
	request := domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: server.URL, APIKey: "synthetic-reconnect-currentness", Model: "deepseek-chat",
		Messages: []domainmodel.Message{{Role: "user", Content: "bounded reconnect request"}},
		BeforeSend: func(attempt int) error {
			beforeSendAttempts = append(beforeSendAttempts, attempt)
			return nil
		},
		OnPipelineStage: func(stage domainmodel.PipelineStage) error {
			pipelineStages = append(pipelineStages, stage.Stage)
			return nil
		},
	}
	currentnessField := reflect.ValueOf(&request).Elem().FieldByName("PrivateProviderCurrentnessBeforeSend")
	if currentnessField.IsValid() && currentnessField.CanSet() {
		currentnessField.Set(reflect.ValueOf(func(attempt int) error {
			currentnessAttempts = append(currentnessAttempts, attempt)
			if attempt == 2 {
				return errors.New("provider execution authority changed")
			}
			return nil
		}))
	}
	client := NewHTTPProviderClient(server.Client())
	client.MaxStreamReconnects = 1
	_, err := client.Stream(context.Background(), request)
	if err == nil {
		t.Fatal("second physical send survived currentness failure")
	}
	mu.Lock()
	gotRequests := requests
	mu.Unlock()
	if gotRequests != 1 {
		t.Fatalf("currentness failure reached reconnect transport: requests=%d", gotRequests)
	}
	if !reflect.DeepEqual(beforeSendAttempts, []int{1, 2}) ||
		!reflect.DeepEqual(currentnessAttempts, []int{1, 2}) {
		t.Fatalf("pre-send callbacks lost physical attempt ordering: before=%v currentness=%v",
			beforeSendAttempts, currentnessAttempts)
	}
	if !reflect.DeepEqual(pipelineStages, []string{"pre_send", "post_send"}) {
		t.Fatalf("currentness failure left an unmatched durable send pair: stages=%v", pipelineStages)
	}
}

func collectClientChunkText(chunks []domainmodel.Chunk, kind domainmodel.ChunkKind) string {
	var text strings.Builder
	for _, chunk := range chunks {
		if chunk.Kind == kind {
			text.WriteString(chunk.Text)
		}
	}
	return text.String()
}

func TestHTTPProviderClientClassifiesReasoningOnlyAbortWithoutPublicOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"reasoning_content":"PRIVATE_ABORTED_REASONING"}}]}` + "\n\n"))
	}))
	defer server.Close()
	client := NewHTTPProviderClient(server.Client())
	client.MaxStreamReconnects = 0
	_, err := client.Stream(context.Background(), domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: server.URL, APIKey: "private-key", Model: "deepseek-chat",
		Messages: []domainmodel.Message{{Role: "user", Content: "case text"}},
	})
	if err == nil {
		t.Fatal("reasoning-only abort unexpectedly succeeded")
	}
	var observation interface {
		ProviderOutputObservationV1() domainmodel.ProviderOutputObservationV1
	}
	if !errors.As(err, &observation) ||
		observation.ProviderOutputObservationV1() != domainmodel.ProviderOutputObservationPrivateReasoningV1.MergeV1(domainmodel.ProviderOutputObservationControlV1) {
		t.Fatalf("reasoning-only abort lost its typed observation: %T %v", err, err)
	}
	var started interface{ ProviderOutputStarted() bool }
	if !errors.As(err, &started) || started.ProviderOutputStarted() {
		t.Fatalf("private reasoning was classified as public output: %T %v", err, err)
	}
}

func TestHTTPProviderClientReconnectAuthorityUsesTypedPayloadObservationAcrossEndpointFamilies(t *testing.T) {
	tests := []struct {
		name           string
		family         string
		format         string
		firstPayload   string
		successPayload string
		expectRetry    bool
		required       domainmodel.ProviderOutputObservationV1
	}{
		{
			name: "chat reasoning", family: "deepseek", format: "chat_completions", expectRetry: true,
			firstPayload: `data: {"choices":[{"delta":{"reasoning_content":"private-chat"}}]}` + "\n\n",
			successPayload: `data: {"choices":[{"delta":{"content":"replacement"}}]}` + "\n\n" +
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n",
			required: domainmodel.ProviderOutputObservationPrivateReasoningV1,
		},
		{
			name: "chat control", family: "deepseek", format: "chat_completions", expectRetry: true,
			firstPayload: `data: {"choices":[]}` + "\n\n",
			successPayload: `data: {"choices":[{"delta":{"content":"replacement"}}]}` + "\n\n" +
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n",
			required: domainmodel.ProviderOutputObservationControlV1,
		},
		{
			name: "chat public", family: "deepseek", format: "chat_completions",
			firstPayload: `data: {"choices":[{"delta":{"content":"uncommitted-public"}}]}` + "\n\n",
			successPayload: `data: {"choices":[{"delta":{"content":"must-not-run"}}]}` + "\n\n" +
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n",
			required: domainmodel.ProviderOutputObservationPublicTextV1,
		},
		{
			name: "chat reasoning then public", family: "deepseek", format: "chat_completions",
			firstPayload: `data: {"choices":[{"delta":{"reasoning_content":"private","content":"uncommitted-public"}}]}` + "\n\n",
			successPayload: `data: {"choices":[{"delta":{"content":"must-not-run"}}]}` + "\n\n" +
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n",
			required: domainmodel.ProviderOutputObservationPrivateReasoningV1.MergeV1(domainmodel.ProviderOutputObservationPublicTextV1),
		},
		{
			name: "chat tool", family: "deepseek", format: "chat_completions",
			firstPayload: `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\""}}]}}]}` + "\n\n",
			successPayload: `data: {"choices":[{"delta":{"content":"must-not-run"}}]}` + "\n\n" +
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n",
			required: domainmodel.ProviderOutputObservationToolStartedV1,
		},
		{
			name: "responses reasoning", family: "openai", format: "responses", expectRetry: true,
			firstPayload:   `data: {"type":"response.reasoning_text.delta","delta":"private-responses"}` + "\n\n",
			successPayload: `data: {"type":"response.output_text.delta","delta":"replacement"}` + "\n\n" + `data: {"type":"response.completed"}` + "\n\n",
			required:       domainmodel.ProviderOutputObservationPrivateReasoningV1,
		},
		{
			name: "responses public", family: "openai", format: "responses",
			firstPayload:   `data: {"type":"response.output_text.delta","delta":"uncommitted-public"}` + "\n\n",
			successPayload: `data: {"type":"response.output_text.delta","delta":"must-not-run"}` + "\n\n" + `data: {"type":"response.completed"}` + "\n\n",
			required:       domainmodel.ProviderOutputObservationPublicTextV1,
		},
		{
			name: "responses tool", family: "openai", format: "responses",
			firstPayload:   `data: {"type":"response.function_call_arguments.delta","item_id":"call_1","delta":"{\"x\""}` + "\n\n",
			successPayload: `data: {"type":"response.output_text.delta","delta":"must-not-run"}` + "\n\n" + `data: {"type":"response.completed"}` + "\n\n",
			required:       domainmodel.ProviderOutputObservationToolStartedV1,
		},
		{
			name: "messages reasoning", family: "anthropic", format: "messages", expectRetry: true,
			firstPayload: `data: {"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"private-messages"}}` + "\n\n",
			successPayload: `data: {"type":"message_start","message":{"usage":{"input_tokens":1}}}` + "\n\n" +
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"replacement"}}` + "\n\n" +
				`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}` + "\n\n" +
				`data: {"type":"message_stop"}` + "\n\n",
			required: domainmodel.ProviderOutputObservationPrivateReasoningV1,
		},
		{
			name: "messages public", family: "anthropic", format: "messages",
			firstPayload:   `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"uncommitted-public"}}` + "\n\n",
			successPayload: `data: {"type":"message_stop"}` + "\n\n",
			required:       domainmodel.ProviderOutputObservationPublicTextV1,
		},
		{
			name: "messages tool", family: "anthropic", format: "messages",
			firstPayload:   `data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"x\""}}` + "\n\n",
			successPayload: `data: {"type":"message_stop"}` + "\n\n",
			required:       domainmodel.ProviderOutputObservationToolStartedV1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				attempt := requests
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				if attempt == 1 {
					_, _ = w.Write([]byte(test.firstPayload))
					return
				}
				_, _ = w.Write([]byte(test.successPayload))
			}))
			defer server.Close()

			client := NewHTTPProviderClient(server.Client())
			client.MaxStreamReconnects = 1
			result, err := client.Stream(context.Background(), domainmodel.Request{
				ProviderID: test.family, Family: test.family, EndpointFormat: test.format,
				BaseURL: server.URL, APIKey: "private-key", Model: "test-model",
				Messages: []domainmodel.Message{{Role: "user", Content: "case text"}},
			})
			mu.Lock()
			totalRequests := requests
			mu.Unlock()
			if test.expectRetry {
				if err != nil || totalRequests != 2 || len(result.CacheObservations) != 2 || collectClientChunkText(result.Chunks, domainmodel.ChunkText) != "replacement" {
					t.Fatalf("retry-safe private output did not restart cleanly: requests=%d result=%#v err=%v", totalRequests, result, err)
				}
				return
			}
			if err == nil || totalRequests != 1 || len(result.CacheObservations) != 1 {
				t.Fatalf("retry-unsafe output was replayed: requests=%d observations=%#v err=%v", totalRequests, result.CacheObservations, err)
			}
			var observed interface {
				ProviderOutputObservationV1() domainmodel.ProviderOutputObservationV1
			}
			if !errors.As(err, &observed) || !observed.ProviderOutputObservationV1().IncludesV1(test.required) {
				t.Fatalf("retry-unsafe output lost typed evidence: %T %v", err, err)
			}
		})
	}
}

func TestHTTPProviderClientPreservesUsageOnPostUsageStreamCut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}` + "\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if hijacker, ok := w.(http.Hijacker); ok {
			conn, _, _ := hijacker.Hijack()
			_ = conn.Close()
		}
	}))
	defer server.Close()

	result, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID:     "deepseek",
		Family:         "deepseek",
		EndpointFormat: "chat_completions",
		BaseURL:        server.URL,
		APIKey:         "sk-test-partial-usage",
		Model:          "deepseek-chat",
		Messages:       []domainmodel.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected post-usage stream interruption")
	}
	if result.Usage.PromptTokens != 7 || result.Usage.CompletionTokens != 3 || result.Usage.TotalTokens != 10 {
		t.Fatalf("post-usage interruption should preserve usage: %#v", result.Usage)
	}
}

func TestHTTPProviderClientSettlesSuccessfulDeepSeekUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"prompt_cache_hit_tokens":1,"prompt_cache_miss_tokens":9}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	result, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: server.URL, APIKey: "private-key", Model: "deepseek-chat",
		Messages: []domainmodel.Message{{Role: "user", Content: "case text"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CacheObservations) != 1 || result.CacheObservations[0].Status != domaincache.ProviderCallStatusSucceeded ||
		!result.CacheObservations[0].Usage.CacheHitTokens.Known || result.CacheObservations[0].Usage.CacheHitTokens.Value != 1 ||
		!result.CacheObservations[0].Usage.CacheMissTokens.Known || result.CacheObservations[0].Usage.CacheMissTokens.Value != 9 {
		t.Fatalf("successful DeepSeek attempt was not settled with exact native usage: %#v", result.CacheObservations)
	}
}

func TestHTTPProviderClientNeverSignsContradictoryDeepSeekUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"not accepted"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11,"prompt_cache_hit_tokens":8,"prompt_cache_miss_tokens":8}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	result, err := NewHTTPProviderClient(server.Client()).Stream(context.Background(), domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: server.URL, APIKey: "private-key", Model: "deepseek-chat",
		Messages: []domainmodel.Message{{Role: "user", Content: "case text"}},
	})
	if err == nil {
		t.Fatal("contradictory DeepSeek usage was accepted")
	}
	if len(result.CacheObservations) != 1 || result.CacheObservations[0].Status != domaincache.ProviderCallStatusStreamAborted {
		t.Fatalf("contradictory attempt was not settled as a failed physical stream: %#v", result.CacheObservations)
	}
	usage := result.CacheObservations[0].Usage
	if usage.CacheHitTokens.Known || usage.CacheMissTokens.Known {
		t.Fatalf("contradictory cache counters entered the signed settlement: %#v", usage)
	}
}

func TestHTTPProviderClientSettlesHostCancellationAndDeadline(t *testing.T) {
	tests := []struct {
		name   string
		ctx    func() context.Context
		status domaincache.ProviderCallStatusV1
	}{
		{
			name: "cancelled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			status: domaincache.ProviderCallStatusCancelled,
		},
		{
			name: "timed out",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				cancel()
				return ctx
			},
			status: domaincache.ProviderCallStatusTimedOut,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewHTTPProviderClient(&http.Client{})
			result, err := client.Stream(test.ctx(), domainmodel.Request{
				ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
				BaseURL: "https://127.0.0.1:1", APIKey: "private-key", Model: "deepseek-chat",
				Messages: []domainmodel.Message{{Role: "user", Content: "case text"}},
			})
			if err == nil || len(result.CacheObservations) != 1 || result.CacheObservations[0].Status != test.status {
				t.Fatalf("host terminal path did not settle provider telemetry exactly: result=%#v err=%v", result, err)
			}
		})
	}
}

func TestHTTPProviderClientTrustedHTTPSReuseRotationAndRetirement(t *testing.T) {
	var mu sync.Mutex
	var headers []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		headers = append(headers, r.Header.Get("Authorization"))
		mu.Unlock()
		if r.Header.Get("Cookie") != "" {
			t.Error("provider transport resurrected cookies")
		}
		w.Header().Set("Set-Cookie", "synthetic=must-not-return")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"TLS fixture\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	})
	a, b := httptest.NewTLSServer(handler), httptest.NewTLSServer(handler)
	defer a.Close()
	defer b.Close()
	roots := x509.NewCertPool()
	roots.AddCert(a.Certificate())
	roots.AddCert(b.Certificate())
	client := NewHTTPProviderClient(nil)
	defer client.Close()
	request := domainmodel.Request{ProviderID: "tls-fixture", Family: "openai", Model: "fixture", EndpointFormat: "chat_completions", PrivateProviderProxyAuthority: true, APIKey: "synthetic-first"}
	reused := []bool{}
	trusted := map[*http.Transport]bool{}
	for i, endpoint := range []string{a.URL, a.URL, b.URL, a.URL} {
		request.BaseURL = endpoint + "/v1"
		if i > 0 {
			request.APIKey = "synthetic-rotated"
		}
		httpClient, release, err := client.httpClientForAttempt(request, endpoint+"/v1/chat/completions")
		if err != nil {
			t.Fatal(err)
		}
		transport := httpClient.Transport.(*http.Transport)
		// Configure this fixture's CA before first use of each new transport.
		if !trusted[transport] {
			transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			trusted[transport] = true
		}
		if transport.TLSClientConfig.InsecureSkipVerify {
			t.Fatal("TLS verification disabled")
		}
		release()
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = append(reused, info.Reused) }})
		if _, err := client.Stream(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(reused, []bool{false, true, false, false}) {
		t.Fatalf("TLS connection reuse/retirement = %v", reused)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(headers, []string{"Bearer synthetic-first", "Bearer synthetic-rotated", "Bearer synthetic-rotated", "Bearer synthetic-rotated"}) {
		t.Fatal("authentication headers were stale")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Stream(context.Background(), request); err == nil {
		t.Fatal("closed pool admitted a send")
	}
	if len(headers) != 4 {
		t.Fatal("close triggered an extra send")
	}
}
