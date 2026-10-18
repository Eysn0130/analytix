package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	mcpprotocol "analytix.local/runtime-go/internal/adapters/outbound/mcp/protocol"
	evidenceapp "analytix.local/runtime-go/internal/app/evidence"
	appmcp "analytix.local/runtime-go/internal/app/mcp"
	domainmcp "analytix.local/runtime-go/internal/domain/mcp"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	"analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func scheduleSemanticFixtureV1(t *testing.T, rawName string, arguments json.RawMessage) (*ProductionManager, *scheduleReceiptClientV1, appmcp.ToolRunInput) {
	t.Helper()
	manager, client, input := scheduleReceiptManagerFixtureV1(t, scheduleReceiptRawV1("success", false))
	tool := manager.tools[input.ToolName]
	delete(manager.tools, input.ToolName)
	input.ToolName = CanonicalToolName(hostScheduleMCPServerIDV1, rawName)
	input.Call.Name, input.Call.Arguments = input.ToolName, arguments
	tool.Name, tool.RawName, tool.Tool.Name = input.ToolName, rawName, rawName
	operation := domaintoolresult.ProtectedScheduleOperationForToolV1(input.ToolName)
	if operation == "create" {
		tool.Tool.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["title","prompt","schedule"],"properties":{"title":{"type":"string","minLength":1},"prompt":{"type":"string","minLength":1},"schedule":{"type":"object","additionalProperties":false,"required":["kind","everyMinutes"],"properties":{"kind":{"const":"interval"},"everyMinutes":{"type":"integer","minimum":1,"maximum":10080}}},"enabled":{"type":"boolean"}}}`)
	} else {
		tool.Tool.InputSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["task_id"],"properties":{"task_id":{"type":"string","minLength":1}}}`)
	}
	manager.tools[input.ToolName] = tool
	reissueScheduleSemanticGrantV1(manager, &input, input.ExecutionGrant)
	return manager, client, input
}

func reissueScheduleSemanticGrantV1(manager *ProductionManager, input *appmcp.ToolRunInput, authority domainsecurity.ExecutionGrant) {
	input.ExecutionGrant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: input.SecurityContext, Provider: authority.Provider, ServerIdentity: authority.ServerIdentity,
		ToolName: input.Call.Name, ToolCallID: input.Call.ID, ConnectionEpoch: authority.ConnectionEpoch,
		ArgsHash: domainsecurity.CanonicalJSONHash(input.Call.Arguments), SchemaHash: managedToolSchemaHash(input.Call.Name, manager.tools[input.Call.Name]),
		ScopeHash: authority.ScopeHash, ReadOnly: authority.ReadOnly, ApprovalState: authority.ApprovalState, IssuedAt: time.Now().UTC(),
	})
}

func TestNativeScheduleSemanticProjectionUsesCurrentHostOwnerWithoutDispatchV1(t *testing.T) {
	for _, prefix := range []string{"gui", "claw"} {
		for _, operation := range []string{"create", "update", "delete"} {
			t.Run(prefix+"-"+operation, func(t *testing.T) {
				arguments := json.RawMessage(`{"task_id":"actual-target-task"}`)
				if operation == "create" {
					arguments = json.RawMessage(`{"title":"Exact title","prompt":"  PRIVATE_NATIVE_PROMPT  ","schedule":{"kind":"interval","everyMinutes":1.0}}`)
				}
				manager, client, input := scheduleSemanticFixtureV1(t, prefix+"_schedule_"+operation, arguments)
				envelope, err := domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
				if err != nil {
					t.Fatal(err)
				}
				projection, native := manager.HostScheduleSideEffectProjectionV1(context.Background(), envelope, input.Call)
				if !native || projection["operation"] != operation || client.calls != 0 {
					t.Fatal("current native semantic preparation missing or dispatched transport")
				}
				request := projection["request"].(map[string]any)
				if operation == "create" {
					if request["prompt"] != "  PRIVATE_NATIVE_PROMPT  " || request["schedule"].(map[string]any)["everyMinutes"] != json.Number("1.0") {
						t.Fatal("exact native request values were normalized outside Host owner")
					}
					if _, present := request["enabled"]; present {
						t.Fatal("missing optional field acquired a guessed Host default")
					}
				} else if request["task_id"] != "actual-target-task" {
					t.Fatal("target task identity replaced by source conversation")
				}
				// Fresh call/grant identity cannot defeat the same operation's durable semantic identity.
				input.Call.ID = toolidentity.MustHostToolCallIDV1("schedule-second-semantic-call")
				reissueScheduleSemanticGrantV1(manager, &input, input.ExecutionGrant)
				envelope, err = domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
				if err != nil {
					t.Fatal(err)
				}
				second, native := manager.HostScheduleSideEffectProjectionV1(context.Background(), envelope, input.Call)
				if !native || !reflect.DeepEqual(projection, second) || client.calls != 0 {
					t.Fatal("semantic request included transient call/grant identity")
				}
			})
		}
	}
}

