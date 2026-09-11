package loop

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincache "analytix.local/runtime-go/internal/domain/cachetelemetry"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type runtimeRunnerTerminalClaimContextKey struct{}

type runtimeRunnerReportTerminalCapability struct {
	context domainsecurity.TurnSecurityContext
	grant   domainsecurity.ExecutionGrant
	callID  string
	digest  string
}

func (capability runtimeRunnerReportTerminalCapability) VerifyReportTerminal(
	securityContext domainsecurity.TurnSecurityContext,
	grant domainsecurity.ExecutionGrant,
	callID string,
) (string, bool) {
	return capability.digest,
		securityContext == capability.context && grant == capability.grant && callID == capability.callID
}

type runtimeRunnerTerminalEffectDriver struct {
	*toolStepDriverStub
	claimHeld          *bool
	executedUnderClaim bool
	reportTerminal     bool
}

func (driver *runtimeRunnerTerminalEffectDriver) ExecuteAndSettle(
	ctx context.Context,
	pending appmodel.PendingToolCall,
	override any,
	transform ToolOutputTransform,
) (SettledToolExecution, error) {
	if driver.claimHeld != nil && *driver.claimHeld && ctx.Value(runtimeRunnerTerminalClaimContextKey{}) == true {
		driver.executedUnderClaim = true
	}
	settled, err := driver.toolStepDriverStub.ExecuteAndSettle(ctx, pending, override, transform)
	if err == nil && driver.reportTerminal {
		settled.ReportTerminal = runtimeRunnerReportTerminalCapability{
			context: pending.SecurityContext,
			grant:   pending.ExecutionGrant,
			callID:  pending.Call.ID,
			digest:  domainsecurity.SHA256Hex([]byte("runtime-runner-report-terminal")),
		}
	}
	return settled, err
}

type pipelineStageProviderStub struct {
	afterPreSend  func() error
	transported   bool
	omitAll       bool
	omitAllText   bool
	notSentFirst  bool
	omitPost      bool
	returnErr     error
	dispatchState domaincache.ProviderDispatchStateV1
	cancel        func()
	calls         int
}

type providerDispatchStateTestErrorV1 struct {
	err   error
	state domaincache.ProviderDispatchStateV1
}

func (err providerDispatchStateTestErrorV1) Error() string { return err.err.Error() }
func (err providerDispatchStateTestErrorV1) Unwrap() error { return err.err }
func (err providerDispatchStateTestErrorV1) ProviderDispatchStateV1() domaincache.ProviderDispatchStateV1 {
	return err.state
}

func (*pipelineStageProviderStub) RequiresDurablePipelineStagesV1() {}

func (stub *pipelineStageProviderStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	stub.calls++
	if request.OnPipelineStage == nil {
		return domainmodel.Result{}, errors.New("provider pipeline callback is unavailable")
	}
	if stub.notSentFirst && stub.calls == 1 {
		return domainmodel.Result{}, providerDispatchStateTestErrorV1{
			err: fakeProviderRetryError{retryable: true}, state: domaincache.ProviderDispatchStateNotSent,
		}
	}
	base := time.Unix(100, 0).UTC()
	if stub.omitAll {
		if stub.cancel != nil {
			stub.cancel()
		}
		if stub.omitAllText && request.OnChunk != nil {
			if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "unbound provider response"}); err != nil {
				return domainmodel.Result{}, err
			}
		}
		err := stub.returnErr
		if err != nil && stub.dispatchState != "" {
			err = providerDispatchStateTestErrorV1{err: err, state: stub.dispatchState}
		}
		return domainmodel.Result{StreamCompleted: true, Usage: domainmodel.Usage{FinishReason: "stop"}}, err
	}
	if err := request.OnPipelineStage(domainmodel.PipelineStage{Stage: "pre_send", At: base}); err != nil {
		return domainmodel.Result{}, err
	}
	if stub.afterPreSend != nil {
		if err := stub.afterPreSend(); err != nil {
			return domainmodel.Result{}, err
		}
	}
	stub.transported = true
	if stub.omitPost {
		return domainmodel.Result{StreamCompleted: true, Usage: domainmodel.Usage{FinishReason: "stop"}}, stub.returnErr
	}
	if err := request.OnPipelineStage(domainmodel.PipelineStage{Stage: "post_send", At: base.Add(time.Millisecond)}); err != nil {
		return domainmodel.Result{}, err
	}
	if request.OnChunk != nil {
		if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "batched provider response"}); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{
		StreamCompleted: true,
		Usage:           domainmodel.Usage{FinishReason: "stop"},
	}, nil
}

type providerWithoutDurablePipelineContractStub struct {
	calls int
}

func (stub *providerWithoutDurablePipelineContractStub) Stream(_ context.Context, _ domainmodel.Request) (domainmodel.Result, error) {
	stub.calls++
	return domainmodel.Result{StreamCompleted: true, Usage: domainmodel.Usage{FinishReason: "stop"}}, nil
}

type retryingPipelineStageProviderStub struct {
	calls      int
	omitSecond bool
}

func (*retryingPipelineStageProviderStub) RequiresDurablePipelineStagesV1() {}

func (stub *retryingPipelineStageProviderStub) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	stub.calls++
	if stub.omitSecond && stub.calls == 2 {
		if request.OnChunk != nil {
			if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "unbound retry response"}); err != nil {
				return domainmodel.Result{}, err
			}
		}
		return domainmodel.Result{StreamCompleted: true, Usage: domainmodel.Usage{FinishReason: "stop"}}, nil
	}
	base := time.Unix(100+int64(stub.calls), 0).UTC()
	if err := request.OnPipelineStage(domainmodel.PipelineStage{Stage: "pre_send", At: base}); err != nil {
		return domainmodel.Result{}, err
	}
	if err := request.OnPipelineStage(domainmodel.PipelineStage{Stage: "post_send", At: base.Add(time.Millisecond)}); err != nil {
		return domainmodel.Result{}, err
	}
	if stub.calls == 1 {
		return domainmodel.Result{}, fakeProviderRetryError{retryable: true}
	}
	if request.OnChunk != nil {
		if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "retry completed"}); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{
		StreamCompleted: true,
		Usage:           domainmodel.Usage{FinishReason: "stop"},
	}, nil
}

func TestHostGeneralOnlyTurnEntersProviderLoopWithoutExternalWitness(t *testing.T) {
	workspace := t.TempDir()
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-provider", TurnID: "turn-general-provider", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary general response"}},
	}}}
	events := &runtimeLoopEventRecorderStub{}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "summarize the requested procedure", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages:        []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "summarize the requested procedure"}},
	}, runtimeRunnerTestDependencies(provider, events))
	if err != nil || result.AssistantText != "ordinary general response" || len(provider.requests) != 1 {
		t.Fatalf("host general-only turn did not enter exactly one provider attempt: calls=%d result=%#v err=%v", len(provider.requests), result, err)
	}
}

func TestRuntimeRunnerClassifiesPostProviderCandidateAuthorityFailure(t *testing.T) {
	workspace := t.TempDir()
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-host-candidate-failure", TurnID: "turn-host-candidate-failure", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "provider transport completed"}},
	}}}
	events := &runtimeLoopEventRecorderStub{}
	internalCause := errors.New("PRIVATE_INTERNAL_CANDIDATE_AUTHORITY_SENTINEL")
	deps := runtimeRunnerTestDependencies(provider, events)
	deps.AcquireCandidateTerminal = func(context.Context, RuntimeProviderStep) (context.Context, func(), error) {
		return nil, nil, internalCause
	}
	_, err = RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "finish the ordinary task", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages:        []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish the ordinary task"}},
	}, deps)
	if err == nil || !errors.Is(err, internalCause) {
		t.Fatalf("post-provider candidate authority failure was not retained internally: %v", err)
	}
	if strings.Contains(err.Error(), "PRIVATE_INTERNAL_CANDIDATE_AUTHORITY_SENTINEL") {
		t.Fatalf("post-provider candidate authority failure leaked its raw cause: %v", err)
	}
	public := PublicFailureForError(err)
	if public.Code() != domainfailure.CodeHostCandidateAuthorityFailed || public.Details() != nil {
		t.Fatalf("post-provider candidate authority failure projection = %#v", public)
	}
	for _, event := range events.events {
		if event["kind"] == "pipeline_stage" && event["stage"] == "provider_error" {
			t.Fatalf("host candidate authority failure was misreported as provider_error: %#v", event)
		}
	}
}

func TestRuntimeRunnerHoldsTerminalClaimAcrossFinalSteeringPromotion(t *testing.T) {
	workspace := t.TempDir()
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-steer-terminal", TurnID: "turn-steer-terminal", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "first draft"}}},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "steered final"}}},
	}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	promotions := 0
	claimHeld := false
	releases := 0
	promotedInsideClaim := false
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		if promotions == 2 {
			promotedInsideClaim = claimHeld
			return ordinaryRuntimeSteeringBatch("Mid-turn user follow-up for the current task:\nUse the smaller CSV file instead."), nil
		}
		return RuntimeSteeringBatch{}, nil
	}
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		if claimHeld {
			return nil, nil, errors.New("candidate terminal claim overlapped")
		}
		claimHeld = true
		return ctx, func() {
			if claimHeld {
				claimHeld = false
				releases++
			}
		}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Start the answer.", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages:        []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Start the answer."}},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	var secondMessages []domainmodel.Message
	if len(provider.requests) == 2 {
		secondMessages = provider.requests[1].Messages
	}
	if !promotedInsideClaim || len(provider.requests) != 2 || len(secondMessages) < 2 ||
		secondMessages[len(secondMessages)-2].Role != "assistant" || secondMessages[len(secondMessages)-2].Content != "first draft" ||
		!strings.Contains(secondMessages[len(secondMessages)-1].Content, "Use the smaller CSV file instead.") {
		t.Fatalf("steer was not promoted inside the terminal claim: inside=%v calls=%d second=%#v", promotedInsideClaim, len(provider.requests), provider.requests)
	}
	if releases != 1 || !claimHeld || result.CandidateTerminalContext == nil || result.ReleaseCandidateTerminal == nil || result.AssistantText != "steered final" {
		t.Fatalf("terminal handoff mismatch: releases=%d held=%v result=%#v", releases, claimHeld, result)
	}
	result.ReleaseCandidateTerminal()
	if releases != 2 || claimHeld {
		t.Fatalf("terminal handoff did not release exactly once: releases=%d held=%v", releases, claimHeld)
	}
}

func TestRuntimeRunnerPlanClarificationUsesFinalSteeringBoundary(t *testing.T) {
	workspace := t.TempDir()
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-plan-steer", TurnID: "turn-plan-steer", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "Which implementation path do you prefer?"}}},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "Which safe path should I use?"}}},
	}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	promotions := 0
	claimHeld := false
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		if promotions == 2 {
			if !claimHeld {
				return RuntimeSteeringBatch{}, errors.New("plan clarification promoted steering outside terminal claim")
			}
			return ordinaryRuntimeSteeringBatch("Use the safe path."), nil
		}
		return RuntimeSteeringBatch{}, nil
	}
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		if claimHeld {
			return nil, nil, errors.New("candidate terminal claim overlapped")
		}
		claimHeld = true
		return ctx, func() { claimHeld = false }, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Create a plan.", ProviderID: "provider", Model: "model", Mode: "plan",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", NormalizedSandboxMode: "workspace-write",
		EffectiveMaxModelSteps: 1, SecurityContext: securityContext,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Create a plan."}},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || len(provider.requests[1].Messages) < 2 ||
		provider.requests[1].Messages[len(provider.requests[1].Messages)-2].Content != "Which implementation path do you prefer?" ||
		provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content != "Use the safe path." ||
		result.AssistantText != "Which safe path should I use?" || result.CandidateTerminalContext == nil || result.ReleaseCandidateTerminal == nil || !claimHeld {
		t.Fatalf("plan clarification bypassed final steering boundary: requests=%#v result=%#v held=%v", provider.requests, result, claimHeld)
	}
	result.ReleaseCandidateTerminal()
	if claimHeld {
		t.Fatal("plan clarification terminal handoff was not released")
	}
}

func TestRuntimeRunnerProviderContinuationBoundaryHonorsEarlierSteer(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-boundary-steer", "turn-boundary-steer", workspace, "case-boundary-steer")
	toolCallID := "provider-call-boundary-steer"
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{{
				Kind:     domainmodel.ChunkToolCallStart,
				ToolCall: domainmodel.ToolCall{ID: toolCallID, Name: "task"},
			}},
			resultChunks: []domainmodel.Chunk{{
				Kind: domainmodel.ChunkToolCall,
				ToolCall: domainmodel.ToolCall{
					ID: toolCallID, Name: "task", Arguments: []byte(`{}`),
				},
			}},
		},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "steered boundary final"}}},
	}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = &toolStepDriverStub{}
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: zeroArgumentToolSchemas("task")}
	}
	promotions := 0
	claimHeld := false
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		if promotions == 2 {
			if !claimHeld {
				return RuntimeSteeringBatch{}, errors.New("provider continuation boundary promoted steer outside terminal claim")
			}
			return ordinaryRuntimeSteeringBatch("Continue without the protected child result."), nil
		}
		return RuntimeSteeringBatch{}, nil
	}
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		if claimHeld {
			return nil, nil, errors.New("candidate terminal claim overlapped")
		}
		claimHeld = true
		return ctx, func() { claimHeld = false }, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Run the bounded child.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages:        []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Run the bounded child."}},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || result.AssistantText != "steered boundary final" ||
		result.CandidateTerminalContext == nil || result.ReleaseCandidateTerminal == nil || !claimHeld {
		t.Fatalf("provider continuation boundary lost steer: requests=%#v result=%#v held=%v", provider.requests, result, claimHeld)
	}
	for _, message := range provider.requests[1].Messages {
		if message.Role == "tool" || len(message.ToolCalls) > 0 {
			t.Fatalf("protected tool pair crossed steered provider boundary: %#v", provider.requests[1].Messages)
		}
	}
	if messages := provider.requests[1].Messages; len(messages) < 2 ||
		messages[len(messages)-2].Content != childCompletionReceiptBoundary ||
		messages[len(messages)-1].Content != "Continue without the protected child result." {
		t.Fatalf("safe boundary and steer were not preserved: %#v", messages)
	}
	result.ReleaseCandidateTerminal()
	if claimHeld {
		t.Fatal("provider continuation boundary claim was not released")
	}
}

