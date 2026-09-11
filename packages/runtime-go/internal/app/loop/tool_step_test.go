package loop

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	appmodel "analytix.local/runtime-go/internal/app/model"
	toolcatalogapp "analytix.local/runtime-go/internal/app/toolcatalog"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domainevent "analytix.local/runtime-go/internal/domain/event"
	domainfailure "analytix.local/runtime-go/internal/domain/failure"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestHostGeneralOnlyRiskKeepsOrdinaryWorkspaceReadAvailable(t *testing.T) {
	workspace := "/workspace/no-case-marker"
	sourceFile := workspace + "/main.go"
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-read", TurnID: "turn-general-read", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	driver := &toolStepDriverStub{readOnly: map[string]bool{"read_file": true}}
	call := domainmodel.ToolCall{
		ID: loopTestHostToolCallID("general-source-read"), Name: "read_file",
		Arguments: json.RawMessage(`{"path":"` + sourceFile + `"}`),
	}
	schema := domainmodel.ToolSchema{
		Name:       "read_file",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
	}
	result, err := RunToolStep(context.Background(), ToolStepInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: "provider",
		ToolCalls: []domainmodel.ToolCall{call}, ToolSchemas: []domainmodel.ToolSchema{schema},
		AdvertisedTools: advertisedToolNames("read_file"), SecurityContext: securityContext, Driver: driver,
	})
	if err != nil {
		t.Fatalf("ordinary workspace read was rejected by unavailable case authority: %v", err)
	}
	if len(driver.grants) != 1 || len(driver.ready) != 1 || !reflect.DeepEqual(driver.executed, []string{"read_file"}) ||
		len(driver.batches) != 0 || len(result.Messages) != 1 || len(result.SettledToolReferences) != 1 {
		t.Fatalf("ordinary workspace read did not follow the normal grant/effect/result path: driver=%#v result=%#v", driver, result)
	}
}

func TestPendingToolCallRetainsExactLiveProviderStep(t *testing.T) {
	call := domainmodel.ToolCall{ID: "call-live-provider-step", Name: "write_file", Arguments: json.RawMessage(`{}`)}
	pending := pendingToolCallForStep(ToolStepInput{
		Prompt:                      "analyze the snapshot and continue the code edit",
		LogicalEffect:               domainsecurity.LogicalEffectFundsData,
		OrdinaryWork:                true,
		CaseSourceUnavailable:       true,
		OrdinaryResultInputIsolated: true,
	}, call, "item-live-provider-step", nil, domainsecurity.ExecutionGrant{})
	if pending.Prompt != "analyze the snapshot and continue the code edit" ||
		pending.LogicalEffect != domainsecurity.LogicalEffectFundsData || !pending.OrdinaryWork || !pending.ProviderStepExact || !pending.CaseSourceUnavailable ||
		pending.OrdinaryResultInputIsolated {
		t.Fatalf("live provider step was not retained exactly: %#v", pending)
	}

	ordinary := pendingToolCallForStep(ToolStepInput{
		Prompt: "continue the ordinary edit", LogicalEffect: domainsecurity.LogicalEffectOrdinary,
		OrdinaryWork: true, CaseSourceUnavailable: true, OrdinaryResultInputIsolated: true,
	}, call, "item-ordinary-provider-step", nil, domainsecurity.ExecutionGrant{})
	if !ordinary.ProviderStepExact || !ordinary.CaseSourceUnavailable || !ordinary.OrdinaryResultInputIsolated {
		t.Fatalf("sticky source failure overwrote exact ordinary-only provider input provenance: %#v", ordinary)
	}

	mixedDowngrade := pendingToolCallForStep(ToolStepInput{
		Prompt: "continue the mixed request without the source", LogicalEffect: domainsecurity.LogicalEffectOrdinary,
		OrdinaryWork: true, CaseSourceUnavailable: true,
	}, call, "item-mixed-provider-step", nil, domainsecurity.ExecutionGrant{})
	if mixedDowngrade.OrdinaryResultInputIsolated {
		t.Fatalf("an unisolated mixed downgrade acquired ordinary-only provider input provenance: %#v", mixedDowngrade)
	}

	nonExact := pendingToolCallForStep(ToolStepInput{Prompt: "legacy continuation"}, call, "item-legacy-provider-step", nil, domainsecurity.ExecutionGrant{})
	if nonExact.ProviderStepExact {
		t.Fatalf("unclassified pending step was marked exact: %#v", nonExact)
	}
}

func TestHostGeneralOnlyRiskBlocksFundsEffectBeforePersistenceOrExecution(t *testing.T) {
	workspace := "/workspace/no-case-marker"
	securityContext, err := securitycontexttest.HostGeneralOnlyExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-general-funds", TurnID: "turn-general-funds", WorkspaceRealPath: workspace,
		ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	toolName := "mcp__analytix_funds__count_case_rows"
	driver := &toolStepDriverStub{readOnly: map[string]bool{toolName: true}}
	result, err := RunToolStep(context.Background(), ToolStepInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: "provider", Workspace: workspace,
		ToolCalls:       []domainmodel.ToolCall{{ID: loopTestHostToolCallID("general-funds"), Name: toolName, Arguments: json.RawMessage(`{}`)}},
		ToolSchemas:     zeroArgumentToolSchemas(toolName),
		AdvertisedTools: advertisedToolNames(toolName),
		LiveMCPTools:    advertisedToolNames(toolName),
		MCPConnectionEpochs: map[string]uint64{
			toolName: 3,
		},
		MCPServerIdentities: map[string]string{
			toolName: loopTestMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 3),
		},
		MCPReadOnlyPolicies: map[string]bool{toolName: true},
		SecurityContext:     securityContext,
		Driver:              driver,
	})
	var failure TurnFailureError
	if err == nil || !errors.As(err, &failure) || failure.Code != "execution_grant_rejected" {
		t.Fatalf("funds effect without case authority did not fail at grant issuance: result=%#v err=%v", result, err)
	}
	if len(driver.grants) != 0 || len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 || len(result.Messages) != 0 {
		t.Fatalf("unauthorized funds effect produced durable/effect/result state: driver=%#v result=%#v", driver, result)
	}
}

func TestForegroundChildRequiresExactlyOneAcceptedFinalTool(t *testing.T) {
	required := toolcatalogapp.ForegroundSubmitToolName
	for name, calls := range map[string][]domainmodel.ToolCall{
		"wrong tool": {{ID: loopTestHostToolCallID("wrong"), Name: "read", Arguments: json.RawMessage(`{}`)}},
		"multiple calls": {
			{ID: loopTestHostToolCallID("submit-1"), Name: required, Arguments: json.RawMessage(`{"result":"one"}`)},
			{ID: loopTestHostToolCallID("submit-2"), Name: required, Arguments: json.RawMessage(`{"result":"two"}`)},
		},
	} {
		t.Run(name, func(t *testing.T) {
			driver := &toolStepDriverStub{readOnly: map[string]bool{required: true}}
			_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
				ThreadID: "foreground-thread", TurnID: "foreground-turn", Workspace: t.TempDir(),
				EffectiveMaxModelSteps: 1, ToolCalls: calls,
				ToolSchemas:     []domainmodel.ToolSchema{toolcatalogapp.ForegroundSubmitToolSchema()},
				AdvertisedTools: advertisedToolNames(required), RequiredFinalToolName: required, Driver: driver,
			})
			var failure TurnFailureError
			if !errors.As(err, &failure) || failure.Code != "subagent_required_final_tool_invalid" {
				t.Fatalf("invalid required final shape was not rejected: %v", err)
			}
			if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 {
				t.Fatalf("invalid required final call produced effects: %#v", driver)
			}
		})
	}

	for name, executionFails := range map[string]bool{"accepted": false, "tool rejected": true} {
		t.Run(name, func(t *testing.T) {
			driver := &toolStepDriverStub{
				readOnly: map[string]bool{required: true}, executeError: map[string]bool{required: executionFails},
			}
			result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
				ThreadID: "foreground-thread", TurnID: "foreground-turn", Workspace: t.TempDir(),
				EffectiveMaxModelSteps: 1,
				ToolCalls:              []domainmodel.ToolCall{{ID: loopTestHostToolCallID("submit"), Name: required, Arguments: json.RawMessage(`{"result":"bounded result"}`)}},
				ToolSchemas:            []domainmodel.ToolSchema{toolcatalogapp.ForegroundSubmitToolSchema()},
				AdvertisedTools:        advertisedToolNames(required), RequiredFinalToolName: required, Driver: driver,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.RequiredFinalToolAccepted == executionFails || result.RequiredFinalToolRejected != executionFails {
				t.Fatalf("required final settlement drifted: %#v", result)
			}
		})
	}
}

type toolStepDriverStub struct {
	parallel           map[string]bool
	readOnly           map[string]bool
	executed           []string
	batches            [][]string
	batchAttempts      [][]string
	ready              []string
	approvals          []string
	userInputs         []string
	pendingPrompts     []string
	pendingMessages    [][]domainmodel.Message
	pendingRefs        [][]domainsecurity.SettledToolReference
	pendings           []appmodel.PendingToolCall
	loopGuards         []string
	preflight          map[string]toolStepPreflight
	executeError       map[string]bool
	grants             []domainsecurity.ExecutionGrant
	batchActions       []string
	batchReady         []int
	batchError         error
	batchErrorByTool   map[string]error
	batchExecuteErr    error
	batchExecuteByTool map[string]error
	batchProjection    map[string]domaintoolresult.PublicToolResultProjectionV1
	batchSettleErr     error
	batchHook          func()
	continuations      map[string]ParentContinuationAuthority
	admissionError     error
	admissionHook      func(context.Context, appmodel.PendingToolCall) error
}

type toolStepPreflight struct {
	output  any
	isError bool
	blocked bool
}

func (stub *toolStepDriverStub) AcquireToolCallAdmission(ctx context.Context, pending appmodel.PendingToolCall) (context.Context, func(), error) {
	if stub.admissionError != nil {
		return ctx, nil, stub.admissionError
	}
	if stub.admissionHook != nil {
		if err := stub.admissionHook(ctx, pending); err != nil {
			return ctx, nil, err
		}
	}
	return ctx, func() {}, nil
}

func (stub *toolStepDriverStub) PersistToolCallReady(_ context.Context, _, _ string, call domainmodel.ToolCall, _ int, _ domainsecurity.TurnSecurityContext, grant domainsecurity.ExecutionGrant) (string, error) {
	stub.ready = append(stub.ready, call.Name)
	stub.grants = append(stub.grants, grant)
	return "item_" + call.ID, nil
}

func (stub *toolStepDriverStub) PersistToolResultAndMessage(_ context.Context, pending appmodel.PendingToolCall, output any, isError bool) (domainmodel.Message, error) {
	prefix := "ok:"
	if isError {
		prefix = "err:"
	}
	return domainmodel.Message{
		Role:       "tool",
		Name:       pending.Call.Name,
		ToolCallID: pending.Call.ID,
		Content:    prefix + stringOutput(output),
	}, nil
}

func (stub *toolStepDriverStub) RequestUserInput(_ context.Context, pending appmodel.PendingToolCall) (string, error) {
	stub.pendings = append(stub.pendings, pending)
	stub.userInputs = append(stub.userInputs, pending.Call.Name)
	stub.pendingPrompts = append(stub.pendingPrompts, pending.Prompt)
	stub.pendingMessages = append(stub.pendingMessages, append([]domainmodel.Message(nil), pending.Messages...))
	stub.pendingRefs = append(stub.pendingRefs, append([]domainsecurity.SettledToolReference(nil), pending.PriorSettledToolRefs...))
	return "input_" + pending.Call.ID, nil
}

