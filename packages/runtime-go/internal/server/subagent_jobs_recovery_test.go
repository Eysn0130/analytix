package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	controlapp "analytix.local/runtime-go/internal/app/control"
	appmodel "analytix.local/runtime-go/internal/app/model"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	threadapp "analytix.local/runtime-go/internal/app/thread"
	appturn "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	appusage "analytix.local/runtime-go/internal/app/usage"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	jobs "analytix.local/runtime-go/internal/jobs"
	jobsecuritytest "analytix.local/runtime-go/internal/testsupport/jobsecurity"
	"analytix.local/runtime-go/internal/testsupport/workspacetest"
)

type backgroundAutoContinueProvider struct {
	mu       sync.Mutex
	requests []domainmodel.Request
}

type blockingBackgroundDeliveryThreadStore struct {
	*DurableEventSessionStore
	once      sync.Once
	entered   chan struct{}
	attempt   chan struct{}
	release   chan struct{}
	blockItem func(map[string]any) bool
}

type observingBackgroundDeliveryThreadStore struct {
	*DurableEventSessionStore
	results []subagentapp.BackgroundDeliveryEnsureItemResultV1
}

func (store *blockingBackgroundDeliveryThreadStore) EnsureBackgroundDeliveryItemExact(
	threadID, turnID string,
	item map[string]any,
) (subagentapp.BackgroundDeliveryEnsureItemResultV1, error) {
	shouldBlock := store.blockItem == nil || store.blockItem(item)
	if shouldBlock {
		if store.attempt != nil {
			store.attempt <- struct{}{}
		}
		store.once.Do(func() {
			close(store.entered)
			<-store.release
		})
	}
	return store.DurableEventSessionStore.EnsureBackgroundDeliveryItemExact(threadID, turnID, item)
}

func (store *observingBackgroundDeliveryThreadStore) EnsureBackgroundDeliveryItemExact(
	threadID, turnID string,
	item map[string]any,
) (subagentapp.BackgroundDeliveryEnsureItemResultV1, error) {
	result, err := store.DurableEventSessionStore.EnsureBackgroundDeliveryItemExact(threadID, turnID, item)
	store.results = append(store.results, result)
	return result, err
}

func (*backgroundAutoContinueProvider) RequiresDurablePipelineStagesV1() {}

func runtimeStartupRecoveryForTest(handler *runtimeServerHandler) subagentapp.StartupRecoveryService {
	securityAuthority := handler.runtimeJobSecurityAuthorizer()
	return subagentapp.NewStartupRecoveryService(subagentapp.StartupRecoveryDependencies{
		Threads: handler.store, Jobs: handler.jobs, Security: securityAuthority,
		CompletionAuthority: handler.childCompletions,
		AutoContinueStarter: handler.runtimeBackgroundDeliveryService().AutoContinueStarter,
		RecordEvent:         handler.recordRuntimeBestEffortEvent,
	})
}

func runRuntimeStartupRecoveryForTest(handler *runtimeServerHandler) error {
	cleaned, err := handler.jobs.CleanupStaleRunningRecords()
	if err != nil {
		return err
	}
	recovery := runtimeStartupRecoveryForTest(handler)
	if err := recovery.RecoverInterrupted(cleaned); err != nil {
		return err
	}
	return recovery.RecoverPendingDeliveries()
}

func captureRuntimeStderrForTest(t *testing.T, run func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stderr
	os.Stderr = writer
	run()
	_ = writer.Close()
	os.Stderr = previous
	body, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestRuntimeBestEffortEventRecordFailureUsesFixedStderrProjection(t *testing.T) {
	const turnID = "turn_event_record_pii_13900000006"
	const contextSentinel = "tool-started-context_/private/context-pii-13900000007"
	const errorSentinel = "record failed: /private/error-pii-13900000008"
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"title": "Record failure", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	if threadID == "" {
		t.Fatalf("record failure fixture thread identity mismatch: %#v", thread)
	}
	highestSeq := func() int {
		seq, err := store.HighestSeq(threadID)
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return seq
	}
	beforeSeq := highestSeq()
	recordAttempts := 0
	store.beforeRecordEventHook = func(map[string]any) error {
		recordAttempts++
		return errors.New(errorSentinel)
	}
	handler := &runtimeServerHandler{store: store}
	diagnostic := captureRuntimeStderrForTest(t, func() {
		handler.recordRuntimeBestEffortEvent(map[string]any{
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
			"stage": "input_received", "details": map[string]any{"stepIndex": float64(0), "promptBytes": float64(1)},
		}, contextSentinel)
	})
	const expected = "[analytix] event=ANALYTIX_RUNTIME_EVENT_RECORD_FAILED\n"
	if diagnostic != expected {
		t.Fatalf("best-effort event record stderr projection mismatch: got=%q want=%q", diagnostic, expected)
	}
	for _, sentinel := range []string{threadID, turnID, contextSentinel, errorSentinel, "/private/"} {
		if strings.Contains(diagnostic, sentinel) {
			t.Fatalf("best-effort event record stderr leaked hostile sentinel %q: %q", sentinel, diagnostic)
		}
	}
	afterSeq := highestSeq()
	if recordAttempts != 1 || afterSeq != beforeSeq {
		t.Fatalf("best-effort event record semantics changed: attempts=%d beforeSeq=%d afterSeq=%d", recordAttempts, beforeSeq, afterSeq)
	}
}

func (p *backgroundAutoContinueProvider) Stream(_ context.Context, request domainmodel.Request) (domainmodel.Result, error) {
	if err := emitTestDurableProviderPipelinePairV1(request); err != nil {
		return domainmodel.Result{}, err
	}
	p.mu.Lock()
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	chunk := domainmodel.Chunk{Kind: domainmodel.ChunkText, Text: "AUTO_PARENT_CONTINUED"}
	if request.OnChunk != nil {
		if err := request.OnChunk(chunk); err != nil {
			return domainmodel.Result{}, err
		}
	}
	return domainmodel.Result{
		ProviderID:      request.ProviderID,
		EndpointFormat:  request.EndpointFormat,
		Chunks:          []domainmodel.Chunk{chunk},
		StreamCompleted: true,
	}, nil
}

func (p *backgroundAutoContinueProvider) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func (p *backgroundAutoContinueProvider) LastMessages() []domainmodel.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.requests) == 0 {
		return nil
	}
	return append([]domainmodel.Message(nil), p.requests[len(p.requests)-1].Messages...)
}

type backgroundAutoContinueServerFixtureV1 struct {
	handler  *runtimeServerHandler
	store    *DurableEventSessionStore
	manager  *jobs.Manager
	provider *backgroundAutoContinueProvider
	record   jobs.Record
	pending  runtimePendingToolCall
}

func newBackgroundAutoContinueServerFixtureV1(t *testing.T, id string) backgroundAutoContinueServerFixtureV1 {
	t.Helper()
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "analytix-hub", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "test-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	t.Cleanup(func() { _ = handler.Shutdown(context.Background()) })
	configureServerGeneralExecution(t, handler)
	workspace := workspacetest.New(t)
	thread, err := handler.store.CreateThread(map[string]any{
		"id": id, "title": "Auto continue", "workspace": workspace,
		"providerId": "analytix-hub", "model": "test-model",
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	parentTurnID := "turn_parent_auto"
	parent := appendServerBoundJobParent(t, handler.store, threadID, parentTurnID, workspace, "subagent")
	if _, err := handler.store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	record, err := handler.jobs.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal_auto", ParentThreadID: threadID, ParentTurnID: parentTurnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		ChildThreadID: "thr_child_auto", ChildTurnID: "turn_child_auto",
		Kind: "subagent", Label: "Background child", Status: "completed", Background: true,
		AutoContinueParent: true, ProviderID: "analytix-hub", Model: "test-model", SecurityBinding: parent.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &backgroundAutoContinueProvider{}
	handler.provider = provider
	return backgroundAutoContinueServerFixtureV1{
		handler: handler, store: handler.store, manager: handler.jobs, provider: provider, record: record,
		pending: runtimePendingToolCall{
			ThreadID: threadID, TurnID: parentTurnID, ProviderID: "analytix-hub", Model: "test-model",
			ToolCallItemID: parent.ItemID, Call: domainmodel.ToolCall{ID: parent.CallID, Name: "task"},
			SecurityContext: parent.Context, ExecutionGrant: parent.Grant,
		},
	}
}

func admitBackgroundAutoContinueCompletionV1(t *testing.T, fixture backgroundAutoContinueServerFixtureV1) jobs.Record {
	t.Helper()
	thread, err := fixture.store.GetThread(fixture.record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	itemID, callID, toolName, ok := subagentapp.JobToolIdentity(thread, fixture.record)
	if !ok {
		t.Fatal("auto-continue parent tool identity is unavailable")
	}
	buildEvents := func(current domainjob.Record) []map[string]any {
		return subagentapp.BuildDurableJobLifecycleEventsV1(
			current.ParentThreadID, current.ParentTurnID, itemID, callID, toolName, current,
		)
	}
	admitted, ok, err := subagentapp.AdmitAndPublishBackgroundCompletionLifecycleV1(
		fixture.handler.runtimeBackgroundDeliveryService(), fixture.store,
		fixture.record.ParentThreadID, fixture.record.ParentTurnID, callID, toolName, fixture.record, buildEvents,
	)
	if err != nil || !ok {
		t.Fatalf("admit background completion: record=%#v ok=%t err=%v", admitted, ok, err)
	}
	return admitted
}

func waitBackgroundAutoContinueFinalizedV1(
	t *testing.T,
	fixture backgroundAutoContinueServerFixtureV1,
	turnID string,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		replay, err := fixture.store.eventLog.LoadSince(fixture.record.ParentThreadID, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range replay.Events {
			if stringField(event, "kind") == "turn_completed" && stringField(event, "turnId") == turnID {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for auto-continue finalizer turn=%s provider=%d", turnID, fixture.provider.Count())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func backgroundCompletionLifecycleEventsForJob(events []map[string]any, jobID string) []map[string]any {
	matched := []map[string]any{}
	for _, event := range events {
		if subagentapp.BackgroundJobIDFromEvent(event) == jobID {
			matched = append(matched, event)
		}
	}
	return matched
}

func backgroundLifecycleBundleEventsForJob(events []map[string]any, jobID string) []map[string]any {
	matched := []map[string]any{}
	for _, event := range events {
		child, _ := event["child"].(map[string]any)
		candidate := firstNonEmptyString(
			subagentapp.BackgroundJobIDFromEvent(event), stringField(child, "jobId"),
			stringField(child, "childRunId"), stringField(child, "childId"),
		)
		if candidate == jobID {
			matched = append(matched, event)
		}
	}
	return matched
}

func assertCanonicalBackgroundLifecycleBundle(t *testing.T, events []map[string]any, record jobs.Record) {
	t.Helper()
	bundle := backgroundLifecycleBundleEventsForJob(events, record.ID)
	if len(bundle) != 3 {
		t.Fatalf("canonical background lifecycle bundle count=%d: %#v", len(bundle), bundle)
	}
	previous := -1
	for _, event := range bundle {
		sequence, ok := numericSeq(event["seq"])
		if !ok || previous >= 0 && sequence != previous+1 {
			t.Fatalf("canonical background lifecycle bundle sequence is not contiguous: %#v", bundle)
		}
		previous = sequence
	}
}

type backgroundLifecycleCrashFixture struct {
	store     *DurableEventSessionStore
	manager   *jobs.Manager
	handler   *runtimeServerHandler
	record    jobs.Record
	threadID  string
	turnID    string
	itemID    string
	callID    string
	jobRoot   string
	beforeSeq int
}

func newBackgroundLifecycleCrashFixture(t *testing.T, suffix string) backgroundLifecycleCrashFixture {
	t.Helper()
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_lifecycle_crash_" + suffix, "title": "Lifecycle crash", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn_lifecycle_crash_"+suffix
	parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
	jobRoot := t.TempDir()
	manager, err := jobs.NewManager(jobRoot)
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal_lifecycle_crash_" + suffix, ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		ChildThreadID: "child_lifecycle_crash_" + suffix, ChildTurnID: "child_turn_lifecycle_crash_" + suffix,
		Kind: "subagent", Label: "Lifecycle child", Status: "completed", Background: true,
		SecurityBinding: parent.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
		Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID,
		Model: "background-parent-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success",
	})
	if err != nil || !committed.Changed {
		t.Fatalf("commit lifecycle crash parent: result=%#v err=%v", committed, err)
	}
	beforeSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager, runtimeToken: DefaultRuntimeToken}
	events := subagentapp.BuildDurableJobLifecycleEventsV1(
		threadID, turnID, parent.ItemID, parent.CallID, "task", record,
	)
	admitted, ok, err := subagentapp.AdmitBackgroundCompletionLifecycleV1(
		handler.runtimeBackgroundDeliveryService(), threadID, turnID, parent.CallID, "task", events,
	)
	if err != nil || !ok || admitted.CompletionDeliveryStatus != "delivered" {
		t.Fatalf("admit lifecycle crash fixture: record=%#v ok=%t err=%v", admitted, ok, err)
	}
	assertNoBackgroundCompletionLifecycleEventsSince(t, store, threadID, beforeSeq, record.ID)
	return backgroundLifecycleCrashFixture{
		store: store, manager: manager, handler: handler, record: admitted,
		threadID: threadID, turnID: turnID, itemID: parent.ItemID, callID: parent.CallID,
		jobRoot: jobRoot, beforeSeq: beforeSeq,
	}
}

func assertNoBackgroundCompletionLifecycleEventsSince(
	t *testing.T,
	store *DurableEventSessionStore,
	threadID string,
	afterSeq int,
	jobID string,
) {
	t.Helper()
	replay, err := store.LoadEventsSince(threadID, afterSeq)
	if err != nil {
		t.Fatal(err)
	}
	if matched := backgroundCompletionLifecycleEventsForJob(replay.Events, jobID); len(matched) != 0 {
		t.Fatalf("rejected background completion published lifecycle events: %#v", matched)
	}
}

func assertCanonicalAdmittedBackgroundLifecycleEvents(
	t *testing.T,
	events []map[string]any,
	record jobs.Record,
	forbidden string,
) {
	t.Helper()
	matched := backgroundCompletionLifecycleEventsForJob(events, record.ID)
	if len(matched) != 1 {
		t.Fatalf("canonical background lifecycle event count=%d: %#v", len(matched), events)
	}
	for _, event := range matched {
		details, _ := event["details"].(map[string]any)
		body, _ := json.Marshal(event)
		if stringField(event, "stage") != "background_job_"+record.Status ||
			stringField(details, "status") != record.Status ||
			stringField(details, "deliveryId") != record.CompletionDeliveryID ||
			stringField(details, "deliveryItemId") != record.CompletionDeliveryItemID ||
			stringField(details, "deliveryStatus") != "delivered" ||
			(forbidden != "" && strings.Contains(string(body), forbidden)) ||
			(record.Status != "failed" && strings.Contains(string(body), "background_job_failed")) {
			t.Fatalf("background lifecycle event is not durable canonical state: %s", body)
		}
	}
}

func appendActiveBackgroundJobParentForPublicSeam(
	t *testing.T,
	store *DurableEventSessionStore,
	threadID, turnID, workspace string,
) serverBoundJobParent {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	workspaceRealPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	securityContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: threadID, TurnID: turnID, WorkspaceRealPath: workspaceRealPath, ContextEpoch: 1, IssuedAt: now,
	})
	callID := serverTestHostToolCallID("background-public-seam-" + turnID)
	itemID := domaintoolcall.ToolCallItemIDV1(turnID, callID)
	arguments := map[string]any{"prompt": "bounded background work"}
	argumentsBody, _ := json.Marshal(arguments)
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider_1", ServerIdentity: "host:builtin", ToolName: "task", ToolCallID: callID,
		ArgsHash: domainsecurity.CanonicalJSONHash(argumentsBody), SchemaHash: domainsecurity.SHA256Hex([]byte("background-public-seam-schema")),
		ScopeHash: domainsecurity.SHA256Hex([]byte("background-public-seam-scope")), ReadOnly: true, ApprovalState: "not_required",
		IssuedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	})
	grantRecord := map[string]any{}
	grantBody, _ := json.Marshal(grant)
	_ = json.Unmarshal(grantBody, &grantRecord)
	binding, err := domainjob.NewSecurityBinding(securityContext, grant, callID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnToThread(threadID, map[string]any{
		"id": turnID, "threadId": threadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": turnsecurityapp.PublicRecord(securityContext),
		"items": []any{map[string]any{
			"id": itemID, "turnId": turnID, "threadId": threadID, "kind": "tool_call", "toolName": "task",
			"toolKind": "subagent", "callId": callID, "status": "completed", "createdAt": now.Format(time.RFC3339Nano),
			"arguments": arguments, "contextDigest": securityContext.ContextDigest, "contextEpoch": float64(securityContext.ContextEpoch),
			"executionGrantId": grant.GrantID, "executionGrant": grantRecord,
		}},
	}, "provider_1", map[string]any{"securityState": turnsecurityapp.PublicRecord(securityContext)}); err != nil {
		t.Fatal(err)
	}
	return serverBoundJobParent{Context: securityContext, Grant: grant, Binding: binding, ItemID: itemID, CallID: callID}
}

