package goal

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	domaintoolresult "analytix.local/runtime-go/internal/domain/toolresult"
	toolidentitytest "analytix.local/runtime-go/internal/testsupport/toolidentity"
)

func TestToolServiceCompleteStepVerifiesHostEvidenceAndAdvancesTodo(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{
		"id":             "goal_1",
		"threadId":       "thread-1",
		"status":         "active",
		"evidenceLedger": []any{},
	}
	store.todos = map[string]any{
		"threadId": "thread-1",
		"items": []any{
			map[string]any{"id": "test", "content": "Run tests", "status": "in_progress"},
			map[string]any{"id": "ship", "content": "Ship it", "status": "pending"},
		},
	}
	store.thread = settledBashEvidenceThread(t, "go test ./...")
	var goalEvents int
	var todoEvents int
	service := ToolService{
		Store: store,
		RecordGoalUpdated: func(string, map[string]any) {
			goalEvents++
		},
		RecordTodosUpdated: func(string, map[string]any) {
			todoEvents++
		},
	}

	result := service.CompleteStep(PendingToolContext{
		ThreadID:   "thread-1",
		TurnID:     "turn-1",
		ToolCallID: "call-1",
	}, map[string]any{
		"step_index": float64(1),
		"evidence": []any{
			map[string]any{
				"kind":    "verification",
				"summary": "tests pass",
				"command": "go test ./...",
			},
		},
	})
	if result.IsError {
		t.Fatalf("complete_step should pass: %#v", result.Output)
	}
	out := result.Output.(map[string]any)
	if out["hostVerified"] != true || out["todoAdvanced"] != true {
		t.Fatalf("expected verified evidence and todo advance: %#v", out)
	}
	if got := int(floatFromAny(out["evidenceLedgerCount"])); got != 1 {
		t.Fatalf("ledger count mismatch: %d", got)
	}
	items := listAny(store.todos["items"])
	if stringField(items[0].(map[string]any), "status") != "completed" || stringField(items[1].(map[string]any), "status") != "in_progress" {
		t.Fatalf("todo state did not advance: %#v", items)
	}
	if goalEvents != 1 || todoEvents != 1 {
		t.Fatalf("event callbacks mismatch goal=%d todos=%d", goalEvents, todoEvents)
	}
}

func TestPrepareCompleteStepStepAndIndexResolveSameTodoEffect(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{"id": "goal_1", "threadId": "thread-1", "status": "active", "evidenceLedger": []any{}}
	store.todos = map[string]any{"threadId": "thread-1", "items": []any{
		map[string]any{"id": "todo_1", "content": "Run tests", "status": "in_progress"},
	}}
	service := ToolService{Store: store}
	pending := PendingToolContext{ThreadID: "thread-1", TurnID: "turn-1", ToolCallID: "call-1"}
	byIndex, err := service.PrepareCompleteStep(pending, map[string]any{"step_index": float64(1), "evidence": []any{"checked"}})
	if err != nil {
		t.Fatal(err)
	}
	byContent, err := service.PrepareCompleteStep(pending, map[string]any{"step": "Run tests", "evidence": []any{"checked"}})
	if err != nil {
		t.Fatal(err)
	}
	indexProjection, _ := json.Marshal(byIndex.SemanticProjectionV1())
	contentProjection, _ := json.Marshal(byContent.SemanticProjectionV1())
	if string(indexProjection) != string(contentProjection) || byIndex.Step != "Run tests" || byContent.Step != "Run tests" {
		t.Fatalf("todo selector aliases diverged: index=%s content=%s steps=%q/%q", indexProjection, contentProjection, byIndex.Step, byContent.Step)
	}
}

func TestFailedAndCanceledTodosCannotCloseGoalOrCompleteStep(t *testing.T) {
	for _, status := range []string{"failed", "canceled"} {
		reason := "tool_failed"
		if status == "canceled" {
			reason = "user_canceled"
		}
		store := newFakeGoalStore()
		store.goal = map[string]any{
			"id": "goal_1", "threadId": "thread-1", "status": "active", "evidenceLedger": []any{"verified"},
		}
		store.todos = map[string]any{"threadId": "thread-1", "items": []any{
			map[string]any{"id": "todo_1", "content": "Run tests", "status": status, "statusReasonCode": reason},
		}}
		service := ToolService{Store: store}
		pending := PendingToolContext{ThreadID: "thread-1", TurnID: "turn-1", ToolCallID: "call-1"}
		if _, err := service.PrepareCompleteStep(pending, map[string]any{"step": "Run tests", "evidence": []any{"checked"}}); err == nil {
			t.Fatalf("%s todo was completed without explicit retry", status)
		}
		result := service.UpdateGoal(pending, map[string]any{"status": "complete"})
		if !result.IsError || stringField(result.Output.(map[string]any), "code") != "goal_todos_incomplete" {
			t.Fatalf("%s todo allowed goal closure: %#v", status, result.Output)
		}
	}
}

