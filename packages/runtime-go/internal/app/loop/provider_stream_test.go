package loop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	privacyprojectionapp "analytix.local/runtime-go/internal/app/privacyprojection"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domainstreamframe "analytix.local/runtime-go/internal/domain/streamframe"
)

func TestProviderFirstTokenLatencyUsesBoundedHostTrace(t *testing.T) {
	t.Parallel()
	started := time.Unix(100, 0).UTC()
	now := started.Add(80 * time.Millisecond)
	if got := providerFirstTokenLatencyMillis(started, started.Add(25*time.Millisecond), now); got != 25 {
		t.Fatalf("valid host trace latency = %dms, want 25ms", got)
	}
	for name, observed := range map[string]time.Time{
		"missing": {},
		"before":  started.Add(-time.Second),
		"future":  now.Add(time.Second),
	} {
		if got := providerFirstTokenLatencyMillis(started, observed, now); got != 80 {
			t.Fatalf("%s trace latency = %dms, want bounded 80ms", name, got)
		}
	}
}

type providerStreamStub struct {
	responses []providerStreamResponse
	requests  []domainmodel.Request
}

func (*providerStreamStub) RequiresDurablePipelineStagesV1() {}

type providerStreamResponse struct {
	callbackChunks []domainmodel.Chunk
	resultChunks   []domainmodel.Chunk
	cacheTelemetry []domaincache.ProviderCallObservationV1
	nativePresence bool
	nativeContent  bool
	err            error
}

type observedProviderRetryError struct {
	fakeProviderRetryError
	observation domainmodel.ProviderOutputObservationV1
}

func (err observedProviderRetryError) ProviderOutputObservationV1() domainmodel.ProviderOutputObservationV1 {
	return err.observation
}
func (err observedProviderRetryError) ProviderOutputStarted() bool {
	return err.observation.RetryUnsafeV1()
}

type cancellationIgnoringStreamStub struct {
	cancel      context.CancelFunc
	callbackErr error
}

type attemptCancellationStreamStub struct {
	cancel      context.CancelFunc
	callbackErr error
}

type privateAttemptProviderStub struct {
	active    *bool
	responses []providerStreamResponse
	calls     int
	cancel    context.CancelFunc
}

func (stub *privateAttemptProviderStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if stub.active == nil || !*stub.active {
		return domainmodel.Result{}, errors.New("private provider payload escaped its effect lease")
	}
	if len(request.Messages) != 1 || len(request.Messages[0].Parts) != 1 ||
		request.Messages[0].Parts[0].Text != "ATTEMPT_LOCAL_TEXT_SENTINEL" {
		return domainmodel.Result{}, errors.New("attempt-local attachment payload is unavailable")
	}
	if request.BeforeSend == nil {
		return domainmodel.Result{}, errors.New("physical send authority is unavailable")
	}
	if err := request.BeforeSend(1); err != nil {
		return domainmodel.Result{}, err
	}
	if stub.cancel != nil {
		stub.cancel()
	}
	index := stub.calls
	stub.calls++
	if index >= len(stub.responses) {
		return domainmodel.Result{}, errors.New("unexpected private provider attempt")
	}
	response := stub.responses[index]
	return domainmodel.Result{Chunks: response.resultChunks}, response.err
}

func (stub *cancellationIgnoringStreamStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	stub.cancel()
	if request.OnChunk != nil {
		stub.callbackErr = request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "CANCELLED_PROVIDER_SENTINEL"})
	}
	return domainmodel.Result{
		ProviderID: "provider-cancelled", Family: "deepseek", EndpointFormat: "chat_completions",
		Chunks:      []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "CANCELLED_PROVIDER_SENTINEL"}, {Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_CANCELLED_REASONING"}},
		Usage:       domainmodel.Usage{PromptTokens: 12, CompletionTokens: 3, TotalTokens: 15},
		PrefixShape: domainmodel.PrefixShape{PrefixHash: "prefix-cancelled"}, StreamCompleted: true,
		CacheObservations: []domaincache.ProviderCallObservationV1{{}},
	}, nil
}

func (stub *attemptCancellationStreamStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if request.OnChunk != nil {
		stub.callbackErr = request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "UNCOMMITTED_PROVIDER_SENTINEL"})
	}
	if stub.cancel != nil {
		stub.cancel()
	}
	return domainmodel.Result{
		ProviderID: "attempt-cancelled",
		Chunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "UNCOMMITTED_PROVIDER_SENTINEL"},
			{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_CANCELLED_REASONING"},
		},
	}, nil
}

func (stub *providerStreamStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	stub.requests = append(stub.requests, request)
	index := len(stub.requests) - 1
	if index >= len(stub.responses) {
		return domainmodel.Result{}, errors.New("unexpected provider stream call")
	}
	response := stub.responses[index]
	if request.OnPipelineStage != nil {
		base := time.Unix(100+int64(index), 0).UTC()
		if err := request.OnPipelineStage(domainmodel.PipelineStage{Stage: "pre_send", At: base}); err != nil {
			return domainmodel.Result{}, err
		}
		if err := request.OnPipelineStage(domainmodel.PipelineStage{Stage: "post_send", At: base.Add(time.Millisecond)}); err != nil {
			return domainmodel.Result{}, err
		}
	}
	nativeTiming := domainmodel.ProviderTiming{FirstContentPresenceKnown: response.nativePresence}
	if response.nativeContent {
		nativeTiming.FirstContentAt = time.Now()
	}
	for _, chunk := range response.callbackChunks {
		if request.OnChunk != nil {
			if err := request.OnChunk(chunk); err != nil {
				return domainmodel.Result{}, err
			}
		}
	}
	return domainmodel.Result{Chunks: response.resultChunks, CacheObservations: response.cacheTelemetry, HostTiming: nativeTiming}, response.err
}

func TestProviderContextHardLimitMakesZeroProviderCalls(t *testing.T) {
	provider := &providerStreamStub{}
	stages := []domainmodel.PipelineStage{}
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request: domainmodel.Request{
			Messages: []domainmodel.Message{{Role: "user", Content: strings.Repeat("案", 1_000)}},
			OnPipelineStage: func(stage domainmodel.PipelineStage) error {
				stages = append(stages, stage)
				return nil
			},
		},
		ContinuationAuthority: ProviderContinuationAuthorityInput{ProviderConfig: domainmodel.TurnConfig{ContextWindowTokens: 1_000}},
	})
	if got := PublicFailureForError(err).Code(); got != "context_window_hard_limit" {
		t.Fatalf("hard-limit failure code = %q, err=%v", got, err)
	}
	if len(provider.requests) != 0 {
		t.Fatalf("hard-limit request reached provider: %d calls", len(provider.requests))
	}
	if len(stages) != 1 || stages[0].Stage != "provider_admission_rejected" ||
		stages[0].Details["reasonCode"] != "context_window_hard_limit" ||
		stages[0].Details["providerAttemptCount"] != float64(0) ||
		stages[0].Details["projectedRequestTokens"] != float64(appmodel.EstimateMessagesTokensV1(
			[]domainmodel.Message{{Role: "user", Content: strings.Repeat("案", 1_000)}},
		)) || stages[0].Details["hardThresholdTokens"] != float64(850) {
		t.Fatalf("hard-limit admission diagnostic is missing or open: %#v", stages)
	}
}

func TestProviderOutputTokenBudgetIsEnforcedByHostStreamBoundary(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: strings.Repeat("bounded-output-", 32)}},
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, Request: domainmodel.Request{MaxOutputTokens: 4},
	})
	if !errors.Is(err, ErrProviderOutputTokenBudgetExceeded) || len(provider.requests) != 1 ||
		output.Text != "" || output.Reasoning != "" {
		t.Fatalf("provider output exceeded host budget without fail-closed settlement: output=%#v calls=%d err=%v", output, len(provider.requests), err)
	}
}

func TestProviderOutputBudgetIncludesReasoningAndText(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkReasoning, Text: "abcdabcd"},
			{Kind: domainmodel.ChunkText, Text: "efghefgh"},
		},
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, Request: domainmodel.Request{MaxOutputTokens: 3},
	})
	if !errors.Is(err, ErrProviderOutputTokenBudgetExceeded) || len(provider.requests) != 1 ||
		output.Text != "" || output.Reasoning != "" {
		t.Fatal("reasoning and text did not consume one fail-closed output budget")
	}
}

func BenchmarkProviderOutputBudgetChunking(b *testing.B) {
	// Same 64 KiB output and host cap; vary only the delivery granularity.
	const totalBytes = 64 * 1024
	for _, chunkBytes := range []int{totalBytes, 1024, 64} {
		b.Run(fmt.Sprintf("chunk_bytes_%d", chunkBytes), func(b *testing.B) {
			chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: strings.Repeat("a", chunkBytes)}
			b.ReportAllocs()
			b.SetBytes(totalBytes)
			for i := 0; i < b.N; i++ {
				budget := newProviderOutputTokenBudgetV1(totalBytes)
				for sent := 0; sent < totalBytes; sent += chunkBytes {
					if err := budget.Observe(chunk); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func TestPrivateAttemptCannotRemoveProviderOutputTokenBudget(t *testing.T) {
	provider := &providerStreamStub{}
	settled := ""
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, Request: domainmodel.Request{MaxOutputTokens: 64},
		Callbacks: ProviderStreamCallbacks{
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.MaxOutputTokens = 0
				return request, func(_ context.Context, status, reason string) error {
					settled = status + ":" + reason
					return nil
				}, nil
			},
		},
	})
	if !errors.Is(err, ErrProviderOutputTokenBudgetExceeded) || len(provider.requests) != 0 || settled != "failed:provider_request_authority_invalid" {
		t.Fatalf("attempt materialization removed host token budget: calls=%d settled=%q err=%v", len(provider.requests), settled, err)
	}
}

func TestStreamProviderWithRetryRejectsPreparedToolManifestMutationBeforeProvider(t *testing.T) {
	provider := &providerStreamStub{}
	tools := zeroArgumentToolSchemas("read")
	settled := ""
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{Tools: tools},
		Callbacks: ProviderStreamCallbacks{
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.Tools = zeroArgumentToolSchemas("write")
				return request, func(_ context.Context, status, reason string) error {
					settled = status + ":" + reason
					return nil
				}, nil
			},
		},
	})
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		len(provider.requests) != 0 || settled != "failed:provider_request_tool_manifest_mismatch" {
		t.Fatalf("prepared tool manifest mutation crossed provider boundary: calls=%d settled=%q err=%v", len(provider.requests), settled, err)
	}
}

func TestStreamProviderWithRetryRejectsInvalidEffortBeforeContinuationAndAttemptEffects(t *testing.T) {
	for _, effort := range []string{" high ", "HIGH", "adaptive", "xhigh", "SOL_PRIVATE_REASONING_SENTINEL_7F3C"} {
		provider := &providerStreamStub{}
		var acquireCalls, beforeCalls, prepareCalls int
		output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
			Provider: provider,
			Request:  domainmodel.Request{ReasoningEffort: effort},
			Callbacks: ProviderStreamCallbacks{
				AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
					acquireCalls++
					return ctx, func() {}, nil
				},
				BeforeProviderAttempt: func(int) error { beforeCalls++; return nil },
				PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
					prepareCalls++
					return request, func(context.Context, string, string) error { return nil }, nil
				},
			},
		})
		if !errors.Is(err, domainmodel.ErrInvalidReasoningEffort) || !reflect.DeepEqual(output, ProviderStreamOutput{}) ||
			len(provider.requests) != 0 || acquireCalls != 0 || beforeCalls != 0 || prepareCalls != 0 {
			t.Fatalf("invalid effort crossed loop admission: effort=%q output=%#v requests=%d acquire=%d before=%d prepare=%d err=%v",
				effort, output, len(provider.requests), acquireCalls, beforeCalls, prepareCalls, err)
		}
	}

	provider := &providerStreamStub{}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{ReasoningEffort: "high"},
		ContinuationAuthority: ProviderContinuationAuthorityInput{
			References:     []domainsecurity.SettledToolReference{{GrantID: "grant", ResultItemID: "result"}},
			ProviderConfig: domainmodel.TurnConfig{ReasoningEffort: " high "},
		},
	})
	if !errors.Is(err, domainmodel.ErrInvalidReasoningEffort) || !reflect.DeepEqual(output, ProviderStreamOutput{}) || len(provider.requests) != 0 {
		t.Fatalf("invalid continuation effort reached continuation authority: output=%#v requests=%d err=%v", output, len(provider.requests), err)
	}
}

