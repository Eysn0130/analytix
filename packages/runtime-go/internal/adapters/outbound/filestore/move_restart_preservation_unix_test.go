//go:build darwin || linux

package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestMoveRestartPreservationRejectsIndependentRollbackOntoHeldResource(t *testing.T) {
	testMoveRestartPreservationIndependentResourceV1(t, true)
}

func TestMoveRestartPreservationAllowsIndependentRollbackOnDifferentResource(t *testing.T) {
	testMoveRestartPreservationIndependentResourceV1(t, false)
}

func testMoveRestartPreservationIndependentResourceV1(t *testing.T, conflict bool) {
	t.Helper()
	fixture := newMoveRestartRecoveryFixture(t)
	// Both journals start from existing public parents. Their source and
	// destination file identities are separate, valid durable operations.
	if err := os.MkdirAll(filepath.Dir(fixture.destination), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.stagePrivatePending(t)
	independent := fixture
	independent.source = fixture.destination
	sourceRelative := "new/destination.txt"
	if !conflict {
		sourceRelative = "independent/source.txt"
		independent.source = filepath.Join(fixture.workspace, filepath.FromSlash(sourceRelative))
	}
	independent.destination = filepath.Join(fixture.workspace, "independent", "target.txt")
	independent.content = "independent original source"
	if err := os.MkdirAll(filepath.Dir(independent.destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(independent.source, []byte(independent.content), 0o600); err != nil {
		t.Fatal(err)
	}
	independent.arguments, _ = json.Marshal(map[string]string{"source_path": sourceRelative, "destination_path": "independent/target.txt"})
	frozen, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-independent-recovery", TurnID: "turn-independent-recovery", WorkspaceRealPath: fixture.draft.AuthorityIntent.SecurityContext.WorkspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: frozen, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "move_file", ToolCallID: filestoreTestHostToolCallID(t, "independent-recovery"),
		ArgsHash: domainsecurity.CanonicalJSONHash(independent.arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("scope")),
		ApprovalState: "approved", IssuedAt: fixture.now, ExpiresAt: fixture.now.Add(time.Hour),
	})
	service := fixture.reopenedService(t)
	independent.draft, err = service.Begin(fixture.ctx, checkpointapp.BeginOperationInput{
		SecurityContext: frozen, ExecutionGrant: grant, CheckpointID: domaincheckpointref.RuntimeID("independent-recovery"), SourceWorkspaceCheckpointID: "independent-recovery",
		Workspace: fixture.workspace, ToolName: "move_file", ArgumentsJSON: independent.arguments,
		Paths: []checkpointapp.OperationPathRequest{
			{ResolvedPath: independent.source, ArgumentKey: "source_path", RequestedPath: sourceRelative, Role: "source"},
			{ResolvedPath: independent.destination, ArgumentKey: "destination_path", RequestedPath: "independent/target.txt", Role: "destination", ExpectedAfterExisted: true},
		}, CreatedAt: fixture.now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	independent.stagePrivatePending(t)
	// Prove both exact private journals are independently readable without
	// applying either action, before testing the combined held resource gate.
	if _, err := service.PrepareRecoverOpen(fixture.ctx); err != nil {
		t.Fatalf("two-journal fixture is not a valid crash cut: %v", err)
	}
	before := moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)
	service, err = checkpointapp.NewOperationServiceWithRestartPreservationV1(fixture.ctx, service, []domainsecurity.TurnSecurityContext{fixture.draft.AuthorityIntent.SecurityContext})
	if err == nil {
		plan, prepareErr := service.PrepareRecoverOpen(fixture.ctx)
		err = prepareErr
		if err == nil {
			_, err = plan.Apply(fixture.ctx, fixture.now.Add(3*time.Second))
		}
	}
	if conflict {
		if !errors.Is(err, checkpointapp.ErrRestartPreserved) {
			t.Errorf("independent rollback was not refused at held resource admission: %v", err)
		}
		if !reflect.DeepEqual(before, moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)) {
			t.Error("independent recovery changed held original path absence or either journal")
		}
		return
	}
	if err != nil {
		t.Fatalf("same-workspace independent recovery was blocked: %v", err)
	}
	assertAtomicTextContent(t, independent.source, independent.content)
	for _, path := range []string{fixture.source, fixture.destination, independent.destination} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("independent rollback changed held source/destination or installed its own destination")
		}
	}
	open, err := service.Authority.OpenOperationGroups(fixture.ctx)
	if err != nil || len(open) != 1 || open[0].IntentDigest != fixture.draft.AuthorityIntent.IntentDigest {
		t.Fatalf("same-workspace independent recovery changed the held open group: %v", err)
	}
	before = moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)
	results, err := service.RecoverOpen(fixture.ctx, fixture.now.Add(4*time.Second))
	if err != nil || len(results) != 0 || !reflect.DeepEqual(before, moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)) {
		t.Fatalf("second mixed restart changed retained held state: count=%d err=%v", len(results), err)
	}
}

