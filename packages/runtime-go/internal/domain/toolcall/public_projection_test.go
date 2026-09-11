package toolcall

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func TestPublicToolCallArgumentsProjectionIsClosed(t *testing.T) {
	projection := PublicToolCallArgumentsProjectionRecordV1()
	if _, err := ParsePublicToolCallArgumentsProjectionV1(projection); err != nil {
		t.Fatalf("parse projection: %v", err)
	}
	projection["path"] = "/private/case/account-622202.txt"
	if _, err := ParsePublicToolCallArgumentsProjectionV1(projection); err == nil {
		t.Fatal("unknown argument projection fields must fail closed")
	}
}

func TestPrivateDurableToolCallWithholdsArgumentsAndRetainsExactHostGrant(t *testing.T) {
	issuedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: issuedAt,
	})
	arguments := map[string]any{"path": "/private/case/account-622202.txt", "content": "PRIVATE_ARGUMENT_SENTINEL"}
	argumentsBody, _ := json.Marshal(arguments)
	toolCallID := toolidentitytest.MustHostToolCallIDV1("domain-toolcall-private-durable")
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "write_file", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentsBody), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ReadOnly: false, ApprovalState: "approved",
		IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(time.Minute),
	})
	grantBody, _ := json.Marshal(grant)
	grantRecord := map[string]any{}
	_ = json.Unmarshal(grantBody, &grantRecord)
	item := map[string]any{
		"id": "item-1", "turnId": "turn-1", "threadId": "thread-1", "kind": "tool_call",
		"toolName": "write_file", "callId": toolCallID, "arguments": arguments,
		"contextDigest": securityContext.ContextDigest, "executionGrantId": grant.GrantID, "executionGrant": grantRecord,
	}
	durable, ok := PrivateDurableToolCallItemRecordV1(item)
	if !ok || durable["executionGrant"] == nil {
		t.Fatalf("valid host grant was not retained: ok=%v durable=%#v", ok, durable)
	}
	serialized, _ := json.Marshal(durable)
	if stringValue(durable, "executionGrantId") != grant.GrantID ||
		bytes.Contains(serialized, []byte("PRIVATE_ARGUMENT_SENTINEL")) || bytes.Contains(serialized, []byte("account-622202")) {
		t.Fatalf("private durable tool call leaked exact arguments or lost identity: %s", serialized)
	}

	tampered := cloneJSONValue(item).(map[string]any)
	tampered["arguments"] = map[string]any{"path": "/different"}
	if projected, ok := PrivateDurableToolCallItemRecordV1(tampered); ok || projected["executionGrant"] != nil {
		t.Fatalf("mismatched raw arguments retained execution authority: ok=%v projected=%#v", ok, projected)
	}
}

func TestPublicToolCallItemDropsRawArgumentsAndAuthority(t *testing.T) {
	const account = "6222020202020202020"
	item := map[string]any{
		"id": "item_tool_turn-1_provider_call_" + account, "turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "pending",
		"createdAt": "2026-07-14T00:00:00Z", "kind": "tool_call", "toolName": "read_file", "callId": "provider_call_" + account,
		"toolKind": "file_change", "arguments": map[string]any{"path": "/private/case/account-622202.txt"},
		"contextDigest": "private-digest", "contextEpoch": float64(9), "executionGrantId": "private-grant",
		"executionGrant": map[string]any{"argsHash": "private"}, "summary": "private", "localFilePath": "/private",
	}
	got := PublicToolCallItemRecordV1(item)
	encoded, _ := json.Marshal(got)
	wantArguments := PublicToolCallArgumentsProjectionRecordV1()
	if !reflect.DeepEqual(got["arguments"], wantArguments) {
		t.Fatalf("unexpected arguments projection: %#v", got["arguments"])
	}
	for _, forbidden := range []string{"contextDigest", "contextEpoch", "executionGrantId", "executionGrant", "summary", "localFilePath"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("private field %q crossed the public projection: %#v", forbidden, got)
		}
	}
	if strings.Contains(string(encoded), account) || got["legacyIdentityWithheld"] != true || got["id"] != nil || got["callId"] != nil {
		t.Fatalf("legacy provider identity crossed the public projection: %s", encoded)
	}
}

func TestPublicToolCallItemNormalizesSkillKindToClosedContract(t *testing.T) {
	callID := toolidentitytest.MustHostToolCallIDV1("domain-toolcall-public-skill-kind")
	item := map[string]any{
		"turnId": "turn-1", "threadId": "thread-1", "role": "tool", "status": "running",
		"kind": "tool_call", "toolName": "run_skill", "callId": callID, "toolKind": "skill",
		"arguments": map[string]any{"name": "private-skill-package"},
	}
	private, ok := PrivateDurableToolCallItemRecordV1(item)
	if !ok || private["toolKind"] != "skill" {
		t.Fatalf("private durable skill kind was not retained: ok=%v item=%#v", ok, private)
	}
	got := PublicToolCallItemRecordV1(item)

	if got["toolKind"] != "tool_call" || got["callId"] != callID ||
		!reflect.DeepEqual(got["arguments"], PublicToolCallArgumentsProjectionRecordV1()) {
		t.Fatalf("skill tool call did not use the closed public tool kind: %#v", got)
	}
}

func TestPublicToolCallItemDoesNotReflectLifecycleControlText(t *testing.T) {
	const hostile = "PRIVATE_PROVIDER_LIFECYCLE_SENTINEL"
	item := map[string]any{
		"turnId": "turn-1", "threadId": "thread-1", "role": hostile, "status": hostile,
		"kind": hostile, "toolName": "read_file", "callId": "call_host_" + strings.Repeat("a", 64), "toolKind": hostile,
	}
	got := PublicToolCallItemRecordV1(item)
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "tool_call" || got["role"] != "tool" || got["status"] != "failed" ||
		got["lifecycleStatusWithheld"] != true || got["toolKind"] != nil || strings.Contains(string(body), hostile) {
		t.Fatalf("tool-call lifecycle control text was reflected: %s", body)
	}
}
