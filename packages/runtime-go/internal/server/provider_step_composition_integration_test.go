package server

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	apploop "analytix.local/runtime-go/internal/app/loop"
	appmodel "analytix.local/runtime-go/internal/app/model"
	nativecomponentapp "analytix.local/runtime-go/internal/app/nativecomponent"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainnative "analytix.local/runtime-go/internal/domain/nativecomponent"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	nativecomponentport "analytix.local/runtime-go/internal/ports/nativecomponent"
	provider "analytix.local/runtime-go/internal/provider"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

const (
	providerStepDocsTool       = "mcp__docs__lookup"
	providerStepFundsTool      = "mcp__analytix_funds__analyze_account_flows"
	providerStepFundsCountTool = "mcp__analytix_funds__count_case_rows"
)

type providerStepIntegrationMCP struct {
	*admissionFailureMCP
	advertisements                 []domainmcp.ToolAdvertisementV1
	nativeFailureInvalidationCalls int
	toolCalls                      atomic.Int64
	beforeCurrentValidation        func() error
	healthTrace                    *providerStepHealthTrace
}

func (source *providerStepIntegrationMCP) InvalidateHostFundsSourceAfterSettledNativeFailure() {
	source.nativeFailureInvalidationCalls++
	source.revokeCurrentProbe()
}

func (source *providerStepIntegrationMCP) CallTool(
	name string,
	readOnly bool,
	arguments ...map[string]any,
) map[string]any {
	source.toolCalls.Add(1)
	return source.admissionFailureMCP.CallTool(name, readOnly, arguments...)
}

func (source *providerStepIntegrationMCP) ValidateCurrentProbe(
	ctx context.Context,
	serverID string,
	securityContext domainsecurity.TurnSecurityContext,
) error {
	if source.beforeCurrentValidation != nil {
		if err := source.beforeCurrentValidation(); err != nil {
			return err
		}
	}
	if source.healthTrace != nil {
		source.healthTrace.record("source_validate")
	}
	return source.admissionFailureMCP.ValidateCurrentProbe(ctx, serverID, securityContext)
}

func (source *providerStepIntegrationMCP) Tools() []string {
	tools := make([]string, 0, len(source.advertisements))
	for _, advertisement := range source.advertisements {
		tools = append(tools, advertisement.Name)
	}
	return tools
}

func (source *providerStepIntegrationMCP) LiveTools() []string {
	return source.Tools()
}

func (source *providerStepIntegrationMCP) LiveToolsForSecurityContext(domainsecurity.TurnSecurityContext) []string {
	return source.Tools()
}

func (source *providerStepIntegrationMCP) MCPToolAdvertisementSnapshotV1(domainsecurity.TurnSecurityContext) []domainmcp.ToolAdvertisementV1 {
	return append([]domainmcp.ToolAdvertisementV1(nil), source.advertisements...)
}

func (*providerStepIntegrationMCP) ToolReadOnlyHint(string) bool { return true }

func (source *providerStepIntegrationMCP) ToolInputSchema(toolName string) (json.RawMessage, bool) {
	for _, advertisement := range source.advertisements {
		if advertisement.Name == toolName {
			return append(json.RawMessage(nil), advertisement.InputSchema...), true
		}
	}
	return nil, false
}

func (source *providerStepIntegrationMCP) ToolOutputSchema(toolName string) (json.RawMessage, bool) {
	for _, advertisement := range source.advertisements {
		if advertisement.Name == toolName {
			return append(json.RawMessage(nil), advertisement.OutputSchema...), true
		}
	}
	return nil, false
}

func (source *providerStepIntegrationMCP) ToolDescription(toolName string) (string, bool) {
	for _, advertisement := range source.advertisements {
		if advertisement.Name == toolName {
			return advertisement.Description, true
		}
	}
	return "", false
}

type providerStepRecordingProvider struct {
	mu           sync.Mutex
	requests     []domainmodel.Request
	onCall       func(int, domainmodel.Request) error
	responseText func(int, domainmodel.Request) string
}

func (*providerStepRecordingProvider) RequiresDurablePipelineStagesV1() {}

func (providerClient *providerStepRecordingProvider) Stream(
	_ context.Context,
	request domainmodel.Request,
) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	providerClient.mu.Lock()
	call := len(providerClient.requests) + 1
	providerClient.requests = append(providerClient.requests, cloneProviderStepRequest(request))
	onCall := providerClient.onCall
	responseText := providerClient.responseText
	providerClient.mu.Unlock()
	if onCall != nil {
		if err := onCall(call, request); err != nil {
			return domainmodel.Result{}, err
		}
	}
	text := fmt.Sprintf("provider-step-candidate-%d", call)
	if responseText != nil {
		if custom := responseText(call, request); custom != "" {
			text = custom
		}
	}
	if request.OnChunk != nil {
		if err := request.OnChunk(domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: text}); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{StreamCompleted: true}, nil
}

func (providerClient *providerStepRecordingProvider) Requests() []domainmodel.Request {
	providerClient.mu.Lock()
	defer providerClient.mu.Unlock()
	requests := make([]domainmodel.Request, 0, len(providerClient.requests))
	for _, request := range providerClient.requests {
		requests = append(requests, cloneProviderStepRequest(request))
	}
	return requests
}

