package server

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	httpapi "analytix.local/runtime-go/internal/adapters/inbound/httpapi"
	executiongrantapp "analytix.local/runtime-go/internal/app/executiongrant"
	processapp "analytix.local/runtime-go/internal/app/processdisplay"
	snapshotapp "analytix.local/runtime-go/internal/app/toolresultsnapshot"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	contracts "analytix.local/runtime-go/internal/contracts"
	domainidentity "analytix.local/runtime-go/internal/domain/identity"
	domainprocess "analytix.local/runtime-go/internal/domain/processdisplay"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	port "analytix.local/runtime-go/internal/ports/processdisplay"
)

type runtimeProcessDisplayAccessV1 struct{ handler *runtimeServerHandler }

func (a runtimeProcessDisplayAccessV1) AcquireCurrent(ctx context.Context, binding port.BindingV1) (port.AuthorizedV1, func(), error) {
	h := a.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.runtimeControl().ShuttingDown() || h.store == nil || h.turnSecurity.Observer == nil || h.toolSnapshotHistoryValidate == nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil || !domainidentity.SamePrincipalV1(principal, binding.Principal) {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	first, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, binding.ThreadID)
	if err != nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	observation, err := h.turnSecurity.Observer.Observe(stringField(first.Thread, "workspace"))
	if err != nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	release, err := h.runtimeSubagentState().AcquireSecurityScopeRead(ctx, binding.ThreadID, observation.WorkspaceRealPath, principal.TenantID, principal.UserID)
	if err != nil || release == nil {
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	if observation.WorkspaceRealPath != binding.Original.WorkspaceRealPath || a.validateBoundV1(ctx, binding) != nil {
		release()
		return port.AuthorizedV1{}, nil, port.ErrUnavailableV1
	}
	return port.AuthorizedV1{Binding: binding}, release, nil
}
func (a runtimeProcessDisplayAccessV1) ValidateCurrent(ctx context.Context, expected port.AuthorizedV1) error {
	return a.validateBoundV1(ctx, expected.Binding)
}
func (a runtimeProcessDisplayAccessV1) validateBoundV1(ctx context.Context, binding port.BindingV1) error {
	h := a.handler
	if ctx == nil || ctx.Err() != nil || h == nil || h.runtimeControl().ShuttingDown() || h.store == nil || h.turnSecurity.Observer == nil || h.toolSnapshotHistoryValidate == nil || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, binding.Principal) != nil {
		return port.ErrUnavailableV1
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, binding.ThreadID)
	if err != nil || stringField(snapshot.Thread, "status") == "deleted" || stringField(snapshot.Thread, "id") != binding.ThreadID {
		return port.ErrUnavailableV1
	}
	frozen, err := turnapp.FrozenSecurityContextForTurn(snapshot.Thread, binding.TurnID)
	if err != nil || !reflect.DeepEqual(frozen, binding.Original) || frozen.TenantID != binding.Principal.TenantID || frozen.UserID != binding.Principal.UserID {
		return port.ErrUnavailableV1
	}
	current, err := snapshotapp.CurrentOwnContextV1(snapshot.Thread, frozen)
	if err != nil {
		return port.ErrUnavailableV1
	}
	observation, err := h.turnSecurity.Observer.Observe(stringField(snapshot.Thread, "workspace"))
	if err != nil || observation.WorkspaceRealPath != frozen.WorkspaceRealPath {
		return port.ErrUnavailableV1
	}
	if turnsecurityapp.ValidateCurrentOrdinaryEffect(turnsecurityapp.CurrentValidationInput{OperationContext: ctx, Identity: h.turnSecurity.Identity, Observer: h.turnSecurity.Observer, RiskAuthority: h.turnSecurity.RiskAuthority, SnapshotAuthority: h.turnSecurity.SnapshotAuthority, SnapshotAuthorityV2: h.turnSecurity.SnapshotAuthorityV2, Context: current, Workspace: frozen.WorkspaceRealPath}) != nil || h.toolSnapshotHistoryValidate(ctx, snapshot.Thread, frozen) != nil {
		return port.ErrUnavailableV1
	}
	if ctx.Err() != nil || h.runtimeControl().ShuttingDown() || turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, binding.Principal) != nil {
		return port.ErrUnavailableV1
	}
	return nil
}
func (h *runtimeServerHandler) beginProcessDisplayV1(ctx context.Context, frozen domainsecurity.TurnSecurityContext, secrets ...string) *processapp.ProducerV1 {
	if ctx == nil || ctx.Err() != nil || h == nil || h.runtimeControl().ShuttingDown() || h.processDisplay == nil {
		return nil
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil || principal.TenantID != frozen.TenantID || principal.UserID != frozen.UserID {
		return nil
	}
	return h.processDisplay.BeginV1(port.BindingV1{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, Principal: principal, Original: frozen}, secrets...)
}

// Called under the actual settlement effect scope. Read back the real closed
// primary records and settled grant; a capture neither supplies this identity
// nor authorizes it. Delivery retains its independent fresh current-scope lease.
func (h *runtimeServerHandler) processToolSettlementBindingV2(ctx context.Context, pending runtimePendingToolCall, expectedIsError ...bool) (port.BindingV1, domainprocess.ToolBindingV1, bool) {
	if ctx == nil || ctx.Err() != nil || h == nil || h.store == nil || h.processDisplay == nil || h.runtimeControl().ShuttingDown() {
		return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil || principal.TenantID != pending.SecurityContext.TenantID || principal.UserID != pending.SecurityContext.UserID {
		return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
	}
	binding := port.BindingV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, Principal: principal, Original: pending.SecurityContext}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, pending.ThreadID)
	if err != nil {
		return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
	}
	var call, result map[string]any
	seen := false
	turns, _ := snapshot.Thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != pending.TurnID {
			continue
		}
		if seen {
			return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
		}
		seen = true
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "tool_call" && stringField(item, "callId") == pending.Call.ID {
				if call != nil {
					return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
				}
				call = item
			}
			if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == pending.Call.ID {
				if result != nil {
					return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
				}
				result = item
			}
		}
	}
	if len(expectedIsError) > 1 || call == nil || result == nil || stringField(call, "id") != pending.ToolCallItemID {
		return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
	}
	if len(expectedIsError) == 1 {
		actual, ok := result["isError"].(bool)
		if !ok || actual != expectedIsError[0] {
			return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
		}
	}
	tool := domainprocess.ToolBindingV1{CallID: pending.Call.ID, ResultItemID: stringField(result, "id"), ToolName: pending.Call.Name, GrantID: pending.ExecutionGrant.GrantID}
	if (runtimeProcessDisplayAccessV1{handler: h}).ValidateToolCurrent(ctx, binding, tool) != nil {
		return port.BindingV1{}, domainprocess.ToolBindingV1{}, false
	}
	return binding, tool, true
}