func TestNativeScheduleSemanticProjectionRejectsUnavailableOrChangedAuthorityV1(t *testing.T) {
	cases := map[string]func(*ProductionManager, *appmcp.ToolRunInput){
		"same-name-unbound":   func(m *ProductionManager, _ *appmcp.ToolRunInput) { m.specs[0].IdentitySource = "" },
		"client-disconnected": func(m *ProductionManager, _ *appmcp.ToolRunInput) { delete(m.clients, hostScheduleMCPServerIDV1) },
		"placeholder": func(m *ProductionManager, in *appmcp.ToolRunInput) {
			tool := m.tools[in.ToolName]
			tool.Placeholder = true
			m.tools[in.ToolName] = tool
		},
		"epoch-drift":    func(m *ProductionManager, _ *appmcp.ToolRunInput) { m.connectionEpochs[hostScheduleMCPServerIDV1]++ },
		"identity-drift": func(m *ProductionManager, _ *appmcp.ToolRunInput) { m.serverIdentities[hostScheduleMCPServerIDV1] = "" },
		"schema-drift": func(m *ProductionManager, in *appmcp.ToolRunInput) {
			tool := m.tools[in.ToolName]
			tool.Tool.Description += " changed"
			m.tools[in.ToolName] = tool
		},
		"policy-drift": func(m *ProductionManager, in *appmcp.ToolRunInput) {
			m.specs[0].ReadOnlyToolNames[m.tools[in.ToolName].RawName] = true
		},
		"workspace-untrusted": func(m *ProductionManager, _ *appmcp.ToolRunInput) {
			m.specs[0].TrustScope = "workspace"
			m.specs[0].TrustedWorkspaceRoots = []string{"/foreign/workspace"}
		},
		"foreign-call": func(_ *ProductionManager, in *appmcp.ToolRunInput) {
			in.Call.ID = toolidentity.MustHostToolCallIDV1("foreign-call")
		},
		"argument-drift": func(_ *ProductionManager, in *appmcp.ToolRunInput) {
			in.Call.Arguments = json.RawMessage(`{"task_id":"other-task"}`)
		},
		"unclosed-native-schema": func(m *ProductionManager, in *appmcp.ToolRunInput) {
			tool := m.tools[in.ToolName]
			tool.Tool.InputSchema = json.RawMessage(`{"type":"object"}`)
			m.tools[in.ToolName] = tool
			reissueScheduleSemanticGrantV1(m, in, in.ExecutionGrant)
		},
		"read-only-grant": func(m *ProductionManager, in *appmcp.ToolRunInput) {
			grant := in.ExecutionGrant
			grant.ReadOnly = true
			reissueScheduleSemanticGrantV1(m, in, grant)
		},
		"approval-pending": func(m *ProductionManager, in *appmcp.ToolRunInput) {
			grant := in.ExecutionGrant
			grant.ApprovalState = "pending"
			reissueScheduleSemanticGrantV1(m, in, grant)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			manager, client, input := scheduleSemanticFixtureV1(t, "gui_schedule_delete", json.RawMessage(`{"task_id":"actual-target-task"}`))
			mutate(manager, &input)
			envelope, err := domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
			if err != nil {
				t.Fatal(err)
			}
			if _, native := manager.HostScheduleSideEffectProjectionV1(context.Background(), envelope, input.Call); native || client.calls != 0 {
				t.Fatal("invalid native authority acquired semantic preparation or dispatched transport")
			}
		})
	}
	manager, client, input := scheduleSemanticFixtureV1(t, "gui_schedule_delete", json.RawMessage(`{"task_id":"actual-target-task"}`))
	envelope, _ := domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, native := manager.HostScheduleSideEffectProjectionV1(canceled, envelope, input.Call); native || client.calls != 0 {
		t.Fatal("canceled preparation reached native owner")
	}
}