func cloneProviderStepRequest(request domainmodel.Request) domainmodel.Request {
	cloned := request
	cloned.Messages = append([]domainmodel.Message(nil), request.Messages...)
	cloned.Tools = append([]domainmodel.ToolSchema(nil), request.Tools...)
	return cloned
}

type providerStepNativeDurableAuthority struct{}

func (providerStepNativeDurableAuthority) ValidateCurrent(
	_ string,
	thread map[string]any,
) (domainsecurity.TurnSecurityContext, error) {
	return domainsecurity.ParseTurnSecurityContext(thread["securityState"])
}

type providerStepHealthTrace struct {
	mu                   sync.Mutex
	events               []string
	ordinaryCurrentness  int
	componentCurrentness int
	healthRunnerCalls    int
	generalCurrentness   int
}

func (trace *providerStepHealthTrace) record(event string) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.events = append(trace.events, event)
	trace.mu.Unlock()
}

func (trace *providerStepHealthTrace) recordOrdinaryCurrentness() {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.ordinaryCurrentness++
	trace.events = append(trace.events, fmt.Sprintf("h1_ordinary_%d", trace.ordinaryCurrentness))
	trace.mu.Unlock()
}

func (trace *providerStepHealthTrace) recordComponentCurrentness() {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.componentCurrentness++
	trace.events = append(trace.events, fmt.Sprintf("h1_component_%d", trace.componentCurrentness))
	trace.mu.Unlock()
}

func (trace *providerStepHealthTrace) recordHealthRunner() {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.healthRunnerCalls++
	trace.events = append(trace.events, "h1_runner")
	trace.mu.Unlock()
}

func (trace *providerStepHealthTrace) recordGeneralCurrentness() {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.generalCurrentness++
	trace.events = append(trace.events, "general_currentness")
	trace.mu.Unlock()
}

