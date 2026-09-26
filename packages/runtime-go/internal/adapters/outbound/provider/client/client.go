package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	providercompat "analytix.local/runtime-go/internal/adapters/outbound/provider/compat"
	providerstream "analytix.local/runtime-go/internal/adapters/outbound/provider/stream"
	providerusage "analytix.local/runtime-go/internal/adapters/outbound/provider/usage"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"analytix.local/runtime-go/internal/netclient"
	"analytix.local/runtime-go/internal/ports"
	cachetelemetryport "analytix.local/runtime-go/internal/ports/cachetelemetry"
)

const DefaultStreamIdleTimeout = 120 * time.Second
const DefaultStreamMaxReconnects = 3

type HTTPProviderClient struct {
	HTTP                *http.Client
	StreamIdleTimeout   time.Duration
	MaxStreamReconnects int
	ProviderBodyAuditor ProviderRequestBodyAuditor
	cacheAuthorityMu    sync.Mutex
	cacheAuthority      *cacheDigestAuthority
	cacheLogicalCalls   atomic.Uint64
	telemetryRecorder   cachetelemetryport.Recorder
	productionTelemetry bool
	transportMu         sync.Mutex
	transportSlots      map[providerTransportOwner]*providerTransportSlot
	transportUse        uint64
	transportClosed     bool
}

const maxProviderTransportSlots = 8

type providerTransportOwner struct{ providerID, family string }

type providerTransportKey struct {
	proxyURL, endpointOrigin string
	authoritative            bool
}

type providerTransportSlot struct {
	key     providerTransportKey
	client  *http.Client
	active  int
	lastUse uint64
	retired bool
}

var _ ports.DurablePipelineProviderClient = (*HTTPProviderClient)(nil)

// RequiresDurablePipelineStagesV1 declares the physical-send contract used by
// the runtime Agent loop. streamOnce emits pre_send before http.Client.Do and
// closes it with post_send before returning control to the loop.
func (*HTTPProviderClient) RequiresDurablePipelineStagesV1() {}

type ProviderError struct {
	ProviderID     string `json:"providerId"`
	Model          string `json:"model"`
	Family         string `json:"family"`
	EndpointFormat string `json:"endpointFormat"`
	BaseURL        string `json:"baseUrl"`
	RequestURL     string `json:"requestUrl"`
	Status         int    `json:"status"`
	Kind           string `json:"kind"`
	Message        string `json:"message"`
	HasAPIKey      bool   `json:"hasApiKey"`
	RetryAfterMs   int    `json:"retryAfterMs,omitempty"`
}

// ProviderFailureStageV1 is the closed, byte-free attribution for a provider
// failure. It describes the last host boundary that is actually observable;
// it never asserts that request bytes crossed a socket when the transport did
// not return a response.
type ProviderFailureStageV1 string

const (
	ProviderFailureStageRequestValidation       ProviderFailureStageV1 = "request_validation"
	ProviderFailureStageRequestBuild            ProviderFailureStageV1 = "request_build"
	ProviderFailureStagePreSendBodyAudit        ProviderFailureStageV1 = "pre_send_body_audit"
	ProviderFailureStageTelemetryBegin          ProviderFailureStageV1 = "telemetry_begin"
	ProviderFailureStageTransportBeforeObserved ProviderFailureStageV1 = "transport_before_observed_send"
	ProviderFailureStageTransportAfterObserved  ProviderFailureStageV1 = "transport_after_observed_send"
	ProviderFailureStageLocalAdmissionConfig    ProviderFailureStageV1 = "local_admission_config"
	ProviderFailureStageCallbackProjection      ProviderFailureStageV1 = "callback_projection"
	ProviderFailureStageUnclassified            ProviderFailureStageV1 = "unclassified"
)

func (stage ProviderFailureStageV1) valid() bool {
	switch stage {
	case ProviderFailureStageRequestValidation, ProviderFailureStageRequestBuild,
		ProviderFailureStagePreSendBodyAudit, ProviderFailureStageTelemetryBegin,
		ProviderFailureStageTransportBeforeObserved, ProviderFailureStageTransportAfterObserved,
		ProviderFailureStageLocalAdmissionConfig, ProviderFailureStageCallbackProjection,
		ProviderFailureStageUnclassified:
		return true
	default:
		return false
	}
}

func normalizeProviderFailureStageV1(stage ProviderFailureStageV1) ProviderFailureStageV1 {
	if stage.valid() {
		return stage
	}
	return ProviderFailureStageUnclassified
}

type providerFailureStageObserverV1 interface {
	FailureStageV1() string
}

// providerFailureStageErrorV1 marks a host callback/projection boundary
// without changing the underlying error identity or retaining provider bytes.
// The dispatch state is added only by the outer transport wrapper.
type providerFailureStageErrorV1 struct {
	err   error
	stage ProviderFailureStageV1
}

func (err providerFailureStageErrorV1) Error() string {
	if err.err == nil {
		return "provider failure stage is unavailable"
	}
	return err.err.Error()
}

func (err providerFailureStageErrorV1) Unwrap() error { return err.err }

func (err providerFailureStageErrorV1) FailureStageV1() string {
	return string(normalizeProviderFailureStageV1(err.stage))
}

func withProviderFailureStageV1(err error, stage ProviderFailureStageV1) error {
	if err == nil {
		return nil
	}
	return providerFailureStageErrorV1{err: err, stage: normalizeProviderFailureStageV1(stage)}
}

