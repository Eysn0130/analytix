package model

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstreamframe "analytix.local/runtime-go/internal/domain/streamframe"
)

type Message struct {
	// Minted only from trusted signed original Question settlement or audited
	// history. It preserves protocol identity, never user content or authority.
	PrivateUserInputPendingBinding any `json:"-"`

	// This independent, process-only carrier is minted from verified signed
	// late Question steering. It grants no Case reference or data authority.
	PrivateUserInputReplyBinding  any  `json:"-"`
	PrivateUserInputReplyRequired bool `json:"-"`

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
	// StepStartedAt is host-authored and shared by the outer retries of this
	// provider step. It is not a physical attempt or provider-supplied clock.
	StepStartedAt time.Time
	// PublicAttribution is frozen by the host from the approved execution intent.
	PublicAttribution domaincache.ProviderPublicAttributionV1
	SecurityContext   domainsecurity.TurnSecurityContext
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

// ProviderRetryV1 describes host-owned retry lifecycle observations. Started
// records a host invocation; signed dispatch telemetry owns network truth.
// Scheduled deadlines describe only the frozen wait, never request start.
type ProviderRetryV1 struct {
	SchemaVersion   int
	Source          string
	Phase           string
	LogicalSequence uint64
	OuterAttempt    int
	PhysicalAttempt int
	DelayMs         *int64
	DeadlineUnixMs  *int64
	CancelReason    string
	At              time.Time
}

func (retry ProviderRetryV1) Details() map[string]any {
	details := map[string]any{"schemaVersion": float64(retry.SchemaVersion), "source": retry.Source,
		"phase": retry.Phase, "logicalSequence": float64(retry.LogicalSequence), "outerAttempt": float64(retry.OuterAttempt)}
	if retry.PhysicalAttempt > 0 {
		details["physicalAttempt"] = float64(retry.PhysicalAttempt)
	}
	if retry.DelayMs != nil {
		details["delayMs"] = float64(*retry.DelayMs)
	}
	if retry.DeadlineUnixMs != nil {
		details["deadlineUnixMs"] = float64(*retry.DeadlineUnixMs)
	}
	if retry.CancelReason != "" {
		details["cancelReason"] = retry.CancelReason
	}
	return details
}

func ProviderRetryCancelReasonV1(ctxErr, cause error) string {
	// Current host cancellation is authoritative over an older provider cause.
	if ctxErr != nil {
		cause = ctxErr
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(cause, context.Canceled) {
		return "context_cancelled"
	}
	return "attempt_rejected"
}

// ProjectProviderRetryDetailsV1 is shared by the producer and the public
// SSE/replay boundary. It accepts only closed lifecycle observations; none
// of these fields authorizes a network dispatch or a new provider call.
func ProjectProviderRetryDetailsV1(input map[string]any) (map[string]any, bool) {
	result := make(map[string]any, len(input))
	for key, value := range input {
		switch key {
		case "source":
			v, ok := providerRetryEnumV1(value, "provider_retry", "transport_reconnect")
			if !ok {
				return nil, false
			}
			result[key] = v
		case "phase":
			v, ok := providerRetryEnumV1(value, "scheduled", "started", "cancelled")
			if !ok {
				return nil, false
			}
			result[key] = v
		case "cancelReason":
			v, ok := providerRetryEnumV1(value, "context_cancelled", "deadline_exceeded", "attempt_rejected")
			if !ok {
				return nil, false
			}
			result[key] = v
		case "schemaVersion", "logicalSequence", "outerAttempt", "physicalAttempt", "delayMs", "deadlineUnixMs":
			n, ok := providerRetryNumberV1(value)
			if !ok || n < 0 || n > 1e15 || math.Trunc(n) != n || key != "delayMs" && n == 0 {
				return nil, false
			}
			result[key] = n
		default:
			return nil, false
		}
	}
	if result["schemaVersion"] != float64(1) || result["source"] == nil || result["phase"] == nil || result["logicalSequence"] == nil || result["outerAttempt"] == nil {
		return nil, false
	}
	if result["source"] == "transport_reconnect" && result["physicalAttempt"] == nil {
		return nil, false
	}
	if result["source"] == "provider_retry" && result["physicalAttempt"] != nil {
		return nil, false
	}
	_, delay := result["delayMs"]
	_, deadline := result["deadlineUnixMs"]
	_, reason := result["cancelReason"]
	if result["phase"] == "scheduled" {
		if !delay || !deadline || reason {
			return nil, false
		}
	} else if delay || deadline {
		return nil, false
	}
	if (result["phase"] == "cancelled") != reason {
		return nil, false
	}
	return result, true
}

func providerRetryEnumV1(value any, allowed ...string) (any, bool) {
	text, ok := value.(string)
	if !ok {
		return nil, false
	}
	for _, candidate := range allowed {
		if text == candidate {
			return text, true
		}
	}
	return nil, false
}

func providerRetryNumberV1(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

type PipelineStage struct {
	Stage   string
	At      time.Time
	Details map[string]any
	Trace   map[string]any
}

type Usage struct {
	OutputTokensFinality    domaincache.ProviderOutputTokensFinalityV1 `json:"-"`
	ReasoningTokensFinality domaincache.ProviderOutputTokensFinalityV1 `json:"-"`
	MessagesInput           domaincache.MessagesInputV1                `json:"-"`
	UsagePresenceKnown      bool                                       `json:"-"`
	HasPromptTokens         bool                                       `json:"-"`
	HasReasoningTokens      bool                                       `json:"-"`
	HasCompletionTokens     bool                                       `json:"-"`
	PromptTokens            int                                        `json:"promptTokens"`
	CompletionTokens        int                                        `json:"completionTokens"`
	ReasoningTokens         int                                        `json:"reasoningTokens"`
	TotalTokens             int                                        `json:"totalTokens"`
	CacheHitTokens          int                                        `json:"cacheHitTokens"`
	CacheMissTokens         int                                        `json:"cacheMissTokens"`
	CacheHitRate            float64                                    `json:"cacheHitRate"`
	HasCacheHit             bool                                       `json:"-"`
	HasCacheMiss            bool                                       `json:"-"`
	FinishReason            string                                     `json:"finishReason,omitempty"`
	CostUSD                 float64                                    `json:"costUsd,omitempty"`
	CostCNY                 float64                                    `json:"costCny,omitempty"`
	CacheSavingsUSD         float64                                    `json:"cacheSavingsUsd,omitempty"`
	CacheSavingsCNY         float64                                    `json:"cacheSavingsCny,omitempty"`
	Currency                string                                     `json:"currency,omitempty"`
	PriceConfigured         bool                                       `json:"priceConfigured,omitempty"`
}

func (u Usage) HasCacheTelemetry() bool {
	// Native numeric presence is independent of the old comparable cache pair.
	if u.UsagePresenceKnown {
		return u.HasPromptTokens && u.HasCacheHit && u.HasCacheMiss
	}
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

// ProcessFrameV1 is an ephemeral, display-only view of reasoning block framing,
// explicit provider summaries or a classified, still-open assistant draft.
// It is neither public output nor signed provider continuation. The
// host supplies ProviderSequence and the original turn/scope binding separately.
// No signatures, encrypted content, redacted blocks or tool grants belong here.
type ProcessFrameV1 struct {
	// Source distinguishes host-observed reasoning and private drafts from native framing.
	Source string `json:"-"`
	// ReasoningForm identifies explicitly emitted provider summaries. It never
	// claims that a summary contains the model's complete private reasoning.
	ReasoningForm string `json:"-"`
	// HostSegmentOrdinal labels host-observed reasoning segments or explicit
	// summary parts within this physical call, never a provider-native index.
	HostSegmentOrdinal    uint64                               `json:"-"`
	ToolPreparation       *domainstreamframe.ToolPreparationV1 `json:"-"`
	ContentClassification string                               `json:"-"`
	Kind                  string                               `json:"-"`
	Attempt               uint64                               `json:"-"`
	NativeIndex           int                                  `json:"-"`
	NativeID              string                               `json:"-"`
	Text                  string                               `json:"-"`
	ProviderSequence      uint64                               `json:"-"`
	// ProjectionSecrets is replaced by the host from the actual prepared
	// request. It is used only to project capture and never reaches the UI.
	ProjectionSecrets []string `json:"-"`
}

type Chunk struct {
	Kind         ChunkKind        `json:"kind"`
	Text         string           `json:"text,omitempty"`
	ToolCall     ToolCall         `json:"toolCall,omitempty"`
	Usage        Usage            `json:"usage,omitempty"`
	Signature    string           `json:"signature,omitempty"`
	RetryAttempt int              `json:"retryAttempt,omitempty"`
	RetryMax     int              `json:"retryMax,omitempty"`
	Retry        *ProviderRetryV1 `json:"-"`
	Trace        ChunkTrace       `json:"-"`
	ProcessFrame *ProcessFrameV1  `json:"-"`
	// DiscardedToolMaterial is private host budget input from unfinished capped blocks.
	// It never represents a tool call, native usage, or model replay.
	DiscardedToolMaterial string `json:"-"`
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

// ProviderTiming is process-local Host observation. Only an explicitly validated
// first-content timestamp may be copied into the numeric attempt settlement.
type ProviderTiming struct {
	// PresenceKnown distinguishes an installed native observer with no content
	// from diagnostic clients that report only display callbacks.
	FirstContentPresenceKnown bool
	StartedAt                 time.Time
	FirstContentAt            time.Time
	LastContentAt             time.Time
	FirstTextAt               time.Time
	LastTextAt                time.Time
	FinishBoundaryAt          time.Time
	FinishedAt                time.Time
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
