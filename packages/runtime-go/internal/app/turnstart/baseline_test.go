package turnstart

import (
	"context"
	"errors"
	"testing"
	"time"

	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
)

type preparedBaselineStoreStub struct {
	thread map[string]any
	digest string
	err    error
	reads  int
}

func TestTurnStartCannotReuseCommittedIdentityBeforeWorkspaceEffects(t *testing.T) {
	thread := map[string]any{"id": "thread-reused", "workspace": "/workspace/canonical", "status": "idle", "turns": []any{map[string]any{"id": "turn-existing", "status": "completed"}}}
	digest, err := BaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	appendInput := threadapp.AppendTurnInput{Thread: thread, Turn: map[string]any{"id": "turn-existing"}, ThreadPatch: map[string]any{}}
	if _, err := AppendIfBaseline(appendInput, "thread-reused", "/workspace/canonical", digest); err == nil {
		t.Error("baseline CAS appended a second turn with the same identity")
	}
	appendInput.Turn = map[string]any{"id": "turn-new"}
	if next, err := AppendIfBaseline(appendInput, "thread-reused", "/workspace/canonical", digest); err != nil || len(next["turns"].([]any)) != 2 {
		t.Fatalf("new identity cannot continue an existing child thread: %v", err)
	}
	reader := &transitionWorkspaceReaderStub{realPath: "/workspace/canonical"}
	transition, err := BeginSecurityTransition(context.Background(), subagentapp.NewRuntimeState(), testIdentityAuthority(), testIdentityPrincipal(), reader, nil, thread, "thread-reused", "turn-existing", "/workspace/canonical", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	defer transition.Abort()
	workspaceEffects := 0
	_, err = CommitPreparedTurn(PreparedCommitInput{Context: context.Background(), HostContext: context.Background(), Store: &preparedBaselineStoreStub{thread: thread, digest: digest},
		Transition: transition, Principal: testIdentityPrincipal(), Reader: reader, SecurityAuthority: turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority()}, InitialBaselineDigest: digest,
		ThreadID: "thread-reused", TurnID: "turn-existing", Workspace: "/workspace/canonical",
		PrepareWorkspaceBeforeFreeze: func(context.Context, string) error {
			workspaceEffects++
			return errors.New("synthetic workspace effect reached")
		},
	})
	if err == nil || workspaceEffects != 0 {
		t.Fatalf("reused turn reached first workspace effect: effects=%d err=%v", workspaceEffects, err)
	}
}

type preparedBaselineCompactorStub struct {
	calls  int
	result threadapp.AutoCompactionResultV1
	err    error
}

func (stub *preparedBaselineCompactorStub) AutoCompactBeforeTurnV1(context.Context, threadapp.AutoCompactionInputV1) (threadapp.AutoCompactionResultV1, error) {
	stub.calls++
	return stub.result, stub.err
}

func (store *preparedBaselineStoreStub) ReadThreadStartBaseline(string) (map[string]any, string, error) {
	store.reads++
	return store.thread, store.digest, store.err
}

func (*preparedBaselineStoreStub) AppendTurnToThreadIfBaseline(string, map[string]any, string, map[string]any, string, string) error {
	return nil
}

func (*preparedBaselineStoreStub) GetThread(string) (map[string]any, error) {
	return nil, nil
}

func TestPrepareReservedStartBaselineValidatesBeforeCompaction(t *testing.T) {
	thread := map[string]any{"id": "thread-reserved", "status": "idle", "turns": []any{}}
	store := &preparedBaselineStoreStub{thread: thread, digest: "baseline"}
	compactor := &preparedBaselineCompactorStub{err: errors.New("compaction must not run")}
	validated := 0
	got, digest, err := PrepareStartBaselineV1(PrepareStartBaselineInputV1{
		Context: context.Background(), Store: store, Compactor: compactor, ThreadID: "thread-reserved", Reserved: true,
		ValidateReserved: func(current map[string]any) error {
			validated++
			if current["id"] != "thread-reserved" {
				t.Fatal("reserved validator received the wrong baseline")
			}
			return nil
		},
	})
	if err != nil || got["id"] != "thread-reserved" || digest != "baseline" || validated != 1 || compactor.calls != 0 || store.reads != 1 {
		t.Fatalf("reserved baseline ordering changed: got=%#v digest=%q validated=%d compact=%d reads=%d err=%v", got, digest, validated, compactor.calls, store.reads, err)
	}
}

