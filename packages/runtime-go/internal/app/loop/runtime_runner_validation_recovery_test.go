package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

func TestRuntimeRunnerRetriesPrePersistenceToolArgumentValidation(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-tool-args-recovery", "turn-tool-args-recovery", workspace)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("invalid-read", "read", `{}`),
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary task completed"}}},
	}}
	driver := &toolStepDriverStub{readOnly: map[string]bool{"read": true}}
	events := &runtimeLoopEventRecorderStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{requiredPathToolSchema("read")}}
	}

	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Read the repository file and finish the task.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "Read the repository file and finish the task."},
		},
	}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.AssistantText != "ordinary task completed" || len(provider.requests) != 2 ||
		len(driver.ready) != 0 || len(driver.executed) != 0 {
		t.Fatalf("schema recovery did not converge to a final answer: calls=%d ready=%#v executed=%#v result=%#v",
			len(provider.requests), driver.ready, driver.executed, result)
	}
	for index, request := range provider.requests {
		if request.PrivateProviderTelemetry == nil || request.PrivateProviderTelemetry.LogicalSequence != uint64(index+1) {
			t.Fatalf("provider sequence %d was not monotonic: %#v", index+1, request.PrivateProviderTelemetry)
		}
	}
	secondMessages := provider.requests[1].Messages
	if len(secondMessages) < 1 || secondMessages[len(secondMessages)-1].Role != "user" ||
		secondMessages[len(secondMessages)-1].Content != providerToolArgumentValidationRecoveryPrompt {
		t.Fatalf("safe correction prompt is unavailable: %#v", secondMessages)
	}
	for _, message := range secondMessages {
		if len(message.ToolCalls) > 0 || message.Role == "tool" || strings.Contains(message.Content, "invalid-read") {
			t.Fatalf("invalid tool draft crossed the recovery boundary: %#v", secondMessages)
		}
	}
	if countLoopGuardEvents(events.events, "invalid_tool_arguments") != 1 {
		t.Fatalf("schema recovery loop guard count mismatch: %#v", events.events)
	}
	if result.ReleaseCandidateTerminal != nil {
		result.ReleaseCandidateTerminal()
	}
}

func TestRuntimeRunnerRetriesDuplicateCreatePlanBatchBeforePersistence(t *testing.T) {
	const marker = "DUPLICATE_CREATE_PLAN_ARGUMENT_MARKER"
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-duplicate-create-plan", "turn-duplicate-create-plan", workspace)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		{
			callbackChunks: []domainmodel.Chunk{
				{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: "duplicate-plan-1", Name: toolcatalogapp.ToolCreatePlanName}},
				{Kind: domainmodel.ChunkToolCallStart, ToolCall: domainmodel.ToolCall{ID: "duplicate-plan-2", Name: toolcatalogapp.ToolCreatePlanName}},
			},
			resultChunks: []domainmodel.Chunk{
				{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: "duplicate-plan-1", Name: toolcatalogapp.ToolCreatePlanName, Arguments: json.RawMessage(`{"markdown":"` + marker + ` one","operation":"draft"}`)}},
				{Kind: domainmodel.ChunkToolCall, ToolCall: domainmodel.ToolCall{ID: "duplicate-plan-2", Name: toolcatalogapp.ToolCreatePlanName, Arguments: json.RawMessage(`{"markdown":"` + marker + ` two","operation":"draft"}`)}},
			},
		},
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "Do you want me to retry with exactly one plan?"}}},
	}}
	driver := &toolStepDriverStub{}
	events := &runtimeLoopEventRecorderStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{toolcatalogapp.CreatePlanToolSchema()}}
	}

	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Create the implementation plan.", ProviderID: "provider", Model: "model", Mode: "plan",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", NormalizedSandboxMode: "workspace-write", EffectiveMaxModelSteps: 3,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "Create the implementation plan."},
		},
	}, deps)
	if err != nil || result.AssistantText != "Do you want me to retry with exactly one plan?" || len(provider.requests) != 2 ||
		len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
		t.Fatalf("duplicate create_plan recovery did not converge without first-batch effects: requests=%d ready=%#v executed=%#v batches=%#v result=%#v err=%v",
			len(provider.requests), driver.ready, driver.executed, driver.batches, result, err)
	}
	secondMessages := provider.requests[1].Messages
	if len(secondMessages) == 0 || secondMessages[len(secondMessages)-1].Role != "user" ||
		secondMessages[len(secondMessages)-1].Content != providerDuplicateCreatePlanRecoveryPrompt ||
		!strings.Contains(secondMessages[len(secondMessages)-1].Content, "exactly one create_plan") ||
		strings.Contains(secondMessages[len(secondMessages)-1].Content, marker) {
		t.Fatalf("duplicate create_plan correction prompt is not fixed and data-free: %#v", secondMessages)
	}
	for _, message := range secondMessages {
		if len(message.ToolCalls) > 0 || message.Role == "tool" || strings.Contains(message.Content, marker) {
			t.Fatalf("rejected duplicate tool draft crossed the recovery boundary: %#v", secondMessages)
		}
	}
	if countLoopGuardEvents(events.events, duplicateCreatePlanValidationCode) != 1 {
		t.Fatalf("duplicate create_plan recovery loop guard count mismatch: %#v", events.events)
	}
	eventBody, marshalErr := json.Marshal(events.events)
	if marshalErr != nil || strings.Contains(string(eventBody), marker) {
		t.Fatalf("duplicate create_plan arguments crossed the loop-event boundary: %s err=%v", eventBody, marshalErr)
	}
	if result.ReleaseCandidateTerminal != nil {
		result.ReleaseCandidateTerminal()
	}
}

