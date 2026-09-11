package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	cachetelemetryport "analytix.local/runtime-go/internal/ports/cachetelemetry"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type productionTelemetryRecorderStub struct {
	mu            sync.Mutex
	beginErr      error
	settlementErr error
	registrations []cachetelemetryport.AttemptRegistrationInputV1
	settlements   []cachetelemetryport.AttemptSettlementInputV1
}

func (stub *productionTelemetryRecorderStub) BeginAttempt(_ context.Context, input cachetelemetryport.AttemptRegistrationInputV1) (cachetelemetryport.AttemptHandleV1, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.registrations = append(stub.registrations, cloneAttemptRegistrationInputV1(input))
	if stub.beginErr != nil {
		return cachetelemetryport.AttemptHandleV1{}, stub.beginErr
	}
	return cachetelemetryport.AttemptHandleV1{Intent: domaincache.ProviderAttemptIntentV1{Shape: domaincache.CacheVisibleShapeV1{
		SchemaVersion: domaincache.CacheVisibleShapeV1SchemaVersion,
		Attempt:       input.PhysicalAttempt,
		StartedAt:     input.StartedAt.UTC().Format(time.RFC3339Nano),
	}}}, nil
}

func (stub *productionTelemetryRecorderStub) SettleAttempt(_ context.Context, input cachetelemetryport.AttemptSettlementInputV1) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.settlements = append(stub.settlements, input)
	return stub.settlementErr
}

func (stub *productionTelemetryRecorderStub) snapshot() ([]cachetelemetryport.AttemptRegistrationInputV1, []cachetelemetryport.AttemptSettlementInputV1) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	registrations := append([]cachetelemetryport.AttemptRegistrationInputV1(nil), stub.registrations...)
	settlements := append([]cachetelemetryport.AttemptSettlementInputV1(nil), stub.settlements...)
	return registrations, settlements
}

type productionTelemetryFailure struct{ message string }

func (err productionTelemetryFailure) Error() string            { return err.message }
func (productionTelemetryFailure) ProviderRetryForbidden() bool { return true }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestNewProductionHTTPProviderClientRequiresDurableRecorder(t *testing.T) {
	if _, err := NewProductionHTTPProviderClient(nil, nil); err == nil {
		t.Fatal("production provider client accepted a nil durable recorder")
	}
}

func TestInvalidReasoningEffortStopsBeforeBeforeSendHTTPAndTelemetry(t *testing.T) {
	for _, effort := range []string{
		" ", " high ", "\thigh\n", "HIGH", "adaptive", "disabled", "none", "false",
		"minimal", "mid", "maximum", "xhigh", "high\u200b", "unknown", "extreme",
		"SOL_PRIVATE_REASONING_SENTINEL_7F3C", `{"effort":"high"}`, "<think>private</think>",
	} {
		t.Run(effort, func(t *testing.T) {
			var httpCalls atomic.Int32
			var beforeSendCalls atomic.Int32
			var pipelineCalls atomic.Int32
			httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				httpCalls.Add(1)
				return nil, errors.New("must not send")
			})}
			recorder := &productionTelemetryRecorderStub{}
			client, err := NewProductionHTTPProviderClient(httpClient, recorder)
			if err != nil {
				t.Fatal(err)
			}
			request := productionTelemetryRequestV1(t, "https://provider.invalid")
			request.ReasoningEffort = effort
			request.BeforeSend = func(int) error { beforeSendCalls.Add(1); return nil }
			request.OnPipelineStage = func(domainmodel.PipelineStage) error { pipelineCalls.Add(1); return nil }
			result, err := client.Stream(context.Background(), request)
			registrations, settlements := recorder.snapshot()
			if !errors.Is(err, domainmodel.ErrInvalidReasoningEffort) ||
				!reflect.DeepEqual(result, domainmodel.Result{}) ||
				httpCalls.Load() != 0 || beforeSendCalls.Load() != 0 || pipelineCalls.Load() != 0 ||
				len(registrations) != 0 || len(settlements) != 0 ||
				client.cacheLogicalCalls.Load() != 0 || client.cacheAuthority != nil {
				t.Fatalf("invalid effort crossed provider admission: result=%#v http=%d before=%d pipeline=%d registrations=%d settlements=%d logical=%d authority=%v err=%v",
					result, httpCalls.Load(), beforeSendCalls.Load(), pipelineCalls.Load(), len(registrations), len(settlements), client.cacheLogicalCalls.Load(), client.cacheAuthority != nil, err)
			}
			if effort == "SOL_PRIVATE_REASONING_SENTINEL_7F3C" && strings.Contains(err.Error(), effort) {
				t.Fatalf("provider error reflected private effort sentinel: %v", err)
			}
		})
	}
}

