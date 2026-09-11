package thread

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	contextepochapp "analytix.local/runtime-go/internal/app/contextepoch"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincontextepoch "analytix.local/runtime-go/internal/domain/contextepoch"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type repositoryStub struct {
	listLimitSearch   string
	created           map[string]any
	patched           bool
	events            []map[string]any
	thread            map[string]any
	goal              map[string]any
	todos             map[string]any
	compactionCommits int
	highestSeq        int
	highestSeqErr     error
	err               error
}

type compactionTransitionStub struct {
	target     domainsecurity.TurnSecurityContext
	prepared   bool
	committed  bool
	aborted    bool
	prepareErr error
}

func (transition *compactionTransitionStub) Prepare(_ context.Context, target domainsecurity.TurnSecurityContext, _ time.Duration) error {
	if transition.prepareErr != nil {
		return transition.prepareErr
	}
	if transition.target.ContextDigest != "" && target != transition.target {
		return errors.New("transition target mismatch")
	}
	transition.prepared = true
	return nil
}

func (transition *compactionTransitionStub) Commit() error {
	if !transition.prepared {
		return errors.New("transition was not prepared")
	}
	transition.committed = true
	return nil
}

func (transition *compactionTransitionStub) Abort() {
	if !transition.committed {
		transition.aborted = true
	}
}

func (r *repositoryStub) ListThreads(_ bool, _ bool, _ bool, search string) ([]map[string]any, error) {
	r.listLimitSearch = search
	if r.err != nil {
		return nil, r.err
	}
	return []map[string]any{{"id": "thr_1"}, {"id": "thr_2"}}, nil
}

func (r *repositoryStub) CreateThread(request map[string]any, _ string) (map[string]any, error) {
	r.created = request
	if r.err != nil {
		return nil, r.err
	}
	return map[string]any{"id": "thr_new", "title": "New", "status": "idle"}, nil
}

func (r *repositoryStub) GetThread(threadID string) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.thread == nil {
		return map[string]any{"id": threadID, "title": "needle " + threadID, "workspace": "/workspace", "status": "idle", "turns": []any{}}, nil
	}
	return r.thread, nil
}

func (r *repositoryStub) PatchThread(_ string, patch map[string]any) (map[string]any, error) {
	r.patched = true
	if r.err != nil {
		return nil, r.err
	}
	return map[string]any{"id": "thr_1", "title": patch["title"], "status": "idle"}, nil
}

func (r *repositoryStub) ReadThreadMutationBaseline(threadID string) (map[string]any, string, error) {
	thread, err := r.GetThread(threadID)
	if err != nil {
		return nil, "", err
	}
	digest, err := MutationBaselineDigest(thread)
	return thread, digest, err
}

func (r *repositoryStub) PatchThreadIfBaseline(threadID string, patch map[string]any, expectedDigest string) (map[string]any, error) {
	thread, digest, err := r.ReadThreadMutationBaseline(threadID)
	if err != nil {
		return nil, err
	}
	if digest != expectedDigest {
		return nil, ErrThreadMutationBaselineConflict
	}
	r.thread = ApplyPatch(thread, patch, time.Now().UTC().Format(time.RFC3339Nano))
	r.patched = true
	return r.thread, nil
}

func (r *repositoryStub) CommitWorkspaceMutation(request WorkspaceMutationCommitRequest) (map[string]any, error) {
	thread, err := ApplyWorkspaceMutationCommit(r.thread, request)
	if err != nil {
		return nil, err
	}
	r.thread = thread
	r.patched = true
	return thread, nil
}

func TestServiceBlocksCaseWorkspacePatchBeforeMutation(t *testing.T) {
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case", TurnID: "turn_case", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 2, IssuedAt: time.Unix(2, 0),
	})
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_case", "workspace": "/cases/a", "securityState": publicProjectionSecurityRecord(securityContext),
		"turns": []any{map[string]any{"id": "turn_case", "status": "running", "securityContext": publicProjectionSecurityRecord(securityContext), "items": []any{}}},
	}}
	service := newMutationService(repo)
	if _, err := service.Patch(context.Background(), "thr_case", map[string]any{"workspace": "/cases/b"}); !errors.Is(err, ErrCaseWorkspaceSignedRebind) {
		t.Fatalf("case workspace patch was not blocked: %v", err)
	}
	if repo.patched {
		t.Fatal("case workspace mutation reached persistence before an epoch transition")
	}
}