func (h *runtimeServerHandler) registerProcessToolSettlementV2(ctx context.Context, pending runtimePendingToolCall) {
	binding, tool, valid := h.processToolSettlementBindingV2(ctx, pending)
	if valid {
		h.processDisplay.ToolMetadataV2(binding, pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, tool.ResultItemID, 0)
	}
}

func (h *runtimeServerHandler) publishProcessToolCaptureV1(ctx context.Context, pending runtimePendingToolCall, raw domainprocess.RawCaptureV1) {
	var expectedIsError []bool
	if raw.Kind == "shell" {
		switch raw.Status {
		case "completed", "unknown":
			expectedIsError = []bool{false}
		case "failed", "timeout", "canceled":
			expectedIsError = []bool{true}
		default:
			return
		}
	}
	// Foreground Bash capture status and the actual signed durable tool result
	// must agree before a leaf can select the terminal/error fallback.
	binding, tool, valid := h.processToolSettlementBindingV2(ctx, pending, expectedIsError...)
	if !valid {
		return
	}
	producer := h.beginProcessDisplayV1(ctx, pending.SecurityContext, domainprocess.KnownSecretsV1(ctx)...)
	if producer == nil {
		return
	}
	defer producer.FinishV1()
	if producer.BindingV2() != binding || !producer.ToolReadyV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, pending.ToolScope...) || !producer.ToolMetadataV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, tool.ResultItemID, 0) {
		return
	}
	raw.ToolBinding = tool
	producer.ToolV1(pending.Call.ID, raw)
}
func (h *runtimeServerHandler) ProcessLocalDisplayV1() http.Handler {
	return httpapi.ProcessLocalDisplayHandlerV1{Service: h.processDisplay}
}