func TestMoveRestartPreservationKeepsPrivateRecoveryAndTerminalOpen(t *testing.T) {
	for _, scenario := range []string{"private-pending", "private-stage", "empty-stage"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newMoveRestartRecoveryFixture(t)
			switch scenario {
			case "private-pending":
				fixture.stagePrivatePending(t)
			case "private-stage":
				fixture.stagePrivateLeaf(t)
			case "empty-stage":
				fixture.stageEmptyPrivateTree(t)
			}
			before := moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)
			for range 2 {
				service, err := checkpointapp.NewOperationServiceWithRestartPreservationV1(fixture.ctx, fixture.reopenedService(t), []domainsecurity.TurnSecurityContext{fixture.draft.AuthorityIntent.SecurityContext})
				if err != nil {
					t.Fatal(err)
				}
				plan, err := service.PrepareRecoverOpen(fixture.ctx)
				if err != nil {
					t.Fatal(err)
				}
				results, err := plan.Apply(fixture.ctx, fixture.now.Add(3*time.Second))
				if err != nil || len(results) != 0 {
					t.Errorf("held recovery emitted a result: count=%d err=%v", len(results), err)
				}
				open, err := service.Authority.OpenOperationGroups(fixture.ctx)
				if err != nil || len(open) != 1 || open[0].IntentDigest != fixture.draft.AuthorityIntent.IntentDigest {
					t.Errorf("held operation was settled: count=%d err=%v", len(open), err)
				}
				if !reflect.DeepEqual(before, moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)) {
					t.Error("held original pending/stage/workspace/authority inventory changed")
				}
			}
		})
	}
}

func TestMoveRestartPreservationDeniesLiveSettlementBeforeObservation(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	service, err := checkpointapp.NewOperationServiceWithRestartPreservationV1(fixture.ctx, fixture.reopenedService(t), []domainsecurity.TurnSecurityContext{fixture.draft.AuthorityIntent.SecurityContext})
	if err != nil {
		t.Fatal(err)
	}
	before := moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)
	if _, err := service.Settle(fixture.ctx, fixture.draft, false, fixture.now.Add(3*time.Second)); !errors.Is(err, checkpointapp.ErrRestartPreserved) {
		t.Errorf("held settlement did not fail at admission: %v", err)
	}
	if _, err := service.Begin(fixture.ctx, checkpointapp.BeginOperationInput{SecurityContext: fixture.draft.AuthorityIntent.SecurityContext}); !errors.Is(err, checkpointapp.ErrRestartPreserved) {
		t.Errorf("held begin did not fail before request materialization: %v", err)
	}
	if !reflect.DeepEqual(before, moveRestartPreservedTreeV1(t, fixture.dataDir, fixture.workspace)) {
		t.Error("held live operation changed durable state")
	}
}

func moveRestartPreservedTreeV1(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			value := info.Mode().String()
			if info.Mode().IsRegular() {
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				digest := sha256.Sum256(body)
				value += ":" + hex.EncodeToString(digest[:])
			}
			result[path] = value
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	return result
}
