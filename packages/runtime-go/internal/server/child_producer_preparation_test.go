package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	caseentityadapter "analytix.local/runtime-go/internal/adapters/outbound/caseentity"
	caseentityapp "analytix.local/runtime-go/internal/app/caseentity"
	pendingworkapp "analytix.local/runtime-go/internal/app/pendingwork"
	sideeffectidentityapp "analytix.local/runtime-go/internal/app/sideeffectidentity"
	subagentapp "analytix.local/runtime-go/internal/app/subagent"
	turnapp "analytix.local/runtime-go/internal/app/turn"
	turnsecurityapp "analytix.local/runtime-go/internal/app/turnsecurity"
	domaincaseentity "analytix.local/runtime-go/internal/domain/caseentity"
	domaincontrolledaccount "analytix.local/runtime-go/internal/domain/controlledaccount"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainmodel "analytix.local/runtime-go/internal/domain/model"
	domainpendingwork "analytix.local/runtime-go/internal/domain/pendingwork"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	domaintoolcall "analytix.local/runtime-go/internal/domain/toolcall"
	currentdatasettest "analytix.local/runtime-go/internal/testsupport/currentdataset"
	privatecastest "analytix.local/runtime-go/internal/testsupport/privatecas"
	securitycontexttest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

func runtimeChildProducerPendingFixtureV1(t *testing.T, tool string, arguments json.RawMessage, caseRoot ...string) (*runtimeServerHandler, runtimePendingToolCall) {
	t.Helper()
	workspace, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := NewRuntimeServerHandler(RuntimeServerConfig{
		RuntimeToken: DefaultRuntimeToken, DurableTempDir: t.TempDir(), DataDir: t.TempDir(),
		ProviderID: "synthetic-provider", BaseURL: "http://127.0.0.1:18997/v1", APIKey: "synthetic-placeholder",
		EndpointFormat: "chat_completions", Model: "synthetic-model",
	}).(*runtimeServerHandler)
	h.subagents = subagentapp.ProfileSettings{Enabled: true, DefaultToolPolicy: "readOnly", MaxParallel: 2, MaxChildRuns: 8,
		Profiles: map[string]subagentapp.ProfileConfig{"reviewer": {Name: "reviewer", ToolPolicy: "readOnly", MaxSteps: 4, MaxStepsSet: true}}}
	thread, err := h.store.CreateThread(map[string]any{"title": "synthetic parent"}, workspace)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	contextInput := domainsecurity.TurnSecurityContextInput{
		ThreadID: stringField(thread, "id"), TurnID: "turn_100", WorkspaceRealPath: workspace, ContextEpoch: 1, IssuedAt: now,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID,
	}
	frozen := newServerGeneralContextV2(t, contextInput)
	if len(caseRoot) == 1 {
		frozen = runtimeChildProducerCaseContextV1(t, h, contextInput, caseRoot[0])
	}
	securityRecord := turnsecurityapp.PublicRecord(frozen)
	if err := h.store.AppendTurnToThread(frozen.ThreadID, map[string]any{"id": frozen.TurnID, "threadId": frozen.ThreadID, "status": "running", "createdAt": now.Format(time.RFC3339Nano), "securityContext": securityRecord, "items": []any{}}, "synthetic-provider", map[string]any{"securityState": securityRecord}); err != nil {
		t.Fatal(err)
	}
	h.turnSeq = 100
	call := domainmodel.ToolCall{ID: serverTestHostToolCallID("child-producer-" + tool), Name: tool, Arguments: arguments}
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: frozen, Provider: "synthetic-provider", ServerIdentity: "host:builtin", ToolName: tool, ToolCallID: call.ID,
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("synthetic-child-schema")), ScopeHash: domainsecurity.SHA256Hex([]byte("synthetic-child-scope")),
		ReadOnly: false, ApprovalState: "not_required", IssuedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	})
	itemID := domaintoolcall.ToolCallItemIDV1(frozen.TurnID, call.ID)
	item, _, err := turnapp.ToolCallReadyRecords(turnapp.ToolCallReadyInput{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, ItemID: itemID, CreatedAt: now.Format(time.RFC3339Nano), Call: call, ToolKind: "task", Context: frozen, Grant: grant})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.AppendItemToTurn(frozen.ThreadID, frozen.TurnID, item); err != nil {
		t.Fatal(err)
	}
	return h, runtimePendingToolCall{ThreadID: frozen.ThreadID, TurnID: frozen.TurnID, ToolCallItemID: itemID, ProviderID: "synthetic-provider", Model: "synthetic-model", Effort: "auto", Workspace: workspace, Call: call, SecurityContext: frozen, ExecutionGrant: grant}
}