func TestRecordRuntimeRecoveredJobEventsSettlesParentToolResult(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id":        "thr_recovered_job",
		"title":     "Recovered job",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_recovered_job"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	startRequest := jobs.StartRequest{
		ParentGoalID: "goal-recovered", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		ChildThreadID: "thr_child", Kind: "subagent", Label: "Desktop scan",
		Status: "running", Background: true, SecurityBinding: parent.Binding,
		ToolPolicy: "inherit", ToolInvocations: 10,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	if _, err := manager.StartChildRun(startRequest); err != nil {
		t.Fatal(err)
	}
	cleaned, err := manager.CleanupStaleRunningRecords()
	if err != nil || len(cleaned) != 1 {
		t.Fatalf("cleanup recovered fixture: records=%#v err=%v", cleaned, err)
	}
	record := cleaned[0]
	handler := &runtimeServerHandler{store: store, jobs: manager}

	recovery := runtimeStartupRecoveryForTest(handler)
	const recoveryWorkers = 16
	errs := make(chan error, recoveryWorkers)
	var group sync.WaitGroup
	for index := 0; index < recoveryWorkers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			errs <- recovery.RecoverInterrupted([]jobs.Record{record})
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected one turn, got %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	items := listAny(turn["items"])
	resultCount := 0
	for _, rawItem := range items {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "id") == parent.ItemID && stringField(item, "status") != "failed" {
			t.Fatalf("parent tool call should be failed after recovered settlement: %#v", item)
		}
		if stringField(item, "kind") != "tool_result" || stringField(item, "callId") != parent.CallID {
			continue
		}
		resultCount++
		if stringField(item, "status") != "failed" || boolField(item, "isError") != true {
			t.Fatalf("recovered parent tool result should be failed: %#v", item)
		}
		output, _ := item["output"].(map[string]any)
		if stringField(output, "code") != "tool_failed" ||
			stringField(output, "projectionKind") != "host_status" ||
			stringField(output, "disclosure") != "metadata_only" ||
			!boolField(output, "privatePayloadWithheld") {
			t.Fatalf("recovered output is not a closed public projection: %#v", output)
		}
		if stringField(output, "childRunId") != "" || stringField(output, "childThreadId") != "" {
			t.Fatalf("recovered child authority leaked through tool result output: %#v", output)
		}
	}
	if resultCount != 1 {
		t.Fatalf("expected exactly one recovered tool result, got %d in %#v", resultCount, items)
	}
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RecoveryStatus != "recovered" || updated.CompletionDeliveryStatus != "delivered" {
		t.Fatalf("current-bound recovery did not close its audit state: %#v", updated)
	}
	recoveryEvents, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalAdmittedBackgroundLifecycleEvents(t, recoveryEvents.Events, updated, "")
}

func TestStartupRecoveryDeadLettersArchivedParentWithoutSettlementOrProvider(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_recovered_archived", "title": "Recovered archived", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_recovered_archived"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	startRequest := jobs.StartRequest{
		ParentGoalID: "goal-recovered-archived", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		ChildThreadID: "thr_child_archived", Kind: "subagent", Label: "Archived recovery",
		Status: "running", Background: true, SecurityBinding: parent.Binding,
	}
	jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
	if _, err := manager.StartChildRun(startRequest); err != nil {
		t.Fatal(err)
	}
	cleaned, err := manager.CleanupStaleRunningRecords()
	if err != nil || len(cleaned) != 1 {
		t.Fatalf("cleanup archived fixture: records=%#v err=%v", cleaned, err)
	}
	record := cleaned[0]
	if _, err := store.PatchThread(threadID, map[string]any{"status": "archived"}); err != nil {
		t.Fatal(err)
	}
	parentBefore, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	parentBeforeBody, _ := json.Marshal(parentBefore)
	eventsBefore, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventsBeforeBody, _ := json.Marshal(eventsBefore.Events)
	provider := &backgroundAutoContinueProvider{}
	handler := &runtimeServerHandler{store: store, jobs: manager, provider: provider}

	if err := runtimeStartupRecoveryForTest(handler).RecoverInterrupted([]jobs.Record{record}); err != nil {
		t.Fatal(err)
	}

	parentAfter, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	parentAfterBody, _ := json.Marshal(parentAfter)
	eventsAfter, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventsAfterBody, _ := json.Marshal(eventsAfter.Events)
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(parentBeforeBody, parentAfterBody) || !bytes.Equal(eventsBeforeBody, eventsAfterBody) || provider.Count() != 0 {
		t.Fatalf("archived recovery changed protected parent surfaces: parent=%t events=%t provider=%d",
			bytes.Equal(parentBeforeBody, parentAfterBody), bytes.Equal(eventsBeforeBody, eventsAfterBody), provider.Count())
	}
	if updated.Status != record.Status || updated.CompletionDeliveryStatus != "dead_letter" ||
		updated.CompletionDeliveryReason != "parent_thread_archived" || updated.RecoveryStatus != "dead_lettered" ||
		updated.RecoveryReason != "parent_thread_archived" || updated.DeadLetterReason != "parent_thread_archived" ||
		!updated.LateCompletionSuppressed || updated.LateCompletionReason != "parent_thread_archived" ||
		updated.CompletionDeliveryAttempts != record.CompletionDeliveryAttempts+1 ||
		updated.RecoveryAttempt != record.RecoveryAttempt+1 || updated.CompletionDeliveryID != record.CompletionDeliveryID ||
		updated.CompletionDeliveryItemID != record.CompletionDeliveryItemID || updated.AutoContinueStatus != record.AutoContinueStatus {
		t.Fatalf("archived recovery durable disposition escaped its bounded dead letter: before=%#v after=%#v", record, updated)
	}
	normalized := updated
	normalized.LateCompletionSuppressed = record.LateCompletionSuppressed
	normalized.LateCompletionReason = record.LateCompletionReason
	normalized.LateCompletionUpdatedAt = record.LateCompletionUpdatedAt
	normalized.CompletionDeliveryStatus = record.CompletionDeliveryStatus
	normalized.CompletionDeliveryReason = record.CompletionDeliveryReason
	normalized.CompletionDeadLetterAt = record.CompletionDeadLetterAt
	normalized.CompletionDeliveryAttempts = record.CompletionDeliveryAttempts
	normalized.RecoveryStatus = record.RecoveryStatus
	normalized.RecoveryAttempt = record.RecoveryAttempt
	normalized.RecoveryReason = record.RecoveryReason
	normalized.RecoveryUpdatedAt = record.RecoveryUpdatedAt
	normalized.DeadLetterReason = record.DeadLetterReason
	normalized.UpdatedAt = record.UpdatedAt
	normalized.SteerState = record.SteerState
	normalized.PauseState = record.PauseState
	normalizedBody, _ := json.Marshal(normalized)
	recordBody, _ := json.Marshal(record)
	if !bytes.Equal(normalizedBody, recordBody) {
		t.Fatalf("archived recovery changed non-disposition job bytes: before=%s after=%s", recordBody, normalizedBody)
	}
}

func TestRecordRuntimeJobLifecycleEventPersistsBackgroundCompletionReplayItem(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id":        "thr_background_job",
		"title":     "Background job",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_background_job"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_background_job",
		ParentThreadID:   threadID,
		ParentTurnID:     turnID,
		ParentToolItemID: parent.ItemID,
		ParentToolCallID: parent.CallID,
		ChildThreadID:    "thr_child",
		ChildTurnID:      "turn_child",
		Kind:             "subagent",
		Label:            "Background child",
		Status:           "completed",
		Background:       true,
		SecurityBinding:  parent.Binding,
		ToolPolicy:       "readOnly",
		ToolInvocations:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	if blocker := handler.runtimeJobSecurityAuthorizer().Blocker(record); blocker != "" {
		t.Fatalf("bound background fixture is not current: %s", blocker)
	}

	handler.recordRuntimeJobLifecycleEvent(record, "completed", "")
	handler.recordRuntimeJobLifecycleEvent(record, "completed", "")
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleReplay, err := store.LoadEventsSince(threadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalAdmittedBackgroundLifecycleEvents(t, lifecycleReplay.Events, updated, "")

	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("expected one turn, got %#v", turns)
	}
	turn, _ := turns[0].(map[string]any)
	progressCount := 0
	for _, rawItem := range listAny(turn["items"]) {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "kind") != "tool_progress" {
			continue
		}
		args, _ := item["arguments"].(map[string]any)
		diagnostics, _ := args["diagnostics"].(map[string]any)
		if stringField(args, "stage") != "background_job_completed" {
			continue
		}
		progressCount++
		if stringField(item, "status") != "completed" ||
			stringField(item, "toolKind") != "subagent" ||
			stringField(item, "callId") != parent.CallID ||
			stringField(item, "message") != "security-bound child output withheld" {
			t.Fatalf("progress item mismatch: %#v", item)
		}
		if stringField(args, "runtimeStatus") != "tool_progress" ||
			stringField(args, "stage") != "background_job_completed" ||
			stringField(args, "message") != "security-bound child output withheld" {
			t.Fatalf("progress arguments mismatch: %#v", item)
		}
		if stringField(diagnostics, "outputPreview") != "" ||
			boolField(diagnostics, "canContinueParent") || diagnostics["outputWithheld"] != true ||
			stringField(diagnostics, "outputTrustStatus") != "untrusted_child_output" {
			t.Fatalf("progress diagnostics mismatch: %#v", item)
		}
		child, _ := args["child"].(map[string]any)
		if stringField(child, "childRunId") != record.ID ||
			stringField(child, "childStatus") != "completed" || child["outputWithheld"] != true ||
			boolField(child, "factAnswerAllowed") {
			t.Fatalf("progress child mismatch: %#v", item)
		}
	}
	if progressCount != 1 {
		t.Fatalf("expected exactly one replayable progress item, got %d in %#v", progressCount, turn["items"])
	}
}

func TestRecordRuntimeSubagentEventUsesFreshDurableBackgroundStatus(t *testing.T) {
	const hostile = "CALLER_COMPLETED_6222028888888888888_aref_hostile_/private/case.duckdb"
	t.Run("durable_running_rejects_forged_completion", func(t *testing.T) {
		store, err := NewTempDurableEventSessionStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		workspace := workspacetest.New(t)
		thread, err := store.CreateThread(map[string]any{
			"id": "thr_fresh_running", "title": "Fresh running", "workspace": workspace,
		}, workspace)
		if err != nil {
			t.Fatal(err)
		}
		threadID, turnID := stringField(thread, "id"), "turn_fresh_running"
		parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
		manager, err := jobs.NewManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		request := jobs.StartRequest{
			ParentGoalID: "goal_fresh_running", ParentThreadID: threadID, ParentTurnID: turnID,
			ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
			Kind: "subagent", Status: "running", Background: true, SecurityBinding: parent.Binding,
		}
		jobsecuritytest.BindDelegatedToolManifestRequest(t, &request)
		record, err := manager.StartChildRun(request)
		if err != nil {
			t.Fatal(err)
		}
		beforeSeq, err := store.HighestSeq(threadID)
		if err != nil {
			t.Fatal(err)
		}
		handler := &runtimeServerHandler{store: store, jobs: manager}
		pending := runtimePendingToolCall{
			ThreadID: threadID, TurnID: turnID, ToolCallItemID: parent.ItemID,
			Call: domainmodel.ToolCall{ID: parent.CallID, Name: "task"},
		}
		handler.recordRuntimeSubagentEvent(pending, record, "completed", hostile)
		replay, err := store.LoadEventsSince(threadID, beforeSeq)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(replay.Events)
		if len(backgroundCompletionLifecycleEventsForJob(replay.Events, record.ID)) != 0 ||
			bytes.Contains(body, []byte("background_job_completed")) || bytes.Contains(body, []byte(hostile)) ||
			!bytes.Contains(body, []byte("subagent_running")) {
			t.Fatalf("caller status/message escaped durable running projection: %s", body)
		}
	})

	t.Run("stale_running_caller_observes_durable_completion", func(t *testing.T) {
		store, err := NewTempDurableEventSessionStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		workspace := workspacetest.New(t)
		thread, err := store.CreateThread(map[string]any{
			"id": "thr_fresh_completed", "title": "Fresh completed", "workspace": workspace,
		}, workspace)
		if err != nil {
			t.Fatal(err)
		}
		threadID, turnID := stringField(thread, "id"), "turn_fresh_completed"
		parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
		manager, err := jobs.NewManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		request := jobs.StartRequest{
			ParentGoalID: "goal_fresh_completed", ParentThreadID: threadID, ParentTurnID: turnID,
			ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
			Kind: "subagent", Status: "running", Background: true, SecurityBinding: parent.Binding,
		}
		jobsecuritytest.BindDelegatedToolManifestRequest(t, &request)
		stale, err := manager.StartChildRun(request)
		if err != nil {
			t.Fatal(err)
		}
		handler := &runtimeServerHandler{store: store, jobs: manager}
		pending := runtimePendingToolCall{
			ThreadID: threadID, TurnID: turnID, ToolCallItemID: parent.ItemID,
			Call: domainmodel.ToolCall{ID: parent.CallID, Name: "task"},
		}
		handler.recordRuntimeSubagentEvent(pending, stale, "running", "")
		if _, err := manager.UpdateChildRun(stale.ID, domainjob.UpdateRequest{Status: "completed"}); err != nil {
			t.Fatal(err)
		}
		committed, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
			Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID,
			Model: "background-parent-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success",
		})
		if err != nil || !committed.Changed {
			t.Fatalf("commit stale-caller parent: result=%#v err=%v", committed, err)
		}
		beforeSeq, err := store.HighestSeq(threadID)
		if err != nil {
			t.Fatal(err)
		}
		live, unsubscribe := store.SubscribeEvents(threadID)
		defer unsubscribe()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		store.beforeRecordEventHook = func(event map[string]any) error {
			if stringField(event, "kind") != "pipeline_stage" || stringField(event, "stage") != "subagent_completed" {
				return nil
			}
			once.Do(func() { close(entered) })
			<-release
			return nil
		}
		done := make(chan struct{})
		go func() {
			handler.recordRuntimeSubagentEvent(pending, stale, "failed", hostile)
			close(done)
		}()
		<-entered
		admitted, err := manager.LoadChildRun(stale.ID)
		if err != nil || admitted.CompletionDeliveryStatus != "delivered" {
			close(release)
			<-done
			t.Fatalf("fresh durable terminal did not pass admission before event: record=%#v err=%v", admitted, err)
		}
		streamedBeforePersist := []map[string]any{}
		for {
			select {
			case event := <-live:
				streamedBeforePersist = append(streamedBeforePersist, event)
			default:
				goto subscriberDrained
			}
		}
	subscriberDrained:
		if matched := backgroundCompletionLifecycleEventsForJob(streamedBeforePersist, stale.ID); len(matched) != 0 {
			close(release)
			<-done
			t.Fatalf("subscriber observed completion lifecycle before event persistence: %#v", matched)
		}
		persistedBeforeRelease, err := store.eventLog.LoadSince(threadID, beforeSeq)
		if err != nil || len(backgroundCompletionLifecycleEventsForJob(persistedBeforeRelease.Events, stale.ID)) != 0 {
			close(release)
			<-done
			t.Fatalf("durable replay observed lifecycle before event persistence: events=%#v err=%v", persistedBeforeRelease.Events, err)
		}
		close(release)
		<-done
		store.beforeRecordEventHook = nil
		replay, err := store.LoadEventsSince(threadID, beforeSeq)
		if err != nil {
			t.Fatal(err)
		}
		updated, err := manager.LoadChildRun(stale.ID)
		if err != nil {
			t.Fatal(err)
		}
		assertCanonicalAdmittedBackgroundLifecycleEvents(t, replay.Events, updated, hostile)
		assertCanonicalBackgroundLifecycleBundle(t, replay.Events, updated)
	})

	t.Run("wrong_caller_item_identity_emits_zero_events", func(t *testing.T) {
		store, err := NewTempDurableEventSessionStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		workspace := workspacetest.New(t)
		thread, err := store.CreateThread(map[string]any{
			"id": "thr_wrong_caller_item", "title": "Wrong caller item", "workspace": workspace,
		}, workspace)
		if err != nil {
			t.Fatal(err)
		}
		threadID, turnID := stringField(thread, "id"), "turn_wrong_caller_item"
		parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
		manager, err := jobs.NewManager(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		record, err := manager.StartChildRun(jobs.StartRequest{
			ParentGoalID: "goal_wrong_caller_item", ParentThreadID: threadID, ParentTurnID: turnID,
			ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
			Kind: "subagent", Status: "completed", Background: true, SecurityBinding: parent.Binding,
		})
		if err != nil {
			t.Fatal(err)
		}
		committed, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
			Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID,
			Model: "background-parent-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success",
		})
		if err != nil || !committed.Changed {
			t.Fatalf("commit wrong-item parent: result=%#v err=%v", committed, err)
		}
		beforeSeq, err := store.HighestSeq(threadID)
		if err != nil {
			t.Fatal(err)
		}
		handler := &runtimeServerHandler{store: store, jobs: manager}
		pending := runtimePendingToolCall{
			ThreadID: threadID, TurnID: turnID, ToolCallItemID: "item_tool_forged",
			Call: domainmodel.ToolCall{ID: parent.CallID, Name: "task"},
		}
		handler.recordRuntimeSubagentEvent(pending, record, "completed", hostile)
		replay, err := store.LoadEventsSince(threadID, beforeSeq)
		if err != nil {
			t.Fatal(err)
		}
		if len(replay.Events) != 0 {
			t.Fatalf("wrong caller item identity polluted durable event log: %#v", replay.Events)
		}
	})
}