func TestStreamProviderWithRetryRejectsAttemptEffortMutationBeforeProvider(t *testing.T) {
	provider := &providerStreamStub{}
	var releases, settlements int
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{ReasoningEffort: "high"},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				return ctx, func() { releases++ }, nil
			},
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.ReasoningEffort = " high "
				return request, func(_ context.Context, status, reason string) error {
					settlements++
					if status != "failed" || reason != "reasoning_effort_invalid" {
						return errors.New("unexpected settlement")
					}
					return nil
				}, nil
			},
		},
	})
	if !errors.Is(err, domainmodel.ErrInvalidReasoningEffort) || !reflect.DeepEqual(output, ProviderStreamOutput{}) ||
		len(provider.requests) != 0 || releases != 1 || settlements != 1 {
		t.Fatalf("mutated effort crossed physical provider boundary: output=%#v requests=%d releases=%d settlements=%d err=%v",
			output, len(provider.requests), releases, settlements, err)
	}
}

func TestStreamProviderWithRetryStreamsTypedChunks(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkRetrying, Text: "retrying upstream", RetryAttempt: 2, RetryMax: 3},
			{Kind: domainmodel.ChunkReasoning, Text: "think", Signature: "sig-1"},
			{Kind: domainmodel.ChunkText, Text: "hello"},
			{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: "call-1", Name: "read"}},
		},
		resultChunks: []domainmodel.Chunk{{
			Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: "call-1", Name: "read", Arguments: []byte(`{}`)},
		}},
	}}}
	events := []string{}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "hi"}}},
		Callbacks: ProviderStreamCallbacks{
			OnTextDelta: func(text string) error {
				events = append(events, "text:"+text)
				return nil
			},
			OnToolCallStart: func(call domainmodel.ToolCall) error {
				events = append(events, "tool:"+call.Name)
				return nil
			},
			OnProviderRetrying: func(attempt int, maxAttempts int, cause error) error {
				events = append(events, "retry:"+cause.Error())
				if attempt != 2 || maxAttempts != 3 {
					t.Fatalf("callback retry metadata mismatch: attempt=%d max=%d", attempt, maxAttempts)
				}
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("stream provider: %v", err)
	}
	if !output.StreamedDeltas || !output.PartialToolStarted || output.Text != "hello" || output.Reasoning != "think" || output.ReasoningSignature != "sig-1" {
		t.Fatalf("unexpected stream output: %#v", output)
	}
	if !output.Result.HasDuration || !output.Result.HasFirstTokenLatency {
		t.Fatalf("expected timing metadata on streamed result: %#v", output.Result)
	}
	if !output.Result.HasFirstReasoningLatency || !output.Result.HasFirstRawTextLatency ||
		output.Result.FirstRawTextLatencyMs < output.Result.FirstReasoningLatencyMs {
		t.Fatalf("private reasoning and raw text timing were not distinguished: %#v", output.Result)
	}
	if got := strings.Join(events, "|"); got != "retry:retrying upstream|tool:read|text:hello" {
		t.Fatalf("unexpected callback order: %s", got)
	}
}

func TestProviderToolCallIdentityIsOpaqueBeforeCallbackAndDurableResult(t *testing.T) {
	const raw = "provider_call_6222020202020202020"
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{
			Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: raw, Name: "read"},
		}},
		resultChunks: []domainmodel.Chunk{{
			Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: raw, Name: "read", Arguments: []byte(`{}`)},
		}},
	}}}
	callbackID := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, toolCallIDRandom: bytes.NewReader(bytes.Repeat([]byte{0x41}, domainmodel.HostToolCallIDEntropyBytesV1)),
		Callbacks: ProviderStreamCallbacks{OnToolCallStart: func(call domainmodel.ToolCall) error {
			callbackID = call.ID
			return nil
		}},
	})
	if err != nil || len(output.Result.Chunks) != 1 {
		t.Fatalf("provider tool call normalization failed: output=%#v err=%v", output, err)
	}
	resultID := output.Result.Chunks[0].ToolCall.ID
	if callbackID == "" || callbackID != resultID || !domainmodel.IsHostToolCallIDV1(resultID) ||
		strings.Contains(callbackID, raw) || strings.Contains(resultID, "6222020202020202020") {
		t.Fatalf("provider tool-call ID crossed the host boundary: callback=%q result=%q", callbackID, resultID)
	}
}

func TestProviderToolCallIdentityIsFreshWhenRawIDIsReusedInSameTurn(t *testing.T) {
	const raw = "provider_call_reused"
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: raw, Name: "read", Arguments: []byte(`{}`)}}}},
		{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: raw, Name: "read", Arguments: []byte(`{}`)}}}},
	}}
	random := bytes.NewReader(append(
		bytes.Repeat([]byte{0x11}, domainmodel.HostToolCallIDEntropyBytesV1),
		bytes.Repeat([]byte{0x22}, domainmodel.HostToolCallIDEntropyBytesV1)...,
	))
	first, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider, toolCallIDRandom: random})
	if err != nil {
		t.Fatal(err)
	}
	second, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider, toolCallIDRandom: random})
	if err != nil {
		t.Fatal(err)
	}
	firstID, secondID := first.Result.Chunks[0].ToolCall.ID, second.Result.Chunks[0].ToolCall.ID
	if firstID == secondID || !domainmodel.IsHostToolCallIDV1(firstID) || !domainmodel.IsHostToolCallIDV1(secondID) {
		t.Fatalf("reused provider ID did not receive fresh host identities: first=%q second=%q", firstID, secondID)
	}
}

func TestProviderToolCallIdentityRejectsStartFinalMismatch(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: "raw-a", Name: "read"}}},
		resultChunks:   []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: "raw-b", Name: "read", Arguments: []byte(`{}`)}}},
	}}}
	callbackID := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		toolCallIDRandom: bytes.NewReader(append(
			bytes.Repeat([]byte{0x31}, domainmodel.HostToolCallIDEntropyBytesV1),
			bytes.Repeat([]byte{0x32}, domainmodel.HostToolCallIDEntropyBytesV1)...,
		)),
		Callbacks: ProviderStreamCallbacks{OnToolCallStart: func(call domainmodel.ToolCall) error {
			callbackID = call.ID
			return nil
		}},
	})
	if !errors.Is(err, errProviderToolCallTranscriptMismatchV1) || !domainmodel.IsHostToolCallIDV1(callbackID) ||
		len(output.Result.Chunks) != 0 || strings.Contains(err.Error(), "raw-a") || strings.Contains(err.Error(), "raw-b") {
		t.Fatalf("mismatched provider transcript escaped fail-closed handling: callback=%q output=%#v err=%v", callbackID, output, err)
	}
}

func TestProviderCannotForgeHostToolCallIdentityPrefix(t *testing.T) {
	raw := domainmodel.HostToolCallIDPrefixV1 + strings.Repeat("a", domainmodel.HostToolCallIDEntropyBytesV1*2)
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: raw, Name: "read", Arguments: []byte(`{}`)}}},
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, toolCallIDRandom: bytes.NewReader(bytes.Repeat([]byte{0x55}, domainmodel.HostToolCallIDEntropyBytesV1)),
	})
	if err != nil || len(output.Result.Chunks) != 1 || output.Result.Chunks[0].ToolCall.ID == raw || !domainmodel.IsHostToolCallIDV1(output.Result.Chunks[0].ToolCall.ID) {
		t.Fatalf("provider-forged host prefix was trusted: output=%#v err=%v", output, err)
	}
}

func TestStreamProviderWithRetryKeepsReasoningPrivateWhileStreamingText(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkReasoning, Text: "think"},
			{Kind: domainmodel.ChunkText, Text: "1\n"},
			{Kind: domainmodel.ChunkText, Text: "2\n"},
		},
	}}}
	events := []string{}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "count"}}},
		Callbacks: ProviderStreamCallbacks{
			OnTextDelta: func(text string) error {
				events = append(events, "text:"+text)
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("stream provider: %v", err)
	}
	if got := strings.Join(events, "|"); got != "text:1\n2\n" {
		t.Fatalf("only public text may reach callbacks, got %q", got)
	}
	if output.Reasoning != "think" || output.Text != "1\n2\n" || !output.StreamedDeltas {
		t.Fatalf("unexpected stream output: %#v", output)
	}
}

func TestStreamProviderWithRetryEmitsFreshSyntheticPublicChunk(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{
			Kind: domainmodel.ChunkText, Text: "public",
			ToolCall:  domainmodel.ToolCall{ID: "forged-call", Name: "forged-tool"},
			Signature: "PRIVATE_SIGNATURE", RetryAttempt: 9, RetryMax: 9,
		}},
	}}}
	var seen domainmodel.Chunk
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnTextChunk: func(chunk domainmodel.Chunk) error {
			seen = chunk
			return nil
		}},
	})
	want := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "public"}
	if err != nil || output.Text != "public" || !reflect.DeepEqual(seen, want) {
		t.Fatalf("public callback reused untrusted chunk fields: output=%#v seen=%#v err=%v", output, seen, err)
	}
}

func TestStreamProviderWithRetryPublishesAfterSettlementBeforeLeaseRelease(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "public"}},
	}}}
	active := false
	settled := false
	callbackSeen := false
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				active = true
				return ctx, func() {
					if !callbackSeen || !settled {
						t.Fatal("provider attempt lease released before settlement and final callback")
					}
					active = false
				}, nil
			},
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				return request, func(context.Context, string, string) error {
					if !active {
						t.Fatal("provider attempt settled outside its effect lease")
					}
					settled = true
					return nil
				}, nil
			},
			OnTextDelta: func(text string) error {
				if !active || !settled || text != "public" {
					t.Fatalf("final callback escaped authority: active=%v settled=%v text=%q", active, settled, text)
				}
				callbackSeen = true
				return nil
			},
		},
	})
	if err != nil || active || !settled || !callbackSeen || output.Text != "public" {
		t.Fatalf("provider final authority ordering mismatch: output=%#v active=%v settled=%v callback=%v err=%v", output, active, settled, callbackSeen, err)
	}
}

func TestStreamProviderWithRetryRejectsOutputReturnedAfterHostCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &cancellationIgnoringStreamStub{cancel: cancel}
	public := ""
	output, err := StreamProviderWithRetry(ctx, ProviderStreamInput{
		Provider: provider,
		Request: domainmodel.Request{
			ProviderID: "provider-cancelled", Family: "deepseek", EndpointFormat: "chat_completions", Model: "deepseek-chat",
		},
		Callbacks: ProviderStreamCallbacks{OnTextDelta: func(text string) error {
			public += text
			return nil
		}},
	})
	if !errors.Is(err, context.Canceled) || public != "" || len(output.Result.Chunks) != 0 || output.Text != "" || output.Reasoning != "" {
		t.Fatalf("cancelled provider output crossed the host barrier: output=%#v public=%q err=%v", output, public, err)
	}
	if output.Result.ProviderID != "provider-cancelled" || output.Result.Usage.TotalTokens != 15 ||
		output.Result.PrefixShape.PrefixHash == "" || len(output.Result.CacheObservations) != 1 {
		t.Fatalf("cancelled provider diagnostics were discarded: %#v", output.Result)
	}
	var callbackErr ProviderStreamCallbackError
	if !errors.As(provider.callbackErr, &callbackErr) || !errors.Is(provider.callbackErr, context.Canceled) {
		t.Fatalf("provider callback did not receive the cancellation blocker: %v", provider.callbackErr)
	}
}

func TestStreamProviderWithRetryRejectsOutputAfterAttemptLeaseCancellation(t *testing.T) {
	provider := &attemptCancellationStreamStub{}
	public := ""
	releases := 0
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{ProviderID: "attempt-cancelled"},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				attemptCtx, cancel := context.WithCancel(ctx)
				provider.cancel = cancel
				return attemptCtx, func() { releases++ }, nil
			},
			OnTextDelta: func(text string) error {
				public += text
				return nil
			},
		},
	})
	var callbackErr ProviderStreamCallbackError
	if !errors.As(err, &callbackErr) || !errors.Is(err, context.Canceled) || releases != 1 ||
		public != "" || output.Text != "" || output.Reasoning != "" || len(output.Result.Chunks) != 0 ||
		output.Result.ProviderID != "attempt-cancelled" {
		t.Fatalf("cancelled attempt lease crossed host boundary: output=%#v public=%q releases=%d err=%v", output, public, releases, err)
	}
}