func TestWorkspacePatchWaitsForForegroundTurnAndCommitsEpochAtomically(t *testing.T) {
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_rebind", "title": "Rebind", "workspace": "/workspace/a", "status": "running", "turns": []any{},
	}}
	transitionStarted := make(chan struct{})
	quiesceEntered := make(chan struct{})
	releaseTurn := make(chan struct{})
	observer := mutationWorkspaceReaderStub{}
	service := NewService(Dependencies{
		Repository: repo, WorkspaceReader: observer,
		WorkspaceSecurity: turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer: observer, RiskAuthority: &mutationServiceRiskAuthority{mutationRiskAuthority: newMutationRiskAuthority()},
		},
		BeginWorkspaceTransition: func(_ context.Context, _, _, _, _, _ string, barrier func() error) (CompactionTransition, error) {
			close(transitionStarted)
			if err := barrier(); err != nil {
				return nil, err
			}
			return &compactionTransitionStub{}, nil
		},
		QuiesceThreadTurns: func(ctx context.Context, threadID string) error {
			if threadID != "thr_rebind" {
				return errors.New("wrong thread quiesced")
			}
			close(quiesceEntered)
			select {
			case <-releaseTurn:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	patched := make(chan error, 1)
	go func() {
		_, err := service.Patch(context.Background(), "thr_rebind", map[string]any{"workspace": "/workspace/b"})
		patched <- err
	}()
	select {
	case <-transitionStarted:
	case <-time.After(time.Second):
		t.Fatal("workspace union writer did not start")
	}
	select {
	case <-quiesceEntered:
	case <-time.After(time.Second):
		t.Fatal("foreground turn barrier did not start under the writer")
	}
	if repo.patched {
		t.Fatal("workspace changed before foreground terminal ownership settled")
	}
	close(releaseTurn)
	select {
	case err := <-patched:
		if err != nil {
			t.Fatalf("workspace patch failed after foreground turn settled: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("workspace patch remained blocked")
	}
	if repo.thread["workspace"] != "/workspace/b" || repo.thread["status"] != "idle" {
		t.Fatalf("workspace mutation did not commit one durable state: %#v", repo.thread)
	}
	current, found, err := turnsecurityapp.LatestContext(repo.thread)
	if err != nil || !found || current.WorkspaceRealPath != "/workspace/b" || current.ContextEpoch < 2 {
		t.Fatalf("workspace mutation current authority mismatch: context=%#v found=%t err=%v", current, found, err)
	}
	state, ok, err := contextepochapp.StateFromThread(repo.thread)
	if err != nil || !ok || state.AcceptedSnapshot.Epoch != current.ContextEpoch {
		t.Fatalf("workspace mutation restart authority mismatch: state=%#v ok=%t err=%v", state, ok, err)
	}
}

func (r *repositoryStub) DeleteThread(string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return true, nil
}

func (r *repositoryStub) DeleteThreadIfBaseline(threadID string, expectedDigest string) (bool, error) {
	_, digest, err := r.ReadThreadMutationBaseline(threadID)
	if err != nil {
		return false, err
	}
	if digest != expectedDigest {
		return false, ErrThreadMutationBaselineConflict
	}
	return r.DeleteThread(threadID)
}

func (r *repositoryStub) ForkThread(_ string, request map[string]any) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	return map[string]any{"id": "thr_fork", "title": request["title"], "status": "idle"}, nil
}

func (r *repositoryStub) RewindThread(threadID string, turnID string) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	return map[string]any{
		"threadId":       threadID,
		"turnId":         turnID,
		"removedTurns":   float64(1),
		"remainingTurns": float64(2),
		"removedTurnIds": []any{turnID},
	}, nil
}

func (r *repositoryStub) RewindThreadIfBaseline(threadID string, turnID string, expectedDigest string) (map[string]any, error) {
	_, digest, err := r.ReadThreadMutationBaseline(threadID)
	if err != nil {
		return nil, err
	}
	if digest != expectedDigest {
		return nil, ErrThreadMutationBaselineConflict
	}
	return r.RewindThread(threadID, turnID)
}

func (r *repositoryStub) CommitRewindMutation(request RewindMutationCommitRequest) (RewindMutationCommitResult, error) {
	prepared, err := ApplyRewindMutationCommit(r.thread, request)
	if err != nil {
		return RewindMutationCommitResult{}, err
	}
	r.thread = prepared.Thread
	return RewindMutationCommitResult{
		Response: prepared.Response, SecurityContext: prepared.SecurityContext, EpochState: prepared.EpochState, Committed: true,
	}, nil
}

func (r *repositoryStub) CommitCompaction(request CompactionCommitRequest) (CompactionCommitResult, error) {
	if r.err != nil {
		return CompactionCommitResult{}, r.err
	}
	prepared, err := ApplyCompactionCommit(r.thread, request)
	if err != nil {
		return CompactionCommitResult{}, err
	}
	r.compactionCommits++
	r.thread = prepared.Thread
	return CompactionCommitResult{
		Result: prepared.Result, SecurityContext: prepared.SecurityContext, EpochState: prepared.EpochState, Committed: true,
	}, nil
}

func (r *repositoryStub) GetGoal(string) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.goal, nil
}

func (r *repositoryStub) SetGoal(_ string, patch map[string]any) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.goal = patch
	return patch, nil
}