func (trace *providerStepHealthTrace) snapshot() (
	[]string,
	int,
	int,
	int,
	int,
) {
	if trace == nil {
		return nil, 0, 0, 0, 0
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return append([]string(nil), trace.events...), trace.ordinaryCurrentness,
		trace.componentCurrentness, trace.healthRunnerCalls, trace.generalCurrentness
}

type providerStepNativeOwner struct {
	err         error
	currentErr  error
	healthTrace *providerStepHealthTrace
}

func (owner providerStepNativeOwner) Execute(
	context.Context,
	domainnative.Request,
) (domainnative.Result, error) {
	owner.healthTrace.recordHealthRunner()
	if owner.err != nil {
		return domainnative.Result{}, owner.err
	}
	return domainnative.Result{
		SchemaVersion: domainnative.ResultSchemaVersion,
		ComponentID:   domainnative.ComponentDataEngine,
		Operation:     "health",
		Status:        "ready",
		RegistryDigest: domainsecurity.SHA256Hex(
			[]byte("provider-step-native-registry"),
		),
	}, nil
}

func (providerStepNativeOwner) Close() error { return nil }

func (owner *providerStepNativeOwner) ValidateCurrentDataEngine(ctx context.Context) error {
	if owner == nil || ctx == nil || ctx.Err() != nil {
		return nativecomponentapp.ErrAuthorityInvalid
	}
	owner.healthTrace.recordComponentCurrentness()
	if owner.currentErr != nil {
		return owner.currentErr
	}
	return nil
}

type providerStepCaseAuthority struct {
	*caseThreadAuthorityStub
	securityContext domainsecurity.TurnSecurityContext
}

func (authority *providerStepCaseAuthority) ContainsContext(
	securityContext domainsecurity.TurnSecurityContext,
) bool {
	return authority != nil && securityContext == authority.securityContext
}

func (authority *providerStepCaseAuthority) ContextTurnIDs(threadID string) []string {
	if authority == nil || threadID != authority.securityContext.ThreadID {
		return nil
	}
	return []string{authority.securityContext.TurnID}
}

type providerStepIntegrationFixture struct {
	handler         *runtimeServerHandler
	input           runtimeAgentLoopInput
	source          *providerStepIntegrationMCP
	snapshot        *admissionFailureSnapshotAuthority
	nativeOwner     *providerStepNativeOwner
	healthTrace     *providerStepHealthTrace
	securityContext domainsecurity.TurnSecurityContext
}

func TestRuntimeProviderStepCompositionPromotesOrdinaryFundsOrdinary(t *testing.T) {
	fixture := newProviderStepIntegrationFixture(t, "promoted-sequence")
	ordinaryBefore := "修改普通源码文件并运行单元测试。"
	fundsPrompt := "查询当前案件账户在指定期间的流入、流出、净额和交易笔数。"
	ordinaryAfter := "继续修改普通源码文件并重新运行单元测试。"
	recordingProvider := &providerStepRecordingProvider{}
	recordingProvider.onCall = func(call int, _ domainmodel.Request) error {
		steer := ""
		clientID := ""
		switch call {
		case 1:
			steer = fundsPrompt
			clientID = "018f47a0-13d2-4c9c-8f51-4ae4f62e8b12"
		case 2:
			steer = ordinaryAfter
			clientID = "018f47a0-13d2-4d9c-9f51-4ae4f62e8b13"
		default:
			return nil
		}
		result, err := fixture.handler.steerRuntimeTurn(context.Background(), controlapp.SteerTurnRequest{
			ThreadID: fixture.input.ThreadID, TurnID: fixture.input.TurnID,
			ExpectedTurnID: fixture.input.TurnID, ClientUserMessageID: clientID, Text: steer,
		})
		if err != nil {
			return err
		}
		if result.StatusCode != 200 || result.Body["ok"] != true {
			return fmt.Errorf("steer admission failed: %#v", result)
		}
		return nil
	}
	fixture.handler.provider = recordingProvider
	fixture.input.Request.Prompt = ordinaryBefore
	fixture.input.SystemPrompt = "provider step integration base"

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
			{Role: "system", Content: fixture.input.SystemPrompt},
			{Role: "user", Content: ordinaryBefore},
		},
	)
	if err != nil {
		thread, _ := fixture.handler.store.GetThread(fixture.input.ThreadID)
		turn, _ := appmodel.TurnByID(thread, fixture.input.TurnID)
		t.Fatalf("run provider-step sequence: %v items=%#v", err, turn["items"])
	}
	if result.ReleaseCandidateTerminal == nil || result.CandidateTerminalContext == nil {
		t.Fatalf("provider-step sequence did not return the real candidate terminal handoff: %#v", result)
	}
	result.ReleaseCandidateTerminal()
	if result.AssistantText != "provider-step-candidate-3" {
		t.Fatalf("unexpected terminal candidate: %#v", result)
	}

	requests := recordingProvider.Requests()
	const promotedSteeringPrefix = "Mid-turn user follow-up for the current task. Treat this as additional guidance for the active turn, not a new independent task. Any earlier assistant draft for this turn was private and discarded; return a complete, self-contained replacement response."
	wantPrompts := []string{
		ordinaryBefore,
		promotedSteeringPrefix + "\n\n" + fundsPrompt,
		promotedSteeringPrefix + "\n\n" + ordinaryAfter,
	}
	wantOrdinary := []bool{true, false, true}
	wantFundsTool := []bool{false, true, false}
	if len(requests) != len(wantPrompts) {
		t.Fatalf("provider dispatch count=%d want=%d", len(requests), len(wantPrompts))
	}
	for index, request := range requests {
		if prompt := lastProviderStepUserPrompt(request.Messages); prompt != wantPrompts[index] {
			t.Fatalf("provider request %d prompt=%q want=%q", index, prompt, wantPrompts[index])
		}
		if request.PrivateProviderTelemetry == nil || request.PrivateProviderTelemetry.OrdinaryEffect != wantOrdinary[index] {
			t.Fatalf("provider request %d effect telemetry=%#v wantOrdinary=%t", index, request.PrivateProviderTelemetry, wantOrdinary[index])
		}
		if got := providerStepRequestHasTool(request, providerStepFundsTool); got != wantFundsTool[index] {
			t.Fatalf("provider request %d funds tool=%t want=%t tools=%#v", index, got, wantFundsTool[index], request.Tools)
		}
		if !providerStepRequestHasTool(request, providerStepDocsTool) {
			t.Fatalf("provider request %d lost the ordinary MCP base: %#v", index, request.Tools)
		}
	}
	if validations := fixture.source.currentValidations.Load(); validations != 1 {
		t.Fatalf("source current validations=%d want exactly one funds attempt", validations)
	}
}