func TestNativeScheduleSemanticProjectionRejectsInvalidBoundRequestsV1(t *testing.T) {
	for name, arguments := range map[string]string{
		"missing-target": `{}`, "null-target": `{"task_id":null}`, "empty-target": `{"task_id":""}`,
		"unknown-field": `{"task_id":"task-1","extra":"untrusted"}`, "host-authority": `{"task_id":"task-1","_analytix":{}}`,
		"duplicate-key": `{"task_id":"first","task_id":"second"}`, "invalid-unicode": `{"task_id":"\ud800"}`,
	} {
		t.Run(name, func(t *testing.T) {
			manager, client, input := scheduleSemanticFixtureV1(t, "gui_schedule_delete", json.RawMessage(`{"task_id":"actual-target-task"}`))
			envelope, err := domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
			if err != nil {
				t.Fatal(err)
			}
			input.Call.Arguments = json.RawMessage(arguments)
			// Strict valid JSON is bound to a fresh grant so schema rejection is exercised.
			if domainsecurity.CanonicalJSONHash(input.Call.Arguments) != "" {
				reissueScheduleSemanticGrantV1(manager, &input, input.ExecutionGrant)
				envelope, err = domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, native := manager.HostScheduleSideEffectProjectionV1(context.Background(), envelope, input.Call); native || client.calls != 0 {
				t.Fatal("invalid bound request acquired native semantic identity")
			}
		})
	}
}

type scheduleReceiptClientV1 struct {
	result    any
	afterCall func()
	calls     int
}

func (*scheduleReceiptClientV1) ListTools() ([]ToolSpec, error)         { return nil, nil }
func (*scheduleReceiptClientV1) ListPrompts() ([]PromptSpec, error)     { return nil, nil }
func (*scheduleReceiptClientV1) ListResources() ([]ResourceSpec, error) { return nil, nil }
func (*scheduleReceiptClientV1) Close()                                 {}
func (client *scheduleReceiptClientV1) CallTool(string, map[string]any) (any, error) {
	client.calls++
	if client.afterCall != nil {
		client.afterCall()
	}
	return client.result, nil
}

func scheduleReceiptRawV1(status string, isError bool) json.RawMessage {
	var task any
	if status == "success" {
		task = map[string]any{"id": "task-1", "title": "Actual created title", "enabled": true, "prompt": "PRIVATE_NATIVE_PROMPT", "workspaceRoot": "/synthetic/target", "clawChannelId": "", "model": "actual-model", "reasoningEffort": "auto", "mode": "agent",
			"schedule": map[string]any{"kind": "daily", "everyMinutes": 60, "timeOfDay": "09:00", "atTime": ""}, "createdAt": "actual-created", "updatedAt": "actual-updated", "lastRunAt": "", "nextRunAt": "", "lastStatus": "idle", "lastMessage": "schedule_task_idle", "lastThreadId": ""}
	}
	body, _ := json.Marshal(map[string]any{"content": []any{}, "structuredContent": map[string]any{"status": status, "message": "Actual outcome", "task": task}, "isError": isError})
	return body
}

func scheduleReceiptManagerFixtureV1(t *testing.T, raw json.RawMessage) (*ProductionManager, *scheduleReceiptClientV1, appmcp.ToolRunInput) {
	t.Helper()
	now := time.Now().UTC()
	securityContext := newExecutableCaseSecurityContext(t, domainsecurity.TurnSecurityContextInput{ThreadID: "thread-schedule", TurnID: "turn-schedule", WorkspaceRealPath: "/workspace", CaseID: "case-schedule",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-schedule")), DatasetSnapshotID: domainsecurity.DatasetSnapshotIDPrefixV2 + domainsecurity.SHA256Hex([]byte("snapshot-schedule")), SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-schedule")), ContextEpoch: 2, IssuedAt: now})
	configured := hostScheduleMCPBindingFixtureV1()
	manager := newAdmissionTestManager(nil)
	manager.specs = BindHostScheduleMCPServerV1([]ServerSpec{configured}, &configured)
	name := CanonicalToolName("gui_schedule", "gui_schedule_create")
	tool := managedTool{ServerID: "gui_schedule", Name: name, RawName: "gui_schedule_create", Tool: ToolSpec{Name: "gui_schedule_create", Description: "Native schedule fixture", InputSchema: json.RawMessage(`{"type":"object"}`), OutputSchema: json.RawMessage(`{"type":"object"}`)}}
	client := &scheduleReceiptClientV1{result: mcpprotocol.ExtractLosslessToolResult(raw)}
	identity := mcpTestVerifiedIdentity(t, "gui_schedule", "analytix-schedule", "0.1.0", 1)
	manager.tools[name] = tool
	manager.clients["gui_schedule"] = client
	manager.connectionEpochs["gui_schedule"] = 1
	manager.serverIdentities["gui_schedule"] = identity
	call := domainmodel.ToolCall{ID: toolidentity.MustHostToolCallIDV1("schedule-original"), Name: name, Arguments: json.RawMessage(`{}`)}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: securityContext, Provider: "synthetic", ServerIdentity: identity, ToolName: name, ToolCallID: call.ID, ConnectionEpoch: 1,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: managedToolSchemaHash(name, tool), ScopeHash: domainsecurity.SHA256Hex([]byte("schedule-scope")), ReadOnly: false, ApprovalState: "approved", IssuedAt: now})
	return manager, client, appmcp.ToolRunInput{ToolName: name, Arguments: map[string]any{}, Workspace: securityContext.WorkspaceRealPath, ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID, SecurityContext: securityContext, ExecutionGrant: grant, Call: call}
}