func (r *repositoryStub) ClearGoal(string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return true, nil
}

func (r *repositoryStub) GetTodos(string) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.todos, nil
}

func (r *repositoryStub) SetTodos(_ string, items []any) (map[string]any, error) {
	if r.err != nil {
		return nil, r.err
	}
	r.todos = map[string]any{"items": items}
	return r.todos, nil
}

func (r *repositoryStub) ClearTodos(string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	return true, nil
}

func (r *repositoryStub) HighestSeq(string) (int, error) {
	return r.highestSeq, r.highestSeqErr
}

func TestServiceGetPreservesObservationErrors(t *testing.T) {
	for _, source := range []string{"usage", "sequence", "pending_gates"} {
		t.Run(source, func(t *testing.T) {
			cause := errors.New("synthetic thread observation unavailable")
			repo := &repositoryStub{thread: map[string]any{"id": "thr_observation", "title": "Synthetic"}}
			deps := Dependencies{Repository: repo}
			switch source {
			case "usage":
				deps.UsageSnapshot = func(string) (map[string]any, error) { return nil, cause }
			case "sequence":
				repo.highestSeqErr = cause
			case "pending_gates":
				deps.PendingGateSnapshot = func(string) ([]string, []string, error) { return nil, nil, cause }
			}
			thread, err := NewService(deps).Get("thr_observation")
			if thread != nil || !errors.Is(err, cause) || len(repo.events) != 0 {
				t.Fatalf("observation error became a successful thread: returned=%v err=%v events=%d", thread != nil, err, len(repo.events))
			}
		})
	}
}

func (r *repositoryStub) RecordEvent(event map[string]any) (map[string]any, []string, error) {
	event["seq"] = float64(len(r.events) + 1)
	r.events = append(r.events, event)
	return event, nil, nil
}

type mutationWorkspaceReaderStub struct{}

func (mutationWorkspaceReaderStub) Observe(workspace string) (domainsecurity.CaseBindingObservationV1, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "/workspace"
	}
	return domainsecurity.NewCaseBindingObservationV1(domainsecurity.CaseBindingObservationInputV1{
		WorkspaceRealPath: workspace, State: domainsecurity.CaseBindingStateMissing,
	})
}

func (mutationWorkspaceReaderStub) ReadOptional(string) (domainsecurity.CaseBinding, bool, error) {
	return domainsecurity.CaseBinding{}, false, nil
}

func (mutationWorkspaceReaderStub) WorkspaceRealPath(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "/workspace"
	}
	return workspace, nil
}

func newMutationService(repo Repository) *Service {
	observer := mutationWorkspaceReaderStub{}
	return NewService(Dependencies{
		Repository: repo, WorkspaceReader: observer,
		WorkspaceSecurity: turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer: observer, RiskAuthority: &mutationServiceRiskAuthority{mutationRiskAuthority: newMutationRiskAuthority()},
		},
		BeginTransition: func(_ context.Context, _ domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
			return &compactionTransitionStub{}, nil
		},
		BeginScopeTransition: func(_ context.Context, _, _, _, _ string) (CompactionTransition, error) {
			return &compactionTransitionStub{}, nil
		},
		BeginWorkspaceTransition: func(_ context.Context, _, _, _, _, _ string, barrier func() error) (CompactionTransition, error) {
			if err := barrier(); err != nil {
				return nil, err
			}
			return &compactionTransitionStub{}, nil
		},
		BeginMetadataRead: func(context.Context, string, string, string, string) (func(), error) {
			return func() {}, nil
		},
		QuiesceThreadTurns: func(context.Context, string) error { return nil },
	})
}

type mutationServiceRiskAuthority struct{ *mutationRiskAuthority }

func (authority *mutationServiceRiskAuthority) ValidateCurrent(_ context.Context, securityContext domainsecurity.TurnSecurityContext) error {
	return domainsecurity.ValidateTurnSecurityContextForExecution(securityContext)
}

