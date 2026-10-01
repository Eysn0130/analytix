package runtimeapp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	providerregistryapp "analytix.local/runtime-go/internal/app/providerregistry"
	domainregistry "analytix.local/runtime-go/internal/domain/providerregistry"
	registryport "analytix.local/runtime-go/internal/ports/providerregistry"
	secretstoreport "analytix.local/runtime-go/internal/ports/secretstore"
)

const (
	providerRegistryProbeTimeoutV1            = 5 * time.Second
	providerRegistryProbeMaxBodyBytesV1       = 1 << 20
	providerRegistryObservationMaxBodyBytesV1 = 64 << 10
	providerRegistryObservationTTLMaxV1       = time.Hour
)

type providerRegistryOperationsV1 struct {
	*providerregistryapp.Manager
	now func() time.Time
}

func newProviderRegistryOperationsV1(manager *providerregistryapp.Manager) *providerRegistryOperationsV1 {
	return &providerRegistryOperationsV1{Manager: manager, now: time.Now}
}

func (service *providerRegistryOperationsV1) Probe(
	ctx context.Context,
	command providerregistryapp.ProviderOperationCommand,
) (providerregistryapp.ProviderProbeResult, error) {
	resolution, err := service.resolve(ctx, command)
	if err != nil {
		return providerregistryapp.ProviderProbeResult{}, err
	}
	defer resolution.Clear()

	started := service.now()
	outcome := executeProviderRegistryProbeV1(ctx, resolution.Provider, resolution.Credential)
	latency := service.now().Sub(started)
	if latency < 0 {
		latency = 0
	}
	if err := service.Manager.ValidateProviderOperationCurrent(ctx, command); err != nil {
		return providerregistryapp.ProviderProbeResult{}, err
	}
	return providerregistryapp.ProviderProbeResult{
		RegistryRevision: resolution.RegistryRevision, RegistryIncarnation: resolution.RegistryIncarnation,
		ProviderRevision: resolution.Provider.Revision, ProviderGeneration: resolution.Provider.Generation,
		ProviderIncarnation: resolution.Provider.Incarnation,
		Status:              outcome.status, Code: outcome.code, ModelCount: len(outcome.models),
		LatencyMillis: uint64(latency / time.Millisecond),
	}, nil
}

func (service *providerRegistryOperationsV1) DiscoverModels(
	ctx context.Context,
	command providerregistryapp.ProviderOperationCommand,
) (domainregistry.Provider, error) {
	resolution, err := service.resolve(ctx, command)
	if err != nil {
		return domainregistry.Provider{}, err
	}
	outcome := executeProviderRegistryProbeV1(ctx, resolution.Provider, resolution.Credential)
	resolution.Clear()
	if outcome.status != providerregistryapp.ProviderProbeStatusReachable || len(outcome.models) == 0 {
		return domainregistry.Provider{}, registryport.ErrVerification
	}
	if err := service.Manager.ValidateProviderOperationCurrent(ctx, command); err != nil {
		return domainregistry.Provider{}, err
	}

	provider := resolution.Provider
	provider.Models = slices.Clone(outcome.models)
	if provider.SelectedModel != "" && !slices.Contains(provider.Models, provider.SelectedModel) {
		provider.SelectedModel = ""
	}
	return service.Manager.Update(ctx, providerregistryapp.UpdateCommand{
		Expected: command.Expected,
		Provider: domainregistry.ProviderInput{
			ID: provider.ID, Kind: provider.Kind, Endpoint: provider.Endpoint, Proxy: provider.Proxy,
			Models: provider.Models, MediaModels: provider.MediaModels,
			SelectedModel: provider.SelectedModel, SelectedMedia: provider.SelectedMedia,
			SelectedRoutes: provider.SelectedRoutes, OAuthBinding: provider.OAuthBinding,
			AccountObservation: provider.AccountObservation,
		},
		Credential: secretstoreport.KeepCredential(),
	})
}

func (service *providerRegistryOperationsV1) ObserveAccount(
	ctx context.Context,
	command providerregistryapp.ProviderOperationCommand,
) (providerregistryapp.ProviderAccountObservationResult, error) {
	resolution, err := service.resolve(ctx, command)
	if err != nil {
		return providerregistryapp.ProviderAccountObservationResult{}, err
	}
	defer resolution.Clear()
	observedAt := service.now().UTC()
	result := providerregistryapp.ProviderAccountObservationResult{
		RegistryRevision: resolution.RegistryRevision, RegistryIncarnation: resolution.RegistryIncarnation,
		ProviderID: resolution.Provider.ID, ProviderRevision: resolution.Provider.Revision,
		ProviderGeneration: resolution.Provider.Generation, ProviderIncarnation: resolution.Provider.Incarnation,
		ProviderCredentialPurpose: resolution.Provider.CredentialPurpose,
		Status:                    providerregistryapp.ProviderAccountObservationUnavailable,
		ObservedAt:                observedAt, ExpiresAt: observedAt.Add(5 * time.Minute),
	}
	if resolution.Provider.AccountObservation == nil {
		if err := service.Manager.ValidateProviderOperationCurrent(ctx, command); err != nil {
			return providerregistryapp.ProviderAccountObservationResult{}, err
		}
		return result, nil
	}
	if err := service.Manager.ValidateProviderOperationCurrent(ctx, command); err != nil {
		return providerregistryapp.ProviderAccountObservationResult{}, err
	}
	outcome := executeProviderRegistryAccountObservationV1(
		ctx, resolution.Provider, resolution.Credential, observedAt,
	)
	if err := service.Manager.ValidateProviderOperationCurrent(ctx, command); err != nil {
		return providerregistryapp.ProviderAccountObservationResult{}, err
	}
	result.Status = outcome.status
	result.ExpiresAt = outcome.expiresAt
	result.Quota = outcome.quota
	result.Usage = outcome.usage
	result.Remaining = outcome.remaining
	return result, nil
}

