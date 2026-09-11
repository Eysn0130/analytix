package client

import (
	"context"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	appcache "analytix.local/runtime-go/internal/app/cachetelemetry"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	cachetelemetryport "analytix.local/runtime-go/internal/ports/cachetelemetry"
)

type cacheDigestAuthority struct {
	key   [32]byte
	epoch uint64
}

type providerCallTelemetry struct {
	ledger              *appcache.Service
	authority           *cacheDigestAuthority
	baseShape           domaincache.CacheVisibleShapeV1
	durable             cachetelemetryport.Recorder
	registration        cachetelemetryport.AttemptRegistrationInputV1
	handles             map[uint32]cachetelemetryport.AttemptHandleV1
	durableObservations []domaincache.ProviderCallObservationV1
}

func (c *HTTPProviderClient) newProviderCallTelemetry(
	request domainmodel.Request,
	endpoint string,
	wireBody []byte,
	wireHeaders []byte,
) (*providerCallTelemetry, error) {
	reasoningEffort, valid := domainmodel.ProjectReasoningEffortV1(request.ReasoningEffort)
	if !valid {
		return nil, domainmodel.ErrInvalidReasoningEffort
	}
	providerFamily, err := cacheProviderFamily(request)
	if err != nil {
		return nil, err
	}
	endpointFormat, err := cacheEndpointFormat(request.EndpointFormat)
	if err != nil {
		return nil, err
	}
	providerConfigBody, err := json.Marshal(struct {
		ProviderID        string `json:"providerId"`
		Family            string `json:"family"`
		EndpointFormat    string `json:"endpointFormat"`
		BaseURL           string `json:"baseUrl"`
		Model             string `json:"model"`
		Route             string `json:"route"`
		ReasoningEffort   string `json:"reasoningEffort"`
		ReasoningProtocol string `json:"reasoningProtocol"`
	}{
		ProviderID: strings.TrimSpace(request.ProviderID), Family: strings.TrimSpace(request.Family),
		EndpointFormat: domainmodel.NormalizeEndpointFormat(request.EndpointFormat), BaseURL: strings.TrimSpace(request.BaseURL),
		Model: strings.TrimSpace(request.Model), Route: strings.TrimSpace(request.Route),
		ReasoningEffort: reasoningEffort, ReasoningProtocol: strings.TrimSpace(request.ReasoningProtocol),
	})
	if err != nil {
		return nil, errors.New("provider cache telemetry configuration cannot be encoded")
	}
	credentialScope, err := json.Marshal(struct {
		ProviderID string `json:"providerId"`
		APIKey     string `json:"apiKey"`
	}{ProviderID: strings.TrimSpace(request.ProviderID), APIKey: request.APIKey})
	if err != nil {
		return nil, errors.New("provider cache telemetry credential scope cannot be encoded")
	}
	if c != nil && c.telemetryRecorder != nil {
		binding := request.PrivateProviderTelemetry
		if binding == nil || binding.OuterAttempt == 0 {
			return nil, errors.New("production provider cache telemetry binding is required")
		}
		return &providerCallTelemetry{
			durable: c.telemetryRecorder, handles: make(map[uint32]cachetelemetryport.AttemptHandleV1),
			registration: cachetelemetryport.AttemptRegistrationInputV1{
				SecurityContext: binding.SecurityContext, OrdinaryEffect: binding.OrdinaryEffect,
				UsageSource: binding.UsageSource,
				ChildRunID:  []byte(binding.ChildRunID), Channel: binding.Channel,
				LogicalSequence: binding.LogicalSequence, OuterAttempt: binding.OuterAttempt,
				LaneCallSequence: binding.LaneCallSequence,
				ProviderFamily:   providerFamily, EndpointFormat: endpointFormat,
				Model: []byte(request.Model), Endpoint: []byte(endpoint), WireBody: append([]byte(nil), wireBody...),
				WireHeaders: append([]byte(nil), wireHeaders...), CredentialScope: credentialScope, ProviderConfig: providerConfigBody,
			},
		}, nil
	}
	if c != nil && c.productionTelemetry {
		return nil, errors.New("production provider cache telemetry recorder is unavailable")
	}
	authority, err := c.loadCacheDigestAuthority()
	if err != nil {
		return nil, err
	}
	logicalSequence := c.cacheLogicalCalls.Add(1)
	if logicalSequence == 0 {
		return nil, errors.New("provider cache telemetry logical-call sequence is exhausted")
	}
	credentialScopeHMAC := authority.digest("credential-scope", credentialScope)
	sequenceBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(sequenceBytes, logicalSequence)
	return &providerCallTelemetry{
		ledger:    appcache.NewService(),
		authority: authority,
		baseShape: domaincache.CacheVisibleShapeV1{
			SchemaVersion:       domaincache.CacheVisibleShapeV1SchemaVersion,
			LogicalCallHMAC:     authority.digest("logical-call", sequenceBytes),
			ProviderFamily:      providerFamily,
			ModelHMAC:           authority.digest("model", []byte(request.Model)),
			Endpoint:            endpointFormat,
			EndpointHMAC:        authority.digest("endpoint", []byte(endpoint)),
			WireBodyHMAC:        authority.digest("wire-body", wireBody),
			CredentialScopeHMAC: credentialScopeHMAC,
			ProviderConfigHMAC:  authority.digest("provider-config", providerConfigBody),
			DigestEpoch:         authority.epoch,
		},
	}, nil
}