func (stub *toolStepDriverStub) RequestApproval(_ context.Context, pending appmodel.PendingToolCall) (string, error) {
	stub.pendings = append(stub.pendings, pending)
	stub.approvals = append(stub.approvals, pending.Call.Name)
	stub.pendingPrompts = append(stub.pendingPrompts, pending.Prompt)
	stub.pendingMessages = append(stub.pendingMessages, append([]domainmodel.Message(nil), pending.Messages...))
	stub.pendingRefs = append(stub.pendingRefs, append([]domainsecurity.SettledToolReference(nil), pending.PriorSettledToolRefs...))
	return "appr_" + pending.Call.ID, nil
}

func (stub *toolStepDriverStub) ToolPolicy(toolName string) (bool, bool) {
	if stub.readOnly != nil {
		return stub.readOnly[toolName], stub.parallel[toolName]
	}
	return stub.parallel[toolName], stub.parallel[toolName]
}

func (stub *toolStepDriverStub) Preflight(pending appmodel.PendingToolCall) (any, bool, bool) {
	if stub.preflight == nil {
		return nil, false, false
	}
	decision := stub.preflight[pending.Call.Name]
	return decision.output, decision.isError, decision.blocked
}

func (stub *toolStepDriverStub) ExecuteAndSettle(ctx context.Context, pending appmodel.PendingToolCall, override any, transform ToolOutputTransform) (SettledToolExecution, error) {
	stub.executed = append(stub.executed, pending.Call.Name)
	output, isError := any("exec:"+pending.Call.Name), stub.executeError[pending.Call.Name]
	if override != nil {
		output, isError = override, false
	}
	var err error
	if transform != nil {
		output, isError, err = transform(output, isError)
		if err != nil {
			return SettledToolExecution{Output: output, IsError: isError}, err
		}
	}
	message, err := stub.PersistToolResultAndMessage(ctx, pending, output, isError)
	return SettledToolExecution{Output: output, IsError: isError, Message: message, ParentContinuation: stub.continuations[pending.Call.Name]}, err
}

func (stub *toolStepDriverStub) ExecuteAuthorizedReadOnlyBatch(_ context.Context, calls []appmodel.PendingToolCall) (string, []domainmodel.Message, error) {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Call.Name)
	}
	stub.batchAttempts = append(stub.batchAttempts, append([]string(nil), names...))
	if stub.batchError != nil {
		return "", nil, stub.batchError
	}
	if err := stub.batchErrorByTool[calls[0].Call.Name]; err != nil {
		return "", nil, err
	}
	stub.batchActions = append(stub.batchActions, "persist:"+calls[0].Call.ID)
	stub.batchReady = append(stub.batchReady, len(stub.ready))
	messages := make([]domainmodel.Message, 0, len(calls))
	for _, call := range calls {
		message := domainmodel.Message{
			Role:       "tool",
			Name:       call.Call.Name,
			ToolCallID: call.Call.ID,
			Content:    "batch:" + call.Call.Name,
		}
		if projection, ok := stub.batchProjection[call.Call.Name]; ok {
			encoded, _ := json.Marshal(domaintoolresult.PublicToolResultProjectionRecordV1(projection))
			message.Content = string(encoded)
		}
		messages = append(messages, message)
	}
	if len(names) > 0 {
		stub.batches = append(stub.batches, names)
	}
	if stub.batchHook != nil {
		stub.batchHook()
	}
	executeErr := stub.batchExecuteErr
	if err := stub.batchExecuteByTool[calls[0].Call.Name]; err != nil {
		executeErr = err
	}
	return "batch_" + calls[0].Call.ID, messages, executeErr
}

func (stub *toolStepDriverStub) SettleToolBatchAuthority(_ context.Context, batchID string, _ domainsecurity.TurnSecurityContext, _ []appmodel.PendingToolCall, status, reason string) error {
	stub.batchActions = append(stub.batchActions, "settle:"+batchID+":"+status+":"+reason)
	return stub.batchSettleErr
}

func (stub *toolStepDriverStub) RecordLoopGuard(_, _, toolName string, count int, guardKind string) error {
	stub.loopGuards = append(stub.loopGuards, toolName+":"+guardKind+":"+string(rune('0'+count)))
	return nil
}

func TestRunToolStepFlushesReadOnlyBeforeSerialWrite(t *testing.T) {
	driver := &toolStepDriverStub{parallel: map[string]bool{"read": true, "grep": true}}
	result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		Workspace:              "/tmp/work",
		ApprovalPolicy:         "auto",
		SandboxMode:            "workspace-write",
		EffectiveMaxModelSteps: 4,
		Messages:               []domainmodel.Message{{Role: "assistant", Content: "tools"}},
		ToolCalls: []domainmodel.ToolCall{
			{ID: "call_1", Name: "read"},
			{ID: "call_2", Name: "grep"},
			{ID: "call_3", Name: "write"},
		},
		ToolSchemas:     zeroArgumentToolSchemas("read", "grep", "write"),
		AdvertisedTools: advertisedToolNames("read", "grep", "write"),
		State:           ToolStepState{},
		Driver:          driver,
	})
	if err != nil {
		t.Fatalf("run tool step: %v", err)
	}
	if got := toolMessageContents(result.Messages[1:]); !reflect.DeepEqual(got, []string{"batch:read", "batch:grep", "ok:exec:write"}) {
		t.Fatalf("tool messages should keep batch-before-write order: %#v", got)
	}
	if !reflect.DeepEqual(driver.batches, [][]string{{"read", "grep"}}) {
		t.Fatalf("read-only batch mismatch: %#v", driver.batches)
	}
	firstCallID := loopTestHostToolCallID("call_1")
	if !reflect.DeepEqual(driver.batchActions, []string{"persist:" + firstCallID, "settle:batch_" + firstCallID + ":completed:batch_completed"}) {
		t.Fatalf("batch authority lifecycle mismatch: %#v", driver.batchActions)
	}
	if !reflect.DeepEqual(driver.batchReady, []int{2}) {
		t.Fatalf("serial grant entered prior batch registry prefix: %#v", driver.batchReady)
	}
	if !reflect.DeepEqual(driver.executed, []string{"write"}) {
		t.Fatalf("serial executions mismatch: %#v", driver.executed)
	}
	if got := settledResultItemIDs(result.SettledToolReferences); !reflect.DeepEqual(got, []string{
		toolcatalogapp.ToolResultItemID("turn_1", loopTestHostToolCallID("call_1")),
		toolcatalogapp.ToolResultItemID("turn_1", loopTestHostToolCallID("call_2")),
		toolcatalogapp.ToolResultItemID("turn_1", loopTestHostToolCallID("call_3")),
	}) {
		t.Fatalf("durable settled references mismatch: %#v", got)
	}
}