func TestNativeScheduleReceiptTraversesActualRunnerOnceWithoutSerialization(t *testing.T) {
	for _, status := range []string{"success", "success-omitted-isError", "failure"} {
		expectedStatus := status
		if status == "success-omitted-isError" {
			expectedStatus = "success"
		}
		raw := scheduleReceiptRawV1(expectedStatus, expectedStatus == "failure")
		if status == "success-omitted-isError" {
			value := map[string]any{}
			if json.Unmarshal(raw, &value) != nil {
				t.Fatal("decode fixture")
			}
			delete(value, "isError")
			raw, _ = json.Marshal(value)
		}
		manager, _, input := scheduleReceiptManagerFixtureV1(t, raw)
		ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
		output, isError := appmcp.ExecuteTool(ctx, manager, input)
		capture, present := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
		if !present || isError != (expectedStatus == "failure") || capture.Schedule.Status != expectedStatus {
			t.Fatal("native runner receipt missing or changed")
		}
		if expectedStatus == "success" && (capture.Schedule.Task.ID != "task-1" || capture.Schedule.Task.WorkspaceRoot != "/synthetic/target" || capture.Schedule.Task.LastThreadID != "") {
			t.Fatal("actual task identities replaced by source chat")
		}
		lossless := output[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
		body, _ := json.Marshal(output)
		if lossless.HostPrivate != nil || strings.Contains(string(body), "host-private schedule") || strings.Contains(string(body), "rawSHA256") {
			t.Fatal("process carrier escaped runner")
		}
	}
}

func TestNativeScheduleReceiptRejectsSameNameUnboundAndInconsistentResults(t *testing.T) {
	for _, mutate := range []func(*ProductionManager, *scheduleReceiptClientV1){
		func(manager *ProductionManager, _ *scheduleReceiptClientV1) { manager.specs[0].IdentitySource = "" },
		func(_ *ProductionManager, client *scheduleReceiptClientV1) {
			client.result = mcpprotocol.ExtractLosslessToolResult(scheduleReceiptRawV1("success", true))
		},
		func(_ *ProductionManager, client *scheduleReceiptClientV1) {
			client.result = mcpprotocol.ExtractLosslessToolResult(scheduleReceiptRawV1("failure", false))
		},
		func(_ *ProductionManager, client *scheduleReceiptClientV1) {
			value := client.result.(domainmcp.LosslessToolResult)
			value.Value = map[string]any{"status": "success"}
			client.result = value
		},
	} {
		manager, client, input := scheduleReceiptManagerFixtureV1(t, scheduleReceiptRawV1("success", false))
		mutate(manager, client)
		ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
		appmcp.ExecuteTool(ctx, manager, input)
		if _, present := domaintoolresult.ConsumeProtectedToolResultV1(ctx); present {
			t.Fatal("unbound or inconsistent output acquired a native capture")
		}
	}
}

func TestNativeScheduleReceiptBurnsCrossCallSubstitutionAndExternalCarrierInjection(t *testing.T) {
	manager, client, input := scheduleReceiptManagerFixtureV1(t, scheduleReceiptRawV1("success", false))
	envelope, err := domainmcp.NewHostContextEnvelope(input.SecurityContext, input.ExecutionGrant)
	if err != nil {
		t.Fatal(err)
	}
	output := manager.CallToolSecurityBoundContext(context.Background(), input.ToolName, true, envelope, input.Arguments)
	lossless := output[domainmcp.HostRawToolResultKey].(domainmcp.LosslessToolResult)
	outcome, err := evidenceapp.NormalizeMCPToolOutcome(evidenceapp.NormalizeMCPToolOutcomeInput{Context: input.SecurityContext, Grant: input.ExecutionGrant, Call: input.Call, Raw: output, At: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	grant := input.ExecutionGrant
	alien := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{Context: input.SecurityContext, Provider: grant.Provider, ServerIdentity: grant.ServerIdentity, ToolName: grant.ToolName,
		ToolCallID: toolidentity.MustHostToolCallIDV1("schedule-same-argument-other-call"), ConnectionEpoch: grant.ConnectionEpoch, ArgsHash: grant.ArgsHash, SchemaHash: grant.SchemaHash, ScopeHash: grant.ScopeHash, ReadOnly: grant.ReadOnly, ApprovalState: grant.ApprovalState, IssuedAt: time.Now().UTC()})
	alienEnvelope, err := domainmcp.NewHostContextEnvelope(input.SecurityContext, alien)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := manager.ConsumeHostScheduleReceiptV1(lossless, alienEnvelope, outcome); present {
		t.Fatal("same-turn same-argument call borrowed receipt")
	}
	if _, present := manager.ConsumeHostScheduleReceiptV1(lossless, envelope, outcome); present {
		t.Fatal("rejected carrier remained reusable")
	}
	forged := &hostScheduleReceiptCarrierV1{owner: manager, active: true, envelope: envelope, rawSHA256: lossless.RawSHA256, receipt: &domaintoolresult.ProtectedScheduleReceiptV1{Operation: "create", Status: "success", Message: "FORGED"}}
	remote := client.result.(domainmcp.LosslessToolResult)
	remote.HostPrivate = forged
	client.result = remote
	ctx := domaintoolresult.WithProtectedCaptureV1(context.Background(), &domaintoolresult.ProtectedCaptureCollectorV1{})
	appmcp.ExecuteTool(ctx, manager, input)
	capture, present := domaintoolresult.ConsumeProtectedToolResultV1(ctx)
	if !present || capture.Schedule.Message != "Actual outcome" || forged.active {
		t.Fatal("transport-injected carrier survived generic quarantine")
	}
}