// providerDispatchStateErrorV1 is a byte-free transport disposition carried
// only on provider failures. The Agent loop uses it to distinguish a trusted
// pre-transport failure from a sent or indeterminate attempt when no durable
// pre_send/post_send pair was completed.
type providerDispatchStateErrorV1 struct {
	err          error
	state        domaincache.ProviderDispatchStateV1
	failureStage ProviderFailureStageV1
	attempt      int
}

func (err providerDispatchStateErrorV1) Error() string {
	if err.err == nil {
		return "provider dispatch failed"
	}
	return err.err.Error()
}

func (err providerDispatchStateErrorV1) Unwrap() error { return err.err }

func (err providerDispatchStateErrorV1) ProviderDispatchStateV1() domaincache.ProviderDispatchStateV1 {
	return err.state
}

func (err providerDispatchStateErrorV1) FailureStageV1() string {
	return string(normalizeProviderFailureStageV1(err.failureStage))
}

// Diagnostics exposes only the closed attribution fields. Provider-specific
// diagnostics remain available through the wrapped ProviderError and are
// merged and projected by the loop recovery boundary.
func (err providerDispatchStateErrorV1) Diagnostics() map[string]any {
	state := err.state
	switch state {
	case domaincache.ProviderDispatchStateNotSent, domaincache.ProviderDispatchStateSent,
		domaincache.ProviderDispatchStateIndeterminate:
	default:
		state = domaincache.ProviderDispatchStateIndeterminate
	}
	diagnostics := map[string]any{
		"dispatchState": string(state),
		"failureStage":  string(normalizeProviderFailureStageV1(err.failureStage)),
	}
	if err.attempt > 0 {
		diagnostics["attempt"] = float64(err.attempt)
	}
	return diagnostics
}

func withProviderFailureContextV1(
	err error,
	state domaincache.ProviderDispatchStateV1,
	stage ProviderFailureStageV1,
	attempt int,
) error {
	if err == nil {
		return nil
	}
	stage = normalizeProviderFailureStageV1(stage)
	if observed := providerFailureStageFromErrorV1(err); observed != ProviderFailureStageUnclassified {
		stage = observed
	}
	if attempt < 0 {
		attempt = 0
	}
	switch state {
	case domaincache.ProviderDispatchStateNotSent, domaincache.ProviderDispatchStateSent,
		domaincache.ProviderDispatchStateIndeterminate:
	default:
		state = domaincache.ProviderDispatchStateIndeterminate
	}
	return providerDispatchStateErrorV1{err: err, state: state, failureStage: stage, attempt: attempt}
}

func withProviderDispatchStateV1(err error, state domaincache.ProviderDispatchStateV1) error {
	return withProviderFailureContextV1(err, state, ProviderFailureStageUnclassified, 0)
}

func providerFailureStageFromErrorV1(err error) ProviderFailureStageV1 {
	var observer providerFailureStageObserverV1
	if err == nil || !errors.As(err, &observer) {
		return ProviderFailureStageUnclassified
	}
	rawStage := observer.FailureStageV1()
	if strings.TrimSpace(rawStage) != rawStage {
		return ProviderFailureStageUnclassified
	}
	stage := ProviderFailureStageV1(rawStage)
	return normalizeProviderFailureStageV1(stage)
}

func providerFailureStageForDispatchStateV1(state domaincache.ProviderDispatchStateV1) ProviderFailureStageV1 {
	switch state {
	case domaincache.ProviderDispatchStateSent:
		return ProviderFailureStageTransportAfterObserved
	default:
		return ProviderFailureStageUnclassified
	}
}

// interruptedProviderOutputError carries only byte-free host observations.
// Provider payload remains private and transactional, while callers can
// distinguish retry-safe private reasoning from public text and tool state.
type interruptedProviderOutputError struct {
	cause       error
	observation domainmodel.ProviderOutputObservationV1
}

func (err interruptedProviderOutputError) Error() string {
	if err.cause == nil {
		return "provider output was interrupted"
	}
	return err.cause.Error()
}

func (err interruptedProviderOutputError) Unwrap() error { return err.cause }

func (err interruptedProviderOutputError) ProviderOutputStarted() bool {
	return err.observation.RetryUnsafeV1()
}

func (err interruptedProviderOutputError) ProviderOutputObservationV1() domainmodel.ProviderOutputObservationV1 {
	return err.observation
}

func NewHTTPProviderClient(httpClient *http.Client) *HTTPProviderClient {
	if httpClient == nil {
		httpClient = NewDefaultHTTPClient(10 * time.Second)
	}
	return &HTTPProviderClient{
		HTTP:                httpClient,
		StreamIdleTimeout:   DefaultStreamIdleTimeout,
		MaxStreamReconnects: DefaultStreamMaxReconnects,
	}
}

func NewProductionHTTPProviderClient(httpClient *http.Client, recorder cachetelemetryport.Recorder) (*HTTPProviderClient, error) {
	if recorder == nil {
		return nil, errors.New("production provider cache telemetry recorder is required")
	}
	client := NewHTTPProviderClient(httpClient)
	client.telemetryRecorder = recorder
	client.productionTelemetry = true
	return client, nil
}

func NewDefaultHTTPClient(timeout time.Duration) *http.Client {
	return NewDefaultHTTPClientWithProxy("")
}