func TestRunToolStepPartitionsMixedReadOnlyEffectsAndRestoresProviderOrder(t *testing.T) {
	const fundsTool = "mcp__analytix_funds__count_case_rows"
	readID := loopTestHostToolCallID("mixed-read")
	fundsID := loopTestHostToolCallID("mixed-funds")
	grepID := loopTestHostToolCallID("mixed-grep")
	newInput := func(t *testing.T, driver *toolStepDriverStub) ToolStepInput {
		t.Helper()
		workspace := "/tmp/analytix-mixed-batch-test"
		securityContext := newLoopCaseContextV2(t, "thread-mixed-batch", "turn-mixed-batch", workspace, "case-mixed-batch")
		return ToolStepInput{
			ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
			ProviderID: "provider-mixed-batch", Workspace: workspace, EffectiveMaxModelSteps: 3,
			ToolCalls: []domainmodel.ToolCall{
				{ID: readID, Name: "read", Arguments: json.RawMessage(`{}`)},
				{ID: fundsID, Name: fundsTool, Arguments: json.RawMessage(`{}`)},
				{ID: grepID, Name: "grep", Arguments: json.RawMessage(`{}`)},
			},
			ToolSchemas:     zeroArgumentToolSchemas("read", fundsTool, "grep"),
			AdvertisedTools: advertisedToolNames("read", fundsTool, "grep"),
			LiveMCPTools:    advertisedToolNames(fundsTool),
			MCPConnectionEpochs: map[string]uint64{
				fundsTool: 3,
			},
			MCPServerIdentities: map[string]string{
				fundsTool: loopTestMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 3),
			},
			MCPReadOnlyPolicies: map[string]bool{fundsTool: true},
			SecurityContext:     securityContext,
			Driver:              driver,
		}
	}
	newDriver := func() *toolStepDriverStub {
		return &toolStepDriverStub{
			parallel: map[string]bool{"read": true, fundsTool: true, "grep": true},
			readOnly: map[string]bool{"read": true, fundsTool: true, "grep": true},
		}
	}

	t.Run("successful partitions", func(t *testing.T) {
		driver := newDriver()
		result, err := RunToolStep(context.Background(), newInput(t, driver))
		if err != nil {
			t.Fatal(err)
		}
		wantPartitions := [][]string{{"read", "grep"}, {fundsTool}}
		if !reflect.DeepEqual(driver.batchAttempts, wantPartitions) || !reflect.DeepEqual(driver.batches, wantPartitions) {
			t.Fatalf("mixed effects were not dispatched as ordinary-first homogeneous batches: attempts=%#v batches=%#v", driver.batchAttempts, driver.batches)
		}
		if got := toolMessageContents(result.Messages); !reflect.DeepEqual(got, []string{"batch:read", "batch:" + fundsTool, "batch:grep"}) {
			t.Fatalf("partitioned results did not return to provider call order: %#v", got)
		}
		if got := settledResultItemIDs(result.SettledToolReferences); !reflect.DeepEqual(got, []string{
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", readID),
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", fundsID),
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", grepID),
		}) {
			t.Fatalf("partitioned settled references lost provider call order: %#v", got)
		}
	})

	t.Run("protected authority failure preserves ordinary results", func(t *testing.T) {
		blocked := errors.New("protected batch authority unavailable")
		driver := newDriver()
		driver.batchErrorByTool = map[string]error{fundsTool: blocked}
		result, err := RunToolStep(context.Background(), newInput(t, driver))
		if !errors.Is(err, blocked) {
			t.Fatalf("protected batch failure was hidden: %v", err)
		}
		if !reflect.DeepEqual(driver.batchAttempts, [][]string{{"read", "grep"}, {fundsTool}}) ||
			!reflect.DeepEqual(driver.batches, [][]string{{"read", "grep"}}) {
			t.Fatalf("protected failure affected ordinary batch execution: attempts=%#v batches=%#v", driver.batchAttempts, driver.batches)
		}
		if got := toolMessageContents(result.Messages); !reflect.DeepEqual(got, []string{"batch:read", "batch:grep"}) {
			t.Fatalf("protected failure discarded or reordered ordinary results: %#v", got)
		}
		if got := settledResultItemIDs(result.SettledToolReferences); !reflect.DeepEqual(got, []string{
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", readID),
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", grepID),
		}) {
			t.Fatalf("protected failure discarded ordinary settlement authority: %#v", got)
		}
		if !reflect.DeepEqual(driver.batchActions, []string{
			"persist:" + readID,
			"settle:batch_" + readID + ":completed:batch_completed",
		}) {
			t.Fatalf("protected failure rewrote the completed ordinary authority lifecycle: %#v", driver.batchActions)
		}
	})

	t.Run("protected execution failure closes only protected authority as failed", func(t *testing.T) {
		failed := errors.New("protected batch execution failed")
		driver := newDriver()
		driver.batchExecuteByTool = map[string]error{fundsTool: failed}
		result, err := RunToolStep(context.Background(), newInput(t, driver))
		if !errors.Is(err, failed) {
			t.Fatalf("protected batch execution failure was hidden: %v", err)
		}
		if got := toolMessageContents(result.Messages); !reflect.DeepEqual(got, []string{"batch:read", "batch:grep"}) {
			t.Fatalf("failed protected authority discarded or reordered completed ordinary results: %#v", got)
		}
		if got := settledResultItemIDs(result.SettledToolReferences); !reflect.DeepEqual(got, []string{
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", readID),
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", grepID),
		}) {
			t.Fatalf("failed protected authority entered continuation references or discarded ordinary authority: %#v", got)
		}
		if !reflect.DeepEqual(driver.batchActions, []string{
			"persist:" + readID,
			"settle:batch_" + readID + ":completed:batch_completed",
			"persist:" + fundsID,
			"settle:batch_" + fundsID + ":failed:batch_execution_failed",
		}) {
			t.Fatalf("protected failure misclassified a batch disposition: %#v", driver.batchActions)
		}
	})

	t.Run("protected-first unavailable source executes ordinary partition", func(t *testing.T) {
		driver := newDriver()
		input := newInput(t, driver)
		input.OrdinaryWork = true
		input.ToolCalls = []domainmodel.ToolCall{
			{ID: fundsID, Name: fundsTool, Arguments: json.RawMessage(`{}`)},
			{ID: readID, Name: "read", Arguments: json.RawMessage(`{}`)},
			{ID: grepID, Name: "grep", Arguments: json.RawMessage(`{}`)},
		}
		input.LiveMCPTools = map[string]bool{}
		result, err := RunToolStep(context.Background(), input)
		var failure TurnFailureError
		if !errors.As(err, &failure) || failure.Code != "tool_source_unavailable" {
			t.Fatalf("protected source failure was not preserved exactly: %T %v", err, err)
		}
		if result.ProtectedLaneUnavailable == nil || result.ProtectedLaneUnavailable.Code != "tool_source_unavailable" {
			t.Fatalf("protected source failure did not set the typed partial signal: %#v", result)
		}
		if !reflect.DeepEqual(driver.batchAttempts, [][]string{{"read", "grep"}}) ||
			!reflect.DeepEqual(driver.batches, [][]string{{"read", "grep"}}) {
			t.Fatalf("protected-first failure preempted ordinary execution: attempts=%#v batches=%#v", driver.batchAttempts, driver.batches)
		}
		if got := toolMessageContents(result.Messages); !reflect.DeepEqual(got, []string{"batch:read", "batch:grep"}) {
			t.Fatalf("protected-first failure discarded ordinary results: %#v", got)
		}
	})

	t.Run("recoverable protected authority error preserves partial result and exact code", func(t *testing.T) {
		driver := newDriver()
		driver.batchErrorByTool = map[string]error{
			fundsTool: executiongrantapp.ValidationError{Code: "execution_grant_connection_epoch_mismatch"},
		}
		input := newInput(t, driver)
		input.OrdinaryWork = true
		result, err := RunToolStep(context.Background(), input)
		code, recoverable := RecoverableProtectedLaneFailureCodeV1(err)
		if !recoverable || code != "execution_grant_connection_epoch_mismatch" {
			t.Fatalf("exact protected authority error was lost: code=%q recoverable=%t err=%v", code, recoverable, err)
		}
		if result.ProtectedLaneUnavailable == nil || result.ProtectedLaneUnavailable.Code != code {
			t.Fatalf("partial signal did not retain the exact host code: %#v", result.ProtectedLaneUnavailable)
		}
		if got := toolMessageContents(result.Messages); !reflect.DeepEqual(got, []string{"batch:read", "batch:grep"}) {
			t.Fatalf("recoverable protected failure discarded or reordered ordinary results: %#v", got)
		}
	})

	t.Run("mixed unsafe failure remains fatal", func(t *testing.T) {
		driver := newDriver()
		unsafe := errors.Join(
			executiongrantapp.ValidationError{Code: "execution_grant_connection_epoch_mismatch"},
			errors.New("batch signer integrity failure"),
		)
		driver.batchErrorByTool = map[string]error{fundsTool: unsafe}
		input := newInput(t, driver)
		input.OrdinaryWork = true
		result, err := RunToolStep(context.Background(), input)
		if !errors.Is(err, unsafe) {
			t.Fatalf("unsafe protected failure was hidden: %v", err)
		}
		if _, recoverable := RecoverableProtectedLaneFailureCodeV1(err); recoverable || result.ProtectedLaneUnavailable != nil {
			t.Fatalf("mixed unsafe failure was misclassified as recoverable: %#v", result.ProtectedLaneUnavailable)
		}
	})

	t.Run("settled public source error reaches provider and sets sticky signal", func(t *testing.T) {
		driver := newDriver()
		driver.batchProjection = map[string]domaintoolresult.PublicToolResultProjectionV1{
			fundsTool: {
				SchemaVersion:  domaintoolresult.PublicProjectionSchemaVersion,
				ProjectionKind: domaintoolresult.ProjectionHostStatus,
				Disclosure:     domaintoolresult.MetadataOnlyDisclosure, MessageKey: "tool_blocked",
				Status: "blocked", Code: "tool_source_unavailable", PrivatePayloadWithheld: true,
			},
		}
		input := newInput(t, driver)
		input.OrdinaryWork = true
		result, err := RunToolStep(context.Background(), input)
		if err != nil {
			t.Fatalf("settled protected tool error was incorrectly promoted to a Go failure: %v", err)
		}
		if result.ProtectedLaneUnavailable == nil || result.ProtectedLaneUnavailable.Code != "tool_source_unavailable" {
			t.Fatalf("settled protected tool error did not set the typed signal: %#v", result.ProtectedLaneUnavailable)
		}
		if len(result.Messages) != 3 || result.Messages[0].Name != "read" || result.Messages[1].Name != fundsTool || result.Messages[2].Name != "grep" {
			t.Fatalf("typed protected tool result did not reach the provider in original order: %#v", result.Messages)
		}
		if code, ok := RecoverableProtectedLaneToolMessageCodeV1(result.Messages[1]); !ok || code != "tool_source_unavailable" {
			t.Fatalf("provider result was not the strict public projection: code=%q ok=%t message=%#v", code, ok, result.Messages[1])
		}
		if got := settledResultItemIDs(result.OrdinarySettledToolReferences); !reflect.DeepEqual(got, []string{
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", readID),
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", grepID),
		}) {
			t.Fatalf("ordinary continuation references retained protected settlement authority: %#v", got)
		}
		if got := settledResultItemIDs(result.SettledToolReferences); !reflect.DeepEqual(got, []string{
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", readID),
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", fundsID),
			toolcatalogapp.ToolResultItemID("turn-mixed-batch", grepID),
		}) {
			t.Fatalf("typed provider result lost its protected settlement reference: %#v", got)
		}
	})
}

func TestRunToolStepRejectsProviderRawToolCallIdentityBeforePersistence(t *testing.T) {
	const raw = "provider_call_6222020202020202020"
	driver := &toolStepDriverStub{}
	securityContext := newLoopGeneralContextV2(t, "thread-raw-call-id", "turn-raw-call-id", t.TempDir())
	_, err := RunToolStep(context.Background(), ToolStepInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, ProviderID: "provider-raw-id",
		Workspace: securityContext.WorkspaceRealPath, EffectiveMaxModelSteps: 1,
		ToolCalls:       []domainmodel.ToolCall{{ID: raw, Name: "read", Arguments: json.RawMessage(`{}`)}},
		ToolSchemas:     zeroArgumentToolSchemas("read"),
		AdvertisedTools: advertisedToolNames("read"),
		SecurityContext: securityContext,
		Driver:          driver,
	})
	var failure TurnFailureError
	encoded, _ := json.Marshal(err)
	if !errors.As(err, &failure) || failure.Code != "tool_call_identity_invalid" ||
		len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 ||
		strings.Contains(err.Error(), raw) || strings.Contains(string(encoded), "6222020202020202020") {
		t.Fatalf("provider raw tool-call identity crossed admission: failure=%#v driver=%#v", failure, driver)
	}
}

func TestContextTransitionWinsBeforeGrantReadyLeavesNoDurableGrant(t *testing.T) {
	driver := &toolStepDriverStub{
		admissionError: executiongrantapp.ValidationError{Code: "execution_grant_context_mismatch"},
	}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thr_stale", TurnID: "turn_stale", Workspace: "/tmp/work",
		ProviderID: "provider_stale", EffectiveMaxModelSteps: 2,
		ToolCalls:       []domainmodel.ToolCall{{ID: "call_stale", Name: "read", Arguments: []byte(`{}`)}},
		ToolSchemas:     zeroArgumentToolSchemas("read"),
		AdvertisedTools: advertisedToolNames("read"),
		Driver:          driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "execution_grant_context_mismatch" {
		t.Fatalf("stale tool admission did not return the exact host blocker: %v", err)
	}
	if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.approvals) != 0 || len(driver.userInputs) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
		t.Fatalf("stale admission produced durable or executable effects: %#v", driver)
	}
}

func TestToolBatchAuthorityPersistsBeforeParallelExecution(t *testing.T) {
	blocked := errors.New("batch authority unavailable")
	driver := &toolStepDriverStub{
		parallel:   map[string]bool{"read": true, "grep": true},
		batchError: blocked,
	}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thr_1", TurnID: "turn_1", Workspace: "/tmp/work",
		EffectiveMaxModelSteps: 2,
		ToolCalls:              []domainmodel.ToolCall{{ID: "call_1", Name: "read"}, {ID: "call_2", Name: "grep"}},
		ToolSchemas:            zeroArgumentToolSchemas("read", "grep"), AdvertisedTools: advertisedToolNames("read", "grep"),
		Driver: driver,
	})
	if !errors.Is(err, blocked) || len(driver.batches) != 0 || len(driver.executed) != 0 {
		t.Fatalf("batch executed without durable authority: err=%v driver=%#v", err, driver)
	}
	if len(driver.ready) != 2 || len(driver.grants) != 2 {
		t.Fatalf("batch authority must follow complete ready/grant persistence: %#v", driver)
	}
}

func TestToolBatchAuthorityClosesFailedAndJoinsExecutionError(t *testing.T) {
	failed := errors.New("durable batch settlement failed")
	driver := &toolStepDriverStub{parallel: map[string]bool{"read": true, "grep": true}, batchExecuteErr: failed}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thr_1", TurnID: "turn_1", Workspace: "/tmp/work", EffectiveMaxModelSteps: 2,
		ToolCalls:   []domainmodel.ToolCall{{ID: "call_1", Name: "read"}, {ID: "call_2", Name: "grep"}},
		ToolSchemas: zeroArgumentToolSchemas("read", "grep"), AdvertisedTools: advertisedToolNames("read", "grep"), Driver: driver,
	})
	if !errors.Is(err, failed) || !reflect.DeepEqual(driver.batchActions, []string{
		"persist:" + loopTestHostToolCallID("call_1"), "settle:batch_" + loopTestHostToolCallID("call_1") + ":failed:batch_execution_failed",
	}) {
		t.Fatalf("failed batch authority was not closed: err=%v actions=%#v", err, driver.batchActions)
	}
}

func TestToolBatchAuthorityClosesCancelledWithoutProviderReferences(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	driver := &toolStepDriverStub{parallel: map[string]bool{"read": true, "grep": true}, batchHook: cancel}
	result, err := runSecureToolStep(t, ctx, ToolStepInput{
		ThreadID: "thr_1", TurnID: "turn_1", Workspace: "/tmp/work", EffectiveMaxModelSteps: 2,
		ToolCalls:   []domainmodel.ToolCall{{ID: "call_1", Name: "read"}, {ID: "call_2", Name: "grep"}},
		ToolSchemas: zeroArgumentToolSchemas("read", "grep"), AdvertisedTools: advertisedToolNames("read", "grep"), Driver: driver,
	})
	if !errors.Is(err, context.Canceled) || len(result.SettledToolReferences) != 0 || !reflect.DeepEqual(driver.batchActions, []string{
		"persist:" + loopTestHostToolCallID("call_1"), "settle:batch_" + loopTestHostToolCallID("call_1") + ":cancelled:batch_cancelled",
	}) {
		t.Fatalf("cancelled batch became continuation authority: err=%v result=%#v actions=%#v", err, result, driver.batchActions)
	}
}