func TestStreamProviderWithRetryDropsThinkMarkupAcrossChunks(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "public<Th"},
			{Kind: domainmodel.ChunkText, Text: "InK class=\"x>y\">PRIVATE_REASONING_SENTINEL</tHi"},
			{Kind: domainmodel.ChunkText, Text: "Nk >answer"},
		},
	}}}
	events := []string{}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{OnTextDelta: func(text string) error {
			events = append(events, text)
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("stream provider: %v", err)
	}
	if output.Text != "publicanswer" || strings.Contains(strings.Join(events, ""), "PRIVATE_REASONING_SENTINEL") {
		t.Fatalf("private think markup leaked: output=%#v callbacks=%#v", output, events)
	}
}

func TestStreamProviderWithRetryDiscardsCandidateBeforeOrphanClose(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "PRIVATE_REASONING_SENTINEL"},
			{Kind: domainmodel.ChunkText, Text: "</think>public"},
		},
	}}}
	public := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnTextDelta: func(text string) error {
			public += text
			return nil
		}},
	})
	if err != nil || output.Text != "public" || public != "public" {
		t.Fatalf("orphan close did not fail closed: output=%#v public=%q err=%v", output, public, err)
	}
}

func TestStreamProviderWithRetryRejectsIncompleteReasoningMarkupWithoutRecovery(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_STRUCTURED_REASONING", Signature: "private-signature"},
			{Kind: domainmodel.ChunkText, Text: "public<think>PRIVATE_REASONING_SENTINEL"},
		},
	}}}
	public := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnTextDelta: func(text string) error {
			public += text
			return nil
		}},
	})
	var markupErr domainreasoningmarkup.ProtocolError
	if !errors.As(err, &markupErr) || !errors.Is(err, domainreasoningmarkup.ErrIncomplete) ||
		ProviderErrorLooksRetryable(err) || InterruptedStreamCanRecover(err) || len(provider.requests) != 1 ||
		public != "" || output.Text != "" || output.Reasoning != "" || output.ReasoningSignature != "" {
		t.Fatalf("invalid markup escaped its terminal blocker: output=%#v public=%q calls=%d err=%v", output, public, len(provider.requests), err)
	}
	if failure := PublicFailureForError(err); failure.Code() != domainfailure.CodeProviderReasoningMarkupInvalid {
		t.Fatalf("invalid markup public failure = %q, want %q", failure.Code(), domainfailure.CodeProviderReasoningMarkupInvalid)
	}
}

func TestStreamProviderWithRetryRejectsProviderNativeReasoningBeforeCallbacksWithoutRecovery(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: `{"type":"response.reasoning_summary_text.delta",`},
			{Kind: domainmodel.ChunkText, Text: `"delta":"PRIVATE_NATIVE_REASONING"}`},
		},
	}}}
	public := ""
	retries := 0
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{
			OnTextDelta: func(text string) error {
				public += text
				return nil
			},
			OnProviderRetrying: func(int, int, error) error {
				retries++
				return nil
			},
		},
	})
	var protocolErr domainreasoningmarkup.ProtocolError
	if !errors.As(err, &protocolErr) || !errors.Is(err, domainreasoningmarkup.ErrPrivateContent) ||
		ProviderErrorLooksRetryable(err) || InterruptedStreamCanRecover(err) || len(provider.requests) != 1 || retries != 0 ||
		public != "" || output.Text != "" || output.Reasoning != "" || output.ReasoningSignature != "" || len(output.Result.Chunks) != 0 {
		t.Fatalf("provider-native reasoning escaped terminal admission: output=%#v public=%q calls=%d retries=%d err=%v",
			output, public, len(provider.requests), retries, err)
	}
}

func TestStreamProviderWithRetryRetriesBeforeVisibleOutput(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{cacheTelemetry: []domaincache.ProviderCallObservationV1{providerStreamCacheObservation("a", domaincache.ProviderCallStatusFailed)}, err: fakeProviderRetryError{retryable: true}},
		{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "final"}}, cacheTelemetry: []domaincache.ProviderCallObservationV1{providerStreamCacheObservation("b", domaincache.ProviderCallStatusSucceeded)}},
	}}
	retries := []int{}
	attempts := []int{}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{
			BeforeProviderAttempt: func(attempt int) error {
				attempts = append(attempts, attempt)
				return nil
			},
			OnProviderRetrying: func(attempt int, _ int, _ error) error {
				retries = append(retries, attempt)
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("stream provider retry: %v", err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("expected retry before visible output, calls=%d", len(provider.requests))
	}
	if len(retries) != 1 || retries[0] != 2 {
		t.Fatalf("unexpected retry callbacks: %#v", retries)
	}
	if !reflect.DeepEqual(attempts, []int{1, 2}) {
		t.Fatalf("provider authority was not revalidated before every attempt: %#v", attempts)
	}
	if output.StreamedDeltas || output.Result.HasFirstTokenLatency || output.Result.Chunks[0].Text != "final" {
		t.Fatalf("unexpected retry output: %#v", output)
	}
	if len(output.Result.CacheObservations) != 2 ||
		output.Result.CacheObservations[0].Status != domaincache.ProviderCallStatusFailed ||
		output.Result.CacheObservations[1].Status != domaincache.ProviderCallStatusSucceeded {
		t.Fatalf("outer provider retry discarded physical attempt settlements: %#v", output.Result.CacheObservations)
	}
}

func TestStreamProviderWithRetryDiscardsFailedAttemptReasoningBeforeRetry(t *testing.T) {
	const failedReasoning = "PRIVATE_FAILED_ATTEMPT_REASONING_SENTINEL"
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkReasoning, Text: failedReasoning, Signature: "failed-signature"}},
			resultChunks:   []domainmodel.Chunk{{Kind: domainmodel.ChunkReasoning, Text: failedReasoning, Signature: "failed-signature"}},
			err:            fakeProviderRetryError{retryable: true},
		},
		{
			callbackChunks: []domainmodel.Chunk{
				{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_SUCCESSFUL_ATTEMPT_REASONING", Signature: "successful-signature"},
				{Kind: domainmodel.ChunkText, Text: "verified public answer"},
			},
			resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "verified public answer"}},
		},
	}}
	public := ""
	retries := 0
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{
			OnTextDelta: func(text string) error {
				public += text
				return nil
			},
			OnProviderRetrying: func(int, int, error) error {
				retries++
				return nil
			},
		},
	})
	if err != nil {
		t.Fatalf("reasoning-only provider retry: %v", err)
	}
	if len(provider.requests) != 2 || retries != 1 {
		t.Fatalf("reasoning-only failure did not retry exactly once: calls=%d retries=%d", len(provider.requests), retries)
	}
	if public != "verified public answer" || output.Text != public {
		t.Fatalf("private reasoning crossed the public callback boundary: public=%q output=%#v", public, output)
	}
	if strings.Contains(output.Reasoning, failedReasoning) || output.ReasoningSignature == "failed-signature" ||
		strings.Contains(output.Text, failedReasoning) || strings.Contains(public, failedReasoning) {
		t.Fatalf("failed-attempt reasoning survived retry: output=%#v public=%q", output, public)
	}
	if output.Reasoning != "PRIVATE_SUCCESSFUL_ATTEMPT_REASONING" || output.ReasoningSignature != "successful-signature" {
		t.Fatalf("successful attempt state was not isolated: %#v", output)
	}
}

func TestStreamProviderWithRetryUsesTypedAdapterOutputObservation(t *testing.T) {
	t.Run("explicit none remains retryable", func(t *testing.T) {
		provider := &providerStreamStub{responses: []providerStreamResponse{
			{err: observedProviderRetryError{
				fakeProviderRetryError: fakeProviderRetryError{retryable: true},
				observation:            domainmodel.ProviderOutputObservationNoneV1,
			}},
			{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "replacement"}}},
		}}
		output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider})
		if err != nil || len(provider.requests) != 2 || output.Result.Chunks[0].Text != "replacement" {
			t.Fatalf("typed no-output observation did not retry safely: output=%#v calls=%d err=%v", output, len(provider.requests), err)
		}
	})

	t.Run("private reasoning remains retryable", func(t *testing.T) {
		provider := &providerStreamStub{responses: []providerStreamResponse{
			{
				callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_TYPED_REASONING"}},
				err: observedProviderRetryError{
					fakeProviderRetryError: fakeProviderRetryError{retryable: true},
					observation:            domainmodel.ProviderOutputObservationPrivateReasoningV1,
				},
			},
			{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "replacement"}}},
		}}
		output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider})
		if err != nil || len(provider.requests) != 2 || output.Result.Chunks[0].Text != "replacement" ||
			strings.Contains(output.Reasoning, "PRIVATE_TYPED_REASONING") {
			t.Fatalf("typed private reasoning did not retry safely: output=%#v calls=%d err=%v", output, len(provider.requests), err)
		}
	})

	for _, test := range []struct {
		name        string
		observation domainmodel.ProviderOutputObservationV1
	}{
		{name: "public_text", observation: domainmodel.ProviderOutputObservationPublicTextV1},
		{name: "tool_started", observation: domainmodel.ProviderOutputObservationToolStartedV1},
		{name: "public_text_and_tool_started", observation: domainmodel.ProviderOutputObservationPublicTextV1.MergeV1(domainmodel.ProviderOutputObservationToolStartedV1)},
		{name: "unknown", observation: domainmodel.ProviderOutputObservationV1(1 << 7)},
	} {
		t.Run(test.name+" is terminal", func(t *testing.T) {
			provider := &providerStreamStub{responses: []providerStreamResponse{
				{err: observedProviderRetryError{
					fakeProviderRetryError: fakeProviderRetryError{retryable: true},
					observation:            test.observation,
				}},
				{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "must-not-run"}}},
			}}
			output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider})
			if err == nil || len(provider.requests) != 1 {
				t.Fatalf("typed public output was retried: output=%#v calls=%d err=%v", output, len(provider.requests), err)
			}
			if test.observation.IncludesV1(domainmodel.ProviderOutputObservationPublicTextV1) && !output.PartialTextStarted {
				t.Fatal("typed public text did not set the retry barrier")
			}
			if test.observation.IncludesV1(domainmodel.ProviderOutputObservationToolStartedV1) && !output.PartialToolStarted {
				t.Fatal("typed tool start did not set the retry barrier")
			}
		})
	}
}

func TestStreamProviderWithRetryRejectsTransportReconnectAfterText(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_FAILED_REASONING", Signature: "failed-signature"},
			{Kind: domainmodel.ChunkText, Text: "discarded partial"},
			{Kind: domainmodel.ChunkRetrying, Text: "transport reconnect", RetryAttempt: 2, RetryMax: 2},
			{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_CURRENT_REASONING", Signature: "current-signature"},
			{Kind: domainmodel.ChunkText, Text: "replacement"},
		},
	}}}
	public := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnTextDelta: func(text string) error {
			public += text
			return nil
		}},
	})
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() || public != "" || output.Text != "" ||
		!output.PartialTextStarted || !output.StreamedDeltas || output.Reasoning != "" || output.ReasoningSignature != "" {
		t.Fatalf("post-output transport reconnect did not fail closed: output=%#v public=%q err=%v", output, public, err)
	}
}

func TestStreamProviderWithRetryTimingExcludesDiscardedPrivateAttempt(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkReasoning, Text: "discarded private attempt"},
			{Kind: domainmodel.ChunkRetrying, RetryAttempt: 2, RetryMax: 2},
			{Kind: domainmodel.ChunkText, Text: "replacement"},
			{Kind: domainmodel.ChunkDone},
		},
	}}}
	var retryAt time.Time
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnProviderRetrying: func(int, int, error) error {
			retryAt = time.Now()
			return nil
		}},
	})
	if err != nil || retryAt.IsZero() || output.Result.HostTiming.FirstContentAt.Before(retryAt) ||
		output.Result.HostTiming.FirstTextAt.Before(retryAt) || output.Reasoning != "" {
		t.Fatalf("replacement candidate retained discarded timing or reasoning: err=%v timing=%+v", err, output.Result.HostTiming)
	}
}

