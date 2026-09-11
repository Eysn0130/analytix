package server

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	filestore "analytix.local/runtime-go/internal/adapters/outbound/filestore"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/jobs"
	"analytix.local/runtime-go/internal/provider"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
)

func TestRuntimeTaskJobDiagnosticsUsesClosedMetadataProjection(t *testing.T) {
	const privateSentinel = "ZXQ_PRIV_7F3C9A2D_41B6"
	now := time.Date(2026, 6, 26, 5, 30, 0, 0, time.UTC)
	record := jobs.Record{
		ID:             "job-stalled",
		Kind:           "background-shell",
		ParentGoalID:   "goal-stalled",
		ParentThreadID: "thread-stalled",
		Status:         "running",
		Background:     true,
		ArtifactPath:   "/tmp/analytix/" + privateSentinel + ".log",
		Output:         "<think>" + privateSentinel + "</think>",
		Error:          privateSentinel,
		StartedAt:      now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		UpdatedAt:      now.Add(-45 * time.Second).Format(time.RFC3339Nano),
	}

	diagnostics := subagentapp.TaskJobDiagnostics(record, now, 30*time.Second)
	assertRuntimeTaskJobClosedMetadata(t, diagnostics, "job-stalled", "background-shell", "running", true, false)
	if strings.Contains(fmt.Sprint(diagnostics), privateSentinel) || strings.Contains(strings.ToLower(fmt.Sprint(diagnostics)), "<think>") {
		t.Fatalf("public task-job diagnostics reflected private output/error/reasoning bytes: %#v", diagnostics)
	}

	record.Status = "completed"
	record.FinishedAt = now.Format(time.RFC3339Nano)
	diagnostics = subagentapp.TaskJobDiagnostics(record, now, 30*time.Second)
	assertRuntimeTaskJobClosedMetadata(t, diagnostics, "job-stalled", "background-shell", "completed", true, true)
}

func assertRuntimeTaskJobClosedMetadata(t *testing.T, metadata map[string]any, id string, kind string, status string, background bool, terminal bool) {
	t.Helper()
	allowed := map[string]struct{}{
		"schemaVersion": {}, "id": {}, "kind": {}, "status": {}, "background": {}, "terminal": {},
		"outputWithheld": {}, "outputTrustStatus": {}, "factAnswerAllowed": {}, "evidenceAuthority": {},
		"canReadOutput": {}, "canContinueParent": {},
	}
	if len(metadata) != len(allowed) {
		t.Fatalf("task-job public metadata must use the exact closed root: %#v", metadata)
	}
	for key := range metadata {
		if _, ok := allowed[key]; !ok {
			t.Fatalf("task-job public metadata exposed forbidden key %q: %#v", key, metadata)
		}
	}
	if metadata["schemaVersion"] != 1 || stringDiagnostic(metadata, "id") != id ||
		stringDiagnostic(metadata, "kind") != kind || stringDiagnostic(metadata, "status") != status ||
		boolDiagnostic(metadata, "background") != background || boolDiagnostic(metadata, "terminal") != terminal ||
		metadata["outputWithheld"] != true || stringDiagnostic(metadata, "outputTrustStatus") != "untrusted_child_output" ||
		boolDiagnostic(metadata, "factAnswerAllowed") || boolDiagnostic(metadata, "evidenceAuthority") ||
		boolDiagnostic(metadata, "canReadOutput") || boolDiagnostic(metadata, "canContinueParent") {
		t.Fatalf("task-job public metadata mismatch: %#v", metadata)
	}
}