func TestCaseContextKeepsOrdinaryMutationAndShellAvailable(t *testing.T) {
	for _, toolName := range []string{"write_file", "bash"} {
		t.Run(toolName, func(t *testing.T) {
			driver := &toolStepDriverStub{}
			workspace := "/tmp/analytix-case-tool-step-test"
			securityContext := newLoopCaseContextV2(t, "thr_case", "turn_case", workspace, "case-a")
			input := ToolStepInput{
				ThreadID: "thr_case", TurnID: "turn_case", ProviderID: "provider_test", Workspace: workspace,
				ApprovalPolicy: "auto", SandboxMode: "danger-full-access", EffectiveMaxModelSteps: 2,
				Messages:    []domainmodel.Message{{Role: "assistant", Content: "ordinary workspace work"}},
				ToolCalls:   []domainmodel.ToolCall{{ID: loopTestHostToolCallID("call-ordinary"), Name: toolName, Arguments: json.RawMessage(`{}`)}},
				ToolSchemas: zeroArgumentToolSchemas(toolName), AdvertisedTools: advertisedToolNames(toolName),
				SecurityContext: securityContext, Driver: driver,
			}
			result, err := RunToolStep(context.Background(), input)
			if err != nil {
				t.Fatalf("case context replaced ordinary tool %s: %v", toolName, err)
			}
			if len(driver.ready) != 1 || len(driver.grants) != 1 || len(driver.approvals) != 0 ||
				!reflect.DeepEqual(driver.executed, []string{toolName}) || len(driver.batches) != 0 ||
				len(result.Messages) != 2 || len(result.SettledToolReferences) != 1 {
				t.Fatalf("ordinary tool %s did not follow its normal grant/effect/result path: driver=%#v result=%#v", toolName, driver, result)
			}
		})
	}
}

func TestLegacyCaseArtifactMCPStillFailsBeforeGrantPersistenceOrExecution(t *testing.T) {
	for _, toolName := range []string{
		"mcp__spoofed__run_full_case_analysis",
		"mcp__spoofed__create_case_notebook",
		"mcp__spoofed__export_cleaned_case_data",
	} {
		t.Run(toolName, func(t *testing.T) {
			driver := &toolStepDriverStub{}
			workspace := "/tmp/analytix-case-tool-step-test"
			securityContext := newLoopCaseContextV2(t, "thr_case", "turn_case", workspace, "case-a")
			_, err := RunToolStep(context.Background(), ToolStepInput{
				ThreadID: "thr_case", TurnID: "turn_case", ProviderID: "provider_test", Workspace: workspace,
				ApprovalPolicy: "auto", SandboxMode: "danger-full-access", EffectiveMaxModelSteps: 2,
				Messages:    []domainmodel.Message{{Role: "assistant", Content: "UNSUPPORTED_CASE_FACT"}},
				ToolCalls:   []domainmodel.ToolCall{{ID: loopTestHostToolCallID("call-artifact"), Name: toolName, Arguments: json.RawMessage(`{}`)}},
				ToolSchemas: zeroArgumentToolSchemas(toolName), AdvertisedTools: advertisedToolNames(toolName),
				LiveMCPTools: map[string]bool{toolName: true}, MCPConnectionEpochs: map[string]uint64{toolName: 3},
				SecurityContext: securityContext, Driver: driver,
			})
			var failure TurnFailureError
			if !errors.As(err, &failure) || failure.Code != "publication_receipt_required" {
				t.Fatalf("legacy case artifact MCP did not fail at the host boundary: %T %v", err, err)
			}
			if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.approvals) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
				t.Fatalf("legacy case artifact MCP entered a durable or executable path: %#v", driver)
			}
		})
	}
}

func TestCaseContextWritableSubagentUsesNormalToolPolicy(t *testing.T) {
	t.Run("writable subagent", func(t *testing.T) {
		workspace := "/tmp/analytix-case-tool-step-test"
		securityContext := newLoopCaseContextV2(t, "thr_case", "turn_case", workspace, "case-a")
		call := domainmodel.ToolCall{ID: loopTestHostToolCallID("call-task"), Name: "task", Arguments: json.RawMessage(`{"prompt":"write report","toolPolicy":"inherit"}`)}
		capability := continuationCapabilityStub{
			parent: securityContext, callID: call.ID, digest: domainsecurity.SHA256Hex([]byte("case-context-task-receipt")), allow: true,
		}
		driver := &toolStepDriverStub{continuations: map[string]ParentContinuationAuthority{
			"task": NewParentContinuationAuthority(1, []ParentContinuationCapability{capability}),
		}}
		result, err := RunToolStep(context.Background(), ToolStepInput{
			ThreadID: "thr_case", TurnID: "turn_case", ProviderID: "provider_test", Workspace: workspace,
			ApprovalPolicy: "auto", SandboxMode: "danger-full-access", EffectiveMaxModelSteps: 2,
			Messages: []domainmodel.Message{{Role: "assistant", Content: "ordinary delegated work"}}, ToolCalls: []domainmodel.ToolCall{call},
			ToolSchemas:     []domainmodel.ToolSchema{{Name: "task", Parameters: json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"toolPolicy":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`)}},
			AdvertisedTools: advertisedToolNames("task"), SecurityContext: securityContext, Driver: driver,
		})
		if err != nil {
			t.Fatalf("case context replaced a writable subagent authorized by normal policy: %v", err)
		}
		if len(driver.ready) != 1 || len(driver.grants) != 1 || !reflect.DeepEqual(driver.executed, []string{"task"}) ||
			len(result.SettledToolReferences) != 1 || result.ProviderContinuationBlocked {
			t.Fatalf("writable case subagent did not follow the normal tool path: driver=%#v result=%#v", driver, result)
		}
	})
}

func TestAuditOnlyV1SecurityContextFailsBeforeGrantPersistenceOrExecution(t *testing.T) {
	driver := &toolStepDriverStub{}
	workspace := "/tmp/analytix-tool-step-v1-audit-test"
	auditContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_v1_audit", TurnID: "turn_v1_audit", WorkspaceRealPath: workspace,
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: 1,
		IssuedAt: time.Date(2026, 7, 12, 6, 0, 0, 0, time.UTC),
	})
	_, err := RunToolStep(context.Background(), ToolStepInput{
		ThreadID: "thr_v1_audit", TurnID: "turn_v1_audit", ProviderID: "provider_test", Workspace: workspace,
		EffectiveMaxModelSteps: 1,
		ToolCalls:              []domainmodel.ToolCall{{ID: "call_v1_audit", Name: "read", Arguments: json.RawMessage(`{}`)}},
		ToolSchemas:            zeroArgumentToolSchemas("read"),
		AdvertisedTools:        advertisedToolNames("read"),
		SecurityContext:        auditContext,
		Driver:                 driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "turn_security_context_invalid" {
		t.Fatalf("audit-only V1 context must be rejected at execution admission: %T %v", err, err)
	}
	if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 ||
		len(driver.approvals) != 0 || len(driver.userInputs) != 0 || len(driver.batchActions) != 0 {
		t.Fatalf("audit-only V1 context reached persistence or execution: %#v", driver)
	}
}

func TestRunToolStepFlushesBatchBeforeUserInputPause(t *testing.T) {
	driver := &toolStepDriverStub{parallel: map[string]bool{"read": true}}
	result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		Prompt:                 "核验当前案件资金报告",
		EffectiveMaxModelSteps: 2,
		Messages:               []domainmodel.Message{{Role: "assistant", Content: "tools"}},
		ToolCalls: []domainmodel.ToolCall{
			{ID: "call_1", Name: "read"},
			{ID: "call_2", Name: "request_user_input"},
		},
		ToolSchemas:     zeroArgumentToolSchemas("read", "request_user_input"),
		AdvertisedTools: advertisedToolNames("read", "request_user_input"),
		Driver:          driver,
	})
	if err != nil {
		t.Fatalf("run user input step: %v", err)
	}
	if !result.Paused || result.PendingKind != "user_input" || result.PendingID != "input_"+loopTestHostToolCallID("call_2") {
		t.Fatalf("pause result mismatch: %#v", result)
	}
	if got := toolMessageContents(result.Messages[1:]); !reflect.DeepEqual(got, []string{"batch:read"}) {
		t.Fatalf("batch should flush before user input pause: %#v", got)
	}
	if !reflect.DeepEqual(driver.userInputs, []string{"request_user_input"}) {
		t.Fatalf("user input requests mismatch: %#v", driver.userInputs)
	}
	if !reflect.DeepEqual(driver.pendingPrompts, []string{"核验当前案件资金报告"}) {
		t.Fatalf("pending gate lost immutable turn prompt: %#v", driver.pendingPrompts)
	}
	if !reflect.DeepEqual(driver.batchReady, []int{1}) {
		t.Fatalf("user-input grant entered prior batch registry prefix: %#v", driver.batchReady)
	}
	if len(driver.pendingMessages) != 1 || !reflect.DeepEqual(toolMessageContents(driver.pendingMessages[0][1:]), []string{"batch:read"}) {
		t.Fatalf("pending gate lost already-settled batch results: %#v", driver.pendingMessages)
	}
	if got := settledResultItemIDs(result.SettledToolReferences); !reflect.DeepEqual(got, []string{toolcatalogapp.ToolResultItemID("turn_1", loopTestHostToolCallID("call_1"))}) {
		t.Fatalf("pause lost already-settled batch authority: %#v", got)
	}
	if len(driver.pendingRefs) != 1 || !reflect.DeepEqual(settledResultItemIDs(driver.pendingRefs[0]), []string{toolcatalogapp.ToolResultItemID("turn_1", loopTestHostToolCallID("call_1"))}) {
		t.Fatalf("pending gate did not retain batch authority: %#v", driver.pendingRefs)
	}
}

func TestPausedApprovalAndUserInputCarryNoUnleasedAttachmentBytes(t *testing.T) {
	planDigest := domainsecurity.SHA256Hex([]byte("attachment-plan"))
	for _, test := range []struct {
		name     string
		toolName string
		wantKind string
		approval string
		sandbox  string
	}{
		{name: "approval", toolName: "write", wantKind: "approval", approval: "on-request", sandbox: "workspace-write"},
		{name: "user input", toolName: "request_user_input", wantKind: "user_input", approval: "never", sandbox: "workspace-write"},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := &toolStepDriverStub{}
			result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
				ThreadID: "thr_attachment_pause", TurnID: "turn_attachment_pause", ProviderID: "provider_test",
				Prompt: "analyze attachment", AttachmentIDs: []string{"att_0123456789abcdef01234567"}, AttachmentPlanDigest: planDigest,
				ApprovalPolicy: test.approval, SandboxMode: test.sandbox, EffectiveMaxModelSteps: 2,
				Messages: []domainmodel.Message{
					{Role: "user", Content: "analyze attachment", PrivateAttachmentPlanDigest: planDigest},
					{Role: "assistant", Content: "tool requested"},
				},
				ToolCalls:   []domainmodel.ToolCall{{ID: "call_pause", Name: test.toolName}},
				ToolSchemas: zeroArgumentToolSchemas(test.toolName), AdvertisedTools: advertisedToolNames(test.toolName), Driver: driver,
			})
			if err != nil || !result.Paused || result.PendingKind != test.wantKind || len(driver.pendings) != 1 {
				t.Fatalf("attachment pause mismatch: result=%#v pendings=%d err=%v", result, len(driver.pendings), err)
			}
			pending := driver.pendings[0]
			if !reflect.DeepEqual(pending.AttachmentIDs, []string{"att_0123456789abcdef01234567"}) || pending.AttachmentPlanDigest != planDigest {
				t.Fatalf("pending gate lost safe attachment authority: %#v", pending)
			}
			for _, message := range pending.Messages {
				for _, part := range message.Parts {
					if part.Text != "" || part.Data != "" || part.ImageURL != "" {
						t.Fatalf("pending gate retained unleased attachment payload: %#v", pending.Messages)
					}
				}
			}
		})
	}
}

func TestRunToolStepRejectsCancellationBeforePublicToolPersistence(t *testing.T) {
	driver := &toolStepDriverStub{parallel: map[string]bool{"read": true}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runSecureToolStep(t, ctx, ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		EffectiveMaxModelSteps: 2,
		Messages:               []domainmodel.Message{{Role: "assistant", Content: "tools"}},
		ToolCalls:              []domainmodel.ToolCall{{ID: "call_1", Name: "read"}},
		ToolSchemas:            zeroArgumentToolSchemas("read"),
		AdvertisedTools:        advertisedToolNames("read"),
		Driver:                 driver,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled step did not stop before admission: %v", err)
	}
	if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
		t.Fatalf("cancelled tool reached persistence or execution: %#v", driver)
	}
}

func TestRunToolStepSettlesSandboxBlockedShellBeforeApprovalOrExecution(t *testing.T) {
	driver := &toolStepDriverStub{
		preflight: map[string]toolStepPreflight{
			"bash": {
				output:  map[string]any{"code": "sandbox_blocked", "error": "bash requires danger-full-access"},
				isError: true,
				blocked: true,
			},
		},
	}
	result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		ApprovalPolicy:         "on-request",
		SandboxMode:            "workspace-write",
		EffectiveMaxModelSteps: 2,
		Messages:               []domainmodel.Message{{Role: "assistant", Content: "tools"}},
		ToolCalls:              []domainmodel.ToolCall{{ID: "call_1", Name: "bash"}},
		ToolSchemas:            zeroArgumentToolSchemas("bash"),
		AdvertisedTools:        advertisedToolNames("bash"),
		Driver:                 driver,
	})
	if err != nil {
		t.Fatalf("sandbox rejection should settle as a tool result: %v", err)
	}
	if result.Paused || len(driver.approvals) != 0 || len(driver.executed) != 0 {
		t.Fatalf("blocked shell must not request approval or execute: result=%#v approvals=%#v executed=%#v", result, driver.approvals, driver.executed)
	}
	if got := toolMessageContents(result.Messages[1:]); len(got) != 1 || !strings.Contains(got[0], "danger-full-access") {
		t.Fatalf("sandbox rejection result mismatch: %#v", got)
	}
}

func TestRunToolStepPropagatesLoopGuardErrors(t *testing.T) {
	driver := &toolStepDriverStub{
		preflight: map[string]toolStepPreflight{
			"write": {output: map[string]any{"code": "validation_error", "error": "same"}, isError: true, blocked: true},
		},
	}
	result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		ApprovalPolicy:         "auto",
		SandboxMode:            "workspace-write",
		EffectiveMaxModelSteps: 2,
		Messages:               []domainmodel.Message{{Role: "assistant", Content: "tools"}},
		ToolCalls: []domainmodel.ToolCall{
			{ID: "call_1", Name: "write"},
			{ID: "call_2", Name: "write"},
		},
		ToolSchemas:     zeroArgumentToolSchemas("write"),
		AdvertisedTools: advertisedToolNames("write"),
		State:           ToolStepState{FailureStormSignature: "write\x00validation_error\x00\x00same", FailureStormCount: 2},
		Driver:          driver,
	})
	if err != nil {
		t.Fatalf("run guarded step: %v", err)
	}
	if result.State.FailureStormCount <= 2 || len(driver.loopGuards) == 0 {
		t.Fatalf("expected loop guard to update state and record event: state=%#v guards=%#v", result.State, driver.loopGuards)
	}
}

func TestInvalidToolArgumentsNeverPersistBeforeValidation(t *testing.T) {
	driver := &toolStepDriverStub{}
	result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		ApprovalPolicy:         "auto",
		SandboxMode:            "workspace-write",
		EffectiveMaxModelSteps: 2,
		Messages:               []domainmodel.Message{{Role: "assistant", Content: "tools"}},
		ToolCalls:              []domainmodel.ToolCall{{ID: "call_1", Name: "task", Arguments: json.RawMessage(`{}`)}},
		ToolSchemas: []domainmodel.ToolSchema{{
			Name:       "task",
			Parameters: json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"]}`),
		}},
		AdvertisedTools: advertisedToolNames("task"),
		State:           ToolStepState{},
		Driver:          driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "validation_error" {
		t.Fatalf("missing required arguments must fail closed before persistence: %T %v", err, err)
	}
	if len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
		t.Fatalf("invalid tool call must not persist or execute: %#v", driver)
	}
	if len(result.Messages) != 1 || !strings.Contains(failure.Message, "prompt is required") {
		t.Fatalf("invalid call should preserve only preexisting messages and safe diagnostic: result=%#v failure=%#v", result, failure)
	}
}

func TestRunToolStepImmediatelyRejectsRepeatedEmptyRequiredArguments(t *testing.T) {
	driver := &toolStepDriverStub{}
	result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		ApprovalPolicy:         "auto",
		SandboxMode:            "workspace-write",
		EffectiveMaxModelSteps: 2,
		Messages:               []domainmodel.Message{{Role: "assistant", Content: "tools"}},
		ToolCalls:              []domainmodel.ToolCall{{ID: "call_1", Name: "bash", Arguments: json.RawMessage(`{}`)}},
		ToolSchemas: []domainmodel.ToolSchema{{
			Name:       "bash",
			Parameters: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
		}},
		AdvertisedTools: advertisedToolNames("bash"),
		State: ToolStepState{
			FailureStormSignature: "bash\x00validation_error\x00\x00command is required",
			FailureStormCount:     InvalidToolArgumentsHardStopThreshold - 1,
		},
		Driver: driver,
	})
	if err == nil {
		t.Fatal("expected repeated empty required arguments to hard-stop the turn")
	}
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "validation_error" {
		t.Fatalf("expected immediate validation failure, got %T %v", err, err)
	}
	if len(driver.executed) != 0 {
		t.Fatalf("invalid tool call should not execute: %#v", driver.executed)
	}
	if len(result.Messages) != 1 || len(driver.ready) != 0 || len(driver.loopGuards) != 0 {
		t.Fatalf("invalid arguments must not persist a tool call/result or loop guard: messages=%#v driver=%#v", result.Messages, driver)
	}
}

