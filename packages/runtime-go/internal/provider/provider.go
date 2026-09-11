// Portions in this provider lineage were adapted from DeepSeek-Reasonix and
// modified by Analytix contributors. See THIRD_PARTY_NOTICES.md and the
// upstream provenance ledger.
package provider

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	providerclient "analytix.local/runtime-go/internal/adapters/outbound/provider/client"
	providercompat "analytix.local/runtime-go/internal/adapters/outbound/provider/compat"
	providerstream "analytix.local/runtime-go/internal/adapters/outbound/provider/stream"
	providerusage "analytix.local/runtime-go/internal/adapters/outbound/provider/usage"
	appmodel "analytix.local/runtime-go/internal/app/model"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	"analytix.local/runtime-go/internal/ports"
)

const interruptedToolResult = "[no result: the previous turn was interrupted before this tool call completed]"

type RuntimeProviderClient = ports.ProviderClient
type HTTPProviderClient = providerclient.HTTPProviderClient

type Message = domainmodel.Message
type MessagePart = domainmodel.MessagePart
type ToolSchema = domainmodel.ToolSchema
type ToolCall = domainmodel.ToolCall
type Pricing = domainmodel.Pricing
type Request = domainmodel.Request
type Usage = domainmodel.Usage
type ChunkKind = domainmodel.ChunkKind

const (
	ChunkReasoning     = domainmodel.ChunkReasoning
	ChunkText          = domainmodel.ChunkText
	ChunkToolCallStart = domainmodel.ChunkToolCallStart
	ChunkToolCall      = domainmodel.ChunkToolCall
	ChunkUsage         = domainmodel.ChunkUsage
	ChunkRetrying      = domainmodel.ChunkRetrying
	ChunkError         = domainmodel.ChunkError
	ChunkDone          = domainmodel.ChunkDone
)

type Chunk = domainmodel.Chunk
type PipelineStage = domainmodel.PipelineStage
type PrefixShape = domainmodel.PrefixShape
type Result = domainmodel.Result

type ProviderError = providerclient.ProviderError

type RequestShape = domainmodel.RequestShape
type TurnConfig = domainmodel.TurnConfig
type ModelProvidersConfig = domainmodel.ModelProvidersConfig
type ModelProviderConfig = domainmodel.ModelProviderConfig
type ModelProviderProfile = domainmodel.ModelProviderProfile
type ModelProviderReasoning = domainmodel.ModelProviderReasoning
type RuntimeProviderConfigInput = domainmodel.RuntimeProviderConfigInput
type TurnExecutionInput = appmodel.TurnExecutionInput
type TurnExecutionResult = appmodel.TurnExecutionResult
type TurnExecutionAuthority = appmodel.TurnExecutionAuthority

type RuntimeProviderConfigSet struct {
	inner appmodel.RuntimeProviderConfigSet
}

func BuildRequestShapeForTest(request Request) (RequestShape, error) {
	endpoint, body, headers, err := buildHTTPRequest(request)
	if err != nil {
		return RequestShape{}, err
	}
	return RequestShape{
		RequestURL:        endpoint,
		RequestBodyFields: sortedMapKeys(body),
		RequestHeaders:    sortedMapKeysString(headers),
	}, nil
}

func NewHTTPProviderClient(httpClient *http.Client) *HTTPProviderClient {
	return providerclient.NewHTTPProviderClient(httpClient)
}

func NewDefaultHTTPClient(timeout time.Duration) *http.Client {
	return providerclient.NewDefaultHTTPClient(timeout)
}

func NewDefaultHTTPClientWithProxy(proxyURL string) *http.Client {
	return providerclient.NewDefaultHTTPClientWithProxy(proxyURL)
}

func NetworkProxyDiagnostics(proxyURL string) map[string]any {
	return providerclient.NetworkProxyDiagnostics(proxyURL)
}

func NewRuntimeProviderConfigSet(input RuntimeProviderConfigInput) RuntimeProviderConfigSet {
	return RuntimeProviderConfigSet{inner: appmodel.NewRuntimeProviderConfigSet(input)}
}

func (set RuntimeProviderConfigSet) ConfigurationError() error {
	return set.inner.ConfigurationError()
}

func (set RuntimeProviderConfigSet) TurnConfig(providerID, model string) TurnConfig {
	return set.inner.TurnConfig(providerID, model)
}

func (set RuntimeProviderConfigSet) TurnConfigForExecution(providerID, model string) TurnConfig {
	return set.inner.TurnConfigForExecution(providerID, model)
}

func (set RuntimeProviderConfigSet) ResolveTurnExecution(input TurnExecutionInput) (TurnExecutionResult, error) {
	result, err := set.inner.ResolveTurnExecution(input)
	if err == nil {
		return result, nil
	}
	return TurnExecutionResult{}, providerExecutionError(err)
}

func (set RuntimeProviderConfigSet) HasProvider(providerID string) bool {
	return set.inner.HasProvider(providerID)
}

