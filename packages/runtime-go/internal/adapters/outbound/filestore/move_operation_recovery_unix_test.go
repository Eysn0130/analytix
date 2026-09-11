//go:build darwin || linux

package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	checkpointauthority "analytix.local/runtime-go/internal/adapters/outbound/checkpointauthority"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
	"golang.org/x/sys/unix"
)

func TestMovePrivatePendingRestartRecoveryUsesDurableOpenIntent(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	fixture.stagePrivatePending(t)

	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatalf("prepare read-only recovery: %v", err)
	}
	if _, err := os.Stat(fixture.source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("semantic planning mutated private pending: %v", err)
	}
	results, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second))
	if err != nil {
		t.Fatalf("apply recovery: %v", err)
	}
	if len(results) != 1 || results[0].Terminal.Status != "no_effect" {
		t.Fatalf("unexpected recovery result: %#v", results)
	}
	body, err := os.ReadFile(fixture.source)
	if err != nil || string(body) != fixture.content {
		t.Fatalf("authorized source was not restored: body=%q err=%v", body, err)
	}
	if _, err := os.Stat(fixture.destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery created a destination: %v", err)
	}
}

func TestMovePrivatePendingRestartRecoveryRejectsPostPlanSourceRace(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	fixture.stagePrivatePending(t)
	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.source, []byte("attacker source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second)); err == nil {
		t.Fatal("post-plan source race was accepted")
	}
	assertAtomicTextContent(t, fixture.source, "attacker source")
	if _, err := os.Stat(fixture.destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("race recovery published a destination: %v", err)
	}
	open, err := service.Authority.OpenOperationGroups(fixture.ctx)
	if err != nil || len(open) != 1 {
		t.Fatalf("race wrote a terminal before repair: open=%#v err=%v", open, err)
	}
}

func TestMovePrivateStageRestartRecoveryRestoresSource(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	fixture.stagePrivateLeaf(t)
	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture.source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("semantic planning changed staged source: %v", err)
	}
	results, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second))
	if err != nil || len(results) != 1 || results[0].Terminal.Status != "no_effect" {
		t.Fatalf("private stage recovery mismatch: results=%#v err=%v", results, err)
	}
	assertAtomicTextContent(t, fixture.source, fixture.content)
	if _, err := os.Stat(filepath.Join(fixture.workspace, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private stage recovery leaked a public tree: %v", err)
	}
}

func TestMoveEmptyPrivateStageRestartRecoveryHasNoPublicEffect(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	fixture.stageEmptyPrivateTree(t)
	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertAtomicTextContent(t, fixture.source, fixture.content)
	results, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second))
	if err != nil || len(results) != 1 || results[0].Terminal.Status != "no_effect" {
		t.Fatalf("empty private stage recovery mismatch: results=%#v err=%v", results, err)
	}
	assertAtomicTextContent(t, fixture.source, fixture.content)
	if _, err := os.Stat(filepath.Join(fixture.workspace, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty private stage recovery created public state: %v", err)
	}
}

func TestMoveQuarantinedPrivateStageRestartRecoverySettlesIdempotently(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	fixture.stageEmptyPrivateTree(t)
	plan := fixture.boundPlan(t)

	func() {
		authorityRoot, err := openConditionalUnixBoundAuthorityRoot(fixture.authority)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(authorityRoot)
		journal, err := openConditionalUnixMoveJournal(authorityRoot, plan, false)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(journal)
		binding, found, err := readConditionalUnixMoveIntent(journal)
		if err != nil || !found {
			t.Fatalf("read move intent: found=%v err=%v", found, err)
		}
		plan = hydrateConditionalUnixMovePlanTopology(plan, binding)
		receipt, found, err := readConditionalUnixMoveStageAuthority(journal)
		if err != nil || !found {
			t.Fatalf("read move stage authority: found=%v err=%v", found, err)
		}
		if err := quarantineConditionalUnixMoveStage(fixture.authority, plan, receipt.ReceiptDigest); err != nil {
			t.Fatalf("simulate crash after private stage quarantine: %v", err)
		}
	}()

	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertAtomicTextContent(t, fixture.source, fixture.content)
	results, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second))
	if err != nil || len(results) != 1 || results[0].Terminal.Status != "no_effect" {
		t.Fatalf("quarantined private stage recovery mismatch: results=%#v err=%v", results, err)
	}
	assertAtomicTextContent(t, fixture.source, fixture.content)
	if _, err := os.Stat(filepath.Join(fixture.workspace, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantined stage recovery created public state: %v", err)
	}
	open, err := service.Authority.OpenOperationGroups(fixture.ctx)
	if err != nil || len(open) != 0 {
		t.Fatalf("quarantined stage recovery left an open intent: open=%#v err=%v", open, err)
	}
}

func TestMovePublishedStageRestartRecoveryCompletes(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	plan := fixture.boundPlan(t)
	if err := ApplyMoveRegularFile(plan); err != nil {
		t.Fatalf("publish move before simulated restart: %v", err)
	}
	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	results, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second))
	if err != nil || len(results) != 1 || results[0].Terminal.Status != "completed" {
		t.Fatalf("published stage recovery mismatch: results=%#v err=%v", results, err)
	}
	assertAtomicTextContent(t, fixture.destination, fixture.content)
}