func TestServiceListAppliesLimitAndClones(t *testing.T) {
	repo := &repositoryStub{}
	service := NewService(Dependencies{Repository: repo})
	threads, err := service.List(ListInput{Search: "needle", Limit: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if repo.listLimitSearch != "" || len(threads) != 1 || threads[0]["id"] != "thr_2" {
		t.Fatalf("list result mismatch: search=%q threads=%#v", repo.listLimitSearch, threads)
	}
}

func TestServiceCreateAppliesDefaultsAndRecordsEvent(t *testing.T) {
	repo := &repositoryStub{}
	service := NewService(Dependencies{Repository: repo, DefaultApprovalPolicy: "never", DefaultSandboxMode: "read-only"})
	thread, err := service.Create(map[string]any{"title": "Draft", "approvalPolicy": "invalid"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if repo.created["approvalPolicy"] != "never" || repo.created["sandboxMode"] != "read-only" {
		t.Fatalf("defaults not applied: %#v", repo.created)
	}
	if thread["id"] != "thr_new" || len(repo.events) != 1 || repo.events[0]["kind"] != "thread_created" {
		t.Fatalf("create result/event mismatch: thread=%#v events=%#v", thread, repo.events)
	}
	if repo.events[0]["title"] != nil {
		t.Fatalf("thread lifecycle event exposed canonical title: %#v", repo.events[0])
	}
}

func TestServiceGetProjectsLatestSeqUsageAndSummary(t *testing.T) {
	repo := &repositoryStub{
		highestSeq: 4,
		thread: map[string]any{
			"id":    "thr_1",
			"title": "Hello",
			"turns": []any{
				map[string]any{"items": []any{map[string]any{"kind": "user_message", "role": "user"}}},
			},
		},
	}
	service := NewService(Dependencies{
		Repository: repo,
		UsageSnapshot: func(string) (map[string]any, error) {
			return map[string]any{"totalTokens": float64(12)}, nil
		},
	})
	thread, err := service.Get("thr_1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if thread["latestSeq"] != float64(4) || thread["messageCount"] != float64(1) {
		t.Fatalf("projection mismatch: %#v", thread)
	}
	usage, _ := thread["usage"].(map[string]any)
	if usage["totalTokens"] != float64(12) {
		t.Fatalf("usage mismatch: %#v", usage)
	}
}

func TestServiceGetCaseThreadWithholdsUnsignedAssistantToolGoalAndTodoFacts(t *testing.T) {
	securityContext := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case", TurnID: "turn_case", WorkspaceRealPath: "/cases/a", TenantID: "tenant", UserID: "user",
		CaseID: "case-a", CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 2, IssuedAt: time.Unix(1, 0),
	})
	securityRecord := map[string]any{
		"version": securityContext.Version, "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"workspaceRealPath": securityContext.WorkspaceRealPath, "tenantId": securityContext.TenantID, "userId": securityContext.UserID,
		"caseId": securityContext.CaseID, "caseBindingHash": securityContext.CaseBindingHash,
		"datasetSnapshotId": securityContext.DatasetSnapshotID, "sourceManifestHash": securityContext.SourceManifestHash,
		"contextEpoch": securityContext.ContextEpoch, "issuedAt": securityContext.IssuedAt, "contextDigest": securityContext.ContextDigest,
	}
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_case", "title": "Case", "workspace": "/cases/a", "model": "model", "mode": "agent", "status": "idle",
		"createdAt": "2026-07-11T00:00:00Z", "updatedAt": "2026-07-11T00:00:00Z", "securityState": securityRecord,
		"goal":  map[string]any{"objective": "GOAL_FACT_SENTINEL_4200000"},
		"todos": map[string]any{"items": []any{map[string]any{"content": "TODO_ACCOUNT_SENTINEL_622202"}}},
		"turns": []any{map[string]any{
			"id": "turn_case", "threadId": "thr_case", "status": "completed", "prompt": "USER_CASE_REQUEST",
			"securityContext": securityRecord,
			"items": []any{
				map[string]any{"id": "user", "kind": "user_message", "role": "user", "text": "USER_CASE_REQUEST", "summary": "NESTED_USER_FACT_SENTINEL"},
				map[string]any{"id": "assistant", "kind": "assistant_text", "role": "assistant", "text": "ASSISTANT_AMOUNT_SENTINEL_4200000"},
				map[string]any{"id": "tool", "kind": "tool_result", "role": "tool", "output": "TOOL_MAC_SENTINEL_00:11:22:33:44:55"},
			},
		}},
	}}
	service := NewService(Dependencies{Repository: repo})
	thread, err := service.Get("thr_case")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(thread)
	text := string(body)
	for _, sentinel := range []string{
		"ASSISTANT_AMOUNT_SENTINEL", "TOOL_MAC_SENTINEL", "GOAL_FACT_SENTINEL", "TODO_ACCOUNT_SENTINEL", "NESTED_USER_FACT_SENTINEL",
	} {
		if strings.Contains(text, sentinel) {
			t.Fatalf("untrusted case content reached the public thread response: sentinel=%s response=%s", sentinel, text)
		}
	}
	if strings.Contains(text, "USER_CASE_REQUEST") || thread["historyAuthority"] != CaseBoundaryOnlyHistoryAuthority {
		t.Fatalf("case projection exposed unadmitted user content: %s", text)
	}
	turns, _ := thread["turns"].([]any)
	items, _ := turns[0].(map[string]any)["items"].([]any)
	if len(items) != 0 {
		t.Fatalf("case public projection retained unadmitted items: %#v", turns)
	}
}