func runtimeChildProducerCaseContextV1(t *testing.T, h *runtimeServerHandler, input domainsecurity.TurnSecurityContextInput, root string) domainsecurity.TurnSecurityContext {
	t.Helper()
	harness := currentdatasettest.NewHarness()
	input.CaseID, input.CaseBindingHash = "case-synthetic", strings.Repeat("a", 64)
	input.DatasetSnapshotID = securitycontexttest.DatasetSnapshotID("synthetic-child-dataset")
	input.SourceManifestHash = domainsecurity.SHA256Hex([]byte("synthetic-child-manifest"))
	input.ContextEpoch = 2
	frozen, err := harness.NewCaseContext(input)
	if err != nil {
		t.Fatal(err)
	}
	input.ThreadID, input.TurnID, input.ContextEpoch = "thread-origin", "turn-origin", 1
	origin, err := harness.NewCaseContext(input)
	if err != nil {
		t.Fatal(err)
	}
	access, err := privatecastest.NewAccessAuthority(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := caseentityadapter.NewStore(filepath.Join(root, "case-entity"), access)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	h.caseEntities = caseentityapp.NewPersistentService(serverCaseIngressKeyedDigesterV1{key: []byte("synthetic-child-installation-key")}, store, harness, harness, harness.ValidateCurrent)
	reference, err := h.caseEntities.BindReferenceV1(context.Background(), caseentityapp.NewDeriveReferenceInputV1(origin, domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, "6222020000000000017"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := domaincaseentity.NewCaseLongitudinalIndexRecordV1(domaincaseentity.ThreadCaseContextRecordInputV1{
		SecurityContext: origin, Generation: 1, EntityReferences: []domaincaseentity.ReferenceV1{reference},
		EntityIdentities: []domaincaseentity.CaseEntityIdentityStateV1{{Reference: reference, EntityType: domaincontrolledaccount.ControlledAccountFinancialFieldBankAccountNumberV1, StableOrdinal: 1}},
		Snapshots:        []domaincaseentity.CaseSnapshotStateV1{{DatasetSnapshotID: origin.DatasetSnapshotID, ContextEpoch: origin.ContextEpoch, Currentness: domaincaseentity.SnapshotCurrentV1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutThreadContextIfAbsent(context.Background(), index); err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestRuntimeChildCasePreparationDoesNotWriteBeforeReceipt(t *testing.T) {
	for _, rejectSibling := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid_task", true: "invalid_later_sibling"}[rejectSibling], func(t *testing.T) {
			tool, arguments := "task", json.RawMessage(`{"prompt":"inspect acct:1","profile":"reviewer"}`)
			if rejectSibling {
				tool, arguments = "parallel_tasks", json.RawMessage(`{"tasks":[{"id":"first","prompt":"inspect acct:1","profile":"reviewer"},{"id":"second","prompt":"inspect acct:1","profile":"missing-profile"}]}`)
			}
			root := t.TempDir()
			h, pending := runtimeChildProducerPendingFixtureV1(t, tool, arguments, root)
			before := runtimeRestoreFileDigestsV1(t, root)
			prepared, err := h.prepareRuntimeSideEffect(context.Background(), pending, time.Now().UTC())
			if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, root)) {
				t.Fatal("case alias preparation persisted before signed receipt or later sibling rejection")
			}
			if rejectSibling {
				if err == nil && prepared.Rejection == nil {
					t.Fatal("invalid later sibling was admitted")
				}
			} else if err != nil || prepared.Rejection != nil || prepared.ChildProducer == nil {
				t.Fatalf("valid case preparation failed: %v", err)
			}
			if len(h.jobs.AllRecords()) != 0 {
				t.Fatal("preparation created a child job")
			}
		})
	}
}

func TestRuntimeChildProducerSignsCompleteAllocationBeforeFirstQueuedJob(t *testing.T) {
	for _, mode := range []string{"task", "parallel_tasks", "case_task", "case_parallel_tasks"} {
		t.Run(mode, func(t *testing.T) {
			tool := strings.TrimPrefix(mode, "case_")
			arguments := json.RawMessage(`{"prompt":"inspect","profile":"reviewer"}`)
			if tool == "parallel_tasks" {
				arguments = json.RawMessage(`{"tasks":[{"id":"first","prompt":"inspect first","profile":"reviewer"},{"id":"second","prompt":"inspect second","profile":"reviewer"}]}`)
			}
			var caseRoot []string
			if mode != tool {
				caseRoot = []string{t.TempDir()}
				arguments = json.RawMessage(strings.ReplaceAll(string(arguments), "inspect", "inspect acct:1"))
			}
			h, pending := runtimeChildProducerPendingFixtureV1(t, tool, arguments, caseRoot...)
			var beforeCase map[string]string
			if len(caseRoot) == 1 {
				beforeCase = runtimeRestoreFileDigestsV1(t, caseRoot[0])
			}
			beforeDurable, beforeData := runtimeRestoreFileDigestsV1(t, h.store.root), runtimeRestoreFileDigestsV1(t, h.dataDir)
			issuedAt := time.Now().UTC()
			prepared, err := h.prepareRuntimeSideEffect(context.Background(), pending, issuedAt)
			if err != nil || prepared.Rejection != nil || prepared.ChildProducer == nil {
				t.Fatalf("complete host preparation failed: %v rejection=%#v", err, prepared.Rejection)
			}
			if !reflect.DeepEqual(beforeDurable, runtimeRestoreFileDigestsV1(t, h.store.root)) || !reflect.DeepEqual(beforeData, runtimeRestoreFileDigestsV1(t, h.dataDir)) {
				t.Fatal("host allocation wrote before receipt")
			}
			request := pendingworkapp.SideEffectIntentRequest{Pending: pending, IssuedAt: issuedAt, SemanticIdentity: prepared.SemanticIdentity, ChildProducer: prepared.ChildProducer}
			lease, err := h.pendingWork.BeginSideEffectIntent(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			inventory, err := h.pendingWork.TrustedInventoryV1(context.Background())
			if err != nil || len(inventory.Receipts) != 1 {
				t.Fatalf("complete signed child readback failed: %v", err)
			}
			if len(caseRoot) == 1 && !reflect.DeepEqual(beforeCase, runtimeRestoreFileDigestsV1(t, caseRoot[0])) {
				t.Fatal("case continuity wrote before complete receipt readback")
			}
			expected := 1
			if tool == "parallel_tasks" {
				expected = 2
			}
			vector := inventory.Receipts[0].ChildProducer
			if vector == nil || len(vector.Children) != expected || len(h.jobs.AllRecords()) != 0 {
				t.Fatal("receipt-before-first-job denominator is incomplete")
			}
			if err := h.pendingWork.VerifySideEffectIntentAtSend(context.Background(), lease, request, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			ctx, err := h.pendingWork.BindChildProducerExecutionV1(context.Background(), lease, request)
			if err != nil {
				t.Fatal(err)
			}
			ctx = prepared.Bind(ctx)
			plan := ctx.Value(runtimeChildProducerPlanKeyV1{}).(*runtimeChildProducerPlanV1)
			// Partial-parallel crash prefix: only the last ordinal writes a job;
			// every missing sibling already has its original signed identity.
			slot := plan.children[expected-1]
			preparation, err := h.admitRuntimeChildProducerV1(ctx, pending, slot.request)
			if err != nil {
				t.Fatal(err)
			}
			defer preparation.ReleaseSourceLock()
			preparation.Request.ParallelIndex = expected
			selected, err := h.childProducerForRunV1(ctx, pending, preparation, slot.request)
			if err != nil || selected != slot {
				t.Fatalf("original ordinal selection failed: %v", err)
			}
			binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
			if err != nil {
				t.Fatal(err)
			}
			record, err := subagentapp.StartPreparedTaskRun(subagentapp.PreparedTaskRunStartInput{Preparation: preparation, Pending: pending, Security: binding, Settings: h.subagents, StartChildRun: func(start domainjob.StartRequest) (domainjob.Record, error) {
				return h.startPreparedReservedChildV1(ctx, pending, slot, start)
			}})
			if err != nil {
				t.Fatal(err)
			}
			if record.ID != slot.target.JobID || record.ChildThreadID != slot.target.ChildThreadID || record.ChildTurnID != slot.target.ChildTurnID || record.Status != "queued" {
				t.Fatal("first queued record differs from signed producer")
			}
			if len(caseRoot) == 1 && reflect.DeepEqual(beforeCase, runtimeRestoreFileDigestsV1(t, caseRoot[0])) {
				t.Fatal("admitted case alias predecessor was not committed before queued job")
			}
			after, err := h.pendingWork.TrustedInventoryV1(context.Background())
			if err != nil || !reflect.DeepEqual(inventory, after) {
				t.Fatal("partial job production rewrote original signed denominator")
			}
			if len(h.jobs.AllRecords()) != 1 {
				t.Fatal("missing sibling job was automatically produced")
			}
		})
	}
}

func TestRuntimeChildProducerKeepsLegacySemanticIdentityWithProfilePrompt(t *testing.T) {
	h, pending := runtimeChildProducerPendingFixtureV1(t, "task", json.RawMessage(`{"prompt":"inspect","profile":"reviewer"}`))
	profile := h.subagents.Profiles["reviewer"]
	profile.SystemPrompt = "synthetic custom reviewer instruction"
	h.subagents.Profiles["reviewer"] = profile
	legacy, err := sideeffectidentityapp.ResolveV1(sideeffectidentityapp.Input{ToolName: pending.Call.Name, Arguments: pending.Call.Arguments, WorkspaceRealPath: pending.SecurityContext.WorkspaceRealPath, ResolveTask: func(request subagentapp.TaskRequest) (any, error) {
		return h.resolveRuntimeSubagentSideEffectProjection(context.Background(), pending, request)
	}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := h.prepareRuntimeSideEffect(context.Background(), pending, time.Now().UTC())
	if err != nil || prepared.Rejection != nil {
		t.Fatalf("preparation failed: %v", err)
	}
	if prepared.SemanticIdentity != legacy {
		t.Fatal("new child preparation changed legacy semantic identity and WorkID")
	}
	plan := prepared.Bind(context.Background()).Value(runtimeChildProducerPlanKeyV1{}).(*runtimeChildProducerPlanV1)
	legacyPlan, err := pendingworkapp.NewChildProducerPlanV1(pending, legacy, []domainpendingwork.ChildProducerTargetV1{plan.children[0].target}, plan.children[0].revalidateReservationsV1)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt := time.Now().UTC()
	if _, err := h.pendingWork.BeginSideEffectIntent(context.Background(), pendingworkapp.SideEffectIntentRequest{Pending: pending, IssuedAt: issuedAt, SemanticIdentity: legacy, ChildProducer: &legacyPlan}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pendingWork.BeginSideEffectIntent(context.Background(), pendingworkapp.SideEffectIntentRequest{Pending: pending, IssuedAt: issuedAt, SemanticIdentity: prepared.SemanticIdentity, ChildProducer: prepared.ChildProducer}); !errors.Is(err, pendingworkapp.ErrWorkAlreadyOpen) {
		t.Fatalf("original semantic CAS did not block new child preparation: %v", err)
	}
}

func TestRuntimeChildProducerAcceptsOriginalParallelOrdinalWithReturnFormat(t *testing.T) {
	for _, format := range []string{"evidence", "transcriptRef"} {
		t.Run(format, func(t *testing.T) {
			arguments, _ := json.Marshal(map[string]any{"tasks": []any{map[string]any{"id": "first", "prompt": "inspect first", "profile": "reviewer"}, map[string]any{"id": "second", "prompt": "inspect second", "profile": "reviewer", "dependsOn": []string{"first"}, "returnFormat": format}}})
			h, pending := runtimeChildProducerPendingFixtureV1(t, "parallel_tasks", arguments)
			prepared, err := h.prepareRuntimeSideEffect(context.Background(), pending, time.Now().UTC())
			if err != nil || prepared.Rejection != nil {
				t.Fatalf("preparation failed: %v", err)
			}
			ctx := prepared.Bind(context.Background())
			decoded, err := domainsecurity.DecodeCanonicalJSONObject(arguments)
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := subagentapp.ParallelTaskRequestsFromArgs(decoded)
			if err != nil {
				t.Fatal(err)
			}
			result := subagentapp.RunParallelTasks(ctx, tasks, time.Now().UTC(), func(ctx context.Context, request subagentapp.TaskRequest) subagentapp.RunResult {
				defer close(request.AcquireQueued)
				preparation, err := h.admitRuntimeChildProducerV1(ctx, pending, request)
				if err != nil {
					t.Error(err)
					return subagentapp.RunResult{IsError: true}
				}
				defer preparation.ReleaseSourceLock()
				if _, err := h.childProducerForRunV1(ctx, pending, preparation, request); err != nil {
					t.Errorf("original dependency ordinal rejected: %v", err)
					return subagentapp.RunResult{IsError: true}
				}
				return subagentapp.RunResult{Output: map[string]any{"summary": "synthetic dependency result"}}
			})
			if result.IsError {
				t.Fatal("valid parallel dependency failed")
			}
		})
	}
}

func TestRuntimeChildProducerRejectsUnwitnessedDependencyPrompt(t *testing.T) {
	h, pending := runtimeChildProducerPendingFixtureV1(t, "parallel_tasks", json.RawMessage(`{"tasks":[{"id":"first","prompt":"inspect first","profile":"reviewer"},{"id":"second","prompt":"inspect second","profile":"reviewer","dependsOn":["first"]}]}`))
	prepared, err := h.prepareRuntimeSideEffect(context.Background(), pending, time.Now().UTC())
	if err != nil || prepared.Rejection != nil {
		t.Fatalf("preparation failed: %v", err)
	}
	ctx := prepared.Bind(context.Background())
	plan := ctx.Value(runtimeChildProducerPlanKeyV1{}).(*runtimeChildProducerPlanV1)
	request := plan.children[1].request
	request.ParallelIndex = 2
	request.Prompt += "\n\nDependency results:\ncaller supplied text"
	preparation, err := h.admitRuntimeChildProducerV1(ctx, pending, request)
	if err != nil {
		t.Fatal(err)
	}
	defer preparation.ReleaseSourceLock()
	if _, err := h.childProducerForRunV1(ctx, pending, preparation, request); err == nil {
		t.Fatal("text prefix forged the host dependency expansion")
	}
}

func runtimePreparedChildExecutionFixtureV1(t *testing.T, background ...bool) (*runtimeServerHandler, runtimePendingToolCall, context.Context, *runtimeChildProducerSlotV1, *providerStepRecordingProvider, func()) {
	t.Helper()
	arguments := json.RawMessage(`{"prompt":"inspect","profile":"reviewer"}`)
	if len(background) == 1 && background[0] {
		arguments = json.RawMessage(`{"prompt":"inspect","profile":"reviewer","run_in_background":true}`)
	}
	h, pending := runtimeChildProducerPendingFixtureV1(t, "task", arguments)
	configureServerGeneralExecution(t, h)
	if err := h.runtimeSubagentState().ObserveSecurityContextAndCancelInvalidatedJobs(context.Background(), pending.SecurityContext, time.Second); err != nil {
		t.Fatal(err)
	}
	parentEffect, releaseParent, err := h.runtimeSubagentState().AcquireOrdinaryContextEffect(context.Background(), pending.SecurityContext)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseParent)
	providerClient := &providerStepRecordingProvider{}
	h.provider = providerClient
	issuedAt := time.Now().UTC()
	prepared, err := h.prepareRuntimeSideEffect(context.Background(), pending, issuedAt)
	if err != nil || prepared.Rejection != nil {
		t.Fatalf("prepare: %v", err)
	}
	intent := pendingworkapp.SideEffectIntentRequest{Pending: pending, IssuedAt: issuedAt, SemanticIdentity: prepared.SemanticIdentity, ChildProducer: prepared.ChildProducer}
	lease, err := h.pendingWork.BeginSideEffectIntent(context.Background(), intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.pendingWork.VerifySideEffectIntentAtSend(context.Background(), lease, intent, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ctx, err := h.pendingWork.BindChildProducerExecutionV1(parentEffect, lease, intent)
	if err != nil {
		t.Fatal(err)
	}
	ctx = prepared.Bind(ctx)
	slot := ctx.Value(runtimeChildProducerPlanKeyV1{}).(*runtimeChildProducerPlanV1).children[0]
	return h, pending, ctx, slot, providerClient, releaseParent
}

func TestRuntimeChildReservedTurnAllowsPendingSteerBeforeStart(t *testing.T) {
	for _, status := range []string{"queued", "running"} {
		t.Run(status, func(t *testing.T) {
			h, pending, ctx, slot, providerClient, releaseParent := runtimePreparedChildExecutionFixtureV1(t, true)
			preparation, err := h.admitRuntimeChildProducerV1(ctx, pending, slot.request)
			if err != nil {
				t.Fatal(err)
			}
			defer preparation.ReleaseSourceLock()
			binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
			if err != nil {
				t.Fatal(err)
			}
			record, err := subagentapp.StartPreparedTaskRun(subagentapp.PreparedTaskRunStartInput{Preparation: preparation, Pending: pending, Security: binding, Settings: h.subagents, StartChildRun: func(start domainjob.StartRequest) (domainjob.Record, error) {
				return h.startPreparedReservedChildV1(ctx, pending, slot, start)
			}})
			if err != nil {
				t.Fatal(err)
			}
			control, executionCtx, err := subagentapp.BeginBoundChildAdmission(ctx, true, h.runtimeSubagentState(), record.ID, record.SecurityBinding)
			if err != nil {
				t.Fatal(err)
			}
			defer control.Close()
			executionCtx = context.WithValue(executionCtx, runtimeChildProducerSlotKeyV1{}, slot)
			execution := preparation.Execution
			threadID, err := h.prepareRuntimeSubagentThread(executionCtx, pending, preparation.Request, preparation.Source, preparation.HasSource, execution.ProviderID, execution.Model, execution.EndpointFormat, execution.Effort, preparation.Workspace)
			if err != nil {
				t.Fatal(err)
			}
			if status == "running" {
				record, err = h.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: status})
				if err != nil {
					t.Fatal(err)
				}
			}
			before := runtimeRestoreFileDigestsV1(t, h.store.root)
			result := subagentapp.SteerRuntimeTaskJob(subagentapp.TaskJobSteerRuntimeDeps{Context: ctx, Jobs: h.jobs, Turns: h.store, BeginAuthority: h.beginRuntimeTaskJobSteerAuthority}, pending.ThreadID, subagentapp.TaskJobSteerRequest{JobID: record.ID, Message: "focus on the second check", ClientMessageID: "f47ac10b-58cc-4372-a567-0e02b2c3d479"}, time.Now().UTC())
			if result.IsError {
				t.Fatalf("signed reserved first turn rejected pending steer: %v", result.Err)
			}
			current, err := h.jobs.LoadChildRun(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.ChildTurnID != slot.target.ChildTurnID || current.ChildThreadID != threadID || len(current.Steers) != 1 || current.Steers[0].ContextDigest != "" || current.Steers[0].Status != "queued" {
				t.Fatal("pending steer changed reserved identity or gained turn authority")
			}
			if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, h.store.root)) || len(providerClient.Requests()) != 0 {
				t.Fatal("pending steer mutated the uncommitted turn or called Provider")
			}
			// Release the synthetic parent dispatch lease just as the real
			// background task tool does before its detached child executes.
			releaseParent()
			if status == "queued" {
				record, err = h.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: "running"})
				if err != nil {
					t.Fatal(err)
				}
			}
			// In the running case, record intentionally precedes the queue write.
			// The host claim may advance only through that exact successful queue.
			observation := &runtimeChildSteerStartObservationV1{authority: control}
			completed := h.completeRuntimeSubagentTask(executionCtx, pending, preparation.Request, record, execution.ProviderID, execution.Model, execution.Effort, record.ToolScope, observation)
			if completed.IsError {
				t.Fatalf("pending steer prevented reserved child execution: %s", domainjob.ProjectPersistableUntrustedOutputV1(fmt.Sprint(observation.err)))
			}
			requests := providerClient.Requests()
			if len(requests) != 1 {
				t.Fatalf("pending child made %d synthetic Provider requests", len(requests))
			}
			body, err := json.Marshal(requests[0].Messages)
			if err != nil || !strings.Contains(string(body), "focus on the second check") {
				t.Fatal("queued guidance did not reach the first synthetic Provider request")
			}
			current, err = h.jobs.LoadChildRun(record.ID)
			if err != nil || len(current.Steers) != 1 || current.Steers[0].Status != "admitted" || current.Steers[0].Text != "focus on the second check" || current.ChildTurnID != slot.target.ChildTurnID {
				t.Fatal("first turn lost or rewrote the queued guidance")
			}

		})
	}
}

func TestRuntimeChildProducerConsumesExactFirstTurnWithoutNewAllocation(t *testing.T) {
	h, pending, ctx, slot, providerClient, _ := runtimePreparedChildExecutionFixtureV1(t)
	reservedSequence := h.turnSeq
	result := h.runRuntimeSubagentTask(ctx, pending, slot.request)
	if result.IsError {
		t.Fatalf("synthetic child execution failed: %s", domainjob.ProjectPersistableUntrustedOutputV1(result.Record.Error))
	}
	if result.Record.ID != slot.target.JobID || result.Record.ChildTurnID != slot.target.ChildTurnID || result.Record.ChildThreadID != slot.target.ChildThreadID || h.turnSeq != reservedSequence {
		t.Fatal("first child execution minted or substituted a turn identity")
	}
	thread, err := h.store.GetThread(slot.target.ChildThreadID)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := turnapp.FrozenSecurityContextForTurn(thread, slot.target.ChildTurnID)
	if err != nil || frozen.ThreadID != slot.target.ChildThreadID || frozen.TurnID != slot.target.ChildTurnID || len(thread["turns"].([]any)) != 1 || len(providerClient.Requests()) != 1 {
		t.Fatalf("reserved first turn was not the single committed execution: %v", err)
	}
	if err := h.revalidateRuntimeChildTurnV1(ctx, slot.turn); err == nil {
		t.Fatal("consumed first-turn reservation remains reusable")
	}
	before := runtimeRestoreFileDigestsV1(t, h.store.root)
	if replay := h.runRuntimeSubagentTask(ctx, pending, slot.request); !replay.IsError {
		t.Fatal("copied process context repeated a child execution")
	}
	if !reflect.DeepEqual(before, runtimeRestoreFileDigestsV1(t, h.store.root)) || len(providerClient.Requests()) != 1 {
		t.Fatal("rejected replay changed child state or called Provider")
	}
}

func TestRuntimeChildProducerDirectFirstTurnAdmission(t *testing.T) {
	h, pending, ctx, slot, providerClient, _ := runtimePreparedChildExecutionFixtureV1(t)
	preparation, err := h.admitRuntimeChildProducerV1(ctx, pending, slot.request)
	if err != nil {
		t.Fatal(err)
	}
	defer preparation.ReleaseSourceLock()
	binding, err := domainjob.NewSecurityBinding(pending.SecurityContext, pending.ExecutionGrant, pending.Call.ID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := subagentapp.StartPreparedTaskRun(subagentapp.PreparedTaskRunStartInput{Preparation: preparation, Pending: pending, Security: binding, Settings: h.subagents, StartChildRun: func(start domainjob.StartRequest) (domainjob.Record, error) {
		return h.startPreparedReservedChildV1(ctx, pending, slot, start)
	}})
	if err != nil {
		t.Fatal(err)
	}
	control, executionCtx, err := subagentapp.BeginBoundChildAdmission(ctx, false, h.runtimeSubagentState(), record.ID, record.SecurityBinding)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	executionCtx = context.WithValue(executionCtx, runtimeChildProducerSlotKeyV1{}, slot)
	execution := preparation.Execution
	threadID, err := h.prepareRuntimeSubagentThread(executionCtx, pending, preparation.Request, preparation.Source, preparation.HasSource, execution.ProviderID, execution.Model, execution.EndpointFormat, execution.Effort, preparation.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	record, err = h.jobs.UpdateChildRun(record.ID, domainjob.UpdateRequest{Status: "running", ChildThreadID: threadID})
	if err != nil {
		t.Fatal(err)
	}
	record, err = h.authorizeRuntimeJobStart(record)
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	err = control.StartIfActive(executionCtx, func() error {
		var err error
		response, err = h.startRuntimeTurn(executionCtx, threadID, startRuntimeTurnRequest{Prompt: record.Prompt, ProviderID: execution.ProviderID, Model: execution.Model, ReasoningEffort: execution.Effort, InternalToolScope: record.ToolScope, InternalSubagentDepth: 1, InternalChildRunID: record.ID, DisableUserInput: true, DisableUserInputSet: true})
		return err
	})
	if err != nil {
		t.Fatalf("direct synthetic first-turn admission: %s", domainjob.ProjectPersistableUntrustedOutputV1(err.Error()))
	}
	if response["turnId"] != slot.target.ChildTurnID || len(providerClient.Requests()) != 1 {
		t.Fatal("direct first turn did not execute the reserved identity exactly once")
	}
}

// Test-only observation keeps the real CompleteTask/StartTurn call path while
// exposing its synthetic fixture error without relaxing bound-job projection.
type runtimeChildSteerStartObservationV1 struct {
	authority subagentapp.ChildTurnStartAuthority
	err       error
}

func (observation *runtimeChildSteerStartObservationV1) StartIfActive(ctx context.Context, start func() error) error {
	observation.err = observation.authority.StartIfActive(ctx, start)
	return observation.err
}