func TestRuntimeRunnerRequiredFinalSteerPreemptsFirstSubmissionBeforeEffect(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-required-final-steer", "turn-required-final-steer", workspace)
	toolName := toolcatalogapp.ForegroundSubmitToolName
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("stale-submit", toolName, `{"result":"stale result"}`),
		runtimeRunnerTerminalToolResponse("current-submit", toolName, `{"result":"current result"}`),
	}}
	claimHeld := false
	releases := 0
	promotions := 0
	driver := &runtimeRunnerTerminalEffectDriver{
		toolStepDriverStub: &toolStepDriverStub{readOnly: map[string]bool{toolName: true}},
		claimHeld:          &claimHeld,
	}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{toolcatalogapp.ForegroundSubmitToolSchema()}}
	}
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		if promotions == 2 {
			if !claimHeld {
				return RuntimeSteeringBatch{}, errors.New("required-final steer was promoted outside the terminal claim")
			}
			return ordinaryRuntimeSteeringBatch("Submit the corrected bounded result."), nil
		}
		return RuntimeSteeringBatch{}, nil
	}
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		if claimHeld {
			return nil, nil, errors.New("candidate terminal claim overlapped")
		}
		claimHeld = true
		return context.WithValue(ctx, runtimeRunnerTerminalClaimContextKey{}, true), func() {
			if claimHeld {
				claimHeld = false
				releases++
			}
		}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Return one bounded result.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "never", SandboxMode: "read-only", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext, RequiredFinalToolName: toolName, OutputTokenBudget: 128,
		ToolScope: []string{toolName},
		Messages:  []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Return one bounded result."}},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || len(driver.executed) != 1 || len(driver.ready) != 1 ||
		!driver.executedUnderClaim || result.AssistantText != "Foreground child result submitted." ||
		result.CandidateTerminalContext == nil || result.ReleaseCandidateTerminal == nil || !claimHeld || releases != 1 {
		t.Fatalf("required-final terminal effect was replayed or escaped its claim: requests=%d executed=%#v ready=%#v underClaim=%v releases=%d held=%v result=%#v",
			len(provider.requests), driver.executed, driver.ready, driver.executedUnderClaim, releases, claimHeld, result)
	}
	for _, message := range provider.requests[1].Messages {
		if message.Role == "tool" || len(message.ToolCalls) > 0 {
			t.Fatalf("superseded required-final tool pair reached the next provider request: %#v", provider.requests[1].Messages)
		}
	}
	if messages := provider.requests[1].Messages; len(messages) < 2 ||
		messages[len(messages)-2].Content != terminalToolSupersededBoundary ||
		messages[len(messages)-1].Content != "Submit the corrected bounded result." {
		t.Fatalf("required-final steer boundary was not preserved: %#v", messages)
	}
	result.ReleaseCandidateTerminal()
	if claimHeld || releases != 2 {
		t.Fatalf("required-final terminal claim was not released exactly once per acquisition: held=%v releases=%d", claimHeld, releases)
	}
}

func TestRuntimeRunnerReportSteerPreemptsDeliveryBeforeEffect(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-report-steer", "turn-report-steer", workspace, "case-report-steer")
	toolName := toolcatalogapp.ReportDeliveryToolName
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("stale-report", toolName, `{}`),
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "Report delivery was not performed."}}},
	}}
	claimHeld := false
	promotions := 0
	driver := &runtimeRunnerTerminalEffectDriver{
		toolStepDriverStub: &toolStepDriverStub{readOnly: map[string]bool{toolName: false}},
		claimHeld:          &claimHeld,
		reportTerminal:     true,
	}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = driver
	catalogSteps := []RuntimeProviderStep{}
	preparedSteps := []RuntimeProviderStep{}
	attemptSteps := []RuntimeProviderStep{}
	deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
		preparedSteps = append(preparedSteps, step)
		return step, nil
	}
	deps.ResolveToolCatalog = func(_ bool, step RuntimeProviderStep) RuntimeRunnerToolCatalog {
		if len(preparedSteps) != len(catalogSteps)+1 || preparedSteps[len(preparedSteps)-1] != step {
			t.Fatalf("provider step was not prepared before catalog resolution: prepared=%#v catalog=%#v step=%#v", preparedSteps, catalogSteps, step)
		}
		catalogSteps = append(catalogSteps, step)
		catalog := RuntimeRunnerToolCatalog{PromptRoute: "tool_agent"}
		if !step.OrdinaryEffect() {
			catalog.Schemas = []domainmodel.ToolSchema{toolcatalogapp.ReportDeliveryToolSchemaV1()}
		}
		return catalog
	}
	deps.AcquireProviderAttempt = func(ctx context.Context, _ int, step RuntimeProviderStep) (context.Context, func(), error) {
		attemptSteps = append(attemptSteps, step)
		return ctx, func() {}, nil
	}
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		if promotions == 2 {
			if !claimHeld {
				return RuntimeSteeringBatch{}, errors.New("report steer was promoted outside the terminal claim")
			}
			return ordinaryRuntimeSteeringBatch("Do not deliver the report; summarize the boundary only."), nil
		}
		return RuntimeSteeringBatch{}, nil
	}
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		if claimHeld {
			return nil, nil, errors.New("candidate terminal claim overlapped")
		}
		claimHeld = true
		return context.WithValue(ctx, runtimeRunnerTerminalClaimContextKey{}, true), func() { claimHeld = false }, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Deliver the current report.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "never", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext, AdditionalCaseDataEffect: true,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Deliver the current report."}},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || len(driver.executed) != 0 || len(driver.ready) != 0 ||
		result.ReportDeliveryCompleted || result.AssistantText != "Report delivery was not performed." ||
		result.CandidateTerminalContext == nil || result.ReleaseCandidateTerminal == nil || !claimHeld {
		t.Fatalf("superseded report delivery executed or lost its final: requests=%d executed=%#v ready=%#v held=%v result=%#v",
			len(provider.requests), driver.executed, driver.ready, claimHeld, result)
	}
	wantSteps := []RuntimeProviderStep{
		{Prompt: "Deliver the current report.", LogicalEffect: domainsecurity.LogicalEffectCaseData},
		{Prompt: "Do not deliver the report; summarize the boundary only.", LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
	}
	if !reflect.DeepEqual(catalogSteps, wantSteps) || !reflect.DeepEqual(attemptSteps, wantSteps) {
		t.Fatalf("terminal promotion did not update exact provider state: catalog=%#v attempts=%#v want=%#v", catalogSteps, attemptSteps, wantSteps)
	}
	for _, message := range provider.requests[1].Messages {
		if message.Role == "tool" || len(message.ToolCalls) > 0 {
			t.Fatalf("superseded report tool pair reached the next provider request: %#v", provider.requests[1].Messages)
		}
	}
	if messages := provider.requests[1].Messages; len(messages) < 2 ||
		messages[len(messages)-2].Content != terminalToolSupersededBoundary ||
		messages[len(messages)-1].Content != "Do not deliver the report; summarize the boundary only." {
		t.Fatalf("report steer boundary was not preserved: %#v", messages)
	}
	result.ReleaseCandidateTerminal()
	if claimHeld {
		t.Fatal("report replacement final claim was not released")
	}
}

func TestRuntimeRunnerOneTimeTerminalEffectsExecuteUnderReturnedClaim(t *testing.T) {
	for _, test := range []struct {
		name            string
		toolName        string
		arguments       string
		requiredFinal   bool
		reportTerminal  bool
		wantText        string
		wantReportState bool
	}{
		{
			name: "required final", toolName: toolcatalogapp.ForegroundSubmitToolName,
			arguments: `{"result":"bounded result"}`, requiredFinal: true,
			wantText: "Foreground child result submitted.",
		},
		{
			name: "report delivery", toolName: toolcatalogapp.ReportDeliveryToolName,
			arguments: `{}`, reportTerminal: true, wantText: reportDeliveryTerminalBoundaryV1, wantReportState: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			securityContext := newLoopGeneralContextV2(t, "thread-terminal-"+strings.ReplaceAll(test.name, " ", "-"), "turn-terminal-"+strings.ReplaceAll(test.name, " ", "-"), workspace)
			var schema domainmodel.ToolSchema
			if test.requiredFinal {
				schema = toolcatalogapp.ForegroundSubmitToolSchema()
			} else {
				securityContext = newLoopCaseContextV2(t, securityContext.ThreadID, securityContext.TurnID, workspace, "case-terminal-report")
				schema = toolcatalogapp.ReportDeliveryToolSchemaV1()
			}
			provider := &providerStreamStub{responses: []providerStreamResponse{
				runtimeRunnerTerminalToolResponse("terminal-effect", test.toolName, test.arguments),
			}}
			claimHeld := false
			releases := 0
			driver := &runtimeRunnerTerminalEffectDriver{
				toolStepDriverStub: &toolStepDriverStub{readOnly: map[string]bool{test.toolName: test.requiredFinal}},
				claimHeld:          &claimHeld,
				reportTerminal:     test.reportTerminal,
			}
			deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
			deps.ToolDriver = driver
			deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
				return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{schema}}
			}
			deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
				if claimHeld {
					return nil, nil, errors.New("candidate terminal claim overlapped")
				}
				claimHeld = true
				return context.WithValue(ctx, runtimeRunnerTerminalClaimContextKey{}, true), func() {
					if claimHeld {
						claimHeld = false
						releases++
					}
				}, nil
			}
			input := RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
				Prompt: "Perform the terminal effect.", ProviderID: "provider", Model: "model",
				ProviderConfig: domainmodel.TurnConfig{
					ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
				},
				ApprovalPolicy: "never", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
				SecurityContext: securityContext,
				Messages:        []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Perform the terminal effect."}},
			}
			if test.reportTerminal {
				input.AdditionalCaseDataEffect = true
			}
			if test.requiredFinal {
				input.RequiredFinalToolName = test.toolName
				input.OutputTokenBudget = 128
				input.ToolScope = []string{test.toolName}
			}
			result, err := RunRuntimeAgentLoop(context.Background(), input, deps)
			if err != nil {
				t.Fatal(err)
			}
			if len(provider.requests) != 1 || len(driver.executed) != 1 || len(driver.ready) != 1 ||
				!driver.executedUnderClaim || !claimHeld || releases != 0 ||
				result.AssistantText != test.wantText || result.ReportDeliveryCompleted != test.wantReportState ||
				result.CandidateTerminalContext == nil || result.ReleaseCandidateTerminal == nil {
				t.Fatalf("one-time effect did not stay under its returned claim: requests=%d executed=%#v ready=%#v underClaim=%v held=%v releases=%d result=%#v",
					len(provider.requests), driver.executed, driver.ready, driver.executedUnderClaim, claimHeld, releases, result)
			}
			if test.requiredFinal && !RuntimeResultCarriesOrdinaryCandidate(result) {
				t.Fatalf("ordinary terminal effect did not return a typed result: %#v", result)
			}
			if test.reportTerminal && (result.OrdinaryResult != nil || RuntimeResultCarriesOrdinaryCandidate(result)) {
				t.Fatalf("case-data terminal effect returned an ordinary result: %#v", result)
			}
			result.ReleaseCandidateTerminal()
			if claimHeld || releases != 1 {
				t.Fatalf("returned terminal claim did not release exactly once: held=%v releases=%d", claimHeld, releases)
			}
		})
	}
}

func TestRuntimeRunnerRejectedRequiredFinalReturnsHeldClaim(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-required-final-rejected", "turn-required-final-rejected", workspace)
	toolName := toolcatalogapp.ForegroundSubmitToolName
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("rejected-submit", toolName, `{"result":"rejected result"}`),
	}}
	claimHeld := false
	driver := &runtimeRunnerTerminalEffectDriver{
		toolStepDriverStub: &toolStepDriverStub{
			readOnly:     map[string]bool{toolName: true},
			executeError: map[string]bool{toolName: true},
		},
		claimHeld: &claimHeld,
	}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{toolcatalogapp.ForegroundSubmitToolSchema()}}
	}
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		claimHeld = true
		return context.WithValue(ctx, runtimeRunnerTerminalClaimContextKey{}, true), func() { claimHeld = false }, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Return one bounded result.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "never", SandboxMode: "read-only", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext, RequiredFinalToolName: toolName, OutputTokenBudget: 128,
		ToolScope: []string{toolName},
		Messages:  []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Return one bounded result."}},
	}, deps)
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "subagent_required_final_tool_rejected" ||
		len(driver.executed) != 1 || !driver.executedUnderClaim || !claimHeld ||
		result.CandidateTerminalContext == nil || result.ReleaseCandidateTerminal == nil {
		t.Fatalf("rejected required-final released its terminal claim before fixed failure: executed=%#v underClaim=%v held=%v result=%#v err=%v",
			driver.executed, driver.executedUnderClaim, claimHeld, result, err)
	}
	result.ReleaseCandidateTerminal()
	if claimHeld {
		t.Fatal("rejected required-final claim was not released by its consumer")
	}
}