func TestRuntimeBackgroundLifecyclePublisherErrorIsBoundedAndStartupRepairable(t *testing.T) {
	fixture := newBackgroundLifecycleCrashFixture(t, "publisher_error")
	const hostile = "PUBLISHER_ERROR_6222027777777777777_aref_private_/private/case.duckdb"
	fixture.store.beforeRecordEventHook = func(map[string]any) error { return errors.New(hostile) }
	pending := runtimePendingToolCall{
		ThreadID: fixture.threadID, TurnID: fixture.turnID, ToolCallItemID: fixture.itemID,
		Call: domainmodel.ToolCall{ID: fixture.callID, Name: "task"},
	}
	diagnostic := captureRuntimeStderrForTest(t, func() {
		fixture.handler.recordRuntimeSubagentEvent(pending, fixture.record, "failed", hostile)
	})
	fixture.store.beforeRecordEventHook = nil
	if !strings.Contains(diagnostic, "code=background_lifecycle_reconcile_failed") ||
		strings.Contains(diagnostic, hostile) || strings.Contains(diagnostic, "6222027777777777777") ||
		strings.Contains(diagnostic, "aref_private") || strings.Contains(diagnostic, "/private/") {
		t.Fatalf("background lifecycle diagnostic was missing or unsafe: %q", diagnostic)
	}
	failed, err := fixture.manager.LoadChildRun(fixture.record.ID)
	if err != nil || failed.CompletionDeliveryStatus != "delivered" || failed.CompletionDeliveryReason == "dead_letter" {
		t.Fatalf("publisher failure changed delivered job disposition: record=%#v err=%v", failed, err)
	}
	assertNoBackgroundCompletionLifecycleEventsSince(t, fixture.store, fixture.threadID, fixture.beforeSeq, fixture.record.ID)
	restarted, err := jobs.NewManager(fixture.jobRoot)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewTempDurableEventSessionStore(fixture.store.root)
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: reopened, jobs: restarted}
	if err := runRuntimeStartupRecoveryForTest(handler); err != nil {
		t.Fatalf("formal startup did not repair live publisher failure: %v", err)
	}
	replay, err := reopened.LoadEventsSince(fixture.threadID, fixture.beforeSeq)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := restarted.LoadChildRun(fixture.record.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalAdmittedBackgroundLifecycleEvents(t, replay.Events, updated, hostile)
	assertCanonicalBackgroundLifecycleBundle(t, replay.Events, updated)
}

func TestBackgroundJobCompletionDeliveryLedgerDeliveredAndDeduped(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id":        "thr_delivery_job",
		"title":     "Delivery job",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_delivery_job"
	parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
	jobRoot := t.TempDir()
	manager, err := jobs.NewManager(jobRoot)
	if err != nil {
		t.Fatalf("create job manager: %v", err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_delivery",
		ParentThreadID:   threadID,
		ParentTurnID:     turnID,
		ParentToolItemID: parent.ItemID,
		ParentToolCallID: parent.CallID,
		ChildThreadID:    "thr_child_delivery",
		ChildTurnID:      "turn_child_delivery",
		Kind:             "subagent",
		Label:            "Delivery child",
		Status:           "completed",
		Background:       true,
		SecurityBinding:  parent.Binding,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	committed, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
		Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID,
		Model: "background-parent-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success",
	})
	if err != nil || !committed.Changed || committed.Status != "completed" {
		t.Fatalf("commit terminal parent: result=%#v err=%v", committed, err)
	}
	terminalBefore, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	terminalTurnsBefore := listAny(terminalBefore["turns"])
	terminalTurnBefore, _ := terminalTurnsBefore[0].(map[string]any)
	bindingBefore, _ := json.Marshal(terminalTurnBefore["generalTerminalCASBinding"])
	publicationBefore, _ := json.Marshal(terminalTurnBefore["generalTerminalPublication"])
	handler := &runtimeServerHandler{store: store, jobs: manager, runtimeToken: DefaultRuntimeToken}
	beforeLifecycleSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}

	const hostileOutput = "CHILD_RAW_PII_6222020000000000000_AUTHORITY_REF_aref_hostile_/private/case.duckdb_<think>provider draft</think>"
	hostileEvent := map[string]any{
		"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID,
		"stage": "background_job_completed", "message": hostileOutput,
		"child": map[string]any{
			"jobId": record.ID, "status": "completed", "background": true,
			"outputWithheld": true, "outputTrustStatus": "untrusted_child_output", "output": hostileOutput,
		},
		"details": map[string]any{
			"notificationKind": "background_job_completion", "jobId": record.ID, "childRunId": record.ID,
			"status": "completed", "background": true, "outputWithheld": true,
			"outputTrustStatus": "untrusted_child_output", "rawBody": hostileOutput, "providerDraft": hostileOutput,
		},
		"rawProviderFailure": hostileOutput,
	}
	_, _, _ = subagentapp.AdmitBackgroundCompletionLifecycleV1(
		handler.runtimeBackgroundDeliveryService(), threadID, turnID, parent.CallID, "task", []map[string]any{hostileEvent},
	)
	admittedBeforeEvent, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if admittedBeforeEvent.CompletionDeliveryStatus != "delivered" {
		t.Fatalf("completion item admission did not settle before event crash cut: %#v", admittedBeforeEvent)
	}
	assertNoBackgroundCompletionLifecycleEventsSince(t, store, threadID, beforeLifecycleSeq, record.ID)
	restarted, err := jobs.NewManager(jobRoot)
	if err != nil {
		t.Fatalf("restart job manager after admission crash cut: %v", err)
	}
	reopenedStore, err := NewTempDurableEventSessionStore(store.root)
	if err != nil {
		t.Fatalf("reopen durable event store after admission crash cut: %v", err)
	}
	restartedHandler := &runtimeServerHandler{store: reopenedStore, jobs: restarted, runtimeToken: DefaultRuntimeToken}
	if err := runRuntimeStartupRecoveryForTest(restartedHandler); err != nil {
		t.Fatalf("formal startup recovery did not repair admission/event crash cut: %v", err)
	}
	deliveredBeforeReplay, err := restarted.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load delivered child before exact replay: %v", err)
	}
	publishedBeforeRestart, err := reopenedStore.LoadEventsSince(threadID, beforeLifecycleSeq)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalAdmittedBackgroundLifecycleEvents(t, publishedBeforeRestart.Events, deliveredBeforeReplay, hostileOutput)
	assertCanonicalBackgroundLifecycleBundle(t, publishedBeforeRestart.Events, deliveredBeforeReplay)
	publishedBeforeRestartBody, _ := json.Marshal(publishedBeforeRestart.Events)
	if err := runRuntimeStartupRecoveryForTest(restartedHandler); err != nil {
		t.Fatalf("second formal startup exact replay failed: %v", err)
	}
	publishedAfterRestart, err := reopenedStore.LoadEventsSince(threadID, beforeLifecycleSeq)
	if err != nil {
		t.Fatal(err)
	}
	publishedAfterRestartBody, _ := json.Marshal(publishedAfterRestart.Events)
	if !bytes.Equal(publishedBeforeRestartBody, publishedAfterRestartBody) {
		t.Fatalf("formal startup exact replay changed durable lifecycle bundle: before=%s after=%s", publishedBeforeRestartBody, publishedAfterRestartBody)
	}
	store, manager, handler = reopenedStore, restarted, restartedHandler
	durableBeforeReplay, _ := json.Marshal(deliveredBeforeReplay)
	deliveredItemID := subagentapp.BackgroundCompletionDeliveryItemID(
		turnID, deliveredBeforeReplay.CompletionDeliveryID, "delivered",
	)
	threadBeforeReplay, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("load delivered ledger before exact replay: %v", err)
	}
	var deliveredItemBefore []byte
	for _, rawItem := range listAny(listAny(threadBeforeReplay["turns"])[0].(map[string]any)["items"]) {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "id") == deliveredItemID {
			deliveredItemBefore, _ = json.Marshal(item)
		}
	}
	if len(deliveredItemBefore) == 0 {
		t.Fatal("delivered ledger item missing before exact replay")
	}
	observingStore := &observingBackgroundDeliveryThreadStore{DurableEventSessionStore: store}
	replayed := (subagentapp.BackgroundDeliveryService{
		Threads: observingStore, Jobs: manager, SecurityBlocker: handler.runtimeJobSecurityAuthorizer().Blocker,
	}).UpdateCompletion(
		record.ID, deliveredBeforeReplay.CompletionDeliveryID, deliveredBeforeReplay.CompletionDeliveryItemID,
		"delivered", "duplicate_delivery", "HOSTILE_DUPLICATE_DELIVERY_ERROR",
	)
	if replayed.ID == "" {
		t.Fatal("delivered exact replay returned an empty durable record")
	}
	durableAfterReplay, _ := json.Marshal(replayed)
	if !bytes.Equal(durableBeforeReplay, durableAfterReplay) {
		t.Fatalf("delivered exact replay changed durable record: before=%s after=%s", durableBeforeReplay, durableAfterReplay)
	}
	if len(observingStore.results) != 1 || observingStore.results[0] != subagentapp.BackgroundDeliveryItemExistingExactV1 {
		t.Fatalf("delivered exact replay ensure results=%#v", observingStore.results)
	}
	threadAfterReplay, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("load delivered ledger after exact replay: %v", err)
	}
	deliveredCount := 0
	for _, rawItem := range listAny(listAny(threadAfterReplay["turns"])[0].(map[string]any)["items"]) {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "id") != deliveredItemID {
			continue
		}
		deliveredCount++
		deliveredItemAfter, _ := json.Marshal(item)
		if !bytes.Equal(deliveredItemBefore, deliveredItemAfter) {
			t.Fatalf("delivered exact replay changed ledger item: before=%s after=%s", deliveredItemBefore, deliveredItemAfter)
		}
	}
	if deliveredCount != 1 {
		t.Fatalf("delivered exact replay item count=%d", deliveredCount)
	}
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load child run: %v", err)
	}
	if updated.CompletionDeliveryID == "" ||
		updated.CompletionDeliveryStatus != "delivered" ||
		updated.CompletionDeliveryItemID == "" ||
		updated.CompletionDeliveryAttempts < 2 {
		t.Fatalf("completion delivery ledger mismatch: %#v", updated)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	completionProgressCount := 0
	deliveryPendingCount := 0
	deliveryDeliveredCount := 0
	for _, rawTurn := range listAny(reloaded["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "kind") != "tool_progress" {
				continue
			}
			args, _ := item["arguments"].(map[string]any)
			diagnostics, _ := args["diagnostics"].(map[string]any)
			if stringField(args, "stage") == "background_job_completed" {
				completionProgressCount++
				continue
			}
			switch stringField(diagnostics, "notificationKind") {
			case "background_job_delivery":
				switch stringField(diagnostics, "deliveryStatus") {
				case "pending":
					deliveryPendingCount++
				case "delivered":
					deliveryDeliveredCount++
					if stringField(diagnostics, "jobId") != record.ID || diagnostics["outputWithheld"] != true ||
						boolField(diagnostics, "factAnswerAllowed") {
						t.Fatalf("delivered ledger diagnostics mismatch: %#v", item)
					}
				default:
					t.Fatalf("unexpected delivery ledger status: %#v", item)
				}
			}
		}
	}
	if completionProgressCount != 1 || deliveryPendingCount != 1 || deliveryDeliveredCount != 1 {
		t.Fatalf("duplicate delivery should not append duplicate ledger rows, completion=%d pending=%d delivered=%d thread=%#v", completionProgressCount, deliveryPendingCount, deliveryDeliveredCount, reloaded)
	}
	turns := listAny(reloaded["turns"])
	if len(turns) != 1 {
		t.Fatalf("background completion created a second parent turn: %#v", turns)
	}
	parentTurn, _ := turns[0].(map[string]any)
	bindingAfter, _ := json.Marshal(parentTurn["generalTerminalCASBinding"])
	publicationAfter, _ := json.Marshal(parentTurn["generalTerminalPublication"])
	if stringField(parentTurn, "status") != "completed" || parentTurn["acceptedFinal"] != nil ||
		parentTurn["generalTerminalPublication"] == nil || !bytes.Equal(bindingBefore, bindingAfter) || !bytes.Equal(publicationBefore, publicationAfter) {
		t.Fatalf("background completion re-finalized the terminal parent: %#v", parentTurn)
	}
	durableBody, _ := json.Marshal(reloaded)
	providerBody, _ := json.Marshal(appmodel.ProviderHistoryFromThread(reloaded))
	publicThread, err := threadapp.EnsurePublicProjector(nil).ProjectThread(reloaded)
	if err != nil {
		t.Fatalf("project public thread: %v", err)
	}
	publicThreadBody, _ := json.Marshal(publicThread)
	httpRecorder := httptest.NewRecorder()
	httpRequest := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID+"/summary", nil)
	httpRequest.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	handler.ServeHTTP(httpRecorder, httpRequest)
	if httpRecorder.Code != http.StatusOK {
		t.Fatalf("public HTTP thread read failed: status=%d body=%s", httpRecorder.Code, httpRecorder.Body.String())
	}
	sseRecorder := httptest.NewRecorder()
	sseRequest := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil)
	sseRequest.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	handler.ServeHTTP(sseRecorder, sseRequest)
	if sseRecorder.Code != http.StatusOK {
		t.Fatalf("public SSE replay failed: status=%d body=%s", sseRecorder.Code, sseRecorder.Body.String())
	}
	for sink, body := range map[string][]byte{
		"durable parent":           durableBody,
		"provider history":         providerBody,
		"public thread projection": publicThreadBody,
		"public HTTP":              httpRecorder.Body.Bytes(),
		"public SSE":               sseRecorder.Body.Bytes(),
	} {
		if bytes.Contains(body, []byte(hostileOutput)) || bytes.Contains(body, []byte("6222020000000000000")) ||
			bytes.Contains(body, []byte("aref_hostile")) || bytes.Contains(body, []byte("/private/case.duckdb")) ||
			bytes.Contains(body, []byte("provider draft")) {
			t.Fatalf("hostile child bytes reached %s: %s", sink, body)
		}
	}
}