func (call *providerCallTelemetry) begin(ctx context.Context, attempt int, startedAt time.Time) (domaincache.CacheVisibleShapeV1, error) {
	if call == nil || attempt <= 0 {
		return domaincache.CacheVisibleShapeV1{}, errors.New("provider cache telemetry call authority is unavailable")
	}
	if call.durable != nil {
		input := call.registration
		input.PhysicalAttempt = uint32(attempt)
		input.StartedAt = startedAt.UTC()
		handle, err := call.durable.BeginAttempt(ctx, input)
		if err != nil {
			return domaincache.CacheVisibleShapeV1{}, err
		}
		call.handles[uint32(attempt)] = handle
		return handle.Intent.Shape, nil
	}
	if call.ledger == nil || call.authority == nil {
		return domaincache.CacheVisibleShapeV1{}, errors.New("provider cache telemetry call authority is unavailable")
	}
	shape := call.baseShape
	shape.Attempt = uint32(attempt)
	shape.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	if err := call.ledger.Begin(shape); err != nil {
		return domaincache.CacheVisibleShapeV1{}, err
	}
	return shape, nil
}

func (call *providerCallTelemetry) settle(
	ctx context.Context,
	shape domaincache.CacheVisibleShapeV1,
	result domainmodel.Result,
	emitted bool,
	streamErr error,
	contextErr error,
	dispatchState domaincache.ProviderDispatchStateV1,
	settledAt time.Time,
) ([]domaincache.ProviderCallObservationV1, error) {
	if call == nil {
		return nil, errors.New("provider cache telemetry call authority is unavailable")
	}
	startedAt, _ := time.Parse(time.RFC3339Nano, shape.StartedAt)
	if settledAt.Before(startedAt) {
		settledAt = startedAt
	}
	usage, err := cacheProviderUsage(result, shape.ProviderFamily)
	if err != nil {
		return nil, err
	}
	observation := domaincache.ProviderCallObservationV1{
		SchemaVersion: domaincache.ProviderCallObservationV1SchemaVersion,
		Shape:         shape,
		Status:        cacheProviderCallStatus(streamErr, contextErr, emitted),
		Usage:         usage,
		SettledAt:     settledAt.UTC().Format(time.RFC3339Nano),
	}
	if call.durable != nil {
		handle, found := call.handles[shape.Attempt]
		if !found || handle.Intent.Shape != shape {
			return nil, errors.New("provider cache telemetry durable handle is unavailable")
		}
		if err := call.durable.SettleAttempt(ctx, cachetelemetryport.AttemptSettlementInputV1{
			Handle: handle, DispatchState: dispatchState, Status: observation.Status, Usage: observation.Usage,
			SafeReasonCode: cacheProviderSettlementReasonCode(observation.Status, dispatchState), SettledAt: settledAt,
		}); err != nil {
			return nil, err
		}
		delete(call.handles, shape.Attempt)
		call.durableObservations = append(call.durableObservations, observation)
		return append([]domaincache.ProviderCallObservationV1(nil), call.durableObservations...), nil
	}
	if call.ledger == nil {
		return nil, errors.New("provider cache telemetry call authority is unavailable")
	}
	if err := call.ledger.Settle(observation); err != nil {
		return nil, err
	}
	snapshot, err := call.ledger.Snapshot()
	if err != nil {
		return nil, err
	}
	return snapshot.Observations, nil
}