func TestMovePrivateStageRestartRecoveryRejectsPostPlanDestinationRace(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	fixture.stagePrivateLeaf(t)
	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(fixture.destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.destination, []byte("attacker destination"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second)); err == nil {
		t.Fatal("post-plan destination race was accepted")
	}
	assertAtomicTextContent(t, fixture.destination, "attacker destination")
	if _, err := os.Stat(fixture.source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed recovery moved the private authorized inode: %v", err)
	}
	open, err := service.Authority.OpenOperationGroups(fixture.ctx)
	if err != nil || len(open) != 1 {
		t.Fatalf("destination race wrote a terminal: open=%#v err=%v", open, err)
	}
}

func TestMovePrivateStageRestartRecoveryRejectsStageIdentityReplacement(t *testing.T) {
	fixture := newMoveRestartRecoveryFixture(t)
	fixture.stagePrivateLeaf(t)
	service := fixture.reopenedService(t)
	semanticPlan, err := service.PrepareRecoverOpen(fixture.ctx)
	if err != nil {
		t.Fatal(err)
	}
	operationID := fixture.draft.AuthorityIntent.OperationGroupID
	journalPath := filepath.Join(fixture.mutationRoot, "moves-v1", operationID[:2], operationID)
	bindingBody, err := os.ReadFile(filepath.Join(journalPath, conditionalUnixMoveIntentFile))
	if err != nil {
		t.Fatal(err)
	}
	var binding conditionalUnixMoveIntentBinding
	if err := json.Unmarshal(bindingBody, &binding); err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(journalPath, binding.DestinationStageName)
	if err := os.Rename(stagePath, stagePath+".displaced"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stagePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := semanticPlan.Apply(fixture.ctx, fixture.now.Add(3*time.Second)); err == nil {
		t.Fatal("stage inode replacement was accepted")
	}
	if _, err := os.Stat(fixture.source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed stage identity recovery moved source: %v", err)
	}
	open, err := service.Authority.OpenOperationGroups(fixture.ctx)
	if err != nil || len(open) != 1 {
		t.Fatalf("stage identity replacement wrote a terminal: open=%#v err=%v", open, err)
	}
}

type moveRestartRecoveryFixture struct {
	ctx          context.Context
	now          time.Time
	dataDir      string
	workspace    string
	source       string
	destination  string
	content      string
	arguments    []byte
	access       *privatecastest.AccessAuthority
	storeRoot    string
	mutationRoot string
	authority    ConditionalMutationAuthority
	draft        checkpointapp.OperationDraft
}

func newMoveRestartRecoveryFixture(t *testing.T) moveRestartRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "old", "source.txt")
	destination := filepath.Join(workspace, "new", "destination.txt")
	content := "durable move source\n"
	if err := os.WriteFile(source, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	storeRoot := filepath.Join(dataDir, "private", "checkpoint-authority")
	store, err := checkpointauthority.NewStoreContext(ctx, storeRoot, access)
	if err != nil {
		t.Fatal(err)
	}
	mutationRoot := filepath.Join(dataDir, "file-mutation-quarantine-v1")
	mutationAuthority, err := OpenConditionalMutationAuthority(mutationRoot)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_300_000, 0).UTC()
	arguments, _ := json.Marshal(map[string]string{"source_path": "old/source.txt", "destination_path": "new/destination.txt"})
	securityContext, grant := moveRecoverySecurity(t, now, workspace, arguments)
	checkpointAuthority := checkpointapp.SnapshotAuthority{Store: store}
	observer := CheckpointOperationObserver{MutationAuthority: mutationAuthority}
	service := checkpointapp.OperationService{Authority: checkpointAuthority, Observer: observer, Recovery: observer}
	draft, err := service.Begin(ctx, checkpointapp.BeginOperationInput{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("move-restart"), SourceWorkspaceCheckpointID: "move-restart",
		Workspace: workspace, ToolName: "move_file", ArgumentsJSON: arguments,
		Paths: []checkpointapp.OperationPathRequest{
			{ResolvedPath: source, ArgumentKey: "source_path", RequestedPath: "old/source.txt", Role: "source"},
			{ResolvedPath: destination, ArgumentKey: "destination_path", RequestedPath: "new/destination.txt", Role: "destination", ExpectedAfterExisted: true},
		},
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return moveRestartRecoveryFixture{
		ctx: ctx, now: now, dataDir: dataDir, workspace: workspace,
		source: source, destination: destination, content: content, arguments: arguments,
		access: access, storeRoot: storeRoot, mutationRoot: mutationRoot,
		authority: mutationAuthority, draft: draft,
	}
}

func (fixture moveRestartRecoveryFixture) stagePrivatePending(t *testing.T) {
	t.Helper()
	plan := fixture.boundPlan(t)
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(fixture.authority)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(authorityRoot)
	journal, err := openConditionalUnixMoveJournal(authorityRoot, plan, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(journal)
	plan, err = ensureConditionalUnixMoveIntent(journal, plan)
	if err != nil {
		t.Fatal(err)
	}
	if plan.destinationParentMissing {
		stage, _, err := ensureConditionalUnixMoveStage(journal, plan)
		if err != nil {
			t.Fatal(err)
		}
		if stage >= 0 {
			_ = unix.Close(stage)
		}
	}
	sourceParent, sourceBase, missing, err := openAtomicUnixParent(fixture.source, false)
	if err != nil || missing {
		t.Fatalf("open source: missing=%v err=%v", missing, err)
	}
	defer unix.Close(sourceParent)
	pending, err := conditionalUnixMovePendingName(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameConditionalUnixNoReplace(sourceParent, sourceBase, journal, pending); err != nil {
		t.Fatal(err)
	}
	if err := syncConditionalUnixParents(sourceParent, journal); err != nil {
		t.Fatal(err)
	}
}

func (fixture moveRestartRecoveryFixture) stagePrivateLeaf(t *testing.T) {
	t.Helper()
	plan := fixture.boundPlan(t)
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(fixture.authority)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(authorityRoot)
	journal, err := openConditionalUnixMoveJournal(authorityRoot, plan, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(journal)
	plan, err = ensureConditionalUnixMoveIntent(journal, plan)
	if err != nil {
		t.Fatal(err)
	}
	stage, _, err := ensureConditionalUnixMoveStage(journal, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(stage)
	sourceParent, sourceBase, missing, err := openAtomicUnixParent(fixture.source, false)
	if err != nil || missing {
		t.Fatal(err)
	}
	defer unix.Close(sourceParent)
	pendingName, err := conditionalUnixMovePendingName(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := renameConditionalUnixNoReplace(sourceParent, sourceBase, journal, pendingName); err != nil {
		t.Fatal(err)
	}
	if err := syncConditionalUnixParents(sourceParent, journal); err != nil {
		t.Fatal(err)
	}
	leafParent, leafBase, err := openConditionalUnixMoveStageLeafParent(stage, plan.destinationRelativeTail, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(leafParent)
	if err := renameConditionalUnixNoReplace(journal, pendingName, leafParent, leafBase); err != nil {
		t.Fatal(err)
	}
	if err := syncConditionalUnixParents(journal, leafParent); err != nil {
		t.Fatal(err)
	}
	if err := fsyncConditionalUnixRegularAt(leafParent, leafBase); err != nil {
		t.Fatal(err)
	}
}

func (fixture moveRestartRecoveryFixture) stageEmptyPrivateTree(t *testing.T) {
	t.Helper()
	plan := fixture.boundPlan(t)
	authorityRoot, err := openConditionalUnixBoundAuthorityRoot(fixture.authority)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(authorityRoot)
	journal, err := openConditionalUnixMoveJournal(authorityRoot, plan, true)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(journal)
	plan, err = ensureConditionalUnixMoveIntent(journal, plan)
	if err != nil {
		t.Fatal(err)
	}
	stage, _, err := ensureConditionalUnixMoveStage(journal, plan)
	if err != nil {
		t.Fatal(err)
	}
	_ = unix.Close(stage)
}

func (fixture moveRestartRecoveryFixture) boundPlan(t *testing.T) MoveRegularFilePlan {
	t.Helper()
	plan, err := PrepareMoveRegularFile(MoveRegularFileOptions{
		Workspace: fixture.workspace, SourcePath: fixture.source, DestinationPath: fixture.destination,
		MutationAuthority: fixture.authority,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = BindMoveRegularFilePlanToIntent(plan, fixture.draft.AuthorityIntent)
	if err != nil {
		t.Fatalf("bind durable intent: %v", err)
	}
	return plan
}

func (fixture moveRestartRecoveryFixture) reopenedService(t *testing.T) checkpointapp.OperationService {
	t.Helper()
	store, present, err := checkpointauthority.OpenExistingStoreContext(fixture.ctx, fixture.storeRoot, fixture.access)
	if err != nil || !present {
		t.Fatalf("reopen checkpoint authority: present=%v err=%v", present, err)
	}
	mutationAuthority, present, err := OpenExistingConditionalMutationAuthority(fixture.mutationRoot)
	if err != nil || !present {
		t.Fatalf("reopen mutation authority: present=%v err=%v", present, err)
	}
	observer := CheckpointOperationObserver{MutationAuthority: mutationAuthority}
	return checkpointapp.OperationService{
		Authority: checkpointapp.SnapshotAuthority{Store: store}, Observer: observer, Recovery: observer,
	}
}

func moveRecoverySecurity(
	t *testing.T,
	now time.Time,
	workspace string,
	arguments []byte,
) (domainsecurity.TurnSecurityContext, domainsecurity.ExecutionGrant) {
	t.Helper()
	workspaceRealPath, err := WorkspaceRealPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-move-restart", TurnID: "turn-move-restart", WorkspaceRealPath: workspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal([]string{"move_file"})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "move_file", ToolCallID: filestoreTestHostToolCallID(t, "call-move-restart"),
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scope), ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	return securityContext, grant
}
