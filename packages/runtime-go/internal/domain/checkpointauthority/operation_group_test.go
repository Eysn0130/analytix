package checkpointauthority

import (
	"encoding/json"
	"testing"
	"time"

	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestOperationGroupIntentBindsHostGrantArgumentsAndExpectedState(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	arguments := []byte(`{"path":"a.txt","content":"after"}`)
	securityContext, grant := operationGroupSecurity(t, now, "write_file", "call-1", arguments)
	before := "before"
	intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("workspace-checkpoint-1"), SourceWorkspaceCheckpointID: "workspace-checkpoint-1",
		OperationOrdinal: 1, ToolName: "write_file", ArgumentsJSON: arguments, CreatedAt: now.Add(time.Second),
		Paths: []OperationPathInputV2{operationPathInputWithWorkspaceAuthority(OperationPathInputV2{
			ArgumentKey: "path", RequestedPath: "a.txt", RelativePath: "a.txt", Role: "target",
			BeforeExisted: true, BeforeAvailable: true, BeforeHash: domainsecurity.SHA256Hex([]byte(before)), BeforeContent: before,
			ExpectedAfterExisted: true, ExpectedAfterHash: domainsecurity.SHA256Hex([]byte("after")),
		}, securityContext.WorkspaceRealPath)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if intent.OperationGroupID == "" || intent.IntentDigest == "" || intent.OperationGroupID == intent.IntentDigest {
		t.Fatalf("operation group identity is not independently sealed: %#v", intent)
	}
	if intent.Paths[0].PathAuthoritySchemaVersion != 1 || intent.Paths[0].AuthorityKind != "workspace" ||
		intent.Paths[0].AuthorityRoot != securityContext.WorkspaceRealPath || intent.Paths[0].AuthorityRootIdentity == "" ||
		intent.Paths[0].AuthorityRootHash != operationAuthorityRootHash(intent.Paths[0].AuthorityRoot, intent.Paths[0].AuthorityRootIdentity) ||
		intent.Paths[0].BeforeSnapshotSchemaVersion != 1 ||
		intent.Paths[0].BeforeEncoding != "utf8" || intent.Paths[0].BeforeContent != "" {
		t.Fatalf("new operation intent did not canonicalize root/raw snapshot authority: %#v", intent.Paths[0])
	}
	body, err := OperationGroupIntentV2Bytes(intent)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseOperationGroupIntentV2(body)
	if err != nil || parsed.OperationGroupID != intent.OperationGroupID {
		t.Fatalf("operation group round trip failed: parsed=%#v err=%v", parsed, err)
	}

	for name, mutate := range map[string]func(*OperationGroupIntentV2){
		"pending grant": func(candidate *OperationGroupIntentV2) {
			candidate.ExecutionGrant = resealOperationGrant(candidate.ExecutionGrant, "pending")
		},
		"wrong server": func(candidate *OperationGroupIntentV2) {
			candidate.ExecutionGrant.ServerIdentity = "host:forged"
		},
		"arguments": func(candidate *OperationGroupIntentV2) {
			candidate.ArgumentsJSON = json.RawMessage(`{"path":"other.txt","content":"after"}`)
		},
		"requested path": func(candidate *OperationGroupIntentV2) {
			candidate.Paths[0].RequestedPath = "other.txt"
		},
		"authority root": func(candidate *OperationGroupIntentV2) {
			candidate.Paths[0].AuthorityRoot = "/forged"
			candidate.Paths[0].AuthorityRootHash = operationAuthorityRootHash("/forged", candidate.Paths[0].AuthorityRootIdentity)
		},
		"legacy path authority": func(candidate *OperationGroupIntentV2) {
			candidate.Paths[0].PathAuthoritySchemaVersion = 0
			candidate.Paths[0].AuthorityKind = ""
			candidate.Paths[0].AuthorityRoot = ""
			candidate.Paths[0].AuthorityRootIdentity = ""
			candidate.Paths[0].AuthorityRootHash = ""
		},
		"snapshot bytes": func(candidate *OperationGroupIntentV2) {
			candidate.Paths[0].BeforeBytesBase64 = "Zm9yZ2Vk"
		},
		"snapshot encoding": func(candidate *OperationGroupIntentV2) {
			candidate.Paths[0].BeforeEncoding = "binary"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := parsed
			candidate.Paths = append([]OperationPathV2(nil), parsed.Paths...)
			mutate(&candidate)
			candidate.OperationGroupID = operationGroupIdentity(candidate)
			candidate.IntentDigest = operationGroupIntentDigest(candidate)
			if ValidateOperationGroupIntentV2(candidate) == nil {
				t.Fatalf("mutated operation group was accepted: %#v", candidate)
			}
		})
	}
}

func TestOperationGroupIntentAllowsOrdinaryMutationAtWitnessedCaseBoundary(t *testing.T) {
	now := time.Unix(1_700_000_100, 0).UTC()
	arguments := []byte(`{"path":"a.txt","content":"after"}`)
	quarantined, err := securitytest.BoundaryOnlyContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-operation-boundary", TurnID: "turn-operation-boundary", WorkspaceRealPath: "/workspace",
		ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := securitytest.WitnessedRiskBinding(
		quarantined.ThreadID,
		quarantined.WorkspaceRealPath,
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
		SourceManifestHash: quarantined.SourceManifestHash, ContextEpoch: quarantined.ContextEpoch, IssuedAt: now,
		PublicationPolicy: quarantined.PublicationPolicy, RiskAuthorityBinding: binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "write_file",
		ToolCallID: checkpointTestHostToolCallID(t, "boundary-write"), ArgsHash: domainsecurity.CanonicalJSONHash(arguments),
		SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ReadOnly: false, ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("workspace-checkpoint-boundary"), SourceWorkspaceCheckpointID: "workspace-checkpoint-boundary",
		OperationOrdinal: 1, ToolName: "write_file", ArgumentsJSON: arguments, CreatedAt: now.Add(time.Second),
		Paths: []OperationPathInputV2{operationPathInputWithWorkspaceAuthority(OperationPathInputV2{
			ArgumentKey: "path", RequestedPath: "a.txt", RelativePath: "a.txt", Role: "target",
			ExpectedAfterExisted: true, ExpectedAfterHash: domainsecurity.SHA256Hex([]byte("after")),
		}, securityContext.WorkspaceRealPath)},
	})
	if err != nil || intent.OperationGroupID == "" {
		t.Fatalf("ordinary mutation checkpoint was blocked by unavailable case authority: intent=%#v err=%v", intent, err)
	}
}