func TestRunToolStepRejectsDuplicateCreatePlanBeforePersistence(t *testing.T) {
	const marker = "DUPLICATE_CREATE_PLAN_ARGUMENT_MARKER"
	driver := &toolStepDriverStub{}
	result, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:           "thread-duplicate-create-plan",
		TurnID:             "turn-duplicate-create-plan",
		Mode:               "plan",
		CreatePlanToolName: toolcatalogapp.ToolCreatePlanName,
		ToolCalls: []domainmodel.ToolCall{
			{ID: "duplicate-create-plan-1", Name: toolcatalogapp.ToolCreatePlanName, Arguments: json.RawMessage(`{"markdown":"` + marker + ` one","operation":"draft"}`)},
			{ID: "duplicate-create-plan-2", Name: toolcatalogapp.ToolCreatePlanName, Arguments: json.RawMessage(`{"markdown":"` + marker + ` two","operation":"draft"}`)},
		},
		ToolSchemas:     []domainmodel.ToolSchema{toolcatalogapp.CreatePlanToolSchema()},
		AdvertisedTools: advertisedToolNames(toolcatalogapp.ToolCreatePlanName),
		Driver:          driver,
	})
	var failure TurnFailureError
	if err == nil || !errors.As(err, &failure) || failure.Code != "validation_error" ||
		failure.Details["code"] != duplicateCreatePlanValidationCode ||
		failure.Details["callCount"] != float64(2) || failure.Details["executed"] != false {
		t.Fatalf("duplicate create_plan batch did not fail closed: result=%#v err=%v", result, err)
	}
	if strings.Contains(failure.Error(), marker) {
		t.Fatalf("duplicate create_plan arguments escaped validation failure: %q", failure.Error())
	}
	if len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 || len(driver.grants) != 0 || len(result.Messages) != 0 {
		t.Fatalf("duplicate create_plan batch reached persistence or execution: driver=%#v result=%#v", driver, result)
	}
	single, singleErr := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thread-duplicate-create-plan", TurnID: "turn-duplicate-create-plan", Mode: "plan",
		CreatePlanToolName: toolcatalogapp.ToolCreatePlanName,
		ToolCalls: []domainmodel.ToolCall{{
			ID: "single-create-plan", Name: toolcatalogapp.ToolCreatePlanName,
			Arguments: json.RawMessage(`{"markdown":"one plan","operation":"draft"}`),
		}},
		ToolSchemas:     []domainmodel.ToolSchema{toolcatalogapp.CreatePlanToolSchema()},
		AdvertisedTools: advertisedToolNames(toolcatalogapp.ToolCreatePlanName),
		Driver:          driver,
	})
	if singleErr != nil || !single.State.CreatePlanSatisfied ||
		!reflect.DeepEqual(driver.ready, []string{toolcatalogapp.ToolCreatePlanName}) ||
		!reflect.DeepEqual(driver.executed, []string{toolcatalogapp.ToolCreatePlanName}) {
		t.Fatalf("single create_plan retry did not remain executable exactly once: driver=%#v result=%#v err=%v", driver, single, singleErr)
	}
}

func TestPrivateToolArgumentsNeverPersistBeforeValidation(t *testing.T) {
	driver := &toolStepDriverStub{}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:        "thr_1",
		TurnID:          "turn_1",
		ToolCalls:       []domainmodel.ToolCall{{ID: "call_1", Name: "lookup", Arguments: json.RawMessage(`{"query":"<think>PRIVATE_REASONING_SENTINEL</think>"}`)}},
		ToolSchemas:     []domainmodel.ToolSchema{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)}},
		AdvertisedTools: advertisedToolNames("lookup"),
		Driver:          driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "tool_private_arguments" {
		t.Fatalf("private tool arguments must fail closed: %T %v", err, err)
	}
	if len(driver.ready) != 0 || len(driver.executed) != 0 {
		t.Fatalf("private arguments reached persistence or execution: %#v", driver)
	}
}

func TestCreatePlanMarkdownCodeURLDoesNotTripCredentialBoundary(t *testing.T) {
	workspace := "/Volumes/AnalytixCache/development-v3/tmp/fresh-a0-owner/repository"
	relativePath := ".analytixsdd/plan/work-only-in-this-pre-existing-isolated-non-case-code-repository-and-inspect-the-task-through-to.md"
	argumentFields := map[string]any{
		"markdown": "# Correct the README link quotation\n\n" +
			"## Implementation\n\n" +
			"- Remove the duplicated closing quotation mark from the final `README.md` link text.\n" +
			"- Preserve the surrounding prose and the `http://guides.github.com/overviews/forking/` target.\n\n" +
			"## Tests\n\n" +
			"- Run the contract-provided focused README verification.\n",
		"operation":          "draft",
		"source_request":     "Correct the duplicated quotation mark in the final README Markdown link while preserving the surrounding prose and link target.",
		"title":              "Correct README link quotation",
		"plan_id":            workspace + ":" + relativePath,
		"plan_relative_path": relativePath,
	}
	arguments, err := json.Marshal(argumentFields)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAttemptPrivateToolArgumentsV1(domainmodel.ToolCall{
		ID: loopTestHostToolCallID("create-plan-markdown-code-url"), Name: "create_plan", Arguments: arguments,
	}); err != nil {
		t.Fatalf("public plan content with a host-reserved plan identity was rejected: %v", err)
	}
}