func NewDefaultHTTPClientWithProxy(proxyURL string) *http.Client {
	client, err := netclient.NewHTTPClient(providerProxySpec(proxyURL), netclient.TransportOptions{
		DialTimeout:           30 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	})
	if err != nil {
		client, err = netclient.NewHTTPClient(netclient.ProxySpec{Mode: netclient.ModeAuto}, netclient.TransportOptions{
			DialTimeout:           30 * time.Second,
			KeepAlive:             30 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: 120 * time.Second,
		})
		if err != nil {
			return &http.Client{}
		}
	}
	return client
}

func NetworkProxyDiagnostics(proxyURL string) map[string]any {
	spec := providerProxySpec(proxyURL)
	configured := strings.TrimSpace(proxyURL) != ""
	out := map[string]any{
		"mode":              netclient.NormalizeMode(spec.Mode),
		"configured":        configured,
		"source":            map[bool]string{true: "settings.provider.proxy", false: "environment"}[configured],
		"summary":           netclient.Summary(spec),
		"valid":             true,
		"credentialsMasked": true,
	}
	if err := netclient.Validate(spec); err != nil {
		out["valid"] = false
		out["error"] = sanitizeProviderSecretText(err.Error(), "")
	}
	return out
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	providerID := firstNonEmpty(e.ProviderID, e.Family, "model provider")
	status := "failed"
	if e.Status > 0 {
		status = fmt.Sprintf("returned %d", e.Status)
	}
	kind := strings.TrimSpace(e.Kind)
	if kind != "" {
		kind = " (" + kind + ")"
	}
	message := strings.TrimSpace(e.Message)
	if message != "" {
		message = ": " + message
	}
	requestURL := strings.TrimSpace(e.RequestURL)
	if requestURL != "" {
		requestURL = " requestUrl=" + requestURL
	}
	model := strings.TrimSpace(e.Model)
	if model != "" {
		model = " model=" + model
	}
	return fmt.Sprintf("provider %s %s%s%s%s%s", providerID, status, kind, model, requestURL, message)
}

func (e *ProviderError) Diagnostics() map[string]any {
	if e == nil {
		return nil
	}
	authStatus := "none"
	if e.AuthRequired() {
		authStatus = "required"
	}
	diagnostics := map[string]any{
		"providerId":     e.ProviderID,
		"model":          e.Model,
		"family":         e.Family,
		"endpointFormat": e.EndpointFormat,
		"baseUrl":        e.BaseURL,
		"requestUrl":     e.RequestURL,
		"status":         float64(e.Status),
		"kind":           e.Kind,
		"message":        e.Message,
		"hasApiKey":      e.HasAPIKey,
		"authStatus":     authStatus,
		"retryable":      e.Retryable(),
	}
	if e.RetryAfterMs > 0 {
		diagnostics["retryAfterMs"] = float64(e.RetryAfterMs)
	}
	return diagnostics
}

