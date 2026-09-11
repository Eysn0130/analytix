package server

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	controlapp "analytix.local/runtime-go/internal/app/control"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	provider "analytix.local/runtime-go/internal/provider"
)

type boundedProviderExecutionResolverStub struct {
	calls int
}

type stepCurrentProviderExecutionResolverStub struct {
	calls       int
	intentCalls int
	configs     []provider.TurnConfig
}

type postResponseCurrentnessResolverStub struct {
	config      provider.TurnConfig
	drifted     bool
	validations int
}

type partialProviderExecutionResolverStub struct {
	config provider.TurnConfig
}

type preparedAuthorityDriftProviderStub struct {
	resolver       *postResponseCurrentnessResolverStub
	transportCalls int
}

func (*preparedAuthorityDriftProviderStub) RequiresDurablePipelineStagesV1() {}

func (providerClient *preparedAuthorityDriftProviderStub) Stream(
	_ context.Context,
	request domainmodel.Request,
) (domainmodel.Result, error) {
	providerClient.resolver.drifted = true
	currentnessField := reflect.ValueOf(&request).Elem().FieldByName("PrivateProviderCurrentnessBeforeSend")
	if currentnessField.IsValid() && !currentnessField.IsNil() {
		callback, ok := currentnessField.Interface().(func(int) error)
		if !ok {
			return domainmodel.Result{}, errors.New("provider pre-send currentness callback is invalid")
		}
		if err := callback(1); err != nil {
			return domainmodel.Result{}, err
		}
	}
	providerClient.transportCalls++
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	if request.OnChunk != nil {
		if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "prepared candidate"}); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{StreamCompleted: true}, nil
}

type handlerProviderExecutionResolverForTest struct {
	handler *runtimeServerHandler
}

func (resolver handlerProviderExecutionResolverForTest) ResolveTurnExecution(
	_ context.Context,
	input provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	return resolver.handler.providerConfig.ResolveTurnExecution(input)
}

func (resolver handlerProviderExecutionResolverForTest) ResolveTurnIntent(
	_ context.Context,
	input provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	result, err := resolver.handler.providerConfig.ResolveTurnExecution(input)
	if err != nil {
		return provider.TurnExecutionResult{}, err
	}
	result.Config.APIKey = ""
	return result, nil
}

func installHandlerProviderExecutionResolverForTest(handler *runtimeServerHandler) {
	handler.providerExecution = handlerProviderExecutionResolverForTest{handler: handler}
}

func (resolver handlerProviderExecutionResolverForTest) ValidateTurnExecutionCurrent(
	context.Context,
	provider.TurnExecutionAuthority,
) error {
	return nil
}

func (resolver *boundedProviderExecutionResolverStub) ResolveTurnExecution(
	_ context.Context,
	_ provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	resolver.calls++
	credential := "synthetic-bounded-first"
	if resolver.calls > 1 {
		credential = "synthetic-bounded-second"
	}
	return provider.TurnExecutionResult{
		ProviderID: "provider-bounded", Model: "model-bounded",
		Config: provider.TurnConfig{
			ProviderID: "provider-bounded", Model: "model-bounded",
			BaseURL: "https://provider.invalid/v1", APIKey: credential,
		},
	}, nil
}

func (resolver *stepCurrentProviderExecutionResolverStub) ResolveTurnExecution(
	_ context.Context,
	_ provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	resolver.calls++
	index := resolver.calls - 1
	if index >= len(resolver.configs) {
		index = len(resolver.configs) - 1
	}
	config := resolver.configs[index]
	return provider.TurnExecutionResult{
		ProviderID: config.ProviderID,
		Model:      config.Model,
		Effort:     config.ReasoningEffort,
		Config:     config,
	}, nil
}

func (resolver *stepCurrentProviderExecutionResolverStub) ResolveTurnIntent(
	_ context.Context,
	_ provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	resolver.intentCalls++
	index := resolver.calls
	if index >= len(resolver.configs) {
		index = len(resolver.configs) - 1
	}
	config := resolver.configs[index]
	config.APIKey = ""
	return provider.TurnExecutionResult{
		ProviderID: config.ProviderID,
		Model:      config.Model,
		Effort:     config.ReasoningEffort,
		Config:     config,
	}, nil
}