func cacheProviderSettlementReasonCode(status domaincache.ProviderCallStatusV1, dispatch domaincache.ProviderDispatchStateV1) string {
	switch status {
	case domaincache.ProviderCallStatusSucceeded:
		return "provider_succeeded"
	case domaincache.ProviderCallStatusCancelled:
		return "provider_cancelled"
	case domaincache.ProviderCallStatusTimedOut:
		return "provider_timed_out"
	case domaincache.ProviderCallStatusStreamAborted:
		return "provider_stream_aborted"
	case domaincache.ProviderCallStatusRestartInterrupted:
		return "restart_interrupted"
	default:
		if dispatch == domaincache.ProviderDispatchStateNotSent {
			return "provider_not_sent"
		}
		if dispatch == domaincache.ProviderDispatchStateIndeterminate {
			return "provider_send_indeterminate"
		}
		return "provider_failed"
	}
}

func (c *HTTPProviderClient) loadCacheDigestAuthority() (*cacheDigestAuthority, error) {
	if c == nil {
		return nil, errors.New("provider cache telemetry authority is unavailable")
	}
	c.cacheAuthorityMu.Lock()
	defer c.cacheAuthorityMu.Unlock()
	if c.cacheAuthority != nil {
		return c.cacheAuthority, nil
	}
	authority := &cacheDigestAuthority{}
	if _, err := cryptorand.Read(authority.key[:]); err != nil {
		return nil, errors.New("provider cache telemetry authority is unavailable")
	}
	epochBytes := make([]byte, 8)
	if _, err := cryptorand.Read(epochBytes); err != nil {
		return nil, errors.New("provider cache telemetry authority is unavailable")
	}
	authority.epoch = binary.BigEndian.Uint64(epochBytes)
	if authority.epoch == 0 {
		authority.epoch = 1
	}
	c.cacheAuthority = authority
	return authority, nil
}

func (authority *cacheDigestAuthority) digest(label string, parts ...[]byte) string {
	mac := hmac.New(sha256.New, authority.key[:])
	writeLengthPrefixed(mac, []byte("analytix.cache-visible-shape.v1"))
	writeLengthPrefixed(mac, []byte(label))
	for _, part := range parts {
		writeLengthPrefixed(mac, part)
	}
	return hex.EncodeToString(mac.Sum(nil))
}

type hashWriter interface {
	Write([]byte) (int, error)
}

func writeLengthPrefixed(writer hashWriter, value []byte) {
	length := make([]byte, 8)
	binary.BigEndian.PutUint64(length, uint64(len(value)))
	_, _ = writer.Write(length)
	_, _ = writer.Write(value)
}