func TestRuntimeRunnerReportApprovalPauseReleasesPreclaim(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-report-pause", "turn-report-pause", workspace, "case-report-pause")
	toolName := toolcatalogapp.ReportDeliveryToolName
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("pending-report", toolName, `{}`),
	}}
	claimHeld := false
	releases := 0
	driver := &runtimeRunnerTerminalEffectDriver{
		toolStepDriverStub: &toolStepDriverStub{readOnly: map[string]bool{toolName: false}},
		claimHeld:          &claimHeld,
		reportTerminal:     true,
	}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{toolcatalogapp.ReportDeliveryToolSchemaV1()}}
	}
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		claimHeld = true
		return context.WithValue(ctx, runtimeRunnerTerminalClaimContextKey{}, true), func() {
			claimHeld = false
			releases++
		}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Deliver the current report.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext, AdditionalCaseDataEffect: true,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "Deliver the current report."}},
	}, deps)
	if err != nil || !result.Paused || result.PendingKind != "approval" || result.PendingID == "" ||
		len(driver.approvals) != 1 || len(driver.executed) != 0 || claimHeld || releases != 1 ||
		result.CandidateTerminalContext != nil || result.ReleaseCandidateTerminal != nil {
		t.Fatalf("report approval pause retained a terminal claim or executed the effect: approvals=%#v executed=%#v held=%v releases=%d result=%#v err=%v",
			driver.approvals, driver.executed, claimHeld, releases, result, err)
	}
}

func runtimeRunnerTerminalToolResponse(rawCallID, name, arguments string) providerStreamResponse {
	return providerStreamResponse{
		callbackChunks: []domainmodel.Chunk{{
			Kind: domainmodel.ChunkToolCallStart,
			ToolCall: domainmodel.ToolCall{
				ID: rawCallID, Name: name,
			},
		}},
		resultChunks: []domainmodel.Chunk{{
			Kind: domainmodel.ChunkToolCall,
			ToolCall: domainmodel.ToolCall{
				ID: rawCallID, Name: name, Arguments: []byte(arguments),
			},
		}},
	}
}

func TestRuntimeRunnerDeepSeekMissingPrivateProtocolHasClosedFailureCode(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(
		t,
		"thread-deepseek-private-protocol-missing",
		"turn-deepseek-private-protocol-missing",
		workspace,
	)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("missing-private-protocol", "read", `{"path":"README.md"}`),
	}}
	events := &runtimeLoopEventRecorderStub{}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "inspect the repository", ProviderID: "provider", Model: "deepseek-v4-flash", Effort: "high",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", Family: "deepseek", EndpointFormat: "chat_completions",
			BaseURL: "https://hub.example/v1", Model: "deepseek-v4-flash",
			ReasoningProtocol: "deepseek-chat-completions", ReasoningEffort: "high",
		},
		ApprovalPolicy: "never", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "inspect the repository"},
		},
	}, runtimeRunnerTestDependencies(provider, events))
	publicFailure := PublicFailureForError(err)
	if err == nil || !errors.Is(err, appmodel.ErrAnthropicPrivateProtocolMissing) ||
		publicFailure.Code() != domainfailure.CodeProviderReasoningMarkupInvalid ||
		err.Error() != publicFailure.Message() || len(provider.requests) != 1 || result.AssistantText != "" {
		t.Fatalf("missing DeepSeek private protocol was not closed: calls=%d result=%#v code=%q err=%v",
			len(provider.requests), result, publicFailure.Code(), err)
	}
	for _, event := range events.events {
		if event["kind"] == "pipeline_stage" && event["stage"] == "provider_error" {
			t.Fatalf("post-transport private protocol failure was misreported as a stream failure: %#v", event)
		}
	}
}

func TestSourceUnavailablePolicyBlocksOnlyPureCaseProviderWork(t *testing.T) {
	securityContext := childContinuationWitnessedBoundaryContext(t)
	rawAccount := "6222020000000000000"
	for name, ordinaryWorkRequested := range map[string]bool{
		"pure case": false,
		"mixed":     true,
	} {
		t.Run(name, func(t *testing.T) {
			provider := &providerStreamStub{responses: []providerStreamResponse{{
				callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary work completed"}},
			}}}
			prompt := "查询银行账号 " + rawAccount + " 的流入"
			if ordinaryWorkRequested {
				prompt = "修改普通代码；" + prompt
			}
			policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil)
			prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
				SystemPrompt: "system", Policy: policy, UserPrompt: prompt, CaseSensitive: true,
			})
			result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: securityContext.WorkspaceRealPath,
				Prompt: prompt, ProviderID: "provider", Model: "model",
				ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
				SecurityContext: securityContext, CaseFundPolicy: policy,
				Messages: prepared.Messages, OrdinaryLaneMessages: prepared.OrdinaryLaneMessages,
				OrdinaryResultInputIsolated: prepared.OrdinaryResultInputIsolated,
			}, runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{}))
			if err != nil {
				t.Fatal(err)
			}
			if !ordinaryWorkRequested {
				if len(provider.requests) != 0 || result.AssistantText != CaseFundSourceUnavailableAnswer() {
					t.Fatalf("pure case unavailable path reached provider: calls=%d result=%#v", len(provider.requests), result)
				}
				return
			}
			if len(provider.requests) != 1 || result.AssistantText != "ordinary work completed" {
				t.Fatalf("mixed unavailable path disabled ordinary provider work: calls=%d result=%#v", len(provider.requests), result)
			}
			if result.OrdinaryResult == nil || result.OrdinaryResult.Text != "ordinary work completed" ||
				result.CandidateInputClass != RuntimeCandidateInputClassOrdinaryOnly || !RuntimeResultCarriesOrdinaryCandidate(result) {
				t.Fatalf("partitioned mixed input lost its typed ordinary candidate: %#v", result)
			}
			for _, message := range provider.requests[0].Messages {
				if strings.Contains(message.Content, rawAccount) || strings.Contains(message.Content, "查询银行账号") {
					t.Fatalf("mixed ordinary provider received protected input: %#v", provider.requests[0].Messages)
				}
			}
			if provider.requests[0].PrivateProviderTelemetry == nil ||
				!provider.requests[0].PrivateProviderTelemetry.OrdinaryEffect {
				t.Fatalf("mixed boundary provider telemetry lost ordinary effect classification: %#v", provider.requests[0].PrivateProviderTelemetry)
			}
			result.ReleaseCandidateTerminal()
		})
	}
}

func TestSourceUnavailableBoundCaseFollowupNeverReachesProvider(t *testing.T) {
	securityContext := childContinuationWitnessedBoundaryContext(t)
	for _, prompt := range []string{
		"分析该账户上月有什么异常",
		"那净额呢？",
		"再看流出。",
	} {
		t.Run(prompt, func(t *testing.T) {
			provider := &providerStreamStub{responses: []providerStreamResponse{{
				callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "must not be called"}},
			}}}
			policy := CaseFundAnalysisPolicyForWorkspace(true, prompt, nil)
			if !policy.MustReturnBoundaryBeforeProvider() || policy.OrdinaryWorkRequested || policy.OrdinaryPrompt != "" {
				t.Fatalf("bound follow-up did not freeze a pure source boundary: %#v", policy)
			}
			prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
				SystemPrompt: "system", Policy: policy, UserPrompt: prompt, CaseSensitive: true,
			})
			result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: securityContext.WorkspaceRealPath,
				Prompt: prompt, ProviderID: "provider", Model: "model",
				ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
				SecurityContext: securityContext, CaseFundPolicy: policy,
				Messages: prepared.Messages, OrdinaryLaneMessages: prepared.OrdinaryLaneMessages,
				OrdinaryResultInputIsolated: prepared.OrdinaryResultInputIsolated,
			}, runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{}))
			if err != nil || len(provider.requests) != 0 || result.AssistantText != CaseFundSourceUnavailableAnswer() ||
				result.OrdinaryResult != nil {
				t.Fatalf("source-unavailable follow-up crossed provider boundary: calls=%d result=%#v err=%v",
					len(provider.requests), result, err)
			}
		})
	}
}

func TestRuntimeRunnerMixedFundsPreparationDowngradesOnlyProtectedEffect(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-mixed-native-unavailable", "turn-mixed-native-unavailable", workspace, "mixed-native-unavailable")
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary work completed"}},
	}}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	prepared := []RuntimeProviderStep{}
	resolved := []RuntimeProviderStep{}
	deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
		prepared = append(prepared, step)
		return RuntimeProviderStep{
			Prompt: step.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
			CaseSourceUnavailable: true,
		}, nil
	}
	deps.ResolveToolCatalog = func(_ bool, step RuntimeProviderStep) RuntimeRunnerToolCatalog {
		resolved = append(resolved, step)
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", SystemPrompt: "ordinary source-unavailable policy"}
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "modify ordinary code and inspect current-case funds", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		CaseFundPolicy:  CaseFundAnalysisPolicy{Active: true, OrdinaryWorkRequested: true},
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "modify ordinary code and inspect current-case funds"},
		},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssistantText != "ordinary work completed" || !result.CaseSourceUnavailable ||
		len(prepared) != 1 || !prepared[0].UsesFundsDataAuthority() ||
		len(resolved) != 1 || !resolved[0].OrdinaryEffect() || !resolved[0].OrdinaryWork {
		t.Fatalf("mixed funds preparation did not preserve ordinary work with a sticky case boundary: prepared=%#v resolved=%#v result=%#v", prepared, resolved, result)
	}
	if result.OrdinaryResult == nil || result.OrdinaryResult.Text != "ordinary work completed" ||
		result.CandidateInputClass != RuntimeCandidateInputClassOrdinaryOnly || !RuntimeResultCarriesOrdinaryCandidate(result) {
		t.Fatalf("protected-to-ordinary downgrade lost ordinary-only provenance: %#v", result)
	}
	if len(provider.requests) != 1 || provider.requests[0].PrivateProviderTelemetry == nil ||
		!provider.requests[0].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("downgraded provider attempt retained protected effect telemetry: %#v", provider.requests)
	}
	for _, message := range provider.requests[0].Messages {
		if strings.Contains(message.Content, "current-case funds") {
			t.Fatalf("downgraded ordinary provider received the protected clause: %#v", provider.requests[0].Messages)
		}
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerDeepSeekProtectedDowngradeClearsPrivateCapsule(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-deepseek-downgrade", "turn-deepseek-downgrade", workspace, "deepseek-downgrade")
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary work completed"}},
	}}}
	providerConfig := domainmodel.TurnConfig{
		ProviderID: "provider", Family: "deepseek", EndpointFormat: "chat_completions",
		BaseURL: "https://provider.example/v1", Model: "model",
		ReasoningProtocol: "deepseek-chat-completions", ReasoningEffort: "high",
	}
	call := domainmodel.ToolCall{
		ID: loopTestHostToolCallID("protected-read"), Name: "read",
		Arguments: []byte(`{"path":"protected.txt"}`),
	}
	protectedMessages := []domainmodel.Message{
		{Role: "system", Content: "protected system"},
		{Role: "user", Content: "modify ordinary code and inspect current-case funds"},
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}},
		{Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: "protected result"},
	}
	issued, err := appmodel.IssueAnthropicPrivateProtocol(appmodel.AnthropicPrivateProtocolIssueInput{
		ProviderConfig: providerConfig,
		EndpointFormat: providerConfig.EndpointFormat,
		ContextDigest:  securityContext.ContextDigest,
		PromptRoute:    "protected",
		ToolManifestHash: domainsecurity.SHA256Hex(
			[]byte("protected tools"),
		),
		Sequence:              1,
		Messages:              protectedMessages[:2],
		AssistantMessageIndex: 2,
		AssistantMessage:      protectedMessages[2],
		Thinking:              "DEEPSEEK_PRIVATE_DOWNGRADE_REASONING",
		ToolCallCount:         1,
		Effort:                "high",
	})
	if err != nil || issued.Session == nil || issued.Capsule == nil {
		t.Fatalf("issue DeepSeek downgrade capsule: issued=%#v err=%v", issued, err)
	}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
		return RuntimeProviderStep{
			Prompt: step.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary,
			OrdinaryWork: true, CaseSourceUnavailable: true,
		}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "modify ordinary code and inspect current-case funds", ProviderID: "provider", Model: "model",
		ProviderConfig: providerConfig, Effort: "high",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		CaseFundPolicy:  CaseFundAnalysisPolicy{Active: true, OrdinaryWorkRequested: true},
		Messages:        protectedMessages,
		OrdinaryLaneMessages: []domainmodel.Message{
			{Role: "system", Content: "ordinary system"},
			{Role: "user", Content: "modify ordinary code"},
		},
		PrivateProtocolSession: issued.Session,
		AnthropicCapsule:       issued.Capsule,
		ProviderCallSequence:   1,
	}, deps)
	if err != nil || result.AssistantText != "ordinary work completed" || len(provider.requests) != 1 {
		t.Fatalf("DeepSeek protected-to-ordinary downgrade failed: result=%#v requests=%#v err=%v", result, provider.requests, err)
	}
	for _, message := range provider.requests[0].Messages {
		if message.Role == "tool" || len(message.ToolCalls) > 0 ||
			strings.Contains(message.Content, "protected result") ||
			strings.Contains(message.Content, "current-case funds") {
			t.Fatalf("DeepSeek protected capsule/history crossed the ordinary downgrade: %#v", provider.requests[0].Messages)
		}
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerAnthropicHistorySeparatesOrdinaryAndPrivateContinuation(t *testing.T) {
	tests := []struct {
		name       string
		protocol   string
		effort     string
		wantNative bool
	}{
		{
			name:       "ordinary blank protocol off retains native tool wire",
			effort:     "off",
			wantNative: true,
		},
		{
			name:       "ordinary blank protocol default effort retains native tool wire",
			wantNative: true,
		},
		{
			name:     "explicit private protocol missing state uses safe history",
			protocol: "anthropic-thinking",
			effort:   "high",
		},
		{
			name:       "legacy blank protocol high without observed state retains native tool wire",
			effort:     "high",
			wantNative: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			securityContext := newLoopGeneralContextV2(
				t,
				"thread-anthropic-history-"+strings.ReplaceAll(test.name, " ", "-"),
				"turn-anthropic-history-"+strings.ReplaceAll(test.name, " ", "-"),
				workspace,
			)
			call := domainmodel.ToolCall{
				ID: loopTestHostToolCallID(test.name), Name: "read",
				Arguments: json.RawMessage(`{"path":"README.md"}`),
			}
			provider := &providerStreamStub{responses: []providerStreamResponse{{
				callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "complete"}},
			}}}
			result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
				Workspace: workspace, Prompt: "continue",
				ProviderID: "anthropic", Model: "claude-test", Effort: test.effort,
				ProviderConfig: domainmodel.TurnConfig{
					ProviderID: "anthropic", Family: "anthropic-compatible",
					EndpointFormat: "messages", BaseURL: "https://api.anthropic.test/v1",
					Model: "claude-test", ReasoningProtocol: test.protocol,
					ReasoningEffort: test.effort,
				},
				ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
				EffectiveMaxModelSteps: 1, SecurityContext: securityContext,
				Messages: []domainmodel.Message{
					{Role: "system", Content: "system"},
					{Role: "user", Content: "inspect"},
					{Role: "assistant", ToolCalls: []domainmodel.ToolCall{call}},
					{Role: "tool", Name: call.Name, ToolCallID: call.ID, Content: "result"},
				},
			}, runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{}))
			if err != nil || result.AssistantText != "complete" || len(provider.requests) != 1 {
				t.Fatalf("Anthropic history continuation failed: result=%#v calls=%d err=%v",
					result, len(provider.requests), err)
			}
			requestMessages := provider.requests[0].Messages
			hasNativeAssistant := false
			hasNativeToolResult := false
			hasSafeHistory := false
			for _, message := range requestMessages {
				hasNativeAssistant = hasNativeAssistant ||
					(message.Role == "assistant" && len(message.ToolCalls) == 1)
				hasNativeToolResult = hasNativeToolResult || message.Role == "tool"
				hasSafeHistory = hasSafeHistory ||
					strings.Contains(message.Content, "Analytix host-selected prior tool activity")
			}
			if test.wantNative {
				if !hasNativeAssistant || !hasNativeToolResult || hasSafeHistory {
					t.Fatalf("ordinary Anthropic history was rewritten: %#v", requestMessages)
				}
			} else if hasNativeAssistant || hasNativeToolResult || !hasSafeHistory {
				t.Fatalf("private Anthropic history was not safely recompiled: %#v", requestMessages)
			}
			result.ReleaseCandidateTerminal()
		})
	}
}