func (e *ProviderError) AuthRequired() bool {
	if e == nil {
		return false
	}
	return e.Kind == "auth" || e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

func (e *ProviderError) Retryable() bool {
	if e == nil {
		return false
	}
	switch e.Status {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return e.Kind == "network"
}

func (e *ProviderError) RetryAfterDelay() time.Duration {
	if e == nil || e.RetryAfterMs <= 0 {
		return 0
	}
	return time.Duration(e.RetryAfterMs) * time.Millisecond
}

func (c *HTTPProviderClient) Stream(ctx context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if err := domainmodel.ValidateReasoningEffortV1(request.ReasoningEffort); err != nil {
		return domainmodel.Result{}, withProviderFailureContextV1(
			err, domaincache.ProviderDispatchStateNotSent, ProviderFailureStageRequestValidation, 0,
		)
	}
	requestBuildStartedAt := time.Now()
	endpoint, body, headers, err := buildHTTPRequest(request)
	if err != nil {
		return domainmodel.Result{}, withProviderFailureContextV1(
			err, domaincache.ProviderDispatchStateNotSent, ProviderFailureStageRequestBuild, 0,
		)
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return domainmodel.Result{}, withProviderFailureContextV1(
			err, domaincache.ProviderDispatchStateNotSent, ProviderFailureStageRequestBuild, 0,
		)
	}
	headerBytes, err := json.Marshal(headers)
	if err != nil {
		return domainmodel.Result{}, withProviderFailureContextV1(
			err, domaincache.ProviderDispatchStateNotSent, ProviderFailureStageRequestBuild, 0,
		)
	}
	maxReconnects := c.effectiveMaxStreamReconnects()
	parseEndpointFormat := request.EndpointFormat
	if domainmodel.NormalizeEndpointFormat(request.EndpointFormat) == "custom_endpoint" {
		parseEndpointFormat = providercompat.CustomEndpointRequestShape(request.BaseURL)
	}
	var telemetry *providerCallTelemetry
	for attempt := 0; ; attempt++ {
		physicalAttempt := attempt + 1
		if request.BeforeSend != nil {
			if err := request.BeforeSend(physicalAttempt); err != nil {
				return domainmodel.Result{}, withProviderFailureContextV1(
					err, domaincache.ProviderDispatchStateNotSent, ProviderFailureStageCallbackProjection, physicalAttempt,
				)
			}
		}
		if c != nil && c.ProviderBodyAuditor != nil {
			if err := c.ProviderBodyAuditor.AuditProviderRequestBody(
				ctx, request.ProviderID, physicalAttempt, bodyBytes,
			); err != nil {
				return domainmodel.Result{}, withProviderFailureContextV1(
					err, domaincache.ProviderDispatchStateNotSent, ProviderFailureStagePreSendBodyAudit, physicalAttempt,
				)
			}
		}
		if telemetry == nil {
			telemetry, err = c.newProviderCallTelemetry(request, endpoint, bodyBytes, headerBytes)
			if err != nil {
				return domainmodel.Result{}, withProviderFailureContextV1(
					err, domaincache.ProviderDispatchStateNotSent, ProviderFailureStageLocalAdmissionConfig, physicalAttempt,
				)
			}
		}
		shape, beginErr := telemetry.begin(ctx, physicalAttempt, time.Now().UTC())
		if beginErr != nil {
			return domainmodel.Result{}, withProviderFailureContextV1(
				beginErr, domaincache.ProviderDispatchStateNotSent, ProviderFailureStageTelemetryBegin, physicalAttempt,
			)
		}
		result, emitted, dispatchState, err := c.streamOnce(ctx, request, parseEndpointFormat, endpoint, body, bodyBytes, headers, requestBuildStartedAt, physicalAttempt)
		hostErr := ctx.Err()
		settleCtx, cancelSettle := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		observations, settleErr := telemetry.settle(settleCtx, shape, result, emitted, err, hostErr, dispatchState, time.Now().UTC())
		cancelSettle()
		result.CacheObservations = observations
		if settleErr != nil {
			failureStage := providerFailureStageFromErrorV1(err)
			if failureStage == ProviderFailureStageUnclassified {
				failureStage = providerFailureStageForDispatchStateV1(dispatchState)
			}
			settlementError := errors.Join(hostErr, err, settleErr)
			if c != nil && c.productionTelemetry {
				return domainmodel.Result{}, withProviderFailureContextV1(
					settleErr, dispatchState, failureStage, physicalAttempt,
				)
			}
			return result, withProviderFailureContextV1(
				settlementError, dispatchState, failureStage, physicalAttempt,
			)
		}
		if hostErr != nil {
			result.Chunks = nil
			return result, withProviderFailureContextV1(
				hostErr, dispatchState, providerFailureStageForDispatchStateV1(dispatchState), physicalAttempt,
			)
		}
		if err == nil {
			return result, nil
		}
		failureStage := providerFailureStageFromErrorV1(err)
		if failureStage == ProviderFailureStageUnclassified {
			failureStage = providerFailureStageForDispatchStateV1(dispatchState)
		}
		if emitted || attempt >= maxReconnects || !isPreOutputStreamReplayableError(err) {
			return result, withProviderFailureContextV1(err, dispatchState, failureStage, physicalAttempt)
		}
		if request.OnChunk != nil {
			if callbackErr := request.OnChunk(domainmodel.Chunk{
				Kind:         domainmodel.ChunkRetrying,
				Text:         err.Error(),
				RetryAttempt: attempt + 2,
				RetryMax:     maxReconnects + 1,
			}); callbackErr != nil {
				return result, withProviderFailureContextV1(
					withProviderFailureStageV1(callbackErr, ProviderFailureStageCallbackProjection),
					dispatchState, ProviderFailureStageCallbackProjection, physicalAttempt,
				)
			}
		}
	}
}

func (c *HTTPProviderClient) streamOnce(ctx context.Context, request domainmodel.Request, parseEndpointFormat string, endpoint string, body map[string]any, bodyBytes []byte, headers map[string]string, requestBuildStartedAt time.Time, attempt int) (domainmodel.Result, bool, domaincache.ProviderDispatchStateV1, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return domainmodel.Result{}, false, domaincache.ProviderDispatchStateNotSent,
			withProviderFailureStageV1(err, ProviderFailureStageRequestBuild)
	}
	for key, value := range headers {
		httpReq.Header.Set(key, value)
	}
	baseHTTPClient, releaseTransport, err := c.httpClientForAttempt(request, endpoint)
	if err != nil {
		return domainmodel.Result{}, false, domaincache.ProviderDispatchStateNotSent,
			withProviderFailureStageV1(err, ProviderFailureStageRequestBuild)
	}
	if releaseTransport != nil {
		defer releaseTransport()
	}
	httpClient := *baseHTTPClient
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if request.PrivateProviderCurrentnessBeforeSend != nil {
		if err := request.PrivateProviderCurrentnessBeforeSend(attempt); err != nil {
			return domainmodel.Result{}, false, domaincache.ProviderDispatchStateNotSent,
				withProviderFailureStageV1(err, ProviderFailureStageCallbackProjection)
		}
	}
	requestPreSendAt := time.Now()
	if err := emitProviderPipelineStage(request, "pre_send", requestPreSendAt, providerRequestPipelineDetails(request, endpoint, body, bodyBytes, attempt, map[string]any{
		"provider_request_build_started_at": unixMillis(requestBuildStartedAt),
		"provider_request_pre_send_at":      unixMillis(requestPreSendAt),
	}), map[string]any{
		"provider_request_build_started_at": unixMillis(requestBuildStartedAt),
		"provider_request_pre_send_at":      unixMillis(requestPreSendAt),
	}); err != nil {
		return domainmodel.Result{}, false, domaincache.ProviderDispatchStateNotSent,
			withProviderFailureStageV1(err, ProviderFailureStageCallbackProjection)
	}
	// This timestamp is the bounded host instant immediately before invoking the
	// transport. It is diagnostic latency metadata, not proof that request bytes
	// crossed the socket; the signed dispatchState remains the authority for
	// sent/indeterminate settlement.
	requestSentAt := time.Now()
	var transportTrace *providerTransportTrace
	if request.OnPipelineStage != nil {
		transportTrace = newProviderTransportTrace(requestSentAt)
		httpReq = httpReq.WithContext(httptrace.WithClientTrace(httpReq.Context(), transportTrace.hooks()))
	}
	dispatchState := domaincache.ProviderDispatchStateIndeterminate
	resp, err := httpClient.Do(httpReq)
	if resp != nil {
		dispatchState = domaincache.ProviderDispatchStateSent
	}
	responseHeadersAt := time.Now()
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	postSendDetails := map[string]any{
		"status":                            float64(status),
		"provider_request_pre_send_at":      unixMillis(requestPreSendAt),
		"provider_request_sent_at":          unixMillis(requestSentAt),
		"provider_response_headers_at":      unixMillis(responseHeadersAt),
		"provider_response_headers_wait_ms": float64(responseHeadersAt.Sub(requestSentAt).Milliseconds()),
	}
	transportTrace.snapshotInto(postSendDetails)
	if stageErr := emitProviderPipelineStage(request, "post_send", responseHeadersAt, providerRequestPipelineDetails(request, endpoint, body, bodyBytes, attempt, postSendDetails), map[string]any{
		"provider_request_pre_send_at": unixMillis(requestPreSendAt),
		"provider_request_sent_at":     unixMillis(requestSentAt),
		"provider_response_headers_at": unixMillis(responseHeadersAt),
	}); stageErr != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return domainmodel.Result{}, false, dispatchState,
			withProviderFailureStageV1(stageErr, ProviderFailureStageCallbackProjection)
	}
	if err != nil {
		return domainmodel.Result{}, false, dispatchState, newProviderError(request, endpoint, 0, "network", err.Error(), 0)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		kind := classifyProviderError(resp.StatusCode, string(data))
		return domainmodel.Result{}, false, dispatchState, newProviderError(request, endpoint, resp.StatusCode, kind, publicProviderHTTPErrorMessage(resp.StatusCode, kind), retryAfterFromHeader(resp.Header.Get("Retry-After")))
	}
	bodyReader := newStreamIdleWatchdogBody(ctx, resp.Body, c.effectiveStreamIdleTimeout())
	defer bodyReader.Close()
	outputObservation := domainmodel.ProviderOutputObservationNoneV1
	payloadObserved := false
	chunks, usage, err := providerstream.ParseSSEWithCallbackAndPayloadObservation(parseEndpointFormat, bodyReader, func(chunk domainmodel.Chunk) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if chunk.Trace.ProviderRequestSentAt.IsZero() {
			chunk.Trace.ProviderRequestSentAt = requestSentAt
		}
		if chunk.Trace.ProviderResponseHeadersAt.IsZero() {
			chunk.Trace.ProviderResponseHeadersAt = responseHeadersAt
		}
		outputObservation = outputObservation.MergeV1(observeProviderOutputChunk(chunk))
		if request.OnChunk != nil {
			if callbackErr := request.OnChunk(chunk); callbackErr != nil {
				return withProviderFailureStageV1(callbackErr, ProviderFailureStageCallbackProjection)
			}
		}
		return nil
	}, func(observation providerstream.PayloadObservation) {
		payloadObserved = true
		outputObservation = outputObservation.MergeV1(observation)
	})
	retryUnsafeOutput := outputObservation.RetryUnsafeV1()
	if bodyReader.Stalled() {
		err = fmt.Errorf("provider stream stalled: no data for %s: %w", bodyReader.Timeout(), firstNonNilError(err, io.ErrUnexpectedEOF))
	}
	if err != nil {
		if payloadObserved {
			if outputObservation == domainmodel.ProviderOutputObservationNoneV1 {
				outputObservation = domainmodel.ProviderOutputObservationControlV1
			}
			err = interruptedProviderOutputError{cause: err, observation: outputObservation}
		}
		usage = providerusage.ApplyPricing(usage, request.Pricing)
		return domainmodel.Result{
			ProviderID:        request.ProviderID,
			Family:            request.Family,
			EndpointFormat:    request.EndpointFormat,
			RequestURL:        endpoint,
			RequestBodyFields: sortedMapKeys(body),
			Chunks:            chunks,
			Usage:             usage,
			PrefixShape:       appmodel.CapturePrefixShape(request),
			StreamCompleted:   hasChunkKind(chunks, domainmodel.ChunkDone),
		}, retryUnsafeOutput, dispatchState, err
	}
	usage = providerusage.ApplyPricing(usage, request.Pricing)
	return domainmodel.Result{
		ProviderID:        request.ProviderID,
		Family:            request.Family,
		EndpointFormat:    request.EndpointFormat,
		RequestURL:        endpoint,
		RequestBodyFields: sortedMapKeys(body),
		Chunks:            chunks,
		Usage:             usage,
		PrefixShape:       appmodel.CapturePrefixShape(request),
		StreamCompleted:   hasChunkKind(chunks, domainmodel.ChunkDone),
	}, retryUnsafeOutput, dispatchState, nil
}

