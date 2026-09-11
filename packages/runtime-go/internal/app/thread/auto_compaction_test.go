package thread

import (
	"context"
	"strings"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainordinaryresult "analytix.local/runtime-go/internal/domain/ordinaryresult"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	threaddomain "analytix.local/runtime-go/internal/domain/thread"
)

type emptyHistoryProjectorV1 struct {
	PublicProjector
	caseSlotCalls int
}

func (projector *emptyHistoryProjectorV1) CommittedCaseOrdinaryResultsV1(
	map[string]any,
) (map[string]domainordinaryresult.ResultSlotV1, error) {
	projector.caseSlotCalls++
	return nil, nil
}

type emptyHistoryCaseAuthorityV1 struct {
	isCaseThreadCalls int
}

func (authority *emptyHistoryCaseAuthorityV1) IsCaseThread(string) bool {
	authority.isCaseThreadCalls++
	return false
}

func (*emptyHistoryCaseAuthorityV1) ContainsContext(domainsecurity.TurnSecurityContext) bool {
	return false
}

func TestAutomaticCompactionRunsOnceAndRecordsAutoContinuation(t *testing.T) {
	at := time.Date(2026, 7, 22, 1, 0, 0, 0, time.UTC)
	current := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_auto", TurnID: "turn_8", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: at.Add(8 * time.Second),
	})
	epochState, err := contextepochapp.BootstrapState(
		current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]any, 0, 8)
	for index := 1; index <= 8; index++ {
		turnID := "turn_" + string(rune('0'+index))
		frozen := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: current.ThreadID, TurnID: turnID, WorkspaceRealPath: current.WorkspaceRealPath,
			ContextEpoch: current.ContextEpoch, IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		turns = append(turns, map[string]any{
			"id": turnID, "threadId": current.ThreadID, "status": "completed", "securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{"id": "item_" + turnID, "kind": "user_message", "text": strings.Repeat("案", 2_000)}},
		})
	}
	repo := &repositoryStub{thread: map[string]any{
		"id": current.ThreadID, "workspace": current.WorkspaceRealPath, "status": "idle", "turns": turns,
		"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(epochState),
		"goal":  map[string]any{"id": "goal_auto", "objective": "preserve objective", "status": "active", "evidenceLedger": []any{}},
		"todos": map[string]any{"items": []any{map[string]any{"id": "todo_auto", "content": "preserve unfinished", "status": "pending"}}},
	}}
	var transition *compactionTransitionStub
	service := NewService(Dependencies{
		Repository: repo, WorkspaceReader: mutationWorkspaceReaderStub{},
		BeginTransition: func(_ context.Context, target domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
			transition = &compactionTransitionStub{target: target}
			return transition, nil
		},
	})
	result, err := service.AutoCompactBeforeTurnV1(context.Background(), AutoCompactionInputV1{
		ThreadID: current.ThreadID, Prompt: "continue", ContextWindowTokens: 20_000, MainThread: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compacted || result.BeforeTokens < result.Thresholds.SoftThresholdTokens || result.AfterTokens >= result.Thresholds.HardThresholdTokens ||
		repo.compactionCommits != 1 || transition == nil || !transition.committed {
		t.Fatalf("automatic compaction result=%#v commits=%d transition=%#v", result, repo.compactionCommits, transition)
	}
	if len(repo.events) != 2 || repo.events[0]["auto"] != true || repo.events[1]["auto"] != true {
		t.Fatalf("automatic lifecycle events = %#v", repo.events)
	}
	compactionTurn, _ := listAny(repo.thread["turns"])[0].(map[string]any)
	compactionItem, _ := listAny(compactionTurn["items"])[0].(map[string]any)
	continuation, parseErr := threaddomain.ParseTaskContinuationSnapshotV1(compactionItem["taskContinuation"])
	if parseErr != nil || continuation.Goal == nil || len(continuation.Todos) != 1 || compactionItem["auto"] != true {
		t.Fatalf("automatic continuation = %#v item=%#v err=%v", continuation, compactionItem, parseErr)
	}
	projectedItem, ok := projectedThreadItem(repo.thread, stringField(compactionTurn, "id"), stringField(compactionItem, "id"))
	if !ok {
		t.Fatalf("automatic V4 compaction item did not survive the closed ordinary projection")
	}
	if _, err := threaddomain.ParseTaskContinuationSnapshotV1(projectedItem["taskContinuation"]); err != nil {
		t.Fatalf("projected automatic continuation is invalid: %#v err=%v", projectedItem, err)
	}
	projectedEvent, visible, projectionErr := ProjectPublicThreadEvent(current.ThreadID, repo.thread, repo.events[1])
	if projectionErr != nil || !visible || projectedEvent["auto"] != true {
		t.Fatalf("automatic compaction lifecycle projection=%#v visible=%t err=%v", projectedEvent, visible, projectionErr)
	}
}

func TestAutomaticCompactionBelowSoftThresholdDoesNotMutate(t *testing.T) {
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_auto_below", "status": "idle", "turns": []any{},
	}}
	service := NewService(Dependencies{Repository: repo})
	result, err := service.AutoCompactBeforeTurnV1(context.Background(), AutoCompactionInputV1{
		ThreadID: "thr_auto_below", Prompt: "short prompt", ContextWindowTokens: 1_000, MainThread: true,
	})
	if err != nil || result.Compacted || result.BeforeTokens >= result.Thresholds.SoftThresholdTokens ||
		repo.compactionCommits != 0 || len(repo.events) != 0 {
		t.Fatalf("below-threshold automatic compaction result=%#v commits=%d events=%#v err=%v", result, repo.compactionCommits, repo.events, err)
	}
}