func TestExecutePreparedCompleteStepRejectsStateDriftBeforeAnyWrite(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{"id": "goal_1", "threadId": "thread-1", "status": "active", "evidenceLedger": []any{}}
	store.todos = map[string]any{"threadId": "thread-1", "items": []any{
		map[string]any{"id": "todo_1", "content": "Run tests", "status": "in_progress"},
	}}
	service := ToolService{Store: store}
	pending := PendingToolContext{ThreadID: "thread-1", TurnID: "turn-1", ToolCallID: "call-1"}
	prepared, err := service.PrepareCompleteStep(pending, map[string]any{"step": "Run tests", "evidence": []any{"checked"}})
	if err != nil {
		t.Fatal(err)
	}
	store.todos["items"].([]any)[0].(map[string]any)["note"] = "changed concurrently"
	result := service.ExecutePreparedCompleteStep(pending, prepared)
	if !result.IsError || stringField(result.Output.(map[string]any), "code") != "goal_prepared_state_stale" {
		t.Fatalf("stale prepared step was not rejected: %#v", result.Output)
	}
	if len(listAny(store.goal["evidenceLedger"])) != 0 {
		t.Fatalf("stale prepared step wrote evidence: %#v", store.goal)
	}
}

func TestGoalHasSuccessfulCommandRejectsUnsettledOrMismatchedClosedProjection(t *testing.T) {
	store := newFakeGoalStore()
	store.thread = settledBashEvidenceThread(t, "go test ./...")
	service := ToolService{Store: store}
	if !service.GoalHasSuccessfulCommand("thread-1", "go test ./...") {
		t.Fatal("matching host-private grant and closed settled result should verify")
	}
	if service.GoalHasSuccessfulCommand("thread-1", "go test ./internal/domain/...") {
		t.Fatal("a different cited command must not match a settled grant")
	}

	turn := listAny(store.thread["turns"])[0].(map[string]any)
	result := listAny(turn["items"])[1].(map[string]any)
	result["executionGrantId"] = domainsecurity.SHA256Hex([]byte("fake-grant"))
	if service.GoalHasSuccessfulCommand("thread-1", "go test ./...") {
		t.Fatal("a mismatched result/grant binding must fail closed")
	}
}