func TestRuntimeRunnerBindsUnadvertisedToolDiagnosticsToProviderRequest(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-tool-diagnostic", "turn-tool-diagnostic", workspace)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("unadvertised-read-alias", "read_file", `{}`),
	}}
	events := &runtimeLoopEventRecorderStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	deps.ToolDriver = &toolStepDriverStub{}
	toolSchemas := append(zeroArgumentToolSchemas("read", "read_file"), toolcatalogapp.CreatePlanToolSchema())
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: toolSchemas}
	}

	_, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Inspect the repository and create a plan.", ProviderID: "provider", Model: "model", Mode: "plan",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "never", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 3,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "Inspect the repository and create a plan."},
		},
	}, deps)
	public := PublicFailureForError(err)
	details := public.Details()
	if err == nil || public.Code() != "tool_not_advertised" ||
		strings.TrimSpace(fmt.Sprint(details["rejectedToolNormalizedNameSha256"])) != domainmodel.BytesHash([]byte("read_file")) ||
		strings.TrimSpace(fmt.Sprint(details["rejectedToolCategory"])) != "known_alias_not_advertised" ||
		strings.TrimSpace(fmt.Sprint(details["promptRoute"])) != "tool_agent" || details["loopStep"] != float64(0) ||
		details["providerRequestRunToolStepManifestSame"] != true {
		t.Fatalf("runtime request/tool-step diagnostic binding mismatch: err=%v details=%#v requests=%#v", err, details, provider.requests)
	}
}