func (resolver *stepCurrentProviderExecutionResolverStub) ValidateTurnExecutionCurrent(
	context.Context,
	provider.TurnExecutionAuthority,
) error {
	return nil
}

func (resolver *partialProviderExecutionResolverStub) ResolveTurnIntent(
	_ context.Context,
	_ provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	config := resolver.config
	config.APIKey = ""
	return provider.TurnExecutionResult{
		ProviderID: config.ProviderID, Model: config.Model, Effort: config.ReasoningEffort, Config: config,
	}, nil
}

func (resolver *partialProviderExecutionResolverStub) ResolveTurnExecution(
	_ context.Context,
	_ provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	return provider.TurnExecutionResult{
		ProviderID: resolver.config.ProviderID, Model: resolver.config.Model,
		Effort: resolver.config.ReasoningEffort, Config: resolver.config,
	}, nil
}

func (resolver *postResponseCurrentnessResolverStub) ResolveTurnIntent(
	_ context.Context,
	_ provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	config := resolver.config
	config.APIKey = ""
	return provider.TurnExecutionResult{
		ProviderID: config.ProviderID, Model: config.Model, Effort: config.ReasoningEffort, Config: config,
	}, nil
}

func (resolver *postResponseCurrentnessResolverStub) ResolveTurnExecution(
	_ context.Context,
	_ provider.TurnExecutionInput,
) (provider.TurnExecutionResult, error) {
	return provider.TurnExecutionResult{
		ProviderID: resolver.config.ProviderID, Model: resolver.config.Model,
		Effort: resolver.config.ReasoningEffort, Config: resolver.config,
		Authority: provider.TurnExecutionAuthority{
			RegistryRevision: 1, RegistryIncarnation: "registry-currentness",
			ProviderID: resolver.config.ProviderID, ProviderRevision: 1, ProviderGeneration: 1,
			ProviderIncarnation: "provider-currentness", ProviderCredentialRef: "opaque-currentness-ref",
			ProviderCredentialPurpose: "provider-api-key",
		},
	}, nil
}

func (resolver *postResponseCurrentnessResolverStub) ValidateTurnExecutionCurrent(
	_ context.Context,
	_ provider.TurnExecutionAuthority,
) error {
	resolver.validations++
	if resolver.drifted {
		return errors.New("provider execution authority changed")
	}
	return nil
}

func TestRuntimeTurnExecutionUsesCurrentBoundedResolverEveryTime(t *testing.T) {
	t.Parallel()

	resolver := &boundedProviderExecutionResolverStub{}
	handler := &runtimeServerHandler{providerExecution: resolver}
	first, err := handler.resolveRuntimeTurnExecution(context.Background(), provider.TurnExecutionInput{})
	if err != nil {
		t.Fatalf("resolveRuntimeTurnExecution(first) error = %v", err)
	}
	second, err := handler.resolveRuntimeTurnExecution(context.Background(), provider.TurnExecutionInput{})
	if err != nil {
		t.Fatalf("resolveRuntimeTurnExecution(second) error = %v", err)
	}
	if resolver.calls != 2 || first.Config.APIKey == second.Config.APIKey ||
		second.Config.APIKey != "synthetic-bounded-second" {
		t.Fatal("runtime turn execution cached a stale bounded Provider resolution")
	}
}

func TestRuntimeTurnExecutionFailsClosedWithoutResolver(t *testing.T) {
	t.Parallel()

	handler := &runtimeServerHandler{
		providerConfig: provider.NewRuntimeProviderConfigSet(provider.RuntimeProviderConfigInput{
			DefaultProviderID: "provider-static", DefaultBaseURL: "https://provider.invalid/v1",
			DefaultAPIKey: "synthetic-static-credential", DefaultModel: "model-static",
			DefaultEndpointFormat: "chat_completions",
		}),
	}
	if _, err := handler.resolveRuntimeTurnExecution(
		context.Background(), provider.TurnExecutionInput{},
	); err == nil {
		t.Fatal("runtime turn execution accepted static credentials without the Registry authority")
	}
}