// Tool display retains its actual signed settlement authority. Reasoning never
// calls this method or invents a tool call/grant to obtain read authority.
func (a runtimeProcessDisplayAccessV1) ValidateToolCurrent(ctx context.Context, binding port.BindingV1, tool domainprocess.ToolBindingV1) error {
	h := a.handler
	if tool.InputArgsHash != "" {
		return a.validateActiveToolInputV2(ctx, binding, tool)
	}
	if ctx == nil || ctx.Err() != nil || h == nil || h.runtimeControl().ShuttingDown() || h.store == nil || !domainsecurity.IsSHA256Hex(tool.GrantID) || tool.ResultItemID != domaintoolresult.ToolResultItemIDV1(binding.TurnID, tool.CallID) {
		return port.ErrUnavailableV1
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, binding.ThreadID)
	if err != nil {
		return port.ErrUnavailableV1
	}
	var call, result map[string]any
	turns, _ := snapshot.Thread["turns"].([]any)
	seen := false
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != binding.TurnID {
			continue
		}
		if seen {
			return port.ErrUnavailableV1
		}
		seen = true
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "tool_call" && stringField(item, "callId") == tool.CallID {
				if call != nil {
					return port.ErrUnavailableV1
				}
				call = item
			}
			if stringField(item, "id") == tool.ResultItemID || stringField(item, "kind") == "tool_result" && stringField(item, "callId") == tool.CallID {
				if result != nil || stringField(item, "id") != tool.ResultItemID || stringField(item, "kind") != "tool_result" {
					return port.ErrUnavailableV1
				}
				result = item
			}
		}
	}
	if call == nil || result == nil || stringField(call, "id") != domaintoolcall.ToolCallItemIDV1(binding.TurnID, tool.CallID) || stringField(call, "toolName") != tool.ToolName || stringField(result, "toolName") != tool.ToolName || stringField(call, "threadId") != binding.ThreadID || stringField(call, "turnId") != binding.TurnID || stringField(result, "threadId") != binding.ThreadID || stringField(result, "turnId") != binding.TurnID || stringField(result, "callId") != tool.CallID || stringField(result, "executionGrantId") != tool.GrantID || stringField(result, "contextDigest") != binding.Original.ContextDigest {
		return port.ErrUnavailableV1
	}
	projected, closed := domaintoolresult.PrivateDurableToolResultItemRecordV1(result)
	if !closed || !reflect.DeepEqual(contracts.CloneMap(projected), contracts.CloneMap(result)) || projected["contextEpoch"] != binding.Original.ContextEpoch {
		return port.ErrUnavailableV1
	}
	_, changed, err := turnapp.EnsureOrdinaryToolSettlementExact(turnapp.OrdinaryToolSettlementInput{Thread: snapshot.Thread, ThreadID: binding.ThreadID, TurnID: binding.TurnID, ToolCallItemID: stringField(call, "id"), CallID: tool.CallID, ToolName: tool.ToolName, Status: stringField(result, "status"), Timestamp: stringField(result, "finishedAt"), ResultItem: result})
	if err != nil || changed {
		return port.ErrUnavailableV1
	}
	registry, err := executiongrantapp.RegistryFromThread(binding.ThreadID, snapshot.Thread, binding.TurnID)
	if err != nil {
		return port.ErrUnavailableV1
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, tool.GrantID)
	if !found || entry.Status != domainsecurity.GrantRegistrySettled || entry.Grant.ContextDigest != binding.Original.ContextDigest || entry.Grant.ToolCallID != tool.CallID || entry.Grant.ToolName != tool.ToolName || executiongrantapp.VerifyThreadGrantMembership(binding.ThreadID, snapshot.Thread, binding.TurnID, entry.Grant, domainsecurity.GrantRegistrySettled) != nil {
		return port.ErrUnavailableV1
	}
	if ctx.Err() != nil || h.runtimeControl().ShuttingDown() {
		return port.ErrUnavailableV1
	}
	return turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, binding.Principal)
}