func TestRuntimeRunnerGivesStrictZeroArgumentToolExactDataFreeRecovery(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-zero-args-recovery", "turn-zero-args-recovery", workspace)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("invalid-status", "status", `{"unexpected":"private-marker"}`),
		{callbackChunks: []domainmodel.Chunk{{Kind: domainmodel.ChunkText, Text: "ordinary task completed"}}},
	}}
	driver := &toolStepDriverStub{readOnly: map[string]bool{"status": true}}
	events := &runtimeLoopEventRecorderStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	deps.ToolDriver = driver
	toolSchemas := []domainmodel.ToolSchema{{
		Name:       "status",
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}}
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: toolSchemas}
	}

	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Check status and finish the task.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 3,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "Check status and finish the task."},
		},
	}, deps)
	if err != nil || result.AssistantText != "ordinary task completed" || len(provider.requests) != 2 ||
		len(driver.ready) != 0 || len(driver.executed) != 0 {
		t.Fatalf("strict zero-argument recovery did not converge: calls=%d ready=%#v executed=%#v result=%#v err=%v",
			len(provider.requests), driver.ready, driver.executed, result, err)
	}
	recovery := provider.requests[1].Messages
	wantPrompt := providerToolArgumentValidationRecoveryPromptForTool("status", toolSchemas)
	if len(recovery) == 0 || recovery[len(recovery)-1].Role != "user" ||
		recovery[len(recovery)-1].Content != wantPrompt || !strings.Contains(wantPrompt, "exactly {}") {
		t.Fatalf("exact zero-argument correction prompt is unavailable: %#v", recovery)
	}
	for _, request := range provider.requests[1:] {
		for _, message := range request.Messages {
			if strings.Contains(message.Content, "private-marker") || strings.Contains(message.Content, "invalid-status") {
				t.Fatalf("rejected provider arguments crossed the recovery boundary: %#v", request.Messages)
			}
		}
	}
	eventBody, marshalErr := json.Marshal(events.events)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(eventBody), "private-marker") || strings.Contains(string(eventBody), "unexpected") ||
		strings.Contains(string(eventBody), "invalid-status") {
		t.Fatalf("rejected provider arguments crossed the loop-event boundary: %s", eventBody)
	}
	if countLoopGuardEvents(events.events, "invalid_tool_arguments") != 1 {
		t.Fatalf("schema recovery loop guard count mismatch: %#v", events.events)
	}
	if result.ReleaseCandidateTerminal != nil {
		result.ReleaseCandidateTerminal()
	}
}

func TestRuntimeRunnerRecoversIntoValidRequiredFinalTool(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-required-tool-args-recovery", "turn-required-tool-args-recovery", workspace)
	required := toolcatalogapp.ForegroundSubmitToolName
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("invalid-submit", required, `{}`),
		runtimeRunnerTerminalToolResponse("valid-submit", required, `{"result":"bounded result"}`),
	}}
	driver := &toolStepDriverStub{readOnly: map[string]bool{required: true}}
	events := &runtimeLoopEventRecorderStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{toolcatalogapp.ForegroundSubmitToolSchema()}}
	}
	claims := 0
	releases := 0
	deps.AcquireCandidateTerminal = func(ctx context.Context, _ RuntimeProviderStep) (context.Context, func(), error) {
		claims++
		return ctx, func() { releases++ }, nil
	}

	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Return the bounded child result.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext, RequiredFinalToolName: required, OutputTokenBudget: 64,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "Return the bounded child result."},
		},
	}, deps)
	if err != nil || result.AssistantText != "Foreground child result submitted." ||
		len(provider.requests) != 2 || len(driver.ready) != 1 || driver.ready[0] != required ||
		len(driver.executed) != 1 || driver.executed[0] != required || claims != 2 || releases != 1 ||
		countLoopGuardEvents(events.events, "invalid_tool_arguments") != 1 ||
		result.ReleaseCandidateTerminal == nil {
		t.Fatalf("required final tool did not recover exactly once: calls=%d ready=%#v executed=%#v claims=%d releases=%d result=%#v err=%v",
			len(provider.requests), driver.ready, driver.executed, claims, releases, result, err)
	}
	result.ReleaseCandidateTerminal()
	if releases != 2 {
		t.Fatalf("required final terminal claim was not released exactly once per attempt: %d", releases)
	}
}

func TestRuntimeRunnerHardStopsRepeatedPrePersistenceToolArgumentValidation(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-tool-args-storm", "turn-tool-args-storm", workspace)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("invalid-read-1", "read", `{}`),
		runtimeRunnerTerminalToolResponse("invalid-read-2", "read", `{}`),
		runtimeRunnerTerminalToolResponse("invalid-read-3", "read", `{}`),
	}}
	driver := &toolStepDriverStub{readOnly: map[string]bool{"read": true}}
	events := &runtimeLoopEventRecorderStub{}
	deps := runtimeRunnerTestDependencies(provider, events)
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{requiredPathToolSchema("read")}}
	}

	result, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Read the repository file.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 1,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "Read the repository file."},
		},
	}, deps)
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "tool_invalid_arguments_storm" ||
		len(provider.requests) != InvalidToolArgumentsHardStopThreshold ||
		len(driver.ready) != 0 || len(driver.executed) != 0 || result.AssistantText != "" ||
		result.TerminalRecoveryKind != RuntimeTerminalRecoveryNone {
		t.Fatalf("invalid tool arguments did not hard-stop safely: calls=%d ready=%#v executed=%#v result=%#v err=%v",
			len(provider.requests), driver.ready, driver.executed, result, err)
	}
	if failure.Message != "Turn stopped because the provider repeatedly issued tool calls that did not satisfy the advertised JSON Schema." ||
		failure.Details["toolName"] != "read" || failure.Details["stormCount"] != float64(InvalidToolArgumentsHardStopThreshold) {
		t.Fatalf("existing JSON Schema storm contract changed: %#v", failure)
	}
	if _, changed := failure.Details["reason"]; changed {
		t.Fatalf("existing JSON Schema storm details gained duplicate-only fields: %#v", failure.Details)
	}
	if countLoopGuardEvents(events.events, "invalid_tool_arguments") != InvalidToolArgumentsHardStopThreshold-1 {
		t.Fatalf("invalid argument recovery exceeded its bounded nudges: %#v", events.events)
	}
}