func TestRuntimeTaskJobWaitToolDefaultsToBlockingUntilBackgroundJobCompletes(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	thread, err := store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	parent := appendServerBoundJobParent(t, store, threadID, "turn_wait_default", workspace, "subagent")
	handler := &runtimeServerHandler{store: store}
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler.jobs = manager
	startRequest := jobs.StartRequest{
		ParentGoalID:     "goal_wait_default",
		ParentThreadID:   threadID,
		ParentTurnID:     "turn_wait_default",
		ParentToolItemID: parent.ItemID,
		ParentToolCallID: parent.CallID,
		Kind:             "subagent",
		Label:            "wait default",
		Status:           "running",
		Background:       true,
		SecurityBinding:  parent.Binding,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := handler.jobs.StartChildRun(startRequest)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		_, _ = handler.jobs.UpdateChildRun(record.ID, jobs.UpdateRequest{
			Status: "completed",
		})
	}()

	started := time.Now()
	output, isError := handler.executeRuntimeTaskJobWait(runtimePendingToolCall{
		ThreadID:        threadID,
		TurnID:          "turn_wait_default",
		SecurityContext: parent.Context,
		ExecutionGrant:  parent.Grant,
		Call: provider.ToolCall{
			ID:   "call_wait_default",
			Name: "wait",
		},
	}, map[string]any{"jobId": record.ID})
	elapsed := time.Since(started)
	if isError {
		t.Fatalf("wait without timeout should complete successfully: %#v", output)
	}
	if elapsed < 20*time.Millisecond {
		t.Fatalf("wait without timeout returned before the background job completed: %s %#v", elapsed, output)
	}
	jobsRaw, _ := output.(map[string]any)["jobs"].([]any)
	if len(jobsRaw) != 1 {
		t.Fatalf("wait should return one job: %#v", output)
	}
	job := jobsRaw[0].(map[string]any)
	if stringDiagnostic(job, "status") != "completed" || job["outputWithheld"] != true ||
		stringDiagnostic(job, "output") != "" || boolDiagnostic(job, "factAnswerAllowed") {
		t.Fatalf("wait should return only completed metadata for a bound job: %#v", job)
	}
}

