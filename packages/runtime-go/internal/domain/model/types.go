package model

import (
	"encoding/json"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type Message struct {
	Role       string        `json:"role"`
	Name       string        `json:"name,omitempty"`
	Content    string        `json:"content,omitempty"`
	Parts      []MessagePart `json:"parts,omitempty"`
	ToolCalls  []ToolCall    `json:"toolCalls,omitempty"`
	ToolCallID string        `json:"toolCallId,omitempty"`
	// PrivateAttachmentPlanDigest marks the one logical user message that may
	// receive attempt-local attachment parts. It is never serialized.
	PrivateAttachmentPlanDigest string `json:"-"`
	// PrivateProviderSemanticBinding is an opaque process-local provenance
	// sidecar. Only the final provider privacy boundary recognizes its private
	// concrete value; JSON, history, compaction, and provider serializers omit it.
	PrivateProviderSemanticBinding any `json:"-"`
	// PrivateProviderReferenceBinding binds host-produced case-entity references
	// to this exact in-process message. Ordinary serialization deliberately drops
	// it, so restart must recompile the relevant host evidence instead of trusting
	// durable prose or a copied token.
	PrivateProviderReferenceBinding any `json:"-"`
}

type MessagePart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ImageURL  string `json:"imageUrl,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Data      string `json:"data,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	// OutputSchema is a host-only MCP result contract. Provider serializers
	// must advertise only Parameters, while grants bind both directions.
	OutputSchema json.RawMessage `json:"-"`
	Source       string          `json:"-"`
	// TaskSupport is a host-only MCP execution contract. Provider serializers
	// must not expose it, while execution grants bind its normalized value.
	TaskSupport string `json:"-"`
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Pricing struct {
	CacheHit float64 `json:"cacheHit,omitempty"`
	Input    float64 `json:"input,omitempty"`
	Output   float64 `json:"output,omitempty"`
	Currency string  `json:"currency,omitempty"`
}

type Request struct {
	ProviderID     string
	Family         string
	EndpointFormat string
	BaseURL        string
	// ProxyURL is process-local Registry route authority for this physical
	// Provider attempt. It must never be serialized, persisted, or reused by
	// MCP/web transports.
	ProxyURL          string
	APIKey            string
	Model             string
	Route             string
	ReasoningEffort   string
	ReasoningProtocol string
	// MaxOutputTokens is a host-owned upper bound for one physical provider
	// response. Zero keeps the ordinary provider default. Positive values must
	// be carried to the provider wire contract and are independently enforced
	// by the host stream boundary so an endpoint cannot ignore the limit.
	MaxOutputTokens int
	SystemPrompt    string
	Messages        []Message
	// PrivateAttachmentPlanDigest binds process-local attachment authority to
	// continuation/retry state. Provider serializers must not emit it.
	PrivateAttachmentPlanDigest string
	// AnthropicThinkingReplay is legacy-named process-local protocol state for
	// exact Anthropic or DeepSeek tool continuation. It is never serialized as
	// runtime history and can only be created by consuming a host-issued
	// volatile capsule for the exact logical provider call.
	AnthropicThinkingReplay *AnthropicThinkingReplay
	// DeepSeekReasoningReplays is legacy-named process-local protocol state
	// containing every exact thinking/reasoning segment required by assistant
	// tool-call messages that remain in this uninterrupted request chain. Both
	// Anthropic and DeepSeek serializers accept only replays matching their own
	// protocol. The slice is never persisted or projected.
	DeepSeekReasoningReplays []*AnthropicThinkingReplay `json:"-"`
	Tools                    []ToolSchema
	Pricing                  *Pricing
	OnChunk                  func(Chunk) error
	BeforeSend               func(attempt int) error
	OnPipelineStage          func(PipelineStage) error
	// PrivateProviderTelemetry is host-only authority for the exact physical
	// provider send. Provider serializers must never emit it.
	PrivateProviderTelemetry *ProviderTelemetryBindingV1 `json:"-"`
	// PrivateProviderProxyAuthority distinguishes an explicitly empty Registry
	// proxy (direct transport) from legacy clients that still own their injected
	// HTTP transport. It is process-local and never serialized.
	PrivateProviderProxyAuthority bool `json:"-"`
	// PrivateProviderCurrentnessBeforeSend is the final process-local authority
	// fence immediately before each physical Provider send, including internal
	// stream reconnects. It is never serialized or persisted.
	PrivateProviderCurrentnessBeforeSend func(attempt int) error `json:"-"`
	// PrivateProviderReferenceBinding is the request-wide, process-local
	// reference provenance selected by the host for this exact security context.
	// The final privacy boundary validates its private concrete type before any
	// case-entity reference can survive projection.
	PrivateProviderReferenceBinding any `json:"-"`
}

type ProviderTelemetryBindingV1 struct {
	SecurityContext domainsecurity.TurnSecurityContext
	// OrdinaryEffect is host-owned, process-local effect classification. It
	// lets durable telemetry observe an ordinary provider call on a witnessed
	// boundary-only turn without upgrading that call to case execution.
	OrdinaryEffect  bool
	UsageSource     domaincache.ProviderUsageSourceV1
	ChildRunID      string
	Channel         domaincache.ProviderChannelV1
	LogicalSequence uint64
	OuterAttempt    uint32
	// LaneCallSequence distinguishes multiple calls made by one logical parent
	// in a non-primary provider lane, such as one call per attachment image.
	// Primary calls must leave it zero.
	LaneCallSequence uint32
}

type PipelineStage struct {
	Stage   string
	At      time.Time
	Details map[string]any
	Trace   map[string]any
}

type Usage struct {
	MessagesInput       domaincache.MessagesInputV1 `json:"-"`
	UsagePresenceKnown  bool                        `json:"-"`
	HasPromptTokens     bool                        `json:"-"`
	HasCompletionTokens bool                        `json:"-"`
	PromptTokens        int                         `json:"promptTokens"`
	CompletionTokens    int                         `json:"completionTokens"`
	ReasoningTokens     int                         `json:"reasoningTokens"`
	TotalTokens         int                         `json:"totalTokens"`
	CacheHitTokens      int                         `json:"cacheHitTokens"`
	CacheMissTokens     int                         `json:"cacheMissTokens"`
	CacheHitRate        float64                     `json:"cacheHitRate"`
	HasCacheHit         bool                        `json:"-"`
	HasCacheMiss        bool                        `json:"-"`
	FinishReason        string                      `json:"finishReason,omitempty"`
	CostUSD             float64                     `json:"costUsd,omitempty"`
	CostCNY             float64                     `json:"costCny,omitempty"`
	CacheSavingsUSD     float64                     `json:"cacheSavingsUsd,omitempty"`
	CacheSavingsCNY     float64                     `json:"cacheSavingsCny,omitempty"`
	Currency            string                      `json:"currency,omitempty"`
	PriceConfigured     bool                        `json:"priceConfigured,omitempty"`
}

func (u Usage) HasCacheTelemetry() bool {
	return u.HasCacheHit || u.HasCacheMiss || u.CacheHitTokens > 0 || u.CacheMissTokens > 0
}

type ChunkKind string

const (
	ChunkReasoning     ChunkKind = "reasoning"
	ChunkText          ChunkKind = "text"
	ChunkToolCallStart ChunkKind = "tool_call_start"
	ChunkToolCall      ChunkKind = "tool_call"
	ChunkUsage         ChunkKind = "usage"
	ChunkRetrying      ChunkKind = "retrying"
	ChunkError         ChunkKind = "error"
	ChunkDone          ChunkKind = "done"
)

type Chunk struct {
	Kind         ChunkKind  `json:"kind"`
	Text         string     `json:"text,omitempty"`
	ToolCall     ToolCall   `json:"toolCall,omitempty"`
	Usage        Usage      `json:"usage,omitempty"`
	Signature    string     `json:"signature,omitempty"`
	RetryAttempt int        `json:"retryAttempt,omitempty"`
	RetryMax     int        `json:"retryMax,omitempty"`
	Trace        ChunkTrace `json:"-"`
}

type ChunkTrace struct {
	ProviderResponseModel     string
	ProviderRequestSentAt     time.Time
	ProviderResponseHeadersAt time.Time
	ProviderRawSSEChunkAt     time.Time
	ProviderLastRawSSEChunkAt time.Time
	ProviderChunkParsedAt     time.Time
	LoopChunkCallbackAt       time.Time
}

type PrefixShape struct {
	SystemHash       string   `json:"systemHash"`
	ToolsHash        string   `json:"toolsHash"`
	PrefixHash       string   `json:"prefixHash"`
	PrefixItemsHash  string   `json:"prefixItemsHash"`
	ToolSchemaTokens int      `json:"toolSchemaTokens"`
	ToolCount        int      `json:"toolCount,omitempty"`
	ToolSourcesHash  string   `json:"toolSourcesHash,omitempty"`
	ToolSourceIDs    []string `json:"toolSourceIds,omitempty"`
	Route            string   `json:"route,omitempty"`
	Provider         string   `json:"provider"`
	ProviderID       string   `json:"providerId"`
	EndpointFormat   string   `json:"endpointFormat"`
	Model            string   `json:"model"`
	// Deprecated: old serialized records may contain this boolean. No current
	// producer performs a dynamic-state check; false is not evidence of safety.
	DynamicStateLeaked  bool   `json:"dynamicStateLeaked,omitempty"`
	DynamicStateCheck   string `json:"dynamicStateCheck,omitempty"`
	ToolSchemaEstimator string `json:"toolSchemaEstimator,omitempty"`
}

// ProviderTiming is process-local Host observation. It is never signed or persisted.
type ProviderTiming struct {
	StartedAt, FirstContentAt, LastContentAt, FirstTextAt, LastTextAt, FinishBoundaryAt, FinishedAt time.Time
}

type Result struct {
	ResponseObservedModel string         `json:"-"`
	HostTiming            ProviderTiming `json:"-"`
	ProviderID            string         `json:"providerId"`
	Family                string         `json:"family"`
	EndpointFormat        string         `json:"endpointFormat"`
	RequestURL            string         `json:"requestUrl"`
	RequestBodyFields     []string       `json:"requestBodyFields"`
	Chunks                []Chunk        `json:"chunks"`
	Usage                 Usage          `json:"usage"`
	PrefixShape           PrefixShape    `json:"prefixShape"`
	StreamCompleted       bool           `json:"streamCompleted"`
	FirstTokenLatencyMs   int64          `json:"firstTokenLatencyMs,omitempty"`
	HasFirstTokenLatency  bool           `json:"-"`
	// These host-observed timings distinguish private reasoning and raw text
	// from an authorized final answer. They are never provider output fields.
	FirstReasoningLatencyMs  int64 `json:"-"`
	HasFirstReasoningLatency bool  `json:"-"`
	FirstRawTextLatencyMs    int64 `json:"-"`
	HasFirstRawTextLatency   bool  `json:"-"`
	DurationMs               int64 `json:"durationMs,omitempty"`
	HasDuration              bool  `json:"-"`
	// CacheObservations contains only validated HMAC identities and numeric
	// usage settlements for every real HTTP attempt. It is internal authority
	// input and must never be serialized as provider output or public history.
	CacheObservations []domaincache.ProviderCallObservationV1 `json:"-"`
}

type RequestShape struct {
	RequestURL        string   `json:"requestUrl"`
	RequestBodyFields []string `json:"requestBodyFields"`
	RequestHeaders    []string `json:"requestHeaders"`
}

type ModelRef struct {
	ProviderID string `json:"providerId"`
	ID         string `json:"id"`
	Variant    string `json:"variant,omitempty"`
}

type TurnConfig struct {
	ProviderID                string
	Family                    string
	EndpointFormat            string
	BaseURL                   string
	ProxyURL                  string
	APIKey                    string
	Model                     string
	ReasoningEffort           string
	ReasoningProtocol         string
	ReasoningSupportedEfforts []string
	ReasoningDefaultEffort    string
	CacheTelemetrySupported   bool
	DeepSeekPrefixEnhancement bool
	SupportsImageInput        bool
	InputModalities           []string
	MessageParts              []string
	ContextWindowTokens       int
	Pricing                   *Pricing
}

type ModelProvidersConfig struct {
	DefaultProviderID string                `json:"defaultProviderId"`
	Providers         []ModelProviderConfig `json:"providers"`
}

type ModelProviderConfig struct {
	ID             string                          `json:"id"`
	Name           string                          `json:"name,omitempty"`
	APIKey         string                          `json:"apiKey,omitempty"`
	BaseURL        string                          `json:"baseUrl"`
	ModelProxyURL  string                          `json:"modelProxyUrl,omitempty"`
	EndpointFormat string                          `json:"endpointFormat,omitempty"`
	Models         []string                        `json:"models,omitempty"`
	ModelProfiles  map[string]ModelProviderProfile `json:"modelProfiles,omitempty"`
	Price          *Pricing                        `json:"price,omitempty"`
	Prices         map[string]*Pricing             `json:"prices,omitempty"`
}

type ModelProviderProfile struct {
	Aliases             []string                `json:"aliases,omitempty"`
	EndpointFormat      string                  `json:"endpointFormat,omitempty"`
	InputModalities     []string                `json:"inputModalities,omitempty"`
	OutputModalities    []string                `json:"outputModalities,omitempty"`
	SupportsToolCalling *bool                   `json:"supportsToolCalling,omitempty"`
	MessageParts        []string                `json:"messageParts,omitempty"`
	ContextWindowTokens int                     `json:"contextWindowTokens,omitempty"`
	Reasoning           *ModelProviderReasoning `json:"reasoning,omitempty"`
	Price               *Pricing                `json:"price,omitempty"`
}

type ModelProviderReasoning struct {
	SupportedEfforts []string `json:"supportedEfforts,omitempty"`
	DefaultEffort    string   `json:"defaultEffort,omitempty"`
	RequestProtocol  string   `json:"requestProtocol,omitempty"`
}

type RuntimeProviderConfigInput struct {
	DefaultProviderID     string
	DefaultBaseURL        string
	DefaultAPIKey         string
	DefaultEndpointFormat string
	DefaultModel          string
	ModelProvidersJSON    string
}