func TestRuntimeRunnerNeverRetriesValidationErrorAfterReadyPersistenceStarts(t *testing.T) {
	workspace := t.TempDir()
	securityContext := newLoopGeneralContextV2(t, "thread-tool-ready-validation", "turn-tool-ready-validation", workspace)
	provider := &providerStreamStub{responses: []providerStreamResponse{
		runtimeRunnerTerminalToolResponse("valid-read", "read", `{"path":"README.md"}`),
	}}
	driver := &validationAfterReadyDriver{toolStepDriverStub: &toolStepDriverStub{readOnly: map[string]bool{"read": true}}}
	deps := runtimeRunnerTestDependencies(provider, &runtimeLoopEventRecorderStub{})
	deps.ToolDriver = driver
	deps.ResolveToolCatalog = func(bool, RuntimeProviderStep) RuntimeRunnerToolCatalog {
		return RuntimeRunnerToolCatalog{PromptRoute: "tool_agent", Schemas: []domainmodel.ToolSchema{requiredPathToolSchema("read")}}
	}

	_, err := RunRuntimeAgentLoop(context.Background(), RuntimeRunnerInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, Workspace: workspace,
		Prompt: "Read the repository file.", ProviderID: "provider", Model: "model",
		ProviderConfig: domainmodel.TurnConfig{
			ProviderID: "provider", EndpointFormat: "chat_completions", BaseURL: "https://provider.invalid", Model: "model",
		},
		ApprovalPolicy: "auto", SandboxMode: "workspace-write", EffectiveMaxModelSteps: 3,
		SecurityContext: securityContext,
		Messages: []domainmodel.Message{
			{Role: "system", Content: "system"},
			{Role: "user", Content: "Read the repository file."},
		},
	}, deps)
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "validation_error" ||
		len(provider.requests) != 1 || len(driver.ready) != 1 || len(driver.executed) != 0 {
		t.Fatalf("post-ready validation error was retried: calls=%d ready=%#v executed=%#v err=%v",
			len(provider.requests), driver.ready, driver.executed, err)
	}
}

type validationAfterReadyDriver struct {
	*toolStepDriverStub
}

func (driver *validationAfterReadyDriver) PersistToolCallReady(
	_ context.Context,
	_, _ string,
	call domainmodel.ToolCall,
	_ int,
	_ domainsecurity.TurnSecurityContext,
	_ domainsecurity.ExecutionGrant,
) (string, error) {
	driver.ready = append(driver.ready, call.Name)
	return "", TurnFailureError{Message: "ready persistence validation failed", Code: "validation_error", Severity: "error"}
}

func requiredPathToolSchema(name string) domainmodel.ToolSchema {
	return domainmodel.ToolSchema{
		Name: name,
		Parameters: json.RawMessage(
			`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`,
		),
	}
}

func countLoopGuardEvents(events []map[string]any, guardKind string) int {
	count := 0
	for _, event := range events {
		if event["stage"] != "loop_guard" {
			continue
		}
		details, _ := event["details"].(map[string]any)
		if details["guardKind"] == guardKind {
			count++
		}
	}
	return count
}

var _ ToolStepDriver = (*validationAfterReadyDriver)(nil)