func TestMoveOperationGroupHasOneTwoPathTerminal(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	arguments := []byte(`{"source_path":"old/a.txt","destination_path":"new/a.txt"}`)
	securityContext, grant := operationGroupSecurity(t, now, "move_file", "call-move", arguments)
	content := "move me"
	hash := domainsecurity.SHA256Hex([]byte(content))
	intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("workspace-checkpoint-move"), SourceWorkspaceCheckpointID: "workspace-checkpoint-move",
		OperationOrdinal: 1, ToolName: "move_file", ArgumentsJSON: arguments, CreatedAt: now.Add(time.Second),
		Paths: []OperationPathInputV2{
			operationPathInputWithWorkspaceAuthority(OperationPathInputV2{ArgumentKey: "source_path", RequestedPath: "old/a.txt", RelativePath: "old/a.txt", Role: "source", BeforeExisted: true, BeforeAvailable: true, BeforeHash: hash, BeforeContent: content}, securityContext.WorkspaceRealPath),
			operationPathInputWithWorkspaceAuthority(OperationPathInputV2{ArgumentKey: "destination_path", RequestedPath: "new/a.txt", RelativePath: "new/a.txt", Role: "destination", ExpectedAfterExisted: true, ExpectedAfterHash: hash}, securityContext.WorkspaceRealPath),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := []ObservedOperationPathV2{observedForOperationPath(intent.Paths[0], "exact", false, ""), observedForOperationPath(intent.Paths[1], "exact", true, hash)}
	terminal, err := NewOperationGroupTerminalV2(OperationGroupTerminalInputV2{
		Intent: intent, Status: "completed", ReasonCode: "mutation_completed", ObservedPaths: after, SettledAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status, ok := ClassifyObservedOperationGroup(intent, terminal.ObservedPaths); !ok || status != "completed" {
		t.Fatalf("move after state classification mismatch: %q %v", status, ok)
	}
	before := []ObservedOperationPathV2{observedForOperationPath(intent.Paths[0], "exact", true, hash), observedForOperationPath(intent.Paths[1], "exact", false, "")}
	if status, ok := ClassifyObservedOperationGroup(intent, before); !ok || status != "no_effect" {
		t.Fatalf("move before state classification mismatch: %q %v", status, ok)
	}
	mixed := []ObservedOperationPathV2{observedForOperationPath(intent.Paths[0], "exact", false, ""), observedForOperationPath(intent.Paths[1], "exact", false, "")}
	if status, ok := ClassifyObservedOperationGroup(intent, mixed); !ok || status != "quarantined" {
		t.Fatalf("move mixed state classification mismatch: %q %v", status, ok)
	}
	unavailableSource := observedForOperationPath(intent.Paths[0], "unavailable", false, "")
	unavailableSource.BlockerCode = "path_unsafe"
	unavailable := []ObservedOperationPathV2{unavailableSource, observedForOperationPath(intent.Paths[1], "exact", false, "")}
	if status, ok := ClassifyObservedOperationGroup(intent, unavailable); !ok || status != "quarantined" {
		t.Fatalf("unavailable move state classification mismatch: %q %v", status, ok)
	}
	if _, err := NewOperationGroupTerminalV2(OperationGroupTerminalInputV2{
		Intent: intent, Status: "quarantined", ReasonCode: "filesystem_observation_failed", ObservedPaths: unavailable, SettledAt: now.Add(2 * time.Second),
	}); err != nil {
		t.Fatalf("unavailable state could not be closed as quarantined: %v", err)
	}
	if _, err := NewOperationGroupTerminalV2(OperationGroupTerminalInputV2{
		Intent: intent, Status: "completed", ReasonCode: "mutation_completed", ObservedPaths: mixed, SettledAt: now.Add(2 * time.Second),
	}); err == nil {
		t.Fatal("mixed move state was accepted as completed")
	}
}

func TestOperationGroupTerminalSemanticIdempotencyIgnoresRetryTimestamp(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	arguments := []byte(`{"path":"a.txt","content":"after"}`)
	securityContext, grant := operationGroupSecurity(t, now, "write_file", "call-idempotent", arguments)
	afterHash := domainsecurity.SHA256Hex([]byte("after"))
	intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("workspace-checkpoint-idempotent"), SourceWorkspaceCheckpointID: "workspace-checkpoint-idempotent",
		OperationOrdinal: 1, ToolName: "write_file", ArgumentsJSON: arguments, CreatedAt: now.Add(time.Second),
		Paths: []OperationPathInputV2{operationPathInputWithWorkspaceAuthority(OperationPathInputV2{ArgumentKey: "path", RequestedPath: "a.txt", RelativePath: "a.txt", Role: "target", ExpectedAfterExisted: true, ExpectedAfterHash: afterHash}, securityContext.WorkspaceRealPath)},
	})
	if err != nil {
		t.Fatal(err)
	}
	input := OperationGroupTerminalInputV2{Intent: intent, Status: "completed", ReasonCode: "mutation_completed", ObservedPaths: []ObservedOperationPathV2{observedForOperationPath(intent.Paths[0], "exact", true, afterHash)}}
	input.SettledAt = now.Add(2 * time.Second)
	first, err := NewOperationGroupTerminalV2(input)
	if err != nil {
		t.Fatal(err)
	}
	input.SettledAt = now.Add(3 * time.Second)
	second, err := NewOperationGroupTerminalV2(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.TerminalDigest == second.TerminalDigest || !TerminalSemanticEqual(first, second) {
		t.Fatalf("semantic retry identity mismatch: first=%#v second=%#v", first, second)
	}
}

func TestOperationGroupTerminalCanonicalizesSameRelativePathAcrossAuthorityRoots(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	arguments := []byte(`{"source_path":"/workspace/a.txt","destination_path":"/allowed/a.txt"}`)
	securityContext, grant := operationGroupSecurity(t, now, "move_file", "call-cross-root", arguments)
	contentHash := domainsecurity.SHA256Hex([]byte("move across roots"))
	source := operationPathInputWithWorkspaceAuthority(OperationPathInputV2{
		ArgumentKey: "source_path", RequestedPath: "/workspace/a.txt", RelativePath: "a.txt", Role: "source",
		BeforeExisted: true, BeforeAvailable: true, BeforeHash: contentHash, BeforeContent: "move across roots",
	}, securityContext.WorkspaceRealPath)
	destination := OperationPathInputV2{
		ArgumentKey: "destination_path", RequestedPath: "/allowed/a.txt", RelativePath: "a.txt", Role: "destination",
		ExpectedAfterExisted: true, ExpectedAfterHash: contentHash,
		PathAuthoritySchemaVersion: 1, AuthorityKind: "allow_write", AuthorityRoot: "/allowed",
		AuthorityRootIdentity: "test:allow-write:1",
	}
	destination.AuthorityRootHash = operationAuthorityRootHash(destination.AuthorityRoot, destination.AuthorityRootIdentity)
	intent, err := NewOperationGroupIntentV2(OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("workspace-checkpoint-cross-root"), SourceWorkspaceCheckpointID: "workspace-checkpoint-cross-root",
		OperationOrdinal: 1, ToolName: "move_file", ArgumentsJSON: arguments, CreatedAt: now.Add(time.Second),
		Paths: []OperationPathInputV2{source, destination},
	})
	if err != nil {
		t.Fatal(err)
	}
	observed := []ObservedOperationPathV2{
		observedForOperationPath(intent.Paths[0], "exact", false, ""),
		observedForOperationPath(intent.Paths[1], "exact", true, contentHash),
	}
	first, err := NewOperationGroupTerminalV2(OperationGroupTerminalInputV2{
		Intent: intent, Status: "completed", ReasonCode: "mutation_completed", ObservedPaths: observed, SettledAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewOperationGroupTerminalV2(OperationGroupTerminalInputV2{
		Intent: intent, Status: "completed", ReasonCode: "mutation_completed",
		ObservedPaths: []ObservedOperationPathV2{observed[1], observed[0]}, SettledAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.TerminalDigest != second.TerminalDigest || !TerminalSemanticEqual(first, second) {
		t.Fatalf("same cross-root observations were not canonical: first=%#v second=%#v", first.ObservedPaths, second.ObservedPaths)
	}
}

func observedForOperationPath(path OperationPathV2, status string, existed bool, hash string) ObservedOperationPathV2 {
	return ObservedOperationPathV2{
		PathAuthoritySchemaVersion: path.PathAuthoritySchemaVersion,
		AuthorityKind:              path.AuthorityKind, AuthorityRootHash: path.AuthorityRootHash,
		RelativePath: path.RelativePath, ObservationStatus: status, Existed: existed, Hash: hash,
	}
}

func operationPathInputWithWorkspaceAuthority(input OperationPathInputV2, workspace string) OperationPathInputV2 {
	input.PathAuthoritySchemaVersion = 1
	input.AuthorityKind = "workspace"
	input.AuthorityRoot = workspace
	input.AuthorityRootIdentity = "test:workspace:1"
	input.AuthorityRootHash = operationAuthorityRootHash(workspace, input.AuthorityRootIdentity)
	return input
}

func operationGroupSecurity(t *testing.T, now time.Time, toolName, callID string, arguments []byte) (domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) {
	t.Helper()
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-operation", TurnID: "turn-operation", WorkspaceRealPath: "/workspace",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal([]string{toolName})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: toolName, ToolCallID: checkpointTestHostToolCallID(t, callID),
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scope), ReadOnly: false, ApprovalState: "approved",
		IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	return securityContext, grant
}

func resealOperationGrant(grant domainsecurity.ExecutionGrant, approvalState string) domainsecurity.ExecutionGrant {
	issuedAt, _ := time.Parse(time.RFC3339Nano, grant.IssuedAt)
	expiresAt, _ := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	context := domainsecurity.TurnSecurityContext{TurnID: grant.TurnID, ContextDigest: grant.ContextDigest}
	return domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: context, Provider: grant.Provider, ServerIdentity: grant.ServerIdentity, ToolName: grant.ToolName,
		ToolCallID: grant.ToolCallID, ConnectionEpoch: grant.ConnectionEpoch, ArgsHash: grant.ArgsHash,
		SchemaHash: grant.SchemaHash, ScopeHash: grant.ScopeHash, ReadOnly: grant.ReadOnly, ApprovalState: approvalState,
		IssuedAt: issuedAt, ExpiresAt: expiresAt,
	})
}