func TestRuntimeProviderFundsStepRunsH1BeforeCurrentSourceProbe(t *testing.T) {
	t.Run("current source reaches provider after settled H1", func(t *testing.T) {
		fixture := newProviderStepIntegrationFixture(t, "h1-source-provider-order")
		fixture.source.beforeCurrentValidation = func() error {
			if !providerStepHealthSettled(fixture) {
				return fmt.Errorf("provider-step H1 settlement is unavailable")
			}
			fixture.healthTrace.record("h1_settled")
			return nil
		}
		recordingProvider := &providerStepRecordingProvider{
			onCall: func(_ int, _ domainmodel.Request) error {
				fixture.healthTrace.record("provider_dispatch")
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
			t.Fatal("provider-step order loop ownership unavailable")
		}
		defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)

		fundsStep := apploop.RuntimeProviderStep{
			Prompt:        "查询当前案件账户的流入、流出、净额和交易笔数。",
			LogicalEffect: domainsecurity.LogicalEffectFundsData,
		}
		result, err := runRestoredProviderStepLoop(loopContext, fixture, fundsStep)
		if err != nil {
			t.Fatal("provider-step order failed before provider completion")
		}
		if result.ReleaseCandidateTerminal == nil || result.CandidateTerminalContext == nil {
			t.Fatal("provider-step order lost candidate terminal authority")
		}
		result.ReleaseCandidateTerminal()
		requests := recordingProvider.Requests()
		if len(requests) != 1 || !providerStepRequestHasTool(requests[0], providerStepFundsTool) ||
			providerStepRequestHasTool(requests[0], providerStepFundsCountTool) ||
			!providerStepRequestHasTool(requests[0], providerStepDocsTool) {
			t.Fatalf("provider-step catalog phase mismatch: requests=%d", len(requests))
		}
		if fixture.source.currentValidations.Load() != 1 ||
			fixture.source.nativeFailureInvalidationCalls != 0 || fixture.source.toolCalls.Load() != 0 {
			t.Fatalf(
				"provider-step currentness phase mismatch: source=%d invalidations=%d tools=%d",
				fixture.source.currentValidations.Load(),
				fixture.source.nativeFailureInvalidationCalls,
				fixture.source.toolCalls.Load(),
			)
		}
		providerStepAssertHealthTrace(t, fixture.healthTrace, []string{
			"h1_ordinary_1", "h1_component_1",
			"h1_ordinary_2", "h1_component_2",
			"h1_runner",
			"h1_ordinary_3", "h1_component_3",
			"h1_settled", "source_validate", "provider_dispatch",
		})
	})

	t.Run("revoked source blocks funds and preserves following ordinary step", func(t *testing.T) {
		fixture := newProviderStepIntegrationFixture(t, "h1-source-revoked-before-provider")
		fixture.source.beforeCurrentValidation = func() error {
			if !providerStepHealthSettled(fixture) {
				return fmt.Errorf("provider-step H1 settlement is unavailable")
			}
			fixture.healthTrace.record("h1_settled")
			fixture.source.revokeCurrentProbe()
			return nil
		}
		recordingProvider := &providerStepRecordingProvider{
			onCall: func(_ int, request domainmodel.Request) error {
				if request.PrivateProviderTelemetry != nil && request.PrivateProviderTelemetry.OrdinaryEffect {
					fixture.healthTrace.record("ordinary_provider_dispatch")
				} else {
					fixture.healthTrace.record("funds_provider_dispatch")
				}
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
			t.Fatal("provider-step revoked loop ownership unavailable")
		}
		defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)

		fundsStep := apploop.RuntimeProviderStep{
			Prompt:        "查询当前案件账户的流入和流出。",
			LogicalEffect: domainsecurity.LogicalEffectFundsData,
		}
		if result, err := runRestoredProviderStepLoop(loopContext, fixture, fundsStep); err == nil {
			if result.ReleaseCandidateTerminal != nil {
				result.ReleaseCandidateTerminal()
			}
			t.Fatal("revoked source reached funds provider completion")
		}
		if len(recordingProvider.Requests()) != 0 || fixture.source.currentValidations.Load() != 1 ||
			fixture.source.nativeFailureInvalidationCalls != 0 || fixture.source.toolCalls.Load() != 0 {
			t.Fatalf(
				"revoked source phase mismatch: requests=%d source=%d invalidations=%d tools=%d",
				len(recordingProvider.Requests()), fixture.source.currentValidations.Load(),
				fixture.source.nativeFailureInvalidationCalls, fixture.source.toolCalls.Load(),
			)
		}

		ordinaryStep := apploop.RuntimeProviderStep{
			Prompt:        "读取普通源码并运行普通单元测试。",
			LogicalEffect: domainsecurity.LogicalEffectOrdinary,
			OrdinaryWork:  true,
		}
		ordinaryResult, err := runRestoredProviderStepLoop(loopContext, fixture, ordinaryStep)
		if err != nil {
			t.Fatal("ordinary provider step did not survive Funds revocation")
		}
		if ordinaryResult.ReleaseCandidateTerminal == nil || ordinaryResult.CandidateTerminalContext == nil {
			t.Fatal("ordinary provider step lost candidate terminal authority")
		}
		ordinaryResult.ReleaseCandidateTerminal()
		requests := recordingProvider.Requests()
		if len(requests) != 1 || requests[0].PrivateProviderTelemetry == nil ||
			!requests[0].PrivateProviderTelemetry.OrdinaryEffect ||
			providerStepRequestHasTool(requests[0], providerStepFundsTool) ||
			providerStepRequestHasTool(requests[0], providerStepFundsCountTool) ||
			!providerStepRequestHasTool(requests[0], providerStepDocsTool) {
			t.Fatalf("ordinary provider isolation phase mismatch: requests=%d", len(requests))
		}
		if fixture.source.currentValidations.Load() != 1 ||
			fixture.source.nativeFailureInvalidationCalls != 0 || fixture.source.toolCalls.Load() != 0 {
			t.Fatalf(
				"ordinary isolation currentness mismatch: source=%d invalidations=%d tools=%d",
				fixture.source.currentValidations.Load(),
				fixture.source.nativeFailureInvalidationCalls,
				fixture.source.toolCalls.Load(),
			)
		}
		providerStepAssertHealthTrace(t, fixture.healthTrace, []string{
			"h1_ordinary_1", "h1_component_1",
			"h1_ordinary_2", "h1_component_2",
			"h1_runner",
			"h1_ordinary_3", "h1_component_3",
			"h1_settled", "source_validate", "ordinary_provider_dispatch",
		})
	})
}