// Every running input binds the actual primary call and its original signed
// argument hash. Dispatch is not proof of effect completion. Settled reads
// retain their independent exact result/grant validation.
func (a runtimeProcessDisplayAccessV1) validateActiveToolInputV2(ctx context.Context, binding port.BindingV1, tool domainprocess.ToolBindingV1) error {
	h := a.handler
	if tool.ResultItemID != "" || !domainsecurity.IsHostToolCallIDV1(tool.CallID) || !domainsecurity.IsSHA256Hex(tool.GrantID) || !domainsecurity.IsSHA256Hex(tool.InputArgsHash) || a.validateBoundV1(ctx, binding) != nil {
		return port.ErrUnavailableV1
	}
	snapshot, err := h.store.ReadPrimaryThreadSnapshotV1(ctx, binding.ThreadID)
	if err != nil {
		return port.ErrUnavailableV1
	}
	current, err := snapshotapp.CurrentOwnContextV1(snapshot.Thread, binding.Original)
	if err != nil || current.ContextDigest != binding.Original.ContextDigest || current.ContextEpoch != binding.Original.ContextEpoch {
		return port.ErrUnavailableV1
	}
	var call map[string]any
	seen := false
	turns, _ := snapshot.Thread["turns"].([]any)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		if stringField(turn, "id") != binding.TurnID {
			continue
		}
		if seen {
			return port.ErrUnavailableV1
		}
		seen = true
		items, _ := turn["items"].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") == "tool_result" && stringField(item, "callId") == tool.CallID {
				return port.ErrUnavailableV1
			}
			if stringField(item, "kind") == "tool_call" && stringField(item, "callId") == tool.CallID {
				if call != nil {
					return port.ErrUnavailableV1
				}
				call = item
			}
		}
	}
	if call == nil || stringField(call, "id") != domaintoolcall.ToolCallItemIDV1(binding.TurnID, tool.CallID) || stringField(call, "status") != "running" || stringField(call, "toolName") != tool.ToolName || stringField(call, "threadId") != binding.ThreadID || stringField(call, "turnId") != binding.TurnID || stringField(call, "contextDigest") != binding.Original.ContextDigest {
		return port.ErrUnavailableV1
	}
	if tool.ToolName == "bash" && stringField(call, "toolKind") != "command_execution" || tool.ToolName == "web_fetch" && stringField(call, "toolKind") != "tool_call" {
		return port.ErrUnavailableV1
	}
	epochValid := false
	switch epoch := call["contextEpoch"].(type) {
	case json.Number:
		value, err := epoch.Int64()
		epochValid = err == nil && value > 0 && uint64(value) == binding.Original.ContextEpoch
	case float64:
		epochValid = epoch > 0 && epoch <= 9007199254740991 && epoch == float64(binding.Original.ContextEpoch)
	case uint64:
		epochValid = epoch == binding.Original.ContextEpoch
	}
	if !epochValid {
		return port.ErrUnavailableV1
	}
	registry, err := executiongrantapp.RegistryFromThread(binding.ThreadID, snapshot.Thread, binding.TurnID)
	if err != nil {
		return port.ErrUnavailableV1
	}
	entry, found := domainsecurity.ExecutionGrantRegistryEntryByID(registry, tool.GrantID)
	if !found || entry.Status != domainsecurity.GrantRegistryActive || entry.Grant.ToolCallID != tool.CallID || entry.Grant.ToolName != tool.ToolName || entry.Grant.ContextDigest != binding.Original.ContextDigest || entry.Grant.ArgsHash != tool.InputArgsHash || executiongrantapp.ValidateExecutionGrantForToolName(binding.Original, entry.Grant, tool.ToolName) != nil || executiongrantapp.VerifyThreadGrantMembership(binding.ThreadID, snapshot.Thread, binding.TurnID, entry.Grant, domainsecurity.GrantRegistryActive) != nil {
		return port.ErrUnavailableV1
	}
	// The primary call may retain the pre-approval grant. Its validated registry
	// chain must still bind the same real arguments; never accept an arbitrary ID.
	originalGrant, grantErr := domainsecurity.ParseExecutionGrant(call["executionGrant"])
	if grantErr != nil || originalGrant.ArgsHash != tool.InputArgsHash || originalGrant.ToolCallID != tool.CallID || originalGrant.ToolName != tool.ToolName || executiongrantapp.ValidateStoredExecutionGrantItemV1(call, originalGrant) != nil {
		return port.ErrUnavailableV1
	}
	if ctx.Err() != nil || h.runtimeControl().ShuttingDown() {
		return port.ErrUnavailableV1
	}
	return turnsecurityapp.ValidateResolvedPrincipal(ctx, h.turnSecurity.Identity, binding.Principal)
}