func (c *HTTPProviderClient) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return NewDefaultHTTPClient(10 * time.Second)
}

func (c *HTTPProviderClient) httpClientForAttempt(request domainmodel.Request, endpoint string) (*http.Client, func(), error) {
	proxyURL := strings.TrimSpace(request.ProxyURL)
	authoritative := request.PrivateProviderProxyAuthority
	owner := providerTransportOwner{providerID: request.ProviderID, family: request.Family}
	if proxyURL == "" && !authoritative {
		c.transportMu.Lock()
		if c.transportClosed {
			c.transportMu.Unlock()
			return nil, nil, errors.New("provider client is closed")
		}
		if previous := c.transportSlots[owner]; previous != nil {
			delete(c.transportSlots, owner)
			c.retireTransportLocked(previous)
		}
		c.transportMu.Unlock()
		return c.httpClient(), nil, nil
	}
	proxySpec := netclient.ProxySpec{Mode: netclient.ModeOff}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
			parsed.Fragment != "" || parsed.RawFragment != "" {
			return nil, nil, errors.New("provider proxy authority is invalid")
		}
		proxySpec = providerProxySpec(proxyURL)
	}
	endpointURL, err := url.Parse(endpoint)
	if err != nil || endpointURL.Scheme == "" || endpointURL.Host == "" {
		return nil, nil, errors.New("provider endpoint authority is invalid")
	}
	key := providerTransportKey{
		proxyURL: proxyURL, endpointOrigin: endpointURL.Scheme + "://" + endpointURL.Host,
		authoritative: authoritative,
	}
	c.transportMu.Lock()
	if c.transportClosed {
		c.transportMu.Unlock()
		return nil, nil, errors.New("provider client is closed")
	}
	if c.transportSlots == nil {
		c.transportSlots = make(map[providerTransportOwner]*providerTransportSlot)
	}
	if existing := c.transportSlots[owner]; existing != nil {
		if existing.key == key {
			c.transportUse++
			existing.lastUse = c.transportUse
			existing.active++
			c.transportMu.Unlock()
			return existing.client, c.releaseTransport(existing), nil
		}
	}
	client, err := netclient.NewHTTPClient(proxySpec, netclient.TransportOptions{
		DialTimeout:           30 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 120 * time.Second,
	})
	if err != nil {
		c.transportMu.Unlock()
		return nil, nil, errors.New("provider proxy authority is invalid")
	}
	if base := c.httpClient(); base != nil {
		client.Timeout = base.Timeout
		client.Jar = base.Jar
	}
	if existing := c.transportSlots[owner]; existing != nil {
		delete(c.transportSlots, owner)
		c.retireTransportLocked(existing)
	}
	if len(c.transportSlots) >= maxProviderTransportSlots {
		var oldestOwner providerTransportOwner
		var oldest *providerTransportSlot
		for candidateOwner, candidate := range c.transportSlots {
			if oldest == nil || candidate.lastUse < oldest.lastUse {
				oldestOwner, oldest = candidateOwner, candidate
			}
		}
		delete(c.transportSlots, oldestOwner)
		c.retireTransportLocked(oldest)
	}
	c.transportUse++
	slot := &providerTransportSlot{key: key, client: client, active: 1, lastUse: c.transportUse}
	c.transportSlots[owner] = slot
	c.transportMu.Unlock()
	return client, c.releaseTransport(slot), nil
}