func providerStepHealthSettled(fixture *providerStepIntegrationFixture) bool {
	if fixture == nil || fixture.handler == nil {
		return false
	}
	thread, err := fixture.handler.store.GetThread(fixture.input.ThreadID)
	if err != nil {
		return false
	}
	turn, found := appmodel.TurnByID(thread, fixture.input.TurnID)
	if !found {
		return false
	}
	registry, err := executiongrantapp.RegistryFromThread(
		fixture.input.ThreadID,
		thread,
		fixture.input.TurnID,
	)
	if err != nil {
		return false
	}
	items, _ := turn["items"].([]any)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok || stringField(item, "status") != "completed" {
			continue
		}
		grant, err := domainsecurity.ParseExecutionGrant(item["executionGrant"])
		if err != nil || grant.Provider != domainnative.NativeProvider {
			continue
		}
		return domainsecurity.VerifyExecutionGrantMembership(
			registry,
			fixture.input.ThreadID,
			fixture.input.TurnID,
			grant,
			domainsecurity.GrantRegistrySettled,
		) == nil
	}
	return false
}

func providerStepAssertHealthTrace(
	t *testing.T,
	trace *providerStepHealthTrace,
	wantEvents []string,
) {
	t.Helper()
	events, ordinary, component, runners, general := trace.snapshot()
	if ordinary != 3 || component != 3 || runners != 1 || general != 0 ||
		!reflect.DeepEqual(events, wantEvents) {
		t.Fatalf(
			"provider-step H1 order mismatch: ordinary=%d component=%d runners=%d general=%d events=%v",
			ordinary,
			component,
			runners,
			general,
			events,
		)
	}
}

func TestRuntimeProviderCaseDataStepDoesNotProbeFundsSource(t *testing.T) {
	fixture := newProviderStepIntegrationFixture(t, "generic-case-data")
	fixture.source.revokeCurrentProbe()
	recordingProvider := &providerStepRecordingProvider{}
	fixture.handler.provider = recordingProvider
	step := apploop.RuntimeProviderStep{
		Prompt:        "比较当前案件附件的受控元数据。",
		LogicalEffect: domainsecurity.LogicalEffectCaseData,
	}
	loopContext, cancelLoop := context.WithCancel(context.Background())
	defer cancelLoop()
	if !fixture.handler.runtimeControl().RegisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID, cancelLoop) {
		t.Fatal("generic case-data loop ownership registration failed")
	}
	defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)
	result, err := runRestoredProviderStepLoop(loopContext, fixture, step)
	if err != nil {
		t.Fatalf("generic case-data provider dispatch: %v", err)
	}
	if result.ReleaseCandidateTerminal == nil || result.CandidateTerminalContext == nil {
		t.Fatalf("generic case-data loop did not return candidate terminal handoff: %#v", result)
	}
	result.ReleaseCandidateTerminal()
	requests := recordingProvider.Requests()
	if len(requests) != 1 {
		t.Fatalf("generic case-data provider dispatches=%d want=1", len(requests))
	}
	if validations := fixture.source.currentValidations.Load(); validations != 0 {
		t.Fatalf("generic case-data effect consulted funds source %d times", validations)
	}
	if providerStepRequestHasTool(requests[0], providerStepFundsTool) {
		t.Fatalf("generic case-data provider inherited funds tool: %#v", requests[0].Tools)
	}
	if !providerStepRequestHasTool(requests[0], providerStepDocsTool) {
		t.Fatalf("generic case-data provider lost ordinary MCP tool: %#v", requests[0].Tools)
	}
}