func TestStreamProviderWithRetryBindsDistinctOuterAttempts(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{err: fakeProviderRetryError{retryable: true}},
		{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "final"}}},
	}}
	binding := &domainmodel.ProviderTelemetryBindingV1{
		UsageSource: domaincache.ProviderUsageSourceTurn, Channel: domaincache.ProviderChannelPrimary, LogicalSequence: 9,
	}
	if _, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{PrivateProviderTelemetry: binding},
	}); err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || provider.requests[0].PrivateProviderTelemetry == nil || provider.requests[1].PrivateProviderTelemetry == nil ||
		provider.requests[0].PrivateProviderTelemetry.OuterAttempt != 1 || provider.requests[1].PrivateProviderTelemetry.OuterAttempt != 2 ||
		provider.requests[0].PrivateProviderTelemetry.LogicalSequence != 9 || provider.requests[1].PrivateProviderTelemetry.LogicalSequence != 9 ||
		binding.OuterAttempt != 0 {
		t.Fatalf("outer provider retry reused or mutated telemetry authority: requests=%#v binding=%#v", provider.requests, binding)
	}
}

func TestStreamProviderWithRetryNeverRetriesForbiddenAuthorityFailure(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		resultChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "UNCOMMITTED_PROVIDER_SENTINEL"},
			{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_FAILED_REASONING"},
		},
		err: fakeProviderRetryForbiddenError{},
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider})
	if err == nil {
		t.Fatal("expected durable authority failure")
	}
	if len(provider.requests) != 1 || len(output.Result.Chunks) != 0 || output.Text != "" || output.Reasoning != "" {
		t.Fatalf("durable authority failure was retried or retained provider bytes: calls=%d output=%#v", len(provider.requests), output)
	}
}

func TestStreamProviderWithRetrySealsPriorAttemptBeforeNextAuthorityFailure(t *testing.T) {
	blocked := errors.New("next provider authority unavailable")
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		resultChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "UNCOMMITTED_PROVIDER_SENTINEL"},
			{Kind: domainmodel.ChunkReasoning, Text: "PRIVATE_FAILED_REASONING"},
		},
		err: fakeProviderRetryError{retryable: true},
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{BeforeProviderAttempt: func(attempt int) error {
			if attempt == 2 {
				return blocked
			}
			return nil
		}},
	})
	if !errors.Is(err, blocked) || len(provider.requests) != 1 || len(output.Result.Chunks) != 0 ||
		output.Text != "" || output.Reasoning != "" || output.ReasoningSignature != "" {
		t.Fatalf("prior attempt bytes survived authority failure: output=%#v calls=%d err=%v", output, len(provider.requests), err)
	}
}

func TestStreamProviderWithRetryTreatsMalformedToolStartAsRetryBarrier(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkToolCallStart}},
		err:            fakeProviderRetryError{retryable: true},
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider})
	if err == nil || len(provider.requests) != 1 || !output.PartialToolStarted || len(output.Result.Chunks) != 0 {
		t.Fatalf("malformed tool start was retried or retained: output=%#v calls=%d err=%v", output, len(provider.requests), err)
	}
}

func TestStreamProviderWithRetryPreservesSettledAttemptWhenCancelledBetweenRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		cacheTelemetry: []domaincache.ProviderCallObservationV1{providerStreamCacheObservation("a", domaincache.ProviderCallStatusFailed)},
		err:            fakeProviderRetryError{retryable: true},
	}}}
	output, err := StreamProviderWithRetry(ctx, ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{OnProviderRetrying: func(int, int, error) error {
			cancel()
			return nil
		}},
	})
	if !errors.Is(err, context.Canceled) || len(provider.requests) != 1 || len(output.Result.CacheObservations) != 1 ||
		output.Result.CacheObservations[0].Status != domaincache.ProviderCallStatusFailed {
		t.Fatalf("cancellation between retries discarded a settled provider attempt: output=%#v calls=%d err=%v", output, len(provider.requests), err)
	}
}

func providerStreamCacheObservation(logicalByte string, status domaincache.ProviderCallStatusV1) domaincache.ProviderCallObservationV1 {
	return domaincache.ProviderCallObservationV1{
		SchemaVersion: domaincache.ProviderCallObservationV1SchemaVersion,
		Shape: domaincache.CacheVisibleShapeV1{
			SchemaVersion:   domaincache.CacheVisibleShapeV1SchemaVersion,
			LogicalCallHMAC: strings.Repeat(logicalByte, 64), Attempt: 1,
			ProviderFamily: domaincache.ProviderFamilyDeepSeek, ModelHMAC: strings.Repeat("c", 64),
			Endpoint: domaincache.EndpointFormatChatCompletions, EndpointHMAC: strings.Repeat("d", 64),
			WireBodyHMAC: strings.Repeat("e", 64), CredentialScopeHMAC: strings.Repeat("f", 64),
			ProviderConfigHMAC: strings.Repeat("1", 64), DigestEpoch: 1, StartedAt: "2026-07-13T00:00:00Z",
		},
		Status: status,
		Usage: domaincache.ProviderUsageV1{
			InputTokens: domaincache.TokenCountV1{}, OutputTokens: domaincache.TokenCountV1{},
			CacheHitTokens: domaincache.TokenCountV1{}, CacheMissTokens: domaincache.TokenCountV1{},
			ReasoningTokens: domaincache.TokenCountV1{},
		},
		SettledAt: "2026-07-13T00:00:01Z",
	}
}

func TestStreamProviderWithRetryStopsBeforeTransportWhenAttemptAuthorityFails(t *testing.T) {
	blocked := errors.New("provider continuation authority closed")
	provider := &providerStreamStub{responses: []providerStreamResponse{{}}}
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider:  provider,
		Request:   domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{BeforeProviderAttempt: func(int) error { return blocked }},
	})
	var callbackErr ProviderStreamCallbackError
	if !errors.As(err, &callbackErr) || !errors.Is(err, blocked) || len(provider.requests) != 0 {
		t.Fatalf("provider transport ran without open authority: err=%v calls=%d", err, len(provider.requests))
	}
}

func TestStreamProviderWithRetryHoldsAndReleasesExactEffectLeasePerAttempt(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{err: fakeProviderRetryError{retryable: true}},
		{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "done"}}},
	}}
	active := false
	acquired := []int{}
	released := []int{}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, attempt int) (context.Context, func(), error) {
				if active {
					t.Fatal("next provider attempt acquired before the prior effect lease was released")
				}
				active = true
				acquired = append(acquired, attempt)
				releasedOnce := false
				return ctx, func() {
					if releasedOnce || !active {
						t.Fatal("provider effect lease was released more than once")
					}
					releasedOnce = true
					active = false
					released = append(released, attempt)
				}, nil
			},
			BeforeProviderAttempt: func(int) error {
				if !active {
					t.Fatal("provider authority validation ran outside the effect lease")
				}
				return nil
			},
		},
	})
	if err != nil || active || output.Result.Chunks[0].Text != "done" ||
		!reflect.DeepEqual(acquired, []int{1, 2}) || !reflect.DeepEqual(released, []int{1, 2}) {
		t.Fatalf("provider attempt leases diverged: output=%#v active=%v acquired=%#v released=%#v err=%v", output, active, acquired, released, err)
	}
}

func TestStreamProviderWithRetryMaterializesAndSettlesPrivatePayloadInsideEveryEffectLease(t *testing.T) {
	active := false
	settlements := []string{}
	releases := 0
	provider := &privateAttemptProviderStub{active: &active, responses: []providerStreamResponse{
		{err: fakeProviderRetryError{retryable: true}},
		{resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "done"}}},
	}}
	baseRequest := domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "analyze"}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  baseRequest,
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				if active {
					t.Fatal("private provider attempts overlapped")
				}
				active = true
				settledAtRelease := len(settlements)
				return ctx, func() {
					if !active || len(settlements) != settledAtRelease+1 {
						t.Fatal("private payload lease released before durable settlement")
					}
					active = false
					releases++
				}, nil
			},
			PrepareProviderAttempt: func(_ context.Context, attempt int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				if !active || len(request.Messages[0].Parts) != 0 {
					t.Fatal("private payload was retained outside attempt materialization")
				}
				request.Messages = []domainmodel.Message{{
					Role: "user", Content: "analyze", Parts: []domainmodel.MessagePart{{Type: "text", Text: "ATTEMPT_LOCAL_TEXT_SENTINEL"}},
				}}
				request.BeforeSend = func(int) error {
					if !active {
						return errors.New("physical send escaped effect lease")
					}
					return nil
				}
				return request, func(_ context.Context, status, reason string) error {
					if !active {
						t.Fatal("private provider settlement ran outside effect lease")
					}
					settlements = append(settlements, status+":"+reason+":"+string(rune('0'+attempt)))
					return nil
				}, nil
			},
		},
	})
	if err != nil || active || releases != 2 || provider.calls != 2 || output.Result.Chunks[0].Text != "done" ||
		!reflect.DeepEqual(settlements, []string{"failed:provider_attempt_failed:1", "consumed:provider_attempt_completed:2"}) ||
		len(baseRequest.Messages[0].Parts) != 0 {
		t.Fatalf("private attempt lifecycle mismatch: output=%#v settlements=%#v releases=%d calls=%d active=%v err=%v", output, settlements, releases, provider.calls, active, err)
	}
}

func TestStreamProviderWithRetrySettlesPrivatePayloadAsCancelledBeforeRelease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	active := false
	settled := ""
	provider := &privateAttemptProviderStub{active: &active, cancel: cancel, responses: []providerStreamResponse{{}}}
	_, err := StreamProviderWithRetry(ctx, ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{Messages: []domainmodel.Message{{Role: "user"}}},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				active = true
				return ctx, func() {
					if settled != "cancelled:provider_attempt_cancelled" {
						t.Fatal("cancelled private payload released before cancellation settlement")
					}
					active = false
				}, nil
			},
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.Messages[0].Parts = []domainmodel.MessagePart{{Type: "text", Text: "ATTEMPT_LOCAL_TEXT_SENTINEL"}}
				request.BeforeSend = func(int) error { return nil }
				return request, func(_ context.Context, status, reason string) error {
					settled = status + ":" + reason
					return nil
				}, nil
			},
		},
	})
	if !errors.Is(err, context.Canceled) || active || settled != "cancelled:provider_attempt_cancelled" {
		t.Fatalf("cancelled private attempt mismatch: active=%v settled=%q err=%v", active, settled, err)
	}
}

func TestStreamProviderWithRetryRejectsProviderResultWhenPrivateSettlementFails(t *testing.T) {
	active := false
	releases := 0
	settlementFailure := errors.New("attachment disposition readback failed")
	provider := &privateAttemptProviderStub{active: &active, responses: []providerStreamResponse{{
		resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "must not be accepted"}},
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{Messages: []domainmodel.Message{{Role: "user"}}},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				active = true
				return ctx, func() { active = false; releases++ }, nil
			},
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.Messages[0].Parts = []domainmodel.MessagePart{{Type: "text", Text: "ATTEMPT_LOCAL_TEXT_SENTINEL"}}
				request.BeforeSend = func(int) error { return nil }
				return request, func(context.Context, string, string) error { return settlementFailure }, nil
			},
		},
	})
	var callbackErr ProviderStreamCallbackError
	if !errors.As(err, &callbackErr) || !errors.Is(err, settlementFailure) || active || releases != 1 ||
		output.Text != "" || len(output.Result.Chunks) != 0 {
		t.Fatalf("unsettled private provider result was accepted: output=%#v active=%v releases=%d err=%v", output, active, releases, err)
	}
}

func TestStreamProviderWithRetryReleasesEffectLeaseWhenValidationFails(t *testing.T) {
	blocked := errors.New("stale witnessed turn authority")
	provider := &providerStreamStub{responses: []providerStreamResponse{{}}}
	releases := 0
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				return ctx, func() { releases++ }, nil
			},
			BeforeProviderAttempt: func(int) error { return blocked },
		},
	})
	var callbackErr ProviderStreamCallbackError
	if !errors.As(err, &callbackErr) || !errors.Is(err, blocked) || releases != 1 || len(provider.requests) != 0 {
		t.Fatalf("failed provider validation leaked lease or transport: err=%v releases=%d calls=%d", err, releases, len(provider.requests))
	}
}