func TestRuntimeProviderStepsReResolveCurrentAuthorityBeforeEveryEffect(t *testing.T) {
	fixture := newProviderStepIntegrationFixture(t, "provider-authority-reresolve")
	firstConfig := fixture.input.ProviderConfig
	firstConfig.APIKey = "synthetic-currentness-first"
	firstConfig.BaseURL = "https://provider-first.invalid/v1"
	firstConfig.ProxyURL = "http://proxy-first.invalid"
	secondConfig := firstConfig
	secondConfig.APIKey = "synthetic-currentness-second"
	secondConfig.BaseURL = "https://provider-second.invalid/v1"
	secondConfig.ProxyURL = "http://proxy-second.invalid"
	resolver := &stepCurrentProviderExecutionResolverStub{configs: []provider.TurnConfig{firstConfig, secondConfig}}
	fixture.handler.providerExecution = resolver

	recordingProvider := &providerStepRecordingProvider{}
	recordingProvider.onCall = func(call int, _ domainmodel.Request) error {
		if call != 1 {
			return nil
		}
		result, err := fixture.handler.steerRuntimeTurn(context.Background(), controlapp.SteerTurnRequest{
			ThreadID: fixture.input.ThreadID, TurnID: fixture.input.TurnID,
			ExpectedTurnID:      fixture.input.TurnID,
			ClientUserMessageID: "018f47a0-13d2-4e9c-af51-4ae4f62e8b14",
			Text:                "继续执行同一普通任务的下一步。",
		})
		if err != nil {
			return err
		}
		if result.StatusCode != 200 || result.Body["ok"] != true {
			return fmt.Errorf("steer admission failed")
		}
		return nil
	}
	fixture.handler.provider = recordingProvider

	loopContext, cancelLoop := context.WithCancel(context.Background())
	defer cancelLoop()
	if !fixture.handler.runtimeControl().RegisterTurnCancel(
		fixture.input.ThreadID,
		fixture.input.TurnID,
		cancelLoop,
	) {
		t.Fatal("runtime loop ownership registration failed")
	}
	defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)
	result, err := fixture.handler.runRuntimeAgentLoopWithMessages(
		loopContext,
		fixture.input,
		[]provider.Message{
			{Role: "system", Content: "provider currentness integration"},
			{Role: "user", Content: "执行普通任务第一步。"},
		},
	)
	if err != nil {
		t.Fatalf("run provider currentness sequence: %v", err)
	}
	if result.ReleaseCandidateTerminal == nil {
		t.Fatal("provider currentness sequence did not return a candidate terminal handoff")
	}
	result.ReleaseCandidateTerminal()

	requests := recordingProvider.Requests()
	if resolver.intentCalls != 2 || resolver.calls != 2 || len(requests) != 2 {
		t.Fatalf("provider currentness intents=%d resolutions=%d requests=%d want=2/2/2", resolver.intentCalls, resolver.calls, len(requests))
	}
	if requests[0].APIKey != firstConfig.APIKey || requests[0].BaseURL != firstConfig.BaseURL ||
		requests[0].ProxyURL != firstConfig.ProxyURL {
		t.Fatal("first provider effect did not use the first current authority")
	}
	if requests[1].APIKey != secondConfig.APIKey || requests[1].BaseURL != secondConfig.BaseURL ||
		requests[1].ProxyURL != secondConfig.ProxyURL || requests[1].ProxyURL == requests[0].ProxyURL {
		t.Fatal("second provider effect reused stale authority")
	}
}

