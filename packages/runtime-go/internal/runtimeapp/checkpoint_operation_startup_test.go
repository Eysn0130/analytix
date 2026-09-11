package runtimeapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	eventlog "analytix.local/runtime-go/internal/adapters/outbound/eventlog"
	checkpointapp "analytix.local/runtime-go/internal/app/checkpoint"
	domaincheckpointref "analytix.local/runtime-go/internal/domain/checkpointref"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
	"analytix.local/runtime-go/internal/server"
	securitytest "analytix.local/runtime-go/internal/testsupport/securitycontext"
)

type checkpointRecoveryMemoryEvents struct {
	events            map[string][]map[string]any
	threads           map[string]map[string]any
	diagnostics       []eventlog.JSONLDiagnostic
	recordErr         error
	commitBeforeError bool
	dropWrite         bool
	recordCalls       int
}

func (store *checkpointRecoveryMemoryEvents) AllThreadIDs() ([]string, error) {
	ids := make([]string, 0, len(store.threads))
	for threadID := range store.threads {
		ids = append(ids, threadID)
	}
	sort.Strings(ids)
	return ids, nil
}

func (store *checkpointRecoveryMemoryEvents) GetThread(threadID string) (map[string]any, error) {
	return store.threads[threadID], nil
}

func (store *checkpointRecoveryMemoryEvents) LoadEventsSince(threadID string, _ int) (server.DurableLoadEventsResult, error) {
	return server.DurableLoadEventsResult{
		Events:      append([]map[string]any(nil), store.events[threadID]...),
		Diagnostics: append([]eventlog.JSONLDiagnostic(nil), store.diagnostics...),
	}, nil
}

func (store *checkpointRecoveryMemoryEvents) ReconcileCheckpointCapturedEventForStartup(event map[string]any, allowWrite bool) error {
	checkpoint, _ := event["checkpoint"].(map[string]any)
	eventID, _ := checkpoint["captureEventId"].(string)
	if !domaincheckpointref.IsCaptureEventIDV2(eventID) || !domaincheckpointref.CapturedPayloadDigestMatches(checkpoint) {
		return errors.New("invalid checkpoint capture event")
	}
	threadID, _ := event["threadId"].(string)
	count := 0
	for _, existing := range store.events[threadID] {
		existingCheckpoint, _ := existing["checkpoint"].(map[string]any)
		if existingCheckpoint == nil || existingCheckpoint["captureEventId"] != eventID {
			continue
		}
		if !reflect.DeepEqual(existing, event) {
			return errors.New("checkpoint capture event payload conflict")
		}
		count++
	}
	if count > 1 {
		return errors.New("duplicate checkpoint capture event")
	}
	if count == 1 {
		return nil
	}
	if !allowWrite {
		return errors.New("checkpoint capture event is missing")
	}
	store.recordCalls++
	if !store.dropWrite && (store.recordErr == nil || store.commitBeforeError) {
		store.events[threadID] = append(store.events[threadID], event)
	}
	if len(store.events[threadID]) != 0 {
		last := store.events[threadID][len(store.events[threadID])-1]
		if reflect.DeepEqual(last, event) {
			return nil
		}
	}
	if store.recordErr != nil {
		return store.recordErr
	}
	return errors.New("checkpoint capture event write was not committed")
}

type checkpointRecoveryCaseAuthorityStub struct {
	caseThreads map[string]bool
	contexts    map[string]bool
}

func (authority checkpointRecoveryCaseAuthorityStub) IsCaseThread(threadID string) bool {
	return authority.caseThreads[threadID]
}

func (authority checkpointRecoveryCaseAuthorityStub) ContainsContext(securityContext domainsecurity.TurnSecurityContext) bool {
	return authority.contexts[securityContext.ContextDigest]
}

func newCheckpointRecoveryMemoryEvents(t *testing.T, securityContext domainsecurity.TurnSecurityContext) *checkpointRecoveryMemoryEvents {
	t.Helper()
	body, err := json.Marshal(securityContext)
	if err != nil {
		t.Fatal(err)
	}
	contextRecord := map[string]any{}
	if err := json.Unmarshal(body, &contextRecord); err != nil {
		t.Fatal(err)
	}
	return &checkpointRecoveryMemoryEvents{
		events: map[string][]map[string]any{},
		threads: map[string]map[string]any{
			securityContext.ThreadID: {
				"id": securityContext.ThreadID,
				"turns": []any{map[string]any{
					"id": securityContext.TurnID, "threadId": securityContext.ThreadID,
					"securityContext": contextRecord,
				}},
			},
		},
	}
}