func TestStreamProviderWithRetryProjectsCasePIIAfterAttachmentMaterialization(t *testing.T) {
	securityContext := newLoopCaseContextV2(t, "thread-provider-privacy", "turn-provider-privacy", t.TempDir(), "case-provider-privacy")
	provider := &providerStreamStub{responses: []providerStreamResponse{{}}}
	active := false
	settled := ""
	rawAccount := "6222020000000000000"
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider:        provider,
		SecurityContext: securityContext,
		Request: domainmodel.Request{Messages: []domainmodel.Message{{
			Role: "user", Content: "核查账号 " + rawAccount,
		}}},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				active = true
				return ctx, func() { active = false }, nil
			},
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.Messages[0].Parts = []domainmodel.MessagePart{{Type: "text", Text: "附件账号 " + rawAccount}}
				return request, func(_ context.Context, status, reason string) error {
					settled = status + ":" + reason
					return nil
				}, nil
			},
		},
	})
	if err != nil || active || settled != "consumed:provider_attempt_completed" || len(provider.requests) != 1 {
		t.Fatalf("projected provider attempt failed: calls=%d active=%v settled=%q err=%v", len(provider.requests), active, settled, err)
	}
	request := provider.requests[0]
	if strings.Contains(request.Messages[0].Content+request.Messages[0].Parts[0].Text, rawAccount) ||
		!strings.Contains(request.Messages[0].Content, "[ACCOUNT]") || !strings.Contains(request.Messages[0].Parts[0].Text, "[ACCOUNT]") {
		t.Fatalf("case PII reached provider after attachment materialization: %#v", request.Messages[0])
	}
}

func TestStreamProviderWithRetryKeepsOrdinaryProviderAtWitnessedCaseBoundary(t *testing.T) {
	securityContext := childContinuationWitnessedBoundaryContext(t)
	provider := &providerStreamStub{responses: []providerStreamResponse{{}}}
	rawAccount := "6222020000000000000"
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider:        provider,
		SecurityContext: securityContext,
		OrdinaryEffect:  true,
		Request: domainmodel.Request{
			Messages: []domainmodel.Message{{Role: "user", Content: "修改普通代码；案件账号 " + rawAccount + " 暂无授权"}},
			PrivateProviderTelemetry: &domainmodel.ProviderTelemetryBindingV1{
				SecurityContext: securityContext, UsageSource: domaincache.ProviderUsageSourceTurn,
				Channel: domaincache.ProviderChannelPrimary, LogicalSequence: 1,
			},
			Tools: []domainmodel.ToolSchema{{
				Name: "read", Description: "Read an ordinary workspace file.",
				Parameters: []byte(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
			}},
		},
		Callbacks: ProviderStreamCallbacks{
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				telemetry := *request.PrivateProviderTelemetry
				telemetry.OrdinaryEffect = false
				request.PrivateProviderTelemetry = &telemetry
				return request, func(context.Context, string, string) error { return nil }, nil
			},
		},
	})
	if err != nil || len(provider.requests) != 1 {
		t.Fatalf("ordinary provider did not continue at a witnessed case boundary: calls=%d err=%v", len(provider.requests), err)
	}
	sent := provider.requests[0]
	if strings.Contains(sent.Messages[0].Content, rawAccount) || !strings.Contains(sent.Messages[0].Content, "[ACCOUNT]") {
		t.Fatalf("ordinary boundary provider received unprojected PII: %#v", sent.Messages)
	}
	if sent.PrivateProviderTelemetry == nil || !sent.PrivateProviderTelemetry.OrdinaryEffect ||
		sent.PrivateProviderTelemetry.OuterAttempt != 1 {
		t.Fatalf("attachment preparation changed host-owned ordinary telemetry effect: %#v", sent.PrivateProviderTelemetry)
	}
}

func TestStreamProviderWithRetryRejectsUninspectableCaseAttachmentBeforeProvider(t *testing.T) {
	securityContext := newLoopCaseContextV2(t, "thread-provider-image", "turn-provider-image", t.TempDir(), "case-provider-image")
	provider := &providerStreamStub{}
	active := false
	settled := ""
	_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider:        provider,
		SecurityContext: securityContext,
		Request:         domainmodel.Request{Messages: []domainmodel.Message{{Role: "user"}}},
		Callbacks: ProviderStreamCallbacks{
			AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
				active = true
				return ctx, func() { active = false }, nil
			},
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.Messages[0].Parts = []domainmodel.MessagePart{{Type: "image", Data: "PRIVATE_CASE_IMAGE"}}
				return request, func(_ context.Context, status, reason string) error {
					settled = status + ":" + reason
					return nil
				}, nil
			},
		},
	})
	var callbackErr ProviderStreamCallbackError
	if !errors.As(err, &callbackErr) || len(provider.requests) != 0 || active || settled != "failed:privacy_projection_failed" {
		t.Fatalf("uninspectable case attachment reached provider or escaped settlement: calls=%d active=%v settled=%q err=%v", len(provider.requests), active, settled, err)
	}
}

func TestStreamProviderWithRetryNativeVisionCapabilityCannotBypassFinalProjection(t *testing.T) {
	securityContext := newLoopGeneralContextV2(t, "thread-provider-general-image", "turn-provider-general-image", t.TempDir())
	const rawSentinel = "UNINSPECTED_GENERAL_IMAGE_731"
	for name, part := range map[string]domainmodel.MessagePart{
		"inline data": {Type: "image", MediaType: "image/png", Data: rawSentinel},
		"image URL":   {Type: "image_url", ImageURL: "https://private.example.test/" + rawSentinel},
		"data URL":    {Type: "image_url", ImageURL: "data:image/png;base64," + rawSentinel},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &providerStreamStub{}
			active := false
			settled := ""
			_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
				Provider:        provider,
				SecurityContext: securityContext,
				OrdinaryEffect:  true,
				Request:         domainmodel.Request{Messages: []domainmodel.Message{{Role: "user"}}},
				ContinuationAuthority: ProviderContinuationAuthorityInput{ProviderConfig: domainmodel.TurnConfig{
					SupportsImageInput: true, InputModalities: []string{"text", "image"}, MessageParts: []string{"text", "image_url"},
				}},
				Callbacks: ProviderStreamCallbacks{
					AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
						active = true
						return ctx, func() { active = false }, nil
					},
					PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
						request.Messages[0].Parts = []domainmodel.MessagePart{part}
						return request, func(_ context.Context, status, reason string) error {
							settled = status + ":" + reason
							return nil
						}, nil
					},
				},
			})
			var callbackErr ProviderStreamCallbackError
			if !errors.As(err, &callbackErr) || len(provider.requests) != 0 || active || settled != "failed:privacy_projection_failed" {
				t.Fatalf(
					"native vision capability bypassed final projection or settlement: calls=%d active=%v settled=%q err=%v",
					len(provider.requests), active, settled, err,
				)
			}
			if strings.Contains(err.Error(), rawSentinel) {
				t.Fatalf("final projection error reflected raw image source: %v", err)
			}
		})
	}
}

func TestStreamProviderWithRetryDoesNotRetryAfterTransactionalTextWasDiscarded(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "discarded partial"}},
			err:            fakeProviderRetryError{retryable: true},
		},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "replacement"}}},
	}}
	public := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{},
		Callbacks: ProviderStreamCallbacks{OnTextDelta: func(text string) error {
			public += text
			return nil
		}},
	})
	if err == nil || len(provider.requests) != 1 || output.Text != "" || public != "" || !output.StreamedDeltas || !output.PartialTextStarted {
		t.Fatalf("transactional text retry did not fail closed: calls=%d output=%#v public=%q err=%v", len(provider.requests), output, public, err)
	}
}

func TestStreamProviderWithRetryDoesNotPublishTextFromFailedToolAttempt(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkText, Text: "discarded partial"},
			{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: "call-1", Name: "read"}},
		},
		err: fakeProviderRetryError{retryable: true},
	}}}
	public := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnTextDelta: func(text string) error {
			public += text
			return nil
		}},
	})
	if err == nil || len(provider.requests) != 1 || !output.PartialToolStarted || output.Text != "" || public != "" {
		t.Fatalf("failed tool attempt published text: calls=%d output=%#v public=%q err=%v", len(provider.requests), output, public, err)
	}
}

func TestProviderStreamByteBudgetRejectsBeforeCallbacksAndReturnedMaterial(t *testing.T) {
	for _, mode := range []string{"callback", "ignored callback", "result", "reasoning", "tool", "tool identity"} {
		t.Run(mode, func(t *testing.T) {
			observed := 0
			chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "12345"}
			if mode == "reasoning" {
				chunk.Kind = domainmodel.ChunkReasoning
			}
			if mode == "tool" {
				chunk = domainmodel.Chunk{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{Name: "abc", Arguments: []byte(`{}`)}}
			}
			if mode == "tool identity" {
				chunk = domainmodel.Chunk{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: "12345"}}
			}
			provider := auxiliaryProviderFunc(func(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
				if mode != "result" {
					err := request.OnChunk(chunk)
					if !errors.Is(err, ErrProviderOutputByteBudgetExceeded) {
						t.Fatalf("oversized chunk accepted: %v", err)
					}
					if mode != "ignored callback" {
						return domainmodel.Result{}, err
					}
				}
				return domainmodel.Result{Chunks: []domainmodel.Chunk{chunk}, StreamCompleted: true}, nil
			})
			output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider, MaxOutputBytes: 4, MaxAttempts: 1,
				Request:   domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "continue"}}},
				Callbacks: ProviderStreamCallbacks{OnTextDelta: func(string) error { observed++; return nil }},
			})
			if !errors.Is(err, ErrProviderOutputByteBudgetExceeded) || observed != 0 || output.Text != "" || len(output.Result.Chunks) != 0 {
				t.Fatalf("oversized material escaped budget: callbacks=%d err=%v", observed, err)
			}
		})
	}
}

// This measures the complete local stream use case (validation, projection,
// chunk budgets, decoder, normalization and final callback), without sockets,
// persistence or UI. The separate installed measurement owns those stages.
func BenchmarkProviderStreamChunking(b *testing.B) {
	for _, kind := range []domainmodel.ChunkKind{domainmodel.ChunkText, domainmodel.ChunkReasoning} {
		for _, count := range []int{1, 64, 1024} {
			for _, unit := range []string{"a", "字"} {
				b.Run(fmt.Sprintf("%s/%d/%d", kind, count, len(unit)), func(b *testing.B) {
					value := strings.Repeat(unit, (64*1024)/len(unit))
					runes := []rune(value)
					chunks := make([]domainmodel.Chunk, 0, count+1)
					for i := 0; i < count; i++ {
						chunks = append(chunks, domainmodel.Chunk{Kind: kind, Text: string(runes[len(runes)*i/count : len(runes)*(i+1)/count])})
					}
					chunks = append(chunks, domainmodel.Chunk{Kind: domainmodel.ChunkDone})
					b.ReportAllocs()
					b.SetBytes(int64(len(value)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						provider := &providerStreamStub{responses: []providerStreamResponse{{callbackChunks: chunks, resultChunks: chunks}}}
						out, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider, Request: domainmodel.Request{MaxOutputTokens: 65536}, MaxOutputBytes: 256 * 1024})
						if err != nil {
							b.Fatal(err)
						}
						got := out.Text
						if kind == domainmodel.ChunkReasoning {
							got = out.Reasoning
						}
						if got != value {
							b.Fatal("stream output changed")
						}
					}
				})
			}
		}
	}
}

func processFrameChunkForTest(kind string, attempt uint64, nativeIndex int, text string) domainmodel.Chunk {
	chunk := domainmodel.Chunk{
		Kind: domainmodel.ChunkReasoning,
		ProcessFrame: &domainmodel.ProcessFrameV1{
			Kind: kind, Attempt: attempt, NativeIndex: nativeIndex, Text: text,
		},
	}
	if kind == "delta" {
		chunk.Text = text
	}
	return chunk
}