func TestRuntimeProviderEffectDiscardsResponseAfterAuthorityDrift(t *testing.T) {
	fixture := newProviderStepIntegrationFixture(t, "provider-authority-post-response")
	config := fixture.input.ProviderConfig
	config.APIKey = "synthetic-post-response-currentness"
	resolver := &postResponseCurrentnessResolverStub{config: config}
	fixture.handler.providerExecution = resolver
	recordingProvider := &providerStepRecordingProvider{
		onCall: func(_ int, _ domainmodel.Request) error {
			resolver.drifted = true
			return nil
		},
	}
	fixture.handler.provider = recordingProvider

	loopContext, cancelLoop := context.WithCancel(context.Background())
	defer cancelLoop()
	if !fixture.handler.runtimeControl().RegisterTurnCancel(
		fixture.input.ThreadID,
		fixture.input.TurnID,
		cancelLoop,
	) {
		t.Fatal("runtime loop ownership registration failed")
	}
	defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)
	result, err := fixture.handler.runRuntimeAgentLoopWithMessages(
		loopContext,
		fixture.input,
		[]provider.Message{
			{Role: "system", Content: "provider currentness integration"},
			{Role: "user", Content: "执行普通任务。"},
		},
	)
	if err == nil {
		t.Fatal("provider response survived post-response authority drift")
	}
	if resolver.validations != 1 || len(recordingProvider.Requests()) != 1 {
		t.Fatalf("post-response validations=%d requests=%d want=1/1", resolver.validations, len(recordingProvider.Requests()))
	}
	if result.AssistantText != "" || len(result.LastResult.Chunks) != 0 {
		t.Fatal("drifted provider response crossed the public or durable result boundary")
	}
}

func TestRuntimeProviderEffectRevalidatesAfterPreparationBeforePhysicalSend(t *testing.T) {
	fixture := newProviderStepIntegrationFixture(t, "provider-authority-pre-send")
	config := fixture.input.ProviderConfig
	config.APIKey = "synthetic-pre-send-currentness"
	resolver := &postResponseCurrentnessResolverStub{config: config}
	fixture.handler.providerExecution = resolver
	providerClient := &preparedAuthorityDriftProviderStub{resolver: resolver}
	fixture.handler.provider = providerClient

	loopContext, cancelLoop := context.WithCancel(context.Background())
	defer cancelLoop()
	if !fixture.handler.runtimeControl().RegisterTurnCancel(
		fixture.input.ThreadID,
		fixture.input.TurnID,
		cancelLoop,
	) {
		t.Fatal("runtime loop ownership registration failed")
	}
	defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)
	result, err := fixture.handler.runRuntimeAgentLoopWithMessages(
		loopContext,
		fixture.input,
		[]provider.Message{
			{Role: "system", Content: "provider currentness integration"},
			{Role: "user", Content: "执行普通任务。"},
		},
	)
	if err == nil {
		if result.ReleaseCandidateTerminal != nil {
			result.ReleaseCandidateTerminal()
		}
		t.Fatal("drifted prepared Provider authority survived before physical send")
	}
	if providerClient.transportCalls != 0 {
		t.Fatalf("stale prepared Provider authority reached transport: calls=%d", providerClient.transportCalls)
	}
	if resolver.validations == 0 || result.AssistantText != "" || len(result.LastResult.Chunks) != 0 {
		t.Fatal("pre-send authority drift crossed the public or durable result boundary")
	}
}

func TestRuntimeProviderEffectFailsClosedWithoutCurrentnessValidator(t *testing.T) {
	fixture := newProviderStepIntegrationFixture(t, "provider-authority-partial-resolver")
	config := fixture.input.ProviderConfig
	config.APIKey = "synthetic-partial-resolver-credential"
	resolver := &partialProviderExecutionResolverStub{config: config}
	fixture.handler.providerExecution = resolver
	recordingProvider := &providerStepRecordingProvider{}
	fixture.handler.provider = recordingProvider

	loopContext, cancelLoop := context.WithCancel(context.Background())
	defer cancelLoop()
	if !fixture.handler.runtimeControl().RegisterTurnCancel(
		fixture.input.ThreadID,
		fixture.input.TurnID,
		cancelLoop,
	) {
		t.Fatal("runtime loop ownership registration failed")
	}
	defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)
	result, err := fixture.handler.runRuntimeAgentLoopWithMessages(
		loopContext,
		fixture.input,
		[]provider.Message{
			{Role: "system", Content: "partial Provider resolver must fail closed"},
			{Role: "user", Content: "执行普通任务。"},
		},
	)
	if err == nil {
		if result.ReleaseCandidateTerminal != nil {
			result.ReleaseCandidateTerminal()
		}
		t.Fatal("late-bound Provider output survived without a currentness validator")
	}
	if len(recordingProvider.Requests()) != 0 || result.AssistantText != "" || len(result.LastResult.Chunks) != 0 {
		t.Fatal("partial Provider resolver crossed the physical or public result boundary")
	}
}