func TestCheckpointOperationStartupRecoveryRepairsCapturedAuditOnlyInStage(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, path, "before")
	now := time.Unix(1_700_100_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	securityContext := beginStartupWriteOperation(t, ctx, service, now, workspace, path, "startup-audit")
	writeOperationFixture(t, path, "after")
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, false, authority, events, nil); err == nil {
		t.Fatal("read-only activation repaired a missing checkpoint audit")
	}
	if events.recordCalls != 0 {
		t.Fatalf("read-only activation wrote persistence: calls=%d", events.recordCalls)
	}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, nil); err != nil {
		t.Fatal(err)
	}
	if events.recordCalls != 1 || len(events.events[securityContext.ThreadID]) != 1 {
		t.Fatalf("stage did not repair one captured audit: calls=%d events=%#v", events.recordCalls, events.events)
	}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, false, authority, events, nil); err != nil {
		t.Fatal(err)
	}
	if events.recordCalls != 1 {
		t.Fatalf("activation rewrote an existing captured audit: calls=%d", events.recordCalls)
	}
}

func TestCheckpointOperationStartupNoEffectHasNoCaptureEvent(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, path, "before")
	now := time.Unix(1_700_200_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	securityContext := beginStartupWriteOperation(t, ctx, service, now, workspace, path, "startup-no-effect")
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, nil); err != nil {
		t.Fatal(err)
	}
	if events.recordCalls != 0 {
		t.Fatalf("no-effect operation emitted a capture event: calls=%d", events.recordCalls)
	}
	states, err := authority.OperationGroups(ctx)
	if err != nil || len(states) != 1 || states[0].Terminal == nil || states[0].Terminal.Status != "no_effect" {
		t.Fatalf("operation was not recovered as no-effect: %#v err=%v", states, err)
	}
}

func TestCheckpointOperationStartupQuarantineIsStickyAndImmediatelyBlocking(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	source := filepath.Join(workspace, "a.txt")
	destination := filepath.Join(workspace, "b.txt")
	writeOperationFixture(t, source, "move")
	now := time.Unix(1_700_300_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	arguments := []byte(`{"source_path":"a.txt","destination_path":"b.txt"}`)
	securityContext, grant := operationRecoverySecurity(t, now, workspace, "move_file", arguments)
	_, err := service.Begin(ctx, checkpointapp.BeginOperationInput{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("startup-mixed"), SourceWorkspaceCheckpointID: "startup-mixed",
		Workspace: workspace, ToolName: "move_file", ArgumentsJSON: arguments,
		Paths: []checkpointapp.OperationPathRequest{
			{ResolvedPath: source, ArgumentKey: "source_path", RequestedPath: "a.txt", Role: "source"},
			{ResolvedPath: destination, ArgumentKey: "destination_path", RequestedPath: "b.txt", Role: "destination", ExpectedAfterExisted: true},
		},
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); !errors.Is(err, checkpointapp.ErrOperationRecoveryQuarantined) {
		t.Fatalf("mixed live recovery did not write a sticky quarantine: %v", err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	for _, allowWrite := range []bool{true, false} {
		if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, allowWrite, authority, events, nil); err == nil {
			t.Fatalf("quarantined inventory was accepted: allowWrite=%v", allowWrite)
		}
	}
	if events.recordCalls != 0 {
		t.Fatalf("quarantined operation emitted an audit: calls=%d", events.recordCalls)
	}
}

func TestCheckpointOperationStartupAcceptsCommittedAtomicAppendReadback(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, path, "before")
	now := time.Unix(1_700_400_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	securityContext := beginStartupWriteOperation(t, ctx, service, now, workspace, path, "startup-readback")
	writeOperationFixture(t, path, "after")
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	events.recordErr = errors.New("append acknowledgement lost")
	events.commitBeforeError = true
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, nil); err != nil {
		t.Fatalf("committed append readback was not reconciled: %v", err)
	}
	if len(events.events[securityContext.ThreadID]) != 1 {
		t.Fatalf("committed append was not retained exactly once: %#v", events.events)
	}
}

func TestCheckpointOperationStartupRejectsUncommittedAppendReadback(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, path, "before")
	now := time.Unix(1_700_450_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	securityContext := beginStartupWriteOperation(t, ctx, service, now, workspace, path, "startup-missing-readback")
	writeOperationFixture(t, path, "after")
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	events.dropWrite = true
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, nil); err == nil {
		t.Fatal("startup accepted an append that was not present on readback")
	}
	if len(events.events[securityContext.ThreadID]) != 0 {
		t.Fatalf("uncommitted append unexpectedly appeared: %#v", events.events)
	}
}

func TestCheckpointOperationStartupRepairsEveryCompletedPrefixExactlyOnce(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		writeOperationFixture(t, filepath.Join(workspace, name), "before-"+name)
	}
	now := time.Unix(1_700_460_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	checkpointID := domaincheckpointref.RuntimeID("startup-prefixes")
	securityContext := settleStartupWriteOperation(t, ctx, service, now, workspace, checkpointID, "startup-prefixes", "a.txt", "after-a", true)
	settleStartupWriteOperation(t, ctx, service, now, workspace, checkpointID, "startup-prefixes", "b.txt", "after-b", false)
	settleStartupWriteOperation(t, ctx, service, now, workspace, checkpointID, "startup-prefixes", "a.txt", "before-a.txt", true)

	inventory, err := authority.CapturedOperationInventory(ctx, securityContext.ThreadID, checkpointID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Audits) != 2 {
		t.Fatalf("completed prefixes did not produce two audits: %#v", inventory.Audits)
	}
	latestCheckpoint, _ := inventory.Audits[1].Event["checkpoint"].(map[string]any)
	if latestCheckpoint["changedFileCount"] != float64(0) {
		t.Fatalf("net-zero completed prefix was not retained as an authoritative zero-change audit: %#v", latestCheckpoint)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	events.events[securityContext.ThreadID] = append(events.events[securityContext.ThreadID], inventory.Audits[0].Event)
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, nil); err != nil {
		t.Fatal(err)
	}
	if events.recordCalls != 1 || len(events.events[securityContext.ThreadID]) != 2 {
		t.Fatalf("startup did not repair only the missing prefix: calls=%d events=%#v", events.recordCalls, events.events)
	}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, false, authority, events, nil); err != nil {
		t.Fatal(err)
	}
	if events.recordCalls != 1 {
		t.Fatalf("read-only activation rewrote completed prefixes: calls=%d", events.recordCalls)
	}
}