// Background jobs retain their own product surface and no command is inferred.
func (h *runtimeServerHandler) registerProcessShellInputV1(ctx context.Context, pending runtimePendingToolCall) {
	if pending.Call.Name != "bash" || h == nil || h.processDisplay == nil || executiongrantapp.ValidateExecutionGrantForCall(pending.SecurityContext, pending.ExecutionGrant, pending.Call) != nil {
		return
	}
	args, err := domainsecurity.DecodeCanonicalJSONObject(pending.Call.Arguments)
	if err != nil || boolField(args, "run_in_background") || boolField(args, "runInBackground") {
		return
	}
	command, ok := args["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		return
	}
	producer := h.beginProcessDisplayV1(ctx, pending.SecurityContext, domainprocess.KnownSecretsV1(ctx)...)
	if producer == nil {
		return
	}
	defer producer.FinishV1()
	tool := domainprocess.ToolBindingV1{CallID: pending.Call.ID, ToolName: "bash", GrantID: pending.ExecutionGrant.GrantID, InputArgsHash: domainsecurity.CanonicalJSONHash(pending.Call.Arguments)}
	if (runtimeProcessDisplayAccessV1{handler: h}).ValidateToolCurrent(ctx, producer.BindingV2(), tool) != nil || !producer.ToolReadyV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, pending.ToolScope...) {
		return
	}
	producer.ShellInputV1(pending.Call.ID, command, tool)
}