func TestStreamProviderProcessCallbackPanicCannotAlterContinuationOrOutcome(t *testing.T) {
	const reasoning = "PRIVATE_ORIGINAL_CONTINUATION_TEXT"
	const signature = "PRIVATE_ORIGINAL_CONTINUATION_SIGNATURE"
	chunks := []domainmodel.Chunk{
		processFrameChunkForTest("start", 1, 8, ""),
		processFrameChunkForTest("delta", 1, 8, reasoning),
		{Kind: domainmodel.ChunkReasoning, Signature: signature},
		processFrameChunkForTest("end", 1, 8, ""),
		{Kind: domainmodel.ChunkText, Text: "verified public answer"},
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: chunks,
		resultChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkReasoning, Text: reasoning, Signature: signature},
			{Kind: domainmodel.ChunkText, Text: "verified public answer"},
		},
	}}}
	var delivered []string
	public := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{
			OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
				if frame.Kind != "start" && frame.Kind != "delta" && frame.Kind != "end" {
					return
				}
				delivered = append(delivered, frame.Kind)
				frame.Text = "CALLBACK_MUTATED_PRIVATE_TEXT"
				if frame.Kind == "delta" {
					panic("display sink unavailable")
				}
			},
			OnTextDelta: func(text string) error {
				public += text
				return nil
			},
		},
	})
	if err != nil || len(provider.requests) != 1 || output.Text != "verified public answer" || public != output.Text {
		t.Fatalf("display callback changed provider completion: calls=%d public=%q err=%v", len(provider.requests), public, err)
	}
	if !reflect.DeepEqual(delivered, []string{"start", "delta", "end"}) ||
		output.Reasoning != reasoning || output.ReasoningSignature != signature {
		t.Fatal("display panic or mutation changed native framing or continuation")
	}
	if chunks[1].Text != reasoning || chunks[1].ProcessFrame.Text != reasoning {
		t.Fatal("callback received mutable access to the provider continuation")
	}
}

func TestStreamProviderProcessAttemptsRemainMonotonicAcrossTransportAndOuterRetry(t *testing.T) {
	const first = "PRIVATE_OUTER_ONE_PHYSICAL_ONE"
	const second = "PRIVATE_OUTER_ONE_PHYSICAL_TWO"
	const final = "PRIVATE_OUTER_TWO_PHYSICAL_ONE"
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{
				processFrameChunkForTest("reset", 1, -1, ""),
				processFrameChunkForTest("start", 1, 8, ""),
				processFrameChunkForTest("delta", 1, 8, first),
				processFrameChunkForTest("terminal", 1, -1, ""),
				{Kind: domainmodel.ChunkRetrying, RetryAttempt: 2, RetryMax: 2},
				processFrameChunkForTest("reset", 2, -1, ""),
				processFrameChunkForTest("start", 2, 8, ""),
				processFrameChunkForTest("delta", 2, 8, second),
				processFrameChunkForTest("terminal", 2, -1, ""),
			},
			err: fakeProviderRetryError{retryable: true},
		},
		{
			callbackChunks: []domainmodel.Chunk{
				processFrameChunkForTest("reset", 1, -1, ""),
				processFrameChunkForTest("start", 1, 8, ""),
				processFrameChunkForTest("delta", 1, 8, final),
				processFrameChunkForTest("end", 1, 8, ""),
				{Kind: domainmodel.ChunkText, Text: "verified replacement"},
			},
			resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "verified replacement"}},
		},
	}}
	var frames []domainmodel.ProcessFrameV1
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, MaxAttempts: 2,
		Callbacks: ProviderStreamCallbacks{OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
			frames = append(frames, frame)
		}},
	})
	if err != nil || len(provider.requests) != 2 || output.Text != "verified replacement" || output.Reasoning != final {
		t.Fatalf("nested retry did not isolate replacement continuation: calls=%d err=%v", len(provider.requests), err)
	}
	var deltaAttempts []uint64
	var deltaText []string
	lastAttempt := uint64(0)
	resetAttempt := uint64(0)
	for _, frame := range frames {
		if frame.Attempt < lastAttempt || frame.Attempt == 0 {
			t.Fatalf("physical process attempt regressed: %#v", frames)
		}
		lastAttempt = frame.Attempt
		if frame.Kind == "reset" {
			resetAttempt = frame.Attempt
		}
		if frame.Kind == "delta" {
			if resetAttempt != frame.Attempt || frame.NativeIndex != 8 {
				t.Fatalf("replacement bytes arrived before their physical reset: %#v", frames)
			}
			deltaAttempts = append(deltaAttempts, frame.Attempt)
			deltaText = append(deltaText, frame.Text)
		}
		if frame.Kind != "delta" && frame.Text != "" {
			t.Fatal("process lifecycle frame retained private bytes")
		}
	}
	if !reflect.DeepEqual(deltaAttempts, []uint64{1, 2, 3}) || !reflect.DeepEqual(deltaText, []string{first, second, final}) {
		t.Fatalf("nested physical sends were not independently identified: %#v", frames)
	}
}

func TestStreamProviderSummaryFramesStayPrivateAcrossRetryAndRejectLateAttempts(t *testing.T) {
	summary := func(kind string, attempt uint64, text string) domainmodel.Chunk {
		return domainmodel.Chunk{Kind: domainmodel.ChunkReasoning, ProcessFrame: &domainmodel.ProcessFrameV1{Source: "provider_reasoning", ReasoningForm: "summary", Kind: kind, Attempt: attempt, HostSegmentOrdinal: 1, Text: text}}
	}
	invalidForm := summary("delta", 1, "INVALID_FORM_CANARY")
	invalidForm.ProcessFrame.ReasoningForm = "full"
	invalidSource := summary("delta", 1, "INVALID_SOURCE_CANARY")
	invalidSource.ProcessFrame.Source = "assistant_draft"
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{
				invalidForm, invalidSource,
				summary("start", 1, ""), summary("delta", 1, "first summary"),
				{Kind: domainmodel.ChunkRetrying, RetryAttempt: 2, RetryMax: 2},
				summary("delta", 1, "LATE_OLD_ATTEMPT_CANARY"),
				summary("start", 2, ""), summary("delta", 2, "second summary"),
			},
			err: fakeProviderRetryError{retryable: true},
		},
		{
			callbackChunks: []domainmodel.Chunk{
				summary("start", 0, ""), summary("delta", 0, "replacement summary"), summary("end", 0, ""),
				{Kind: domainmodel.ChunkText, Text: "verified answer"},
			},
			resultChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "verified answer"}},
		},
	}}
	var frames []domainmodel.ProcessFrameV1
	public := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, MaxAttempts: 2,
		Callbacks: ProviderStreamCallbacks{
			OnProcessFrame: func(frame domainmodel.ProcessFrameV1) { frames = append(frames, frame) },
			OnTextDelta:    func(text string) error { public += text; return nil },
		},
	})
	if err != nil || len(provider.requests) != 2 || output.Text != "verified answer" || public != output.Text || output.Reasoning != "" || output.ReasoningSignature != "" {
		t.Fatalf("summary entered continuation or changed retry/publication: calls=%d err=%v", len(provider.requests), err)
	}
	var attempts []uint64
	var bodies []string
	reset := uint64(0)
	for _, frame := range frames {
		if frame.Kind == "reset" {
			reset = frame.Attempt
		}
		if frame.Kind == "reset" || frame.Kind == "terminal" {
			if frame.ReasoningForm != "" || frame.Text != "" {
				t.Fatal("lifecycle carried summary identity or text")
			}
			continue
		}
		if frame.Source != "provider_reasoning" || frame.ReasoningForm != "summary" || frame.NativeIndex != 0 || frame.NativeID != "" || frame.HostSegmentOrdinal != 1 || frame.Attempt != reset {
			t.Fatal("summary lost its immutable form or physical attempt")
		}
		if frame.Kind == "delta" {
			attempts = append(attempts, frame.Attempt)
			bodies = append(bodies, frame.Text)
		}
	}
	if !reflect.DeepEqual(attempts, []uint64{1, 2, 3}) || !reflect.DeepEqual(bodies, []string{"first summary", "second summary", "replacement summary"}) {
		t.Fatal("summary retries merged attempts or admitted rejected/late content")
	}
	for _, chunk := range output.Result.Chunks {
		if chunk.ProcessFrame != nil || chunk.Kind == domainmodel.ChunkReasoning {
			t.Fatal("summary sidecar entered retained result")
		}
	}
}

type lateProcessFrameProviderStub struct {
	cancel      context.CancelFunc
	callbackErr error
}

func (stub *lateProcessFrameProviderStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	stub.cancel()
	late := processFrameChunkForTest("delta", 1, 8, "PRIVATE_CANCELLED_PROCESS_FRAME")
	if request.OnChunk != nil {
		stub.callbackErr = request.OnChunk(late)
	}
	return domainmodel.Result{Chunks: []domainmodel.Chunk{late}, StreamCompleted: true}, nil
}

func TestStreamProviderSuppressesLateNativeProcessFramesAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := &lateProcessFrameProviderStub{cancel: cancel}
	nativeFrames := 0
	output, err := StreamProviderWithRetry(ctx, ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
			if frame.Kind == "start" || frame.Kind == "delta" || frame.Kind == "end" {
				nativeFrames++
			}
		}},
	})
	if !errors.Is(err, context.Canceled) || !errors.Is(provider.callbackErr, context.Canceled) || nativeFrames != 0 ||
		output.Text != "" || output.Reasoning != "" || len(output.Result.Chunks) != 0 {
		t.Fatal("cancelled producer bytes crossed the process display callback or result")
	}
}

func TestStreamProviderProcessFramesRequireNativeEvidenceAndBindZeroAttempt(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			const reasoning = "PRIVATE_FRAME_EVIDENCE_REASONING"
			chunks := []domainmodel.Chunk{{Kind: domainmodel.ChunkReasoning, Text: reasoning}}
			if native {
				chunks = []domainmodel.Chunk{
					processFrameChunkForTest("start", 0, 8, ""),
					processFrameChunkForTest("delta", 0, 8, reasoning),
					processFrameChunkForTest("end", 0, 8, ""),
				}
			}
			provider := &providerStreamStub{responses: []providerStreamResponse{{callbackChunks: chunks}}}
			var nativeFrames []domainmodel.ProcessFrameV1
			output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
				Provider: provider,
				Callbacks: ProviderStreamCallbacks{OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
					if frame.Kind == "start" || frame.Kind == "delta" || frame.Kind == "end" {
						nativeFrames = append(nativeFrames, frame)
					}
				}},
			})
			if err != nil || output.Reasoning != reasoning {
				t.Fatal("process evidence changed the original continuation")
			}
			if !native && len(nativeFrames) != 0 {
				t.Fatal("ordinary reasoning chunks fabricated native display evidence")
			}
			if native {
				if len(nativeFrames) != 3 {
					t.Fatalf("explicit native boundaries were lost: %#v", nativeFrames)
				}
				for _, frame := range nativeFrames {
					if frame.Attempt != 1 || frame.NativeIndex != 8 {
						t.Fatalf("zero local attempt was not bound to the current physical send: %#v", frame)
					}
				}
			}
		})
	}
}

func TestStreamProviderProcessSecretHintsUsePreparedRequestAuthority(t *testing.T) {
	const initialKey = "PRIVATE_INITIAL_PROVIDER_KEY"
	const preparedKey = "PRIVATE_PREPARED_PROVIDER_KEY"
	const forgedKey = "PRIVATE_FORGED_FRAME_SECRET_HINT"
	chunks := []domainmodel.Chunk{
		processFrameChunkForTest("reset", 1, -1, ""),
		processFrameChunkForTest("start", 1, 8, ""),
		processFrameChunkForTest("delta", 1, 8, "private continuation"),
		processFrameChunkForTest("end", 1, 8, ""),
		processFrameChunkForTest("terminal", 1, -1, ""),
	}
	for _, chunk := range chunks {
		chunk.ProcessFrame.ProjectionSecrets = []string{forgedKey}
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{callbackChunks: chunks}}}
	settlements := 0
	var kinds []string
	var hints [][]string
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Request:  domainmodel.Request{APIKey: initialKey},
		Callbacks: ProviderStreamCallbacks{
			PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
				request.APIKey = preparedKey
				return request, func(context.Context, string, string) error {
					settlements++
					return nil
				}, nil
			},
			OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
				kinds = append(kinds, frame.Kind)
				hints = append(hints, append([]string(nil), frame.ProjectionSecrets...))
			},
		},
	})
	if err != nil || len(provider.requests) != 1 || provider.requests[0].APIKey != preparedKey ||
		settlements != 1 || output.Reasoning != "private continuation" {
		t.Fatalf("prepared provider authority or continuation changed: calls=%d settlements=%d err=%v", len(provider.requests), settlements, err)
	}
	if !reflect.DeepEqual(kinds, []string{"reset", "start", "delta", "end", "terminal"}) {
		t.Fatalf("expected every display lifecycle callback: %#v", kinds)
	}
	for _, actual := range hints {
		if !reflect.DeepEqual(actual, []string{preparedKey}) {
			t.Fatal("process projection trusted an incoming hint or initial request key over prepared authority")
		}
	}
}

