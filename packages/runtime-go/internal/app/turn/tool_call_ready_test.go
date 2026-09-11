package turn

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestToolCallReadyRecordsBuildDurableItemAndReplayEvent(t *testing.T) {
	createdAt := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	context := newTurnExecutionContextV2(t, "thread_1", "turn_1", "/workspace", 1, createdAt)
	call := domainmodel.ToolCall{ID: turnTestHostToolCallID("ready"), Name: "bash", Arguments: json.RawMessage(`{"cmd":"date"}`)}
	itemID := domaintoolcall.ToolCallItemIDV1(context.TurnID, call.ID)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ApprovalState: "not_required", IssuedAt: createdAt,
	})
	item, event, err := ToolCallReadyRecords(ToolCallReadyInput{
		ThreadID:   " thread_1 ",
		TurnID:     " turn_1 ",
		ItemID:     itemID,
		CreatedAt:  "2026-07-02T00:00:00Z",
		Call:       call,
		ReadyCount: 2,
		ToolKind:   "command_execution",
		Context:    context,
		Grant:      grant,
	})
	if err != nil {
		t.Fatalf("valid tool call rejected: %v", err)
	}
	if item["id"] != itemID ||
		item["threadId"] != "thread_1" ||
		item["turnId"] != "turn_1" ||
		item["toolName"] != "bash" ||
		item["callId"] != call.ID ||
		item["toolKind"] != "command_execution" {
		t.Fatalf("item mismatch: %#v", item)
	}
	args, _ := item["arguments"].(map[string]any)
	if _, err := domaintoolcall.ParsePublicToolCallArgumentsProjectionV1(args); err != nil {
		t.Fatalf("arguments were not replaced by the closed public projection: %#v err=%v", args, err)
	}
	if _, leaked := args["cmd"]; leaked {
		t.Fatalf("raw arguments crossed durable history: %#v", args)
	}
	if event["kind"] != "tool_call_ready" ||
		event["itemId"] != itemID ||
		event["readyCount"] != float64(2) {
		t.Fatalf("event mismatch: %#v", event)
	}
	if item["executionGrant"] == nil {
		t.Fatal("host-issued full grant must be persisted as registry material")
	}
}

func TestToolCallReadyRecordsInvalidArgumentsFailClosed(t *testing.T) {
	input := validToolCallReadyInput(t, "invalid-arguments")
	input.Call.Arguments = json.RawMessage(`{"cmd"`)
	item, event, err := ToolCallReadyRecords(input)
	if err == nil || item != nil || event != nil {
		t.Fatalf("invalid args must fail closed without records, item=%#v event=%#v err=%v", item, event, err)
	}
}

func TestToolCallReadyRecordsRejectsNonHostCallIdentity(t *testing.T) {
	input := validToolCallReadyInput(t, "raw-provider-call")
	const rawProviderID = "provider_call_6222020202020202020"
	input.Call.ID = rawProviderID
	input.ItemID = "item_provider_call_6222020202020202020"
	input.Grant = domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: input.Context, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: input.Call.Name, ToolCallID: rawProviderID,
		ArgsHash: domainsecurity.CanonicalJSONHash(input.Call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ApprovalState: "not_required", IssuedAt: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
	})
	if input.Grant.ToolCallID != rawProviderID {
		t.Fatalf("constructor silently rewrote invalid identity: %#v", input.Grant)
	}
	item, event, err := ToolCallReadyRecords(input)
	if err == nil || item != nil || event != nil || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("raw provider identity must fail closed without echo or records, item=%#v event=%#v err=%v", item, event, err)
	}
}

func TestToolCallReadyRecordsRejectsProviderCallEvenWithValidGrant(t *testing.T) {
	input := validToolCallReadyInput(t, "valid-grant-raw-call")
	const rawProviderID = "provider_call_4111111111111111"
	input.Call.ID = rawProviderID
	input.ItemID = "item_provider_call_4111111111111111"
	item, event, err := ToolCallReadyRecords(input)
	if err == nil || item != nil || event != nil || strings.Contains(err.Error(), rawProviderID) {
		t.Fatalf("provider call identity must fail closed without echo or records, item=%#v event=%#v err=%v", item, event, err)
	}
}