func TestServiceGetProjectsPendingGateIDs(t *testing.T) {
	repo := &repositoryStub{
		thread: map[string]any{
			"id": "thr_gate",
			"turns": []any{
				map[string]any{"items": []any{
					map[string]any{"id": "item_appr_pending", "kind": "approval", "status": "pending", "approvalId": "appr_live"},
					map[string]any{"id": "item_appr_done", "kind": "approval", "status": "allowed", "approvalId": "appr_done"},
					map[string]any{"id": "item_input_pending", "kind": "user_input", "status": "pending", "inputId": "input_live"},
					map[string]any{"id": "item_input_cancelled", "kind": "user_input", "status": "cancelled", "inputId": "input_old"},
					map[string]any{"id": "item_input_fallback", "kind": "user_input", "status": "pending"},
				}},
			},
		},
	}
	service := NewService(Dependencies{Repository: repo})
	thread, err := service.Get("thr_gate")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	approvals, _ := thread["pendingApprovalIds"].([]string)
	inputs, _ := thread["pendingUserInputIds"].([]string)
	if len(approvals) != 1 || approvals[0] != "appr_live" {
		t.Fatalf("pending approvals mismatch: %#v", thread["pendingApprovalIds"])
	}
	if len(inputs) != 2 || inputs[0] != "input_live" || inputs[1] != "item_input_fallback" {
		t.Fatalf("pending user inputs mismatch: %#v", thread["pendingUserInputIds"])
	}
}

func TestServiceGetUsesReplayPendingGateSnapshotWhenAvailable(t *testing.T) {
	repo := &repositoryStub{
		thread: map[string]any{
			"id": "thr_gate",
			"turns": []any{
				map[string]any{"items": []any{
					map[string]any{"id": "item_stale_appr", "kind": "approval", "status": "pending", "approvalId": "appr_stale"},
					map[string]any{"id": "item_stale_input", "kind": "user_input", "status": "pending", "inputId": "input_stale"},
				}},
			},
		},
	}
	service := NewService(Dependencies{
		Repository: repo,
		PendingGateSnapshot: func(string) ([]string, []string, error) {
			return []string{"appr_replay_live"}, []string{}, nil
		},
	})
	thread, err := service.Get("thr_gate")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	approvals, _ := thread["pendingApprovalIds"].([]string)
	inputs, _ := thread["pendingUserInputIds"].([]string)
	if len(approvals) != 1 || approvals[0] != "appr_replay_live" {
		t.Fatalf("replay approvals mismatch: %#v", thread["pendingApprovalIds"])
	}
	if len(inputs) != 0 {
		t.Fatalf("replay inputs mismatch: %#v", thread["pendingUserInputIds"])
	}
}

func TestServicePatchDeleteForkRecordEvents(t *testing.T) {
	at := time.Date(2026, 7, 12, 5, 0, 0, 0, time.UTC)
	current := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: at,
	})
	epochState, err := contextepochapp.BootstrapState("thr_1", 1, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at)
	if err != nil {
		t.Fatal(err)
	}
	repo := &repositoryStub{thread: map[string]any{
		"id": "thr_1", "title": "needle thr_1", "workspace": "/workspace", "status": "idle",
		"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(epochState),
		"turns": []any{map[string]any{
			"id": "turn_1", "threadId": "thr_1", "status": "completed", "securityContext": turnsecurityapp.PublicRecord(current), "items": []any{},
		}},
	}}
	service := newMutationService(repo)
	if _, err := service.Patch(context.Background(), "thr_1", map[string]any{"title": "Updated"}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if _, err := service.Patch(context.Background(), "thr_1", map[string]any{"executionPolicyVersion": float64(2)}); !errors.Is(err, ErrExecutionPolicyVersionRuntimeOwned) {
		t.Fatalf("public marker patch should be rejected: %v", err)
	}
	if _, err := service.Patch(context.Background(), "thr_1", map[string]any{"status": "idle"}); !errors.Is(err, ErrThreadStatusRuntimeOwned) {
		t.Fatalf("public status patch should be rejected: %v", err)
	}
	if deleted, err := service.Delete(context.Background(), "thr_1"); err != nil || !deleted {
		t.Fatalf("delete: deleted=%v err=%v", deleted, err)
	}
	if _, err := service.Fork("thr_1", map[string]any{"title": "Fork"}); err != nil {
		t.Fatalf("fork: %v", err)
	}
	if len(repo.events) != 3 {
		t.Fatalf("expected patch/delete/fork events, got %#v", repo.events)
	}
	for _, event := range repo.events {
		if event["title"] != nil {
			t.Fatalf("thread lifecycle event exposed canonical title: %#v", event)
		}
	}
}