func TestWebFetchIPLiteralHostIsInspectedWithoutPublishingTheNetworkAddress(t *testing.T) {
	allowed := domainmodel.ToolCall{
		ID:        "call_web_fetch_private_network_target",
		Name:      "web_fetch",
		Arguments: json.RawMessage(`{"url":"http://169.254.169.254/latest/meta-data/"}`),
	}
	if err := validateAttemptPrivateToolArgumentsV1(allowed); err != nil {
		t.Fatalf("IP-literal web target did not reach the host SSRF boundary: %v", err)
	}

	for name, raw := range map[string]json.RawMessage{
		"private reference in path": json.RawMessage(`{"url":"https://example.test/cer1_` + strings.Repeat("a", 64) + `"}`),
		"reasoning in query":        json.RawMessage(`{"url":"https://example.test/?q=%3Cthink%3Eprivate%3C%2Fthink%3E"}`),
	} {
		t.Run(name, func(t *testing.T) {
			candidate := allowed
			candidate.Arguments = raw
			if err := validateAttemptPrivateToolArgumentsV1(candidate); err == nil {
				t.Fatal("web target carried private material past attempt inspection")
			}
		})
	}
}

func TestForegroundCaseCommitmentTokensPassPrivateToolArgumentInspection(t *testing.T) {
	for name, body := range map[string]string{
		"all-zero-derived":  "a",
		"all-nine-derived":  "j",
		"all-hex-f-derived": "p",
	} {
		t.Run(name, func(t *testing.T) {
			token := "cmt1_" + strings.Repeat(body, 64)
			if err := domainjob.ValidateCaseDelegationProviderCommitmentTokenV1(token); err != nil {
				t.Fatalf("test token is outside the closed provider grammar: %v", err)
			}
			arguments, err := json.Marshal(map[string]any{"caseResult": map[string]any{
				"schemaVersion":    domainjob.CaseForegroundChildResultSchemaVersionV1,
				"purpose":          domainjob.CaseForegroundChildSelectionPurposeV1,
				"delegationDigest": token, "answerSlotDigest": token,
			}})
			if err != nil {
				t.Fatal(err)
			}
			if err := validateAttemptPrivateToolArgumentsV1(domainmodel.ToolCall{
				ID:   loopTestHostToolCallID("provider-safe-case-token-" + name),
				Name: toolcatalogapp.ForegroundSubmitToolName, Arguments: arguments,
			}); err != nil {
				t.Fatalf("provider-safe case selector failed generic tool privacy inspection: %v", err)
			}
		})
	}

	rawAccountShapedDigest := strings.Repeat("9", 64)
	rawArguments, err := json.Marshal(map[string]any{"caseResult": map[string]any{
		"schemaVersion":    domainjob.CaseForegroundChildResultSchemaVersionV1,
		"purpose":          domainjob.CaseForegroundChildSelectionPurposeV1,
		"delegationDigest": rawAccountShapedDigest, "answerSlotDigest": rawAccountShapedDigest,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAttemptPrivateToolArgumentsV1(domainmodel.ToolCall{
		ID:   loopTestHostToolCallID("raw-account-shaped-case-digest"),
		Name: toolcatalogapp.ForegroundSubmitToolName, Arguments: rawArguments,
	}); err == nil {
		t.Fatal("raw account-shaped digest bypassed the generic tool privacy boundary")
	}
}

func TestRunToolStepAdmitsOnlyTheAccountFlowAliasWithHostPrivateProvenance(t *testing.T) {
	const toolName = providerFundsAccountFlowToolNameV1
	workspace := "/tmp/analytix-private-account-flow-arguments"
	securityContext := newLoopCaseContextV2(
		t, "thread-private-account-flow", "turn-private-account-flow", workspace, "case-private-account-flow",
	)
	reference := "cer1_" + strings.Repeat("a", 64)
	selection, err := NewHostCaseEntitySelectionFromPersistedIngressAliasesV1(
		securityContext,
		caseentityapp.PrivateRecordReferenceV1{
			RecordID: domainsecurity.SHA256Hex(
				[]byte("private-account-flow-ingress-record"),
			),
			RecordDigest: domainsecurity.SHA256Hex(
				[]byte("private-account-flow-ingress-record-digest"),
			),
		},
		[]domaincaseentity.ReferenceV1{domaincaseentity.ReferenceV1(reference)},
		[]domaincaseentity.ModelEntityAliasV1{"acct:1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []struct {
		name      string
		arguments json.RawMessage
	}{
		{
			name: "canonical JSON",
			arguments: json.RawMessage(`{"subject_alias":"acct:1` +
				`","start_inclusive":"2026-01-01T00:00:00.000000Z","end_inclusive":"2026-01-31T23:59:59.999000Z","evidence_row_limit":100}`),
		},
		{
			name: "Unicode-escaped equivalent",
			arguments: json.RawMessage(`{"subject_alias":"acct\u003a1` +
				`","start_inclusive":"2026-01-01T00:00:00.000000Z","end_inclusive":"2026-01-31T23:59:59.999000Z","evidence_row_limit":100}`),
		},
	}
	argumentHashes := make([]string, 0, len(inputs))
	for _, test := range inputs {
		t.Run(test.name, func(t *testing.T) {
			driver := &toolStepDriverStub{
				readOnly: map[string]bool{toolName: true}, parallel: map[string]bool{toolName: true},
			}
			call := domainmodel.ToolCall{
				ID: loopTestHostToolCallID("private-account-flow-" + test.name), Name: toolName, Arguments: test.arguments,
			}
			result, err := RunToolStep(context.Background(), ToolStepInput{
				ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
				ProviderID: "provider-private-account-flow", Workspace: workspace,
				LogicalEffect: domainsecurity.LogicalEffectFundsData, EffectiveMaxModelSteps: 1,
				ToolCalls: []domainmodel.ToolCall{call}, ToolSchemas: []domainmodel.ToolSchema{accountFlowToolSchemaV1ForLoopTest()},
				AdvertisedTools: advertisedToolNames(toolName), LiveMCPTools: advertisedToolNames(toolName),
				MCPConnectionEpochs: map[string]uint64{toolName: 3},
				MCPServerIdentities: map[string]string{
					toolName: loopTestMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 3),
				},
				MCPReadOnlyPolicies: map[string]bool{toolName: true}, SecurityContext: securityContext,
				HostEntitySelection: selection, Driver: driver,
			})
			if err != nil || len(driver.grants) != 1 || len(driver.ready) != 1 ||
				!reflect.DeepEqual(driver.batches, [][]string{{toolName}}) || len(result.SettledToolReferences) != 1 {
				t.Fatalf("account-flow subject reference did not stay on the private effect path: driver=%#v result=%#v err=%v", driver, result, err)
			}
			grant := driver.grants[0]
			argumentHashes = append(argumentHashes, grant.ArgsHash)
			arguments, decodeErr := domainsecurity.DecodeCanonicalJSONObject(call.Arguments)
			if decodeErr != nil || arguments["subject_alias"] != "acct:1" {
				t.Fatalf("strict argument equivalence mismatch: arguments=%#v err=%v", arguments, decodeErr)
			}
			item := map[string]any{
				"kind": "tool_call", "status": "completed",
				"threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
				"toolName": toolName, "callId": call.ID, "arguments": arguments,
				"contextDigest": securityContext.ContextDigest, "executionGrantId": grant.GrantID,
				"executionGrant": grant,
			}
			durable, ok := domaintoolcall.PrivateDurableToolCallItemRecordV1(item)
			if !ok {
				t.Fatal("private durable tool-call projection rejected its exact grant")
			}
			public := domaintoolcall.PublicToolCallItemRecordV1(item)
			sse := BuildToolCallPartialEvent(securityContext.ThreadID, securityContext.TurnID, call.ID, toolName)
			if domainevent.ValidatePublicRecord(public) != nil || domainevent.ValidatePublicRecord(sse) != nil {
				t.Fatalf("metadata-only tool projections were not public-safe: public=%#v sse=%#v", public, sse)
			}
			projected, marshalErr := json.Marshal(map[string]any{"durable": durable, "public": public, "sse": sse})
			if marshalErr != nil || strings.Contains(string(projected), reference) || strings.Contains(string(projected), "subject_alias") ||
				!strings.Contains(string(projected), domaintoolcall.ArgumentsWithheldMessageKey) {
				t.Fatalf("durable/SSE projection exposed attempt-private funds arguments: %s err=%v", projected, marshalErr)
			}
		})
	}
	if len(argumentHashes) != 2 || argumentHashes[0] != argumentHashes[1] {
		t.Fatalf("raw and escaped equivalent arguments did not bind the same semantic value: %#v", argumentHashes)
	}
}

func TestRunToolStepRejectsUnknownFundsSubjectBeforeGrantButContinuesOrdinaryLane(t *testing.T) {
	const toolName = providerFundsAccountFlowToolNameV1
	workspace := "/tmp/analytix-unknown-account-flow-subject"
	securityContext := newLoopCaseContextV2(
		t, "thread-unknown-account-flow-subject", "turn-unknown-account-flow-subject", workspace, "case-unknown-account-flow-subject",
	)
	allowed := domaincaseentity.ReferenceV1("cer1_" + strings.Repeat("a", 64))
	selection, err := NewHostCaseEntitySelectionFromPersistedIngressAliasesV1(
		securityContext,
		caseentityapp.PrivateRecordReferenceV1{
			RecordID: domainsecurity.SHA256Hex(
				[]byte("unknown-account-flow-ingress-record"),
			),
			RecordDigest: domainsecurity.SHA256Hex(
				[]byte("unknown-account-flow-ingress-record-digest"),
			),
		},
		[]domaincaseentity.ReferenceV1{allowed},
		[]domaincaseentity.ModelEntityAliasV1{"acct:1"},
	)
	if err != nil {
		t.Fatal(err)
	}
	fundsID := loopTestHostToolCallID("unknown-account-flow-subject")
	readID := loopTestHostToolCallID("ordinary-read-after-unknown-account-flow")
	grepID := loopTestHostToolCallID("ordinary-grep-after-unknown-account-flow")
	driver := &toolStepDriverStub{
		parallel: map[string]bool{toolName: true, "read": true, "grep": true},
		readOnly: map[string]bool{toolName: true, "read": true, "grep": true},
	}
	result, err := RunToolStep(context.Background(), ToolStepInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		ProviderID: "provider-unknown-account-flow-subject", Workspace: workspace,
		LogicalEffect: domainsecurity.LogicalEffectFundsData, OrdinaryWork: true,
		EffectiveMaxModelSteps: 3,
		ToolCalls: []domainmodel.ToolCall{
			{
				ID: fundsID, Name: toolName,
				Arguments: json.RawMessage(`{"subject_alias":"acct:2` +
					`","start_inclusive":"2026-01-01T00:00:00.000000Z","end_inclusive":"2026-01-31T23:59:59.999000Z","evidence_row_limit":100}`),
			},
			{ID: readID, Name: "read", Arguments: json.RawMessage(`{}`)},
			{ID: grepID, Name: "grep", Arguments: json.RawMessage(`{}`)},
		},
		ToolSchemas: append(
			zeroArgumentToolSchemas("read", "grep"),
			accountFlowToolSchemaV1ForLoopTest(),
		),
		AdvertisedTools: advertisedToolNames(toolName, "read", "grep"),
		LiveMCPTools:    advertisedToolNames(toolName),
		MCPConnectionEpochs: map[string]uint64{
			toolName: 3,
		},
		MCPServerIdentities: map[string]string{
			toolName: loopTestMCPIdentity(t, "analytix_funds", "analytix_funds", "0.16.16", 3),
		},
		MCPReadOnlyPolicies: map[string]bool{toolName: true},
		SecurityContext:     securityContext,
		HostEntitySelection: selection,
		Driver:              driver,
	})
	code, recoverable := RecoverableProtectedLaneFailureCodeV1(err)
	if !recoverable || code != executionGrantEntityReferenceUnavailableV1 {
		t.Fatalf("unknown funds subject did not fail with exact recoverable provenance code: code=%q recoverable=%t err=%v", code, recoverable, err)
	}
	if result.ProtectedLaneUnavailable == nil ||
		result.ProtectedLaneUnavailable.Code != executionGrantEntityReferenceUnavailableV1 {
		t.Fatalf("unknown subject did not retain the typed protected-lane signal: %#v", result.ProtectedLaneUnavailable)
	}
	if got := toolMessageContents(result.Messages); !reflect.DeepEqual(got, []string{"batch:read", "batch:grep"}) {
		t.Fatalf("unknown funds subject blocked or reordered ordinary results: %#v", got)
	}
	if !reflect.DeepEqual(driver.batchAttempts, [][]string{{"read", "grep"}}) ||
		!reflect.DeepEqual(driver.batches, [][]string{{"read", "grep"}}) ||
		!reflect.DeepEqual(driver.ready, []string{"read", "grep"}) || len(driver.grants) != 2 ||
		len(driver.executed) != 0 {
		t.Fatalf("unknown subject reached funds grant/runner or blocked ordinary execution: %#v", driver)
	}
}

func TestRunToolStepRejectsEscapedPrivateReferencesFromOrdinaryEffects(t *testing.T) {
	references := map[string]string{
		"case entity": `\u0063\u0065\u0072\u0031\u005f` + strings.Repeat(`\u0061`, 64),
		"source row":  `\u0073\u0072\u006f\u0077\u0031\u005f` + strings.Repeat(`\u0061`, 64),
	}
	surfaces := []struct {
		name     string
		toolName string
		field    string
	}{
		{name: "write", toolName: "write_file", field: "content"},
		{name: "bash", toolName: "bash", field: "command"},
		{name: "ordinary MCP", toolName: "mcp__docs__lookup", field: "query"},
		{name: "subagent", toolName: "task", field: "prompt"},
	}
	for referenceName, reference := range references {
		for _, surface := range surfaces {
			t.Run(referenceName+"/"+surface.name, func(t *testing.T) {
				driver := &toolStepDriverStub{}
				_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
					ThreadID: "thread-ordinary-private-ref", TurnID: "turn-ordinary-private-ref",
					EffectiveMaxModelSteps: 1,
					ToolCalls: []domainmodel.ToolCall{{
						ID: "ordinary-private-ref-" + referenceName + "-" + surface.name, Name: surface.toolName,
						Arguments: json.RawMessage(`{"` + surface.field + `":"` + reference + `"}`),
					}},
					ToolSchemas: []domainmodel.ToolSchema{{
						Name: surface.toolName,
						Parameters: json.RawMessage(`{"type":"object","properties":{"` + surface.field +
							`":{"type":"string"}},"required":["` + surface.field + `"],"additionalProperties":false}`),
					}},
					AdvertisedTools: advertisedToolNames(surface.toolName), Driver: driver,
				})
				var failure TurnFailureError
				if !errors.As(err, &failure) || failure.Code != "tool_private_arguments" {
					t.Fatalf("escaped private reference was not rejected at the ordinary effect: %T %v", err, err)
				}
				if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 ||
					len(driver.batches) != 0 || len(driver.batchAttempts) != 0 || len(driver.pendings) != 0 {
					t.Fatalf("rejected ordinary private reference produced an effect: %#v", driver)
				}
			})
		}
	}
}