func TestRuntimeProviderFundsAuthorityFailureDoesNotDisableFollowingOrdinaryStep(t *testing.T) {
	tests := []struct {
		name                  string
		invalidate            func(*providerStepIntegrationFixture)
		wantSourceValidations int64
		wantInvalidations     int
	}{
		{
			name: "stale DSV2",
			invalidate: func(fixture *providerStepIntegrationFixture) {
				fixture.snapshot.resolved.Record.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID("provider-step-stale-snapshot")
			},
			wantSourceValidations: 0,
		},
		{
			name: "revoked funds source",
			invalidate: func(fixture *providerStepIntegrationFixture) {
				fixture.source.revokeCurrentProbe()
			},
			wantSourceValidations: 1,
		},
		{
			name: "settled native failure",
			invalidate: func(fixture *providerStepIntegrationFixture) {
				fixture.nativeOwner.err = nativecomponentport.ErrProtocolInvalid
			},
			wantSourceValidations: 0,
			wantInvalidations:     1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProviderStepIntegrationFixture(t, test.name)
			test.invalidate(fixture)
			recordingProvider := &providerStepRecordingProvider{}
			fixture.handler.provider = recordingProvider
			loopContext, cancelLoop := context.WithCancel(context.Background())
			defer cancelLoop()
			if !fixture.handler.runtimeControl().RegisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID, cancelLoop) {
				t.Fatal("funds-failure loop ownership registration failed")
			}
			defer fixture.handler.runtimeControl().UnregisterTurnCancel(fixture.input.ThreadID, fixture.input.TurnID)
			fundsStep := apploop.RuntimeProviderStep{
				Prompt:        "查询当前案件账户在指定期间的流入和流出。",
				LogicalEffect: domainsecurity.LogicalEffectFundsData,
			}
			if result, err := runRestoredProviderStepLoop(loopContext, fixture, fundsStep); err == nil {
				if result.ReleaseCandidateTerminal != nil {
					result.ReleaseCandidateTerminal()
				}
				t.Fatal("invalid funds authority reached provider dispatch")
			}
			if calls := len(recordingProvider.Requests()); calls != 0 {
				t.Fatalf("invalid funds authority provider calls=%d want=0", calls)
			}
			if calls := fixture.source.nativeFailureInvalidationCalls; calls != test.wantInvalidations {
				t.Fatalf("settled native failure invalidations=%d want=%d", calls, test.wantInvalidations)
			}

			ordinaryPrompt := "读取普通源码并运行普通单元测试。"
			ordinaryStep := apploop.RuntimeProviderStep{
				Prompt: ordinaryPrompt, LogicalEffect: domainsecurity.LogicalEffectOrdinary, OrdinaryWork: true,
			}
			ordinaryResult, err := runRestoredProviderStepLoop(loopContext, fixture, ordinaryStep)
			if err != nil {
				t.Fatalf("ordinary provider dispatch after funds failure: %v", err)
			}
			if ordinaryResult.ReleaseCandidateTerminal == nil || ordinaryResult.CandidateTerminalContext == nil {
				t.Fatalf("ordinary loop after funds failure did not return candidate terminal handoff: %#v", ordinaryResult)
			}
			ordinaryResult.ReleaseCandidateTerminal()
			requests := recordingProvider.Requests()
			if len(requests) != 1 || lastProviderStepUserPrompt(requests[0].Messages) != ordinaryPrompt {
				t.Fatalf("ordinary dispatch after funds failure=%#v", requests)
			}
			if requests[0].PrivateProviderTelemetry == nil || !requests[0].PrivateProviderTelemetry.OrdinaryEffect {
				t.Fatalf("following request lost ordinary effect telemetry: %#v", requests[0].PrivateProviderTelemetry)
			}
			if providerStepRequestHasTool(requests[0], providerStepFundsTool) || !providerStepRequestHasTool(requests[0], providerStepDocsTool) {
				t.Fatalf("following ordinary catalog is not additive-safe: %#v", requests[0].Tools)
			}
			if validations := fixture.source.currentValidations.Load(); validations != test.wantSourceValidations {
				t.Fatalf("source current validations=%d want=%d", validations, test.wantSourceValidations)
			}
		})
	}
}