func TestServiceGoalTodosAndRewindRecordEvents(t *testing.T) {
	at := time.Date(2026, 7, 12, 6, 0, 0, 0, time.UTC)
	current := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_1", WorkspaceRealPath: "/workspace", ContextEpoch: 1, IssuedAt: at,
	})
	epochState, err := contextepochapp.BootstrapState("thr_1", 1, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at)
	if err != nil {
		t.Fatal(err)
	}
	repo := &repositoryStub{
		goal: map[string]any{"status": "active"}, todos: map[string]any{"items": []any{}},
		thread: map[string]any{
			"id": "thr_1", "workspace": "/workspace", "status": "idle", "securityState": turnsecurityapp.PublicRecord(current),
			"contextEpochState": contextepochapp.PublicState(epochState), "turns": []any{map[string]any{
				"id": "turn_1", "threadId": "thr_1", "status": "completed", "securityContext": turnsecurityapp.PublicRecord(current), "items": []any{},
			}},
		},
	}
	service := newMutationService(repo)
	if goal, err := service.GetGoal("thr_1"); err != nil || goal["status"] != "active" {
		t.Fatalf("get goal: goal=%#v err=%v", goal, err)
	}
	if _, err := service.SetGoal("thr_1", map[string]any{"objective": "Ship"}); err != nil {
		t.Fatalf("set goal: %v", err)
	}
	if cleared, err := service.ClearGoal("thr_1"); err != nil || !cleared {
		t.Fatalf("clear goal: cleared=%v err=%v", cleared, err)
	}
	if todos, err := service.GetTodos("thr_1"); err != nil || todos == nil {
		t.Fatalf("get todos: todos=%#v err=%v", todos, err)
	}
	if _, err := service.SetTodos("thr_1", []any{map[string]any{"content": "one"}}); err != nil {
		t.Fatalf("set todos: %v", err)
	}
	if cleared, err := service.ClearTodos("thr_1"); err != nil || !cleared {
		t.Fatalf("clear todos: cleared=%v err=%v", cleared, err)
	}
	if rewind, err := service.Rewind(context.Background(), "thr_1", "turn_1"); err != nil || rewind["turnId"] != "turn_1" {
		t.Fatalf("rewind: response=%#v err=%v", rewind, err)
	}
	kinds := []string{}
	for _, event := range repo.events {
		kinds = append(kinds, event["kind"].(string))
	}
	want := []string{"goal_updated", "goal_cleared", "todos_updated", "todos_cleared", "thread_rewound"}
	if len(kinds) != len(want) {
		t.Fatalf("event kinds length mismatch: got %#v want %#v", kinds, want)
	}
	for index := range want {
		if kinds[index] != want[index] {
			t.Fatalf("event kinds mismatch: got %#v want %#v", kinds, want)
		}
	}
}

func TestServiceCaseGoalAndTodoSubroutesDoNotExposeFacts(t *testing.T) {
	repo := &repositoryStub{
		thread: map[string]any{
			"id": "thr_case", "title": "Case", "historyAuthority": CaseBoundaryOnlyHistoryAuthority, "turns": []any{},
		},
		goal:  map[string]any{"objective": "GOAL_MAC_SENTINEL_00:11:22:33:44:55"},
		todos: map[string]any{"items": []any{map[string]any{"content": "TODO_QUOTE_SENTINEL_7654321"}}},
	}
	service := NewService(Dependencies{Repository: repo})
	if goal, err := service.GetGoal("thr_case"); err != nil || goal != nil {
		t.Fatalf("case goal subroute exposed raw content: goal=%#v err=%v", goal, err)
	}
	if todos, err := service.GetTodos("thr_case"); err != nil || todos != nil {
		t.Fatalf("case todos subroute exposed raw content: todos=%#v err=%v", todos, err)
	}
	if _, err := service.SetGoal("thr_case", map[string]any{"objective": "new"}); !errors.Is(err, ErrCaseControlProjectionRestricted) {
		t.Fatalf("case goal mutation was not blocked: %v", err)
	}
	if _, err := service.SetTodos("thr_case", []any{map[string]any{"content": "new"}}); !errors.Is(err, ErrCaseControlProjectionRestricted) {
		t.Fatalf("case todo mutation was not blocked: %v", err)
	}
	if cleared, err := service.ClearGoal("thr_case"); !errors.Is(err, ErrCaseControlProjectionRestricted) || cleared {
		t.Fatalf("case goal clear was not blocked: cleared=%t err=%v", cleared, err)
	}
	if cleared, err := service.ClearTodos("thr_case"); !errors.Is(err, ErrCaseControlProjectionRestricted) || cleared {
		t.Fatalf("case todo clear was not blocked: cleared=%t err=%v", cleared, err)
	}
}