func (h *runtimeServerHandler) registerProcessWebInputV1(ctx context.Context, pending runtimePendingToolCall) {
	if pending.Call.Name != "web_fetch" || h == nil || h.processDisplay == nil || executiongrantapp.ValidateExecutionGrantForCall(pending.SecurityContext, pending.ExecutionGrant, pending.Call) != nil {
		return
	}
	args, decodeErr := domainsecurity.DecodeCanonicalJSONObject(pending.Call.Arguments)
	if decodeErr != nil {
		return
	}
	requestURL, ok := args["url"].(string)
	requestURL = strings.TrimSpace(requestURL)
	if !ok || !domainprocess.ValidWebRequestURLV1(requestURL) {
		return
	}
	principal, err := turnsecurityapp.ResolveCurrentPrincipal(ctx, h.turnSecurity.Identity)
	if err != nil || principal.TenantID != pending.SecurityContext.TenantID || principal.UserID != pending.SecurityContext.UserID {
		return
	}
	binding := port.BindingV1{ThreadID: pending.ThreadID, TurnID: pending.TurnID, Principal: principal, Original: pending.SecurityContext}
	tool := domainprocess.ToolBindingV1{CallID: pending.Call.ID, ToolName: "web_fetch", GrantID: pending.ExecutionGrant.GrantID, InputArgsHash: domainsecurity.CanonicalJSONHash(pending.Call.Arguments)}
	if (runtimeProcessDisplayAccessV1{handler: h}).ValidateToolCurrent(ctx, binding, tool) != nil {
		return
	}
	producer := h.beginProcessDisplayV1(ctx, pending.SecurityContext, domainprocess.KnownSecretsV1(ctx)...)
	if producer == nil {
		return
	}
	defer producer.FinishV1()
	if producer.BindingV2() != binding {
		return
	}
	producer.WebInputV1(pending.Call.ID, requestURL, tool)
}

func (h *runtimeServerHandler) registerProcessToolStartedV2(pending runtimePendingToolCall, seq uint64) {
	producer := h.beginProcessDisplayV1(context.Background(), pending.SecurityContext)
	if producer == nil {
		return
	}
	defer producer.FinishV1()
	if producer.ToolReadyV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, pending.ToolScope...) {
		producer.ToolMetadataV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, "", seq)
	}
}

// A completed native source may remain valid as own history, while a running
// assistant draft must belong to the exact current epoch/digest. This check is
// performed under the already acquired native scope lease.
func (a runtimeProcessDisplayAccessV1) ValidateAssistantDraftCurrent(ctx context.Context, binding port.BindingV1) error {
	if a.validateBoundV1(ctx, binding) != nil {
		return port.ErrUnavailableV1
	}
	snapshot, err := a.handler.store.ReadPrimaryThreadSnapshotV1(ctx, binding.ThreadID)
	if err != nil {
		return port.ErrUnavailableV1
	}
	current, err := snapshotapp.CurrentOwnContextV1(snapshot.Thread, binding.Original)
	if err != nil || current.ContextEpoch != binding.Original.ContextEpoch || current.ContextDigest != binding.Original.ContextDigest {
		return port.ErrUnavailableV1
	}
	return nil
}

// Register a full input only at the actual authorized dispatch, not from a
// public projection or the decoded execution map.
func (h *runtimeServerHandler) registerProcessToolInputV2(ctx context.Context, pending runtimePendingToolCall) {
	if h == nil || h.processDisplay == nil || executiongrantapp.ValidateExecutionGrantForCall(pending.SecurityContext, pending.ExecutionGrant, pending.Call) != nil {
		return
	}
	producer := h.beginProcessDisplayV1(ctx, pending.SecurityContext, domainprocess.KnownSecretsV1(ctx)...)
	if producer == nil {
		return
	}
	defer producer.FinishV1()
	tool := domainprocess.ToolBindingV1{CallID: pending.Call.ID, ToolName: pending.Call.Name, GrantID: pending.ExecutionGrant.GrantID, InputArgsHash: executiongrantapp.ArgumentsHash(pending.Call.Arguments)}
	if (runtimeProcessDisplayAccessV1{handler: h}).ValidateToolCurrent(ctx, producer.BindingV2(), tool) != nil || !producer.ToolReadyV2(pending.ProviderNamespace, pending.Call.ID, pending.ToolCallItemID, pending.ToolScope...) {
		return
	}
	producer.ToolInputV2(pending.Call.ID, pending.Call.Arguments, tool)
}