func TestRuntimeRunnerAnthropicRetainsEveryThinkingBlockAcrossToolChain(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(
		t,
		"thread-anthropic-thinking-chain",
		"turn-anthropic-thinking-chain",
		workspace,
	)
	providerConfig := domainmodel.TurnConfig{
		ProviderID: "anthropic", Family: "anthropic-compatible",
		EndpointFormat: "messages", BaseURL: "https://api.anthropic.test/v1",
		Model: "claude-test", ReasoningProtocol: "anthropic-thinking",
	}
	const (
		firstSignatureSentinel  = "sig_anthropic_signature_only"
		secondThinkingSentinel  = "ANTHROPIC_SECOND_PRIVATE_THINKING"
		secondSignatureSentinel = "sig_anthropic_second"
	)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{
				{Kind: domainmodel.ChunkReasoning, Signature: firstSignatureSentinel},
				{
					Kind: domainmodel.ChunkToolCallStart,
					ToolCall: domainmodel.ToolCall{
						ID: "provider_anthropic_first", Name: "lookup",
					},
				},
			},
			resultChunks: []domainmodel.Chunk{{
				Kind: domainmodel.ChunkToolCall,
				ToolCall: domainmodel.ToolCall{
					ID: "provider_anthropic_first", Name: "lookup", Arguments: json.RawMessage(`{}`),
				},
			}},
		},
		{
			callbackChunks: []domainmodel.Chunk{
				{
					Kind: domainmodel.ChunkReasoning, Text: secondThinkingSentinel,
					Signature: secondSignatureSentinel,
				},
				{
					Kind: domainmodel.ChunkToolCallStart,
					ToolCall: domainmodel.ToolCall{
						ID: "provider_anthropic_second", Name: "lookup",
					},
				},
			},
			resultChunks: []domainmodel.Chunk{{
				Kind: domainmodel.ChunkToolCall,
				ToolCall: domainmodel.ToolCall{
					ID: "provider_anthropic_second", Name: "lookup", Arguments: json.RawMessage(`{}`),
				},
			}},
		},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "complete"}}},
	}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	driver := &toolStepDriverStub{}
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{
			PromptRoute: "tool_agent",
			Schemas:     zeroArgumentToolSchemas("lookup"),
		}
	}
	first, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Workspace: workspace, Prompt: "look up both values",
		ProviderID: "anthropic", Model: "claude-test", Effort: "high",
		ProviderConfig: providerConfig,
		ApprovalPolicy: "always", SandboxMode: "read-only",
		EffectiveMaxModelSteps: 1, SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "look up both values"},
		},
	}, deps)
	if err != nil || !first.Paused || len(driver.pendings) != 1 ||
		len(driver.pendings[0].PrivateProtocolCapsules) != 1 ||
		!driver.pendings[0].PrivateProtocolObserved {
		t.Fatalf("first Anthropic response did not issue a signature-only capsule: result=%#v pendings=%#v err=%v",
			first, driver.pendings, err)
	}
	firstPending := driver.pendings[0]
	firstMessages := appmodel.CloneProviderMessages(firstPending.Messages)
	firstMessages = append(firstMessages, domainmodel.Message{
		Role:       "tool",
		Name:       firstPending.Call.Name,
		ToolCallID: firstPending.Call.ID,
		Content:    "first lookup result",
	})
	second, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Workspace: workspace, Prompt: "look up both values",
		ProviderID: "anthropic", Model: "claude-test", Effort: "high",
		ProviderConfig: providerConfig,
		ApprovalPolicy: "always", SandboxMode: "read-only",
		EffectiveMaxModelSteps: 1, SecurityContext: securityContext,
		Messages: firstMessages, PrivateProtocolSession: firstPending.PrivateProtocolSession,
		PrivateProtocolCapsules: firstPending.PrivateProtocolCapsules, ProviderCallSequence: 1,
	}, deps)
	if err != nil || !second.Paused || len(driver.pendings) != 2 ||
		len(driver.pendings[1].PrivateProtocolCapsules) != 2 ||
		!driver.pendings[1].PrivateProtocolObserved {
		t.Fatalf("second Anthropic response did not retain both response-derived capsules: result=%#v pendings=%#v err=%v",
			second, driver.pendings, err)
	}
	secondPending := driver.pendings[1]
	secondMessages := appmodel.CloneProviderMessages(secondPending.Messages)
	secondMessages = append(secondMessages, domainmodel.Message{
		Role:       "tool",
		Name:       secondPending.Call.Name,
		ToolCallID: secondPending.Call.ID,
		Content:    "second lookup result",
	})
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Workspace: workspace, Prompt: "look up both values",
		ProviderID: "anthropic", Model: "claude-test", Effort: "high",
		ProviderConfig: providerConfig,
		ApprovalPolicy: "always", SandboxMode: "read-only",
		EffectiveMaxModelSteps: 1, SecurityContext: securityContext,
		Messages: secondMessages, PrivateProtocolSession: secondPending.PrivateProtocolSession,
		PrivateProtocolCapsules: secondPending.PrivateProtocolCapsules, ProviderCallSequence: 2,
	}, deps)
	if result.AssistantText != "complete" || len(provider.requests) != 3 ||
		len(provider.requests[0].DeepSeekReasoningReplays) != 0 ||
		len(provider.requests[1].DeepSeekReasoningReplays) != 1 ||
		len(provider.requests[2].DeepSeekReasoningReplays) != 2 ||
		err != nil {
		t.Fatalf("Anthropic thinking chain replay mismatch: result=%#v requests=%#v err=%v", result, provider.requests, err)
	}
	for _, request := range provider.requests {
		body, marshalErr := json.Marshal(request.Messages)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		for _, private := range []string{
			firstSignatureSentinel,
			secondThinkingSentinel,
			secondSignatureSentinel,
		} {
			if strings.Contains(string(body), private) {
				t.Fatalf("Anthropic private bytes became generic request JSON: %s", body)
			}
		}
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerAnthropicPlanCatalogTransitionCompilesSafeHistory(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(
		t,
		"thread-anthropic-plan-catalog",
		"turn-anthropic-plan-catalog",
		workspace,
	)
	const (
		privateThinking  = "ANTHROPIC_PLAN_PRIVATE_THINKING"
		privateSignature = "sig_anthropic_plan_catalog"
	)
	providerConfig := domainmodel.TurnConfig{
		ProviderID: "anthropic", Family: "anthropic-compatible",
		EndpointFormat: "messages", BaseURL: "https://api.anthropic.test/v1",
		Model: "claude-test", ReasoningProtocol: "anthropic-thinking",
	}
	oldSchemas := zeroArgumentToolSchemas("read", toolcatalogapp.ToolCreatePlanName)
	readCall := domainmodel.ToolCall{
		ID: loopTestHostToolCallID("anthropic-plan-catalog"), Name: "read",
		Arguments: json.RawMessage(`{}`),
	}
	messages := []domainmodel.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "inspect the source and propose a plan"},
		{Role: "assistant", ToolCalls: []domainmodel.ToolCall{readCall}},
		{Role: "tool", Name: readCall.Name, ToolCallID: readCall.ID, Content: "source contents"},
	}
	issued, err := appmodel.IssueAnthropicPrivateProtocol(appmodel.AnthropicPrivateProtocolIssueInput{
		ProviderConfig: providerConfig, EndpointFormat: providerConfig.EndpointFormat,
		ContextDigest: securityContext.ContextDigest, PromptRoute: "tool_agent",
		ToolManifestHash: toolcatalogapp.ToolSchemaHash(oldSchemas), Sequence: 2,
		Messages: messages[:2], AssistantMessageIndex: 2, AssistantMessage: messages[2],
		Thinking: privateThinking, Signature: privateSignature, ToolCallCount: 1, Effort: "high",
	})
	if err != nil || issued.Session == nil || issued.Capsule == nil {
		t.Fatalf("issue plan-catalog capsule: issued=%#v err=%v", issued, err)
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{callbackChunks: []domainmodel.Chunk{{
		Kind: domainmodel.ChunkText, Text: "Which implementation path do you prefer?",
	}}}}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{
			PromptRoute: "tool_agent",
			Schemas:     zeroArgumentToolSchemas(toolcatalogapp.ToolCreatePlanName),
		}
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Workspace: workspace, Prompt: "inspect the source and propose a plan", Mode: "plan",
		ProviderID: "anthropic", Model: "claude-test", Effort: "high",
		ProviderConfig: providerConfig,
		ApprovalPolicy: "never", SandboxMode: "read-only", NormalizedSandboxMode: "read-only",
		EffectiveMaxModelSteps: 1, SecurityContext: securityContext, Messages: messages,
		PrivateProtocolSession: issued.Session, PrivateProtocolCapsules: []*domainmodel.AnthropicThinkingCapsule{issued.Capsule},
		ProviderCallSequence: 1,
	}, deps)
	if err != nil || result.AssistantText != "Which implementation path do you prefer?" ||
		len(provider.requests) != 1 {
		t.Fatalf("Anthropic plan catalog transition failed: result=%#v requests=%d err=%v",
			result, len(provider.requests), err)
	}
	request := provider.requests[0]
	if request.AnthropicThinkingReplay != nil || len(request.DeepSeekReasoningReplays) != 0 ||
		len(request.Tools) != 1 || request.Tools[0].Name != toolcatalogapp.ToolCreatePlanName {
		t.Fatalf("changed plan request retained old private replay or manifest: %#v", request)
	}
	safeHistory := false
	for _, message := range request.Messages {
		if message.Role == "tool" || len(message.ToolCalls) > 0 {
			t.Fatalf("changed plan request retained native private tool wire: %#v", request.Messages)
		}
		safeHistory = safeHistory ||
			strings.Contains(message.Content, "Analytix host-selected prior tool activity")
		encoded, marshalErr := json.Marshal(message)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(encoded), privateThinking) ||
			strings.Contains(string(encoded), privateSignature) {
			t.Fatalf("private Anthropic fields entered generic history: %s", encoded)
		}
	}
	if !safeHistory {
		t.Fatalf("changed plan request lacks safe semantic tool history: %#v", request.Messages)
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerImplicitAnthropicSteeringClosesPrivateToolWire(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(
		t,
		"thread-anthropic-steering",
		"turn-anthropic-steering",
		workspace,
	)
	const (
		thinkingSentinel  = "ANTHROPIC_STEERING_PRIVATE_THINKING"
		signatureSentinel = "sig_anthropic_steering_private"
	)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{
				{
					Kind: domainmodel.ChunkReasoning, Text: thinkingSentinel,
					Signature: signatureSentinel,
				},
				{
					Kind: domainmodel.ChunkToolCallStart,
					ToolCall: domainmodel.ToolCall{
						ID: "provider_anthropic_steer", Name: "lookup",
					},
				},
			},
			resultChunks: []domainmodel.Chunk{{
				Kind: domainmodel.ChunkToolCall,
				ToolCall: domainmodel.ToolCall{
					ID: "provider_anthropic_steer", Name: "lookup", Arguments: json.RawMessage(`{}`),
				},
			}},
		},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "steered complete"}}},
	}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = &toolStepDriverStub{readOnly: map[string]bool{"lookup": true}}
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{
			PromptRoute: "tool_agent",
			Schemas:     zeroArgumentToolSchemas("lookup"),
		}
	}
	promotions := 0
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		if promotions == 2 {
			return ordinaryRuntimeSteeringBatch("Continue after the mid-turn correction."), nil
		}
		return RuntimeSteeringBatch{}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		Workspace: workspace, Prompt: "look up the value",
		ProviderID: "anthropic", Model: "claude-test",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "anthropic", Family: "anthropic-compatible",
			EndpointFormat: "messages", BaseURL: "https://api.anthropic.test/v1",
			Model: "claude-test",
		},
		ApprovalPolicy: "never", SandboxMode: "read-only",
		EffectiveMaxModelSteps: 3, SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "look up the value"},
		},
	}, deps)
	if err != nil || result.AssistantText != "steered complete" || len(provider.requests) != 2 {
		t.Fatalf("implicit Anthropic steering failed: result=%#v calls=%d err=%v",
			result, len(provider.requests), err)
	}
	second := provider.requests[1]
	if len(second.DeepSeekReasoningReplays) != 0 || second.AnthropicThinkingReplay != nil {
		t.Fatalf("steering retained attempt-private Anthropic replay: %#v", second)
	}
	hasSafeHistory := false
	for _, message := range second.Messages {
		if message.Role == "tool" || (message.Role == "assistant" && len(message.ToolCalls) > 0) {
			t.Fatalf("steering retained native Anthropic tool wire: %#v", second.Messages)
		}
		hasSafeHistory = hasSafeHistory ||
			strings.Contains(message.Content, "Analytix host-selected prior tool activity")
		if strings.Contains(message.Content, thinkingSentinel) ||
			strings.Contains(message.Content, signatureSentinel) {
			t.Fatalf("steering exposed private Anthropic bytes: %#v", second.Messages)
		}
	}
	if !hasSafeHistory {
		t.Fatalf("steering lost safe semantic tool history: %#v", second.Messages)
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerSourceUnavailableRemainsStickyAcrossProtectedSteering(t *testing.T) {
	workspace := t.TempDir()
	rawAccount := "6222020000000000000"
	for _, test := range []struct {
		name              string
		steering          RuntimeSteeringBatch
		responses         []providerStreamResponse
		wantProviderCalls int
		wantFinal         string
		wantOrdinary      bool
	}{
		{
			name: "pure protected steering stays closed",
			steering: runtimeSteeringBatch(
				"查询银行账号 "+rawAccount+" 的流入", domainsecurity.LogicalEffectFundsData, false,
			),
			responses: []providerStreamResponse{{
				callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "initial ordinary result"}},
			}},
			wantProviderCalls: 1,
			wantFinal:         CaseFundSourceUnavailableAnswer(),
		},
		{
			name: "mixed protected steering keeps only exact ordinary clause",
			steering: runtimeSteeringBatch(
				"查询银行账号 "+rawAccount+" 的流入；运行测试", domainsecurity.LogicalEffectFundsData, true,
			),
			responses: []providerStreamResponse{
				{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "initial ordinary result"}}},
				{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "steered ordinary result"}}},
			},
			wantProviderCalls: 2,
			wantFinal:         "steered ordinary result",
			wantOrdinary:      true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			securityContext := newLoopCaseContextV2(
				t, "thread-sticky-steer", "turn-sticky-steer", workspace, "case-sticky-steer",
			)
			prompt := "修改普通代码；查询银行账号 " + rawAccount + " 的流入"
			policy := CaseFundAnalysisPolicyForWorkspace(false, prompt, nil)
			prepared := PrepareInitialProviderMessagesV1(InitialProviderMessagesInputV1{
				SystemPrompt: "system", Policy: policy, UserPrompt: prompt, CaseSensitive: true,
			})
			provider := &providerStreamStub{responses: test.responses}
			deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
			promotions := 0
			deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
				promotions++
				if promotions == 2 {
					return test.steering, nil
				}
				return RuntimeSteeringBatch{}, nil
			}
			preparedSteps := []RuntimeProviderStep{}
			attemptSteps := []RuntimeProviderStep{}
			deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
				preparedSteps = append(preparedSteps, step)
				return step, nil
			}
			deps.AcquireProviderAttempt = func(ctx context.Context, _ int, step RuntimeProviderStep) (context.Context, func(), error) {
				attemptSteps = append(attemptSteps, step)
				return ctx, func() {}, nil
			}
			result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
				Prompt: prompt, ProviderID: "provider", Model: "model",
				ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
				SecurityContext: securityContext, CaseFundPolicy: policy,
				Messages: prepared.Messages, OrdinaryLaneMessages: prepared.OrdinaryLaneMessages,
				OrdinaryResultInputIsolated: prepared.OrdinaryResultInputIsolated,
			}, deps)
			if err != nil || result.AssistantText != test.wantFinal || !result.CaseSourceUnavailable ||
				len(provider.requests) != test.wantProviderCalls {
				t.Fatalf("sticky source boundary result mismatch: prepared=%#v attempts=%#v calls=%d result=%#v err=%v",
					preparedSteps, attemptSteps, len(provider.requests), result, err)
			}
			for _, step := range append(append([]RuntimeProviderStep{}, preparedSteps...), attemptSteps...) {
				if !step.OrdinaryEffect() || !step.CaseSourceUnavailable {
					t.Fatalf("sticky source boundary reached protected preparation/currentness: %#v", step)
				}
			}
			for _, request := range provider.requests {
				for _, message := range request.Messages {
					if strings.Contains(message.Content, rawAccount) || strings.Contains(message.Content, "查询银行账号") {
						t.Fatalf("protected steering reached ordinary provider payload: %#v", request.Messages)
					}
				}
			}
			if test.wantOrdinary {
				if result.OrdinaryResult == nil || !RuntimeResultCarriesOrdinaryCandidate(result) {
					t.Fatalf("mixed steering lost its typed ordinary result: %#v", result)
				}
			} else if result.OrdinaryResult != nil || RuntimeResultCarriesOrdinaryCandidate(result) {
				t.Fatalf("pure protected steering published an ordinary candidate as its terminal: %#v", result)
			}
			if result.ReleaseCandidateTerminal != nil {
				result.ReleaseCandidateTerminal()
			}
		})
	}
}

