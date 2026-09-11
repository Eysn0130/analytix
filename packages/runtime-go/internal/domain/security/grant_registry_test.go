package security

import (
	"reflect"
	"testing"
	"time"
)

func TestExecutionGrantRegistryRequiresHostMembershipAndTracksApproval(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	context := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", IssuedAt: now,
	})
	pending := registryGrantForTest(context, "pending", now, now.Add(15*time.Minute))
	registry := NewExecutionGrantRegistry(context.ThreadID)
	if err := VerifyExecutionGrantMembership(registry, context.ThreadID, context.TurnID, pending, GrantRegistryPending); err == nil {
		t.Fatal("a structurally valid self-hashed grant must not have host registry membership")
	}
	registered, err := RegisterExecutionGrant(registry, context.ThreadID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutionGrantMembership(registered, context.ThreadID, context.TurnID, pending, GrantRegistryPending); err != nil {
		t.Fatalf("registered pending grant rejected: %v", err)
	}
	approved := registryGrantForTest(context, "approved", now.Add(time.Minute), now.Add(15*time.Minute))
	transitioned, err := ApproveRegisteredExecutionGrant(registered, pending, approved, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutionGrantMembership(transitioned, context.ThreadID, context.TurnID, pending, GrantRegistryPending); err == nil {
		t.Fatal("superseded pending grant remained executable")
	}
	if err := VerifyExecutionGrantMembership(transitioned, context.ThreadID, context.TurnID, approved, GrantRegistryActive); err != nil {
		t.Fatalf("approved registry member rejected: %v", err)
	}
	settled, err := SettleRegisteredExecutionGrant(transitioned, approved.GrantID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyExecutionGrantMembership(settled, context.ThreadID, context.TurnID, approved, GrantRegistryActive); err == nil {
		t.Fatal("settled grant remained executable")
	}
}

func TestExecutionGrantRegistryTransitionsDoNotMutateAuthoritySnapshots(t *testing.T) {
	now := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	context := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread_snapshot", TurnID: "turn_snapshot", WorkspaceRealPath: "/workspace", IssuedAt: now,
	})
	pending := registryGrantForTest(context, "pending", now, now.Add(15*time.Minute))
	registered, err := RegisterExecutionGrant(NewExecutionGrantRegistry(context.ThreadID), context.ThreadID, pending, now)
	if err != nil {
		t.Fatal(err)
	}
	registeredSnapshot, err := ParseExecutionGrantRegistry(ExecutionGrantRegistryRecord(registered))
	if err != nil {
		t.Fatal(err)
	}
	approved := registryGrantForTest(context, "approved", now.Add(time.Minute), now.Add(15*time.Minute))
	transitioned, err := ApproveRegisteredExecutionGrant(registered, pending, approved, now.Add(time.Minute))
	if err != nil || !reflect.DeepEqual(registered, registeredSnapshot) || ValidateExecutionGrantRegistry(registered) != nil {
		t.Fatalf("approval mutated its pending registry snapshot: err=%v", err)
	}
	activeSnapshot, err := ParseExecutionGrantRegistry(ExecutionGrantRegistryRecord(transitioned))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SettleRegisteredExecutionGrant(transitioned, approved.GrantID, now.Add(2*time.Minute)); err != nil || !reflect.DeepEqual(transitioned, activeSnapshot) || ValidateExecutionGrantRegistry(transitioned) != nil {
		t.Fatalf("settlement mutated its active registry snapshot: err=%v", err)
	}
	if _, err := RevokeRegisteredExecutionGrant(transitioned, approved.GrantID, now.Add(2*time.Minute)); err != nil || !reflect.DeepEqual(transitioned, activeSnapshot) || ValidateExecutionGrantRegistry(transitioned) != nil {
		t.Fatalf("revocation mutated its active registry snapshot: err=%v", err)
	}
}

func TestExecutionGrantRegistryFailsClosedOnTamperAndReplay(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	context := NewTurnSecurityContext(TurnSecurityContextInput{
		ThreadID: "thread_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", IssuedAt: now,
	})
	grant := registryGrantForTest(context, "not_required", now, now.Add(15*time.Minute))
	registry, err := RegisterExecutionGrant(NewExecutionGrantRegistry(context.ThreadID), context.ThreadID, grant, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterExecutionGrant(registry, context.ThreadID, grant, now); err == nil {
		t.Fatal("duplicate grant replay must fail closed")
	}
	record := ExecutionGrantRegistryRecord(registry)
	entries := record["entries"].([]any)
	entry := entries[0].(map[string]any)
	entry["status"] = string(GrantRegistrySettled)
	if _, err := ParseExecutionGrantRegistry(record); err == nil {
		t.Fatal("registry status tamper without a new host digest was accepted")
	}
}

func registryGrantForTest(context TurnSecurityContext, approvalState string, issuedAt time.Time, expiresAt time.Time) ExecutionGrant {
	return NewExecutionGrant(ExecutionGrantInput{
		Context: context, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "read", ToolCallID: securityTestHostToolCallID("registry"),
		ArgsHash: SHA256Hex([]byte("args")), SchemaHash: SHA256Hex([]byte("schema")), ScopeHash: SHA256Hex([]byte("scope")),
		ReadOnly: true, ApprovalState: approvalState, IssuedAt: issuedAt, ExpiresAt: expiresAt,
	})
}