func TestCheckpointOperationStartupPreservesGeneralCaptureBeforeCaseLineage(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, path, "before")
	now := time.Unix(1_700_465_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	frozen := beginStartupWriteOperation(t, ctx, service, now, workspace, path, "general-before-case")
	if !domainsecurity.TurnSecurityContextIsGeneral(frozen) {
		t.Fatal("fixture did not authenticate an ordinary checkpoint context")
	}
	writeOperationFixture(t, path, "after")
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, frozen)
	// A later protected turn registers the thread, but cannot retroactively
	// register an earlier GENERAL context as private case authority.
	laterCase := checkpointRecoveryCaseAuthorityStub{
		caseThreads: map[string]bool{frozen.ThreadID: true}, contexts: map[string]bool{},
	}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, laterCase); err != nil {
		t.Fatalf("authenticated earlier ordinary capture could not recover: %v", err)
	}
	if events.recordCalls != 1 {
		t.Fatal("ordinary checkpoint audit was not reconciled exactly once")
	}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, false, authority, events, laterCase); err != nil {
		t.Fatal(err)
	}
	turn := events.threads[frozen.ThreadID]["turns"].([]any)[0].(map[string]any)
	turn["securityContext"].(map[string]any)["contextEpoch"] = float64(frozen.ContextEpoch + 1)
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, false, authority, events, laterCase); err == nil {
		t.Fatal("later case lineage admitted a changed frozen checkpoint context")
	}
}