func TestServiceCaseTodoSubrouteReturnsOnlyTerminalAuditProjection(t *testing.T) {
	repo := &repositoryStub{
		thread: map[string]any{
			"id": "thr_case", "title": "Case", "historyAuthority": CaseBoundaryOnlyHistoryAuthority,
			"updatedAt": "2026-07-26T08:00:00Z", "turns": []any{},
		},
		todos: map[string]any{"items": []any{
			map[string]any{
				"id": "todo_6222021234567890", "content": "核验账号 6222021234567890",
				"status": "failed", "statusReasonCode": "tool_failed",
			},
			map[string]any{
				"id": "todo_pending", "content": "CASE_TODO_FACT_SENTINEL_7654321",
				"status": "pending",
			},
		}},
	}
	service := NewService(Dependencies{Repository: repo})
	todos, err := service.GetTodos("thr_case")
	body, _ := json.Marshal(todos)
	items := listAny(todos["items"])
	if err != nil || len(items) != 1 || items[0].(map[string]any)["status"] != "failed" ||
		strings.Contains(string(body), "6222021234567890") ||
		strings.Contains(string(body), "CASE_TODO_FACT_SENTINEL_7654321") {
		t.Fatalf("case Todo subroute did not return its bounded audit projection: body=%s err=%v", body, err)
	}
}

func TestServiceCompactRecordsLifecycleEvents(t *testing.T) {
	at := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	current := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_1", TurnID: "turn_4", WorkspaceRealPath: "/workspace",
		ContextEpoch: 1, IssuedAt: at.Add(4 * time.Second),
	})
	epochState, err := contextepochapp.BootstrapState(
		current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]any, 0, 4)
	for index := 1; index <= 4; index++ {
		turnID := "turn_" + string(rune('0'+index))
		frozen := threadServiceGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
			ThreadID: current.ThreadID, TurnID: turnID, WorkspaceRealPath: current.WorkspaceRealPath,
			ContextEpoch: current.ContextEpoch, IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		turns = append(turns, map[string]any{
			"id": turnID, "threadId": current.ThreadID, "status": "completed", "securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{"id": "item_" + turnID, "kind": "user_message", "text": "compact source"}},
		})
	}
	repo := &repositoryStub{thread: map[string]any{
		"id": current.ThreadID, "workspace": current.WorkspaceRealPath, "status": "idle", "turns": turns,
		"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(epochState),
	}}
	var transition *compactionTransitionStub
	service := NewService(Dependencies{
		Repository:      repo,
		WorkspaceReader: mutationWorkspaceReaderStub{},
		BeginTransition: func(_ context.Context, target domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
			transition = &compactionTransitionStub{target: target}
			return transition, nil
		},
	})
	response, err := service.Compact(context.Background(), "thr_1", "")
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if response["eventSeq"] != float64(2) || response["replacedTokens"].(float64) <= 0 {
		t.Fatalf("compact response mismatch: %#v", response)
	}
	if transition == nil || !transition.prepared || !transition.committed || transition.aborted || repo.compactionCommits != 1 {
		t.Fatalf("compaction did not commit through the transition: transition=%#v commits=%d", transition, repo.compactionCommits)
	}
	if len(repo.events) != 2 || repo.events[0]["kind"] != "compaction_started" || repo.events[1]["kind"] != "compaction_completed" ||
		repo.events[0]["auto"] != false || repo.events[1]["auto"] != false {
		t.Fatalf("compact events mismatch: %#v", repo.events)
	}
	manualTurn, _ := listAny(repo.thread["turns"])[0].(map[string]any)
	manualItem, _ := listAny(manualTurn["items"])[0].(map[string]any)
	if manualItem["schemaVersion"] != float64(3) || manualItem["taskContinuation"] != nil || manualItem["auto"] != false {
		t.Fatalf("manual compaction must remain V3 auto=false without continuation: %#v", manualItem)
	}
}