func TestStreamProviderOversizedProcessFrameClosesCaptureWithoutChangingContinuation(t *testing.T) {
	reasoning := strings.Repeat("x", 1024*1024+1)
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			processFrameChunkForTest("start", 1, 8, ""),
			processFrameChunkForTest("delta", 1, 8, reasoning),
			processFrameChunkForTest("end", 1, 8, ""),
			{Kind: domainmodel.ChunkText, Text: "completed public answer"},
		},
		resultChunks: []domainmodel.Chunk{
			{Kind: domainmodel.ChunkReasoning, Text: reasoning},
			{Kind: domainmodel.ChunkText, Text: "completed public answer"},
		},
	}}}
	var kinds []string
	capturedBytes := 0
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider,
		Callbacks: ProviderStreamCallbacks{OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
			kinds = append(kinds, frame.Kind)
			capturedBytes += len(frame.Text)
		}},
	})
	if err != nil || len(provider.requests) != 1 || output.Reasoning != reasoning || output.Text != "completed public answer" {
		t.Fatal("display capture size limit changed the original provider continuation or outcome")
	}
	if !reflect.DeepEqual(kinds, []string{"reset", "start", "terminal"}) || capturedBytes != 0 {
		t.Fatalf("oversized native block was truncated or marked complete: kinds=%#v capturedBytes=%d", kinds, capturedBytes)
	}
	var resultReasoning strings.Builder
	for _, chunk := range output.Result.Chunks {
		if chunk.Kind == domainmodel.ChunkReasoning {
			resultReasoning.WriteString(chunk.Text)
		}
	}
	if resultReasoning.String() != reasoning {
		t.Fatal("display capture bound truncated the original provider result")
	}
}

func TestStreamProviderStripsReturnedDisplaySidecarsBeforeResultRetention(t *testing.T) {
	frame := &domainmodel.ProcessFrameV1{
		Kind: "delta", Attempt: 1, NativeIndex: 8,
		Text: "private continuation", ProjectionSecrets: []string{"FORGED_RETURNED_SECRET"},
	}
	resultChunks := []domainmodel.Chunk{
		{Kind: domainmodel.ChunkReasoning, Text: "private continuation", ProcessFrame: frame},
		{Kind: domainmodel.ChunkText, Text: "public answer"},
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{
			processFrameChunkForTest("start", 1, 8, ""),
			processFrameChunkForTest("delta", 1, 8, "private continuation"),
			processFrameChunkForTest("end", 1, 8, ""),
			{Kind: domainmodel.ChunkText, Text: "public answer"},
		},
		resultChunks: resultChunks,
	}}}
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider})
	if err != nil || output.Text != "public answer" || output.Reasoning != "private continuation" {
		t.Fatal("private result cleanup changed provider continuation or completion")
	}
	if len(output.Result.Chunks) != 2 {
		t.Fatal("private result cleanup changed existing chunk count")
	}
	for _, chunk := range output.Result.Chunks {
		if chunk.ProcessFrame != nil {
			t.Fatal("display sidecar or projection secrets entered retained result")
		}
	}
	if resultChunks[0].ProcessFrame != frame || len(frame.ProjectionSecrets) != 1 || frame.ProjectionSecrets[0] != "FORGED_RETURNED_SECRET" {
		t.Fatal("result cleanup mutated a provider-owned sidecar")
	}
}

type frozenProviderDelayError struct {
	reads *int
	delay time.Duration
}

func (err frozenProviderDelayError) Error() string   { return "synthetic retry" }
func (err frozenProviderDelayError) Retryable() bool { return true }
func (err frozenProviderDelayError) RetryAfterDelay() time.Duration {
	*err.reads++
	if *err.reads > 1 {
		return time.Hour
	}
	return err.delay
}

func TestOuterRetryLifecycleFreezesActualTimerAndInvocation(t *testing.T) {
	reads := 0
	provider := &providerStreamStub{responses: []providerStreamResponse{{err: frozenProviderDelayError{&reads, 20 * time.Millisecond}}, {}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var retries []domainmodel.ProviderRetryV1
	settled := 0
	_, err := StreamProviderWithRetry(ctx, ProviderStreamInput{Provider: provider, ContinuationAuthority: ProviderContinuationAuthorityInput{Sequence: 9}, Callbacks: ProviderStreamCallbacks{
		PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
			return request, func(context.Context, string, string) error { settled++; return nil }, nil
		},
		OnProviderRetryLifecycle: func(retry domainmodel.ProviderRetryV1, max int, _ error) error {
			if settled != 1 || len(provider.requests) != 1 || max != 2 {
				t.Fatal("retry precedes failed settlement or true invocation")
			}
			retries = append(retries, retry)
			return nil
		},
	}})
	if err != nil || reads != 1 || len(provider.requests) != 2 || len(retries) != 2 {
		t.Fatalf("timer/invocation mismatch: err=%v delayReads=%d calls=%d retries=%#v", err, reads, len(provider.requests), retries)
	}
	a, b := retries[0], retries[1]
	if a.Phase != "scheduled" || b.Phase != "started" || a.Source != "provider_retry" || a.LogicalSequence != 9 || a.OuterAttempt != 2 || a.PhysicalAttempt != 0 || *a.DelayMs != 20 || *a.DeadlineUnixMs != a.At.Add(20*time.Millisecond).UnixMilli() || b.At.UnixMilli() < *a.DeadlineUnixMs {
		t.Fatalf("frozen timer metadata=%#v started=%#v", a, b)
	}
}

func TestOuterRetryLifecycleCancellationAndRejection(t *testing.T) {
	for _, name := range []string{"context_cancelled", "deadline_exceeded", "attempt_rejected", "scheduled_record_failure", "started_record_failure", "settlement_failure"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if name == "deadline_exceeded" {
				ctx, cancel = context.WithTimeout(context.Background(), 15*time.Millisecond)
				defer cancel()
			}
			delay := time.Duration(0)
			if name == "deadline_exceeded" {
				delay = time.Second
			}
			provider := &providerStreamStub{responses: []providerStreamResponse{{err: fakeProviderRetryError{retryable: true, delay: delay}}, {}}}
			var retries []domainmodel.ProviderRetryV1
			_, err := StreamProviderWithRetry(ctx, ProviderStreamInput{Provider: provider, ContinuationAuthority: ProviderContinuationAuthorityInput{Sequence: 1}, Callbacks: ProviderStreamCallbacks{
				PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
					return request, func(context.Context, string, string) error {
						if name == "settlement_failure" {
							return errors.New("settlement blocked")
						}
						return nil
					}, nil
				},
				BeforeProviderInvocation: func(attempt int) error {
					if attempt == 2 && name == "attempt_rejected" {
						return errors.New("authority rejected")
					}
					return nil
				},
				OnProviderRetryLifecycle: func(retry domainmodel.ProviderRetryV1, _ int, _ error) error {
					retries = append(retries, retry)
					if retry.Phase == "scheduled" && name == "context_cancelled" {
						cancel()
					}
					if (retry.Phase == "scheduled" && name == "scheduled_record_failure") || (retry.Phase == "started" && name == "started_record_failure") {
						return errors.New("event recorder failed")
					}
					return nil
				},
			}})
			if err == nil || len(provider.requests) != 1 {
				t.Fatalf("exit crossed invocation: err=%v calls=%d", err, len(provider.requests))
			}
			if strings.HasSuffix(name, "record_failure") {
				return
			}
			if name == "settlement_failure" {
				if len(retries) != 0 {
					t.Fatal("unsettled failure scheduled retry")
				}
				return
			}
			if len(retries) != 2 || retries[0].Phase != "scheduled" || retries[1].Phase != "cancelled" || retries[1].CancelReason != name {
				t.Fatalf("cancel lifecycle=%#v", retries)
			}
		})
	}
}

func TestRetryStartedAndCancelledDoNotResetNativeAttempt(t *testing.T) {
	seq := uint64(4)
	makeRetry := func(phase string) *domainmodel.ProviderRetryV1 {
		return &domainmodel.ProviderRetryV1{SchemaVersion: 1, Source: "transport_reconnect", Phase: phase, LogicalSequence: seq, OuterAttempt: 1, PhysicalAttempt: 2, At: time.Now().UTC()}
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{callbackChunks: []domainmodel.Chunk{
		{Kind: domainmodel.ChunkRetrying, RetryAttempt: 2, RetryMax: 2, Retry: makeRetry("scheduled")},
		{Kind: domainmodel.ChunkRetrying, RetryAttempt: 2, RetryMax: 2, Retry: makeRetry("started")},
		{Kind: domainmodel.ChunkReasoning, Text: "new private draft"},
		{Kind: domainmodel.ChunkRetrying, RetryAttempt: 2, RetryMax: 2, Retry: makeRetry("cancelled")},
	}}}}
	resets := 0
	legacy := 0
	lifecycle := 0
	out, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider, Callbacks: ProviderStreamCallbacks{OnProcessFrame: func(frame domainmodel.ProcessFrameV1) {
		if frame.Kind == "reset" {
			resets++
		}
	}, OnProviderRetrying: func(int, int, error) error { legacy++; return nil }, OnProviderRetryLifecycle: func(domainmodel.ProviderRetryV1, int, error) error { lifecycle++; return nil }}})
	if err != nil || resets != 1 || legacy != 1 || lifecycle != 3 || out.Reasoning != "new private draft" {
		t.Fatalf("phase reset/discard leaked: reset=%d legacy=%d lifecycle=%d reasoning=%q err=%v", resets, legacy, lifecycle, out.Reasoning, err)
	}
}

func TestRetryCancelReasonPreservesCurrentHostAuthority(t *testing.T) {
	for _, tt := range []struct {
		ctxErr, cause error
		want          string
	}{
		{context.Canceled, context.DeadlineExceeded, "context_cancelled"},
		{context.DeadlineExceeded, context.Canceled, "deadline_exceeded"},
		{nil, context.Canceled, "context_cancelled"},
		{nil, context.DeadlineExceeded, "deadline_exceeded"},
		{nil, errors.New("admission rejected"), "attempt_rejected"},
	} {
		if got := domainmodel.ProviderRetryCancelReasonV1(tt.ctxErr, tt.cause); got != tt.want {
			t.Fatalf("host=%v cause=%v got=%s want=%s", tt.ctxErr, tt.cause, got, tt.want)
		}
	}
}

func TestRetryLegacyCallbackRejectionClosesAcceptedSchedule(t *testing.T) {
	for _, source := range []string{"provider_retry", "transport_reconnect"} {
		t.Run(source, func(t *testing.T) {
			response := providerStreamResponse{err: fakeProviderRetryError{retryable: true}}
			if source == "transport_reconnect" {
				response = providerStreamResponse{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkRetrying, RetryAttempt: 2, RetryMax: 2, Retry: &domainmodel.ProviderRetryV1{SchemaVersion: 1, Source: source, Phase: "scheduled", LogicalSequence: 1, OuterAttempt: 1, PhysicalAttempt: 2, At: time.Now().UTC()}}}}
			}
			provider := &providerStreamStub{responses: []providerStreamResponse{response, {}}}
			var retries []domainmodel.ProviderRetryV1
			_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider, ContinuationAuthority: ProviderContinuationAuthorityInput{Sequence: 1}, Callbacks: ProviderStreamCallbacks{
				OnProviderRetryLifecycle: func(retry domainmodel.ProviderRetryV1, _ int, _ error) error {
					retries = append(retries, retry)
					return nil
				},
				OnProviderRetrying: func(int, int, error) error { return errors.New("legacy admission rejected") },
			}})
			if err == nil || len(provider.requests) != 1 || len(retries) != 2 || retries[0].Phase != "scheduled" || retries[1].Phase != "cancelled" || retries[1].CancelReason != "attempt_rejected" {
				t.Fatalf("legacy callback left schedule unresolved: calls=%d retries=%#v err=%v", len(provider.requests), retries, err)
			}
		})
	}
}

