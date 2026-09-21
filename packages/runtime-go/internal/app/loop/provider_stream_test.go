package loop

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	appmodel "analytix.local/runtime-go/internal/app/model"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainreasoningmarkup "analytix.local/runtime-go/internal/domain/reasoningmarkup"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
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
	for _, chunk := range response.callbackChunks {
		if request.OnChunk != nil {
			if err := request.OnChunk(chunk); err != nil {
				return domainmodel.Result{}, err
			}
		}
	}
	return domainmodel.Result{Chunks: response.resultChunks, CacheObservations: response.cacheTelemetry}, response.err
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
	if output.StreamedDeltas || !output.Result.HasFirstTokenLatency || output.Result.Chunks[0].Text != "final" {
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