func TestRuntimeRunnerRestoredOrFreshFundsStepCannotOverrideStickySourceClosure(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-sticky-restart", "turn-sticky-restart", workspace, "case-sticky-restart")
	for _, restored := range []bool{false, true} {
		name := "fresh funds classification"
		if restored {
			name = "restored funds step"
		}
		t.Run(name, func(t *testing.T) {
			prompt := "analyze the current case funds"
			provider := &providerStreamStub{}
			deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
			prepared, cataloged, attempted := 0, 0, 0
			deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
				prepared++
				return step, nil
			}
			deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
				cataloged++
				return RuntimeRunnerToolCatalog{}
			}
			deps.AcquireProviderAttempt = func(ctx context.Context, _ int, _ RuntimeProviderStep) (context.Context, func(), error) {
				attempted++
				return ctx, func() {}, nil
			}
			input := RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
				Prompt: prompt, ProviderID: "provider", Model: "model",
				ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
				SecurityContext: securityContext, CaseFundPolicy: CaseFundAnalysisPolicy{Active: true},
				CaseSourceUnavailable: true,
				Messages:              []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: prompt}},
			}
			if restored {
				input.RestoredProviderStep = &RuntimeProviderStep{
					Prompt: prompt, LogicalEffect: domainsecurity.LogicalEffectFundsData,
				}
			}
			result, err := RunRuntimeAgentLoop(context.Background(), input, deps)
			if err != nil || result.AssistantText != CaseFundSourceUnavailableAnswer() || !result.CaseSourceUnavailable ||
				prepared != 0 || cataloged != 0 || attempted != 0 || len(provider.requests) != 0 {
				t.Fatalf("sticky source closure reopened protected work: prepared=%d cataloged=%d attempted=%d calls=%d result=%#v err=%v",
					prepared, cataloged, attempted, len(provider.requests), result, err)
			}
			if result.ReleaseCandidateTerminal != nil {
				result.ReleaseCandidateTerminal()
			}
		})
	}
}

func TestRuntimeRunnerProtectedAttemptCurrentnessFailureContinuesExactOrdinaryLane(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-provider-currentness", "turn-provider-currentness", workspace, "provider-currentness")
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary work completed after protected authority changed"}},
	}}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	attemptSteps := []RuntimeProviderStep{}
	deps.AcquireProviderAttempt = func(ctx context.Context, _ int, step RuntimeProviderStep) (context.Context, func(), error) {
		attemptSteps = append(attemptSteps, step)
		if step.UsesCaseDataAuthority() {
			return nil, nil, executiongrantapp.ValidationError{Code: "turn_security_dataset_snapshot_mismatch"}
		}
		return ctx, func() {}, nil
	}
	var terminalStep RuntimeProviderStep
	deps.AcquireCandidateTerminal = func(ctx context.Context, step RuntimeProviderStep) (context.Context, func(), error) {
		terminalStep = step
		return ctx, func() {}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "inspect protected funds and update ordinary code", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		CaseFundPolicy:  CaseFundAnalysisPolicy{Active: true, OrdinaryWorkRequested: true},
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "inspect protected funds and update ordinary code"},
		},
		OrdinaryLaneMessages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "assistant", Content: "trusted typed ordinary history"},
		},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssistantText != "ordinary work completed after protected authority changed" ||
		!result.CaseSourceUnavailable || result.CandidateUsesCaseData ||
		len(attemptSteps) != 2 || !attemptSteps[0].UsesFundsDataAuthority() ||
		!attemptSteps[1].OrdinaryEffect() || !attemptSteps[1].CaseSourceUnavailable ||
		!terminalStep.OrdinaryEffect() || !terminalStep.CaseSourceUnavailable {
		t.Fatalf("protected attempt did not recover only the ordinary lane: attempts=%#v terminal=%#v result=%#v", attemptSteps, terminalStep, result)
	}
	if result.OrdinaryResult == nil ||
		result.CandidateInputClass != RuntimeCandidateInputClassOrdinaryOnly || !RuntimeResultCarriesOrdinaryCandidate(result) {
		t.Fatalf("protected attempt fallback lost ordinary-only provenance: %#v", result)
	}
	if len(provider.requests) != 1 || provider.requests[0].PrivateProviderTelemetry == nil ||
		!provider.requests[0].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("protected attempt reached transport or ordinary retry lost telemetry: %#v", provider.requests)
	}
	if len(provider.requests[0].Messages) != 3 ||
		provider.requests[0].Messages[1].Content != "trusted typed ordinary history" ||
		provider.requests[0].Messages[2].Content != "update ordinary code" {
		t.Fatalf("ordinary retry reused an unpartitioned case transcript or lost its subrequest: %#v", provider.requests[0].Messages)
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerProtectedAttachmentFailureContinuesIndependentOrdinaryWork(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(
		t, "thread-attachment-currentness", "turn-attachment-currentness", workspace, "case-attachment-currentness",
	)
	prompt := "update ordinary code; inspect the attached case evidence"
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary code update completed"}},
	}}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	attemptSteps := []RuntimeProviderStep{}
	deps.AcquireProviderAttempt = func(ctx context.Context, _ int, step RuntimeProviderStep) (context.Context, func(), error) {
		attemptSteps = append(attemptSteps, step)
		return ctx, func() {}, nil
	}
	materializationCalls := 0
	deps.PrepareProviderAttempt = func(
		context.Context, int, domainmodel.Request, string, string, uint64,
	) (domainmodel.Request, ProviderAttemptSettlement, error) {
		materializationCalls++
		return domainmodel.Request{}, nil, executiongrantapp.ValidationError{Code: "turn_security_dataset_snapshot_mismatch"}
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: prompt, ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		HasAttachments:  true, AttachmentPlanDigest: domainsecurity.SHA256Hex([]byte("protected-attachment-plan")),
		AdditionalCaseDataEffect: true,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: prompt, PrivateAttachmentPlanDigest: domainsecurity.SHA256Hex([]byte("protected-attachment-plan"))},
		},
		OrdinaryLaneMessages: []domainmodel.Message{{Role: "system", Content: "system"}},
	}, deps)
	if err != nil || result.AssistantText != "ordinary code update completed" || !result.CaseSourceUnavailable ||
		materializationCalls != 1 || len(provider.requests) != 1 || len(attemptSteps) != 2 ||
		!attemptSteps[0].UsesCaseDataAuthority() || !attemptSteps[0].OrdinaryWork ||
		!attemptSteps[1].OrdinaryEffect() || !attemptSteps[1].CaseSourceUnavailable {
		t.Fatalf("protected attachment failure did not preserve only independent ordinary work: calls=%d attempts=%#v result=%#v err=%v",
			materializationCalls, attemptSteps, result, err)
	}
	if result.OrdinaryResult == nil || result.OrdinaryResult.Text != "ordinary code update completed" ||
		!RuntimeResultCarriesOrdinaryCandidate(result) {
		t.Fatalf("protected attachment fallback lost its typed ordinary result: %#v", result)
	}
	request := provider.requests[0]
	if request.PrivateAttachmentPlanDigest != "" || request.PrivateProviderTelemetry == nil ||
		!request.PrivateProviderTelemetry.OrdinaryEffect || len(request.Messages) != 2 ||
		request.Messages[1].Content != "update ordinary code" || request.Messages[1].PrivateAttachmentPlanDigest != "" {
		t.Fatalf("protected attachment material reached ordinary provider retry: %#v", request)
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerProtectedAttemptUnsafeAuthorityFailureRemainsFatal(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-provider-unsafe", "turn-provider-unsafe", workspace, "provider-unsafe")
	provider := &providerStreamStub{}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.AcquireProviderAttempt = func(context.Context, int, RuntimeProviderStep) (context.Context, func(), error) {
		return nil, nil, executiongrantapp.ValidationError{Code: "turn_security_identity_mismatch"}
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "inspect protected funds and update ordinary code", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		CaseFundPolicy:  CaseFundAnalysisPolicy{Active: true, OrdinaryWorkRequested: true},
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "inspect protected funds and update ordinary code"},
		},
	}, deps)
	var callbackErr ProviderStreamCallbackError
	if !errors.As(err, &callbackErr) || result.CaseSourceUnavailable || len(provider.requests) != 0 {
		t.Fatalf("unsafe authority failure was downgraded: calls=%d result=%#v err=%v", len(provider.requests), result, err)
	}
}