func (c *HTTPProviderClient) retireTransportLocked(slot *providerTransportSlot) {
	slot.retired = true
	if slot.active == 0 {
		slot.client.CloseIdleConnections()
	}
}

func (c *HTTPProviderClient) releaseTransport(slot *providerTransportSlot) func() {
	return func() {
		c.transportMu.Lock()
		slot.active--
		if slot.retired && slot.active == 0 {
			slot.client.CloseIdleConnections()
		}
		c.transportMu.Unlock()
	}
}

// Close retires idle transports without changing the cancellation or authority
// of an in-flight request. The runtime owner drains those operations first;
// any late release closes its retired transport and cannot repopulate the pool.
func (c *HTTPProviderClient) Close() error {
	if c == nil {
		return nil
	}
	c.transportMu.Lock()
	c.transportClosed = true
	for _, slot := range c.transportSlots {
		c.retireTransportLocked(slot)
	}
	c.transportSlots = nil
	c.transportMu.Unlock()
	if c.HTTP != nil {
		c.HTTP.CloseIdleConnections()
	}
	return nil
}

func (c *HTTPProviderClient) effectiveStreamIdleTimeout() time.Duration {
	if c == nil || c.StreamIdleTimeout < 0 {
		return DefaultStreamIdleTimeout
	}
	return c.StreamIdleTimeout
}

func (c *HTTPProviderClient) effectiveMaxStreamReconnects() int {
	if c == nil {
		return DefaultStreamMaxReconnects
	}
	if c.MaxStreamReconnects < 0 {
		return 0
	}
	return c.MaxStreamReconnects
}

func buildHTTPRequest(request domainmodel.Request) (string, map[string]any, map[string]string, error) {
	prepared, err := providercompat.BuildHTTPRequest(request)
	return prepared.RequestURL, prepared.Body, prepared.Headers, err
}

func providerProxySpec(proxyURL string) netclient.ProxySpec {
	if strings.TrimSpace(proxyURL) == "" {
		return netclient.ProxySpec{Mode: netclient.ModeAuto}
	}
	return netclient.ProxySpec{Mode: netclient.ModeCustom, URL: strings.TrimSpace(proxyURL)}
}