func (set RuntimeProviderConfigSet) ValidateExecutionModel(providerID string, model string) error {
	err := set.inner.ValidateExecutionModel(providerID, model)
	if err == nil {
		return nil
	}
	return providerExecutionError(err)
}

func providerExecutionError(err error) error {
	var modelErr *appmodel.ExecutionModelError
	if !errors.As(err, &modelErr) {
		return err
	}
	return &ProviderError{
		ProviderID:     modelErr.ProviderID,
		Family:         modelErr.Family,
		EndpointFormat: modelErr.EndpointFormat,
		BaseURL:        modelErr.BaseURL,
		Kind:           modelErr.Kind,
		Message:        modelErr.Message,
		HasAPIKey:      modelErr.HasAPIKey,
	}
}

func (set RuntimeProviderConfigSet) Diagnostics() []map[string]any {
	return set.inner.Diagnostics()
}

func NormalizeEndpointFormat(value string) string {
	return domainmodel.NormalizeEndpointFormat(value)
}

func AppendEndpointPath(baseURL string, versionedPath string) string {
	return providercompat.AppendEndpointPath(baseURL, versionedPath)
}

func RuntimeCacheDiagnostics(result Result) map[string]any {
	return providerusage.CacheDiagnostics(result)
}

func RuntimeCacheDiagnosticsWithPrevious(previous PrefixShape, result Result) map[string]any {
	return providerusage.CacheDiagnosticsWithPrevious(previous, result)
}

func SanitizeToolArgumentsJSON(data []byte) string {
	return appmodel.SanitizeToolArgumentsJSON(data)
}

func CanonicalJSONSchema(body json.RawMessage) string {
	return domainmodel.CanonicalProviderJSONSchema(body)
}

func SanitizeToolPairing(messages []Message) []Message {
	return appmodel.SanitizeToolPairing(messages)
}

func NormalizeSessionMessages(messages []Message) []Message {
	return appmodel.NormalizeSessionMessages(messages)
}

func ApplyPricing(usage Usage, pricing *Pricing) Usage {
	return providerusage.ApplyPricing(usage, pricing)
}

func defaultDeepSeekPricing(model string) *Pricing {
	return appmodel.DefaultDeepSeekPricing(model)
}

func CustomEndpointRequestShape(baseURL string) string {
	return providercompat.CustomEndpointRequestShape(baseURL)
}

func customEndpointRequestShape(baseURL string) string {
	return CustomEndpointRequestShape(baseURL)
}

func buildHTTPRequest(request Request) (string, map[string]any, map[string]string, error) {
	prepared, err := providercompat.BuildHTTPRequest(request)
	return prepared.RequestURL, prepared.Body, prepared.Headers, err
}

func parseSSE(endpointFormat string, body io.Reader) ([]Chunk, Usage, error) {
	return providerstream.ParseSSE(endpointFormat, body)
}

func parseSSEWithCallback(endpointFormat string, body io.Reader, onChunk func(Chunk) error) ([]Chunk, Usage, error) {
	return providerstream.ParseSSEWithCallback(endpointFormat, body, onChunk)
}

type toolCallAccumulator struct {
	inner *providerstream.ToolCallAccumulator
}

type thinkSplitter struct {
	inner *providerstream.ThinkSplitter
}

func (a *toolCallAccumulator) stream() *providerstream.ToolCallAccumulator {
	if a == nil {
		return nil
	}
	if a.inner == nil {
		a.inner = providerstream.NewToolCallAccumulator()
	}
	return a.inner
}

func (t *thinkSplitter) stream() *providerstream.ThinkSplitter {
	if t == nil {
		return nil
	}
	if t.inner == nil {
		t.inner = providerstream.NewThinkSplitter()
	}
	return t.inner
}

func parseOpenAIChatSSEPayload(data string, accumulator *toolCallAccumulator, thinkingTags *thinkSplitter) ([]Chunk, Usage, bool, string, error) {
	return providerstream.ParseOpenAIChatSSEPayloadWithState(data, accumulator.stream(), thinkingTags.stream())
}

func parseOpenAISSEPayload(data string) ([]Chunk, Usage, bool, error) {
	return providerstream.ParseOpenAISSEPayload(data)
}

func parseAnthropicSSEPayload(data string) ([]Chunk, Usage, bool, error) {
	return providerstream.ParseAnthropicSSEPayload(data)
}

func CapturePrefixShape(request Request) PrefixShape {
	return appmodel.CapturePrefixShape(request)
}

func appendEndpointPath(baseURL string, versionedPath string) string {
	return providercompat.AppendEndpointPath(baseURL, versionedPath)
}

func countCompleted(results []Result) int {
	count := 0
	for _, result := range results {
		if result.StreamCompleted {
			count++
		}
	}
	return count
}

func hasChunkKind(chunks []Chunk, kind ChunkKind) bool {
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

func sortedMapKeysString(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameStringSet(actual []string, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	left := append([]string(nil), actual...)
	right := append([]string(nil), expected...)
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func hasString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