func TestSyncTurnFinalizationPersistsOrdinaryCandidateAfterDatasetStale(t *testing.T) {
	workspace := writeThreadMutationCaseBinding(t)
	durableRoot := t.TempDir()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: durableRoot, DataDir: t.TempDir(),
		ProviderID: "provider-step-finalization", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "provider-step-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	configureServerCaseExecution(t, handler, workspace)
	configureSteerTestCaseAuthorities(t, handler, durableRoot)
	snapshot, ok := handler.turnSecurity.SnapshotAuthorityV2.(*admissionFailureSnapshotAuthority)
	if !ok {
		t.Fatalf("unexpected sync-finalization snapshot authority: %T", handler.turnSecurity.SnapshotAuthorityV2)
	}

	recordingProvider := &providerStepRecordingProvider{}
	recordingProvider.onCall = func(call int, _ domainmodel.Request) error {
		if call != 1 {
			return fmt.Errorf("ordinary sync turn provider call=%d want=1", call)
		}
		// Invalidate only the strict DatasetSnapshotAuthorityV2 after the exact
		// ordinary provider attempt has begun. Identity, case binding, risk, and
		// workspace authority remain current.
		snapshot.resolved.Record.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID("provider-step-finalization-stale")
		return nil
	}
	handler.provider = recordingProvider
	thread, err := handler.store.CreateThread(map[string]any{
		"title": "ordinary finalization after stale dataset", "workspace": workspace,
		"providerId": "provider-step-finalization", "model": "provider-step-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	ordinaryPrompt := "读取普通源码文件并运行普通单元测试。"
	started, err := handler.startRuntimeTurn(context.Background(), threadID, startRuntimeTurnRequest{
		Prompt: ordinaryPrompt, RiskIntent: domainsecurity.RiskClassCase,
		ProviderID: "provider-step-finalization", Model: "provider-step-model",
	})
	if err != nil {
		t.Fatalf("sync ordinary turn after dataset staleness: %v", err)
	}
	if stringField(started, "status") != "completed" {
		t.Fatalf("sync ordinary turn status=%#v", started)
	}
	requests := recordingProvider.Requests()
	if len(requests) != 1 || lastProviderStepUserPrompt(requests[0].Messages) != ordinaryPrompt ||
		requests[0].PrivateProviderTelemetry == nil || !requests[0].PrivateProviderTelemetry.OrdinaryEffect {
		t.Fatalf("sync ordinary provider dispatch drifted: %#v", requests)
	}

	reloaded, err := handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, found := appmodel.TurnByID(reloaded, stringField(started, "turnId"))
	if !found || stringField(turn, "status") != "completed" {
		t.Fatalf("sync ordinary turn was not durably completed: %#v", turn)
	}
	securityContext, err := domainsecurity.ParseTurnSecurityContext(turn["securityContext"])
	if err != nil {
		t.Fatal(err)
	}
	currentInput := turnsecurityapp.CurrentValidationInput{
		OperationContext: context.Background(), Identity: handler.turnSecurity.Identity,
		Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
		SnapshotAuthority:   handler.turnSecurity.SnapshotAuthority,
		SnapshotAuthorityV2: handler.turnSecurity.SnapshotAuthorityV2,
		Context:             securityContext, Workspace: workspace,
	}
	if err := turnsecurityapp.ValidateCurrentForEffect(currentInput, true); err == nil {
		t.Fatal("sync finalization fixture did not make strict DSV2 authority stale")
	}
	if err := turnsecurityapp.ValidateCurrentForEffect(currentInput, false); err != nil {
		t.Fatalf("dataset staleness also disabled ordinary authority: %v", err)
	}
	const wantCandidate = "provider-step-candidate-1"
	if got := acceptedFinalAssistantText(turn); got != wantCandidate {
		acceptedFinal, _ := turn["acceptedFinal"].(map[string]any)
		publicView, _ := acceptedFinal["publicView"].(map[string]any)
		t.Fatalf(
			"sync finalization lost the ordinary provider result: got=%q want=%q variant=%q blocker=%q",
			got, wantCandidate, stringField(publicView, "variant"), stringField(publicView, "blockerCode"),
		)
	}
}

func runRestoredProviderStepLoop(
	ctx context.Context,
	fixture *providerStepIntegrationFixture,
	step apploop.RuntimeProviderStep,
) (runtimeAgentLoopResult, error) {
	input := fixture.input
	input.Request.Prompt = step.Prompt
	input.OrdinaryResultInputIsolated = step.OrdinaryEffect() && step.OrdinaryWork && !step.CaseSourceUnavailable
	input.SystemPrompt = "provider step integration base"
	input.RestoredProviderStep = &step
	maxSteps := 1
	input.Request.MaxModelSteps = &maxSteps
	return fixture.handler.runRuntimeAgentLoopWithMessages(
		ctx,
		input,
		[]provider.Message{
			{Role: "system", Content: input.SystemPrompt},
			{Role: "user", Content: step.Prompt},
		},
	)
}

func newProviderStepIntegrationFixture(t *testing.T, name string) *providerStepIntegrationFixture {
	t.Helper()
	workspace := writeThreadMutationCaseBinding(t)
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		ProviderID:     "provider-step-integration",
		BaseURL:        "https://provider.invalid",
		APIKey:         "test-key",
		Model:          "provider-step-model",
		EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	baseSource := configureServerCaseExecution(t, handler, workspace)
	healthTrace := &providerStepHealthTrace{}
	source := &providerStepIntegrationMCP{
		admissionFailureMCP: baseSource,
		advertisements:      providerStepAdvertisements(t),
		healthTrace:         healthTrace,
	}
	handler.mcp = source

	thread, err := handler.store.CreateThread(map[string]any{
		"title": "provider step " + name, "workspace": workspace,
		"providerId": "provider-step-integration", "model": "provider-step-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn-provider-step-integration"
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Thread: thread,
		ThreadID: threadID, TurnID: turnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
	})
	if err != nil || !domainsecurity.TurnSecurityContextAllowsCaseEvidence(securityContext) {
		t.Fatalf("freeze provider-step case context: context=%#v err=%v", securityContext, err)
	}
	securityRecord := turnsecurityapp.PublicRecord(securityContext)
	if err := handler.store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "prompt": "provider step integration",
		"items": []any{}, "steering": []any{}, "createdAt": "2026-07-27T12:00:00Z", "startedAt": "2026-07-27T12:00:00Z",
		"securityContext": securityRecord,
	}, "provider-step-integration", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	caseAuthority := &providerStepCaseAuthority{
		caseThreadAuthorityStub: &caseThreadAuthorityStub{threads: map[string]bool{threadID: true}},
		securityContext:         securityContext,
	}
	handler.caseThreads = caseAuthority
	handler.store.SetCaseThreadAuthority(caseAuthority)
	if err := handler.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(
		context.Background(), securityContext, time.Second,
	); err != nil {
		t.Fatalf("observe provider-step context: %v", err)
	}
	binding, err := handler.turnSecurity.Observer.Observe(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := turnsecurityapp.ProbeCurrentCaseSource(context.Background(), source, securityContext, binding); err != nil {
		t.Fatalf("probe provider-step source: %v", err)
	}
	nativeOwner := &providerStepNativeOwner{healthTrace: healthTrace}
	ordinaryHealth := nativecomponentapp.LiveAuthorityFunc(func(
		ctx context.Context,
		current domainsecurity.TurnSecurityContext,
	) error {
		healthTrace.recordOrdinaryCurrentness()
		return turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{
			OperationContext: ctx, Identity: handler.turnSecurity.Identity,
			Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
			Context: current, Workspace: current.WorkspaceRealPath,
		})
	})
	nativeAuthority, err := nativecomponentapp.NewReadyRuntimeAuthority(nativecomponentapp.RuntimeAuthorityDependencies{
		Health: nativecomponentapp.HealthDependencies{
			Store: handler.store, DurableAuthority: providerStepNativeDurableAuthority{},
			AcquireEffect: func(
				ctx context.Context,
				current domainsecurity.TurnSecurityContext,
			) (context.Context, func(), error) {
				return handler.runtimeSubagentState().AcquireContextEffectForAuthority(ctx, current, true)
			},
			Now: time.Now,
		},
		LiveAuthority: nativecomponentapp.LiveAuthorityFunc(func(
			ctx context.Context,
			current domainsecurity.TurnSecurityContext,
		) error {
			healthTrace.recordGeneralCurrentness()
			return turnsecurityapp.ValidateCurrent(turnsecurityapp.CurrentValidationInput{
				OperationContext: ctx, Identity: handler.turnSecurity.Identity,
				Observer: handler.turnSecurity.Observer, RiskAuthority: handler.turnSecurity.RiskAuthority,
				SnapshotAuthority:   handler.turnSecurity.SnapshotAuthority,
				SnapshotAuthorityV2: handler.turnSecurity.SnapshotAuthorityV2,
				Context:             current, Workspace: current.WorkspaceRealPath,
			})
		}),
		HealthOnlyAuthority: nativecomponentapp.NewHealthOnlyCurrentnessV1(
			ordinaryHealth,
			nativeOwner.ValidateCurrentDataEngine,
		),
		Owner: nativeOwner,
	})
	if err != nil {
		t.Fatalf("assemble provider-step native authority: %v", err)
	}
	handler.nativeAuthority = nativeAuthority
	thread, err = handler.store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := handler.turnSecurity.SnapshotAuthorityV2.(*admissionFailureSnapshotAuthority)
	if !ok {
		t.Fatalf("unexpected provider-step snapshot authority: %T", handler.turnSecurity.SnapshotAuthorityV2)
	}
	return &providerStepIntegrationFixture{
		handler: handler, source: source, snapshot: snapshot, securityContext: securityContext,
		nativeOwner: nativeOwner, healthTrace: healthTrace,
		input: runtimeAgentLoopInput{
			ThreadID: threadID, TurnID: turnID, Thread: thread,
			Request: startRuntimeTurnRequest{Prompt: "provider step integration"},
			ProviderConfig: provider.TurnConfig{
				ProviderID: "provider-step-integration", BaseURL: "https://provider.invalid",
				APIKey: "test-key", Model: "provider-step-model", EndpointFormat: "chat_completions",
			},
			ProviderID: "provider-step-integration", Model: "provider-step-model", Workspace: workspace,
			ApprovalPolicy: "never", SandboxMode: "workspace-write", SecurityContext: securityContext,
		},
	}
}

func providerStepAdvertisements(t *testing.T) []domainmcp.ToolAdvertisementV1 {
	t.Helper()
	closedInput := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)
	closedOutput := json.RawMessage(`{"type":"object","properties":{"result":{"type":"string"}},"required":["result"],"additionalProperties":false}`)
	identity := func(serverID string, epoch uint64) string {
		verified, err := domainsecurity.NewVerifiedMCPServerIdentity(
			serverID, serverID+"-server", "1.0.0",
			domainsecurity.SHA256Hex([]byte(serverID+"-provider-step-instance")), epoch,
		)
		if err != nil {
			t.Fatal(err)
		}
		return verified
	}
	return []domainmcp.ToolAdvertisementV1{
		{
			Name: providerStepDocsTool, Description: "Lookup ordinary documentation",
			InputSchema: closedInput, OutputSchema: closedOutput,
			TaskSupport: domainmcp.ToolTaskSupportForbidden, ReadOnly: true,
			ConnectionEpoch: 3, ServerIdentity: identity("docs", 3),
		},
		{
			Name: providerStepFundsTool, Description: "Analyze current-case account flows",
			InputSchema: closedInput, OutputSchema: closedOutput,
			TaskSupport: domainmcp.ToolTaskSupportForbidden, ReadOnly: true,
			ConnectionEpoch: 7, ServerIdentity: identity("analytix_funds", 7),
		},
		{
			Name: providerStepFundsCountTool, Description: "Host-only source canary",
			InputSchema: closedInput, OutputSchema: closedOutput,
			TaskSupport: domainmcp.ToolTaskSupportForbidden, ReadOnly: true,
			ConnectionEpoch: 7, ServerIdentity: identity("analytix_funds", 7),
		},
	}
}

func lastProviderStepUserPrompt(messages []domainmodel.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return messages[index].Content
		}
	}
	return ""
}

func providerStepRequestHasTool(request domainmodel.Request, toolName string) bool {
	for _, tool := range request.Tools {
		if tool.Name == toolName {
			return true
		}
	}
	return false
}
