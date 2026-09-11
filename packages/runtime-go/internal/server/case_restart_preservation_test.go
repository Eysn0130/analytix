package server

import (
	"context"
	"errors"
	"reflect"
	"testing"

	casethreadapp "analytix.local/runtime-go/internal/app/casethread"
	threadapp "analytix.local/runtime-go/internal/app/thread"
)

type restartHeldServerAuthorityV1 struct {
	caseThreadAuthorityStub
	held string
}

func TestRestartPreservedCommittedCompactionHasHistoricalProofWithoutExecution(t *testing.T) {
	ctx := context.Background()
	fixture := newCaseCompactionCrashFixtureV1(t)
	fixture.stageTarget(t, fixture.authority)
	result, err := fixture.store.CommitCompaction(fixture.prepared.CommitRequest())
	if err != nil || !result.Committed || !result.AuthorityCommitted {
		t.Fatalf("original compaction did not commit: %v", err)
	}
	if err := fixture.authority.PreserveRestartScopeV1(ctx, []string{fixture.threadID}); err != nil {
		t.Fatal(err)
	}
	before := runtimeRestoreFileDigestsV1(t, fixture.store.root)
	authorityBefore := runtimeRestoreFileDigestsV1(t, fixture.authorityStoreDir)
	reader := threadapp.NewCaseThreadRestartAuthorityReaderV1(ctx, fixture.store, fixture.authority)
	thread, err := reader.GetThread(fixture.threadID)
	if err != nil {
		t.Fatal(err)
	}
	var target map[string]any
	for _, value := range listAny(thread["turns"]) {
		turn, _ := value.(map[string]any)
		if stringField(turn, "id") == fixture.prepared.SecurityContext.TurnID {
			target = turn
		}
	}
	if target == nil {
		t.Fatal("original compaction target is absent")
	}
	validator := reader.(interface {
		ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
	})
	if err := validator.ValidateCaseCompactionAuthorityTurnV1(fixture.threadID, thread, target); err != nil {
		t.Fatalf("exact held compaction history was refused: %v", err)
	}
	live := threadapp.NewCaseThreadAuthorityReader(fixture.store, fixture.authority).(interface {
		ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
	})
	if err := live.ValidateCaseCompactionAuthorityTurnV1(fixture.threadID, thread, target); err == nil {
		t.Fatal("ordinary reader admitted held historical authority")
	}
	changed := cloneMap(target)
	changed["items"] = []any{}
	if err := validator.ValidateCaseCompactionAuthorityTurnV1(fixture.threadID, thread, changed); err == nil {
		t.Fatal("historical reader admitted changed compaction metadata")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	cancelledReader := threadapp.NewCaseThreadRestartAuthorityReaderV1(cancelled, fixture.store, fixture.authority).(interface {
		ValidateCaseCompactionAuthorityTurnV1(string, map[string]any, map[string]any) error
	})
	if err := cancelledReader.ValidateCaseCompactionAuthorityTurnV1(fixture.threadID, thread, target); !errors.Is(err, context.Canceled) {
		t.Fatalf("historical reader lost context cancellation: %v", err)
	}
	if fixture.authority.ContainsContext(fixture.prepared.SecurityContext) || fixture.authority.CanExecute(fixture.threadID) || len(threadapp.TrustedCaseCompactionTurnIDsV1(thread, fixture.authority)) != 0 {
		t.Fatal("historical compaction proof became execution or provider-history authority")
	}
	if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, fixture.store.root)) || !reflect.DeepEqual(authorityBefore, runtimeRestoreFileDigestsV1(t, fixture.authorityStoreDir)) {
		t.Fatal("historical compaction validation changed original authority")
	}
}

func (authority *restartHeldServerAuthorityV1) RestartPreservesThreadV1(id string) bool {
	return id == authority.held
}