func settledBashEvidenceThread(t *testing.T, command string) map[string]any {
	t.Helper()
	issuedAt := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	finishedAt := issuedAt.Add(time.Second)
	observationDigest := domainsecurity.SHA256Hex([]byte("risk-observation"))
	publicationPolicy, err := domainsecurity.NewTurnPublicationPolicyV1(domainsecurity.TurnPublicationPolicyInputV1{
		ThreadRiskPolicyDigest: domainsecurity.SHA256Hex([]byte("thread-risk-policy")), RiskClass: domainsecurity.RiskClassGeneral,
		Disposition: domainsecurity.PublicationDispositionGeneralOutput, CaseBindingState: domainsecurity.CaseBindingStateMissing,
		BindingObservationDigest: observationDigest, BlockerCode: domainsecurity.PublicationBlockerNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	riskBinding := domainsecurity.RiskAuthorityBindingV1{
		SchemaVersion: domainsecurity.RiskAuthorityBindingSchemaVersion, Purpose: domainsecurity.RiskAuthorityBindingPurpose,
		State: domainsecurity.RiskAuthorityBindingStateWitnessed, IndexDigest: domainsecurity.SHA256Hex([]byte("risk-index")), Generation: 1,
		CheckpointDigest: domainsecurity.SHA256Hex([]byte("risk-checkpoint")), ObservationDigest: observationDigest,
	}
	securityContext, err := domainsecurity.NewTurnSecurityContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thread-1", TurnID: "turn-1", WorkspaceRealPath: "/workspace", TenantID: domainsecurity.LocalTenantID,
		UserID: domainsecurity.LocalUserID, CaseID: domainsecurity.UnboundCaseID,
		CaseBindingHash: domainsecurity.UnboundCaseBindingHash("/workspace"), DatasetSnapshotID: domainsecurity.NoDatasetSnapshotID,
		SourceManifestHash: domainsecurity.EmptySourceManifestHash, ContextEpoch: 1, IssuedAt: issuedAt,
		PublicationPolicy: publicationPolicy, RiskAuthorityBinding: riskBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	toolCallID := toolidentitytest.MustHostToolCallIDV1("goal-bash-evidence-" + command)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "test-provider", ServerIdentity: "host:builtin", ToolName: "bash", ToolCallID: toolCallID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("bash-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("bash-scope")), ReadOnly: false, ApprovalState: "not_required",
		IssuedAt: issuedAt, ExpiresAt: issuedAt.Add(time.Minute),
	})
	grantRecord := jsonRecordForGoalTest(t, grant)
	securityRecord := jsonRecordForGoalTest(t, securityContext)
	projection := domaintoolresult.PublicToolResultProjectionV1{
		SchemaVersion: domaintoolresult.PublicProjectionSchemaVersion, ProjectionKind: domaintoolresult.ProjectionHostStatus,
		Disclosure: domaintoolresult.MetadataOnlyDisclosure, MessageKey: "tool_completed", Status: "completed", Code: "tool_completed",
		PrivatePayloadWithheld: true, FactAnswerAllowed: false, EvidenceAuthority: false,
	}
	callItem := map[string]any{
		"id": "item-call", "threadId": "thread-1", "turnId": "turn-1", "kind": "tool_call", "role": "tool", "status": "completed",
		"createdAt": issuedAt.Format(time.RFC3339Nano), "toolName": "bash", "callId": toolCallID,
		"arguments": domaintoolcall.PublicToolCallArgumentsProjectionRecordV1(), "contextDigest": securityContext.ContextDigest,
		"contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": grant.GrantID, "executionGrant": grantRecord,
	}
	resultItem := map[string]any{
		"id": "item-result", "threadId": "thread-1", "turnId": "turn-1", "kind": "tool_result", "role": "tool", "status": "completed",
		"createdAt": issuedAt.Format(time.RFC3339Nano), "finishedAt": finishedAt.Format(time.RFC3339Nano),
		"toolName": "bash", "callId": toolCallID, "output": domaintoolresult.PublicToolResultProjectionRecordV1(projection), "isError": false,
		"contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch), "executionGrantId": grant.GrantID,
	}
	return map[string]any{
		"id":    "thread-1",
		"turns": []any{map[string]any{"id": "turn-1", "securityContext": securityRecord, "items": []any{callItem, resultItem}}},
	}
}

func jsonRecordForGoalTest(t *testing.T, value any) map[string]any {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	record := map[string]any{}
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestToolServiceStrictCompletionSelfCheckGate(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{
		"id":               "goal_1",
		"threadId":         "thread-1",
		"status":           "active",
		"strictCompletion": true,
		"evidenceLedger": []any{
			map[string]any{"id": "goal_ev_1", "step": "Done", "evidence": []any{"manual"}},
		},
	}
	service := ToolService{Store: store}

	first := service.UpdateGoal(PendingToolContext{ThreadID: "thread-1", TurnID: "turn-1"}, map[string]any{"status": "complete"})
	if first.IsError {
		t.Fatalf("first strict completion request should ask for self-check: %#v", first.Output)
	}
	firstOut := first.Output.(map[string]any)
	if firstOut["completionAudit"] == nil || store.goal["selfCheckRequired"] != true {
		t.Fatalf("self-check audit missing: %#v goal=%#v", firstOut, store.goal)
	}

	second := service.UpdateGoal(PendingToolContext{ThreadID: "thread-1", TurnID: "turn-2"}, map[string]any{"status": "complete"})
	if !second.IsError || stringField(second.Output.(map[string]any), "code") != "goal_self_check_required" {
		t.Fatalf("second strict completion should be blocked: %#v", second.Output)
	}
}

func TestPrepareUpdateGoalUsesDistinctStrictCompletionOwnerPhases(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{
		"id": "goal_1", "threadId": "thread-1", "status": "active", "strictCompletion": true,
		"evidenceLedger": []any{map[string]any{"id": "goal_ev_1"}},
	}
	service := ToolService{Store: store}
	pending := PendingToolContext{ThreadID: "thread-1", TurnID: "turn-1"}
	request, err := service.PrepareUpdateGoal(pending, map[string]any{"status": "complete"})
	if err != nil || request.Phase != "request_self_check" {
		t.Fatalf("request phase = %#v err=%v", request, err)
	}
	if result := service.ExecutePreparedUpdateGoal(pending, request); result.IsError {
		t.Fatalf("request self-check failed: %#v", result.Output)
	}
	store.goal["selfCheckCompleted"] = true
	finalize, err := service.PrepareUpdateGoal(pending, map[string]any{"status": "complete"})
	if err != nil || finalize.Phase != "finalize_complete" {
		t.Fatalf("finalize phase = %#v err=%v", finalize, err)
	}
	firstBody, _ := json.Marshal(request.SemanticProjectionV1())
	finalBody, _ := json.Marshal(finalize.SemanticProjectionV1())
	if string(firstBody) == string(finalBody) {
		t.Fatalf("strict completion phases shared identity: %s", firstBody)
	}
}

func TestPrepareUpdateGoalFailsClosedWhenTodoReadFails(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{"id": "goal_1", "threadId": "thread-1", "status": "active", "evidenceLedger": []any{map[string]any{"id": "goal_ev_1"}}}
	store.todosErr = errors.New("todo store unavailable")
	_, err := (ToolService{Store: store}).PrepareUpdateGoal(PendingToolContext{ThreadID: "thread-1"}, map[string]any{"status": "complete"})
	if err == nil {
		t.Fatal("todo read failure minted a finalize_complete owner phase")
	}
}

func TestToolServiceBlockedGoalRequiresRepeatedSameReason(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{
		"id":             "goal_1",
		"threadId":       "thread-1",
		"status":         "active",
		"evidenceLedger": []any{},
	}
	service := ToolService{Store: store}

	for index, turnID := range []string{"turn-1", "turn-2", "turn-3"} {
		result := service.UpdateGoal(PendingToolContext{ThreadID: "thread-1", TurnID: turnID}, map[string]any{
			"status": "blocked",
			"reason": "Waiting on external API",
		})
		if result.IsError {
			t.Fatalf("blocked update %d failed: %#v", index+1, result.Output)
		}
	}
	if stringField(store.goal, "status") != "blocked" || int(floatFromAny(store.goal["blockedCount"])) != 3 {
		t.Fatalf("blocked FSM did not reach terminal blocked: %#v", store.goal)
	}
}

func TestPrepareUpdateGoalCannotResurrectTerminalGoal(t *testing.T) {
	for _, status := range []string{"complete", "blocked"} {
		store := newFakeGoalStore()
		store.goal = map[string]any{"id": "goal_1", "threadId": "thread-1", "status": status, "evidenceLedger": []any{map[string]any{"id": "goal_ev_1"}}}
		_, err := (ToolService{Store: store}).PrepareUpdateGoal(
			PendingToolContext{ThreadID: "thread-1", TurnID: "turn-1"},
			map[string]any{"status": "blocked", "reason": "waiting"},
		)
		if err == nil {
			t.Fatalf("terminal goal %q was accepted for blocked transition", status)
		}
	}
}

func TestToolServiceTodoOpsDoesNotCreateGoalEvidence(t *testing.T) {
	store := newFakeGoalStore()
	store.goal = map[string]any{
		"id":             "goal_1",
		"threadId":       "thread-1",
		"status":         "active",
		"evidenceLedger": []any{},
	}
	store.todos = map[string]any{
		"threadId": "thread-1",
		"items": []any{
			map[string]any{"id": "one", "content": "One", "status": "in_progress"},
		},
	}
	var todoEvents int
	service := ToolService{
		Store: store,
		RecordTodosUpdated: func(string, map[string]any) {
			todoEvents++
		},
	}

	result := service.TodoOps("thread-1", map[string]any{
		"ops": []any{
			map[string]any{"op": "done", "id": "one"},
		},
	})
	if result.IsError {
		t.Fatalf("todo_ops should pass: %#v", result.Output)
	}
	items := listAny(store.todos["items"])
	if stringField(items[0].(map[string]any), "status") != "completed" {
		t.Fatalf("todo should be completed: %#v", items)
	}
	if todoEvents != 1 {
		t.Fatalf("todo event count = %d", todoEvents)
	}
	goalResult := service.UpdateGoal(PendingToolContext{ThreadID: "thread-1", TurnID: "turn-1"}, map[string]any{"status": "complete"})
	if !goalResult.IsError || stringField(goalResult.Output.(map[string]any), "code") != "goal_evidence_required" {
		t.Fatalf("goal completion should still require evidence: %#v", goalResult.Output)
	}
}

type fakeGoalStore struct {
	goal     map[string]any
	todos    map[string]any
	thread   map[string]any
	todosErr error
}

func newFakeGoalStore() *fakeGoalStore {
	return &fakeGoalStore{
		thread: map[string]any{"id": "thread-1", "turns": []any{}},
	}
}

func (s *fakeGoalStore) GetGoal(string) (map[string]any, error) {
	return cloneMapForTest(s.goal), nil
}

func (s *fakeGoalStore) SetGoal(threadID string, patch map[string]any) (map[string]any, error) {
	if s.goal == nil {
		s.goal = map[string]any{"id": "goal_1", "threadId": threadID, "status": "active", "evidenceLedger": []any{}}
	}
	for key, value := range patch {
		s.goal[key] = cloneValueForTest(value)
	}
	return cloneMapForTest(s.goal), nil
}

func (s *fakeGoalStore) AppendGoalEvidence(threadID string, entry map[string]any) (map[string]any, map[string]any, error) {
	if s.goal == nil {
		s.goal = map[string]any{"id": "goal_1", "threadId": threadID, "status": "active", "evidenceLedger": []any{}}
	}
	ledger := append([]any{}, listAny(s.goal["evidenceLedger"])...)
	record := cloneMapForTest(entry)
	record["id"] = "goal_ev_1"
	ledger = append(ledger, record)
	s.goal["evidenceLedger"] = ledger
	return cloneMapForTest(s.goal), cloneMapForTest(record), nil
}

func (s *fakeGoalStore) GetTodos(string) (map[string]any, error) {
	if s.todosErr != nil {
		return nil, s.todosErr
	}
	return cloneMapForTest(s.todos), nil
}

func (s *fakeGoalStore) SetTodos(threadID string, items []any) (map[string]any, error) {
	s.todos = map[string]any{"threadId": threadID, "items": cloneSliceForTest(items)}
	return cloneMapForTest(s.todos), nil
}

func (s *fakeGoalStore) GetThread(string) (map[string]any, error) {
	return cloneMapForTest(s.thread), nil
}

func (s *fakeGoalStore) CommitPreparedCompleteStep(request PreparedCompleteStepCommit) (PreparedCompleteStepCommitResult, error) {
	if SemanticStateHashV1(s.goal) != request.ExpectedGoalHash || SemanticStateHashV1(s.todos) != request.ExpectedTodoHash {
		return PreparedCompleteStepCommitResult{}, ErrPreparedStateStale
	}
	nextGoal := cloneMapForTest(s.goal)
	entry := cloneMapForTest(request.Entry)
	entry["id"] = "goal_ev_1"
	nextGoal["evidenceLedger"] = append(cloneSliceForTest(listAny(nextGoal["evidenceLedger"])), entry)
	for key, value := range request.GoalPatch {
		nextGoal[key] = cloneValueForTest(value)
	}
	nextTodos := cloneMapForTest(s.todos)
	if request.NextTodoItems != nil {
		nextTodos = map[string]any{"threadId": request.ThreadID, "items": cloneSliceForTest(request.NextTodoItems)}
	}
	s.goal, s.todos = nextGoal, nextTodos
	return PreparedCompleteStepCommitResult{Goal: cloneMapForTest(nextGoal), Entry: cloneMapForTest(entry), Todos: cloneMapForTest(nextTodos)}, nil
}

func (s *fakeGoalStore) CommitPreparedUpdateGoal(request PreparedUpdateGoalCommit) (map[string]any, error) {
	if SemanticStateHashV1(s.goal) != request.ExpectedGoalHash ||
		(request.ExpectedTodoHash != "" && SemanticStateHashV1(s.todos) != request.ExpectedTodoHash) {
		return nil, ErrPreparedStateStale
	}
	next := cloneMapForTest(s.goal)
	for key, value := range request.Patch {
		next[key] = cloneValueForTest(value)
	}
	s.goal = next
	return cloneMapForTest(next), nil
}

func (s *fakeGoalStore) CommitPreparedTodoOps(request PreparedTodoOpsCommit) (map[string]any, error) {
	if SemanticStateHashV1(s.todos) != request.ExpectedTodoHash {
		return nil, ErrPreparedStateStale
	}
	s.todos = map[string]any{"threadId": request.ThreadID, "items": cloneSliceForTest(request.NextTodoItems)}
	return cloneMapForTest(s.todos), nil
}

func cloneMapForTest(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneValueForTest(item)
	}
	return out
}

func cloneSliceForTest(values []any) []any {
	out := make([]any, 0, len(values))
	for _, item := range values {
		out = append(out, cloneValueForTest(item))
	}
	return out
}

func cloneValueForTest(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMapForTest(typed)
	case []any:
		return cloneSliceForTest(typed)
	default:
		return typed
	}
}