func TestRejectProviderReasoningSmuggledInTaskPromptBeforeChildRun(t *testing.T) {
	driver := &toolStepDriverStub{}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:  "thr_1",
		TurnID:    "turn_1",
		ToolCalls: []domainmodel.ToolCall{{ID: "call_task_private", Name: "task", Arguments: json.RawMessage(`{"prompt":"{\"reasoning\":\"PRIVATE_CHILD_REASONING\"}"}`)}},
		ToolSchemas: []domainmodel.ToolSchema{{
			Name:       "task",
			Parameters: json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"}},"required":["prompt"],"additionalProperties":false}`),
		}},
		AdvertisedTools: advertisedToolNames("task"),
		Driver:          driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "tool_private_arguments" {
		t.Fatalf("provider reasoning in task prompt must fail closed: %T %v", err, err)
	}
	if len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 ||
		len(driver.approvals) != 0 || len(driver.pendings) != 0 {
		t.Fatalf("provider reasoning reached child-run persistence or execution: %#v", driver)
	}
}

func TestRunToolStepReturnsDriverErrors(t *testing.T) {
	driver := &toolStepDriverStub{}
	driverErr := errors.New("driver boom")
	driverWithError := toolStepReadyErrorDriver{ToolStepDriver: driver, err: driverErr}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID:               "thr_1",
		TurnID:                 "turn_1",
		EffectiveMaxModelSteps: 1,
		ToolCalls:              []domainmodel.ToolCall{{ID: "call_1", Name: "read"}},
		ToolSchemas:            zeroArgumentToolSchemas("read"),
		AdvertisedTools:        advertisedToolNames("read"),
		Driver:                 driverWithError,
	})
	if !errors.Is(err, driverErr) {
		t.Fatalf("expected driver error, got %v", err)
	}
}

func TestRejectUnadvertisedToolInAgentMode(t *testing.T) {
	for _, test := range []struct {
		name          string
		mode          string
		subagentDepth int
		toolName      string
	}{
		{name: "agent builtin", mode: "agent", toolName: "write"},
		{name: "plan builtin", mode: "plan", toolName: "write"},
		{name: "subagent builtin", mode: "agent", subagentDepth: 1, toolName: "write"},
		{name: "recovery MCP", mode: "recovery", toolName: "mcp__analytix_funds__count_case_rows"},
		{name: "resume MCP", mode: "resume", toolName: "mcp__analytix_funds__count_case_rows"},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := &toolStepDriverStub{}
			_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
				ThreadID:        "thr_1",
				TurnID:          "turn_1",
				Mode:            test.mode,
				SubagentDepth:   test.subagentDepth,
				ToolCalls:       []domainmodel.ToolCall{{ID: "call_1", Name: test.toolName}},
				ToolSchemas:     zeroArgumentToolSchemas(test.toolName),
				AdvertisedTools: map[string]bool{},
				Driver:          driver,
			})
			var failure TurnFailureError
			if !errors.As(err, &failure) || failure.Code != "tool_not_advertised" {
				t.Fatalf("unadvertised tool must fail closed, got %T %v", err, err)
			}
			if len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
				t.Fatalf("unadvertised tool must be rejected before persistence or execution: %#v", driver)
			}
		})
	}
}

func TestRejectUnadvertisedToolEmitsOnlyClosedManifestBoundDiagnostics(t *testing.T) {
	toolSchemas := zeroArgumentToolSchemas("read")
	manifestHash := toolcatalogapp.ToolSchemaHash(toolSchemas)
	nameSetHash := toolcatalogapp.ToolSchemaNameSetHash(toolSchemas)
	driver := &toolStepDriverStub{}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thr_diagnostic", TurnID: "turn_diagnostic", Mode: "plan",
		PromptRoute: "tool_agent", LoopStep: 1,
		ToolCalls:   []domainmodel.ToolCall{{ID: "call_diagnostic", Name: "read_file"}},
		ToolSchemas: toolSchemas, AdvertisedTools: advertisedToolNames("read"),
		AdvertisedToolCount: len(toolSchemas), AdvertisedToolManifestHash: manifestHash,
		AdvertisedNameSetSortedHash: nameSetHash, ProviderRequestToolManifestHash: manifestHash,
		KnownBuiltinTools: advertisedToolNames("read", "read_file"), Driver: driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "tool_not_advertised" {
		t.Fatalf("unadvertised alias must fail closed, got %T %v", err, err)
	}
	expectedNameHash := sha256.Sum256([]byte("read_file"))
	expectedDetails := map[string]any{
		"rejectedToolNormalizedNameSha256":       fmt.Sprintf("%x", expectedNameHash),
		"rejectedToolCategory":                   "known_alias_not_advertised",
		"promptRoute":                            "tool_agent",
		"loopStep":                               float64(1),
		"advertisedToolCount":                    float64(1),
		"advertisedToolManifestHash":             manifestHash,
		"advertisedNameSetSortedHash":            nameSetHash,
		"providerRequestToolManifestHash":        manifestHash,
		"runToolStepManifestHash":                manifestHash,
		"providerRequestRunToolStepManifestSame": true,
	}
	if !reflect.DeepEqual(failure.Details, expectedDetails) {
		t.Fatalf("closed unadvertised-tool diagnostics mismatch:\nwant=%#v\ngot=%#v", expectedDetails, failure.Details)
	}
	serialized, marshalErr := json.Marshal(failure)
	if marshalErr != nil || strings.Contains(failure.Error(), "read_file") || strings.Contains(string(serialized), `"toolName"`) {
		t.Fatalf("raw rejected tool name escaped diagnostics: error=%q body=%s marshalErr=%v", failure.Error(), serialized, marshalErr)
	}
	if len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
		t.Fatalf("diagnostic rejection reached persistence or execution: %#v", driver)
	}
}

func TestUnadvertisedToolDiagnosticUsesClosedHostClassification(t *testing.T) {
	toolSchemas := zeroArgumentToolSchemas("read")
	manifestHash := toolcatalogapp.ToolSchemaHash(toolSchemas)
	input := ToolStepInput{
		PromptRoute: "tool_agent", LoopStep: 2,
		ToolSchemas: toolSchemas, AdvertisedToolCount: len(toolSchemas),
		AdvertisedToolManifestHash:      manifestHash,
		AdvertisedNameSetSortedHash:     toolcatalogapp.ToolSchemaNameSetHash(toolSchemas),
		ProviderRequestToolManifestHash: manifestHash,
		KnownBuiltinTools:               advertisedToolNames("read_file", "write"),
		KnownMCPTools:                   advertisedToolNames("mcp__docs__lookup"),
	}
	for _, test := range []struct {
		name     string
		toolName string
		category string
	}{
		{name: "compatibility alias", toolName: "read_file", category: "known_alias_not_advertised"},
		{name: "builtin", toolName: "write", category: "known_builtin_not_advertised"},
		{name: "MCP", toolName: "mcp__docs__lookup", category: "known_mcp_not_advertised"},
		{name: "unknown", toolName: "provider_invented_tool", category: "unknown_provider_name"},
	} {
		t.Run(test.name, func(t *testing.T) {
			details := unadvertisedToolDiagnosticV1(input, test.toolName)
			expectedNameHash := sha256.Sum256([]byte(test.toolName))
			if details["rejectedToolCategory"] != test.category ||
				details["rejectedToolNormalizedNameSha256"] != fmt.Sprintf("%x", expectedNameHash) ||
				!domainfailure.ValidateToolNotAdvertisedDetails(details) {
				t.Fatalf("closed classification mismatch for %q: %#v", test.toolName, details)
			}
			serialized, err := json.Marshal(details)
			if err != nil || strings.Contains(string(serialized), test.toolName) {
				t.Fatalf("raw tool name escaped closed classification: body=%s err=%v", serialized, err)
			}
		})
	}
}

func TestToolAddedDuringStreamStillRejected(t *testing.T) {
	originalTool := "mcp__docs__lookup"
	lateTool := "mcp__docs__late_lookup"
	driver := &toolStepDriverStub{
		parallel: map[string]bool{lateTool: true},
		readOnly: map[string]bool{lateTool: true},
	}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thr_stream_catalog", TurnID: "turn_stream_catalog", ProviderID: "provider_stream_catalog",
		ToolCalls: []domainmodel.ToolCall{{ID: "call_late", Name: lateTool}},
		// Simulate a live catalog refresh after the provider request started: the
		// late tool is now live and has a schema, but it was not in the request's
		// immutable advertised-name set.
		ToolSchemas:     zeroArgumentToolSchemas(originalTool, lateTool),
		AdvertisedTools: advertisedToolNames(originalTool),
		LiveMCPTools:    advertisedToolNames(originalTool, lateTool),
		MCPConnectionEpochs: map[string]uint64{
			originalTool: 4,
			lateTool:     4,
		},
		MCPServerIdentities: map[string]string{
			originalTool: loopTestMCPIdentity(t, "docs", "docs", "1.0.0", 4),
			lateTool:     loopTestMCPIdentity(t, "docs", "docs", "1.0.0", 4),
		},
		MCPReadOnlyPolicies: map[string]bool{originalTool: true, lateTool: true},
		Driver:              driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "tool_not_advertised" {
		t.Fatalf("tool added after provider request must remain unavailable to that request: %T %v", err, err)
	}
	if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 ||
		len(driver.approvals) != 0 || len(driver.userInputs) != 0 {
		t.Fatalf("late tool reached grant, persistence, pause, or execution: %#v", driver)
	}
}