func TestRuntimeRunnerProtectedToolAdmissionFailureContinuesOrdinaryLane(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-tool-currentness", "turn-tool-currentness", workspace, "tool-currentness")
	toolName := toolcatalogapp.ReportDeliveryToolName
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("stale-protected-report", toolName, `{}`),
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary work completed after protected tool was blocked"}}},
	}}
	driver := &toolStepDriverStub{
		parallel: map[string]bool{toolName: true},
		readOnly: map[string]bool{toolName: true},
		admissionHook: func(_ context.Context, pending appmodel.PendingToolCall) error {
			if pending.Call.Name == toolName {
				return executiongrantapp.ValidationError{Code: "execution_grant_connection_epoch_mismatch"}
			}
			return nil
		},
	}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(_ bool, step RuntimeProviderStep) RuntimeRunnerToolCatalog {
		catalog := RuntimeRunnerToolCatalog{PromptRoute: "tool_agent"}
		if step.UsesCaseDataAuthority() {
			catalog.Schemas = zeroArgumentToolSchemas(toolName)
		}
		return catalog
	}
	releases := 0
	terminalSteps := []RuntimeProviderStep{}
	deps.AcquireCandidateTerminal = func(ctx context.Context, step RuntimeProviderStep) (context.Context, func(), error) {
		terminalSteps = append(terminalSteps, step)
		return ctx, func() { releases++ }, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "update ordinary code; deliver the protected report", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "never", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		CaseFundPolicy:  CaseFundAnalysisPolicy{Active: true, OrdinaryWorkRequested: true},
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "update ordinary code; deliver the protected report"},
		},
	}, deps)
	if err != nil {
		t.Fatalf("protected tool recovery failed: result=%#v requests=%#v driver=%#v err=%v", result, provider.requests, driver, err)
	}
	if result.AssistantText != "ordinary work completed after protected tool was blocked" ||
		!result.CaseSourceUnavailable || result.CandidateUsesCaseData || releases != 1 ||
		len(terminalSteps) != 2 || !terminalSteps[0].UsesCaseDataAuthority() ||
		!terminalSteps[1].OrdinaryEffect() || !terminalSteps[1].CaseSourceUnavailable {
		t.Fatalf("protected tool failure did not transition to exact ordinary terminal: steps=%#v releases=%d result=%#v", terminalSteps, releases, result)
	}
	if result.OrdinaryResult == nil ||
		result.CandidateInputClass != RuntimeCandidateInputClassOrdinaryOnly || !RuntimeResultCarriesOrdinaryCandidate(result) {
		t.Fatalf("protected tool fallback lost ordinary-only provenance: %#v", result)
	}
	if len(driver.ready) != 0 || len(driver.executed) != 0 || len(provider.requests) != 2 ||
		provider.requests[1].PrivateProviderTelemetry == nil || !provider.requests[1].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("blocked protected tool crossed effect or ordinary retry was unavailable: driver=%#v requests=%#v", driver, provider.requests)
	}
	for _, schema := range provider.requests[1].Tools {
		if schema.Name == toolName {
			t.Fatalf("protected report tool remained advertised to ordinary retry: %#v", provider.requests[1].Tools)
		}
	}
	if result.ReleaseCandidateTerminal == nil {
		t.Fatal("ordinary terminal handoff is unavailable")
	}
	result.ReleaseCandidateTerminal()
	if releases != 2 {
		t.Fatalf("terminal claims were not released exactly once: %d", releases)
	}
}

func TestRuntimeRunnerRejectsProviderPreparationEffectChangeWithoutMixedOrdinaryWork(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-pure-native-unavailable", "turn-pure-native-unavailable", workspace, "pure-native-unavailable")
	provider := &providerStreamStub{}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
		return RuntimeProviderStep{Prompt: step.Prompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary}, nil
	}
	_, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "inspect current-case funds", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext, CaseFundPolicy: CaseFundAnalysisPolicy{Active: true},
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "inspect current-case funds"}},
	}, deps)
	if err == nil || !strings.Contains(err.Error(), "changed the authorized effect") || len(provider.requests) != 0 {
		t.Fatalf("pure protected provider step was downgraded: calls=%d err=%v", len(provider.requests), err)
	}
}

func TestRuntimeRunnerCombinesFundsAndAdditionalCaseDataEffects(t *testing.T) {
	for _, test := range []struct {
		name             string
		policy           CaseFundAnalysisPolicy
		additionalEffect bool
		wantEffect       domainsecurity.LogicalEffect
		wantOrdinary     bool
	}{
		{name: "zero value ordinary", wantEffect: domainsecurity.LogicalEffectOrdinary, wantOrdinary: true},
		{name: "funds case effect", policy: CaseFundAnalysisPolicy{Active: true}, wantEffect: domainsecurity.LogicalEffectFundsData},
		{name: "attachment case effect", additionalEffect: true, wantEffect: domainsecurity.LogicalEffectCaseData},
	} {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			securityContext, err := securitycontexttest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
				ThreadID:          "thread-effect-" + strings.ReplaceAll(test.name, " ", "-"),
				TurnID:            "turn-effect-" + strings.ReplaceAll(test.name, " ", "-"),
				WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Fatal(err)
			}
			provider := &providerStreamStub{responses: []providerStreamResponse{{
				callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "done"}},
			}}}
			deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
			catalogEffects := []domainsecurity.LogicalEffect{}
			attemptEffects := []domainsecurity.LogicalEffect{}
			deps.ResolveToolCatalog = func(_ bool, step RuntimeProviderStep) RuntimeRunnerToolCatalog {
				catalogEffects = append(catalogEffects, step.LogicalEffect)
				return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent"}
			}
			deps.AcquireProviderAttempt = func(ctx context.Context, _ int, step RuntimeProviderStep) (context.Context, func(), error) {
				attemptEffects = append(attemptEffects, step.LogicalEffect)
				return ctx, func() {}, nil
			}
			result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
				Prompt: "ordinary work", ProviderID: "provider", Model: "model",
				ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
				SecurityContext: securityContext, CaseFundPolicy: test.policy,
				AdditionalCaseDataEffect: test.additionalEffect,
				Messages: []domainmodel.Message{
					{Role: "system", Content: "system"},
					{Role: "user", Content: "ordinary work"},
				},
			}, deps)
			if err != nil || result.AssistantText != "done" || len(provider.requests) != 1 {
				t.Fatalf("provider effect run failed: calls=%d result=%#v err=%v", len(provider.requests), result, err)
			}
			telemetry := provider.requests[0].PrivateProviderTelemetry
			if telemetry == nil || telemetry.OrdinaryEffect != test.wantOrdinary {
				t.Fatalf("provider ordinary effect = %#v, want %t", telemetry, test.wantOrdinary)
			}
			if !reflect.DeepEqual(catalogEffects, []domainsecurity.LogicalEffect{test.wantEffect}) ||
				!reflect.DeepEqual(attemptEffects, []domainsecurity.LogicalEffect{test.wantEffect}) {
				t.Fatalf("exact provider effect was lost: catalog=%#v attempts=%#v want=%q", catalogEffects, attemptEffects, test.wantEffect)
			}
		})
	}
}

func TestRuntimeRunnerSteeringDrivesExactProviderStepOrder(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-effect-order", "turn-effect-order", workspace, "case-effect-order")
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary draft"}}},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "funds draft"}}},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary final"}}},
	}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	promotions := 0
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		switch promotions {
		case 2:
			return runtimeSteeringBatch("analyze the authorized funds snapshot", domainsecurity.LogicalEffectFundsData, true), nil
		case 4:
			return ordinaryRuntimeSteeringBatch("continue the ordinary code review"), nil
		default:
			return RuntimeSteeringBatch{}, nil
		}
	}
	preparedSteps := []RuntimeProviderStep{}
	catalogSteps := []RuntimeProviderStep{}
	attemptSteps := []RuntimeProviderStep{}
	deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
		preparedSteps = append(preparedSteps, step)
		return step, nil
	}
	deps.ResolveToolCatalog = func(_ bool, step RuntimeProviderStep) RuntimeRunnerToolCatalog {
		if len(preparedSteps) != len(catalogSteps)+1 || preparedSteps[len(preparedSteps)-1] != step {
			t.Fatalf("provider step was not prepared before catalog resolution: prepared=%#v catalog=%#v step=%#v", preparedSteps, catalogSteps, step)
		}
		catalogSteps = append(catalogSteps, step)
		return RuntimeRunnerToolCatalog{
			PromptRoute: "tool_agent", SystemPrompt: "system-for-" + string(step.LogicalEffect),
		}
	}
	deps.AcquireProviderAttempt = func(ctx context.Context, _ int, step RuntimeProviderStep) (context.Context, func(), error) {
		attemptSteps = append(attemptSteps, step)
		return ctx, func() {}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "review the code", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "initial system"},
			{Role: "user", Content: "review the code"},
		},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	want := []RuntimeProviderStep{
		{Prompt: "review the code", LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
		{Prompt: "analyze the authorized funds snapshot", LogicalEffect: domainsecurity.LogicalEffectFundsData, OrdinaryWork: true},
		{Prompt: "continue the ordinary code review", LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true},
	}
	if !reflect.DeepEqual(preparedSteps, want) || !reflect.DeepEqual(catalogSteps, want) || !reflect.DeepEqual(attemptSteps, want) {
		t.Fatalf("provider step order mismatch: prepared=%#v catalog=%#v attempts=%#v want=%#v", preparedSteps, catalogSteps, attemptSteps, want)
	}
	if len(provider.requests) != 3 || result.AssistantText != "ordinary final" {
		t.Fatalf("steered provider sequence mismatch: calls=%d result=%#v", len(provider.requests), result)
	}
	for index, effect := range []domainsecurity.LogicalEffect{
		domainsecurity.LogicalEffectOrdinary,
		domainsecurity.LogicalEffectFundsData,
		domainsecurity.LogicalEffectOrdinary,
	} {
		request := provider.requests[index]
		wantSystem := "system-for-" + string(effect)
		if request.SystemPrompt != wantSystem || len(request.Messages) == 0 || request.Messages[0].Content != wantSystem ||
			request.PrivateProviderTelemetry == nil || request.PrivateProviderTelemetry.OrdinaryEffect != (effect == domainsecurity.LogicalEffectOrdinary) {
			t.Fatalf("provider request %d lost exact effect/system state: %#v", index, request)
		}
	}
	result.ReleaseCandidateTerminal()
}

func TestInitialRuntimeProviderStepPreservesHostVerifiedContinuationEffect(t *testing.T) {
	tests := []struct {
		name  string
		input RuntimeRunnerInput
		want  RuntimeProviderStep
	}{
		{
			name: "signed funds effect is not downgraded by prompt classification",
			input: RuntimeRunnerInput{
				Prompt: "continue the pending ordinary edit approval",
				RestoredProviderStep: &RuntimeProviderStep{
					Prompt: "continue the pending ordinary edit approval", LogicalEffect: domainsecurity.LogicalEffectFundsData, OrdinaryWork: true,
				},
			},
			want: RuntimeProviderStep{
				Prompt: "continue the pending ordinary edit approval", LogicalEffect: domainsecurity.LogicalEffectFundsData, OrdinaryWork: true,
			},
		},
		{
			name: "signed ordinary effect is not upgraded by stale funds policy",
			input: RuntimeRunnerInput{
				Prompt:         "continue ordinary code work",
				CaseFundPolicy: CaseFundAnalysisPolicy{Active: true},
				RestoredProviderStep: &RuntimeProviderStep{
					Prompt: "continue ordinary code work", LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
				},
			},
			want: RuntimeProviderStep{
				Prompt: "continue ordinary code work", LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := initialRuntimeProviderStep(test.input); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("restored provider step mismatch: got=%#v want=%#v", got, test.want)
			}
		})
	}
}