func TestStartupRecoveryFailsClosedForIncompleteConflictingOrCorruptLifecycleReplay(t *testing.T) {
	for _, mode := range []string{"incomplete_bundle", "conflicting_bundle", "conflicting_sibling_identity", "corrupt_replay"} {
		t.Run(mode, func(t *testing.T) {
			fixture := newBackgroundLifecycleCrashFixture(t, mode)
			drafts := subagentapp.CanonicalBackgroundCompletionLifecycleEventsV1(
				subagentapp.BuildDurableJobLifecycleEventsV1(
					fixture.threadID, fixture.turnID, fixture.itemID, fixture.callID, "task", fixture.record,
				),
				fixture.record,
			)
			if len(drafts) != 3 {
				t.Fatalf("test lifecycle bundle count=%d", len(drafts))
			}
			switch mode {
			case "incomplete_bundle":
				if _, _, err := fixture.store.RecordEvent(drafts[len(drafts)-1]); err != nil {
					t.Fatal(err)
				}
			case "conflicting_bundle":
				conflicting := make([]map[string]any, len(drafts))
				for index := range drafts {
					conflicting[index] = cloneMap(drafts[index])
				}
				conflicting[0]["message"] = "background lifecycle unavailable"
				if _, _, err := fixture.store.RecordEventsAtomic(conflicting); err != nil {
					t.Fatal(err)
				}
			case "conflicting_sibling_identity":
				conflicting := cloneMap(drafts[1])
				conflicting["itemId"] = "item_tool_forged"
				if _, _, err := fixture.store.RecordEvent(conflicting); err != nil {
					t.Fatal(err)
				}
			case "corrupt_replay":
				file, err := os.OpenFile(fixture.store.eventLog.EventsPath(fixture.threadID), os.O_WRONLY|os.O_APPEND, 0o600)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.WriteString("{not-json\n"); err != nil {
					_ = file.Close()
					t.Fatal(err)
				}
				if err := file.Sync(); err != nil {
					_ = file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			beforeEvents, err := os.ReadFile(fixture.store.eventLog.EventsPath(fixture.threadID))
			if err != nil {
				t.Fatal(err)
			}
			beforeJob, _ := json.Marshal(fixture.record)
			restarted, err := jobs.NewManager(fixture.jobRoot)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := NewTempDurableEventSessionStore(fixture.store.root)
			if err != nil {
				t.Fatal(err)
			}
			handler := &runtimeServerHandler{store: reopened, jobs: restarted}
			if err := runRuntimeStartupRecoveryForTest(handler); err == nil {
				t.Fatal("formal startup accepted an incomplete, conflicting, or corrupt lifecycle replay")
			}
			afterEvents, err := os.ReadFile(reopened.eventLog.EventsPath(fixture.threadID))
			if err != nil {
				t.Fatal(err)
			}
			after, err := restarted.LoadChildRun(fixture.record.ID)
			if err != nil {
				t.Fatal(err)
			}
			afterJob, _ := json.Marshal(after)
			if !bytes.Equal(beforeEvents, afterEvents) || !bytes.Equal(beforeJob, afterJob) ||
				after.CompletionDeliveryStatus != "delivered" {
				t.Fatalf("failed-closed startup changed protected state: events=%t job=%t record=%#v",
					bytes.Equal(beforeEvents, afterEvents), bytes.Equal(beforeJob, afterJob), after)
			}
		})
	}
}

func TestBackgroundJobCompletionConcurrentLiveRecoveryIsExactlyOnce(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_delivery_concurrent", "title": "Concurrent delivery", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn_delivery_concurrent"
	parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal_delivery_concurrent", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		ChildThreadID: "thr_child_concurrent", ChildTurnID: "turn_child_concurrent",
		Kind: "subagent", Label: "Concurrent child", Status: "completed", Background: true,
		SecurityBinding: parent.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
		Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID,
		Model: "background-parent-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success",
	})
	if err != nil || !committed.Changed {
		t.Fatalf("commit terminal parent: result=%#v err=%v", committed, err)
	}
	terminalBefore, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turnBefore, _ := listAny(terminalBefore["turns"])[0].(map[string]any)
	bindingBefore, _ := json.Marshal(turnBefore["generalTerminalCASBinding"])
	publicationBefore, _ := json.Marshal(turnBefore["generalTerminalPublication"])
	beforeSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	liveEvents, unsubscribe := store.SubscribeEvents(threadID)
	defer unsubscribe()

	handler := &runtimeServerHandler{store: store, jobs: manager, runtimeToken: DefaultRuntimeToken}
	blockingStore := &blockingBackgroundDeliveryThreadStore{
		DurableEventSessionStore: store,
		entered:                  make(chan struct{}),
		attempt:                  make(chan struct{}, 64),
		release:                  make(chan struct{}),
		blockItem: func(item map[string]any) bool {
			return strings.Contains(stringField(item, "id"), "_background_job_completed_")
		},
	}
	recovery := subagentapp.NewStartupRecoveryService(subagentapp.StartupRecoveryDependencies{
		Threads: blockingStore, Jobs: manager, Security: handler.runtimeJobSecurityAuthorizer(),
		RecordEvent: handler.recordRuntimeBestEffortEvent,
	})
	recoveryErr := make(chan error, 1)
	go func() {
		recoveryErr <- recovery.RecoverPendingDeliveries()
	}()
	<-blockingStore.entered
	<-blockingStore.attempt

	const hostileCallerText = "HOSTILE_CALLER_STATUS_PII_6222029999999999999_aref_wrong_/private/wrong.duckdb"
	pending := runtimePendingToolCall{
		ThreadID:       threadID,
		TurnID:         turnID,
		ToolCallItemID: parent.ItemID,
		Call:           domainmodel.ToolCall{ID: parent.CallID, Name: "task"},
	}
	blockedDelivery := subagentapp.BackgroundDeliveryService{
		Threads: blockingStore, Jobs: manager, SecurityBlocker: handler.runtimeJobSecurityAuthorizer().Blocker,
		RecordEvent: handler.recordRuntimeBestEffortEvent,
	}
	start := make(chan struct{})
	var live sync.WaitGroup
	for index := 0; index < 16; index++ {
		live.Add(1)
		go func() {
			defer live.Done()
			<-start
			_, _ = subagentapp.RecordBackgroundSubagentLifecycleV1(
				blockedDelivery, handler.store, pending.ThreadID, pending.TurnID,
				pending.ToolCallItemID, pending.Call.ID, pending.Call.Name, record, "failed", hostileCallerText,
			)
		}()
	}
	close(start)
	for index := 0; index < 16; index++ {
		<-blockingStore.attempt
	}
	streamedBeforeAdmission := []map[string]any{}
	for {
		select {
		case event := <-liveEvents:
			streamedBeforeAdmission = append(streamedBeforeAdmission, event)
		default:
			goto preAdmissionStreamDrained
		}
	}

preAdmissionStreamDrained:
	if matched := backgroundCompletionLifecycleEventsForJob(streamedBeforeAdmission, record.ID); len(matched) != 0 {
		t.Fatalf("subscriber observed lifecycle before durable admission: %#v", matched)
	}
	blockedReplay, err := store.LoadEventsSince(threadID, beforeSeq)
	if err != nil {
		t.Fatal(err)
	}
	if matched := backgroundCompletionLifecycleEventsForJob(blockedReplay.Events, record.ID); len(matched) != 0 {
		t.Fatalf("replay observed lifecycle before durable admission: %#v", matched)
	}
	close(blockingStore.release)
	live.Wait()
	if err := <-recoveryErr; err != nil {
		t.Fatalf("concurrent recovery failed: %v", err)
	}

	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CompletionDeliveryStatus != "delivered" || updated.CompletionDeliveryItemID == "" {
		t.Fatalf("concurrent delivery did not settle: %#v", updated)
	}
	streamed := []map[string]any{}
	for {
		select {
		case event := <-liveEvents:
			streamed = append(streamed, event)
		default:
			goto streamDrained
		}
	}

streamDrained:
	assertCanonicalAdmittedBackgroundLifecycleEvents(t, streamed, updated, hostileCallerText)
	replayed, err := store.LoadEventsSince(threadID, beforeSeq)
	if err != nil {
		t.Fatal(err)
	}
	assertCanonicalAdmittedBackgroundLifecycleEvents(t, replayed.Events, updated, hostileCallerText)
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, rawTurn := range listAny(reloaded["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			itemID := stringField(item, "id")
			if itemID != "" {
				counts[itemID]++
			}
		}
	}
	for itemID, count := range counts {
		if count != 1 {
			t.Fatalf("item %s persisted %d times: %#v", itemID, count, reloaded)
		}
	}
	for _, status := range []string{"retry", "pending", "delivered"} {
		itemID := subagentapp.BackgroundCompletionDeliveryItemID(turnID, updated.CompletionDeliveryID, status)
		if counts[itemID] != 1 {
			t.Fatalf("delivery ledger %s count=%d items=%#v", status, counts[itemID], reloaded)
		}
	}
	if counts[updated.CompletionDeliveryItemID] != 1 {
		t.Fatalf("completion item count=%d items=%#v", counts[updated.CompletionDeliveryItemID], reloaded)
	}
	turnAfter, _ := listAny(reloaded["turns"])[0].(map[string]any)
	bindingAfter, _ := json.Marshal(turnAfter["generalTerminalCASBinding"])
	publicationAfter, _ := json.Marshal(turnAfter["generalTerminalPublication"])
	if !bytes.Equal(bindingBefore, bindingAfter) || !bytes.Equal(publicationBefore, publicationAfter) ||
		stringField(turnAfter, "status") != "completed" || turnAfter["acceptedFinal"] != nil {
		t.Fatalf("concurrent delivery changed parent terminal authority: %#v", turnAfter)
	}

	sseRecorder := httptest.NewRecorder()
	sseRequest := httptest.NewRequest(http.MethodGet, "/v1/threads/"+threadID+"/events?since_seq=0", nil)
	sseRequest.Header.Set("Authorization", "Bearer "+DefaultRuntimeToken)
	handler.ServeHTTP(sseRecorder, sseRequest)
	if sseRecorder.Code != http.StatusOK {
		t.Fatalf("SSE replay failed: status=%d body=%s", sseRecorder.Code, sseRecorder.Body.String())
	}
	combined, _ := json.Marshal(reloaded)
	combined = append(combined, sseRecorder.Body.Bytes()...)
	if bytes.Contains(combined, []byte(hostileCallerText)) ||
		bytes.Contains(combined, []byte("background_job_failed")) ||
		!bytes.Contains(combined, []byte("background_job_completed")) {
		t.Fatalf("caller status/message bypassed durable canonicalization: %s", combined)
	}
}

func TestBackgroundJobCompletionRejectsStatusForgeryBeforeParentAppend(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"id": "thr_delivery_status_forgery", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn_delivery_status_forgery"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal_delivery_status_forgery", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID, Kind: "subagent",
		Status: "completed", Background: true, SecurityBinding: parent.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	beforeBody, _ := json.Marshal(before)
	beforeSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	_, _, _ = subagentapp.AdmitBackgroundCompletionLifecycleV1(
		handler.runtimeBackgroundDeliveryService(), threadID, turnID, parent.CallID, "task", []map[string]any{{
			"kind": "pipeline_stage", "threadId": threadID, "turnId": turnID, "stage": "background_job_failed",
			"details": map[string]any{
				"notificationKind": "background_job_completion", "jobId": record.ID, "childRunId": record.ID,
				"status": "failed", "background": true, "outputWithheld": true, "outputTrustStatus": "untrusted_child_output",
			},
		}},
	)
	after, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := json.Marshal(after)
	if !bytes.Equal(beforeBody, afterBody) {
		t.Fatalf("forged completion status mutated terminal parent: before=%s after=%s", beforeBody, afterBody)
	}
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CompletionDeliveryStatus != "dead_letter" || updated.CompletionDeliveryReason != "parent_tool_identity_invalid" {
		t.Fatalf("forged completion status did not fail closed: %#v", updated)
	}
	assertNoBackgroundCompletionLifecycleEventsSince(t, store, threadID, beforeSeq, record.ID)
}

func TestBackgroundJobCompletionIdentityConflictDoesNotOverwriteOrRefinalizeParent(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_delivery_identity_conflict", "title": "Delivery conflict", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn_delivery_identity_conflict"
	parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal_delivery_identity_conflict", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		Kind: "subagent", Status: "completed", Background: true, SecurityBinding: parent.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
		Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID,
		Model: "background-parent-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success",
	})
	if err != nil || !committed.Changed {
		t.Fatalf("commit terminal parent: result=%#v err=%v", committed, err)
	}
	terminalBefore, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turnBefore, _ := listAny(terminalBefore["turns"])[0].(map[string]any)
	bindingBefore, _ := json.Marshal(turnBefore["generalTerminalCASBinding"])
	publicationBefore, _ := json.Marshal(turnBefore["generalTerminalPublication"])

	event, ok := subagentapp.BuildBackgroundJobCompletionNotificationEvent(subagentapp.JobLifecycleEventInput{
		ThreadID: threadID, TurnID: turnID, ItemID: parent.ItemID, CallID: parent.CallID,
		ToolName: "task", Record: record, Status: record.Status,
	})
	if !ok {
		t.Fatal("build canonical completion event")
	}
	itemID := subagentapp.BackgroundJobCompletionItemID(turnID, event)
	expected, ok := subagentapp.BuildBackgroundJobCompletionNotificationItem(subagentapp.BackgroundJobCompletionNotificationItemInput{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, CallID: parent.CallID,
		ToolName: "task", CreatedAt: record.FinishedAt, Event: event,
	})
	if !ok {
		t.Fatal("build canonical completion item")
	}
	conflict := cloneMap(expected)
	conflictTimestamp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	conflict["createdAt"] = conflictTimestamp
	if err := store.AppendItemToTurn(threadID, turnID, conflict); err != nil {
		t.Fatalf("seed conflicting item: %v", err)
	}
	result, err := store.EnsureBackgroundDeliveryItemExact(threadID, turnID, expected)
	if err != nil || result != subagentapp.BackgroundDeliveryItemConflictV1 {
		t.Fatalf("exact conflict result=%q err=%v", result, err)
	}
	beforeSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}

	handler := &runtimeServerHandler{store: store, jobs: manager}
	handler.recordRuntimeJobLifecycleEvent(record, "failed", "HOSTILE_CONFLICT_CALLER_MESSAGE")
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CompletionDeliveryStatus != "dead_letter" ||
		updated.CompletionDeliveryReason != "parent_tool_identity_invalid" {
		t.Fatalf("identity conflict did not fail closed: %#v", updated)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	itemCount := 0
	for _, rawItem := range listAny(listAny(reloaded["turns"])[0].(map[string]any)["items"]) {
		item, _ := rawItem.(map[string]any)
		if stringField(item, "id") != itemID {
			continue
		}
		itemCount++
		if stringField(item, "createdAt") != conflictTimestamp {
			t.Fatalf("conflicting item was overwritten: %#v", item)
		}
	}
	if itemCount != 1 {
		t.Fatalf("conflicting item count=%d thread=%#v", itemCount, reloaded)
	}
	turnAfter, _ := listAny(reloaded["turns"])[0].(map[string]any)
	bindingAfter, _ := json.Marshal(turnAfter["generalTerminalCASBinding"])
	publicationAfter, _ := json.Marshal(turnAfter["generalTerminalPublication"])
	if !bytes.Equal(bindingBefore, bindingAfter) || !bytes.Equal(publicationBefore, publicationAfter) ||
		stringField(turnAfter, "status") != "completed" || turnAfter["acceptedFinal"] != nil {
		t.Fatalf("identity conflict changed parent terminal authority: %#v", turnAfter)
	}
	assertNoBackgroundCompletionLifecycleEventsSince(t, store, threadID, beforeSeq, record.ID)
}

func TestBackgroundJobDeliveryLedgerIdentityConflictDeadLettersWithoutOverwrite(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id": "thr_delivery_ledger_conflict", "title": "Delivery ledger conflict", "workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID, turnID := stringField(thread, "id"), "turn_delivery_ledger_conflict"
	parent := appendActiveBackgroundJobParentForPublicSeam(t, store, threadID, turnID, workspace)
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal_delivery_ledger_conflict", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		Kind: "subagent", Status: "completed", Background: true, SecurityBinding: parent.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := appturn.CommitCompletedTurn(appturn.CommitCompletionInput{
		Store: store, SecurityContext: parent.Context, ThreadID: threadID, TurnID: turnID,
		Model: "background-parent-model", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano), TerminalReason: "success",
	})
	if err != nil || !committed.Changed {
		t.Fatalf("commit terminal parent: result=%#v err=%v", committed, err)
	}
	terminalBefore, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	turnBefore, _ := listAny(terminalBefore["turns"])[0].(map[string]any)
	bindingBefore, _ := json.Marshal(turnBefore["generalTerminalCASBinding"])
	publicationBefore, _ := json.Marshal(turnBefore["generalTerminalPublication"])

	event, ok := subagentapp.BuildBackgroundJobCompletionNotificationEvent(subagentapp.JobLifecycleEventInput{
		ThreadID: threadID, TurnID: turnID, ItemID: parent.ItemID, CallID: parent.CallID,
		ToolName: "task", Record: record, Status: record.Status,
	})
	if !ok {
		t.Fatal("build canonical completion event")
	}
	deliveryID := subagentapp.BackgroundJobCompletionDeliveryID(turnID, event)
	completionItemID := subagentapp.BackgroundJobCompletionItemID(turnID, event)
	pending, err := manager.UpdateChildRun(record.ID, jobs.UpdateRequest{
		CompletionDeliveryID: deliveryID, CompletionDeliveryItemID: completionItemID,
		CompletionDeliveryStatus: "pending",
	})
	if err != nil {
		t.Fatal(err)
	}
	pendingItemID := subagentapp.BackgroundCompletionDeliveryItemID(turnID, deliveryID, "pending")
	expected, ok := subagentapp.BuildBackgroundJobDeliveryLedgerItem(subagentapp.BackgroundJobDeliveryLedgerItemInput{
		ThreadID: threadID, TurnID: turnID, ItemID: pendingItemID,
		CallID:    subagentapp.BackgroundCompletionDeliveryCallID(deliveryID, "pending"),
		CreatedAt: pending.UpdatedAt, Record: pending, Status: "pending",
	})
	if !ok {
		t.Fatal("build canonical pending ledger item")
	}
	conflict := cloneMap(expected)
	conflictTimestamp := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano)
	conflict["createdAt"] = conflictTimestamp
	if err := store.AppendItemToTurn(threadID, turnID, conflict); err != nil {
		t.Fatalf("seed pending ledger conflict: %v", err)
	}
	beforeSeq, err := store.HighestSeq(threadID)
	if err != nil {
		t.Fatal(err)
	}

	handler := &runtimeServerHandler{store: store, jobs: manager}
	handler.recordRuntimeJobLifecycleEvent(record, "failed", "HOSTILE_LEDGER_CONFLICT_MESSAGE")
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CompletionDeliveryStatus != "dead_letter" ||
		updated.CompletionDeliveryReason != "parent_tool_identity_invalid" {
		t.Fatalf("ledger conflict did not fail closed: %#v", updated)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	pendingCount, completionCount := 0, 0
	turnAfter, _ := listAny(reloaded["turns"])[0].(map[string]any)
	for _, rawItem := range listAny(turnAfter["items"]) {
		item, _ := rawItem.(map[string]any)
		switch stringField(item, "id") {
		case pendingItemID:
			pendingCount++
			if stringField(item, "createdAt") != conflictTimestamp {
				t.Fatalf("pending ledger conflict was overwritten: %#v", item)
			}
		case completionItemID:
			completionCount++
		}
	}
	if pendingCount != 1 || completionCount != 0 {
		t.Fatalf("ledger conflict counts pending=%d completion=%d thread=%#v", pendingCount, completionCount, reloaded)
	}
	bindingAfter, _ := json.Marshal(turnAfter["generalTerminalCASBinding"])
	publicationAfter, _ := json.Marshal(turnAfter["generalTerminalPublication"])
	if !bytes.Equal(bindingBefore, bindingAfter) || !bytes.Equal(publicationBefore, publicationAfter) ||
		stringField(turnAfter, "status") != "completed" || turnAfter["acceptedFinal"] != nil {
		t.Fatalf("ledger conflict changed parent terminal authority: %#v", turnAfter)
	}
	assertNoBackgroundCompletionLifecycleEventsSince(t, store, threadID, beforeSeq, record.ID)
}