func TestPreparedCommitBaselineReusesPostBarrierReadPair(t *testing.T) {
	thread := map[string]any{
		"id": "thread-prepared-baseline", "workspace": "/workspace/prepared", "status": "idle", "turns": []any{},
	}
	digest, err := BaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	store := &preparedBaselineStoreStub{err: errors.New("unexpected redundant baseline read")}
	gotThread, gotDigest, err := preparedCommitBaseline(PreparedCommitInput{
		Store: store, ThreadID: "thread-prepared-baseline", InitialBaselineDigest: digest,
		PostBarrierThread: thread, PostBarrierDigest: digest, ReusePostBarrierBaseline: true,
	})
	if err != nil || gotDigest != digest || gotThread["id"] != thread["id"] || store.reads != 0 {
		t.Fatalf("validated post-barrier baseline was not reused: thread=%#v digest=%q reads=%d err=%v", gotThread, gotDigest, store.reads, err)
	}

	if _, _, err := preparedCommitBaseline(PreparedCommitInput{
		Store: store, ThreadID: "thread-prepared-baseline", InitialBaselineDigest: digest,
		PostBarrierThread: thread, ReusePostBarrierBaseline: true,
	}); !errors.Is(err, ErrBaselineConflict) || store.reads != 0 {
		t.Fatalf("incomplete prepared baseline pair was not rejected without a fallback read: reads=%d err=%v", store.reads, err)
	}
}

func TestPreparedCommitBaselineFallsBackToAuthoritativeStoreRead(t *testing.T) {
	thread := map[string]any{"id": "thread-store-baseline", "workspace": "/workspace/store", "status": "idle", "turns": []any{}}
	digest, err := BaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	store := &preparedBaselineStoreStub{thread: thread, digest: digest}
	gotThread, gotDigest, err := preparedCommitBaseline(PreparedCommitInput{Store: store, ThreadID: "thread-store-baseline"})
	if err != nil || gotDigest != digest || gotThread["id"] != thread["id"] || store.reads != 1 {
		t.Fatalf("store baseline fallback changed: thread=%#v digest=%q reads=%d err=%v", gotThread, gotDigest, store.reads, err)
	}
}

func TestTurnStartBaselineCASRejectsStaleAndRunningThread(t *testing.T) {
	thread := map[string]any{
		"id": "thread-a", "workspace": "/workspace/a", "status": "idle", "updatedAt": "before", "turns": []any{},
	}
	digest, err := BaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	input := threadapp.AppendTurnInput{
		Thread: thread, Turn: map[string]any{"id": "turn-a"}, ProviderID: "provider-a",
		ThreadPatch: map[string]any{}, UpdatedAt: "after",
	}
	if _, err := AppendIfBaseline(input, "thread-a", "/workspace/a", digest); err != nil {
		t.Fatalf("exact baseline was rejected: %v", err)
	}

	changed := map[string]any{
		"id": "thread-a", "workspace": "/workspace/a", "status": "idle", "updatedAt": "concurrent", "turns": []any{},
	}
	input.Thread = changed
	if _, err := AppendIfBaseline(input, "thread-a", "/workspace/a", digest); !errors.Is(err, ErrBaselineConflict) {
		t.Fatalf("stale prepared turn crossed a concurrent durable update: %v", err)
	}
	running := map[string]any{
		"id": "thread-a", "workspace": "/workspace/a", "status": "running", "updatedAt": "before", "turns": []any{},
	}
	runningDigest, err := BaselineDigest(running)
	if err != nil {
		t.Fatal(err)
	}
	input.Thread = running
	if _, err := AppendIfBaseline(input, "thread-a", "/workspace/a", runningDigest); !errors.Is(err, ErrBaselineConflict) {
		t.Fatalf("a second active turn crossed the high-water check: %v", err)
	}
}

func TestTurnStartBaselineBindsThreadAndWorkspace(t *testing.T) {
	thread := map[string]any{"id": "thread-a", "workspace": "/workspace/a", "status": "idle", "turns": []any{}}
	digest, err := BaselineDigest(thread)
	if err != nil {
		t.Fatal(err)
	}
	for _, mismatch := range []struct{ threadID, workspace string }{
		{threadID: "thread-b", workspace: "/workspace/a"},
		{threadID: "thread-a", workspace: "/workspace/b"},
	} {
		if err := ValidateBaseline(thread, mismatch.threadID, mismatch.workspace, digest); !errors.Is(err, ErrBaselineConflict) {
			t.Fatalf("turn baseline identity mismatch was accepted: %#v err=%v", mismatch, err)
		}
	}
}