func TestRuntimeRunnerDynamicFundsDowngradePauseRetainsSourceBoundaryAcrossResume(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-funds-downgrade-pause", "turn-funds-downgrade-pause", workspace, "case-funds-downgrade-pause")
	toolName := "write_file"
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("pending-ordinary-edit", toolName, `{}`),
	}}
	driver := &toolStepDriverStub{readOnly: map[string]bool{toolName: false}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = driver
	deps.PrepareProviderStep = func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
		if step.LogicalEffect == domainsecurity.LogicalEffectFundsData {
			step.LogicalEffect = domainsecurity.LogicalEffectOrdinary
			step.CaseSourceUnavailable = true
		}
		return step, nil
	}
	deps.ResolveToolCatalog = func(_ bool, step RuntimeProviderStep) RuntimeRunnerToolCatalog {
		if !step.OrdinaryEffect() {
			t.Fatalf("funds source downgrade did not occur before catalog resolution: %#v", step)
		}
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: zeroArgumentToolSchemas(toolName)}
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "analyze funds and then update the code", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "always", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		CaseFundPolicy:  CaseFundAnalysisPolicy{Active: true, OrdinaryWorkRequested: true},
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "analyze funds and then update the code"},
		},
	}, deps)
	if err != nil || !result.Paused || len(driver.pendings) != 1 {
		t.Fatalf("dynamic downgrade did not pause on the ordinary effect: result=%#v pendings=%#v err=%v", result, driver.pendings, err)
	}
	pending := driver.pendings[0]
	if pending.LogicalEffect != domainsecurity.LogicalEffectOrdinary || !pending.OrdinaryWork ||
		!pending.ProviderStepExact || !pending.CaseSourceUnavailable {
		t.Fatalf("downgraded pending continuation lost exact effect or source boundary: %#v", pending)
	}

	restoredStep := RuntimeProviderStep{
		Prompt: pending.Prompt, LogicalEffect: pending.LogicalEffect, OrdinaryWork: pending.OrdinaryWork,
		CaseSourceUnavailable: pending.CaseSourceUnavailable,
	}
	resumeProvider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary edit completed; funds source remains unavailable"}},
	}}}
	resumeResult, resumeErr := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: pending.Prompt, ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "always", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext:       securityContext,
		CaseFundPolicy:        CaseFundAnalysisPolicy{Active: true, OrdinaryWorkRequested: true},
		RestoredProviderStep:  &restoredStep,
		CaseSourceUnavailable: pending.CaseSourceUnavailable,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: pending.Prompt},
		},
	}, runtimeRunnerTestDependencies(resumeProvider, &runtimeLoopEventRecorderStub{}))
	if resumeErr != nil || resumeResult.AssistantText == "" || !resumeResult.CaseSourceUnavailable {
		t.Fatalf("resumed continuation lost sticky source boundary: result=%#v err=%v", resumeResult, resumeErr)
	}
}

func TestRuntimeRunnerOrdinarySteerRemovesProtectedToolPair(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopCaseContextV2(t, "thread-ordinary-projection", "turn-ordinary-projection", workspace, "case-ordinary-projection")
	protectedCall := domainmodel.ToolCall{
		ID: "funds-call", Name: "mcp__analytix_funds__analyze_account_flow", Arguments: []byte(`{"entity":"entity_ref"}`),
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "funds draft"}}},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary final"}}},
	}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	promotions := 0
	deps.PromoteSteering = func(context.Context) (RuntimeSteeringBatch, error) {
		promotions++
		if promotions == 2 {
			return ordinaryRuntimeSteeringBatch("continue without protected case data"), nil
		}
		return RuntimeSteeringBatch{}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "analyze funds", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext, CaseFundPolicy: CaseFundAnalysisPolicy{Active: true},
		Messages: []domainmodel.Message{
			{Role: "system", Content: "funds system"},
			{Role: "user", Content: "analyze funds"},
			{Role: "assistant", ToolCalls: []domainmodel.ToolCall{protectedCall}},
			{Role: "tool", ToolCallID: protectedCall.ID, Name: protectedCall.Name, Content: `{"amount":10}`},
		},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || result.AssistantText != "ordinary final" {
		t.Fatalf("ordinary projection sequence mismatch: calls=%d result=%#v", len(provider.requests), result)
	}
	if provider.requests[0].PrivateProviderTelemetry == nil || provider.requests[0].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("initial funds attempt was not protected: %#v", provider.requests[0].PrivateProviderTelemetry)
	}
	if provider.requests[1].PrivateProviderTelemetry == nil || !provider.requests[1].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("steered attempt was not ordinary: %#v", provider.requests[1].PrivateProviderTelemetry)
	}
	for _, message := range provider.requests[1].Messages {
		if message.ToolCallID == protectedCall.ID {
			t.Fatalf("protected tool result crossed ordinary step: %#v", provider.requests[1].Messages)
		}
		for _, call := range message.ToolCalls {
			if call.ID == protectedCall.ID {
				t.Fatalf("protected tool call crossed ordinary step: %#v", provider.requests[1].Messages)
			}
		}
	}
	result.ReleaseCandidateTerminal()
}

func TestRuntimeRunnerPersistsPreSendBeforeProviderTransport(t *testing.T) {
	events := &runtimeLoopEventRecorderStub{}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, events)
	provider := &pipelineStageProviderStub{afterPreSend: func() error {
		if len(events.events) == 0 || events.events[len(events.events)-1]["stage"] != "pre_send" {
			return errors.New("pre-send marker was not durable before provider transport")
		}
		return nil
	}}
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-batch", TurnID: "turn-provider-pipeline-batch",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "finish"},
		},
	}, deps)
	if err != nil || result.AssistantText != "batched provider response" {
		t.Fatalf("runtime provider pipeline failed: result=%#v err=%v", result, err)
	}
	if !provider.transported {
		t.Fatal("provider transport did not run")
	}
	if !reflect.DeepEqual(events.batchSizes, []int{9, 1}) {
		t.Fatalf("runtime pipeline batch boundaries changed: %#v", events.batchSizes)
	}
	stages := make([]string, 0, len(events.events))
	for _, event := range events.events {
		if stage, _ := event["stage"].(string); stage != "" {
			stages = append(stages, stage)
		}
	}
	if want := []string{
		"setup", "pre_start", "post_start", "input_received", "input_cached",
		"input_routed", "input_compressed", "input_remembered",
		"pre_send", "post_send", "response_received",
	}; !reflect.DeepEqual(stages, want) {
		t.Fatalf("runtime pipeline stage order changed: got=%#v want=%#v", stages, want)
	}
}

func TestRuntimeRunnerClassifiesContextHardLimitAsHostAdmissionFailure(t *testing.T) {
	events := &runtimeLoopEventRecorderStub{}
	provider := &providerStreamStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	_, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-context-admission", TurnID: "turn-context-admission",
		Prompt: "continue", ProviderID: "provider", Model: "small-model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", Model: "small-model", ContextWindowTokens: 1_000,
		},
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: strings.Repeat("bounded ordinary context ", 1_000)},
		},
	}, deps)
	if PublicFailureForError(err).Code() != "context_window_hard_limit" || len(provider.requests) != 0 {
		t.Fatalf("context admission did not stop before provider: calls=%d err=%v", len(provider.requests), err)
	}
	admissionEvents := 0
	for _, event := range events.events {
		if event["stage"] == "provider_error" {
			t.Fatalf("host context admission was misreported as provider transport failure: %#v", event)
		}
		if event["stage"] != "provider_admission_rejected" {
			continue
		}
		admissionEvents++
		details, _ := event["details"].(map[string]any)
		if event["label"] != "Provider admission rejected" ||
			details["reasonCode"] != "context_window_hard_limit" ||
			details["providerAttemptCount"] != float64(0) {
			t.Fatalf("host admission diagnostic is not closed: %#v", event)
		}
	}
	if admissionEvents != 1 {
		t.Fatalf("host context admission diagnostics=%d events=%#v", admissionEvents, events.events)
	}
}

func TestRuntimeRunnerPreSendPersistenceFailureBlocksProviderTransport(t *testing.T) {
	persistErr := errors.New("pre-send persistence failed")
	events := &runtimeLoopEventRecorderStub{batchErr: persistErr, batchErrAt: 1}
	provider := &pipelineStageProviderStub{}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, events)
	deps.Provider = provider
	_, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-failure", TurnID: "turn-provider-pipeline-failure",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "finish"},
		},
	}, deps)
	if !errors.Is(err, persistErr) {
		t.Fatalf("expected pre-send persistence failure, got %v", err)
	}
	if provider.transported {
		t.Fatal("provider transport ran after pre-send persistence failed")
	}
}

func TestRuntimeRunnerRequiresDurableProviderPipelineContract(t *testing.T) {
	provider := &providerWithoutDurablePipelineContractStub{}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	_, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-contract", TurnID: "turn-provider-pipeline-contract",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	if err == nil || !strings.Contains(err.Error(), "durable pipeline contract") || provider.calls != 0 {
		t.Fatalf("non-durable provider reached the Agent loop: calls=%d err=%v", provider.calls, err)
	}
}