func firstNonNilError(err error, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}

func observeProviderOutputChunk(chunk domainmodel.Chunk) domainmodel.ProviderOutputObservationV1 {
	switch chunk.Kind {
	case domainmodel.ChunkReasoning:
		if chunk.Text != "" || chunk.Signature != "" {
			return domainmodel.ProviderOutputObservationPrivateReasoningV1
		}
	case domainmodel.ChunkText:
		if chunk.Text != "" {
			return domainmodel.ProviderOutputObservationPublicTextV1
		}
	case domainmodel.ChunkToolCallStart, domainmodel.ChunkToolCall:
		return domainmodel.ProviderOutputObservationToolStartedV1
	case domainmodel.ChunkUsage, domainmodel.ChunkDone:
		return domainmodel.ProviderOutputObservationControlV1
	default:
	}
	return domainmodel.ProviderOutputObservationNoneV1
}

func isPreOutputStreamReplayableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var publicFailure interface{ PublicFailureRecord() domainfailure.Record }
	if errors.As(err, &publicFailure) && publicFailure.PublicFailureRecord().Code() == domainfailure.CodeProviderStreamInterrupted {
		return true
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "stream stalled"),
		strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, "ended before completion"),
		strings.Contains(msg, "ended before completing tool calls"),
		strings.Contains(msg, "use of closed network connection"),
		strings.Contains(msg, "response body closed"):
		return true
	default:
		return false
	}
}

func emitProviderPipelineStage(request domainmodel.Request, stage string, at time.Time, details map[string]any, trace map[string]any) error {
	if request.OnPipelineStage == nil {
		return nil
	}
	return request.OnPipelineStage(domainmodel.PipelineStage{
		Stage:   stage,
		At:      at,
		Details: details,
		Trace:   trace,
	})
}

func providerRequestPipelineDetails(request domainmodel.Request, endpoint string, body map[string]any, bodyBytes []byte, attempt int, extra map[string]any) map[string]any {
	details := map[string]any{
		"attempt":                      float64(attempt),
		"physicalAttempt":              float64(attempt),
		"endpointFormat":               request.EndpointFormat,
		"messageCount":                 float64(len(request.Messages)),
		"historyItems":                 float64(historyItemCount(request.Messages)),
		"contextInstructionCount":      float64(contextInstructionCount(request.Messages)),
		"toolCount":                    float64(len(request.Tools)),
		"toolSchemaBytes":              float64(toolSchemaBytes(request.Tools)),
		"systemBytes":                  float64(messageRoleBytes(request.Messages, "system")),
		"userBytes":                    float64(messageRoleBytes(request.Messages, "user")),
		"assistantBytes":               float64(messageRoleBytes(request.Messages, "assistant")),
		"toolBytes":                    float64(messageRoleBytes(request.Messages, "tool")),
		"requestBodyBytes":             float64(len(bodyBytes)),
		"hasStreamOptionsIncludeUsage": hasStreamOptionsIncludeUsage(body),
	}
	if binding := request.PrivateProviderTelemetry; binding != nil {
		details["logicalSequence"] = float64(binding.LogicalSequence)
		details["outerAttempt"] = float64(binding.OuterAttempt)
		details["laneCallSequence"] = float64(binding.LaneCallSequence)
	}
	if extra != nil {
		for key, value := range extra {
			details[key] = value
		}
	}
	return details
}

func historyItemCount(messages []domainmodel.Message) int {
	count := 0
	for index, message := range messages {
		if index == 0 && message.Role == "system" {
			continue
		}
		count++
	}
	return count
}

func contextInstructionCount(messages []domainmodel.Message) int {
	count := 0
	for _, message := range messages {
		if message.Role == "system" && strings.TrimSpace(message.Content) != "" {
			count++
		}
	}
	return count
}

func messageRoleBytes(messages []domainmodel.Message, role string) int {
	total := 0
	for _, message := range messages {
		if message.Role != role {
			continue
		}
		total += len([]byte(message.Content))
		for _, part := range message.Parts {
			total += len([]byte(part.Text))
			total += len([]byte(part.ImageURL))
			total += len([]byte(part.Data))
		}
	}
	return total
}

func toolSchemaBytes(tools []domainmodel.ToolSchema) int {
	total := 0
	for _, tool := range tools {
		total += len([]byte(tool.Name))
		total += len([]byte(tool.Description))
		total += len(tool.Parameters)
	}
	return total
}

func hasStreamOptionsIncludeUsage(body map[string]any) bool {
	options, ok := body["stream_options"].(map[string]any)
	if !ok {
		return false
	}
	value, ok := options["include_usage"].(bool)
	return ok && value
}

func unixMillis(t time.Time) float64 {
	if t.IsZero() {
		return 0
	}
	return float64(t.UTC().UnixNano()) / float64(time.Millisecond)
}

type streamIdleWatchdogBody struct {
	body     io.ReadCloser
	timeout  time.Duration
	activity chan struct{}
	done     chan struct{}
	once     sync.Once
	stalled  atomic.Bool
}

