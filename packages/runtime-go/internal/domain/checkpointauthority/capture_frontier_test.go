package checkpointauthority

import (
	"encoding/json"
	"testing"
	"time"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestOperationGroupCaptureFrontierBindsEveryOrderedTerminal(t *testing.T) {
	now := time.Unix(1_700_500_000, 0).UTC()
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-frontier", TurnID: "turn-frontier", WorkspaceRealPath: "/workspace",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	states := make([]OperationGroupStateV2, 0, 2)
	for index, path := range []string{"a.txt", "b.txt"} {
		arguments, _ := json.Marshal(map[string]any{"path": path, "content": "after"})
		grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
			Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "write_file",
			ToolCallID: checkpointTestHostToolCallID(t, "call-"+path), ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
			SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte(`["write_file"]`)),
			ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
		})
		intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
			SecurityContext: securityContext, ExecutionGrant: grant,
			CheckpointID: domaincheckpointref.RuntimeID("frontier"), SourceWorkspaceCheckpointID: "frontier",
			OperationOrdinal: uint64(index + 1), ToolName: "write_file", ArgumentsJSON: arguments,
			Paths: []OperationPathInputV2{operationPathInputWithWorkspaceAuthority(OperationPathInputV2{
				ArgumentKey: "path", RequestedPath: path, RelativePath: path, Role: "target",
				BeforeExisted: false, BeforeAvailable: false, ExpectedAfterExisted: true,
				ExpectedAfterHash: domainsecurity.SHA256Hex([]byte("after")),
			}, securityContext.WorkspaceRealPath)},
			CreatedAt: now.Add(time.Duration(index+1) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		terminal, err := NewOperationGroupTerminalV2(OperationGroupTerminalInputV2{
			Intent: intent, Status: "completed", ReasonCode: "mutation_completed",
			ObservedPaths: []ObservedOperationPathV2{
				observedForOperationPath(intent.Paths[0], "exact", true, domainsecurity.SHA256Hex([]byte("after"))),
			},
			SettledAt: now.Add(time.Duration(index+3) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		states = append(states, OperationGroupStateV2{Intent: intent, Terminal: &terminal})
	}
	frontier, hasChanges, err := NewOperationGroupCaptureFrontierV2(states)
	if err != nil || !hasChanges || !domaincheckpointref.IsCaptureEventIDV2(frontier.CaptureEventID) || frontier.CompletedOperationGroupCount != 2 {
		t.Fatalf("frontier mismatch: %#v changed=%v err=%v", frontier, hasChanges, err)
	}
	firstID := frontier.CaptureEventID
	states[1].Terminal = nil
	if _, _, err := NewOperationGroupCaptureFrontierV2(states); err == nil {
		t.Fatal("open frontier was accepted")
	}
	states = states[:1]
	frontier, _, err = NewOperationGroupCaptureFrontierV2(states)
	if err != nil || frontier.CaptureEventID == firstID {
		t.Fatal("frontier id did not bind the complete ordered terminal prefix")
	}
}

func TestOperationGroupCaptureFrontierValidatesNoEffectAndCompleteInventory(t *testing.T) {
	now := time.Unix(1_700_600_000, 0).UTC()
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-frontier-inventory", TurnID: "turn-frontier-inventory", WorkspaceRealPath: "/workspace",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	noEffect := captureFrontierState(t, securityContext, now, 1, "a.txt", "frontier-inventory", "no_effect")
	frontier, hasCompleted, err := NewOperationGroupCaptureFrontierV2([]OperationGroupStateV2{noEffect})
	if err != nil || hasCompleted || frontier.CompletedOperationGroupCount != 0 || !domaincheckpointref.IsCaptureEventIDV2(frontier.CaptureEventID) {
		t.Fatalf("valid no-effect-only frontier was not closed deterministically: %#v completed=%t err=%v", frontier, hasCompleted, err)
	}
	completed := captureFrontierState(t, securityContext, now, 2, "b.txt", "frontier-inventory", "completed")
	frontier, hasCompleted, err = NewOperationGroupCaptureFrontierV2([]OperationGroupStateV2{noEffect, completed})
	if err != nil || !hasCompleted || frontier.CompletedOperationGroupCount != 1 {
		t.Fatalf("mixed no-effect/completed frontier was rejected: %#v completed=%t err=%v", frontier, hasCompleted, err)
	}

	gap := captureFrontierState(t, securityContext, now, 3, "c.txt", "frontier-inventory", "completed")
	if _, _, err := NewOperationGroupCaptureFrontierV2([]OperationGroupStateV2{noEffect, gap}); err == nil {
		t.Fatal("operation ordinal gap was accepted")
	}
	crossSource := captureFrontierState(t, securityContext, now, 2, "b.txt", "other-frontier", "completed")
	if _, _, err := NewOperationGroupCaptureFrontierV2([]OperationGroupStateV2{noEffect, crossSource}); err == nil {
		t.Fatal("operation frontier crossed source checkpoint authority")
	}
	quarantined := captureFrontierState(t, securityContext, now, 2, "b.txt", "frontier-inventory", "quarantined")
	if _, _, err := NewOperationGroupCaptureFrontierV2([]OperationGroupStateV2{noEffect, quarantined}); err == nil {
		t.Fatal("quarantined terminal was accepted into a captured frontier")
	}
}

func captureFrontierState(
	t *testing.T,
	securityContext domainsecurity.TurnSecurityContext,
	now time.Time,
	ordinal uint64,
	path string,
	sourceCheckpointID string,
	status string,
) OperationGroupStateV2 {
	t.Helper()
	arguments, _ := json.Marshal(map[string]any{"path": path, "content": "after"})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "write_file",
		ToolCallID: checkpointTestHostToolCallID(t, "call-"+path+"-"+sourceCheckpointID), ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte(`["write_file"]`)),
		ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	beforeHash := domainsecurity.SHA256Hex([]byte("before"))
	afterHash := domainsecurity.SHA256Hex([]byte("after"))
	intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("frontier-inventory"), SourceWorkspaceCheckpointID: sourceCheckpointID,
		OperationOrdinal: ordinal, ToolName: "write_file", ArgumentsJSON: arguments,
		Paths: []OperationPathInputV2{operationPathInputWithWorkspaceAuthority(OperationPathInputV2{
			ArgumentKey: "path", RequestedPath: path, RelativePath: path, Role: "target",
			BeforeExisted: true, BeforeAvailable: true, BeforeHash: beforeHash, BeforeContent: "before",
			ExpectedAfterExisted: true, ExpectedAfterHash: afterHash,
		}, securityContext.WorkspaceRealPath)},
		CreatedAt: now.Add(time.Duration(ordinal) * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	observed := observedForOperationPath(intent.Paths[0], "exact", true, afterHash)
	reason := "mutation_completed"
	switch status {
	case "no_effect":
		observed.Hash = beforeHash
		reason = "mutation_failed_before_effect"
	case "quarantined":
		observed.Hash = domainsecurity.SHA256Hex([]byte("diverged"))
		reason = "filesystem_diverged"
	}
	terminal, err := NewOperationGroupTerminalV2(OperationGroupTerminalInputV2{
		Intent: intent, Status: status, ReasonCode: reason, ObservedPaths: []ObservedOperationPathV2{observed},
		SettledAt: now.Add(time.Duration(ordinal+10) * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return OperationGroupStateV2{Intent: intent, Terminal: &terminal}
}