func TestAutomaticCompactionEmptyHistorySkipsCaseProjectionAndAuthority(t *testing.T) {
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_auto_empty_fast_path", "status": "idle", "turns": []any{},
	}}
	projector := &emptyHistoryProjectorV1{PublicProjector: NewTrustedPublicProjector(nil)}
	authority := &emptyHistoryCaseAuthorityV1{}
	service := NewService(Dependencies{
		Repository: repo, PublicProjector: projector, CaseThreads: authority,
	})
	result, err := service.AutoCompactBeforeTurnV1(context.Background(), AutoCompactionInputV1{
		ThreadID: "thr_auto_empty_fast_path", Prompt: "short prompt", ContextWindowTokens: 1_000, MainThread: true,
	})
	if err != nil || result.Compacted || projector.caseSlotCalls != 0 || authority.isCaseThreadCalls != 0 {
		t.Fatalf("empty history performed case work: result=%#v projectorCalls=%d authorityCalls=%d err=%v",
			result, projector.caseSlotCalls, authority.isCaseThreadCalls, err)
	}
}

func TestCaseAutomaticCompactionBelowHardLimitDoesNotBlockOrdinaryTurn(t *testing.T) {
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_case_auto_soft", "status": "idle", "caseId": "case-a", "turns": []any{},
	}}
	service := NewService(Dependencies{Repository: repo})
	result, err := service.AutoCompactBeforeTurnV1(context.Background(), AutoCompactionInputV1{
		ThreadID: "thr_case_auto_soft", Prompt: strings.Repeat("案", 700), ContextWindowTokens: 1_200, MainThread: true,
	})
	if err != nil || result.Compacted || result.BeforeTokens < result.Thresholds.SoftThresholdTokens ||
		result.BeforeTokens >= result.Thresholds.HardThresholdTokens || repo.compactionCommits != 0 || len(repo.events) != 0 {
		t.Fatalf("case soft-limit result=%#v err=%v commits=%d events=%#v", result, err, repo.compactionCommits, repo.events)
	}
}

func TestCaseAutomaticCompactionRawHardLimitDefersToExactEffectGuard(t *testing.T) {
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_case_auto", "status": "idle", "caseId": "case-a", "turns": []any{},
	}}
	service := NewService(Dependencies{Repository: repo})
	result, err := service.AutoCompactBeforeTurnV1(context.Background(), AutoCompactionInputV1{
		ThreadID: "thr_case_auto", Prompt: strings.Repeat("案", 1_000), ContextWindowTokens: 1_000, MainThread: true,
	})
	if err != nil || result.BeforeTokens < result.Thresholds.HardThresholdTokens ||
		result.Compacted || repo.compactionCommits != 0 || len(repo.events) != 0 {
		t.Fatalf("case hard-limit result=%#v err=%v commits=%d events=%#v", result, err, repo.compactionCommits, repo.events)
	}
}

func (*emptyHistoryCaseAuthorityV1) RestartPreservesThreadV1(string) bool { return false }
