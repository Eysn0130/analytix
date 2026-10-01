package checkpoint

import (
	"crypto/sha256"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func generatedRewindGroup(t *testing.T, toolName string) domaincheckpoint.MaterializedOperationGroupV2 {
	t.Helper()
	now := time.Unix(1_700_000_000, 0).UTC()
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-rewind", TurnID: "turn-rewind", WorkspaceRealPath: "/synthetic/workspace",
		TenantID: "local", UserID: "local", ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The ordinary write uses the same suffix, so classification cannot depend on the extension.
	arguments := []byte(`{"path":"report.docx","content":"ordinary text"}`)
	principalDigest := ""
	if toolName == "generate_office_document" {
		arguments = []byte(`{"path":"report.docx","kind":"docx","markdown":"Synthetic report"}`)
		principalDigest = domainsecurity.SHA256Hex([]byte("synthetic principal"))
	}
	entropy := sha256.Sum256([]byte("synthetic rewind " + toolName))
	callID, err := domainsecurity.NewHostToolCallIDV1(entropy[:])
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "synthetic-provider", ServerIdentity: "host:builtin", ToolName: toolName, ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("scope")), ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
	})
	rootIdentity := "synthetic:workspace:1"
	path := domaincheckpoint.OperationPathInputV2{
		ArgumentKey: "path", RequestedPath: "report.docx", RelativePath: "report.docx", Role: "target",
		PathAuthoritySchemaVersion: 1, AuthorityKind: "workspace", AuthorityRoot: securityContext.WorkspaceRealPath,
		AuthorityRootIdentity: rootIdentity, AuthorityRootHash: domainsecurity.SHA256Hex([]byte(securityContext.WorkspaceRealPath + "\x00" + rootIdentity)),
		ExpectedAfterExisted: true, ExpectedAfterHash: domainsecurity.SHA256Hex([]byte("synthetic after bytes")),
	}
	intent, err := domaincheckpoint.NewOperationGroupIntentV2(domaincheckpoint.OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant, CheckpointID: domaincheckpointref.RuntimeID("workspace-rewind"),
		SourceWorkspaceCheckpointID: "workspace-rewind", GenerationPrincipalDigest: principalDigest,
		OperationOrdinal: 1, ToolName: toolName, ArgumentsJSON: arguments,
		Paths: []domaincheckpoint.OperationPathInputV2{path}, CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := domaincheckpoint.NewOperationGroupTerminalV2(domaincheckpoint.OperationGroupTerminalInputV2{
		Intent: intent, Status: "completed", ReasonCode: "mutation_completed", SettledAt: now.Add(2 * time.Second),
		ObservedPaths: []domaincheckpoint.ObservedOperationPathV2{{PathAuthoritySchemaVersion: path.PathAuthoritySchemaVersion,
			AuthorityKind: path.AuthorityKind, AuthorityRootHash: path.AuthorityRootHash, RelativePath: path.RelativePath,
			ObservationStatus: "exact", Existed: true, Hash: path.ExpectedAfterHash}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return domaincheckpoint.MaterializedOperationGroupV2{Intent: intent, Terminal: terminal}
}

func TestGeneratedRewindPlanRetainsCreationCapabilityBoundary(t *testing.T) {
	for _, toolName := range []string{"generate_office_document", "write_file"} {
		t.Run(toolName, func(t *testing.T) {
			group := generatedRewindGroup(t, toolName)
			records, err := operationGroupsToRecords([]domaincheckpoint.MaterializedOperationGroupV2{group})
			if err != nil || len(records) != 1 {
				t.Fatalf("materialization failed: %v", err)
			}
			metadata, ok := CapturedMetadataFromAuthorityRecords(AuthorityMetadataInput{
				ThreadID: group.Intent.SecurityContext.ThreadID, CheckpointID: group.Intent.CheckpointID,
				AuthorityWorkspace: group.Intent.SecurityContext.WorkspaceRealPath, Records: records,
			})
			if !ok {
				t.Fatal("private captured metadata was rejected")
			}
			changed := metadata["changedFiles"].([]any)[0].(map[string]any)
			plan := BuildRestoreFilePlan(RestoreFilePlanInput{Changed: changed})
			if toolName == "generate_office_document" {
				if changed["generatedOfficeCreation"] != true || plan["status"] != "manual_review" || plan["action"] != "manual_review" ||
					plan["reason"] != "generated Office file deletion requires manual review because binary checkpoint rescue is not available" {
					t.Fatalf("generated file advertised automatic rewind: %+v", plan)
				}
				preflight := BuildApplyFilePreflight(plan, records[0], ApplyFileState{})
				if preflight.Status != "manual_review" {
					t.Fatalf("apply lost manual-review status: %+v", preflight)
				}
			} else {
				if _, marked := changed["generatedOfficeCreation"]; marked || plan["status"] != "ready" || plan["action"] != "delete_created_file" {
					t.Fatalf("ordinary new-file rewind changed: %+v", plan)
				}
			}
			event, ok := BuildCapturedCheckpointEvent(CapturedCheckpointInput{
				ThreadID: group.Intent.SecurityContext.ThreadID, TurnID: group.Intent.SecurityContext.TurnID,
				CheckpointID: group.Intent.CheckpointID, Records: records,
			})
			body, err := json.Marshal(event)
			if !ok || err != nil || strings.Contains(string(body), "generatedOfficeCreation") || strings.Contains(string(body), "report.docx") {
				t.Fatal("public capture event disclosed private rewind metadata")
			}
		})
	}
}

func TestGeneratedRewindMaterializationRejectsUnverifiedCreation(t *testing.T) {
	group := generatedRewindGroup(t, "generate_office_document")
	group.Intent.GenerationPrincipalDigest = domainsecurity.SHA256Hex([]byte("tampered principal"))
	if _, err := operationGroupsToRecords([]domaincheckpoint.MaterializedOperationGroupV2{group}); err == nil {
		t.Fatal("unverified generated creation entered rewind metadata")
	}
}