func TestProductionProviderTelemetryBlocksBeforeSendAndSettlesDispatchState(t *testing.T) {
	t.Run("missing binding", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
		defer server.Close()
		recorder := &productionTelemetryRecorderStub{}
		client, err := NewProductionHTTPProviderClient(server.Client(), recorder)
		if err != nil {
			t.Fatal(err)
		}
		request := productionTelemetryRequestV1(t, server.URL)
		request.PrivateProviderTelemetry = nil
		if _, err := client.Stream(context.Background(), request); err == nil || requests.Load() != 0 {
			t.Fatalf("missing telemetry binding crossed HTTP boundary: requests=%d err=%v", requests.Load(), err)
		}
		registrations, settlements := recorder.snapshot()
		if len(registrations) != 0 || len(settlements) != 0 {
			t.Fatalf("missing binding touched durable recorder: registrations=%d settlements=%d", len(registrations), len(settlements))
		}
	})

	t.Run("begin failure", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
		defer server.Close()
		blocked := productionTelemetryFailure{message: "intent commit failed"}
		recorder := &productionTelemetryRecorderStub{beginErr: blocked}
		client, err := NewProductionHTTPProviderClient(server.Client(), recorder)
		if err != nil {
			t.Fatal(err)
		}
		request := productionTelemetryRequestV1(t, server.URL)
		request.PrivateProviderTelemetry.OrdinaryEffect = true
		streamErr := error(nil)
		if _, streamErr = client.Stream(context.Background(), request); !errors.Is(streamErr, blocked) || requests.Load() != 0 {
			t.Fatalf("uncommitted intent crossed HTTP boundary: requests=%d err=%v", requests.Load(), streamErr)
		}
		var diagnostic interface{ Diagnostics() map[string]any }
		if !errors.As(streamErr, &diagnostic) || diagnostic.Diagnostics()["failureStage"] != string(ProviderFailureStageTelemetryBegin) ||
			diagnostic.Diagnostics()["dispatchState"] != string(domaincache.ProviderDispatchStateNotSent) ||
			diagnostic.Diagnostics()["attempt"] != float64(1) {
			t.Fatalf("telemetry begin failure lost closed attribution: %#v", diagnostic)
		}
		registrations, settlements := recorder.snapshot()
		if len(registrations) != 1 || !registrations[0].OrdinaryEffect || len(settlements) != 0 {
			t.Fatalf("begin failure lifecycle mismatch: registrations=%d settlements=%d", len(registrations), len(settlements))
		}
	})

	t.Run("before send failure is not sent", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
		defer server.Close()
		recorder := &productionTelemetryRecorderStub{}
		client, err := NewProductionHTTPProviderClient(server.Client(), recorder)
		if err != nil {
			t.Fatal(err)
		}
		request := productionTelemetryRequestV1(t, server.URL)
		request.BeforeSend = func(int) error { return errors.New("stale turn authority") }
		if _, err := client.Stream(context.Background(), request); err == nil || requests.Load() != 0 {
			t.Fatalf("failed before-send authority crossed HTTP boundary: requests=%d err=%v", requests.Load(), err)
		}
		registrations, settlements := recorder.snapshot()
		if len(registrations) != 0 || len(settlements) != 0 {
			t.Fatalf("before-send failure touched body-bearing telemetry: registrations=%d settlements=%d", len(registrations), len(settlements))
		}
	})

	t.Run("HTTP response is sent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
		defer server.Close()
		recorder := &productionTelemetryRecorderStub{}
		client, err := NewProductionHTTPProviderClient(server.Client(), recorder)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Stream(context.Background(), productionTelemetryRequestV1(t, server.URL)); err == nil {
			t.Fatal("expected provider HTTP failure")
		}
		_, settlements := recorder.snapshot()
		if len(settlements) != 1 || settlements[0].DispatchState != domaincache.ProviderDispatchStateSent {
			t.Fatalf("HTTP response was not settled as sent: %#v", settlements)
		}
	})

	t.Run("transport without response is indeterminate", func(t *testing.T) {
		var requests atomic.Int32
		httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("dial failed")
		})}
		recorder := &productionTelemetryRecorderStub{}
		client, err := NewProductionHTTPProviderClient(httpClient, recorder)
		if err != nil {
			t.Fatal(err)
		}
		client.MaxStreamReconnects = 0
		if _, err := client.Stream(context.Background(), productionTelemetryRequestV1(t, "https://provider.invalid")); err == nil || requests.Load() != 1 {
			t.Fatalf("expected one indeterminate transport attempt: requests=%d err=%v", requests.Load(), err)
		}
		_, settlements := recorder.snapshot()
		if len(settlements) != 1 || settlements[0].DispatchState != domaincache.ProviderDispatchStateIndeterminate {
			t.Fatalf("transport without response was not indeterminate: %#v", settlements)
		}
	})
}