func cacheProviderFamily(request domainmodel.Request) (domaincache.ProviderFamilyV1, error) {
	family := strings.ToLower(strings.TrimSpace(request.Family))
	format := domainmodel.NormalizeEndpointFormat(request.EndpointFormat)
	switch {
	case family == "deepseek":
		return domaincache.ProviderFamilyDeepSeek, nil
	case format == "custom_endpoint":
		return domaincache.ProviderFamilyCustomEndpoint, nil
	case format == "messages" || family == "anthropic" || family == "anthropic_compatible":
		return domaincache.ProviderFamilyAnthropicCompatible, nil
	case format == "chat_completions" || format == "responses":
		return domaincache.ProviderFamilyOpenAICompatible, nil
	default:
		return "", errors.New("provider cache telemetry family is unsupported")
	}
}

func cacheEndpointFormat(format string) (domaincache.EndpointFormatV1, error) {
	switch domainmodel.NormalizeEndpointFormat(format) {
	case "chat_completions":
		return domaincache.EndpointFormatChatCompletions, nil
	case "responses":
		return domaincache.EndpointFormatResponses, nil
	case "messages":
		return domaincache.EndpointFormatMessages, nil
	case "custom_endpoint":
		return domaincache.EndpointFormatCustomEndpoint, nil
	default:
		return "", errors.New("provider cache telemetry endpoint format is unsupported")
	}
}

func cacheProviderCallStatus(streamErr error, contextErr error, emitted bool) domaincache.ProviderCallStatusV1 {
	if errors.Is(contextErr, context.DeadlineExceeded) {
		return domaincache.ProviderCallStatusTimedOut
	}
	if errors.Is(contextErr, context.Canceled) {
		return domaincache.ProviderCallStatusCancelled
	}
	if streamErr == nil {
		return domaincache.ProviderCallStatusSucceeded
	}
	message := strings.ToLower(streamErr.Error())
	if errors.Is(streamErr, context.DeadlineExceeded) ||
		strings.Contains(message, "stream stalled") || strings.Contains(message, "client.timeout exceeded") ||
		strings.Contains(message, "context deadline exceeded") {
		return domaincache.ProviderCallStatusTimedOut
	}
	if errors.Is(streamErr, context.Canceled) {
		return domaincache.ProviderCallStatusCancelled
	}
	if emitted {
		return domaincache.ProviderCallStatusStreamAborted
	}
	return domaincache.ProviderCallStatusFailed
}

func cacheProviderUsage(result domainmodel.Result, family domaincache.ProviderFamilyV1) (domaincache.ProviderUsageV1, error) {
	usageObserved := false
	for _, chunk := range result.Chunks {
		if chunk.Kind == domainmodel.ChunkUsage {
			usageObserved = true
			break
		}
	}
	if result.Usage.PromptTokens != 0 || result.Usage.CompletionTokens != 0 || result.Usage.TotalTokens != 0 {
		usageObserved = true
	}
	if result.Usage.PromptTokens < 0 || result.Usage.CompletionTokens < 0 ||
		result.Usage.CacheHitTokens < 0 || result.Usage.CacheMissTokens < 0 || result.Usage.ReasoningTokens < 0 {
		return domaincache.ProviderUsageV1{}, errors.New("provider cache telemetry counters are invalid")
	}
	usage := domaincache.ProviderUsageV1{
		InputTokens:     cacheTokenCount(result.Usage.PromptTokens, usageObserved),
		OutputTokens:    cacheTokenCount(result.Usage.CompletionTokens, usageObserved),
		CacheHitTokens:  cacheTokenCount(result.Usage.CacheHitTokens, result.Usage.HasCacheHit),
		CacheMissTokens: cacheTokenCount(result.Usage.CacheMissTokens, result.Usage.HasCacheMiss),
		ReasoningTokens: cacheTokenCount(result.Usage.ReasoningTokens, result.Usage.ReasoningTokens > 0),
	}
	if err := usage.ValidateForProviderFamily(family); err != nil {
		return domaincache.ProviderUsageV1{}, err
	}
	return usage, nil
}

func cacheTokenCount(value int, known bool) domaincache.TokenCountV1 {
	if !known || value < 0 {
		return domaincache.TokenCountV1{}
	}
	return domaincache.TokenCountV1{Known: true, Value: uint64(value)}
}