func TestMalformedToolCallIdentityFailsBeforePersistence(t *testing.T) {
	for _, calls := range [][]domainmodel.ToolCall{
		{{Name: "read"}},
		{{ID: "call_1", Name: "read"}, {ID: "call_1", Name: "read"}},
	} {
		driver := &toolStepDriverStub{}
		_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
			ThreadID:        "thr_1",
			TurnID:          "turn_1",
			ToolCalls:       calls,
			ToolSchemas:     zeroArgumentToolSchemas("read"),
			AdvertisedTools: advertisedToolNames("read"),
			Driver:          driver,
		})
		var failure TurnFailureError
		if !errors.As(err, &failure) || failure.Code != "tool_call_identity_invalid" {
			t.Fatalf("malformed call identity must fail closed, got %T %v", err, err)
		}
		if len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
			t.Fatalf("malformed call identity reached persistence or execution: %#v", driver)
		}
	}
}

func TestWrongMCPServerAndMissingSchemaFailBeforePersistence(t *testing.T) {
	for _, test := range []struct {
		name       string
		call       domainmodel.ToolCall
		schemas    []domainmodel.ToolSchema
		advertised map[string]bool
		wantCode   string
	}{
		{
			name:       "wrong MCP server namespace",
			call:       domainmodel.ToolCall{ID: "call_1", Name: "mcp__spoofed_funds__count_case_rows"},
			schemas:    zeroArgumentToolSchemas("mcp__analytix_funds__count_case_rows"),
			advertised: advertisedToolNames("mcp__analytix_funds__count_case_rows"),
			wantCode:   "tool_not_advertised",
		},
		{
			name:       "advertised name without schema",
			call:       domainmodel.ToolCall{ID: "call_1", Name: "lookup"},
			advertised: advertisedToolNames("lookup"),
			wantCode:   "tool_schema_missing",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			driver := &toolStepDriverStub{}
			_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
				ThreadID:        "thr_1",
				TurnID:          "turn_1",
				ToolCalls:       []domainmodel.ToolCall{test.call},
				ToolSchemas:     test.schemas,
				AdvertisedTools: test.advertised,
				Driver:          driver,
			})
			var failure TurnFailureError
			if !errors.As(err, &failure) || failure.Code != test.wantCode {
				t.Fatalf("unsafe tool call must fail closed with %s, got %T %v", test.wantCode, err, err)
			}
			if len(driver.ready) != 0 || len(driver.executed) != 0 || len(driver.batches) != 0 {
				t.Fatalf("unsafe tool call reached persistence or execution: %#v", driver)
			}
		})
	}
}

func TestMCPWithoutLiveProbeCannotReceiveExecutionGrant(t *testing.T) {
	toolName := "mcp__analytix_funds__count_case_rows"
	driver := &toolStepDriverStub{parallel: map[string]bool{toolName: true}}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thr_1", TurnID: "turn_1", ProviderID: "provider_1",
		ToolCalls: []domainmodel.ToolCall{{ID: "call_1", Name: toolName}}, ToolSchemas: zeroArgumentToolSchemas(toolName),
		AdvertisedTools: advertisedToolNames(toolName), LiveMCPTools: map[string]bool{}, Driver: driver,
	})
	var failure TurnFailureError
	if !errors.As(err, &failure) || failure.Code != "tool_source_unavailable" {
		t.Fatalf("cached schema without a live source must fail closed: %T %v", err, err)
	}
	if len(driver.ready) != 0 || len(driver.grants) != 0 || len(driver.executed) != 0 {
		t.Fatalf("unavailable MCP source reached grant, persistence, or execution: %#v", driver)
	}
}

func TestReadOnlyGrantIsIndependentFromParallelScheduling(t *testing.T) {
	toolName := "mcp__docs__lookup"
	driver := &toolStepDriverStub{
		parallel: map[string]bool{toolName: false},
		readOnly: map[string]bool{toolName: true},
	}
	_, err := runSecureToolStep(t, context.Background(), ToolStepInput{
		ThreadID: "thr_1", TurnID: "turn_1", ProviderID: "provider_1",
		ToolCalls: []domainmodel.ToolCall{{ID: "call_1", Name: toolName}}, ToolSchemas: zeroArgumentToolSchemas(toolName),
		AdvertisedTools: advertisedToolNames(toolName), LiveMCPTools: map[string]bool{toolName: true},
		MCPConnectionEpochs: map[string]uint64{toolName: 3},
		MCPServerIdentities: map[string]string{toolName: loopTestMCPIdentity(t, "docs", "docs", "1.0.0", 3)}, Driver: driver,
		MCPReadOnlyPolicies: map[string]bool{toolName: true},
	})
	if err != nil {
		t.Fatalf("run host-authorized read-only tool: %v", err)
	}
	if len(driver.grants) != 1 || !driver.grants[0].ReadOnly || len(driver.batches) != 0 || !reflect.DeepEqual(driver.executed, []string{toolName}) {
		t.Fatalf("read-only authority was inferred from scheduling instead of the host policy: %#v", driver)
	}
}

type toolStepReadyErrorDriver struct {
	ToolStepDriver
	err error
}

func (driver toolStepReadyErrorDriver) PersistToolCallReady(context.Context, string, string, domainmodel.ToolCall, int, domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) (string, error) {
	return "", driver.err
}

func runSecureToolStep(t *testing.T, ctx context.Context, input ToolStepInput) (ToolStepResult, error) {
	t.Helper()
	workspace := input.Workspace
	if strings.TrimSpace(workspace) == "" {
		workspace = "/tmp/analytix-tool-step-test"
	}
	if strings.TrimSpace(input.ProviderID) == "" {
		input.ProviderID = "provider_test"
	}
	input = hostToolStepInputForTest(input)
	input.SecurityContext = newLoopGeneralContextV2(t, input.ThreadID, input.TurnID, workspace)
	return RunToolStep(ctx, input)
}

func hostToolStepInputForTest(input ToolStepInput) ToolStepInput {
	for index := range input.ToolCalls {
		if strings.TrimSpace(input.ToolCalls[index].ID) != "" && !domainmodel.IsHostToolCallIDV1(input.ToolCalls[index].ID) {
			input.ToolCalls[index].ID = loopTestHostToolCallID(input.ToolCalls[index].ID)
		}
	}
	return input
}

func loopTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("analytix.loop-test-host-tool-call/v1\x00" + seed))
	identity, err := domainmodel.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}

func newLoopGeneralContextV2(t *testing.T, threadID, turnID, workspace string) domainsecurity.TurnSecurityContext {
	t.Helper()
	return newLoopExecutionContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace,
		CaseID: domainsecurity.UnboundCaseID, CaseBindingHash: domainsecurity.UnboundCaseBindingHash(workspace),
		DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID, SourceManifestHash: domainsecurity.EmptySourceManifestHash,
	}, domainsecurity.RiskClassGeneral, domainsecurity.PublicationDispositionGeneralOutput, domainsecurity.CaseBindingStateMissing)
}

func newLoopCaseContextV2(t *testing.T, threadID, turnID, workspace, caseID string) domainsecurity.TurnSecurityContext {
	t.Helper()
	return newLoopExecutionContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspace, CaseID: caseID,
		CaseBindingHash:    domainsecurity.SHA256Hex([]byte("loop-test-case-binding:\x00" + caseID)),
		DatasetSnapshotID:  securitycontexttest.DatasetSnapshotID(threadID + ":" + turnID + ":" + caseID),
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("loop-test-source-manifest:\x00" + caseID)),
	}, domainsecurity.RiskClassCase, domainsecurity.PublicationDispositionCaseEvidenceGate, domainsecurity.CaseBindingStateValid)
}

func newLoopExecutionContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput, riskClass, disposition, bindingState string) domainsecurity.TurnSecurityContext {
	t.Helper()
	input.TenantID = domainsecurity.LocalTenantID
	input.UserID = domainsecurity.LocalUserID
	input.ContextEpoch = 1
	input.IssuedAt = time.Date(2026, 7, 12, 6, 30, 0, 0, time.UTC)
	policy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("loop-test-risk-policy:\x00" + input.ThreadID + "\x00" + input.WorkspaceRealPath + "\x00" + riskClass)),
		RiskClass:              riskClass,
		Disposition:            disposition,
		CaseBindingState:       bindingState,
		BindingObservationDigest: domainsecurity.SHA256Hex([]byte(
			"loop-test-binding-observation:\x00" + input.ThreadID + "\x00" + input.TurnID + "\x00" + input.CaseBindingHash,
		)),
		BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitycontexttest.WitnessedRiskBinding(input.ThreadID, input.WorkspaceRealPath, riskClass, policy.ThreadRiskPolicyDigest)
	if err != nil {
		t.Fatal(err)
	}
	input.PublicationPolicy = policy
	input.RiskAuthorityBinding = binding
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(input)
	if err != nil || domainsecurity.ValidateTurnSecurityContextForExecution(securityContext) != nil {
		t.Fatalf("loop V2 execution context is invalid: context=%#v err=%v", securityContext, err)
	}
	return securityContext
}

func toolMessageContents(messages []domainmodel.Message) []string {
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		out = append(out, message.Content)
	}
	return out
}

func settledResultItemIDs(references []domainsecurity.SettledToolReference) []string {
	ids := make([]string, 0, len(references))
	for _, reference := range references {
		ids = append(ids, reference.ResultItemID)
	}
	return ids
}

func advertisedToolNames(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

func zeroArgumentToolSchemas(names ...string) []domainmodel.ToolSchema {
	out := make([]domainmodel.ToolSchema, 0, len(names))
	for _, name := range names {
		out = append(out, domainmodel.ToolSchema{
			Name:       name,
			Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		})
	}
	return out
}

func accountFlowToolSchemaV1ForLoopTest() domainmodel.ToolSchema {
	return domainmodel.ToolSchema{
		Name: providerFundsAccountFlowToolNameV1,
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "subject_alias":{"type":"string","pattern":"^(acct|card):(?:[1-9][0-9]{0,8}|[1-3][0-9]{9}|4[01][0-9]{8}|42[0-8][0-9]{7}|429[0-3][0-9]{6}|4294[0-8][0-9]{5}|42949[0-5][0-9]{4}|429496[0-6][0-9]{3}|4294967[0-1][0-9]{2}|42949672[0-8][0-9]|429496729[0-5])$","minLength":6,"maxLength":15},
    "start_inclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
    "end_inclusive":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{6}Z$","minLength":27,"maxLength":27},
    "evidence_row_limit":{"type":"integer","minimum":1,"maximum":512}
  },
  "required":["subject_alias","start_inclusive","end_inclusive","evidence_row_limit"],
  "additionalProperties":false
}`),
	}
}

func stringOutput(value any) string {
	text, ok := value.(string)
	if ok {
		return text
	}
	if record, ok := value.(map[string]any); ok {
		if message, ok := record["error"].(string); ok {
			return message
		}
		if code, ok := record["code"].(string); ok {
			return code
		}
	}
	return "value"
}

func loopTestMCPIdentity(t *testing.T, serverID, observedName, observedVersion string, epoch uint64) string {
	t.Helper()
	identity, err := domainsecurity.NewVerifiedMCPServerIdentity(serverID, observedName, observedVersion, domainsecurity.SHA256Hex([]byte("loop-test-runtime-instance")), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