func TestRuntimeRunnerRejectsProviderSuccessWithoutDurableSendPair(t *testing.T) {
	provider := &pipelineStageProviderStub{omitAll: true}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-missing", TurnID: "turn-provider-pipeline-missing",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		result.AssistantText != "" || provider.calls != 1 {
		t.Fatalf("provider success bypassed durable send pair: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerRejectsProviderTerminalErrorWithoutDurableDisposition(t *testing.T) {
	providerErr := errors.New("provider terminal failure")
	provider := &pipelineStageProviderStub{omitAll: true, returnErr: providerErr}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-terminal-error", TurnID: "turn-provider-pipeline-terminal-error",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.Is(err, providerErr) || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		result.AssistantText != "" || provider.calls != 1 {
		t.Fatalf("provider terminal error bypassed durable disposition: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerSealsPartialOutputWithoutDurableDisposition(t *testing.T) {
	providerErr := errors.New("provider failed after unbound output")
	provider := &pipelineStageProviderStub{omitAll: true, omitAllText: true, returnErr: providerErr}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-unbound-output", TurnID: "turn-provider-pipeline-unbound-output",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		result.AssistantText != "" || len(result.LastResult.Chunks) != 0 || provider.calls != 1 {
		t.Fatalf("unbound partial provider output crossed the failure edge: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerCancellationCannotMaskMissingDurableDisposition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	provider := &pipelineStageProviderStub{omitAll: true, cancel: cancel}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(ctx, RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-cancel-mask", TurnID: "turn-provider-pipeline-cancel-mask",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.Is(err, context.Canceled) || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		result.AssistantText != "" || len(result.LastResult.Chunks) != 0 || provider.calls != 1 {
		t.Fatalf("cancellation masked missing durable disposition: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerCancelledAttemptNeverReachesProviderInvocation(t *testing.T) {
	provider := &pipelineStageProviderStub{}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	var cancelAttempt context.CancelFunc
	deps.AcquireProviderAttempt = func(ctx context.Context, _ int, _ RuntimeProviderStep) (context.Context, func(), error) {
		attemptCtx, cancel := context.WithCancel(ctx)
		cancelAttempt = cancel
		return attemptCtx, func() {}, nil
	}
	settled := false
	deps.PrepareProviderAttempt = func(
		_ context.Context,
		_ int,
		request domainmodel.Request,
		_, _ string,
		_ uint64,
	) (domainmodel.Request, ProviderAttemptSettlement, error) {
		cancelAttempt()
		return request, func(_ context.Context, status, reason string) error {
			settled = status == "cancelled" && reason == "provider_attempt_cancelled"
			return nil
		}, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-attempt-cancel-before-invocation", TurnID: "turn-provider-attempt-cancel-before-invocation",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		HasAttachments: true, AttachmentPlanDigest: domainsecurity.SHA256Hex([]byte("attachment-plan")),
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.Is(err, context.Canceled) || errors.As(err, &forbidden) ||
		result.AssistantText != "" || provider.calls != 0 || !settled {
		t.Fatalf("cancelled effect reached provider invocation: calls=%d settled=%t result=%#v err=%v", provider.calls, settled, result, err)
	}
}

func TestRuntimeRunnerSettlementFailureCannotMaskMissingDurableDisposition(t *testing.T) {
	settleErr := errors.New("attachment settlement failed")
	provider := &pipelineStageProviderStub{omitAll: true}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	deps.PrepareProviderAttempt = func(
		_ context.Context,
		_ int,
		request domainmodel.Request,
		_, _ string,
		_ uint64,
	) (domainmodel.Request, ProviderAttemptSettlement, error) {
		return request, func(context.Context, string, string) error { return settleErr }, nil
	}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-settle-mask", TurnID: "turn-provider-pipeline-settle-mask",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		HasAttachments: true, AttachmentPlanDigest: domainsecurity.SHA256Hex([]byte("attachment-plan")),
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.Is(err, settleErr) || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		result.AssistantText != "" || len(result.LastResult.Chunks) != 0 || provider.calls != 1 {
		t.Fatalf("settlement failure masked missing durable disposition: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerAllowsTrustedNotSentProviderFailure(t *testing.T) {
	providerErr := errors.New("provider request rejected before transport")
	provider := &pipelineStageProviderStub{
		omitAll: true, returnErr: providerErr, dispatchState: domaincache.ProviderDispatchStateNotSent,
	}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-not-sent", TurnID: "turn-provider-pipeline-not-sent",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.Is(err, providerErr) || errors.As(err, &forbidden) ||
		result.AssistantText != "" || provider.calls != 1 {
		t.Fatalf("trusted not-sent provider failure was misclassified: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerRetriesTrustedNotSentProviderFailure(t *testing.T) {
	provider := &pipelineStageProviderStub{notSentFirst: true}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-not-sent-retry", TurnID: "turn-provider-pipeline-not-sent-retry",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	if err != nil || result.AssistantText != "batched provider response" || provider.calls != 2 {
		t.Fatalf("trusted not-sent retry did not reach a durable attempt: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerDoesNotRetryAfterProviderOmitsPostSend(t *testing.T) {
	provider := &pipelineStageProviderStub{omitPost: true, returnErr: fakeProviderRetryError{retryable: true}}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-open", TurnID: "turn-provider-pipeline-open",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		result.AssistantText != "" || provider.calls != 1 {
		t.Fatalf("open provider send pair retried or published: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerRecordsSanitizedProviderFailureWhenCallbackContractAlsoFails(t *testing.T) {
	events := &runtimeLoopEventRecorderStub{}
	provider := &pipelineStageProviderStub{omitPost: true, returnErr: maliciousProviderDiagnosticError{}}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, events)
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-callback-diagnostic", TurnID: "turn-provider-callback-diagnostic",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var callbackErr ProviderStreamCallbackError
	if err == nil || !errors.As(err, &callbackErr) ||
		PublicFailureForError(err).Code() != domainfailure.CodeProviderAuthenticationFailed ||
		result.AssistantText != "" || provider.calls != 1 {
		t.Fatalf("provider callback failure classification changed: calls=%d result=%#v err=%v", provider.calls, result, err)
	}

	providerFailureEvents := make([]map[string]any, 0, 1)
	for _, event := range events.events {
		if event["kind"] == "pipeline_stage" && event["stage"] == "provider_error" {
			providerFailureEvents = append(providerFailureEvents, event)
		}
	}
	if len(providerFailureEvents) != 1 {
		t.Fatalf("expected one durable provider failure diagnostic, got %#v", providerFailureEvents)
	}
	details, _ := providerFailureEvents[0]["details"].(map[string]any)
	providerError, _ := details["providerError"].(map[string]any)
	if details["reasonCode"] != domainfailure.CodeProviderAuthenticationFailed ||
		details["message"] != domainfailure.New(domainfailure.CodeProviderAuthenticationFailed, nil).Message() ||
		providerError["status"] != float64(401) || providerError["kind"] != "auth" {
		t.Fatalf("provider callback diagnostic was not closed and useful: %#v", providerFailureEvents[0])
	}
	serialized, serializeErr := json.Marshal(providerFailureEvents[0])
	if serializeErr != nil {
		t.Fatal(serializeErr)
	}
	for _, forbidden := range []string{"SOL_PRIVATE_TRACE_7C", "thinking_family", "6222021234567890", "requestUrl", "reasoningPayload"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("provider callback diagnostic leaked %q: %s", forbidden, serialized)
		}
	}
}

func TestRuntimeRunnerFlushesProviderSendStagesBeforeRetryEvent(t *testing.T) {
	events := &runtimeLoopEventRecorderStub{}
	provider := &retryingPipelineStageProviderStub{}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, events)
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-retry", TurnID: "turn-provider-pipeline-retry",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "finish"},
		},
	}, deps)
	if err != nil || result.AssistantText != "retry completed" || provider.calls != 2 {
		t.Fatalf("runtime provider retry pipeline failed: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
	if !reflect.DeepEqual(events.batchSizes, []int{9, 1, 1, 1}) {
		t.Fatalf("runtime retry pipeline batch boundaries changed: %#v", events.batchSizes)
	}
	stages := make([]string, 0, len(events.events))
	for _, event := range events.events {
		if stage, _ := event["stage"].(string); stage != "" {
			stages = append(stages, stage)
		}
	}
	if want := []string{
		"setup", "pre_start", "post_start", "input_received", "input_cached",
		"input_routed", "input_compressed", "input_remembered",
		"pre_send", "post_send", "provider_retrying",
		"pre_send", "post_send", "response_received",
	}; !reflect.DeepEqual(stages, want) {
		t.Fatalf("runtime retry pipeline stage order changed: got=%#v want=%#v", stages, want)
	}
}

func TestRuntimeRunnerRejectsRetrySuccessWithoutItsOwnDurableSendPair(t *testing.T) {
	provider := &retryingPipelineStageProviderStub{omitSecond: true}
	deps := runtimeRunnerTestDependencies(&providerStreamStub{}, &runtimeLoopEventRecorderStub{})
	deps.Provider = provider
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-provider-pipeline-retry-missing", TurnID: "turn-provider-pipeline-retry-missing",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "finish"}},
	}, deps)
	var forbidden interface{ ProviderRetryForbidden() bool }
	if err == nil || !errors.As(err, &forbidden) || !forbidden.ProviderRetryForbidden() ||
		result.AssistantText != "" || provider.calls != 2 {
		t.Fatalf("retry success reused an earlier send pair: calls=%d result=%#v err=%v", provider.calls, result, err)
	}
}

func TestRuntimeRunnerPersistsStartupStagesBeforePreProviderFailure(t *testing.T) {
	events := &runtimeLoopEventRecorderStub{}
	provider := &providerStreamStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	pauseErr := errors.New("pause failed")
	deps.PauseChild = func(context.Context) error { return pauseErr }
	_, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-startup-failure", TurnID: "turn-startup-failure",
		Prompt: "finish", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "on-request", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "finish"},
		},
	}, deps)
	if !errors.Is(err, pauseErr) || len(provider.requests) != 0 {
		t.Fatalf("pre-provider failure changed: provider calls=%d err=%v", len(provider.requests), err)
	}
	if !reflect.DeepEqual(events.batchSizes, []int{5}) {
		t.Fatalf("startup stages were not durably flushed before failure: %#v", events.batchSizes)
	}
	stages := make([]string, 0, len(events.events))
	for _, event := range events.events {
		if stage, _ := event["stage"].(string); stage != "" {
			stages = append(stages, stage)
		}
	}
	if want := []string{"setup", "pre_start", "post_start", "input_received", "input_cached"}; !reflect.DeepEqual(stages, want) {
		t.Fatalf("pre-provider failure startup stages changed: got=%#v want=%#v", stages, want)
	}
}

func TestForegroundChildFreeTextCannotBecomeHandoffResult(t *testing.T) {
	workspace := t.TempDir()
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "foreground-child", TurnID: "foreground-turn", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "untrusted assistant result"}},
	}}}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "bounded child prompt", ProviderID: "provider", Model: "model",
		ApprovalPolicy: "never", SandboxMode: "read-only", EffectiveMaxModelSteps: 1,
		RequiredFinalToolName: "submit_child_result", OutputTokenBudget: 128, SecurityContext: securityContext,
		Messages: []domainmodel.Message{{Role: "system", Content: "system"}, {Role: "user", Content: "bounded child prompt"}},
	}, runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{}))
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "subagent_required_final_tool_missing" || result.AssistantText != "" {
		t.Fatalf("assistant free text became a foreground result: result=%#v err=%v", result, err)
	}
	if len(provider.requests) != 1 || provider.requests[0].MaxOutputTokens != 128 {
		t.Fatalf("foreground provider request lost its host token budget: %#v", provider.requests)
	}
}

func TestRuntimeRunnerNonPositiveStepLimitFallsBackToBoundedDefault(t *testing.T) {
	provider := &providerStreamStub{responses: []providerStreamResponse{{
		callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "bounded final"}},
	}}}
	events := &runtimeLoopEventRecorderStub{}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-bounded-default", TurnID: "turn-bounded-default", Prompt: "finish",
		ProviderID: "provider", Model: "model", ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		EffectiveMaxModelSteps: 0,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "finish"},
		},
	}, runtimeRunnerTestDependencies(provider, events))
	if err != nil || result.AssistantText != "bounded final" || len(provider.requests) != 1 {
		t.Fatalf("non-positive limit did not use bounded default: calls=%d result=%#v err=%v", len(provider.requests), result, err)
	}
	for _, event := range events.events {
		if event["stage"] != "post_start" {
			continue
		}
		details, _ := event["details"].(map[string]any)
		if got, _ := details["maxModelSteps"].(float64); got != float64(DefaultModelStepLimit) {
			t.Fatalf("post_start advertised non-bounded limit: %#v", event)
		}
		return
	}
	t.Fatal("post_start event was not recorded")
}

func TestCaseRuntimeStreamInterruptionDoesNotRequestModelRecovery(t *testing.T) {
	workspace := t.TempDir()
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "unsupported case amount 9,999,999"}},
			err:            fakeProviderRetryError{retryable: true},
		},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "replacement must never be requested"}}},
	}}
	events := &runtimeLoopEventRecorderStub{}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-case-stream", TurnID: "turn-case-stream", Workspace: workspace, Prompt: "case task",
		ProviderID: "case-provider", Model: "case-model", ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		EffectiveMaxModelSteps: 1,
		SecurityContext:        newLoopCaseContextV2(t, "thread-case-stream", "turn-case-stream", workspace, "case-stream"),
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "case task"},
		},
	}, runtimeRunnerTestDependencies(provider, events))
	if err == nil || len(provider.requests) != 1 || result.AssistantText != "" || result.TerminalRecoveryKind != RuntimeTerminalRecoveryNone {
		t.Fatalf("case stream interruption requested recovery or retained a candidate: calls=%d result=%#v err=%v", len(provider.requests), result, err)
	}
	for _, event := range events.events {
		if event["stage"] == "provider_retrying" {
			t.Fatalf("case stream interruption emitted a recovery stage: %#v", event)
		}
	}
	for _, request := range provider.requests {
		for _, message := range request.Messages {
			if strings.Contains(message.Content, "complete, self-contained replacement") || strings.Contains(message.Content, "previous assistant response") {
				t.Fatalf("case stream interruption sent a recovery prompt: %#v", request.Messages)
			}
		}
	}
}

func TestCaseRuntimeEmptyFinalDoesNotRequestModelRecovery(t *testing.T) {
	workspace := t.TempDir()
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "replacement must never be requested"}}},
	}}
	events := &runtimeLoopEventRecorderStub{}
	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: "thread-case-empty", TurnID: "turn-case-empty", Workspace: workspace, Prompt: "case task",
		ProviderID: "case-provider", Model: "case-model", ApprovalPolicy: "on-request", SandboxMode: "workspace-write",
		EffectiveMaxModelSteps: 1,
		SecurityContext:        newLoopCaseContextV2(t, "thread-case-empty", "turn-case-empty", workspace, "case-empty"),
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "case task"},
		},
	}, runtimeRunnerTestDependencies(provider, events))
	if err == nil || PublicFailureForError(err).Code() != domainfailure.CodeProviderEmptyFinal || len(provider.requests) != 1 ||
		result.AssistantText != "" || result.TerminalRecoveryKind != RuntimeTerminalRecoveryNone {
		t.Fatalf("case empty final requested recovery or retained a candidate: calls=%d result=%#v err=%v", len(provider.requests), result, err)
	}
	for _, event := range events.events {
		if event["stage"] == "empty_final_recovering" || event["stage"] == "provider_retrying" {
			t.Fatalf("case empty final emitted a recovery stage: %#v", event)
		}
	}
}

func runtimeRunnerTestDependencies(provider *providerStreamStub, events *runtimeLoopEventRecorderStub) RuntimeRunnerDependencies {
	return RuntimeRunnerDependencies{
		Provider: provider,
		Events:   NewRuntimeEventRecorder(events),
		ResolveToolCatalog: func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
			return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent"}
		},
		PromoteSteering: func(context.Context) (RuntimeSteeringBatch, error) { return RuntimeSteeringBatch{}, nil },
		AcquireCandidateTerminal: func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		PrepareProviderStep: func(_ context.Context, step RuntimeProviderStep) (RuntimeProviderStep, error) {
			return step, nil
		},
		PauseChild: func(context.Context) error { return nil },
		AcquireProviderAttempt: func(ctx context.Context, _ int, _ RuntimeProviderStep) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
		ToolDriver: ToolStepDriverFuncs{},
	}
}

func ordinaryRuntimeSteeringBatch(prompt string) RuntimeSteeringBatch {
	return runtimeSteeringBatch(prompt, domainsecurity.LogicalEffectOrdinary, true)
}

func runtimeSteeringBatch(
	prompt string,
	logicalEffect domainsecurity.LogicalEffect,
	ordinaryWork bool,
) RuntimeSteeringBatch {
	return RuntimeSteeringBatch{
		Messages: []domainmodel.Message{{Role: "user", Content: prompt}}, Prompt: prompt,
		LogicalEffect: logicalEffect, OrdinaryWork: ordinaryWork,
	}
}