func TestBackgroundJobCompletionDeliveryDeadLettersMissingParent(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("create job manager: %v", err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_delivery_dead",
		ParentThreadID:   "thr_missing_parent",
		ParentTurnID:     "turn_missing_parent",
		ParentToolCallID: "call_task",
		Kind:             "subagent",
		Label:            "Missing parent child",
		Status:           "completed",
		Background:       true,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	beforeSeq, err := store.HighestSeq(record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}

	handler.recordRuntimeJobLifecycleEvent(record, "completed", "")

	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load child run: %v", err)
	}
	if updated.CompletionDeliveryStatus != "dead_letter" ||
		updated.CompletionDeliveryReason != "parent_thread_missing" ||
		updated.CompletionDeadLetterAt == "" {
		t.Fatalf("expected dead-lettered delivery: %#v", updated)
	}
	assertNoBackgroundCompletionLifecycleEventsSince(t, store, record.ParentThreadID, beforeSeq, record.ID)
}

func TestBackgroundJobCompletionDeliveryDeadLettersIneligibleParent(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	archivedThread, err := store.CreateThread(map[string]any{
		"id":        "thr_archived_parent",
		"title":     "Archived parent",
		"workspace": workspace,
		"status":    "archived",
		"archived":  true,
	}, workspace)
	if err != nil {
		t.Fatalf("create archived thread: %v", err)
	}
	archivedParent := appendServerBoundJobParent(t, store, stringField(archivedThread, "id"), "turn_archived", workspace, "subagent")
	archivedThread, err = store.PatchThread(stringField(archivedThread, "id"), map[string]any{"status": "archived"})
	if err != nil {
		t.Fatalf("archive thread: %v", err)
	}
	rewoundThread, err := store.CreateThread(map[string]any{
		"id":        "thr_rewound_parent",
		"title":     "Rewound parent",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create rewound thread: %v", err)
	}
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("create job manager: %v", err)
	}
	cases := []struct {
		name     string
		threadID string
		turnID   string
		reason   string
		binding  *domainjob.SecurityBinding
	}{
		{name: "archived", threadID: stringField(archivedThread, "id"), turnID: "turn_archived", reason: "parent_thread_archived", binding: archivedParent.Binding},
		{name: "rewound", threadID: stringField(rewoundThread, "id"), turnID: "turn_missing", reason: "parent_security_context_mismatch"},
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	for _, tc := range cases {
		callID := "call_task_" + tc.name
		itemID := ""
		if tc.binding != nil {
			callID = tc.binding.ParentToolCallID
			itemID = archivedParent.ItemID
		}
		record, err := manager.StartChildRun(jobs.StartRequest{
			ParentGoalID:     "goal_delivery_" + tc.name,
			ParentThreadID:   tc.threadID,
			ParentTurnID:     tc.turnID,
			ParentToolItemID: itemID,
			ParentToolCallID: callID,
			Kind:             "subagent",
			Label:            tc.name + " child",
			Status:           "completed",
			Background:       true,
			SecurityBinding:  tc.binding,
		})
		if err != nil {
			t.Fatalf("start child run %s: %v", tc.name, err)
		}
		beforeSeq, err := store.HighestSeq(tc.threadID)
		if err != nil {
			t.Fatal(err)
		}
		handler.recordRuntimeJobLifecycleEvent(record, "completed", "")
		updated, err := manager.LoadChildRun(record.ID)
		if err != nil {
			t.Fatalf("load child run %s: %v", tc.name, err)
		}
		if updated.CompletionDeliveryStatus != "dead_letter" ||
			updated.CompletionDeliveryReason != tc.reason ||
			updated.CompletionDeadLetterAt == "" {
			t.Fatalf("expected %s dead-letter reason %s: %#v", tc.name, tc.reason, updated)
		}
		assertNoBackgroundCompletionLifecycleEventsSince(t, store, tc.threadID, beforeSeq, record.ID)
	}
}

func TestRecoverRuntimeBackgroundJobDeliveriesRepairsMissingCompletionItem(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id":        "thr_delivery_recover",
		"title":     "Delivery recover",
		"workspace": workspace,
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_delivery_recover"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("create job manager: %v", err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_delivery_recover",
		ParentThreadID:   threadID,
		ParentTurnID:     turnID,
		ParentToolItemID: parent.ItemID,
		ParentToolCallID: parent.CallID,
		ChildThreadID:    "thr_child_recover",
		Kind:             "subagent",
		Label:            "Recovered delivery child",
		Status:           "completed",
		Background:       true,
		SecurityBinding:  parent.Binding,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}

	if err := runtimeStartupRecoveryForTest(handler).RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}

	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load child run: %v", err)
	}
	if updated.CompletionDeliveryStatus != "delivered" || updated.CompletionDeliveryItemID == "" {
		t.Fatalf("recovery should deliver missing completion item: %#v", updated)
	}
	reloaded, err := store.GetThread(threadID)
	if err != nil {
		t.Fatalf("reload thread: %v", err)
	}
	found := false
	retryCount := 0
	pendingCount := 0
	deliveredCount := 0
	for _, rawTurn := range listAny(reloaded["turns"]) {
		turn, _ := rawTurn.(map[string]any)
		for _, rawItem := range listAny(turn["items"]) {
			item, _ := rawItem.(map[string]any)
			if stringField(item, "id") == updated.CompletionDeliveryItemID {
				found = true
			}
			args, _ := item["arguments"].(map[string]any)
			diagnostics, _ := args["diagnostics"].(map[string]any)
			if stringField(diagnostics, "notificationKind") != "background_job_delivery" {
				continue
			}
			switch stringField(diagnostics, "deliveryStatus") {
			case "retry":
				retryCount++
				if diagnostics["outputWithheld"] != true || boolField(diagnostics, "factAnswerAllowed") {
					t.Fatalf("retry delivery reason mismatch: %#v", item)
				}
			case "pending":
				pendingCount++
			case "delivered":
				deliveredCount++
			default:
				t.Fatalf("unexpected recovery delivery status: %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("recovered completion item %s not found in thread: %#v", updated.CompletionDeliveryItemID, reloaded)
	}
	if retryCount != 1 || pendingCount != 1 || deliveredCount != 1 {
		t.Fatalf("recovery should replay retry/pending/delivered delivery rows, retry=%d pending=%d delivered=%d thread=%#v", retryCount, pendingCount, deliveredCount, reloaded)
	}
}

func TestRuntimeTaskJobRecoverEndpointSettlesStaleLeaseAndOrphanedJobs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		lastHeartbeat string
		leaseExpires  string
		staleAfterMs  int64
		orphaned      bool
	}{
		{name: "stale", lastHeartbeat: time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano), staleAfterMs: int64(time.Second / time.Millisecond)},
		{name: "lease_expired", leaseExpires: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)},
		{name: "orphaned", orphaned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewTempDurableEventSessionStore(t.TempDir())
			if err != nil {
				t.Fatalf("create durable store: %v", err)
			}
			workspace := workspacetest.New(t)
			thread, err := store.CreateThread(map[string]any{"id": "thr_recover_" + tc.name, "title": tc.name, "workspace": workspace}, workspace)
			if err != nil {
				t.Fatalf("create thread: %v", err)
			}
			threadID := stringField(thread, "id")
			turnID := "turn_recover_" + tc.name
			parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
			manager, err := jobs.NewManager(t.TempDir())
			if err != nil {
				t.Fatalf("create job manager: %v", err)
			}
			startRequest := jobs.StartRequest{
				ParentGoalID:     "goal_recover_" + tc.name,
				ParentThreadID:   threadID,
				ParentTurnID:     turnID,
				ParentToolItemID: parent.ItemID,
				ParentToolCallID: parent.CallID,
				Kind:             "subagent",
				Label:            tc.name + " child",
				Status:           "running",
				Background:       true,
				LastHeartbeatAt:  tc.lastHeartbeat,
				LeaseExpiresAt:   tc.leaseExpires,
				StaleAfterMs:     tc.staleAfterMs,
				SecurityBinding:  parent.Binding,
			}
			jobsecuritytest.BindDelegatedToolManifestRequest(t, &startRequest)
			record, err := manager.StartChildRun(startRequest)
			if err != nil {
				t.Fatalf("start child run: %v", err)
			}
			if tc.orphaned {
				value := true
				record, err = manager.UpdateChildRun(record.ID, jobs.UpdateRequest{Orphaned: &value})
				if err != nil {
					t.Fatalf("mark orphaned: %v", err)
				}
			}
			handler := &runtimeServerHandler{store: store, jobs: manager, insecure: true}

			body := postRuntimeTaskJobRecover(t, handler, threadID, record.ID, "", http.StatusOK)
			if body["outputWithheld"] != true || boolField(body, "factAnswerAllowed") {
				t.Fatalf("recover response exposed child output authority: %#v", body)
			}
			updated, err := manager.LoadChildRun(record.ID)
			if err != nil {
				t.Fatalf("load child run: %v", err)
			}
			if updated.Status != "interrupted" {
				t.Fatalf("recover should interrupt stale runtime job: %#v", updated)
			}
			if updated.RecoveryStatus != "recovered" || updated.CompletionDeliveryStatus != "delivered" {
				t.Fatalf("recover should persist recovered delivery state: %#v", updated)
			}
		})
	}
}

func TestRuntimeTaskJobRecoverEndpointRetriesDeadLetterDelivery(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("create job manager: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"title": "Retry dead letter", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatalf("create parent thread: %v", err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_retry_dead_letter"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:     "goal_retry_dead_letter",
		ParentThreadID:   threadID,
		ParentTurnID:     turnID,
		ParentToolItemID: parent.ItemID,
		ParentToolCallID: parent.CallID,
		Kind:             "subagent",
		Label:            "Retry delivery child",
		Status:           "completed",
		Background:       true,
		SecurityBinding:  parent.Binding,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	dead, err := manager.UpdateChildRun(record.ID, jobs.UpdateRequest{
		CompletionDeliveryID:     "delivery_retry_dead_letter",
		CompletionDeliveryStatus: "dead_letter",
		CompletionDeliveryReason: "append_parent_turn_failed",
		RecoveryStatus:           "dead_lettered",
		RecoveryReason:           "append_parent_turn_failed",
	})
	if err != nil {
		t.Fatalf("mark dead-letter run: %v", err)
	}
	if dead.CompletionDeliveryStatus != "dead_letter" || dead.CompletionDeliveryReason != "append_parent_turn_failed" {
		t.Fatalf("expected synthetic dead-letter: %#v", dead)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager, insecure: true}

	body := postRuntimeTaskJobRecover(t, handler, threadID, record.ID, dead.CompletionDeliveryID, http.StatusOK)
	if body["outputWithheld"] != true || boolField(body, "factAnswerAllowed") {
		t.Fatalf("retry recovery response mismatch: %#v", body)
	}
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load updated run: %v", err)
	}
	if updated.CompletionDeliveryStatus != "delivered" || updated.RecoveryStatus != "recovered" || updated.DeadLetterReason != "" {
		t.Fatalf("dead-letter retry should deliver and mark recovered: body=%#v record=%#v", body, updated)
	}
	duplicate := postRuntimeTaskJobRecover(t, handler, threadID, record.ID, updated.CompletionDeliveryID, http.StatusOK)
	if duplicate["outputWithheld"] != true || boolField(duplicate, "factAnswerAllowed") {
		t.Fatalf("duplicate recovery should be suppressed: %#v", duplicate)
	}
}

func TestRuntimeTaskJobRecoverEndpointClassifiesParentTurnAndSupersededFailures(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("create job manager: %v", err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager, insecure: true}

	missingParent, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:   "goal_missing_recover",
		ParentThreadID: "thr_missing_recover",
		ParentTurnID:   "turn_missing_recover",
		Kind:           "subagent",
		Status:         "completed",
		Background:     true,
	})
	if err != nil {
		t.Fatalf("start missing parent run: %v", err)
	}
	handler.recordRuntimeJobLifecycleEvent(missingParent, "completed", "")
	body := postRuntimeTaskJobRecover(t, handler, missingParent.ParentThreadID, missingParent.ID, "", http.StatusNotFound)
	if stringField(body, "code") != subagentapp.TaskJobErrorNotFound {
		t.Fatalf("missing parent recover result mismatch: %#v", body)
	}

	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"id": "thr_turn_missing_recover", "title": "Turn missing", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	turnMissing, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:   "goal_turn_missing_recover",
		ParentThreadID: stringField(thread, "id"),
		ParentTurnID:   "turn_rewound",
		Kind:           "subagent",
		Status:         "completed",
		Background:     true,
	})
	if err != nil {
		t.Fatalf("start turn missing run: %v", err)
	}
	handler.recordRuntimeJobLifecycleEvent(turnMissing, "completed", "")
	body = postRuntimeTaskJobRecover(t, handler, turnMissing.ParentThreadID, turnMissing.ID, "", http.StatusNotFound)
	if stringField(body, "code") != subagentapp.TaskJobErrorNotFound {
		t.Fatalf("turn missing recover result mismatch: %#v", body)
	}

	superseded, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:   "goal_superseded_recover",
		ParentThreadID: "thr_superseded_recover",
		ParentTurnID:   "turn_superseded_recover",
		Kind:           "subagent",
		Status:         "running",
		Background:     true,
	})
	if err != nil {
		t.Fatalf("start superseded run: %v", err)
	}
	value := true
	superseded, err = manager.UpdateChildRun(superseded.ID, jobs.UpdateRequest{
		Status:                   "killed",
		LateCompletionSuppressed: &value,
		LateCompletionReason:     "superseded by restart",
	})
	if err != nil {
		t.Fatalf("mark superseded: %v", err)
	}
	body = postRuntimeTaskJobRecover(t, handler, superseded.ParentThreadID, superseded.ID, "", http.StatusNotFound)
	if stringField(body, "code") != subagentapp.TaskJobErrorNotFound {
		t.Fatalf("superseded recover result mismatch: %#v", body)
	}
}