func TestTaskJobCaseRiskSteerCannotEnterFrozenGeneralChild(t *testing.T) {
	handler, ok := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken:   DefaultRuntimeToken,
		DurableTempDir: t.TempDir(),
		DataDir:        t.TempDir(),
		Host:           "127.0.0.1",
		Port:           0,
		Insecure:       false,
	}).(*runtimeServerHandler)
	if !ok {
		t.Fatal("runtime server handler type mismatch")
	}
	configureServerGeneralExecution(t, handler)
	workspace := t.TempDir()
	parent, err := handler.store.CreateThread(map[string]any{
		"title":     "parent",
		"workspace": workspace,
		"mode":      "agent",
	}, workspace)
	if err != nil {
		t.Fatalf("create parent thread: %v", err)
	}
	child, err := handler.store.CreateThread(map[string]any{
		"title":     "child",
		"workspace": workspace,
		"mode":      "agent",
		"relation":  "child",
	}, workspace)
	if err != nil {
		t.Fatalf("create child thread: %v", err)
	}
	parentID := stringField(parent, "id")
	childID := stringField(child, "id")
	parentAuthority := appendServerBoundJobParent(t, handler.store, parentID, "turn_parent_steer", workspace, "subagent")
	childTurnID := "turn_child_steer"
	childSecurityContext, err := turnsecurityapp.FreezeWorkspace(turnsecurityapp.WorkspaceFreezeInput{
		Context: context.Background(), Authority: handler.turnSecurity, Reader: filestore.CaseBindingReader{},
		Thread: child, ThreadID: childID, TurnID: childTurnID, Workspace: workspace,
		Principal: testIdentityPrincipal(), IssuedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("freeze child steer context: %v", err)
	}
	if err := handler.store.AppendTurnToThread(childID, map[string]any{
		"id":              childTurnID,
		"threadId":        childID,
		"status":          "running",
		"mode":            "agent",
		"items":           []any{},
		"steering":        []any{},
		"securityContext": turnsecurityapp.PublicRecord(childSecurityContext),
		"createdAt":       time.Now().UTC().Format(time.RFC3339Nano),
	}, "", nil); err != nil {
		t.Fatalf("append child turn: %v", err)
	}
	startRequest := jobs.StartRequest{
		ParentGoalID:     "goal_parent_steer",
		ParentThreadID:   parentID,
		ParentTurnID:     "turn_parent_steer",
		ParentToolCallID: parentAuthority.CallID,
		ParentToolItemID: parentAuthority.ItemID,
		Kind:             "subagent",
		Label:            "steer target",
		Status:           "running",
		Background:       true,
		ChildThreadID:    childID,
		ChildTurnID:      childTurnID,
		SecurityBinding:  parentAuthority.Binding,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	record, err := handler.jobs.StartChildRun(startRequest)
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	loadedChildContext, loadErr := appturn.LoadFrozenSecurityContext(handler.store, childID, childTurnID)
	if loadErr != nil || !domainjob.SecurityBindingMatchesWorkspaceScope(record.SecurityBinding, loadedChildContext) {
		t.Fatalf("child context does not match job binding: context=%#v binding=%#v err=%v", loadedChildContext, record.SecurityBinding, loadErr)
	}
	authorization, blocker, err := handler.beginRuntimeTaskJobSteerAuthority(context.Background(), record, domainjob.SteerMessage{Text: "continue with the second check"})
	if authorization.Release != nil {
		authorization.Release()
	}
	if err != nil || blocker != "" {
		t.Fatalf("child steer authority unavailable: blocker=%q err=%v", blocker, err)
	}
	output, isError := handler.executeRuntimeTaskJobSteer(context.Background(), runtimePendingToolCall{
		ThreadID:        parentID,
		TurnID:          "turn_parent_steer",
		SecurityContext: parentAuthority.Context,
		Call: provider.ToolCall{
			ID:   "call_steer",
			Name: "steer_job",
		},
	}, map[string]any{
		"jobId": record.ID, "message": "continue with the second check",
		"sourceTurnId": "forged_turn", "sourceToolCallId": "forged_call",
	})
	if isError {
		t.Fatalf("steer_job should queue guidance: %#v", output)
	}
	response, _ := output.(map[string]any)
	if response["jobId"] != record.ID || response["outputWithheld"] != true || boolDiagnostic(response, "factAnswerAllowed") {
		t.Fatalf("steer response mismatch: %#v", output)
	}
	updatedChild, err := handler.store.GetThread(childID)
	if err != nil {
		t.Fatalf("load child thread: %v", err)
	}
	turns := listAny(updatedChild["turns"])
	latest, _ := turns[len(turns)-1].(map[string]any)
	steering := listAny(latest["steering"])
	if len(steering) != 1 {
		t.Fatalf("expected one steering entry, got %#v", steering)
	}
	entry, _ := steering[0].(map[string]any)
	if entry["text"] != "continue with the second check" || entry["jobId"] != record.ID ||
		entry["sourceTurnId"] != "turn_parent_steer" || entry["sourceToolCallId"] != "call_steer" {
		t.Fatalf("steering entry mismatch: %#v", entry)
	}
	caseSentinel := "CASE_STEER_ACCOUNT_6222021234567890123"
	blocked, blockedIsError := handler.executeRuntimeTaskJobSteer(context.Background(), runtimePendingToolCall{
		ThreadID: parentID, TurnID: "turn_parent_steer", SecurityContext: parentAuthority.Context,
		Call: provider.ToolCall{ID: "call_steer_case", Name: "steer_job"},
	}, map[string]any{"jobId": record.ID, "message": "请继续分析案件中的银行卡号 " + caseSentinel + " 与金额 100000 元"})
	if !blockedIsError {
		t.Fatalf("case-risk steer entered a frozen general child: %#v", blocked)
	}
	updatedRecord, err := handler.jobs.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	updatedChild, err = handler.store.GetThread(childID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := handler.store.LoadEventsSince(parentID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(updatedRecord.Steers) != 1 || strings.Contains(fmt.Sprint(updatedRecord), caseSentinel) ||
		strings.Contains(fmt.Sprint(updatedChild), caseSentinel) || strings.Contains(fmt.Sprint(events.Events), caseSentinel) {
		t.Fatalf("rejected case-risk steer was persisted: job=%#v child=%#v events=%#v", updatedRecord, updatedChild, events.Events)
	}
}

func TestTaskJobSteerContextDigestRaceRejectsCAS(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	workspaceRealPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateThread(map[string]any{"workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_task_job_steer_cas"
	frozen := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspaceRealPath, ContextEpoch: 1, IssuedAt: time.Now().UTC(),
	})
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "items": []any{}, "steering": []any{},
		"securityContext": turnsecurityapp.PublicRecord(frozen),
	}, "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AdmitSteeringEntryForContext(
		threadID, turnID, turnID, domainsecurity.SHA256Hex([]byte("stale-child-context")),
		map[string]any{"id": "steer_must_not_persist", "text": "STEER_CAS_SENTINEL"},
	); !errors.Is(err, errDurableContextDigestMismatch) {
		t.Fatalf("stale context digest should fail closed: %v", err)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turns := listAny(reloaded["turns"])
	latest, _ := turns[len(turns)-1].(map[string]any)
	if steering := listAny(latest["steering"]); len(steering) != 0 || strings.Contains(fmt.Sprint(reloaded), "STEER_CAS_SENTINEL") {
		t.Fatalf("stale context steer reached durable state: %#v", steering)
	}
}

func boolDiagnostic(record map[string]any, key string) bool {
	value, _ := record[key].(bool)
	return value
}

func stringDiagnostic(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func diagnosticStringListContains(record map[string]any, key string, expected string) bool {
	values, _ := record[key].([]any)
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func numericDiagnostic(record map[string]any, key string) float64 {
	switch value := record[key].(type) {
	case float64:
		return value
	case int64:
		return float64(value)
	case int:
		return float64(value)
	default:
		return 0
	}
}