func TestToolCallReadyRecordsRequiresDerivedItemIdentity(t *testing.T) {
	input := validToolCallReadyInput(t, "derived-item")
	expected := input.ItemID
	for name, itemID := range map[string]string{
		"legacy":     "item_tool_1",
		"whitespace": " " + expected + " ",
		"other turn": domaintoolcall.ToolCallItemIDV1("turn_other", input.Call.ID),
	} {
		t.Run(name, func(t *testing.T) {
			attempt := input
			attempt.ItemID = itemID
			item, event, err := ToolCallReadyRecords(attempt)
			if err == nil || item != nil || event != nil {
				t.Fatalf("non-derived item identity must fail closed, item=%#v event=%#v err=%v", item, event, err)
			}
		})
	}
}

func TestToolCallReadyUsesPerCallAuthorityInWitnessedBoundaryContext(t *testing.T) {
	createdAt := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	securityContext := newTurnWitnessedBoundaryContextV2(t, "thread-ready-boundary", "turn-ready-boundary", "/workspace/boundary", createdAt)
	ordinary := readyInputForContext(t, securityContext, "read", `{"path":"README.md"}`, createdAt)
	if item, event, err := ToolCallReadyRecords(ordinary); err != nil || item == nil || event == nil {
		t.Fatalf("ordinary ready record was blocked by unavailable case authority: item=%#v event=%#v err=%v", item, event, err)
	}

	protected := readyInputForContext(t, securityContext, "stage_case_report", `{}`, createdAt)
	if item, event, err := ToolCallReadyRecords(protected); err == nil || item != nil || event != nil {
		t.Fatalf("protected report ready record escaped boundary-only authority: item=%#v event=%#v err=%v", item, event, err)
	}
}

func validToolCallReadyInput(t *testing.T, seed string) ToolCallReadyInput {
	t.Helper()
	createdAt := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	context := newTurnExecutionContextV2(t, "thread_"+seed, "turn_"+seed, "/workspace", 1, createdAt)
	call := domainmodel.ToolCall{ID: turnTestHostToolCallID(seed), Name: "bash", Arguments: json.RawMessage(`{"cmd":"date"}`)}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ApprovalState: "not_required", IssuedAt: createdAt,
	})
	return ToolCallReadyInput{
		ThreadID: context.ThreadID, TurnID: context.TurnID, ItemID: domaintoolcall.ToolCallItemIDV1(context.TurnID, call.ID),
		CreatedAt: createdAt.Format(time.RFC3339), Call: call, Context: context, Grant: grant,
	}
}

func readyInputForContext(t *testing.T, securityContext domainsecurity.TurnSecurityContext, toolName, arguments string, createdAt time.Time) ToolCallReadyInput {
	t.Helper()
	call := domainmodel.ToolCall{
		ID: turnTestHostToolCallID(securityContext.ThreadID + "-" + toolName), Name: toolName, Arguments: json.RawMessage(arguments),
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: call.Name, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(call.Arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: toolName == "read",
		ApprovalState: "not_required", IssuedAt: createdAt,
	})
	return ToolCallReadyInput{
		ThreadID: securityContext.ThreadID, TurnID: securityContext.TurnID,
		ItemID:    domaintoolcall.ToolCallItemIDV1(securityContext.TurnID, call.ID),
		CreatedAt: createdAt.Format(time.RFC3339Nano), Call: call, Context: securityContext, Grant: grant,
	}
}

func newTurnWitnessedBoundaryContextV2(t *testing.T, threadID, turnID, workspace string, issuedAt time.Time) domainsecurity.TurnSecurityContext {
	t.Helper()
	quarantined := newTurnBoundaryOnlyContextV2(t, threadID, turnID, workspace, 1, issuedAt)
	binding, err := securitycontexttest.WitnessedRiskBinding(
		threadID,
		workspace,
		domainsecurity.RiskClassCase,
		quarantined.PublicationPolicy.ThreadRiskPolicyDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: quarantined.ThreadID, TurnID: quarantined.TurnID, WorkspaceRealPath: quarantined.WorkspaceRealPath,
		TenantID: quarantined.TenantID, UserID: quarantined.UserID, CaseID: quarantined.CaseID,
		CaseBindingHash: quarantined.CaseBindingHash, DatasetSnapshotID: quarantined.DatasetSnapshotID,
		SourceManifestHash: quarantined.SourceManifestHash, ContextEpoch: quarantined.ContextEpoch, IssuedAt: issuedAt,
		PublicationPolicy: quarantined.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func turnTestHostToolCallID(seed string) string {
	entropy := sha256.Sum256([]byte("app-turn-test-tool-call:\x00" + seed))
	identity, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		panic(err)
	}
	return identity
}
