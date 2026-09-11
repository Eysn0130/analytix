package checkpointauthority

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpoint "analytix.local/runtime-go/internal/domain/checkpointauthority"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	checkpointport "analytix.local/runtime-go/internal/ports/checkpointauthority"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func TestPreservedCheckpointCaptureValidatesOpenSuffixWithoutInventingFrontier(t *testing.T) {
	ctx := context.Background()
	store := newOperationStore(t, ctx)
	now := time.Unix(1_700_000_000, 0).UTC()
	input := operationStoreWriteInput(t, now, "capture-prefix", "before", "after")
	first, _, _, err := store.BeginOperationGroup(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SettleOperationGroup(ctx, first.OperationGroupID, "completed", "mutation_completed", []domaincheckpoint.ObservedOperationPathV2{operationStoreObserved(first.Paths[0], "after")}, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	authority := checkpointapp.SnapshotAuthority{Store: store}
	original, err := authority.CapturedOperationInventory(ctx, input.SecurityContext.ThreadID, input.CheckpointID)
	if err != nil || len(original.Audits) != 1 {
		t.Fatalf("original completed prefix is invalid: %v", err)
	}
	suffixInput := operationStoreWriteInput(t, now, "capture-open-suffix", "after", "later")
	suffix, _, _, err := store.BeginOperationGroup(ctx, suffixInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authority.CapturedOperationInventory(ctx, input.SecurityContext.ThreadID, input.CheckpointID); err == nil {
		t.Fatal("ordinary capture authority accepted an open suffix")
	}
	for range 2 {
		preserved, err := authority.CapturedOperationInventoryForPreservedContextV1(ctx, input.SecurityContext, input.CheckpointID)
		if err != nil || len(preserved.Audits) != 1 || preserved.Audits[0].CaptureEventID != original.Audits[0].CaptureEventID {
			t.Fatalf("preserved observation lost the original exact settled prefix: %v", err)
		}
		open, err := store.OpenOperationGroups(ctx)
		if err != nil || len(open) != 1 || open[0].IntentDigest != suffix.IntentDigest {
			t.Fatal("preserved observation closed or changed the original suffix")
		}
	}
	// The suffix remains inside the denominator even though it cannot generate
	// capture authority. Corrupting it must invalidate the whole observation.
	path := filepath.Join(store.root, "operation-group-intents-v2", suffix.OperationGroupID[:2], suffix.OperationGroupID+".json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.CapturedOperationInventoryForPreservedContextV1(ctx, input.SecurityContext, input.CheckpointID); err == nil {
		t.Fatal("preserved capture ignored an invalid later record")
	}
}

func TestOperationGroupInventoryRejectsOrphanTerminalAfterStoreConstruction(t *testing.T) {
	ctx := context.Background()
	store := newOperationStore(t, ctx)
	now := time.Unix(1_700_000_000, 0).UTC()
	input := operationStoreWriteInput(t, now, "orphan-terminal", "before", "after")
	intent, _, _, err := store.BeginOperationGroup(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := store.SettleOperationGroup(ctx, intent.OperationGroupID, "completed", "mutation_completed", []domaincheckpoint.ObservedOperationPathV2{operationStoreObserved(intent.Paths[0], "after")}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	body, err := domaincheckpoint.OperationGroupTerminalV2Bytes(terminal, intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.operationTerminalCAS.PutIfAbsent(ctx, domainsecurity.SHA256Hex([]byte("orphan terminal")), body); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListOperationGroups(ctx); !errors.Is(err, checkpointport.ErrCorrupt) {
		t.Fatalf("complete current inventory ignored an orphan terminal: %v", err)
	}
}

func TestOperationStoreUsesOneGroupIntentAndOneSemanticTerminal(t *testing.T) {
	ctx := context.Background()
	store := newOperationStore(t, ctx)
	now := time.Unix(1_700_000_000, 0).UTC()
	input := operationStoreWriteInput(t, now, "call-1", "before", "after")
	intent, terminal, existing, err := store.BeginOperationGroup(ctx, input)
	if err != nil || existing || terminal != nil || intent.OperationOrdinal != 1 {
		t.Fatalf("begin operation group mismatch: intent=%#v terminal=%#v existing=%t err=%v", intent, terminal, existing, err)
	}

	retry := input
	retry.CreatedAt = now.Add(2 * time.Second)
	retryIntent, retryTerminal, retryExisting, err := store.BeginOperationGroup(ctx, retry)
	if err != nil || !retryExisting || retryTerminal != nil || retryIntent.IntentDigest != intent.IntentDigest {
		t.Fatalf("open intent retry was not idempotent: intent=%#v terminal=%#v existing=%t err=%v", retryIntent, retryTerminal, retryExisting, err)
	}
	after := []domaincheckpoint.ObservedOperationPathV2{operationStoreObserved(intent.Paths[0], "after")}
	firstTerminal, err := store.SettleOperationGroup(ctx, intent.OperationGroupID, "completed", "mutation_completed", after, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	secondTerminal, err := store.SettleOperationGroup(ctx, intent.OperationGroupID, "completed", "mutation_completed", after, now.Add(4*time.Second))
	if err != nil || secondTerminal.TerminalDigest != firstTerminal.TerminalDigest || secondTerminal.SettledAt != firstTerminal.SettledAt {
		t.Fatalf("terminal retry did not return first authority: first=%#v second=%#v err=%v", firstTerminal, secondTerminal, err)
	}
	_, settled, settledExisting, err := store.BeginOperationGroup(ctx, retry)
	if err != nil || !settledExisting || settled == nil || settled.TerminalDigest != firstTerminal.TerminalDigest {
		t.Fatalf("settled operation retry did not return closed authority: terminal=%#v existing=%t err=%v", settled, settledExisting, err)
	}
	resolved, err := store.ResolveOperationGroups(ctx, input.SecurityContext.ThreadID, input.CheckpointID)
	if err != nil || len(resolved) != 1 || resolved[0].Intent.OperationGroupID != intent.OperationGroupID {
		t.Fatalf("resolved operation groups mismatch: %#v err=%v", resolved, err)
	}
}

func TestOperationStoreTerminalRaceHasOneClosedUnionWinner(t *testing.T) {
	ctx := context.Background()
	store := newOperationStore(t, ctx)
	now := time.Unix(1_700_000_000, 0).UTC()
	input := operationStoreWriteInput(t, now, "call-race", "before", "after")
	intent, _, _, err := store.BeginOperationGroup(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	after := []domaincheckpoint.ObservedOperationPathV2{operationStoreObserved(intent.Paths[0], "after")}
	before := []domaincheckpoint.ObservedOperationPathV2{operationStoreObserved(intent.Paths[0], "before")}
	type result struct {
		terminal domaincheckpoint.OperationGroupTerminalV2
		err      error
	}
	results := make(chan result, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, candidate := range []struct {
		status string
		reason string
		state  []domaincheckpoint.ObservedOperationPathV2
	}{
		{status: "completed", reason: "mutation_completed", state: after},
		{status: "no_effect", reason: "mutation_failed_before_effect", state: before},
	} {
		candidate := candidate
		go func() {
			start.Wait()
			terminal, err := store.SettleOperationGroup(ctx, intent.OperationGroupID, candidate.status, candidate.reason, candidate.state, now.Add(2*time.Second))
			results <- result{terminal: terminal, err: err}
		}()
	}
	start.Done()
	first := <-results
	second := <-results
	successes := 0
	conflicts := 0
	for _, result := range []result{first, second} {
		if result.err == nil {
			successes++
		} else if errors.Is(result.err, checkpointport.ErrConflict) {
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("terminal race did not produce one winner: first=%#v second=%#v", first, second)
	}
}

func TestOperationStoreQuarantineAndOpenInventoryFailClosed(t *testing.T) {
	ctx := context.Background()
	store := newOperationStore(t, ctx)
	now := time.Unix(1_700_000_000, 0).UTC()
	input := operationStoreWriteInput(t, now, "call-open", "before", "after")
	intent, _, _, err := store.BeginOperationGroup(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	open, err := store.OpenOperationGroups(ctx)
	if err != nil || len(open) != 1 || open[0].OperationGroupID != intent.OperationGroupID {
		t.Fatalf("open operation inventory mismatch: %#v err=%v", open, err)
	}
	diverged := []domaincheckpoint.ObservedOperationPathV2{operationStoreObserved(intent.Paths[0], "third-state")}
	if _, err := store.SettleOperationGroup(ctx, intent.OperationGroupID, "quarantined", "filesystem_diverged", diverged, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	open, err = store.OpenOperationGroups(ctx)
	if err != nil || len(open) != 0 {
		t.Fatalf("settled quarantine remained open: %#v err=%v", open, err)
	}
	if _, err := store.ResolveOperationGroups(ctx, input.SecurityContext.ThreadID, input.CheckpointID); !errors.Is(err, checkpointport.ErrQuarantined) {
		t.Fatalf("quarantined group materialized: %v", err)
	}
}

func newOperationStore(t *testing.T, ctx context.Context) *Store {
	t.Helper()
	root := t.TempDir()
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStoreContext(ctx, root, access)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func operationStoreWriteInput(t *testing.T, now time.Time, callID, before, after string) domaincheckpoint.OperationGroupIntentInputV2 {
	t.Helper()
	arguments := []byte(`{"path":"a.txt","content":"` + after + `"}`)
	securityContext, err := securitytest.GeneralExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-operation-store", TurnID: "turn-operation-store", WorkspaceRealPath: "/workspace",
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal([]string{"write_file"})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "write_file", ToolCallID: checkpointAuthorityTestHostToolCallID(t, callID),
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scope), ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	return domaincheckpoint.OperationGroupIntentInputV2{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("workspace-checkpoint-store"), SourceWorkspaceCheckpointID: "workspace-checkpoint-store",
		ToolName: "write_file", ArgumentsJSON: arguments, CreatedAt: now.Add(time.Second),
		Paths: []domaincheckpoint.OperationPathInputV2{{
			ArgumentKey: "path", RequestedPath: "a.txt", RelativePath: "a.txt", Role: "target",
			PathAuthoritySchemaVersion: 1, AuthorityKind: "workspace", AuthorityRoot: securityContext.WorkspaceRealPath,
			AuthorityRootIdentity: "test:workspace:1",
			AuthorityRootHash:     domainsecurity.SHA256Hex([]byte(securityContext.WorkspaceRealPath + "\x00test:workspace:1")),
			BeforeExisted:         true, BeforeAvailable: true, BeforeHash: domainsecurity.SHA256Hex([]byte(before)), BeforeContent: before,
			ExpectedAfterExisted: true, ExpectedAfterHash: domainsecurity.SHA256Hex([]byte(after)),
		}},
	}
}

func operationStoreObserved(path domaincheckpoint.OperationPathV2, content string) domaincheckpoint.ObservedOperationPathV2 {
	return domaincheckpoint.ObservedOperationPathV2{
		PathAuthoritySchemaVersion: path.PathAuthoritySchemaVersion,
		AuthorityKind:              path.AuthorityKind,
		AuthorityRootHash:          path.AuthorityRootHash,
		RelativePath:               path.RelativePath,
		ObservationStatus:          "exact",
		Existed:                    true,
		Hash:                       domainsecurity.SHA256Hex([]byte(content)),
	}
}