func newStreamIdleWatchdogBody(ctx context.Context, body io.ReadCloser, timeout time.Duration) *streamIdleWatchdogBody {
	watcher := &streamIdleWatchdogBody{
		body:     body,
		timeout:  timeout,
		activity: make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	if timeout > 0 {
		go watcher.watch(ctx)
	}
	return watcher
}

func (b *streamIdleWatchdogBody) Read(p []byte) (int, error) {
	n, err := b.body.Read(p)
	if n > 0 {
		b.ping()
	}
	return n, err
}

func (b *streamIdleWatchdogBody) Close() error {
	var err error
	b.once.Do(func() {
		close(b.done)
		err = b.body.Close()
	})
	return err
}

func (b *streamIdleWatchdogBody) Stalled() bool {
	return b.stalled.Load()
}

func (b *streamIdleWatchdogBody) Timeout() time.Duration {
	return b.timeout
}

func (b *streamIdleWatchdogBody) ping() {
	select {
	case b.activity <- struct{}{}:
	default:
	}
}

func (b *streamIdleWatchdogBody) watch(ctx context.Context) {
	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = b.body.Close()
			return
		case <-timer.C:
			b.stalled.Store(true)
			_ = b.body.Close()
			return
		case <-b.activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(b.timeout)
		case <-b.done:
			return
		}
	}
}

func newProviderError(request domainmodel.Request, endpoint string, status int, kind string, message string, retryAfter time.Duration) *ProviderError {
	return &ProviderError{
		ProviderID:     strings.TrimSpace(request.ProviderID),
		Model:          strings.TrimSpace(request.Model),
		Family:         strings.TrimSpace(request.Family),
		EndpointFormat: strings.TrimSpace(request.EndpointFormat),
		BaseURL:        sanitizeProviderURL(request.BaseURL, request.APIKey),
		RequestURL:     sanitizeProviderURL(endpoint, request.APIKey),
		Status:         status,
		Kind:           strings.TrimSpace(kind),
		Message:        sanitizeProviderSecretText(message, request.APIKey),
		HasAPIKey:      strings.TrimSpace(request.APIKey) != "",
		RetryAfterMs:   int(retryAfter / time.Millisecond),
	}
}

func retryAfterFromHeader(raw string) time.Duration {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil || seconds <= 0 {
		return 0
	}
	delay := time.Duration(seconds) * time.Second
	if delay > 15*time.Second {
		return 15 * time.Second
	}
	return delay
}

func classifyProviderError(status int, body string) string {
	lower := strings.ToLower(body)
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "auth"
	case strings.Contains(lower, "unauthorized"),
		strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "invalid token"),
		strings.Contains(lower, "authentication"),
		strings.Contains(lower, "not authenticated"):
		return "auth"
	case status == http.StatusTooManyRequests:
		return "rate_limit"
	case status == http.StatusPaymentRequired ||
		strings.Contains(lower, "insufficient balance") ||
		strings.Contains(lower, "insufficient credit") ||
		strings.Contains(lower, "out of credit") ||
		strings.Contains(lower, "余额不足"):
		return "insufficient_balance"
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return "request"
	case status >= 500:
		return "server"
	default:
		return "http"
	}
}

func publicProviderHTTPErrorMessage(status int, kind string) string {
	switch {
	case status == http.StatusNotFound:
		return "model request endpoint was not found; check Provider Base URL and Endpoint format"
	case kind == "auth":
		return "provider authentication failed; check the configured credential"
	case kind == "rate_limit":
		return "provider rate limit was reached; retry after the reported delay"
	case kind == "insufficient_balance":
		return "provider account balance or credit is insufficient"
	case status >= 500:
		return "provider service returned a server error; response body was withheld"
	default:
		return "provider rejected the model request; response body was withheld"
	}
}

func sanitizeProviderURL(raw string, apiKey string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil || parsed.Scheme == "" {
		return sanitizeProviderSecretText(trimmed, apiKey)
	}
	if parsed.User != nil {
		parsed.User = url.User("<redacted>")
	}
	query := parsed.Query()
	for key := range query {
		if isProviderAuthQueryKey(key) {
			query.Set(key, "<redacted>")
		}
	}
	parsed.RawQuery = query.Encode()
	return sanitizeProviderSecretText(parsed.String(), apiKey)
}

func sanitizeProviderSecretText(text string, apiKey string) string {
	value := strings.TrimSpace(text)
	if value == "" {
		return ""
	}
	if key := strings.TrimSpace(apiKey); len(key) >= 4 {
		value = strings.ReplaceAll(value, key, "<redacted>")
	}
	value = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`).ReplaceAllString(value, "<redacted>")
	value = regexp.MustCompile(`(?i)\b(https?|socks5h?|socks4a?|socks4|socks)://([^:/@\s]+):([^@/\s]+)@`).ReplaceAllString(value, "$1://$2:<redacted>@")
	value = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`).ReplaceAllString(value, "Bearer <redacted>")
	value = regexp.MustCompile(`(?i)(authorization|x-api-key|api[_ -]?key|apikey|token)\s*[:=]\s*["']?[^"',\s}]+`).ReplaceAllString(value, "$1=<redacted>")
	if len([]rune(value)) > 512 {
		runes := []rune(value)
		value = string(runes[:512]) + "..."
	}
	return strings.TrimSpace(value)
}

func isProviderAuthQueryKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return lower == "key" ||
		strings.Contains(lower, "auth") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "credential") ||
		strings.Contains(lower, "api_key") ||
		strings.Contains(lower, "api-key") ||
		strings.Contains(lower, "apikey")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func hasChunkKind(chunks []domainmodel.Chunk, kind domainmodel.ChunkKind) bool {
	for _, chunk := range chunks {
		if chunk.Kind == kind {
			return true
		}
	}
	return false
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