func postRuntimeTaskJobRecover(t *testing.T, handler *runtimeServerHandler, threadID string, jobID string, deliveryID string, status int) map[string]any {
	t.Helper()
	payload := map[string]any{"threadId": threadID, "jobId": jobID}
	if strings.TrimSpace(deliveryID) != "" {
		payload["deliveryId"] = deliveryID
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal recover payload: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/runtime/task-jobs/recover", bytes.NewReader(data))
	handler.ServeHTTP(recorder, request)
	if recorder.Code != status {
		t.Fatalf("recover status mismatch got=%d want=%d body=%s", recorder.Code, status, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode recover response: %v body=%s", err, recorder.Body.String())
	}
	return body
}

func TestBackgroundJobAutoContinueOrdinaryChildStartsNewParentTurn(t *testing.T) {
	const childCanary = "CHILD_PII_13800138000_/Users/private/case_AuthorityRef_LONG"
	handler := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "analytix-hub", BaseURL: "https://provider.invalid", APIKey: "test-key",
		Model: "test-model", EndpointFormat: "chat_completions",
	}).(*runtimeServerHandler)
	t.Cleanup(func() { _ = handler.Shutdown(context.Background()) })
	configureServerGeneralExecution(t, handler)
	store := handler.store
	manager := handler.jobs
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{
		"id":         "thr_auto_continue",
		"title":      "Auto continue",
		"workspace":  workspace,
		"providerId": "analytix-hub",
		"model":      "test-model",
	}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	parentTurnID := "turn_parent_auto"
	parent := appendServerBoundJobParent(t, store, threadID, parentTurnID, workspace, "subagent")
	if _, err := store.PatchThread(threadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatalf("patch idle: %v", err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:       "goal_auto",
		ParentThreadID:     threadID,
		ParentTurnID:       parentTurnID,
		ParentToolItemID:   parent.ItemID,
		ParentToolCallID:   parent.CallID,
		ChildThreadID:      "thr_child_auto_PRIVATE_13800138000",
		ChildTurnID:        "turn_child_auto",
		Kind:               "subagent",
		Label:              childCanary,
		Status:             "completed",
		Background:         true,
		AutoContinueParent: true,
		ProviderID:         "analytix-hub",
		Model:              "test-model",
		SecurityBinding:    parent.Binding,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	threadBeforeAutoContinue, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	parentTerminalBefore, ok := subagentapp.FindTurn(threadBeforeAutoContinue, parentTurnID)
	if !ok {
		t.Fatal("old parent terminal is missing")
	}
	parentTerminalBefore = cloneMap(parentTerminalBefore)
	delete(parentTerminalBefore, "items")
	parentTerminalBeforeBody, _ := json.Marshal(parentTerminalBefore)
	fakeProvider := &backgroundAutoContinueProvider{}
	handler.provider = fakeProvider
	pending := runtimePendingToolCall{
		ThreadID:        threadID,
		TurnID:          parentTurnID,
		ProviderID:      "analytix-hub",
		Model:           "test-model",
		ToolCallItemID:  parent.ItemID,
		Call:            domainmodel.ToolCall{ID: parent.CallID, Name: "task"},
		SecurityContext: parent.Context,
		ExecutionGrant:  parent.Grant,
	}

	handler.recordRuntimeSubagentEvent(pending, record, "completed", "")

	var updated jobs.Record
	deadline := time.Now().Add(10 * time.Second)
	for {
		updated, err = manager.LoadChildRun(record.ID)
		if err == nil && updated.AutoContinueStatus == "started" && fakeProvider.Count() == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for ordinary auto-continue: record=%#v err=%v provider=%d", updated, err, fakeProvider.Count())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if updated.AutoContinueStatus != "started" || updated.AutoContinueReason != "" ||
		updated.AutoContinueTurnID == "" {
		t.Fatalf("ordinary completion did not start auto-continue: %#v", updated)
	}
	if fakeProvider.Count() != 1 {
		t.Fatalf("ordinary auto-continue provider calls=%d want=1", fakeProvider.Count())
	}
	var reloaded map[string]any
	var turns []any
	var autoNoticeByStatus map[string]int
	for {
		reloaded, err = store.GetThread(threadID)
		if err != nil {
			t.Fatalf("reload thread: %v", err)
		}
		turns = listAny(reloaded["turns"])
		autoNoticeByStatus = map[string]int{}
		finalized := false
		if len(turns) != 0 {
			parentTurn, _ := turns[0].(map[string]any)
			for _, rawItem := range listAny(parentTurn["items"]) {
				item, _ := rawItem.(map[string]any)
				if stringField(item, "kind") != "tool_progress" {
					continue
				}
				args, _ := item["arguments"].(map[string]any)
				diagnostics, _ := args["diagnostics"].(map[string]any)
				if stringField(diagnostics, "notificationKind") != "background_job_auto_continue" {
					continue
				}
				if stringField(diagnostics, "reason") != "" || stringField(diagnostics, "jobId") != record.ID {
					t.Fatalf("auto continue notice diagnostics mismatch: %#v", item)
				}
				autoNoticeByStatus[stringField(diagnostics, "autoContinueStatus")]++
			}
		}
		var continuation map[string]any
		if len(turns) == 2 {
			continuation, _ = turns[1].(map[string]any)
		}
		replay, replayErr := store.eventLog.LoadSince(threadID, 0)
		if replayErr != nil {
			t.Fatalf("load continuation events: %v", replayErr)
		}
		for _, event := range replay.Events {
			if stringField(event, "kind") == "turn_completed" && stringField(event, "turnId") == updated.AutoContinueTurnID {
				finalized = true
				break
			}
		}
		if len(turns) == 2 && stringField(continuation, "status") == "completed" && finalized && autoNoticeByStatus["started"] == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for exact auto-continue lifecycle: turns=%#v notices=%#v", turns, autoNoticeByStatus)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if autoNoticeByStatus["starting"] != 0 || len(autoNoticeByStatus) != 1 {
		t.Fatalf("pre-admission auto-continue status mutated parent lifecycle: %#v", autoNoticeByStatus)
	}
	eventsBeforeDuplicate, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatalf("load events before duplicate callback: %v", err)
	}
	duplicate, duplicateErr := subagentapp.RecordBackgroundJobLifecycleV1(
		handler.runtimeBackgroundDeliveryService(), handler.store, record,
	)
	if duplicateErr != nil || duplicate.Blocker != "" {
		t.Fatalf("duplicate background lifecycle callback was not exact: result=%#v err=%v", duplicate, duplicateErr)
	}
	eventsAfterDuplicate, err := store.eventLog.LoadSince(threadID, 0)
	if err != nil {
		t.Fatalf("load events after duplicate callback: %v", err)
	}
	if !reflect.DeepEqual(eventsBeforeDuplicate.Events, eventsAfterDuplicate.Events) {
		t.Fatalf("duplicate callback emitted new lifecycle events: before=%d after=%d tail=%#v", len(eventsBeforeDuplicate.Events), len(eventsAfterDuplicate.Events), eventsAfterDuplicate.Events[len(eventsBeforeDuplicate.Events):])
	}
	finalThread, err := store.GetThread(threadID)
	if err != nil {
		t.Fatal(err)
	}
	oldParentAfter, ok := subagentapp.FindTurn(finalThread, parentTurnID)
	if !ok {
		t.Fatal("old parent terminal disappeared")
	}
	oldParentAfter = cloneMap(oldParentAfter)
	delete(oldParentAfter, "items")
	parentTerminalAfterBody, _ := json.Marshal(oldParentAfter)
	if !bytes.Equal(parentTerminalBeforeBody, parentTerminalAfterBody) {
		t.Fatalf("auto-continue rewrote old parent terminal/CAS/publication bytes: before=%s after=%s", parentTerminalBeforeBody, parentTerminalAfterBody)
	}
	providerMessages := fakeProvider.LastMessages()
	providerBody, err := json.Marshal(providerMessages)
	if err != nil {
		t.Fatal(err)
	}
	fixedTriggerCount := 0
	for _, message := range providerMessages {
		if message.Content == subagentapp.BackgroundAutoContinuePromptV1 {
			fixedTriggerCount++
		}
	}
	if fixedTriggerCount != 1 {
		t.Fatalf("provider history fixed trigger count=%d messages=%s", fixedTriggerCount, providerBody)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	publicThread := requestThreadSummaryJSON(t, server.URL, http.MethodGet, "/v1/threads/"+threadID, nil, http.StatusOK)
	publicBody, err := json.Marshal(publicThread)
	if err != nil {
		t.Fatal(err)
	}
	sseBody := []byte(requestRuntimeSSEText(t, server.URL+"/v1/threads/"+threadID+"/events?since_seq=0"))
	publicTurns := listAny(publicThread["turns"])
	if len(publicTurns) != 2 {
		t.Fatalf("authenticated HTTP auto-continue turns=%d body=%s", len(publicTurns), publicBody)
	}
	publicContinuation, _ := publicTurns[1].(map[string]any)
	if stringField(publicContinuation, "id") != updated.AutoContinueTurnID ||
		!bytes.Contains(sseBody, []byte(`"turnId":"`+updated.AutoContinueTurnID+`"`)) ||
		!bytes.Contains(sseBody, []byte(`"kind":"turn_completed"`)) {
		t.Fatalf("HTTP/SSE did not project the new finalized continuation: http=%s sse=%s", publicBody, sseBody)
	}
	for _, surface := range []struct {
		name string
		body []byte
	}{{"provider", providerBody}, {"http", publicBody}, {"sse", sseBody}} {
		for _, forbidden := range []string{childCanary, "13800138000", "/Users/private", "AuthorityRef", "ChildCompletionReceipt", "child_completion_receipt"} {
			if bytes.Contains(surface.body, []byte(forbidden)) {
				t.Fatalf("%s leaked %q: %s", surface.name, forbidden, surface.body)
			}
		}
	}
	if record.Output != "" || updated.Output != "" {
		t.Fatalf("security-bound child output was persisted: record=%#v updated=%#v", record, updated)
	}
}

func TestStartupRecoveryAutoContinueCrashAfterStartingBeforeTurnCreatesExactlyOneTurn(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_crash_before_turn")
	admitted := admitBackgroundAutoContinueCompletionV1(t, fixture)
	starter := fixture.handler.runtimeBackgroundDeliveryService().AutoContinueStarter
	turnID, err := starter.NewTurnID(admitted)
	if err != nil {
		t.Fatal(err)
	}
	reserved, won, err := fixture.manager.ReserveBackgroundAutoContinueV1(
		admitted.ID, turnID,
		func(current domainjob.Record, records []domainjob.Record) string {
			return subagentapp.BackgroundAutoContinueSiblingGate(current, records)
		},
	)
	if err != nil || !won || reserved.AutoContinueStatus != "starting" {
		t.Fatalf("durable starting reservation failed: record=%#v won=%t err=%v", reserved, won, err)
	}
	threadBefore, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listAny(threadBefore["turns"])) != 1 || fixture.provider.Count() != 0 {
		t.Fatalf("starting reservation created an effect before recovery: turns=%d provider=%d", len(listAny(threadBefore["turns"])), fixture.provider.Count())
	}
	recovery := runtimeStartupRecoveryForTest(fixture.handler)
	if err := recovery.RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	waitBackgroundAutoContinueFinalizedV1(t, fixture, turnID)
	started, err := fixture.manager.LoadChildRun(admitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.AutoContinueStatus != "started" || started.AutoContinueTurnID != turnID || fixture.provider.Count() != 1 {
		t.Fatalf("starting crash was not recovered exactly: record=%#v provider=%d", started, fixture.provider.Count())
	}
	threadExact, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	eventsExact, err := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	jobExact, _ := json.Marshal(started)
	threadExactBody, _ := json.Marshal(threadExact)
	eventsExactBody, _ := json.Marshal(eventsExact)
	if err := recovery.RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	threadReplay, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsReplay, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	jobReplay, _ := fixture.manager.LoadChildRun(admitted.ID)
	threadReplayBody, _ := json.Marshal(threadReplay)
	eventsReplayBody, _ := json.Marshal(eventsReplay)
	jobReplayBody, _ := json.Marshal(jobReplay)
	if !bytes.Equal(threadExactBody, threadReplayBody) || !bytes.Equal(eventsExactBody, eventsReplayBody) ||
		!bytes.Equal(jobExact, jobReplayBody) || fixture.provider.Count() != 1 {
		t.Fatalf("exact starting recovery replay changed state: thread=%t events=%t job=%t provider=%d",
			bytes.Equal(threadExactBody, threadReplayBody), bytes.Equal(eventsExactBody, eventsReplayBody),
			bytes.Equal(jobExact, jobReplayBody), fixture.provider.Count())
	}
}

func TestStartupRecoveryAutoContinueCrashAfterTurnBeforeStatusNeverRestartsProvider(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_crash_after_turn")
	admitted := admitBackgroundAutoContinueCompletionV1(t, fixture)
	starter := fixture.handler.runtimeBackgroundDeliveryService().AutoContinueStarter
	turnID, err := starter.NewTurnID(admitted)
	if err != nil {
		t.Fatal(err)
	}
	reserved, won, err := fixture.manager.ReserveBackgroundAutoContinueV1(
		admitted.ID, turnID,
		func(current domainjob.Record, records []domainjob.Record) string {
			return subagentapp.BackgroundAutoContinueSiblingGate(current, records)
		},
	)
	if err != nil || !won {
		t.Fatalf("reserve auto-continue: record=%#v won=%t err=%v", reserved, won, err)
	}
	startedTurnID, err := starter.StartReservedTurn(context.Background(), reserved)
	if err != nil || startedTurnID != turnID {
		t.Fatalf("start reserved turn: id=%q err=%v", startedTurnID, err)
	}
	waitBackgroundAutoContinueFinalizedV1(t, fixture, turnID)
	crashRecord, err := fixture.manager.LoadChildRun(admitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if crashRecord.AutoContinueStatus != "starting" || fixture.provider.Count() != 1 {
		t.Fatalf("crash-cut fixture mismatch: record=%#v provider=%d", crashRecord, fixture.provider.Count())
	}
	threadBeforeRecovery, _ := fixture.store.GetThread(admitted.ParentThreadID)
	if len(listAny(threadBeforeRecovery["turns"])) != 2 {
		t.Fatalf("crash-cut turn count=%d want=2", len(listAny(threadBeforeRecovery["turns"])))
	}
	recovery := runtimeStartupRecoveryForTest(fixture.handler)
	if err := recovery.RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	settled, err := fixture.manager.LoadChildRun(admitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	threadExact, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsExact, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	if settled.AutoContinueStatus != "started" || settled.AutoContinueTurnID != turnID ||
		len(listAny(threadExact["turns"])) != 2 || fixture.provider.Count() != 1 {
		t.Fatalf("existing turn crash recovery reran work: record=%#v turns=%d provider=%d",
			settled, len(listAny(threadExact["turns"])), fixture.provider.Count())
	}
	threadExactBody, _ := json.Marshal(threadExact)
	eventsExactBody, _ := json.Marshal(eventsExact)
	jobExactBody, _ := json.Marshal(settled)
	if err := recovery.RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	threadReplay, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsReplay, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	jobReplay, _ := fixture.manager.LoadChildRun(admitted.ID)
	threadReplayBody, _ := json.Marshal(threadReplay)
	eventsReplayBody, _ := json.Marshal(eventsReplay)
	jobReplayBody, _ := json.Marshal(jobReplay)
	if !bytes.Equal(threadExactBody, threadReplayBody) || !bytes.Equal(eventsExactBody, eventsReplayBody) ||
		!bytes.Equal(jobExactBody, jobReplayBody) || fixture.provider.Count() != 1 {
		t.Fatalf("exact existing-turn replay changed state: thread=%t events=%t job=%t provider=%d",
			bytes.Equal(threadExactBody, threadReplayBody), bytes.Equal(eventsExactBody, eventsReplayBody),
			bytes.Equal(jobExactBody, jobReplayBody), fixture.provider.Count())
	}
}

func TestBackgroundJobAutoContinueLostStartAckInspectsExactTurnAndSettlesStarted(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_lost_start_ack")
	admitted := admitBackgroundAutoContinueCompletionV1(t, fixture)
	delivery := fixture.handler.runtimeBackgroundDeliveryService()
	const privateLostAck = "PRIVATE_LOST_START_ACK_/Users/private_13800138000"
	runtimeStarter, ok := delivery.AutoContinueStarter.(subagentapp.BackgroundAutoContinueRuntimeStarterV1)
	if !ok {
		t.Fatalf("runtime starter type=%T", delivery.AutoContinueStarter)
	}
	runtimeStarter.Send = func(ctx context.Context, request controlapp.StartTurnRequest) (map[string]any, error) {
		response, err := fixture.handler.runtimeControl().SendTurn(ctx, request)
		if err != nil {
			return response, err
		}
		return response, errors.New(privateLostAck)
	}
	delivery.AutoContinueStarter = runtimeStarter
	delivery.MaybeAutoContinue(admitted, admitted.Status, "background_job_completed")

	started, err := fixture.manager.LoadChildRun(admitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.AutoContinueStatus != "started" || started.AutoContinueTurnID == "" ||
		started.AutoContinueReason != "" {
		t.Fatalf("lost start acknowledgement was not reconciled as started: %#v", started)
	}
	waitBackgroundAutoContinueFinalizedV1(t, fixture, started.AutoContinueTurnID)
	if fixture.provider.Count() != 1 {
		t.Fatalf("lost acknowledgement provider count=%d want=1", fixture.provider.Count())
	}
	threadExact, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	eventsExact, err := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	threadExactBody, _ := json.Marshal(threadExact)
	eventsExactBody, _ := json.Marshal(eventsExact)
	jobExactBody, _ := json.Marshal(started)
	if err := delivery.RecoverAutoContinue(started); err != nil {
		t.Fatal(err)
	}
	threadReplay, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsReplay, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	jobReplay, _ := fixture.manager.LoadChildRun(admitted.ID)
	threadReplayBody, _ := json.Marshal(threadReplay)
	eventsReplayBody, _ := json.Marshal(eventsReplay)
	jobReplayBody, _ := json.Marshal(jobReplay)
	if !bytes.Equal(threadExactBody, threadReplayBody) || !bytes.Equal(eventsExactBody, eventsReplayBody) ||
		!bytes.Equal(jobExactBody, jobReplayBody) || len(listAny(threadReplay["turns"])) != 2 ||
		fixture.provider.Count() != 1 {
		t.Fatalf("lost acknowledgement replay changed exact state: thread=%t events=%t job=%t turns=%d provider=%d",
			bytes.Equal(threadExactBody, threadReplayBody), bytes.Equal(eventsExactBody, eventsReplayBody),
			bytes.Equal(jobExactBody, jobReplayBody), len(listAny(threadReplay["turns"])), fixture.provider.Count())
	}
	for name, body := range map[string][]byte{
		"job": jobReplayBody, "thread": threadReplayBody, "events": eventsReplayBody,
	} {
		if bytes.Contains(body, []byte(privateLostAck)) || bytes.Contains(body, []byte("13800138000")) {
			t.Fatalf("%s reflected private lost-ack error: %s", name, body)
		}
	}
}

func TestBackgroundJobAutoContinueLostStartAckInspectionErrorKeepsStartingUntilExactRecovery(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_lost_ack_inspect_error")
	admitted := admitBackgroundAutoContinueCompletionV1(t, fixture)
	delivery := fixture.handler.runtimeBackgroundDeliveryService()
	runtimeStarter, ok := delivery.AutoContinueStarter.(subagentapp.BackgroundAutoContinueRuntimeStarterV1)
	if !ok {
		t.Fatalf("runtime starter type=%T", delivery.AutoContinueStarter)
	}
	const privateLostAck = "PRIVATE_LOST_ACK_WITH_INSPECT_ERROR_/Users/private_13800138000"
	runtimeStarter.Send = func(ctx context.Context, request controlapp.StartTurnRequest) (map[string]any, error) {
		response, err := fixture.handler.runtimeControl().SendTurn(ctx, request)
		if err != nil {
			return response, err
		}
		return response, errors.New(privateLostAck)
	}
	loadThread := runtimeStarter.LoadThread
	inspectCalls := 0
	runtimeStarter.LoadThread = func(threadID string) (map[string]any, error) {
		inspectCalls++
		if inspectCalls == 1 {
			return nil, errors.New("PRIVATE_TRANSIENT_STORE_READ_/Users/private")
		}
		return loadThread(threadID)
	}
	delivery.AutoContinueStarter = runtimeStarter
	delivery.MaybeAutoContinue(admitted, admitted.Status, "background_job_completed")

	starting, err := fixture.manager.LoadChildRun(admitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if starting.AutoContinueStatus != "starting" || starting.AutoContinueTurnID == "" ||
		starting.AutoContinueReason != "" {
		t.Fatalf("indeterminate inspection did not preserve durable starting: %#v", starting)
	}
	threadStarting, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	parentTurn, ok := subagentapp.FindTurn(threadStarting, admitted.ParentTurnID)
	if !ok {
		t.Fatal("parent turn disappeared after indeterminate inspection")
	}
	for _, raw := range listAny(parentTurn["items"]) {
		item, _ := raw.(map[string]any)
		arguments, _ := item["arguments"].(map[string]any)
		diagnostics, _ := arguments["diagnostics"].(map[string]any)
		if stringField(diagnostics, "notificationKind") == "background_job_auto_continue" {
			t.Fatalf("indeterminate inspection appended an auto-continue notice: %#v", item)
		}
	}
	waitBackgroundAutoContinueFinalizedV1(t, fixture, starting.AutoContinueTurnID)
	if err := delivery.RecoverAutoContinue(starting); err != nil {
		t.Fatal(err)
	}
	started, err := fixture.manager.LoadChildRun(admitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	threadExact, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsExact, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	threadExactBody, _ := json.Marshal(threadExact)
	eventsExactBody, _ := json.Marshal(eventsExact)
	jobExactBody, _ := json.Marshal(started)
	if started.AutoContinueStatus != "started" || len(listAny(threadExact["turns"])) != 2 ||
		fixture.provider.Count() != 1 {
		t.Fatalf("exact recovery did not settle indeterminate start once: record=%#v turns=%d provider=%d",
			started, len(listAny(threadExact["turns"])), fixture.provider.Count())
	}
	if err := delivery.RecoverAutoContinue(started); err != nil {
		t.Fatal(err)
	}
	threadReplay, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsReplay, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	jobReplay, _ := fixture.manager.LoadChildRun(admitted.ID)
	threadReplayBody, _ := json.Marshal(threadReplay)
	eventsReplayBody, _ := json.Marshal(eventsReplay)
	jobReplayBody, _ := json.Marshal(jobReplay)
	if !bytes.Equal(threadExactBody, threadReplayBody) || !bytes.Equal(eventsExactBody, eventsReplayBody) ||
		!bytes.Equal(jobExactBody, jobReplayBody) || fixture.provider.Count() != 1 {
		t.Fatalf("indeterminate exact recovery replay changed state: thread=%t events=%t job=%t provider=%d",
			bytes.Equal(threadExactBody, threadReplayBody), bytes.Equal(eventsExactBody, eventsReplayBody),
			bytes.Equal(jobExactBody, jobReplayBody), fixture.provider.Count())
	}
	for name, body := range map[string][]byte{
		"job": jobReplayBody, "thread": threadReplayBody, "events": eventsReplayBody,
	} {
		if bytes.Contains(body, []byte(privateLostAck)) || bytes.Contains(body, []byte("PRIVATE_TRANSIENT_STORE_READ")) ||
			bytes.Contains(body, []byte("13800138000")) {
			t.Fatalf("%s reflected private transient error: %s", name, body)
		}
	}
}

func TestBackgroundJobAutoContinueSiblingConflictNeverMutatesParentOrStartsTurn(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_sibling_conflict")
	first := admitBackgroundAutoContinueCompletionV1(t, fixture)
	second, err := fixture.manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: first.ParentGoalID, ParentThreadID: first.ParentThreadID, ParentTurnID: first.ParentTurnID,
		ParentToolItemID: first.ParentToolItemID, ParentToolCallID: first.ParentToolCallID,
		ChildThreadID: "thr_child_auto_2", ChildTurnID: "turn_child_auto_2", Kind: "subagent",
		Label: "Second background child", Status: "completed", Background: true, AutoContinueParent: true,
		ProviderID: "analytix-hub", Model: "test-model", SecurityBinding: first.SecurityBinding,
	})
	if err != nil {
		t.Fatal(err)
	}
	thread, err := fixture.store.GetThread(first.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	itemID, callID, toolName, ok := subagentapp.JobToolIdentity(thread, second)
	if !ok {
		t.Fatal("second sibling parent tool identity is unavailable")
	}
	buildEvents := func(current domainjob.Record) []map[string]any {
		return subagentapp.BuildDurableJobLifecycleEventsV1(
			current.ParentThreadID, current.ParentTurnID, itemID, callID, toolName, current,
		)
	}
	second, ok, err = subagentapp.AdmitAndPublishBackgroundCompletionLifecycleV1(
		fixture.handler.runtimeBackgroundDeliveryService(), fixture.store,
		second.ParentThreadID, second.ParentTurnID, callID, toolName, second, buildEvents,
	)
	if err != nil || !ok {
		t.Fatalf("admit second sibling: record=%#v ok=%t err=%v", second, ok, err)
	}
	firstTurnID, err := fixture.handler.runtimeBackgroundDeliveryService().AutoContinueStarter.NewTurnID(first)
	if err != nil {
		t.Fatal(err)
	}
	first, won, err := fixture.manager.ReserveBackgroundAutoContinueV1(
		first.ID, firstTurnID,
		func(current domainjob.Record, records []domainjob.Record) string {
			return subagentapp.BackgroundAutoContinueSiblingGate(current, records)
		},
	)
	if err != nil || !won || first.AutoContinueStatus != "starting" {
		t.Fatalf("reserve first sibling: record=%#v won=%t err=%v", first, won, err)
	}
	threadBefore, _ := fixture.store.GetThread(first.ParentThreadID)
	eventsBefore, _ := fixture.store.eventLog.LoadSince(first.ParentThreadID, 0)
	threadBeforeBody, _ := json.Marshal(threadBefore)
	eventsBeforeBody, _ := json.Marshal(eventsBefore)
	delivery := fixture.handler.runtimeBackgroundDeliveryService()
	delivery.MaybeAutoContinue(second, second.Status, "")
	second, err = fixture.manager.LoadChildRun(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.AutoContinueStatus != "skipped" || second.AutoContinueReason != "auto_continue_already_starting" {
		t.Fatalf("sibling conflict was not closed: %#v", second)
	}
	delivery.MaybeAutoContinue(second, second.Status, "duplicate")
	if err := delivery.RecoverAutoContinue(second); err != nil {
		t.Fatal(err)
	}
	threadAfter, _ := fixture.store.GetThread(first.ParentThreadID)
	eventsAfter, _ := fixture.store.eventLog.LoadSince(first.ParentThreadID, 0)
	threadAfterBody, _ := json.Marshal(threadAfter)
	eventsAfterBody, _ := json.Marshal(eventsAfter)
	if !bytes.Equal(threadBeforeBody, threadAfterBody) || !bytes.Equal(eventsBeforeBody, eventsAfterBody) ||
		len(listAny(threadAfter["turns"])) != 1 || fixture.provider.Count() != 0 {
		t.Fatalf("sibling conflict gained side effects: thread=%t events=%t turns=%d provider=%d",
			bytes.Equal(threadBeforeBody, threadAfterBody), bytes.Equal(eventsBeforeBody, eventsAfterBody),
			len(listAny(threadAfter["turns"])), fixture.provider.Count())
	}
}

func TestBackgroundJobAutoContinueReservedCurrentDriftRejectsBeforeCompactionOrAppend(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_reserved_drift")
	admitted := admitBackgroundAutoContinueCompletionV1(t, fixture)
	starter := fixture.handler.runtimeBackgroundDeliveryService().AutoContinueStarter
	turnID, err := starter.NewTurnID(admitted)
	if err != nil {
		t.Fatal(err)
	}
	reserved, won, err := fixture.manager.ReserveBackgroundAutoContinueV1(
		admitted.ID, turnID,
		func(current domainjob.Record, records []domainjob.Record) string {
			return subagentapp.BackgroundAutoContinueSiblingGate(current, records)
		},
	)
	if err != nil || !won {
		t.Fatalf("reserve current-drift turn: record=%#v won=%t err=%v", reserved, won, err)
	}
	thread, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := stringField(thread, "workspace")
	now := time.Now().UTC().Add(time.Minute)
	newContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: admitted.ParentThreadID, TurnID: "turn_new_current", WorkspaceRealPath: workspace,
		ContextEpoch: 2, IssuedAt: now,
	})
	if err := fixture.store.AppendTurnToThread(admitted.ParentThreadID, map[string]any{
		"id": "turn_new_current", "threadId": admitted.ParentThreadID, "status": "completed",
		"createdAt": now.Format(time.RFC3339Nano), "securityContext": turnsecurityapp.PublicRecord(newContext), "items": []any{},
	}, "analytix-hub", map[string]any{"securityState": turnsecurityapp.PublicRecord(newContext)}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.PatchThread(admitted.ParentThreadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	threadBefore, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsBefore, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	threadBeforeBody, _ := json.Marshal(threadBefore)
	eventsBeforeBody, _ := json.Marshal(eventsBefore)
	if _, err := starter.StartReservedTurn(context.Background(), reserved); err == nil ||
		!strings.Contains(err.Error(), "background auto-continue reserved turn already exists or conflicts") {
		t.Fatalf("reserved current drift did not fail in preflight: %v", err)
	}
	threadAfter, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsAfter, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	threadAfterBody, _ := json.Marshal(threadAfter)
	eventsAfterBody, _ := json.Marshal(eventsAfter)
	if !bytes.Equal(threadBeforeBody, threadAfterBody) || !bytes.Equal(eventsBeforeBody, eventsAfterBody) ||
		len(listAny(threadAfter["turns"])) != 2 || fixture.provider.Count() != 0 {
		t.Fatalf("preflight drift caused compaction/append/provider side effects: thread=%t events=%t turns=%d provider=%d",
			bytes.Equal(threadBeforeBody, threadAfterBody), bytes.Equal(eventsBeforeBody, eventsAfterBody),
			len(listAny(threadAfter["turns"])), fixture.provider.Count())
	}
}

func TestBackgroundJobAutoContinueReservedCarrierRejectsEveryExtraFieldBeforeSideEffects(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_hostile_carrier")
	admitted := admitBackgroundAutoContinueCompletionV1(t, fixture)
	starter := fixture.handler.runtimeBackgroundDeliveryService().AutoContinueStarter
	turnID, err := starter.NewTurnID(admitted)
	if err != nil {
		t.Fatal(err)
	}
	reserved, won, err := fixture.manager.ReserveBackgroundAutoContinueV1(
		admitted.ID, turnID,
		func(current domainjob.Record, records []domainjob.Record) string {
			return subagentapp.BackgroundAutoContinueSiblingGate(current, records)
		},
	)
	if err != nil || !won {
		t.Fatalf("reserve hostile-carrier turn: record=%#v won=%t err=%v", reserved, won, err)
	}
	base := controlapp.StartTurnRequest{
		ThreadID: admitted.ParentThreadID, Prompt: subagentapp.BackgroundAutoContinuePromptV1, Async: true,
		InternalUsageSource: appusage.SourceTurn, InternalTurnID: turnID,
		InternalAutoContinueJobID: admitted.ID, InternalAutoContinueParentTurnID: admitted.ParentTurnID,
	}
	steps := 1
	tests := []struct {
		name   string
		mutate func(*controlapp.StartTurnRequest)
	}{
		{"display text", func(value *controlapp.StartTurnRequest) { value.DisplayText = "private" }},
		{"risk intent", func(value *controlapp.StartTurnRequest) { value.RiskIntent = "case" }},
		{"model", func(value *controlapp.StartTurnRequest) { value.Model = "private-model" }},
		{"provider", func(value *controlapp.StartTurnRequest) { value.ProviderID = "private-provider" }},
		{"endpoint", func(value *controlapp.StartTurnRequest) { value.EndpointFormat = "responses" }},
		{"effort", func(value *controlapp.StartTurnRequest) { value.ReasoningEffort = "high" }},
		{"mode", func(value *controlapp.StartTurnRequest) { value.Mode = "plan" }},
		{"approval", func(value *controlapp.StartTurnRequest) { value.ApprovalPolicy = "never" }},
		{"sandbox", func(value *controlapp.StartTurnRequest) { value.SandboxMode = "danger-full-access" }},
		{"attachment", func(value *controlapp.StartTurnRequest) { value.AttachmentIDs = []string{"private"} }},
		{"file reference", func(value *controlapp.StartTurnRequest) {
			value.FileReferences = []any{map[string]any{"path": "/Users/private"}}
		}},
		{"gui plan", func(value *controlapp.StartTurnRequest) { value.GUIPlan = map[string]any{"private": true} }},
		{"disable user input", func(value *controlapp.StartTurnRequest) { value.DisableUserInput = true }},
		{"disable user input set", func(value *controlapp.StartTurnRequest) { value.DisableUserInputSet = true }},
		{"checkpoint", func(value *controlapp.StartTurnRequest) { value.WorkspaceCheckpointID = "private" }},
		{"model steps", func(value *controlapp.StartTurnRequest) { value.MaxModelSteps = &steps }},
		{"tool scope", func(value *controlapp.StartTurnRequest) { value.InternalToolScope = []string{"bash"} }},
		{"subagent depth", func(value *controlapp.StartTurnRequest) { value.InternalSubagentDepth = 1 }},
		{"usage source", func(value *controlapp.StartTurnRequest) { value.InternalUsageSource = "private" }},
		{"system prompt", func(value *controlapp.StartTurnRequest) { value.InternalSystemPrompt = "private" }},
		{"child run", func(value *controlapp.StartTurnRequest) { value.InternalChildRunID = "private" }},
		{"output budget", func(value *controlapp.StartTurnRequest) { value.InternalOutputTokenBudget = 1 }},
	}
	threadBefore, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsBefore, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	threadBeforeBody, _ := json.Marshal(threadBefore)
	eventsBeforeBody, _ := json.Marshal(eventsBefore)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			if _, err := fixture.handler.runtimeControl().SendTurn(context.Background(), request); err == nil ||
				!strings.Contains(err.Error(), "internal reserved turn carrier is invalid") {
				t.Fatalf("hostile reserved carrier reached admission: %v", err)
			}
		})
	}
	threadAfter, _ := fixture.store.GetThread(admitted.ParentThreadID)
	eventsAfter, _ := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	threadAfterBody, _ := json.Marshal(threadAfter)
	eventsAfterBody, _ := json.Marshal(eventsAfter)
	if !bytes.Equal(threadBeforeBody, threadAfterBody) || !bytes.Equal(eventsBeforeBody, eventsAfterBody) ||
		len(listAny(threadAfter["turns"])) != 1 || fixture.provider.Count() != 0 {
		t.Fatalf("hostile carrier caused compaction/append/provider side effects: thread=%t events=%t turns=%d provider=%d",
			bytes.Equal(threadBeforeBody, threadAfterBody), bytes.Equal(eventsBeforeBody, eventsAfterBody),
			len(listAny(threadAfter["turns"])), fixture.provider.Count())
	}
}

func TestBackgroundJobAutoContinueRejectedConcurrentLiveRecoveryNeverMutatesParent(t *testing.T) {
	store, manager, record, provider := backgroundRecoveryExactFixture(t, true)
	updated, err := manager.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		AutoContinueStatus: "skipped", AutoContinueReason: "child_completion_receipt_required",
	})
	if err != nil {
		t.Fatal(err)
	}
	authority := subagentapp.JobSecurityAuthorizer{Threads: store, Jobs: manager}
	delivery := subagentapp.BackgroundDeliveryService{
		Threads: store, Jobs: manager, SecurityBlocker: authority.Blocker, SecurityBlockerAt: authority.BlockerAt,
	}
	const workers = 32
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			if index%2 == 0 {
				delivery.MaybeAutoContinue(updated, updated.Status, "background_job_completed")
				errs <- nil
				return
			}
			errs <- delivery.RepairAutoContinueNotice(updated)
		}(index)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	thread, err := store.GetThread(record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _ := subagentapp.FindTurn(thread, record.ParentTurnID)
	count := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		args, _ := item["arguments"].(map[string]any)
		diagnostics, _ := args["diagnostics"].(map[string]any)
		if stringField(diagnostics, "notificationKind") == "background_job_auto_continue" {
			count++
		}
	}
	if count != 0 || provider.Count() != 0 {
		t.Fatalf("concurrent auto notice count=%d provider=%d", count, provider.Count())
	}
}

func TestStartupRecoveryRejectedAutoContinueDoesNotMutateParent(t *testing.T) {
	store, manager, record, provider := backgroundRecoveryExactFixture(t, true)
	deliveryID := "delivery_" + record.ParentTurnID + "_" + record.ID
	updated, err := manager.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		AutoContinueStatus: "skipped", AutoContinueReason: "child_completion_receipt_required",
		CompletionDeliveryID: deliveryID, CompletionDeliveryItemID: "item_completion_" + record.ID,
		CompletionDeliveryStatus: "skipped", CompletionDeliveryReason: "parent_turn_terminal",
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(updated)
	handler := &runtimeServerHandler{store: store, jobs: manager, provider: provider}
	if err := runtimeStartupRecoveryForTest(handler).RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	after, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterBody, _ := json.Marshal(after)
	if !bytes.Equal(before, afterBody) {
		t.Fatalf("notice-only restart changed durable job: before=%s after=%s", before, afterBody)
	}
	thread, err := store.GetThread(record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	turn, _ := subagentapp.FindTurn(thread, record.ParentTurnID)
	autoCount := 0
	for _, raw := range listAny(turn["items"]) {
		item, _ := raw.(map[string]any)
		args, _ := item["arguments"].(map[string]any)
		diagnostics, _ := args["diagnostics"].(map[string]any)
		if stringField(diagnostics, "notificationKind") == "background_job_auto_continue" {
			autoCount++
		}
	}
	if autoCount != 0 || provider.Count() != 0 {
		t.Fatalf("restart auto repair count=%d provider=%d", autoCount, provider.Count())
	}
}

func TestStartupRecoveryRepairsSettledDeliveryLedgerWithoutChangingJob(t *testing.T) {
	for _, status := range []string{"delivered", "skipped", "dead_letter"} {
		t.Run(status, func(t *testing.T) {
			store, manager, record, provider := backgroundRecoveryExactFixture(t, false)
			deliveryID := "delivery_" + record.ParentTurnID + "_" + record.ID
			handler := &runtimeServerHandler{store: store, jobs: manager, provider: provider}
			var updated jobs.Record
			var err error
			if status == "delivered" {
				thread, err := store.GetThread(record.ParentThreadID)
				if err != nil {
					t.Fatal(err)
				}
				itemID, callID, toolName, ok := subagentapp.JobToolIdentity(thread, record)
				if !ok {
					t.Fatal("delivered settled fixture parent identity is invalid")
				}
				events := subagentapp.BuildDurableJobLifecycleEventsV1(
					record.ParentThreadID, record.ParentTurnID, itemID, callID, toolName, record,
				)
				var admitted bool
				updated, admitted, err = subagentapp.AdmitBackgroundCompletionLifecycleV1(
					handler.runtimeBackgroundDeliveryService(), record.ParentThreadID, record.ParentTurnID,
					callID, toolName, events,
				)
				if err != nil || !admitted {
					t.Fatalf("build complete delivered settled fixture: record=%#v ok=%t err=%v", updated, admitted, err)
				}
				deliveryID = updated.CompletionDeliveryID
			} else {
				request := domainjob.UpdateRequest{
					CompletionDeliveryID: deliveryID, CompletionDeliveryItemID: "item_completion_" + record.ID,
					CompletionDeliveryStatus: status,
				}
				if status == "skipped" {
					request.CompletionDeliveryReason = "parent_turn_terminal"
				} else {
					request.CompletionDeliveryReason = "append_parent_turn_failed"
				}
				updated, err = manager.UpdateChildRun(record.ID, request)
				if err != nil {
					t.Fatal(err)
				}
			}
			before, _ := json.Marshal(updated)
			recovery := runtimeStartupRecoveryForTest(handler)
			if err := recovery.RecoverPendingDeliveries(); err != nil {
				t.Fatal(err)
			}
			eventsAfterInsert, err := store.eventLog.LoadSince(record.ParentThreadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := recovery.RecoverPendingDeliveries(); err != nil {
				t.Fatal(err)
			}
			eventsAfterReplay, err := store.eventLog.LoadSince(record.ParentThreadID, 0)
			if err != nil {
				t.Fatal(err)
			}
			insertBody, _ := json.Marshal(eventsAfterInsert)
			replayBody, _ := json.Marshal(eventsAfterReplay)
			if !bytes.Equal(insertBody, replayBody) {
				t.Fatalf("settled exact replay emitted an event: first=%s replay=%s", insertBody, replayBody)
			}
			matched := backgroundCompletionLifecycleEventsForJob(eventsAfterReplay.Events, record.ID)
			if status == "delivered" {
				if len(matched) != 1 {
					t.Fatalf("delivered startup lifecycle count=%d: %#v", len(matched), eventsAfterReplay.Events)
				}
				assertCanonicalBackgroundLifecycleBundle(t, eventsAfterReplay.Events, updated)
			} else if len(matched) != 0 {
				t.Fatalf("settled %s incorrectly published a completion lifecycle: %#v", status, matched)
			}
			after, err := manager.LoadChildRun(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			afterBody, _ := json.Marshal(after)
			if !bytes.Equal(before, afterBody) {
				t.Fatalf("settled repair changed durable job: before=%s after=%s", before, afterBody)
			}
			thread, err := store.GetThread(record.ParentThreadID)
			if err != nil {
				t.Fatal(err)
			}
			turn, _ := subagentapp.FindTurn(thread, record.ParentTurnID)
			wantedID := subagentapp.BackgroundCompletionDeliveryItemID(record.ParentTurnID, deliveryID, status)
			ledgerCount := 0
			for _, raw := range listAny(turn["items"]) {
				item, _ := raw.(map[string]any)
				if stringField(item, "id") == wantedID {
					ledgerCount++
				}
			}
			if ledgerCount != 1 || provider.Count() != 0 {
				t.Fatalf("settled %s ledger count=%d provider=%d", status, ledgerCount, provider.Count())
			}
		})
	}
}

func TestStartupRecoverySettledLedgerConflictDoesNotChangeProtectedStateOrEvents(t *testing.T) {
	store, manager, record, provider := backgroundRecoveryExactFixture(t, true)
	deliveryID := "delivery_" + record.ParentTurnID + "_" + record.ID
	updated, err := manager.UpdateChildRun(record.ID, domainjob.UpdateRequest{
		CompletionDeliveryID: deliveryID, CompletionDeliveryItemID: "item_completion_" + record.ID,
		CompletionDeliveryStatus: "delivered",
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, won, err := manager.ReserveBackgroundAutoContinueV1(
		updated.ID, "turn_999",
		func(domainjob.Record, []domainjob.Record) string { return "" },
	)
	if err != nil || !won {
		t.Fatalf("reserve conflict fixture: record=%#v won=%t err=%v", updated, won, err)
	}
	updated, err = manager.UpdateChildRun(updated.ID, domainjob.UpdateRequest{
		AutoContinueStatus: "started", AutoContinueTurnID: updated.AutoContinueTurnID,
	})
	if err != nil {
		t.Fatal(err)
	}
	itemID := subagentapp.BackgroundCompletionDeliveryItemID(record.ParentTurnID, deliveryID, "delivered")
	expected, ok := subagentapp.BuildBackgroundJobDeliveryLedgerItem(subagentapp.BackgroundJobDeliveryLedgerItemInput{
		ThreadID: record.ParentThreadID, TurnID: record.ParentTurnID, ItemID: itemID,
		CallID:    subagentapp.BackgroundCompletionDeliveryCallID(deliveryID, "delivered"),
		CreatedAt: updated.CompletionDeliveryAt, Record: updated, Status: "delivered",
	})
	if !ok {
		t.Fatal("build settled ledger")
	}
	conflict := cloneMap(expected)
	conflict["createdAt"] = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	if err := store.AppendItemToTurn(record.ParentThreadID, record.ParentTurnID, conflict); err != nil {
		t.Fatal(err)
	}
	threadBefore, err := store.GetThread(record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	threadBeforeBody, _ := json.Marshal(threadBefore)
	jobBeforeBody, _ := json.Marshal(updated)
	eventsBefore, err := store.eventLog.LoadSince(record.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventsBeforeBody, _ := json.Marshal(eventsBefore)
	handler := &runtimeServerHandler{store: store, jobs: manager, provider: provider}
	if err := runtimeStartupRecoveryForTest(handler).RecoverPendingDeliveries(); err == nil ||
		!strings.Contains(err.Error(), "identity conflicts") {
		t.Fatalf("settled ledger conflict did not fail closed: %v", err)
	}
	threadAfter, err := store.GetThread(record.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	threadAfterBody, _ := json.Marshal(threadAfter)
	jobAfter, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	jobAfterBody, _ := json.Marshal(jobAfter)
	eventsAfter, err := store.eventLog.LoadSince(record.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	eventsAfterBody, _ := json.Marshal(eventsAfter)
	if !bytes.Equal(threadBeforeBody, threadAfterBody) || !bytes.Equal(jobBeforeBody, jobAfterBody) ||
		!bytes.Equal(eventsBeforeBody, eventsAfterBody) || provider.Count() != 0 {
		t.Fatalf("settled conflict changed protected state: thread=%t job=%t events=%t provider=%d",
			bytes.Equal(threadBeforeBody, threadAfterBody), bytes.Equal(jobBeforeBody, jobAfterBody),
			bytes.Equal(eventsBeforeBody, eventsAfterBody), provider.Count())
	}
}

func backgroundRecoveryExactFixture(
	t *testing.T,
	autoContinue bool,
) (*DurableEventSessionStore, *jobs.Manager, domainjob.Record, *backgroundAutoContinueProvider) {
	t.Helper()
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"id": "thr_background_exact", "title": "Background exact", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	threadID := stringField(thread, "id")
	turnID := "turn_background_exact"
	parent := appendServerBoundJobParent(t, store, threadID, turnID, workspace, "subagent")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID: "goal-background-exact", ParentThreadID: threadID, ParentTurnID: turnID,
		ParentToolItemID: parent.ItemID, ParentToolCallID: parent.CallID,
		ChildThreadID: "child-thread-private-long-reference", ChildTurnID: "child-turn-private-long-reference",
		Kind: "subagent", Label: "Caller 13800138000 /Users/private/case 6222020000000000000",
		Status: "completed", Background: true, AutoContinueParent: autoContinue, SecurityBinding: parent.Binding,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, manager, record, &backgroundAutoContinueProvider{}
}

func TestBackgroundJobAutoContinueDoesNotConsumeGateBeforeTerminalState(t *testing.T) {
	store, err := NewTempDurableEventSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("create durable store: %v", err)
	}
	workspace := workspacetest.New(t)
	thread, err := store.CreateThread(map[string]any{"id": "thr_auto_pending", "title": "Auto pending", "workspace": workspace}, workspace)
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := stringField(thread, "id")
	manager, err := jobs.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("create job manager: %v", err)
	}
	record, err := manager.StartChildRun(jobs.StartRequest{
		ParentGoalID:       "goal_auto",
		ParentThreadID:     threadID,
		ParentTurnID:       "turn_parent",
		Kind:               "subagent",
		Status:             "queued",
		Background:         true,
		AutoContinueParent: true,
	})
	if err != nil {
		t.Fatalf("start child run: %v", err)
	}
	handler := &runtimeServerHandler{store: store, jobs: manager}
	handler.runtimeBackgroundDeliveryService().MaybeAutoContinue(record, "queued", "")
	updated, err := manager.LoadChildRun(record.ID)
	if err != nil {
		t.Fatalf("load child run: %v", err)
	}
	if updated.AutoContinueStatus != "" {
		t.Fatalf("non-terminal progress should not consume auto-continue gate: %#v", updated)
	}
}

func TestBackgroundJobAutoContinueRejectsStaleParentSecurityBinding(t *testing.T) {
	fixture := newBackgroundAutoContinueServerFixtureV1(t, "thr_auto_skip")
	admitted := admitBackgroundAutoContinueCompletionV1(t, fixture)
	thread, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := stringField(thread, "workspace")
	now := time.Now().UTC().Add(time.Minute)
	newContext := newServerGeneralContextV2(t, domainsecurity.TurnSecurityContextInput{
		ThreadID: admitted.ParentThreadID, TurnID: "turn_new", WorkspaceRealPath: workspace, ContextEpoch: 2, IssuedAt: now,
	})
	if err := fixture.store.AppendTurnToThread(admitted.ParentThreadID, map[string]any{
		"id": "turn_new", "threadId": admitted.ParentThreadID, "status": "completed", "createdAt": now.Format(time.RFC3339Nano),
		"securityContext": turnsecurityapp.PublicRecord(newContext), "items": []any{},
	}, "analytix-hub", map[string]any{"securityState": turnsecurityapp.PublicRecord(newContext)}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.PatchThread(admitted.ParentThreadID, map[string]any{"status": "idle"}); err != nil {
		t.Fatal(err)
	}
	threadBefore, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	eventsBefore, err := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	threadBeforeBody, _ := json.Marshal(threadBefore)
	eventsBeforeBody, _ := json.Marshal(eventsBefore)
	fixture.handler.runtimeBackgroundDeliveryService().MaybeAutoContinue(admitted, "completed", "")
	updated, err := fixture.manager.LoadChildRun(admitted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AutoContinueStatus != "skipped" || updated.AutoContinueReason != "parent_security_context_mismatch" {
		t.Fatalf("expected stale parent security skip: %#v", updated)
	}
	fixture.handler.runtimeBackgroundDeliveryService().MaybeAutoContinue(updated, "completed", "")
	recovery := runtimeStartupRecoveryForTest(fixture.handler)
	if err := recovery.RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	if err := recovery.RecoverPendingDeliveries(); err != nil {
		t.Fatal(err)
	}
	threadAfter, err := fixture.store.GetThread(admitted.ParentThreadID)
	if err != nil {
		t.Fatal(err)
	}
	eventsAfter, err := fixture.store.eventLog.LoadSince(admitted.ParentThreadID, 0)
	if err != nil {
		t.Fatal(err)
	}
	threadAfterBody, _ := json.Marshal(threadAfter)
	eventsAfterBody, _ := json.Marshal(eventsAfter)
	if !bytes.Equal(threadBeforeBody, threadAfterBody) || !bytes.Equal(eventsBeforeBody, eventsAfterBody) ||
		len(listAny(threadAfter["turns"])) != 2 || fixture.provider.Count() != 0 {
		t.Fatalf("stale duplicate/restart changed parent: thread=%t events=%t turns=%d provider=%d",
			bytes.Equal(threadBeforeBody, threadAfterBody), bytes.Equal(eventsBeforeBody, eventsAfterBody),
			len(listAny(threadAfter["turns"])), fixture.provider.Count())
	}
}