func TestCheckpointOperationStartupRequiresCommittedCaseAuthority(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, path, "before")
	now := time.Unix(1_700_470_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	arguments := []byte(`{"path":"a.txt","content":"after"}`)
	workspaceRealPath, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	securityContext, err := securitytest.CaseExecutionContextV2(domainsecurity.TurnSecurityContextInput{
		ThreadID: "thr-case-recovery", TurnID: "turn-case-recovery", WorkspaceRealPath: workspaceRealPath,
		TenantID: domainsecurity.LocalTenantID, UserID: domainsecurity.LocalUserID, ContextEpoch: 1, IssuedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	scope, _ := json.Marshal([]string{"write_file"})
	grant := domainsecurity.NewExecutionGrant(domainsecurity.ExecutionGrantInput{
		Context: securityContext, Provider: "provider", ServerIdentity: "host:builtin", ToolName: "write_file", ToolCallID: runtimeCheckpointTestHostToolCallID(t, "call-case-write"),
		ArgsHash: domainsecurity.CanonicalJSONHash(arguments), SchemaHash: domainsecurity.SHA256Hex([]byte("schema")),
		ScopeHash: domainsecurity.SHA256Hex(scope), ApprovalState: "approved", IssuedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	})
	_, err = service.Begin(ctx, checkpointapp.BeginOperationInput{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID("startup-case"), SourceWorkspaceCheckpointID: "startup-case",
		Workspace: workspace, ToolName: "write_file", ArgumentsJSON: arguments,
		Paths: []checkpointapp.OperationPathRequest{{
			ResolvedPath: path, ArgumentKey: "path", RequestedPath: "a.txt", Role: "target",
			ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash("after"),
		}},
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeOperationFixture(t, path, "after")
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	missing := checkpointRecoveryCaseAuthorityStub{caseThreads: map[string]bool{securityContext.ThreadID: true}, contexts: map[string]bool{}}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, missing); err == nil {
		t.Fatal("case checkpoint audit was accepted without committed private context authority")
	}
	committed := checkpointRecoveryCaseAuthorityStub{
		caseThreads: map[string]bool{securityContext.ThreadID: true},
		contexts:    map[string]bool{securityContext.ContextDigest: true},
	}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, committed); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointOperationStartupRejectsMissingTurnAndUnknownMarker(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	path := filepath.Join(workspace, "a.txt")
	writeOperationFixture(t, path, "before")
	now := time.Unix(1_700_500_000, 0).UTC()
	authority, service := operationRecoveryService(t, ctx)
	securityContext := beginStartupWriteOperation(t, ctx, service, now, workspace, path, "startup-invalid")
	writeOperationFixture(t, path, "after")
	if _, err := service.RecoverOpen(ctx, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	events := newCheckpointRecoveryMemoryEvents(t, securityContext)
	events.threads[securityContext.ThreadID]["turns"] = []any{}
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, nil); err == nil {
		t.Fatal("checkpoint operation without a durable frozen turn was accepted")
	}
	events = newCheckpointRecoveryMemoryEvents(t, securityContext)
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, true, authority, events, nil); err != nil {
		t.Fatal(err)
	}
	unknown := map[string]any{
		"schemaVersion": float64(1), "captureEventId": domaincheckpointref.CaptureEventID(domainsecurity.SHA256Hex([]byte("unknown"))),
		"changedFileCount": float64(1),
	}
	unknown["capturePayloadDigest"] = domaincheckpointref.CapturedPayloadDigest(unknown)
	events.events[securityContext.ThreadID] = append(events.events[securityContext.ThreadID], map[string]any{
		"kind": "checkpoint_captured", "threadId": securityContext.ThreadID, "turnId": securityContext.TurnID,
		"checkpoint": unknown,
	})
	if err := reconcileCheckpointOperationAuditsBeforeActivation(ctx, false, authority, events, nil); err == nil {
		t.Fatal("unknown checkpoint capture marker was accepted")
	}
}

func beginStartupWriteOperation(
	t *testing.T,
	ctx context.Context,
	service checkpointapp.OperationService,
	now time.Time,
	workspace string,
	path string,
	sourceCheckpointID string,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	arguments := []byte(`{"path":"a.txt","content":"after"}`)
	securityContext, grant := operationRecoverySecurity(t, now, workspace, "write_file", arguments)
	_, err := service.Begin(ctx, checkpointapp.BeginOperationInput{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: domaincheckpointref.RuntimeID(sourceCheckpointID), SourceWorkspaceCheckpointID: sourceCheckpointID,
		Workspace: workspace, ToolName: "write_file", ArgumentsJSON: arguments,
		Paths: []checkpointapp.OperationPathRequest{{
			ResolvedPath: path, ArgumentKey: "path", RequestedPath: "a.txt", Role: "target",
			ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash("after"),
		}},
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return securityContext
}

func settleStartupWriteOperation(
	t *testing.T,
	ctx context.Context,
	service checkpointapp.OperationService,
	now time.Time,
	workspace string,
	checkpointID string,
	sourceCheckpointID string,
	name string,
	after string,
	mutationSucceeded bool,
) domainsecurity.TurnSecurityContext {
	t.Helper()
	arguments := []byte(`{"path":"` + name + `","content":"` + after + `"}`)
	securityContext, grant := operationRecoverySecurity(t, now, workspace, "write_file", arguments)
	path := filepath.Join(workspace, name)
	draft, err := service.Begin(ctx, checkpointapp.BeginOperationInput{
		SecurityContext: securityContext, ExecutionGrant: grant,
		CheckpointID: checkpointID, SourceWorkspaceCheckpointID: sourceCheckpointID,
		Workspace: workspace, ToolName: "write_file", ArgumentsJSON: arguments,
		Paths: []checkpointapp.OperationPathRequest{{
			ResolvedPath: path, ArgumentKey: "path", RequestedPath: name, Role: "target",
			ExpectedAfterExisted: true, ExpectedAfterHash: checkpointapp.Hash(after),
		}},
		CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutationSucceeded {
		writeOperationFixture(t, path, after)
	}
	terminal, err := service.Settle(ctx, draft, mutationSucceeded, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	wantStatus := "completed"
	if !mutationSucceeded {
		wantStatus = "no_effect"
	}
	if terminal.Status != wantStatus {
		t.Fatalf("unexpected startup operation terminal: got=%s want=%s", terminal.Status, wantStatus)
	}
	return securityContext
}