func threadServiceGeneralContextV2(t *testing.T, input domainsecurity.TurnSecurityContextInput) domainsecurity.TurnSecurityContext {
	t.Helper()
	observer := mutationWorkspaceReaderStub{}
	securityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(),
		Authority: turnsecurityapp.WorkspaceSecurityAuthority{Identity: testIdentityAuthority(),
			Observer: observer, RiskAuthority: &mutationServiceRiskAuthority{mutationRiskAuthority: newMutationRiskAuthority()},
		},
		Thread: map[string]any{}, ThreadID: input.ThreadID, TurnID: input.TurnID, Workspace: input.WorkspaceRealPath,
		Principal: testIdentityPrincipal(), IssuedAt: input.IssuedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func TestCaseCompactionPreservesAuthorityUntilTrustedArchiveExists(t *testing.T) {
	at := time.Date(2026, 7, 12, 3, 0, 0, 0, time.UTC)
	current := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr_case_compaction", TurnID: "turn_case_4", WorkspaceRealPath: "/cases/a", CaseID: "case-a",
		CaseBindingHash: domainsecurity.SHA256Hex([]byte("binding-a")), DatasetSnapshotID: "snapshot-a",
		SourceManifestHash: domainsecurity.SHA256Hex([]byte("manifest-a")), ContextEpoch: 4, IssuedAt: at.Add(4 * time.Second),
	})
	epochState, err := contextepochapp.BootstrapState(
		current.ThreadID, current.ContextEpoch, []domaincontextepoch.SourceEntry{contextepochapp.SecurityBindingEntry(current)}, at,
	)
	if err != nil {
		t.Fatal(err)
	}
	turns := make([]any, 0, 4)
	for index := 1; index <= 4; index++ {
		turnID := "turn_case_" + string(rune('0'+index))
		frozen := domainsecurity.NewTurnSecurityContext(domainsecurity.TurnSecurityContextInput{
			ThreadID: current.ThreadID, TurnID: turnID, WorkspaceRealPath: current.WorkspaceRealPath, CaseID: current.CaseID,
			CaseBindingHash: current.CaseBindingHash, DatasetSnapshotID: current.DatasetSnapshotID,
			SourceManifestHash: current.SourceManifestHash, ContextEpoch: current.ContextEpoch, IssuedAt: at.Add(time.Duration(index) * time.Second),
		})
		turns = append(turns, map[string]any{
			"id": turnID, "threadId": current.ThreadID, "status": "completed", "securityContext": turnsecurityapp.PublicRecord(frozen),
			"items": []any{map[string]any{"id": "user_" + turnID, "kind": "user_message", "text": "preserve case history"}},
		})
	}
	repo := &repositoryStub{thread: map[string]any{
		"id": current.ThreadID, "workspace": current.WorkspaceRealPath, "status": "idle", "turns": turns,
		"securityState": turnsecurityapp.PublicRecord(current), "contextEpochState": contextepochapp.PublicState(epochState),
	}}
	before, _ := json.Marshal(repo.thread)
	transitionStarted := false
	service := NewService(Dependencies{
		Repository: repo,
		BeginTransition: func(context.Context, domainsecurity.TurnSecurityContext) (CompactionTransition, error) {
			transitionStarted = true
			return nil, errors.New("must not start")
		},
	})
	response, err := service.Compact(context.Background(), current.ThreadID, "manual")
	if !errors.Is(err, ErrCaseCompactionRequiresTrustedArchive) || response != nil {
		t.Fatalf("case compaction did not fail closed: response=%#v err=%v", response, err)
	}
	after, _ := json.Marshal(repo.thread)
	if string(after) != string(before) || repo.compactionCommits != 0 || len(repo.events) != 0 || transitionStarted {
		t.Fatalf("rejected case compaction changed authority: commits=%d events=%#v transition=%t", repo.compactionCommits, repo.events, transitionStarted)
	}
	state, ok, stateErr := contextepochapp.StateFromThread(repo.thread)
	if stateErr != nil || !ok || state.AcceptedSnapshot.Epoch != current.ContextEpoch {
		t.Fatalf("rejected case compaction advanced epoch: state=%#v ok=%t err=%v", state, ok, stateErr)
	}
}

func TestServicePropagatesRepositoryErrors(t *testing.T) {
	expected := errors.New("disk failed")
	service := NewService(Dependencies{Repository: &repositoryStub{err: expected}})
	if _, err := service.List(ListInput{}); !errors.Is(err, expected) {
		t.Fatalf("expected list error, got %v", err)
	}
}