func TestProviderProcessSidecarsShareHostIssuerAndStayPrivateV1(t *testing.T) {
	const rawID = "raw_preparation_call"
	const reasoning = "PRIVATE_REASONING_SENTINEL"
	const partial = `{"path":"draft.txt","content":"A`
	const finalArgs = `{"path":"draft.txt","content":"A😀"}`
	const publicText = "verified public answer"
	reasoningChunk := func(kind, text string) domainmodel.Chunk {
		chunk := domainmodel.Chunk{Kind: domainmodel.ChunkReasoning, ProcessFrame: &domainmodel.ProcessFrameV1{Source: "provider_reasoning", Kind: kind, HostSegmentOrdinal: 1, Text: text}}
		if kind == "delta" {
			chunk.Text = text
		}
		return chunk
	}
	preparationChunk := func(kind, arguments string) domainmodel.Chunk {
		return domainmodel.Chunk{Kind: domainmodel.ChunkReasoning, ProcessFrame: &domainmodel.ProcessFrameV1{Source: "tool_preparation", Kind: kind, ToolPreparation: &domainstreamframe.ToolPreparationV1{Index: 0, CallID: rawID, ToolName: "write", Arguments: arguments}}}
	}
	origin := preparationChunk("start", "")
	origin.ProcessFrame.ToolPreparation.ToolName = ""
	// An early optional wire hint is not the later actual call identity. It
	// must not consume a second issuer identity or bind a visible preparation.
	origin.ProcessFrame.ToolPreparation.CallID = "raw_before_named_id"
	callbackChunks := []domainmodel.Chunk{
		origin,
		reasoningChunk("start", ""), reasoningChunk("delta", reasoning), reasoningChunk("end", ""),
		preparationChunk("delta", partial),
		{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: rawID, Name: "write"}},
		preparationChunk("end", finalArgs),
		{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: rawID, Name: "write", Arguments: []byte(finalArgs)}},
		{Kind: domainmodel.ChunkText, Text: publicText},
	}
	resultChunks := []domainmodel.Chunk{reasoningChunk("delta", reasoning), callbackChunks[7], {Kind: domainmodel.ChunkText, Text: publicText}}
	provider := &providerStreamStub{responses: []providerStreamResponse{{callbackChunks: callbackChunks, resultChunks: resultChunks}}}
	var frames []domainmodel.ProcessFrameV1
	var publicChunks []domainmodel.Chunk
	startID := ""
	output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
		Provider: provider, MaxAttempts: 1,
		toolCallIDRandom: bytes.NewReader(bytes.Repeat([]byte{0x41}, domainmodel.HostToolCallIDEntropyBytesV1)),
		Callbacks: ProviderStreamCallbacks{
			OnProcessFrame:  func(frame domainmodel.ProcessFrameV1) { frames = append(frames, frame) },
			OnToolCallStart: func(call domainmodel.ToolCall) error { startID = call.ID; return nil },
			OnTextChunk:     func(chunk domainmodel.Chunk) error { publicChunks = append(publicChunks, chunk); return nil },
		},
	})
	if err != nil || len(provider.requests) != 1 || len(output.Result.Chunks) != 3 {
		t.Fatalf("sidecar changed provider completion: calls=%d err=%v", len(provider.requests), err)
	}
	finalID := output.Result.Chunks[1].ToolCall.ID
	if !domainmodel.IsHostToolCallIDV1(startID) || startID != finalID || startID == rawID {
		t.Fatalf("issuer diverged: start=%q final=%q", startID, finalID)
	}
	var lifecycle, preparationArguments []string
	reasoningText := ""
	for _, frame := range frames {
		if frame.Attempt != 1 {
			t.Fatalf("frame escaped the actual attempt: %#v", frame)
		}
		lifecycle = append(lifecycle, frame.Source+"/"+frame.Kind)
		switch frame.Source {
		case "tool_preparation":
			prep := frame.ToolPreparation
			if frame.Kind == "start" {
				if prep == nil || prep.CallID != "" || prep.ToolName != "" || prep.Arguments != "" || prep.Index != 0 || frame.Text != "" {
					t.Fatal("control origin minted an identity or carried bytes")
				}
				continue
			}
			if prep == nil || prep.CallID != startID || prep.Index != 0 || prep.ToolName != "write" || frame.Text != "" || frame.NativeID != "" || frame.NativeIndex != 0 {
				t.Fatalf("preparation lost its host coordinate: %#v", frame)
			}
			preparationArguments = append(preparationArguments, prep.Arguments)
		case "provider_reasoning":
			if frame.HostSegmentOrdinal != 1 || frame.NativeID != "" || frame.NativeIndex != 0 {
				t.Fatalf("reasoning fabricated native identity: %#v", frame)
			}
			if frame.Kind == "delta" {
				reasoningText += frame.Text
			} else if frame.Text != "" {
				t.Fatal("reasoning boundary carried body text")
			}
		}
	}
	wantLifecycle := []string{"/reset", "tool_preparation/start", "provider_reasoning/start", "provider_reasoning/delta", "provider_reasoning/end", "tool_preparation/delta", "tool_preparation/end", "/terminal"}
	if !reflect.DeepEqual(lifecycle, wantLifecycle) || !reflect.DeepEqual(preparationArguments, []string{partial, finalArgs}) || reasoningText != reasoning {
		t.Fatalf("private sidecar lifecycle changed: %v", lifecycle)
	}
	if output.Reasoning != reasoning || output.Text != publicText || !reflect.DeepEqual(publicChunks, []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: publicText}}) {
		t.Fatal("private process content crossed the public publication seam")
	}
	for _, chunk := range output.Result.Chunks {
		if chunk.ProcessFrame != nil {
			t.Fatal("process sidecar entered retained provider continuation")
		}
	}
	if callbackChunks[4].ProcessFrame.ToolPreparation.CallID != rawID || callbackChunks[0].ProcessFrame.ToolPreparation.CallID != "raw_before_named_id" || resultChunks[0].ProcessFrame == nil {
		t.Fatal("host normalization mutated provider-owned sidecars")
	}
}

func TestNativeFirstContentPresenceSurvivesNormalizationAndCannotUseDisplayFallback(t *testing.T) {
	for _, present := range []bool{false, true} {
		provider := &providerStreamStub{responses: []providerStreamResponse{{nativePresence: true, nativeContent: present, callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "display"}}}}}
		output, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{Provider: provider, Request: domainmodel.Request{}})
		if err != nil || !output.Result.HostTiming.FirstContentPresenceKnown || output.Result.HasFirstTokenLatency != present || output.Result.HostTiming.FirstContentAt.IsZero() == present {
			t.Fatalf("native presence lost or inferred from display: present=%v output=%#v err=%v", present, output.Result, err)
		}
	}
}

type providerDiagnosticOpaqueError struct{}

func (providerDiagnosticOpaqueError) Error() string {
	panic("diagnostic must not read error text")
}

func TestProviderFailureDiagnosticUsesOnlyTypedClosedLabels(t *testing.T) {
	const secret = "synthetic-secret-diagnostic-sentinel"
	privacyErrors := []struct {
		err   error
		cause string
	}{
		{privacyprojectionapp.ErrProviderPrivacyAuthorityUnavailable, "provider_privacy_authority_unavailable"},
		{privacyprojectionapp.ErrUninspectableProviderPart, "uninspectable_provider_part"},
		{privacyprojectionapp.ErrProviderSchemaContainsPII, "provider_schema_contains_pii"},
		{privacyprojectionapp.ErrOrdinaryProviderContainsCaseEffect, "ordinary_provider_contains_case_effect"},
		{privacyprojectionapp.ErrProviderLogicalEffectInvalid, "provider_logical_effect_invalid"},
	}
	for _, privacy := range privacyErrors {
		for _, err := range []error{privacy.err, ProviderStreamCallbackError{Err: privacy.err}, errors.Join(providerDiagnosticOpaqueError{}, privacy.err)} {
			stage, cause := ProviderFailureDiagnosticV1(err)
			if stage != "privacy_projection" || cause != privacy.cause {
				t.Fatalf("privacy classification = %q/%q", stage, cause)
			}
		}
	}
	for _, code := range []string{
		"turn_security_authority_unavailable", "turn_security_identity_mismatch",
		"turn_security_workspace_mismatch", "turn_security_case_binding_mismatch",
		"turn_security_dataset_snapshot_mismatch", "turn_security_risk_policy_mismatch",
		"turn_security_context_invalid", "source_probe_unavailable", "tool_source_unavailable",
	} {
		failure := TurnFailureError{Code: code, Message: secret, Details: map[string]any{"secret": secret}}
		for _, err := range []error{failure, &failure, errors.Join(providerDiagnosticOpaqueError{}, ProviderStreamCallbackError{Err: failure})} {
			stage, cause := ProviderFailureDiagnosticV1(err)
			if stage != "unknown" || cause != code {
				t.Fatalf("typed classification = %q/%q", stage, cause)
			}
		}
	}
	for _, err := range []error{
		nil, providerDiagnosticOpaqueError{}, errors.New("source_probe_unavailable " + secret),
		TurnFailureError{Code: "source_probe_unavailable "},
		TurnFailureError{Code: secret, Details: map[string]any{"code": "source_probe_unavailable"}},
		ProviderStreamCallbackError{Err: errors.New(secret)},
	} {
		stage, cause := ProviderFailureDiagnosticV1(err)
		if stage != "unknown" || cause != "unknown" {
			t.Fatalf("untyped or unlisted failure classification = %q/%q", stage, cause)
		}
	}
	for _, test := range []struct {
		err          error
		stage, cause string
	}{
		{errors.Join(context.Canceled, providerDiagnosticOpaqueError{}), "unknown", "context_canceled"},
		{ProviderStreamCallbackError{Err: context.DeadlineExceeded}, "unknown", "context_deadline_exceeded"},
		{errors.Join(context.Canceled, providerAttemptAcquireError{Err: providerDiagnosticOpaqueError{}}), "acquire", "unknown"},
		{ProviderStreamCallbackError{Err: errors.Join(providerAttemptPrepareError{Err: context.DeadlineExceeded}, privacyprojectionapp.ErrProviderSchemaContainsPII)}, "prepare", "context_deadline_exceeded"},
	} {
		stage, cause := ProviderFailureDiagnosticV1(test.err)
		if stage != test.stage || cause != test.cause {
			t.Fatalf("wrapped/joined marker classification = %q/%q, want %q/%q", stage, cause, test.stage, test.cause)
		}
	}
}

func TestProviderFailureDiagnosticPresendBoundariesMakeZeroProviderCalls(t *testing.T) {
	for _, stage := range []string{"acquire", "prepare", "privacy_projection"} {
		t.Run(stage, func(t *testing.T) {
			provider := &providerStreamStub{}
			active := false
			settlements := 0
			cause := error(context.Canceled)
			wantCause := "context_canceled"
			if stage == "prepare" {
				cause = TurnFailureError{Code: "source_probe_unavailable", Message: "synthetic-secret-diagnostic-sentinel"}
				wantCause = "source_probe_unavailable"
			} else if stage == "privacy_projection" {
				cause = privacyprojectionapp.ErrUninspectableProviderPart
				wantCause = "uninspectable_provider_part"
			}
			_, err := StreamProviderWithRetry(context.Background(), ProviderStreamInput{
				Provider: provider, OrdinaryEffect: true,
				SecurityContext: newLoopGeneralContextV2(t, "thread-diagnostic", "turn-diagnostic", t.TempDir()),
				Request:         domainmodel.Request{Messages: []domainmodel.Message{{Role: "user", Content: "synthetic prompt"}}},
				Callbacks: ProviderStreamCallbacks{
					AcquireProviderAttempt: func(ctx context.Context, _ int) (context.Context, func(), error) {
						if stage == "acquire" {
							return nil, nil, cause
						}
						active = true
						return ctx, func() { active = false }, nil
					},
					PrepareProviderAttempt: func(_ context.Context, _ int, request domainmodel.Request) (domainmodel.Request, ProviderAttemptSettlement, error) {
						if stage == "prepare" {
							return domainmodel.Request{}, nil, cause
						}
						request.Messages[0].Parts = []domainmodel.MessagePart{{Type: "image", Data: "synthetic-secret-diagnostic-sentinel"}}
						return request, func(_ context.Context, status, reason string) error {
							settlements++
							if status != "failed" || reason != "privacy_projection_failed" {
								t.Fatal("projection settlement changed")
							}
							return errors.New("synthetic-secret-settlement-sentinel")
						}, nil
					},
				},
			})
			gotStage, gotCause := ProviderFailureDiagnosticV1(err)
			if gotStage != stage || gotCause != wantCause || len(provider.requests) != 0 || active {
				t.Fatalf("presend boundary = %q/%q, calls=%d active=%v", gotStage, gotCause, len(provider.requests), active)
			}
			if stage != "prepare" && !errors.Is(err, cause) {
				t.Fatal("original sentinel no longer unwraps")
			}
			if stage == "privacy_projection" && settlements != 1 || stage != "privacy_projection" && settlements != 0 {
				t.Fatalf("settlements = %d at %s", settlements, stage)
			}
		})
	}
}