func TestProductionProviderTelemetrySettlementFailureRejectsOutputAndRetry(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"must not escape\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	blocked := productionTelemetryFailure{message: "settlement readback failed"}
	recorder := &productionTelemetryRecorderStub{settlementErr: blocked}
	client, err := NewProductionHTTPProviderClient(server.Client(), recorder)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Stream(context.Background(), productionTelemetryRequestV1(t, server.URL))
	var marker interface{ ProviderRetryForbidden() bool }
	if !errors.Is(err, blocked) || !errors.As(err, &marker) || !marker.ProviderRetryForbidden() || requests.Load() != 1 ||
		!reflect.DeepEqual(result, domainmodel.Result{}) {
		t.Fatalf("unsettled provider output escaped: result=%#v requests=%d err=%v", result, requests.Load(), err)
	}
}

func TestProductionProviderTelemetryReconnectKeepsExactLogicalWireIdentity(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := requests.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if attempt == 1 {
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
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	recorder := &productionTelemetryRecorderStub{}
	client, err := NewProductionHTTPProviderClient(server.Client(), recorder)
	if err != nil {
		t.Fatal(err)
	}
	client.MaxStreamReconnects = 1
	if _, err := client.Stream(context.Background(), productionTelemetryRequestV1(t, server.URL)); err != nil {
		t.Fatal(err)
	}
	registrations, settlements := recorder.snapshot()
	if requests.Load() != 2 || len(registrations) != 2 || len(settlements) != 2 ||
		registrations[0].PhysicalAttempt != 1 || registrations[1].PhysicalAttempt != 2 ||
		registrations[0].LogicalSequence != registrations[1].LogicalSequence ||
		registrations[0].OuterAttempt != registrations[1].OuterAttempt ||
		!reflect.DeepEqual(registrations[0].WireBody, registrations[1].WireBody) ||
		!reflect.DeepEqual(registrations[0].WireHeaders, registrations[1].WireHeaders) {
		t.Fatalf("reconnect changed durable logical wire identity: registrations=%#v settlements=%#v requests=%d", registrations, settlements, requests.Load())
	}
}

func TestProductionProviderAuditRejectsBeforeTelemetryAndTransport(t *testing.T) {
	var requests atomic.Int32
	recorder := &productionTelemetryRecorderStub{}
	client, err := NewProductionHTTPProviderClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("transport must not run")
	})}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	client.ProviderBodyAuditor = &recordingProviderBodyAuditorV1{err: errors.New("audit rejected")}
	request := productionTelemetryRequestV1(t, "https://provider.invalid")
	request.ProviderID = "analytix-hub"
	_, streamErr := client.Stream(context.Background(), request)
	var disposition interface {
		ProviderDispatchStateV1() domaincache.ProviderDispatchStateV1
	}
	registrations, settlements := recorder.snapshot()
	if streamErr == nil || !errors.As(streamErr, &disposition) ||
		disposition.ProviderDispatchStateV1() != domaincache.ProviderDispatchStateNotSent ||
		requests.Load() != 0 || len(registrations) != 0 || len(settlements) != 0 {
		t.Fatalf("rejected body reached telemetry or transport: requests=%d registrations=%d settlements=%d err=%v",
			requests.Load(), len(registrations), len(settlements), streamErr)
	}
}

func productionTelemetryRequestV1(t *testing.T, baseURL string) domainmodel.Request {
	t.Helper()
	securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-provider", TurnID: "turn-provider", WorkspaceRealPath: "/workspace",
		CaseID: "case-provider", CaseBindingHash: domainsecurity.SHA256Hex([]byte("case-binding")),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID("provider-snapshot"),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("source-manifest")), ContextEpoch: 3,
		IssuedAt: time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return domainmodel.Request{
		ProviderID: "deepseek", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: baseURL, APIKey: "private-provider-key", Model: "deepseek-chat",
		Messages: []domainmodel.Message{{Role: "user", Content: "private case prompt"}},
		PrivateProviderTelemetry: &domainmodel.ProviderTelemetryBindingV1{
			SecurityContext: securityContext, UsageSource: domaincache.ProviderUsageSourceTurn,
			Channel: domaincache.ProviderChannelPrimary, LogicalSequence: 1, OuterAttempt: 1,
		},
	}
}

func cloneAttemptRegistrationInputV1(input cachetelemetryport.AttemptRegistrationInputV1) cachetelemetryport.AttemptRegistrationInputV1 {
	input.ChildRunID = append([]byte(nil), input.ChildRunID...)
	input.Model = append([]byte(nil), input.Model...)
	input.Endpoint = append([]byte(nil), input.Endpoint...)
	input.WireBody = append([]byte(nil), input.WireBody...)
	input.WireHeaders = append([]byte(nil), input.WireHeaders...)
	input.CredentialScope = append([]byte(nil), input.CredentialScope...)
	input.ProviderConfig = append([]byte(nil), input.ProviderConfig...)
	return input
}