func TestRestartPreservedTurnStartDeniesBeforeAllocationAndProvider(t *testing.T) {
	h, pending, _, _, providerClient, release := runtimePreparedChildExecutionFixtureV1(t)
	release()
	h.caseThreads = &restartHeldServerAuthorityV1{caseThreadAuthorityStub: caseThreadAuthorityStub{threads: map[string]bool{}}, held: pending.ThreadID}
	h.store.SetCaseThreadAuthority(h.caseThreads)
	before := runtimeRestoreFileDigestsV1(t, h.store.root)
	sequence := h.turnSeq
	_, err := h.startRuntimeTurn(context.Background(), pending.ThreadID, startRuntimeTurnRequest{Prompt: "synthetic preserved request"})
	if !errors.Is(err, casethreadapp.ErrRestartPreserved) {
		t.Errorf("turn start did not refuse preserved unindexed thread: %v", err)
	}
	if sequence != h.turnSeq || len(providerClient.Requests()) != 0 || !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, h.store.root)) {
		t.Fatal("held turn start allocated, wrote or called provider")
	}
}

func TestRestartPreservedDerivationDeniesBeforeAllocationAndWrites(t *testing.T) {
	for _, operation := range []string{"fork", "resume"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newCaseCompactionCrashFixtureV1(t)
			ordinary, err := fixture.store.CreateThread(map[string]any{"title": "held original general primary"}, "")
			if err != nil {
				t.Fatal(err)
			}
			id := stringField(ordinary, "id")
			if err := fixture.authority.PreserveRestartScopeV1(context.Background(), []string{fixture.threadID, id}); err != nil {
				t.Fatal(err)
			}
			before := runtimeRestoreFileDigestsV1(t, fixture.store.root)
			for _, held := range []string{fixture.threadID, id} {
				if operation == "fork" {
					_, err = fixture.store.ForkThread(held, nil)
				} else {
					_, err = fixture.store.ResumeSession(held, nil)
				}
				if !errors.Is(err, casethreadapp.ErrRestartPreserved) {
					t.Errorf("%s did not deny preserved thread: %v", operation, err)
				}
			}
			if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, fixture.store.root)) {
				t.Fatal("held derivation changed durable inventory")
			}
		})
	}
}

type restartQuarantineRecorderV1 struct {
	*casethreadapp.Registry
	last map[string]string
}

func (authority *restartQuarantineRecorderV1) ReplaceQuarantine(next map[string]string) {
	authority.last = next
	authority.Registry.ReplaceQuarantine(next)
}

func TestRestartPreservedSignedCompactionRemainsByteIdenticalAcrossTwoRestarts(t *testing.T) {
	fixture := newCaseCompactionCrashFixtureV1(t)
	fixture.stageAndCommitTarget(t, fixture.authority)
	before := runtimeRestoreFileDigestsV1(t, fixture.store.root)
	authorityBefore := runtimeRestoreFileDigestsV1(t, fixture.authorityStoreDir)
	for i := 0; i < 2; i++ {
		store, registry := fixture.reopen(t)
		if err := registry.PreserveRestartScopeV1(context.Background(), []string{fixture.threadID}); err != nil {
			t.Fatal(err)
		}
		authority := &restartQuarantineRecorderV1{Registry: registry}
		if err := threadapp.RecoverCommittedCaseCompactions(context.Background(), authority, store); err != nil {
			t.Fatal(err)
		}
		if err := casethreadapp.RepairCommittedContexts(authority, store); err != nil {
			t.Fatal(err)
		}
		if authority.last[fixture.threadID] != "" {
			t.Error("preservation was reclassified as quarantine")
		}
		if !registry.RestartPreservesThreadV1(fixture.threadID) || !registry.IsCaseThread(fixture.threadID) || registry.CanExecute(fixture.threadID) {
			t.Fatal("restart changed the held case identity or authority")
		}
		if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, store.root)) || !reflect.DeepEqual(authorityBefore, runtimeRestoreFileDigestsV1(t, fixture.authorityStoreDir)) {
			t.Fatal("held crash recovery changed original bytes")
		}
	}
}