type providerRegistryAccountObservationOutcomeV1 struct {
	status    providerregistryapp.ProviderAccountObservationStatus
	expiresAt time.Time
	quota     *float64
	usage     *float64
	remaining *float64
}

func executeProviderRegistryAccountObservationV1(
	ctx context.Context,
	provider domainregistry.Provider,
	credential []byte,
	observedAt time.Time,
) providerRegistryAccountObservationOutcomeV1 {
	closed := providerRegistryAccountObservationOutcomeV1{
		status:    providerregistryapp.ProviderAccountObservationInvalidResponse,
		expiresAt: observedAt.Add(5 * time.Minute),
	}
	binding := provider.AccountObservation
	if binding == nil || binding.Validate() != nil || len(credential) == 0 {
		return closed
	}
	client, err := providerRegistryProbeClientV1("")
	if err != nil {
		closed.status = providerregistryapp.ProviderAccountObservationUnavailable
		return closed
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, binding.Endpoint, nil)
	if err != nil {
		return closed
	}
	request.Header.Set("Accept", "application/json")
	if providerRegistryUsesAnthropicCredentialV1(provider.Kind) {
		request.Header.Set("anthropic-version", "2023-06-01")
		request.Header.Set("x-api-key", string(credential))
	} else {
		request.Header.Set("Authorization", "Bearer "+string(credential))
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			closed.status = providerregistryapp.ProviderAccountObservationTimeout
			return closed
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			closed.status = providerregistryapp.ProviderAccountObservationTimeout
		} else {
			closed.status = providerregistryapp.ProviderAccountObservationUnavailable
		}
		return closed
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		closed.status = providerregistryapp.ProviderAccountObservationRedirectBlocked
		return closed
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		closed.status = providerregistryapp.ProviderAccountObservationAuthFailed
		return closed
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		closed.status = providerregistryapp.ProviderAccountObservationProviderError
		return closed
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, providerRegistryObservationMaxBodyBytesV1+1))
	if err != nil || len(body) > providerRegistryObservationMaxBodyBytesV1 ||
		providerRegistryResponseContainsCredentialV1(body, credential) {
		clear(body)
		return closed
	}
	defer clear(body)
	var payload struct {
		SchemaVersion int      `json:"schemaVersion"`
		Quota         *float64 `json:"quota,omitempty"`
		Usage         *float64 `json:"usage,omitempty"`
		Remaining     *float64 `json:"remaining,omitempty"`
		ExpiresAt     string   `json:"expiresAt,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&payload) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) ||
		payload.SchemaVersion != 1 || !validProviderRegistryObservationValueV1(payload.Quota) ||
		!validProviderRegistryObservationValueV1(payload.Usage) ||
		!validProviderRegistryObservationValueV1(payload.Remaining) ||
		(payload.Quota == nil && payload.Usage == nil && payload.Remaining == nil) {
		return closed
	}
	expiresAt := observedAt.Add(5 * time.Minute)
	if payload.ExpiresAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, payload.ExpiresAt)
		if parseErr != nil || parsed.Before(observedAt) || parsed.After(observedAt.Add(providerRegistryObservationTTLMaxV1)) {
			return closed
		}
		expiresAt = parsed.UTC()
	}
	return providerRegistryAccountObservationOutcomeV1{
		status:    providerregistryapp.ProviderAccountObservationAvailable,
		expiresAt: expiresAt, quota: payload.Quota, usage: payload.Usage, remaining: payload.Remaining,
	}
}

func validProviderRegistryObservationValueV1(value *float64) bool {
	return value == nil || !math.IsNaN(*value) && !math.IsInf(*value, 0) && *value >= 0 && *value <= 1e15
}

func (service *providerRegistryOperationsV1) resolve(
	ctx context.Context,
	command providerregistryapp.ProviderOperationCommand,
) (providerregistryapp.ExecutionResolution, error) {
	if service == nil || service.Manager == nil || service.now == nil || ctx == nil || ctx.Err() != nil {
		return providerregistryapp.ExecutionResolution{}, registryport.ErrInvalidRequest
	}
	return service.Manager.ResolveProviderForOperation(ctx, command)
}

type providerRegistryProbeOutcomeV1 struct {
	status providerregistryapp.ProviderProbeStatus
	code   uint16
	models []string
}

func executeProviderRegistryProbeV1(
	ctx context.Context,
	provider domainregistry.Provider,
	credential []byte,
) providerRegistryProbeOutcomeV1 {
	requestURL, err := providerRegistryModelsURLV1(provider)
	if err != nil || len(credential) == 0 {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusInvalidResponse}
	}
	client, err := providerRegistryProbeClientV1(provider.Proxy)
	if err != nil {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusInvalidResponse}
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusInvalidResponse}
	}
	request.Header.Set("Accept", "application/json")
	if providerRegistryUsesAnthropicCredentialV1(provider.Kind) {
		request.Header.Set("anthropic-version", "2023-06-01")
		request.Header.Set("x-api-key", string(credential))
	} else {
		request.Header.Set("Authorization", "Bearer "+string(credential))
	}

	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusTimeout}
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() {
			return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusTimeout}
		}
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusUnavailable}
	}
	defer response.Body.Close()
	code := uint16(response.StatusCode)
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusRedirectBlocked, code: code}
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusAuthFailed, code: code}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusProviderError, code: code}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, providerRegistryProbeMaxBodyBytesV1+1))
	if err != nil || len(body) > providerRegistryProbeMaxBodyBytesV1 {
		clear(body)
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusInvalidResponse, code: code}
	}
	defer clear(body)
	if providerRegistryResponseContainsCredentialV1(body, credential) {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusInvalidResponse, code: code}
	}
	models, ok := parseProviderRegistryModelsV1(body)
	if !ok {
		return providerRegistryProbeOutcomeV1{status: providerregistryapp.ProviderProbeStatusInvalidResponse, code: code}
	}
	return providerRegistryProbeOutcomeV1{
		status: providerregistryapp.ProviderProbeStatusReachable,
		code:   code,
		models: models,
	}
}

func providerRegistryResponseContainsCredentialV1(body, credential []byte) bool {
	if len(credential) == 0 {
		return false
	}
	if bytes.Contains(body, credential) {
		return true
	}
	encodedLen := base64.StdEncoding.EncodedLen(len(credential))
	if encodedLen > len(body) {
		return false
	}
	encoded := make([]byte, encodedLen)
	defer clear(encoded)
	base64.StdEncoding.Encode(encoded, credential)
	return bytes.Contains(body, encoded)
}

func providerRegistryProbeClientV1(proxySource string) (*http.Client, error) {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("provider probe transport is unavailable")
	}
	clone := transport.Clone()
	clone.Proxy = nil
	if proxySource != "" {
		proxyURL, err := url.Parse(proxySource)
		if err != nil || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") ||
			proxyURL.Host == "" || proxyURL.User != nil {
			clone.CloseIdleConnections()
			return nil, errors.New("provider probe proxy is invalid")
		}
		clone.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{
		Transport: clone,
		Timeout:   providerRegistryProbeTimeoutV1,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func providerRegistryModelsURLV1(provider domainregistry.Provider) (string, error) {
	parsed, err := url.Parse(provider.Endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("provider endpoint is invalid")
	}
	if strings.EqualFold(provider.Kind, "custom-endpoint") || strings.EqualFold(provider.Kind, "custom_endpoint") {
		return parsed.String(), nil
	}
	path := strings.TrimRight(parsed.Path, "/")
	segments := strings.Split(strings.Trim(path, "/"), "/")
	last := ""
	if len(segments) > 0 {
		last = strings.ToLower(segments[len(segments)-1])
	}
	if last == "beta" {
		segments[len(segments)-1] = "v1"
		path = "/" + strings.Join(segments, "/")
	} else if len(last) < 2 || last[0] != 'v' || !allProviderRegistryDigitsV1(last[1:]) {
		path += "/v1"
	}
	parsed.Path = strings.TrimRight(path, "/") + "/models"
	return parsed.String(), nil
}

func allProviderRegistryDigitsV1(value string) bool {
	if value == "" {
		return false
	}
	for _, current := range []byte(value) {
		if current < '0' || current > '9' {
			return false
		}
	}
	return true
}

func providerRegistryUsesAnthropicCredentialV1(kind string) bool {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	return normalized == "anthropic-compatible" || normalized == "anthropic-messages" || normalized == "deepseek-messages" || normalized == "messages"
}

func parseProviderRegistryModelsV1(body []byte) ([]string, bool) {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || len(payload.Data) > domainregistry.MaxModels {
		return nil, false
	}
	seen := make(map[string]struct{}, len(payload.Data))
	models := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" || id != item.ID || !utf8.ValidString(id) || len([]byte(id)) > 256 ||
			strings.ContainsAny(id, "\x00\r\n\t") {
			return nil, false
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	sort.Strings(models)
	return models, true
}
